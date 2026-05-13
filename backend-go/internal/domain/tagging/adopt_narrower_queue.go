package tagging

import (
	"context"
	"strings"
	"sync"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AdoptNarrowerQueueService struct {
	db     *gorm.DB
	logger *zap.Logger

	mu     sync.Mutex
	closed bool
	stopCh chan struct{}
}

func NewAdoptNarrowerQueueService(logger *zap.Logger) *AdoptNarrowerQueueService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AdoptNarrowerQueueService{
		db:     database.DB,
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

func (s *AdoptNarrowerQueueService) Enqueue(abstractTagID uint, source string) error {
	if abstractTagID == 0 {
		return nil
	}

	var activeCount int64
	err := s.db.Model(&models.AdoptNarrowerQueue{}).
		Where("abstract_tag_id = ? AND status IN ?", abstractTagID, []string{
			models.AdoptNarrowerQueueStatusPending,
			models.AdoptNarrowerQueueStatusProcessing,
		}).Count(&activeCount).Error
	if err != nil {
		return err
	}
	if activeCount > 0 {
		return nil
	}

	var recentCompleted int64
	err = s.db.Model(&models.AdoptNarrowerQueue{}).
		Where("abstract_tag_id = ? AND status = ? AND completed_at > ?", abstractTagID,
			models.AdoptNarrowerQueueStatusCompleted, time.Now().Add(-24*time.Hour)).
		Count(&recentCompleted).Error
	if err != nil {
		return err
	}
	if recentCompleted > 0 {
		return nil
	}

	task := models.AdoptNarrowerQueue{
		AbstractTagID: abstractTagID,
		Source:        source,
		Status:        models.AdoptNarrowerQueueStatusPending,
	}
	if err := s.db.Create(&task).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "Duplicate") ||
			strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "UNIQUE") {
			return nil
		}
		return err
	}
	return nil
}

func (s *AdoptNarrowerQueueService) Start() {
	s.mu.Lock()
	if s.closed {
		s.closed = false
		s.stopCh = make(chan struct{})
	}
	s.mu.Unlock()

	result := s.db.Model(&models.AdoptNarrowerQueue{}).
		Where("status = ?", models.AdoptNarrowerQueueStatusProcessing).
		Updates(map[string]interface{}{
			"status":     models.AdoptNarrowerQueueStatusPending,
			"started_at": nil,
		})
	if result.Error != nil {
		s.logger.Error("failed to reset stale processing adopt narrower tasks", zap.Error(result.Error))
	} else if result.RowsAffected > 0 {
		s.logger.Info("reset stale processing adopt narrower tasks", zap.Int64("count", result.RowsAffected))
	}

	go s.worker()
	s.logger.Info("adopt narrower queue worker started")
}

func (s *AdoptNarrowerQueueService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.stopCh)
	s.logger.Info("adopt narrower queue worker stopped")
}

func (s *AdoptNarrowerQueueService) worker() {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("adopt narrower queue worker panic recovered", zap.Any("panic", r))
			time.Sleep(5 * time.Second)
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if !closed {
				go s.worker()
			}
		}
	}()

	ticker := time.NewTicker(5 * time.Second)
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

func (s *AdoptNarrowerQueueService) processNext() {
	var tasks []models.AdoptNarrowerQueue

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ?", models.AdoptNarrowerQueueStatusPending).
			Order("created_at ASC").
			Limit(1).
			Find(&tasks).Error; err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}

		now := time.Now()
		result := tx.Model(&models.AdoptNarrowerQueue{}).
			Where("id = ? AND status = ?", tasks[0].ID, models.AdoptNarrowerQueueStatusPending).
			Updates(map[string]interface{}{
				"status":     models.AdoptNarrowerQueueStatusProcessing,
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
		s.logger.Error("failed to claim adopt narrower task", zap.Error(err))
		return
	}

	if len(tasks) == 0 || tasks[0].ID == 0 {
		return
	}

	task := tasks[0]

	if err := adoptNarrowerAbstractChildren(context.Background(), task.AbstractTagID); err != nil {
		s.markFailed(task.ID, err.Error())
		return
	}

	now := time.Now()
	if err := s.db.Model(&models.AdoptNarrowerQueue{}).
		Where("id = ?", task.ID).
		Updates(map[string]interface{}{
			"status":       models.AdoptNarrowerQueueStatusCompleted,
			"completed_at": now,
		}).Error; err != nil {
		s.logger.Error("failed to mark adopt narrower task completed", zap.Uint("task_id", task.ID), zap.Error(err))
	}

	s.logger.Info("adopt narrower task completed",
		zap.Uint("abstract_tag_id", task.AbstractTagID),
		zap.String("source", task.Source))
}

func (s *AdoptNarrowerQueueService) markFailed(taskID uint, errMsg string) {
	now := time.Now()
	if err := s.db.Model(&models.AdoptNarrowerQueue{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":        models.AdoptNarrowerQueueStatusFailed,
			"error_message": errMsg,
			"completed_at":  now,
			"retry_count":   gorm.Expr("retry_count + 1"),
		}).Error; err != nil {
		s.logger.Error("failed to mark adopt narrower task failed", zap.Uint("task_id", taskID), zap.Error(err))
	}
	s.logger.Warn("adopt narrower task failed", zap.Uint("task_id", taskID), zap.String("error", errMsg))
}

func EnqueueAdoptNarrower(abstractTagID uint, source string) {
	if abstractTagID == 0 {
		return
	}
	if database.DB == nil {
		return
	}
	svc := getAdoptNarrowerQueueService()
	if err := svc.Enqueue(abstractTagID, source); err != nil {
		logging.Warnf("Failed to enqueue adopt narrower for tag %d: %v", abstractTagID, err)
	}
}
