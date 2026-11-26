package auxlabel

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// --- fix-aux-label-dedup-ann tasks 2.2 / 2.4：L2 matcher 两段式 ---
//
// 向量用 3 维（裸 vector 列不限维度）：dim ≤ 4000 走 halfvec 召回分支，测试库里
// 没有对应索引时退化为顺序扫描，但 SQL 语义（halfvec 排序 + 原列 float32 精排 +
// Go 侧裁决）与生产一致。阈值来自 ai_settings（默认 0.95）。

// seedMergeCandidate 插入一行 active auxiliary 标签，merge_embedding 取 vec。
func seedMergeCandidate(t *testing.T, db *gorm.DB, slug string, vec []float64, refCount int) uint {
	t.Helper()
	pgVec := core.FloatsToPgVector(vec)
	row := models.SemanticLabel{
		Label: slug, Slug: slug, LabelType: "auxiliary", Status: "active",
		RefCount: refCount, MergeEmbedding: &pgVec,
	}
	require.NoError(t, db.Create(&row).Error)
	return row.ID
}

// setAuxDedupeSim 覆盖 L2 去重阈值（迁移 seed 的默认 0.95）。
func setAuxDedupeSim(t *testing.T, db *gorm.DB, value string) {
	t.Helper()
	setting := models.AISettings{Key: "auxiliary_label_dedupe_sim", Value: value}
	require.NoError(t, db.Where("key = ?", "auxiliary_label_dedupe_sim").Assign(setting).FirstOrCreate(&setting).Error)
}

func mergeVec(parts ...float64) []float64 { return parts }

// 阈值内多条 → RefCount 大者胜；RefCount 相同 → ID 小者胜。
func TestSQLMergeMatcherPicksHighestRefCountThenLowestID(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	lowRef := seedMergeCandidate(t, db, "gpt-5-low", mergeVec(1, 0, 0), 1)
	highRef := seedMergeCandidate(t, db, "gpt-5-high", mergeVec(1, 0, 0), 9)
	sameRefA := seedMergeCandidate(t, db, "gpt-5-tie-a", mergeVec(1, 0, 0), 5)
	sameRefB := seedMergeCandidate(t, db, "gpt-5-tie-b", mergeVec(1, 0, 0), 5)
	require.Less(t, sameRefA, sameRefB, "fixture order: tie-a is the smaller id")

	best, err := sqlMergeMatcher(context.Background(), db, core.FloatsToPgVector(mergeVec(0.999, 0.001, 0)), mergeVec(0.999, 0.001, 0))

	require.NoError(t, err)
	require.NotNil(t, best)
	require.Equal(t, highRef, best.ID, "highest RefCount wins among candidates above the threshold")
	require.NotEqual(t, lowRef, best.ID)

	// RefCount 相同时取 ID 小者：把唯一的高 RefCount 候选降到同值组之外再查一次。
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", highRef).Update("ref_count", 1).Error)
	best, err = sqlMergeMatcher(context.Background(), db, core.FloatsToPgVector(mergeVec(0.999, 0.001, 0)), mergeVec(0.999, 0.001, 0))
	require.NoError(t, err)
	require.NotNil(t, best)
	require.Equal(t, sameRefA, best.ID, "ties on RefCount go to the smaller ID")
}

// 无候选（空库）→ nil，且不报错（ANN 召回返回空集是合法结果）。
func TestSQLMergeMatcherNoCandidatesReturnsNil(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)

	best, err := sqlMergeMatcher(context.Background(), db, core.FloatsToPgVector(mergeVec(1, 0, 0)), mergeVec(1, 0, 0))

	require.NoError(t, err)
	require.Nil(t, best)
}

// 阈值外 → nil（L3 新建），阈值内 → 命中：只在阈值两侧的一个样本上翻转。
func TestSQLMergeMatcherThresholdGate(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	rowID := seedMergeCandidate(t, db, "prompt-engineering", mergeVec(1, 0, 0), 3)

	// cosine("提示词工程") ≈ 0.85：默认 0.95 下不命中。
	below, err := sqlMergeMatcher(context.Background(), db, core.FloatsToPgVector(mergeVec(0.85, 0.53, 0)), mergeVec(0.85, 0.53, 0))
	require.NoError(t, err)
	require.Nil(t, below, "cosine ~0.85 is below the 0.95 default threshold")

	setAuxDedupeSim(t, db, "0.80")
	above, err := sqlMergeMatcher(context.Background(), db, core.FloatsToPgVector(mergeVec(0.85, 0.53, 0)), mergeVec(0.85, 0.53, 0))
	require.NoError(t, err)
	require.NotNil(t, above, "cosine ~0.85 is above the configured 0.80 threshold")
	require.Equal(t, rowID, above.ID)
}

// 阈值判定只看「精排后的精确距离」：构造一个 halfvec（fp16）与精确 float32
// 分别落在阈值两侧的样本，阈值取二者中点 —— 命中说明裁决用的是原列精确距离，
// 而不是召回时的 halfvec 近似距离。
func TestSQLMergeMatcherThresholdUsesExactDistanceNotHalfvec(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)

	// 向量量级放大到 1024：fp16 在该量级的间距是 1.0（相对误差 ~5e-4），
	// 足以让小角度差异的 halfvec cosine 与精确 cosine 出现可分辨的偏差。
	base := mergeVec(1024, 0, 0)
	candidate := func(angle float64) []float64 {
		return mergeVec(1024, 1024*angle, 0)
	}

	var chosen []float64
	var exactSim, halfSim float64
	for _, angle := range []float64{0.32, 0.31, 0.30, 0.29, 0.28, 0.27, 0.26, 0.25} {
		vec := candidate(angle)
		pgVec := core.FloatsToPgVector(vec)
		require.NoError(t, db.Raw(`
			SELECT 1 - (?::vector <=> ?::vector) AS exact_sim,
			       1 - (?::halfvec(3) <=> ?::halfvec(3)) AS half_sim`,
			core.FloatsToPgVector(base), pgVec, core.FloatsToPgVector(base), pgVec).
			Row().Scan(&exactSim, &halfSim))
		// 需要「精确在阈值内、halfvec 在阈值外」的方向：exact > half。
		if exactSim > halfSim && exactSim-halfSim > 1e-6 {
			chosen = vec
			break
		}
	}
	require.NotNil(t, chosen, "fixture must produce a halfvec-vs-exact cosine gap (fp16 rounding)")

	midpoint := (exactSim + halfSim) / 2
	setAuxDedupeSim(t, db, fmt.Sprintf("%.12f", midpoint))
	seedMergeCandidate(t, db, "halfvec-gap", base, 1)

	best, err := sqlMergeMatcher(context.Background(), db, core.FloatsToPgVector(chosen), chosen)

	require.NoError(t, err)
	require.NotNil(t, best, "exact float32 similarity %.9f ≥ threshold %.9f > halfvec %.9f — must hit", exactSim, midpoint, halfSim)
}

// tasks 2.4：召回池被截断时用更大的 LIMIT + ef_search 重查一次再裁决。
// 构造 33 个互为近重复的标签：32 条低 RefCount + 1 条最高 RefCount，ANN 池
// （LIMIT 32）必然截掉一条；若不做兜底，选出的目标是池内的低 RefCount 行。
func TestSQLMergeMatcherTruncationFallbackWidensPool(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	query := mergeVec(1, 0, 0)

	for i := 0; i < auxMergeCandidateLimit; i++ {
		seedMergeCandidate(t, db, fmt.Sprintf("dup-%02d", i), mergeVec(1, float64(i)*1e-6, 0), 1)
	}
	// 第 33 条：与查询向量距离刚好排在第 33 位（角度稍大 → 排在池外），RefCount 最高。
	outsidePool := seedMergeCandidate(t, db, "dup-outside-pool", mergeVec(1, 0.0005, 0), 99)

	var calls []int
	restore := recallMergeCandidates
	recallMergeCandidates = func(tx *gorm.DB, mergePgVector string, dim, limit int) ([]mergeCandidate, error) {
		calls = append(calls, limit)
		return restore(tx, mergePgVector, dim, limit)
	}
	t.Cleanup(func() { recallMergeCandidates = restore })

	best, err := sqlMergeMatcher(context.Background(), db, core.FloatsToPgVector(query), query)

	require.NoError(t, err)
	require.Equal(t, []int{auxMergeCandidateLimit, auxMergeCandidateLimitWide}, calls,
		"a saturated near-duplicate pool must trigger exactly one wider re-query")
	require.NotNil(t, best)
	require.Equal(t, outsidePool, best.ID, "the wider pool must surface the higher-RefCount target beyond the first 32")
}

// 未饱和（候选不足 LIMIT）时不得触发兜底：普通批次仍是一次召回。
func TestSQLMergeMatcherNoFallbackWhenPoolNotSaturated(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	query := mergeVec(1, 0, 0)
	seedMergeCandidate(t, db, "single-duplicate", mergeVec(1, 0.001, 0), 7)

	var calls []int
	restore := recallMergeCandidates
	recallMergeCandidates = func(tx *gorm.DB, mergePgVector string, dim, limit int) ([]mergeCandidate, error) {
		calls = append(calls, limit)
		return restore(tx, mergePgVector, dim, limit)
	}
	t.Cleanup(func() { recallMergeCandidates = restore })

	best, err := sqlMergeMatcher(context.Background(), db, core.FloatsToPgVector(query), query)

	require.NoError(t, err)
	require.NotNil(t, best)
	require.Equal(t, []int{auxMergeCandidateLimit}, calls, "a non-saturated pool must not re-query")
}

// 召回 SQL 必须与部分索引谓词逐字一致（否则规划器不用部分索引），且 typmod 带维度；
// 超 halfvec 上限（>4000）退化为原列精确扫描。
func TestMergeRecallSQLShape(t *testing.T) {
	sql := mergeRecallSQL(2560)
	for _, want := range []string{
		database.SemanticLabelMergeHalfvecIndexPredicate,
		"ORDER BY merge_embedding::halfvec(2560) <=> ?::halfvec(2560)",
	} {
		require.Contains(t, sql, want)
	}

	sql = mergeRecallSQL(4096)
	require.NotContains(t, sql, "halfvec", "dims above the halfvec limit must not use a halfvec cast")
	require.Contains(t, sql, "ORDER BY merge_embedding <=> ?::vector")
	require.Contains(t, sql, database.SemanticLabelMergeHalfvecIndexPredicate)
}
