package jobs

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"syntopica-backend/internal/domain/preferences"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
)

type PreferenceUpdateScheduler struct {
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

func NewPreferenceUpdateScheduler(checkInterval int) *PreferenceUpdateScheduler {
	return &PreferenceUpdateScheduler{
		checkInterval: checkInterval,
		stopChan:      make(chan bool),
		running:       false,
	}
}

func (s *PreferenceUpdateScheduler) Start() error {
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
				s.runUpdate()
				s.updateNextRun(time.Now().Add(time.Duration(s.checkInterval) * time.Second))
			case <-s.stopChan:
				logging.Infof("Preference update scheduler stopped")
				return
			}
		}
	}()

	logging.Infof("Preference update scheduler started (interval: %d seconds)", s.checkInterval)
	return nil
}

func (s *PreferenceUpdateScheduler) Stop() {
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

func (s *PreferenceUpdateScheduler) runUpdate() {
	tracing.TraceSchedulerTick("preference_update", "cron", func(ctx context.Context) {
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

		logging.Infof("Running preference update...")

		preferenceService := preferences.NewPreferenceService(database.DB)
		if err := preferenceService.UpdateAllPreferences(); err != nil {
			s.mu.Lock()
			s.totalRuns++
			s.failedRuns++
			s.lastError = err.Error()
			s.mu.Unlock()
			logging.Errorf("Preference update failed: %v", err)
		} else {
			s.mu.Lock()
			s.totalRuns++
			s.successRuns++
			s.lastError = ""
			s.mu.Unlock()
			logging.Infof("Preference update completed successfully")
		}
	})
}

func (s *PreferenceUpdateScheduler) TriggerNow() map[string]interface{} {
	s.mu.Lock()
	if s.isExecuting {
		s.mu.Unlock()
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     "偏好更新正在执行中，稍后再试。",
			"status_code": http.StatusConflict,
		}
	}
	s.mu.Unlock()

	logging.Infof("Manual preference update triggered")
	s.runUpdate()

	return map[string]interface{}{
		"accepted": true,
		"started":  true,
		"message":  "Preference update triggered",
	}
}

func (s *PreferenceUpdateScheduler) TriggerManualUpdate() {
	_ = s.TriggerNow()
}

func (s *PreferenceUpdateScheduler) GetStatus() SchedulerStatusResponse {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := "stopped"
	if s.isExecuting {
		status = "running"
	} else if s.running {
		status = "idle"
	}

	return SchedulerStatusResponse{
		Name:          "Preference Update",
		Status:        status,
		CheckInterval: int64(s.checkInterval),
		NextRun:       optionalTimeToUnix(s.nextRun),
		IsExecuting:   s.isExecuting,
	}
}

func (s *PreferenceUpdateScheduler) ResetStats() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastRun = nil
	s.lastError = ""
	s.totalRuns = 0
	s.successRuns = 0
	s.failedRuns = 0
	return nil
}

func (s *PreferenceUpdateScheduler) UpdateInterval(interval int) error {
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

func (s *PreferenceUpdateScheduler) updateNextRun(next time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextRun = &next
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

func optionalTimeToUnix(value *time.Time) int64 {
	if value == nil {
		return 0
	}
	return value.Unix()
}
