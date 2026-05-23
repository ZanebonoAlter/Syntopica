package tagging

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

func setupClusteringTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	database.DB = db

	if err := db.AutoMigrate(&models.TopicTag{}, &models.TopicTagEmbedding{}); err != nil {
		t.Fatalf("migrate test tables: %v", err)
	}

	return db
}

func TestFindSimilarTagsByKeywordOverlap_EmptyInput(t *testing.T) {
	_ = setupClusteringTestDB(t)

	kwEdges, semEdges, err := FindSimilarTagsByKeywordOverlap(context.Background(), nil, 2, 0.80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kwEdges != nil {
		t.Fatalf("expected nil kwEdges for empty input, got %d", len(kwEdges))
	}
	if semEdges != nil {
		t.Fatalf("expected nil semEdges for empty input, got %d", len(semEdges))
	}
}

func TestFindSimilarTagsByKeywordOverlap_SingleTag(t *testing.T) {
	_ = setupClusteringTestDB(t)

	kwEdges, semEdges, err := FindSimilarTagsByKeywordOverlap(context.Background(), []uint{1}, 2, 0.80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kwEdges != nil {
		t.Fatalf("expected nil kwEdges for single tag, got %d", len(kwEdges))
	}
	if semEdges != nil {
		t.Fatalf("expected nil semEdges for single tag, got %d", len(semEdges))
	}
}

func TestLoadClusterConfig_Defaults(t *testing.T) {
	cfg := DefaultClusterConfig

	if cfg.KwMinOverlap != 2 {
		t.Fatalf("default KwMinOverlap = %d, want 2", cfg.KwMinOverlap)
	}
	if cfg.SemThreshold != 0.80 {
		t.Fatalf("default SemThreshold = %.2f, want 0.80", cfg.SemThreshold)
	}
	if cfg.MaxTags != 500 {
		t.Fatalf("default MaxTags = %d, want 500", cfg.MaxTags)
	}
	if cfg.SimilarityThreshold != 0.85 {
		t.Fatalf("default SimilarityThreshold = %.2f, want 0.85", cfg.SimilarityThreshold)
	}
	if cfg.MaxClusterSize != 8 {
		t.Fatalf("default MaxClusterSize = %d, want 8", cfg.MaxClusterSize)
	}
}

func TestFindSimilarTagsByKeywordOverlap_NoEventKeywords(t *testing.T) {
	db := setupClusteringTestDB(t)
	if db.Name() != "postgres" {
		t.Skip("keyword overlap query requires PostgreSQL jsonb functions")
	}

	tag1 := models.TopicTag{Label: "Tag A", Slug: "tag-a", Category: "event"}
	tag2 := models.TopicTag{Label: "Tag B", Slug: "tag-b", Category: "event"}
	if err := database.DB.Create(&tag1).Error; err != nil {
		t.Fatalf("create tag1: %v", err)
	}
	if err := database.DB.Create(&tag2).Error; err != nil {
		t.Fatalf("create tag2: %v", err)
	}

	kwEdges, semEdges, err := FindSimilarTagsByKeywordOverlap(context.Background(), []uint{tag1.ID, tag2.ID}, 2, 0.80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kwEdges) != 0 {
		t.Fatalf("expected 0 keyword pairs for tags without event_keywords, got %d", len(kwEdges))
	}
	if len(semEdges) != 0 {
		t.Fatalf("expected 0 semantic edges for tags without event_keywords, got %d", len(semEdges))
	}
}

func TestFindConnectedComponents_SinglePair(t *testing.T) {
	edges := []SimilarityEdge{
		{TagAID: 1, TagBID: 2, Similarity: 0.85},
	}
	comp := findConnectedComponents([]uint{1, 2}, edges)
	if len(comp) != 1 {
		t.Fatalf("expected 1 component, got %d", len(comp))
	}
	if len(comp[0]) != 2 {
		t.Fatalf("expected component size 2, got %d", len(comp[0]))
	}
}

func TestFindConnectedComponents_Chain(t *testing.T) {
	edges := []SimilarityEdge{
		{TagAID: 1, TagBID: 2, Similarity: 0.85},
		{TagAID: 2, TagBID: 3, Similarity: 0.82},
	}
	comp := findConnectedComponents([]uint{1, 2, 3}, edges)
	if len(comp) != 1 {
		t.Fatalf("expected 1 component, got %d", len(comp))
	}
	if len(comp[0]) != 3 {
		t.Fatalf("expected component size 3, got %d", len(comp[0]))
	}
}

func TestFindConnectedComponents_Disconnected(t *testing.T) {
	edges := []SimilarityEdge{
		{TagAID: 1, TagBID: 2, Similarity: 0.85},
		{TagAID: 3, TagBID: 4, Similarity: 0.90},
	}
	comp := findConnectedComponents([]uint{1, 2, 3, 4}, edges)
	if len(comp) != 2 {
		t.Fatalf("expected 2 components, got %d", len(comp))
	}
}

func TestFindConnectedComponents_SingleNode(t *testing.T) {
	edges := []SimilarityEdge{
		{TagAID: 1, TagBID: 2, Similarity: 0.85},
	}
	comp := findConnectedComponents([]uint{1, 2, 3}, edges)
	if len(comp) != 1 {
		t.Fatalf("expected 1 component (single node excluded), got %d", len(comp))
	}
}

func TestFindConnectedComponents_Empty(t *testing.T) {
	comp := findConnectedComponents(nil, nil)
	if len(comp) != 0 {
		t.Fatalf("expected 0 components, got %d", len(comp))
	}
}
