package scheduler

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/admin/repository"
	adminservice "syntopica-backend/internal/admin/service"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/analysispause"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/safefetch"
)

// ── improve-discovery-recommendations 4.6：发现 v2 三个后台任务 ──
//
// sqlite 内存库 + 全局 database.DB/repository 指向它（analysispause 与 job 都经全局
// repository/database 读设置），覆盖：分析暂停分类（回补跳过、检查照跑）、v2 开关关闭时
// 三个 job 良性跳过、僵尸 run 维护。

func setupDiscoveryV2SchedulerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:sched-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.AISettings{}, &models.FeedCandidate{}, &models.RSSHubRoute{},
		&models.CandidateAvailability{}, &models.CandidateEmbedding{}, &models.DiscoveryRun{},
	))
	database.DB = db
	repository.InitRepository(db)
	return db
}

// TestDiscoveryV2PauseClassification（design D9）：analysis_paused 打开时，分析类的
// 候选向量回补被 PauseAware 跳过（良性成功，不计失败），维护类的可用性检查照常执行。
func TestDiscoveryV2PauseClassification(t *testing.T) {
	db := setupDiscoveryV2SchedulerDB(t)
	require.NoError(t, analysispause.SetPaused(true))
	t.Cleanup(func() { _ = analysispause.SetPaused(false) })
	ctx := context.Background()

	// 分析类：回补 job 外包 PauseAware（runtime 注册形态）→ 跳过，不调 embedding。
	res, err := PauseAware(CandidateEmbeddingBackfillJob)(ctx)
	require.NoError(t, err, "跳过必须是良性成功（err=nil）")
	require.Equal(t, "paused", res.Data["skipped"])
	require.Contains(t, res.Summary, "paused")

	// 造一条到期候选：检查任务必须在暂停期间照跑（纯 HTTP 检查属维护类）。
	url := "https://example.com/feed.xml"
	enabled := true
	cand := models.FeedCandidate{
		StableKey: "rss:check", Kind: "rss", FeedURL: &url, CanonicalKey: url,
		ManualMetadata:        models.MetadataMap{adminservice.ManualFieldName: "检查目标"},
		RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)
	fetchCalls := 0
	restore := adminservice.SetCandidateCheckFetcher(func(_ context.Context, _ string, _ safefetch.Options) (*adminservice.CandidateFetchResult, error) {
		fetchCalls++
		return &adminservice.CandidateFetchResult{Result: &safefetch.Result{
			StatusCode: 200,
			Body:       []byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>t</title><link>https://example.com</link><item><title>i</title></item></channel></rss>`),
		}}, nil
	})
	t.Cleanup(restore)

	res, err = CandidateAvailabilityCheckJob(ctx)
	require.NoError(t, err, "暂停不拦维护类检查")
	require.EqualValues(t, 1, fetchCalls, "暂停期间检查任务仍执行")
	require.EqualValues(t, 1, res.Data["checked"])

	var row models.CandidateAvailability
	require.NoError(t, db.Where("candidate_id = ?", cand.ID).First(&row).Error)
	require.Equal(t, adminservice.AvailabilityStatusOK, row.Status)
}

// TestDiscoveryV2JobsSkipWhenDisabled：ai_settings.discovery_v2=false 时三个 job 都在入口
// 良性跳过（不产生失败噪声），且检查/维护 job 零副作用。
func TestDiscoveryV2JobsSkipWhenDisabled(t *testing.T) {
	db := setupDiscoveryV2SchedulerDB(t)
	require.NoError(t, adminservice.SaveDiscoveryV2Enabled(db, false))
	ctx := context.Background()

	url := "https://example.com/feed.xml"
	enabled := true
	cand := models.FeedCandidate{
		StableKey: "rss:off", Kind: "rss", FeedURL: &url, CanonicalKey: url,
		ManualMetadata:        models.MetadataMap{adminservice.ManualFieldName: "停用目标"},
		RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)
	stale := models.DiscoveryRun{RequestKey: "rk-off", Kind: "ask", Status: "running", StartedAt: time.Now().Add(-3 * time.Hour)}
	require.NoError(t, db.Create(&stale).Error)

	fetchCalls := 0
	restore := adminservice.SetCandidateCheckFetcher(func(_ context.Context, _ string, _ safefetch.Options) (*adminservice.CandidateFetchResult, error) {
		fetchCalls++
		return &adminservice.CandidateFetchResult{Result: &safefetch.Result{StatusCode: 200}}, nil
	})
	t.Cleanup(restore)

	jobs := map[string]JobFunc{
		"candidate_availability_check": CandidateAvailabilityCheckJob,
		"candidate_embedding_backfill": CandidateEmbeddingBackfillJob,
		"discovery_run_maintenance":    DiscoveryRunMaintenanceJob,
	}
	for name, job := range jobs {
		res, err := job(ctx)
		require.NoError(t, err, name)
		require.Equal(t, "discovery_v2_disabled", res.Data["skipped"], name)
	}
	require.Equal(t, 0, fetchCalls, "开关关闭时不得发起检查请求")
	var availabilityCount int64
	require.NoError(t, db.Model(&models.CandidateAvailability{}).Count(&availabilityCount).Error)
	require.EqualValues(t, 0, availabilityCount)
	var run models.DiscoveryRun
	require.NoError(t, db.First(&run, stale.ID).Error)
	require.Equal(t, "running", run.Status, "开关关闭时维护任务不动作")
}

// TestDiscoveryRunMaintenanceJobMarksStaleRunning（4.6 维护 job）：卡死 running 超 1 小时
// 的 run 置 failed（error_code=stale_running），未超阈值与已终态的行不受影响。
func TestDiscoveryRunMaintenanceJobMarksStaleRunning(t *testing.T) {
	db := setupDiscoveryV2SchedulerDB(t)
	now := time.Now()
	stale := models.DiscoveryRun{RequestKey: "rk-stale", Kind: "ask", Status: "running", StartedAt: now.Add(-2 * time.Hour)}
	fresh := models.DiscoveryRun{RequestKey: "rk-fresh", Kind: "refresh", Status: "running", StartedAt: now.Add(-time.Minute)}
	finished := now.Add(-3 * time.Hour)
	done := models.DiscoveryRun{RequestKey: "rk-done", Kind: "ask", Status: "succeeded", StartedAt: now.Add(-5 * time.Hour), FinishedAt: &finished}
	for _, run := range []*models.DiscoveryRun{&stale, &fresh, &done} {
		require.NoError(t, db.Create(run).Error)
	}

	res, err := DiscoveryRunMaintenanceJob(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Data["stale_runs_failed"])

	var gotStale models.DiscoveryRun
	require.NoError(t, db.First(&gotStale, stale.ID).Error)
	require.Equal(t, adminservice.DiscoveryRunStatusFailed, gotStale.Status)
	require.Equal(t, adminservice.DiscoveryRunErrorStaleRunning, gotStale.ErrorCode)
	require.NotNil(t, gotStale.FinishedAt)

	var gotFresh models.DiscoveryRun
	require.NoError(t, db.First(&gotFresh, fresh.ID).Error)
	require.Equal(t, "running", gotFresh.Status, "未超阈值的运行不得误杀")
	var gotDone models.DiscoveryRun
	require.NoError(t, db.First(&gotDone, done.ID).Error)
	require.Equal(t, "succeeded", gotDone.Status)
}

// TestCandidateAvailabilityCheckJobSingleExecution（验收「Registry 单 job 互斥、
// 并发触发第二次 accepted=false」）：用真实检查 job + 阻塞抓取制造真并发窗口，第一次执行
// 中时第二次触发必须返回 accepted=false + 409，第一次仍正常完成。
func TestCandidateAvailabilityCheckJobSingleExecution(t *testing.T) {
	_ = setupDiscoveryV2SchedulerDB(t)
	url := "https://example.com/feed.xml"
	enabled := true
	cand := models.FeedCandidate{
		StableKey: "rss:concurrent", Kind: "rss", FeedURL: &url, CanonicalKey: url,
		ManualMetadata:        models.MetadataMap{adminservice.ManualFieldName: "并发目标"},
		RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, database.DB.Create(&cand).Error)

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	restore := adminservice.SetCandidateCheckFetcher(func(_ context.Context, _ string, _ safefetch.Options) (*adminservice.CandidateFetchResult, error) {
		once.Do(func() { close(entered) })
		<-release
		return &adminservice.CandidateFetchResult{Result: &safefetch.Result{StatusCode: 404}}, nil
	})
	t.Cleanup(restore)

	reg := NewRegistry()
	base := New(Config{Name: "candidate_availability_check", Interval: time.Hour, Job: CandidateAvailabilityCheckJob})
	reg.Register("candidate_availability_check", base)

	firstResult := make(chan map[string]interface{}, 1)
	go func() { firstResult <- base.TriggerNow() }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first trigger did not reach the fetcher")
	}

	second := base.TriggerNow()
	require.Equal(t, false, second["accepted"], "执行中不得并发跑同一 job")
	require.Equal(t, 409, second["status_code"])
	require.Equal(t, "already_running", second["reason"])

	close(release)
	first := <-firstResult
	require.Equal(t, true, first["accepted"], "第一次触发不受第二次影响，正常完成")
}

// TestCandidateAvailabilityCheckJobBatchLimit（4.6 检查 job 分批）：单轮最多取
// DueCandidateIDs 默认批量（50），不一次扫全库。
func TestCandidateAvailabilityCheckJobBatchLimit(t *testing.T) {
	db := setupDiscoveryV2SchedulerDB(t)
	for i := 0; i < adminservice.CandidateCheckDefaultBatchSize+5; i++ {
		url := fmt.Sprintf("https://example.com/feed-%d.xml", i)
		enabled := true
		seed := models.FeedCandidate{
			StableKey: fmt.Sprintf("rss:batch-%d", i), Kind: "rss", FeedURL: &url, CanonicalKey: url,
			ManualMetadata:        models.MetadataMap{adminservice.ManualFieldName: fmt.Sprintf("批量%d", i)},
			RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
		}
		require.NoError(t, db.Create(&seed).Error)
	}
	fetchCalls := 0
	restore := adminservice.SetCandidateCheckFetcher(func(_ context.Context, _ string, _ safefetch.Options) (*adminservice.CandidateFetchResult, error) {
		fetchCalls++
		return &adminservice.CandidateFetchResult{Result: &safefetch.Result{StatusCode: 410}}, nil
	})
	t.Cleanup(restore)

	res, err := CandidateAvailabilityCheckJob(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, adminservice.CandidateCheckDefaultBatchSize, res.Data["due"])
	require.Equal(t, adminservice.CandidateCheckDefaultBatchSize, fetchCalls)
	require.EqualValues(t, fetchCalls, res.Data["checked"])
}
