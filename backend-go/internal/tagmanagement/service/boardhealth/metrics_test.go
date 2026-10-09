package boardhealth

import (
	"testing"

	"github.com/stretchr/testify/require"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// 用例来源：openspec/changes/add-board-health-checkup/test-cases.md 主链路步 2
// （版块级指标）+ 变体（reason 缺失桶 unknown）。
// 2026-10-08 口径修正：删单 aux 占比断言（指标随旧归因口径退役）。

func TestBoardMetrics(t *testing.T) {
	rows := []matchedRow{
		{TopicTagID: 1, MatchReason: "direct_hit"},
		{TopicTagID: 2, MatchReason: "hit_rate"},
		{TopicTagID: 3, MatchReason: "max_sim", DirectionMismatch: true},
		{TopicTagID: 4, MatchReason: "composite_hit"},
		{TopicTagID: 5, MatchReason: "weighted", DirectionMismatch: true},
		{TopicTagID: 6, MatchReason: ""}, // reason 缺失行 → unknown 桶。
	}
	metrics := ComputeBoardMetrics(rows)

	require.Equal(t, 6, metrics.MatchedTags)
	require.Equal(t, map[string]int{
		"direct_hit": 1, "hit_rate": 1, "max_sim": 1,
		"composite_hit": 1, "weighted": 1, "unknown": 1,
	}, metrics.ReasonDist)
	require.Equal(t, 2, metrics.GhostMatches)
}

func TestBoardMetricsEmpty(t *testing.T) {
	// 变体：空集（版块零挂载 tags）——全 0。
	metrics := ComputeBoardMetrics(nil)
	require.Equal(t, 0, metrics.MatchedTags)
	require.Equal(t, map[string]int{}, metrics.ReasonDist)
	require.Equal(t, 0, metrics.GhostMatches)
}

// 用例来源：test-cases.md 主链路步 3/4（贴合度分布 + insufficient）+
// spec「构成标签贴合度分布全覆盖」「样本不足显示数据不足」。
// 口径②修正：构成标签 embedding × 全部挂载 tags identity embedding，
// 不按 aux 挂数筛样本（多 aux tag 同样计入）。

func TestComputeLabelAttribution(t *testing.T) {
	// 构成标签 101/102 均有自身 embedding {1,0}；8 个挂载 tags 中
	// 6 个同向（cosine=1）、2 个正交（cosine=0）——与 tag 挂几个 aux 无关。
	compositions := labelFixture(101, 102)
	auxEmb := map[uint][]float64{101: {1, 0}, 102: {1, 0}}
	tagEmb := map[uint][]float64{}
	rows := make([]matchedRow, 0, 8)
	for i := 1; i <= 6; i++ {
		tagEmb[uint(i)] = []float64{1, 0}
		rows = append(rows, matchedRow{TopicTagID: uint(i)})
	}
	for i := 7; i <= 8; i++ { // 正交样本
		tagEmb[uint(i)] = []float64{0, 1}
		rows = append(rows, matchedRow{TopicTagID: uint(i)})
	}
	config := Config{MinSampleSize: 5}

	labels, vectorMissing := computeLabelAttribution(rows, compositions, auxEmb, tagEmb, config)
	require.Equal(t, 0, vectorMissing)

	byID := labelMetricsByAuxID(labels)
	// 全覆盖：每个构成标签的样本 = 全部 8 个挂载 tags（多 aux 也在内）。
	for _, id := range []uint{101, 102} {
		require.Equal(t, 8, byID[id].Samples)
		require.False(t, byID[id].Insufficient)
	}
	entry := byID[101]
	require.InDelta(t, 0.75, *entry.CosineMean, 1e-9) // 6×1.0 + 2×0.0
	require.InDelta(t, 1.0, *entry.CosineMedian, 1e-9)
	require.InDelta(t, 0.75, *entry.CosineP25, 1e-9) // 线性插值 pos=1.75：0×0.25 + 1×0.75
	require.InDelta(t, 1.0, *entry.CosineP75, 1e-9)
	// 两个标签 mean 相同（0.75）→ P25 线=0.75；严格低于线才算低 → 2/8。
	require.InDelta(t, 0.25, *entry.LowSimRatio, 1e-9)
}

func TestComputeLabelAttributionRelativeLowLine(t *testing.T) {
	// 主链路步 3b + spec「低分线按板块内相对分位」：固定全局线弃用，
	// 低分占比按板块内达标标签 cosine_mean 的 P25 相对线计算。
	// 标签 A={1,0} 对 tags（3-4-5 基向量 {0.6,0.8}）cosine=0.6；标签 B={0.6,0.8}
	// cosine=1.0。means=[0.6,1.0] → P25 线=0.7 → A 全体低于线（low=1.0）、B 全体在线上（low=0）。
	tagVec := []float64{0.6, 0.8} // |tagVec|=1（3-4-5 基）
	tagEmb := map[uint][]float64{}
	rows := make([]matchedRow, 0, 5)
	for i := 1; i <= 5; i++ {
		tagEmb[uint(i)] = tagVec
		rows = append(rows, matchedRow{TopicTagID: uint(i)})
	}
	compositions := labelFixture(201, 202)
	auxEmb := map[uint][]float64{201: {1, 0}, 202: {0.6, 0.8}}
	labels, _ := computeLabelAttribution(rows, compositions, auxEmb, tagEmb, Config{MinSampleSize: 5})

	byID := labelMetricsByAuxID(labels)
	require.InDelta(t, 0.6, *byID[201].CosineMean, 1e-9)
	require.InDelta(t, 1.0, *byID[201].LowSimRatio, 1e-9)
	require.InDelta(t, 1.0, *byID[202].CosineMean, 1e-9)
	require.InDelta(t, 0.0, *byID[202].LowSimRatio, 1e-9)
}

func TestComputeLabelAttributionInsufficient(t *testing.T) {
	// spec「样本不足显示数据不足」：板块有向量的挂载 tags=3 < 5 →
	// 达标不了的标签 insufficient、分布字段全 null。
	tagEmb := map[uint][]float64{1: {1, 0}, 2: {1, 0}, 3: {1, 0}}
	rows := []matchedRow{{TopicTagID: 1}, {TopicTagID: 2}, {TopicTagID: 3}}
	labels, _ := computeLabelAttribution(rows, labelFixture(301), map[uint][]float64{301: {1, 0}}, tagEmb, Config{MinSampleSize: 5})
	entry := labels[0]
	require.Equal(t, 3, entry.Samples)
	require.True(t, entry.Insufficient)
	require.Nil(t, entry.CosineMean)
	require.Nil(t, entry.CosineMedian)
	require.Nil(t, entry.CosineP25)
	require.Nil(t, entry.CosineP75)
	require.Nil(t, entry.LowSimRatio)
}

func TestComputeLabelAttributionLabelEmbeddingMissing(t *testing.T) {
	// 白盒附加「贴合：构成标签缺自身 embedding」：该标签 insufficient
	// （samples=0），不计入 vector_missing（那个计数只管 tag 侧）。
	tagEmb := map[uint][]float64{1: {1, 0}, 2: {1, 0}, 3: {1, 0}, 4: {1, 0}, 5: {1, 0}}
	rows := make([]matchedRow, 0, 5)
	for i := 1; i <= 5; i++ {
		rows = append(rows, matchedRow{TopicTagID: uint(i)})
	}
	labels, vectorMissing := computeLabelAttribution(rows, labelFixture(401), map[uint][]float64{}, tagEmb, Config{MinSampleSize: 5})
	require.Equal(t, 0, vectorMissing, "label-side embedding missing is insufficient, not vector_missing")
	require.Equal(t, 0, labels[0].Samples)
	require.True(t, labels[0].Insufficient)
	require.Nil(t, labels[0].CosineMean)
}

func TestComputeLabelAttributionVectorMissing(t *testing.T) {
	// spec「向量缺失跳过且计数」：tag embedding 缺失的挂载行对所有标签
	// 不可见、计入 vectorMissing（不按 0 计入）；有效样本=有向量数。
	rows := []matchedRow{
		{TopicTagID: 1}, {TopicTagID: 2}, {TopicTagID: 3},
		{TopicTagID: 4}, {TopicTagID: 5},
	}
	tagEmb := map[uint][]float64{1: {1, 0}} // 只有 tag 1 有向量
	labels, vectorMissing := computeLabelAttribution(rows, labelFixture(501), map[uint][]float64{501: {1, 0}}, tagEmb, Config{MinSampleSize: 5})
	require.Equal(t, 4, vectorMissing)
	entry := labels[0]
	require.Equal(t, 1, entry.Samples) // 有效样本口径
	require.True(t, entry.Insufficient)
	require.Nil(t, entry.CosineMean)
}

func TestComputeLabelAttributionAllVectorsMissing(t *testing.T) {
	// 白盒附加「cosine：向量全缺失」：全部挂载 tags 无 identity 向量 →
	// vector_missing=matched 数、labels 全 insufficient（samples=0）。
	rows := []matchedRow{{TopicTagID: 1}, {TopicTagID: 2}}
	labels, vectorMissing := computeLabelAttribution(rows, labelFixture(601), map[uint][]float64{601: {1, 0}}, map[uint][]float64{}, Config{MinSampleSize: 5})
	require.Equal(t, 2, vectorMissing)
	require.True(t, labels[0].Insufficient)
	require.Equal(t, 0, labels[0].Samples)
}

func TestComputeLabelAttributionMinSampleBoundary(t *testing.T) {
	// 白盒附加：samples 恰=min_sample_size（默认 5）→ 达标出分布（≥ 判定）。
	rows := make([]matchedRow, 5)
	tagEmb := make(map[uint][]float64, 5)
	for i := range rows {
		rows[i] = matchedRow{TopicTagID: uint(i + 1)}
		tagEmb[uint(i+1)] = []float64{1, 0}
	}
	labels, _ := computeLabelAttribution(rows, labelFixture(701), map[uint][]float64{701: {1, 0}}, tagEmb, Config{MinSampleSize: 5})
	require.False(t, labels[0].Insufficient)
	require.NotNil(t, labels[0].CosineMean)

	// samples=4 → insufficient。
	labels, _ = computeLabelAttribution(rows[:4], labelFixture(701), map[uint][]float64{701: {1, 0}}, tagEmb, Config{MinSampleSize: 5})
	require.True(t, labels[0].Insufficient)
}

// labelFixture builds composition metric rows for the given aux ids.
func labelFixture(ids ...uint) []tagmodels.BoardHealthLabelMetrics {
	out := make([]tagmodels.BoardHealthLabelMetrics, 0, len(ids))
	for _, id := range ids {
		out = append(out, tagmodels.BoardHealthLabelMetrics{AuxID: id, Label: "L"})
	}
	return out
}

// labelMetricsByAuxID indexes label metrics by aux id for assertions.
func labelMetricsByAuxID(labels []tagmodels.BoardHealthLabelMetrics) map[uint]*tagmodels.BoardHealthLabelMetrics {
	out := make(map[uint]*tagmodels.BoardHealthLabelMetrics, len(labels))
	for i := range labels {
		out[labels[i].AuxID] = &labels[i]
	}
	return out
}
