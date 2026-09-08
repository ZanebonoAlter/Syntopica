package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/platform/tracing"
)

// TestLogCleanupJobRetention verifies the embedding_queues 30-day retention
// added in analysis-remediation: expired completed rows are deleted, recent
// completed rows and non-completed rows are kept, and an empty table is fine.
func TestLogCleanupJobRetention(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	// otel_spans 不在 AutoMigrate 模型清单里，由 tracing 初始化建表；测试中手动补齐
	// LogCleanupJob 触及的表，对齐生产 schema。
	require.NoError(t, tracing.EnsureTracingTable(db))

	old := time.Now().AddDate(0, 0, -31)
	recent := time.Now().AddDate(0, 0, -3)
	seed := []models.EmbeddingQueue{
		{TagID: 1, Status: "completed", CreatedAt: old},    // expired → deleted
		{TagID: 2, Status: "completed", CreatedAt: recent}, // recent → kept
		{TagID: 3, Status: "pending", CreatedAt: old},      // non-completed → kept
		{TagID: 4, Status: "failed", CreatedAt: old},       // non-completed → kept
	}
	for i := range seed {
		require.NoError(t, db.Create(&seed[i]).Error)
	}

	// ai_embedding_cache retention is 14 days (hits land within a day of
	// write; 90d was pure disk waste at ~30KB/row).
	cacheSeed := []models.AIEmbeddingCache{
		{CacheKey: "stale", Model: "m", Operation: "tagmanagement.embedding", Embedding: []byte("[]"), CreatedAt: time.Now().AddDate(0, 0, -15)}, // expired → deleted
		{CacheKey: "fresh", Model: "m", Operation: "tagmanagement.embedding", Embedding: []byte("[]"), CreatedAt: recent},                        // recent → kept
	}
	for i := range cacheSeed {
		require.NoError(t, db.Create(&cacheSeed[i]).Error)
	}

	result, err := LogCleanupJob(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	var statuses []string
	require.NoError(t, db.Model(&models.EmbeddingQueue{}).Order("tag_id").Pluck("status", &statuses).Error)
	require.Equal(t, []string{"completed", "pending", "failed"}, statuses) // tag 2/3/4 remain

	var cacheKeys []string
	require.NoError(t, db.Model(&models.AIEmbeddingCache{}).Order("cache_key").Pluck("cache_key", &cacheKeys).Error)
	require.Equal(t, []string{"fresh"}, cacheKeys, "embedding cache TTL is 14 days")
}
