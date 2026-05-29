package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/domain/narrative"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
)

// NarrativeSummaryScheduler is the legacy narrative generation scheduler.
// Deprecated: Use DailyReportScheduler (daily_report.go) instead. This scheduler is retained
// for backward compatibility during the transition to the new daily report system.
type NarrativeSummaryScheduler struct {
	cron           *cron.Cron
	checkInterval  time.Duration
	isRunning      bool
	executionMutex sync.Mutex
	isExecuting    bool
}

type NarrativeSummaryRunSummary struct {
	TriggerSource string `json:"trigger_source"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	SavedCount    int    `json:"saved_count"`
	Reason        string `json:"reason"`
}

func NewNarrativeSummaryScheduler(checkInterval int) *NarrativeSummaryScheduler {
	return &NarrativeSummaryScheduler{
		cron:          cron.New(),
		checkInterval: time.Duration(checkInterval) * time.Second,
	}
}

func (s *NarrativeSummaryScheduler) Start() error {
	if s.isRunning {
		return fmt.Errorf("narrative-summary scheduler already running")
	}

	s.initSchedulerTask()
	scheduleExpr := fmt.Sprintf("@every %ds", int64(s.checkInterval.Seconds()))
	if _, err := s.cron.AddFunc(scheduleExpr, s.runNarrativeCycleFromCron); err != nil {
		return fmt.Errorf("failed to schedule narrative-summary: %w", err)
	}

	s.cron.Start()
	s.isRunning = true
	logging.Infof("Narrative-summary scheduler started with interval: %v", s.checkInterval)
	return nil
}

func (s *NarrativeSummaryScheduler) Stop() {
	if !s.isRunning {
		return
	}

	s.cron.Stop()
	s.isRunning = false
	logging.Infoln("Narrative-summary scheduler stopped")
}

func (s *NarrativeSummaryScheduler) UpdateInterval(interval int) error {
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
		return s.Start()
	}

	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", "narrative_summary").First(&task).Error; err == nil {
		nextRun := time.Now().Add(s.checkInterval)
		database.DB.Model(&task).Updates(map[string]interface{}{
			"check_interval":      interval,
			"next_execution_time": &nextRun,
		})
	}

	return nil
}

func (s *NarrativeSummaryScheduler) ResetStats() error {
	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", "narrative_summary").First(&task).Error; err != nil {
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

func (s *NarrativeSummaryScheduler) TriggerNow() map[string]interface{} {
	if !s.executionMutex.TryLock() {
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     "叙事摘要正在执行中，请稍后再试。",
			"status_code": http.StatusConflict,
		}
	}

	s.isExecuting = true
	go func() {
		defer s.executionMutex.Unlock()
		defer func() {
			s.isExecuting = false
			if r := recover(); r != nil {
				logging.Errorf("PANIC in manual narrative-summary trigger: %v", r)
				s.updateSchedulerStatus("idle", fmt.Sprintf("Panic: %v", r), nil, nil)
			}
		}()
		s.runNarrativeCycle("manual", time.Now())
	}()

	return map[string]interface{}{
		"accepted": true,
		"started":  true,
		"reason":   "manual_run_started",
		"message":  "叙事摘要生成已经开始运行。",
	}
}

func (s *NarrativeSummaryScheduler) TriggerNowWithDate(dateStr string) map[string]interface{} {
	if !s.executionMutex.TryLock() {
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     "叙事摘要正在执行中，请稍后再试。",
			"status_code": http.StatusConflict,
		}
	}

	targetDate := time.Now()
	if dateStr != "" {
		parsed, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if err != nil {
			s.executionMutex.Unlock()
			return map[string]interface{}{
				"accepted":    false,
				"started":     false,
				"reason":      "invalid_date",
				"message":     "日期格式无效，请使用 YYYY-MM-DD。",
				"status_code": http.StatusBadRequest,
			}
		}
		targetDate = parsed
	}

	s.isExecuting = true
	go func() {
		defer s.executionMutex.Unlock()
		defer func() {
			s.isExecuting = false
			if r := recover(); r != nil {
				logging.Errorf("PANIC in manual narrative-summary trigger: %v", r)
				s.updateSchedulerStatus("idle", fmt.Sprintf("Panic: %v", r), nil, nil)
			}
		}()
		s.runNarrativeCycle("manual", targetDate)
	}()

	return map[string]interface{}{
		"accepted": true,
		"started":  true,
		"reason":   "manual_run_started",
		"message":  fmt.Sprintf("叙事摘要生成已经开始运行（目标日期: %s）。", targetDate.Format("2006-01-02")),
	}
}

func (s *NarrativeSummaryScheduler) initSchedulerTask() {
	var task models.SchedulerTask
	now := time.Now()
	nextRun := now.Add(s.checkInterval)

	if err := database.DB.Where("name = ?", "narrative_summary").First(&task).Error; err == nil {
		if task.CheckInterval > 0 {
			s.checkInterval = time.Duration(task.CheckInterval) * time.Second
			nextRun = now.Add(s.checkInterval)
		}
		updates := map[string]interface{}{
			"description":         "Generate daily narrative summaries from active topic tags",
			"check_interval":      int(s.checkInterval.Seconds()),
			"next_execution_time": &nextRun,
		}
		if task.Status == "" || task.Status == "success" || task.Status == "failed" || task.Status == "running" {
			updates["status"] = "idle"
		}
		database.DB.Model(&task).Updates(updates)
		return
	}

	task = models.SchedulerTask{
		Name:              "narrative_summary",
		Description:       "Generate daily narrative summaries from active topic tags",
		CheckInterval:     int(s.checkInterval.Seconds()),
		Status:            "idle",
		NextExecutionTime: &nextRun,
	}
	database.DB.Create(&task)
}

func (s *NarrativeSummaryScheduler) runNarrativeCycleFromCron() {
	tracing.TraceSchedulerTick("narrative_summary", "cron", func(ctx context.Context) {
		_ = ctx
		if !s.executionMutex.TryLock() {
			logging.Infoln("Narrative summary generation already in progress, skipping this cycle")
			return
		}
		s.isExecuting = true
		defer func() {
			s.executionMutex.Unlock()
			s.isExecuting = false
			if r := recover(); r != nil {
				logging.Errorf("PANIC in runNarrativeCycleFromCron: %v", r)
				s.updateSchedulerStatus("idle", fmt.Sprintf("Panic: %v", r), nil, nil)
			}
		}()

		s.runNarrativeCycle("scheduled", time.Now().In(time.Local))
	})
}

// runNarrativeCycle executes a single narrative generation cycle.
// Deprecated: The new DailyReportScheduler in daily_report.go replaces this functionality.
func (s *NarrativeSummaryScheduler) runNarrativeCycle(triggerSource string, targetDate time.Time) {
	startTime := time.Now()
	summary := &NarrativeSummaryRunSummary{
		TriggerSource: triggerSource,
		StartedAt:     startTime.Format(time.RFC3339),
	}
	s.updateSchedulerStatus("running", "", nil, nil)

	var savedCount int
	var err error
	if triggerSource == "manual" {
		savedCount, err = narrative.NewNarrativeService().RegenerateAndSave(targetDate)
	} else {
		savedCount, err = narrative.NewNarrativeService().GenerateAndSave(targetDate)
	}
	if err != nil {
		summary.FinishedAt = time.Now().Format(time.RFC3339)
		summary.Reason = err.Error()
		s.updateSchedulerStatus("failed", err.Error(), &startTime, summary)
		return
	}

	summary.FinishedAt = time.Now().Format(time.RFC3339)
	summary.SavedCount = savedCount
	summary.Reason = "narrative summaries generated"
	s.updateSchedulerStatus("success", "", &startTime, summary)
}

func (s *NarrativeSummaryScheduler) updateSchedulerStatus(status, lastError string, startTime *time.Time, summary *NarrativeSummaryRunSummary) {
	now := time.Now()
	nextExecution := now.Add(s.checkInterval)
	resultJSON := ""
	if summary != nil {
		if encoded, err := json.Marshal(summary); err == nil {
			resultJSON = string(encoded)
		}
	}

	updates := map[string]interface{}{
		"status":              status,
		"last_error":          lastError,
		"next_execution_time": &nextExecution,
	}
	if startTime != nil {
		duration := float64(time.Since(*startTime).Seconds())
		updates["last_execution_time"] = &now
		updates["last_execution_duration"] = &duration
		updates["last_execution_result"] = resultJSON
	}

	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", "narrative_summary").First(&task).Error; err == nil {
		updates["total_executions"] = task.TotalExecutions
		updates["successful_executions"] = task.SuccessfulExecutions
		updates["failed_executions"] = task.FailedExecutions
		updates["consecutive_failures"] = task.ConsecutiveFailures

		if startTime != nil {
			updates["total_executions"] = task.TotalExecutions + 1
			switch status {
			case "success":
				updates["successful_executions"] = task.SuccessfulExecutions + 1
				updates["consecutive_failures"] = 0
			case "failed":
				updates["failed_executions"] = task.FailedExecutions + 1
				updates["consecutive_failures"] = task.ConsecutiveFailures + 1
				updates["last_error_time"] = &now
			}
		}

		database.DB.Model(&task).Updates(updates)
		return
	}

	task = models.SchedulerTask{
		Name:              "narrative_summary",
		Description:       "Generate daily narrative summaries from active topic tags",
		CheckInterval:     int(s.checkInterval.Seconds()),
		Status:            status,
		LastError:         lastError,
		NextExecutionTime: &nextExecution,
	}
	if startTime != nil {
		duration := float64(time.Since(*startTime).Seconds())
		task.LastExecutionTime = &now
		task.LastExecutionDuration = &duration
		task.LastExecutionResult = resultJSON
		task.TotalExecutions = 1
		switch status {
		case "success":
			task.SuccessfulExecutions = 1
		case "failed":
			task.FailedExecutions = 1
			task.ConsecutiveFailures = 1
			task.LastErrorTime = &now
		}
	}
	database.DB.Create(&task)
}

func (s *NarrativeSummaryScheduler) GetStatus() SchedulerStatusResponse {
	entries := s.cron.Entries()
	var nextRun int64
	if len(entries) > 0 {
		nextRun = entries[0].Next.Unix()
	}

	var task models.SchedulerTask
	err := database.DB.Where("name = ?", "narrative_summary").First(&task).Error

	status := SchedulerStatusResponse{
		Name: "Narrative Summary",
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
	if err == nil && task.NextExecutionTime != nil {
		status.NextRun = task.NextExecutionTime.Unix()
	}
	return status
}
