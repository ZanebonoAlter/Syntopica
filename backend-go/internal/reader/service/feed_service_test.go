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
)

func setupFeedsTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	database.DB = db
	repository.InitRepository(database.DB)
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
