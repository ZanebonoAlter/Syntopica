package tagging

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupRebuildServiceTestDB(t *testing.T) *gorm.DB {
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
		&models.Feed{},
		&models.Article{},
		&models.RebuildJob{},
		&models.AICallLog{},
		&models.BoardConcept{},
		&models.HierarchyPendingChange{},
	); err != nil {
		t.Fatalf("migrate test tables: %v", err)
	}

	return db
}

func TestRebuildServiceEventContract(t *testing.T) {
	body, err := os.ReadFile("rebuild_service.go")
	if err != nil {
		t.Fatalf("read rebuild_service.go: %v", err)
	}
	source := string(body)

	for _, legacyName := range []string{"rebuild_progress", "rebuild_complete"} {
		if strings.Contains(source, legacyName) {
			t.Fatalf("legacy event name %q should be absent", legacyName)
		}
	}

	for _, required := range []string{
		`"type":`,
		`"hierarchy_rebuild"`,
		`"status":`,
		`"processing"`,
		`"completed"`,
		`"failed"`,
		`"failed_count"`,
		`"estimated_remaining_seconds"`,
		`"current_tag"`,
		`"error"`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("rebuild_service.go missing event contract token %s", required)
		}
	}
}

func TestCreateJob(t *testing.T) {
	db := setupRebuildServiceTestDB(t)
	svc := NewRebuildService(db, 10, 100*time.Millisecond)

	t.Run("creates job with correct fields", func(t *testing.T) {
		job, err := svc.CreateJob("keyword", "manual", models.MetadataMap{"key": "val"})
		if err != nil {
			t.Fatalf("CreateJob: %v", err)
		}
		if job.Category != "keyword" {
			t.Errorf("category = %q, want %q", job.Category, "keyword")
		}
		if job.Trigger != "manual" {
			t.Errorf("trigger = %q, want %q", job.Trigger, "manual")
		}
		if job.Status != models.RebuildJobStatusPending {
			t.Errorf("status = %q, want %q", job.Status, models.RebuildJobStatusPending)
		}
		if job.EstimatedEnd == nil {
			t.Error("estimated_end should not be nil")
		}
		if job.ConfigSnapshot == nil {
			t.Error("config_snapshot should not be nil")
		}
	})

	t.Run("rejects duplicate active job", func(t *testing.T) {
		_, err := svc.CreateJob("keyword", "manual", nil)
		if err == nil {
			t.Fatal("expected error for duplicate active job")
		}
		if !strings.Contains(err.Error(), "active rebuild job already exists") {
			t.Errorf("error = %q, want 'active rebuild job already exists'", err.Error())
		}
	})

	t.Run("allows different categories", func(t *testing.T) {
		job, err := svc.CreateJob("event", "manual", nil)
		if err != nil {
			t.Fatalf("CreateJob for event: %v", err)
		}
		if job.Category != "event" {
			t.Errorf("category = %q, want %q", job.Category, "event")
		}
	})
}

func TestExecuteJob_BatchProcessing(t *testing.T) {
	db := setupRebuildServiceTestDB(t)
	svc := NewRebuildService(db, 5, 0)

	for i := 0; i < 8; i++ {
		tag := models.TopicTag{
			Slug:     fmt.Sprintf("tag-%d", i),
			Label:    fmt.Sprintf("Tag %d", i),
			Category: "keyword",
			Source:   "llm",
			Status:   "active",
		}
		if err := db.Create(&tag).Error; err != nil {
			t.Fatalf("create tag %d: %v", i, err)
		}
	}

	origPlaceFn := placeTagInHierarchyFn
	placeTagInHierarchyFn = func(ctx context.Context, tag *models.TopicTag) (*PlacementResult, error) {
		return nil, nil
	}
	t.Cleanup(func() {
		placeTagInHierarchyFn = origPlaceFn
	})

	job, err := svc.CreateJob("keyword", "manual", nil)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	if err := svc.ExecuteJob(job.ID); err != nil {
		t.Fatalf("ExecuteJob: %v", err)
	}

	updated, err := GetRebuildJobByID(db, job.ID)
	if err != nil {
		t.Fatalf("GetRebuildJobByID: %v", err)
	}
	if updated.Status != models.RebuildJobStatusCompleted {
		t.Errorf("status = %q, want %q", updated.Status, models.RebuildJobStatusCompleted)
	}
	if updated.ProcessedTags != 8 {
		t.Errorf("processed_tags = %d, want 8", updated.ProcessedTags)
	}
}

func TestExecuteJob_CheckpointResume(t *testing.T) {
	db := setupRebuildServiceTestDB(t)
	svc := NewRebuildService(db, 5, 0)

	for i := 0; i < 10; i++ {
		tag := models.TopicTag{
			Slug:     fmt.Sprintf("tag-cp-%d", i),
			Label:    fmt.Sprintf("Tag CP %d", i),
			Category: "keyword",
			Source:   "llm",
			Status:   "active",
		}
		if err := db.Create(&tag).Error; err != nil {
			t.Fatalf("create tag %d: %v", i, err)
		}
	}

	callCount := 0
	origPlaceFn := placeTagInHierarchyFn
	placeTagInHierarchyFn = func(ctx context.Context, tag *models.TopicTag) (*PlacementResult, error) {
		callCount++
		if callCount == 3 {
			return nil, fmt.Errorf("simulated failure")
		}
		return nil, nil
	}
	t.Cleanup(func() {
		placeTagInHierarchyFn = origPlaceFn
	})

	job, err := svc.CreateJob("keyword", "manual", nil)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	_ = svc.ExecuteJob(job.ID)

	updated, _ := GetRebuildJobByID(db, job.ID)
	if updated.ProcessedTags == 0 {
		t.Fatal("expected some progress before failure")
	}
	if updated.LastTagID == 0 {
		t.Fatal("expected last_tag_id checkpoint to be set")
	}
	savedProcessed := updated.ProcessedTags

	if err := UpdateRebuildJobStatus(db, job.ID, models.RebuildJobStatusPaused); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	callCount = 0
	placeTagInHierarchyFn = func(ctx context.Context, tag *models.TopicTag) (*PlacementResult, error) {
		callCount++
		return nil, nil
	}

	if err := svc.ResumeJob(job.ID); err != nil {
		t.Fatalf("ResumeJob: %v", err)
	}

	resumed, _ := GetRebuildJobByID(db, job.ID)
	if resumed.ProcessedTags <= savedProcessed {
		t.Errorf("processed_tags = %d, want > %d after resume", resumed.ProcessedTags, savedProcessed)
	}
	if resumed.Status != models.RebuildJobStatusCompleted {
		t.Errorf("status = %q, want %q", resumed.Status, models.RebuildJobStatusCompleted)
	}
}

func TestExecuteJob_RateLimiting(t *testing.T) {
	db := setupRebuildServiceTestDB(t)

	batchInterval := 50 * time.Millisecond
	svc := NewRebuildService(db, 2, batchInterval)

	for i := 0; i < 4; i++ {
		tag := models.TopicTag{
			Slug:     fmt.Sprintf("tag-rl-%d", i),
			Label:    fmt.Sprintf("Tag RL %d", i),
			Category: "keyword",
			Source:   "llm",
			Status:   "active",
		}
		if err := db.Create(&tag).Error; err != nil {
			t.Fatalf("create tag %d: %v", i, err)
		}
	}

	origPlaceFn := placeTagInHierarchyFn
	placeTagInHierarchyFn = func(ctx context.Context, tag *models.TopicTag) (*PlacementResult, error) {
		return nil, nil
	}
	t.Cleanup(func() {
		placeTagInHierarchyFn = origPlaceFn
	})

	job, err := svc.CreateJob("keyword", "manual", nil)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	start := time.Now()
	if err := svc.ExecuteJob(job.ID); err != nil {
		t.Fatalf("ExecuteJob: %v", err)
	}
	elapsed := time.Since(start)

	expectedBatches := 2
	minDuration := time.Duration(expectedBatches-1) * batchInterval
	if elapsed < minDuration {
		t.Errorf("elapsed = %v, want >= %v (rate limiting between batches)", elapsed, minDuration)
	}
}

func TestTriggerTemplateRebuild(t *testing.T) {
	db := setupRebuildServiceTestDB(t)
	svc := NewRebuildService(db, 10, 0)

	t.Run("deletes abstract tags relations and embeddings then creates job", func(t *testing.T) {
		abstractTag := models.TopicTag{
			Slug: "abstract-1", Label: "Abstract 1", Category: "keyword",
			Source: "abstract", Status: "active",
		}
		leafTag := models.TopicTag{
			Slug: "leaf-1", Label: "Leaf 1", Category: "keyword",
			Source: "llm", Status: "active",
		}
		db.Create(&abstractTag)
		db.Create(&leafTag)

		db.Create(&models.TopicTagRelation{
			ParentID: abstractTag.ID, ChildID: leafTag.ID, RelationType: "abstract",
		})
		db.Create(&models.TopicTagEmbedding{
			TopicTagID: abstractTag.ID, Vector: "[0.1,0.2]", Dimension: 2, Model: "test",
		})

		err := svc.TriggerTemplateRebuild("keyword", models.MetadataMap{"tmpl": "v2"})
		if err != nil {
			t.Fatalf("TriggerTemplateRebuild: %v", err)
		}

		var abstractCount int64
		db.Model(&models.TopicTag{}).Where("source = ? AND category = ?", "abstract", "keyword").Count(&abstractCount)
		if abstractCount != 0 {
			t.Errorf("abstract tags remaining = %d, want 0", abstractCount)
		}

		var relCount int64
		db.Model(&models.TopicTagRelation{}).Where("relation_type = ?", "abstract").Count(&relCount)
		if relCount != 0 {
			t.Errorf("abstract relations remaining = %d, want 0", relCount)
		}

		var embCount int64
		db.Model(&models.TopicTagEmbedding{}).Where("topic_tag_id = ?", abstractTag.ID).Count(&embCount)
		if embCount != 0 {
			t.Errorf("abstract embeddings remaining = %d, want 0", embCount)
		}

		var jobs []models.RebuildJob
		db.Where("category = ?", "keyword").Find(&jobs)
		if len(jobs) != 1 {
			t.Fatalf("rebuild jobs = %d, want 1", len(jobs))
		}
		if jobs[0].Trigger != models.RebuildJobTriggerTemplateChange {
			t.Errorf("trigger = %q, want %q", jobs[0].Trigger, models.RebuildJobTriggerTemplateChange)
		}
	})
}
