package feeds

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func setupFeedsTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	database.DB = db
	if err := database.DB.AutoMigrate(&models.Feed{}, &models.Article{}, &models.TopicTag{}, &models.ArticleTopicTag{}, &models.FirecrawlJob{}, &models.TagJob{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
}

func TestBuildArticleFromEntryTracksOnlyRunnableStates(t *testing.T) {
	service := NewFeedService()
	entry := ParsedEntry{
		Title:       "Fresh News",
		Description: "desc",
		Content:     "content",
		Link:        "https://example.com/article",
		Author:      "bot",
	}

	tests := []struct {
		name                  string
		firecrawlEnabled      bool
		articleSummaryEnabled bool
		wantFirecrawlStatus   string
		wantSummaryStatus     string
	}{
		{
			name:                  "both enabled: summary incomplete, firecrawl pending",
			firecrawlEnabled:      true,
			articleSummaryEnabled: true,
			wantFirecrawlStatus:   "pending",
			wantSummaryStatus:     "incomplete",
		},
		{
			name:                  "summary only: summary pending, no firecrawl",
			firecrawlEnabled:      false,
			articleSummaryEnabled: true,
			wantFirecrawlStatus:   "complete",
			wantSummaryStatus:     "pending",
		},
		{
			name:                  "neither enabled: both default",
			firecrawlEnabled:      false,
			articleSummaryEnabled: false,
			wantFirecrawlStatus:   "complete",
			wantSummaryStatus:     "complete",
		},
		{
			name:                  "firecrawl only: summary complete, firecrawl pending",
			firecrawlEnabled:      true,
			articleSummaryEnabled: false,
			wantFirecrawlStatus:   "pending",
			wantSummaryStatus:     "complete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feed := models.Feed{FirecrawlEnabled: tt.firecrawlEnabled, ArticleSummaryEnabled: tt.articleSummaryEnabled}
			article := service.buildArticleFromEntry(feed, entry)
			if article.FirecrawlStatus != tt.wantFirecrawlStatus {
				t.Errorf("firecrawl status = %q, want %q", article.FirecrawlStatus, tt.wantFirecrawlStatus)
			}
			if article.SummaryStatus != tt.wantSummaryStatus {
				t.Errorf("summary status = %q, want %q", article.SummaryStatus, tt.wantSummaryStatus)
			}
		})
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
	if len(remaining) != 2 {
		t.Fatalf("remaining articles = %d, want 2", len(remaining))
	}

	titles := map[string]bool{}
	for _, article := range remaining {
		titles[article.Title] = true
	}

	if !titles["new complete"] || !titles["middle pending"] {
		t.Fatalf("expected newest two articles to remain, remaining = %#v", titles)
	}
	if titles["old incomplete"] {
		t.Fatalf("expected oldest article to be deleted even if incomplete, remaining = %#v", titles)
	}
}

func TestRefreshFeedEnqueuesTagJobWhenCompletionDisabled(t *testing.T) {
	setupFeedsTestDB(t)

	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>OpenAI Feed</title>
    <description>Feed for tests</description>
    <link>https://example.com</link>
    <item>
      <title>OpenAI launches new AI agent runtime</title>
      <link>https://example.com/openai-agent</link>
      <description>OpenAI agentic workflow update</description>
      <pubDate>Sun, 22 Mar 2026 09:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`))
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
	if err := service.RefreshFeed(context.Background(), feed.ID); err != nil {
		t.Fatalf("refresh feed: %v", err)
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

	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Queued Feed</title>
    <description>Feed for tests</description>
    <link>https://example.com</link>
    <item>
      <title>Queued article</title>
      <link>https://example.com/queued</link>
      <description>queued desc</description>
      <pubDate>Sun, 22 Mar 2026 09:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`))
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

	rssServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <description>desc</description>
    <link>https://example.com</link>
    <item>
      <title>Test Article</title>
      <link>https://example.com/test</link>
      <description>desc</description>
      <pubDate>Sun, 22 Mar 2026 09:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`))
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
		wantDeleted bool
	}{
		{
			name:        "max_articles=0 means unlimited, no deletion",
			maxArticles: 0,
			wantDeleted: false,
		},
		{
			name:        "max_articles=9999 means unlimited, no deletion",
			maxArticles: 9999,
			wantDeleted: false,
		},
		{
			name:        "max_articles=2 triggers cleanup",
			maxArticles: 2,
			wantDeleted: true,
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

			if tt.wantDeleted {
				if remaining > int64(tt.maxArticles) {
					t.Errorf("remaining articles = %d, expected <= %d", remaining, tt.maxArticles)
				}
			} else {
				if remaining != 5 {
					t.Errorf("remaining articles = %d, expected 5 (no deletion)", remaining)
				}
			}
		})
	}
}
