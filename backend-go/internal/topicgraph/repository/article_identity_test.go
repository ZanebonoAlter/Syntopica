package repository

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// threadWithRelatedIDs 造一条只关心 related_article_ids 的线索（折叠输入）。
func threadWithRelatedIDs(raw string) DailyReportThread {
	return DailyReportThread{ID: 1, RelatedArticleIDs: JSON(raw)}
}

// TestFoldThreadArticleCopies 覆盖 test-cases.md 的白盒分支表 B1~B13 与变体
// 走查 #1~#13、#15（纯函数层，无 DB）。
func TestFoldThreadArticleCopies(t *testing.T) {
	longLink := "https://example.com/" + strings.Repeat("a", 5000)
	longerLink := longLink + "b"

	cases := []struct {
		name  string
		raw   string
		links map[uint]string
		want  string
	}{
		// ── B2：空 / 非法 JSON / 非数组 → 原样保留 ──────────────────────
		{name: "B2 空字段", raw: "", links: map[uint]string{1: "https://example.com/a"}, want: ""},
		{name: "B2 空数组", raw: "[]", links: map[uint]string{}, want: "[]"},
		{name: "B2 JSON null 标量", raw: "null", links: map[uint]string{}, want: "null"},
		{name: "B2 非数组数字", raw: "123", links: map[uint]string{}, want: "123"},
		{name: "B2 非数组对象", raw: "{}", links: map[uint]string{}, want: "{}"},
		{name: "B2 非法 JSON", raw: "[1,", links: map[uint]string{}, want: "[1,"},
		{name: "B2 数组含非数字元素", raw: "[1,\"x\"]", links: map[uint]string{}, want: "[1,\"x\"]"},

		// ── B3：id 查不到 link → 原样保留，不折叠不删 ────────────────────
		{name: "B3 越界 id 查不到 link", raw: "[999999]", links: map[uint]string{}, want: "[999999]"},
		{name: "变体#11 多个越界 id", raw: "[999999,1000000]", links: map[uint]string{}, want: "[999999,1000000]"},

		// ── B4 / 变体#1#2：link 空 / 纯空白 → 不参与折叠，原样保留 ────────
		{name: "变体#1 link 空串", raw: "[1]", links: map[uint]string{1: ""}, want: "[1]"},
		{name: "变体#2 link 纯空白", raw: "[1]", links: map[uint]string{1: "   "}, want: "[1]"},
		{name: "变体#2 link 全角空格", raw: "[1]", links: map[uint]string{1: "　"}, want: "[1]"},
		{name: "变体#2 link tab", raw: "[1]", links: map[uint]string{1: "\t\n"}, want: "[1]"},
		{name: "B4 两个空 link 各自保留", raw: "[1,2]", links: map[uint]string{1: "", 2: "  "}, want: "[1,2]"},

		// ── B5/B6：归一 link 相同 → 折叠副本，保留首个 ──────────────────
		{name: "同 link 跨 feed 副本折叠", raw: "[1,2]", links: map[uint]string{1: "https://example.com/story", 2: "https://example.com/story"}, want: "[1]"},
		{name: "变体#13 折叠与时间无关：同 link 即折叠（判据无时间项）", raw: "[1,2]", links: map[uint]string{1: "https://example.com/story", 2: "https://example.com/story"}, want: "[1]"},
		{name: "变体#9 单元素原样", raw: "[5]", links: map[uint]string{5: "https://example.com/story"}, want: "[5]"},

		// ── B7：首个出现为准（顺序敏感） ─────────────────────────────────
		{name: "B7 副本在前保留副本", raw: "[2,1]", links: map[uint]string{1: "https://example.com/story", 2: "https://example.com/story"}, want: "[2]"},
		{name: "B7 三个 id 只留首个", raw: "[3,1,2]", links: map[uint]string{1: "https://example.com/story", 2: "https://example.com/story", 3: "https://example.com/story"}, want: "[3]"},

		// ── B8：不同 link / 同标题不同 link → 都保留 ─────────────────────
		{name: "B8 不同 link 都保留", raw: "[1,2]", links: map[uint]string{1: "https://example.com/a", 2: "https://example.com/b"}, want: "[1,2]"},
		{name: "B8 同标题不同 link 都保留", raw: "[1,2]", links: map[uint]string{1: "https://site-a.com/x", 2: "https://site-b.com/x"}, want: "[1,2]"},

		// ── B9/B10/B11：归一后等价（host 大小写 / query / fragment） ──────
		{name: "B9 host 大小写折叠", raw: "[1,2]", links: map[uint]string{1: "https://EXAMPLE.com/a", 2: "https://example.com/a"}, want: "[1]"},
		{name: "B10 仅 query 不同折叠", raw: "[1,2]", links: map[uint]string{1: "https://example.com/a", 2: "https://example.com/a?utm_source=x"}, want: "[1]"},
		{name: "B11 仅 fragment 不同折叠", raw: "[1,2]", links: map[uint]string{1: "https://example.com/a#reply12", 2: "https://example.com/a#reply13"}, want: "[1]"},
		{name: "B11 hashbang fragment 保留故不折叠", raw: "[1,2]", links: map[uint]string{1: "https://example.com/a#!top", 2: "https://example.com/a#!other"}, want: "[1,2]"},
		{name: "变体#5 scheme 大小写折叠", raw: "[1,2]", links: map[uint]string{1: "HTTPS://EXAMPLE.COM/a", 2: "https://example.com/a"}, want: "[1]"},

		// ── B12 / 变体#10：同一 id 重复出现 → 折叠为一条 ─────────────────
		{name: "B12 同一 id 重复折叠", raw: "[7,7]", links: map[uint]string{7: "https://example.com/a"}, want: "[7]"},
		{name: "变体#10 同一 id 重复（查不到 link）保留首个", raw: "[7,7]", links: map[uint]string{}, want: "[7]"},

		// ── B13：link 映射整体为空 → 原样返回（不折叠） ──────────────────
		{name: "B13 空映射原样返回", raw: "[1,2,3]", links: map[uint]string{}, want: "[1,2,3]"},

		// ── 变体#3/#4/#6/#7：非常规输入 ────────────────────────────────
		{name: "变体#3 纯分隔符参与折叠", raw: "[1,2]", links: map[uint]string{1: "///", 2: "///"}, want: "[1]"},
		{name: "变体#4 单 token 参与折叠", raw: "[1,2]", links: map[uint]string{1: "abc", 2: "abc"}, want: "[1]"},
		{name: "变体#6 特殊字符参与折叠", raw: "[1,2]", links: map[uint]string{1: "https://example.com/a\"b'c+d", 2: "https://example.com/a\"b'c+d"}, want: "[1]"},
		{name: "变体#6 emoji 参与折叠", raw: "[1,2]", links: map[uint]string{1: "https://example.com/😀", 2: "https://example.com/😀"}, want: "[1]"},
		{name: "变体#7 超长 link 参与折叠", raw: "[1,2]", links: map[uint]string{1: longLink, 2: longLink}, want: "[1]"},
		{name: "变体#7 超长 link 不同不折叠", raw: "[1,2]", links: map[uint]string{1: longLink, 2: longerLink}, want: "[1,2]"},

		// ── 变体#12：部分满足（有 link 的折叠，查不到的保留） ────────────
		{name: "变体#12 部分满足", raw: "[1,2,999999]", links: map[uint]string{1: "https://example.com/a", 2: "https://example.com/a"}, want: "[1,999999]"},
		{name: "变体#12 副本在前且查不到保留", raw: "[999999,1,2]", links: map[uint]string{1: "https://example.com/a", 2: "https://example.com/a"}, want: "[999999,1]"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			thread := threadWithRelatedIDs(tc.raw)
			folded := FoldThreadArticleCopies([]DailyReportThread{thread}, tc.links)
			require.Len(t, folded, 1)
			require.Equal(t, tc.want, string(folded[0].RelatedArticleIDs))
			// 纯函数：入参不被改写（落库口径不受影响，红线 45）
			require.Equal(t, tc.raw, string(thread.RelatedArticleIDs))
		})
	}
}

// TestFoldThreadArticleCopies_EmptyThreads：空线索数组原样返回（B1）。
func TestFoldThreadArticleCopies_EmptyThreads(t *testing.T) {
	require.Empty(t, FoldThreadArticleCopies(nil, map[uint]string{1: "https://example.com/a"}))
	require.Empty(t, FoldThreadArticleCopies([]DailyReportThread{}, map[uint]string{1: "https://example.com/a"}))
}

// TestFoldThreadArticleCopies_PerThreadIsolation：各线索独立折叠，互不影响。
func TestFoldThreadArticleCopies_PerThreadIsolation(t *testing.T) {
	threads := []DailyReportThread{
		{ID: 1, RelatedArticleIDs: JSON("[1,2]")},
		{ID: 2, RelatedArticleIDs: JSON("[3,4]")},
		{ID: 3, RelatedArticleIDs: JSON("")},
	}
	links := map[uint]string{
		1: "https://example.com/story",
		2: "https://example.com/story",
		3: "https://example.com/other",
		4: "https://example.com/another",
	}

	folded := FoldThreadArticleCopies(threads, links)
	require.Len(t, folded, 3)
	require.Equal(t, "[1]", string(folded[0].RelatedArticleIDs))
	require.Equal(t, "[3,4]", string(folded[1].RelatedArticleIDs))
	require.Equal(t, "", string(folded[2].RelatedArticleIDs))
	// 入参三条线索全部保持原值
	require.Equal(t, "[1,2]", string(threads[0].RelatedArticleIDs))
	require.Equal(t, "[3,4]", string(threads[1].RelatedArticleIDs))
}

// TestFoldThreadArticleCopies_Idempotent：重复折叠两次结果一致（变体#15）。
func TestFoldThreadArticleCopies_Idempotent(t *testing.T) {
	threads := []DailyReportThread{
		{ID: 1, RelatedArticleIDs: JSON("[1,2,3]")},
	}
	links := map[uint]string{
		1: "https://example.com/story",
		2: "https://example.com/story",
		3: "https://example.com/story",
	}

	once := FoldThreadArticleCopies(threads, links)
	twice := FoldThreadArticleCopies(once, links)
	require.Equal(t, string(once[0].RelatedArticleIDs), string(twice[0].RelatedArticleIDs))
	require.Equal(t, "[1]", string(once[0].RelatedArticleIDs))
}
