package scheduler

import (
	"encoding/json"
	"time"

	"syntopica-backend/internal/platform/database"
)

// NewTaskPersistence creates a TaskPersistence with standard behavior:
// create/update the SchedulerTask row with execution stats.
func NewTaskPersistence(name, description string) *TaskPersistence {
	return &TaskPersistence{
		InitTask: func(n string, interval time.Duration) {
			initSchedulerTask(name, description, interval, nil)
		},
		UpdateTask: func(n string, status string, startTime *time.Time, err error, result *JobResult) {
			updateSchedulerTask(name, description, n, status, startTime, err, result, nil)
		},
		ResetTask: func(n string) error {
			return resetSchedulerTask(name, n)
		},
	}
}

// NewTaskPersistenceWithNextRun creates a TaskPersistence that uses a NextRunFn
// to compute the next execution time instead of interval-based calculation.
func NewTaskPersistenceWithNextRun(name, description string, nextRunFn func(time.Time) time.Time) *TaskPersistence {
	return &TaskPersistence{
		InitTask: func(n string, interval time.Duration) {
			initSchedulerTask(name, description, interval, nextRunFn)
		},
		UpdateTask: func(n string, status string, startTime *time.Time, err error, result *JobResult) {
			updateSchedulerTask(name, description, n, status, startTime, err, result, nextRunFn)
		},
		ResetTask: func(n string) error {
			return resetSchedulerTask(name, n)
		},
		NextRunFn: nextRunFn,
	}
}

func initSchedulerTask(name, description string, interval time.Duration, nextRunFn func(time.Time) time.Time) {
	var task SchedulerTask
	now := time.Now()
	var nextRun time.Time
	if nextRunFn != nil {
		nextRun = nextRunFn(now)
	} else {
		nextRun = now.Add(interval)
	}

	if err := database.DB.Where("name = ?", name).First(&task).Error; err == nil {
		updates := map[string]interface{}{
			"description":         description,
			"check_interval":      int(interval.Seconds()),
			"next_execution_time": &nextRun,
		}
		if task.Status == "" || task.Status == "success" || task.Status == "failed" || task.Status == "running" {
			updates["status"] = "idle"
			if task.LastError != "" {
				updates["last_error"] = ""
			}
		}
		database.DB.Model(&task).Updates(updates)
		return
	}

	task = SchedulerTask{
		Name:              name,
		Description:       description,
		CheckInterval:     int(interval.Seconds()),
		Status:            "idle",
		NextExecutionTime: &nextRun,
	}
	database.DB.Create(&task)
}

func updateSchedulerTask(name, description string, _ string, status string, startTime *time.Time, err error, result *JobResult, nextRunFn func(time.Time) time.Time) {
	now := time.Now()

	// Calculate next execution time and check interval
	var nextExecution time.Time
	checkInterval := 60 // default seconds
	if nextRunFn != nil {
		nextExecution = nextRunFn(now)
	} else {
		// Calculate interval from the existing task or use default
		interval := 60 * time.Second
		var existing SchedulerTask
		if err2 := database.DB.Where("name = ?", name).First(&existing).Error; err2 == nil {
			if existing.CheckInterval > 0 {
				interval = time.Duration(existing.CheckInterval) * time.Second
			}
		}
		nextExecution = now.Add(interval)
		checkInterval = int(interval.Seconds())
	}

	resultJSON := ""
	if result != nil {
		summaryData := map[string]interface{}{
			"trigger_source": "scheduled",
		}
		if startTime != nil {
			summaryData["started_at"] = startTime.Format(time.RFC3339)
		}
		summaryData["finished_at"] = now.Format(time.RFC3339)
		if result.Data != nil {
			for k, v := range result.Data {
				summaryData[k] = v
			}
		}
		if result.Summary != "" {
			summaryData["reason"] = result.Summary
		}
		if encoded, marshalErr := json.Marshal(summaryData); marshalErr == nil {
			resultJSON = string(encoded)
		}
	} else if err != nil {
		summaryData := map[string]interface{}{
			"trigger_source": "scheduled",
			"finished_at":    now.Format(time.RFC3339),
			"error":          err.Error(),
		}
		if startTime != nil {
			summaryData["started_at"] = startTime.Format(time.RFC3339)
		}
		if encoded, marshalErr := json.Marshal(summaryData); marshalErr == nil {
			resultJSON = string(encoded)
		}
	}

	hasError := err != nil

	updates := map[string]interface{}{
		"status":              status,
		"next_execution_time": &nextExecution,
	}
	if hasError {
		updates["last_error"] = err.Error()
		errTime := now
		updates["last_error_time"] = &errTime
	} else {
		updates["last_error"] = ""
		updates["last_error_time"] = nil
	}

	if startTime != nil {
		duration := float64(time.Since(*startTime).Seconds())
		updates["last_execution_time"] = &now
		updates["last_execution_duration"] = &duration
		updates["last_execution_result"] = resultJSON
	}

	var task SchedulerTask
	if err2 := database.DB.Where("name = ?", name).First(&task).Error; err2 == nil {
		if startTime != nil {
			updates["total_executions"] = task.TotalExecutions + 1
			if hasError {
				updates["failed_executions"] = task.FailedExecutions + 1
				updates["consecutive_failures"] = task.ConsecutiveFailures + 1
			} else {
				updates["successful_executions"] = task.SuccessfulExecutions + 1
				updates["consecutive_failures"] = 0
			}
		}
		database.DB.Model(&task).Updates(updates)
		return
	}

	// Task doesn't exist yet - create it
	task = SchedulerTask{
		Name:              name,
		Description:       description,
		CheckInterval:     checkInterval,
		Status:            status,
		LastError:         "",
		NextExecutionTime: &nextExecution,
	}
	if hasError {
		task.LastError = err.Error()
		errTime := now
		task.LastErrorTime = &errTime
	}
	if startTime != nil {
		duration := float64(time.Since(*startTime).Seconds())
		task.LastExecutionTime = &now
		task.LastExecutionDuration = &duration
		task.LastExecutionResult = resultJSON
		task.TotalExecutions = 1
		if hasError {
			task.FailedExecutions = 1
			task.ConsecutiveFailures = 1
		} else {
			task.SuccessfulExecutions = 1
		}
	}
	database.DB.Create(&task)
}

func resetSchedulerTask(name, _ string) error {
	var task SchedulerTask
	if err := database.DB.Where("name = ?", name).First(&task).Error; err != nil {
		return err
	}

	nextRun := time.Now().Add(time.Duration(task.CheckInterval) * time.Second)
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

// formatOptionalTime formats a *time.Time pointer to RFC3339 string.
func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}
