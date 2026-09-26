package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service"
)

func setupMergeReembeddingTestDB(t *testing.T) *gorm.DB {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	mergeQueueService = nil
	mergeQueueServiceOnce = sync.Once{}
	service.MergeReembeddingQueueFactory = service.DefaultMergeReembeddingQueueFactory

	return db
}

func seedMergeQueueTags(t *testing.T, db *gorm.DB) (models.TopicTag, models.TopicTag) {
	t.Helper()

	source := models.TopicTag{Slug: "source-tag", Label: "Source Tag", Category: models.TagCategoryKeyword, Status: "active"}
	target := models.TopicTag{Slug: "target-tag", Label: "Target Tag", Category: models.TagCategoryKeyword, Status: "active"}

	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source tag: %v", err)
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create target tag: %v", err)
	}

	return source, target
}

func setupMergeQueueRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	RegisterMergeReembeddingQueueRoutes(api)
	return router
}

func decodeResponseBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func TestMergeReembeddingQueueStatusEndpointReturnsCounts(t *testing.T) {
	db := setupMergeReembeddingTestDB(t)
	source, target := seedMergeQueueTags(t, db)

	tasks := []tagmodels.MergeReembeddingQueue{
		{SourceTagID: source.ID, TargetTagID: target.ID, Status: tagmodels.MergeReembeddingQueueStatusPending},
		{SourceTagID: source.ID, TargetTagID: target.ID, Status: tagmodels.MergeReembeddingQueueStatusProcessing},
		{SourceTagID: source.ID, TargetTagID: target.ID, Status: tagmodels.MergeReembeddingQueueStatusCompleted},
		{SourceTagID: source.ID, TargetTagID: target.ID, Status: tagmodels.MergeReembeddingQueueStatusFailed},
	}
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/embedding/merge-reembedding/status", nil)
	setupMergeQueueRouter().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	body := decodeResponseBody(t, recorder)
	if body["success"] != true {
		t.Fatalf("expected success response, got %#v", body)
	}

	data := body["data"].(map[string]any)
	if got := int(data["pending"].(float64)); got != 1 {
		t.Fatalf("pending = %d, want 1", got)
	}
	if got := int(data["processing"].(float64)); got != 1 {
		t.Fatalf("processing = %d, want 1", got)
	}
	if got := int(data["completed"].(float64)); got != 1 {
		t.Fatalf("completed = %d, want 1", got)
	}
	if got := int(data["failed"].(float64)); got != 1 {
		t.Fatalf("failed = %d, want 1", got)
	}
	if got := int(data["total"].(float64)); got != 4 {
		t.Fatalf("total = %d, want 4", got)
	}
}

func TestMergeReembeddingQueueEnqueueDedupesTargetTag(t *testing.T) {
	db := setupMergeReembeddingTestDB(t)
	source, target := seedMergeQueueTags(t, db)
	service := service.NewMergeReembeddingQueueService(nil)

	if err := service.Enqueue(source.ID, target.ID); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if err := service.Enqueue(source.ID, target.ID); err != nil {
		t.Fatalf("second enqueue: %v", err)
	}

	var count int64
	if err := db.Model(&tagmodels.MergeReembeddingQueue{}).Where("target_tag_id = ?", target.ID).Count(&count).Error; err != nil {
		t.Fatalf("count queue tasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("task count = %d, want 1", count)
	}

	if err := db.Model(&tagmodels.MergeReembeddingQueue{}).Where("target_tag_id = ?", target.ID).Update("status", tagmodels.MergeReembeddingQueueStatusProcessing).Error; err != nil {
		t.Fatalf("mark processing: %v", err)
	}
	if err := service.Enqueue(source.ID, target.ID); err != nil {
		t.Fatalf("enqueue with processing task: %v", err)
	}

	if err := db.Model(&tagmodels.MergeReembeddingQueue{}).Where("target_tag_id = ?", target.ID).Count(&count).Error; err != nil {
		t.Fatalf("count queue tasks after processing dedupe: %v", err)
	}
	if count != 1 {
		t.Fatalf("task count with processing dedupe = %d, want 1", count)
	}
}

func TestMergeReembeddingQueueRetryEndpointResetsFailedTasks(t *testing.T) {
	db := setupMergeReembeddingTestDB(t)
	source, target := seedMergeQueueTags(t, db)

	tasks := []tagmodels.MergeReembeddingQueue{
		{SourceTagID: source.ID, TargetTagID: target.ID, Status: tagmodels.MergeReembeddingQueueStatusFailed, ErrorMessage: "boom", RetryCount: 1},
		{SourceTagID: source.ID, TargetTagID: target.ID, Status: tagmodels.MergeReembeddingQueueStatusFailed, ErrorMessage: "pow", RetryCount: 2},
		{SourceTagID: source.ID, TargetTagID: target.ID, Status: tagmodels.MergeReembeddingQueueStatusCompleted},
	}
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatalf("seed retry tasks: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/embedding/merge-reembedding/retry", nil)
	setupMergeQueueRouter().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var failedTasks []tagmodels.MergeReembeddingQueue
	if err := db.Where("id IN ?", []uint{tasks[0].ID, tasks[1].ID}).Order("id ASC").Find(&failedTasks).Error; err != nil {
		t.Fatalf("reload failed tasks: %v", err)
	}

	for _, task := range failedTasks {
		if task.Status != tagmodels.MergeReembeddingQueueStatusPending {
			t.Fatalf("failed task status = %s, want pending", task.Status)
		}
		if task.ErrorMessage != "" {
			t.Fatalf("failed task error_message = %q, want empty", task.ErrorMessage)
		}
		if task.StartedAt != nil || task.CompletedAt != nil {
			t.Fatalf("failed task timestamps should be reset")
		}
	}

	var completed tagmodels.MergeReembeddingQueue
	if err := db.First(&completed, tasks[2].ID).Error; err != nil {
		t.Fatalf("reload completed task: %v", err)
	}
	if completed.Status != tagmodels.MergeReembeddingQueueStatusCompleted {
		t.Fatalf("completed task status = %s, want completed", completed.Status)
	}
}
