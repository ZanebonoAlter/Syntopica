package database_test

// board-health-triage-advice seam B2 (task 2.2) — review fix B-P2-2 at the
// migration layer, against a testcontainer Postgres.
//
// The failure mode under test: the repair mutex index
// (uq_aux_desc_repair_jobs_label_active) lives in a versioned migration
// (20261028_0001) whose Up no-ops while the repair tables are absent. A boot
// whose AutoMigrate failed (production treats AutoMigrate failure as
// NON-fatal and still runs the migrations) therefore records the migration
// version WITHOUT the index ever being created — and the version is never
// re-applied, so the per-label creation mutex would stay missing forever.
//
// The fix: every successful AutoMigrate re-ensures exactly this one index
// (CREATE UNIQUE INDEX IF NOT EXISTS), before the unrelated discovery backfill
// that may fail. This regression drives the production path end to end:
// migration recorded while the table is absent → the table appears later → the
// heal adds the index anyway → repeating the heal is idempotent.

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

const auxRepairMutexIndexName = "uq_aux_desc_repair_jobs_label_active"

// TestAuxiliaryDescriptionRepairMutexIndexSelfHeal covers B-P2-2: a recorded
// migration whose Up no-opped (table absent) must not leave the mutex index
// missing once the table shows up.
func TestAuxiliaryDescriptionRepairMutexIndexSelfHeal(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)

	// Locate the repair migration's Up closure (same helper the queue-retention
	// migration test uses).
	var up func(*gorm.DB) error
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20261028_0001" {
			up = m.Up
			break
		}
	}
	require.NotNil(t, up, "migration 20261028_0001 must be registered in postgres_migrations")

	// ── Precondition: the repair tables do not exist yet (a boot whose
	// AutoMigrate failed before creating them) and the index is absent.
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS auxiliary_description_repair_items").Error)
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS auxiliary_description_repair_jobs").Error)
	require.NoError(t, db.Exec("DROP INDEX IF EXISTS "+auxRepairMutexIndexName).Error)
	require.Equal(t, int64(0), countIndex(t, db, auxRepairMutexIndexName))

	// ── Step 1: the migration runs while the table is absent → no-op, and the
	// version is recorded (the historical failure mode).
	require.NoError(t, database.RunMigrationsList(db, nil)) // creates schema_migrations
	require.NoError(t, db.Exec("DELETE FROM schema_migrations WHERE version = '20261028_0001'").Error)
	require.NoError(t, database.RunMigrationsList(db, []database.Migration{{
		Version:     "20261028_0001",
		Description: "board-health-triage-advice repair mutex index (self-heal regression)",
		Up:          up,
	}}))
	var recorded int64
	require.NoError(t, db.Raw(
		"SELECT count(*) FROM schema_migrations WHERE version = '20261028_0001'").Scan(&recorded).Error)
	require.Equal(t, int64(1), recorded, "the migration version must be recorded even though the table was absent")
	require.Equal(t, int64(0), countIndex(t, db, auxRepairMutexIndexName),
		"no index can exist while the table is absent")

	// ── Step 2: a later boot creates the table (AutoMigrate owns the tables).
	// The migration version is already recorded, so the migration itself will
	// never run again — only the post-AutoMigrate heal can still add the index.
	require.NoError(t, db.AutoMigrate(
		&tagmodels.AuxiliaryDescriptionRepairJob{},
		&tagmodels.AuxiliaryDescriptionRepairItem{},
	))
	require.Equal(t, int64(0), countIndex(t, db, auxRepairMutexIndexName),
		"creating the table alone must not create the index")

	// ── Step 3: the self-heal (inside RunAutoMigrate, after the model sync and
	// before the discovery backfill) adds the index.
	require.NoError(t, database.RunAutoMigrate(db))
	require.Equal(t, int64(1), countIndex(t, db, auxRepairMutexIndexName),
		"the self-heal must add the partial unique mutex index after the table appeared")

	// ── Step 4: idempotent — a second heal neither fails nor duplicates.
	require.NoError(t, database.RunAutoMigrate(db))
	require.Equal(t, int64(1), countIndex(t, db, auxRepairMutexIndexName),
		"re-running the heal must not duplicate the index")

	// The healed index is the partial unique mutex over the unfinished jobs.
	var def string
	require.NoError(t, db.Raw(
		"SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = ?",
		auxRepairMutexIndexName).Scan(&def).Error)
	require.Contains(t, def, "UNIQUE")
	require.Contains(t, def, "(auxiliary_label_id)")
	// PG normalizes `status NOT IN (...)` to `status <> ALL (ARRAY[...])`.
	for _, terminal := range tagmodels.AuxiliaryDescriptionRepairTerminalStatuses {
		require.Contains(t, def, "'"+terminal+"'",
			"the mutex predicate must exclude terminal status %s: %s", terminal, def)
	}
}
