package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// findDailyReportBackfillRunsMigration locates migration 20261125_0001's Up
// closure (add-daily-report-gap-backfill design D4: 补填运行留档表).
func findDailyReportBackfillRunsMigration() func(*gorm.DB) error {
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20261125_0001" {
			return m.Up
		}
	}
	return nil
}

// TestDailyReportBackfillRunsMigration covers the run-ledger table contract:
// the migration is registered, the table + columns exist, the terminal-state
// CHECK rejects unknown states, and re-running the migration is a no-op.
func TestDailyReportBackfillRunsMigration(t *testing.T) {
	db := testutil.SetupTestDB(t)

	up := findDailyReportBackfillRunsMigration()
	require.NotNil(t, up, "migration 20261125_0001 must be registered")

	// The golden schema already ran it (AutoMigrate + RunMigrations).
	require.True(t, db.Migrator().HasTable("daily_report_backfill_runs"),
		"daily_report_backfill_runs must exist after the production migration path")

	for _, col := range []string{
		"run_id", "trigger_scope", "window_start", "window_end", "status",
		"daily_details", "filled_count", "failed_count", "remaining_count",
		"started_at", "finished_at",
	} {
		require.True(t, db.Migrator().HasColumn("daily_report_backfill_runs", col),
			"missing column %s", col)
	}

	// Idempotent: re-running the Up closure must succeed.
	require.NoError(t, up(db), "re-run must be idempotent")

	insert := func(runID, scope, status string, start, end string) error {
		return db.Exec(`INSERT INTO daily_report_backfill_runs
			(run_id, trigger_scope, window_start, window_end, status, daily_details, started_at)
			VALUES (?, ?, ?, ?, ?, '[]'::jsonb, now())`,
			runID, scope, start, end, status).Error
	}

	// 四态终态 + 运行中态都合法。
	for _, status := range []string{"running", "success", "partial", "failed", "budget_exhausted"} {
		require.NoError(t, insert("run-"+status, "manual", status, "2026-11-17", "2026-11-23"),
			"status %s must be accepted", status)
	}
	// 未知终态被 CHECK 拒绝。
	require.Error(t, insert("run-bogus", "manual", "nonsense", "2026-11-17", "2026-11-23"),
		"unknown status must be rejected")
	// 未知触发来源被 CHECK 拒绝。
	require.Error(t, insert("run-bogus-scope", "whatever", "success", "2026-11-17", "2026-11-23"),
		"unknown trigger scope must be rejected")
	// 窗口倒置被 CHECK 拒绝。
	require.Error(t, insert("run-bogus-window", "manual", "success", "2026-11-23", "2026-11-17"),
		"inverted window must be rejected")

	// run_id 唯一（同一运行不会被拆成两行）。
	require.Error(t, insert("run-success", "manual", "success", "2026-11-17", "2026-11-23"),
		"duplicate run_id must be rejected")
}
