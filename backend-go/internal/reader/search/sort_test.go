package search

// add-search-sort-modes tasks 1.2 — 排序纯函数白盒用例（test-cases.md §3
// 分支表）：ParseSortMode 枚举回退、sortDayKey 日界（+08:00 / 跨年）、
// effectiveTime 空值回退、SortFused date 多级比较与 relevance 原序。

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// cstTime 以 UTC+8 构造测试时间（Asia/Shanghai 固定偏移）。
func cstTime(t *testing.T, value string) time.Time {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02 15:04:05", value, searchSortZone)
	require.NoError(t, err)
	return ts
}

func dateRow(t *testing.T, id uint, pubDate string) ArticleSearchRow {
	t.Helper()
	ts := cstTime(t, pubDate)
	return ArticleSearchRow{ID: id, PubDate: &ts}
}

// TestParseSortMode covers 分支表 ParseSortMode：仅 relevance 精确匹配生效，
// 缺省/空串/非法值静默回退 date（spec「非法排序值回退默认」）。
func TestParseSortMode(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want SortMode
	}{
		{"relevance", SortByRelevance},
		{"date", SortByDate},
		{"", SortByDate},
		{"bogus", SortByDate},
		{"Date", SortByDate},
		{" relevance ", SortByDate},
	} {
		t.Run("raw="+tc.raw, func(t *testing.T) {
			require.Equal(t, tc.want, ParseSortMode(tc.raw))
		})
	}
}

// TestSortDayKey covers 分支表 sortDayKey：UTC+8 日界（23:59:59 与次日
// 00:00:00 分属不同日）、同一时刻不同时区表示同日、跨年有序。
func TestSortDayKey(t *testing.T) {
	late := cstTime(t, "2026-10-22 23:59:59")
	next := cstTime(t, "2026-10-23 00:00:00")
	require.Less(t, sortDayKey(late), sortDayKey(next), "23:59:59 与次日 00:00 必须不同日")

	// 2026-10-22T16:00:00Z == 2026-10-23 00:00 +08:00：同一 UTC+8 日历日
	utcMidnight := time.Date(2026, 10, 22, 16, 0, 0, 0, time.UTC)
	require.Equal(t, sortDayKey(next), sortDayKey(utcMidnight), "日界按 UTC+8 而非 UTC 划分")

	// 同一天不同时刻：日键相同
	require.Equal(t, sortDayKey(cstTime(t, "2026-10-22 00:00:00")), sortDayKey(late))

	// 跨年
	require.Less(t,
		sortDayKey(cstTime(t, "2026-12-31 23:59:59")),
		sortDayKey(cstTime(t, "2027-01-01 00:00:00")),
	)
}

// TestEffectiveTime covers 分支表 effectiveTime：pub_date 优先 / 为空回退
// created_at / 双零值仍为零值（排序自然垫底）。
func TestEffectiveTime(t *testing.T) {
	pub := cstTime(t, "2026-10-20 08:00:00")
	created := cstTime(t, "2026-10-19 08:00:00")

	for _, tc := range []struct {
		name string
		row  ArticleSearchRow
		want time.Time
	}{
		{"pub_date 优先", ArticleSearchRow{PubDate: &pub, CreatedAt: created}, pub},
		{"pub_date 为空回退 created_at", ArticleSearchRow{CreatedAt: created}, created},
		{"双零值", ArticleSearchRow{}, time.Time{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, effectiveTime(tc.row))
		})
	}
}

// TestSortFusedDateOrder covers 分支表 SortFused date 的多级比较全序：
// 日倒序 → RRF 分 → 关键词分 → 语义分 → 精确时间倒序 → ID 升序。
func TestSortFusedDateOrder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fused []FusedResult
		rows  map[uint]ArticleSearchRow
		want  []uint
	}{
		{
			name: "跨日：后一天在前（与精确时间/分数无关）",
			fused: []FusedResult{
				{ArticleID: 1, Score: 1.0 / 61},
				{ArticleID: 2, Score: 1.0 / 62},
			},
			rows: map[uint]ArticleSearchRow{
				1: dateRow(t, 1, "2026-10-22 23:59:59"),
				2: dateRow(t, 2, "2026-10-23 00:01:00"),
			},
			want: []uint{2, 1},
		},
		{
			name: "同日：RRF 分高者在前（不按精确发布时间）",
			fused: []FusedResult{
				{ArticleID: 1, Score: 1.0 / 61},
				{ArticleID: 2, Score: 1.0 / 62},
			},
			rows: map[uint]ArticleSearchRow{
				1: dateRow(t, 1, "2026-10-22 09:00:00"),
				2: dateRow(t, 2, "2026-10-22 20:00:00"),
			},
			want: []uint{1, 2},
		},
		{
			name: "同日同 RRF：关键词分高者在前",
			fused: []FusedResult{
				{ArticleID: 1, Score: 0.5, KeywordScore: 0.6},
				{ArticleID: 2, Score: 0.5, KeywordScore: 1.0},
			},
			rows: map[uint]ArticleSearchRow{
				1: dateRow(t, 1, "2026-10-22 09:00:00"),
				2: dateRow(t, 2, "2026-10-22 20:00:00"),
			},
			want: []uint{2, 1},
		},
		{
			name: "同日同 RRF 同关键词分：语义分高者在前",
			fused: []FusedResult{
				{ArticleID: 1, Score: 0.5, KeywordScore: 0.6, SemanticScore: 0.2},
				{ArticleID: 2, Score: 0.5, KeywordScore: 0.6, SemanticScore: 0.9},
			},
			rows: map[uint]ArticleSearchRow{
				1: dateRow(t, 1, "2026-10-22 09:00:00"),
				2: dateRow(t, 2, "2026-10-22 20:00:00"),
			},
			want: []uint{2, 1},
		},
		{
			name: "同日全平：精确发布时间倒序",
			fused: []FusedResult{
				{ArticleID: 1, Score: 0.5},
				{ArticleID: 2, Score: 0.5},
			},
			rows: map[uint]ArticleSearchRow{
				1: dateRow(t, 1, "2026-10-22 09:00:00"),
				2: dateRow(t, 2, "2026-10-22 18:00:00"),
			},
			want: []uint{2, 1},
		},
		{
			name: "同日同时刻：ID 升序兜底",
			fused: []FusedResult{
				{ArticleID: 2, Score: 0.5},
				{ArticleID: 1, Score: 0.5},
			},
			rows: map[uint]ArticleSearchRow{
				1: dateRow(t, 1, "2026-10-22 09:00:00"),
				2: dateRow(t, 2, "2026-10-22 09:00:00"),
			},
			want: []uint{1, 2},
		},
		{
			name: "跨年日界：元旦在前",
			fused: []FusedResult{
				{ArticleID: 1, Score: 1.0},
				{ArticleID: 2, Score: 0.1},
			},
			rows: map[uint]ArticleSearchRow{
				1: dateRow(t, 1, "2026-12-31 23:59:59"),
				2: dateRow(t, 2, "2027-01-01 00:01:00"),
			},
			want: []uint{2, 1},
		},
		{
			name: "行投影缺失：零值时间排最后",
			fused: []FusedResult{
				{ArticleID: 1, Score: 1.0},
				{ArticleID: 2, Score: 0.1},
			},
			rows: map[uint]ArticleSearchRow{
				1: dateRow(t, 1, "2026-10-20 09:00:00"),
			},
			want: []uint{1, 2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := SortFused(tc.fused, tc.rows, SortByDate)
			require.Equal(t, tc.want, idsOf(got))
		})
	}
}

// TestSortFusedPubDateEmptyFallsBackToCreatedAt：pub_date 为空时用
// created_at 参与日期分组（spec「空发布时间回退创建时间」）。
func TestSortFusedPubDateEmptyFallsBackToCreatedAt(t *testing.T) {
	fused := []FusedResult{
		{ArticleID: 1, Score: 1.0},
		{ArticleID: 2, Score: 0.1},
	}
	rows := map[uint]ArticleSearchRow{
		// 1 无 pub_date，但 created_at 是 23 日 → 归 23 日分组
		1: {ID: 1, CreatedAt: cstTime(t, "2026-10-23 09:00:00")},
		// 2 有 pub_date（22 日）→ 归 22 日分组
		2: dateRow(t, 2, "2026-10-22 23:59:00"),
	}
	require.Equal(t, []uint{1, 2}, idsOf(SortFused(fused, rows, SortByDate)))

	// 双零值：排到有时间的条目之后
	rows[1] = ArticleSearchRow{ID: 1}
	require.Equal(t, []uint{2, 1}, idsOf(SortFused(fused, rows, SortByDate)))
}

// TestSortFusedRelevanceKeepsInputOrder：relevance 模式原样返回（引用与顺序
// 都不变），不改写旧 RRF 顺序。
func TestSortFusedRelevanceKeepsInputOrder(t *testing.T) {
	fused := []FusedResult{
		{ArticleID: 3, Score: 0.9},
		{ArticleID: 1, Score: 0.5},
		{ArticleID: 2, Score: 0.1},
	}
	rows := map[uint]ArticleSearchRow{
		1: dateRow(t, 1, "2026-10-23 09:00:00"),
		2: dateRow(t, 2, "2026-10-24 09:00:00"),
		3: dateRow(t, 3, "2026-10-01 09:00:00"),
	}

	got := SortFused(fused, rows, SortByRelevance)
	require.Equal(t, []uint{3, 1, 2}, idsOf(got), "relevance 不重排，保持 RRF 顺序")
	require.Equal(t, &fused[0], &got[0], "relevance 直接返回入参，不做复制")
}

// TestSortFusedDateDoesNotMutateInput：date 模式复制后排序，入参顺序不变
// （Fuse 输出可被同一请求内复用）。
func TestSortFusedDateDoesNotMutateInput(t *testing.T) {
	fused := []FusedResult{
		{ArticleID: 1, Score: 1.0},
		{ArticleID: 2, Score: 0.1},
	}
	rows := map[uint]ArticleSearchRow{
		1: dateRow(t, 1, "2026-10-20 09:00:00"),
		2: dateRow(t, 2, "2026-10-24 09:00:00"),
	}

	got := SortFused(fused, rows, SortByDate)
	require.Equal(t, []uint{2, 1}, idsOf(got))
	require.Equal(t, []uint{1, 2}, idsOf(fused), "入参顺序不得被改写")
}
