package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/topicextraction"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
	"my-robot-backend/internal/platform/tracing"
)

type TagQualityScoreScheduler struct {
	cron           *cron.Cron
	checkInterval  time.Duration
	isRunning      bool
	executionMutex sync.Mutex
	isExecuting    bool
}

type TagQualityScoreRunSummary struct {
	TriggerSource string `json:"trigger_source"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	UpdatedCount  int    `json:"updated_count"`
	Reason        string `json:"reason"`
}

func NewTagQualityScoreScheduler(checkInterval int) *TagQualityScoreScheduler {
	return &TagQualityScoreScheduler{
		cron:          cron.New(),
		checkInterval: time.Duration(checkInterval) * time.Second,
	}
}

func (s *TagQualityScoreScheduler) Start() error {
	if s.isRunning {
		return fmt.Errorf("tag-quality-score scheduler already running")
	}

	s.initSchedulerTask()
	scheduleExpr := fmt.Sprintf("@every %ds", int64(s.checkInterval.Seconds()))
	if _, err := s.cron.AddFunc(scheduleExpr, s.computeQualityScores); err != nil {
		return fmt.Errorf("failed to schedule tag-quality-score: %w", err)
	}

	s.cron.Start()
	s.isRunning = true
	logging.Infof("Tag-quality-score scheduler started with interval: %v", s.checkInterval)
	return nil
}

func (s *TagQualityScoreScheduler) Stop() {
	if !s.isRunning {
		return
	}

	s.cron.Stop()
	s.isRunning = false
	logging.Infoln("Tag-quality-score scheduler stopped")
}

func (s *TagQualityScoreScheduler) UpdateInterval(interval int) error {
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
	if err := database.DB.Where("name = ?", "tag_quality_score").First(&task).Error; err == nil {
		nextRun := time.Now().Add(s.checkInterval)
		database.DB.Model(&task).Updates(map[string]interface{}{
			"check_interval":      interval,
			"next_execution_time": &nextRun,
		})
	}

	return nil
}

func (s *TagQualityScoreScheduler) ResetStats() error {
	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", "tag_quality_score").First(&task).Error; err != nil {
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

func (s *TagQualityScoreScheduler) TriggerNow() map[string]interface{} {
	if !s.executionMutex.TryLock() {
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     "标签质量分重算正在执行中，请稍后再试。",
			"status_code": http.StatusConflict,
		}
	}

	s.isExecuting = true
	go func() {
		defer s.executionMutex.Unlock()
		defer func() {
			s.isExecuting = false
			if r := recover(); r != nil {
				logging.Errorf("PANIC in manual tag-quality-score trigger: %v", r)
				s.updateSchedulerStatus("idle", fmt.Sprintf("Panic: %v", r), nil, nil)
			}
		}()
		s.runComputeCycle("manual")
	}()

	return map[string]interface{}{
		"accepted": true,
		"started":  true,
		"reason":   "manual_run_started",
		"message":  "标签质量分重算已经开始运行。",
	}
}

func (s *TagQualityScoreScheduler) initSchedulerTask() {
	var task models.SchedulerTask
	now := time.Now()
	nextRun := now.Add(s.checkInterval)

	if err := database.DB.Where("name = ?", "tag_quality_score").First(&task).Error; err == nil {
		if task.CheckInterval > 0 {
			s.checkInterval = time.Duration(task.CheckInterval) * time.Second
			nextRun = now.Add(s.checkInterval)
		}
		updates := map[string]interface{}{
			"description":         "Recompute persistent quality scores for topic tags",
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
		Name:              "tag_quality_score",
		Description:       "Recompute persistent quality scores for topic tags",
		CheckInterval:     int(s.checkInterval.Seconds()),
		Status:            "idle",
		NextExecutionTime: &nextRun,
	}
	database.DB.Create(&task)
}

func (s *TagQualityScoreScheduler) computeQualityScores() {
	tracing.TraceSchedulerTick("tag_quality_score", "cron", func(ctx context.Context) {
		_ = ctx
		if !s.executionMutex.TryLock() {
			logging.Infoln("Tag quality score recompute already in progress, skipping this cycle")
			return
		}
		s.isExecuting = true
		defer func() {
			s.executionMutex.Unlock()
			s.isExecuting = false
			if r := recover(); r != nil {
				logging.Errorf("PANIC in computeQualityScores: %v", r)
				s.updateSchedulerStatus("idle", fmt.Sprintf("Panic: %v", r), nil, nil)
			}
		}()

		s.runComputeCycle("scheduled")
	})
}

func (s *TagQualityScoreScheduler) runComputeCycle(triggerSource string) {
	startTime := time.Now()
	summary := &TagQualityScoreRunSummary{
		TriggerSource: triggerSource,
		StartedAt:     startTime.Format(time.RFC3339),
	}
	s.updateSchedulerStatus("running", "", nil, nil)

	var updatedCount int64
	if err := database.DB.Model(&models.TopicTag{}).Where("status = ?", "active").Count(&updatedCount).Error; err != nil {
		updatedCount = 0
	}

	if err := topicextraction.ComputeAllQualityScores(); err != nil {
		summary.FinishedAt = time.Now().Format(time.RFC3339)
		summary.Reason = err.Error()
		s.updateSchedulerStatus("failed", err.Error(), &startTime, summary)
		return
	}

	summary.FinishedAt = time.Now().Format(time.RFC3339)
	summary.UpdatedCount = int(updatedCount)
	summary.Reason = "quality scores recomputed"
	s.updateSchedulerStatus("success", "", &startTime, summary)
}

func (s *TagQualityScoreScheduler) updateSchedulerStatus(status, lastError string, startTime *time.Time, summary *TagQualityScoreRunSummary) {
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
	if err := database.DB.Where("name = ?", "tag_quality_score").First(&task).Error; err == nil {
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
		Name:              "tag_quality_score",
		Description:       "Recompute persistent quality scores for topic tags",
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

func (s *TagQualityScoreScheduler) GetStatus() SchedulerStatusResponse {
	entries := s.cron.Entries()
	var nextRun int64
	if len(entries) > 0 {
		nextRun = entries[0].Next.Unix()
	}

	var task models.SchedulerTask
	err := database.DB.Where("name = ?", "tag_quality_score").First(&task).Error

	status := SchedulerStatusResponse{
		Name: "Tag Quality Score",
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
