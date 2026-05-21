package tagging

import (
	"fmt"
	"testing"
	"time"

	"my-robot-backend/internal/domain/models"
)

func TestCleanupZombieTagsV2(t *testing.T) {
	db := setupTagCleanupTestDB(t)

	tag := models.TopicTag{
		Slug:      "old-zombie",
		Label:     "Old Zombie Tag",
		Category:  "event",
		Status:    "active",
		CreatedAt: time.Now().AddDate(0, 0, -10),
	}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	deleted, err := CleanupZombieTagsV2(db, "event")
	if err != nil {
		t.Fatalf("CleanupZombieTagsV2 error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	var result models.TopicTag
	err = db.First(&result, tag.ID).Error
	if err == nil {
		t.Error("expected zombie tag to be hard-deleted, but record still exists")
	}
}

func TestCleanupZombieTagsV2_RecentPreserved(t *testing.T) {
	db := setupTagCleanupTestDB(t)

	tag := models.TopicTag{
		Slug:      "recent-tag",
		Label:     "Recent Tag",
		Category:  "event",
		Status:    "active",
		CreatedAt: time.Now().AddDate(0, 0, -3),
	}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	deleted, err := CleanupZombieTagsV2(db, "event")
	if err != nil {
		t.Fatalf("CleanupZombieTagsV2 error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted (recent tag preserved), got %d", deleted)
	}

	var result models.TopicTag
	if err := db.First(&result, tag.ID).Error; err != nil {
		t.Fatal("expected recent tag to be preserved in DB")
	}
	if result.Status != "active" {
		t.Errorf("expected status 'active', got %q", result.Status)
	}
}

func TestCleanupZombieTagsV2_WithArticlePreserved(t *testing.T) {
	db := setupTagCleanupTestDB(t)

	feed := models.Feed{Title: "Test Feed", URL: "https://example.com/z"}
	db.Create(&feed)

	tag := models.TopicTag{
		Slug:      "tag-with-article",
		Label:     "Tag With Article",
		Category:  "event",
		Status:    "active",
		CreatedAt: time.Now().AddDate(0, 0, -10),
	}
	db.Create(&tag)

	article := models.Article{FeedID: feed.ID, Title: "Article"}
	db.Create(&article)
	db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID, Source: "llm"})

	deleted, err := CleanupZombieTagsV2(db, "event")
	if err != nil {
		t.Fatalf("CleanupZombieTagsV2 error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted (tag has article), got %d", deleted)
	}
}

func TestCleanupLowQualityTagsV2(t *testing.T) {
	db := setupTagCleanupTestDB(t)

	feed := models.Feed{Title: "Test Feed", URL: "https://example.com/lq"}
	db.Create(&feed)

	tag := models.TopicTag{
		Slug:         "low-quality",
		Label:        "Low Quality Tag",
		Category:     "event",
		Status:       "active",
		Source:       "llm",
		QualityScore: 0.1,
	}
	db.Create(&tag)

	article := models.Article{FeedID: feed.ID, Title: "Only Article"}
	db.Create(&article)
	db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID, Source: "llm"})

	deleted, err := CleanupLowQualityTagsV2(db, "event")
	if err != nil {
		t.Fatalf("CleanupLowQualityTagsV2 error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	var result models.TopicTag
	err = db.First(&result, tag.ID).Error
	if err == nil {
		t.Error("expected low-quality tag to be hard-deleted")
	}
}

func TestCleanupLowQualityTagsV2_MultiArticle(t *testing.T) {
	db := setupTagCleanupTestDB(t)

	feed := models.Feed{Title: "Test Feed", URL: "https://example.com/multi"}
	db.Create(&feed)

	tag := models.TopicTag{
		Slug:         "multi-article",
		Label:        "Multi Article Tag",
		Category:     "event",
		Status:       "active",
		Source:       "llm",
		QualityScore: 0.1,
	}
	db.Create(&tag)

	for i := 0; i < 2; i++ {
		article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("Article %d", i)}
		db.Create(&article)
		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID, Source: "llm"})
	}

	deleted, err := CleanupLowQualityTagsV2(db, "event")
	if err != nil {
		t.Fatalf("CleanupLowQualityTagsV2 error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted (multi-article preserved), got %d", deleted)
	}

	var result models.TopicTag
	if err := db.First(&result, tag.ID).Error; err != nil {
		t.Fatal("expected multi-article tag to be preserved")
	}
}

func TestCleanupLowQualityTagsV2_AbstractPreserved(t *testing.T) {
	db := setupTagCleanupTestDB(t)

	tag := models.TopicTag{
		Slug:         "abstract-node",
		Label:        "Abstract Node",
		Category:     "event",
		Status:       "active",
		Source:       "abstract",
		Kind:         "abstract",
		QualityScore: 0.1,
	}
	db.Create(&tag)

	feed := models.Feed{Title: "Test Feed", URL: "https://example.com/abs"}
	db.Create(&feed)
	article := models.Article{FeedID: feed.ID, Title: "Article"}
	db.Create(&article)
	db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID, Source: "llm"})

	deleted, err := CleanupLowQualityTagsV2(db, "event")
	if err != nil {
		t.Fatalf("CleanupLowQualityTagsV2 error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted (abstract source excluded), got %d", deleted)
	}
}

func TestCleanupEmptyNodesV2(t *testing.T) {
	db := setupTagCleanupTestDB(t)

	tag := models.TopicTag{
		Slug:     "empty-node",
		Label:    "Empty Abstract Node",
		Category: "keyword",
		Status:   "active",
		Source:   "abstract",
	}
	db.Create(&tag)

	deleted, err := CleanupEmptyNodesV2(db, "keyword")
	if err != nil {
		t.Fatalf("CleanupEmptyNodesV2 error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	var result models.TopicTag
	err = db.First(&result, tag.ID).Error
	if err == nil {
		t.Error("expected empty abstract node to be hard-deleted")
	}
}

func TestCleanupEmptyNodesV2_HasChildren(t *testing.T) {
	db := setupTagCleanupTestDB(t)

	parent := models.TopicTag{
		Slug:     "parent-node",
		Label:    "Parent Abstract Node",
		Category: "keyword",
		Status:   "active",
		Source:   "abstract",
	}
	db.Create(&parent)

	child := models.TopicTag{
		Slug:     "child-tag",
		Label:    "Child Tag",
		Category: "keyword",
		Status:   "active",
	}
	db.Create(&child)

	db.Create(&models.TopicTagRelation{
		ParentID:     parent.ID,
		ChildID:      child.ID,
		RelationType: "abstract",
	})

	deleted, err := CleanupEmptyNodesV2(db, "keyword")
	if err != nil {
		t.Fatalf("CleanupEmptyNodesV2 error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted (parent has children), got %d", deleted)
	}

	var result models.TopicTag
	if err := db.First(&result, parent.ID).Error; err != nil {
		t.Fatal("expected parent node to be preserved")
	}
}

func TestCleanupEmptyNodesV2_NonAbstractPreserved(t *testing.T) {
	db := setupTagCleanupTestDB(t)

	tag := models.TopicTag{
		Slug:     "llm-tag",
		Label:    "LLM Tag",
		Category: "keyword",
		Status:   "active",
		Source:   "llm",
	}
	db.Create(&tag)

	deleted, err := CleanupEmptyNodesV2(db, "keyword")
	if err != nil {
		t.Fatalf("CleanupEmptyNodesV2 error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted (non-abstract source), got %d", deleted)
	}
}

func TestHardDeleteTags_RemovesEmbeddingsAndRelations(t *testing.T) {
	db := setupTagCleanupTestDB(t)

	tag := models.TopicTag{
		Slug:     "doomed-tag",
		Label:    "Doomed Tag",
		Category: "event",
		Status:   "active",
	}
	db.Create(&tag)

	db.Create(&models.TopicTagEmbedding{TopicTagID: tag.ID, EmbeddingType: "identity"})
	db.Create(&models.TopicTagRelation{ParentID: tag.ID, ChildID: 999, RelationType: "abstract"})

	if err := hardDeleteTags(db, []uint{tag.ID}); err != nil {
		t.Fatalf("hardDeleteTags error: %v", err)
	}

	var tagCount int64
	db.Model(&models.TopicTag{}).Where("id = ?", tag.ID).Count(&tagCount)
	if tagCount != 0 {
		t.Errorf("expected tag to be deleted, count=%d", tagCount)
	}

	var embCount int64
	db.Model(&models.TopicTagEmbedding{}).Where("topic_tag_id = ?", tag.ID).Count(&embCount)
	if embCount != 0 {
		t.Errorf("expected embedding to be deleted, count=%d", embCount)
	}

	var relCount int64
	db.Model(&models.TopicTagRelation{}).Where("parent_id = ? AND relation_type = ?", tag.ID, "abstract").Count(&relCount)
	if relCount != 0 {
		t.Errorf("expected relation to be deleted, count=%d", relCount)
	}
}

func TestAnchorSignalCleanupRemovesExpiredSignals(t *testing.T) {
	db := setupTagCleanupTestDB(t)

	active := models.HierarchyAnchorSignal{Category: "keyword", CenterTagID: 1, MemberTagIDs: []uint{1, 2}, ExpiresAt: time.Now().Add(time.Hour)}
	expired := models.HierarchyAnchorSignal{Category: "keyword", CenterTagID: 2, MemberTagIDs: []uint{2, 3}, ExpiresAt: time.Now().Add(-time.Hour)}
	if err := db.Create(&[]models.HierarchyAnchorSignal{active, expired}).Error; err != nil {
		t.Fatalf("create signals: %v", err)
	}

	if err := cleanupExpiredAnchorSignals(db); err != nil {
		t.Fatalf("cleanup expired signals: %v", err)
	}

	var count int64
	db.Model(&models.HierarchyAnchorSignal{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected one active signal, got %d", count)
	}
}
