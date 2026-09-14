package scheduler

import (
	"context"
	"fmt"
	"time"

	"syntopica-backend/internal/admin/repository"
	adminservice "syntopica-backend/internal/admin/service"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/safefetch"
)

// ── improve-discovery-recommendations 4.6：候选检查 / 向量回补 / 运行维护 三个调度任务 ──
//
// 分类（design D9）：纯 HTTP 检查与运行清理属**维护类**——不受 analysis_paused 影响；
// 候选向量回补属**分析类**——在 runtime 用 scheduler.PauseAware 包装（embedding 受总闸约束）。
//
// discovery_v2 开关（ai_settings.discovery_v2，默认启用）：runtime 关闭时不注册这三个
// 任务；运行中把开关改 false 时，三个 job 各自在入口短路为 skipped 成功，不产生失败噪声。

// CandidateAvailabilityCheckJob 候选可用性周期检查（维护类，不包 PauseAware）。
// 单批取 DueCandidateIDs（next_check_at 到期，private_pending 恒排除，
// requires_parameters 不排队），逐个执行状态机检查；单条失败只记日志，不中断整批
// （可用性失败是数据结论，不是 job 故障）。
func CandidateAvailabilityCheckJob(ctx context.Context) (*JobResult, error) {
	if !adminservice.LoadDiscoveryV2Enabled(repository.Repo.DB()) {
		return skippedDiscoveryV2Job("candidate availability check"), nil
	}
	svc := adminservice.NewCandidateCheckService(repository.Repo.DB())
	due, err := svc.DueCandidateIDs(ctx, time.Now(), adminservice.CandidateCheckDefaultBatchSize)
	if err != nil {
		return nil, fmt.Errorf("candidate availability check: list due candidates: %w", err)
	}
	checked, failed := 0, 0
	for _, id := range due {
		if ctx.Err() != nil {
			break
		}
		res, checkErr := svc.CheckCandidate(ctx, id, safefetch.Options{})
		if checkErr != nil {
			logging.Warnf("candidate availability check: candidate %d failed: %v", id, checkErr)
			failed++
			continue
		}
		_ = res
		checked++
	}
	return &JobResult{
		Data: map[string]interface{}{
			"due":     len(due),
			"checked": checked,
			"failed":  failed,
		},
		Summary: fmt.Sprintf("candidate availability: due=%d checked=%d failed=%d", len(due), checked, failed),
	}, nil
}

// CandidateEmbeddingBackfillJob 候选有效介绍向量增量回补（分析类，runtime 包 PauseAware）。
// 单批上限 20（design D9：初始全量回补限批，不阻塞页面请求）；单条失败保留旧向量、下批重试。
func CandidateEmbeddingBackfillJob(ctx context.Context) (*JobResult, error) {
	if !adminservice.LoadDiscoveryV2Enabled(repository.Repo.DB()) {
		return skippedDiscoveryV2Job("candidate embedding backfill"), nil
	}
	svc := adminservice.NewCandidateEmbeddingService(repository.Repo.DB(), airouter.NewRouter())
	summary, err := svc.DirtyCandidateEmbeddings(ctx, adminservice.CandidateEmbeddingBatchSizeDefault)
	if err != nil {
		return nil, fmt.Errorf("candidate embedding backfill: %w", err)
	}
	return &JobResult{
		Data: map[string]interface{}{
			"generated": summary.Generated,
			"stale":     summary.Stale,
			"failed":    summary.Failed,
			"no_text":   summary.NoText,
		},
		Summary: fmt.Sprintf("candidate embedding backfill: generated=%d stale=%d failed=%d no_text=%d",
			summary.Generated, summary.Stale, summary.Failed, summary.NoText),
	}, nil
}

// DiscoveryRunMaintenanceJob 运行账本维护（维护类，不包 PauseAware）：把超过 1 小时仍
// running 的 DiscoveryRun 置 failed（error_code=stale_running），终结进程崩溃留下的僵尸 run。
func DiscoveryRunMaintenanceJob(ctx context.Context) (*JobResult, error) {
	if !adminservice.LoadDiscoveryV2Enabled(repository.Repo.DB()) {
		return skippedDiscoveryV2Job("discovery run maintenance"), nil
	}
	now := time.Now()
	marked, err := adminservice.MarkStaleRunningDiscoveryRuns(
		ctx, repository.Repo.DB(), now, adminservice.DiscoveryRunStaleThresholdDefault)
	if err != nil {
		return nil, fmt.Errorf("discovery run maintenance: %w", err)
	}
	return &JobResult{
		Data:    map[string]interface{}{"stale_runs_failed": marked},
		Summary: fmt.Sprintf("discovery run maintenance: stale running runs marked failed=%d", marked),
	}, nil
}

// skippedDiscoveryV2Job 是开关关闭时的良性跳过结果（err=nil，不计失败）。
func skippedDiscoveryV2Job(name string) *JobResult {
	return &JobResult{
		Summary: fmt.Sprintf("%s skipped: discovery v2 disabled", name),
		Data:    map[string]interface{}{"skipped": "discovery_v2_disabled"},
	}
}
