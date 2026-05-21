package tagging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func TestApprovePendingChangeDeletesExplicitRelation(t *testing.T) {
	db := setupPendingApprovalTestDB(t)
	parent, child := createPendingApprovalTags(t, db, "parent", "child")
	if err := db.Create(&models.TopicTagRelation{ParentID: parent.ID, ChildID: child.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}
	change := models.HierarchyPendingChange{
		TagID:              child.ID,
		TagLabel:           child.Label,
		ChangeType:         "depth_exceeded",
		CurrentParentID:    &parent.ID,
		CurrentParentLabel: parent.Label,
		Reason:             "depth too large",
		Status:             "pending",
	}
	if err := db.Create(&change).Error; err != nil {
		t.Fatalf("create pending change: %v", err)
	}

	body := fmt.Sprintf(`{"ids":[%d]}`, change.ID)
	response := postApprovePendingChanges(t, body)

	data := response["data"].(map[string]any)
	if data["approved"] != float64(1) || data["failed"] != float64(0) {
		t.Fatalf("expected approved=1 failed=0, got %#v", data)
	}
	var relationCount int64
	if err := db.Model(&models.TopicTagRelation{}).Where("parent_id = ? AND child_id = ?", parent.ID, child.ID).Count(&relationCount).Error; err != nil {
		t.Fatalf("count relation: %v", err)
	}
	if relationCount != 0 {
		t.Fatalf("expected relation deleted, got %d", relationCount)
	}
	var stored models.HierarchyPendingChange
	if err := db.First(&stored, change.ID).Error; err != nil {
		t.Fatalf("load change: %v", err)
	}
	if stored.Status != "resolved" {
		t.Fatalf("expected resolved status, got %q", stored.Status)
	}
}

func TestApprovePendingChangesReportsMissingPayloadPartialFailure(t *testing.T) {
	db := setupPendingApprovalTestDB(t)
	parent, child := createPendingApprovalTags(t, db, "parent", "child")
	_, orphan := createPendingApprovalTags(t, db, "other-parent", "orphan")
	if err := db.Create(&models.TopicTagRelation{ParentID: parent.ID, ChildID: child.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}
	good := models.HierarchyPendingChange{
		TagID:           child.ID,
		TagLabel:        child.Label,
		ChangeType:      "cross_category",
		CurrentParentID: &parent.ID,
		Reason:          "wrong parent category",
		Status:          "pending",
	}
	bad := models.HierarchyPendingChange{
		TagID:      orphan.ID,
		TagLabel:   orphan.Label,
		ChangeType: "reparent",
		Reason:     "missing target parent payload",
		Status:     "pending",
	}
	if err := db.Create(&[]models.HierarchyPendingChange{good, bad}).Error; err != nil {
		t.Fatalf("create pending changes: %v", err)
	}

	response := postApprovePendingChanges(t, `{"approve_all":true,"category":"event"}`)

	data := response["data"].(map[string]any)
	if data["approved"] != float64(1) || data["failed"] != float64(1) {
		t.Fatalf("expected approved=1 failed=1, got %#v", data)
	}
	results := data["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("expected two result rows, got %#v", results)
	}
	var failedResult map[string]any
	for _, raw := range results {
		item := raw.(map[string]any)
		if item["status"] == "failed" {
			failedResult = item
			break
		}
	}
	if failedResult == nil || !strings.Contains(failedResult["reason"].(string), "target parent payload") {
		t.Fatalf("expected missing payload failure result, got %#v", results)
	}
}

func setupPendingApprovalTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.TopicTag{}, &models.TopicTagRelation{}, &models.HierarchyPendingChange{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	database.DB = db
	t.Cleanup(func() { database.DB = nil })
	return db
}

func createPendingApprovalTags(t *testing.T, db *gorm.DB, parentSlug, childSlug string) (models.TopicTag, models.TopicTag) {
	t.Helper()
	parent := models.TopicTag{Slug: parentSlug, Label: parentSlug, Category: "event", Status: "active"}
	child := models.TopicTag{Slug: childSlug, Label: childSlug, Category: "event", Status: "active"}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent tag: %v", err)
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child tag: %v", err)
	}
	return parent, child
}

func postApprovePendingChanges(t *testing.T, body string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/pending/approve", approvePendingChangesHandler)
	req := httptest.NewRequest(http.MethodPost, "/pending/approve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", w.Code, w.Body.String())
	}
	var response map[string]any
	if err := json.NewDecoder(bytes.NewReader(w.Body.Bytes())).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["success"] != true {
		t.Fatalf("expected success true, got %#v", response)
	}
	return response
}
