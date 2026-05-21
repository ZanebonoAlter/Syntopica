package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/tagging"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
	"my-robot-backend/internal/platform/tracing"
)

// TagHierarchyPlacementScheduler runs tag placement: retry orphans + aggregate orphan abstracts.
type TagHierarchyPlacementScheduler struct {
	cron           *cron.Cron
	checkInterval  time.Duration
	isRunning      atomic.Bool
	executionMutex sync.Mutex
	isExecuting    atomic.Bool
}

// TagHierarchyPlacementRunSummary records the results of a placement run
type TagHierarchyPlacementRunSummary struct {
	TriggerSource    string `json:"trigger_source"`
	StartedAt        string `json:"started_at"`
	FinishedAt       string `json:"finished_at"`
	OrphanRetried    int    `json:"orphan_retried"`
	OrphanAggregated int    `json:"orphan_aggregated"`
	Errors           int    `json:"errors"`
}

// NewTagHierarchyPlacementScheduler creates a new scheduler
func NewTagHierarchyPlacementScheduler(checkInterval int) *TagHierarchyPlacementScheduler {
	return &TagHierarchyPlacementScheduler{
		cron:          cron.New(),
		checkInterval: time.Duration(checkInterval) * time.Second,
	}
}

// Start begins the scheduler
func (s *TagHierarchyPlacementScheduler) Start() error {
	if s.isRunning.Load() {
		return fmt.Errorf("tag-hierarchy-placement scheduler already running")
	}

	s.initSchedulerTask()
	scheduleExpr := fmt.Sprintf("@every %ds", int64(s.checkInterval.Seconds()))
	if _, err := s.cron.AddFunc(scheduleExpr, s.runPlacementCycle); err != nil {
		return fmt.Errorf("failed to schedule tag-hierarchy-placement: %w", err)
	}

	s.cron.Start()
	s.isRunning.Store(true)
	logging.Infof("Tag-hierarchy-placement scheduler started with interval: %v", s.checkInterval)
	return nil
}

// Stop halts the scheduler
func (s *TagHierarchyPlacementScheduler) Stop() {
	if !s.isRunning.Load() {
		return
	}
	s.cron.Stop()
	s.isRunning.Store(false)
	logging.Infoln("Tag-hierarchy-placement scheduler stopped")
}

// TriggerNow manually triggers a placement run
func (s *TagHierarchyPlacementScheduler) TriggerNow() map[string]interface{} {
	if !s.executionMutex.TryLock() {
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     "标签层级放置正在执行中，请稍后再试。",
			"status_code": http.StatusConflict,
		}
	}

	s.isExecuting.Store(true)
	go func() {
		defer s.executionMutex.Unlock()
		defer func() {
			s.isExecuting.Store(false)
			if r := recover(); r != nil {
				logging.Errorf("PANIC in manual tag-hierarchy-placement trigger: %v", r)
				s.updateSchedulerStatus("idle", fmt.Sprintf("Panic: %v", r), nil, nil)
			}
		}()
		s.executePlacementCycle(context.Background(), "manual")
	}()

	return map[string]interface{}{
		"accepted": true,
		"started":  true,
		"reason":   "manual_run_started",
		"message":  "标签层级放置已经开始运行。",
	}
}

func (s *TagHierarchyPlacementScheduler) initSchedulerTask() {
	var task models.SchedulerTask
	now := time.Now()
	nextRun := now.Add(s.checkInterval)

	if err := database.DB.Where("name = ?", "tag_hierarchy_placement").First(&task).Error; err == nil {
		if task.CheckInterval > 0 {
			s.checkInterval = time.Duration(task.CheckInterval) * time.Second
			nextRun = now.Add(s.checkInterval)
		}
		updates := map[string]interface{}{
			"description":         "tag hierarchy placement: retry orphan placements + aggregate orphan abstracts",
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
		Name:              "tag_hierarchy_placement",
		Description:       "tag hierarchy placement: retry orphan placements + aggregate orphan abstracts",
		CheckInterval:     int(s.checkInterval.Seconds()),
		Status:            "idle",
		NextExecutionTime: &nextRun,
	}
	database.DB.Create(&task)
}

func (s *TagHierarchyPlacementScheduler) runPlacementCycle() {
	tracing.TraceSchedulerTick("tag_hierarchy_placement", "cron", func(ctx context.Context) {
		if !s.executionMutex.TryLock() {
			logging.Infoln("Tag hierarchy placement already in progress, skipping this cycle")
			return
		}
		s.isExecuting.Store(true)
		defer func() {
			s.executionMutex.Unlock()
			s.isExecuting.Store(false)
			if r := recover(); r != nil {
				logging.Errorf("PANIC in runPlacementCycle: %v", r)
				s.updateSchedulerStatus("idle", fmt.Sprintf("Panic: %v", r), nil, nil)
			}
		}()

		s.executePlacementCycle(ctx, "scheduled")
	})
}

func (s *TagHierarchyPlacementScheduler) executePlacementCycle(ctx context.Context, triggerSource string) {
	ctx, span := otel.Tracer("rss-reader-backend").Start(ctx, "workflow.hierarchy_placement.cycle")
	defer span.End()
	span.SetAttributes(
		attribute.String("workflow.name", "hierarchy_placement"),
		attribute.String("workflow.domain", "tag_management"),
		attribute.String("workflow.trigger", triggerSource),
	)
	m1, _ := baggage.NewMember("workflow.name", "hierarchy_placement")
	m2, _ := baggage.NewMember("workflow.domain", "tag_management")
	m3, _ := baggage.NewMember("workflow.trigger", triggerSource)
	bag, _ := baggage.New(m1, m2, m3)
	ctx = baggage.ContextWithBaggage(ctx, bag)

	startTime := time.Now()
	summary := &TagHierarchyPlacementRunSummary{
		TriggerSource: triggerSource,
		StartedAt:     startTime.Format(time.RFC3339),
	}
	s.updateSchedulerStatus("running", "", nil, nil)

	logging.Infoln("Starting tag hierarchy placement cycle")

	orchestrator := tagging.NewHierarchyOrchestrationService(database.DB)

	// Aggregate orphan abstracts for each category
	for _, category := range []string{"event", "keyword", "person"} {
		closure, err := orchestrator.RunCategoryClosureFlow(ctx, category)
		if err != nil {
			logging.Errorf("RunCategoryClosureFlow %s failed: %v", category, err)
			summary.Errors++
		} else if closure != nil {
			summary.OrphanRetried += closure.PlacedCount
			if closure.Bootstrapped {
				logging.Infof("RunCategoryClosureFlow %s: sector bootstrapped", category)
			}
		}

		aggregated, err := tagging.AggregateOrphanTags(ctx, category)
		if err != nil {
			logging.Errorf("AggregateOrphanTags %s failed: %v", category, err)
			summary.Errors++
		} else {
			summary.OrphanAggregated += aggregated
		}
	}
	logging.Infof("AggregateOrphanTags: %d aggregated across all categories", summary.OrphanAggregated)

	summary.FinishedAt = time.Now().Format(time.RFC3339)
	logging.Infof("Tag hierarchy placement cycle completed: retried=%d aggregated=%d", summary.OrphanRetried, summary.OrphanAggregated)

	if summary.Errors > 0 {
		s.updateSchedulerStatus("success_with_errors", "", &startTime, summary)
	} else {
		s.updateSchedulerStatus("success", "", &startTime, summary)
	}
}

func (s *TagHierarchyPlacementScheduler) updateSchedulerStatus(status, lastError string, startTime *time.Time, summary *TagHierarchyPlacementRunSummary) {
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
	if err := database.DB.Where("name = ?", "tag_hierarchy_placement").First(&task).Error; err == nil {
		updates["total_executions"] = task.TotalExecutions
		updates["successful_executions"] = task.SuccessfulExecutions
		updates["failed_executions"] = task.FailedExecutions
		updates["consecutive_failures"] = task.ConsecutiveFailures

		if startTime != nil {
			updates["total_executions"] = task.TotalExecutions + 1
			switch status {
			case "success", "success_with_errors":
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

	task2 := models.SchedulerTask{
		Name:              "tag_hierarchy_placement",
		Description:       "tag hierarchy placement: retry orphan placements + aggregate orphan abstracts",
		CheckInterval:     int(s.checkInterval.Seconds()),
		Status:            status,
		LastError:         lastError,
		NextExecutionTime: &nextExecution,
	}
	if startTime != nil {
		duration := float64(time.Since(*startTime).Seconds())
		task2.LastExecutionTime = &now
		task2.LastExecutionDuration = &duration
		task2.LastExecutionResult = resultJSON
		task2.TotalExecutions = 1
		switch status {
		case "success", "success_with_errors":
			task2.SuccessfulExecutions = 1
		case "failed":
			task2.FailedExecutions = 1
			task2.ConsecutiveFailures = 1
			task2.LastErrorTime = &now
		}
	}
	database.DB.Create(&task2)
}

// GetStatus returns the current scheduler status
func (s *TagHierarchyPlacementScheduler) GetStatus() SchedulerStatusResponse {
	entries := s.cron.Entries()
	var nextRun int64
	if len(entries) > 0 {
		nextRun = entries[0].Next.Unix()
	}

	var task models.SchedulerTask
	err := database.DB.Where("name = ?", "tag_hierarchy_placement").First(&task).Error

	status := SchedulerStatusResponse{
		Name: "Tag Hierarchy Placement",
		Status: func() string {
			if s.isExecuting.Load() {
				return "running"
			}
			if s.isRunning.Load() {
				return "idle"
			}
			return "stopped"
		}(),
		CheckInterval: int64(s.checkInterval.Seconds()),
		NextRun:       nextRun,
		IsExecuting:   s.isExecuting.Load(),
	}
	if err == nil && task.NextExecutionTime != nil {
		status.NextRun = task.NextExecutionTime.Unix()
	}
	return status
}
