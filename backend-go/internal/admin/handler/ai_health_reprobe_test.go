package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/aihealth"
	"syntopica-backend/internal/platform/airouter"
)

// setupReprobeStore builds a store with one enabled route bound to one
// vendor+model, so a probe pass actually reaches probeFn (needed to hold
// probes in flight).
func setupReprobeStore(t *testing.T) *airouter.Store {
	t.Helper()
	db := setupAIHealthTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&models.AIVendor{}, &models.AIModel{}, &models.AIRoute{}, &models.AIRouteModel{},
	))
	v := models.AIVendor{
		Name:         "probe-target",
		ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL:      "http://127.0.0.1:8081/v1",
		Enabled:      true,
	}
	require.NoError(t, db.Create(&v).Error)
	m := models.AIModel{
		VendorID: v.ID, Name: "probe-model", ModelKind: "embedding",
		Enabled: true, Status: airouter.ModelStatusOK,
	}
	require.NoError(t, db.Create(&m).Error)
	r := models.AIRoute{Name: "default", Capability: string(airouter.CapabilityEmbedding), Enabled: true}
	require.NoError(t, db.Create(&r).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{
		RouteID: r.ID, ModelID: m.ID, Priority: 1, Enabled: true,
	}).Error)
	return airouter.NewStore(db)
}

// --- Scenario: 手动触发重探成功 ---

func TestReprobeAIHealth_TriggersProbe(t *testing.T) {
	setupReprobeStore(t)
	useNotReadySnapshot(t)

	probed := make(chan struct{}, 1)
	restore := aihealth.SetProbeFnForTest(func(ctx context.Context, v models.AIVendor) (bool, string) {
		select {
		case probed <- struct{}{}:
		default:
		}
		return true, ""
	})
	defer restore()

	ctx, recorder := newAIHealthGinContext(t, http.MethodPost, "")
	ReprobeAIHealth(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := decodeAIHealthBody(t, recorder)
	require.Equal(t, true, body["success"])
	data := body["data"].(map[string]any)
	require.Equal(t, true, data["triggered"], "no probe in flight -> must trigger")
	require.Equal(t, false, data["skipped"])

	// The async probe actually runs (background ctx keeps it alive).
	select {
	case <-probed:
	case <-time.After(2 * time.Second):
		t.Fatal("async probe was not started")
	}
}

// --- Scenario: 探测 in-flight 时手动触发被跳过 ---

func TestReprobeAIHealth_ProbeInFlight_Skipped(t *testing.T) {
	store := setupReprobeStore(t)
	useNotReadySnapshot(t)

	// Hold a probe in flight: the first probeFn call blocks until release.
	release := make(chan struct{})
	restore := aihealth.SetProbeFnForTest(func(ctx context.Context, v models.AIVendor) (bool, string) {
		<-release
		return true, ""
	})
	defer restore()
	defer close(release)

	require.True(t, aihealth.TryStartProbe(context.Background(), store, false),
		"priming an in-flight probe must succeed")

	ctx, recorder := newAIHealthGinContext(t, http.MethodPost, "")
	ReprobeAIHealth(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := decodeAIHealthBody(t, recorder)
	data := body["data"].(map[string]any)
	require.Equal(t, false, data["triggered"], "probe in flight -> must be skipped")
	require.Equal(t, true, data["skipped"])
}
