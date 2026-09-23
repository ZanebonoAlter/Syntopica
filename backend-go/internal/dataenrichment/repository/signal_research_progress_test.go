package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/testutil"
)

// 研究进展持久化（board-signal-reports tasks 4.7「断了不能白跑」DB 层）。
// 隔离 testcontainer Postgres（golden schema：AutoMigrate + 20260922_0002 的
// CHECK/FK/索引在位）——禁 SQLite。锚点：
//   - 同 job_id 一行滚动更新（ON CONFLICT 覆盖计数/账本/状态/updated_at）；
//   - GetLatest 取候选最近一行；MarkSuperseded 归档保留；
//   - 重启（新 Repository 实例）后直查仍在；
//   - DB 级强制：非法 status 被 CHECK 拒、owner/周期与候选不一致被复合 FK 拒。

func setupSignalProgressDB(t *testing.T) (*repository.Repository, *gorm.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t) // golden schema：AutoMigrate + 全部版本迁移
	return repository.NewRepository(db), db
}

func progressRow(jobID string, candidate *repository.BoardSignalCandidate, rounds int) *repository.BoardSignalResearchProgress {
	return &repository.BoardSignalResearchProgress{
		JobID:            jobID,
		SemanticBoardID:  candidate.SemanticBoardID,
		CandidateID:      candidate.ID,
		Granularity:      candidate.Granularity,
		Period:           candidate.Period,
		RoundsDone:       rounds,
		SourceCalls:      rounds,
		CalculationCalls: 0,
		Ledger:           json.RawMessage(`{"calls":[],"calculations":[],"gaps":[]}`),
		Status:           repository.SignalResearchProgressRunning,
	}
}

// Upsert 滚动更新：同 job_id 写两次仍一行，第二次的计数/账本覆盖第一次；
// 不同 job 各自成行；GetLatest 取最近一行，无进展候选得 nil 而非错误。
func TestUpsertSignalResearchProgressRollsSameJobRow(t *testing.T) {
	repo, db := setupSignalProgressDB(t)
	ctx := context.Background()
	candidate := seedSignalCandidateForProgress(t, repo, 93601)

	require.NoError(t, repo.UpsertSignalResearchProgress(ctx, progressRow("job-r1", candidate, 1)))
	time.Sleep(2 * time.Millisecond) // updated_at 可分辨两次写入
	second := progressRow("job-r1", candidate, 8)
	second.SourceCalls = 9
	second.Ledger = json.RawMessage(`{"calls":[{"call_id":"c1"}],"calculations":[],"gaps":[]}`)
	require.NoError(t, repo.UpsertSignalResearchProgress(ctx, second))

	// 滚动更新：同 job_id 仍只有一行，值为最新快照。
	var sameJobCount int64
	require.NoError(t, db.WithContext(ctx).Model(&repository.BoardSignalResearchProgress{}).
		Where("job_id = ?", "job-r1").Count(&sameJobCount).Error)
	require.Equal(t, int64(1), sameJobCount)
	rolled, err := repo.GetLatestSignalResearchProgress(ctx, candidate.ID)
	require.NoError(t, err)
	require.NotNil(t, rolled)
	require.Equal(t, "job-r1", rolled.JobID)
	require.Equal(t, 8, rolled.RoundsDone)
	require.Equal(t, 9, rolled.SourceCalls)
	require.JSONEq(t, `{"calls":[{"call_id":"c1"}],"calculations":[],"gaps":[]}`, string(rolled.Ledger))

	// 第二个 job 另起一行；每候选最近一行是更晚写入的 job-r2。
	require.NoError(t, repo.UpsertSignalResearchProgress(ctx, progressRow("job-r2", candidate, 2)))
	var allCount int64
	require.NoError(t, db.WithContext(ctx).Model(&repository.BoardSignalResearchProgress{}).
		Where("candidate_id = ?", candidate.ID).Count(&allCount).Error)
	require.Equal(t, int64(2), allCount)
	rows, err := repo.ListLatestSignalResearchProgress(ctx, []uint{candidate.ID})
	require.NoError(t, err)
	require.Len(t, rows, 1) // map 按候选去重，各取最近一行
	require.Equal(t, "job-r2", rows[candidate.ID].JobID)

	// 从未研究过的候选：nil 行而非错误。
	none, err := repo.GetLatestSignalResearchProgress(ctx, candidate.ID+999)
	require.NoError(t, err)
	require.Nil(t, none)
}

// MarkSuperseded：status 翻转、行保留（不删）；重启后（新 Repository 实例）
// 直查仍在——进展在 DB，不随内存 job 蒸发。
func TestMarkSignalResearchProgressSupersededKeepsRow(t *testing.T) {
	repo, db := setupSignalProgressDB(t)
	ctx := context.Background()
	candidate := seedSignalCandidateForProgress(t, repo, 93602)

	require.NoError(t, repo.UpsertSignalResearchProgress(ctx, progressRow("job-s1", candidate, 12)))
	require.NoError(t, repo.MarkSignalResearchProgressSuperseded(ctx, "job-s1"))

	// 模拟进程重启：换新 Repository 实例直查。
	restarted := repository.NewRepository(db)
	got, err := restarted.GetLatestSignalResearchProgress(ctx, candidate.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, repository.SignalResearchProgressSuperseded, got.Status)
	require.Equal(t, 12, got.RoundsDone)
	require.Empty(t, got.StopReason)
	// 未知 job_id 静默成功（尽力而为，不阻塞成功主流程）。
	require.NoError(t, restarted.MarkSignalResearchProgressSuperseded(ctx, "job-never-ran"))
}

// DB 级强制（迁移 20260922_0002）：非法 status 被 CHECK 拒；owner/周期与候
// 选不一致（幽灵归属）被复合 FK 拒。
func TestSignalResearchProgressDBConstraintsRejectIllegalRows(t *testing.T) {
	repo, _ := setupSignalProgressDB(t)
	ctx := context.Background()
	candidate := seedSignalCandidateForProgress(t, repo, 93603)

	bad := progressRow("job-x1", candidate, 1)
	bad.Status = "finished" // CHECK 只认 running|abandoned|superseded
	require.Error(t, repo.UpsertSignalResearchProgress(ctx, bad))

	ghost := progressRow("job-x2", candidate, 1)
	ghost.Period = "2026-07" // 与候选周期不一致 → 复合 FK 拒
	require.Error(t, repo.UpsertSignalResearchProgress(ctx, ghost))

	ghostBoard := progressRow("job-x3", candidate, 1)
	ghostBoard.SemanticBoardID = candidate.SemanticBoardID + 1 // 跨板块归属 → 复合 FK 拒
	require.Error(t, repo.UpsertSignalResearchProgress(ctx, ghostBoard))

	dangling := progressRow("job-x4", candidate, 1)
	dangling.CandidateID = candidate.ID + 999 // 悬空候选 → 复合 FK 拒
	require.Error(t, repo.UpsertSignalResearchProgress(ctx, dangling))
}

// ListLatestSignalResearchProgress 批量取齐：多候选各取最近一行，缺失候选
// 不在 map（列表端摘要一次 IN 查询，不 N+1）。
func TestListLatestSignalResearchProgressBatchLatestPerCandidate(t *testing.T) {
	repo, _ := setupSignalProgressDB(t)
	ctx := context.Background()
	c1 := seedSignalCandidateForProgress(t, repo, 93604)
	c2 := seedSignalCandidateForProgress(t, repo, 93605)

	require.NoError(t, repo.UpsertSignalResearchProgress(ctx, progressRow("job-b1", c1, 3)))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, repo.UpsertSignalResearchProgress(ctx, progressRow("job-b2", c1, 5)))
	require.NoError(t, repo.UpsertSignalResearchProgress(ctx, progressRow("job-b3", c2, 1)))

	rows, err := repo.ListLatestSignalResearchProgress(ctx, []uint{c1.ID, c2.ID, c2.ID + 999})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "job-b2", rows[c1.ID].JobID) // 最近一次
	require.Equal(t, "job-b3", rows[c2.ID].JobID)

	empty, err := repo.ListLatestSignalResearchProgress(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

// 孤儿进展收敛（board-signal-reports tasks 4.10）：启动 sweep 只动 running
// 行——翻 abandoned + stop_reason=orphaned_by_restart；轮次/计数/账本/error
// 列原样保留（候选可查到该次研究已终止与当时轮次）；superseded 行不动；
// 重复 sweep 幂等（0 行）；重启（新 Repository 实例）后终态仍可查。
func TestSweepOrphanedSignalResearchProgressConvergesRunningRows(t *testing.T) {
	repo, db := setupSignalProgressDB(t)
	ctx := context.Background()
	orphan := seedSignalCandidateForProgress(t, repo, 93606)
	done := seedSignalCandidateForProgress(t, repo, 93607)

	orphanRow := progressRow("job-o1", orphan, 34)
	orphanRow.Error = "context deadline exceeded" // 被杀前最后的失败痕迹，sweep 不得抹掉
	require.NoError(t, repo.UpsertSignalResearchProgress(ctx, orphanRow))
	require.NoError(t, repo.UpsertSignalResearchProgress(ctx, progressRow("job-o2", done, 3)))
	require.NoError(t, repo.MarkSignalResearchProgressSuperseded(ctx, "job-o2"))

	converged, err := repo.SweepOrphanedSignalResearchProgress(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), converged)

	// running → abandoned，stop_reason 固定，其余列原样保留。
	var after repository.BoardSignalResearchProgress
	require.NoError(t, db.WithContext(ctx).Where("job_id = ?", "job-o1").First(&after).Error)
	require.Equal(t, repository.SignalResearchProgressAbandoned, after.Status)
	require.Equal(t, repository.SignalResearchStopReasonOrphanedByRestart, after.StopReason)
	require.Equal(t, 34, after.RoundsDone)
	require.Equal(t, 34, after.SourceCalls)
	require.Equal(t, 0, after.CalculationCalls)
	require.JSONEq(t, `{"calls":[],"calculations":[],"gaps":[]}`, string(after.Ledger))
	require.Equal(t, "context deadline exceeded", after.Error)

	// superseded 行不被 sweep 触碰。
	var archived repository.BoardSignalResearchProgress
	require.NoError(t, db.WithContext(ctx).Where("job_id = ?", "job-o2").First(&archived).Error)
	require.Equal(t, repository.SignalResearchProgressSuperseded, archived.Status)
	require.Empty(t, archived.StopReason)

	// 幂等：再 sweep 无行可动。
	convergedAgain, err := repo.SweepOrphanedSignalResearchProgress(ctx)
	require.NoError(t, err)
	require.Zero(t, convergedAgain)

	// 重启后（新 Repository 实例）候选仍可查到已终止与当时轮次。
	restarted := repository.NewRepository(db)
	got, err := restarted.GetLatestSignalResearchProgress(ctx, orphan.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, repository.SignalResearchProgressAbandoned, got.Status)
	require.Equal(t, repository.SignalResearchStopReasonOrphanedByRestart, got.StopReason)
	require.Equal(t, 34, got.RoundsDone)
}

// ── fixtures ─────────────────────────────────────────────────────────────────

func seedSignalCandidateForProgress(t *testing.T, repo *repository.Repository, boardID uint) *repository.BoardSignalCandidate {
	t.Helper()
	ctx := context.Background()
	discovery := newSignalDiscovery(boardID, "2026-08", fmt.Sprintf("signal-progress-%d", boardID))
	persisted, _, err := repo.CreateSignalDiscoveryBatch(ctx, discovery, []*repository.BoardSignalCandidate{
		signalCandidate(boardID, "库存与开工背离信号", []string{"article:1"}),
	})
	require.NoError(t, err)
	require.Len(t, persisted, 1)
	return persisted[0]
}
