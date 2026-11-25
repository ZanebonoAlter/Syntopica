package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/analysispause"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/reader/repository"
)

// ============================================================================
// 疑似 AI 判定 worker（add-ai-generated-article-detection design D1 / D2 / D10）
// ============================================================================
//
// 与标签打标 worker 完全独立：独立队列、独立 goroutine 循环、独立失败路径。
// 一次判定 = 一篇文章 = 一次 LLM 调用：领取一批作业后**逐篇**串行处理，不攒批
// （design D2 的取舍：批量会引入批内判定互相污染）。
//
// 租约与批量参数：单模型并发配额只有 2（airouter 默认），worker 自己不必再加并发；
// 批量 8 × 单篇最坏 60s ≈ 8min，仍小于 10min 租约——正常路径上不会出现「自己没干完
// 就被别人重新领取」。真卡死时租约过期由 Claim 回收重领。

const (
	// aiDetectionPollInterval 是空队列时的轮询间隔。判定池实测约 98 篇/天，
	// 2s 轮询的开销只是每分钟一次 count 级查询；换来显式重判近乎实时。
	aiDetectionPollInterval = 2 * time.Second
	// aiDetectionBatchSize 是一次 claim 的作业数（串行处理，不并发）。
	aiDetectionBatchSize = 8
	// aiDetectionLease 是单批作业的租约时长（见上方参数推算）。
	aiDetectionLease = 10 * time.Minute
)

// AIDetectionWorker 消费 detection_jobs 队列。
type AIDetectionWorker struct {
	mu       sync.Mutex
	stopChan chan struct{}
	wg       sync.WaitGroup
	started  bool

	service *AIDetectionService
	queue   *repository.DetectionJobQueue
}

func newAIDetectionWorker(queue *repository.DetectionJobQueue, svc *AIDetectionService) *AIDetectionWorker {
	return &AIDetectionWorker{
		stopChan: make(chan struct{}),
		service:  svc,
		queue:    queue,
	}
}

var (
	aiDetectionWorkerOnce sync.Once
	aiDetectionWorkerInst *AIDetectionWorker
)

// GetAIDetectionWorker 返回判定 worker 单例。
func GetAIDetectionWorker() *AIDetectionWorker {
	aiDetectionWorkerOnce.Do(func() {
		aiDetectionWorkerInst = newAIDetectionWorker(nil, NewAIDetectionService())
	})
	return aiDetectionWorkerInst
}

// StartAIDetectionWorker 启动判定 worker（应用启动时由 runtime 调用）。
// 数据库未就绪时返回错误，由调用方记日志——不阻塞应用启动。
func StartAIDetectionWorker() error {
	return GetAIDetectionWorker().Start()
}

// StopAIDetectionWorker 停止判定 worker（优雅关停，DB 释放前调用）。
func StopAIDetectionWorker() {
	GetAIDetectionWorker().Stop()
}

// Start 启动轮询循环。重复启动返回错误；DB 未初始化返回错误。
func (w *AIDetectionWorker) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return errors.New("ai detection worker already started")
	}
	if w.queue == nil {
		if repository.Repo == nil || repository.Repo.DB() == nil {
			return errors.New("ai detection worker: database is not initialized")
		}
		w.queue = repository.NewDetectionJobQueue(repository.Repo.DB())
	}
	w.stopChan = make(chan struct{})
	w.started = true
	w.wg.Add(1)
	go w.run()
	return nil
}

// Stop 停止轮询循环并等待当前批次收尾。未启动时是 no-op。
func (w *AIDetectionWorker) Stop() {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	w.started = false
	close(w.stopChan)
	w.mu.Unlock()

	w.wg.Wait()
	logging.Infoln("[AIDetection] worker stopped")
}

func (w *AIDetectionWorker) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(aiDetectionPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			// 全局分析暂停（用户开关或模型层未就绪）时不领取作业：判定属分析类
			// 工作，与打标/嵌入队列同款口径。
			if analysispause.IsPaused() {
				continue
			}
			w.processBatch()
		}
	}
}

// processBatch 领取一批作业并逐篇处理（串行，每篇一次独立判定调用）。
func (w *AIDetectionWorker) processBatch() {
	if w.queue == nil || w.service == nil {
		return
	}

	jobs, err := w.queue.Claim(aiDetectionBatchSize, aiDetectionLease)
	if err != nil {
		logging.Warnf("[AIDetection] Failed to claim detection jobs: %v", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	for i := range jobs {
		select {
		case <-w.stopChan:
			return
		default:
		}
		w.processJob(context.Background(), jobs[i])
	}
}

// processJob 处理单篇判定作业。任何失败都只影响本作业（文章保持未判定），
// 不影响打标链路。
func (w *AIDetectionWorker) processJob(ctx context.Context, job models.DetectionJob) {
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("panic: %v", r)
			logging.Errorf("[AIDetection] Panic while judging article %d: %v", job.ArticleID, r)
			_ = w.queue.MarkFailed(job, msg, aiDetectionFailureBackoff(job.AttemptCount))
		}
	}()

	if repository.Repo == nil || repository.Repo.DB() == nil {
		logging.Warnf("[AIDetection] Skipped job %d: repository is not initialized", job.ID)
		_ = w.queue.MarkFailed(job, "repository is not initialized", aiDetectionFailureBackoff(job.AttemptCount))
		return
	}

	// tag_count / relevance_score 是只读计算列（gorm:"->"），查询时必须 Omit，
	// 否则 SELECT 会带上不存在的列。
	var article models.Article
	if err := repository.Repo.DB().Omit("tag_count", "relevance_score").First(&article, job.ArticleID).Error; err != nil {
		logging.Warnf("[AIDetection] Failed to load article %d for detection: %v", job.ArticleID, err)
		_ = w.queue.MarkFailed(job, err.Error(), aiDetectionFailureBackoff(job.AttemptCount))
		return
	}

	// 人工结论优先（design D10）：已有人工标记/人工取消的文章不做自动判定——
	// 不调 LLM、不覆盖结论，作业直接收尾。显式重判（manual_recheck）绕过此跳过。
	force := repository.IsForceDetectionReason(job.Reason)
	if !force && HasManualAIDetection(&article) {
		if err := w.queue.MarkCompleted(job.ID); err != nil {
			logging.Warnf("[AIDetection] Failed to mark skipped job %d completed: %v", job.ID, err)
			return
		}
		logging.Infof("[AIDetection] Skipped article %d: manual conclusion takes precedence", article.ID)
		return
	}

	result, err := w.service.DetectAndPersist(ctx, &article, force)
	if err != nil {
		if errors.Is(err, ErrAIDetectionManualOverride) {
			// 服务层二次保险（正常路径已在上方跳过）：按「跳过、非失败」处理。
			_ = w.queue.MarkCompleted(job.ID)
			return
		}
		logging.Warnf("[AIDetection] Detection failed for article %d: %v", article.ID, err)
		if markErr := w.queue.MarkFailed(job, err.Error(), aiDetectionFailureBackoff(job.AttemptCount)); markErr != nil {
			logging.Warnf("[AIDetection] Failed to mark job %d failed: %v", job.ID, markErr)
		}
		return
	}

	if err := w.queue.MarkCompleted(job.ID); err != nil {
		logging.Warnf("[AIDetection] Failed to mark job %d completed: %v", job.ID, err)
		return
	}
	logging.Infof("[AIDetection] Article %d judged: score=%.2f status=%s source=%s input=%s threshold=%.2f",
		article.ID, result.Score, result.Status, result.Source, result.InputSource, result.Threshold)
}

// aiDetectionFailureBackoff 与打标队列同款指数退避（1min 起，上限 30min）。
func aiDetectionFailureBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return time.Minute
	}
	backoff := time.Duration(1<<min(attempt-1, 4)) * time.Minute
	if backoff > 30*time.Minute {
		return 30 * time.Minute
	}
	return backoff
}
