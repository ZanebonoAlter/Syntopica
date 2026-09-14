package service

import (
	"fmt"
	"testing"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	tagging "syntopica-backend/internal/tagmanagement"
)

// setupCleanupTestDB extends setupFeedsTestDB with the ReadingBehavior table
// (archive cleanup deletes behavior rows), a search_vector column (sqlite has
// none by default — the archive UPDATE nulls it on Postgres), and the
// tagmanagement repository global. Archiving no longer removes tag edges or
// runs CleanupOrphanedTags (orphan cleanup moved to the time-window EdgeGC),
// but the global stays initialized for tagging-backed code paths.
func setupCleanupTestDB(t *testing.T) {
	t.Helper()
	setupFeedsTestDB(t)
	tagging.InitRepository(database.DB)
	if err := database.DB.AutoMigrate(&models.ReadingBehavior{}); err != nil {
		t.Fatalf("migrate reading behaviors: %v", err)
	}
	if err := database.DB.Exec(`ALTER TABLE articles ADD COLUMN search_vector TEXT`).Error; err != nil {
		t.Fatalf("add search_vector stub column: %v", err)
	}
}

func createCleanupFeed(t *testing.T, maxArticles int) models.Feed {
	t.Helper()
	feed := models.Feed{
		Title:       fmt.Sprintf("Cleanup Feed %s %d", t.Name(), maxArticles),
		URL:         fmt.Sprintf("https://example.com/%s/%d", t.Name(), maxArticles),
		MaxArticles: maxArticles,
	}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}
	return feed
}

func TestCleanupOldArticlesArchivesInsteadOfDelete(t *testing.T) {
	setupCleanupTestDB(t)
	service := NewFeedService()
	feed := createCleanupFeed(t, 2)

	now := time.Now()
	articles := []models.Article{
		{FeedID: feed.ID, Title: "new", Link: "https://example.com/new", Content: "body new", FirecrawlContent: "fc new", PubDate: ptrTime(now.Add(-1 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"},
		{FeedID: feed.ID, Title: "middle", Link: "https://example.com/middle", Content: "body middle", PubDate: ptrTime(now.Add(-2 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"},
		{FeedID: feed.ID, Title: "old", Link: "https://example.com/old", Content: "body old", FirecrawlContent: "fc old", PubDate: ptrTime(now.Add(-3 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"},
	}
	if err := database.DB.Create(&articles).Error; err != nil {
		t.Fatalf("create articles: %v", err)
	}

	service.CleanupOldArticles(&feed)

	// All three rows must still exist — no physical delete.
	var count int64
	if err := database.DB.Model(&models.Article{}).Where("feed_id = ?", feed.ID).Count(&count).Error; err != nil {
		t.Fatalf("count articles: %v", err)
	}
	if count != 3 {
		t.Fatalf("article rows = %d, want 3 (archive must not delete)", count)
	}

	// The oldest is archived; the two newest stay active.
	var archived models.Article
	if err := database.DB.Where("feed_id = ? AND title = ?", feed.ID, "old").First(&archived).Error; err != nil {
		t.Fatalf("load archived article: %v", err)
	}
	if !archived.Archived {
		t.Fatalf("oldest article archived = false, want true")
	}
	// Text fields fully preserved.
	if archived.Content != "body old" || archived.FirecrawlContent != "fc old" || archived.Link != "https://example.com/old" {
		t.Fatalf("archived article text fields mutated: %#v", archived)
	}

	var activeCount int64
	if err := database.DB.Model(&models.Article{}).Where("feed_id = ? AND archived = ?", feed.ID, false).Count(&activeCount).Error; err != nil {
		t.Fatalf("count active: %v", err)
	}
	if activeCount != 2 {
		t.Fatalf("active articles = %d, want 2", activeCount)
	}
}

func TestCleanupOldArticlesFavoriteImmune(t *testing.T) {
	setupCleanupTestDB(t)
	service := NewFeedService()
	feed := createCleanupFeed(t, 2)

	now := time.Now()
	articles := []models.Article{
		{FeedID: feed.ID, Title: "new", Link: "https://example.com/new", PubDate: ptrTime(now.Add(-1 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"},
		{FeedID: feed.ID, Title: "fav old", Link: "https://example.com/fav", Favorite: true, PubDate: ptrTime(now.Add(-2 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"},
		{FeedID: feed.ID, Title: "plain old", Link: "https://example.com/plain", PubDate: ptrTime(now.Add(-3 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"},
	}
	if err := database.DB.Create(&articles).Error; err != nil {
		t.Fatalf("create articles: %v", err)
	}

	service.CleanupOldArticles(&feed)

	var fav models.Article
	if err := database.DB.Where("title = ?", "fav old").First(&fav).Error; err != nil {
		t.Fatalf("load favorite: %v", err)
	}
	if fav.Archived {
		t.Fatalf("favorite article was archived")
	}
	var plain models.Article
	if err := database.DB.Where("title = ?", "plain old").First(&plain).Error; err != nil {
		t.Fatalf("load plain: %v", err)
	}
	if !plain.Archived {
		t.Fatalf("plain article should be archived instead of favorite")
	}
}

func TestCleanupOldArticlesUnlimitedSkips(t *testing.T) {
	setupCleanupTestDB(t)
	service := NewFeedService()

	for _, max := range []int{0, 9999} {
		feed := createCleanupFeed(t, max)
		now := time.Now()
		articles := []models.Article{
			{FeedID: feed.ID, Title: "a1", Link: "https://example.com/1", PubDate: ptrTime(now.Add(-1 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"},
			{FeedID: feed.ID, Title: "a2", Link: "https://example.com/2", PubDate: ptrTime(now.Add(-2 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"},
			{FeedID: feed.ID, Title: "a3", Link: "https://example.com/3", PubDate: ptrTime(now.Add(-3 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"},
		}
		if err := database.DB.Create(&articles).Error; err != nil {
			t.Fatalf("create articles: %v", err)
		}

		service.CleanupOldArticles(&feed)

		var archivedCount int64
		if err := database.DB.Model(&models.Article{}).Where("feed_id = ? AND archived = ?", feed.ID, true).Count(&archivedCount).Error; err != nil {
			t.Fatalf("count archived: %v", err)
		}
		if archivedCount != 0 {
			t.Fatalf("max=%d: archived %d articles, want 0", max, archivedCount)
		}
	}
}

func TestCleanupOldArticlesIdempotentAndWindowNotEroded(t *testing.T) {
	setupCleanupTestDB(t)
	service := NewFeedService()
	feed := createCleanupFeed(t, 2)

	now := time.Now()
	articles := []models.Article{
		{FeedID: feed.ID, Title: "new", Link: "https://example.com/new", PubDate: ptrTime(now.Add(-1 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"},
		{FeedID: feed.ID, Title: "middle", Link: "https://example.com/middle", PubDate: ptrTime(now.Add(-2 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"},
	}
	if err := database.DB.Create(&articles).Error; err != nil {
		t.Fatalf("create articles: %v", err)
	}
	// Pre-existing archived rows: 5 (e.g. archived by earlier runs).
	for i := 3; i <= 7; i++ {
		a := models.Article{
			FeedID: feed.ID, Title: fmt.Sprintf("archived-%d", i), Link: fmt.Sprintf("https://example.com/%d", i),
			Archived: true, PubDate: ptrTime(now.Add(time.Duration(i) * time.Hour * -24)),
			SummaryStatus: "complete", FirecrawlStatus: "completed",
		}
		if err := database.DB.Create(&a).Error; err != nil {
			t.Fatalf("create archived article: %v", err)
		}
	}

	// First run: 2 active == MaxArticles, nothing to archive.
	service.CleanupOldArticles(&feed)

	var activeCount int64
	if err := database.DB.Model(&models.Article{}).Where("feed_id = ? AND archived = ?", feed.ID, false).Count(&activeCount).Error; err != nil {
		t.Fatalf("count active: %v", err)
	}
	if activeCount != 2 {
		t.Fatalf("active articles = %d, want 2 (archived rows must not erode the window)", activeCount)
	}

	// Add one more active article and run again: exactly 1 archived, the oldest.
	extra := models.Article{FeedID: feed.ID, Title: "newest", Link: "https://example.com/newest", PubDate: ptrTime(now), SummaryStatus: "complete", FirecrawlStatus: "completed"}
	if err := database.DB.Create(&extra).Error; err != nil {
		t.Fatalf("create extra: %v", err)
	}
	service.CleanupOldArticles(&feed)

	var middle models.Article
	if err := database.DB.Where("title = ?", "middle").First(&middle).Error; err != nil {
		t.Fatalf("load middle: %v", err)
	}
	if !middle.Archived {
		t.Fatalf("middle should be archived on second run")
	}
	var newest models.Article
	if err := database.DB.Where("title = ?", "newest").First(&newest).Error; err != nil {
		t.Fatalf("load newest: %v", err)
	}
	if newest.Archived {
		t.Fatalf("newest must stay active")
	}

	// Third run changes nothing (idempotent).
	service.CleanupOldArticles(&feed)
	if err := database.DB.Model(&models.Article{}).Where("feed_id = ? AND archived = ?", feed.ID, true).Count(&activeCount).Error; err != nil {
		t.Fatalf("count archived after third run: %v", err)
	}
	if activeCount != 6 { // 5 pre-existing + middle
		t.Fatalf("archived count after third run = %d, want 6 (idempotent)", activeCount)
	}
}

func TestCleanupOldArticlesClearsDerivedData(t *testing.T) {
	setupCleanupTestDB(t)
	service := NewFeedService()
	feed := createCleanupFeed(t, 1)

	now := time.Now()
	keep := models.Article{FeedID: feed.ID, Title: "keep", Link: "https://example.com/keep", PubDate: ptrTime(now), SummaryStatus: "complete", FirecrawlStatus: "completed"}
	victim := models.Article{FeedID: feed.ID, Title: "victim", Link: "https://example.com/victim", PubDate: ptrTime(now.Add(-2 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"}
	if err := database.DB.Create(&keep).Error; err != nil {
		t.Fatalf("create keep: %v", err)
	}
	if err := database.DB.Create(&victim).Error; err != nil {
		t.Fatalf("create victim: %v", err)
	}

	// One tag referenced only by the victim and one also referenced by the keep
	// article. Archiving keeps both edges and both tags — orphan cleanup is the
	// time-window EdgeGC's job now, not the archiving path's.
	orphanTag := models.TopicTag{Label: "orphan-tag", Status: "active"}
	sharedTag := models.TopicTag{Label: "shared-tag", Status: "active"}
	if err := database.DB.Create(&orphanTag).Error; err != nil {
		t.Fatalf("create orphan tag: %v", err)
	}
	if err := database.DB.Create(&sharedTag).Error; err != nil {
		t.Fatalf("create shared tag: %v", err)
	}
	edges := []models.ArticleTopicTag{
		{ArticleID: victim.ID, TopicTagID: orphanTag.ID, Source: "llm"},
		{ArticleID: victim.ID, TopicTagID: sharedTag.ID, Source: "llm"},
		{ArticleID: keep.ID, TopicTagID: sharedTag.ID, Source: "llm"},
	}
	if err := database.DB.Create(&edges).Error; err != nil {
		t.Fatalf("create edges: %v", err)
	}

	behaviors := []models.ReadingBehavior{
		{ArticleID: victim.ID, FeedID: feed.ID, EventType: "read"},
		{ArticleID: keep.ID, FeedID: feed.ID, EventType: "read"},
	}
	if err := database.DB.Create(&behaviors).Error; err != nil {
		t.Fatalf("create behaviors: %v", err)
	}

	// Non-NULL search_vector stubs on both rows.
	if err := database.DB.Exec(`UPDATE articles SET search_vector = 'stub'`).Error; err != nil {
		t.Fatalf("stub search_vector: %v", err)
	}

	service.CleanupOldArticles(&feed)

	// Victim archived, behaviors gone, search_vector nulled — but its tag edges
	// survive (late tagging tasks may even have added more).
	var victimEdges int64
	if err := database.DB.Model(&models.ArticleTopicTag{}).Where("article_id = ?", victim.ID).Count(&victimEdges).Error; err != nil {
		t.Fatalf("count victim edges: %v", err)
	}
	if victimEdges != 2 {
		t.Fatalf("victim edges = %d, want 2 (archiving must not delete tag edges)", victimEdges)
	}
	var victimBehaviors int64
	if err := database.DB.Model(&models.ReadingBehavior{}).Where("article_id = ?", victim.ID).Count(&victimBehaviors).Error; err != nil {
		t.Fatalf("count victim behaviors: %v", err)
	}
	if victimBehaviors != 0 {
		t.Fatalf("victim behaviors = %d, want 0", victimBehaviors)
	}

	type svRow struct {
		Title        string
		SearchVector *string
	}
	var rows []svRow
	if err := database.DB.Raw(`SELECT title, search_vector FROM articles WHERE feed_id = ?`, feed.ID).Scan(&rows).Error; err != nil {
		t.Fatalf("scan search_vector: %v", err)
	}
	for _, r := range rows {
		if r.Title == "victim" && r.SearchVector != nil {
			t.Fatalf("victim search_vector = %v, want NULL", *r.SearchVector)
		}
		if r.Title == "keep" && r.SearchVector == nil {
			t.Fatalf("keep search_vector must stay untouched")
		}
	}

	// Keep article's data intact.
	var keepEdges int64
	if err := database.DB.Model(&models.ArticleTopicTag{}).Where("article_id = ?", keep.ID).Count(&keepEdges).Error; err != nil {
		t.Fatalf("count keep edges: %v", err)
	}
	if keepEdges != 1 {
		t.Fatalf("keep edges = %d, want 1", keepEdges)
	}
	var keepBehaviors int64
	if err := database.DB.Model(&models.ReadingBehavior{}).Where("article_id = ?", keep.ID).Count(&keepBehaviors).Error; err != nil {
		t.Fatalf("count keep behaviors: %v", err)
	}
	if keepBehaviors != 1 {
		t.Fatalf("keep behaviors = %d, want 1", keepBehaviors)
	}

	// The victim-only tag survives: it still has an edge left, so it is not an
	// orphan, and only the time-window GC would ever reclaim it.
	var orphanCount int64
	if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", orphanTag.ID).Count(&orphanCount).Error; err != nil {
		t.Fatalf("count victim-only tag: %v", err)
	}
	if orphanCount != 1 {
		t.Fatalf("victim-only tag count = %d, want 1 (archiving must not clean orphans)", orphanCount)
	}
	var sharedCount int64
	if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", sharedTag.ID).Count(&sharedCount).Error; err != nil {
		t.Fatalf("count shared tag: %v", err)
	}
	if sharedCount != 1 {
		t.Fatalf("shared tag must survive cleanup")
	}
}
