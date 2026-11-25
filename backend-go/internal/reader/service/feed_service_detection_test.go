package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/reader/repository"
	tagRepo "syntopica-backend/internal/tagmanagement/repository"
)

// ============================================================================
// 文章入库入队点包级测试（add-ai-generated-article-detection task 3.3 ③）
// ============================================================================

func setupFeedDetectionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)

	database.DB = db
	repository.InitRepository(db)
	tagRepo.InitRepository(db)
	require.NoError(t, db.AutoMigrate(
		&models.Feed{}, &models.Article{}, &models.TopicTag{}, &models.ArticleTopicTag{},
		&models.FirecrawlJob{}, &models.TagJob{}, &models.DetectionJob{},
	))
	return db
}

func countJobs[T any](t *testing.T, db *gorm.DB, articleID uint) int64 {
	t.Helper()

	var count int64
	require.NoError(t, db.Model(new(T)).Where("article_id = ?", articleID).Count(&count).Error)
	return count
}

// 未开全文抓取的 feed：文章入库即用 RSS 正文入队判定（不受打标开关联动影响）。
// 开了全文抓取的 feed：判定推迟到抓取完成/失败节点，入库时不入队。
func TestEnqueueArticleProcessingAIDetectionMatrix(t *testing.T) {
	tests := []struct {
		name              string
		firecrawlEnabled  bool
		aiDetectionSwitch bool
		taggingEnabled    bool
		wantDetectionJobs int64
		wantFirecrawlJobs int64
	}{
		{
			name:              "firecrawl off + detection on + tagging off: detection at ingest",
			aiDetectionSwitch: true,
			wantDetectionJobs: 1,
		},
		{
			name:              "firecrawl off + detection off + tagging on: no detection",
			taggingEnabled:    true,
			wantDetectionJobs: 0,
		},
		{
			name:              "firecrawl on + detection on: detection waits for crawl completion",
			firecrawlEnabled:  true,
			aiDetectionSwitch: true,
			wantDetectionJobs: 0,
			wantFirecrawlJobs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupFeedDetectionTestDB(t)

			feed := models.Feed{
				Title:              tt.name,
				URL:                "https://example.com/" + tt.name,
				FirecrawlEnabled:   tt.firecrawlEnabled,
				TaggingEnabled:     tt.taggingEnabled,
				AIDetectionEnabled: tt.aiDetectionSwitch,
			}
			require.NoError(t, db.Create(&feed).Error)
			article := models.Article{
				FeedID:      feed.ID,
				Title:       "Article",
				Link:        "https://example.com/article",
				Description: "rss body",
			}
			require.NoError(t, db.Create(&article).Error)

			require.NoError(t, NewFeedService().enqueueArticleProcessing(feed, article))

			require.Equal(t, tt.wantDetectionJobs, countJobs[models.DetectionJob](t, db, article.ID))
			require.Equal(t, tt.wantFirecrawlJobs, countJobs[models.FirecrawlJob](t, db, article.ID))

			if tt.wantDetectionJobs == 1 {
				var job models.DetectionJob
				require.NoError(t, db.Where("article_id = ?", article.ID).First(&job).Error)
				require.Equal(t, repository.DetectionJobReasonArticleIngested, job.Reason)
				require.Equal(t, string(models.JobStatusPending), job.Status)
			}
		})
	}
}

// 端到端（刷新链路）：feed 未开全文抓取且开启判定开关 → 新文章入库即产生判定作业。
func TestRefreshFeedEnqueuesAIDetectionJob(t *testing.T) {
	setupFeedDetectionTestDB(t)

	var rssServer *httptest.Server
	rssServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Detection Feed</title>
    <description>Feed for tests</description>
    <link>%s</link>
    <item>
      <title>Detection article</title>
      <link>%s/detection</link>
      <description>judge me</description>
      <pubDate>Sun, 22 Mar 2026 09:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`, rssServer.URL, rssServer.URL)
	}))
	defer rssServer.Close()

	feed := models.Feed{
		Title:                 "Detection Feed",
		URL:                   rssServer.URL,
		MaxArticles:           10,
		FirecrawlEnabled:      false,
		TaggingEnabled:        false,
		ArticleSummaryEnabled: false,
		AIDetectionEnabled:    true,
	}
	require.NoError(t, database.DB.Create(&feed).Error)

	service := NewFeedService()
	service.iconStore = NewIconStore(t.TempDir())
	require.NoError(t, service.RefreshFeed(context.Background(), feed.ID))

	var article models.Article
	require.NoError(t, database.DB.First(&article).Error)
	require.EqualValues(t, 1, countJobs[models.DetectionJob](t, database.DB, article.ID))
}
