package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
	tagmanrepo "syntopica-backend/internal/tagmanagement/repository"
	tagqueue "syntopica-backend/internal/tagmanagement/handler"
)

// setupPollTestDB wires an isolated Postgres (testcontainers) schema into
// every store the poll bundle reads: scheduler enrichment (admin repository),
// tag-queue counters (tagmanagement repository singleton) and notifications.
func setupPollTestDB(t *testing.T) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	database.DB = db
	repository.InitRepository(db)
	tagmanrepo.InitRepository(db)
}

func newPollRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	api.GET("/poll", GetPollBundle)
	api.GET("/schedulers/status", GetSchedulersStatus)
	api.GET("/tag-queue/status", tagqueue.GetTagQueueStatus)
	api.GET("/notifications/unread-count", GetUnreadCount)
	return r
}

// TestPollBundle pins the client-poll-budget contract: one request carries
// all three resident status payloads (schedulers incl. top-level
// analysis_paused/ai_healthy, tag-queue counters, unread count).
func TestPollBundle(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres (testcontainers)")
	}
	setupPollTestDB(t)
	t.Cleanup(func() { Reg = nil })
	Reg = &fakeRegistry{
		items: map[string]interface{}{"daily_report": fakeScheduler{}},
		order: []string{"daily_report"},
	}

	code, body := doJSON(t, newPollRouter(), "/api/poll")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, true, body["success"])

	// data carries the three sections — existing structure for schedulers.
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "data should be an object, got %T", body["data"])
	schedulers, ok := data["schedulers"].([]any)
	require.True(t, ok, "data.schedulers should be an array, got %T", data["schedulers"])
	require.Len(t, schedulers, 1)

	// tag_queue counters — key set mirrors GET /api/tag-queue/status.
	tagQueue, ok := data["tag_queue"].(map[string]any)
	require.True(t, ok, "tag_queue missing from bundle: %v", body)
	for _, key := range []string{"pending", "processing", "completed", "failed", "total"} {
		require.Contains(t, tagQueue, key)
	}

	// notifications.unread.
	notifications, ok := data["notifications"].(map[string]any)
	require.True(t, ok, "notifications missing from bundle: %v", body)
	require.Contains(t, notifications, "unread")

	// Top-level semantics preserved for zero-consumer-change distribution.
	require.Contains(t, body, "analysis_paused")
	require.Contains(t, body, "ai_healthy")
	require.Contains(t, body, "server_time")

	// Legacy endpoints stay registered and answer 200 so already-open tabs
	// never see a 404 (same test as the bundle: the tag-queue status reader
	// singleton binds once per process, so both legs must share one setup).
	r := newPollRouter()
	for _, path := range []string{
		"/api/schedulers/status",
		"/api/tag-queue/status",
		"/api/notifications/unread-count",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "legacy endpoint %s must stay 200", path)
	}
}
