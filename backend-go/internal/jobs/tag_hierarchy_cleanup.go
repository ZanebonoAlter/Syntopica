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

// TagHierarchyCleanupScheduler runs a multi-phase tag cleanup cycle: zombie, zero-article, low-quality, stale-zero-score, flat merge, event-clustering, hierarchy pruning, whitespace-dup, degenerate-tree, description backfill
type TagHierarchyCleanupScheduler struct {
	cron           *cron.Cron
	checkInterval  time.Duration
	isRunning      atomic.Bool
	executionMutex sync.Mutex
	isExecuting    atomic.Bool
}

// TagHierarchyCleanupRunSummary records the results of a cleanup run
type TagHierarchyCleanupRunSummary struct {
	TriggerSource        string `json:"trigger_source"`
	StartedAt            string `json:"started_at"`
	FinishedAt           string `json:"finished_at"`
	ZombieDeleted        int    `json:"zombie_deleted"`
	LowQualityDeleted    int    `json:"low_quality_deleted"`
	EmptyNodesDeleted    int    `json:"empty_nodes_deleted"`
	SameLevelDeduped     int    `json:"same_level_deduped"`
	TemplateViolations   int    `json:"template_violations"`
	SectorsDeleted       int    `json:"sectors_deleted"`
	SectorsDeclining     int    `json:"sectors_declining"`
	AnchorSignals        int    `json:"anchor_signals"`
	LLMCallsTotal        int    `json:"llm_calls_total"`
	LLMBudgetTotal       int    `json:"llm_budget_total"`
	TimedOut             bool   `json:"timed_out"`
	Errors               int    `json:"errors"`
	Reason               string `json:"reason"`
}

// NewTagHierarchyCleanupScheduler creates a new scheduler
func NewTagHierarchyCleanupScheduler(checkInterval int) *TagHierarchyCleanupScheduler {
	return &TagHierarchyCleanupScheduler{
		cron:          cron.New(),
		checkInterval: time.Duration(checkInterval) * time.Second,
	}
}

// Start begins the scheduler
func (s *TagHierarchyCleanupScheduler) Start() error {
	if s.isRunning.Load() {
		return fmt.Errorf("tag-hierarchy-cleanup scheduler already running")
	}

	s.initSchedulerTask()
	scheduleExpr := fmt.Sprintf("@every %ds", int64(s.checkInterval.Seconds()))
	if _, err := s.cron.AddFunc(scheduleExpr, s.cleanupHierarchy); err != nil {
		return fmt.Errorf("failed to schedule tag-hierarchy-cleanup: %w", err)
	}

	s.cron.Start()
	s.isRunning.Store(true)
	logging.Infof("Tag-hierarchy-cleanup scheduler started with interval: %v", s.checkInterval)
	return nil
}

// Stop halts the scheduler
func (s *TagHierarchyCleanupScheduler) Stop() {
	if !s.isRunning.Load() {
		return
	}
	s.cron.Stop()
	s.isRunning.Store(false)
	logging.Infoln("Tag-hierarchy-cleanup scheduler stopped")
}

// UpdateInterval changes the check interval
func (s *TagHierarchyCleanupScheduler) UpdateInterval(interval int) error {
	if interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}

	wasRunning := s.isRunning.Load()
	if wasRunning {
		s.Stop()
	}

	s.cron = cron.New()
	s.checkInterval = time.Duration(interval) * time.Second

	if wasRunning {
		return s.Start()
	}

	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", "tag_hierarchy_cleanup").First(&task).Error; err == nil {
		nextRun := time.Now().Add(s.checkInterval)
		database.DB.Model(&task).Updates(map[string]interface{}{
			"check_interval":      interval,
			"next_execution_time": &nextRun,
		})
	}

	return nil
}

// ResetStats resets scheduler statistics
func (s *TagHierarchyCleanupScheduler) ResetStats() error {
	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", "tag_hierarchy_cleanup").First(&task).Error; err != nil {
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

// TriggerNow manually triggers a cleanup run
func (s *TagHierarchyCleanupScheduler) TriggerNow() map[string]interface{} {
	if !s.executionMutex.TryLock() {
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     "标签层级清理正在执行中，请稍后再试。",
			"status_code": http.StatusConflict,
		}
	}

	s.isExecuting.Store(true)
	go func() {
		defer s.executionMutex.Unlock()
		defer func() {
			s.isExecuting.Store(false)
			if r := recover(); r != nil {
				logging.Errorf("PANIC in manual tag-hierarchy-cleanup trigger: %v", r)
				s.updateSchedulerStatus("idle", fmt.Sprintf("Panic: %v", r), nil, nil)
			}
		}()
		s.runCleanupCycle(context.Background(), "manual")
	}()

	return map[string]interface{}{
		"accepted": true,
		"started":  true,
		"reason":   "manual_run_started",
		"message":  "标签层级清理已经开始运行。",
	}
}

func (s *TagHierarchyCleanupScheduler) initSchedulerTask() {
	var task models.SchedulerTask
	now := time.Now()
	nextRun := now.Add(s.checkInterval)

	if err := database.DB.Where("name = ?", "tag_hierarchy_cleanup").First(&task).Error; err == nil {
		if task.CheckInterval > 0 {
			s.checkInterval = time.Duration(task.CheckInterval) * time.Second
			nextRun = now.Add(s.checkInterval)
		}
		updates := map[string]interface{}{
			"description":         "multi-phase tag cleanup: zombie, zero-article, low-quality, stale-zero-score, flat merge, event-clustering, hierarchy pruning, whitespace-dup, degenerate-tree, description backfill",
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
		Name:              "tag_hierarchy_cleanup",
		Description:       "multi-phase tag cleanup: zombie, zero-article, low-quality, stale-zero-score, flat merge, event-clustering, hierarchy pruning, whitespace-dup, degenerate-tree, clustering, queued multi-parent, adopt narrower, abstract update, tree review, description backfill",
		CheckInterval:     int(s.checkInterval.Seconds()),
		Status:            "idle",
		NextExecutionTime: &nextRun,
	}
	database.DB.Create(&task)
}

func (s *TagHierarchyCleanupScheduler) cleanupHierarchy() {
	tracing.TraceSchedulerTick("tag_hierarchy_cleanup", "cron", func(ctx context.Context) {
		if !s.executionMutex.TryLock() {
			logging.Infoln("Tag hierarchy cleanup already in progress, skipping this cycle")
			return
		}
		s.isExecuting.Store(true)
		defer func() {
			s.executionMutex.Unlock()
			s.isExecuting.Store(false)
			if r := recover(); r != nil {
				logging.Errorf("PANIC in cleanupHierarchy: %v", r)
				s.updateSchedulerStatus("idle", fmt.Sprintf("Panic: %v", r), nil, nil)
			}
		}()

		s.runCleanupCycle(ctx, "scheduled")
	})
}

func (s *TagHierarchyCleanupScheduler) runCleanupCycle(ctx context.Context, triggerSource string) {
	ctx, span := otel.Tracer("rss-reader-backend").Start(ctx, "workflow.hierarchy_cleanup.cycle")
	defer span.End()
	span.SetAttributes(
		attribute.String("workflow.name", "hierarchy_cleanup"),
		attribute.String("workflow.domain", "tag_management"),
		attribute.String("workflow.trigger", triggerSource),
	)
	m1, _ := baggage.NewMember("workflow.name", "hierarchy_cleanup")
	m2, _ := baggage.NewMember("workflow.domain", "tag_management")
	m3, _ := baggage.NewMember("workflow.trigger", triggerSource)
	bag, _ := baggage.New(m1, m2, m3)
	ctx = baggage.ContextWithBaggage(ctx, bag)

	startTime := time.Now()
	summary := &TagHierarchyCleanupRunSummary{
		TriggerSource: triggerSource,
		StartedAt:     startTime.Format(time.RFC3339),
	}
	s.updateSchedulerStatus("running", "", nil, nil)

	budget := NewCleanupBudget(60, 30*time.Minute)
	categories := []string{"event", "keyword", "person"}

	logging.Infoln("Starting tag cleanup cycle V2 (7-phase)")

	// Phase 1: Hard-delete zombie tags
	for _, cat := range categories {
		if budget.IsTimedOut() {
			break
		}
		n, err := tagging.CleanupZombieTagsV2(database.DB, cat)
		if err != nil {
			logging.Errorf("Phase 1 zombie cleanup failed for %s: %v", cat, err)
			summary.Errors++
		} else {
			summary.ZombieDeleted += n
			logging.Infof("Phase 1 (%s): hard-deleted %d zombie tags", cat, n)
		}
	}

	// Phase 2: Hard-delete low quality tags
	for _, cat := range categories {
		if budget.IsTimedOut() {
			break
		}
		n, err := tagging.CleanupLowQualityTagsV2(database.DB, cat)
		if err != nil {
			logging.Errorf("Phase 2 low-quality cleanup failed for %s: %v", cat, err)
			summary.Errors++
		} else {
			summary.LowQualityDeleted += n
			logging.Infof("Phase 2 (%s): hard-deleted %d low-quality tags", cat, n)
		}
	}

	// Phase 3: Hard-delete empty abstract nodes
	for _, cat := range categories {
		if budget.IsTimedOut() {
			break
		}
		n, err := tagging.CleanupEmptyNodesV2(database.DB, cat)
		if err != nil {
			logging.Errorf("Phase 3 empty nodes cleanup failed for %s: %v", cat, err)
			summary.Errors++
		} else {
			summary.EmptyNodesDeleted += n
			logging.Infof("Phase 3 (%s): hard-deleted %d empty nodes", cat, n)
		}
	}

	// Phase 4: Same-level dedup
	for _, cat := range categories {
		if budget.IsTimedOut() {
			break
		}
		n, err := tagging.CleanupSameLevelDuplicates(database.DB, cat)
		if err != nil {
			logging.Errorf("Phase 4 same-level dedup failed for %s: %v", cat, err)
			summary.Errors++
		} else {
			summary.SameLevelDeduped += n
			logging.Infof("Phase 4 (%s): deduped %d same-level tags", cat, n)
		}
	}

	// Phase 5: Template compliance → pending changes
	for _, cat := range categories {
		if budget.IsTimedOut() {
			break
		}
		n, err := tagging.CleanupTemplateViolationsV2(database.DB, cat)
		if err != nil {
			logging.Errorf("Phase 5 template violations failed for %s: %v", cat, err)
			summary.Errors++
		} else {
			summary.TemplateViolations += n
			logging.Infof("Phase 5 (%s): %d template violations", cat, n)
		}
	}

	// Phase 6: Sector health check
	for _, cat := range categories {
		if budget.IsTimedOut() {
			break
		}
		deletedAuto, markedDeclining, err := tagging.CheckSectorHealth(ctx, database.DB, cat)
		if err != nil {
			logging.Errorf("Phase 6 sector health failed for %s: %v", cat, err)
			summary.Errors++
		} else {
			summary.SectorsDeleted += deletedAuto
			summary.SectorsDeclining += markedDeclining
			logging.Infof("Phase 6 (%s): deleted %d auto sectors, marked %d declining", cat, deletedAuto, markedDeclining)
		}
	}

	// Phase 7: Generate anchor signals
	for _, cat := range categories {
		if budget.IsTimedOut() {
			break
		}
		signals, err := tagging.GenerateAnchorSignals(ctx, database.DB, cat)
		if err != nil {
			logging.Errorf("Phase 7 anchor signals failed for %s: %v", cat, err)
			summary.Errors++
		} else {
			summary.AnchorSignals += len(signals)
			logging.Infof("Phase 7 (%s): %d anchor signals generated", cat, len(signals))
		}
	}

	summary.FinishedAt = time.Now().Format(time.RFC3339)
	budgetStats := budget.Stats()
	summary.LLMCallsTotal = budgetStats.TotalConsumed
	summary.LLMBudgetTotal = budgetStats.TotalBudget
	summary.TimedOut = budgetStats.TimedOut
	summary.Reason = fmt.Sprintf("zombie=%d, low_quality=%d, empty_nodes=%d, deduped=%d, violations=%d, sectors_deleted=%d, sectors_declining=%d, anchors=%d",
		summary.ZombieDeleted, summary.LowQualityDeleted, summary.EmptyNodesDeleted,
		summary.SameLevelDeduped, summary.TemplateViolations,
		summary.SectorsDeleted, summary.SectorsDeclining, summary.AnchorSignals)

	logging.Infof("Tag cleanup V2 cycle completed: %s", summary.Reason)

	if summary.Errors > 0 {
		s.updateSchedulerStatus("success_with_errors", "", &startTime, summary)
	} else {
		s.updateSchedulerStatus("success", "", &startTime, summary)
	}
}

func (s *TagHierarchyCleanupScheduler) updateSchedulerStatus(status, lastError string, startTime *time.Time, summary *TagHierarchyCleanupRunSummary) {
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
	if err := database.DB.Where("name = ?", "tag_hierarchy_cleanup").First(&task).Error; err == nil {
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

	task = models.SchedulerTask{
		Name:              "tag_hierarchy_cleanup",
		Description:       "multi-phase tag cleanup: zombie, zero-article, low-quality, stale-zero-score, flat merge, event-clustering, hierarchy pruning, whitespace-dup, degenerate-tree, description backfill",
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
		case "success", "success_with_errors":
			task.SuccessfulExecutions = 1
		case "failed":
			task.FailedExecutions = 1
			task.ConsecutiveFailures = 1
			task.LastErrorTime = &now
		}
	}
	database.DB.Create(&task)
}

// GetStatus returns the current scheduler status
func (s *TagHierarchyCleanupScheduler) GetStatus() SchedulerStatusResponse {
	entries := s.cron.Entries()
	var nextRun int64
	if len(entries) > 0 {
		nextRun = entries[0].Next.Unix()
	}

	var task models.SchedulerTask
	err := database.DB.Where("name = ?", "tag_hierarchy_cleanup").First(&task).Error

	status := SchedulerStatusResponse{
		Name: "Tag Hierarchy Cleanup",
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
