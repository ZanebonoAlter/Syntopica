package tagging

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRebuildV2TestDB(t *testing.T) *gorm.DB {
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
		&models.RebuildJob{},
		&models.AICallLog{},
		&models.BoardConcept{},
		&models.HierarchyPendingChange{},
	), "migrate test tables")

	return db
}

func seedActiveTag(t *testing.T, db *gorm.DB, slug, category, source string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{
		Slug:     slug,
		Label:    slug,
		Category: category,
		Source:   source,
		Status:   "active",
	}
	require.NoError(t, db.Create(&tag).Error, "create tag "+slug)
	return tag
}

func TestRebuildService_CreateJob(t *testing.T) {
	db := setupRebuildV2TestDB(t)
	svc := NewRebuildService(db, 10, 100*time.Millisecond)

	t.Run("sets correct fields on new job", func(t *testing.T) {
		seedActiveTag(t, db, "kw-1", "keyword", "llm")
		seedActiveTag(t, db, "kw-2", "keyword", "heuristic")

		job, err := svc.CreateJob("keyword", "manual", models.MetadataMap{"reason": "test"})
		require.NoError(t, err)

		assert.Equal(t, "keyword", job.Category)
		assert.Equal(t, "manual", job.Trigger)
		assert.Equal(t, models.RebuildJobStatusPending, job.Status)
		assert.Equal(t, 2, job.TotalTags)
		assert.NotNil(t, job.EstimatedEnd)
		assert.Equal(t, models.MetadataMap{"reason": "test"}, job.ConfigSnapshot)
	})

	t.Run("counts only llm and heuristic active tags", func(t *testing.T) {
		seedActiveTag(t, db, "manual-1", "keyword", "manual")
		seedActiveTag(t, db, "abstract-1", "keyword", "abstract")

		job, err := svc.CreateJob("event", "manual", nil)
		require.NoError(t, err)
		assert.Equal(t, 0, job.TotalTags, "only llm/heuristic sources should be counted")
	})
}

func TestRebuildService_CreateJob_Dedup(t *testing.T) {
	db := setupRebuildV2TestDB(t)
	svc := NewRebuildService(db, 10, 100*time.Millisecond)

	_, err := svc.CreateJob("keyword", "manual", nil)
	require.NoError(t, err)

	_, err = svc.CreateJob("keyword", "manual", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "active rebuild job already exists")

	t.Run("different category allowed", func(t *testing.T) {
		_, err := svc.CreateJob("event", "manual", nil)
		assert.NoError(t, err)
	})
}

func TestRebuildService_ExecuteJob_NoTags(t *testing.T) {
	db := setupRebuildV2TestDB(t)
	svc := NewRebuildService(db, 10, 0)

	job, err := svc.CreateJob("keyword", "manual", nil)
	require.NoError(t, err)

	origFn := placeTagInHierarchyFn
	placeTagInHierarchyFn = func(ctx context.Context, tag *models.TopicTag) (*PlacementResult, error) {
		return nil, nil
	}
	t.Cleanup(func() { placeTagInHierarchyFn = origFn })

	err = svc.ExecuteJob(job.ID)
	require.NoError(t, err)

	updated, err := GetRebuildJobByID(db, job.ID)
	require.NoError(t, err)
	assert.Equal(t, models.RebuildJobStatusCompleted, updated.Status)
	assert.Equal(t, 0, updated.ProcessedTags)
	assert.Equal(t, 0, updated.FailedTags)
}

func TestRebuildService_RecoverIncompleteJobs(t *testing.T) {
	db := setupRebuildV2TestDB(t)
	svc := NewRebuildService(db, 10, 0)

	require.NoError(t, CreateRebuildJob(db, &models.RebuildJob{
		Category:      "keyword",
		Trigger:       "manual",
		Status:        models.RebuildJobStatusRunning,
		TotalTags:     10,
		ProcessedTags: 3,
		LastTagID:     99,
	}))
	require.NoError(t, CreateRebuildJob(db, &models.RebuildJob{
		Category:  "event",
		Trigger:   "manual",
		Status:    models.RebuildJobStatusPending,
		TotalTags: 5,
	}))

	svc.RecoverIncompleteJobs()

	var recovered models.RebuildJob
	require.NoError(t, db.Where("category = ?", "keyword").First(&recovered).Error)
	assert.Equal(t, models.RebuildJobStatusPaused, recovered.Status, "running job should be paused")

	var pending models.RebuildJob
	require.NoError(t, db.Where("category = ?", "event").First(&pending).Error)
	assert.Equal(t, models.RebuildJobStatusPending, pending.Status, "pending job should remain pending")
}
