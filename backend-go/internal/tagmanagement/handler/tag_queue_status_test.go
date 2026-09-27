package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/repository"
)

// setupTagQueueStatusTestDB initializes the shared test DB and resets the
// tag-queue status reader singleton so it binds to the test database
// (same pattern as merge_reembedding_queue_test.go).
func setupTagQueueStatusTestDB(t *testing.T) *gorm.DB {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	tagQueueStatusService = nil
	tagQueueStatusOnce = sync.Once{}

	return db
}

func setupTagQueueStatusRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	RegisterTagQueueRoutes(api)
	return router
}

func seedTagJobAt(t *testing.T, db *gorm.DB, articleID uint, status string, createdAt time.Time) {
	t.Helper()
	job := models.TagJob{
		ArticleID:   articleID,
		Status:      status,
		AvailableAt: createdAt,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("seed tag job: %v", err)
	}
	// created_at is auto-populated by GORM on insert; pin it explicitly
	// to exercise the today/yesterday boundary.
	if err := db.Model(&models.TagJob{}).Where("id = ?", job.ID).
		Update("created_at", createdAt).Error; err != nil {
		t.Fatalf("set created_at: %v", err)
	}
}

func tagQueueStatusBody(t *testing.T, router *gin.Engine) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tag-queue/status", nil)
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status endpoint returned %d: %s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

// TestGetTagQueueStatus_CompletedTodayBoundary covers the today/yesterday
// day-boundary for completed_today (add-notification-center 白盒 C 边界)：
// yesterday's completed rows count only toward cumulative completed, while
// today's completed rows count toward both.
func TestGetTagQueueStatus_CompletedTodayBoundary(t *testing.T) {
	db := setupTagQueueStatusTestDB(t)
	router := setupTagQueueStatusRouter()

	now := time.Now()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := startOfToday.Add(-time.Hour)

	// today 00:00 sharp is IN (>= boundary); yesterday's last second is OUT.
	seedTagJobAt(t, db, 101, string(models.JobStatusCompleted), startOfToday)
	seedTagJobAt(t, db, 102, string(models.JobStatusCompleted), startOfToday.Add(time.Hour))
	seedTagJobAt(t, db, 103, string(models.JobStatusCompleted), yesterday)

	// non-completed rows must not leak into completed_today.
	seedTagJobAt(t, db, 104, string(models.JobStatusPending), startOfToday.Add(time.Hour))
	seedTagJobAt(t, db, 105, string(models.JobStatusFailed), yesterday)

	body := tagQueueStatusBody(t, router)
	data, _ := body["data"].(map[string]any)
	if data == nil {
		t.Fatalf("missing data in response: %v", body)
	}

	if got, ok := data["completed_today"].(float64); !ok || got != 2 {
		t.Fatalf("completed_today = %v (%T), want 2 (2 today-completed, yesterday excluded)", data["completed_today"], data["completed_today"])
	}
	if got, ok := data["completed"].(float64); !ok || got != 3 {
		t.Fatalf("completed = %v (%T), want 3 (cumulative)", data["completed"], data["completed"])
	}
	if got, ok := data["pending"].(float64); !ok || got != 1 {
		t.Fatalf("pending = %v (%T), want 1", data["pending"], data["pending"])
	}
	if got, ok := data["failed"].(float64); !ok || got != 1 {
		t.Fatalf("failed = %v (%T), want 1", data["failed"], data["failed"])
	}
}
