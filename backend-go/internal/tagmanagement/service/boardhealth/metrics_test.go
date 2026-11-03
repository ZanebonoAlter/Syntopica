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

// ── 磁铁信号（board-health-triage-advice seam A 1.2，spec「标签分诊信号计算」）──
// 用例来源：test-cases 主链路 A1 + 白盒「磁铁」组（gap=0.08±ε、失败样本 4/5、
// 任侧 0/NULL）。口径：同一组版块级 mismatch 行对每个构成标签分别算两侧均值，
// 不做单标签吸引归因；样本不足/向量缺失不硬凑。

// magnetRows builds nOK aligned + nFailed orthogonal mounted rows (all with
// identity embeddings); the label embedding is aligned, so all_mean stays
// high while failed_mean drops to 0.
func magnetRows(tagEmb map[uint][]float64, nOK, nFailed int) []matchedRow {
	rows := make([]matchedRow, 0, nOK+nFailed)
	id := uint(1)
	for i := 0; i < nOK; i++ {
		tagEmb[id] = []float64{1, 0}
		rows = append(rows, matchedRow{TopicTagID: id})
		id++
	}
	for i := 0; i < nFailed; i++ {
		tagEmb[id] = []float64{0, 1}
		rows = append(rows, matchedRow{TopicTagID: id, DirectionMismatch: true})
		id++
	}
	return rows
}

func TestComputeMagnetSignalsSuspicious(t *testing.T) {
	// spec Scenario「磁铁信号失败子集贴合差成立」：全体均值 0.68 vs 失败子集
	// 0.52、失败样本 40 ≥ 5 → 嫌疑成立，携带两侧均值与样本数。
	// 用正交基模拟：10 aligned + 5 orthogonal → all_mean=0.667、failed_mean=0。
	tagEmb := map[uint][]float64{}
	rows := magnetRows(tagEmb, 10, 5)
	auxEmb := map[uint][]float64{101: {1, 0}}
	labels := labelFixture(101)
	computeMagnetSignals(labels, auxEmb, rows, tagEmb, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})

	m := labels[0].Magnet
	require.NotNil(t, m)
	require.False(t, m.Insufficient)
	require.True(t, m.Suspicious)
	require.InDelta(t, 10.0/15.0, *m.AllMean, 1e-9)
	require.InDelta(t, 0.0, *m.FailedMean, 1e-9)
	require.Equal(t, 15, m.AllSamples)
	require.Equal(t, 5, m.FailedSamples)
}

func TestComputeMagnetSignalsGapBoundary(t *testing.T) {
	// 白盒「gap=0.08±ε」：阈值 >= 语义。夹具精确可算：5 个失败子集向量
	// cos=0.6（{0.6,0.8} 基），另加 5 个对齐向量 cos=1.0 → all_mean=0.8、
	// failed_mean=0.6、gap=0.2；用计算出的 gap 卡阈值两侧。
	tagEmb := map[uint][]float64{}
	rows := make([]matchedRow, 0, 10)
	id := uint(1)
	for i := 0; i < 5; i++ {
		tagEmb[id] = []float64{1, 0}
		rows = append(rows, matchedRow{TopicTagID: id})
		id++
	}
	for i := 0; i < 5; i++ {
		tagEmb[id] = []float64{0.6, 0.8} // 单位向量，与 {1,0} 的 cosine=0.6
		rows = append(rows, matchedRow{TopicTagID: id, DirectionMismatch: true})
		id++
	}
	auxEmb := map[uint][]float64{201: {1, 0}}
	labels := labelFixture(201)
	computeMagnetSignals(labels, auxEmb, rows, tagEmb, Config{MinSampleSize: 5, MagnetMinFailed: 5, MagnetGap: 0.08})
	m := labels[0].Magnet
	require.InDelta(t, 0.8, *m.AllMean, 1e-9)
	require.InDelta(t, 0.6, *m.FailedMean, 1e-9)
	gap := *m.AllMean - *m.FailedMean
	require.InDelta(t, 0.2, gap, 1e-9)
	require.True(t, m.Suspicious, "gap 0.2 ≥ 0.08 → suspicious")

	// 阈值恰等于 gap → 成立（>= 语义）；阈值高一丝 → 不成立。
	computeMagnetSignals(labels, auxEmb, rows, tagEmb, Config{MinSampleSize: 5, MagnetMinFailed: 5, MagnetGap: gap})
	require.True(t, labels[0].Magnet.Suspicious, "gap == threshold → suspicious (>=)")
	computeMagnetSignals(labels, auxEmb, rows, tagEmb, Config{MinSampleSize: 5, MagnetMinFailed: 5, MagnetGap: gap + 1e-9})
	require.False(t, labels[0].Magnet.Suspicious, "gap < threshold → not suspicious")
}

func TestComputeMagnetSignalsFailedSamplesBoundary(t *testing.T) {
	// 白盒「失败样本 4/5」：failed=4 < min_failed=5 → insufficient、不判嫌疑。
	tagEmb := map[uint][]float64{}
	rows := magnetRows(tagEmb, 10, 4)
	labels := labelFixture(301)
	computeMagnetSignals(labels, map[uint][]float64{301: {1, 0}}, rows, tagEmb, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	m := labels[0].Magnet
	require.True(t, m.Insufficient, "failed samples 4 < 5 → insufficient")
	require.False(t, m.Suspicious, "样本不足不硬凑结论")
	require.NotNil(t, m.AllMean, "观测均值仍记录（真实观测，非结论）")

	// failed=5 恰达标 → 可判。
	tagEmb = map[uint][]float64{}
	rows = magnetRows(tagEmb, 10, 5)
	labels = labelFixture(301)
	computeMagnetSignals(labels, map[uint][]float64{301: {1, 0}}, rows, tagEmb, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	require.False(t, labels[0].Magnet.Insufficient)
	require.True(t, labels[0].Magnet.Suspicious)
}

func TestComputeMagnetSignalsInsufficientScenarios(t *testing.T) {
	// spec Scenario「失败子集样本不足不强判」：全板仅 2 个 mismatch 行 →
	// 全部构成标签磁铁信号 insufficient。
	tagEmb := map[uint][]float64{}
	rows := magnetRows(tagEmb, 8, 2)
	labels := labelFixture(401, 402)
	computeMagnetSignals(labels, map[uint][]float64{401: {1, 0}, 402: {1, 0}}, rows, tagEmb, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	for _, l := range labels {
		require.True(t, l.Magnet.Insufficient)
		require.False(t, l.Magnet.Suspicious)
	}

	// spec Scenario「向量缺失信号不硬算」：标签自身 embedding 缺失 → 两侧
	// 均值不可算（nil），insufficient。
	tagEmb2 := map[uint][]float64{}
	rows2 := magnetRows(tagEmb2, 10, 5)
	labels2 := labelFixture(501)
	computeMagnetSignals(labels2, map[uint][]float64{}, rows2, tagEmb2, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	m := labels2[0].Magnet
	require.True(t, m.Insufficient)
	require.Nil(t, m.AllMean)
	require.Nil(t, m.FailedMean)
	require.False(t, m.Suspicious, "缺向量不按 0 参与嫌疑判定")

	// 白盒「任侧 0」：全部挂载 tag 无 identity 向量 → all_n=0，两侧样本
	// 皆 0（nil 均值），insufficient；tag 侧缺向量对信号不可见。
	rows3 := []matchedRow{{TopicTagID: 1}, {TopicTagID: 2, DirectionMismatch: true}}
	labels3 := labelFixture(601)
	computeMagnetSignals(labels3, map[uint][]float64{601: {1, 0}}, rows3, map[uint][]float64{}, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	m3 := labels3[0].Magnet
	require.Equal(t, 0, m3.AllSamples)
	require.Equal(t, 0, m3.FailedSamples)
	require.Nil(t, m3.AllMean)
	require.True(t, m3.Insufficient)

	// 白盒「all 侧不足」：全体向量样本 3 < MinSampleSize 5 → 不足
	//（即使 failed 侧达标也不可能：failed ⊆ all）。
	tagEmb4 := map[uint][]float64{}
	rows4 := magnetRows(tagEmb4, 0, 3)
	labels4 := labelFixture(701)
	computeMagnetSignals(labels4, map[uint][]float64{701: {1, 0}}, rows4, tagEmb4, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	require.True(t, labels4[0].Magnet.Insufficient)
}

func TestComputeMagnetSignalsVectorMissingTagSkipped(t *testing.T) {
	// 白盒：tag 侧向量缺失的 mismatch 行不进失败子集（与分布口径一致：
	// 缺向量行对所有标签不可见，计入 vector_missing 而非按 0 硬算）。
	tagEmb := map[uint][]float64{1: {1, 0}, 2: {1, 0}, 3: {1, 0}, 4: {1, 0}, 5: {1, 0}}
	rows := []matchedRow{
		{TopicTagID: 1}, {TopicTagID: 2}, {TopicTagID: 3}, {TopicTagID: 4}, {TopicTagID: 5},
		{TopicTagID: 6, DirectionMismatch: true}, // 无向量
		{TopicTagID: 7, DirectionMismatch: true}, // 无向量
	}
	labels := labelFixture(801)
	computeMagnetSignals(labels, map[uint][]float64{801: {1, 0}}, rows, tagEmb, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	m := labels[0].Magnet
	require.Equal(t, 5, m.AllSamples)
	require.Equal(t, 0, m.FailedSamples, "vector-less mismatch rows are invisible")
	require.True(t, m.Insufficient)
	require.NotNil(t, m.AllMean)
}

// ── description 事件性词根预筛（spec「描述污染预筛」，纯函数）──

func TestScreenDescription(t *testing.T) {
	// spec Scenario「description 事件性词根命中」。
	require.Equal(t, []string{"此次"}, screenDescription("位于中东的阿拉伯国家，此次派遣代表团前往伊朗进行外交磋商"))
	// 空 desc 不标记。
	require.Nil(t, screenDescription(""))
	// 未命中。
	require.Nil(t, screenDescription("伊朗首都"))
	// 多个词根、重复命中去重。
	require.Equal(t, []string{"此次", "报道", "期间"}, screenDescription("此次报道了该活动，期间又有新报道"))
	require.Equal(t, []string{"本次", "期间", "近日"}, screenDescription("本次期间近日"))
}

// 防护栏：固定闭集词根，不得从用户输入拼接（变体「正则元字符注入不适用」的
// 前半句——正则本身是常量，输入里的元字符不构成注入面）。
func TestScreenDescriptionPatternIsFixed(t *testing.T) {
	require.Equal(t, `此次|本次|该事件|报道|针对|期间|近日`, descEventPattern.String())
	// 纯正则元字符文本不命中词根（不命中即 nil，不 panic）。
	require.Nil(t, screenDescription(".*+?()[]{}|^$"))
}
