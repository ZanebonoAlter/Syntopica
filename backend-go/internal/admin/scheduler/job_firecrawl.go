package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/ws"
	content "syntopica-backend/internal/reader"
	tagging "syntopica-backend/internal/tagmanagement"
)

// firecrawlWorkerCount is the fixed worker-pool size for parallel crawl
// processing (design D4). Concurrency 3 keeps pressure off target sites while
// roughly tripling throughput vs the old serial loop.
const firecrawlWorkerCount = 3

// firecrawlRateLimit is the polite delay each worker observes after finishing
// one job, before picking up the next. Package-level var so tests can shrink
// it.
var firecrawlRateLimit = 500 * time.Millisecond

// FirecrawlJob runs one firecrawl cycle: claims pending crawl jobs from the
// queue and processes them with a fixed worker pool (concurrency 3).
func FirecrawlJob(queue *content.FirecrawlJobQueue, batchID string) JobFunc {
	return firecrawlJobWithCrawler(queue, batchID, func(config *content.FirecrawlConfig) content.Crawler {
		return content.NewFallbackCrawler(
			content.NewReadabilityCrawler(),
			content.NewFirecrawlService(config),
		)
	})
}

// firecrawlJobWithCrawler is the testable core of FirecrawlJob: the crawler
// is built by newCrawler after the config loads, so tests can inject fakes
// while production wiring (readability → firecrawl fallback) stays unchanged.
func firecrawlJobWithCrawler(queue *content.FirecrawlJobQueue, batchID string, newCrawler func(*content.FirecrawlConfig) content.Crawler) JobFunc {
	return func(ctx context.Context) (*JobResult, error) {
		startTime := time.Now()

		config, err := content.GetFirecrawlConfig()
		if err != nil {
			return nil, fmt.Errorf("firecrawl config error: %w", err)
		}
		if !config.Enabled {
			return &JobResult{
				Data:    map[string]interface{}{},
				Summary: "firecrawl disabled",
			}, nil
		}

		firecrawlService := newCrawler(config)

		jobs, err := queue.Claim(50, firecrawlLeaseDuration(config))
		if err != nil {
			return nil, fmt.Errorf("claim error: %w", err)
		}

		if len(jobs) == 0 {
			return &JobResult{
				Data:    map[string]interface{}{},
				Summary: "no jobs to process",
			}, nil
		}

		// We need to broadcast progress - use a local counter
		var processingCount int32

		broadcastFirecrawlProgress(batchID, "processing", len(jobs), 0, 0, nil, &processingCount)

		logging.Infof("[Firecrawl] Starting parallel processing of %d jobs (concurrency=%d)", len(jobs), firecrawlWorkerCount)

		var completed atomic.Int32
		var failed atomic.Int32

		jobsCh := make(chan models.FirecrawlJob)
		var wg sync.WaitGroup
		for w := 0; w < firecrawlWorkerCount; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobsCh {
					processFirecrawlJob(queue, batchID, len(jobs), job, firecrawlService, &completed, &failed, &processingCount)
					// Rate limiting: polite delay before this worker starts
					// its next job.
					time.Sleep(firecrawlRateLimit)
				}
			}()
		}
		for i := range jobs {
			jobsCh <- jobs[i]
		}
		close(jobsCh)
		wg.Wait()

		completedTotal := int(completed.Load())
		failedTotal := int(failed.Load())

		duration := time.Since(startTime).Seconds()
		logging.Infof("[Firecrawl] Parallel crawl completed: %d completed, %d failed out of %d jobs in %.2fs",
			completedTotal, failedTotal, len(jobs), duration)

		broadcastFirecrawlProgress(batchID, "completed", len(jobs), completedTotal, failedTotal, nil, &processingCount)

		return &JobResult{
			Data: map[string]interface{}{
				"completed": completedTotal,
				"failed":    failedTotal,
				"total":     len(jobs),
				"batch_id":  batchID,
			},
			Summary: fmt.Sprintf("completed=%d failed=%d total=%d", completedTotal, failedTotal, len(jobs)),
		}, nil
	}
}

// processFirecrawlJob handles a single claimed crawl job. Semantics are
// identical to the previous serial loop (success path, failure backoff,
// terminal RSS fallback); it is safe to run from multiple workers
// concurrently.
func processFirecrawlJob(
	queue *content.FirecrawlJobQueue,
	batchID string,
	total int,
	job models.FirecrawlJob,
	crawler content.Crawler,
	completed *atomic.Int32,
	failed *atomic.Int32,
	processingCount *int32,
) {
	broadcast := func(status string, current *ws.FirecrawlArticleProgress) {
		broadcastFirecrawlProgress(batchID, status, total,
			int(completed.Load()), int(failed.Load()), current, processingCount)
	}

	var art models.Article
	if err := repository.Repo.DB().Omit("tag_count", "relevance_score").First(&art, job.ArticleID).Error; err != nil {
		failed.Add(1)
		_ = queue.MarkFailed(job, err.Error(), time.Minute)
		return
	}

	var feed models.Feed
	if err := repository.Repo.DB().First(&feed, art.FeedID).Error; err != nil {
		failed.Add(1)
		repository.Repo.DB().Model(&art).Updates(map[string]interface{}{
			"firecrawl_status": "failed",
			"firecrawl_error":  err.Error(),
		})
		broadcast("processing", &ws.FirecrawlArticleProgress{
			ID:     art.ID,
			Title:  art.Title,
			Status: "failed",
			Error:  err.Error(),
		})
		_ = queue.MarkFailed(job, err.Error(), time.Minute)
		return
	}

	repository.Repo.DB().Model(&art).Update("firecrawl_status", "processing")

	broadcast("processing", &ws.FirecrawlArticleProgress{
		ID:     art.ID,
		Title:  art.Title,
		Status: "processing",
	})

	result, crawlErr := crawler.ScrapePage(context.Background(), art.Link)
	if crawlErr != nil {
		failed.Add(1)
		repository.Repo.DB().Model(&art).Updates(map[string]interface{}{
			"firecrawl_status": "failed",
			"firecrawl_error":  crawlErr.Error(),
		})
		terminal := job.AttemptCount >= job.MaxAttempts
		_ = queue.MarkFailed(job, crawlErr.Error(), firecrawlFailureBackoff(job.AttemptCount))
		broadcast("processing", &ws.FirecrawlArticleProgress{
			ID:     art.ID,
			Title:  art.Title,
			Status: "failed",
			Error:  crawlErr.Error(),
		})
		logging.Errorf("[Firecrawl] Failed to crawl %s: %v", art.Link, crawlErr)

		// 抓取彻底失败（已达重试上限）：降级用 RSS description 继续，避免标签/
		// AI 摘要因 firecrawl 失败被永久阻塞。
		if terminal {
			if feed.ArticleSummaryEnabled && feed.CompletionOnRefresh {
				repository.Repo.DB().Model(&art).Update("summary_status", "incomplete")
			}
			if feed.TaggingEnabled {
				if err := tagging.NewTagJobQueue(repository.Repo.DB()).Enqueue(tagging.TagJobRequest{
					ArticleID:    art.ID,
					FeedName:     feed.Title,
					CategoryName: tagging.FeedCategoryName(feed),
					ForceRetag:   true,
					Reason:       "firecrawl_failed_fallback",
				}); err != nil {
					logging.Warnf("[Firecrawl] Failed to enqueue fallback retag for article %d: %v", art.ID, err)
				}
			}
			logging.Infof("[Firecrawl] Article %d reached crawl retry limit, falling back to RSS content for tagging/summary", art.ID)
		}
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"firecrawl_status":     "completed",
		"firecrawl_content":    result.Markdown,
		"firecrawl_crawled_at": now,
	}
	// Backfill image_url from the scrape metadata (OG image, Twitter card
	// as fallback). Only fill when RSS left it empty — RSS enclosures are
	// editor-selected and take priority over scraped OG images.
	if art.ImageURL == "" && result.OGImage != "" {
		updates["image_url"] = result.OGImage
	}
	if feed.ArticleSummaryEnabled && feed.CompletionOnRefresh {
		updates["summary_status"] = "incomplete"
	}
	repository.Repo.DB().Model(&art).Updates(updates)

	if feed.TaggingEnabled {
		if err := tagging.NewTagJobQueue(repository.Repo.DB()).Enqueue(tagging.TagJobRequest{
			ArticleID:    art.ID,
			FeedName:     feed.Title,
			CategoryName: tagging.FeedCategoryName(feed),
			ForceRetag:   true,
			Reason:       "firecrawl_completed",
		}); err != nil {
			failed.Add(1)
			_ = queue.MarkFailed(job, err.Error(), time.Minute)
			logging.Warnf("[Firecrawl] Failed to enqueue retag for article %d after crawl: %v", art.ID, err)
			return
		}
	}

	if err := queue.MarkCompleted(job.ID); err != nil {
		failed.Add(1)
		logging.Errorf("[Firecrawl] Failed to mark job %d completed: %v", job.ID, err)
		return
	}

	completed.Add(1)
	broadcast("processing", &ws.FirecrawlArticleProgress{
		ID:     art.ID,
		Title:  art.Title,
		Status: "completed",
	})
}

func firecrawlLeaseDuration(config *content.FirecrawlConfig) time.Duration {
	timeout := time.Duration(config.Timeout) * time.Second
	if timeout < 5*time.Minute {
		timeout = 5 * time.Minute
	}
	return timeout + 5*time.Minute
}

func firecrawlFailureBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return time.Minute
	}
	backoff := time.Duration(1<<minInt(attempt-1, 4)) * time.Minute
	if backoff > 30*time.Minute {
		return 30 * time.Minute
	}
	return backoff
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func broadcastFirecrawlProgress(batchID, status string, total, completed, failed int, current *ws.FirecrawlArticleProgress, processingCount *int32) {
	if processingCount != nil {
		v := completed + failed
		if v > math.MaxInt32 {
			v = math.MaxInt32
		}
		atomic.StoreInt32(processingCount, int32(v))
	}

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

// FirecrawlStatusEnricher returns a StatusDetailFunc that adds firecrawl-specific
// status fields.
func FirecrawlStatusEnricher() StatusDetailFunc {
	return func(result *JobResult) map[string]interface{} {
		return map[string]interface{}{
			"concurrency": firecrawlWorkerCount,
		}
	}
}
