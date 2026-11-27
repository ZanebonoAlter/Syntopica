package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// findLaneRecallConfigMigration locates migration 20261126_0001's Up closure
// (lane-first-recall task 1.1: the two ai_settings seeds).
func findLaneRecallConfigMigration() func(*gorm.DB) error {
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20261126_0001" {
			return m.Up
		}
	}
	return nil
}

// TestLaneRecallConfigMigration covers the seed contract: both keys land with the
// calibrated defaults, re-running the migration is a no-op, and an
// operator-modified value is never overwritten (ON CONFLICT DO NOTHING).
func TestLaneRecallConfigMigration(t *testing.T) {
	db := testutil.SetupTestDB(t)

	up := findLaneRecallConfigMigration()
	require.NotNil(t, up, "migration 20261126_0001 must be registered")

	want := map[string]string{
		"lane_recall_top_k":        "8",
		"lane_recall_max_distance": "0.16",
	}
	read := func(key string) string {
		var row models.AISettings
		require.NoError(t, db.Where("key = ?", key).First(&row).Error, "key %s must be seeded", key)
		return row.Value
	}
	for key, value := range want {
		require.Equal(t, value, read(key), "seed value mismatch for %s", key)
	}

	// Idempotent: re-running must not error nor duplicate.
	require.NoError(t, up(db), "re-run must be idempotent")
	for key := range want {
		var n int64
		require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", key).Count(&n).Error)
		require.EqualValues(t, 1, n, "key %s must stay a single row", key)
	}

	// Non-overriding: an operator-tuned value survives the re-run.
	require.NoError(t, db.Model(&models.AISettings{}).
		Where("key = ?", "lane_recall_top_k").Update("value", "10").Error)
	require.NoError(t, up(db), "re-run must not fail on an existing tuned row")
	require.Equal(t, "10", read("lane_recall_top_k"), "migration MUST NOT overwrite an operator value")
}
