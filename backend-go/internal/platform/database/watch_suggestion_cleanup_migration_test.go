package database_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// findWatchSuggestionCleanupMigration locates migration 20260905_0002's Up
// closure (split-board-upgrade-directions: watch 观察池退役存量清理).
func findWatchSuggestionCleanupMigration() func(*gorm.DB) error {
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260905_0002" {
			return m.Up
		}
	}
	return nil
}

// TestWatchSuggestionCleanupMigrationIdempotent verifies the one-shot watch
// cleanup (spec: 存量 watch 建议被清理): pending watch rows are deleted, non-watch
// rows (create_new/merge/compose, any status) survive, and a second run is a
// no-op.
func TestWatchSuggestionCleanupMigrationIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findWatchSuggestionCleanupMigration()
	require.NotNil(t, up, "migration 20260905_0002 must be registered")

	// 表由 RunAutoMigrate 建好（models 注册），seed 只填迁移判定所需列。
	seed := []struct{ decision, status string }{
		{"watch", "pending"},
		{"watch", "pending"},
		{"create_new", "confirmed"},
		{"merge_into_existing", "pending"},
		{"compose", "pending"},
		{"watch", "confirmed"},
		{"create_new", "pending"},
	}
	for i, row := range seed {
		require.NoError(t, db.Exec(`INSERT INTO board_upgrade_suggestions (batch_id, mode, decision, board_label, status, suggestion_hash) VALUES (?, ?, ?, ?, ?, ?)`,
			"seed-batch", "create:aux", row.decision, row.decision+"-probe", row.status, fmt.Sprintf("%s-hash-%d", row.decision, i)).Error)
	}

	require.NoError(t, up(db))

	var watchCount int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM board_upgrade_suggestions WHERE decision = 'watch'`).Scan(&watchCount).Error)
	require.Zero(t, watchCount, "all watch rows deleted regardless of status")
	var otherCount int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM board_upgrade_suggestions WHERE decision <> 'watch'`).Scan(&otherCount).Error)
	require.Equal(t, int64(4), otherCount, "non-watch rows survive (create_new confirmed/pending + merge + compose)")

	// 幂等：二次执行 no-op。
	require.NoError(t, up(db))
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM board_upgrade_suggestions WHERE decision <> 'watch'`).Scan(&otherCount).Error)
	require.Equal(t, int64(4), otherCount)
}

// findLegacyDiscoverNewDismissMigration locates migration 20260907_0001's Up
// closure（split-board-upgrade-directions 报障修复：旧 discover_new 管线 pending
// 存量置 dismissed 清理，防永久混入建议列表）。
func findLegacyDiscoverNewDismissMigration() func(*gorm.DB) error {
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260907_0001" {
			return m.Up
		}
	}
	return nil
}

// TestLegacyDiscoverNewPendingDismissMigrationIdempotent：pending discover_new 行
// 置 dismissed（留痕 dismiss_reason/resolved_at）；非 discover_new 与已 resolved 的
// discover_new 行不受影响；二次执行 no-op。
func TestLegacyDiscoverNewPendingDismissMigrationIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findLegacyDiscoverNewDismissMigration()
	require.NotNil(t, up, "migration 20260907_0001 must be registered")

	seed := []struct{ mode, status string }{
		{"discover_new", "pending"},
		{"discover_new", "pending"},
		{"discover_new", "confirmed"},
		{"create:aux", "pending"},
		{"expand:composite", "pending"},
	}
	for i, row := range seed {
		require.NoError(t, db.Exec(`INSERT INTO board_upgrade_suggestions (batch_id, mode, decision, board_label, status, suggestion_hash) VALUES (?, ?, 'create_new', ?, ?, ?)`,
			"seed-batch", row.mode, row.mode+"-probe", row.status, fmt.Sprintf("%s-hash-%d", row.mode, i)).Error)
	}

	require.NoError(t, up(db))

	var pendingLegacy int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM board_upgrade_suggestions WHERE mode = 'discover_new' AND status = 'pending'`).Scan(&pendingLegacy).Error)
	require.Zero(t, pendingLegacy, "pending discover_new rows dismissed")
	var dismissedWithReason int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM board_upgrade_suggestions WHERE dismiss_reason = 'legacy_discover_new_cleanup'`).Scan(&dismissedWithReason).Error)
	require.Equal(t, int64(2), dismissedWithReason, "exactly the two pending legacy rows dismissed with trace")
	var untouched int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM board_upgrade_suggestions WHERE status = 'pending' AND mode <> 'discover_new'`).Scan(&untouched).Error)
	require.Equal(t, int64(2), untouched, "new-mode pending rows survive")
	var confirmedLegacy int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM board_upgrade_suggestions WHERE mode = 'discover_new' AND status = 'confirmed'`).Scan(&confirmedLegacy).Error)
	require.Equal(t, int64(1), confirmedLegacy, "resolved legacy rows untouched")

	// 幂等：二次执行 no-op（不重复置、不动别人）。
	require.NoError(t, up(db))
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM board_upgrade_suggestions WHERE dismiss_reason = 'legacy_discover_new_cleanup'`).Scan(&dismissedWithReason).Error)
	require.Equal(t, int64(2), dismissedWithReason)
}
