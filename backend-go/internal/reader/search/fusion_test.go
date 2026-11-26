package search

// add-article-search tasks 2.5 — RRF 融合 + 命中层级加权函数单测。
// 覆盖 test-cases.md 白盒「命中层级加权排序断言」表四行对比组 + RRF 求和
// + 命中解释聚合去重 + 平分确定性 + expand 榜预留（tasks 2.7）。

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func hitRow(id uint, details ...HitDetail) ArticleHit {
	return ArticleHit{Article: ArticleSearchRow{ID: id}, Hits: details}
}

// TestKeywordRankOrderingMultiVsSingle covers 断言表行 1：
// 同基础条件下词典多词命中（0.85）排在词典单词命中（0.6）之前。
func TestKeywordRankOrderingMultiVsSingle(t *testing.T) {
	entries := KeywordRankEntries([]ArticleHit{
		hitRow(2, HitDetail{Level: LevelDictSingle, Field: FieldTitle, Text: "人工智能"}),
		hitRow(1, HitDetail{Level: LevelDictMulti, Field: FieldTitle, Text: "人工智能"},
			HitDetail{Level: LevelDictMulti, Field: FieldTitle, Text: "大模型"}),
	})
	require.Len(t, entries, 2)
	require.Equal(t, uint(1), entries[0].ArticleID, "多词命中 > 单词命中")
	require.InDelta(t, 0.85, entries[0].Score, 1e-9)
	require.InDelta(t, 0.6, entries[1].Score, 1e-9)
	require.Len(t, entries[0].Reasons, 2, "逐词展开 hit_reasons")
	require.Equal(t, ReasonKeyword, entries[0].Reasons[0].Type)
}

// TestKeywordRankOrderingWholeVsDictMulti covers 断言表行 2：
// 整串命中（1.0）> 词典多词（0.85）。
func TestKeywordRankOrderingWholeVsDictMulti(t *testing.T) {
	entries := KeywordRankEntries([]ArticleHit{
		hitRow(2, HitDetail{Level: LevelDictMulti, Field: FieldTitle, Text: "人工智能"}),
		hitRow(1, HitDetail{Level: LevelWhole, Field: FieldTitle, Text: "人工智能大模型"}),
	})
	require.Equal(t, uint(1), entries[0].ArticleID, "整串 > 词典多词")
	require.InDelta(t, 1.0, entries[0].Score, 1e-9)
}

// TestKeywordRankOrderingTitleVsDescription covers 断言表行 3：
// 同层级 title 命中（权重 1.0）> description 命中（0.7）。
func TestKeywordRankOrderingTitleVsDescription(t *testing.T) {
	entries := KeywordRankEntries([]ArticleHit{
		hitRow(2, HitDetail{Level: LevelDictMulti, Field: FieldDescription, Text: "人工智能"}),
		hitRow(1, HitDetail{Level: LevelDictMulti, Field: FieldTitle, Text: "人工智能"}),
	})
	require.Equal(t, uint(1), entries[0].ArticleID, "title > description（同层级）")
	require.InDelta(t, 0.85, entries[0].Score, 1e-9)
	require.InDelta(t, 0.85*0.7, entries[1].Score, 1e-9, "desc 分 = 0.85 × 0.7")
}

// TestKeywordRankTrigramUsesRawSimilarity covers 断言表行 4：
// trigram 无固定系数，用原始 similarity × 字段权重。
func TestKeywordRankTrigramUsesRawSimilarity(t *testing.T) {
	entries := KeywordRankEntries([]ArticleHit{
		// desc sim 0.25 → 0.175；title sim 0.2 → 0.2：title 反超。
		hitRow(1, HitDetail{Level: LevelTrigram, Field: FieldTitle, Text: "特斯拉电池自燃", TrigramScore: 0.2}),
		hitRow(2, HitDetail{Level: LevelTrigram, Field: FieldDescription, Text: "特斯拉电池自燃", TrigramScore: 0.25}),
	})
	require.Equal(t, uint(1), entries[0].ArticleID, "title 0.2×1.0 > desc 0.25×0.7")
	require.InDelta(t, 0.2, entries[0].Score, 1e-9)
	require.InDelta(t, 0.175, entries[1].Score, 1e-9)

	// 交叉：字段权重不足以反超更悬殊的原始分（0.1×1.0 < 0.2×0.7）。
	entries = KeywordRankEntries([]ArticleHit{
		hitRow(1, HitDetail{Level: LevelTrigram, Field: FieldTitle, TrigramScore: 0.1}),
		hitRow(2, HitDetail{Level: LevelTrigram, Field: FieldDescription, TrigramScore: 0.2}),
	})
	require.Equal(t, uint(2), entries[0].ArticleID, "desc 0.14 > title 0.1：原始分主导")
}

// TestKeywordScoreTakesMaxAcrossLayers：同一文章多层命中取最高层加权分
// （design D2「关键词分数取最高层」）。
func TestKeywordScoreTakesMaxAcrossLayers(t *testing.T) {
	score := KeywordScore([]HitDetail{
		{Level: LevelTrigram, Field: FieldTitle, TrigramScore: 0.3},
		{Level: LevelDictSingle, Field: FieldDescription}, // 0.42
		{Level: LevelWhole, Field: FieldDescription},      // 0.7 ← 最高
	})
	require.InDelta(t, 0.7, score, 1e-9)
}

// TestFuseRRFSum covers 断言表「融合」行：RRF k=60，各路排名后求和。
func TestFuseRRFSum(t *testing.T) {
	// keyword 榜：A(rank1) B(rank2)；semantic 榜：A(rank2) C(rank1)。
	fused := Fuse(FuseInput{
		Keyword: []RankEntry{
			{ArticleID: 1, Score: 0.9},
			{ArticleID: 2, Score: 0.8},
		},
		Semantic: []RankEntry{
			{ArticleID: 1, Score: 0.7},
			{ArticleID: 3, Score: 0.9},
		},
	})
	require.Len(t, fused, 3)
	wantA := 1.0/61 + 1.0/62 // keyword rank1 + semantic rank2
	wantB := 1.0 / 62        // keyword rank2 only
	wantC := 1.0 / 61        // semantic rank1 only
	require.InDelta(t, wantA, fused[0].Score, 1e-12)
	require.Equal(t, uint(1), fused[0].ArticleID)
	require.InDelta(t, wantC, fused[1].Score, 1e-12)
	require.Equal(t, uint(3), fused[1].ArticleID)
	require.InDelta(t, wantB, fused[2].Score, 1e-12)
	require.Equal(t, uint(2), fused[2].ArticleID)
	require.InDelta(t, 0.9, fused[0].KeywordScore, 1e-9)
	require.InDelta(t, 0.7, fused[0].SemanticScore, 1e-9)
}

// TestFuseMergesHitReasonsAcrossLists：同文章多路命中合并进同一结果的
// hit_reasons；同 (type,text) 去重保最高分（spec 契约）。
func TestFuseMergesHitReasonsAcrossLists(t *testing.T) {
	fused := Fuse(FuseInput{
		Keyword: []RankEntry{
			{
				ArticleID: 7, Score: 1.0,
				Reasons: []HitReason{
					{Type: ReasonKeyword, Text: "特斯拉", Score: 1.0},
					{Type: ReasonKeyword, Text: "特斯拉", Score: 0.6}, // 同词低分层命中
				},
			},
		},
		Semantic: []RankEntry{
			{
				ArticleID: 7, Score: 0.9,
				Reasons: []HitReason{
					{Type: ReasonSemantic, Text: "电动车", Score: 0.9},
					{Type: ReasonSemantic, Text: "电池安全", Score: 0.8},
				},
			},
		},
	})
	require.Len(t, fused, 1)
	require.Len(t, fused[0].HitReasons, 3, "特斯拉 去重 + 两个语义标签")
	require.Equal(t, []HitReason{
		{Type: ReasonKeyword, Text: "特斯拉", Score: 1.0},
		{Type: ReasonSemantic, Text: "电动车", Score: 0.9},
		{Type: ReasonSemantic, Text: "电池安全", Score: 0.8},
	}, fused[0].HitReasons)
}

// TestFuseRankingIsDeterministicOnTies：融合分并列时按关键词分、语义分、
// ID 稳定定序；入参乱序不改变结果。
func TestFuseRankingIsDeterministicOnTies(t *testing.T) {
	input := FuseInput{
		Keyword: []RankEntry{
			{ArticleID: 5, Score: 0.5, Reasons: []HitReason{{Type: ReasonKeyword, Text: "w", Score: 0.5}}},
			{ArticleID: 4, Score: 0.5, Reasons: []HitReason{{Type: ReasonKeyword, Text: "w", Score: 0.5}}},
			{ArticleID: 6, Score: 0.5, Reasons: []HitReason{{Type: ReasonKeyword, Text: "w", Score: 0.5}}},
		},
	}
	first := Fuse(input)
	require.Equal(t, []uint{4, 5, 6}, idsOf(first), "同分同榜按 ArticleID 升序")
}

func idsOf(results []FusedResult) []uint {
	out := make([]uint, len(results))
	for i, r := range results {
		out[i] = r.ArticleID
	}
	return out
}

// TestFuseExpandListReserved：expand 榜参与 RRF 与 reason 聚合
// （type=expand），为 tasks 2.7 LLM 扩展预留。
func TestFuseExpandListReserved(t *testing.T) {
	fused := Fuse(FuseInput{
		Keyword: []RankEntry{{ArticleID: 1, Score: 0.9}},
		Expand: []RankEntry{
			{ArticleID: 2, Score: 0.8, Reasons: []HitReason{{Type: ReasonExpand, Text: "扩词", Score: 0.8}}},
		},
	})
	require.Len(t, fused, 2)
	require.Equal(t, uint(1), fused[0].ArticleID, "keyword rank1 1/61 > expand rank1 1/61 并列后关键词分胜出")
	require.Len(t, fused[1].HitReasons, 1)
	require.Equal(t, ReasonExpand, fused[1].HitReasons[0].Type)
}

// TestFuseEmptyInput：三路全空 → 空结果（无结果空态路径）。
func TestFuseEmptyInput(t *testing.T) {
	require.Empty(t, Fuse(FuseInput{}))
}
