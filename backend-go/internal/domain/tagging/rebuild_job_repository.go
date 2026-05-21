package tagging

import (
	"fmt"
	"time"

	"my-robot-backend/internal/domain/models"

	"gorm.io/gorm"
)

func CreateRebuildJob(db *gorm.DB, job *models.RebuildJob) error {
	return db.Create(job).Error
}

func GetRebuildJobByID(db *gorm.DB, id uint) (*models.RebuildJob, error) {
	var job models.RebuildJob
	if err := db.First(&job, id).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func UpdateRebuildJobProgress(db *gorm.DB, id uint, processedTags int, lastTagID int) error {
	return db.Model(&models.RebuildJob{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"processed_tags": processedTags,
			"last_tag_id":    lastTagID,
		}).Error
}

func ListRebuildJobsByCategory(db *gorm.DB, category string) ([]models.RebuildJob, error) {
	var jobs []models.RebuildJob
	err := db.Where("category = ?", category).
		Order("created_at DESC").
		Find(&jobs).Error
	return jobs, err
}

func ListActiveRebuildJobs(db *gorm.DB) ([]models.RebuildJob, error) {
	var jobs []models.RebuildJob
	err := db.Where("status IN ?", []string{
		models.RebuildJobStatusPending,
		models.RebuildJobStatusRunning,
		models.RebuildJobStatusPaused,
	}).Order("created_at ASC").Find(&jobs).Error
	return jobs, err
}

func UpdateRebuildJobStatus(db *gorm.DB, id uint, status string, errorDetail ...string) error {
	updates := map[string]interface{}{
		"status": status,
	}

	switch status {
	case models.RebuildJobStatusRunning:
		now := time.Now()
		updates["started_at"] = &now
	case models.RebuildJobStatusCompleted:
		now := time.Now()
		updates["completed_at"] = &now
	case models.RebuildJobStatusFailed:
		now := time.Now()
		updates["completed_at"] = &now
		if len(errorDetail) > 0 {
			updates["error_detail"] = errorDetail[0]
		}
	}

	result := db.Model(&models.RebuildJob{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("rebuild job %d not found", id)
	}
	return nil
}
