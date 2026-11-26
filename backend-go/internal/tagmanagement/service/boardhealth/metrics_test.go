package boardhealth

import (
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
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

// ── 磁铁信号（fix-board-health-magnet-signal design D1/D2，spec「标签分诊信号计算」）──
// 用例来源：test-cases.md §2 继承与调整表 + §7.1 白盒分支表。口径（新）：
// 失败子集（direction_mismatch=true）均值显著**高于**健康子集
// （direction_mismatch=false）均值才成立；两侧样本各自达标；all 侧保留
// 但退出判定。样本不足/向量缺失不硬凑。

// magnetRowsSuspect builds nGood healthy mounts orthogonal to the label
// (cos=0) and nFailed failed mounts aligned with it (cos=1) — the 磁铁形态
// 「把跑题的挂载吸过来」：failed side higher than the healthy side.
func magnetRowsSuspect(tagEmb map[uint][]float64, nGood, nFailed int) []matchedRow {
	rows := make([]matchedRow, 0, nGood+nFailed)
	id := uint(1)
	for i := 0; i < nGood; i++ {
		tagEmb[id] = []float64{0, 1}
		rows = append(rows, matchedRow{TopicTagID: id})
		id++
	}
	for i := 0; i < nFailed; i++ {
		tagEmb[id] = []float64{1, 0}
		rows = append(rows, matchedRow{TopicTagID: id, DirectionMismatch: true})
		id++
	}
	return rows
}

// magnetRowsCoreSubject builds nGood healthy mounts aligned with the label
// (cos=1) and nFailed failed mounts orthogonal to it (cos=0) — the 核心主体
// 形态：失败挂载与本板方向不符、与核心主体天然不相似，failed side LOWER.
func magnetRowsCoreSubject(tagEmb map[uint][]float64, nGood, nFailed int) []matchedRow {
	rows := make([]matchedRow, 0, nGood+nFailed)
	id := uint(1)
	for i := 0; i < nGood; i++ {
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
	// spec Scenario「磁铁信号按失败子集相对健康子集判定」：失败子集均值显著
	// 高于健康子集均值、两侧样本各自达标 → 嫌疑成立，携带两侧均值与样本数。
	// 夹具：5 个健康挂载（与标签正交 cos=0）+ 5 个失败挂载（与标签同向 cos=1）。
	tagEmb := map[uint][]float64{}
	rows := magnetRowsSuspect(tagEmb, 5, 5)
	auxEmb := map[uint][]float64{101: {1, 0}}
	labels := labelFixture(101)
	computeMagnetSignals(labels, auxEmb, rows, tagEmb, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})

	m := labels[0].Magnet
	require.NotNil(t, m)
	require.False(t, m.Insufficient)
	require.True(t, m.Suspicious)
	require.InDelta(t, 0.0, *m.GoodMean, 1e-9)
	require.InDelta(t, 1.0, *m.FailedMean, 1e-9)
	require.Equal(t, 5, m.GoodSamples)
	require.Equal(t, 5, m.FailedSamples)
	// all 侧保留但退出判定：全体均值 = (5×0 + 5×1)/10 = 0.5。
	require.InDelta(t, 0.5, *m.AllMean, 1e-9)
	require.Equal(t, 10, m.AllSamples)
	// 口径标记：新口径（失败子集 vs 健康子集）——消费方按它区分「旧快照」与
	// 「新快照但健康侧不可算」（design D12 / review R-4）。
	require.Equal(t, tagmodels.BoardHealthMagnetCaliberHealthySubset, m.Caliber)
}

func TestComputeMagnetSignalsCoreSubjectShape(t *testing.T) {
	// 白盒 B7 + 2026-11-09 事故回归锚：核心主体与健康挂载贴合度高、与失败
	// 子集不相似 → 磁铁嫌疑 MUST NOT 成立（旧口径 all-failed 会判成立，
	// 于是全板 41/47 成立、伊朗被系统性点名）。
	tagEmb := map[uint][]float64{}
	rows := magnetRowsCoreSubject(tagEmb, 10, 5)
	labels := labelFixture(901)
	computeMagnetSignals(labels, map[uint][]float64{901: {1, 0}}, rows, tagEmb, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	m := labels[0].Magnet
	require.False(t, m.Insufficient)
	require.False(t, m.Suspicious, "失败子集均值低于健康子集 → 不成立")
	require.InDelta(t, 1.0, *m.GoodMean, 1e-9)
	require.InDelta(t, 0.0, *m.FailedMean, 1e-9)
}

func TestComputeMagnetSignalsGapBoundary(t *testing.T) {
	// 白盒「gap=0.08±ε」：阈值 >= 语义。新 gap = failed_mean - good_mean。
	// 夹具精确可算：5 个健康子集向量 cos=0（{0,1} 基）、5 个失败子集向量
	// cos=0.6（{0.6,0.8} 基）→ gap=0.6；用计算出的 gap 卡阈值两侧。
	tagEmb := map[uint][]float64{}
	rows := make([]matchedRow, 0, 10)
	id := uint(1)
	for i := 0; i < 5; i++ {
		tagEmb[id] = []float64{0, 1} // 健康侧：与标签正交 cos=0
		rows = append(rows, matchedRow{TopicTagID: id})
		id++
	}
	for i := 0; i < 5; i++ {
		tagEmb[id] = []float64{0.6, 0.8} // 失败侧：与标签 cosine=0.6
		rows = append(rows, matchedRow{TopicTagID: id, DirectionMismatch: true})
		id++
	}
	auxEmb := map[uint][]float64{201: {1, 0}}
	labels := labelFixture(201)
	computeMagnetSignals(labels, auxEmb, rows, tagEmb, Config{MinSampleSize: 5, MagnetMinFailed: 5, MagnetGap: 0.08})
	m := labels[0].Magnet
	require.InDelta(t, 0.0, *m.GoodMean, 1e-9)
	require.InDelta(t, 0.6, *m.FailedMean, 1e-9)
	gap := *m.FailedMean - *m.GoodMean
	require.InDelta(t, 0.6, gap, 1e-9)
	require.True(t, m.Suspicious, "gap 0.6 ≥ 0.08 → suspicious")

	// 阈值恰等于 gap → 成立（>= 语义）；阈值高一丝 → 不成立。
	computeMagnetSignals(labels, auxEmb, rows, tagEmb, Config{MinSampleSize: 5, MagnetMinFailed: 5, MagnetGap: gap})
	require.True(t, labels[0].Magnet.Suspicious, "gap == threshold → suspicious (>=)")
	computeMagnetSignals(labels, auxEmb, rows, tagEmb, Config{MinSampleSize: 5, MagnetMinFailed: 5, MagnetGap: gap + 1e-9})
	require.False(t, labels[0].Magnet.Suspicious, "gap < threshold → not suspicious")
}

func TestComputeMagnetSignalsFailedSamplesBoundary(t *testing.T) {
	// 白盒「失败样本 4/5」：failed=4 < min_failed=5 → insufficient、不判嫌疑。
	tagEmb := map[uint][]float64{}
	rows := magnetRowsSuspect(tagEmb, 10, 4)
	labels := labelFixture(301)
	computeMagnetSignals(labels, map[uint][]float64{301: {1, 0}}, rows, tagEmb, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	m := labels[0].Magnet
	require.True(t, m.Insufficient, "failed samples 4 < 5 → insufficient")
	require.False(t, m.Suspicious, "样本不足不硬凑结论")
	require.NotNil(t, m.GoodMean, "观测均值仍记录（真实观测，非结论）")
	// 口径标记与样本充足度无关：insufficient 也写新口径（消费方据此区分
	// 「旧快照」与「新快照但健康侧不可算」）。
	require.Equal(t, tagmodels.BoardHealthMagnetCaliberHealthySubset, m.Caliber)

	// failed=5 恰达标 → 可判（磁铁形态：failed 高于 good）。
	tagEmb = map[uint][]float64{}
	rows = magnetRowsSuspect(tagEmb, 10, 5)
	labels = labelFixture(301)
	computeMagnetSignals(labels, map[uint][]float64{301: {1, 0}}, rows, tagEmb, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	require.False(t, labels[0].Magnet.Insufficient)
	require.True(t, labels[0].Magnet.Suspicious)
}

func TestComputeMagnetSignalsHealthySamplesBoundary(t *testing.T) {
	// spec Scenario「健康子集样本不足不强判」：good=4 < min_sample_size=5
	// （failed 侧 6 个达标）→ 两侧各自达标才判，不足即 insufficient。
	tagEmb := map[uint][]float64{}
	rows := magnetRowsSuspect(tagEmb, 4, 6)
	labels := labelFixture(302)
	computeMagnetSignals(labels, map[uint][]float64{302: {1, 0}}, rows, tagEmb, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	m := labels[0].Magnet
	require.True(t, m.Insufficient, "健康侧样本不足 → insufficient")
	require.False(t, m.Suspicious, "不硬凑嫌疑结论")
	require.Equal(t, 4, m.GoodSamples)
	require.Equal(t, 6, m.FailedSamples)
	// 均值仍是真实观测（不因不足而抹掉）。
	require.NotNil(t, m.GoodMean)
	require.NotNil(t, m.FailedMean)
}

func TestComputeMagnetSignalsInsufficientScenarios(t *testing.T) {
	// spec Scenario「失败子集样本不足不强判」：全板仅 2 个 mismatch 行 →
	// 全部构成标签磁铁信号 insufficient。
	tagEmb := map[uint][]float64{}
	rows := magnetRowsSuspect(tagEmb, 8, 2)
	labels := labelFixture(401, 402)
	computeMagnetSignals(labels, map[uint][]float64{401: {1, 0}, 402: {1, 0}}, rows, tagEmb, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	for _, l := range labels {
		require.True(t, l.Magnet.Insufficient)
		require.False(t, l.Magnet.Suspicious)
	}

	// spec Scenario「向量缺失信号不硬算」：标签自身 embedding 缺失 → 两侧
	// 均值不可算（nil），insufficient。
	tagEmb2 := map[uint][]float64{}
	rows2 := magnetRowsSuspect(tagEmb2, 10, 5)
	labels2 := labelFixture(501)
	computeMagnetSignals(labels2, map[uint][]float64{}, rows2, tagEmb2, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	m := labels2[0].Magnet
	require.True(t, m.Insufficient)
	require.Nil(t, m.AllMean)
	require.Nil(t, m.GoodMean)
	require.Nil(t, m.FailedMean)
	require.False(t, m.Suspicious, "缺向量不按 0 参与嫌疑判定")

	// 白盒「任侧 0」：全部挂载 tag 无 identity 向量 → 两侧样本皆 0
	//（nil 均值），insufficient；tag 侧缺向量对信号不可见。
	rows3 := []matchedRow{{TopicTagID: 1}, {TopicTagID: 2, DirectionMismatch: true}}
	labels3 := labelFixture(601)
	computeMagnetSignals(labels3, map[uint][]float64{601: {1, 0}}, rows3, map[uint][]float64{}, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	m3 := labels3[0].Magnet
	require.Equal(t, 0, m3.AllSamples)
	require.Equal(t, 0, m3.GoodSamples)
	require.Equal(t, 0, m3.FailedSamples)
	require.Nil(t, m3.AllMean)
	require.True(t, m3.Insufficient)

	// 白盒「健康侧 0」：没有方向通过的挂载行 → 两侧样本皆不足，insufficient。
	tagEmb4 := map[uint][]float64{}
	rows4 := magnetRowsSuspect(tagEmb4, 0, 3)
	labels4 := labelFixture(701)
	computeMagnetSignals(labels4, map[uint][]float64{701: {1, 0}}, rows4, tagEmb4, Config{MinSampleSize: 5, MagnetGap: 0.08, MagnetMinFailed: 5})
	require.True(t, labels4[0].Magnet.Insufficient)
	require.False(t, labels4[0].Magnet.Suspicious)
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
	require.Equal(t, 5, m.GoodSamples)
	require.Equal(t, 0, m.FailedSamples, "vector-less mismatch rows are invisible")
	require.True(t, m.Insufficient)
	require.NotNil(t, m.AllMean)
}

// ── 核心主体标记（fix-board-health-magnet-signal design D3 / review R-6，
// spec「核心主体按版块文本判定」）────────────────────────────

// TestComputeCoreSubjects —— 核心主体判定分支表（test-cases.md §7.2
// C1–C8）：description / aliases 命中即成立（不限字数）；**仅**标题命中需
// 标签名归一后 ≥ 3 字符（2 字短词既可能是简称「美伊」也可能是泛化地域词
// 「中东」，不据标题推断全名）；三处均不出现 → false；nil 板 / 空标签名 →
// false 且不 panic。
func TestComputeCoreSubjects(t *testing.T) {
	cases := []struct {
		name  string
		board *models.SemanticLabel
		label string
		want  bool
	}{
		{"C1 description 命中（2 字标签名也算成立）", &models.SemanticLabel{Label: "某版块", Description: "本板覆盖伊朗局势"}, "伊朗", true},
		{"C1 description 命中（3 字标签名）", &models.SemanticLabel{Label: "某版块", Description: "本板覆盖中东地缘政治"}, "中东地缘政治", true},
		{"C1 description 命中优先于标题长度约束", &models.SemanticLabel{Label: "伊朗局势观察", Description: "本板讨论伊朗"}, "伊朗", true},
		{"C2 aliases 命中", &models.SemanticLabel{Label: "某版块", Aliases: []string{"中东观察", "伊朗专辑"}}, "伊朗", true},
		{"C2 aliases 命中优先于标题长度约束", &models.SemanticLabel{Label: "伊朗局势观察", Aliases: []string{"伊朗"}}, "伊朗", true},
		{"C3 仅标题命中且标签名 ≥ 3 字", &models.SemanticLabel{Label: "中东地缘政治与美伊关系"}, "中东地缘", true},
		{"C4 仅标题命中且标签名 = 2 字（简称不算）", &models.SemanticLabel{Label: "伊朗局势观察"}, "伊朗", false},
		{"C4 短泛化词命中标题不算核心主体", &models.SemanticLabel{Label: "中东地缘政治"}, "中东", false},
		{"C5 三处均不命中", &models.SemanticLabel{Label: "某版块", Description: "无关描述", Aliases: []string{"别的专辑"}}, "伊朗", false},
		{"C6 大小写归一命中", &models.SemanticLabel{Label: "IRAN 观察"}, "iran", true},
		{"C6 空白归一命中（description）", &models.SemanticLabel{Description: "中 东 地 缘"}, "中东地缘", true},
		{"C7 board 为 nil", nil, "伊朗", false},
		{"C7 三个文本全空", &models.SemanticLabel{}, "伊朗", false},
		{"C8 标签名为空串", &models.SemanticLabel{Label: "伊朗局势观察", Description: "本板覆盖伊朗局势"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			labels := []tagmodels.BoardHealthLabelMetrics{{AuxID: 262, Label: tc.label}}
			computeCoreSubjects(tc.board, labels)
			require.Equal(t, tc.want, labels[0].CoreSubject)
		})
	}
}

// TestComputeCoreSubjectsTitleAbbreviation —— 简称形态的事实取证（2026-11-09
// 板块 1974 真库核对：title=「中东地缘政治与美伊关系」、aliases=[]、
// description=''）：标题里出现的是**简称**「美伊」，不是「伊朗」——纯子串
// 规则（design D3 契约：文本子串匹配、不做向量/模糊推断）判不了「伊朗」。
// 本板要保护「伊朗」靠 design D10 给版块补定义式 description（点明伊朗/
// 美伊为核心主体）——补齐后 description 含「伊朗」→ core_subject=true。
//
// 这条用例固定住「简称 ≠ 全名」的边界：后续维护者不得把规则改成模糊重叠
// （那会让任何共享一个字的标签都变成核心主体）。
func TestComputeCoreSubjectsTitleAbbreviation(t *testing.T) {
	board := &models.SemanticLabel{Label: "中东地缘政治与美伊关系", Aliases: []string{}, Description: ""}
	labels := []tagmodels.BoardHealthLabelMetrics{
		{AuxID: 262, Label: "伊朗"},
		{AuxID: 263, Label: "卡塔尔"},
	}
	computeCoreSubjects(board, labels)
	require.False(t, labels[0].CoreSubject, "标题含的是简称「美伊」，不含「伊朗」→ 不靠标题标记")
	require.False(t, labels[1].CoreSubject)

	// D10 补齐 description 后 → 命中（这才是本板保护伊朗的实际路径）。
	board.Description = "本板讨论中东地缘政治，核心主体是伊朗与美伊关系。"
	labels = []tagmodels.BoardHealthLabelMetrics{{AuxID: 262, Label: "伊朗"}}
	computeCoreSubjects(board, labels)
	require.True(t, labels[0].CoreSubject, "description 补齐后 → 核心主体")
}

// TestComputeCoreSubjectsVariants —— 变体走查（test-cases.md §4.1）：空名 /
// 纯空白（含全角空格）不参与匹配；大小写归一后英文标签可命中；空白归一后
// 中文标签可命中；仅标题命中且标签名 2 字 → 不成立。
func TestComputeCoreSubjectsVariants(t *testing.T) {
	// 变体走查（test-cases.md §4.1）：空名 / 纯空白（含全角空格）不参与
	// 匹配；大小写归一后英文标签可命中；空白归一后中文标签可命中。
	board := &models.SemanticLabel{Label: "IRAN 观察", Description: "中 东 地 缘"}
	cases := []struct {
		name  string
		label string
		want  bool
	}{
		{"空标签名", "", false},
		{"纯空白标签名", "   ", false},
		{"全角空格标签名", "　", false},
		{"纯分隔符标签名", "-", false},
		{"英文大小写归一", "iran", true},
		{"中文空白归一", "中东地缘", true},
		{"不命中", "俄罗斯", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			labels := []tagmodels.BoardHealthLabelMetrics{{AuxID: 1, Label: tc.label}}
			computeCoreSubjects(board, labels)
			require.Equal(t, tc.want, labels[0].CoreSubject)
		})
	}

	// 仅标题命中 + 2 字标签名 → 不成立（简称/泛化词不据标题推断）。
	titleOnly := &models.SemanticLabel{Label: "伊朗局势观察"}
	labels := []tagmodels.BoardHealthLabelMetrics{{AuxID: 1, Label: "伊朗"}}
	computeCoreSubjects(titleOnly, labels)
	require.False(t, labels[0].CoreSubject, "仅标题命中且 2 字 → 非核心主体")

	// nil 板块不 panic（防御）。
	labels = []tagmodels.BoardHealthLabelMetrics{{AuxID: 1, Label: "伊朗"}}
	computeCoreSubjects(nil, labels)
	require.False(t, labels[0].CoreSubject)
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
