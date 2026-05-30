package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"syntopica-backend/internal/domain/content"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
)

type ContentCompletionScheduler struct {
	cron              *cron.Cron
	completionService *content.ContentCompletionService
	checkInterval     time.Duration
	taskName          string
	isRunning         bool
	isExecuting       bool
	currentArticle    *content.ContentCompletionArticleRef
	lastProcessed     *content.ContentCompletionArticleRef
	lastError         string
	lastExecutionTime *time.Time
	lastRunSummary    *ContentCompletionRunSummary
	mu                sync.RWMutex
	executionMutex    sync.Mutex
}

type ContentCompletionRunSummary struct {
	StartedAt              string                               `json:"started_at"`
	FinishedAt             string                               `json:"finished_at"`
	CompletedCount         int                                  `json:"completed_count"`
	FailedCount            int                                  `json:"failed_count"`
	BlockedCount           int                                  `json:"blocked_count"`
	StaleProcessingCount   int                                  `json:"stale_processing_count"`
	LiveProcessingCount    int                                  `json:"live_processing_count"`
	CurrentArticle         *content.ContentCompletionArticleRef `json:"current_article,omitempty"`
	LastProcessed          *content.ContentCompletionArticleRef `json:"last_processed,omitempty"`
	StaleProcessingArticle *content.ContentCompletionArticleRef `json:"stale_processing_article,omitempty"`
	ErrorSamples           []ContentCompletionErrorSample       `json:"error_samples,omitempty"`
}

type ContentCompletionErrorSample struct {
	ArticleID uint   `json:"article_id"`
	Message   string `json:"message"`
	Category  string `json:"category"`
}

func NewContentCompletionScheduler(completionService *content.ContentCompletionService, checkIntervalMinutes int) *ContentCompletionScheduler {
	taskName := "ai_summary"

	scheduler := &ContentCompletionScheduler{
		cron:              cron.New(),
		completionService: completionService,
		checkInterval:     time.Duration(checkIntervalMinutes) * time.Minute,
		taskName:          taskName,
	}

	interval := fmt.Sprintf("@every %dm", checkIntervalMinutes)
	_, err := scheduler.cron.AddFunc(interval, scheduler.checkAndCompleteArticles)
	if err != nil {
		logging.Errorf("Failed to schedule AI summary: %v", err)
	}

	return scheduler
}

func (s *ContentCompletionScheduler) Start() error {
	if s.isRunning {
		return fmt.Errorf("content completion scheduler already running")
	}
	if err := s.reconcileSchedulerTask(); err != nil {
		return fmt.Errorf("reconcile content completion scheduler task: %w", err)
	}
	s.cron.Start()
	s.isRunning = true
	logging.Infof("AI summary scheduler started (interval: %v)", s.checkInterval)
	return nil
}

func (s *ContentCompletionScheduler) Stop() {
	if !s.isRunning {
		return
	}
	s.cron.Stop()
	s.isRunning = false
	logging.Infoln("AI summary scheduler stopped")
}

func (s *ContentCompletionScheduler) TriggerNow() map[string]interface{} {
	if !s.executionMutex.TryLock() {
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     "内容补全正在执行中，稍后再试。",
			"status_code": http.StatusConflict,
		}
	}

	go s.runCompletionCycle("manual", nextContentCompletionRunID("manual"))
	return map[string]interface{}{
		"accepted": true,
		"started":  true,
		"message":  "文章总结任务已触发",
	}
}

func (s *ContentCompletionScheduler) UpdateInterval(interval int) error {
	if interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}

	wasRunning := s.isRunning
	if wasRunning {
		s.Stop()
	}

	s.cron = cron.New()
	s.checkInterval = time.Duration(interval) * time.Second
	if _, err := s.cron.AddFunc(fmt.Sprintf("@every %ds", interval), s.checkAndCompleteArticles); err != nil {
		return fmt.Errorf("failed to reschedule content completion: %w", err)
	}

	if wasRunning {
		if err := s.Start(); err != nil {
			return err
		}
	}

	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", s.taskName).First(&task).Error; err == nil {
		nextRun := time.Now().Add(s.checkInterval)
		database.DB.Model(&task).Updates(map[string]interface{}{
			"check_interval":      interval,
			"next_execution_time": &nextRun,
		})
	}

	return nil
}

func (s *ContentCompletionScheduler) ResetStats() error {
	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", s.taskName).First(&task).Error; err != nil {
		return err
	}

	nextRun := time.Now().Add(s.checkInterval)
	updates := map[string]interface{}{
		"status":                  "idle",
		"last_error":              "",
		"last_error_time":         nil,
		"total_executions":        0,
		"successful_executions":   0,
		"failed_executions":       0,
		"consecutive_failures":    0,
		"last_execution_time":     nil,
		"last_execution_duration": nil,
		"last_execution_result":   "",
		"next_execution_time":     &nextRun,
	}

	s.mu.Lock()
	s.lastError = ""
	s.lastExecutionTime = nil
	s.lastProcessed = nil
	s.currentArticle = nil
	s.lastRunSummary = nil
	s.mu.Unlock()

	return database.DB.Model(&task).Updates(updates).Error
}

func (s *ContentCompletionScheduler) checkAndCompleteArticles() {
	tracing.TraceSchedulerTick("content_completion", "cron", func(ctx context.Context) {
		if !s.executionMutex.TryLock() {
			logging.Infoln("Content completion scheduler already running, skipping this cycle")
			return
		}

		s.runCompletionCycle("scheduled", nextContentCompletionRunID("scheduled"))
	})
}

func (s *ContentCompletionScheduler) runCompletionCycle(triggerSource, runID string) {
	defer s.executionMutex.Unlock()

	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", s.taskName).First(&task).Error; err != nil {
		logging.Errorf("Scheduler task not found: %v", err)
		return
	}

	now := time.Now().In(time.FixedZone("CST", 8*3600))
	s.mu.Lock()
	s.isExecuting = true
	s.currentArticle = nil
	s.lastError = ""
	s.lastExecutionTime = &now
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.isExecuting = false
		s.currentArticle = nil
		s.mu.Unlock()
	}()

	task.Status = "running"
	task.LastExecutionTime = &now
	database.DB.Save(&task)

	startTime := time.Now()
	runSummary := &ContentCompletionRunSummary{
		StartedAt: now.Format(time.RFC3339),
	}
	articles, err := s.completionService.ListReadyArticles(50)
	if err != nil {
		task.Status = "error"
		task.LastError = err.Error()
		task.LastErrorTime = &now
		task.FailedExecutions++
		task.ConsecutiveFailures++
		nextRun := now.Add(s.checkInterval)
		task.NextExecutionTime = &nextRun
		database.DB.Save(&task)
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}

	completedIDs := make([]uint, 0, len(articles))
	errors := make([]error, 0)
	for _, article := range articles {
		s.mu.Lock()
		s.currentArticle = content.ToArticleRef(article)
		runSummary.CurrentArticle = s.currentArticle
		s.mu.Unlock()

		if article.CompletionAttempts >= article.Feed.MaxCompletionRetries {
			errors = append(errors, fmt.Errorf("article %d: max completion retries exceeded", article.ID))
			runSummary.BlockedCount++
			appendRunError(runSummary, article.ID, "max completion retries exceeded", "retries")
			continue
		}

		if err := s.completionService.CompleteArticleWithMetadata(context.Background(), article.ID, false, map[string]any{
			"scheduler_run_id": runID,
			"trigger_source":   triggerSource,
		}); err != nil {
			errors = append(errors, fmt.Errorf("article %d: %w", article.ID, err))
			runSummary.FailedCount++
			appendRunError(runSummary, article.ID, err.Error(), classifyCompletionError(err.Error()))
			s.mu.Lock()
			s.lastError = err.Error()
			s.lastProcessed = content.ToArticleRef(article)
			runSummary.LastProcessed = s.lastProcessed
			s.mu.Unlock()
			continue
		}

		completedIDs = append(completedIDs, article.ID)
		runSummary.CompletedCount++
		s.mu.Lock()
		s.lastProcessed = content.ToArticleRef(article)
		runSummary.LastProcessed = s.lastProcessed
		s.mu.Unlock()
	}
	duration := time.Since(startTime).Seconds()

	task.LastExecutionDuration = &duration

	if len(errors) > 0 {
		task.Status = "error"
		task.FailedExecutions++
		task.ConsecutiveFailures++
		task.LastError = errors[0].Error()
		task.LastErrorTime = &now
		logging.Warnf("AI summary completed with errors: %d completed, %d failed", len(completedIDs), len(errors))
	} else {
		task.Status = "idle"
		task.SuccessfulExecutions++
		task.ConsecutiveFailures = 0
		task.LastError = ""
		logging.Infof("AI summary completed successfully: %d articles processed", len(completedIDs))
	}

	overview, overviewErr := s.completionService.GetOverview()
	if overviewErr == nil && overview != nil {
		runSummary.BlockedCount = overview.BlockedCount
		runSummary.StaleProcessingCount = overview.StaleProcessingCount
		runSummary.LiveProcessingCount = 0
		runSummary.StaleProcessingArticle = overview.StaleProcessingArticle
	}
	runSummary.FinishedAt = time.Now().In(time.FixedZone("CST", 8*3600)).Format(time.RFC3339)
	if encoded, err := json.Marshal(runSummary); err == nil {
		task.LastExecutionResult = string(encoded)
	}

	task.TotalExecutions++

	nextRun := now.Add(s.checkInterval)
	task.NextExecutionTime = &nextRun
	database.DB.Save(&task)
	s.mu.Lock()
	s.lastRunSummary = runSummary
	s.mu.Unlock()
}

func nextContentCompletionRunID(triggerSource string) string {
	return fmt.Sprintf("content-completion-%s-%d", triggerSource, time.Now().UnixNano())
}

func (s *ContentCompletionScheduler) initSchedulerTask() {
	var task models.SchedulerTask
	err := database.DB.Where("name = ?", s.taskName).First(&task).Error

	if err == nil {
		return
	}

	now := time.Now().In(time.FixedZone("CST", 8*3600))
	nextRun := now.Add(s.checkInterval)

	task = models.SchedulerTask{
		Name:                 s.taskName,
		Description:          "AI summarize article content based on Firecrawl content",
		CheckInterval:        int(s.checkInterval.Seconds()),
		Status:               "idle",
		NextExecutionTime:    &nextRun,
		TotalExecutions:      0,
		SuccessfulExecutions: 0,
		FailedExecutions:     0,
		ConsecutiveFailures:  0,
	}

	database.DB.Create(&task)
	logging.Infoln("AI summary scheduler task initialized")
}

func (s *ContentCompletionScheduler) reconcileSchedulerTask() error {
	const legacyTaskName = "content_completion"

	var primary models.SchedulerTask
	primaryErr := database.DB.Where("name = ?", s.taskName).First(&primary).Error
	if primaryErr != nil && !errors.Is(primaryErr, gorm.ErrRecordNotFound) {
		return primaryErr
	}

	var legacy models.SchedulerTask
	legacyErr := database.DB.Where("name = ?", legacyTaskName).First(&legacy).Error
	if legacyErr != nil && !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
		return legacyErr
	}

	hasPrimary := primaryErr == nil
	hasLegacy := legacyErr == nil

	if !hasPrimary && !hasLegacy {
		s.initSchedulerTask()
		return nil
	}

	if !hasPrimary && hasLegacy {
		legacy.Name = s.taskName
		primary = legacy
		hasPrimary = true
		hasLegacy = false
	}

	if hasLegacy {
		if legacy.CheckInterval > 0 {
			primary.CheckInterval = legacy.CheckInterval
		}
		if primary.Description == "" && legacy.Description != "" {
			primary.Description = legacy.Description
		}
		if primary.LastExecutionResult == "" && legacy.LastExecutionResult != "" {
			primary.LastExecutionResult = legacy.LastExecutionResult
		}
		if err := database.DB.Delete(&legacy).Error; err != nil {
			return err
		}
	}

	if primary.CheckInterval <= 0 {
		primary.CheckInterval = int(s.checkInterval.Seconds())
	}
	if err := s.reschedule(primary.CheckInterval); err != nil {
		return err
	}

	if primary.Status == "running" {
		nextRun := time.Now().In(time.FixedZone("CST", 8*3600)).Add(s.checkInterval)
		primary.Status = "idle"
		primary.NextExecutionTime = &nextRun
	}

	if hasPrimary && primary.ID != 0 {
		return database.DB.Model(&models.SchedulerTask{}).Where("id = ?", primary.ID).Updates(map[string]interface{}{
			"name":                primary.Name,
			"description":         primary.Description,
			"check_interval":      primary.CheckInterval,
			"status":              primary.Status,
			"next_execution_time": primary.NextExecutionTime,
		}).Error
	}

	return database.DB.Create(&primary).Error
}

func (s *ContentCompletionScheduler) reschedule(intervalSeconds int) error {
	if intervalSeconds <= 0 {
		return nil
	}

	if s.checkInterval == time.Duration(intervalSeconds)*time.Second {
		return nil
	}

	rescheduled := cron.New()
	if _, err := rescheduled.AddFunc(fmt.Sprintf("@every %ds", intervalSeconds), s.checkAndCompleteArticles); err != nil {
		return err
	}

	s.cron = rescheduled
	s.checkInterval = time.Duration(intervalSeconds) * time.Second
	return nil
}

func (s *ContentCompletionScheduler) GetStatus() SchedulerStatusResponse {
	var task models.SchedulerTask
	database.DB.Where("name = ?", s.taskName).First(&task)

	s.mu.RLock()
	defer s.mu.RUnlock()

	status := SchedulerStatusResponse{
		Name: "Content Completion",
		Status: func() string {
			if s.isExecuting {
				return "running"
			}
			if s.isRunning {
				return "idle"
			}
			return "stopped"
		}(),
		CheckInterval: int64(s.checkInterval.Seconds()),
		IsExecuting:   s.isExecuting,
	}
	if task.NextExecutionTime != nil {
		status.NextRun = task.NextExecutionTime.Unix()
	}

	return status
}

func (s *ContentCompletionScheduler) GetTaskStatusDetails() map[string]interface{} {
	var task models.SchedulerTask
	var taskData map[string]interface{}
	if err := database.DB.Where("name = ?", s.taskName).First(&task).Error; err == nil {
		taskData = task.ToDict()
	}

	overview, err := s.completionService.GetOverview()
	if err != nil {
		logging.Warnf("failed to load ai summary overview: %v", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"status":              s.GetStatus().Status,
		"is_executing":        s.isExecuting,
		"current_article":     s.currentArticle,
		"last_processed":      s.lastProcessed,
		"check_interval":      int(s.checkInterval.Seconds()),
		"last_execution_time": s.lastExecutionTime,
		"last_error": func() string {
			if task.LastError != "" {
				return task.LastError
			}
			return s.lastError
		}(),
		"live_processing_count": func() int {
			if s.isExecuting && s.currentArticle != nil {
				return 1
			}
			return 0
		}(),
	}
	if taskData != nil {
		status["database_state"] = taskData
		status["next_run"] = task.NextExecutionTime
		if summary := parseLastRunSummary(task); summary != nil {
			status["last_run_summary"] = summary
		}
	}
	if overview != nil {
		liveProcessingCount := 0
		if s.isExecuting && s.currentArticle != nil {
			liveProcessingCount = 1
		}
		status["overview"] = map[string]interface{}{
			"pending_count":          overview.PendingCount,
			"processing_count":       overview.ProcessingCount,
			"live_processing_count":  liveProcessingCount,
			"stale_processing_count": overview.StaleProcessingCount,
			"completed_count":        overview.CompletedCount,
			"failed_count":           overview.FailedCount,
			"blocked_count":          overview.BlockedCount,
			"total_count":            overview.TotalCount,
			"ai_configured":          overview.AIConfigured,
			"blocked_reasons": map[string]interface{}{
				"waiting_for_firecrawl_count":     overview.BlockedReasons.WaitingForFirecrawlCount,
				"feed_disabled_count":             overview.BlockedReasons.FeedDisabledCount,
				"ai_unconfigured_count":           overview.BlockedReasons.AIUnconfiguredCount,
				"ready_but_missing_content_count": overview.BlockedReasons.ReadyButMissingContentCount,
			},
			"stale_processing_article": overview.StaleProcessingArticle,
		}
		status["stale_processing_count"] = overview.StaleProcessingCount
		status["stale_processing_article"] = overview.StaleProcessingArticle
	}

	return status
}

func parseLastRunSummary(task models.SchedulerTask) *ContentCompletionRunSummary {
	if task.LastExecutionResult == "" {
		return nil
	}

	var summary ContentCompletionRunSummary
	if err := json.Unmarshal([]byte(task.LastExecutionResult), &summary); err != nil {
		return nil
	}

	return &summary
}

func appendRunError(summary *ContentCompletionRunSummary, articleID uint, message, category string) {
	if len(summary.ErrorSamples) >= 5 {
		return
	}
	summary.ErrorSamples = append(summary.ErrorSamples, ContentCompletionErrorSample{
		ArticleID: articleID,
		Message:   message,
		Category:  category,
	})
}

func classifyCompletionError(message string) string {
	message = strings.ToLower(message)
	switch {
	case strings.Contains(message, "unexpected eof"), strings.Contains(message, "timeout"), strings.Contains(message, "connection reset"), strings.Contains(message, "tls"):
		return "network"
	case strings.Contains(message, "not configured"), strings.Contains(message, "api key"), strings.Contains(message, "model"):
		return "config"
	case strings.Contains(message, "no firecrawl content"):
		return "content"
	case strings.Contains(message, "max completion retries"):
		return "retries"
	default:
		return "unknown"
	}
}

func (s *ContentCompletionScheduler) Trigger() {
	go s.checkAndCompleteArticles()
}
