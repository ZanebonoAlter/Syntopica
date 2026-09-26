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
	"syntopica-backend/internal/platform/scheduler"
	"syntopica-backend/internal/platform/testutil"
	content "syntopica-backend/internal/reader"
	tagging "syntopica-backend/internal/tagmanagement"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
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
	job := func(ctx context.Context) (*scheduler.JobResult, error) {
		atomic.AddInt32(&called, 1)
		return &scheduler.JobResult{Summary: "real job ran", Data: map[string]interface{}{"ran": true}}, nil
	}

	wrapped := scheduler.PauseAware(job)
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
	job := func(ctx context.Context) (*scheduler.JobResult, error) {
		atomic.AddInt32(&called, 1)
		return &scheduler.JobResult{Summary: "real job ran"}, nil
	}

	result, err := scheduler.PauseAware(job)(context.Background())

	require.NoError(t, err)
	require.EqualValues(t, 0, atomic.LoadInt32(&called), "real job must NOT run when models are unhealthy")
	require.NotNil(t, result)
	require.True(t, strings.Contains(result.Summary, "model_unhealthy"), "summary should flag the health reason")
}

// TestAuxLabelCleanupEdgeGCRunsWhileAnalysisPaused pins the maintenance-class
// contract for aux_label_cleanup (offline-catchup step 6 / design D9 of
// pause-analysis): tag edge GC is data hygiene, not analysis, so it MUST NOT sit
// behind the scheduler.PauseAware gate — otherwise a long AI outage would let the edge
// table grow without bound exactly when nobody is watching.
//
// The assertion is structural + behavioral: runtime registers AuxLabelCleanupJob
// bare (no scheduler.PauseAware wrapper), so the raw job still reclaims edges while paused,
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
		&tagmodels.TopicTagSemanticLabel{},
		&tagmodels.BoardComposition{},
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
	skipped, err := scheduler.PauseAware(AuxLabelCleanupJob)(context.Background())
	require.NoError(t, err)
	require.Equal(t, "paused", skipped.Data["skipped"], "scheduler.PauseAware must gate analysis-class jobs")

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
	job := func(ctx context.Context) (*scheduler.JobResult, error) {
		atomic.AddInt32(&called, 1)
		return &scheduler.JobResult{Summary: "real job ran", Data: map[string]interface{}{"ran": true}}, nil
	}

	wrapped := scheduler.PauseAware(job)
	result, err := wrapped(context.Background())

	require.NoError(t, err)
	require.EqualValues(t, 1, atomic.LoadInt32(&called), "real job must run when not paused")
	require.NotNil(t, result)
	require.Equal(t, "real job ran", result.Summary)
	require.Equal(t, true, result.Data["ran"])
}

// ── night-window-alignment：firecrawl 摘门 + 恢复续跑顺序 ──
//
// runtime.go 已把 firecrawl 注册处的暂停包裹摘除（与 auto_refresh 同列）。断言
// 结构 + 行为双口径（对齐 TestAuxLabelCleanupEdgeGCRunsWhileAnalysisPaused 的
// 既有模式）：scheduler.PauseAware 包裹形态在暂停/健康门下仍 skip（content_completion
// 等分析类的对照），而裸 job 形态（runtime 实际注册方式）照常抓取。

// TestFirecrawlCrawlRunsWhileAnalysisPaused（A1）：用户暂停路径下 firecrawl tick
// 照常执行抓取，scheduler.JobResult 不带 "analysis paused"。
func TestFirecrawlCrawlRunsWhileAnalysisPaused(t *testing.T) {
	db := setupFirecrawlJobTest(t)
	queue := content.NewFirecrawlJobQueue(db)
	_, links := seedFirecrawlArticles(t, db, queue, 2, false)

	require.NoError(t, analysispause.SetPaused(true))
	t.Cleanup(func() { _ = analysispause.SetPaused(false) })

	crawler := newFakeCrawler(time.Millisecond, nil)
	res, err := firecrawlJobWithCrawler(queue, "paused-crawl", func(*content.FirecrawlConfig) content.Crawler {
		return crawler
	})(context.Background())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotContains(t, res.Summary, "paused", "firecrawl must not be gated by the pause")
	require.NotContains(t, res.Summary, "skipped")
	require.Equal(t, 2, res.Data["completed"].(int), "crawl must actually run while paused")
	for _, link := range links {
		require.Equal(t, 1, crawler.callCount(link), "%s must be crawled while paused", link)
	}

	// 对照：分析类注册形态（scheduler.PauseAware 包裹）暂停态仍 skip。
	var called int32
	skipped, err := scheduler.PauseAware(func(ctx context.Context) (*scheduler.JobResult, error) {
		atomic.AddInt32(&called, 1)
		return &scheduler.JobResult{Summary: "analysis ran"}, nil
	})(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 0, atomic.LoadInt32(&called))
	require.Contains(t, skipped.Summary, "skipped")
}

// TestFirecrawlCrawlRunsWhenModelUnhealthy（A2）：健康门路径（快照 NOT 健康、
// 用户未暂停）下 firecrawl 照常执行；content_completion 形态的 tick 仍 skip。
func TestFirecrawlCrawlRunsWhenModelUnhealthy(t *testing.T) {
	db := setupFirecrawlJobTest(t)
	queue := content.NewFirecrawlJobQueue(db)
	seedFirecrawlArticles(t, db, queue, 2, false)

	require.NoError(t, analysispause.SetPaused(false))
	now := time.Now()
	aihealth.SetSnapshotForTest(aihealth.Snapshot{Healthy: false, CheckedAt: &now})
	t.Cleanup(func() { aihealth.SetSnapshotForTest(aihealth.Snapshot{}) })

	// 对照：content_completion 注册形态（scheduler.PauseAware 包裹）健康门下 skip。
	var called int32
	skipped, err := scheduler.PauseAware(func(ctx context.Context) (*scheduler.JobResult, error) {
		atomic.AddInt32(&called, 1)
		return &scheduler.JobResult{Summary: "content completion ran"}, nil
	})(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 0, atomic.LoadInt32(&called), "analysis-class tick must be skipped while model unhealthy")
	require.Contains(t, skipped.Summary, "model_unhealthy")

	// firecrawl（零 LLM、无暂停包裹）照常抓取。
	res, err := firecrawlJobWithCrawler(queue, "unhealthy-crawl", func(*content.FirecrawlConfig) content.Crawler {
		return newFakeCrawler(time.Millisecond, nil)
	})(context.Background())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotContains(t, res.Summary, "skipped")
	require.Equal(t, 2, res.Data["completed"].(int), "crawl must run when the model is NOT healthy")
}

// TestTagQueueResumeDrainsNewestFirst（B4，spec「恢复后自动续跑」改写口径）：
// 暂停期间任务堆积不被消费，恢复后按新任务优先顺序 lease（不再是 FIFO）。
func TestTagQueueResumeDrainsNewestFirst(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:pause-tagresume-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)

	prevTaggingRepo := taggingrepo.Repo
	prevDatabase := database.DB
	tagging.InitRepository(db)
	database.DB = db
	t.Cleanup(func() {
		taggingrepo.Repo = prevTaggingRepo
		database.DB = prevDatabase
	})
	// AISettings：analysispause.SetPaused 持久化走 ai_settings 表。
	require.NoError(t, db.AutoMigrate(&models.TagJob{}, &models.AISettings{}))

	now := time.Now()
	nextID := uint(1)
	mk := func(age time.Duration) uint {
		job := models.TagJob{
			ArticleID:   nextID,
			Status:      string(models.JobStatusPending),
			AvailableAt: now.Add(-age),
		}
		nextID++
		require.NoError(t, db.Create(&job).Error)
		// created_at is auto-populated by GORM on insert; pin it explicitly.
		require.NoError(t, db.Model(&models.TagJob{}).Where("id = ?", job.ID).
			Update("created_at", now.Add(-age)).Error)
		return job.ID
	}
	oldest := mk(48 * time.Hour)
	mid := mk(8 * time.Hour)
	newest := mk(time.Minute)

	// 暂停期：任务堆积、无任何 lease 发生（tag worker 停消费）。
	require.NoError(t, analysispause.SetPaused(true))
	t.Cleanup(func() { _ = analysispause.SetPaused(false) })
	var leased int64
	require.NoError(t, db.Model(&models.TagJob{}).Where("status = ?", string(models.JobStatusLeased)).Count(&leased).Error)
	require.Zero(t, leased, "no job may be leased while paused")

	// 恢复后：按新任务优先顺序消化（新契约，不再是 created_at FIFO）。
	require.NoError(t, analysispause.SetPaused(false))
	jobs, err := taggingrepo.NewTagJobQueue(db).Claim(3, time.Minute)
	require.NoError(t, err)
	require.Len(t, jobs, 3)
	require.Equal(t, []uint{newest, mid, oldest},
		[]uint{jobs[0].ID, jobs[1].ID, jobs[2].ID},
		"resume must drain newest-first")
}
