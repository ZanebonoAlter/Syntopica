package jobs

import (
	"reflect"
	"testing"
	"time"

	"syntopica-backend/internal/domain/content"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

func TestSchedulerStatusResponseDefinition(t *testing.T) {
	type fieldExpectation struct {
		name    string
		typeOf  reflect.Type
		jsonTag string
	}

	expected := []fieldExpectation{
		{name: "Name", typeOf: reflect.TypeOf(""), jsonTag: "name"},
		{name: "Status", typeOf: reflect.TypeOf(""), jsonTag: "status"},
		{name: "CheckInterval", typeOf: reflect.TypeOf(int64(0)), jsonTag: "check_interval"},
		{name: "NextRun", typeOf: reflect.TypeOf(int64(0)), jsonTag: "next_run"},
		{name: "IsExecuting", typeOf: reflect.TypeOf(false), jsonTag: "is_executing"},
	}

	typ := reflect.TypeOf(SchedulerStatusResponse{})
	if typ.NumField() < len(expected) {
		t.Fatalf("field count = %d, want at least %d", typ.NumField(), len(expected))
	}

	for _, want := range expected {
		field, ok := typ.FieldByName(want.name)
		if !ok {
			t.Fatalf("missing field %s", want.name)
		}
		if field.Type != want.typeOf {
			t.Fatalf("field %s type = %v, want %v", want.name, field.Type, want.typeOf)
		}
		if field.Tag.Get("json") != want.jsonTag {
			t.Fatalf("field %s json tag = %q, want %q", want.name, field.Tag.Get("json"), want.jsonTag)
		}
	}
}

func TestSchedulerStatusFormat(t *testing.T) {
	setupSchedulersTestDB(t)

	nextRun := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Second)
	if err := database.DB.Create(&models.SchedulerTask{
		Name:              "auto_refresh",
		Description:       "Auto-refresh RSS feeds",
		CheckInterval:     60,
		Status:            "idle",
		NextExecutionTime: &nextRun,
	}).Error; err != nil {
		t.Fatalf("create auto_refresh task: %v", err)
	}

	if err := database.DB.Create(&models.SchedulerTask{
		Name:              "ai_summary",
		Description:       "AI summarize Firecrawl content",
		CheckInterval:     3600,
		Status:            "idle",
		NextExecutionTime: &nextRun,
	}).Error; err != nil {
		t.Fatalf("create ai_summary task: %v", err)
	}

	autoRefresh := NewAutoRefreshScheduler(60)
	autoRefresh.isRunning = true
	autoRefresh.isExecuting = true
	autoRefreshStatus := autoRefresh.GetStatus()
	assertSchedulerStatus(t, autoRefreshStatus, SchedulerStatusResponse{
		Name:          "Auto Refresh",
		Status:        "running",
		CheckInterval: 60,
		NextRun:       nextRun.Unix(),
		IsExecuting:   true,
	})

	preference := NewPreferenceUpdateScheduler(1800)
	preference.running = true
	preference.isExecuting = true
	preference.nextRun = &nextRun
	preferenceStatus := preference.GetStatus()
	assertSchedulerStatus(t, preferenceStatus, SchedulerStatusResponse{
		Name:          "Preference Update",
		Status:        "running",
		CheckInterval: 1800,
		NextRun:       nextRun.Unix(),
		IsExecuting:   true,
	})

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

	completion := &ContentCompletionScheduler{
		completionService: content.NewContentCompletionService(),
		checkInterval:     time.Hour,
		taskName:          "ai_summary",
		isRunning:         true,
	}
	completionStatus := completion.GetStatus()
	assertSchedulerStatus(t, completionStatus, SchedulerStatusResponse{
		Name:          "Content Completion",
		Status:        "idle",
		CheckInterval: 3600,
		NextRun:       nextRun.Unix(),
		IsExecuting:   false,
	})

	firecrawl := NewFirecrawlScheduler()
	firecrawl.status = "running"
	firecrawl.nextRun = &nextRun
	firecrawlStatus := firecrawl.GetStatus()
	assertSchedulerStatus(t, firecrawlStatus, SchedulerStatusResponse{
		Name:          "Firecrawl Crawler",
		Status:        "running",
		CheckInterval: 300,
		NextRun:       nextRun.Unix(),
		IsExecuting:   true,
	})
}

func assertSchedulerStatus(t *testing.T, got, want SchedulerStatusResponse) {
	t.Helper()
	if got.Name != want.Name {
		t.Fatalf("Name = %q, want %q", got.Name, want.Name)
	}
	if got.Status != want.Status {
		t.Fatalf("Status = %q, want %q", got.Status, want.Status)
	}
	if got.CheckInterval != want.CheckInterval {
		t.Fatalf("CheckInterval = %d, want %d", got.CheckInterval, want.CheckInterval)
	}
	if got.NextRun != want.NextRun {
		t.Fatalf("NextRun = %d, want %d", got.NextRun, want.NextRun)
	}
	if got.IsExecuting != want.IsExecuting {
		t.Fatalf("IsExecuting = %v, want %v", got.IsExecuting, want.IsExecuting)
	}
}
