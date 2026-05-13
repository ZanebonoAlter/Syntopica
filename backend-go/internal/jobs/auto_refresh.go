package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"my-robot-backend/internal/domain/feed"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
	"my-robot-backend/internal/platform/tracing"
	"my-robot-backend/internal/platform/ws"
)

type AutoRefreshScheduler struct {
	cron           *cron.Cron
	checkInterval  time.Duration
	feedService    *feed.FeedService
	refreshFeed    func(ctx context.Context, feedID uint) error
	isRunning      bool
	isExecuting    bool
	executionMutex sync.Mutex
}

type AutoRefreshRunSummary struct {
	TriggerSource          string `json:"trigger_source"`
	StartedAt              string `json:"started_at"`
	FinishedAt             string `json:"finished_at"`
	ScannedFeeds           int    `json:"scanned_feeds"`
	DueFeeds               int    `json:"due_feeds"`
	TriggeredFeeds         int    `json:"triggered_feeds"`
	AlreadyRefreshingFeeds int    `json:"already_refreshing_feeds"`
	StaleResetFeeds        int    `json:"stale_reset_feeds"`
	Reason                 string `json:"reason"`
}

const staleRefreshingTimeout = 5 * time.Minute

func NewAutoRefreshScheduler(checkInterval int) *AutoRefreshScheduler {
	feedService := feed.NewFeedService()
	return &AutoRefreshScheduler{
		cron:          cron.New(),
		checkInterval: time.Duration(checkInterval) * time.Second,
		feedService:   feedService,
		refreshFeed:   feedService.RefreshFeed,
		isRunning:     false,
	}
}

func (s *AutoRefreshScheduler) Start() error {
	if s.isRunning {
		return fmt.Errorf("scheduler already running")
	}

	scheduleExpr := fmt.Sprintf("@every %ds", int64(s.checkInterval.Seconds()))
	if _, err := s.cron.AddFunc(scheduleExpr, s.checkAndRefreshFeeds); err != nil {
		return fmt.Errorf("failed to schedule auto-refresh: %w", err)
	}

	s.cron.Start()
	s.isRunning = true
	logging.Infof("Auto-refresh scheduler started with interval: %v", s.checkInterval)
	s.initSchedulerTask()

	return nil
}

func (s *AutoRefreshScheduler) Stop() {
	if !s.isRunning {
		return
	}

	s.cron.Stop()
	s.isRunning = false
	logging.Infoln("Auto-refresh scheduler stopped")
}

func (s *AutoRefreshScheduler) UpdateInterval(interval int) error {
	if interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}

	wasRunning := s.isRunning
	if wasRunning {
		s.Stop()
	}

	s.cron = cron.New()
	s.checkInterval = time.Duration(interval) * time.Second

	if wasRunning {
		if err := s.Start(); err != nil {
			return err
		}
	}

	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", "auto_refresh").First(&task).Error; err == nil {
		nextRun := time.Now().Add(s.checkInterval)
		database.DB.Model(&task).Updates(map[string]interface{}{
			"check_interval":      interval,
			"next_execution_time": &nextRun,
		})
	}

	return nil
}

func (s *AutoRefreshScheduler) ResetStats() error {
	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", "auto_refresh").First(&task).Error; err != nil {
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

	return database.DB.Model(&task).Updates(updates).Error
}

func (s *AutoRefreshScheduler) checkAndRefreshFeeds() {
	tracing.TraceSchedulerTick("auto_refresh", "cron", func(ctx context.Context) {
		if !s.executionMutex.TryLock() {
			logging.Infoln("Auto-refresh scheduler already running, skipping this cycle")
			return
		}
		s.isExecuting = true
		defer func() {
			s.isExecuting = false
			s.executionMutex.Unlock()
		}()

		_, _ = s.runRefreshCycle("scheduled")
	})
}

func (s *AutoRefreshScheduler) runRefreshCycle(triggerSource string) (*AutoRefreshRunSummary, error) {
	startTime := time.Now()
	s.updateSchedulerStatus("running", "", nil, nil)

	summary := &AutoRefreshRunSummary{
		TriggerSource: triggerSource,
		StartedAt:     startTime.Format(time.RFC3339),
	}

	var feeds []models.Feed
	if err := database.DB.Where("refresh_interval > 0").Find(&feeds).Error; err != nil {
		logging.Errorf("Error querying feeds: %v", err)
		summary.Reason = "query_failed"
		summary.FinishedAt = time.Now().Format(time.RFC3339)
		s.updateSchedulerStatus("idle", err.Error(), &startTime, summary)
		return summary, err
	}
	summary.ScannedFeeds = len(feeds)

	now := time.Now()
	summary.StaleResetFeeds = s.resetStaleRefreshingFeeds(now)

	for _, feed := range feeds {
		if !s.needsRefresh(&feed, now) {
			continue
		}

		summary.DueFeeds++
		if feed.RefreshStatus == "refreshing" {
			summary.AlreadyRefreshingFeeds++
			continue
		}

		s.markFeedRefreshing(feed.ID)
		go func(feedID uint) {
			s.refreshFeedAsync(context.Background(), feedID)
		}(feed.ID)
		summary.TriggeredFeeds++
	}

	if summary.TriggeredFeeds > 0 {
		go s.broadcastRefreshCompletion(startTime, summary)
	}

	summary.FinishedAt = time.Now().Format(time.RFC3339)
	summary.Reason = autoRefreshReason(summary)
	if summary.TriggeredFeeds > 0 {
		logging.Infof("Auto-refresh: triggered %d feed(s)", summary.TriggeredFeeds)
	}
	if summary.StaleResetFeeds > 0 {
		logging.Infof("Auto-refresh: reset %d stale feed(s)", summary.StaleResetFeeds)
	}

	s.updateSchedulerStatus("idle", "", &startTime, summary)
	return summary, nil
}

func (s *AutoRefreshScheduler) needsRefresh(feed *models.Feed, now time.Time) bool {
	if feed.LastRefreshAt == nil {
		return true
	}

	timeSinceRefresh := now.Sub(*feed.LastRefreshAt)
	interval := time.Duration(feed.RefreshInterval) * time.Minute

	return timeSinceRefresh >= interval
}

func (s *AutoRefreshScheduler) refreshFeedAsync(ctx context.Context, feedID uint) {
	defer func() {
		if r := recover(); r != nil {
			logging.Errorf("PANIC in refreshFeedAsync for feed %d: %v", feedID, r)
			s.resetFeedStatus(feedID, fmt.Sprintf("panic: %v", r))
		}
	}()

	if err := s.refreshFeed(ctx, feedID); err != nil {
		logging.Errorf("Error refreshing feed %d: %v", feedID, err)
		s.resetFeedStatus(feedID, err.Error())
	}
}

func (s *AutoRefreshScheduler) resetFeedStatus(feedID uint, errMsg string) {
	now := time.Now().In(time.FixedZone("CST", 8*3600))
	database.DB.Model(&models.Feed{}).Where("id = ? AND refresh_status = ?", feedID, "refreshing").Updates(map[string]interface{}{
		"refresh_status":  "error",
		"refresh_error":   errMsg,
		"last_refresh_at": &now,
	})
}

func (s *AutoRefreshScheduler) broadcastRefreshCompletion(startTime time.Time, summary *AutoRefreshRunSummary) {
	duration := time.Since(startTime).Seconds()
	msg := ws.AutoRefreshCompleteMessage{
		Type:            "auto_refresh_complete",
		TriggeredFeeds:  summary.TriggeredFeeds,
		StaleResetFeeds: summary.StaleResetFeeds,
		DurationSeconds: duration,
		Timestamp:       time.Now().Format(time.RFC3339),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		logging.Warnf("Auto-refresh completion message marshal failed: %v", err)
	} else {
		ws.GetHub().BroadcastRaw(data)
	}
}

func (s *AutoRefreshScheduler) GetStatus() SchedulerStatusResponse {
	entries := s.cron.Entries()

	var nextRun int64
	if len(entries) > 0 {
		nextRun = entries[0].Next.Unix()
	}

	status := SchedulerStatusResponse{
		Name: "Auto Refresh",
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
		NextRun:       nextRun,
		IsExecuting:   s.isExecuting,
	}

	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", "auto_refresh").First(&task).Error; err == nil {
		if task.NextExecutionTime != nil {
			status.NextRun = task.NextExecutionTime.Unix()
		}
	}

	return status
}

func (s *AutoRefreshScheduler) Trigger() {
	go s.checkAndRefreshFeeds()
}

func (s *AutoRefreshScheduler) TriggerNow() map[string]interface{} {
	if !s.executionMutex.TryLock() {
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     "后台刷新正在执行中，先别重复点。",
			"status_code": http.StatusConflict,
		}
	}
	s.isExecuting = true
	defer func() {
		s.isExecuting = false
		s.executionMutex.Unlock()
	}()

	summary, err := s.runRefreshCycle("manual")
	if err != nil {
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "refresh_cycle_failed",
			"message":     err.Error(),
			"summary":     summary,
			"status_code": http.StatusInternalServerError,
		}
	}

	message := "手动扫描完成，没有 feed 到点。"
	switch {
	case summary.TriggeredFeeds > 0:
		message = fmt.Sprintf("手动扫描完成，已触发 %d 个 feed 刷新。", summary.TriggeredFeeds)
		if summary.StaleResetFeeds > 0 {
			message += fmt.Sprintf(" 重置了 %d 个卡住的 feed。", summary.StaleResetFeeds)
		}
	case summary.AlreadyRefreshingFeeds > 0:
		message = "手动扫描完成，但到点的 feed 已在刷新中。"
	case summary.ScannedFeeds == 0:
		message = "当前没有开启自动刷新的 feed。"
	case summary.StaleResetFeeds > 0:
		message = fmt.Sprintf("手动扫描完成，重置了 %d 个卡住的 feed。", summary.StaleResetFeeds)
	}

	return map[string]interface{}{
		"accepted":  true,
		"started":   summary.TriggeredFeeds > 0,
		"effectful": summary.TriggeredFeeds > 0,
		"reason":    summary.Reason,
		"message":   message,
		"summary":   summary,
	}
}

func (s *AutoRefreshScheduler) initSchedulerTask() {
	var task models.SchedulerTask
	now := time.Now()
	nextRun := now.Add(s.checkInterval)

	if err := database.DB.Where("name = ?", "auto_refresh").First(&task).Error; err == nil {
		if task.CheckInterval > 0 {
			s.checkInterval = time.Duration(task.CheckInterval) * time.Second
			nextRun = now.Add(s.checkInterval)
		}
		updates := map[string]interface{}{
			"description":         "Auto-refresh RSS feeds",
			"check_interval":      int(s.checkInterval.Seconds()),
			"next_execution_time": &nextRun,
		}

		if task.Status == "" || task.Status == "success" || task.Status == "failed" || task.Status == "running" {
			updates["status"] = "idle"
			updates["last_error"] = ""
		}

		database.DB.Model(&task).Updates(updates)
		return
	}

	task = models.SchedulerTask{
		Name:              "auto_refresh",
		Description:       "Auto-refresh RSS feeds",
		CheckInterval:     int(s.checkInterval.Seconds()),
		Status:            "idle",
		NextExecutionTime: &nextRun,
	}
	database.DB.Create(&task)
}

func (s *AutoRefreshScheduler) resetStaleRefreshingFeeds(now time.Time) int {
	cutoff := now.Add(-staleRefreshingTimeout)

	// First, query the stale feeds to log details
	var staleFeeds []models.Feed
	if err := database.DB.Model(&models.Feed{}).
		Where("refresh_status = ? AND last_refresh_at < ?", "refreshing", cutoff).
		Find(&staleFeeds).Error; err != nil {
		logging.Errorf("Error querying stale feeds: %v", err)
		return 0
	}

	// Log each stale feed's details
	for _, feed := range staleFeeds {
		if feed.LastRefreshAt != nil {
			staleDuration := now.Sub(*feed.LastRefreshAt)
			logging.Warnf("Feed %d stuck for %.1f minutes, resetting", feed.ID, staleDuration.Minutes())
		}
	}

	// Then perform the batch update
	if len(staleFeeds) == 0 {
		return 0
	}

	result := database.DB.Model(&models.Feed{}).
		Where("refresh_status = ? AND last_refresh_at < ?", "refreshing", cutoff).
		Updates(map[string]interface{}{
			"refresh_status": "idle",
			"refresh_error":  "stale refreshing state reset after 5 minutes",
		})
	count := int(result.RowsAffected)
	if count > 0 {
		logging.Infof("Auto-refresh: reset %d stale refreshing feed(s) (stuck > %v)", count, staleRefreshingTimeout)
	}
	return count
}

func (s *AutoRefreshScheduler) markFeedRefreshing(feedID uint) {
	database.DB.Model(&models.Feed{}).Where("id = ?", feedID).Updates(map[string]interface{}{
		"refresh_status": "refreshing",
		"refresh_error":  "",
	})
}

func autoRefreshReason(summary *AutoRefreshRunSummary) string {
	switch {
	case summary.ScannedFeeds == 0:
		return "no_feeds_enabled"
	case summary.StaleResetFeeds > 0 && summary.TriggeredFeeds > 0:
		return "stale_reset_and_feeds_triggered"
	case summary.StaleResetFeeds > 0:
		return "stale_reset"
	case summary.TriggeredFeeds > 0:
		return "feeds_triggered"
	case summary.DueFeeds == 0:
		return "no_feeds_due"
	case summary.AlreadyRefreshingFeeds > 0:
		return "all_due_feeds_already_refreshing"
	default:
		return "scan_complete"
	}
}

func (s *AutoRefreshScheduler) updateSchedulerStatus(status, lastError string, startTime *time.Time, summary *AutoRefreshRunSummary) {
	var task models.SchedulerTask
	err := database.DB.Where("name = ?", "auto_refresh").First(&task).Error
	now := time.Now()

	resultJSON := ""
	if summary != nil {
		if data, marshalErr := json.Marshal(summary); marshalErr == nil {
			resultJSON = string(data)
		}
	}

	if err == nil {
		task.Status = status
		task.LastError = lastError
		if lastError != "" {
			errTime := now
			task.LastErrorTime = &errTime
		}
		if resultJSON != "" {
			task.LastExecutionResult = resultJSON
		}

		if startTime != nil {
			duration := time.Since(*startTime)
			durationFloat := float64(duration.Seconds())
			task.LastExecutionDuration = &durationFloat
			task.LastExecutionTime = &now
			task.TotalExecutions++
			if lastError == "" {
				task.SuccessfulExecutions++
				task.ConsecutiveFailures = 0
			} else {
				task.FailedExecutions++
				task.ConsecutiveFailures++
			}
			nextExecution := now.Add(s.checkInterval)
			task.NextExecutionTime = &nextExecution
		}

		database.DB.Save(&task)
		return
	}

	nextExecution := now.Add(s.checkInterval)
	var durationFloat *float64
	if startTime != nil {
		d := time.Since(*startTime)
		df := float64(d.Seconds())
		durationFloat = &df
	}

	task = models.SchedulerTask{
		Name:                  "auto_refresh",
		Description:           "Auto-refresh RSS feeds",
		CheckInterval:         int(s.checkInterval.Seconds()),
		Status:                status,
		LastError:             lastError,
		NextExecutionTime:     &nextExecution,
		LastExecutionDuration: durationFloat,
		LastExecutionResult:   resultJSON,
	}
	if startTime != nil {
		task.LastExecutionTime = &now
		task.TotalExecutions = 1
		if lastError == "" {
			task.SuccessfulExecutions = 1
		} else {
			task.FailedExecutions = 1
			task.ConsecutiveFailures = 1
		}
	}
	database.DB.Create(&task)
}
