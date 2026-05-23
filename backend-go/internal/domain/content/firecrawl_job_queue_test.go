package content

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func setupFirecrawlJobQueueTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	database.DB = db
	if err := database.DB.AutoMigrate(&models.Feed{}, &models.Article{}, &models.FirecrawlJob{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
}

func TestEnqueueFirecrawlJobDedupesActiveJob(t *testing.T) {
	setupFirecrawlJobQueueTestDB(t)

	feed := models.Feed{Title: "Feed", URL: fmt.Sprintf("https://example.com/%s", t.Name())}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	article := models.Article{FeedID: feed.ID, Title: "Article", Link: "https://example.com/article", FirecrawlStatus: "pending"}
	if err := database.DB.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	queue := NewFirecrawlJobQueue(database.DB)
	if err := queue.Enqueue(article); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if err := queue.Enqueue(article); err != nil {
		t.Fatalf("second enqueue: %v", err)
	}

	var count int64
	if err := database.DB.Model(&models.FirecrawlJob{}).Where("article_id = ?", article.ID).Count(&count).Error; err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("job count = %d, want 1", count)
	}
}

func TestClaimFirecrawlJobsReclaimsExpiredLease(t *testing.T) {
	setupFirecrawlJobQueueTestDB(t)

	now := time.Now().Add(-time.Hour)
	job := models.FirecrawlJob{
		ArticleID:      1,
		Status:         string(models.JobStatusLeased),
		AttemptCount:   1,
		MaxAttempts:    5,
		AvailableAt:    now,
		LeasedAt:       &now,
		LeaseExpiresAt: &now,
	}
	if err := database.DB.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	queue := NewFirecrawlJobQueue(database.DB)
	jobs, err := queue.Claim(1, 5*time.Minute)
	if err != nil {
		t.Fatalf("claim jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("claimed jobs = %d, want 1", len(jobs))
	}
	if jobs[0].Status != string(models.JobStatusLeased) {
		t.Fatalf("job status = %q, want leased", jobs[0].Status)
	}
	if jobs[0].LeaseExpiresAt == nil || !jobs[0].LeaseExpiresAt.After(time.Now()) {
		t.Fatalf("lease expiry = %#v, want future time", jobs[0].LeaseExpiresAt)
	}
}
