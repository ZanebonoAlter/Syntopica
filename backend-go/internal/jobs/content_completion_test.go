package jobs

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"syntopica-backend/internal/domain/content"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

func setupSchedulersTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	database.DB = db
	if err := database.DB.AutoMigrate(&models.Feed{}, &models.Article{}, &models.SchedulerTask{}, &models.AISettings{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
}

func TestContentCompletionSchedulerGetStatusIncludesOverviewAndCurrentArticle(t *testing.T) {
	setupSchedulersTestDB(t)

	feed := models.Feed{
		Title:                 "Feed",
		URL:                   "https://feed.example/rss",
		ArticleSummaryEnabled: true,
		FirecrawlEnabled:      true,
		MaxCompletionRetries:  3,
	}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	article := models.Article{
		FeedID:           feed.ID,
		Title:            "Queue me",
		Link:             "https://feed.example/a1",
		FirecrawlStatus:  "completed",
		SummaryStatus:    "incomplete",
		FirecrawlContent: "ready",
	}
	if err := database.DB.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	nextRun := time.Now().Add(time.Hour)
	task := models.SchedulerTask{
		Name:              "ai_summary",
		Description:       "AI summarize Firecrawl content",
		CheckInterval:     3600,
		Status:            "running",
		NextExecutionTime: &nextRun,
	}
	if err := database.DB.Create(&task).Error; err != nil {
		t.Fatalf("create scheduler task: %v", err)
	}

	scheduler := &ContentCompletionScheduler{
		completionService: content.NewContentCompletionService(),
		checkInterval:     time.Hour,
		taskName:          "ai_summary",
		isExecuting:       true,
		currentArticle: &content.ContentCompletionArticleRef{
			ID:     article.ID,
			FeedID: article.FeedID,
			Title:  article.Title,
		},
	}

	status := scheduler.GetStatus()
	if !status.IsExecuting {
		t.Fatalf("is_executing = %v, want true", status.IsExecuting)
	}
	if status.Name != "Content Completion" {
		t.Fatalf("name = %q, want Content Completion", status.Name)
	}

	details := scheduler.GetTaskStatusDetails()

	overview, ok := details["overview"].(map[string]interface{})
	if !ok {
		t.Fatalf("overview missing or invalid: %#v", details["overview"])
	}
	if overview["pending_count"] != 1 {
		t.Fatalf("pending_count = %v, want 1", overview["pending_count"])
	}

	current, ok := details["current_article"].(*content.ContentCompletionArticleRef)
	if !ok {
		t.Fatalf("current article missing or invalid: %#v", details["current_article"])
	}
	if current.Title != "Queue me" {
		t.Fatalf("current article title = %q, want Queue me", current.Title)
	}

	if details["live_processing_count"] != 1 {
		t.Fatalf("live_processing_count = %v, want 1", details["live_processing_count"])
	}
	if details["stale_processing_count"] != 0 {
		t.Fatalf("stale_processing_count = %v, want 0", details["stale_processing_count"])
	}
}

func TestParseLastRunSummaryFromSchedulerTask(t *testing.T) {
	nextRun := time.Now().Add(time.Hour)
	task := models.SchedulerTask{
		Name:                "ai_summary",
		NextExecutionTime:   &nextRun,
		LastExecutionResult: `{"started_at":"2026-03-08T10:00:00+08:00","finished_at":"2026-03-08T10:01:00+08:00","completed_count":3,"failed_count":1,"blocked_count":2,"stale_processing_count":1,"error_samples":[{"article_id":18118,"message":"unexpected EOF","category":"network"}]}`,
	}

	summary := parseLastRunSummary(task)
	if summary == nil {
		t.Fatal("expected parsed last run summary")
	}
	if summary.CompletedCount != 3 {
		t.Fatalf("completed_count = %d, want 3", summary.CompletedCount)
	}
	if len(summary.ErrorSamples) != 1 || summary.ErrorSamples[0].Category != "network" {
		t.Fatalf("error samples = %#v, want network category", summary.ErrorSamples)
	}
}

func TestContentCompletionSchedulerStartRepairsLegacyTaskRows(t *testing.T) {
	setupSchedulersTestDB(t)

	now := time.Now()
	staleRun := now.Add(-2 * time.Hour)
	legacyNextRun := now.Add(15 * time.Minute)

	primary := models.SchedulerTask{
		Name:              "ai_summary",
		Description:       "primary",
		CheckInterval:     3600,
		Status:            "running",
		LastExecutionTime: &staleRun,
		NextExecutionTime: &legacyNextRun,
	}
	if err := database.DB.Create(&primary).Error; err != nil {
		t.Fatalf("create primary task: %v", err)
	}

	legacy := models.SchedulerTask{
		Name:                "content_completion",
		Description:         "legacy",
		CheckInterval:       900,
		Status:              "idle",
		LastExecutionResult: `{"completed_count":1}`,
	}
	if err := database.DB.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy task: %v", err)
	}

	scheduler := NewContentCompletionScheduler(content.NewContentCompletionService(), 60)
	if err := scheduler.Start(); err != nil {
		t.Fatalf("start scheduler: %v", err)
	}
	defer scheduler.Stop()

	if scheduler.checkInterval != 15*time.Minute {
		t.Fatalf("check interval = %v, want 15m", scheduler.checkInterval)
	}

	var repaired models.SchedulerTask
	if err := database.DB.Where("name = ?", "ai_summary").First(&repaired).Error; err != nil {
		t.Fatalf("load repaired task: %v", err)
	}
	if repaired.Status != "idle" {
		t.Fatalf("repaired status = %q, want idle", repaired.Status)
	}
	if repaired.CheckInterval != 900 {
		t.Fatalf("repaired check interval = %d, want 900", repaired.CheckInterval)
	}

	var legacyCount int64
	if err := database.DB.Model(&models.SchedulerTask{}).Where("name = ?", "content_completion").Count(&legacyCount).Error; err != nil {
		t.Fatalf("count legacy tasks: %v", err)
	}
	if legacyCount != 0 {
		t.Fatalf("legacy content_completion rows = %d, want 0", legacyCount)
	}
}

func TestContentCompletionSchedulerSkipsCycleWhenAlreadyRunning(t *testing.T) {
	setupSchedulersTestDB(t)

	task := models.SchedulerTask{
		Name:          "ai_summary",
		Description:   "AI summarize Firecrawl content",
		CheckInterval: 3600,
		Status:        "idle",
	}
	if err := database.DB.Create(&task).Error; err != nil {
		t.Fatalf("create scheduler task: %v", err)
	}

	scheduler := NewContentCompletionScheduler(content.NewContentCompletionService(), 60)
	scheduler.executionMutex.Lock()
	defer scheduler.executionMutex.Unlock()

	scheduler.checkAndCompleteArticles()

	var refreshed models.SchedulerTask
	if err := database.DB.Where("name = ?", "ai_summary").First(&refreshed).Error; err != nil {
		t.Fatalf("reload scheduler task: %v", err)
	}
	if refreshed.LastExecutionTime != nil {
		t.Fatalf("last execution time = %v, want nil when cycle is skipped", refreshed.LastExecutionTime)
	}
	if refreshed.Status != "idle" {
		t.Fatalf("status = %q, want idle when cycle is skipped", refreshed.Status)
	}
}
