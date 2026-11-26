package textutil

import (
	"strings"
	"testing"
)

func TestNormalizeLabelKey(t *testing.T) {
	cases := map[string]string{
		"SK海力士":        "sk海力士",
		"SK 海力士":       "sk海力士",
		"  SK   海力士  ": "sk海力士",
		"SK\t海力士":      "sk海力士",
		"DeepSeek":     "deepseek",
		"  ":           "",
		"":             "",
	}
	for input, want := range cases {
		if got := NormalizeLabelKey(input); got != want {
			t.Errorf("NormalizeLabelKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestStripURLFragment(t *testing.T) {
	cases := map[string]string{
		// Plain link untouched.
		"https://example.com/a": "https://example.com/a",
		// Fragment stripped (the V2EX reply-anchor drift case).
		"https://www.v2ex.com/t/1#reply2": "https://www.v2ex.com/t/1",
		// Query kept, fragment stripped.
		"https://example.com/a?x=1#top": "https://example.com/a?x=1",
		// Trailing bare '#' stripped.
		"https://example.com/a#": "https://example.com/a",
		// Hashbang fragments carry SPA route identity: preserved verbatim.
		"https://example.com/#!/article/1": "https://example.com/#!/article/1",
		"https://example.com/p#!k=v":       "https://example.com/p#!k=v",
		// Empty stays empty.
		"": "",
	}
	for input, want := range cases {
		if got := StripURLFragment(input); got != want {
			t.Errorf("StripURLFragment(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeArticleLink(t *testing.T) {
	cases := map[string]string{
		// Empty / blank input → empty key (content identity undecidable).
		"":     "",
		"   ":  "",
		"\t\n": "",
		// A pure fragment leaves nothing behind.
		"#":       "",
		"#anchor": "",
		// Query differences vanish: same content, same key.
		"https://example.com/p/1":           "https://example.com/p/1",
		"https://example.com/p/1?utm=x":     "https://example.com/p/1",
		"https://example.com/p/1?utm=x&y=2": "https://example.com/p/1",
		"https://example.com/p/1?":          "https://example.com/p/1",
		// Fragment differences vanish; fragment+query combined too.
		"https://example.com/p/1#comments":  "https://example.com/p/1",
		"https://EXAMPLE.com/p/1#comments":  "https://example.com/p/1",
		"https://example.com/p/1?utm=x#top": "https://example.com/p/1",
		// Scheme + host lowercase, path untouched.
		"HTTPS://EXAMPLE.COM":           "https://example.com",
		"https://EXAMPLE.com/Path/Case": "https://example.com/Path/Case",
		"https://example.com/":          "https://example.com/",
		// Hashbang carries SPA route identity: kept verbatim.
		"https://example.com/#!/article/1": "https://example.com/#!/article/1",
		// Surrounding whitespace is trimmed.
		"  https://example.com/p/1  ": "https://example.com/p/1",
		// Non-URL text passes through (trim/fragment/query only).
		"not a url":         "not a url",
		"example.com/p":     "example.com/p",
		"www.example.com/p": "www.example.com/p",
		// Prose containing "://" is not a URL: left alone.
		"see https://example.com/p now": "see https://example.com/p now",
	}
	for input, want := range cases {
		if got := NormalizeArticleLink(input); got != want {
			t.Errorf("NormalizeArticleLink(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestNormalizeArticleLinkLongInput 保证超长链接不被截断、不 panic。
func TestNormalizeArticleLinkLongInput(t *testing.T) {
	const pathLen = 2048
	long := "https://Example.com/" + strings.Repeat("a", pathLen)
	got := NormalizeArticleLink(long + "?utm_source=x")
	want := "https://example.com/" + strings.Repeat("a", pathLen)
	if got != want {
		t.Fatalf("NormalizeArticleLink(long) = %d chars, want %d chars", len(got), len(want))
	}
}

func TestNormalizeSubjectKey(t *testing.T) {
	cases := map[string]string{
		// Compact key passes through.
		"person:trump": "person:trump",
		// Case + trim + space/hyphen → underscore in the name part only.
		" Person:Donald-Trump ": "person:donald_trump",
		"Person:Donald Trump":   "person:donald_trump",
		"COMPANY:Byte\tDance":   "company:byte_dance",
		// Space after the colon is trimmed around the name part, not turned into
		// a leading underscore.
		"company: 字节跳动": "company:字节跳动",
		// Untyped keys: normalized as-is, never given an invented prefix.
		"Byte-Dance Ltd": "byte_dance_ltd",
		"字节跳动":           "字节跳动",
		"":               "",
		"   ":            "",
	}
	for input, want := range cases {
		if got := NormalizeSubjectKey(input); got != want {
			t.Errorf("NormalizeSubjectKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeEntityText(t *testing.T) {
	cases := map[string]string{
		// Same company, mixed scripts/case/suffixes → one literal key.
		"字节跳动":       "字节跳动",
		"字节跳动集团":     "字节跳动",
		"ByteDance":  "bytedance",
		"OpenAI Inc": "openai",
		"OpenAI INC": "openai",
		"ACME Corp":  "acme",
		// Trailing punctuation blocks the suffix trim (only bare suffixes strip).
		"ACME Corp.": "acmecorp.",
		// Full-width fold (NFKC) + full-width space removal.
		"ＯｐｅｎＡＩ":       "openai",
		"ＳＫ 海力士":       "sk海力士",
		"  Apple  公司 ": "apple",
		// Stacked suffixes trimmed repeatedly.
		"某控股集团公司": "某控股",
		// ASCII-suffix guard (add-event-story-heat batch 2b): a plain word that
		// merely ends in a suffix must not be gutted — "zinc" keeps its "inc"
		// because the remainder "z" is under minEntityRemainderRunes.
		"zinc":          "zinc",
		"Zinc Corp":     "zinc",
		"minc":          "minc",
		"ByteDance Inc": "bytedance",
		"Sina Ltd":      "sina",
		// Plain names untouched.
		"特朗普":   "特朗普",
		"Trump": "trump",
		"":      "",
		"   ":   "",
	}
	for input, want := range cases {
		if got := NormalizeEntityText(input); got != want {
			t.Errorf("NormalizeEntityText(%q) = %q, want %q", input, got, want)
		}
	}
}
