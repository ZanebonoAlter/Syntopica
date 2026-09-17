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

// TestLogCleanupJobRetention verifies the queue-job retention policy from
// add-notification-center (log-cleanup delta): completed rows are kept 1 day
// (daily reset), failed rows 30 days (TagQueuePanel retry stays usable), and
// an empty run is fine. Inherits the analysis-remediation coverage with the
// embedding window tightened 30d → 1d (test-cases「继承与调整」).
func TestLogCleanupJobRetention(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	// otel_spans 不在 AutoMigrate 模型清单里，由 tracing 初始化建表；测试中手动补齐
	// LogCleanupJob 触及的表，对齐生产 schema。
	require.NoError(t, tracing.EnsureTracingTable(db))

	now := time.Now()
	over24h := now.Add(-25 * time.Hour)  // completed: expired (>24h) → deleted
	under24h := now.Add(-23 * time.Hour) // completed: fresh (≤24h) → kept
	over30d := now.AddDate(0, 0, -31)    // failed: expired (>30d) → deleted
	under30d := now.AddDate(0, 0, -29)   // failed: fresh (≤30d) → kept (retry usable)

	// ── embedding_queues：白盒 C 边界两端（>24h 删 / ≤24h 留）+ failed 30d ──
	embeddingSeed := []models.EmbeddingQueue{
		{TagID: 1, Status: "completed", CreatedAt: over24h},  // → deleted
		{TagID: 2, Status: "completed", CreatedAt: under24h}, // → kept
		{TagID: 3, Status: "pending", CreatedAt: over24h},    // non-completed → kept
		{TagID: 4, Status: "failed", CreatedAt: over30d},     // failed expired → deleted
		{TagID: 5, Status: "failed", CreatedAt: under30d},    // failed fresh → kept
	}
	for i := range embeddingSeed {
		require.NoError(t, db.Create(&embeddingSeed[i]).Error)
	}

	// ── tag_jobs：completed 1d / failed 30d 两侧边界 ──
	tagSeed := []models.TagJob{
		{ArticleID: 1, Status: "completed", AvailableAt: over24h, CreatedAt: over24h},   // → deleted
		{ArticleID: 2, Status: "completed", AvailableAt: under24h, CreatedAt: under24h}, // → kept
		{ArticleID: 3, Status: "failed", AvailableAt: over30d, CreatedAt: over30d},      // → deleted
		{ArticleID: 4, Status: "failed", AvailableAt: under30d, CreatedAt: under30d},    // → kept (retry available)
		{ArticleID: 5, Status: "pending", AvailableAt: over24h, CreatedAt: over24h},     // active → kept
	}
	for i := range tagSeed {
		require.NoError(t, db.Create(&tagSeed[i]).Error)
	}

	// ── firecrawl_jobs：同口径 ──
	firecrawlSeed := []models.FirecrawlJob{
		{ArticleID: 1, Status: "completed", AvailableAt: over24h, CreatedAt: over24h},   // → deleted
		{ArticleID: 2, Status: "completed", AvailableAt: under24h, CreatedAt: under24h}, // → kept
		{ArticleID: 3, Status: "failed", AvailableAt: over30d, CreatedAt: over30d},      // → deleted
		{ArticleID: 4, Status: "failed", AvailableAt: under30d, CreatedAt: under30d},    // → kept
		{ArticleID: 5, Status: "pending", AvailableAt: over24h, CreatedAt: over24h},     // active → kept
	}
	for i := range firecrawlSeed {
		require.NoError(t, db.Create(&firecrawlSeed[i]).Error)
	}

	// ai_embedding_cache retention is 14 days (hits land within a day of
	// write; 90d was pure disk waste at ~30KB/row).
	cacheSeed := []models.AIEmbeddingCache{
		{CacheKey: "stale", Model: "m", Operation: "tagmanagement.embedding", Embedding: []byte("[]"), CreatedAt: time.Now().AddDate(0, 0, -15)}, // expired → deleted
		{CacheKey: "fresh", Model: "m", Operation: "tagmanagement.embedding", Embedding: []byte("[]"), CreatedAt: under24h},                      // recent → kept
	}
	for i := range cacheSeed {
		require.NoError(t, db.Create(&cacheSeed[i]).Error)
	}

	result, err := LogCleanupJob(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	// embedding_queues：只剩 fresh completed (tag 2)、pending (tag 3)、fresh failed (tag 5)
	var embeddingStatuses []int64
	require.NoError(t, db.Model(&models.EmbeddingQueue{}).Order("tag_id").Pluck("tag_id", &embeddingStatuses).Error)
	require.Equal(t, []int64{2, 3, 5}, embeddingStatuses)

	// tag_jobs：fresh completed (2)、fresh failed (4)、active pending (5)
	var tagJobArticles []int64
	require.NoError(t, db.Model(&models.TagJob{}).Order("article_id").Pluck("article_id", &tagJobArticles).Error)
	require.Equal(t, []int64{2, 4, 5}, tagJobArticles)

	// firecrawl_jobs：同口径
	var firecrawlArticles []int64
	require.NoError(t, db.Model(&models.FirecrawlJob{}).Order("article_id").Pluck("article_id", &firecrawlArticles).Error)
	require.Equal(t, []int64{2, 4, 5}, firecrawlArticles)

	var cacheKeys []string
	require.NoError(t, db.Model(&models.AIEmbeddingCache{}).Order("cache_key").Pluck("cache_key", &cacheKeys).Error)
	require.Equal(t, []string{"fresh"}, cacheKeys, "embedding cache TTL is 14 days")

	// JobResult.Data 计数字段（task 2.2）
	data := result.Data
	require.Equal(t, int64(1), data["last_embedding_queue_deleted"])
	require.Equal(t, int64(2), data["last_tag_jobs_deleted"])
	require.Equal(t, int64(2), data["last_firecrawl_jobs_deleted"])
	require.Equal(t, int64(3), data["last_queue_completed_deleted"])
	require.Equal(t, int64(3), data["last_queue_failed_deleted"])

	// ── 幂等：第二次运行删除 0 行且不报错（白盒 C 空跑） ──
	result2, err := LogCleanupJob(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(0), result2.Data["last_tag_jobs_deleted"])
	require.Equal(t, int64(0), result2.Data["last_firecrawl_jobs_deleted"])
	require.Equal(t, int64(0), result2.Data["last_embedding_queue_deleted"])

	var tagCountAfter int64
	require.NoError(t, db.Model(&models.TagJob{}).Count(&tagCountAfter).Error)
	require.Equal(t, int64(3), tagCountAfter, "second run must not touch kept rows")
}
