package jobs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/app/runtimeinfo"
)

type stubManagedScheduler struct {
	status             SchedulerStatusResponse
	taskStatus         map[string]interface{}
	triggerResult      map[string]interface{}
	updatedInterval    int
	resetCalled        bool
	triggerCalledCount int
}

func (s *stubManagedScheduler) GetStatus() SchedulerStatusResponse {
	return s.status
}

func (s *stubManagedScheduler) GetTaskStatusDetails() map[string]interface{} {
	return s.taskStatus
}

func (s *stubManagedScheduler) TriggerNow() map[string]interface{} {
	s.triggerCalledCount++
	if s.triggerResult != nil {
		return s.triggerResult
	}
	return map[string]interface{}{
		"accepted": true,
		"started":  true,
		"message":  "ok",
	}
}

func (s *stubManagedScheduler) ResetStats() error {
	s.resetCalled = true
	return nil
}

func (s *stubManagedScheduler) UpdateInterval(interval int) error {
	s.updatedInterval = interval
	return nil
}

func resetSchedulerInterfaces() {
	runtimeinfo.AutoRefreshSchedulerInterface = nil
	runtimeinfo.PreferenceUpdateSchedulerInterface = nil
	runtimeinfo.ContentCompletionSchedulerInterface = nil
	runtimeinfo.FirecrawlSchedulerInterface = nil
}

func TestGetSchedulersStatusIncludesAllRegisteredSchedulers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetSchedulerInterfaces()
	defer resetSchedulerInterfaces()

	autoRefresh := &stubManagedScheduler{status: SchedulerStatusResponse{Name: "Auto Refresh", Status: "idle"}}
	preference := &stubManagedScheduler{status: SchedulerStatusResponse{Name: "Preference Update", Status: "idle"}}
	completion := &stubManagedScheduler{status: SchedulerStatusResponse{Name: "Content Completion", Status: "idle"}}
	firecrawl := &stubManagedScheduler{status: SchedulerStatusResponse{Name: "Firecrawl Crawler", Status: "idle"}}

	runtimeinfo.AutoRefreshSchedulerInterface = autoRefresh
	runtimeinfo.PreferenceUpdateSchedulerInterface = preference
	runtimeinfo.ContentCompletionSchedulerInterface = completion
	runtimeinfo.FirecrawlSchedulerInterface = firecrawl

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	GetSchedulersStatus(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	data := body["data"].([]any)

	names := map[string]bool{}
	for _, item := range data {
		entry := item.(map[string]any)
		names[entry["name"].(string)] = true
	}

	require.True(t, names["auto_refresh"])
	require.True(t, names["preference_update"])
	require.True(t, names["content_completion"])
	require.True(t, names["firecrawl"])
}

func TestTriggerSchedulerSupportsContentCompletionAliasAndLegacyName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetSchedulerInterfaces()
	defer resetSchedulerInterfaces()

	completion := &stubManagedScheduler{triggerResult: map[string]interface{}{"accepted": true, "started": true, "message": "triggered"}}
	runtimeinfo.ContentCompletionSchedulerInterface = completion

	for _, name := range []string{"content_completion", "ai_summary"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "name", Value: name}}

		TriggerScheduler(ctx)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	}

	require.Equal(t, 2, completion.triggerCalledCount)
}

func TestResetSchedulerStatsCallsSchedulerImplementation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetSchedulerInterfaces()
	defer resetSchedulerInterfaces()

	autoRefresh := &stubManagedScheduler{}
	runtimeinfo.AutoRefreshSchedulerInterface = autoRefresh

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "name", Value: "auto_refresh"}}

	ResetSchedulerStats(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.True(t, autoRefresh.resetCalled)
}

func TestUpdateSchedulerIntervalCallsSchedulerImplementation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetSchedulerInterfaces()
	defer resetSchedulerInterfaces()

	completion := &stubManagedScheduler{}
	runtimeinfo.ContentCompletionSchedulerInterface = completion

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "name", Value: "content_completion"}}
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/schedulers/content_completion/interval", strings.NewReader(`{"interval":120}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateSchedulerInterval(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 120, completion.updatedInterval)
}

func TestGetTasksStatusAggregatesRuntimeWork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetSchedulerInterfaces()
	defer resetSchedulerInterfaces()

	completion := &stubManagedScheduler{taskStatus: map[string]interface{}{
		"overview": map[string]interface{}{"pending_count": 3, "processing_count": 1},
	}}
	firecrawl := &stubManagedScheduler{taskStatus: map[string]interface{}{"status": "running", "queue_size": 2, "processing": 1}}
	runtimeinfo.ContentCompletionSchedulerInterface = completion
	runtimeinfo.FirecrawlSchedulerInterface = firecrawl

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	GetTasksStatus(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	data := body["data"].(map[string]any)
	require.EqualValues(t, 2, data["active_tasks"])
	require.EqualValues(t, 5, data["queue_size"])

	tasks := data["tasks"].([]any)
	require.Len(t, tasks, 2)
}

func TestGetSchedulerStatusReturnsUnifiedResponseShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetSchedulerInterfaces()
	defer resetSchedulerInterfaces()

	runtimeinfo.AutoRefreshSchedulerInterface = &stubManagedScheduler{status: SchedulerStatusResponse{
		Name:          "Auto Refresh",
		Status:        "idle",
		CheckInterval: 60,
		NextRun:       1710000000,
		IsExecuting:   false,
	}}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "name", Value: "auto_refresh"}}

	GetSchedulerStatus(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	data := body["data"].(map[string]any)

	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	require.Contains(t, keys, "check_interval")
	require.Contains(t, keys, "is_executing")
	require.Contains(t, keys, "name")
	require.Contains(t, keys, "next_run")
	require.Contains(t, keys, "status")
	require.Equal(t, "auto_refresh", data["name"])
	require.EqualValues(t, 60, data["check_interval"])
	require.EqualValues(t, 1710000000, data["next_run"])
	require.Equal(t, false, data["is_executing"])
}

func TestGetSchedulerStatusAliasUsesSameUnifiedShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetSchedulerInterfaces()
	defer resetSchedulerInterfaces()

	runtimeinfo.ContentCompletionSchedulerInterface = &stubManagedScheduler{status: SchedulerStatusResponse{
		Name:          "Content Completion",
		Status:        "running",
		CheckInterval: 3600,
		NextRun:       1710003600,
		IsExecuting:   true,
	}}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "name", Value: "ai_summary"}}

	GetSchedulerStatus(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	data := body["data"].(map[string]any)
	require.NotContains(t, data, "requested_name")
	require.NotContains(t, data, "alias_of")
	require.Equal(t, "content_completion", data["name"])
	require.Equal(t, "running", data["status"])
}
