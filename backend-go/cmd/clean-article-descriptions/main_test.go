package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:clean-desc-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Article{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedArticle(t *testing.T, db *gorm.DB, description, content string) models.Article {
	t.Helper()
	article := models.Article{
		FeedID:      1,
		Title:       "测试文章",
		Link:        fmt.Sprintf("https://example.com/%d", time.Now().UnixNano()),
		Description: description,
		Content:     content,
	}
	require.NoError(t, db.Create(&article).Error)
	return article
}

func descriptionOf(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var stored models.Article
	require.NoError(t, db.First(&stored, id).Error)
	return stored.Description
}

func TestParseOptions(t *testing.T) {
	opts, err := parseOptions([]string{})
	require.NoError(t, err)
	require.False(t, opts.Apply, "the backfill must be a dry-run unless --apply is given")
	require.Equal(t, 200, opts.Batch)
	require.Equal(t, 0, opts.Limit)
	require.Equal(t, 5, opts.Samples)

	opts, err = parseOptions([]string{"--apply", "--batch=10", "--limit=3", "--samples=1"})
	require.NoError(t, err)
	require.True(t, opts.Apply)
	require.Equal(t, 10, opts.Batch)
	require.Equal(t, 3, opts.Limit)
	require.Equal(t, 1, opts.Samples)

	_, err = parseOptions([]string{"--batch=0"})
	require.Error(t, err)
	_, err = parseOptions([]string{"extra-arg"})
	require.Error(t, err)
}

func TestPlanUpdatesSkipsAlreadyCleanRows(t *testing.T) {
	rows := []candidate{
		{ID: 1, Description: "<p>正文</p>"},
		{ID: 2, Description: "已经清洗好的摘要"},
		{ID: 3, Description: `<div style="opacity:0">隐藏</div>`},
	}

	updates, bytesBefore, bytesAfter := planUpdates(rows, descriptionSummaryMaxRunes)

	require.Len(t, updates, 2, "the already clean row must not be rewritten")
	require.Equal(t, uint(1), updates[0].ID)
	require.Equal(t, "正文", updates[0].After)
	require.Equal(t, uint(3), updates[1].ID)
	require.Equal(t, "", updates[1].After, "a hidden subtree carries no visible text")
	require.Greater(t, bytesBefore, bytesAfter)
}

func TestRunDryRunDoesNotWrite(t *testing.T) {
	db := testDB(t)
	html := "<div style=\"visibility: visible; opacity: 0;\"><p>" + "正文内容" + "</p></div>"
	article := seedArticle(t, db, html, html)

	stats, err := run(context.Background(), db, options{Batch: 10, Samples: 1}, io.Discard)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Scanned)
	require.Equal(t, 1, stats.Affected)
	require.Equal(t, 0, stats.Updated, "dry-run must not write")
	require.Equal(t, html, descriptionOf(t, db, article.ID), "dry-run must leave the row untouched")
}

func TestRunApplyOnlyTouchesDescription(t *testing.T) {
	db := testDB(t)
	html := "<div style=\"visibility: visible; opacity: 0;\"><p>" + "正文内容" + "</p></div>"
	article := seedArticle(t, db, html, html)
	clean := seedArticle(t, db, "普通摘要", "<p>正文</p>")

	stats, err := run(context.Background(), db, options{Apply: true, Batch: 10, Samples: 1}, io.Discard)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Affected)
	require.Equal(t, 1, stats.Updated)

	var stored models.Article
	require.NoError(t, db.First(&stored, article.ID).Error)
	require.Equal(t, "", stored.Description, "the hidden subtree carries no visible text")
	require.Equal(t, html, stored.Content, "the body source field must not be rewritten")
	require.Equal(t, "普通摘要", descriptionOf(t, db, clean.ID), "already clean rows stay untouched")
}

func TestRunSecondPassAffectsNothing(t *testing.T) {
	db := testDB(t)
	seedArticle(t, db, "<div><p>正文内容</p></div>", "<p>正文</p>")

	first, err := run(context.Background(), db, options{Apply: true, Batch: 10}, io.Discard)
	require.NoError(t, err)
	require.Equal(t, 1, first.Affected)

	second, err := run(context.Background(), db, options{Apply: true, Batch: 10}, io.Discard)
	require.NoError(t, err)
	require.Equal(t, 0, second.Affected, "a second run must not change anything")
	require.Equal(t, 0, second.Updated)
}

func TestPlanUpdatesLeavesPlainTextWithAngleBracketAlone(t *testing.T) {
	// A bare comparison sign is not a tag: the SQL pre-filter ("contains a <")
	// catches the row, so the real markup test must reject it — otherwise the
	// backfill rewrites a summary the spec promises to keep verbatim.
	//
	// (`使用 <br> 标签` deliberately does NOT belong here: <br> is a real tag,
	// so cleaning it is the contract, and the cross-implementation sample table
	// pins that behaviour on both sides.)
	plain := "a < b 是正常比较符号"
	rows := []candidate{
		{ID: 1, Description: plain},
		{ID: 2, Description: "<p>真标记</p>"},
	}

	updates, _, _ := planUpdates(rows, descriptionSummaryMaxRunes)

	require.Len(t, updates, 1, "only the row carrying real markup may be rewritten")
	require.Equal(t, uint(2), updates[0].ID)
	require.Equal(t, "真标记", updates[0].After)
}

// TestRunCleansEscapedMarkupRows locks the scan filter to the cleaner's own
// markup judgment: htmltext.HasMarkup decodes entities first, so an escaped
// tag is markup even though the stored value carries no literal '<'. The SQL
// pre-filter must cover those rows too, otherwise the cleaner would rewrite
// them but they are never scanned.
func TestRunCleansEscapedMarkupRows(t *testing.T) {
	db := testDB(t)
	article := seedArticle(t, db, "&lt;b&gt;加粗的摘要&lt;/b&gt;", "<p>正文</p>")

	stats, err := run(context.Background(), db, options{Apply: true, Batch: 10}, io.Discard)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Affected, "escaped markup must be scanned and cleaned")
	require.Equal(t, "加粗的摘要", descriptionOf(t, db, article.ID))
}

func TestPlanUpdatesTruncatesOverlongPlainText(t *testing.T) {
	// No markup at all, but longer than the cap: the "summary ≤300 characters"
	// invariant still has to hold after a backfill.
	long := strings.Repeat("导", 350)
	rows := []candidate{{ID: 1, Description: long}}

	updates, bytesBefore, bytesAfter := planUpdates(rows, descriptionSummaryMaxRunes)

	require.Len(t, updates, 1)
	require.LessOrEqual(t, len([]rune(updates[0].After)), descriptionSummaryMaxRunes)
	require.True(t, strings.HasSuffix(updates[0].After, "…"), "truncation must be visible")
	require.Greater(t, bytesBefore, bytesAfter)
}
