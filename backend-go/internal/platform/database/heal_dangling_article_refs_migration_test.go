package database_test

// File-name note (test isolation, NOT style): this package shares one
// testcontainer and builds its golden schema on the first testutil.SetupTestDB
// call, which is ordered by file name. TestBoardLevelAnalysisScopeMigration calls
// database.RunAutoMigrate mid-process, and GORM relaxes NOT NULL on columns whose
// model tag no longer declares it — migration 20260723_0001 re-materializes those
// constraints, but only when the golden build runs afterwards. Naming this file
// "heal_…" keeps it after constraints_test.go (the previous first SetupTestDB
// caller) so the golden build still happens before that test, exactly as before.

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"

	// RegisterModels side effect: daily_report_threads is AutoMigrated from the
	// daily-report models, so this package must link them in for the golden test
	// schema to contain the table the migration repairs.
	_ "syntopica-backend/internal/topicgraph/repository"
)

// findHealDanglingArticleRefsMigration locates migration 20260917_0002's Up
// closure, so the test drives the repair directly instead of only trusting the
// golden-schema build.
func findHealDanglingArticleRefsMigration() func(*gorm.DB) error {
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260917_0002" {
			return m.Up
		}
	}
	return nil
}

func seedRefArticle(t *testing.T, db *gorm.DB, name string) models.Article {
	t.Helper()
	feed := models.Feed{Title: name, URL: fmt.Sprintf("https://example.com/%s", name)}
	require.NoError(t, db.Create(&feed).Error)
	article := models.Article{
		FeedID: feed.ID,
		Title:  name,
		Link:   fmt.Sprintf("https://example.com/%s/story", name),
	}
	require.NoError(t, db.Create(&article).Error)
	return article
}

func seedThreadRefs(t *testing.T, db *gorm.DB, id uint, refs string) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO daily_report_threads (id, report_id, section_id, title, related_article_ids)
		VALUES (?, ?, ?, ?, ?::jsonb)`, id, id, id, fmt.Sprintf("thread-%d", id), refs).Error)
}

func seedThreadNullRefs(t *testing.T, db *gorm.DB, id uint) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO daily_report_threads (id, report_id, section_id, title, related_article_ids)
		VALUES (?, ?, ?, ?, NULL)`, id, id, id, fmt.Sprintf("thread-%d", id)).Error)
}

func threadRefsText(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var text string
	require.NoError(t, db.Raw(`SELECT related_article_ids::text FROM daily_report_threads WHERE id = ?`, id).Scan(&text).Error)
	return text
}

func threadRefsType(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var kind string
	require.NoError(t, db.Raw(`SELECT COALESCE(jsonb_typeof(related_article_ids), 'sql-null') FROM daily_report_threads WHERE id = ?`, id).Scan(&kind).Error)
	return kind
}

// threadXMins records the writer transaction of every thread row. The repair
// must not issue an UPDATE for rows that are already correct, and
// daily_report_threads carries no updated_at while a raw Exec bypasses GORM
// callbacks — xmin is the available write detector.
func threadXMins(t *testing.T, db *gorm.DB) map[uint]string {
	t.Helper()
	rows := []struct {
		ID   uint
		Xmin string
	}{}
	require.NoError(t, db.Raw(`SELECT id, xmin::text AS xmin FROM daily_report_threads ORDER BY id`).Scan(&rows).Error)
	out := make(map[uint]string, len(rows))
	for _, row := range rows {
		out[row.ID] = row.Xmin
	}
	return out
}

// TestHealDanglingArticleRefsMigrationRepairsReferences covers the production
// shape the migration was written for: threads citing articles deleted before
// any delete path maintained references, mixed with the JSON null / SQL NULL
// scalars that made those rows unreadable.
func TestHealDanglingArticleRefsMigrationRepairsReferences(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findHealDanglingArticleRefsMigration()
	require.NotNil(t, up, "migration 20260917_0002 must be registered")

	aliveA := seedRefArticle(t, db, "integrity-a")
	aliveB := seedRefArticle(t, db, "integrity-b")

	seedThreadRefs(t, db, 1, fmt.Sprintf("[%d, 900001, %d]", aliveA.ID, aliveB.ID))
	seedThreadRefs(t, db, 2, "[900002, 900003]")
	seedThreadRefs(t, db, 3, fmt.Sprintf("[%d]", aliveA.ID))
	seedThreadRefs(t, db, 4, "null")
	seedThreadNullRefs(t, db, 5)
	clean := threadRefsText(t, db, 3)

	require.NoError(t, up(db))

	require.Equal(t, fmt.Sprintf("[%d, %d]", aliveA.ID, aliveB.ID), threadRefsText(t, db, 1),
		"orphaned reference dropped, order of the survivors preserved")
	require.Equal(t, "array", threadRefsType(t, db, 2))
	require.Equal(t, "[]", threadRefsText(t, db, 2), "a fully orphaned array becomes []")
	require.Equal(t, clean, threadRefsText(t, db, 3), "a clean row is untouched")
	require.Equal(t, "array", threadRefsType(t, db, 4))
	require.Equal(t, "[]", threadRefsText(t, db, 4), "the JSON null scalar becomes []")
	require.Equal(t, "array", threadRefsType(t, db, 5))
	require.Equal(t, "[]", threadRefsText(t, db, 5), "SQL NULL becomes []")

	// The repaired column must be readable by the query that motivated the
	// normalization (a scalar raises SQLSTATE 22023 here).
	var refCount int
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM (
		SELECT jsonb_array_elements_text(related_article_ids) FROM daily_report_threads
	) AS refs`).Scan(&refCount).Error)
}

// TestHealDanglingArticleRefsMigrationIsIdempotent covers the re-run path: the
// migration is recorded once, but a restored dump or a manual re-run must not
// rewrite rows that are already correct.
func TestHealDanglingArticleRefsMigrationIsIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findHealDanglingArticleRefsMigration()
	require.NotNil(t, up)

	alive := seedRefArticle(t, db, "idempotent-alive")
	seedThreadRefs(t, db, 1, fmt.Sprintf("[%d, 900011]", alive.ID))
	seedThreadRefs(t, db, 2, "null")

	require.NoError(t, up(db))
	afterFirst := threadXMins(t, db)
	refsAfterFirst := threadRefsText(t, db, 1)

	require.NoError(t, up(db), "re-running the migration must not fail")
	require.Equal(t, afterFirst, threadXMins(t, db), "the second run must not issue any UPDATE")
	require.Equal(t, refsAfterFirst, threadRefsText(t, db, 1))
}

// TestHealDanglingArticleRefsMigrationWritesNothingWhenClean covers the
// no-dangling-data scenario: a database whose references all resolve must not be
// rewritten at all.
func TestHealDanglingArticleRefsMigrationWritesNothingWhenClean(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findHealDanglingArticleRefsMigration()
	require.NotNil(t, up)

	aliveA := seedRefArticle(t, db, "clean-a")
	aliveB := seedRefArticle(t, db, "clean-b")
	seedThreadRefs(t, db, 1, fmt.Sprintf("[%d, %d]", aliveA.ID, aliveB.ID))
	seedThreadRefs(t, db, 2, "[]")

	before := threadXMins(t, db)
	beforeText := threadRefsText(t, db, 1)

	require.NoError(t, up(db))

	require.Equal(t, before, threadXMins(t, db), "clean data must not be rewritten")
	require.Equal(t, beforeText, threadRefsText(t, db, 1))
}

// TestHealDanglingArticleRefsMigrationSkipsMissingTable covers the binaries that
// never register the daily-report models (CLI tools): the table is AutoMigrated
// from the model rather than created by a versioned migration, so the repair has
// to step aside instead of failing their startup. The table is renamed rather
// than dropped: dropping it and re-running AutoMigrate would relax constraints
// the versioned migrations materialized for other tables, breaking every later
// test in this package.
func TestHealDanglingArticleRefsMigrationSkipsMissingTable(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findHealDanglingArticleRefsMigration()
	require.NotNil(t, up)

	require.NoError(t, db.Exec(`ALTER TABLE daily_report_threads RENAME TO daily_report_threads_absent`).Error)
	defer func() {
		require.NoError(t, db.Exec(`ALTER TABLE daily_report_threads_absent RENAME TO daily_report_threads`).Error)
	}()

	require.NoError(t, up(db), "a missing table is a no-op, not a startup failure")
}
