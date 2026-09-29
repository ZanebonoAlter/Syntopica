package handler

import (
	"context"
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
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
)

// setupAnalysisPauseResumeTestDB builds an in-memory SQLite DB with the AI
// vendor/model/route tables so RunStartupProbe (triggered on resume) can read
// the seeded routes. Mirrors the aihealth package's test setup.
func setupAnalysisPauseResumeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.AISettings{}, &models.AIVendor{}, &models.AIModel{}, &models.AIRoute{},
		&models.AIRouteModel{}, &models.AICallLog{},
	))
	database.DB = db
	return db
}

// seedResumeVendorModel seeds one vendor plus one model under it (the
// vendor-chain replacement of the old per-provider seeding).
func seedResumeVendorModel(t *testing.T, db *gorm.DB, vendorName, modelName, kind string) models.AIModel {
	t.Helper()
	v := models.AIVendor{
		Name: vendorName, ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL: "http://127.0.0.1:8081/v1", Enabled: true,
	}
	require.NoError(t, db.Create(&v).Error)
	m := models.AIModel{
		VendorID: v.ID, Name: modelName, ModelKind: kind,
		Enabled: true, Status: airouter.ModelStatusOK,
	}
	require.NoError(t, db.Create(&m).Error)
	return m
}

func seedResumeRoute(t *testing.T, db *gorm.DB, capability string) models.AIRoute {
	t.Helper()
	r := models.AIRoute{Name: "default", Capability: capability, Enabled: true}
	require.NoError(t, db.Create(&r).Error)
	return r
}

func seedResumeBinding(t *testing.T, db *gorm.DB, routeID, modelID uint) {
	t.Helper()
	require.NoError(t, db.Create(&models.AIRouteModel{
		RouteID: routeID, ModelID: modelID, Priority: 1, Enabled: true,
	}).Error)
}

func newResumeGinContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

// TestSetAnalysisPause_Resume_TriggersHealthReprobe verifies that resuming
// (paused:false) asynchronously re-runs the startup probe so the health gate
// can self-heal (e.g. after a transient startup-probe failure). The probe is
// driven through the fake seam; the snapshot must flip to healthy=true.
// The fixture is the core pair (embedding + topic_tagging): those are the two
// capabilities the health verdict hard-requires.
func TestSetAnalysisPause_Resume_TriggersHealthReprobe(t *testing.T) {
	db := setupAnalysisPauseResumeTestDB(t)
	emb := seedResumeVendorModel(t, db, "emb-main", "emb-main-model", "embedding")
	tag := seedResumeVendorModel(t, db, "tag-main", "tag-main-model", "llm")
	er := seedResumeRoute(t, db, string(airouter.CapabilityEmbedding))
	tr := seedResumeRoute(t, db, string(airouter.CapabilityTopicTagging))
	seedResumeBinding(t, db, er.ID, emb.ID)
	seedResumeBinding(t, db, tr.ID, tag.ID)

	// Start from the not-ready (startup-race) state; restore on cleanup.
	aihealth.SetSnapshotForTest(aihealth.Snapshot{})
	t.Cleanup(func() { aihealth.SetSnapshotForTest(aihealth.Snapshot{}) })

	// Fake probe: every primary vendor reachable -> probe marks healthy.
	restoreProbe := aihealth.SetProbeFnForTest(func(context.Context, models.AIVendor) (bool, string) {
		return true, ""
	})
	t.Cleanup(restoreProbe)

	require.False(t, aihealth.Healthy(), "precondition: snapshot not ready -> not healthy")

	ctx, recorder := newResumeGinContext(t, `{"paused":false}`)
	SetAnalysisPause(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)

	// The probe runs in a goroutine; poll until the snapshot reflects it.
	require.Eventually(t, aihealth.Healthy, time.Second, 5*time.Millisecond,
		"resume must trigger a reprobe that flips the snapshot to healthy")
}

// TestSetAnalysisPause_Pause_DoesNotTriggerReprobe verifies the reprobe fires
// only on resume, not on pause. After paused:true the snapshot must stay
// not-ready and the probe seam must not be invoked.
func TestSetAnalysisPause_Pause_DoesNotTriggerReprobe(t *testing.T) {
	setupAnalysisPauseResumeTestDB(t)

	aihealth.SetSnapshotForTest(aihealth.Snapshot{}) // not ready
	t.Cleanup(func() { aihealth.SetSnapshotForTest(aihealth.Snapshot{}) })

	probeCalled := false
	restoreProbe := aihealth.SetProbeFnForTest(func(context.Context, models.AIVendor) (bool, string) {
		probeCalled = true
		return true, ""
	})
	t.Cleanup(restoreProbe)

	ctx, recorder := newResumeGinContext(t, `{"paused":true}`)
	SetAnalysisPause(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	// Brief window for any (incorrectly) spawned goroutine to run.
	time.Sleep(30 * time.Millisecond)
	require.False(t, probeCalled, "pause must NOT trigger a reprobe")
	require.Nil(t, aihealth.GetSnapshot().CheckedAt, "snapshot stays not-ready; no probe ran")
}
