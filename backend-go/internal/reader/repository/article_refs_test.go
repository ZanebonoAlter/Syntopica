package repository_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/reader/repository"

	// RegisterModels side effect: daily_report_threads is AutoMigrated from the
	// daily-report models, so this package must link them in for the golden test
	// schema to contain the table whose references the delete paths maintain.
	_ "syntopica-backend/internal/topicgraph/repository"
)

func seedRefFeed(t *testing.T, db *gorm.DB, name string, articleCount int) (models.Feed, []models.Article) {
	t.Helper()
	feed := models.Feed{Title: name, URL: fmt.Sprintf("https://example.com/%s", name)}
	require.NoError(t, db.Create(&feed).Error)
	articles := make([]models.Article, 0, articleCount)
	for i := 0; i < articleCount; i++ {
		article := models.Article{
			FeedID: feed.ID,
			Title:  fmt.Sprintf("%s-%d", name, i),
			Link:   fmt.Sprintf("https://example.com/%s/story-%d", name, i),
		}
		require.NoError(t, db.Create(&article).Error)
		articles = append(articles, article)
	}
	return feed, articles
}

// seedRefTopicTag creates the event tag an article edge points at. The tests
// here seed edges by hand so the dependent-row cleanup is exercised without the
// tagging pipeline.
func seedRefTopicTag(t *testing.T, db *gorm.DB, label string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{Label: label, Slug: label, Category: models.TagCategoryEvent, Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

// seedArticleDependents gives every article the rows that belong to it: a tag
// edge, a queued tag job and a queued firecrawl job. A long-lived database
// cascades these from the article through foreign keys
// (fk_article_topic_tags_article / fk_tag_jobs_article / fk_firecrawl_jobs_article),
// a freshly built one carries no such constraints — so the delete paths must
// remove them explicitly or leave orphans behind.
func seedArticleDependents(t *testing.T, db *gorm.DB, tagID uint, articleIDs ...uint) {
	t.Helper()
	for _, articleID := range articleIDs {
		require.NoError(t, db.Create(&models.ArticleTopicTag{
			ArticleID: articleID, TopicTagID: tagID, Score: 0.5, Source: "llm",
		}).Error)
		require.NoError(t, db.Create(&models.TagJob{
			ArticleID: articleID, Status: "pending", AvailableAt: time.Now(),
		}).Error)
		require.NoError(t, db.Create(&models.FirecrawlJob{
			ArticleID: articleID, Status: "pending", AvailableAt: time.Now(),
		}).Error)
	}
}

// countArticleDependents counts the per-article rows still attached to ids.
func countArticleDependents(t *testing.T, db *gorm.DB, articleIDs []uint) (edges, tagJobs, crawlJobs int64) {
	t.Helper()
	require.NoError(t, db.Model(&models.ArticleTopicTag{}).Where("article_id IN ?", articleIDs).Count(&edges).Error)
	require.NoError(t, db.Model(&models.TagJob{}).Where("article_id IN ?", articleIDs).Count(&tagJobs).Error)
	require.NoError(t, db.Model(&models.FirecrawlJob{}).Where("article_id IN ?", articleIDs).Count(&crawlJobs).Error)
	return edges, tagJobs, crawlJobs
}

func seedRefThread(t *testing.T, db *gorm.DB, id uint, refs string) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO daily_report_threads (id, report_id, section_id, title, related_article_ids)
		VALUES (?, ?, ?, ?, ?::jsonb)`, id, id, id, fmt.Sprintf("thread-%d", id), refs).Error)
}

func seedRefThreadNullRefs(t *testing.T, db *gorm.DB, id uint) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO daily_report_threads (id, report_id, section_id, title, related_article_ids)
		VALUES (?, ?, ?, ?, NULL)`, id, id, id, fmt.Sprintf("thread-%d", id)).Error)
}

func refThreadText(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var text string
	require.NoError(t, db.Raw(`SELECT related_article_ids::text FROM daily_report_threads WHERE id = ?`, id).Scan(&text).Error)
	return text
}

func refThreadType(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var kind string
	require.NoError(t, db.Raw(`SELECT COALESCE(jsonb_typeof(related_article_ids), 'sql-null') FROM daily_report_threads WHERE id = ?`, id).Scan(&kind).Error)
	return kind
}

// TestDeleteFeedCascadePrunesArticleReferences covers the feed-deletion path: the
// articles used to disappear through the foreign-key cascade with their daily
// report references left behind, so the surviving 线索 resolved to "文章 #<id>".
func TestDeleteFeedCascadePrunesArticleReferences(t *testing.T) {
	db := testutil.SetupTestDB(t)
	feed, articles := seedRefFeed(t, db, "cascade-source", 2)
	_, keep := seedRefFeed(t, db, "cascade-keep", 1)

	require.NoError(t, db.Exec(`INSERT INTO reading_behaviors (article_id, feed_id, event_type, created_at)
		VALUES (?, ?, 'favorite', now())`, articles[0].ID, feed.ID).Error)

	seedRefThread(t, db, 1, fmt.Sprintf("[%d, %d, %d]", articles[0].ID, keep[0].ID, articles[1].ID))

	require.NoError(t, repository.NewReaderRepository(db).DeleteFeedCascade(feed.ID))

	var feedCount int64
	require.NoError(t, db.Model(&models.Feed{}).Where("id = ?", feed.ID).Count(&feedCount).Error)
	require.Zero(t, feedCount, "the feed row is deleted")
	var articleCount int64
	require.NoError(t, db.Model(&models.Article{}).Where("feed_id = ?", feed.ID).Count(&articleCount).Error)
	require.Zero(t, articleCount, "its articles are deleted")
	var behaviorCount int64
	require.NoError(t, db.Model(&models.ReadingBehavior{}).Where("feed_id = ?", feed.ID).Count(&behaviorCount).Error)
	require.Zero(t, behaviorCount, "existing behavior cleanup is preserved")

	require.Equal(t, fmt.Sprintf("[%d]", keep[0].ID), refThreadText(t, db, 1),
		"the deleted articles' references are pruned, the survivor keeps its order")
	require.Equal(t, "array", refThreadType(t, db, 1))
}

// TestDeleteArticlesByFeedPrunesReferences covers the raw delete primitive: the
// feed survives, only its articles go, and an array emptied by the deletion
// becomes [] rather than a JSON null.
func TestDeleteArticlesByFeedPrunesReferences(t *testing.T) {
	db := testutil.SetupTestDB(t)
	feed, articles := seedRefFeed(t, db, "articles-only", 1)
	seedRefThread(t, db, 1, fmt.Sprintf("[%d]", articles[0].ID))

	require.NoError(t, repository.NewReaderRepository(db).DeleteArticlesByFeed(feed.ID))

	var articleCount int64
	require.NoError(t, db.Model(&models.Article{}).Where("feed_id = ?", feed.ID).Count(&articleCount).Error)
	require.Zero(t, articleCount)
	var feedCount int64
	require.NoError(t, db.Model(&models.Feed{}).Where("id = ?", feed.ID).Count(&feedCount).Error)
	require.EqualValues(t, 1, feedCount, "the primitive deletes articles, not the feed")

	require.Equal(t, "array", refThreadType(t, db, 1))
	require.Equal(t, "[]", refThreadText(t, db, 1))
}

// TestDeleteFeedCascadeLeavesUnrelatedRowsAlone pins the privacy of the
// maintenance: only rows citing the deleted articles are rewritten, so a thread
// unrelated to the feed (and a scalar row nobody can read) stay byte-identical.
func TestDeleteFeedCascadeLeavesUnrelatedRowsAlone(t *testing.T) {
	db := testutil.SetupTestDB(t)
	feed, _ := seedRefFeed(t, db, "cascade-unrelated", 1)
	_, keep := seedRefFeed(t, db, "cascade-unrelated-keep", 2)

	seedRefThread(t, db, 1, fmt.Sprintf("[%d, %d]", keep[0].ID, keep[1].ID))
	seedRefThreadNullRefs(t, db, 2)
	before := refThreadText(t, db, 1)

	require.NoError(t, repository.NewReaderRepository(db).DeleteFeedCascade(feed.ID))

	require.Equal(t, before, refThreadText(t, db, 1), "a thread without a reference to the feed must not be rewritten")
	require.Equal(t, "sql-null", refThreadType(t, db, 2), "an unrelated scalar row stays untouched")
}

// seedRefCategory creates a category and files the given feeds under it.
func seedRefCategory(t *testing.T, db *gorm.DB, name string, feedIDs ...uint) models.Category {
	t.Helper()
	category := models.Category{Name: name, Slug: name}
	require.NoError(t, db.Create(&category).Error)
	for _, feedID := range feedIDs {
		require.NoError(t, db.Model(&models.Feed{}).Where("id = ?", feedID).
			Update("category_id", category.ID).Error)
	}
	return category
}

// refThreadXMins records each thread's xmin — the transaction that last wrote the
// row. daily_report_threads carries no updated_at and a raw UPDATE is invisible
// to GORM callbacks, so xmin is the available write detector for the scenarios
// that must issue no write at all.
func refThreadXMins(t *testing.T, db *gorm.DB) map[uint]string {
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

// TestDeleteCategoryCascadePrunesArticleReferences covers the category path:
// deleting a category takes its feeds and their articles with it, so the
// references those articles carried are pruned first. A feed outside the
// category proves the maintenance stays scoped.
func TestDeleteCategoryCascadePrunesArticleReferences(t *testing.T) {
	db := testutil.SetupTestDB(t)
	firstFeed, firstArticles := seedRefFeed(t, db, "category-prune-first", 2)
	secondFeed, secondArticles := seedRefFeed(t, db, "category-prune-second", 1)
	outsideFeed, outsideArticles := seedRefFeed(t, db, "category-prune-outside", 1)
	category := seedRefCategory(t, db, "category-prune", firstFeed.ID, secondFeed.ID)

	seedRefThread(t, db, 1, fmt.Sprintf("[%d, %d, %d]", firstArticles[0].ID, outsideArticles[0].ID, firstArticles[1].ID))
	seedRefThread(t, db, 2, fmt.Sprintf("[%d]", secondArticles[0].ID))

	require.NoError(t, repository.NewReaderRepository(db).DeleteCategoryCascade(category.ID))

	var categoryCount int64
	require.NoError(t, db.Model(&models.Category{}).Where("id = ?", category.ID).Count(&categoryCount).Error)
	require.Zero(t, categoryCount, "the category row is deleted")
	var feedCount int64
	require.NoError(t, db.Model(&models.Feed{}).Where("category_id = ?", category.ID).Count(&feedCount).Error)
	require.Zero(t, feedCount, "its feeds are deleted")
	var articleCount int64
	require.NoError(t, db.Model(&models.Article{}).
		Where("feed_id IN ?", []uint{firstFeed.ID, secondFeed.ID}).Count(&articleCount).Error)
	require.Zero(t, articleCount, "its articles are deleted")
	var outsideCount int64
	require.NoError(t, db.Model(&models.Article{}).Where("feed_id = ?", outsideFeed.ID).Count(&outsideCount).Error)
	require.EqualValues(t, 1, outsideCount, "a feed outside the category keeps its articles")

	require.Equal(t, fmt.Sprintf("[%d]", outsideArticles[0].ID), refThreadText(t, db, 1),
		"references to the deleted articles are pruned and the survivor keeps its position")
	require.Equal(t, "array", refThreadType(t, db, 1))
	require.Equal(t, "[]", refThreadText(t, db, 2),
		"an array emptied by the deletion stays a readable array, not a JSON null")
}

// TestDeleteCategoryCascadeLeavesUnrelatedRowsAlone pins the zero-write case: a
// category whose articles nobody cites must not rewrite a single thread row, and
// an unrelated scalar row stays untouched.
func TestDeleteCategoryCascadeLeavesUnrelatedRowsAlone(t *testing.T) {
	db := testutil.SetupTestDB(t)
	feed, _ := seedRefFeed(t, db, "category-unrelated", 1)
	_, keep := seedRefFeed(t, db, "category-unrelated-keep", 2)
	category := seedRefCategory(t, db, "category-unrelated", feed.ID)

	seedRefThread(t, db, 1, fmt.Sprintf("[%d, %d]", keep[0].ID, keep[1].ID))
	seedRefThreadNullRefs(t, db, 2)
	before := refThreadText(t, db, 1)
	xmins := refThreadXMins(t, db)

	require.NoError(t, repository.NewReaderRepository(db).DeleteCategoryCascade(category.ID))

	require.Equal(t, before, refThreadText(t, db, 1), "a thread citing no deleted article must not be rewritten")
	require.Equal(t, xmins, refThreadXMins(t, db), "no citation means no UPDATE at all")
	require.Equal(t, "sql-null", refThreadType(t, db, 2),
		"normalizing unrelated null rows is the repair migration's job, not the delete path's")
}

// TestDeleteCategoryCascadeHandlesCategoryWithoutFeeds covers the empty branch:
// a category with no feeds is deleted without touching any thread row.
func TestDeleteCategoryCascadeHandlesCategoryWithoutFeeds(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_, keep := seedRefFeed(t, db, "category-empty-keep", 1)
	category := seedRefCategory(t, db, "category-empty")
	seedRefThread(t, db, 1, fmt.Sprintf("[%d]", keep[0].ID))
	xmins := refThreadXMins(t, db)

	require.NoError(t, repository.NewReaderRepository(db).DeleteCategoryCascade(category.ID))

	var categoryCount int64
	require.NoError(t, db.Model(&models.Category{}).Where("id = ?", category.ID).Count(&categoryCount).Error)
	require.Zero(t, categoryCount)
	require.Equal(t, xmins, refThreadXMins(t, db), "an empty category issues no thread write")
}

// TestDeleteFeedCascadeRemovesArticleDependents covers the environment-independent
// dependent cleanup: the live database cascades tag edges and queued jobs from the
// article through foreign keys, a freshly built one does not, so the delete removes
// them explicitly. The deleted feed's rows must vanish, another feed's rows must
// survive, and the tag left without edges stays for the maintenance job that owns
// orphan reclamation.
func TestDeleteFeedCascadeRemovesArticleDependents(t *testing.T) {
	db := testutil.SetupTestDB(t)
	tag := seedRefTopicTag(t, db, "dep-feed-tag")
	keepTag := seedRefTopicTag(t, db, "dep-feed-keep-tag")
	feed, articles := seedRefFeed(t, db, "dep-feed", 2)
	_, keep := seedRefFeed(t, db, "dep-feed-keep", 1)

	seedArticleDependents(t, db, tag.ID, articles[0].ID, articles[1].ID)
	seedArticleDependents(t, db, keepTag.ID, keep[0].ID)

	require.NoError(t, repository.NewReaderRepository(db).DeleteFeedCascade(feed.ID))

	edges, tagJobs, crawlJobs := countArticleDependents(t, db, []uint{articles[0].ID, articles[1].ID})
	require.Zero(t, edges, "the deleted articles' tag edges are gone")
	require.Zero(t, tagJobs, "their queued tag jobs are gone")
	require.Zero(t, crawlJobs, "their queued firecrawl jobs are gone")

	keepEdges, keepTagJobs, keepCrawlJobs := countArticleDependents(t, db, []uint{keep[0].ID})
	require.EqualValues(t, 1, keepEdges, "another feed's tag edges survive")
	require.EqualValues(t, 1, keepTagJobs, "another feed's queued tag jobs survive")
	require.EqualValues(t, 1, keepCrawlJobs, "another feed's queued firecrawl jobs survive")

	var tagCount int64
	require.NoError(t, db.Model(&models.TopicTag{}).Where("id = ?", tag.ID).Count(&tagCount).Error)
	require.EqualValues(t, 1, tagCount,
		"orphan reclamation belongs to the aux label cleanup job, not to this delete")
}

// TestDeleteCategoryCascadeRemovesArticleDependents is the two-level variant:
// deleting a category removes the dependent rows of every article filed under it
// and leaves an outside feed's rows untouched.
func TestDeleteCategoryCascadeRemovesArticleDependents(t *testing.T) {
	db := testutil.SetupTestDB(t)
	tag := seedRefTopicTag(t, db, "dep-category-tag")
	keepTag := seedRefTopicTag(t, db, "dep-category-keep-tag")
	firstFeed, firstArticles := seedRefFeed(t, db, "dep-category-first", 1)
	secondFeed, secondArticles := seedRefFeed(t, db, "dep-category-second", 2)
	_, outsideArticles := seedRefFeed(t, db, "dep-category-outside", 1)
	category := seedRefCategory(t, db, "dep-category", firstFeed.ID, secondFeed.ID)

	seedArticleDependents(t, db, tag.ID, firstArticles[0].ID, secondArticles[0].ID, secondArticles[1].ID)
	seedArticleDependents(t, db, keepTag.ID, outsideArticles[0].ID)

	require.NoError(t, repository.NewReaderRepository(db).DeleteCategoryCascade(category.ID))

	edges, tagJobs, crawlJobs := countArticleDependents(t, db,
		[]uint{firstArticles[0].ID, secondArticles[0].ID, secondArticles[1].ID})
	require.Zero(t, edges, "the deleted articles' tag edges are gone")
	require.Zero(t, tagJobs, "their queued tag jobs are gone")
	require.Zero(t, crawlJobs, "their queued firecrawl jobs are gone")

	keepEdges, keepTagJobs, keepCrawlJobs := countArticleDependents(t, db, []uint{outsideArticles[0].ID})
	require.EqualValues(t, 1, keepEdges, "a feed outside the category keeps its edges")
	require.EqualValues(t, 1, keepTagJobs, "a feed outside the category keeps its tag jobs")
	require.EqualValues(t, 1, keepCrawlJobs, "a feed outside the category keeps its firecrawl jobs")
}

// TestDeleteArticlesByFeedCoversEveryChunk pins the windowed deletion: a feed can
// hold more rows than one IN (?) list should carry, so the articles are removed in
// chunks and the boundary must not leave a remainder behind.
func TestDeleteArticlesByFeedCoversEveryChunk(t *testing.T) {
	db := testutil.SetupTestDB(t)
	feed := models.Feed{Title: "chunk-feed", URL: "https://example.com/chunk-feed"}
	require.NoError(t, db.Create(&feed).Error)
	const articleCount = 1200 // deliberately above the 1000-row delete window
	articles := make([]models.Article, 0, articleCount)
	for i := 0; i < articleCount; i++ {
		articles = append(articles, models.Article{
			FeedID: feed.ID,
			Title:  fmt.Sprintf("chunk-%d", i),
			Link:   fmt.Sprintf("https://example.com/chunk-feed/story-%d", i),
		})
	}
	require.NoError(t, db.CreateInBatches(articles, 500).Error)

	require.NoError(t, repository.NewReaderRepository(db).DeleteArticlesByFeed(feed.ID))

	var remaining int64
	require.NoError(t, db.Model(&models.Article{}).Where("feed_id = ?", feed.ID).Count(&remaining).Error)
	require.Zero(t, remaining, "every chunk is deleted, not just the first")
}

// TestDeleteFeedCascadeHandlesFeedWithoutArticles covers the empty branch: a feed
// with nothing filed under it deletes without touching the dependent tables.
func TestDeleteFeedCascadeHandlesFeedWithoutArticles(t *testing.T) {
	db := testutil.SetupTestDB(t)
	empty := models.Feed{Title: "dep-feed-empty", URL: "https://example.com/dep-feed-empty"}
	require.NoError(t, db.Create(&empty).Error)
	tag := seedRefTopicTag(t, db, "dep-feed-empty-tag")
	_, keep := seedRefFeed(t, db, "dep-feed-empty-keep", 1)
	seedArticleDependents(t, db, tag.ID, keep[0].ID)

	require.NoError(t, repository.NewReaderRepository(db).DeleteFeedCascade(empty.ID))

	// The feed row itself must be gone: without this the test would still pass if
	// DeleteFeedCascade silently became a no-op on the no-articles branch.
	var remaining int64
	require.NoError(t, db.Model(&models.Feed{}).Where("id = ?", empty.ID).Count(&remaining).Error)
	require.Zero(t, remaining, "the deleted feed row must not survive")

	edges, tagJobs, crawlJobs := countArticleDependents(t, db, []uint{keep[0].ID})
	require.EqualValues(t, 1, edges)
	require.EqualValues(t, 1, tagJobs)
	require.EqualValues(t, 1, crawlJobs)
}
