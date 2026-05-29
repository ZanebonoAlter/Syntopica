package tagging

import (
	"fmt"
	"strings"
	"testing"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupHardMergeTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err, "open sqlite")

	database.DB = db
	t.Cleanup(func() { database.DB = nil })

	require.NoError(t, db.AutoMigrate(
		&models.TopicTag{},
		&models.TopicTagRelation{},
		&models.ArticleTopicTag{},
		&models.TopicTagEmbedding{},
		&models.Feed{},
		&models.Article{},
	), "migrate test tables")

	return db
}

func seedTag(t *testing.T, db *gorm.DB, slug, source string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{
		Slug:     slug,
		Label:    slug,
		Category: models.TagCategoryKeyword,
		Source:   source,
		Status:   "active",
	}
	require.NoError(t, db.Create(&tag).Error, "create tag "+slug)
	return tag
}

func seedArticle(t *testing.T, db *gorm.DB, feedID uint) models.Article {
	t.Helper()
	a := models.Article{FeedID: feedID}
	require.NoError(t, db.Create(&a).Error, "create article")
	return a
}

func seedFeed(t *testing.T, db *gorm.DB) models.Feed {
	t.Helper()
	f := models.Feed{Title: "test-feed"}
	require.NoError(t, db.Create(&f).Error, "create feed")
	return f
}

func TestHardMergeTags_SelfMerge(t *testing.T) {
	db := setupHardMergeTestDB(t)
	tag := seedTag(t, db, "self", "llm")

	err := HardMergeTags(db, tag.ID, tag.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot merge tag into itself")
}

func TestHardMergeTags_SourceNotFound(t *testing.T) {
	db := setupHardMergeTestDB(t)
	target := seedTag(t, db, "target", "llm")

	err := HardMergeTags(db, 99999, target.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source tag")
}

func TestHardMergeTags_ArticleMigration(t *testing.T) {
	db := setupHardMergeTestDB(t)
	source := seedTag(t, db, "source-art", "llm")
	target := seedTag(t, db, "target-art", "llm")
	feed := seedFeed(t, db)
	art1 := seedArticle(t, db, feed.ID)
	art2 := seedArticle(t, db, feed.ID)

	require.NoError(t, db.Create(&models.ArticleTopicTag{
		ArticleID: art1.ID, TopicTagID: source.ID, Score: 0.9, Source: "llm",
	}).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{
		ArticleID: art2.ID, TopicTagID: source.ID, Score: 0.7, Source: "heuristic",
	}).Error)

	err := HardMergeTags(db, source.ID, target.ID)
	require.NoError(t, err)

	var links []models.ArticleTopicTag
	require.NoError(t, db.Where("topic_tag_id = ?", target.ID).Find(&links).Error)
	assert.Len(t, links, 2, "both article links should be migrated to target")

	var sourceLinks []models.ArticleTopicTag
	require.NoError(t, db.Where("topic_tag_id = ?", source.ID).Find(&sourceLinks).Error)
	assert.Len(t, sourceLinks, 0, "no links should remain on source")
}

func TestHardMergeTags_ArticleMigration_Dedup(t *testing.T) {
	db := setupHardMergeTestDB(t)
	source := seedTag(t, db, "source-dup", "llm")
	target := seedTag(t, db, "target-dup", "llm")
	feed := seedFeed(t, db)
	art := seedArticle(t, db, feed.ID)

	require.NoError(t, db.Create(&models.ArticleTopicTag{
		ArticleID: art.ID, TopicTagID: source.ID, Score: 0.9, Source: "llm",
	}).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{
		ArticleID: art.ID, TopicTagID: target.ID, Score: 0.8, Source: "llm",
	}).Error)

	err := HardMergeTags(db, source.ID, target.ID)
	require.NoError(t, err)

	var links []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ? AND topic_tag_id = ?", art.ID, target.ID).Find(&links).Error)
	assert.Len(t, links, 1, "duplicate link should be removed, not doubled")
}

func TestHardMergeTags_RelationsNotMigrated(t *testing.T) {
	db := setupHardMergeTestDB(t)
	source := seedTag(t, db, "source-rel", "abstract")
	target := seedTag(t, db, "target-rel", "abstract")
	child := seedTag(t, db, "child-rel", "llm")
	parent := seedTag(t, db, "parent-rel", "abstract")

	require.NoError(t, db.Create(&models.TopicTagRelation{
		ParentID: source.ID, ChildID: child.ID, RelationType: "abstract",
	}).Error)
	require.NoError(t, db.Create(&models.TopicTagRelation{
		ParentID: parent.ID, ChildID: source.ID, RelationType: "abstract",
	}).Error)

	err := HardMergeTags(db, source.ID, target.ID)
	require.NoError(t, err)

	var parentRels []models.TopicTagRelation
	require.NoError(t, db.Where("parent_id = ? AND child_id = ?", target.ID, child.ID).Find(&parentRels).Error)
	assert.Len(t, parentRels, 0, "hierarchy migration is deprecated, no relations moved")

	var childRels []models.TopicTagRelation
	require.NoError(t, db.Where("parent_id = ? AND child_id = ?", parent.ID, target.ID).Find(&childRels).Error)
	assert.Len(t, childRels, 0, "hierarchy migration is deprecated, no relations moved")
}

func TestHardMergeTags_SourceDeleted(t *testing.T) {
	db := setupHardMergeTestDB(t)
	source := seedTag(t, db, "source-del", "llm")
	target := seedTag(t, db, "target-del", "llm")

	err := HardMergeTags(db, source.ID, target.ID)
	require.NoError(t, err)

	var count int64
	db.Model(&models.TopicTag{}).Where("id = ?", source.ID).Count(&count)
	assert.Equal(t, int64(0), count, "source tag should be hard-deleted")

	var targetExists models.TopicTag
	require.NoError(t, db.First(&targetExists, target.ID).Error)
	assert.Equal(t, "target-del", targetExists.Slug)
}

func TestHardMergeTags_EmbeddingsDeleted(t *testing.T) {
	db := setupHardMergeTestDB(t)
	source := seedTag(t, db, "source-emb", "llm")
	target := seedTag(t, db, "target-emb", "llm")

	require.NoError(t, db.Create(&models.TopicTagEmbedding{
		TopicTagID: source.ID, Vector: "[0.1,0.2]", Dimension: 2, Model: "test",
	}).Error)
	require.NoError(t, db.Create(&models.TopicTagEmbedding{
		TopicTagID: target.ID, Vector: "[0.3,0.4]", Dimension: 2, Model: "test",
	}).Error)

	err := HardMergeTags(db, source.ID, target.ID)
	require.NoError(t, err)

	var sourceEmbs []models.TopicTagEmbedding
	require.NoError(t, db.Where("topic_tag_id = ?", source.ID).Find(&sourceEmbs).Error)
	assert.Len(t, sourceEmbs, 0, "source embeddings should be deleted")

	var targetEmbs []models.TopicTagEmbedding
	require.NoError(t, db.Where("topic_tag_id = ?", target.ID).Find(&targetEmbs).Error)
	assert.Len(t, targetEmbs, 1, "target embeddings should be preserved")
}
