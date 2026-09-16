package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/reader/repository"
	tagRepo "syntopica-backend/internal/tagmanagement/repository"
)

func setupFeedsTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	database.DB = db
	repository.InitRepository(database.DB)
	// tagmanagement shares the same sqlite DB: refreshExistingArticle's orphan
	// cleanup goes through the tagmanagement repository global.
	tagRepo.InitRepository(database.DB)
	if err := database.DB.AutoMigrate(&models.Feed{}, &models.Article{}, &models.TopicTag{}, &models.ArticleTopicTag{}, &models.FirecrawlJob{}, &models.TagJob{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
}

func TestCleanupOldArticlesDoesNotPreservePendingOrIncompleteArticles(t *testing.T) {
	setupFeedsTestDB(t)

	service := NewFeedService()
	feed := models.Feed{
		Title:                 "Feed",
		URL:                   fmt.Sprintf("https://example.com/%s", t.Name()),
		MaxArticles:           2,
		FirecrawlEnabled:      true,
		ArticleSummaryEnabled: true,
	}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	now := time.Now()
	articles := []models.Article{
		{FeedID: feed.ID, Title: "new complete", Link: "https://example.com/new", PubDate: ptrTime(now.Add(-1 * time.Hour)), SummaryStatus: "complete", FirecrawlStatus: "completed"},
		{FeedID: feed.ID, Title: "middle pending", Link: "https://example.com/middle", PubDate: ptrTime(now.Add(-2 * time.Hour)), SummaryStatus: "pending", FirecrawlStatus: "pending"},
		{FeedID: feed.ID, Title: "old incomplete", Link: "https://example.com/old", PubDate: ptrTime(now.Add(-3 * time.Hour)), SummaryStatus: "incomplete", FirecrawlStatus: "completed", FirecrawlContent: "ready"},
	}
	if err := database.DB.Create(&articles).Error; err != nil {
		t.Fatalf("create articles: %v", err)
	}

	service.CleanupOldArticles(&feed)

	var remaining []models.Article
	if err := database.DB.Where("feed_id = ?", feed.ID).Order("pub_date DESC").Find(&remaining).Error; err != nil {
		t.Fatalf("load remaining articles: %v", err)
	}
	if len(remaining) != 3 {
		t.Fatalf("article rows = %d, want 3 (archive must not delete)", len(remaining))
	}

	for _, article := range remaining {
		wantArchived := article.Title == "old incomplete"
		if article.Archived != wantArchived {
			t.Fatalf("article %q archived = %v, want %v", article.Title, article.Archived, wantArchived)
		}
	}
}

func TestRefreshFeedEnqueuesTagJobWhenCompletionDisabled(t *testing.T) {
	setupFeedsTestDB(t)

	var rssServer *httptest.Server
	rssServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>OpenAI Feed</title>
    <description>Feed for tests</description>
    <link>%s</link>
    <item>
      <title>OpenAI launches new AI agent runtime</title>
      <link>%s/openai-agent</link>
      <description>OpenAI agentic workflow update</description>
      <pubDate>Sun, 22 Mar 2026 09:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`, rssServer.URL, rssServer.URL)
	}))
	defer rssServer.Close()

	feed := models.Feed{
		Title:                 "Seed Feed",
		URL:                   rssServer.URL,
		MaxArticles:           10,
		FirecrawlEnabled:      false,
		ArticleSummaryEnabled: false,
	}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	service := NewFeedService()
	service.iconStore = NewIconStore(t.TempDir())
	if err := service.RefreshFeed(context.Background(), feed.ID); err != nil {
		t.Fatalf("refresh feed: %v", err)
	}

	// Icon candidates all fail against the local test server (no RSS image, no
	// HTML icon link, /favicon.ico returns non-image content): the feed must
	// stay fallback while the refresh itself succeeds.
	var refreshed models.Feed
	if err := database.DB.First(&refreshed, feed.ID).Error; err != nil {
		t.Fatalf("reload feed: %v", err)
	}
	if refreshed.IconSource != "fallback" || refreshed.Icon != "mdi:rss" {
		t.Errorf("after failed icon candidates: icon_source=%q icon=%q, want fallback mdi:rss", refreshed.IconSource, refreshed.Icon)
	}

	var article models.Article
	if err := database.DB.First(&article).Error; err != nil {
		t.Fatalf("load article: %v", err)
	}

	var jobCount int64
	if err := database.DB.Model(&models.TagJob{}).Where("article_id = ?", article.ID).Count(&jobCount).Error; err != nil {
		t.Fatalf("count tag jobs: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("tag job count = %d, want 1", jobCount)
	}
}

func TestRefreshFeedEnqueuesFirecrawlJobWhenEnabled(t *testing.T) {
	setupFeedsTestDB(t)

	var rssServer *httptest.Server
	rssServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Queued Feed</title>
    <description>Feed for tests</description>
    <link>%s</link>
    <item>
      <title>Queued article</title>
      <link>%s/queued</link>
      <description>queued desc</description>
      <pubDate>Sun, 22 Mar 2026 09:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`, rssServer.URL, rssServer.URL)
	}))
	defer rssServer.Close()

	feed := models.Feed{
		Title:            "Queued Feed",
		URL:              rssServer.URL,
		MaxArticles:      10,
		FirecrawlEnabled: true,
	}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	service := NewFeedService()
	service.iconStore = NewIconStore(t.TempDir())
	if err := service.RefreshFeed(context.Background(), feed.ID); err != nil {
		t.Fatalf("refresh feed: %v", err)
	}

	var article models.Article
	if err := database.DB.First(&article).Error; err != nil {
		t.Fatalf("load article: %v", err)
	}

	var count int64
	if err := database.DB.Model(&models.FirecrawlJob{}).Where("article_id = ?", article.ID).Count(&count).Error; err != nil {
		t.Fatalf("count firecrawl jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("firecrawl job count = %d, want 1", count)
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func TestEnqueueArticleProcessingTaggingMatrix(t *testing.T) {
	setupFeedsTestDB(t)

	var rssServer *httptest.Server
	rssServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <description>desc</description>
    <link>%s</link>
    <item>
      <title>Test Article</title>
      <link>%s/test</link>
      <description>desc</description>
      <pubDate>Sun, 22 Mar 2026 09:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`, rssServer.URL, rssServer.URL)
	}))
	defer rssServer.Close()

	tests := []struct {
		name              string
		firecrawlEnabled  bool
		taggingEnabled    bool
		wantFirecrawlJobs int64
		wantTagJobs       int64
	}{
		{
			name:              "firecrawl off + tagging on: tag job only",
			firecrawlEnabled:  false,
			taggingEnabled:    true,
			wantFirecrawlJobs: 0,
			wantTagJobs:       1,
		},
		{
			name:              "firecrawl off + tagging off: no jobs",
			firecrawlEnabled:  false,
			taggingEnabled:    false,
			wantFirecrawlJobs: 0,
			wantTagJobs:       0,
		},
		{
			name:              "firecrawl on + tagging on: firecrawl job only (tag comes later via callback)",
			firecrawlEnabled:  true,
			taggingEnabled:    true,
			wantFirecrawlJobs: 1,
			wantTagJobs:       0,
		},
		{
			name:              "firecrawl on + tagging off: firecrawl job only",
			firecrawlEnabled:  true,
			taggingEnabled:    false,
			wantFirecrawlJobs: 1,
			wantTagJobs:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupFeedsTestDB(t)

			feed := models.Feed{
				Title:            tt.name,
				URL:              rssServer.URL,
				MaxArticles:      10,
				FirecrawlEnabled: tt.firecrawlEnabled,
				TaggingEnabled:   tt.taggingEnabled,
			}
			if err := database.DB.Create(&feed).Error; err != nil {
				t.Fatalf("create feed: %v", err)
			}
			database.DB.Model(&feed).Update("tagging_enabled", tt.taggingEnabled)

			service := NewFeedService()
			service.iconStore = NewIconStore(t.TempDir())
			if err := service.RefreshFeed(context.Background(), feed.ID); err != nil {
				t.Fatalf("refresh feed: %v", err)
			}

			var firecrawlCount int64
			database.DB.Model(&models.FirecrawlJob{}).Count(&firecrawlCount)
			if firecrawlCount != tt.wantFirecrawlJobs {
				t.Errorf("firecrawl jobs = %d, want %d", firecrawlCount, tt.wantFirecrawlJobs)
			}

			var tagCount int64
			database.DB.Model(&models.TagJob{}).Count(&tagCount)
			if tagCount != tt.wantTagJobs {
				t.Errorf("tag jobs = %d, want %d", tagCount, tt.wantTagJobs)
			}
		})
	}
}

func TestCleanupOldArticlesUnlimited(t *testing.T) {
	tests := []struct {
		name        string
		maxArticles int
		wantArchive bool
	}{
		{
			name:        "max_articles=0 means unlimited, no archiving",
			maxArticles: 0,
			wantArchive: false,
		},
		{
			name:        "max_articles=9999 means unlimited, no archiving",
			maxArticles: 9999,
			wantArchive: false,
		},
		{
			name:        "max_articles=2 triggers cleanup",
			maxArticles: 2,
			wantArchive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupFeedsTestDB(t)

			feed := models.Feed{
				Title:       tt.name,
				URL:         fmt.Sprintf("https://example.com/%s", t.Name()),
				MaxArticles: 100,
			}
			if err := database.DB.Create(&feed).Error; err != nil {
				t.Fatalf("create feed: %v", err)
			}
			if tt.maxArticles != 100 {
				database.DB.Model(&feed).Update("max_articles", tt.maxArticles)
				feed.MaxArticles = tt.maxArticles
			}

			now := time.Now()
			for i := 0; i < 5; i++ {
				article := models.Article{
					FeedID:  feed.ID,
					Title:   fmt.Sprintf("Article %d", i),
					Link:    fmt.Sprintf("https://example.com/%s/%d", t.Name(), i),
					PubDate: ptrTime(now.Add(-time.Duration(i) * time.Hour)),
				}
				if err := database.DB.Create(&article).Error; err != nil {
					t.Fatalf("create article: %v", err)
				}
			}

			service := NewFeedService()
			service.CleanupOldArticles(&feed)

			var remaining int64
			database.DB.Model(&models.Article{}).Where("feed_id = ?", feed.ID).Count(&remaining)
			var active int64
			database.DB.Model(&models.Article{}).Where("feed_id = ? AND archived = ?", feed.ID, false).Count(&active)

			if remaining != 5 {
				t.Errorf("article rows = %d, expected 5 (archive must not delete)", remaining)
			}
			if tt.wantArchive {
				if active > int64(tt.maxArticles) {
					t.Errorf("active articles = %d, expected <= %d", active, tt.maxArticles)
				}
			} else {
				if active != 5 {
					t.Errorf("active articles = %d, expected 5 (no archiving)", active)
				}
			}
		})
	}
}

// rssItemBody renders a one-item RSS payload (dedupe-rss-articles tests).
func rssItemBody(channelTitle, itemTitle, itemLink, itemDesc string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>%s</title>
    <description>Feed for tests</description>
    <link>https://example.com</link>
    <item>
      <title>%s</title>
      <link>%s</link>
      <description>%s</description>
      <pubDate>Sun, 22 Mar 2026 09:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`, channelTitle, itemTitle, itemLink, itemDesc)
}

// startSwitchableRSSServer serves the RSS payload currently referenced by
// *body, letting a test flip the feed content between refreshes (live-news
// rolling-update scenarios).
func startSwitchableRSSServer(t *testing.T, body *string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(*body))
	}))
	t.Cleanup(server.Close)
	return server
}

func newDedupeTestService(t *testing.T) *FeedService {
	t.Helper()
	service := NewFeedService()
	service.iconStore = NewIconStore(t.TempDir())
	return service
}

// TestRefreshFeedDedupesByLinkWithinFeed covers spec scenario "快讯同 link 改标题
// 不产生新文章": a live-news feed rolling the title under one URL must not
// produce a second article row — the stored row is refreshed instead.
func TestRefreshFeedDedupesByLinkWithinFeed(t *testing.T) {
	setupFeedsTestDB(t)

	body := rssItemBody("Live Feed", "headline v1", "https://example.com/live/1", "desc v1")
	server := startSwitchableRSSServer(t, &body)

	feed := models.Feed{Title: "Live Feed", URL: server.URL, MaxArticles: 10, TaggingEnabled: true}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	service := newDedupeTestService(t)
	if err := service.RefreshFeed(context.Background(), feed.ID); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	// Live-news roll: same link, new headline + description.
	body = rssItemBody("Live Feed", "headline v2 (rolling update)", "https://example.com/live/1", "desc v2")
	if err := service.RefreshFeed(context.Background(), feed.ID); err != nil {
		t.Fatalf("second refresh: %v", err)
	}

	var articles []models.Article
	if err := database.DB.Where("feed_id = ?", feed.ID).Find(&articles).Error; err != nil {
		t.Fatalf("load articles: %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("article rows = %d, want 1 (same-link entry must not re-insert)", len(articles))
	}
	if articles[0].Title != "headline v2 (rolling update)" {
		t.Fatalf("stored title = %q, want refreshed %q", articles[0].Title, "headline v2 (rolling update)")
	}
	if articles[0].Description != "desc v2" {
		t.Fatalf("stored description = %q, want %q", articles[0].Description, "desc v2")
	}
}

// TestRefreshFeedKeepsCrossFeedCopies covers spec scenario "跨 feed 同 link 各自
// 保留": the same link syndicated by two feeds keeps one row per feed.
func TestRefreshFeedKeepsCrossFeedCopies(t *testing.T) {
	setupFeedsTestDB(t)

	body := rssItemBody("Shared", "same story", "https://example.com/shared/1", "shared desc")
	serverA := startSwitchableRSSServer(t, &body)
	serverB := startSwitchableRSSServer(t, &body)

	feedA := models.Feed{Title: "Feed A", URL: serverA.URL, MaxArticles: 10, TaggingEnabled: true}
	feedB := models.Feed{Title: "Feed B", URL: serverB.URL, MaxArticles: 10, TaggingEnabled: true}
	if err := database.DB.Create(&feedA).Error; err != nil {
		t.Fatalf("create feed A: %v", err)
	}
	if err := database.DB.Create(&feedB).Error; err != nil {
		t.Fatalf("create feed B: %v", err)
	}

	service := newDedupeTestService(t)
	if err := service.RefreshFeed(context.Background(), feedA.ID); err != nil {
		t.Fatalf("refresh feed A: %v", err)
	}
	if err := service.RefreshFeed(context.Background(), feedB.ID); err != nil {
		t.Fatalf("refresh feed B: %v", err)
	}

	var countA, countB int64
	database.DB.Model(&models.Article{}).Where("feed_id = ? AND link = ?", feedA.ID, "https://example.com/shared/1").Count(&countA)
	database.DB.Model(&models.Article{}).Where("feed_id = ? AND link = ?", feedB.ID, "https://example.com/shared/1").Count(&countB)
	if countA != 1 || countB != 1 {
		t.Fatalf("copies: feed A = %d, feed B = %d, want 1 and 1 (cross-feed copies preserved)", countA, countB)
	}
}

// TestRefreshFeedSkipsUnchangedEntry covers spec scenario "内容未实质变化时跳过":
// an entry whose title+description are both unchanged triggers no update and
// no extra processing-chain enqueue.
func TestRefreshFeedSkipsUnchangedEntry(t *testing.T) {
	setupFeedsTestDB(t)

	body := rssItemBody("Stable", "stable headline", "https://example.com/stable/1", "stable desc")
	server := startSwitchableRSSServer(t, &body)

	feed := models.Feed{Title: "Stable", URL: server.URL, MaxArticles: 10, FirecrawlEnabled: true}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	service := newDedupeTestService(t)
	if err := service.RefreshFeed(context.Background(), feed.ID); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	var article models.Article
	if err := database.DB.First(&article).Where("link = ?", "https://example.com/stable/1").Error; err != nil {
		t.Fatalf("load article: %v", err)
	}
	// Mark the row as if the chain had fully run.
	markProcessed := func() {
		if err := database.DB.Model(&models.Article{}).Where("id = ?", article.ID).Updates(map[string]interface{}{
			"firecrawl_status": "completed",
			"firecrawl_content": "<p>crawled body</p>",
		}).Error; err != nil {
			t.Fatalf("mark processed: %v", err)
		}
	}
	markProcessed()

	if err := service.RefreshFeed(context.Background(), feed.ID); err != nil {
		t.Fatalf("second refresh: %v", err)
	}

	var reloaded models.Article
	if err := database.DB.First(&reloaded, article.ID).Error; err != nil {
		t.Fatalf("reload article: %v", err)
	}
	if reloaded.FirecrawlContent != "<p>crawled body</p>" || reloaded.FirecrawlStatus != "completed" {
		t.Fatalf("unchanged entry must not reset chain state: content=%q status=%q", reloaded.FirecrawlContent, reloaded.FirecrawlStatus)
	}

	var jobCount int64
	database.DB.Model(&models.FirecrawlJob{}).Where("article_id = ?", article.ID).Count(&jobCount)
	if jobCount != 1 {
		t.Fatalf("firecrawl jobs = %d, want 1 (unchanged entry must not re-enqueue)", jobCount)
	}
}

// TestRefreshFeedUpdatesChangedEntryAndClearsDerivedState covers spec scenario
// "内容实质变化时更新并重走处理链": content fields update, chain state resets
// like a fresh insert, derived fields clear, stale tags drop, processing
// re-enqueues.
func TestRefreshFeedUpdatesChangedEntryAndClearsDerivedState(t *testing.T) {
	setupFeedsTestDB(t)

	body := rssItemBody("Rolling", "original headline", "https://example.com/rolling/1", "original desc")
	server := startSwitchableRSSServer(t, &body)

	feed := models.Feed{Title: "Rolling", URL: server.URL, MaxArticles: 10, FirecrawlEnabled: true, ArticleSummaryEnabled: true}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	service := newDedupeTestService(t)
	if err := service.RefreshFeed(context.Background(), feed.ID); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	var article models.Article
	if err := database.DB.First(&article).Where("link = ?", "https://example.com/rolling/1").Error; err != nil {
		t.Fatalf("load article: %v", err)
	}

	// Simulate a fully processed article: crawl body, AI summary, stale tags.
	tag := models.TopicTag{Label: "旧话题", Slug: "stale-topic", Category: "keyword", Status: "active"}
	if err := database.DB.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := database.DB.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID, Score: 0.9, Source: "llm"}).Error; err != nil {
		t.Fatalf("create tag link: %v", err)
	}
	if err := database.DB.Model(&models.Article{}).Where("id = ?", article.ID).Updates(map[string]interface{}{
		"firecrawl_status":    "completed",
		"firecrawl_content":   "<p>old crawled body</p>",
		"summary_status":      "complete",
		"ai_content_summary":  "旧摘要",
		"completion_attempts": 3,
		"completion_error":    "old error",
		"content_form":        "mono",
	}).Error; err != nil {
		t.Fatalf("simulate processed state: %v", err)
	}
	// tag_count is read-only on the model (`gorm:"->"`), so seed it directly:
	// the refresh that drops the tag below must also drop this counter.
	if err := database.DB.Exec("UPDATE articles SET tag_count = 1 WHERE id = ?", article.ID).Error; err != nil {
		t.Fatalf("seed tag_count: %v", err)
	}

	body = rssItemBody("Rolling", "rolled headline", "https://example.com/rolling/1", "rolled desc")
	// First-round crawl job is done (worker consumed it): the refresh must
	// enqueue a fresh one (the queue itself dedupes while a job is still
	// pending, which is also fine — here we assert the completed case).
	if err := database.DB.Model(&models.FirecrawlJob{}).
		Where("article_id = ?", article.ID).
		Update("status", string(models.JobStatusCompleted)).Error; err != nil {
		t.Fatalf("complete first job: %v", err)
	}
	if err := service.RefreshFeed(context.Background(), feed.ID); err != nil {
		t.Fatalf("second refresh: %v", err)
	}

	var reloaded models.Article
	if err := database.DB.First(&reloaded, article.ID).Error; err != nil {
		t.Fatalf("reload article: %v", err)
	}
	if reloaded.Title != "rolled headline" || reloaded.Description != "rolled desc" {
		t.Fatalf("content not refreshed: title=%q desc=%q", reloaded.Title, reloaded.Description)
	}
	if reloaded.FirecrawlStatus != "pending" {
		t.Fatalf("firecrawl_status = %q, want pending (reset like fresh insert)", reloaded.FirecrawlStatus)
	}
	if reloaded.SummaryStatus != "incomplete" {
		t.Fatalf("summary_status = %q, want incomplete (reset like fresh insert)", reloaded.SummaryStatus)
	}
	if reloaded.FirecrawlContent != "" || reloaded.AIContentSummary != "" {
		t.Fatalf("derived fields must clear: crawl=%q summary=%q", reloaded.FirecrawlContent, reloaded.AIContentSummary)
	}
	if reloaded.CompletionAttempts != 0 || reloaded.CompletionError != "" || reloaded.ContentForm != "" {
		t.Fatalf("completion bookkeeping must clear: attempts=%d err=%q form=%q", reloaded.CompletionAttempts, reloaded.CompletionError, reloaded.ContentForm)
	}

	var tagLinks int64
	database.DB.Model(&models.ArticleTopicTag{}).Where("article_id = ?", article.ID).Count(&tagLinks)
	if tagLinks != 0 {
		t.Fatalf("stale tag links = %d, want 0", tagLinks)
	}
	var orphanCount int64
	database.DB.Model(&models.TopicTag{}).Where("id = ?", tag.ID).Count(&orphanCount)
	if orphanCount != 0 {
		t.Fatalf("orphaned topic tag must be cleaned up")
	}

	// The denormalised counter follows the dropped edges (same contract the
	// merge migration and the cross-feed reuse path maintain).
	var tagCount int64
	database.DB.Model(&models.Article{}).Where("id = ?", article.ID).Pluck("tag_count", &tagCount)
	if tagCount != 0 {
		t.Fatalf("articles.tag_count = %d after dropping stale tags, want 0", tagCount)
	}

	var jobCount int64
	database.DB.Model(&models.FirecrawlJob{}).Where("article_id = ? AND status = ?", article.ID, models.JobStatusPending).Count(&jobCount)
	if jobCount != 1 {
		t.Fatalf("pending firecrawl jobs = %d, want 1 (refresh must re-enqueue processing)", jobCount)
	}
}

// TestRefreshExistingArticleStatusMatrix drives refreshExistingArticle across
// the four feed switch combinations and asserts the reset matches the
// buildArticleFromEntry state machine (spec scenario "内容实质变化时更新并重走
// 处理链", matrix variant).
func TestRefreshExistingArticleStatusMatrix(t *testing.T) {
	cases := []struct {
		name               string
		firecrawl, summary bool
		wantFirecrawl      string
		wantSummary        string
	}{
		{"firecrawl+summary", true, true, "pending", "incomplete"},
		{"firecrawl only", true, false, "pending", "complete"},
		{"summary only", false, true, "completed", "pending"},
		{"neither", false, false, "completed", "complete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupFeedsTestDB(t)

			feed := models.Feed{Title: "Matrix", URL: "https://example.com/matrix", FirecrawlEnabled: tc.firecrawl, ArticleSummaryEnabled: tc.summary}
			if err := database.DB.Create(&feed).Error; err != nil {
				t.Fatalf("create feed: %v", err)
			}
			article := models.Article{
				FeedID: feed.ID, Title: "v1", Description: "v1", Link: "https://example.com/matrix/1",
				FirecrawlStatus: "completed", SummaryStatus: "complete",
			}
			if err := database.DB.Create(&article).Error; err != nil {
				t.Fatalf("create article: %v", err)
			}

			service := newDedupeTestService(t)
			entry := ParsedEntry{Title: "v2", Description: "v2", Link: article.Link}
			if err := service.refreshExistingArticle(feed, entry); err != nil {
				t.Fatalf("refreshExistingArticle: %v", err)
			}

			var reloaded models.Article
			if err := database.DB.First(&reloaded, article.ID).Error; err != nil {
				t.Fatalf("reload article: %v", err)
			}
			if reloaded.FirecrawlStatus != tc.wantFirecrawl {
				t.Fatalf("firecrawl_status = %q, want %q", reloaded.FirecrawlStatus, tc.wantFirecrawl)
			}
			if reloaded.SummaryStatus != tc.wantSummary {
				t.Fatalf("summary_status = %q, want %q", reloaded.SummaryStatus, tc.wantSummary)
			}
		})
	}
}
