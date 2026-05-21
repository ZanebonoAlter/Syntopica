package concept

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func setupSuggestTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	database.DB = db

	if err := db.AutoMigrate(&models.TopicTag{}); err != nil {
		t.Fatalf("migrate test tables: %v", err)
	}

	return db
}

func TestBuildSuggestPrompt(t *testing.T) {
	tags := []models.TopicTag{
		{Label: "特朗普访华", Description: "描述A"},
		{Label: "中美贸易战", Description: ""},
	}
	prompt := buildSuggestPrompt(tags)

	if !strings.Contains(prompt, "特朗普访华") {
		t.Error("prompt should contain first tag label")
	}
	if !strings.Contains(prompt, "描述A") {
		t.Error("prompt should contain first tag description")
	}
	if !strings.Contains(prompt, "中美贸易战") {
		t.Error("prompt should contain second tag label")
	}
	if strings.Contains(prompt, "建议3-5个板块概念") {
		// Should contain the range
	} else {
		t.Error("prompt should mention 3-5 suggestions")
	}
	if !strings.Contains(prompt, "suggestions") {
		t.Error("prompt should mention suggestions JSON field")
	}
}

func TestSuggestConcepts_EmptyCategory(t *testing.T) {
	_ = setupSuggestTestDB(t)

	suggestions, err := SuggestConcepts(context.Background(), "person")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 0 {
		t.Errorf("expected empty suggestions for empty category, got %d", len(suggestions))
	}
}

func TestSuggestConcepts_FewTags(t *testing.T) {
	db := setupSuggestTestDB(t)

	for i := range 5 {
		tag := models.TopicTag{
			Slug:     fmt.Sprintf("tag-%d", i),
			Label:    fmt.Sprintf("标签%d", i),
			Category: "person",
			Status:   "active",
		}
		if err := db.Create(&tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}

	suggestions, err := SuggestConcepts(context.Background(), "person")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 0 {
		t.Errorf("expected empty suggestions with only 5 tags, got %d", len(suggestions))
	}
}

func TestSuggestConcepts_WithLLMResponse(t *testing.T) {
	db := setupSuggestTestDB(t)

	for i := range 15 {
		tag := models.TopicTag{
			Slug:     fmt.Sprintf("tag-%d", i),
			Label:    fmt.Sprintf("标签%d", i),
			Category: "event",
			Status:   "active",
		}
		if err := db.Create(&tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}

	origLLM := callSuggestLLMFn
	callSuggestLLMFn = func(_ context.Context, _ string) (string, error) {
		return `{"suggestions":[{"name":"中美贸易","description":"关于中美贸易谈判、关税政策的讨论"},{"name":"AI监管","description":"全球AI法规和监管政策动态"}]}`, nil
	}
	t.Cleanup(func() { callSuggestLLMFn = origLLM })

	suggestions, err := SuggestConcepts(context.Background(), "event")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) == 0 {
		t.Fatal("expected non-empty suggestions")
	}
	if len(suggestions) > 2 {
		t.Errorf("expected at most 2 suggestions, got %d", len(suggestions))
	}
	if suggestions[0].Name == "" {
		t.Error("first suggestion name should not be empty")
	}
}

func TestSuggestConcepts_LLMFailure(t *testing.T) {
	db := setupSuggestTestDB(t)

	for i := range 15 {
		tag := models.TopicTag{
			Slug:     fmt.Sprintf("tag-%d", i),
			Label:    fmt.Sprintf("标签%d", i),
			Category: "event",
			Status:   "active",
		}
		if err := db.Create(&tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}

	origLLM := callSuggestLLMFn
	callSuggestLLMFn = func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("network error")
	}
	t.Cleanup(func() { callSuggestLLMFn = origLLM })

	suggestions, err := SuggestConcepts(context.Background(), "event")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 0 {
		t.Errorf("expected empty suggestions on LLM failure, got %d", len(suggestions))
	}
}

func TestSuggestConcepts_UnparseableJSON(t *testing.T) {
	db := setupSuggestTestDB(t)

	for i := range 15 {
		tag := models.TopicTag{
			Slug:     fmt.Sprintf("tag-%d", i),
			Label:    fmt.Sprintf("标签%d", i),
			Category: "event",
			Status:   "active",
		}
		if err := db.Create(&tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}

	origLLM := callSuggestLLMFn
	callSuggestLLMFn = func(_ context.Context, _ string) (string, error) {
		return "not valid json at all", nil
	}
	t.Cleanup(func() { callSuggestLLMFn = origLLM })

	suggestions, err := SuggestConcepts(context.Background(), "event")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 0 {
		t.Errorf("expected empty suggestions on unparseable JSON, got %d", len(suggestions))
	}
}

func TestLoadUnassignedTags_FiltersCorrectly(t *testing.T) {
	db := setupSuggestTestDB(t)

	activeEvent := models.TopicTag{Slug: "active-event", Label: "活跃事件", Category: "event", Status: "active"}
	mergedEvent := models.TopicTag{Slug: "merged-event", Label: "已合并", Category: "event", Status: "merged"}
	activePerson := models.TopicTag{Slug: "active-person", Label: "活跃人物", Category: "person", Status: "active"}
	_ = db.Create(&activeEvent).Error
	_ = db.Create(&mergedEvent).Error
	_ = db.Create(&activePerson).Error

	tags, err := loadUnassignedTags("event")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 active event tag, got %d", len(tags))
	}
	if tags[0].Label != "活跃事件" {
		t.Errorf("expected '活跃事件', got %q", tags[0].Label)
	}
}
