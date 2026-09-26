// Package scheduler provides a factory pattern for scheduler implementations,
// eliminating repetitive scaffolding via a unified BaseScheduler.
package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
)

// JobFunc is the pure business logic of a scheduler job.
type JobFunc func(ctx context.Context) (*JobResult, error)

// JobResult contains the result of a single job execution.
type JobResult struct {
	Data    map[string]interface{} // Business metrics surfaced in GetStatus / GetTaskStatusDetails
	Summary string                 // Human-readable log line
}

// StatusDetailFunc optionally returns extra status fields for the scheduler.
// Called after each job execution with the latest JobResult.
type StatusDetailFunc func(result *JobResult) map[string]interface{}

// TaskPersistence provides optional hooks to persist scheduler state to
// the SchedulerTask database table.
type TaskPersistence struct {
	// InitTask is called once during Start() to create/refresh the DB row.
	InitTask func(name string, interval time.Duration)
	// UpdateTask is called after each job execution.
	UpdateTask func(name string, status string, startTime *time.Time, err error, result *JobResult)
	// ResetTask resets the DB row when ResetStats() is called.
	ResetTask func(name string) error
	// NextRunFn optionally computes the next execution time for persistence.
	// When set, InitTask and UpdateTask use this instead of interval-based calculation.
	NextRunFn func(now time.Time) time.Time
}

// Config configures a BaseScheduler.
type Config struct {
	Name         string        // Display name (for logging)
	Interval     time.Duration // Tick interval
	StartupDelay time.Duration // Delay before first execution (0 = no extra delay)
	Job          JobFunc       // Business logic

	// NextRun is an optional callback that computes the next wall-clock
	// trigger time. When set, the scheduler ignores Interval/StartupDelay
	// and instead loops: compute next → sleep until next → run → recompute.
	// Each call uses a fresh time.Now(), so config changes are picked up
	// on the next cycle. Use for wall-clock-aligned schedules like DailyReport.
	NextRun func(now time.Time) time.Time

	// Optional enrichment. If nil, GetTaskStatusDetails falls back
	// to embedding JobResult.Data directly.
	StatusDetail StatusDetailFunc

	// Optional DB persistence. If nil, no SchedulerTask table interaction.
	Persistence *TaskPersistence

	// Auto-discovery metadata. The admin handler reads these fields to
	// surface any registry-registered scheduler in /api/schedulers/status
	// and make it triggerable via /trigger, without a second descriptor list.
	// Description is the UI text; TaskName is the SchedulerTask DB row key
	// used by the enrichStatus fallback (empty defaults to the registry key);
	// Aliases are alternative trigger names (e.g. "ai_summary" for
	// content_completion, kept for backward compatibility).
	Description string
	TaskName    string
	Aliases     []string
}

// BaseScheduler implements the Scheduler interface with unified state
// management, concurrency guards, and a Ticker-based scheduling loop.
//
// Usage:
//
//	s := scheduler.New(scheduler.Config{
//	    Name:     "Log Cleanup",
//	    Interval: 86400 * time.Second,
//	    Job:      logCleanupJob,
//	})
//	registry.Register("log_cleanup", s)
type BaseScheduler struct {
	cfg Config

	mu          sync.RWMutex
	running     bool
	isExecuting bool
	stopChan    chan struct{}
	wg          sync.WaitGroup

	nextRun     *time.Time
	lastRun     *time.Time
	lastError   string
	totalRuns   int
	successRuns int
	failedRuns  int
	lastResult  *JobResult
}

// New creates a new BaseScheduler with the given configuration.
func New(cfg Config) *BaseScheduler {
	return &BaseScheduler{
		cfg:      cfg,
		stopChan: make(chan struct{}),
	}
}

// GetConfig returns the scheduler's configuration. The admin handler reads
// Description/TaskName/Aliases from here for auto-discovery. Wrappers that
// embed *BaseScheduler (e.g. DailyReportSchedulerWrapper) inherit this method,
// so the wrapper's config is the wrapped base's config.
func (s *BaseScheduler) GetConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Start implements Scheduler.Start. It launches a background goroutine
// that runs the Job on each tick.
func (s *BaseScheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.running = true
	s.wg.Add(1)

	// Initialize persistence if configured
	if s.cfg.Persistence != nil && s.cfg.Persistence.InitTask != nil {
		s.cfg.Persistence.InitTask(s.cfg.Name, s.cfg.Interval)
	}

	firstDelay := s.cfg.Interval
	if s.cfg.StartupDelay > 0 {
		firstDelay = s.cfg.StartupDelay
	}
	nextRun := time.Now().Add(firstDelay)
	s.nextRun = &nextRun

	go func() {
		defer s.wg.Done()

		if s.cfg.NextRun != nil {
			// Wall-clock scheduling loop
			for {
				next := s.cfg.NextRun(time.Now())
				s.updateNextRun(next)
				delay := time.Until(next)
				if delay > 0 {
					timer := time.NewTimer(delay)
					select {
					case <-timer.C:
					case <-s.stopChan:
						timer.Stop()
						logging.Infof("%s scheduler stopped", s.cfg.Name)
						return
					}
				}
				s.runJob()
			}
		}

		// Interval-based scheduling loop (original path)
		if s.cfg.StartupDelay > 0 {
			timer := time.NewTimer(s.cfg.StartupDelay)
			defer timer.Stop()
			select {
			case <-timer.C:
				s.runJob()
			case <-s.stopChan:
				logging.Infof("%s scheduler stopped during startup delay", s.cfg.Name)
				return
			}
		}

		ticker := time.NewTicker(s.cfg.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.runJob()
				s.updateNextRun(time.Now().Add(s.cfg.Interval))
			case <-s.stopChan:
				logging.Infof("%s scheduler stopped", s.cfg.Name)
				return
			}
		}
	}()

	if s.cfg.NextRun != nil {
		logging.Infof("%s scheduler started (wall-clock mode)", s.cfg.Name)
	} else {
		logging.Infof("%s scheduler started (interval: %v, startupDelay: %v)", s.cfg.Name, s.cfg.Interval, s.cfg.StartupDelay)
	}
	return nil
}

// Stop implements Scheduler.Stop.
func (s *BaseScheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)
	s.mu.Unlock()

	// Wait outside the lock: runJob acquires s.mu to update state before
	// and after executing the job, so holding s.mu while waiting would
	// deadlock whenever Stop races an in-flight tick (the loop could never
	// exit and wg.Wait would block forever). Concurrent Stop/Start is not
	// supported (single-user system), so no new loop can be added while
	// we wait.
	s.wg.Wait()

	s.mu.Lock()
	s.stopChan = make(chan struct{})
	s.nextRun = nil
	s.mu.Unlock()
}

// TriggerNow implements the Scheduler interface. It executes the job
// synchronously (in the calling goroutine) and returns the result.
func (s *BaseScheduler) TriggerNow() map[string]interface{} {
	s.mu.Lock()
	if s.isExecuting {
		s.mu.Unlock()
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     fmt.Sprintf("%s 正在执行中，稍后再试。", s.cfg.Name),
			"status_code": 409,
		}
	}
	s.isExecuting = true
	s.mu.Unlock()

	logging.Infof("Manual %s triggered", s.cfg.Name)
	result, err := s.execute()

	s.mu.Lock()
	isExecuting := s.isExecuting
	s.isExecuting = false
	s.mu.Unlock()

	return s.buildTriggerNowResult(isExecuting, result, err)
}

// buildTriggerNowResult constructs the map returned by TriggerNow.
// It is separate so that sub-schedulers (e.g. DailyReportScheduler)
// can reuse the same logic with different execution paths.
func (s *BaseScheduler) buildTriggerNowResult(wasExecuting bool, result *JobResult, err error) map[string]interface{} {
	if !wasExecuting {
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "not_running",
			"message":     "调度器未在运行。",
			"status_code": 500,
		}
	}
	if err != nil {
		return map[string]interface{}{
			"accepted": true,
			"started":  true,
			"reason":   "execution_failed",
			"message":  err.Error(),
		}
	}

	resp := map[string]interface{}{
		"accepted": true,
		"started":  true,
		"message":  fmt.Sprintf("%s triggered", s.cfg.Name),
	}
	if result != nil && result.Data != nil {
		for k, v := range result.Data {
			resp[k] = v
		}
	}
	return resp
}

// UpdateInterval implements Scheduler.UpdateInterval.
func (s *BaseScheduler) UpdateInterval(seconds int) error {
	if seconds <= 0 {
		return fmt.Errorf("interval must be positive")
	}

	s.mu.Lock()
	wasRunning := s.running
	s.mu.Unlock()

	if wasRunning {
		s.Stop()
	}

	s.mu.Lock()
	s.cfg.Interval = time.Duration(seconds) * time.Second
	s.mu.Unlock()

	if wasRunning {
		return s.Start()
	}

	s.updateNextRun(time.Now().Add(time.Duration(seconds) * time.Second))

	// Persist interval change
	return nil
}

// ResetStats implements Scheduler.ResetStats.
func (s *BaseScheduler) ResetStats() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastRun = nil
	s.lastError = ""
	s.totalRuns = 0
	s.successRuns = 0
	s.failedRuns = 0
	s.lastResult = nil

	if s.cfg.Persistence != nil && s.cfg.Persistence.ResetTask != nil {
		return s.cfg.Persistence.ResetTask(s.cfg.Name)
	}
	return nil
}

// GetStatus returns the scheduler status as a map (legacy interface).
func (s *BaseScheduler) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := "stopped"
	if s.isExecuting {
		status = "running"
	} else if s.running {
		status = "idle"
	}

	m := map[string]interface{}{
		"status":                status,
		"check_interval":        int(s.cfg.Interval.Seconds()),
		"is_executing":          s.isExecuting,
		"next_run":              formatOptionalTime(s.nextRun),
		"last_execution_time":   formatOptionalTime(s.lastRun),
		"last_error":            s.lastError,
		"total_executions":      s.totalRuns,
		"successful_executions": s.successRuns,
		"failed_executions":     s.failedRuns,
	}

	// Embed business data from last result
	if s.lastResult != nil && s.lastResult.Data != nil {
		for k, v := range s.lastResult.Data {
			m[k] = v
		}
	}

	return m
}

// GetTaskStatusDetails returns enriched status for the handler's
// enrichStatus function.
func (s *BaseScheduler) GetTaskStatusDetails() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := "stopped"
	if s.isExecuting {
		status = "running"
	} else if s.running {
		status = "idle"
	}

	successRate := 0.0
	if s.totalRuns > 0 {
		successRate = float64(s.successRuns) / float64(s.totalRuns) * 100
	}

	lastExecutionTime := ""
	if s.lastRun != nil {
		lastExecutionTime = s.lastRun.Format(time.RFC3339)
	}

	databaseState := map[string]interface{}{
		"name":                  s.cfg.Name,
		"status":                status,
		"total_executions":      s.totalRuns,
		"successful_executions": s.successRuns,
		"failed_executions":     s.failedRuns,
		"last_execution_time":   lastExecutionTime,
		"last_error":            s.lastError,
		"success_rate":          successRate,
	}

	details := map[string]interface{}{
		"status":         status,
		"is_executing":   s.isExecuting,
		"check_interval": int(s.cfg.Interval.Seconds()),
		"next_run":       formatOptionalTime(s.nextRun),
		"database_state": databaseState,
	}

	// Build last_run_summary
	if s.lastResult != nil {
		summary := map[string]interface{}{}
		if s.lastRun != nil {
			summary["started_at"] = s.lastRun.Format(time.RFC3339)
		}
		if s.lastResult.Data != nil {
			for k, v := range s.lastResult.Data {
				summary[k] = v
			}
		}
		details["last_run_summary"] = summary
	}

	// Allow custom enrichment
	if s.cfg.StatusDetail != nil {
		extra := s.cfg.StatusDetail(s.lastResult)
		for k, v := range extra {
			details[k] = v
		}
	}

	return details
}

// TrySetExecuting attempts to set isExecuting to true.
// Returns true if the state was successfully set (was not executing before).
// Sub-schedulers (e.g. DailyReportSchedulerWrapper) use this instead of
// manipulating internal state directly.
func (s *BaseScheduler) TrySetExecuting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isExecuting {
		return false
	}
	s.isExecuting = true
	return true
}

// ClearExecuting sets isExecuting back to false.
func (s *BaseScheduler) ClearExecuting() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isExecuting = false
}

// runJob is the inner execution loop called on each tick.
func (s *BaseScheduler) runJob() {
	tracing.TraceSchedulerTick(s.cfg.Name, "cron", func(ctx context.Context) {
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

		result, err := s.execute()
		_ = result
		_ = err
	})
}

// execute runs the job function and updates internal state.
func (s *BaseScheduler) execute() (*JobResult, error) {
	startTime := time.Now()

	result, err := s.cfg.Job(context.Background())

	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalRuns++
	if err != nil {
		s.failedRuns++
		s.lastError = err.Error()
		s.lastResult = nil
	} else {
		s.successRuns++
		s.lastError = ""
		s.lastResult = result
	}

	// Log summary
	switch {
	case err != nil:
		logging.Errorf("%s: job failed: %v", s.cfg.Name, err)
	case result != nil && result.Summary != "":
		logging.Infof("%s: %s", s.cfg.Name, result.Summary)
	default:
		logging.Infof("%s: completed", s.cfg.Name)
	}

	// Persist to DB if configured
	if s.cfg.Persistence != nil && s.cfg.Persistence.UpdateTask != nil {
		status := "success"
		if err != nil {
			status = "failed"
		}
		s.cfg.Persistence.UpdateTask(s.cfg.Name, status, &startTime, err, result)
	}

	return result, err
}

func (s *BaseScheduler) updateNextRun(next time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextRun = &next
}
