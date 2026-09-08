package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// TestBoardLevelAnalysisScopeMigration exercises migration 20260826_0001
// end-to-end against a testcontainer PG in the production upgrade scenario:
// AutoMigrate has added topic_enrichment_result.{semantic_board_id,analysis_scope}
// and the reference_roles table, but persistent_topic_id still carries its
// legacy NOT NULL (AutoMigrate never drops constraints). The migration must
// backfill analysis_scope='topic' for NULL rows and drop NOT NULL on both
// enrichment id columns so board-scoped rows (NULL topic) can be inserted.
//
// Docker required. Skipped under -short.
func TestBoardLevelAnalysisScopeMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))

	// Hermetic cleanup (shared process-singleton container): wipe everything
	// this test writes, including board-scope rows, so later tests that count
	// table rows (e.g. destructive-guard probe) see a clean table.
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'bld-mig-%' OR persistent_topic_id IN (9201, 9202)`).Error
		_ = db.Exec(`DELETE FROM topic_enrichment_review WHERE deviation_summary LIKE 'bld-mig-%' OR semantic_board_id = 9301`).Error
		_ = db.Exec(`DELETE FROM reference_roles WHERE name IN ('bld-mig-role', 'inside-america-v2')`).Error
	})
	require.NoError(t, db.Exec(`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'bld-mig-%' OR persistent_topic_id = 9201`).Error)
	require.NoError(t, db.Exec(`DELETE FROM topic_enrichment_review WHERE deviation_summary LIKE 'bld-mig-%'`).Error)

	// Seed pre-migration edge: a row with analysis_scope forced to '' (raw
	// SQL can write empty strings; AutoMigrate's DEFAULT only covers omitted
	// columns, and NULL cannot exist on a NOT NULL column).
	require.NoError(t, db.Exec(`INSERT INTO topic_enrichment_result (persistent_topic_id, evolution_assessment, session_id, created_at)
		VALUES (9201, 'legacy row', 'bld-mig-legacy', now())`).Error)
	require.NoError(t, db.Exec(`UPDATE topic_enrichment_result SET analysis_scope = '' WHERE session_id = 'bld-mig-legacy'`).Error)

	// Locate migration 20260826_0001's Up closure and run it in-tx (mirrors the
	// production in-transaction path).
	var up func(*gorm.DB) error
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260826_0001" {
			up = m.Up
			break
		}
	}
	require.NotNil(t, up, "migration 20260826_0001 not found in list")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))

	// 1. Legacy row backfilled to 'topic'.
	var scope string
	require.NoError(t, db.Raw(`SELECT analysis_scope FROM topic_enrichment_result WHERE session_id = 'bld-mig-legacy'`).Scan(&scope).Error)
	require.Equal(t, "topic", scope, "NULL analysis_scope must be backfilled to 'topic'")

	// 2. NOT NULL dropped: board-scoped rows (NULL topic) insert cleanly on
	// both tables.
	require.NoError(t, db.Exec(`INSERT INTO topic_enrichment_result (persistent_topic_id, semantic_board_id, analysis_scope, session_id, created_at)
		VALUES (NULL, 9301, 'board', 'bld-mig-board', now())`).Error)
	require.NoError(t, db.Exec(`INSERT INTO topic_enrichment_review (persistent_topic_id, semantic_board_id, curr_result_id, deviation_summary, created_at, updated_at)
		VALUES (NULL, 9301, 1, 'bld-mig-board-review', now(), now())`).Error)

	// 3. reference_roles table exists via AutoMigrate with the unique name index.
	require.NoError(t, db.Exec(`INSERT INTO reference_roles (name, content, enabled, created_at, updated_at)
		VALUES ('bld-mig-role', 'test content', true, now(), now())`).Error)
	err := db.Exec(`INSERT INTO reference_roles (name, content, enabled, created_at, updated_at)
		VALUES ('bld-mig-role', 'dup', true, now(), now())`).Error
	require.Error(t, err, "reference_roles.name must be unique")

	// 4. Topic-scoped inserts still work (regression: dropping NOT NULL must
	// not break the old write path).
	require.NoError(t, db.Exec(`INSERT INTO topic_enrichment_result (persistent_topic_id, analysis_scope, session_id, created_at)
		VALUES (9201, 'topic', 'bld-mig-topic2', now())`).Error)

	// 5. Seed migration: first reference role present with real content and
	// enabled (tasks 2.3 — 库中可查询到 enabled 文档). Locate and run
	// 20260826_0002 the same way (this test drives Ups directly, not RunMigrations).
	var upSeed func(*gorm.DB) error
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260826_0002" {
			upSeed = m.Up
			break
		}
	}
	require.NotNil(t, upSeed, "migration 20260826_0002 not found in list")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return upSeed(tx) }))
	// Idempotent: seeding twice is a no-op (ON CONFLICT DO NOTHING).
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return upSeed(tx) }))
	var roleName, roleTitle string
	var roleEnabled bool
	var roleLen int
	require.NoError(t, db.Raw(`SELECT name, title, enabled, length(content) FROM reference_roles WHERE name = 'inside-america-v2'`).Row().Scan(&roleName, &roleTitle, &roleEnabled, &roleLen))
	require.Equal(t, "inside-america-v2", roleName)
	require.Equal(t, true, roleEnabled)
	require.Greater(t, roleLen, 2000, "profile content should be the full v2 snapshot")

	// Idempotency: re-running Up is a no-op.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))
}
