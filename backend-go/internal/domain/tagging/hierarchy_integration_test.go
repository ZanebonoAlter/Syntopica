package tagging

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func setupIntegrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		database.DB = nil
	})

	if err := db.AutoMigrate(
		&models.TopicTag{},
		&models.TopicTagRelation{},
		&models.ArticleTopicTag{},
		&models.TopicTagEmbedding{},
		&models.EmbeddingConfig{},
		&models.HierarchyConfig{},
		&models.HierarchyPendingChange{},
		&models.HierarchyConfigVersion{},
		&models.MergeReembeddingQueue{},
		&models.Feed{},
		&models.Article{},
	); err != nil {
		t.Fatalf("migrate test tables: %v", err)
	}

	return db
}

func TestIntegration_TemplateConstraintsHold(t *testing.T) {
	db := setupIntegrationTestDB(t)
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()

	allTemplates := BuildAllDefaultTemplates()
	for _, tmpl := range allTemplates {
		t.Run(tmpl.TemplateKey(), func(t *testing.T) {
			if tmpl.MaxLevel <= 0 {
				t.Errorf("MaxLevel = %d, want > 0", tmpl.MaxLevel)
			}
			if len(tmpl.Levels) != tmpl.MaxLevel {
				t.Errorf("Levels count = %d, want MaxLevel = %d", len(tmpl.Levels), tmpl.MaxLevel)
			}
			hasLeaf := false
			for _, level := range tmpl.Levels {
				if level.IsLeaf {
					hasLeaf = true
					break
				}
			}
			if !hasLeaf {
				t.Error("template has no leaf level")
			}
			if tmpl.GetLeafLevel() != tmpl.MaxLevel {
				t.Errorf("GetLeafLevel = %d, want MaxLevel = %d", tmpl.GetLeafLevel(), tmpl.MaxLevel)
			}
		})
	}

	_ = db
}

func TestIntegration_PersonTemplate_TwoLevelTree(t *testing.T) {
	db := setupIntegrationTestDB(t)
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()
	tmpl := mgr.GetTemplate("person", "")
	if tmpl == nil {
		t.Fatal("person template not loaded")
	}

	l1 := models.TopicTag{Label: "体育人物", Slug: "sports-person", Category: "person", Kind: "person", Source: "abstract", Status: "active"}
	l2 := models.TopicTag{Label: "李宗伟", Slug: "lee-chong-wei", Category: "person", Kind: "person", Source: "abstract", Status: "active"}
	db.Create(&l1)
	db.Create(&l2)
	db.Create(&models.TopicTagRelation{ParentID: l1.ID, ChildID: l2.ID, RelationType: "abstract", CreatedAt: time.Now()})

	forest, err := BuildTagForest("person", 2)
	if err != nil {
		t.Fatalf("BuildTagForest: %v", err)
	}
	if len(forest) != 1 {
		t.Fatalf("expected 1 tree, got %d", len(forest))
	}
}

func TestIntegration_ForestDepthRespectsMinDepth(t *testing.T) {
	db := setupIntegrationTestDB(t)

	root := models.TopicTag{Label: "R", Slug: "r", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Label: "C", Slug: "c", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	db.Create(&root)
	db.Create(&child)
	db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: child.ID, RelationType: "abstract"})

	forest, err := BuildTagForest("event", 3)
	if err != nil {
		t.Fatalf("BuildTagForest: %v", err)
	}
	if len(forest) != 0 {
		t.Fatalf("minDepth=3 should exclude depth-2 tree, got %d trees", len(forest))
	}

	forest, err = BuildTagForest("event", 2)
	if err != nil {
		t.Fatalf("BuildTagForest: %v", err)
	}
	if len(forest) != 1 {
		t.Fatalf("minDepth=2 should include depth-2 tree, got %d trees", len(forest))
	}
}
