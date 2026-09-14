package scheduler

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/aihealth"
	"syntopica-backend/internal/platform/analysispause"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
	tagging "syntopica-backend/internal/tagmanagement"
	taggingrepo "syntopica-backend/internal/tagmanagement/repository"
)

// useHealthySnapshot installs a ready+healthy aihealth snapshot so IsPaused()
// reflects only the user switch, and restores the not-ready state on cleanup so
// the process-global snapshot never leaks between tests. The health gate now
// folds into IsPaused(): a not-ready snapshot (Healthy()==false) would make
// every "not paused" case look paused.
func useHealthySnapshot(t *testing.T) {
	t.Helper()
	now := time.Now()
	aihealth.SetSnapshotForTest(aihealth.Snapshot{Healthy: true, CheckedAt: &now})
	t.Cleanup(func() { aihealth.SetSnapshotForTest(aihealth.Snapshot{}) })
}

// TestPauseAware_SkipsWhenPaused verifies the D1/D3 gate behavior: while the
// global analysis pause is on, the wrapped job is NOT invoked and the wrapper
// returns a benign success result ("skipped: <reason>", err=nil) instead of an
// error — so it never pollutes the scheduler's failed-runs counter. The summary
// carries the PauseReason (here "user_paused") for observability.
func TestPauseAware_SkipsWhenPaused(t *testing.T) {
	testutil.SetupTestDB(t)
	require.NoError(t, analysispause.SetPaused(true))

	var called int32
	job := func(ctx context.Context) (*JobResult, error) {
		atomic.AddInt32(&called, 1)
		return &JobResult{Summary: "real job ran", Data: map[string]interface{}{"ran": true}}, nil
	}

	wrapped := PauseAware(job)
	result, err := wrapped(context.Background())

	require.NoError(t, err, "skipped result must be a success (err=nil)")
	require.EqualValues(t, 0, atomic.LoadInt32(&called), "real job must NOT run while paused")
	require.NotNil(t, result)
	require.Contains(t, result.Summary, "skipped")
	require.Contains(t, result.Summary, "user_paused", "summary should carry the pause reason")
	require.Equal(t, "paused", result.Data["skipped"])
}

// TestPauseAware_SkipsWhenModelUnhealthy verifies the health-gate path: with
// the user switch released but models NOT healthy, the job is still skipped and
// the reason reflects the health dimension (model_unhealthy), not user_paused.
func TestPauseAware_SkipsWhenModelUnhealthy(t *testing.T) {
	testutil.SetupTestDB(t)
	require.NoError(t, analysispause.SetPaused(false))
	// Ready snapshot but unhealthy.
	now := time.Now()
	aihealth.SetSnapshotForTest(aihealth.Snapshot{Healthy: false, CheckedAt: &now})
	t.Cleanup(func() { aihealth.SetSnapshotForTest(aihealth.Snapshot{}) })

	var called int32
	job := func(ctx context.Context) (*JobResult, error) {
		atomic.AddInt32(&called, 1)
		return &JobResult{Summary: "real job ran"}, nil
	}

	result, err := PauseAware(job)(context.Background())

	require.NoError(t, err)
	require.EqualValues(t, 0, atomic.LoadInt32(&called), "real job must NOT run when models are unhealthy")
	require.NotNil(t, result)
	require.True(t, strings.Contains(result.Summary, "model_unhealthy"), "summary should flag the health reason")
}

// TestAuxLabelCleanupEdgeGCRunsWhileAnalysisPaused pins the maintenance-class
// contract for aux_label_cleanup (offline-catchup step 6 / design D9 of
// pause-analysis): tag edge GC is data hygiene, not analysis, so it MUST NOT sit
// behind the PauseAware gate — otherwise a long AI outage would let the edge
// table grow without bound exactly when nobody is watching.
//
// The assertion is structural + behavioral: runtime registers AuxLabelCleanupJob
// bare (no PauseAware wrapper), so the raw job still reclaims edges while paused,
// whereas the wrapped form used by analysis-class jobs is skipped.
func TestAuxLabelCleanupEdgeGCRunsWhileAnalysisPaused(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:pause-auxgc-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)

	prevAdminRepo := repository.Repo
	prevTaggingRepo := taggingrepo.Repo
	prevDatabase := database.DB
	repository.InitRepository(db)
	tagging.InitRepository(db)
	// analysispause.SetPaused persists through the platform database global.
	database.DB = db
	t.Cleanup(func() {
		repository.Repo = prevAdminRepo
		taggingrepo.Repo = prevTaggingRepo
		database.DB = prevDatabase
	})

	require.NoError(t, db.AutoMigrate(
		&models.Feed{},
		&models.Article{},
		&models.TopicTag{},
		&models.ArticleTopicTag{},
		&models.SemanticLabel{},
		&models.TopicTagSemanticLabel{},
		&models.BoardComposition{},
		&models.AISettings{},
	))

	// An edge past the default 7-day window on an ARCHIVED article: the GC
	// step must delete it (M5-B: only archived articles' edges are reclaimed).
	pubDate := time.Now().AddDate(0, 0, -8)
	feed := models.Feed{Title: "pause-auxgc", URL: "https://example.com/pause-auxgc"}
	require.NoError(t, db.Create(&feed).Error)
	article := models.Article{FeedID: feed.ID, Title: "expired", Link: "https://example.com/pause-auxgc/a", PubDate: &pubDate, Archived: true}
	require.NoError(t, db.Create(&article).Error)
	tag := models.TopicTag{Slug: "pause-auxgc", Label: "pause-auxgc", Category: models.TagCategoryEvent, Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{
		ArticleID:  article.ID,
		TopicTagID: tag.ID,
		Source:     "llm",
		CreatedAt:  time.Now().AddDate(0, 0, -8),
	}).Error)

	require.NoError(t, analysispause.SetPaused(true))
	t.Cleanup(func() { _ = analysispause.SetPaused(false) })

	// Analysis-class composition (what runtime does NOT do for this job): skipped.
	skipped, err := PauseAware(AuxLabelCleanupJob)(context.Background())
	require.NoError(t, err)
	require.Equal(t, "paused", skipped.Data["skipped"], "PauseAware must gate analysis-class jobs")

	// Maintenance-class composition (how runtime actually registers it): runs.
	result, err := AuxLabelCleanupJob(context.Background())
	require.NoError(t, err, "edge GC must keep running while analysis is paused")
	require.NotNil(t, result)
	require.NotContains(t, result.Data, "edge_gc_error")
	require.EqualValues(t, 1, result.Data["edge_deleted_count"], "expired edge reclaimed during pause")
	require.Contains(t, result.Summary, "reclaimed 1 tag edges")

	var remaining int64
	require.NoError(t, db.Model(&models.ArticleTopicTag{}).Where("topic_tag_id = ?", tag.ID).Count(&remaining).Error)
	require.EqualValues(t, 0, remaining)
}

// TestPauseAware_RunsWhenNotPaused verifies the pass-through path: with the
// user switch released AND models healthy, the wrapper invokes the original job
// and returns its result unchanged.
func TestPauseAware_RunsWhenNotPaused(t *testing.T) {
	testutil.SetupTestDB(t)
	require.NoError(t, analysispause.SetPaused(false))
	useHealthySnapshot(t)

	var called int32
	job := func(ctx context.Context) (*JobResult, error) {
		atomic.AddInt32(&called, 1)
		return &JobResult{Summary: "real job ran", Data: map[string]interface{}{"ran": true}}, nil
	}

	wrapped := PauseAware(job)
	result, err := wrapped(context.Background())

	require.NoError(t, err)
	require.EqualValues(t, 1, atomic.LoadInt32(&called), "real job must run when not paused")
	require.NotNil(t, result)
	require.Equal(t, "real job ran", result.Summary)
	require.Equal(t, true, result.Data["ran"])
}
