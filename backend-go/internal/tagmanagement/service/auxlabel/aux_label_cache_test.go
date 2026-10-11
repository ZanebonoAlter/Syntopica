package auxlabel

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// --- fix-aux-label-idle-tx-cache design D2：缓存失效增量化 ---

// warmAuxCacheForTest 直接把包级缓存置为「热」状态（模拟一次全量加载的产物），
// 避免测试依赖 L2 匹配路径的隐式加载时机。返回便于断言的时间戳快照。
func warmAuxCacheForTest(t *testing.T, labels []models.SemanticLabel, embeddings map[uint][]float64) (labelsAt, embeddingsAt time.Time) {
	t.Helper()
	now := time.Now()
	packageAuxLabelCache.mu.Lock()
	packageAuxLabelCache.activeLabels = labels
	packageAuxLabelCache.activeLabelsAt = now
	packageAuxLabelCache.mergeEmbeddings = embeddings
	packageAuxLabelCache.mergeEmbeddingsAt = now
	packageAuxLabelCache.mu.Unlock()
	return now, now
}

// auxCacheMergeEmbeddingsLenForTest 在锁内读取当前 mergeEmbeddings 长度。
// （hotfix 后 map 不再被原地写入，len 快照即稳定值；保留锁内读防未来回归。）
func auxCacheMergeEmbeddingsLenForTest() int {
	packageAuxLabelCache.mu.RLock()
	defer packageAuxLabelCache.mu.RUnlock()
	return len(packageAuxLabelCache.mergeEmbeddings)
}

func cacheSnapshotForTest() (labels []models.SemanticLabel, labelsAt time.Time, embeddings map[uint][]float64, embeddingsAt time.Time) {
	packageAuxLabelCache.mu.RLock()
	defer packageAuxLabelCache.mu.RUnlock()
	return packageAuxLabelCache.activeLabels, packageAuxLabelCache.activeLabelsAt,
		packageAuxLabelCache.mergeEmbeddings, packageAuxLabelCache.mergeEmbeddingsAt
}

// 任务 1.1（hotfix 2026-10-07 后语义）：L3 新建后不全量重拉（mergeEmbeddingsAt
// 不重置）、activeLabels 线性 append；mergeEmbeddings map **不原位写入**——
// sqlMergeMatcher 持引用无锁读，原位写会触发 fatal error: concurrent map
// read and map write（不可恢复，杀进程；2026-10-07 生产实测）。新向量由
// 10min TTL 全量重拉带入。历史 invalidateOnCreate 全清同样不许回来。
func TestAuxLabelCacheCreateIsIncremental(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	seed := models.SemanticLabel{Label: "既有标签", Slug: "jiyou", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&seed).Error)
	_, embeddingsAt := warmAuxCacheForTest(t, []models.SemanticLabel{{ID: seed.ID, Label: seed.Label, Slug: seed.Slug, LabelType: "auxiliary", Status: "active"}}, map[uint][]float64{seed.ID: {1, 0, 0}})

	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{
		"全新标签甲": {0, 1, 0},
		"全新标签乙": {0, 0, 1},
		"全新标签丙": {1, 1, 0},
	}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	// 连续 3 次 L3 新建：map 大小不变（不原位写）、时间戳全程不动、
	// activeLabels 线性 +1。
	labelsWarm, _, _, _ := cacheSnapshotForTest()
	labelsLen := len(labelsWarm)
	for _, name := range []string{"全新标签甲", "全新标签乙", "全新标签丙"} {
		beforeLen := auxCacheMergeEmbeddingsLenForTest()
		_, err := service.ResolveAuxiliaryLabel(context.Background(), name, "")
		require.NoError(t, err)
		labels, _, embeddings, at := cacheSnapshotForTest()
		require.Len(t, embeddings, beforeLen, "map must NOT be mutated in place (lock-free readers would fatal); freshness is the TTL's job")
		require.Equal(t, embeddingsAt, at, "mergeEmbeddingsAt must NOT be reset on create (no full re-pull)")
		labelsLen++
		require.Len(t, labels, labelsLen, "activeLabels grows by exactly one per create (safe slice append)")
	}
}

// 任务 1.1 补充：新建标签的轻量副本进入 activeLabels（列集合对齐——向量字段为 nil）。
func TestAuxLabelCacheCreateAppendsLightweightCopy(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	warmAuxCacheForTest(t, []models.SemanticLabel{}, map[uint][]float64{})
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"轻量副本": {0, 1, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	created, err := service.ResolveAuxiliaryLabel(context.Background(), "轻量副本", "描述文本")
	require.NoError(t, err)

	labels, _, _, _ := cacheSnapshotForTest()
	require.Len(t, labels, 1)
	require.Equal(t, created.ID, labels[0].ID)
	require.Equal(t, "轻量副本", labels[0].Label)
	require.Equal(t, "描述文本", labels[0].Description)
	require.Nil(t, labels[0].Embedding, "lightweight copy must not carry the storage vector")
	require.Nil(t, labels[0].MergeEmbedding, "lightweight copy must not carry the merge vector")
}

// 任务 1.2：DisableAuxiliaryLabel 后条目删除、无全量重拉（mergeEmbeddingsAt 不变）。
func TestAuxLabelCacheDisableIsIncremental(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	vec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	keep := models.SemanticLabel{Label: "保留", Slug: "keep", LabelType: "auxiliary", Status: "active", MergeEmbedding: &vec}
	drop := models.SemanticLabel{Label: "禁用", Slug: "drop", LabelType: "auxiliary", Status: "active", MergeEmbedding: &vec}
	require.NoError(t, db.Create(&keep).Error)
	require.NoError(t, db.Create(&drop).Error)
	warmAuxCacheForTest(t,
		[]models.SemanticLabel{{ID: keep.ID, Label: keep.Label, Slug: keep.Slug}, {ID: drop.ID, Label: drop.Label, Slug: drop.Slug}},
		map[uint][]float64{keep.ID: {1}, drop.ID: {2}})
	_, embeddingsAt, _, _ := cacheSnapshotForTest()

	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
	require.NoError(t, service.DisableAuxiliaryLabel(context.Background(), drop.ID))

	labels, _, embeddings, at := cacheSnapshotForTest()
	require.Contains(t, embeddings, drop.ID, "in-place delete is a fatal-race hazard for lock-free readers — stale entry must linger until TTL reload")
	require.Contains(t, embeddings, keep.ID, "unrelated entries must survive")
	require.Equal(t, embeddingsAt, at, "disable must not reset mergeEmbeddingsAt (no full re-pull)")
	require.Len(t, labels, 1)
	require.Equal(t, keep.ID, labels[0].ID, "disabled label must be filtered out of activeLabels")
}

// 任务 1.2：MergeAuxiliaryLabelAlias 的 source 禁用路径——source 向量删除、
// target 以最终 aliases 刷新、mergeEmbeddingsAt 不变。
func TestAuxLabelCacheMergeAliasIsIncremental(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	vec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	target := models.SemanticLabel{Label: "目标", Slug: "target", LabelType: "auxiliary", Status: "active", MergeEmbedding: &vec, Aliases: []string{"旧别名"}}
	source := models.SemanticLabel{Label: "源头", Slug: "source", LabelType: "auxiliary", Status: "active", MergeEmbedding: &vec, Aliases: []string{"源别名"}}
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&source).Error)
	warmAuxCacheForTest(t,
		[]models.SemanticLabel{{ID: target.ID, Label: target.Label, Slug: target.Slug}, {ID: source.ID, Label: source.Label, Slug: source.Slug}},
		map[uint][]float64{target.ID: {1}, source.ID: {2}})
	_, embeddingsAt, _, _ := cacheSnapshotForTest()

	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
	require.NoError(t, service.MergeAuxiliaryLabelAlias(context.Background(), source.ID, target.ID))

	labels, _, embeddings, at := cacheSnapshotForTest()
	require.Contains(t, embeddings, source.ID, "in-place delete is a fatal-race hazard for lock-free readers — stale entry must linger until TTL reload")
	require.Contains(t, embeddings, target.ID)
	require.Equal(t, embeddingsAt, at, "merge must not reset mergeEmbeddingsAt")
	require.Len(t, labels, 1, "source filtered out, target retained")
	require.Equal(t, target.ID, labels[0].ID)
	require.Contains(t, labels[0].Aliases, "源头", "target cached copy must be refreshed with its final aliases")
}

// 任务 1.3：alias 变更路径只清 activeLabels，mergeEmbeddings 保留（现状回归）。
func TestAuxLabelCacheAliasChangePreservesMergeEmbeddings(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	existingVec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	existing := models.SemanticLabel{Label: "高市早苗", Slug: "takaichi", LabelType: "auxiliary", Status: "active", MergeEmbedding: &existingVec}
	require.NoError(t, db.Create(&existing).Error)
	warmAuxCacheForTest(t,
		[]models.SemanticLabel{{ID: existing.ID, Label: existing.Label, Slug: existing.Slug}},
		map[uint][]float64{existing.ID: testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)})
	_, embeddingsAt, _, _ := cacheSnapshotForTest()

	// merge 向量与既有 cosine ~0.999 ≥ 0.95 → L2 命中走 alias 追加路径
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"高市早苗政府": {0.999, 0.001, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)
	label, err := service.ResolveAuxiliaryLabel(context.Background(), "高市早苗政府", "")
	require.NoError(t, err)
	require.Equal(t, existing.ID, label.ID)

	labels, _, embeddings, at := cacheSnapshotForTest()
	require.Contains(t, label.Aliases, "高市早苗政府")
	require.Empty(t, labels, "alias change clears activeLabels (cheap list reload)")
	require.Contains(t, embeddings, existing.ID, "alias change must preserve mergeEmbeddings")
	require.Equal(t, embeddingsAt, at, "alias change must not reset mergeEmbeddingsAt")
}

// 任务 1.4：TTL 过期兜底——9:59 仍命中缓存 / 10:01 过期重拉。
func TestAuxLabelCacheTTLExpiryReloads(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	seedVec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	seed := models.SemanticLabel{Label: "过期样本", Slug: "expiry", LabelType: "auxiliary", Status: "active", MergeEmbedding: &seedVec}
	require.NoError(t, db.Create(&seed).Error)

	sentinel := map[uint][]float64{999: {1}}
	warmAuxCacheForTest(t, nil, sentinel)

	ctx := context.Background()

	// 9:59 —— 未过期，命中缓存（哨兵仍在，未触库）。
	packageAuxLabelCache.mu.Lock()
	packageAuxLabelCache.mergeEmbeddingsAt = time.Now().Add(-auxMergeEmbeddingsTTL + time.Second)
	packageAuxLabelCache.mu.Unlock()
	got, err := packageAuxLabelCacheGetOrLoad(ctx, db)
	require.NoError(t, err)
	require.True(t, &got[999][0] == &sentinel[999][0], "9:59 boundary must hit the cached map (no reload)")
	require.Len(t, got, 1)

	// 10:01 —— 过期，重拉（哨兵消失，种子行入库数据出现）。
	packageAuxLabelCache.mu.Lock()
	packageAuxLabelCache.mergeEmbeddingsAt = time.Now().Add(-auxMergeEmbeddingsTTL - time.Second)
	packageAuxLabelCache.mu.Unlock()
	got, err = packageAuxLabelCacheGetOrLoad(ctx, db)
	require.NoError(t, err)
	require.NotContains(t, got, 999, "10:01 boundary must reload from DB (sentinel gone)")
	require.Contains(t, got, seed.ID)
}
