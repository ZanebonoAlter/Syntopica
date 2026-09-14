package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/admin"
	"syntopica-backend/internal/admin/scheduler"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/analysispause"
	"syntopica-backend/internal/platform/database"
)

// ── improve-discovery-recommendations 4.6：发现 v2 后台任务的注册接线 ──
//
// 这里测注册函数本身（runtime 无 StartRuntime 级测试惯例）：v2 开关决定三个任务是否
// 进 Registry、分析类回补包 PauseAware 而维护类检查不包、以及 BaseScheduler 的单 job
// 互斥（执行中再次触发 → accepted=false + 409）。

func setupRuntimeSchedulerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:runtime-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.AISettings{}, &models.FeedCandidate{}, &models.RSSHubRoute{},
		&models.CandidateAvailability{}, &models.CandidateEmbedding{}, &models.DiscoveryRun{},
	))
	database.DB = db
	admin.InitRepository(db)
	return db
}

func baseSchedulerOf(t *testing.T, reg *admin.SchedulerRegistry, name string) *scheduler.BaseScheduler {
	t.Helper()
	s, ok := reg.Get(name)
	require.True(t, ok, "scheduler %s should be registered", name)
	bs, ok := s.(*scheduler.BaseScheduler)
	require.True(t, ok, "scheduler %s should be a BaseScheduler, got %T", name, s)
	return bs
}

// TestRegisterDiscoveryV2JobsToggleAndIntervals：v2 开关关闭时三个任务一个都不注册
// （回滚手段）；开启时按固定名字与 1 小时间隔注册（检查/回补/运行维护）。
func TestRegisterDiscoveryV2JobsToggleAndIntervals(t *testing.T) {
	disabled := admin.NewSchedulerRegistry()
	registerDiscoveryV2Jobs(disabled, false)
	require.Empty(t, disabled.OrderedNames(), "v2 关闭时不得注册任何后台任务")

	reg := admin.NewSchedulerRegistry()
	registerDiscoveryV2Jobs(reg, true)
	require.Equal(t, []string{
		"candidate_availability_check",
		"candidate_embedding_backfill",
		"discovery_run_maintenance",
	}, reg.OrderedNames())
	for _, name := range reg.OrderedNames() {
		require.Equal(t, 3600*time.Second, baseSchedulerOf(t, reg, name).GetConfig().Interval, name)
	}
}

// TestDiscoveryV2RegistrationPauseAndSingleExecution：注册形态的语义验证——
//   - 分析类 candidate_embedding_backfill 被 PauseAware 包装：暂停期触发返回 skipped=paused；
//   - 维护类 candidate_availability_check 未包装：暂停期触发照跑（返回业务数据）；
//   - 单 job 互斥：执行中再次触发 accepted=false + status_code 409；
//   - 维护类 discovery_run_maintenance 真实执行并把僵尸 run 置 failed。
func TestDiscoveryV2RegistrationPauseAndSingleExecution(t *testing.T) {
	db := setupRuntimeSchedulerTestDB(t)
	require.NoError(t, analysispause.SetPaused(true))
	t.Cleanup(func() { _ = analysispause.SetPaused(false) })

	reg := admin.NewSchedulerRegistry()
	registerDiscoveryV2Jobs(reg, true)
	backfill := baseSchedulerOf(t, reg, "candidate_embedding_backfill")
	check := baseSchedulerOf(t, reg, "candidate_availability_check")
	maintenance := baseSchedulerOf(t, reg, "discovery_run_maintenance")

	// 分析类：暂停期跳过（良性成功，不计失败）。
	res := backfill.TriggerNow()
	require.Equal(t, true, res["accepted"])
	require.Equal(t, "paused", res["skipped"], "回补 job 必须被 PauseAware 拦住")

	// 维护类：暂停期照跑（零到期候选 → due=0，不是 skipped）。
	res = check.TriggerNow()
	require.Equal(t, true, res["accepted"])
	require.NotContains(t, res, "skipped", "纯 HTTP 检查不受分析暂停影响")
	require.EqualValues(t, 0, res["due"])

	// 单 job 互斥：模拟执行中窗口，第二次触发 accepted=false + 409。
	require.True(t, backfill.TrySetExecuting())
	res = backfill.TriggerNow()
	require.Equal(t, false, res["accepted"])
	require.Equal(t, 409, res["status_code"])
	require.Equal(t, "already_running", res["reason"])
	backfill.ClearExecuting()

	// 维护 job 真实执行：卡死 running 超 1 小时 → failed/stale_running。
	stale := models.DiscoveryRun{
		RequestKey: "rk-runtime-stale", Kind: "ask", Status: "running",
		StartedAt: time.Now().Add(-90 * time.Minute),
	}
	require.NoError(t, db.Create(&stale).Error)
	res = maintenance.TriggerNow()
	require.Equal(t, true, res["accepted"])
	require.EqualValues(t, 1, res["stale_runs_failed"])
	var got models.DiscoveryRun
	require.NoError(t, db.First(&got, stale.ID).Error)
	require.Equal(t, "failed", got.Status)
	require.Equal(t, "stale_running", got.ErrorCode)
}
