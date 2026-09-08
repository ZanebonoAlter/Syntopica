package board

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/repository"
)

func setupClusteringTestDB(t *testing.T) *gorm.DB {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	InvalidateBoardCache() // 避免包级缓存跨测试残留
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

func TestFindSimilarTagsByKeywordOverlap_NoEventKeywords(t *testing.T) {
	db := setupClusteringTestDB(t)
	if db.Name() != "postgres" {
		t.Skip("keyword overlap query requires PostgreSQL jsonb functions")
	}

	tag1 := models.TopicTag{Label: "Tag A", Slug: "tag-a", Category: "event"}
	tag2 := models.TopicTag{Label: "Tag B", Slug: "tag-b", Category: "event"}
	if err := repository.Repo.DB().Create(&tag1).Error; err != nil {
		t.Fatalf("create tag1: %v", err)
	}
	if err := repository.Repo.DB().Create(&tag2).Error; err != nil {
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
