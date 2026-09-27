package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

const boardSignalMigrationVersion = "20260922_0001"

func boardSignalMigration(t *testing.T) database.Migration {
	t.Helper()
	for _, migration := range database.ExportedPostgresMigrations() {
		if migration.Version == boardSignalMigrationVersion {
			return migration
		}
	}
	t.Fatalf("migration %s not found", boardSignalMigrationVersion)
	return database.Migration{}
}

// prepareBoardSignalMigration rewinds exactly this migration's constraints and
// rows so every test can exercise its Up closure independently on the shared
// testcontainer (same pattern as result_kind_migration_test.go).
func prepareBoardSignalMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	for _, statement := range []string{
		`ALTER TABLE topic_enrichment_result DROP CONSTRAINT IF EXISTS fk_topic_enrichment_result_signal_candidate`,
		`ALTER TABLE topic_enrichment_result DROP CONSTRAINT IF EXISTS chk_topic_enrichment_result_kind`,
		`ALTER TABLE topic_enrichment_result DROP CONSTRAINT IF EXISTS chk_topic_enrichment_result_parent_shape`,
		`ALTER TABLE board_signal_candidate DROP CONSTRAINT IF EXISTS fk_board_signal_candidate_discovery`,
		`ALTER TABLE board_signal_candidate DROP CONSTRAINT IF EXISTS uq_board_signal_candidate_id_owner`,
		`ALTER TABLE board_signal_discovery DROP CONSTRAINT IF EXISTS uq_board_signal_discovery_id_owner`,
		`DROP INDEX IF EXISTS idx_topic_enrichment_result_signal_board_period`,
		`DROP INDEX IF EXISTS idx_topic_enrichment_result_source_signal`,
		`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'signal-mig-%'`,
		`DELETE FROM board_signal_candidate WHERE semantic_board_id BETWEEN 95000 AND 95999`,
		`DELETE FROM board_signal_discovery WHERE semantic_board_id BETWEEN 95000 AND 95999`,
	} {
		require.NoError(t, db.Exec(statement).Error, statement)
	}
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'signal-mig-%'`).Error
		_ = db.Exec(`DELETE FROM board_signal_candidate WHERE semantic_board_id BETWEEN 95000 AND 95999`).Error
		_ = db.Exec(`DELETE FROM board_signal_discovery WHERE semantic_board_id BETWEEN 95000 AND 95999`).Error
	})
}

func runBoardSignalMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	migration := boardSignalMigration(t)
	require.Nil(t, migration.Down, "migration framework has no Down executor; migration must stay forward-only")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return migration.Up(tx) }))
}

func seedBoardSignalDiscovery(t *testing.T, db *gorm.DB, boardID uint) uint {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO board_signal_discovery
		(semantic_board_id, granularity, period, analysis_mode, cutoff, input_snapshot, session_id, candidate_count)
		VALUES (?, 'month', '2026-08', 'current', now(), '{}'::jsonb, 'signal-mig-disc', 0)`, boardID).Error)
	var id uint
	require.NoError(t, db.Raw(`SELECT id FROM board_signal_discovery WHERE session_id = 'signal-mig-disc' AND semantic_board_id = ?`, boardID).Scan(&id).Error)
	require.NotZero(t, id)
	require.NoError(t, db.Exec(`INSERT INTO board_signal_candidate
		(discovery_id, semantic_board_id, granularity, period, signal, why_it_matters, research_question, evidence_refs, score, rationale)
		VALUES (?, ?, 'month', '2026-08', '库存与开工背离', '判断依据', '研究问题', '["a:1"]'::jsonb, 7, '理由')`, id, boardID).Error)
	var candidateID uint
	require.NoError(t, db.Raw(`SELECT id FROM board_signal_candidate WHERE discovery_id = ? LIMIT 1`, id).Scan(&candidateID).Error)
	require.NotZero(t, candidateID)
	return candidateID
}

func TestBoardSignalMigrationLegacyRowsKeepNullColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	prepareBoardSignalMigration(t, db)

	// Simulate a pre-signal database: drop the three columns, then store
	// legacy rows exactly as the old writer would have (DB-4).
	for _, statement := range []string{
		`ALTER TABLE topic_enrichment_result DROP COLUMN IF EXISTS granularity`,
		`ALTER TABLE topic_enrichment_result DROP COLUMN IF EXISTS period`,
		`ALTER TABLE topic_enrichment_result DROP COLUMN IF EXISTS source_signal_id`,
	} {
		require.NoError(t, db.Exec(statement).Error, statement)
	}
	topicSectors := `{"form":"event_chain","legacy":[1,2]}`
	boardSectors := `{"scope":"board","thesis":"旧论文原样"}`
	require.NoError(t, db.Exec(`INSERT INTO topic_enrichment_result
		(persistent_topic_id, semantic_board_id, analysis_scope, result_kind, sectors, session_id)
		VALUES
		(95101, NULL, 'topic', 'topic_analysis', ?::jsonb, 'signal-mig-legacy-topic'),
		(NULL, 95102, 'board', 'board_brief', ?::jsonb, 'signal-mig-legacy-board')`, topicSectors, boardSectors).Error)

	runBoardSignalMigration(t, db)

	// Columns exist again and legacy rows keep NULL (旧行新列全 NULL 不回填).
	var gran, period *string
	var source *uint
	var storedTopic, storedBoard string
	require.NoError(t, db.Raw(`SELECT granularity, period, source_signal_id, sectors::text FROM topic_enrichment_result WHERE session_id = 'signal-mig-legacy-topic'`).Row().Scan(&gran, &period, &source, &storedTopic))
	require.Nil(t, gran)
	require.Nil(t, period)
	require.Nil(t, source)
	require.JSONEq(t, topicSectors, storedTopic, "sectors payload must stay byte-equivalent")
	require.NoError(t, db.Raw(`SELECT granularity, period, source_signal_id, sectors::text FROM topic_enrichment_result WHERE session_id = 'signal-mig-legacy-board'`).Row().Scan(&gran, &period, &source, &storedBoard))
	require.Nil(t, gran)
	require.Nil(t, period)
	require.Nil(t, source)
	require.JSONEq(t, boardSectors, storedBoard)

	// Legacy shapes must still be insertable after the CHECK re-ADD.
	require.NoError(t, db.Exec(`INSERT INTO topic_enrichment_result
		(persistent_topic_id, semantic_board_id, analysis_scope, result_kind, sectors, session_id)
		VALUES (95103, NULL, 'topic', 'topic_analysis', '{}'::jsonb, 'signal-mig-post-topic')`).Error)
}

func TestBoardSignalMigrationShapeAndFKRejectIllegalRows(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	prepareBoardSignalMigration(t, db)
	runBoardSignalMigration(t, db)

	boardID := uint(95201)
	candidateID := seedBoardSignalDiscovery(t, db, boardID)

	// Legal signal_report row passes.
	require.NoError(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period, source_signal_id)
		VALUES (?, 'board', 'signal_report', '{}'::jsonb, 'signal-mig-ok', 'month', '2026-08', ?)`,
		boardID, candidateID).Error)

	// Unknown kind → kind CHECK (re-ADDed with signal_report included).
	require.Error(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id)
		VALUES (?, 'board', 'signal_weird', '{}'::jsonb, 'signal-mig-bad-kind')`, boardID).Error)

	// signal_report without granularity → shape CHECK (NULL must fail).
	require.Error(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, period, source_signal_id)
		VALUES (?, 'board', 'signal_report', '{}'::jsonb, 'signal-mig-no-gran', '2026-08', ?)`,
		boardID, candidateID).Error)

	// signal_report with an impossible month → shape CHECK.
	require.Error(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period, source_signal_id)
		VALUES (?, 'board', 'signal_report', '{}'::jsonb, 'signal-mig-bad-month', 'month', '2026-13', ?)`,
		boardID, candidateID).Error)

	// A legacy kind carrying signal columns → shape CHECK.
	require.Error(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period)
		VALUES (?, 'board', 'board_brief', '{}'::jsonb, 'signal-mig-brief-gran', 'month', '2026-08')`, boardID).Error)

	// Result period disagreeing with its candidate → composite FK.
	require.Error(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period, source_signal_id)
		VALUES (?, 'board', 'signal_report', '{}'::jsonb, 'signal-mig-bad-period', 'month', '2026-07', ?)`,
		boardID, candidateID).Error)

	// Result pointing at a candidate of another board → composite FK.
	otherCandidate := seedBoardSignalDiscovery(t, db, boardID+1)
	require.Error(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period, source_signal_id)
		VALUES (?, 'board', 'signal_report', '{}'::jsonb, 'signal-mig-bad-board', 'month', '2026-08', ?)`,
		boardID, otherCandidate).Error)
}

func TestBoardSignalMigrationIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	prepareBoardSignalMigration(t, db)
	runBoardSignalMigration(t, db)
	// A second full run (restart replay) must be a no-op, not an error.
	runBoardSignalMigration(t, db)

	// Constraints and indexes are in place exactly once.
	var checkCount int
	require.NoError(t, db.Raw(`SELECT count(*) FROM pg_constraint
		WHERE conname IN ('chk_topic_enrichment_result_kind', 'chk_topic_enrichment_result_parent_shape',
			'uq_board_signal_discovery_id_owner', 'uq_board_signal_candidate_id_owner',
			'fk_board_signal_candidate_discovery', 'fk_topic_enrichment_result_signal_candidate')`).Scan(&checkCount).Error)
	require.Equal(t, 6, checkCount)
	var indexCount int
	require.NoError(t, db.Raw(`SELECT count(*) FROM pg_indexes
		WHERE indexname IN ('idx_board_signal_discovery_board_period', 'idx_board_signal_candidate_board_period',
			'idx_board_signal_candidate_discovery', 'idx_topic_enrichment_result_signal_board_period',
			'idx_topic_enrichment_result_source_signal')`).Scan(&indexCount).Error)
	require.Equal(t, 5, indexCount)
}
