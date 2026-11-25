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

// --- fix-aux-label-dedup-ann design D4：merge 向量缓存已删除，缓存只剩 ---
// --- active 标签轻量列表 + 它的三个 L1 精确匹配索引（同生命周期、同 TTL）。 ---

// warmAuxCacheForTest 直接把包级缓存置为「热」状态（模拟一次全量加载的产物），
// 避免测试依赖 L2 匹配路径的隐式加载时机。返回便于断言的时间戳快照。
func warmAuxCacheForTest(t *testing.T, labels []models.SemanticLabel) time.Time {
	t.Helper()
	now := time.Now()
	packageAuxLabelCache.mu.Lock()
	packageAuxLabelCache.active = newAuxLabelIndex(labels)
	packageAuxLabelCache.activeAt = now
	packageAuxLabelCache.mu.Unlock()
	return now
}

// cacheSnapshotForTest 在锁内读取当前 active 列表快照与加载时间。
func cacheSnapshotForTest() (labels []models.SemanticLabel, at time.Time) {
	packageAuxLabelCache.mu.RLock()
	defer packageAuxLabelCache.mu.RUnlock()
	if packageAuxLabelCache.active == nil {
		return nil, packageAuxLabelCache.activeAt
	}
	return packageAuxLabelCache.active.labels, packageAuxLabelCache.activeAt
}

// 任务 3.1：L3 新建后不全量重拉（activeAt 不重置）、activeLabels 线性 append，
// 且新行立即可被 L1 索引命中（旧实现靠缓存回灌 + 线性扫达到同一效果）。
func TestAuxLabelCacheCreateIsIncremental(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	seed := models.SemanticLabel{Label: "既有标签", Slug: "jiyou", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&seed).Error)
	activeAt := warmAuxCacheForTest(t, []models.SemanticLabel{{ID: seed.ID, Label: seed.Label, Slug: seed.Slug, LabelType: "auxiliary", Status: "active"}})

	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{
		"全新标签甲": {0, 1, 0},
		"全新标签乙": {0, 0, 1},
		"全新标签丙": {1, 1, 0},
	}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	// 连续 3 次 L3 新建：加载时间戳全程不动、activeLabels 线性 +1。
	labelsWarm, _ := cacheSnapshotForTest()
	labelsLen := len(labelsWarm)
	for _, name := range []string{"全新标签甲", "全新标签乙", "全新标签丙"} {
		_, err := service.ResolveAuxiliaryLabel(context.Background(), name, "")
		require.NoError(t, err)
		labels, at := cacheSnapshotForTest()
		require.Equal(t, activeAt, at, "activeAt must NOT be reset on create (no full re-pull)")
		labelsLen++
		require.Len(t, labels, labelsLen, "activeLabels grows by exactly one per create (safe slice append)")
	}

	// 新增行已进 L1 索引：同 slug 的后续请求直接复用，不再调 embedder。
	before := len(embedder.calls)
	label, err := service.ResolveAuxiliaryLabel(context.Background(), "全新标签甲", "")
	require.NoError(t, err)
	require.Equal(t, "全新标签甲", label.Label)
	require.Len(t, embedder.calls, before, "created row must be L1-matchable without a cache reload")
}

// 任务 3.1 补充：新建标签的轻量副本进入 activeLabels（列集合对齐——向量字段为 nil）。
func TestAuxLabelCacheCreateAppendsLightweightCopy(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	warmAuxCacheForTest(t, []models.SemanticLabel{})
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"轻量副本": {0, 1, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	created, err := service.ResolveAuxiliaryLabel(context.Background(), "轻量副本", "描述文本")
	require.NoError(t, err)

	labels, _ := cacheSnapshotForTest()
	require.Len(t, labels, 1)
	require.Equal(t, created.ID, labels[0].ID)
	require.Equal(t, "轻量副本", labels[0].Label)
	require.Equal(t, "描述文本", labels[0].Description)
	require.Nil(t, labels[0].Embedding, "lightweight copy must not carry the storage vector")
	require.Nil(t, labels[0].MergeEmbedding, "lightweight copy must not carry the merge vector")
}

// 任务 3.1：DisableAuxiliaryLabel 后条目删除、无全量重拉（activeAt 不变）。
func TestAuxLabelCacheDisableIsIncremental(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	vec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	keep := models.SemanticLabel{Label: "保留", Slug: "keep", LabelType: "auxiliary", Status: "active", MergeEmbedding: &vec}
	drop := models.SemanticLabel{Label: "禁用", Slug: "drop", LabelType: "auxiliary", Status: "active", MergeEmbedding: &vec}
	require.NoError(t, db.Create(&keep).Error)
	require.NoError(t, db.Create(&drop).Error)
	activeAt := warmAuxCacheForTest(t, []models.SemanticLabel{
		{ID: keep.ID, Label: keep.Label, Slug: keep.Slug},
		{ID: drop.ID, Label: drop.Label, Slug: drop.Slug},
	})

	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
	require.NoError(t, service.DisableAuxiliaryLabel(context.Background(), drop.ID))

	labels, at := cacheSnapshotForTest()
	require.Equal(t, activeAt, at, "disable must not reset activeAt (no full re-pull)")
	require.Len(t, labels, 1)
	require.Equal(t, keep.ID, labels[0].ID, "disabled label must be filtered out of activeLabels")
}

// 任务 3.1：MergeAuxiliaryLabelAlias 的 source 禁用路径——source 掉出列表、
// target 以最终 aliases 刷新并可被新别名 L1 命中、activeAt 不变。
func TestAuxLabelCacheMergeAliasIsIncremental(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	vec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	target := models.SemanticLabel{Label: "目标", Slug: "target", LabelType: "auxiliary", Status: "active", MergeEmbedding: &vec, Aliases: []string{"旧别名"}}
	source := models.SemanticLabel{Label: "源头", Slug: "source", LabelType: "auxiliary", Status: "active", MergeEmbedding: &vec, Aliases: []string{"源别名"}}
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&source).Error)
	activeAt := warmAuxCacheForTest(t, []models.SemanticLabel{
		{ID: target.ID, Label: target.Label, Slug: target.Slug, Aliases: []string{"旧别名"}},
		{ID: source.ID, Label: source.Label, Slug: source.Slug, Aliases: []string{"源别名"}},
	})

	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
	require.NoError(t, service.MergeAuxiliaryLabelAlias(context.Background(), source.ID, target.ID))

	labels, at := cacheSnapshotForTest()
	require.Equal(t, activeAt, at, "merge must not reset activeAt")
	require.Len(t, labels, 1, "source filtered out, target retained")
	require.Equal(t, target.ID, labels[0].ID)
	require.Contains(t, labels[0].Aliases, "源头", "target cached copy must be refreshed with its final aliases")

	// 合并进来的新别名已进 L1 索引（alias fold 口径）。
	service.cache.mu.RLock()
	idx := service.cache.active
	service.cache.mu.RUnlock()
	require.NotNil(t, idx)
	row, hit := idx.match("", "源头", "")
	require.True(t, hit, "merged alias must be L1-matchable from the refreshed index")
	require.Equal(t, target.ID, row.ID)
}

// 任务 3.1：alias 变更路径只清 active 列表（下次加载重拉）。
func TestAuxLabelCacheAliasChangeInvalidatesActiveList(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	existingVec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	existing := models.SemanticLabel{Label: "高市早苗", Slug: "takaichi", LabelType: "auxiliary", Status: "active", MergeEmbedding: &existingVec}
	require.NoError(t, db.Create(&existing).Error)
	warmAuxCacheForTest(t, []models.SemanticLabel{{ID: existing.ID, Label: existing.Label, Slug: existing.Slug}})

	// merge 向量与既有 cosine ~0.999 ≥ 0.95 → L2 命中走 alias 追加路径
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"高市早苗政府": {0.999, 0.001, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)
	label, err := service.ResolveAuxiliaryLabel(context.Background(), "高市早苗政府", "")
	require.NoError(t, err)
	require.Equal(t, existing.ID, label.ID)

	labels, _ := cacheSnapshotForTest()
	require.Contains(t, label.Aliases, "高市早苗政府")
	require.Empty(t, labels, "alias change clears the active list (cheap list reload)")

	// 列表已清 → 下次加载回源（新别名已入库，可被 L1 命中）。
	active, err := service.loadActiveAuxiliaryLabels(context.Background())
	require.NoError(t, err)
	require.Len(t, active.labels, 1)
	require.Contains(t, active.labels[0].Aliases, "高市早苗政府")
}

// 任务 3.1：TTL 过期兜底——4:59 仍命中缓存 / 5:01 过期重拉。
func TestAuxLabelCacheTTLExpiryReloads(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	seed := models.SemanticLabel{Label: "过期样本", Slug: "expiry", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&seed).Error)

	warmAuxCacheForTest(t, []models.SemanticLabel{{ID: 999, Label: "哨兵", Slug: "sentinel"}})
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
	ctx := context.Background()

	// 4:59 —— 未过期，命中缓存（哨兵仍在，未触库）。
	packageAuxLabelCache.mu.Lock()
	packageAuxLabelCache.activeAt = time.Now().Add(-auxActiveLabelsTTL + time.Second)
	packageAuxLabelCache.mu.Unlock()
	got, err := service.loadActiveAuxiliaryLabels(ctx)
	require.NoError(t, err)
	require.Len(t, got.labels, 1)
	require.Equal(t, uint(999), got.labels[0].ID, "4:59 boundary must hit the cached list (no reload)")

	// 5:01 —— 过期，重拉（哨兵消失，种子行入库数据出现）。
	packageAuxLabelCache.mu.Lock()
	packageAuxLabelCache.activeAt = time.Now().Add(-auxActiveLabelsTTL - time.Second)
	packageAuxLabelCache.mu.Unlock()
	got, err = service.loadActiveAuxiliaryLabels(ctx)
	require.NoError(t, err)
	require.Len(t, got.labels, 1)
	require.Equal(t, seed.ID, got.labels[0].ID, "5:01 boundary must reload from DB")
}
