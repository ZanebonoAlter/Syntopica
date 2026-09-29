package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"syntopica-backend/internal/platform/aihealth"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/database"
)

// aiHealthRouteDTO is the per-route projection of aihealth.RouteHealth exposed
// by GET /api/ai/health. Field names are snake_case to match the public API
// contract (spec: 健康快照与查询 API — route_name/capability/primary_model/
// vendor/model_kind/reachable/launched_by_backend/last_checked/error).
type aiHealthRouteDTO struct {
	RouteName         string `json:"route_name"`
	Capability        string `json:"capability"`
	PrimaryModel      string `json:"primary_model"`
	Vendor            string `json:"vendor"`
	ModelKind         string `json:"model_kind"`
	Reachable         bool   `json:"reachable"`
	LaunchedByBackend bool   `json:"launched_by_backend"`
	LastChecked       string `json:"last_checked"`
	Error             string `json:"error"`
}

// GetAIHealth returns the in-memory AI model health snapshot plus the current
// auto_start_models master-switch value. checked_at is null while the snapshot
// is not ready (first probe not finished yet), during which healthy is false.
func GetAIHealth(c *gin.Context) {
	snap := aihealth.GetSnapshot()

	autoStart, err := aisettings.LoadAutoStartModelsConfig()
	if err != nil {
		// Fail-open: a read error must not turn the health view into a 500; the
		// switch defaults to false (do not auto-launch).
		autoStart = false
	}

	routes := make([]aiHealthRouteDTO, 0, len(snap.Routes))
	for _, r := range snap.Routes {
		lastChecked := ""
		if !r.LastCheckedAt.IsZero() {
			lastChecked = r.LastCheckedAt.Format(time.RFC3339)
		}
		routes = append(routes, aiHealthRouteDTO{
			RouteName:         r.RouteName,
			Capability:        r.Capability,
			PrimaryModel:      r.PrimaryModel,
			Vendor:            r.Vendor,
			ModelKind:         r.ModelKind,
			Reachable:         r.Reachable,
			LaunchedByBackend: r.LaunchedByBackend,
			LastChecked:       lastChecked,
			Error:             r.Error,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"healthy":           snap.Healthy,
			"checked_at":        checkedAtOrNull(snap.CheckedAt),
			"auto_start_models": autoStart,
			"routes":            routes,
		},
	})
}

// checkedAtOrNull returns nil (→ JSON null) when the snapshot is not ready, so
// the API signals the startup-race state explicitly rather than emitting "".
func checkedAtOrNull(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

// SetAutoStartModelsRequest is the request body for SetAutoStartModels.
type SetAutoStartModelsRequest struct {
	Enabled bool `json:"enabled"`
}

// ReprobeAIHealth triggers one asynchronous health reprobe (POST
// /api/ai/health/reprobe) and returns immediately. triggered=true means a
// probe pass actually started; skipped=true means one was already in flight
// (idempotent, never queues or runs concurrently). The probe runs on a
// background context so it is not cancelled when the HTTP request ends.
func ReprobeAIHealth(c *gin.Context) {
	autoStart, _ := aisettings.LoadAutoStartModelsConfig()
	triggered := aihealth.TryStartProbe(context.Background(), airouter.NewStore(database.DB), autoStart)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"triggered": triggered,
			"skipped":   !triggered,
		},
	})
}

// SetAutoStartModels updates the auto_start_models master switch and returns
// the persisted value.
func SetAutoStartModels(c *gin.Context) {
	var req SetAutoStartModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	if err := aisettings.SaveAutoStartModelsConfig(req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled": req.Enabled,
		},
	})
}
