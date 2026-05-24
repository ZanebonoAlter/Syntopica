package jobs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"syntopica-backend/internal/app/runtimeinfo"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
)

type UpdateSchedulerIntervalRequest struct {
	Interval int `json:"interval" binding:"required"`
}

type SchedulerStatusResponse struct {
	Name                   string                 `json:"name"`
	Status                 string                 `json:"status"`
	CheckInterval          int64                  `json:"check_interval"`
	NextRun                int64                  `json:"next_run"`
	IsExecuting            bool                   `json:"is_executing"`
	Description            string                 `json:"description,omitempty"`
	DatabaseState          map[string]interface{} `json:"database_state,omitempty"`
	Overview               map[string]interface{} `json:"overview,omitempty"`
	LastRunSummary         interface{}            `json:"last_run_summary,omitempty"`
	CurrentArticle         interface{}            `json:"current_article,omitempty"`
	LastProcessed          interface{}            `json:"last_processed,omitempty"`
	LiveProcessingCount    int                    `json:"live_processing_count,omitempty"`
	StaleProcessingCount   int                    `json:"stale_processing_count,omitempty"`
	StaleProcessingArticle interface{}            `json:"stale_processing_article,omitempty"`
	AIConfigured           bool                   `json:"ai_configured,omitempty"`
}

type schedulerDescriptor struct {
	Name        string
	DisplayName string
	Aliases     []string
	Description string
	TaskName    string
	Get         func() interface{}
}

func schedulerDescriptors() []schedulerDescriptor {
	return []schedulerDescriptor{
		{
			Name:        "auto_refresh",
			DisplayName: "Auto Refresh",
			Description: "Auto-refresh RSS feeds",
			TaskName:    "auto_refresh",
			Get: func() interface{} {
				return runtimeinfo.AutoRefreshSchedulerInterface
			},
		},
		{
			Name:        "preference_update",
			DisplayName: "Preference Update",
			Description: "Update reading preferences from behavior data",
			Get: func() interface{} {
				return runtimeinfo.PreferenceUpdateSchedulerInterface
			},
		},
		{
			Name:        "content_completion",
			DisplayName: "Content Completion",
			Aliases:     []string{"ai_summary"},
			Description: "Complete article content and generate article summaries",
			TaskName:    "ai_summary",
			Get: func() interface{} {
				return runtimeinfo.ContentCompletionSchedulerInterface
			},
		},
		{
			Name:        "firecrawl",
			DisplayName: "Firecrawl Crawler",
			Description: "Auto-crawl full content for articles",
			Get: func() interface{} {
				return runtimeinfo.FirecrawlSchedulerInterface
			},
		},
		{
			Name:        "tag_quality_score",
			DisplayName: "Tag Quality Score",
			Description: "Recompute persistent quality scores for topic tags",
			Get: func() interface{} {
				return runtimeinfo.TagQualityScoreSchedulerInterface
			},
		},
		{
			Name:        "narrative_summary",
			DisplayName: "Narrative Summary",
			Description: "Generate daily narrative summaries from active topic tags",
			Get: func() interface{} {
				return runtimeinfo.NarrativeSummarySchedulerInterface
			},
		},
		{
			Name:        "log_cleanup",
			DisplayName: "Log Cleanup",
			Description: "Clean up expired ai_call_logs and otel_spans rows",
			Get: func() interface{} {
				return runtimeinfo.LogCleanupSchedulerInterface
			},
		},
	}
}

func resolveScheduler(name string) (*schedulerDescriptor, interface{}) {
	for _, descriptor := range schedulerDescriptors() {
		if descriptor.Name == name {
			return &descriptor, descriptor.Get()
		}
		for _, alias := range descriptor.Aliases {
			if alias == name {
				return &descriptor, descriptor.Get()
			}
		}
	}
	return nil, nil
}

func safeGetStatus(scheduler interface{}, displayName string) *SchedulerStatusResponse {
	if scheduler == nil {
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			logging.Errorf("Panic in %s scheduler GetStatus: %v", displayName, r)
		}
	}()

	if status, ok := scheduler.(interface {
		GetStatus() SchedulerStatusResponse
	}); ok {
		result := status.GetStatus()
		result = normalizeSchedulerStatus(result, displayName)
		return &result
	}

	if legacy, ok := scheduler.(interface{ GetStatus() map[string]interface{} }); ok {
		result := schedulerStatusFromMap(legacy.GetStatus(), displayName)
		return &result
	}
	return nil
}

func GetSchedulersStatus(c *gin.Context) {
	schedulers := make([]SchedulerStatusResponse, 0)
	for _, descriptor := range schedulerDescriptors() {
		scheduler := descriptor.Get()
		if status := safeGetStatus(scheduler, descriptor.DisplayName); status != nil {
			enrichStatus(scheduler, descriptor, status)
			schedulers = append(schedulers, *status)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    schedulers,
	})
}

func GetSchedulerStatus(c *gin.Context) {
	name := c.Param("name")
	descriptor, scheduler := resolveScheduler(name)
	if descriptor == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Scheduler not found: " + name})
		return
	}

	if status := safeGetStatus(scheduler, descriptor.DisplayName); status != nil {
		enrichStatus(scheduler, *descriptor, status)
		c.JSON(http.StatusOK, gin.H{"success": true, "data": status})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Scheduler not found: " + name})
}

func TriggerScheduler(c *gin.Context) {
	requestedName := c.Param("name")
	descriptor, scheduler := resolveScheduler(requestedName)
	if descriptor == nil || scheduler == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Scheduler not found or cannot be triggered: " + requestedName})
		return
	}

	if triggerable, ok := scheduler.(interface {
		TriggerNowWithDate(dateStr string) map[string]interface{}
	}); ok {
		dateStr := c.Query("date")
		respondTriggerResult(c, descriptor.Name, triggerable.TriggerNowWithDate(dateStr))
		return
	}

	if triggerable, ok := scheduler.(interface{ TriggerNow() map[string]interface{} }); ok {
		respondTriggerResult(c, descriptor.Name, triggerable.TriggerNow())
		return
	}

	if triggerable, ok := scheduler.(interface{ Trigger() }); ok {
		logging.Infof("Triggering %s scheduler manually", descriptor.Name)
		triggerable.Trigger()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": descriptor.Description + " triggered",
			"data": gin.H{
				"name":   descriptor.Name,
				"status": "triggered",
			},
		})
		return
	}

	c.JSON(http.StatusConflict, gin.H{"success": false, "error": "Scheduler cannot be triggered: " + requestedName})
}

func respondTriggerResult(c *gin.Context, name string, result map[string]interface{}) {
	statusCode := http.StatusOK
	if rawCode, ok := result["status_code"].(int); ok {
		statusCode = rawCode
	}
	delete(result, "status_code")
	result["name"] = name

	accepted, _ := result["accepted"].(bool)
	message, _ := result["message"].(string)
	if accepted {
		c.JSON(statusCode, gin.H{"success": true, "message": message, "data": result})
		return
	}

	c.JSON(statusCode, gin.H{"success": false, "error": message, "data": result})
}

func ResetSchedulerStats(c *gin.Context) {
	requestedName := c.Param("name")
	descriptor, scheduler := resolveScheduler(requestedName)
	if descriptor == nil || scheduler == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Scheduler not found: " + requestedName})
		return
	}

	if resettable, ok := scheduler.(interface{ ResetStats() error }); ok {
		if err := resettable.ResetStats(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("Statistics reset for scheduler '%s'", descriptor.Name)})
		return
	}

	if descriptor.TaskName != "" {
		if err := resetSchedulerTask(descriptor.TaskName); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("Statistics reset for scheduler '%s'", descriptor.Name)})
		return
	}

	c.JSON(http.StatusConflict, gin.H{"success": false, "error": "Scheduler stats cannot be reset: " + requestedName})
}

func UpdateSchedulerInterval(c *gin.Context) {
	requestedName := c.Param("name")
	descriptor, scheduler := resolveScheduler(requestedName)
	if descriptor == nil || scheduler == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Scheduler not found: " + requestedName})
		return
	}

	var req UpdateSchedulerIntervalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Valid interval (positive integer) is required"})
		return
	}
	if req.Interval <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Interval must be a positive integer"})
		return
	}

	updatable, ok := scheduler.(interface{ UpdateInterval(int) error })
	if !ok {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": "Scheduler interval cannot be updated: " + requestedName})
		return
	}

	if err := updatable.UpdateInterval(req.Interval); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Interval updated for scheduler '%s'", descriptor.Name),
		"data": gin.H{
			"name":           descriptor.Name,
			"check_interval": req.Interval,
		},
	})
}

func GetTasksStatus(c *gin.Context) {
	tasks := make([]gin.H, 0)
	queueSize := 0
	activeTasks := 0

	if status := safeGetTaskStatus(runtimeinfo.ContentCompletionSchedulerInterface); status != nil {
		if overview, ok := status["overview"].(map[string]interface{}); ok {
			pendingCount := asInt(overview["pending_count"])
			processingCount := asInt(overview["processing_count"])
			if pendingCount > 0 || processingCount > 0 {
				queueSize += pendingCount
				activeTasks++
				tasks = append(tasks, gin.H{
					"type":             "content_completion",
					"status":           status["status"],
					"pending_count":    pendingCount,
					"processing_count": processingCount,
					"overview":         overview,
				})
			}
		}
	}

	if status := safeGetTaskStatus(runtimeinfo.FirecrawlSchedulerInterface); status != nil {
		queueCount := asInt(status["queue_size"])
		processingCount := asInt(status["processing"])
		if queueCount > 0 || processingCount > 0 {
			queueSize += queueCount
			activeTasks++
			tasks = append(tasks, gin.H{
				"type":             "firecrawl",
				"status":           status["status"],
				"queue_size":       queueCount,
				"processing_count": processingCount,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"queue_size":   queueSize,
			"active_tasks": activeTasks,
			"tasks":        tasks,
		},
	})
}

func safeGetTaskStatus(scheduler interface{}) map[string]interface{} {
	if scheduler == nil {
		return nil
	}

	if status, ok := scheduler.(interface{ GetTaskStatusDetails() map[string]interface{} }); ok {
		return status.GetTaskStatusDetails()
	}

	if legacy, ok := scheduler.(interface{ GetStatus() map[string]interface{} }); ok {
		return legacy.GetStatus()
	}

	return nil
}

func enrichStatus(scheduler interface{}, descriptor schedulerDescriptor, status *SchedulerStatusResponse) {
	status.Name = descriptor.Name
	status.Description = descriptor.Description

	if detailer, ok := scheduler.(interface{ GetTaskStatusDetails() map[string]interface{} }); ok {
		details := detailer.GetTaskStatusDetails()
		if details == nil {
			return
		}
		if v, ok := details["database_state"].(map[string]interface{}); ok {
			status.DatabaseState = v
		}
		if v, ok := details["overview"].(map[string]interface{}); ok {
			status.Overview = v
		}
		if v, ok := details["last_run_summary"]; ok && v != nil {
			status.LastRunSummary = v
		}
		if v, ok := details["current_article"]; ok && v != nil {
			status.CurrentArticle = v
		}
		if v, ok := details["last_processed"]; ok && v != nil {
			status.LastProcessed = v
		}
		if v, ok := details["live_processing_count"]; ok {
			if n, ok := v.(int); ok && n > 0 {
				status.LiveProcessingCount = n
			}
		}
		if v, ok := details["stale_processing_count"]; ok {
			if n, ok := v.(int); ok && n > 0 {
				status.StaleProcessingCount = n
			}
		}
		if v, ok := details["stale_processing_article"]; ok && v != nil {
			status.StaleProcessingArticle = v
		}
		if v, ok := details["ai_configured"]; ok {
			if b, ok := v.(bool); ok {
				status.AIConfigured = b
			}
		}
		return
	}

	taskName := descriptor.TaskName
	if taskName == "" {
		taskName = descriptor.Name
	}
	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", taskName).First(&task).Error; err == nil {
		status.DatabaseState = task.ToDict()
		if task.LastExecutionResult != "" {
			var summary interface{}
			if err := json.Unmarshal([]byte(task.LastExecutionResult), &summary); err == nil {
				status.LastRunSummary = summary
			}
		}
	}
}

func normalizeSchedulerStatus(status SchedulerStatusResponse, displayName string) SchedulerStatusResponse {
	if status.Name == "" {
		status.Name = displayName
	}
	return status
}

func schedulerStatusFromMap(status map[string]interface{}, displayName string) SchedulerStatusResponse {
	if status == nil {
		return SchedulerStatusResponse{Name: displayName}
	}

	response := SchedulerStatusResponse{
		Name:        displayName,
		Status:      asString(status["status"]),
		NextRun:     toUnixTimestamp(status["next_run"]),
		IsExecuting: asBool(status["is_executing"]),
	}
	if response.Status == "" {
		if asBool(status["running"]) {
			response.Status = "running"
		} else {
			response.Status = "idle"
		}
	}
	if name := asString(status["name"]); name != "" {
		response.Name = name
	}
	response.CheckInterval = asInt64(status["check_interval"])
	if !response.IsExecuting && response.Status == "running" {
		response.IsExecuting = true
	}
	return response
}

func resetSchedulerTask(taskName string) error {
	var task models.SchedulerTask
	if err := database.DB.Where("name = ?", taskName).First(&task).Error; err != nil {
		return err
	}

	return database.DB.Model(&task).Updates(map[string]interface{}{
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
	}).Error
}

func asInt(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func asInt64(value interface{}) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func asString(value interface{}) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func asBool(value interface{}) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	return false
}

func toUnixTimestamp(value interface{}) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case time.Time:
		if typed.IsZero() {
			return 0
		}
		return typed.Unix()
	case *time.Time:
		if typed == nil || typed.IsZero() {
			return 0
		}
		return typed.Unix()
	case string:
		if typed == "" {
			return 0
		}
		parsed, err := time.Parse(time.RFC3339, typed)
		if err != nil {
			return 0
		}
		return parsed.Unix()
	default:
		return 0
	}
}
