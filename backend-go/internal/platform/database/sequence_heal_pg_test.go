package database_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/testutil"
)

// These integration tests exercise database.HealSequenceDrift against a real
// PostgreSQL sequence on the throwaway testcontainer (Docker required; skipped
// under -short). They force the incident state by hand — setval to a stale
// value — because that is exactly what a `TRUNCATE ... RESTART IDENTITY`
// followed by a table-filtered `pg_restore` leaves behind.

// sequenceDriftState is the raw (last_value, is_called) pair of one sequence.
type sequenceDriftState struct {
	LastValue int64
	IsCalled  bool
}

func readSequenceDriftState(t *testing.T, db *gorm.DB, seq string) sequenceDriftState {
	t.Helper()
	var state sequenceDriftState
	require.NoError(t, db.Raw("SELECT last_value, is_called FROM "+seq).Row().
		Scan(&state.LastValue, &state.IsCalled))
	return state
}

// alignSequenceOnCleanup re-aligns a sequence to COALESCE(MAX(id),0)+1 with
// is_called=false when the test ends, so a test that forced drift cannot leak
// that state into another test in the same process. SetupTestDB truncates on
// the next call anyway; this also covers OpenTestDB-only callers.
func alignSequenceOnCleanup(t *testing.T, db *gorm.DB, table, seq string) {
	t.Helper()
	t.Cleanup(func() {
		require.NoError(t, db.Exec(
			"SELECT setval('"+seq+"', COALESCE((SELECT MAX(id) FROM "+table+"), 0) + 1, false)").Error)
	})
}

func seedSequenceDriftFeeds(t *testing.T, db *gorm.DB, prefix string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		feed := models.Feed{
			Title: fmt.Sprintf("%s-%d", prefix, i),
			URL:   fmt.Sprintf("https://example.com/%s/%d", prefix, i),
		}
		require.NoError(t, db.Create(&feed).Error)
	}
}

func maxIDOf(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var maxID int64
	require.NoError(t, db.Raw("SELECT COALESCE(MAX(id), 0) FROM "+table).Scan(&maxID).Error)
	return maxID
}

// TestHealSequenceDriftRepairsBehindSequence is the incident scenario end to
// end: rows carry ids up to max(id) while the sequence sits at (1, called), so
// the next nextval would hand out 2 and collide. The heal must realign the
// sequence to max(id)+1 (is_called=false), log the repair, and the next insert
// must really receive max(id)+1 without a duplicate-key error.
func TestHealSequenceDriftRepairsBehindSequence(t *testing.T) {
	db := testutil.SetupTestDB(t)
	alignSequenceOnCleanup(t, db, "feeds", "feeds_id_seq")
	seedSequenceDriftFeeds(t, db, "heal-behind", 3)
	maxID := maxIDOf(t, db, "feeds")
	require.GreaterOrEqual(t, maxID, int64(3))

	require.NoError(t, db.Exec("SELECT setval('feeds_id_seq', 1, true)").Error)

	var buf bytes.Buffer
	logging.SetWriters(&buf, &buf)
	defer logging.ResetWriters()

	require.GreaterOrEqual(t, database.HealSequenceDrift(db), 1)

	state := readSequenceDriftState(t, db, "feeds_id_seq")
	require.Equal(t, maxID+1, state.LastValue)
	require.False(t, state.IsCalled, "is_called=false makes nextval return max(id)+1 exactly")

	log := buf.String()
	require.Contains(t, log, "Sequence drift repaired")
	require.Contains(t, log, "feeds_id_seq")
	require.Contains(t, log, "next_would_be=2")
	require.Contains(t, log, fmt.Sprintf("max_id=%d", maxID))
	require.Contains(t, log, fmt.Sprintf("new_next=%d", maxID+1))

	feed := models.Feed{Title: "heal-behind-new", URL: "https://example.com/heal-behind-new"}
	require.NoError(t, db.Create(&feed).Error)
	require.Equal(t, maxID+1, int64(feed.ID), "the repaired sequence must hand out max(id)+1")
}

// TestHealSequenceDriftIsIdempotent covers the "no regression" requirement: an
// already-aligned table (next allocation > max(id)) must not be written again,
// and a second pass over the whole database repairs nothing.
func TestHealSequenceDriftIsIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	alignSequenceOnCleanup(t, db, "feeds", "feeds_id_seq")
	seedSequenceDriftFeeds(t, db, "heal-idempotent", 3)
	maxID := maxIDOf(t, db, "feeds")

	// Normalize anything the golden schema left behind, then snapshot.
	_ = database.HealSequenceDrift(db)

	before := readSequenceDriftState(t, db, "feeds_id_seq")
	require.Equal(t, maxID, before.LastValue)
	require.True(t, before.IsCalled, "fresh inserts leave the sequence called at max(id)")

	require.Zero(t, database.HealSequenceDrift(db), "a second pass must not repair anything")
	require.Equal(t, before, readSequenceDriftState(t, db, "feeds_id_seq"),
		"an aligned sequence must never regress")
}

// TestHealSequenceDriftRepairsOnlyDriftedTables covers the mixed state: feeds is
// forced behind while categories stays naturally aligned. Only the drifted
// sequence is rewritten.
func TestHealSequenceDriftRepairsOnlyDriftedTables(t *testing.T) {
	db := testutil.SetupTestDB(t)
	alignSequenceOnCleanup(t, db, "feeds", "feeds_id_seq")
	alignSequenceOnCleanup(t, db, "categories", "categories_id_seq")

	seedSequenceDriftFeeds(t, db, "heal-mixed", 3)
	for i := 0; i < 2; i++ {
		category := models.Category{
			Name: fmt.Sprintf("heal-mixed-%d", i),
			Slug: fmt.Sprintf("heal-mixed-%d", i),
		}
		require.NoError(t, db.Create(&category).Error)
	}

	categoriesBefore := readSequenceDriftState(t, db, "categories_id_seq")
	feedsMaxID := maxIDOf(t, db, "feeds")

	require.NoError(t, db.Exec("SELECT setval('feeds_id_seq', 1, true)").Error)
	require.GreaterOrEqual(t, database.HealSequenceDrift(db), 1)

	require.Equal(t, sequenceDriftState{LastValue: feedsMaxID + 1, IsCalled: false},
		readSequenceDriftState(t, db, "feeds_id_seq"))
	require.Equal(t, categoriesBefore, readSequenceDriftState(t, db, "categories_id_seq"),
		"an aligned table's sequence must stay untouched")
}

// TestHealSequenceDriftContinuesAfterSingleTableFailure injects a per-table
// failure without test seams: a table with an `id` column plus a matching
// `<table>_id_seq` sequence passes the catalog filter, but MAX(id) is undefined
// for bytea, so its probe errors. The failure must be logged and skipped while
// the healthy drifted table is still repaired (design D7: non-fatal, startup
// must not be blocked).
func TestHealSequenceDriftContinuesAfterSingleTableFailure(t *testing.T) {
	db := testutil.SetupTestDB(t)
	alignSequenceOnCleanup(t, db, "feeds", "feeds_id_seq")
	seedSequenceDriftFeeds(t, db, "heal-failure", 3)
	maxID := maxIDOf(t, db, "feeds")

	require.NoError(t, db.Exec("CREATE TABLE heal_probe_bad (id bytea)").Error)
	require.NoError(t, db.Exec("CREATE SEQUENCE heal_probe_bad_id_seq").Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS heal_probe_bad").Error)
		require.NoError(t, db.Exec("DROP SEQUENCE IF EXISTS heal_probe_bad_id_seq").Error)
	})

	require.NoError(t, db.Exec("SELECT setval('feeds_id_seq', 1, true)").Error)

	var buf bytes.Buffer
	logging.SetWriters(&buf, &buf)
	defer logging.ResetWriters()

	healed := database.HealSequenceDrift(db)
	require.GreaterOrEqual(t, healed, 1, "the healthy drifted table must still be repaired")
	require.Equal(t, sequenceDriftState{LastValue: maxID + 1, IsCalled: false},
		readSequenceDriftState(t, db, "feeds_id_seq"))

	log := buf.String()
	require.Contains(t, log, "Sequence drift self-check failed for table")
	require.Contains(t, log, "heal_probe_bad")
	require.Contains(t, log, "Sequence drift repaired")
	require.Contains(t, log, "feeds_id_seq")
}
