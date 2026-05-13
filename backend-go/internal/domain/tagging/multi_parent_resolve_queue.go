package tagging

import (
	"fmt"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm/clause"
)

// ProcessPendingMultiParentResolveTasks drains legacy pending tasks from the queue.
// New multi-parent conflicts are resolved synchronously via resolveMultiParentConflict
// (picks highest-similarity parent without LLM). This function only processes tasks
// that were enqueued before the migration.
func ProcessPendingMultiParentResolveTasks() (int, error) {
	var tasks []models.MultiParentResolveQueue
	if err := database.DB.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("status = ?", models.MultiParentResolveStatusPending).
		Order("id ASC").
		Limit(50).
		Find(&tasks).Error; err != nil || len(tasks) == 0 {
		return 0, err
	}

	now := time.Now()
	for i := range tasks {
		tasks[i].Status = models.MultiParentResolveStatusProcessing
		tasks[i].StartedAt = &now
		database.DB.Save(&tasks[i])
	}

	var conflicts []multiParentConflict
	var completedIDs []uint
	for _, task := range tasks {
		var relations []models.TopicTagRelation
		if err := database.DB.
			Where("child_id = ? AND relation_type = ?", task.ChildTagID, "abstract").
			Preload("Parent").
			Find(&relations).Error; err != nil {
			markMPTaskFailed(task.ID, fmt.Sprintf("load relations: %v", err))
			continue
		}
		if len(relations) <= 1 {
			completedIDs = append(completedIDs, task.ID)
			continue
		}
		var childTag models.TopicTag
		if err := database.DB.First(&childTag, task.ChildTagID).Error; err != nil {
			markMPTaskFailed(task.ID, fmt.Sprintf("load child: %v", err))
			continue
		}
		var parents []parentWithInfo
		for _, r := range relations {
			if r.Parent != nil {
				parents = append(parents, parentWithInfo{RelationID: r.ID, Parent: r.Parent, SimilarityScore: r.SimilarityScore})
			}
		}
		if len(parents) <= 1 {
			completedIDs = append(completedIDs, task.ID)
			continue
		}
		conflicts = append(conflicts, multiParentConflict{
			ChildID: task.ChildTagID,
			Parents: parents,
			Child:   &childTag,
		})
	}

	for _, id := range completedIDs {
		database.DB.Model(&models.MultiParentResolveQueue{}).Where("id = ?", id).
			Updates(map[string]any{"status": models.MultiParentResolveStatusCompleted, "completed_at": time.Now()})
	}

	if len(conflicts) > 0 {
		resolved, errs := batchResolveMultiParentConflicts(conflicts)
		logging.Infof("ProcessPendingMultiParentResolveTasks: resolved %d/%d conflicts, %d errors",
			resolved, len(conflicts), len(errs))

		failedCount := len(errs)
		if failedCount > 0 {
			errorSet := make(map[string]bool, len(errs))
			for _, e := range errs {
				errorSet[e] = true
			}
			for _, c := range conflicts {
				childErrPrefix := fmt.Sprintf("child %d:", c.ChildID)
				hasErr := false
				for e := range errorSet {
					if len(e) >= len(childErrPrefix) && e[:len(childErrPrefix)] == childErrPrefix {
						hasErr = true
						break
					}
				}
				if hasErr {
					database.DB.Model(&models.MultiParentResolveQueue{}).
						Where("child_tag_id = ? AND status = ?", c.ChildID, models.MultiParentResolveStatusProcessing).
						Updates(map[string]any{"status": models.MultiParentResolveStatusFailed, "error_message": "batch resolve failed"})
				} else {
					database.DB.Model(&models.MultiParentResolveQueue{}).
						Where("child_tag_id = ? AND status = ?", c.ChildID, models.MultiParentResolveStatusProcessing).
						Updates(map[string]any{"status": models.MultiParentResolveStatusCompleted, "completed_at": time.Now()})
				}
			}
		} else {
			for _, c := range conflicts {
				database.DB.Model(&models.MultiParentResolveQueue{}).
					Where("child_tag_id = ? AND status = ?", c.ChildID, models.MultiParentResolveStatusProcessing).
					Updates(map[string]any{"status": models.MultiParentResolveStatusCompleted, "completed_at": time.Now()})
			}
		}
	}

	return len(conflicts), nil
}

func markMPTaskFailed(id uint, errMsg string) {
	database.DB.Model(&models.MultiParentResolveQueue{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":        models.MultiParentResolveStatusFailed,
			"error_message": errMsg,
		})
}
