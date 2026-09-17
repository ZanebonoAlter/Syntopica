package scheduler

import (
	"context"
	"fmt"
	"time"

	"syntopica-backend/internal/admin/repository"
)

// LogCleanupJob deletes expired ai_call_logs, otel_spans, ai_embedding_cache
// and queue-job rows (add-notification-center: queue retention daily reset).
//
// Queue retention (白盒 C, spec log-cleanup delta):
//   - completed rows: kept 1 day (daily reset — total counts then reflect
//     active work + today's completions)
//   - failed rows: kept 30 days (TagQueuePanel retry entry stays usable)
func LogCleanupJob(ctx context.Context) (*JobResult, error) {
	cutoff := time.Now().AddDate(0, 0, -7)

	var aiCallLogsDeleted int64
	result := repository.Repo.DB().Exec("DELETE FROM ai_call_logs WHERE created_at < ?", cutoff)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to clean ai_call_logs: %w", result.Error)
	}
	aiCallLogsDeleted = result.RowsAffected

	var otelSpansDeleted int64
	cutoffNano := cutoff.UnixNano()
	result = repository.Repo.DB().Exec("DELETE FROM otel_spans WHERE start_time_unix_nano < ?", cutoffNano)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to clean otel_spans: %w", result.Error)
	}
	otelSpansDeleted = result.RowsAffected

	// Embedding cache rows older than 14 days are stale: same-name model
	// upgrades repopulate fresh vectors anyway, and old ones would shadow-hit.
	// 14d (not 90d): hits overwhelmingly land within a day of write (nightly
	// processing windows), so longer retention was pure disk waste at
	// ~30KB/row for jsonb vectors.
	embeddingCacheCutoff := time.Now().AddDate(0, 0, -14)
	result = repository.Repo.DB().Exec("DELETE FROM ai_embedding_cache WHERE created_at < ?", embeddingCacheCutoff)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to clean ai_embedding_cache: %w", result.Error)
	}
	embeddingCacheDeleted := result.RowsAffected

	// Queue-job retention (log-cleanup delta, 2026-09): completed rows are
	// history-only processing artifacts — the business results live elsewhere
	// (article_tags / firecrawl_status), so a 1-day window gives the literal
	// "daily reset". The old 30-day window was pointless at ~2400 rows/day
	// (measured 72k accumulation). failed rows keep 30 days for panel retry.
	completedCutoff := time.Now().Add(-24 * time.Hour)
	failedCutoff := time.Now().AddDate(0, 0, -30)

	embeddingCompletedDeleted, embeddingFailedDeleted, err := cleanQueueJobsStatus("embedding_queues", completedCutoff, failedCutoff)
	if err != nil {
		return nil, err
	}
	tagJobsCompletedDeleted, tagJobsFailedDeleted, err := cleanQueueJobsStatus("tag_jobs", completedCutoff, failedCutoff)
	if err != nil {
		return nil, err
	}
	firecrawlCompletedDeleted, firecrawlFailedDeleted, err := cleanQueueJobsStatus("firecrawl_jobs", completedCutoff, failedCutoff)
	if err != nil {
		return nil, err
	}

	return &JobResult{
		Data: map[string]interface{}{
			"last_ai_call_logs_deleted":    aiCallLogsDeleted,
			"last_otel_spans_deleted":      otelSpansDeleted,
			"last_embedding_cache_deleted": embeddingCacheDeleted,
			"last_embedding_queue_deleted": embeddingCompletedDeleted,
			"last_tag_jobs_deleted":        tagJobsCompletedDeleted + tagJobsFailedDeleted,
			"last_firecrawl_jobs_deleted":  firecrawlCompletedDeleted + firecrawlFailedDeleted,
			"last_queue_completed_deleted": embeddingCompletedDeleted + tagJobsCompletedDeleted + firecrawlCompletedDeleted,
			"last_queue_failed_deleted":    embeddingFailedDeleted + tagJobsFailedDeleted + firecrawlFailedDeleted,
		},
		Summary: fmt.Sprintf("ai_call_logs=%d, otel_spans=%d, ai_embedding_cache=%d, embedding_queues=%d, tag_jobs=%d, firecrawl_jobs=%d",
			aiCallLogsDeleted, otelSpansDeleted, embeddingCacheDeleted,
			embeddingCompletedDeleted+embeddingFailedDeleted, tagJobsCompletedDeleted+tagJobsFailedDeleted, firecrawlCompletedDeleted+firecrawlFailedDeleted),
	}, nil
}

// cleanQueueJobsStatus deletes expired completed and failed rows from a job
// table with a `status` string column and created_at index. Idempotent: a
// second run deletes 0 rows (test-cases 白盒 C).
func cleanQueueJobsStatus(table string, completedCutoff, failedCutoff time.Time) (completed, failed int64, err error) {
	db := repository.Repo.DB()
	result := db.Exec(
		fmt.Sprintf("DELETE FROM %s WHERE status = 'completed' AND created_at < ?", table), completedCutoff)
	if result.Error != nil {
		return 0, 0, fmt.Errorf("failed to clean %s completed: %w", table, result.Error)
	}
	completed = result.RowsAffected

	result = db.Exec(
		fmt.Sprintf("DELETE FROM %s WHERE status = 'failed' AND created_at < ?", table), failedCutoff)
	if result.Error != nil {
		return completed, 0, fmt.Errorf("failed to clean %s failed: %w", table, result.Error)
	}
	return completed, result.RowsAffected, nil
}
