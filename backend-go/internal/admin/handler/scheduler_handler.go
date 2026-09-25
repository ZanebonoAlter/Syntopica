package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/admin/scheduler"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/aihealth"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/analysispause"
	"syntopica-backend/internal/platform/logging"
)

// SchedulerRegistry is the minimal interface for the global scheduler registry.
type SchedulerRegistry interface {
	Get(name string) (interface{}, bool)
	// OrderedNames returns scheduler keys in registration order, so /status
	// rendering is stable and auto-discovers every registered scheduler.
	OrderedNames() []string
}

// Reg is the global scheduler registry, set by app.StartRuntime via admin.SetRegistry.
var Reg SchedulerRegistry

// lookupScheduler is the nil-safe Reg.Get: in read-only demo mode StartRuntime
// never runs, Reg stays nil, and a raw call would panic on the nil receiver —
// surfacing as a 500 on every status poll (openspec: scheduler-observability /
// 无调度器的运行模式返回空集合而非错误). "No scheduler" is a legal state:
// these helpers make status handlers answer 200 + empty collections instead.
func lookupScheduler(name string) (interface{}, bool) {
	if Reg == nil {
		return nil, false
	}
	return Reg.Get(name)
}

// orderedSchedulerNames is the nil-safe Reg.OrderedNames (see lookupScheduler).
func orderedSchedulerNames() []string {
	if Reg == nil {
		return nil
	}
	return Reg.OrderedNames()
}

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
	ScheduleTime           string                 `json:"schedule_time,omitempty"`
}

// schedulerConfig returns the scheduler's Config, or the zero value if the
// scheduler does not expose one (e.g. a non-BaseScheduler implementation).
// The admin handler reads Description/TaskName/Aliases from here for
// auto-discovery, instead of a hardcoded descriptor list.
func schedulerConfig(s interface{}) scheduler.Config {
	if cfg, ok := s.(interface{ GetConfig() scheduler.Config }); ok {
		return cfg.GetConfig()
	}
	return scheduler.Config{}
}

// schedulerLabel returns a human-readable label (Config.Name) for log/error
// messages, falling back to the registry key.
func schedulerLabel(s interface{}, key string) string {
	if name := schedulerConfig(s).Name; name != "" {
		return name
	}
	return key
}

// ResolveScheduler finds a scheduler by registry key or alias. It returns the
// canonical registry key and the scheduler instance. Auto-discovered: any
// scheduler registered with the registry (plus its Config.Aliases) is
// resolvable here without a separate descriptor list.
func ResolveScheduler(name string) (string, interface{}) {
	if s, ok := lookupScheduler(name); ok {
		return name, s
	}
	for _, key := range orderedSchedulerNames() {
		s, ok := lookupScheduler(key)
		if !ok {
			continue
		}
		for _, alias := range schedulerConfig(s).Aliases {
			if alias == name {
				return key, s
			}
		}
	}
	return "", nil
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
	for _, key := range orderedSchedulerNames() {
		s, ok := lookupScheduler(key)
		if !ok {
			continue
		}
		if status := safeGetStatus(s, schedulerLabel(s, key)); status != nil {
			enrichStatus(s, key, status)
			schedulers = append(schedulers, *status)
		}
	}

	c.JSON(http.StatusOK, buildSchedulerStatusPayload(schedulers))
}

// buildSchedulerStatusPayload assembles the schedulers/status body (data plus
// the analysis_paused / ai_healthy top-level fields) — shared verbatim by
// GetSchedulersStatus and GetPollBundle so the merged /api/poll response keeps
// the exact shape the frontend already consumes.
func buildSchedulerStatusPayload(schedulers []SchedulerStatusResponse) gin.H {
	pausedAt := analysispause.PausedAt()
	pausedAtStr := ""
	if pausedAt != nil {
		pausedAtStr = pausedAt.Format(time.RFC3339)
	}

	// ai_healthy + ai_health_routes expose a lightweight model-health summary so
	// the frontend can render the not-ready banner without an extra request.
	// analysis_paused reflects USER intent only (UserPaused), so the pause
	// button never flips because of model health (spec: 健康未就绪时开关不变).
	healthSnap := aihealth.GetSnapshot()
	healthRoutes := make([]gin.H, 0, len(healthSnap.Routes))
	for _, r := range healthSnap.Routes {
		healthRoutes = append(healthRoutes, gin.H{
			"route_name":       r.RouteName,
			"capability":       r.Capability,
			"primary_provider": r.PrimaryProvider,
			"reachable":        r.Reachable,
		})
	}

	return gin.H{
		"success":            true,
		"data":               schedulers,
		"analysis_paused":    analysispause.UserPaused(),
		"analysis_paused_at": pausedAtStr,
		"ai_healthy":         aihealth.Healthy(),
		"ai_health_routes":   healthRoutes,
	}
}

func GetSchedulerStatus(c *gin.Context) {
	name := c.Param("name")
	key, scheduler := ResolveScheduler(name)
	if scheduler == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Scheduler not found: " + name})
		return
	}

	if status := safeGetStatus(scheduler, schedulerLabel(scheduler, key)); status != nil {
		enrichStatus(scheduler, key, status)
		c.JSON(http.StatusOK, gin.H{"success": true, "data": status})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Scheduler not found: " + name})
}

func TriggerScheduler(c *gin.Context) {
	requestedName := c.Param("name")
	key, scheduler := ResolveScheduler(requestedName)
	if scheduler == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Scheduler not found or cannot be triggered: " + requestedName})
		return
	}

	if triggerable, ok := scheduler.(interface {
		TriggerNowWithDate(dateStr string) map[string]interface{}
	}); ok {
		dateStr := c.Query("date")
		respondTriggerResult(c, key, triggerable.TriggerNowWithDate(dateStr))
		return
	}

	if triggerable, ok := scheduler.(interface{ TriggerNow() map[string]interface{} }); ok {
		respondTriggerResult(c, key, triggerable.TriggerNow())
		return
	}

	if triggerable, ok := scheduler.(interface{ Trigger() }); ok {
		logging.Infof("Triggering %s scheduler manually", key)
		triggerable.Trigger()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": schedulerConfig(scheduler).Description + " triggered",
			"data": gin.H{
				"name":   key,
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
	key, scheduler := ResolveScheduler(requestedName)
	if scheduler == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Scheduler not found: " + requestedName})
		return
	}

	if resettable, ok := scheduler.(interface{ ResetStats() error }); ok {
		if err := resettable.ResetStats(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("Statistics reset for scheduler '%s'", key)})
		return
	}

	// Fallback: reset the SchedulerTask DB row directly. taskName defaults to
	// the registry key when Config.TaskName is unset (e.g. "ai_summary" for
	// content_completion).
	taskName := schedulerConfig(scheduler).TaskName
	if taskName == "" {
		taskName = key
	}
	if err := resetSchedulerTask(taskName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("Statistics reset for scheduler '%s'", key)})
}

func UpdateSchedulerInterval(c *gin.Context) {
	requestedName := c.Param("name")
	key, scheduler := ResolveScheduler(requestedName)
	if scheduler == nil {
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
		"message": fmt.Sprintf("Interval updated for scheduler '%s'", key),
		"data": gin.H{
			"name":           key,
			"check_interval": req.Interval,
		},
	})
}

type UpdateSchedulerScheduleTimeRequest struct {
	Time string `json:"time" binding:"required"`
}

// UpdateSchedulerScheduleTime updates the wall-clock trigger time (HH:MM) for
// the two wall-clock schedulers (board_upgrade_suggest, daily_report). The time
// is persisted to ai_settings under the same key the scheduler reads at trigger
// computation, so the change takes effect on the next tick. Other schedulers
// (cron-based, interval-based) return 400.
func UpdateSchedulerScheduleTime(c *gin.Context) {
	requestedName := c.Param("name")
	key, scheduler := ResolveScheduler(requestedName)
	if scheduler == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Scheduler not found: " + requestedName})
		return
	}

	var req UpdateSchedulerScheduleTimeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Valid time (HH:MM) is required"})
		return
	}

	switch key {
	case "board_upgrade_suggest":
		if err := aisettings.SaveBoardUpgradeSuggestTimeConfig(req.Time); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
	case "daily_report":
		if err := aisettings.SaveDailyReportTimeConfig(req.Time); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Schedule time configuration is not supported for scheduler: " + requestedName})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Schedule time updated for scheduler '%s'", key),
		"data": gin.H{
			"name": key,
			"time": req.Time,
		},
	})
}

func GetTasksStatus(c *gin.Context) {
	tasks := make([]gin.H, 0)
	queueSize := 0
	activeTasks := 0

	if status := safeGetTaskStatus(func() interface{} { s, _ := lookupScheduler("content_completion"); return s }()); status != nil {
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

	if status := safeGetTaskStatus(func() interface{} { s, _ := lookupScheduler("firecrawl"); return s }()); status != nil {
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

// enrichStatus fills in display metadata (Name/Description) and, when the
// scheduler implements GetTaskStatusDetails, the runtime/database detail
// fields. It is fully generic — no per-scheduler branching. The DB-table
// fallback at the end handles schedulers that only expose a SchedulerTask row.
func enrichStatus(scheduler interface{}, key string, status *SchedulerStatusResponse) {
	status.Name = key
	cfg := schedulerConfig(scheduler)
	status.Description = cfg.Description
	taskName := key
	if cfg.TaskName != "" {
		taskName = cfg.TaskName
	}

	// Wall-clock schedulers surface their configured HH:MM trigger time so the
	// status panel can show/edit it. Only these two read HH:MM from ai_settings.
	switch key {
	case "board_upgrade_suggest":
		if t, err := aisettings.LoadBoardUpgradeSuggestTimeConfig(); err == nil {
			status.ScheduleTime = t
		}
	case "daily_report":
		if t, err := aisettings.LoadDailyReportTimeConfig(); err == nil {
			status.ScheduleTime = t
		}
	}

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

	var task models.SchedulerTask
	if err := repository.Repo.DB().Where("name = ?", taskName).First(&task).Error; err == nil {
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
	if err := repository.Repo.DB().Where("name = ?", taskName).First(&task).Error; err != nil {
		return err
	}

	return repository.Repo.DB().Model(&task).Updates(map[string]interface{}{
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
