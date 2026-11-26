package core

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/analysispause"

	"go.uber.org/zap"
	"gorm.io/gorm"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

// 队列 worker 参数（speed-up-tag-embedding design D2，硬编码对齐仓库现状）。
const (
	// embeddingWorkerCount 并发 worker 数：qwen3-embedding 自托管，4 并发在承载范围。
	embeddingWorkerCount = 4
	// embeddingIdleBackoff 空队列退避：去 2s ticker 空转（实测 38% 时间在轮询）。
	embeddingIdleBackoff = 200 * time.Millisecond
	// embeddingBatchMax 单批任务上限（对齐 auxlabel 路径 batch=32）。
	embeddingBatchMax = 32
	// embeddingRetryBaseDelay / embeddingMaxRetries 失败退避（D4）：
	// retry_count<3 时 next_retry_at = now + 30s*2^(retry-1)，第 4 次失败终态 failed。
	embeddingRetryBaseDelay = 30 * time.Second
	embeddingMaxRetries     = 3
)

// isPaused 由 worker 每5轮询一次；抽成变量便于测试注入（testcontainer 环境没有
// aihealth 探针，Healthy() 恒 false 会让 worker 永久暂停）。
var isAnalysisPaused = analysispause.IsPaused

// tagEmbedder 抽象 worker 需要的 embedding 生成能力，便于测试注入 fake
// provider（同包测试可直接替换 svc.embedding 字段）。
type tagEmbedder interface {
	GenerateEmbeddingsBatch(ctx context.Context, tags []*models.TopicTag, embeddingType string) ([]*tagmodels.TopicTagEmbedding, error)
	GenerateEmbeddingForText(ctx context.Context, tagID uint, embeddingType string, text string) (*tagmodels.TopicTagEmbedding, []float64, error)
	SaveEmbedding(embedding *tagmodels.TopicTagEmbedding) error
}

// EmbeddingQueueService manages the embedding generation queue.
type EmbeddingQueueService struct {
	db        *gorm.DB
	embedding tagEmbedder
	logger    *zap.Logger

	mu     sync.Mutex
	closed bool
	stopCh chan struct{}
	wg     sync.WaitGroup
	// workerGauge 当前活跃 worker 数（可观测 + Stop/Start 竞态测试断言用）。
	workerGauge atomic.Int32
}

// NewEmbeddingQueueService creates a new embedding queue service.
func NewEmbeddingQueueService(logger *zap.Logger) *EmbeddingQueueService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &EmbeddingQueueService{
		db:        repository.Repo.DB(),
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
	var identityEmb tagmodels.TopicTagEmbedding
	if err := s.db.Where("topic_tag_id = ? AND embedding_type = ?", tag.ID, EmbeddingTypeIdentity).First(&identityEmb).Error; err != nil {
		return false
	}
	identityHash := hashText(EmbeddingTypeIdentity + "\n" + buildTagEmbeddingText(tag, EmbeddingTypeIdentity))
	if identityEmb.TextHash != identityHash {
		return false
	}

	var semanticEmb tagmodels.TopicTagEmbedding
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
			s.db.Model(&tagmodels.TopicTagEmbedding{}).
				Where("topic_tag_id = ? AND embedding_type = ? AND text_hash = ?", tag.ID, EmbeddingTypeEventKeyword, kwHash).
				Count(&kwCount)
			if kwCount == 0 {
				return false
			}
		}
	}

	return true
}

// GetStatus returns a count of tasks grouped by status, plus observability
// fields (speed-up-tag-embedding D5): oldest_pending_age_seconds（最老 pending
// 任务年龄，秒；无 pending 时为 null）与 throughput_per_min（最近 60s 完成数）。
func (s *EmbeddingQueueService) GetStatus() (map[string]any, error) {
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

	result := map[string]any{
		"pending":                    int64(0),
		"processing":                 int64(0),
		"completed":                  int64(0),
		"failed":                     int64(0),
		"total":                      int64(0),
		"oldest_pending_age_seconds": nil,
		"throughput_per_min":         int64(0),
	}
	var total int64
	for _, r := range rows {
		result[r.Status] = r.Count
		total += r.Count
	}
	result["total"] = total

	var oldest sql.NullFloat64
	if err := s.db.Raw(
		`SELECT EXTRACT(EPOCH FROM (now() - MIN(created_at))) FROM embedding_queues WHERE status = ?`,
		models.EmbeddingQueueStatusPending,
	).Row().Scan(&oldest); err == nil && oldest.Valid {
		result["oldest_pending_age_seconds"] = int64(oldest.Float64)
	}

	var throughput int64
	if err := s.db.Model(&models.EmbeddingQueue{}).
		Where("status = ? AND completed_at >= ?", models.EmbeddingQueueStatusCompleted, time.Now().Add(-time.Minute)).
		Count(&throughput).Error; err == nil {
		result["throughput_per_min"] = throughput
	}
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
// Failed tasks whose tag no longer exists are dropped first — they can never
// succeed and would otherwise bounce between failed/pending forever.
func (s *EmbeddingQueueService) RetryFailed() (int64, error) {
	if err := s.dropOrphanFailedTasks(); err != nil {
		return 0, err
	}
	result := s.db.Model(&models.EmbeddingQueue{}).
		Where("status = ?", models.EmbeddingQueueStatusFailed).
		Updates(map[string]interface{}{
			"status":        models.EmbeddingQueueStatusPending,
			"error_message": "",
			"started_at":    nil,
			"completed_at":  nil,
			"next_retry_at": nil,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// dropTaskForMissingTag deletes a queue task whose tag no longer exists.
// Retrying can never succeed, so the row is dropped instead of markFailed.
func (s *EmbeddingQueueService) dropTaskForMissingTag(taskID, tagID uint) {
	if err := s.db.Delete(&models.EmbeddingQueue{}, taskID).Error; err != nil {
		s.logger.Error("failed to drop embedding task for missing tag",
			zap.Uint("task_id", taskID), zap.Uint("tag_id", tagID), zap.Error(err))
		return
	}
	s.logger.Warn("dropped embedding task: tag no longer exists",
		zap.Uint("task_id", taskID), zap.Uint("tag_id", tagID))
}

// dropOrphanFailedTasks deletes failed tasks whose tag is gone (FKs are
// intentionally dropped by migration, so orphan rows are allowed to exist).
func (s *EmbeddingQueueService) dropOrphanFailedTasks() error {
	res := s.db.Where(
		"status = ? AND NOT EXISTS (SELECT 1 FROM topic_tags t WHERE t.id = embedding_queues.tag_id)",
		models.EmbeddingQueueStatusFailed,
	).Delete(&models.EmbeddingQueue{})
	if res.Error != nil {
		s.logger.Error("failed to delete orphan failed embedding tasks", zap.Error(res.Error))
		return res.Error
	}
	if res.RowsAffected > 0 {
		s.logger.Info("deleted failed embedding tasks with missing tag", zap.Int64("count", res.RowsAffected))
	}
	return nil
}

// Start begins the background workers that process pending embedding tasks.
// 修 Stop/Start 竞态（D2）：先等旧 worker 全部退出（wg 清零）再拉新 worker，
// 保证任何时刻最多 embeddingWorkerCount 个 goroutine 抢队列。
func (s *EmbeddingQueueService) Start() {
	s.wg.Wait() // assert no residual workers from a previous incarnation

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

	for i := 0; i < embeddingWorkerCount; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	s.logger.Info("embedding queue workers started", zap.Int("workers", embeddingWorkerCount))
}

// Stop gracefully stops the background workers and waits for all of them to
// exit before returning（修 stopCh 竞态：旧代码 close 后不等 goroutine 退出，
// Start 重建字段会让旧 worker 读新 channel 永不退出）。
func (s *EmbeddingQueueService) Stop() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
	} else {
		s.closed = true
		close(s.stopCh)
		s.mu.Unlock()
	}
	s.wg.Wait()
	s.logger.Info("embedding queue workers stopped", zap.Int32("workers", s.workerGauge.Load()))
}

func (s *EmbeddingQueueService) worker() {
	defer s.wg.Done()
	s.workerGauge.Add(1)
	defer s.workerGauge.Add(-1)

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}
		if isAnalysisPaused() {
			select {
			case <-s.stopCh:
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		if !s.processNext() {
			// 空队列短退避：不再 2s ticker 空转打 DB。
			select {
			case <-s.stopCh:
				return
			case <-time.After(embeddingIdleBackoff):
			}
		}
	}
}

// processNext claims a batch of pending tasks and runs it. Returns false when
// nothing was claimable（含全部任务退避未到期）—— worker 据此退避。panic 由
// 此处 recover，worker 循环继续（不再靠 goroutine 重启，wg 计数不乱）。
func (s *EmbeddingQueueService) processNext() bool {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("embedding batch panic recovered", zap.Any("panic", r))
		}
	}()

	tasks := s.claimBatch(embeddingBatchMax)
	if len(tasks) == 0 {
		return false
	}
	s.processTasks(context.Background(), tasks)
	return true
}

// claimBatch atomically claims up to max pending-and-due tasks with
// FOR UPDATE SKIP LOCKED（沿用原 claim 事务，多 worker 不重领）；
// next_retry_at 未到期的任务不领取（D4）。
func (s *EmbeddingQueueService) claimBatch(max int) []models.EmbeddingQueue {
	var tasks []models.EmbeddingQueue
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(
			"SELECT * FROM embedding_queues WHERE status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?) ORDER BY created_at ASC LIMIT ? FOR UPDATE SKIP LOCKED",
			models.EmbeddingQueueStatusPending, time.Now(), max,
		).Scan(&tasks).Error; err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}
		ids := make([]uint, len(tasks))
		for i, t := range tasks {
			ids[i] = t.ID
		}
		now := time.Now()
		return tx.Model(&models.EmbeddingQueue{}).Where("id IN ?", ids).Updates(map[string]interface{}{
			"status":     models.EmbeddingQueueStatusProcessing,
			"started_at": now,
		}).Error
	})
	if err != nil {
		s.logger.Error("failed to claim embedding tasks", zap.Error(err))
		return nil
	}
	return tasks
}

// processTasks runs one claimed batch: same-type texts go to the provider in
// one batch call (identity + semantic 各一次，事件关键词沿用单条路径)，然后
// 逐条落库/标记——中途失败只影响批内后续，已落库不回滚（幂等：唯一索引兜底）。
func (s *EmbeddingQueueService) processTasks(ctx context.Context, tasks []models.EmbeddingQueue) {
	// Load tags first; missing tags are permanent → drop the task.
	tags := make(map[uint]*models.TopicTag, len(tasks))
	ordered := make([]*models.TopicTag, 0, len(tasks))
	for _, task := range tasks {
		if _, ok := tags[task.TagID]; ok {
			continue
		}
		var tag models.TopicTag
		if err := s.db.First(&tag, task.TagID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				s.dropTaskForMissingTag(task.ID, task.TagID)
			} else {
				s.markFailed(task.ID, "failed to load tag: "+err.Error())
			}
			continue
		}
		t := tag
		tags[task.TagID] = &t
		ordered = append(ordered, &t)
	}
	if len(ordered) == 0 {
		return
	}

	// 同类型一次批量调用（D2/D3），缓存命中由 Router.Embed 批量路径按单条 key 处理。
	idEmbs, idErr := s.embedding.GenerateEmbeddingsBatch(ctx, ordered, EmbeddingTypeIdentity)
	semEmbs, semErr := s.embedding.GenerateEmbeddingsBatch(ctx, ordered, EmbeddingTypeSemantic)
	idByTag := make(map[uint]*tagmodels.TopicTagEmbedding, len(idEmbs))
	for _, e := range idEmbs {
		idByTag[e.TopicTagID] = e
	}
	semByTag := make(map[uint]*tagmodels.TopicTagEmbedding, len(semEmbs))
	for _, e := range semEmbs {
		semByTag[e.TopicTagID] = e
	}

	for _, task := range tasks {
		tag, ok := tags[task.TagID]
		if !ok {
			continue // already dropped / markFailed during load
		}
		if idErr != nil {
			s.markFailed(task.ID, "failed to generate identity embedding: "+idErr.Error())
			continue
		}
		identityEmb, ok2 := idByTag[tag.ID]
		if !ok2 {
			s.markFailed(task.ID, "identity embedding missing from batch result")
			continue
		}
		if err := s.embedding.SaveEmbedding(identityEmb); err != nil {
			s.markFailed(task.ID, "failed to save identity embedding: "+err.Error())
			continue
		}

		if semErr != nil {
			s.markFailed(task.ID, "failed to generate semantic embedding: "+semErr.Error())
			continue
		}
		semanticEmb, ok3 := semByTag[tag.ID]
		if !ok3 {
			s.markFailed(task.ID, "semantic embedding missing from batch result")
			continue
		}
		if err := s.embedding.SaveEmbedding(semanticEmb); err != nil {
			s.markFailed(task.ID, "failed to save semantic embedding: "+err.Error())
			continue
		}

		if tag.Category == "event" {
			s.db.Where("topic_tag_id = ? AND embedding_type = ?", tag.ID, EmbeddingTypeEventKeyword).Delete(&tagmodels.TopicTagEmbedding{})

			keywords := getEventKeywords(tag)
			for _, kw := range keywords {
				kwEmb, _, kwErr := s.embedding.GenerateEmbeddingForText(ctx, tag.ID, EmbeddingTypeEventKeyword, kw)
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

	// Auto-trigger board matching for event tags after embedding completion
	for _, tag := range ordered {
		if tag.Category == "event" {
			if matcher := BoardMatcherFactory(); matcher != nil {
				if _, matchErr := matcher.MatchTopicTag(ctx, tag.ID); matchErr != nil {
					s.logger.Warn("auto board match failed", zap.Uint("tag_id", tag.ID), zap.Error(matchErr))
				}
			}
		}
	}
}

// markFailed records a task failure. retry_count < embeddingMaxRetries 时回
// pending 并设 next_retry_at = now + 30s*2^(retry-1)（指数退避，D4）；达到上限
// 置终态 failed（保留手动 POST /retry 接口）。
func (s *EmbeddingQueueService) markFailed(taskID uint, errMsg string) {
	var task models.EmbeddingQueue
	if err := s.db.Select("id", "retry_count").First(&task, taskID).Error; err != nil {
		s.logger.Error("failed to load task for markFailed", zap.Uint("task_id", taskID), zap.Error(err))
		return
	}
	retry := task.RetryCount + 1
	now := time.Now()
	updates := map[string]interface{}{
		"error_message": errMsg,
		"retry_count":   retry,
	}
	if retry <= embeddingMaxRetries {
		delay := embeddingRetryBaseDelay << (retry - 1)
		updates["status"] = models.EmbeddingQueueStatusPending
		updates["next_retry_at"] = now.Add(delay)
	} else {
		updates["status"] = models.EmbeddingQueueStatusFailed
		updates["completed_at"] = now
	}
	if err := s.db.Model(&models.EmbeddingQueue{}).Where("id = ?", taskID).Updates(updates).Error; err != nil {
		s.logger.Error("failed to mark embedding task failed", zap.Uint("task_id", taskID), zap.Error(err))
	}
	s.logger.Warn("embedding task failed", zap.Uint("task_id", taskID), zap.String("error", errMsg), zap.Int("retry_count", retry))
}
