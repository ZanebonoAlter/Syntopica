package content

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
)

func setupServicesTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	database.DB = db
	if err := database.DB.AutoMigrate(&models.TopicTag{}, &models.ArticleTopicTag{}, &models.TagJob{}, &models.FirecrawlJob{}); err != nil {
		t.Fatalf("migrate topic tag tables: %v", err)
	}
	if err := database.DB.AutoMigrate(&models.Feed{}, &models.Article{}, &models.SchedulerTask{}, &models.AIProvider{}, &models.AIRoute{}, &models.AIRouteProvider{}, &models.AICallLog{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
}

func TestCompleteArticleWithForceUsesAIRouterRoute(t *testing.T) {
	setupServicesTestDB(t)

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"# Test\n\n## 导读\n- routed"}}]}`))
	}))
	defer aiServer.Close()

	provider := models.AIProvider{Name: "completion-primary", ProviderType: airouter.ProviderTypeOpenAICompatible, BaseURL: aiServer.URL, APIKey: "token", Model: "test-model", Enabled: true}
	if err := database.DB.Create(&provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	route := models.AIRoute{Name: airouter.DefaultRouteName, Capability: string(airouter.CapabilityArticleCompletion), Enabled: true, Strategy: "ordered_failover"}
	if err := database.DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	if err := database.DB.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: provider.ID, Priority: 1, Enabled: true}).Error; err != nil {
		t.Fatalf("create route provider: %v", err)
	}

	feed := models.Feed{Title: "Feed", URL: fmt.Sprintf("https://example.com/%s", t.Name()), ArticleSummaryEnabled: true, FirecrawlEnabled: true, MaxCompletionRetries: 2}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	article := models.Article{FeedID: feed.ID, Title: "Need routing", Link: "https://example.com/a1", FirecrawlStatus: "completed", FirecrawlContent: "body", SummaryStatus: "incomplete"}
	if err := database.DB.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	service := NewContentCompletionService("http://localhost:11235")
	if err := service.CompleteArticleWithForce(context.Background(), article.ID, false); err != nil {
		t.Fatalf("complete article: %v", err)
	}

	var refreshed models.Article
	if err := database.DB.First(&refreshed, article.ID).Error; err != nil {
		t.Fatalf("reload article: %v", err)
	}
	if refreshed.SummaryStatus != "complete" {
		t.Fatalf("summary status = %q, want complete", refreshed.SummaryStatus)
	}
	if refreshed.SummaryGeneratedAt == nil {
		t.Fatal("expected summary generated timestamp to be populated")
	}
	if refreshed.AIContentSummary == "" {
		t.Fatal("expected AI content summary to be populated from router")
	}

	var jobCount int64
	if err := database.DB.Model(&models.TagJob{}).Where("article_id = ?", article.ID).Count(&jobCount).Error; err != nil {
		t.Fatalf("count tag jobs: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("tag job count = %d, want 1", jobCount)
	}
}

func TestGetOverviewCountsQueueState(t *testing.T) {
	setupServicesTestDB(t)

	feedEnabled := models.Feed{
		Title:                 "Enabled Feed",
		URL:                   "https://enabled.example/rss",
		ArticleSummaryEnabled: true,
		FirecrawlEnabled:      true,
		MaxCompletionRetries:  3,
	}
	feedDisabled := models.Feed{
		Title:                 "Disabled Feed",
		URL:                   "https://disabled.example/rss",
		ArticleSummaryEnabled: false,
		FirecrawlEnabled:      true,
		MaxCompletionRetries:  3,
	}

	if err := database.DB.Create(&feedEnabled).Error; err != nil {
		t.Fatalf("create enabled feed: %v", err)
	}
	if err := database.DB.Create(&feedDisabled).Error; err != nil {
		t.Fatalf("create disabled feed: %v", err)
	}

	now := time.Now()
	articles := []models.Article{
		{FeedID: feedEnabled.ID, Title: "eligible-1", Link: "https://a/1", FirecrawlStatus: "completed", SummaryStatus: "incomplete", FirecrawlContent: "ready"},
		{FeedID: feedEnabled.ID, Title: "eligible-2", Link: "https://a/2", FirecrawlStatus: "completed", SummaryStatus: "incomplete", FirecrawlContent: "ready"},
		{FeedID: feedEnabled.ID, Title: "processing", Link: "https://a/3", FirecrawlStatus: "completed", SummaryStatus: "pending", FirecrawlContent: "ready"},
		{FeedID: feedEnabled.ID, Title: "done", Link: "https://a/4", FirecrawlStatus: "completed", SummaryStatus: "complete", SummaryGeneratedAt: &now},
		{FeedID: feedEnabled.ID, Title: "failed", Link: "https://a/5", FirecrawlStatus: "completed", SummaryStatus: "failed", CompletionError: "boom"},
		{FeedID: feedEnabled.ID, Title: "waiting crawl", Link: "https://a/6", FirecrawlStatus: "pending", SummaryStatus: "incomplete"},
		{FeedID: feedDisabled.ID, Title: "feed disabled", Link: "https://a/7", FirecrawlStatus: "completed", SummaryStatus: "incomplete", FirecrawlContent: "ready"},
	}

	if err := database.DB.Create(&articles).Error; err != nil {
		t.Fatalf("create articles: %v", err)
	}

	service := NewContentCompletionService("http://localhost:11235")
	overview, err := service.GetOverview()
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}

	if overview.PendingCount != 2 {
		t.Fatalf("pending count = %d, want 2", overview.PendingCount)
	}
	if overview.ProcessingCount != 1 {
		t.Fatalf("processing count = %d, want 1", overview.ProcessingCount)
	}
	if overview.CompletedCount != 1 {
		t.Fatalf("completed count = %d, want 1", overview.CompletedCount)
	}
	if overview.FailedCount != 1 {
		t.Fatalf("failed count = %d, want 1", overview.FailedCount)
	}
	if overview.BlockedCount != 2 {
		t.Fatalf("blocked count = %d, want 2", overview.BlockedCount)
	}
	if overview.TotalCount != 7 {
		t.Fatalf("total count = %d, want 7", overview.TotalCount)
	}
	if overview.BlockedReasons.WaitingForFirecrawlCount != 1 {
		t.Fatalf("waiting for firecrawl = %d, want 1", overview.BlockedReasons.WaitingForFirecrawlCount)
	}
	if overview.BlockedReasons.FeedDisabledCount != 1 {
		t.Fatalf("feed disabled = %d, want 1", overview.BlockedReasons.FeedDisabledCount)
	}
	if overview.BlockedReasons.AIUnconfiguredCount != 2 {
		t.Fatalf("ai unconfigured = %d, want 2", overview.BlockedReasons.AIUnconfiguredCount)
	}
	if overview.BlockedReasons.ReadyButMissingContentCount != 0 {
		t.Fatalf("ready but missing content = %d, want 0", overview.BlockedReasons.ReadyButMissingContentCount)
	}
	if overview.LiveProcessingCount != 0 {
		t.Fatalf("live processing count = %d, want 0", overview.LiveProcessingCount)
	}
	if overview.StaleProcessingCount != 1 {
		t.Fatalf("stale processing count = %d, want 1", overview.StaleProcessingCount)
	}
	if overview.StaleProcessingArticle == nil || overview.StaleProcessingArticle.Title != "processing" {
		t.Fatalf("stale processing article = %#v, want processing", overview.StaleProcessingArticle)
	}
}

func TestCompleteArticleWithForceRetriesUntilMaxAndLogsMetadata(t *testing.T) {
	setupServicesTestDB(t)

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":{"code":"1302","message":"rate limited"}}`))
	}))
	defer aiServer.Close()

	provider := models.AIProvider{Name: "completion-primary", ProviderType: airouter.ProviderTypeOpenAICompatible, BaseURL: aiServer.URL, APIKey: "token", Model: "test-model", Enabled: true}
	if err := database.DB.Create(&provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	route := models.AIRoute{Name: airouter.DefaultRouteName, Capability: string(airouter.CapabilityArticleCompletion), Enabled: true, Strategy: "ordered_failover"}
	if err := database.DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	if err := database.DB.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: provider.ID, Priority: 1, Enabled: true}).Error; err != nil {
		t.Fatalf("create route provider: %v", err)
	}

	feed := models.Feed{Title: "Feed", URL: fmt.Sprintf("https://example.com/%s", t.Name()), ArticleSummaryEnabled: true, FirecrawlEnabled: true, MaxCompletionRetries: 2}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	article := models.Article{FeedID: feed.ID, Title: "Need retry", Link: "https://example.com/a1", FirecrawlStatus: "completed", FirecrawlContent: "body", SummaryStatus: "incomplete"}
	if err := database.DB.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	service := NewContentCompletionService("http://localhost:11235")
	if err := service.CompleteArticleWithForce(context.Background(), article.ID, false); err == nil {
		t.Fatal("expected first attempt to fail")
	}

	var refreshed models.Article
	if err := database.DB.First(&refreshed, article.ID).Error; err != nil {
		t.Fatalf("reload article after first attempt: %v", err)
	}
	if refreshed.SummaryStatus != "incomplete" {
		t.Fatalf("summary status after first attempt = %q, want incomplete", refreshed.SummaryStatus)
	}
	if refreshed.CompletionAttempts != 1 {
		t.Fatalf("completion attempts after first attempt = %d, want 1", refreshed.CompletionAttempts)
	}

	var callLogs []models.AICallLog
	if err := database.DB.Order("id ASC").Find(&callLogs).Error; err != nil {
		t.Fatalf("load call logs: %v", err)
	}
	if len(callLogs) != 1 {
		t.Fatalf("call log count after first attempt = %d, want 1", len(callLogs))
	}
	if !strings.Contains(callLogs[0].RequestMeta, fmt.Sprintf(`"article_id":%d`, article.ID)) {
		t.Fatalf("request_meta missing article_id: %s", callLogs[0].RequestMeta)
	}
	if !strings.Contains(callLogs[0].RequestMeta, fmt.Sprintf(`"feed_id":%d`, feed.ID)) {
		t.Fatalf("request_meta missing feed_id: %s", callLogs[0].RequestMeta)
	}

	if err := service.CompleteArticleWithForce(context.Background(), article.ID, false); err == nil {
		t.Fatal("expected second attempt to fail")
	}
	if err := database.DB.First(&refreshed, article.ID).Error; err != nil {
		t.Fatalf("reload article after second attempt: %v", err)
	}
	if refreshed.SummaryStatus != "failed" {
		t.Fatalf("summary status after second attempt = %q, want failed", refreshed.SummaryStatus)
	}
	if refreshed.CompletionAttempts != 2 {
		t.Fatalf("completion attempts after second attempt = %d, want 2", refreshed.CompletionAttempts)
	}
}

func TestCompleteArticleWithForceSkipsFreshPendingAndReclaimsStalePending(t *testing.T) {
	setupServicesTestDB(t)

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"# Test\n\n- reclaimed"}}]}`))
	}))
	defer aiServer.Close()

	provider := models.AIProvider{Name: "completion-primary", ProviderType: airouter.ProviderTypeOpenAICompatible, BaseURL: aiServer.URL, APIKey: "token", Model: "test-model", Enabled: true}
	if err := database.DB.Create(&provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	route := models.AIRoute{Name: airouter.DefaultRouteName, Capability: string(airouter.CapabilityArticleCompletion), Enabled: true, Strategy: "ordered_failover"}
	if err := database.DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	if err := database.DB.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: provider.ID, Priority: 1, Enabled: true}).Error; err != nil {
		t.Fatalf("create route provider: %v", err)
	}

	feed := models.Feed{Title: "Feed", URL: fmt.Sprintf("https://example.com/%s", t.Name()), ArticleSummaryEnabled: true, FirecrawlEnabled: true, MaxCompletionRetries: 2}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	now := time.Now().In(time.FixedZone("CST", 8*3600))
	freshStartedAt := now.Add(-2 * time.Minute)
	staleStartedAt := now.Add(-2 * time.Hour)

	fresh := models.Article{FeedID: feed.ID, Title: "Fresh pending", Link: "https://example.com/fresh", FirecrawlStatus: "completed", FirecrawlContent: "body", SummaryStatus: "pending", SummaryProcessingStartedAt: &freshStartedAt}
	if err := database.DB.Create(&fresh).Error; err != nil {
		t.Fatalf("create fresh article: %v", err)
	}
	stale := models.Article{FeedID: feed.ID, Title: "Stale pending", Link: "https://example.com/stale", FirecrawlStatus: "completed", FirecrawlContent: "body", SummaryStatus: "pending", SummaryProcessingStartedAt: &staleStartedAt}
	if err := database.DB.Create(&stale).Error; err != nil {
		t.Fatalf("create stale article: %v", err)
	}

	service := NewContentCompletionService("http://localhost:11235")
	if err := service.CompleteArticleWithForce(context.Background(), fresh.ID, false); err != nil {
		t.Fatalf("fresh pending should be skipped without error: %v", err)
	}
	if err := service.CompleteArticleWithForce(context.Background(), stale.ID, false); err != nil {
		t.Fatalf("stale pending should be reclaimed: %v", err)
	}

	var freshReloaded models.Article
	if err := database.DB.First(&freshReloaded, fresh.ID).Error; err != nil {
		t.Fatalf("reload fresh article: %v", err)
	}
	if freshReloaded.SummaryStatus != "pending" {
		t.Fatalf("fresh status = %q, want pending", freshReloaded.SummaryStatus)
	}
	if freshReloaded.CompletionAttempts != 0 {
		t.Fatalf("fresh completion attempts = %d, want 0", freshReloaded.CompletionAttempts)
	}

	var staleReloaded models.Article
	if err := database.DB.First(&staleReloaded, stale.ID).Error; err != nil {
		t.Fatalf("reload stale article: %v", err)
	}
	if staleReloaded.SummaryStatus != "complete" {
		t.Fatalf("stale status = %q, want complete", staleReloaded.SummaryStatus)
	}
	if staleReloaded.CompletionAttempts != 1 {
		t.Fatalf("stale completion attempts = %d, want 1", staleReloaded.CompletionAttempts)
	}
	if staleReloaded.SummaryProcessingStartedAt != nil {
		t.Fatalf("stale summary_processing_started_at = %v, want nil after completion", staleReloaded.SummaryProcessingStartedAt)
	}

	var callLogs []models.AICallLog
	if err := database.DB.Order("id ASC").Find(&callLogs).Error; err != nil {
		t.Fatalf("load call logs: %v", err)
	}
	if len(callLogs) != 1 {
		t.Fatalf("call log count = %d, want 1 for reclaimed stale article only", len(callLogs))
	}
}

func TestCompleteArticleWithMetadataAddsSchedulerRunIDToCallLogs(t *testing.T) {
	setupServicesTestDB(t)

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"# Test\n\n- ok"}}]}`))
	}))
	defer aiServer.Close()

	provider := models.AIProvider{Name: "completion-primary", ProviderType: airouter.ProviderTypeOpenAICompatible, BaseURL: aiServer.URL, APIKey: "token", Model: "test-model", Enabled: true}
	if err := database.DB.Create(&provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	route := models.AIRoute{Name: airouter.DefaultRouteName, Capability: string(airouter.CapabilityArticleCompletion), Enabled: true, Strategy: "ordered_failover"}
	if err := database.DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	if err := database.DB.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: provider.ID, Priority: 1, Enabled: true}).Error; err != nil {
		t.Fatalf("create route provider: %v", err)
	}

	feed := models.Feed{Title: "Feed", URL: fmt.Sprintf("https://example.com/%s", t.Name()), ArticleSummaryEnabled: true, FirecrawlEnabled: true, MaxCompletionRetries: 2}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	article := models.Article{FeedID: feed.ID, Title: "Need run id", Link: "https://example.com/run-id", FirecrawlStatus: "completed", FirecrawlContent: "body", SummaryStatus: "incomplete"}
	if err := database.DB.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	service := NewContentCompletionService("http://localhost:11235")
	if err := service.CompleteArticleWithMetadata(context.Background(), article.ID, false, map[string]any{
		"scheduler_run_id": "run-123",
		"trigger_source":   "scheduler",
	}); err != nil {
		t.Fatalf("complete article with metadata: %v", err)
	}

	var callLog models.AICallLog
	if err := database.DB.First(&callLog).Error; err != nil {
		t.Fatalf("load call log: %v", err)
	}

	meta := map[string]any{}
	if err := json.Unmarshal([]byte(callLog.RequestMeta), &meta); err != nil {
		t.Fatalf("unmarshal request meta: %v", err)
	}
	if meta["scheduler_run_id"] != "run-123" {
		t.Fatalf("scheduler_run_id = %v, want run-123", meta["scheduler_run_id"])
	}
	if meta["trigger_source"] != "scheduler" {
		t.Fatalf("trigger_source = %v, want scheduler", meta["trigger_source"])
	}
}

func TestCompleteArticleEnqueuesRetagJobAfterSuccessfulCompletion(t *testing.T) {
	setupServicesTestDB(t)

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"# OpenAI Agent Brief\n\n- OpenAI shipped an AI agent workflow."}}]}`))
	}))
	defer aiServer.Close()

	provider := models.AIProvider{Name: "completion-primary", ProviderType: airouter.ProviderTypeOpenAICompatible, BaseURL: aiServer.URL, APIKey: "token", Model: "test-model", Enabled: true}
	if err := database.DB.Create(&provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	route := models.AIRoute{Name: airouter.DefaultRouteName, Capability: string(airouter.CapabilityArticleCompletion), Enabled: true, Strategy: "ordered_failover"}
	if err := database.DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	if err := database.DB.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: provider.ID, Priority: 1, Enabled: true}).Error; err != nil {
		t.Fatalf("create route provider: %v", err)
	}

	feed := models.Feed{Title: "OpenAI Feed", URL: fmt.Sprintf("https://example.com/%s", t.Name()), ArticleSummaryEnabled: true, FirecrawlEnabled: true, MaxCompletionRetries: 2}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	article := models.Article{FeedID: feed.ID, Title: "Daily brief", Link: "https://example.com/tag-after-completion", FirecrawlStatus: "completed", FirecrawlContent: "OpenAI built an AI agent runtime.", SummaryStatus: "incomplete"}
	if err := database.DB.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	service := NewContentCompletionService("http://localhost:11235")
	if err := service.CompleteArticle(context.Background(), article.ID); err != nil {
		t.Fatalf("complete article: %v", err)
	}

	var jobCount int64
	if err := database.DB.Model(&models.TagJob{}).Where("article_id = ?", article.ID).Count(&jobCount).Error; err != nil {
		t.Fatalf("count tag jobs: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("tag job count = %d, want 1", jobCount)
	}
}
