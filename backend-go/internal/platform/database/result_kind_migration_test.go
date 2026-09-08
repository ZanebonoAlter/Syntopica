package database_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

const resultKindMigrationVersion = "20260828_0001"

func resultKindMigration(t *testing.T) database.Migration {
	t.Helper()
	for _, migration := range database.ExportedPostgresMigrations() {
		if migration.Version == resultKindMigrationVersion {
			return migration
		}
	}
	t.Fatalf("migration %s not found", resultKindMigrationVersion)
	return database.Migration{}
}

func prepareResultKindLegacyShape(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))

	// The testcontainer is shared by the package. Rewind only this migration's
	// constraints so every test can exercise its Up closure independently.
	for _, statement := range []string{
		`DROP TRIGGER IF EXISTS trg_validate_topic_enrichment_result_parent ON topic_enrichment_result`,
		`DROP FUNCTION IF EXISTS validate_topic_enrichment_result_parent()`,
		`ALTER TABLE topic_enrichment_result DROP CONSTRAINT IF EXISTS fk_topic_enrichment_result_parent`,
		`ALTER TABLE topic_enrichment_result DROP CONSTRAINT IF EXISTS fk_topic_enrichment_result_parent_board`,
		`ALTER TABLE topic_enrichment_result DROP CONSTRAINT IF EXISTS uq_topic_enrichment_result_id_board`,
		`ALTER TABLE topic_enrichment_result DROP CONSTRAINT IF EXISTS chk_topic_enrichment_result_kind`,
		`ALTER TABLE topic_enrichment_result DROP CONSTRAINT IF EXISTS chk_topic_enrichment_result_parent_shape`,
		`ALTER TABLE topic_enrichment_result ALTER COLUMN result_kind DROP NOT NULL`,
		`ALTER TABLE topic_enrichment_result ALTER COLUMN result_kind DROP DEFAULT`,
		`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'result-kind-mig-%'`,
	} {
		require.NoError(t, db.Exec(statement).Error, statement)
	}
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'result-kind-mig-%' AND parent_result_id IS NOT NULL`).Error
		_ = db.Exec(`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'result-kind-mig-%'`).Error
	})
}

func runResultKindMigration(t *testing.T, db *gorm.DB) database.Migration {
	t.Helper()
	migration := resultKindMigration(t)
	require.Nil(t, migration.Down, "migration framework has no Down executor; migration must stay forward-only")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return migration.Up(tx) }))
	return migration
}

func TestResultKindMigrationBackfillPreservesSectors(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	prepareResultKindLegacyShape(t, db)

	topicSectors := `{"form":"event_chain","legacy":[1,2]}`
	boardSectors := `{"scope":"board","thesis":"旧论文","argument":{"layers":["原样"]}}`
	require.NoError(t, db.Exec(`INSERT INTO topic_enrichment_result
		(persistent_topic_id, semantic_board_id, analysis_scope, result_kind, sectors, session_id, created_at)
		VALUES
		(98101, NULL, 'topic', NULL, ?::jsonb, 'result-kind-mig-topic', now()),
		(NULL, 98201, 'board', 'topic_analysis', ?::jsonb, 'result-kind-mig-board', now())`, topicSectors, boardSectors).Error)

	type row struct {
		SessionID  string
		ResultKind string
		Sectors    string
	}
	var before []row
	require.NoError(t, db.Raw(`SELECT session_id, COALESCE(result_kind, '') AS result_kind, sectors::text AS sectors
		FROM topic_enrichment_result WHERE session_id LIKE 'result-kind-mig-%' ORDER BY session_id`).Scan(&before).Error)
	require.Len(t, before, 2)

	migration := runResultKindMigration(t, db)
	// Re-running Up is idempotent and must not rewrite legacy JSON.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return migration.Up(tx) }))

	var after []row
	require.NoError(t, db.Raw(`SELECT session_id, result_kind, sectors::text AS sectors
		FROM topic_enrichment_result WHERE session_id LIKE 'result-kind-mig-%' ORDER BY session_id`).Scan(&after).Error)
	require.Len(t, after, 2)
	for i := range after {
		require.Equal(t, before[i].SessionID, after[i].SessionID)
		require.Equal(t, before[i].Sectors, after[i].Sectors, "migration must preserve the exact sectors::text database value")
	}
	require.Equal(t, "legacy_board_analysis", after[0].ResultKind)
	require.JSONEq(t, boardSectors, after[0].Sectors)
	require.Equal(t, "topic_analysis", after[1].ResultKind)
	require.JSONEq(t, topicSectors, after[1].Sectors)
}

func TestResultKindAutoMigratePreservesDatabaseContract(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	prepareResultKindLegacyShape(t, db)
	runResultKindMigration(t, db)

	// This is the regression: future startup AutoMigrate runs must preserve the
	// explicit migration's NOT NULL/default/CHECK contract.
	require.NoError(t, database.RunAutoMigrate(db))

	type columnMetadata struct {
		IsNullable    string
		ColumnDefault string
	}
	var metadata columnMetadata
	require.NoError(t, db.Raw(`SELECT is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'topic_enrichment_result'
		  AND column_name = 'result_kind'`).Scan(&metadata).Error)
	require.Equal(t, "NO", metadata.IsNullable)
	require.True(t, strings.Contains(metadata.ColumnDefault, "'topic_analysis'"), metadata.ColumnDefault)

	var defaultKind string
	require.NoError(t, db.Raw(`INSERT INTO topic_enrichment_result
		(persistent_topic_id, analysis_scope, session_id, created_at)
		VALUES (98231, 'topic', 'result-kind-mig-default', now())
		RETURNING result_kind`).Scan(&defaultKind).Error)
	require.Equal(t, repository.ResultKindTopicAnalysis, defaultKind)

	err := db.Exec(`INSERT INTO topic_enrichment_result
		(persistent_topic_id, analysis_scope, result_kind, session_id, created_at)
		VALUES (98232, 'topic', 'invalid_kind', 'result-kind-mig-invalid-kind', now())`).Error
	require.Error(t, err, "result_kind CHECK must remain effective after AutoMigrate")
}

func TestResultKindOwnerShapeConstraint(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	prepareResultKindLegacyShape(t, db)
	runResultKindMigration(t, db)

	invalidStatements := map[string]string{
		"topic-missing-owner": `INSERT INTO topic_enrichment_result
			(analysis_scope, result_kind, session_id, created_at)
			VALUES ('topic', 'topic_analysis', 'result-kind-mig-owner-topic-missing', now())`,
		"topic-mixed-owner": `INSERT INTO topic_enrichment_result
			(persistent_topic_id, semantic_board_id, analysis_scope, result_kind, session_id, created_at)
			VALUES (98241, 98242, 'topic', 'topic_analysis', 'result-kind-mig-owner-topic-mixed', now())`,
		"board-missing-owner": `INSERT INTO topic_enrichment_result
			(analysis_scope, result_kind, session_id, created_at)
			VALUES ('board', 'board_brief', 'result-kind-mig-owner-board-missing', now())`,
		"board-mixed-owner": `INSERT INTO topic_enrichment_result
			(persistent_topic_id, semantic_board_id, analysis_scope, result_kind, session_id, created_at)
			VALUES (98243, 98244, 'board', 'legacy_board_analysis', 'result-kind-mig-owner-board-mixed', now())`,
	}
	for name, statement := range invalidStatements {
		t.Run(name, func(t *testing.T) {
			require.Error(t, db.Exec(statement).Error)
		})
	}
}

func TestResultKindMigrationRejectsMixedHistoricalOwnerWithoutRepair(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	prepareResultKindLegacyShape(t, db)
	require.NoError(t, db.Exec(`INSERT INTO topic_enrichment_result
		(persistent_topic_id, semantic_board_id, analysis_scope, result_kind, session_id, created_at)
		VALUES (98251, 98252, 'topic', NULL, 'result-kind-mig-dirty-owner', now())`).Error)

	migration := resultKindMigration(t)
	err := db.Transaction(func(tx *gorm.DB) error { return migration.Up(tx) })
	require.Error(t, err)
	require.Contains(t, err.Error(), "mixed or missing owner")

	type ownerRow struct {
		PersistentTopicID uint
		SemanticBoardID   uint
	}
	var row ownerRow
	require.NoError(t, db.Raw(`SELECT persistent_topic_id, semantic_board_id
		FROM topic_enrichment_result WHERE session_id = 'result-kind-mig-dirty-owner'`).Scan(&row).Error)
	require.Equal(t, uint(98251), row.PersistentTopicID)
	require.Equal(t, uint(98252), row.SemanticBoardID)
}

func TestBoardInvestigationParentConstraint(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	prepareResultKindLegacyShape(t, db)
	runResultKindMigration(t, db)

	var briefID uint
	require.NoError(t, db.Raw(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, session_id, created_at)
		VALUES (98211, 'board', 'board_brief', 'result-kind-mig-brief', now()) RETURNING id`).Scan(&briefID).Error)
	var otherBoardBriefID uint
	require.NoError(t, db.Raw(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, session_id, created_at)
		VALUES (98212, 'board', 'board_brief', 'result-kind-mig-other-brief', now()) RETURNING id`).Scan(&otherBoardBriefID).Error)
	var legacyID uint
	require.NoError(t, db.Raw(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, session_id, created_at)
		VALUES (98211, 'board', 'legacy_board_analysis', 'result-kind-mig-legacy', now()) RETURNING id`).Scan(&legacyID).Error)

	validKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var validChildID uint
	require.NoError(t, db.Raw(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, parent_result_id, question_key, session_id, created_at)
		VALUES (98211, 'board', 'board_investigation', ?, ?, 'result-kind-mig-valid-child', now()) RETURNING id`, briefID, validKey).Scan(&validChildID).Error)

	err := db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, parent_result_id, question_key, session_id, created_at)
		VALUES (98211, 'board', 'board_investigation', ?, ?, 'result-kind-mig-invalid-cross-board', now())`, otherBoardBriefID, validKey).Error
	require.Error(t, err, "database must reject a parent from another board")

	err = db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, parent_result_id, question_key, session_id, created_at)
		VALUES (98211, 'board', 'board_investigation', ?, ?, 'result-kind-mig-invalid-legacy-parent', now())`, legacyID, validKey).Error
	require.Error(t, err, "trigger must reject a same-board legacy parent")

	keyCopy := validKey
	invalidInvestigationParent := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(98211),
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindBoardInvestigation,
		ParentResultID:  repository.TopicIDPtr(validChildID),
		QuestionKey:     &keyCopy,
		SessionID:       "result-kind-mig-invalid-investigation-parent",
	}
	require.Error(t, db.Create(invalidInvestigationParent).Error, "trigger must reject an investigation parent through GORM")

	err = db.Model(&repository.TopicEnrichmentResult{}).
		Where("id = ?", validChildID).
		Update("parent_result_id", legacyID).Error
	require.Error(t, err, "trigger must validate parent changes on UPDATE")

	err = db.Model(&repository.TopicEnrichmentResult{}).
		Where("id = ?", briefID).
		Update("result_kind", repository.ResultKindLegacyBoardAnalysis).Error
	require.Error(t, err, "a referenced brief cannot change kind and invalidate existing children")

	err = db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, parent_result_id, question_key, session_id, created_at)
		VALUES (98211, 'board', 'board_brief', ?, NULL, 'result-kind-mig-illegal-parent', now())`, briefID).Error
	require.Error(t, err, "non-investigation kinds must not carry a parent")
}

func TestQuestionKeyConstraint(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	prepareResultKindLegacyShape(t, db)
	runResultKindMigration(t, db)

	var briefID uint
	require.NoError(t, db.Raw(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, session_id, created_at)
		VALUES (98221, 'board', 'board_brief', 'result-kind-mig-question-brief', now()) RETURNING id`).Scan(&briefID).Error)

	for name, key := range map[string]any{
		"null":       nil,
		"empty":      "",
		"not-hex":    "z123",
		"wrong-size": "abcdef",
	} {
		t.Run(name, func(t *testing.T) {
			err := db.Exec(`INSERT INTO topic_enrichment_result
				(semantic_board_id, analysis_scope, result_kind, parent_result_id, question_key, session_id, created_at)
				VALUES (98221, 'board', 'board_investigation', ?, ?, ?, now())`, briefID, key, "result-kind-mig-key-"+name).Error
			require.Error(t, err)
		})
	}
}
