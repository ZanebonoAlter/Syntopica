package tagging

import (
	"context"
	"sync"
	"time"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// EmbeddingQueueService manages the embedding generation queue.
type EmbeddingQueueService struct {
	db        *gorm.DB
	embedding *EmbeddingService
	logger    *zap.Logger

	mu     sync.Mutex
	closed bool
	stopCh chan struct{}
}

// NewEmbeddingQueueService creates a new embedding queue service.
func NewEmbeddingQueueService(logger *zap.Logger) *EmbeddingQueueService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &EmbeddingQueueService{
		db:        database.DB,
		embedding: NewEmbeddingService(),
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

// Enqueue creates a new embedding queue task for the given tag.
// Skips if there is already a pending/processing task for the same tag,
// or if the tag already has an embedding with a matching text hash.
func (s *EmbeddingQueueService) Enqueue(tagID uint) error {
	var activeCount int64
	err := s.db.Model(&models.EmbeddingQueue{}).
		Where("tag_id = ? AND status IN ?", tagID, []string{
			models.EmbeddingQueueStatusPending,
			models.EmbeddingQueueStatusProcessing,
		}).Count(&activeCount).Error
	if err != nil {
		return err
	}
	if activeCount > 0 {
		return nil
	}

	var tag models.TopicTag
	if err := s.db.First(&tag, tagID).Error; err != nil {
		return err
	}

	if s.allEmbeddingsCurrent(&tag) {
		return nil
	}

	task := models.EmbeddingQueue{
		TagID:  tagID,
		Status: models.EmbeddingQueueStatusPending,
	}
	return s.db.Create(&task).Error
}

func (s *EmbeddingQueueService) allEmbeddingsCurrent(tag *models.TopicTag) bool {
	var identityEmb models.TopicTagEmbedding
	if err := s.db.Where("topic_tag_id = ? AND embedding_type = ?", tag.ID, EmbeddingTypeIdentity).First(&identityEmb).Error; err != nil {
		return false
	}
	identityHash := hashText(EmbeddingTypeIdentity + "\n" + buildTagEmbeddingText(tag, EmbeddingTypeIdentity))
	if identityEmb.TextHash != identityHash {
		return false
	}

	var semanticEmb models.TopicTagEmbedding
	if err := s.db.Where("topic_tag_id = ? AND embedding_type = ?", tag.ID, EmbeddingTypeSemantic).First(&semanticEmb).Error; err != nil {
		return false
	}
	semanticHash := hashText(EmbeddingTypeSemantic + "\n" + buildTagEmbeddingText(tag, EmbeddingTypeSemantic))
	if semanticEmb.TextHash != semanticHash {
		return false
	}

	if tag.Category == "event" {
		keywords := getEventKeywords(tag)
		for _, kw := range keywords {
			kwHash := hashText(EmbeddingTypeEventKeyword + "\n" + kw)
			var kwCount int64
			s.db.Model(&models.TopicTagEmbedding{}).
				Where("topic_tag_id = ? AND embedding_type = ? AND text_hash = ?", tag.ID, EmbeddingTypeEventKeyword, kwHash).
				Count(&kwCount)
			if kwCount == 0 {
				return false
			}
		}
	}

	return true
}

// GetStatus returns a count of tasks grouped by status.
func (s *EmbeddingQueueService) GetStatus() (map[string]int64, error) {
	type statusRow struct {
		Status string
		Count  int64
	}
	var rows []statusRow
	err := s.db.Model(&models.EmbeddingQueue{}).
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
	for _, r := range rows {
		result[r.Status] = r.Count
		total += r.Count
	}
	result["total"] = total
	return result, nil
}

// GetTasks returns tasks filtered by status with pagination.
// Preloads the associated Tag information.
func (s *EmbeddingQueueService) GetTasks(status string, limit, offset int) ([]models.EmbeddingQueue, int64, error) {
	query := s.db.Model(&models.EmbeddingQueue{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var tasks []models.EmbeddingQueue
	err := query.Preload("Tag").
		Order("created_at ASC").
		Limit(limit).
		Offset(offset).
		Find(&tasks).Error
	if err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

// RetryFailed resets all failed tasks back to pending status.
func (s *EmbeddingQueueService) RetryFailed() (int64, error) {
	result := s.db.Model(&models.EmbeddingQueue{}).
		Where("status = ?", models.EmbeddingQueueStatusFailed).
		Updates(map[string]interface{}{
			"status":        models.EmbeddingQueueStatusPending,
			"error_message": "",
			"started_at":    nil,
			"completed_at":  nil,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// Start begins the background worker that processes pending embedding tasks.
func (s *EmbeddingQueueService) Start() {
	s.mu.Lock()
	if s.closed {
		s.closed = false
		s.stopCh = make(chan struct{})
	}
	s.mu.Unlock()

	result := s.db.Model(&models.EmbeddingQueue{}).
		Where("status = ?", models.EmbeddingQueueStatusProcessing).
		Updates(map[string]interface{}{
			"status":     models.EmbeddingQueueStatusPending,
			"started_at": nil,
		})
	if result.Error != nil {
		s.logger.Error("failed to reset stale processing embedding tasks", zap.Error(result.Error))
	} else if result.RowsAffected > 0 {
		s.logger.Info("reset stale processing embedding tasks", zap.Int64("count", result.RowsAffected))
	}

	go s.worker()
	s.logger.Info("embedding queue worker started")
}

// Stop gracefully stops the background worker.
func (s *EmbeddingQueueService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.stopCh)
	s.logger.Info("embedding queue worker stopped")
}

func (s *EmbeddingQueueService) worker() {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("embedding queue worker panic recovered", zap.Any("panic", r))
			// Restart worker after a delay
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

func (s *EmbeddingQueueService) processNext() {
	var task models.EmbeddingQueue

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Lock a pending task using SELECT FOR UPDATE
		if err := tx.Raw(
			"SELECT * FROM embedding_queues WHERE status = ? ORDER BY created_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED",
			models.EmbeddingQueueStatusPending,
		).Scan(&task).Error; err != nil {
			return err
		}

		if task.ID == 0 {
			return nil // no pending tasks
		}

		now := time.Now()
		return tx.Model(&task).Updates(map[string]interface{}{
			"status":     models.EmbeddingQueueStatusProcessing,
			"started_at": now,
		}).Error
	})

	if err != nil {
		s.logger.Error("failed to lock embedding task", zap.Error(err))
		return
	}

	if task.ID == 0 {
		return // nothing to process
	}

	// Load the tag
	var tag models.TopicTag
	if err := s.db.First(&tag, task.TagID).Error; err != nil {
		s.markFailed(task.ID, "failed to load tag: "+err.Error())
		return
	}

	// Generate and save embedding
	ctx := context.Background()
	identityEmb, err := s.embedding.GenerateEmbedding(ctx, &tag, EmbeddingTypeIdentity)
	if err != nil {
		s.markFailed(task.ID, "failed to generate identity embedding: "+err.Error())
		return
	}

	if err := s.embedding.SaveEmbedding(identityEmb); err != nil {
		s.markFailed(task.ID, "failed to save identity embedding: "+err.Error())
		return
	}

	semanticEmb, semErr := s.embedding.GenerateEmbedding(ctx, &tag, EmbeddingTypeSemantic)
	if semErr != nil {
		s.markFailed(task.ID, "failed to generate semantic embedding: "+semErr.Error())
		return
	}

	if err := s.embedding.SaveEmbedding(semanticEmb); err != nil {
		s.markFailed(task.ID, "failed to save semantic embedding: "+semErr.Error())
		return
	}

	if tag.Category == "event" {
		s.db.Where("topic_tag_id = ? AND embedding_type = ?", tag.ID, EmbeddingTypeEventKeyword).Delete(&models.TopicTagEmbedding{})

		keywords := getEventKeywords(&tag)
		for _, kw := range keywords {
			kwEmb, kwErr := s.embedding.GenerateEmbeddingForText(ctx, tag.ID, EmbeddingTypeEventKeyword, kw)
			if kwErr != nil {
				s.logger.Warn("failed to generate keyword embedding", zap.Uint("tag_id", tag.ID), zap.String("keyword", kw), zap.Error(kwErr))
				continue
			}
			if err := s.embedding.SaveEmbedding(kwEmb); err != nil {
				s.logger.Warn("failed to save keyword embedding", zap.Uint("tag_id", tag.ID), zap.String("keyword", kw), zap.Error(err))
				continue
			}
		}
		s.logger.Info("event keyword embeddings generated", zap.Uint("tag_id", tag.ID), zap.Int("keyword_count", len(keywords)))
	}

	// Auto-trigger board matching for event tags after embedding completion
	if tag.Category == "event" {
		if matcher := getSemanticBoardMatchingService(); matcher != nil {
			if _, matchErr := matcher.MatchTopicTag(ctx, tag.ID); matchErr != nil {
				s.logger.Warn("auto board match failed", zap.Uint("tag_id", tag.ID), zap.Error(matchErr))
			}
		}
	}

	// Mark completed
	now := time.Now()
	if err := s.db.Model(&models.EmbeddingQueue{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":       models.EmbeddingQueueStatusCompleted,
		"completed_at": now,
	}).Error; err != nil {
		s.logger.Error("failed to mark embedding task completed", zap.Uint("task_id", task.ID), zap.Error(err))
	}

	s.logger.Info("embedding generated", zap.Uint("tag_id", task.TagID))
}

func (s *EmbeddingQueueService) markFailed(taskID uint, errMsg string) {
	now := time.Now()
	if err := s.db.Model(&models.EmbeddingQueue{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":        models.EmbeddingQueueStatusFailed,
		"error_message": errMsg,
		"completed_at":  now,
		"retry_count":   gorm.Expr("retry_count + 1"),
	}).Error; err != nil {
		s.logger.Error("failed to mark embedding task failed", zap.Uint("task_id", taskID), zap.Error(err))
	}
	s.logger.Warn("embedding task failed", zap.Uint("task_id", taskID), zap.String("error", errMsg))
}
