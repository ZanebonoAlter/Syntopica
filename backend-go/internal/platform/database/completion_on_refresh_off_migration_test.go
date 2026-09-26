package database_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// TestCompletionOnRefreshOffMigrationIdempotent verifies the one-shot gate
// shutdown (spec: 存量 feed 自动总结一次性关闭): every feed's
// completion_on_refresh flips to false, legacy pending/incomplete
// summary_status freezes to complete, failed/complete statuses survive, and
// a second run is a no-op.
func TestCompletionOnRefreshOffMigrationIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	var up func(*gorm.DB) error
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260920_0002" {
			up = m.Up
		}
	}
	require.NotNil(t, up, "migration 20260920_0002 must be registered")

	// Seed feeds: gate on (legacy default true) + already off.
	feedOn := models.Feed{Title: "gate-on", URL: "https://example.com/on", ArticleSummaryEnabled: true, CompletionOnRefresh: true, TaggingEnabled: true}
	feedOff := models.Feed{Title: "gate-off", URL: "https://example.com/off", ArticleSummaryEnabled: true, CompletionOnRefresh: false, TaggingEnabled: true}
	require.NoError(t, db.Create(&feedOn).Error)
	require.NoError(t, db.Create(&feedOff).Error)

	// Seed articles across every summary_status the migration must reason about.
	// "" must go through raw SQL: SummaryStatus carries gorm default:complete,
	// so a Create with the zero value would land as 'complete' instead.
	seedStatus := []string{"pending", "incomplete", "failed", "complete", ""}
	for i, status := range seedStatus {
		if status == "" {
			require.NoError(t, db.Exec(`INSERT INTO articles (feed_id, title, link, summary_status, firecrawl_status) VALUES (?, ?, ?, '', 'completed')`,
				feedOn.ID, "art-empty", fmt.Sprintf("https://example.com/a/%d", i)).Error)
			continue
		}
		art := models.Article{
			FeedID: feedOn.ID, Title: fmt.Sprintf("art-%s", status),
			Link:          fmt.Sprintf("https://example.com/a/%d", i),
			SummaryStatus: status, FirecrawlStatus: "completed",
		}
		require.NoError(t, db.Create(&art).Error)
	}

	require.NoError(t, up(db))

	// MG-2: every feed gate is now off.
	var gateTrueCount int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM feeds WHERE completion_on_refresh = true`).Scan(&gateTrueCount).Error)
	require.Zero(t, gateTrueCount, "all feeds must have completion_on_refresh=false after migration")

	// MG-3: pending/incomplete → complete; failed/complete/'' untouched.
	statusCount := func(status string) int64 {
		var n int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM articles WHERE summary_status = ?`, status).Scan(&n).Error)
		return n
	}
	require.Equal(t, int64(0), statusCount("pending"), "pending frozen to complete")
	require.Equal(t, int64(0), statusCount("incomplete"), "incomplete frozen to complete")
	require.Equal(t, int64(3), statusCount("complete"), "2 frozen + 1 original complete")
	require.Equal(t, int64(1), statusCount("failed"), "failed preserved for observability")
	require.Equal(t, int64(1), statusCount(""), "empty status untouched")

	// MG-4: idempotent — second run is a no-op.
	require.NoError(t, up(db))
	require.Equal(t, int64(0), statusCount("pending"))
	require.Equal(t, int64(0), statusCount("incomplete"))
	require.Equal(t, int64(3), statusCount("complete"))
	require.Equal(t, int64(1), statusCount("failed"))

	// MG-5: a feed created after the gate default flip (no explicit toggle)
	// lands completion_on_refresh=false in the DB (column default).
	fresh := models.Feed{Title: "fresh-feed", URL: "https://example.com/fresh", ArticleSummaryEnabled: true}
	require.NoError(t, db.Create(&fresh).Error)
	var freshGate bool
	require.NoError(t, db.Raw(`SELECT completion_on_refresh FROM feeds WHERE id = ?`, fresh.ID).Scan(&freshGate).Error)
	require.False(t, freshGate, "new feed must default completion_on_refresh=false")
}

// TestCompletionOnRefreshOffMigrationBacklogNotScanned verifies the scan side
// after the one-shot migration: legacy pending/incomplete backlog of a
// gate-off feed is frozen to complete, so the completion scheduler query
// (same predicate as ListReadyArticles) picks up nothing.
func TestCompletionOnRefreshOffMigrationBacklogNotScanned(t *testing.T) {
	db := testutil.SetupTestDB(t)
	var up func(*gorm.DB) error
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260920_0002" {
			up = m.Up
		}
	}
	require.NotNil(t, up, "migration 20260920_0002 must be registered")

	feed := models.Feed{Title: "legacy", URL: "https://example.com/legacy", ArticleSummaryEnabled: true, CompletionOnRefresh: true}
	require.NoError(t, db.Create(&feed).Error)
	for i, status := range []string{"pending", "incomplete"} {
		art := models.Article{
			FeedID: feed.ID, Title: "art-" + status,
			Link:          fmt.Sprintf("https://example.com/b/%d", i),
			SummaryStatus: status, FirecrawlStatus: "completed",
		}
		require.NoError(t, db.Create(&art).Error)
	}

	require.NoError(t, up(db))

	// Same predicate family as ListReadyArticles (scan side): with every gate
	// off, nothing is eligible for automatic completion anymore.
	var ready int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM articles a
		JOIN feeds f ON f.id = a.feed_id
		WHERE f.article_summary_enabled = true
		  AND f.completion_on_refresh = true
		  AND (a.summary_status = 'incomplete' OR a.summary_status = 'pending')
		  AND a.firecrawl_status IN ('completed', 'failed')
	`).Scan(&ready).Error)
	require.Zero(t, ready, "frozen backlog must not be scanned after migration")
}
