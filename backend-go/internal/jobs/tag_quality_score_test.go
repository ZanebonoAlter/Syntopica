package jobs

import (
	"testing"
	"time"

	"syntopica-backend/internal/app/runtimeinfo"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

func TestTagQualityScoreSchedulerManualTriggerLifecycle(t *testing.T) {
	setupSchedulersTestDB(t)
	if err := database.DB.AutoMigrate(&models.TopicTag{}, &models.ArticleTopicTag{}, &models.TopicTagEmbedding{}, &models.TopicTagRelation{}); err != nil {
		t.Fatalf("extra migrate: %v", err)
	}

	tag := models.TopicTag{Slug: "scheduler-tag", Label: "Scheduler Tag", Category: models.TagCategoryKeyword, Status: "active"}
	if err := database.DB.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	scheduler := NewTagQualityScoreScheduler(60)
	if err := scheduler.Start(); err != nil {
		t.Fatalf("start scheduler: %v", err)
	}
	defer scheduler.Stop()

	runtimeinfo.TagQualityScoreSchedulerInterface = scheduler
	descriptor, resolved := resolveScheduler("tag_quality_score")
	if descriptor == nil || resolved == nil {
		t.Fatal("expected tag_quality_score scheduler to be resolvable from handler registry")
	}

	result := scheduler.TriggerNow()
	if accepted, _ := result["accepted"].(bool); !accepted {
		t.Fatalf("expected manual trigger accepted, got %#v", result)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var task models.SchedulerTask
		if err := database.DB.Where("name = ?", "tag_quality_score").First(&task).Error; err == nil && task.TotalExecutions > 0 {
			if err := scheduler.ResetStats(); err != nil {
				t.Fatalf("reset stats: %v", err)
			}

			var resetTask models.SchedulerTask
			if err := database.DB.Where("name = ?", "tag_quality_score").First(&resetTask).Error; err != nil {
				t.Fatalf("reload reset task: %v", err)
			}
			if resetTask.TotalExecutions != 0 {
				t.Fatalf("total executions after reset = %d, want 0", resetTask.TotalExecutions)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", "tag_quality_score").First(&task).Error; err != nil {
		t.Fatalf("load scheduler task: %v", err)
	}
	t.Fatalf("expected tag_quality_score manual trigger to update task, got total executions %d", task.TotalExecutions)
}
