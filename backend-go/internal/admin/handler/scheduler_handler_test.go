package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// setupSchedulerStatusTestDB mirrors setupAIHealthTestDB: sqlite in-memory +
// database.DB swap. Only ai_settings is needed (analysispause reads the
// persisted switch through aisettings; empty-registry handlers never touch
// the scheduler task tables).
func setupSchedulerStatusTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AISettings{}))
	database.DB = db
	return db
}

func newSchedulerStatusRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	api.GET("/schedulers/status", GetSchedulersStatus)
	r.GET("/api/tasks/status", GetTasksStatus)
	return r
}

func doJSON(t *testing.T, r http.Handler, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET %s: body not valid JSON (%v): %q", path, err, w.Body.String())
	}
	return w.Code, body
}

// TestStatusEndpointsEmptyRegistry pins the scheduler-observability contract:
// in modes where no scheduler ever registered (read-only demo — StartRuntime
// skipped, Reg left nil) the status endpoints MUST answer 200 with parseable
// empty collections, never a 5xx (spec: 无调度器的运行模式返回空集合而非错误).
func TestStatusEndpointsEmptyRegistry(t *testing.T) {
	setupSchedulerStatusTestDB(t)
	t.Cleanup(func() { Reg = nil })
	Reg = nil // demo/only mode: SetRegistry never ran

	r := newSchedulerStatusRouter()

	code, body := doJSON(t, r, "/api/schedulers/status")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, true, body["success"])
	data, ok := body["data"].([]any)
	require.True(t, ok, "data should be a JSON array, got %T", body["data"])
	require.Empty(t, data)
	// Top-level semantics stay present so the frontend renders its normal
	// idle state rather than an error banner.
	require.Contains(t, body, "analysis_paused")
	require.Contains(t, body, "ai_healthy")

	code, body = doJSON(t, r, "/api/tasks/status")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, true, body["success"])
	tasks, ok := body["data"].(map[string]any)
	require.True(t, ok, "data should be an object, got %T", body["data"])
	require.InDelta(t, 0, tasks["queue_size"], 0)
	require.InDelta(t, 0, tasks["active_tasks"], 0)
}

// fakeScheduler + fakeRegistry back the "normal mode" side of the contract:
// status handlers keep their real structure when schedulers ARE registered.
type fakeScheduler struct{}

func (fakeScheduler) GetStatus() SchedulerStatusResponse {
	return SchedulerStatusResponse{Status: "idle"}
}

func (fakeScheduler) GetTaskStatusDetails() map[string]interface{} {
	// Short-circuits enrichStatus before its DB-table fallback.
	return map[string]interface{}{"status": "idle"}
}

type fakeRegistry struct {
	items map[string]interface{}
	order []string
}

func (f *fakeRegistry) Get(name string) (interface{}, bool) {
	s, ok := f.items[name]
	return s, ok
}

func (f *fakeRegistry) OrderedNames() []string { return f.order }

// TestStatusEndpointsNormalModeKeepsSemantics: 读模式不影响生产模式语义 —
// with a registered scheduler the same endpoints still return the full
// existing structure (non-empty list + top-level fields).
func TestStatusEndpointsNormalModeKeepsSemantics(t *testing.T) {
	setupSchedulerStatusTestDB(t)
	t.Cleanup(func() { Reg = nil })
	Reg = &fakeRegistry{
		items: map[string]interface{}{"daily_report": fakeScheduler{}},
		order: []string{"daily_report"},
	}

	r := newSchedulerStatusRouter()

	code, body := doJSON(t, r, "/api/schedulers/status")
	require.Equal(t, http.StatusOK, code)
	data, ok := body["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 1)
	entry := data[0].(map[string]any)
	require.Equal(t, "daily_report", entry["name"])
	require.Contains(t, body, "analysis_paused")
	require.Contains(t, body, "ai_healthy")

	code, body = doJSON(t, r, "/api/tasks/status")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, true, body["success"])
}
