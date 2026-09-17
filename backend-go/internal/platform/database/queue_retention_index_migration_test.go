package database_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// TestQueueRetentionIndexMigration exercises migration 20260917_0003
// (add-notification-center, log-cleanup delta) against a testcontainer PG:
// the extended queue-row retention DELETEs in job_log_cleanup (completed rows
// older than 1 day, failed rows older than 30 days) MUST be backed by partial
// indexes on created_at WHERE status for tag_jobs / firecrawl_jobs /
// embedding_queues (spec: 队列表 completed/failed 行保留清理 — created_at 索引
// 支撑 MUST).
//
// Pattern mirrors composite_components_migration_test.go: locate the
// versioned migration's Up closure and run it in-tx, then assert index
// presence in pg_indexes.
//
// Docker required. Skipped under -short.
func TestQueueRetentionIndexMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))

	// Locate migration 20260917_0003's Up closure and run it in-tx (mirrors the
	// production in-transaction path).
	var up func(*gorm.DB) error
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260917_0003" {
			up = m.Up
			break
		}
	}
	require.NotNil(t, up, "migration 20260917_0003 not found in list")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))

	// ── 五个部分索引全部存在（幂等 Up 后） ──
	expected := []string{
		"idx_tag_jobs_completed_created",
		"idx_tag_jobs_failed_created",
		"idx_firecrawl_jobs_completed_created",
		"idx_firecrawl_jobs_failed_created",
		"idx_embedding_queues_failed_created",
	}
	for _, idxName := range expected {
		require.Equal(t, int64(1), countIndex(t, db, idxName),
			fmt.Sprintf("partial index %s must exist after migration 20260917_0003", idxName))
	}

	// ── 幂等：重复执行不报错、不重复建 ──
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))
	for _, idxName := range expected {
		require.Equal(t, int64(1), countIndex(t, db, idxName),
			fmt.Sprintf("re-running 20260917_0003 must not duplicate %s", idxName))
	}
}

func countIndex(t *testing.T, db *gorm.DB, name string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname = ?`,
		name).Scan(&n).Error)
	return n
}
