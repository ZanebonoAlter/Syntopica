package tagging

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
)

func setupSectorTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := setupTagCleanupTestDB(t)
	if err := db.AutoMigrate(&models.BoardConcept{}); err != nil {
		t.Fatalf("migrate board_concepts: %v", err)
	}

	return db
}

func TestAutoGenerateSectors_BelowThreshold(t *testing.T) {
	db := setupSectorTestDB(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		tag := models.TopicTag{
			Slug:     fmt.Sprintf("tag-%d", i),
			Label:    fmt.Sprintf("Tag %d", i),
			Category: "event",
			Status:   "active",
		}
		if err := db.Create(&tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}

	err := AutoGenerateSectors(ctx, db, "event", 10)
	if err != nil {
		t.Fatalf("AutoGenerateSectors error: %v", err)
	}

	var conceptCount int64
	db.Model(&models.BoardConcept{}).Count(&conceptCount)
	if conceptCount != 0 {
		t.Errorf("expected 0 concepts created below threshold, got %d", conceptCount)
	}
}

func TestRuntimeColdStartDoesNotTriggerAutoGenerateSectors(t *testing.T) {
	content, err := os.ReadFile("../../app/runtime.go")
	if err != nil {
		t.Fatalf("read runtime.go: %v", err)
	}
	if strings.Contains(string(content), "AutoGenerateSectors") {
		t.Fatal("runtime cold start unexpectedly triggers AutoGenerateSectors")
	}
}

func TestManualCreateSector(t *testing.T) {
	t.Skip("requires LLM API for description generation and embedding")

	_ = ManualCreateSector
}

func TestManualCreateSector_EmptyLabel(t *testing.T) {
	ctx := context.Background()
	_, err := ManualCreateSector(ctx, nil, "event", "", "test description")
	if err == nil {
		t.Fatal("expected error for empty label, got nil")
	}
}

func TestManualCreateSector_WhitespaceLabel(t *testing.T) {
	ctx := context.Background()
	_, err := ManualCreateSector(ctx, nil, "event", "   ", "test description")
	if err == nil {
		t.Fatal("expected error for whitespace-only label, got nil")
	}
}

func TestCheckSectorHealth_AutoEmpty_Deleted(t *testing.T) {
	db := setupSectorTestDB(t)
	ctx := context.Background()

	c := models.BoardConcept{
		Name:     "Auto Sector",
		Category: "event",
		Status:   "active",
		Source:   "auto",
	}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("create concept: %v", err)
	}

	deletedAuto, markedDeclining, err := CheckSectorHealth(ctx, db, "event")
	if err != nil {
		t.Fatalf("CheckSectorHealth error: %v", err)
	}
	if deletedAuto != 1 {
		t.Errorf("expected 1 deleted auto sector, got %d", deletedAuto)
	}
	if markedDeclining != 0 {
		t.Errorf("expected 0 marked declining, got %d", markedDeclining)
	}

	var updated models.BoardConcept
	db.First(&updated, c.ID)
	if updated.Status != "inactive" {
		t.Errorf("expected auto concept status 'inactive', got %q", updated.Status)
	}
}

func TestCheckSectorHealth_ManualEmpty_NotDeleted(t *testing.T) {
	db := setupSectorTestDB(t)
	ctx := context.Background()

	c := models.BoardConcept{
		Name:     "Manual Sector",
		Category: "event",
		Status:   "active",
		Source:   "manual",
	}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("create concept: %v", err)
	}

	deletedAuto, _, err := CheckSectorHealth(ctx, db, "event")
	if err != nil {
		t.Fatalf("CheckSectorHealth error: %v", err)
	}
	if deletedAuto != 0 {
		t.Errorf("expected 0 deleted auto sectors (manual skipped), got %d", deletedAuto)
	}

	var updated models.BoardConcept
	db.First(&updated, c.ID)
	if updated.Status != "active" {
		t.Errorf("expected manual concept to remain 'active', got %q", updated.Status)
	}
}

func TestCheckSectorHealth_AutoWithTags_NotDeleted(t *testing.T) {
	db := setupSectorTestDB(t)
	ctx := context.Background()

	c := models.BoardConcept{
		Name:     "Active Auto Sector",
		Category: "event",
		Status:   "active",
		Source:   "auto",
	}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("create concept: %v", err)
	}

	tag := models.TopicTag{
		Slug:      "linked-tag",
		Label:     "Linked Tag",
		Category:  "event",
		Status:    "active",
		ConceptID: &c.ID,
	}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	deletedAuto, _, err := CheckSectorHealth(ctx, db, "event")
	if err != nil {
		t.Fatalf("CheckSectorHealth error: %v", err)
	}
	if deletedAuto != 0 {
		t.Errorf("expected 0 deleted (auto sector has tags), got %d", deletedAuto)
	}

	var updated models.BoardConcept
	db.First(&updated, c.ID)
	if updated.Status != "active" {
		t.Errorf("expected concept with tags to remain 'active', got %q", updated.Status)
	}
}

func TestParseConceptEmbedding_JSONFormat(t *testing.T) {
	vec, err := parseConceptEmbedding("[1.0, 2.0, 3.0]")
	if err != nil {
		t.Fatalf("parseConceptEmbedding error: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(vec))
	}
	if vec[0] != 1.0 || vec[1] != 2.0 || vec[2] != 3.0 {
		t.Errorf("expected [1,2,3], got %v", vec)
	}
}

func TestParseConceptEmbedding_CSVFormat(t *testing.T) {
	vec, err := parseConceptEmbedding("0.5, 0.3, 0.2")
	if err != nil {
		t.Fatalf("parseConceptEmbedding error: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(vec))
	}
}

func TestParseConceptEmbedding_Empty(t *testing.T) {
	_, err := parseConceptEmbedding("")
	if err == nil {
		t.Fatal("expected error for empty embedding string")
	}
}

func TestBuildAutoGeneratePrompt(t *testing.T) {
	labels := []string{"AI", "半导体", "新能源"}
	prompt := buildAutoGeneratePrompt(labels)
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	for _, l := range labels {
		if !containsSubstring(prompt, l) {
			t.Errorf("prompt missing label %q", l)
		}
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
