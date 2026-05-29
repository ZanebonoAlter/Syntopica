package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
)

const blockedArticleThreshold = 50

type BlockedArticleRecoveryScheduler struct {
	checkInterval int
	stopChan      chan bool
	wg            sync.WaitGroup
	mu            sync.Mutex
	running       bool
	isExecuting   bool
	nextRun       *time.Time
	lastRun       *time.Time
	lastError     string
	totalRuns     int
	successRuns   int
	failedRuns    int
}

func NewBlockedArticleRecoveryScheduler(intervalSeconds int) *BlockedArticleRecoveryScheduler {
	return &BlockedArticleRecoveryScheduler{
		checkInterval: intervalSeconds,
		stopChan:      make(chan bool),
		running:       false,
	}
}

func (s *BlockedArticleRecoveryScheduler) Start() error {
	s.mu.Lock()

	if s.running {
		s.mu.Unlock()
		return nil
	}

	s.running = true
	s.wg.Add(1)
	nextRun := time.Now().Add(time.Duration(s.checkInterval) * time.Second)
	s.nextRun = &nextRun
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(time.Duration(s.checkInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.runRecoveryCycle()
				s.updateNextRun(time.Now().Add(time.Duration(s.checkInterval) * time.Second))
			case <-s.stopChan:
				logging.Infof("Blocked article recovery scheduler stopped")
				return
			}
		}
	}()

	logging.Infof("Blocked article recovery scheduler started (interval: %d seconds)", s.checkInterval)
	return nil
}

func (s *BlockedArticleRecoveryScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	close(s.stopChan)
	s.wg.Wait()
	s.stopChan = make(chan bool)
	s.nextRun = nil
}

func (s *BlockedArticleRecoveryScheduler) runRecoveryCycle() {
	tracing.TraceSchedulerTick("blocked_article_recovery", "cron", func(ctx context.Context) {
		s.mu.Lock()
		if s.isExecuting {
			s.mu.Unlock()
			return
		}
		s.isExecuting = true
		now := time.Now()
		s.lastRun = &now
		s.lastError = ""
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.isExecuting = false
			s.mu.Unlock()
		}()

		logging.Infof("Running blocked article recovery...")

		// STAT-04: Recover blocked articles (per D-04, D-05, D-06)
		var blockedArticles []models.Article
		err := database.DB.
			Joins("JOIN feeds ON feeds.id = articles.feed_id").
			Where("articles.firecrawl_status IN ?", []string{"waiting_for_firecrawl", "blocked"}).
			Find(&blockedArticles).Error

		if err != nil {
			s.mu.Lock()
			s.totalRuns++
			s.failedRuns++
			s.lastError = err.Error()
			s.mu.Unlock()
			logging.Errorf("BlockedArticleRecovery: failed to query blocked articles: %v", err)
			return
		}

		recoveredCount := 0
		for _, article := range blockedArticles {
			var feed models.Feed
			if err := database.DB.First(&feed, article.FeedID).Error; err != nil {
				// D-06: feed deleted, skip (defensive check)
				continue
			}

			// D-05: feed.FirecrawlEnabled changed to true, unblock
			if feed.FirecrawlEnabled {
				result := database.DB.Model(&models.Article{}).
					Where("id = ?", article.ID).
					Update("firecrawl_status", "pending")

				if result.Error == nil && result.RowsAffected > 0 {
					recoveredCount++
					logging.Infof("BlockedArticleRecovery: recovered article %d from feed %d", article.ID, feed.ID)
				}
			}
		}

		if recoveredCount > 0 {
			logging.Infof("BlockedArticleRecovery: recovered %d blocked articles", recoveredCount)
		}

		// STAT-05: Blocked count warning (per D-07, D-08)
		var blockedCount int64
		err = database.DB.Model(&models.Article{}).
			Joins("JOIN feeds ON feeds.id = articles.feed_id").
			Where("articles.summary_status = ?", "incomplete").
			Where("feeds.article_summary_enabled = ?", true).
			Where("articles.firecrawl_status <> ?", "completed").
			Count(&blockedCount).Error

		if err != nil {
			logging.Errorf("BlockedArticleRecovery: failed to count blocked articles: %v", err)
		} else if blockedCount > blockedArticleThreshold {
			logging.Warnf("ContentCompletion blocked articles exceeded threshold: %d > %d", blockedCount, blockedArticleThreshold)
		}

		s.mu.Lock()
		s.totalRuns++
		s.successRuns++
		s.lastError = ""
		s.mu.Unlock()
		logging.Infof("Blocked article recovery completed successfully")
	})
}

func (s *BlockedArticleRecoveryScheduler) TriggerNow() map[string]interface{} {
	s.mu.Lock()
	if s.isExecuting {
		s.mu.Unlock()
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     "阻塞文章恢复正在执行中，稍后再试。",
			"status_code": 409,
		}
	}
	s.mu.Unlock()

	logging.Infof("Manual blocked article recovery triggered")
	s.runRecoveryCycle()

	return map[string]interface{}{
		"accepted": true,
		"started":  true,
		"message":  "Blocked article recovery triggered",
	}
}

func (s *BlockedArticleRecoveryScheduler) GetStatus() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := "stopped"
	if s.isExecuting {
		status = "running"
	} else if s.running {
		status = "idle"
	}

	return map[string]interface{}{
		"status":                status,
		"check_interval":        s.checkInterval,
		"is_executing":          s.isExecuting,
		"next_run":              formatOptionalTime(s.nextRun),
		"last_execution_time":   formatOptionalTime(s.lastRun),
		"last_error":            s.lastError,
		"total_executions":      s.totalRuns,
		"successful_executions": s.successRuns,
		"failed_executions":     s.failedRuns,
	}
}

func (s *BlockedArticleRecoveryScheduler) ResetStats() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastRun = nil
	s.lastError = ""
	s.totalRuns = 0
	s.successRuns = 0
	s.failedRuns = 0
	return nil
}

func (s *BlockedArticleRecoveryScheduler) UpdateInterval(interval int) error {
	if interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}

	wasRunning := false
	s.mu.Lock()
	wasRunning = s.running
	s.mu.Unlock()

	if wasRunning {
		s.Stop()
	}

	s.mu.Lock()
	s.checkInterval = interval
	s.mu.Unlock()

	if wasRunning {
		return s.Start()
	}

	s.updateNextRun(time.Now().Add(time.Duration(interval) * time.Second))
	return nil
}

func (s *BlockedArticleRecoveryScheduler) updateNextRun(next time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextRun = &next
}
