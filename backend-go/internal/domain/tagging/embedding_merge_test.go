package tagging

import (
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func setupMergeTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)

	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		database.DB = nil
	})

	require.NoError(t, db.AutoMigrate(&models.TopicTag{}, &models.TopicTagRelation{}))

	return db
}

func TestMigrateTagRelations_RejectsCycle(t *testing.T) {
	db := setupMergeTestDB(t)

	root := models.TopicTag{Slug: "root", Label: "Root", Kind: "event", Category: "event", Source: "abstract", Status: "active"}
	mid := models.TopicTag{Slug: "mid", Label: "Mid", Kind: "event", Category: "event", Source: "abstract", Status: "active"}
	leaf := models.TopicTag{Slug: "leaf", Label: "Leaf", Kind: "event", Category: "event", Source: "abstract", Status: "active"}

	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&mid).Error)
	require.NoError(t, db.Create(&leaf).Error)

	require.NoError(t, db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: mid.ID, RelationType: "abstract"}).Error)
	require.NoError(t, db.Create(&models.TopicTagRelation{ParentID: mid.ID, ChildID: leaf.ID, RelationType: "abstract"}).Error)
	cycleEdge := models.TopicTagRelation{ParentID: leaf.ID, ChildID: root.ID, RelationType: "abstract"}
	require.NoError(t, db.Create(&cycleEdge).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		return migrateTagRelations(tx, leaf.ID, mid.ID)
	})
	require.NoError(t, err)

	assertAbstractRelationExists(t, db, root.ID, mid.ID)

	assertAbstractRelationMissing(t, db, mid.ID, leaf.ID)

	assertAbstractRelationMissing(t, db, mid.ID, root.ID)

	var count int64
	db.Model(&models.TopicTagRelation{}).Where("parent_id = ? AND child_id = ?", leaf.ID, root.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestMigrateTagRelations_SkipsNonAbstractTarget(t *testing.T) {
	db := setupMergeTestDB(t)

	source := models.TopicTag{Slug: "abstract-parent", Label: "Abstract Parent", Kind: "event", Category: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Slug: "normal-child", Label: "Normal Child", Kind: "keyword", Category: "keyword", Source: "llm", Status: "active"}
	target := models.TopicTag{Slug: "normal-target", Label: "Normal Target", Kind: "keyword", Category: "keyword", Source: "llm", Status: "active"}

	require.NoError(t, db.Create(&source).Error)
	require.NoError(t, db.Create(&child).Error)
	require.NoError(t, db.Create(&target).Error)

	require.NoError(t, db.Create(&models.TopicTagRelation{ParentID: source.ID, ChildID: child.ID, RelationType: "abstract"}).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		return migrateTagRelations(tx, source.ID, target.ID)
	})
	require.NoError(t, err)

	assertAbstractRelationMissing(t, db, target.ID, child.ID)

	var count int64
	db.Model(&models.TopicTagRelation{}).Where("parent_id = ? AND child_id = ?", source.ID, child.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestMigrateTagRelations_MigratesToAbstractTarget(t *testing.T) {
	db := setupMergeTestDB(t)

	source := models.TopicTag{Slug: "abstract-source", Label: "Abstract Source", Kind: "event", Category: "event", Source: "abstract", Status: "active"}
	target := models.TopicTag{Slug: "abstract-target", Label: "Abstract Target", Kind: "event", Category: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Slug: "normal-child", Label: "Normal Child", Kind: "keyword", Category: "keyword", Source: "llm", Status: "active"}

	require.NoError(t, db.Create(&source).Error)
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&child).Error)

	require.NoError(t, db.Create(&models.TopicTagRelation{ParentID: source.ID, ChildID: child.ID, RelationType: "abstract"}).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		return migrateTagRelations(tx, source.ID, target.ID)
	})
	require.NoError(t, err)

	assertAbstractRelationExists(t, db, target.ID, child.ID)

	var count int64
	db.Model(&models.TopicTagRelation{}).Where("parent_id = ? AND child_id = ?", source.ID, child.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestMigrateTagRelations_RejectsInverseCycle(t *testing.T) {
	db := setupMergeTestDB(t)

	grandparent := models.TopicTag{Slug: "gp", Label: "Grandparent", Kind: "event", Category: "event", Source: "abstract", Status: "active"}
	parent := models.TopicTag{Slug: "p", Label: "Parent", Kind: "event", Category: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Slug: "c", Label: "Child", Kind: "event", Category: "event", Source: "abstract", Status: "active"}

	require.NoError(t, db.Create(&grandparent).Error)
	require.NoError(t, db.Create(&parent).Error)
	require.NoError(t, db.Create(&child).Error)

	require.NoError(t, db.Create(&models.TopicTagRelation{ParentID: grandparent.ID, ChildID: parent.ID, RelationType: "abstract"}).Error)
	require.NoError(t, db.Create(&models.TopicTagRelation{ParentID: parent.ID, ChildID: child.ID, RelationType: "abstract"}).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		return migrateTagRelations(tx, child.ID, grandparent.ID)
	})
	require.NoError(t, err)

	assertAbstractRelationExists(t, db, grandparent.ID, parent.ID)

	assertAbstractRelationMissing(t, db, grandparent.ID, child.ID)

	assertAbstractRelationMissing(t, db, parent.ID, grandparent.ID)
}

func TestMigrateTagRelations_ChildRoleMigration(t *testing.T) {
	db := setupMergeTestDB(t)

	grandparent := models.TopicTag{Slug: "gp", Label: "Grandparent", Kind: "event", Category: "event", Source: "abstract", Status: "active"}
	parent := models.TopicTag{Slug: "p", Label: "Parent", Kind: "event", Category: "event", Source: "abstract", Status: "active"}
	target := models.TopicTag{Slug: "t", Label: "Target", Kind: "event", Category: "event", Source: "abstract", Status: "active"}

	require.NoError(t, db.Create(&grandparent).Error)
	require.NoError(t, db.Create(&parent).Error)
	require.NoError(t, db.Create(&target).Error)

	require.NoError(t, db.Create(&models.TopicTagRelation{ParentID: grandparent.ID, ChildID: parent.ID, RelationType: "abstract"}).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		return migrateTagRelations(tx, parent.ID, target.ID)
	})
	require.NoError(t, err)

	assertAbstractRelationExists(t, db, grandparent.ID, target.ID)

	assertAbstractRelationMissing(t, db, grandparent.ID, parent.ID)
}
