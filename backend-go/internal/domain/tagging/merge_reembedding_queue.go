package tagging

import (
	"context"
	"sync"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MergeReembeddingQueueService manages async target-tag embedding regeneration after merges.
type MergeReembeddingQueueService struct {
	db        *gorm.DB
	embedding *EmbeddingService
	logger    *zap.Logger

	mu     sync.Mutex
	closed bool
	stopCh chan struct{}
}

func NewMergeReembeddingQueueService(logger *zap.Logger) *MergeReembeddingQueueService {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &MergeReembeddingQueueService{
		db:        database.DB,
		embedding: NewEmbeddingService(),
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

func (s *MergeReembeddingQueueService) Enqueue(sourceTagID, targetTagID uint) error {
	var activeCount int64
	err := s.db.Model(&models.MergeReembeddingQueue{}).
		Where("target_tag_id = ? AND status IN ?", targetTagID, []string{
			models.MergeReembeddingQueueStatusPending,
			models.MergeReembeddingQueueStatusProcessing,
		}).
		Count(&activeCount).Error
	if err != nil {
		return err
	}
	if activeCount > 0 {
		return nil
	}

	task := models.MergeReembeddingQueue{
		SourceTagID: sourceTagID,
		TargetTagID: targetTagID,
		Status:      models.MergeReembeddingQueueStatusPending,
	}

	return s.db.Create(&task).Error
}

func (s *MergeReembeddingQueueService) GetStatus() (map[string]int64, error) {
	type statusRow struct {
		Status string
		Count  int64
	}

	var rows []statusRow
	err := s.db.Model(&models.MergeReembeddingQueue{}).
		Select("status, count(*) as count").
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := map[string]int64{
		"pending":    0,
		"processing": 0,
		"completed":  0,
		"failed":     0,
		"total":      0,
	}

	var total int64
	for _, row := range rows {
		result[row.Status] = row.Count
		total += row.Count
	}
	result["total"] = total

	return result, nil
}

func (s *MergeReembeddingQueueService) GetTasks(status string, limit, offset int) ([]models.MergeReembeddingQueue, int64, error) {
	query := s.db.Model(&models.MergeReembeddingQueue{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var tasks []models.MergeReembeddingQueue
	err := query.
		Preload("SourceTag").
		Preload("TargetTag").
		Order("created_at ASC").
		Limit(limit).
		Offset(offset).
		Find(&tasks).Error
	if err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

func (s *MergeReembeddingQueueService) RetryFailed() (int64, error) {
	result := s.db.Model(&models.MergeReembeddingQueue{}).
		Where("status = ?", models.MergeReembeddingQueueStatusFailed).
		Updates(map[string]interface{}{
			"status":        models.MergeReembeddingQueueStatusPending,
			"error_message": "",
			"started_at":    nil,
			"completed_at":  nil,
		})
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

func (s *MergeReembeddingQueueService) Start() {
	s.mu.Lock()
	if s.closed {
		s.closed = false
		s.stopCh = make(chan struct{})
	}
	s.mu.Unlock()

	result := s.db.Model(&models.MergeReembeddingQueue{}).
		Where("status = ?", models.MergeReembeddingQueueStatusProcessing).
		Updates(map[string]interface{}{
			"status":     models.MergeReembeddingQueueStatusPending,
			"started_at": nil,
		})
	if result.Error != nil {
		s.logger.Error("failed to reset stale processing merge re-embedding tasks", zap.Error(result.Error))
	} else if result.RowsAffected > 0 {
		s.logger.Info("reset stale processing merge re-embedding tasks", zap.Int64("count", result.RowsAffected))
	}

	go s.worker()
	s.logger.Info("merge re-embedding queue worker started")
}

func (s *MergeReembeddingQueueService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.stopCh)
	s.logger.Info("merge re-embedding queue worker stopped")
}

func (s *MergeReembeddingQueueService) worker() {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("merge re-embedding queue worker panic recovered", zap.Any("panic", r))
			time.Sleep(5 * time.Second)
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if !closed {
				go s.worker()
			}
		}
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.processNext()
		}
	}
}

func (s *MergeReembeddingQueueService) processNext() {
	var tasks []models.MergeReembeddingQueue

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("status = ?", models.MergeReembeddingQueueStatusPending).
			Order("created_at ASC").
			Limit(1).
			Find(&tasks).Error; err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}

		now := time.Now()
		result := tx.Model(&models.MergeReembeddingQueue{}).
			Where("id = ? AND status = ?", tasks[0].ID, models.MergeReembeddingQueueStatusPending).
			Updates(map[string]interface{}{
				"status":     models.MergeReembeddingQueueStatusProcessing,
				"started_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			tasks[0].ID = 0
		}
		return nil
	})
	if err != nil {
		s.logger.Error("failed to claim merge re-embedding task", zap.Error(err))
		return
	}

	if len(tasks) == 0 || tasks[0].ID == 0 {
		return
	}

	task := tasks[0]

	var target models.TopicTag
	if err := s.db.First(&target, task.TargetTagID).Error; err != nil {
		s.markFailed(task.ID, "failed to load target tag: "+err.Error())
		return
	}

	identityEmb, err := s.embedding.GenerateEmbedding(context.Background(), &target, EmbeddingTypeIdentity)
	if err != nil {
		s.markFailed(task.ID, "failed to generate identity embedding: "+err.Error())
		return
	}

	if err := s.embedding.SaveEmbedding(identityEmb); err != nil {
		s.markFailed(task.ID, "failed to save identity embedding: "+err.Error())
		return
	}

	var semOpts []EmbeddingTextOptions
	if target.Category == "event" {
		titles := GetTagContextTitles(target.ID, 5)
		if len(titles) > 0 {
			semOpts = append(semOpts, EmbeddingTextOptions{ContextTitles: titles})
		}
	}
	semanticEmb, semErr := s.embedding.GenerateEmbedding(context.Background(), &target, EmbeddingTypeSemantic, semOpts...)
	if semErr != nil {
		s.markFailed(task.ID, "failed to generate semantic embedding: "+semErr.Error())
		return
	}

	if err := s.embedding.SaveEmbedding(semanticEmb); err != nil {
		s.markFailed(task.ID, "failed to save semantic embedding: "+semErr.Error())
		return
	}

	now := time.Now()
	if err := s.db.Model(&models.MergeReembeddingQueue{}).
		Where("id = ?", task.ID).
		Updates(map[string]interface{}{
			"status":       models.MergeReembeddingQueueStatusCompleted,
			"completed_at": now,
		}).Error; err != nil {
		s.logger.Error("failed to mark merge re-embedding task completed", zap.Uint("task_id", task.ID), zap.Error(err))
		return
	}

	s.logger.Info("merge re-embedding completed", zap.Uint("target_tag_id", task.TargetTagID), zap.Uint("source_tag_id", task.SourceTagID))
}

func (s *MergeReembeddingQueueService) markFailed(taskID uint, errMsg string) {
	now := time.Now()
	if err := s.db.Model(&models.MergeReembeddingQueue{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":        models.MergeReembeddingQueueStatusFailed,
			"error_message": errMsg,
			"completed_at":  now,
			"retry_count":   gorm.Expr("retry_count + 1"),
		}).Error; err != nil {
		s.logger.Error("failed to mark merge re-embedding task failed", zap.Uint("task_id", taskID), zap.Error(err))
	}

	s.logger.Warn("merge re-embedding task failed", zap.Uint("task_id", taskID), zap.String("error", errMsg))
}
