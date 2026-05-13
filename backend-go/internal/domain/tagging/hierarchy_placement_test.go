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

func TestPlaceTagInHierarchy_NoTemplate(t *testing.T) {
	tag := &models.TopicTag{ID: 999, Category: "nonexistent", Label: "test"}
	result, err := PlaceTagInHierarchy(context.Background(), tag)
	if err == nil {
		t.Fatal("expected error for category with no template")
	}
	if result != nil {
		t.Fatal("expected nil result on error")
	}
}

func setupPlacementTestDB(t *testing.T) *gorm.DB {
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
		&models.TopicTagEmbedding{},
		&models.HierarchyConfig{},
		&models.HierarchyPendingChange{},
		&models.HierarchyConfigVersion{},
	); err != nil {
		t.Fatalf("migrate test tables: %v", err)
	}

	return db
}
