package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"my-robot-backend/internal/domain/contentprocessing"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/topicextraction"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
	"my-robot-backend/internal/platform/tracing"
	"my-robot-backend/internal/platform/ws"
)

type FirecrawlScheduler struct {
	name              string
	checkInterval     int
	stopChan          chan struct{}
	wg                sync.WaitGroup
	executionMutex    sync.Mutex
	status            string
	nextRun           *time.Time
	lastExecutionTime *time.Time
	lastError         string
	concurrency       int
	queueSize         int32
	processingCount   int32
	queue             *contentprocessing.FirecrawlJobQueue
}

func NewFirecrawlScheduler() *FirecrawlScheduler {
	return &FirecrawlScheduler{
		name:          "Firecrawl Content Fetcher",
		checkInterval: 300,
		stopChan:      make(chan struct{}),
		status:        "idle",
		concurrency:   1,
		queue:         contentprocessing.NewFirecrawlJobQueue(database.DB),
	}
}

func (s *FirecrawlScheduler) Start() error {
	s.stopChan = make(chan struct{})
	nextRun := time.Now().Add(time.Duration(s.checkInterval) * time.Second)
	s.nextRun = &nextRun
	s.wg.Add(1)
	go s.run()
	logging.Infof("[Firecrawl] Scheduler started")
	return nil
}

func (s *FirecrawlScheduler) Stop() {
	close(s.stopChan)
	s.wg.Wait()
	s.nextRun = nil
	logging.Infof("[Firecrawl] Scheduler stopped")
}

func (s *FirecrawlScheduler) TriggerNow() map[string]interface{} {
	if !s.executionMutex.TryLock() {
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     "Firecrawl 正在执行中，稍后再试。",
			"status_code": http.StatusConflict,
		}
	}

	batchID := time.Now().Format("20060102150405")

	go s.runCrawlCycle(batchID)
	return map[string]interface{}{
		"accepted": true,
		"started":  true,
		"message":  "Firecrawl scheduler triggered",
		"batch_id": batchID,
	}
}

func (s *FirecrawlScheduler) ResetStats() error {
	s.lastExecutionTime = nil
	s.lastError = ""
	s.status = "idle"
	atomic.StoreInt32(&s.queueSize, 0)
	atomic.StoreInt32(&s.processingCount, 0)
	return nil
}

func (s *FirecrawlScheduler) UpdateInterval(interval int) error {
	if interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}

	s.checkInterval = interval
	s.Stop()
	return s.Start()
}

func (s *FirecrawlScheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(s.checkInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			nextRun := time.Now().Add(time.Duration(s.checkInterval) * time.Second)
			s.nextRun = &nextRun
			s.checkAndCrawl()
		case <-s.stopChan:
			return
		}
	}
}

func (s *FirecrawlScheduler) checkAndCrawl() {
	tracing.TraceSchedulerTick("firecrawl", "cron", func(ctx context.Context) {
		if !s.executionMutex.TryLock() {
			logging.Infof("[Firecrawl] Scheduler already running, skipping this cycle")
			return
		}

		batchID := time.Now().Format("20060102150405")
		s.runCrawlCycle(batchID)
	})
}

func (s *FirecrawlScheduler) runCrawlCycle(batchID string) {
	defer s.executionMutex.Unlock()

	startTime := time.Now()
	s.status = "running"
	s.lastExecutionTime = &startTime
	atomic.StoreInt32(&s.processingCount, 0)

	defer func() {
		s.status = "idle"
		atomic.StoreInt32(&s.queueSize, 0)
		atomic.StoreInt32(&s.processingCount, 0)
	}()

	config, err := contentprocessing.GetFirecrawlConfig()
	if err != nil {
		s.lastError = err.Error()
		logging.Errorf("[Firecrawl] Config error: %v", err)
		return
	}

	if !config.Enabled {
		return
	}

	firecrawlService := contentprocessing.NewFirecrawlService(config)

	jobs, err := s.queue.Claim(50, s.leaseDuration(config))
	if err != nil {
		s.lastError = err.Error()
		logging.Errorf("[Firecrawl] Claim error: %v", err)
		return
	}

	if len(jobs) == 0 {
		return
	}

	s.broadcastProgress(batchID, "processing", len(jobs), 0, 0, nil)

	s.setQueueSize(len(jobs))
	atomic.StoreInt32(&s.processingCount, 0)
	logging.Infof("[Firecrawl] Starting sequential processing of %d jobs (concurrency=1)", len(jobs))

	completed := 0
	failed := 0

	// 单线程串行处理，一个一个来
	for i := range jobs {
		job := jobs[i]

		var art models.Article
		if err := database.DB.Omit("tag_count", "relevance_score").First(&art, job.ArticleID).Error; err != nil {
			failed++
			_ = s.queue.MarkFailed(job, err.Error(), time.Minute)
			continue
		}

		var feed models.Feed
		if err := database.DB.First(&feed, art.FeedID).Error; err != nil {
			failed++
			database.DB.Model(&art).Updates(map[string]interface{}{
				"firecrawl_status": "failed",
				"firecrawl_error":  err.Error(),
			})
			s.broadcastProgress(batchID, "processing", len(jobs), completed, failed, &ws.FirecrawlArticleProgress{
				ID:     art.ID,
				Title:  art.Title,
				Status: "failed",
				Error:  err.Error(),
			})
			_ = s.queue.MarkFailed(job, err.Error(), time.Minute)
			continue
		}

		database.DB.Model(&art).Update("firecrawl_status", "processing")

		s.broadcastProgress(batchID, "processing", len(jobs), completed, failed, &ws.FirecrawlArticleProgress{
			ID:     art.ID,
			Title:  art.Title,
			Status: "processing",
		})

		result, crawlErr := firecrawlService.ScrapePage(context.Background(), art.Link)
		if crawlErr != nil {
			failed++
			database.DB.Model(&art).Updates(map[string]interface{}{
				"firecrawl_status": "failed",
				"firecrawl_error":  crawlErr.Error(),
			})
			_ = s.queue.MarkFailed(job, crawlErr.Error(), s.failureBackoff(job.AttemptCount))
			s.broadcastProgress(batchID, "processing", len(jobs), completed, failed, &ws.FirecrawlArticleProgress{
				ID:     art.ID,
				Title:  art.Title,
				Status: "failed",
				Error:  crawlErr.Error(),
			})
			logging.Errorf("[Firecrawl] Failed to crawl %s: %v", art.Link, crawlErr)
			continue
		}

		now := time.Now()
		updates := map[string]interface{}{
			"firecrawl_status":     "completed",
			"firecrawl_content":    result.Data.Markdown,
			"firecrawl_crawled_at": now,
		}
		if feed.ArticleSummaryEnabled {
			updates["summary_status"] = "incomplete"
		}
		database.DB.Model(&art).Updates(updates)

		if feed.TaggingEnabled {
			if err := topicextraction.NewTagJobQueue(database.DB).Enqueue(topicextraction.TagJobRequest{
				ArticleID:    art.ID,
				FeedName:     feed.Title,
				CategoryName: topicextraction.FeedCategoryName(feed),
				ForceRetag:   true,
				Reason:       "firecrawl_completed",
			}); err != nil {
				failed++
				_ = s.queue.MarkFailed(job, err.Error(), time.Minute)
				logging.Warnf("[Firecrawl] Failed to enqueue retag for article %d after crawl: %v", art.ID, err)
				continue
			}
		}

		if err := s.queue.MarkCompleted(job.ID); err != nil {
			failed++
			logging.Errorf("[Firecrawl] Failed to mark job %d completed: %v", job.ID, err)
			continue
		}

		completed++
		s.broadcastProgress(batchID, "processing", len(jobs), completed, failed, &ws.FirecrawlArticleProgress{
			ID:     art.ID,
			Title:  art.Title,
			Status: "completed",
		})

		// 更新队列状态
		s.setQueueSize(len(jobs) - completed - failed)
		atomic.StoreInt32(&s.processingCount, 0)

		// 每次处理完一个后稍微停顿一下，避免对目标站点造成压力
		time.Sleep(500 * time.Millisecond)
	}

	atomic.StoreInt32(&s.queueSize, 0)
	atomic.StoreInt32(&s.processingCount, 0)

	duration := time.Since(startTime).Seconds()
	logging.Infof("[Firecrawl] Sequential crawl completed: %d completed, %d failed out of %d jobs in %.2fs", completed, failed, len(jobs), duration)

	s.broadcastProgress(batchID, "completed", len(jobs), completed, failed, nil)
	s.lastError = ""
}

func (s *FirecrawlScheduler) setQueueSize(n int) {
	if n > math.MaxInt32 {
		atomic.StoreInt32(&s.queueSize, math.MaxInt32)
	} else {
		atomic.StoreInt32(&s.queueSize, int32(n)) //nolint:gosec
	}
}

func (s *FirecrawlScheduler) leaseDuration(config *contentprocessing.FirecrawlConfig) time.Duration {
	timeout := time.Duration(config.Timeout) * time.Second
	if timeout < 5*time.Minute {
		timeout = 5 * time.Minute
	}
	return timeout + 5*time.Minute
}

func (s *FirecrawlScheduler) failureBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return time.Minute
	}
	backoff := time.Duration(1<<min(attempt-1, 4)) * time.Minute
	if backoff > 30*time.Minute {
		return 30 * time.Minute
	}
	return backoff
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *FirecrawlScheduler) broadcastProgress(batchID, status string, total, completed, failed int, current *ws.FirecrawlArticleProgress) {
	hub := ws.GetHub()
	msg := ws.FirecrawlProgressMessage{
		Type:      "firecrawl_progress",
		BatchID:   batchID,
		Status:    status,
		Total:     total,
		Completed: completed,
		Failed:    failed,
		Current:   current,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		logging.Warnf("[Firecrawl] Failed to marshal progress: %v", err)
		return
	}
	hub.BroadcastRaw(data)
}

func (s *FirecrawlScheduler) GetStatus() SchedulerStatusResponse {
	return SchedulerStatusResponse{
		Name:          "Firecrawl Crawler",
		Status:        s.status,
		CheckInterval: int64(s.checkInterval),
		NextRun:       optionalTimeToUnix(s.nextRun),
		IsExecuting:   s.status == "running",
	}
}

func (s *FirecrawlScheduler) GetTaskStatusDetails() map[string]interface{} {
	return map[string]interface{}{
		"status":              s.status,
		"check_interval":      s.checkInterval,
		"next_run":            s.nextRun,
		"is_executing":        s.status == "running",
		"last_execution_time": s.lastExecutionTime,
		"last_error":          s.lastError,
		"concurrency":         s.concurrency,
		"queue_size":          atomic.LoadInt32(&s.queueSize),
		"processing":          atomic.LoadInt32(&s.processingCount),
	}
}

func (s *FirecrawlScheduler) Trigger() {
	logging.Infof("[Firecrawl] Manual trigger received")
	go s.checkAndCrawl()
}
