package tagging

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

func setupTaggerEmbeddingTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	database.DB = db
	t.Cleanup(func() {
		database.DB = nil
	})

	if err := database.DB.AutoMigrate(
		&models.TopicTag{},
		&models.TopicTagRelation{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
}

func TestShouldDeleteAbstractChildEmbeddingPreservesNormalChildWithAbstractSibling(t *testing.T) {
	t.Skip("shouldDeleteAbstractChildEmbedding removed — abstract child creation path removed")
}

func TestShouldDeleteAbstractChildEmbeddingDeletesNormalChildWithoutAbstractSibling(t *testing.T) {
	t.Skip("shouldDeleteAbstractChildEmbedding removed — abstract child creation path removed")
}
