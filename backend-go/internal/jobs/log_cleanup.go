package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
)

const logCleanupRetentionDays = 7
const logCleanupStartupDelay = 5 * time.Minute

type LogCleanupScheduler struct {
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

	lastAICallLogsDeleted int64
	lastOtelSpansDeleted  int64
}

func NewLogCleanupScheduler(intervalSeconds int) *LogCleanupScheduler {
	return &LogCleanupScheduler{
		checkInterval: intervalSeconds,
		stopChan:      make(chan bool),
		running:       false,
	}
}

func (s *LogCleanupScheduler) Start() error {
	s.mu.Lock()

	if s.running {
		s.mu.Unlock()
		return nil
	}

	s.running = true
	s.wg.Add(1)
	nextRun := time.Now().Add(logCleanupStartupDelay)
	s.nextRun = &nextRun
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()

		timer := time.NewTimer(logCleanupStartupDelay)
		defer timer.Stop()

		select {
		case <-timer.C:
			s.runCleanupCycle()
		case <-s.stopChan:
			logging.Infof("Log cleanup scheduler stopped during startup delay")
			return
		}

		ticker := time.NewTicker(time.Duration(s.checkInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.runCleanupCycle()
				s.updateNextRun(time.Now().Add(time.Duration(s.checkInterval) * time.Second))
			case <-s.stopChan:
				logging.Infof("Log cleanup scheduler stopped")
				return
			}
		}
	}()

	logging.Infof("Log cleanup scheduler started (interval: %d seconds, first run in %v)", s.checkInterval, logCleanupStartupDelay)
	return nil
}

func (s *LogCleanupScheduler) Stop() {
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

func (s *LogCleanupScheduler) runCleanupCycle() {
	tracing.TraceSchedulerTick("log_cleanup", "cron", func(ctx context.Context) {
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

		logging.Infof("Running log cleanup...")

		cutoff := time.Now().AddDate(0, 0, -logCleanupRetentionDays)

		var aiCallLogsDeleted int64
		result := database.DB.Exec("DELETE FROM ai_call_logs WHERE created_at < ?", cutoff)
		if result.Error != nil {
			s.mu.Lock()
			s.totalRuns++
			s.failedRuns++
			s.lastError = result.Error.Error()
			s.mu.Unlock()
			logging.Errorf("LogCleanup: failed to clean ai_call_logs: %v", result.Error)
			return
		}
		aiCallLogsDeleted = result.RowsAffected

		var otelSpansDeleted int64
		cutoffNano := cutoff.UnixNano()
		result = database.DB.Exec("DELETE FROM otel_spans WHERE start_time_unix_nano < ?", cutoffNano)
		if result.Error != nil {
			s.mu.Lock()
			s.totalRuns++
			s.failedRuns++
			s.lastError = result.Error.Error()
			s.mu.Unlock()
			logging.Errorf("LogCleanup: failed to clean otel_spans: %v", result.Error)
			return
		}
		otelSpansDeleted = result.RowsAffected

		s.mu.Lock()
		s.totalRuns++
		s.successRuns++
		s.lastError = ""
		s.lastAICallLogsDeleted = aiCallLogsDeleted
		s.lastOtelSpansDeleted = otelSpansDeleted
		s.mu.Unlock()

		if aiCallLogsDeleted == 0 && otelSpansDeleted == 0 {
			logging.Infof("Log cleanup completed: no rows to clean")
		} else {
			logging.Infof("Log cleanup completed: ai_call_logs=%d, otel_spans=%d", aiCallLogsDeleted, otelSpansDeleted)
		}
	})
}

func (s *LogCleanupScheduler) TriggerNow() map[string]interface{} {
	s.mu.Lock()
	if s.isExecuting {
		s.mu.Unlock()
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     "日志清理正在执行中，稍后再试。",
			"status_code": 409,
		}
	}
	s.mu.Unlock()

	logging.Infof("Manual log cleanup triggered")
	s.runCleanupCycle()

	s.mu.Lock()
	defer s.mu.Unlock()

	return map[string]interface{}{
		"accepted":                  true,
		"started":                   true,
		"message":                   "Log cleanup triggered",
		"last_ai_call_logs_deleted": s.lastAICallLogsDeleted,
		"last_otel_spans_deleted":   s.lastOtelSpansDeleted,
	}
}

func (s *LogCleanupScheduler) GetStatus() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := "stopped"
	if s.isExecuting {
		status = "running"
	} else if s.running {
		status = "idle"
	}

	return map[string]interface{}{
		"status":                    status,
		"check_interval":            s.checkInterval,
		"is_executing":              s.isExecuting,
		"next_run":                  formatOptionalTime(s.nextRun),
		"last_execution_time":       formatOptionalTime(s.lastRun),
		"last_error":                s.lastError,
		"total_executions":          s.totalRuns,
		"successful_executions":     s.successRuns,
		"failed_executions":         s.failedRuns,
		"last_ai_call_logs_deleted": s.lastAICallLogsDeleted,
		"last_otel_spans_deleted":   s.lastOtelSpansDeleted,
	}
}

func (s *LogCleanupScheduler) ResetStats() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastRun = nil
	s.lastError = ""
	s.totalRuns = 0
	s.successRuns = 0
	s.failedRuns = 0
	s.lastAICallLogsDeleted = 0
	s.lastOtelSpansDeleted = 0
	return nil
}

func (s *LogCleanupScheduler) UpdateInterval(interval int) error {
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

func (s *LogCleanupScheduler) updateNextRun(next time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextRun = &next
}
