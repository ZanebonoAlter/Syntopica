package core

import (
	"strings"
	"testing"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

func setupEmbeddingTestDB(t *testing.T) *gorm.DB {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	return db
}

func TestSaveEmbeddingReturnsTagNotFoundWhenParentDeleted(t *testing.T) {
	db := setupEmbeddingTestDB(t)
	service := NewEmbeddingService()

	tag := models.TopicTag{
		Slug:     "deleted-tag",
		Label:    "Deleted Tag",
		Category: models.TagCategoryKeyword,
		Status:   "active",
	}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	if err := db.Delete(&tag).Error; err != nil {
		t.Fatalf("delete tag: %v", err)
	}

	err := service.SaveEmbedding(&tagmodels.TopicTagEmbedding{
		TopicTagID:    tag.ID,
		EmbeddingType: EmbeddingTypeIdentity,
		Model:         "test-model",
		TextHash:      "abc123",
	})
	if err == nil {
		t.Fatal("expected missing parent tag error, got nil")
	}
	if err != ErrTopicTagNotFound {
		t.Fatalf("expected ErrTopicTagNotFound, got %v", err)
	}

	var count int64
	if err := db.Model(&tagmodels.TopicTagEmbedding{}).Count(&count).Error; err != nil {
		t.Fatalf("count embeddings: %v", err)
	}
	if count != 0 {
		t.Fatalf("embedding count = %d, want 0", count)
	}
}

func TestSaveEmbeddingCleansUpStaleRecords(t *testing.T) {
	db := setupEmbeddingTestDB(t)
	service := NewEmbeddingService()

	tag := models.TopicTag{
		Slug:     "test-tag",
		Label:    "Test Tag",
		Category: models.TagCategoryKeyword,
		Status:   "active",
	}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	vec4096 := makeValidVector(4096)
	for i, hash := range []string{"stale-hash-1", "stale-hash-2", "stale-hash-3"} {
		if err := db.Create(&tagmodels.TopicTagEmbedding{
			TopicTagID:    tag.ID,
			EmbeddingType: EmbeddingTypeIdentity,
			EmbeddingVec:  vec4096,
			Dimension:     4096,
			Model:         "test-model",
			TextHash:      hash,
		}).Error; err != nil {
			t.Fatalf("create stale embedding %d: %v", i, err)
		}
	}

	var count int64
	db.Model(&tagmodels.TopicTagEmbedding{}).Where("topic_tag_id = ? AND embedding_type = ?", tag.ID, EmbeddingTypeIdentity).Count(&count)
	if count != 3 {
		t.Fatalf("stale embedding count = %d, want 3 before cleanup", count)
	}

	newHash := "fresh-hash"
	if err := service.SaveEmbedding(&tagmodels.TopicTagEmbedding{
		TopicTagID:    tag.ID,
		EmbeddingType: EmbeddingTypeIdentity,
		EmbeddingVec:  vec4096,
		Dimension:     4096,
		Model:         "test-model",
		TextHash:      newHash,
	}); err != nil {
		t.Fatalf("SaveEmbedding: %v", err)
	}

	db.Model(&tagmodels.TopicTagEmbedding{}).Where("topic_tag_id = ? AND embedding_type = ?", tag.ID, EmbeddingTypeIdentity).Count(&count)
	if count != 1 {
		t.Fatalf("embedding count after cleanup = %d, want 1", count)
	}

	var remaining tagmodels.TopicTagEmbedding
	if err := db.Where("topic_tag_id = ? AND embedding_type = ?", tag.ID, EmbeddingTypeIdentity).First(&remaining).Error; err != nil {
		t.Fatalf("find remaining embedding: %v", err)
	}
	if remaining.TextHash != newHash {
		t.Fatalf("remaining text_hash = %q, want %q", remaining.TextHash, newHash)
	}
}

func TestProcessNextEventKeywordEmbeddings(t *testing.T) {
	db := setupEmbeddingTestDB(t)

	tag := models.TopicTag{
		Label:       "特朗普访华",
		Category:    "event",
		Kind:        "topic",
		Slug:        "trump-visits-china",
		Status:      "active",
		Description: "美国总统特朗普对中国的国事访问",
		Metadata: models.MetadataMap{
			"event_keywords": []interface{}{"特朗普", "中国", "访华", "中美关系"},
		},
	}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	keywords := getEventKeywords(&tag)
	if len(keywords) != 4 {
		t.Fatalf("expected 4 keywords, got %d: %v", len(keywords), keywords)
	}

	expectedKeywords := []string{"特朗普", "中国", "访华", "中美关系"}
	for i, kw := range keywords {
		if kw != expectedKeywords[i] {
			t.Errorf("keyword[%d] = %q, want %q", i, kw, expectedKeywords[i])
		}
	}

	for _, kw := range keywords {
		kwHash := hashText(EmbeddingTypeEventKeyword + "\n" + kw)
		if kwHash == "" {
			t.Errorf("empty hash for keyword %q", kw)
		}
		var count int64
		db.Model(&tagmodels.TopicTagEmbedding{}).
			Where("topic_tag_id = ? AND embedding_type = ? AND text_hash = ?", tag.ID, EmbeddingTypeEventKeyword, kwHash).
			Count(&count)
		if count != 0 {
			t.Errorf("embedding should not exist yet for keyword %q", kw)
		}
	}
}

// makeValidVector creates a pgvector-compatible string with the given dimension.
func makeValidVector(dim int) string {
	vals := make([]string, dim)
	for i := range vals {
		vals[i] = "0.000001"
	}
	return "[" + strings.Join(vals, ",") + "]"
}
