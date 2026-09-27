package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"

	// Side-effect: register margin note models so AutoMigrate can create
	// annotation_qas for the migration target.
	_ "syntopica-backend/internal/topicgraph/repository"
)

// findMarginNotesWebSourcesMigration locates migration 20260924_0001's Up
// closure (daily-report-margin-notes design D7: cited_web_sources jsonb).
func findMarginNotesWebSourcesMigration() func(*gorm.DB) error {
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260924_0001" {
			return m.Up
		}
	}
	return nil
}

// TestMarginNotesWebSourcesMigration exercises migration 20260924_0001
// against a testcontainer PG: column added with NOT NULL DEFAULT '[]'::jsonb,
// historical rows backfilled to [], and idempotent on re-run (WS-9).
func TestMarginNotesWebSourcesMigration(t *testing.T) {
	db := testutil.SetupTestDB(t)

	up := findMarginNotesWebSourcesMigration()
	require.NotNil(t, up, "migration 20260924_0001 must be registered")

	// annotation_qas must exist (AutoMigrate via model registration) — create
	// one historical row first so the backfill path is exercised. FK 要求先有
	// 父行 report_annotations（CASCADE 关系，tasks 2.1）。
	require.NoError(t, db.Exec(`INSERT INTO report_annotations (report_id, section_id, quoted_text, created_at)
		VALUES (1, 0, '历史划词', now())`).Error)
	require.NoError(t, db.Exec(`INSERT INTO annotation_qas (annotation_id, question, answer, created_at)
		VALUES (1, '历史问题', '历史回答', now())`).Error)

	require.NoError(t, up(db), "first run must succeed")

	// Historical row backfilled to [] (NOT NULL + DEFAULT semantics).
	var cnt int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM annotation_qas WHERE cited_web_sources IS NULL`).Scan(&cnt).Error)
	require.Zero(t, cnt, "no NULL cited_web_sources after migration")

	// Idempotent: second run must succeed (WS-9).
	require.NoError(t, up(db), "re-run must be idempotent")

	// Default applies to fresh inserts without the column specified.
	require.NoError(t, db.Exec(`INSERT INTO report_annotations (report_id, section_id, quoted_text, created_at)
		VALUES (1, 0, '第二划词', now())`).Error)
	require.NoError(t, db.Exec(`INSERT INTO annotation_qas (annotation_id, question, answer, created_at)
		VALUES (2, 'q', 'a', now())`).Error)
	var emptyCnt int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM annotation_qas WHERE cited_web_sources = '[]'::jsonb`).Scan(&emptyCnt).Error)
	require.EqualValues(t, 2, emptyCnt, "default '[]' applies to all rows")
}
