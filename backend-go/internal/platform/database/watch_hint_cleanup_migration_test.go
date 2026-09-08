package database_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"

	// Side-effect: register the watch/report models so RunAutoMigrate creates
	// their tables (the migration target).
	_ "syntopica-backend/internal/topicgraph/repository"
)

// findWatchHintCleanupMigration locates migration 20260905_0001's Up closure.
func findWatchHintCleanupMigration() func(*gorm.DB) error {
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260905_0001" {
			return m.Up
		}
	}
	return nil
}

// seedHintCleanupFixture seeds one board with four watches (all track types)
// and one hint row per watch, returning the seeded watch ids by type.
func seedHintCleanupFixture(t *testing.T, db *gorm.DB) map[string]int {
	t.Helper()
	const boardID = 4345
	require.NoError(t, db.Exec(`INSERT INTO board_daily_reports
		(semantic_board_id, period_date, title, status, created_at, updated_at)
		VALUES (?, '2026-09-05', 'hint cleanup probe', 'completed', now(), now())`, boardID).Error)
	var reportID int
	require.NoError(t, db.Raw(`SELECT id FROM board_daily_reports WHERE semantic_board_id=? ORDER BY id DESC LIMIT 1`, boardID).Scan(&reportID).Error)
	require.NoError(t, db.Exec(`INSERT INTO daily_report_sections
		(report_id, cluster_label, article_count, created_at)
		VALUES (?, 'probe section', 1, now())`, reportID).Error)
	var sectionID int
	require.NoError(t, db.Raw(`SELECT id FROM daily_report_sections WHERE report_id=? ORDER BY id DESC LIMIT 1`, reportID).Scan(&sectionID).Error)

	ids := make(map[string]int)
	for _, typ := range []string{"label", "keyword", "keyword_topic", "sentence_topic"} {
		var watchID int
		require.NoError(t, db.Raw(`INSERT INTO board_topic_watches
			(semantic_board_id, label, type, status, created_at, updated_at)
			VALUES (?, ?, ?, 'active', now(), now()) RETURNING id`, boardID, typ+" probe", typ).Scan(&watchID).Error)
		ids[typ] = watchID
		require.NoError(t, db.Exec(`INSERT INTO topic_watch_hits
			(watch_id, section_id, report_id, period_date, reason, created_at)
			VALUES (?, ?, ?, '2026-09-05', 'probe hit', now())`, watchID, sectionID, reportID).Error)
	}
	return ids
}

// TestWatchMaterializedHintCleanupMigration verifies 20260905_0001: hint rows
// owned by materialized-track watches (keyword_topic / sentence_topic) are
// deleted, label/keyword-track hits survive, and the migration is idempotent.
// These rows violate the topic-watch spec (物化轨 SHALL NOT 产生命中提示记录)
// and were produced by the pre-fix hint evaluation fallthrough.
//
// Docker required. Skipped under -short.
func TestWatchMaterializedHintCleanupMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))

	up := findWatchHintCleanupMigration()
	require.NotNil(t, up, "migration 20260905_0001 not found in list")

	const boardID = 4345
	ids := seedHintCleanupFixture(t, db)
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM topic_watch_hits WHERE watch_id IN (SELECT id FROM board_topic_watches WHERE semantic_board_id=?)`, boardID).Error
		_ = db.Exec(`DELETE FROM board_topic_watches WHERE semantic_board_id = ?`, boardID).Error
		_ = db.Exec(`DELETE FROM daily_report_sections WHERE report_id IN (SELECT id FROM board_daily_reports WHERE semantic_board_id=?)`, boardID).Error
		_ = db.Exec(`DELETE FROM board_daily_reports WHERE semantic_board_id = ?`, boardID).Error
	})

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"migration Up must succeed with seeded fixture")

	// Materialized tracks: hint rows gone.
	for _, typ := range []string{"keyword_topic", "sentence_topic"} {
		var count int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM topic_watch_hits WHERE watch_id = ?`, ids[typ]).Scan(&count).Error)
		assert.Zero(t, count, "type=%s watch hint rows must be deleted", typ)
	}
	// Hint tracks: hits untouched.
	for _, typ := range []string{"label", "keyword"} {
		var count int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM topic_watch_hits WHERE watch_id = ?`, ids[typ]).Scan(&count).Error)
		assert.EqualValues(t, 1, count, "type=%s watch hint rows must survive", typ)
	}

	// Idempotency: re-run must succeed and change nothing.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"migration must be idempotent (re-run must not error)")
	var labelCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM topic_watch_hits WHERE watch_id = ?`, ids["label"]).Scan(&labelCount).Error)
	assert.EqualValues(t, 1, labelCount, "re-run must not touch hint-track rows")
}
