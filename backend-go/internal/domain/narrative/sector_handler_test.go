package narrative

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

func TestConfirmRegenerateSectorsReturnsExecutionResults(t *testing.T) {
	setupSectorHandlerTestDB(t)
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.POST("/sectors/regenerate/confirm", confirmRegenerateSectorsHandler)
	body := strings.NewReader(`{"category":"event","diff":{"keep":[],"add":[],"merge":[{"source_ids":[1],"target_id":999,"name":"missing target"}],"split":[],"affected_tag_count":0}}`)
	req := httptest.NewRequest(http.MethodPost, "/sectors/regenerate/confirm", body)
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
		t.Fatalf("expected success true, got %#v", response["success"])
	}
	if _, ok := response["message"]; ok {
		t.Fatal("expected no top-level message")
	}
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected execution result data, got %#v", response["data"])
	}
	if data["success_count"] != float64(0) {
		t.Fatalf("expected success_count 0, got %#v", data["success_count"])
	}
	if data["failed_count"] != float64(1) {
		t.Fatalf("expected failed_count 1, got %#v", data["failed_count"])
	}
	results, ok := data["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one item result, got %#v", data["results"])
	}
	item, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("expected item map, got %#v", results[0])
	}
	if item["operation"] != "merge" || item["status"] != "failed" {
		t.Fatalf("expected failed merge result, got %#v", item)
	}
	if item["error"] == "" {
		t.Fatalf("expected item error, got %#v", item)
	}
}

func setupSectorHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.BoardConcept{}, &models.TopicTag{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() { database.DB = nil })
	return db
}
