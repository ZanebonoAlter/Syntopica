package textutil

import "testing"

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
