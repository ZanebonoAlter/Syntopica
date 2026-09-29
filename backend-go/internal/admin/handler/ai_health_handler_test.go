package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/aihealth"
	"syntopica-backend/internal/platform/database"
)

// setupAIHealthTestDB mirrors setupAnalysisPauseTestDB: sqlite in-memory +
// database.DB swap. Only ai_settings is needed (auto_start_models lives there);
// the handler reaches the model layer only through the in-memory aihealth
// snapshot, never the DB.
func setupAIHealthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AISettings{}))
	database.DB = db
	return db
}

func newAIHealthGinContext(t *testing.T, method, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, "/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

func decodeAIHealthBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}

// useNotReadySnapshot resets aihealth to the startup-race (not-ready) state for
// the test and restores not-ready on cleanup so the process-global snapshot
// never leaks between tests.
func useNotReadySnapshot(t *testing.T) {
	t.Helper()
	aihealth.SetSnapshotForTest(aihealth.Snapshot{})
	t.Cleanup(func() { aihealth.SetSnapshotForTest(aihealth.Snapshot{}) })
}

// useReadySnapshot installs a ready snapshot with the given healthy flag and
// route entries.
func useReadySnapshot(t *testing.T, healthy bool, routes []aihealth.RouteHealth) {
	t.Helper()
	now := time.Now()
	aihealth.SetSnapshotForTest(aihealth.Snapshot{
		Healthy:   healthy,
		CheckedAt: &now,
		Routes:    routes,
	})
	t.Cleanup(func() { aihealth.SetSnapshotForTest(aihealth.Snapshot{}) })
}

// TestGetAIHealth_NotReady covers the startup-race view: before the first probe
// completes the snapshot is not ready, so healthy=false, checked_at=null and
// routes is empty.
func TestGetAIHealth_NotReady(t *testing.T) {
	setupAIHealthTestDB(t)
	useNotReadySnapshot(t)

	ctx, recorder := newAIHealthGinContext(t, http.MethodGet, "")
	GetAIHealth(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := decodeAIHealthBody(t, recorder)
	require.Equal(t, true, body["success"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "response should carry a data object")
	require.Equal(t, false, data["healthy"])
	require.Nil(t, data["checked_at"], "checked_at must be null while not ready")
	require.Equal(t, false, data["auto_start_models"])
	require.Empty(t, data["routes"])
}

// TestGetAIHealth_Ready covers a ready, healthy snapshot: checked_at is a
// non-empty RFC3339 string, healthy=true, and the per-route projection carries
// every contract field (spec: route_name/capability/primary_model/vendor/
// model_kind/reachable/launched_by_backend/last_checked/error).
func TestGetAIHealth_Ready(t *testing.T) {
	setupAIHealthTestDB(t)
	checked := time.Now()
	useReadySnapshot(t, true, []aihealth.RouteHealth{
		{
			RouteName: "default", Capability: "embedding", PrimaryModel: "emb-main",
			Vendor: "emb-local", ModelKind: "embedding", Reachable: true,
			LaunchedByBackend: false, LastCheckedAt: checked, Error: "",
		},
		{
			RouteName: "default", Capability: "summary", PrimaryModel: "llm-main",
			Vendor: "llm-cloud", ModelKind: "llm", Reachable: true,
			LaunchedByBackend: true, LastCheckedAt: checked, Error: "",
		},
	})

	ctx, recorder := newAIHealthGinContext(t, http.MethodGet, "")
	GetAIHealth(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := decodeAIHealthBody(t, recorder)
	data := body["data"].(map[string]any)
	require.Equal(t, true, data["healthy"])
	require.NotNil(t, data["checked_at"])
	checkedStr, _ := data["checked_at"].(string)
	require.NotEmpty(t, checkedStr)
	_, err := time.Parse(time.RFC3339, checkedStr)
	require.NoError(t, err, "checked_at must be RFC3339")

	routes, ok := data["routes"].([]any)
	require.True(t, ok)
	require.Len(t, routes, 2)
	first := routes[0].(map[string]any)
	require.Equal(t, "embedding", first["capability"])
	require.Equal(t, "emb-main", first["primary_model"])
	require.Equal(t, "emb-local", first["vendor"])
	require.Equal(t, "embedding", first["model_kind"])
	require.Equal(t, true, first["reachable"])
	require.Equal(t, false, first["launched_by_backend"])
	require.NotContains(t, first, "primary_provider", "legacy primary_provider field must be gone")
}

// TestSetAutoStartModels_RoundTrip covers PUT {enabled:true} then GET: the
// persisted value is read back by GetAIHealth.
func TestSetAutoStartModels_RoundTrip(t *testing.T) {
	setupAIHealthTestDB(t)
	useNotReadySnapshot(t)

	// Default off.
	ctx, recorder := newAIHealthGinContext(t, http.MethodGet, "")
	GetAIHealth(ctx)
	require.Equal(t, false, decodeAIHealthBody(t, recorder)["data"].(map[string]any)["auto_start_models"])

	// Turn on.
	ctx, recorder = newAIHealthGinContext(t, http.MethodPut, `{"enabled":true}`)
	SetAutoStartModels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	body := decodeAIHealthBody(t, recorder)
	require.Equal(t, true, body["success"])
	require.Equal(t, true, body["data"].(map[string]any)["enabled"])

	// Read back via GET /api/ai/health.
	ctx, recorder = newAIHealthGinContext(t, http.MethodGet, "")
	GetAIHealth(ctx)
	require.Equal(t, true, decodeAIHealthBody(t, recorder)["data"].(map[string]any)["auto_start_models"])

	// Turn off again.
	ctx, recorder = newAIHealthGinContext(t, http.MethodPut, `{"enabled":false}`)
	SetAutoStartModels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, false, decodeAIHealthBody(t, recorder)["data"].(map[string]any)["enabled"])
	ctx, recorder = newAIHealthGinContext(t, http.MethodGet, "")
	GetAIHealth(ctx)
	require.Equal(t, false, decodeAIHealthBody(t, recorder)["data"].(map[string]any)["auto_start_models"])
}

// TestSetAutoStartModels_BadRequest covers the bind failure path: a body
// without a valid {enabled} field returns 400 with success=false.
func TestSetAutoStartModels_BadRequest(t *testing.T) {
	setupAIHealthTestDB(t)
	useNotReadySnapshot(t)

	ctx, recorder := newAIHealthGinContext(t, http.MethodPut, "")
	SetAutoStartModels(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	body := decodeAIHealthBody(t, recorder)
	require.Equal(t, false, body["success"])
	require.NotEmpty(t, body["error"])
}
