package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/reader/repository"
	"syntopica-backend/internal/reader/service"
)

func setupHandlersTestDB(t *testing.T) {
	t.Helper()

	// 每个测试（含 -count>1 的每一轮）独占一个内存库。固定 DSN "file::memory:?cache=shared"
	// 是进程内共享库，多个测试/多轮共用同一份数据，第二轮起会撞上上一轮残留行
	// （feeds.url / ai_providers.name UNIQUE 冲突）。
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:content_completion_handler_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	database.DB = db
	repository.InitRepository(database.DB)
	if err := database.DB.AutoMigrate(&models.Feed{}, &models.Article{}, &models.SchedulerTask{}, &models.TopicTag{}, &models.ArticleTopicTag{}, &models.TagJob{}, &models.FirecrawlJob{}, &models.AIProvider{}, &models.AIRoute{}, &models.AIRouteProvider{}, &models.AICallLog{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
}

func TestCompleteFeedArticlesRetriesFailedArticlesWhenTriggeredManually(t *testing.T) {
	setupHandlersTestDB(t)
	gin.SetMode(gin.TestMode)

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"# Test\n\n## 导读\n- ok"}}]}`))
	}))
	defer aiServer.Close()

	completionService = service.NewContentCompletionService()
	completionService.SetAICredentials(aiServer.URL, "token", "test-model")

	feed := models.Feed{
		Title:                 "Feed",
		URL:                   "https://example.com/rss",
		ArticleSummaryEnabled: true,
		FirecrawlEnabled:      true,
		MaxCompletionRetries:  1,
	}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	article := models.Article{
		FeedID:             feed.ID,
		Title:              "Retry me manually",
		Link:               "https://example.com/a1",
		FirecrawlStatus:    "completed",
		FirecrawlContent:   "body",
		SummaryStatus:      "failed",
		CompletionAttempts: 1,
		CompletionError:    "Max retries exceeded",
	}
	if err := database.DB.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/feeds/1/complete", nil)
	ctx.Params = gin.Params{{Key: "feed_id", Value: "1"}}

	CompleteFeedArticles(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["completed"] != float64(1) {
		t.Fatalf("completed = %v, want 1", body["completed"])
	}

	var refreshed models.Article
	if err := database.DB.First(&refreshed, article.ID).Error; err != nil {
		t.Fatalf("reload article: %v", err)
	}
	if refreshed.SummaryStatus != "complete" {
		t.Fatalf("summary status = %q, want complete", refreshed.SummaryStatus)
	}
	if refreshed.CompletionAttempts != 2 {
		t.Fatalf("completion attempts = %d, want 2", refreshed.CompletionAttempts)
	}
}

func TestCompleteArticleContentReloadsProviderWithoutSummaryRouteOrAPIKey(t *testing.T) {
	setupHandlersTestDB(t)
	gin.SetMode(gin.TestMode)

	completionService = service.NewContentCompletionService()

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"# Test\n\n## 导读\n- reloaded"}}]}`))
	}))
	defer aiServer.Close()

	provider := models.AIProvider{
		Name:         "late-local-provider",
		ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL:      aiServer.URL,
		APIKey:       "",
		Model:        "test-model",
		Enabled:      true,
	}
	if err := database.DB.Create(&provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}

	feed := models.Feed{
		Title:                 "Feed",
		URL:                   "https://example.com/late-provider.xml",
		ArticleSummaryEnabled: true,
		FirecrawlEnabled:      true,
		MaxCompletionRetries:  1,
	}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	article := models.Article{
		FeedID:           feed.ID,
		Title:            "Configured after startup",
		Link:             "https://example.com/late-provider",
		FirecrawlStatus:  "completed",
		FirecrawlContent: "body",
		SummaryStatus:    "incomplete",
	}
	if err := database.DB.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/articles/%d/complete", article.ID), nil)
	ctx.Params = gin.Params{{Key: "article_id", Value: fmt.Sprint(article.ID)}}

	CompleteArticleContent(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}

	var refreshed models.Article
	if err := database.DB.First(&refreshed, article.ID).Error; err != nil {
		t.Fatalf("reload article: %v", err)
	}
	if refreshed.SummaryStatus != "complete" {
		t.Fatalf("summary status = %q, want complete; error=%q", refreshed.SummaryStatus, refreshed.CompletionError)
	}
}
