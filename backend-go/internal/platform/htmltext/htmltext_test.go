package htmltext

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// zhiyuanSample is a verbatim excerpt of the 智源社区 (RSSHub `baai` route)
// feed item description: the whole article HTML is carried in the summary and
// its root node is marked invisible (`opacity: 0`), which is what produced
// the 8046px blank block in the reader (change fix-reader-lede-blank-space).
const zhiyuanSample = `<div class="rich_media_content js_underline_content defaultNoSetting" id="js_content" style="visibility: visible; opacity: 0; "><h5 style="...">克雷西 发自 凹非寺</h5><p style="...">正文第一段…</p><section style="..."><img loading="lazy" data-src="https://simg.baai.ac.cn/hub-detail/3a8fed6844ebaaaae1a18986c50781fa1790748001235.webp" class="rich_pages wxw-img"></section></div>`

func TestPlainText_RealSampleDropsHiddenSubtree(t *testing.T) {
	got := PlainText(zhiyuanSample)

	require.Empty(t, got, "the whole article sits under an opacity:0 root, so no text is visible")
	require.NotContains(t, got, "正文第一段")
	require.NotContains(t, got, "opacity")
}

func TestPlainText_RemovesHiddenSubtrees(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"display none", `<div style="display:none">hidden text</div><p>visible</p>`, "visible"},
		{"visibility hidden", `<div style="visibility: hidden">hidden text</div><p>visible</p>`, "visible"},
		{"opacity zero", `<div style="opacity: 0">hidden text</div><p>visible</p>`, "visible"},
		{"opacity zero decimal", `<div style="opacity:0.0">hidden text</div><p>visible</p>`, "visible"},
		{"font size zero", `<div style="font-size:0">hidden text</div><p>visible</p>`, "visible"},
		{"hidden attribute", `<div hidden>hidden text</div><p>visible</p>`, "visible"},
		{"hidden ancestor drops descendants", `<div style="opacity:0"><p>deep hidden</p></div><p>visible</p>`, "visible"},
		{"visible style keeps text", `<div style="color:red;font-size:16px">visible text</div>`, "visible text"},
		{"script content removed", `<script>alert(1)</script><p>visible</p>`, "visible"},
		{"style content removed", `<style>p{color:red}</style><p>visible</p>`, "visible"},
		{"script inside hidden subtree", `<div style="display:none"><script>alert(1)</script></div><p>visible</p>`, "visible"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, PlainText(tc.raw))
		})
	}
}

func TestPlainText_ResolvesEntities(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"ampersand", "a&amp;b", "a&b"},
		{"less than", "a&lt;b", "a<b"},
		{"greater than", "a&gt;b", "a>b"},
		{"quot", "a&quot;b", `a"b`},
		{"apostrophe", "a&#39;b", "a'b"},
		{"nbsp becomes space", "a&nbsp;b", "a b"},
		{"decimal entity", "&#20013;&#25991;", "中文"},
		{"hex entity", "&#x4E2D;&#x6587;", "中文"},
		{"single pass only", "a&amp;lt;b", "a&lt;b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, PlainText(tc.raw))
		})
	}
}

// sharedPlainTextSamples is the cross-language drift anchor: the frontend
// implements the same cleaning in TypeScript (front/app/utils/htmlSanitize.ts)
// and asserts the very same expectations on the very same inputs. A change on
// one side breaks the other side's test, which is the point.
var sharedPlainTextSamples = []struct {
	name string
	raw  string
	want string
}{
	{"entities", `a &amp; b &lt;c&gt; &quot;d&quot; &#39;e&#39; &nbsp;f`, `a & b "d" 'e' f`},
	{"hidden subtree", `<div style="opacity:0"><p>藏起来的</p></div><p>可见的</p>`, "可见的"},
	{"void tag", `使用 <br> 标签`, "使用 标签"},
	{"plain text", `普通的一段摘要文字，不含任何标记。`, "普通的一段摘要文字，不含任何标记。"},
	{"display none block", `<p style="display:none">藏块</p><p>正文</p>`, "正文"},
}

func TestPlainText_SharedSamplesWithFrontend(t *testing.T) {
	for _, tc := range sharedPlainTextSamples {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, PlainText(tc.raw))
		})
	}
}

func TestHasMarkup(t *testing.T) {
	require.True(t, HasMarkup("<p>x</p>"))
	require.True(t, HasMarkup("a &lt;b&gt; c"), "an escaped tag is markup for the reader too")
	require.False(t, HasMarkup("a < b"), "a bare comparison sign is not a tag")
	require.False(t, HasMarkup("普通的一段摘要文字"))
	require.False(t, HasMarkup(""))
}

func TestPlainTextAll_KeepsHiddenSubtrees(t *testing.T) {
	// The visible projection drops the hidden subtree: the whole 智源 article
	// sits under an opacity:0 root, so nothing is visible.
	require.Empty(t, PlainText(zhiyuanSample))

	// The "all" projection keeps it: a source that hides its own body must
	// still leave the summary-generation fallback with text to work on.
	all := PlainTextAll(zhiyuanSample)
	require.Contains(t, all, "正文第一段")
	require.Contains(t, all, "克雷西 发自 凹非寺")
	require.NotContains(t, all, "opacity")
}

func TestPlainTextAll_DropsScriptAndStylePayloads(t *testing.T) {
	require.Equal(t, "可见", PlainTextAll(`<script>alert(1)</script><style>p{color:red}</style><p>可见</p>`))
}

func TestPlainText_EscapedMarkupBecomesPlainText(t *testing.T) {
	// An escaped tag is markup for the reader too: the summary is a plain
	// text projection of the rendered document, so "&lt;b&gt;x" reads as "x"
	// rather than leaking the tag into the lede.
	require.Equal(t, "x", PlainText("&lt;b&gt;x"))
	require.Equal(t, "a c", PlainText("a&lt;b&gt;c"))
}

func TestPlainText_FoldsWhitespace(t *testing.T) {
	require.Equal(t, "a b", PlainText("  a \t\n\r b  "))
	require.Equal(t, "a b", PlainText("a&nbsp;&nbsp;b"))
	require.Equal(t, "", PlainText("   \t\n "))
	require.Equal(t, "a b", PlainText("<p>a</p><p>b</p>"))
}

func TestPlainText_PlainTextPassesThrough(t *testing.T) {
	require.Equal(t, "abc", PlainText("abc"))
	require.Equal(t, "普通中文摘要", PlainText("普通中文摘要"))
	require.Equal(t, "a b", PlainText("a   b"))
	require.Equal(t, "", PlainText(""))
}

func TestPlainSummary_TruncatesByRunes(t *testing.T) {
	long := strings.Repeat("导", 500)

	got := PlainSummary(long, 300)
	require.Len(t, []rune(got), 300)
	require.True(t, strings.HasSuffix(got, "…"))
	require.Equal(t, strings.Repeat("导", 299)+"…", got)

	require.Equal(t, long[:300], PlainSummary(long[:300], 300), "a summary that fits is not truncated")
	require.Equal(t, "…", PlainSummary("abcdef", 1))
	require.Equal(t, "", PlainSummary("abcdef", 0))
	require.Equal(t, "", PlainSummary("", 300))
}

func TestPlainSummary_RealSampleIsBoundedPlainText(t *testing.T) {
	got := PlainSummary(zhiyuanSample, 300)

	require.LessOrEqual(t, len([]rune(got)), 300)
	require.NotContains(t, got, "<")
	require.NotContains(t, got, "opacity")
	require.NotContains(t, got, "正文第一段")
}

func TestPlainSummary_Idempotent(t *testing.T) {
	inputs := []string{
		zhiyuanSample,
		"a&amp;b&lt;c",
		"a&b<c",
		"<p>Hello <b>world</b></p>",
		"<script>alert(1)</script>text",
		"普通中文摘要",
		strings.Repeat("导", 500),
		"",
	}

	for _, raw := range inputs {
		once := PlainSummary(raw, 300)
		twice := PlainSummary(once, 300)
		require.Equal(t, once, twice, "cleaning an already cleaned summary must not change it: %q", raw)
	}
}

func TestSanitizeHTML_KeepsStructureAndDropsDangerousParts(t *testing.T) {
	// The script and the style block sit at the TOP LEVEL here on purpose: the
	// previous version nested them inside a div, so the "script removed"
	// assertion passed through the child-removal path and never covered the
	// top level (a false green).
	raw := `<div class="post" style="color:#333;font-size:16px"><p>正文段落</p>` +
		`<img src="https://example.com/a.png" onerror="alert(1)"></div>` +
		`<script>alert(2)</script><style>p{color:red}</style>` +
		`<div style="display:none">广告</div><span hidden>隐藏</span>` +
		`<em style="opacity: 0">淡出文字</em>`

	got := SanitizeHTML(raw)

	require.Contains(t, got, `<p>正文段落</p>`, "visible structure survives")
	require.Contains(t, got, `src="https://example.com/a.png"`, "images survive")
	require.Contains(t, got, `color:#333`, "other inline styles survive")
	require.NotContains(t, got, "onerror", "event handler attributes are removed")
	require.NotContains(t, got, "<script", "script elements are removed")
	require.NotContains(t, got, "alert(2)", "script content is removed")
	require.NotContains(t, got, "<style", "style elements are removed")
	require.NotContains(t, got, "广告", "display:none elements are removed")
	require.NotContains(t, got, "隐藏", "hidden attribute elements are removed")
	require.Contains(t, got, "淡出文字", "opacity:0 content stays, only the declaration is stripped")
	require.NotContains(t, got, "opacity")
}

// TestSanitizeHTML_DropsTopLevelScriptStyleAndHiddenBlocks covers the top
// level of the fragment: a script, a style block and a display:none element
// that START the document must disappear as well — only the children were
// checked for removal before, so a document beginning with one of them kept
// it (and its payload).
func TestSanitizeHTML_DropsTopLevelScriptStyleAndHiddenBlocks(t *testing.T) {
	got := SanitizeHTML(`<script>alert(1)</script><style>p{color:red}</style>` +
		`<p style="display:none">藏块</p><p>正文</p>`)

	require.NotContains(t, got, "<script", "a top level script element must be removed")
	require.NotContains(t, got, "alert(1)", "the script payload must be removed")
	require.NotContains(t, got, "<style", "a top level style element must be removed")
	require.NotContains(t, got, "藏块", "a top level display:none element must be removed")
	require.Contains(t, got, "<p>正文</p>", "the visible content survives")
}

// TestSanitizeHTML_DropsNestedScriptAndStyle keeps the child path covered:
// the removal of a nested payload is a different code path than the top
// level one.
func TestSanitizeHTML_DropsNestedScriptAndStyle(t *testing.T) {
	got := SanitizeHTML(`<div><script>alert(1)</script><style>p{color:red}</style><p>正文</p></div>`)

	require.NotContains(t, got, "<script")
	require.NotContains(t, got, "<style")
	require.Contains(t, got, "<p>正文</p>")
}

func TestSanitizeHTML_StripsHidingDeclarations(t *testing.T) {
	require.Equal(t,
		`<div style="color:red">x</div>`,
		SanitizeHTML(`<div style="color:red;opacity:0">x</div>`),
	)
	require.Equal(t,
		`<div style="color:red">x</div>`,
		SanitizeHTML(`<div style="color:red;visibility:hidden;font-size:0">x</div>`),
	)
}

func TestPlainSummary_SameCleaningForBothCaps(t *testing.T) {
	// The list lede (≤200) and the stored summary (≤300) share this
	// implementation: below the smaller cap both must produce the very same
	// text, only the truncation differs (spec article-description-sanitization:
	// “仅允许长度上限不同”).
	//
	// The sample cleans to MORE than 200 runes on purpose: a shorter sample
	// would make both caps return the very same string, and the assertion
	// would carry no information about the shared cleaning.
	raw := "<p>" + strings.Repeat("导", 250) + "</p>"

	full := PlainSummary(raw, 300)
	short := PlainSummary(raw, 200)

	require.Len(t, []rune(full), 250, "the 300 cap must not truncate this sample")
	require.False(t, strings.HasSuffix(full, "…"), "the 300 cap must not mark a truncation that did not happen")
	require.True(t, strings.HasSuffix(short, "…"), "the 200 cap must mark the truncation")
	require.Equal(t, string([]rune(full)[:199])+"…", short,
		"both caps must share the same cleaning and differ only in the cap")
}

func TestSanitizeHTML_RealSampleKeepsBodyVisible(t *testing.T) {
	got := SanitizeHTML(zhiyuanSample)

	// The whole 智源 article sits under a visually hidden root
	// (visibility: visible; opacity: 0). Stripping the hiding declaration keeps
	// the body readable instead of emptying the "original content" source.
	require.Contains(t, got, "正文第一段")
	require.Contains(t, got, "克雷西 发自 凹非寺")
	require.NotContains(t, got, "opacity")
	require.NotContains(t, got, "visibility: hidden")
}

func TestSanitizeHTML_Idempotent(t *testing.T) {
	inputs := []string{
		zhiyuanSample,
		`<div style="color:red" onclick="alert(1)">正文</div>`,
		`<p>plain</p>`,
		"",
	}

	for _, raw := range inputs {
		once := SanitizeHTML(raw)
		twice := SanitizeHTML(once)
		require.Equal(t, once, twice, "sanitizing an already sanitized value must not change it: %q", raw)
	}
}

func TestSanitizeHTML_EmptyInput(t *testing.T) {
	require.Equal(t, "", SanitizeHTML(""))
}
