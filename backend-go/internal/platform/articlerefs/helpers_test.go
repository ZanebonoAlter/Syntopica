package articlerefs_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"

	// RegisterModels side effect: daily_report_threads is AutoMigrated from the
	// daily-report models, so this test package must link them in for the golden
	// test schema to contain the table the references live in.
	_ "syntopica-backend/internal/topicgraph/repository"
)

// seedArticleRow creates a live article row. articles.feed_id carries a foreign
// key, so every seed needs its own feed.
func seedArticleRow(t *testing.T, db *gorm.DB, name string) models.Article {
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

// seedThreadRow inserts a daily-report thread with a raw jsonb reference value,
// so tests reproduce the shapes the schema actually holds: arrays, the JSON null
// scalar historical writers left behind, and arrays carrying dirty elements.
// report_id/section_id carry no foreign key, so any value is valid here.
func seedThreadRow(t *testing.T, db *gorm.DB, id uint, refs string) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO daily_report_threads (id, report_id, section_id, title, related_article_ids)
		VALUES (?, ?, ?, ?, ?::jsonb)`, id, id, id, fmt.Sprintf("thread-%d", id), refs).Error)
}

// seedThreadNullRefs inserts a thread whose reference column is SQL NULL.
func seedThreadNullRefs(t *testing.T, db *gorm.DB, id uint) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO daily_report_threads (id, report_id, section_id, title, related_article_ids)
		VALUES (?, ?, ?, ?, NULL)`, id, id, id, fmt.Sprintf("thread-%d", id)).Error)
}

// refsText renders the stored jsonb as text: the cheapest exact assertion that
// includes element order.
func refsText(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var text string
	require.NoError(t, db.Raw(`SELECT related_article_ids::text FROM daily_report_threads WHERE id = ?`, id).Scan(&text).Error)
	return text
}

// refsType returns jsonb_typeof: "array" is the only value downstream readers
// (jsonb_array_elements_text) can consume.
func refsType(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var kind string
	require.NoError(t, db.Raw(`SELECT COALESCE(jsonb_typeof(related_article_ids), 'sql-null') FROM daily_report_threads WHERE id = ?`, id).Scan(&kind).Error)
	return kind
}

// refsIDs decodes the reference array in stored order. It raises SQLSTATE 22023
// when the value is a scalar, which is exactly the failure NormalizeThreadRefs
// exists to remove.
func refsIDs(t *testing.T, db *gorm.DB, id uint) []int64 {
	t.Helper()
	var ids []int64
	require.NoError(t, db.Raw(`SELECT e.elem::bigint AS id
		FROM daily_report_threads t
		CROSS JOIN LATERAL jsonb_array_elements_text(t.related_article_ids) WITH ORDINALITY AS e(elem, ord)
		WHERE t.id = ?
		ORDER BY e.ord`, id).Scan(&ids).Error)
	return ids
}

// refsXMins records each thread's xmin — the transaction that last wrote the row.
// It is the write detector for the "no UPDATE" scenarios: daily_report_threads
// stores no updated_at, and a raw UPDATE is invisible to GORM callbacks.
func refsXMins(t *testing.T, db *gorm.DB) map[uint]string {
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
