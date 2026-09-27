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

// findDedupeRSSArticlesMigration locates migration 20260917_0001's Up closure.
func findDedupeRSSArticlesMigration() func(*gorm.DB) error {
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260917_0001" {
			return m.Up
		}
	}
	return nil
}

// seedDuplicateArticleGroup reproduces the pre-migration state: the golden
// schema already ran 20260917_0001 (so the unique index is in place), hence the
// index is dropped first — the migration must cope with a tree that still
// carries duplicate rows for one (feed_id, link).
func seedDuplicateArticleGroup(t *testing.T, db *gorm.DB, feedID uint, link string, n int) []models.Article {
	t.Helper()
	require.NoError(t, db.Exec(`DROP INDEX IF EXISTS uq_articles_feed_link`).Error)
	rows := make([]models.Article, 0, n)
	for i := 0; i < n; i++ {
		a := models.Article{FeedID: feedID, Title: fmt.Sprintf("快讯标题%d", i), Link: link}
		require.NoError(t, db.Create(&a).Error)
		rows = append(rows, a)
	}
	return rows
}

func countRowsForArticle(t *testing.T, db *gorm.DB, table string, articleID uint) int {
	t.Helper()
	var n int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM "+table+" WHERE article_id = ?", articleID).Scan(&n).Error)
	return n
}

// TestDedupeRSSArticlesMigrationMergesGroup covers the core merge: the most
// complete row survives, tag links from the copies are unioned without
// duplicates, reading behaviors are re-pointed, queued jobs are re-pointed or
// dropped, and the keeper's tag_count is recomputed.
func TestDedupeRSSArticlesMigrationMergesGroup(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findDedupeRSSArticlesMigration()
	require.NotNil(t, up, "migration 20260917_0001 must be registered")

	feed := models.Feed{Title: "重复源", URL: "https://example.com/dup-group"}
	require.NoError(t, db.Create(&feed).Error)
	link := "https://example.com/dup-group/story"
	rows := seedDuplicateArticleGroup(t, db, feed.ID, link, 3)
	bare, rich, overlapping := rows[0], rows[1], rows[2]

	// rich: crawled body + generated summary + two tags.
	require.NoError(t, db.Model(&models.Article{}).Where("id = ?", rich.ID).
		Updates(map[string]interface{}{"firecrawl_content": "正文", "ai_content_summary": "摘要"}).Error)

	tags := []models.TopicTag{
		{Label: "美联储", Slug: "fed-merge", Category: "event", Status: "active"},
		{Label: "美股", Slug: "us-stocks-merge", Category: "keyword", Status: "active"},
		{Label: "鲍威尔", Slug: "powell-merge", Category: "person", Status: "active"},
	}
	for i := range tags {
		require.NoError(t, db.Create(&tags[i]).Error)
	}
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: rich.ID, TopicTagID: tags[0].ID, Score: 0.9, Source: "llm"}).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: rich.ID, TopicTagID: tags[1].ID, Score: 0.8, Source: "llm"}).Error)
	// overlapping: one shared tag (conflict) + one exclusive tag + a behavior.
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: overlapping.ID, TopicTagID: tags[1].ID, Score: 0.7, Source: "reuse"}).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: overlapping.ID, TopicTagID: tags[2].ID, Score: 0.6, Source: "llm"}).Error)
	require.NoError(t, db.Exec(`INSERT INTO reading_behaviors (article_id, feed_id, event_type, created_at)
		VALUES (?, ?, 'favorite', now())`, overlapping.ID, feed.ID).Error)
	// Queued work of a doomed copy: finished rows are dropped, pending rows move.
	require.NoError(t, db.Exec(`INSERT INTO tag_jobs (article_id, status, available_at, created_at, updated_at)
		VALUES (?, 'pending', now(), now(), now())`, overlapping.ID).Error)
	require.NoError(t, db.Exec(`INSERT INTO tag_jobs (article_id, status, available_at, created_at, updated_at)
		VALUES (?, 'completed', now(), now(), now())`, overlapping.ID).Error)
	require.NoError(t, db.Exec(`INSERT INTO firecrawl_jobs (article_id, status, available_at, created_at, updated_at)
		VALUES (?, 'pending', now(), now(), now())`, overlapping.ID).Error)

	require.NoError(t, up(db))

	var survivors []models.Article
	require.NoError(t, db.Where("feed_id = ? AND link = ?", feed.ID, link).Find(&survivors).Error)
	require.Len(t, survivors, 1, "group must collapse to a single row")
	require.Equal(t, rich.ID, survivors[0].ID, "crawled + summarised + tagged copy must win")

	var links []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", rich.ID).Order("topic_tag_id").Find(&links).Error)
	require.Len(t, links, 3, "tag links must be unioned without duplicates")

	var counter int
	require.NoError(t, db.Raw(`SELECT tag_count FROM articles WHERE id = ?`, rich.ID).Scan(&counter).Error)
	require.Equal(t, 3, counter, "keeper tag_count must be recomputed")

	require.Equal(t, 1, countRowsForArticle(t, db, "reading_behaviors", rich.ID))
	require.Zero(t, countRowsForArticle(t, db, "reading_behaviors", overlapping.ID))
	require.Equal(t, 1, countRowsForArticle(t, db, "tag_jobs", rich.ID))
	require.Zero(t, countRowsForArticle(t, db, "tag_jobs", overlapping.ID), "finished jobs of the copy are dropped")
	require.Equal(t, 1, countRowsForArticle(t, db, "firecrawl_jobs", rich.ID))
	require.Zero(t, countRowsForArticle(t, db, "firecrawl_jobs", overlapping.ID))
	require.Zero(t, countRowsForArticle(t, db, "article_topic_tags", bare.ID))
}

// TestDedupeRSSArticlesMigrationKeepsActiveOverArchived covers the 89 mixed
// groups found in production: an archived-but-tagged row must still lose to an
// active row, otherwise the merge would drop the article out of the live window.
func TestDedupeRSSArticlesMigrationKeepsActiveOverArchived(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findDedupeRSSArticlesMigration()
	require.NotNil(t, up)

	feed := models.Feed{Title: "归档源", URL: "https://example.com/dup-archived"}
	require.NoError(t, db.Create(&feed).Error)
	link := "https://example.com/dup-archived/story"
	rows := seedDuplicateArticleGroup(t, db, feed.ID, link, 2)
	archived, active := rows[0], rows[1]
	require.NoError(t, db.Model(&models.Article{}).Where("id = ?", archived.ID).Update("archived", true).Error)

	tag := models.TopicTag{Label: "归档话题", Slug: "archived-topic", Category: "keyword", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: archived.ID, TopicTagID: tag.ID, Score: 0.5, Source: "llm"}).Error)

	require.NoError(t, up(db))

	var survivors []models.Article
	require.NoError(t, db.Where("feed_id = ? AND link = ?", feed.ID, link).Find(&survivors).Error)
	require.Len(t, survivors, 1)
	require.Equal(t, active.ID, survivors[0].ID, "active row must win over the archived one")
	require.False(t, survivors[0].Archived)
	require.Equal(t, 1, countRowsForArticle(t, db, "article_topic_tags", active.ID), "archived copy's tag moves to the survivor")
}

// TestDedupeRSSArticlesMigrationIdempotent covers the re-run path: applying the
// migration twice leaves the very same rows behind.
func TestDedupeRSSArticlesMigrationIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findDedupeRSSArticlesMigration()
	require.NotNil(t, up)

	feed := models.Feed{Title: "幂等源", URL: "https://example.com/dup-idempotent"}
	require.NoError(t, db.Create(&feed).Error)
	seedDuplicateArticleGroup(t, db, feed.ID, "https://example.com/dup-idempotent/story", 3)

	require.NoError(t, up(db))
	var afterFirst []uint
	require.NoError(t, db.Model(&models.Article{}).Where("feed_id = ?", feed.ID).Order("id").Pluck("id", &afterFirst).Error)
	require.Len(t, afterFirst, 1)

	require.NoError(t, up(db), "re-running the migration must not fail")
	var afterSecond []uint
	require.NoError(t, db.Model(&models.Article{}).Where("feed_id = ?", feed.ID).Order("id").Pluck("id", &afterSecond).Error)
	require.Equal(t, afterFirst, afterSecond)
}

// TestDedupeRSSArticlesMigrationEnforcesUniqueIndex covers the constraint the
// merge installs: duplicates are rejected afterwards while empty links stay
// exempt (legacy rows cannot be deduped).
func TestDedupeRSSArticlesMigrationEnforcesUniqueIndex(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findDedupeRSSArticlesMigration()
	require.NotNil(t, up)

	feed := models.Feed{Title: "约束源", URL: "https://example.com/dup-unique"}
	require.NoError(t, db.Create(&feed).Error)
	link := "https://example.com/dup-unique/story"
	seedDuplicateArticleGroup(t, db, feed.ID, link, 2)

	require.NoError(t, up(db))

	dup := models.Article{FeedID: feed.ID, Title: "并发重复", Link: link}
	require.Error(t, db.Create(&dup).Error, "unique (feed_id, link) index must reject a second row")

	require.NoError(t, db.Create(&models.Article{FeedID: feed.ID, Title: "无链接1"}).Error)
	require.NoError(t, db.Create(&models.Article{FeedID: feed.ID, Title: "无链接2"}).Error,
		"empty-link rows must stay exempt from the unique index")
}

// TestDedupeRSSArticlesMigrationRewiresDailyReportRefs covers the reference
// maintenance the merge gained (heal-dangling-article-refs D2). Before it, the
// loser copy was deleted while daily-report threads kept citing it, which is how
// production 线索 ended up resolving to "文章 #<id>" instead of a source.
func TestDedupeRSSArticlesMigrationRewiresDailyReportRefs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findDedupeRSSArticlesMigration()
	require.NotNil(t, up)

	feed := models.Feed{Title: "引用源", URL: "https://example.com/dup-refs"}
	require.NoError(t, db.Create(&feed).Error)
	link := "https://example.com/dup-refs/story"
	rows := seedDuplicateArticleGroup(t, db, feed.ID, link, 3)
	bare, rich, overlapping := rows[0], rows[1], rows[2]

	// rich wins the keeper score (crawled body), so the other two are doomed.
	require.NoError(t, db.Model(&models.Article{}).Where("id = ?", rich.ID).
		Update("firecrawl_content", "正文").Error)

	seedThreadRefs(t, db, 1, fmt.Sprintf("[%d, 7001]", overlapping.ID))
	seedThreadRefs(t, db, 2, fmt.Sprintf("[%d, %d, 7002]", bare.ID, rich.ID))
	seedThreadRefs(t, db, 3, fmt.Sprintf("[%d]", rich.ID))

	require.NoError(t, up(db))

	var survivors []models.Article
	require.NoError(t, db.Where("feed_id = ? AND link = ?", feed.ID, link).Find(&survivors).Error)
	require.Len(t, survivors, 1)
	require.Equal(t, rich.ID, survivors[0].ID)

	require.Equal(t, fmt.Sprintf("[%d, 7001]", rich.ID), threadRefsText(t, db, 1),
		"the doomed copy's slot now points at the keeper")
	require.Equal(t, fmt.Sprintf("[%d, 7002]", rich.ID), threadRefsText(t, db, 2),
		"a keeper already present in the array is deduplicated")
	require.Equal(t, fmt.Sprintf("[%d]", rich.ID), threadRefsText(t, db, 3))
}
