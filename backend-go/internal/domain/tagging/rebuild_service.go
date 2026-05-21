package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
	"my-robot-backend/internal/platform/ws"

	"gorm.io/gorm"
)

const (
	defaultBatchSize        = 20
	defaultBatchInterval    = 1 * time.Second
	defaultAvgPlacementTime = 500 * time.Millisecond
	estimationHistoryLimit  = 100
)

var placeTagInHierarchyFn = PlaceTagInHierarchy

type RebuildService struct {
	db            *gorm.DB
	batchSize     int
	batchInterval time.Duration
}

var (
	rebuildServiceInstance *RebuildService
	rebuildServiceOnce     sync.Once
)

func GetRebuildService() *RebuildService {
	rebuildServiceOnce.Do(func() {
		rebuildServiceInstance = &RebuildService{
			db:            database.DB,
			batchSize:     defaultBatchSize,
			batchInterval: defaultBatchInterval,
		}
	})
	return rebuildServiceInstance
}

func NewRebuildService(db *gorm.DB, batchSize int, batchInterval time.Duration) *RebuildService {
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	if batchInterval <= 0 {
		batchInterval = defaultBatchInterval
	}
	return &RebuildService{
		db:            db,
		batchSize:     batchSize,
		batchInterval: batchInterval,
	}
}

func (s *RebuildService) CreateJob(category, trigger string, configSnapshot models.MetadataMap) (*models.RebuildJob, error) {
	var activeCount int64
	s.db.Model(&models.RebuildJob{}).
		Where("category = ? AND status IN ?", category, []string{
			models.RebuildJobStatusPending,
			models.RebuildJobStatusRunning,
			models.RebuildJobStatusPaused,
		}).Count(&activeCount)
	if activeCount > 0 {
		return nil, fmt.Errorf("active rebuild job already exists for category %q", category)
	}

	var totalTags int64
	s.db.Model(&models.TopicTag{}).
		Where("category = ? AND source IN ? AND status = ?", category, []string{"llm", "heuristic"}, "active").
		Count(&totalTags)

	avgTime, err := s.estimateAvgPlacementTime()
	if err != nil {
		logging.Warnf("RebuildService: failed to estimate avg placement time: %v, using default", err)
		avgTime = defaultAvgPlacementTime
	}

	estimatedDuration := time.Duration(int64(totalTags) * int64(avgTime))
	estimatedEnd := time.Now().Add(estimatedDuration)

	if configSnapshot == nil {
		configSnapshot = models.MetadataMap{}
	}

	job := &models.RebuildJob{
		Category:       category,
		Trigger:        trigger,
		Status:         models.RebuildJobStatusPending,
		TotalTags:      int(totalTags),
		EstimatedEnd:   &estimatedEnd,
		ConfigSnapshot: configSnapshot,
	}

	if err := CreateRebuildJob(s.db, job); err != nil {
		return nil, fmt.Errorf("create rebuild job: %w", err)
	}

	return job, nil
}

func (s *RebuildService) ExecuteJob(jobID uint) error {
	job, err := GetRebuildJobByID(s.db, jobID)
	if err != nil {
		return fmt.Errorf("get rebuild job %d: %w", jobID, err)
	}

	if job.Status != models.RebuildJobStatusPending && job.Status != models.RebuildJobStatusPaused {
		return fmt.Errorf("job %d is in status %q, cannot execute", jobID, job.Status)
	}

	if err := UpdateRebuildJobStatus(s.db, jobID, models.RebuildJobStatusRunning); err != nil {
		return fmt.Errorf("update job status to running: %w", err)
	}

	processed := job.ProcessedTags
	failed := job.FailedTags
	lastTagID := job.LastTagID

	ctx := context.Background()
	if _, err := NewHierarchyOrchestrationService(s.db).BootstrapCategoryForRebuild(ctx, job.Category, job.ID); err != nil {
		errorDetail := fmt.Sprintf("bootstrap category: %v", err)
		_ = UpdateRebuildJobStatus(s.db, jobID, models.RebuildJobStatusFailed, errorDetail)
		s.broadcastFailed(job.ID, job.Category, processed, job.TotalTags, failed, errorDetail)
		return fmt.Errorf("bootstrap category before rebuild: %w", err)
	}

	for {
		var tags []models.TopicTag
		err := s.db.Where("category = ? AND source IN ? AND status = ? AND id > ?",
			job.Category, []string{"llm", "heuristic"}, "active", lastTagID).
			Order("id ASC").
			Limit(s.batchSize).
			Find(&tags).Error
		if err != nil {
			errorDetail := fmt.Sprintf("query tags: %v", err)
			_ = UpdateRebuildJobStatus(s.db, jobID, models.RebuildJobStatusFailed, errorDetail)
			s.broadcastFailed(job.ID, job.Category, processed, job.TotalTags, failed, errorDetail)
			return fmt.Errorf("query tags for rebuild: %w", err)
		}

		if len(tags) == 0 {
			break
		}

		for _, tag := range tags {
			_, placeErr := placeTagInHierarchyFn(ctx, &tag)
			if placeErr != nil {
				failed++
				logging.Warnf("RebuildService: PlaceTagInHierarchy failed for tag %d: %v", tag.ID, placeErr)
			}
			processed++
			lastTagID = int(tag.ID)
		}

		if err := UpdateRebuildJobProgress(s.db, jobID, processed, lastTagID); err != nil {
			logging.Warnf("RebuildService: failed to update progress for job %d: %v", jobID, err)
		}

		s.db.Model(&models.RebuildJob{}).Where("id = ?", jobID).
			Update("failed_tags", failed)

		currentTag := tags[len(tags)-1].Label
		s.broadcastProgress(job.ID, job.Category, processed, job.TotalTags, failed, currentTag)

		if len(tags) < s.batchSize {
			break
		}

		time.Sleep(s.batchInterval)
	}

	if err := UpdateRebuildJobStatus(s.db, jobID, models.RebuildJobStatusCompleted); err != nil {
		errorDetail := fmt.Sprintf("update job status to completed: %v", err)
		_ = UpdateRebuildJobStatus(s.db, jobID, models.RebuildJobStatusFailed, errorDetail)
		s.broadcastFailed(job.ID, job.Category, processed, job.TotalTags, failed, errorDetail)
		return fmt.Errorf("update job status to completed: %w", err)
	}

	s.broadcastComplete(job.ID, job.Category, processed, job.TotalTags, failed)

	return nil
}

func (s *RebuildService) ResumeJob(jobID uint) error {
	job, err := GetRebuildJobByID(s.db, jobID)
	if err != nil {
		return fmt.Errorf("get rebuild job %d: %w", jobID, err)
	}

	if job.Status != models.RebuildJobStatusPaused {
		return fmt.Errorf("job %d is in status %q, cannot resume (must be paused)", jobID, job.Status)
	}

	logging.Infof("RebuildService: resuming job %d for category %q from tag_id %d (processed: %d/%d)",
		jobID, job.Category, job.LastTagID, job.ProcessedTags, job.TotalTags)

	return s.ExecuteJob(jobID)
}

func (s *RebuildService) RecoverIncompleteJobs() {
	var runningJobs []models.RebuildJob
	if err := s.db.Where("status = ?", models.RebuildJobStatusRunning).Find(&runningJobs).Error; err != nil {
		logging.Warnf("RebuildService: failed to find running jobs on startup: %v", err)
		return
	}

	for _, job := range runningJobs {
		logging.Infof("RebuildService: pausing incomplete job %d (category=%q, processed=%d/%d)",
			job.ID, job.Category, job.ProcessedTags, job.TotalTags)
		if err := UpdateRebuildJobStatus(s.db, job.ID, models.RebuildJobStatusPaused); err != nil {
			logging.Warnf("RebuildService: failed to pause job %d: %v", job.ID, err)
		}
	}
}

func (s *RebuildService) estimateRebuildTime(category string) (time.Duration, error) {
	var totalTags int64
	s.db.Model(&models.TopicTag{}).
		Where("category = ? AND source IN ? AND status = ?", category, []string{"llm", "heuristic"}, "active").
		Count(&totalTags)

	if totalTags == 0 {
		return 0, nil
	}

	avgTime, err := s.estimateAvgPlacementTime()
	if err != nil {
		return 0, err
	}

	return time.Duration(int64(totalTags) * int64(avgTime)), nil
}

func (s *RebuildService) estimateAvgPlacementTime() (time.Duration, error) {
	var avgLatency float64
	err := s.db.Model(&models.AICallLog{}).
		Where("capability = ? AND success = ?", string("topic_tagging"), true).
		Where("request_meta LIKE ?", "%PlaceTagInHierarchy%").
		Select("COALESCE(AVG(latency_ms), 0)").
		Order("created_at DESC").
		Limit(estimationHistoryLimit).
		Scan(&avgLatency).Error
	if err != nil {
		return defaultAvgPlacementTime, fmt.Errorf("query avg placement time: %w", err)
	}

	if avgLatency == 0 {
		return defaultAvgPlacementTime, nil
	}

	return time.Duration(avgLatency) * time.Millisecond, nil
}

func (s *RebuildService) TriggerTemplateRebuild(category string, configSnapshot models.MetadataMap) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var abstractTagIDs []uint
		if err := tx.Model(&models.TopicTag{}).
			Where("category = ? AND source = ?", category, "abstract").
			Pluck("id", &abstractTagIDs).Error; err != nil {
			return fmt.Errorf("find abstract tags for category %q: %w", category, err)
		}

		if len(abstractTagIDs) > 0 {
			if err := tx.Where("relation_type = ? AND (parent_id IN ? OR child_id IN ?)",
				"abstract", abstractTagIDs, abstractTagIDs).
				Delete(&models.TopicTagRelation{}).Error; err != nil {
				return fmt.Errorf("delete abstract relations for category %q: %w", category, err)
			}

			if err := tx.Where("topic_tag_id IN ?", abstractTagIDs).
				Delete(&models.TopicTagEmbedding{}).Error; err != nil {
				return fmt.Errorf("delete abstract embeddings for category %q: %w", category, err)
			}

			if err := tx.Where("id IN ?", abstractTagIDs).
				Delete(&models.TopicTag{}).Error; err != nil {
				return fmt.Errorf("delete abstract tags for category %q: %w", category, err)
			}
		}

		var activeCount int64
		tx.Model(&models.RebuildJob{}).
			Where("category = ? AND status IN ?", category, []string{
				models.RebuildJobStatusPending,
				models.RebuildJobStatusRunning,
				models.RebuildJobStatusPaused,
			}).Count(&activeCount)
		if activeCount > 0 {
			return fmt.Errorf("active rebuild job already exists for category %q", category)
		}

		var totalTags int64
		tx.Model(&models.TopicTag{}).
			Where("category = ? AND source IN ? AND status = ?", category, []string{"llm", "heuristic"}, "active").
			Count(&totalTags)

		avgTime, err := s.estimateAvgPlacementTime()
		if err != nil {
			avgTime = defaultAvgPlacementTime
		}
		estimatedDuration := time.Duration(int64(totalTags) * int64(avgTime))
		estimatedEnd := time.Now().Add(estimatedDuration)

		if configSnapshot == nil {
			configSnapshot = models.MetadataMap{}
		}

		job := &models.RebuildJob{
			Category:       category,
			Trigger:        models.RebuildJobTriggerTemplateChange,
			Status:         models.RebuildJobStatusPending,
			TotalTags:      int(totalTags),
			EstimatedEnd:   &estimatedEnd,
			ConfigSnapshot: configSnapshot,
		}

		if err := tx.Create(job).Error; err != nil {
			return fmt.Errorf("create rebuild job: %w", err)
		}

		logging.Infof("RebuildService: triggered template rebuild for category %q, %d leaf tags, job %d",
			category, totalTags, job.ID)

		return nil
	})
}

func (s *RebuildService) broadcastProgress(jobID uint, category string, processed, total, failedCount int, currentTag string) {
	hub := ws.GetHub()

	var remainingSeconds float64
	if total > 0 && processed < total {
		avgTime, err := s.estimateAvgPlacementTime()
		if err != nil {
			avgTime = defaultAvgPlacementTime
		}
		remainingSeconds = float64(total-processed) * avgTime.Seconds()
	}

	msg := map[string]interface{}{
		"type":                        "hierarchy_rebuild",
		"status":                      "processing",
		"job_id":                      jobID,
		"category":                    category,
		"processed":                   processed,
		"total":                       total,
		"failed_count":                failedCount,
		"estimated_remaining_seconds": remainingSeconds,
		"current_tag":                 currentTag,
		"error":                       "",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		logging.Warnf("RebuildService: failed to marshal hierarchy_rebuild processing: %v", err)
		return
	}

	hub.BroadcastRaw(data)
}

func (s *RebuildService) broadcastComplete(jobID uint, category string, processed, total, failedCount int) {
	hub := ws.GetHub()

	msg := map[string]interface{}{
		"type":                        "hierarchy_rebuild",
		"status":                      "completed",
		"job_id":                      jobID,
		"category":                    category,
		"processed":                   processed,
		"total":                       total,
		"failed_count":                failedCount,
		"estimated_remaining_seconds": 0,
		"current_tag":                 "",
		"error":                       "",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		logging.Warnf("RebuildService: failed to marshal hierarchy_rebuild completed: %v", err)
		return
	}

	hub.BroadcastRaw(data)
}

func (s *RebuildService) broadcastFailed(jobID uint, category string, processed, total, failedCount int, errorDetail string) {
	hub := ws.GetHub()

	msg := map[string]interface{}{
		"type":                        "hierarchy_rebuild",
		"status":                      "failed",
		"job_id":                      jobID,
		"category":                    category,
		"processed":                   processed,
		"total":                       total,
		"failed_count":                failedCount,
		"estimated_remaining_seconds": 0,
		"current_tag":                 "",
		"error":                       errorDetail,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		logging.Warnf("RebuildService: failed to marshal hierarchy_rebuild failed: %v", err)
		return
	}

	hub.BroadcastRaw(data)
}
