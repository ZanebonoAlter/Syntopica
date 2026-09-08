package service_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/config"
)

// ── 自动发现触发（add-evidence-backed-cross-board-relations 7.1/7.2）──────────
//
// 覆盖：板级开关关闭/全局预算为零/全 sparse 简报三道闸门均不 enqueue；
// 开启时按稳定 observation 顺序截取预算内 source；batch 执行失败不回滚简报；
// 同 board 冲突批次跳过留痕。source 选择纯函数覆盖预算 0/1/上限/上限+1。

type autoExecRecorder struct {
	mu        sync.Mutex
	batches   [][]service.RelationSourceRef
	boards    []uint
	parentIDs []uint
	err       error // returned error to simulate per-run failure
}

func (r *autoExecRecorder) record(ctx context.Context, boardID, parentID uint, sources []service.RelationSourceRef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, sources)
	r.boards = append(r.boards, boardID)
	r.parentIDs = append(r.parentIDs, parentID)
}

func (r *autoExecRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.batches)
}

// setRelationAutoConfig pins the global auto budget for one test (restored
// via t.Cleanup).
func setRelationAutoConfig(t *testing.T, budget int) {
	t.Helper()
	saved := config.AppConfig
	config.AppConfig = &config.Config{CrossBoardRel: config.CrossBoardRelationConfig{AutoMaxSourcesPerBrief: budget}}
	t.Cleanup(func() { config.AppConfig = saved })
}

// enableAutoRelations flips the resolver to relation auto on (mock resolver
// drives the gate; the DB column path is covered by board_config_relation_test).
func enableAutoRelations(orch *service.OrchestratorService) {
	orch.SetBoardConfigResolver(&mockBoardResolver{enabled: true, relationAuto: true})
}

func injectAutoRecorder(t *testing.T, orch *service.OrchestratorService) *autoExecRecorder {
	t.Helper()
	rec := &autoExecRecorder{}
	orch.SetAutoDiscoveryExec(rec.record)
	t.Cleanup(func() { orch.SetAutoDiscoveryExec(nil) })
	return rec
}

// TestAutoDiscoveryGates: 板级开关关 / 全局预算零 / 全 sparse → 三种情形都
// 不 enqueue（零后台批次），且简报本身照常落库。
func TestAutoDiscoveryGates(t *testing.T) {
	setRelationAutoConfig(t, 3) // sane global budget for the board-switch case

	t.Run("board switch off", func(t *testing.T) {
		orch, router, repo := newEnrichBoardOrch(t, true) // enrichment on...
		// ...but relation auto discovery off (resolver default).
		seedBoardLane(t, repo, 901, 8822, "泳道甲")
		seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容：一期产能落地", time.Now())
		router.addResponse(validBriefLLM)
		rec := injectAutoRecorder(t, orch)

		out, err := orch.EnrichBoard(context.Background(), 8822)
		require.NoError(t, err)
		require.NotZero(t, out.Result.ID)
		require.Zero(t, rec.count(), "board switch off must not enqueue")
	})

	t.Run("global budget zero", func(t *testing.T) {
		setRelationAutoConfig(t, -1) // explicit disable → effective 0
		require.Zero(t, config.EffectiveCrossBoardRelationConfig().AutoMaxSourcesPerBrief)
		orch, router, repo := newEnrichBoardOrch(t, true)
		enableAutoRelations(orch)
		seedBoardLane(t, repo, 901, 8822, "泳道甲")
		seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容：一期产能落地", time.Now())
		router.addResponse(validBriefLLM)
		rec := injectAutoRecorder(t, orch)

		_, err := orch.EnrichBoard(context.Background(), 8822)
		require.NoError(t, err)
		require.Zero(t, rec.count(), "zero budget must not enqueue")
	})

	t.Run("all sparse brief", func(t *testing.T) {
		setRelationAutoConfig(t, 3)
		orch, router, repo := newEnrichBoardOrch(t, true)
		enableAutoRelations(orch)
		// Lane exists but carries no lifeline material → sparse brief path.
		seedBoardLane(t, repo, 901, 8822, "泳道甲")
		router.addResponse(`{"summary":"素材不足","observations":[],"relationships":[],"uncertainties":[],"research_questions":[],"lane_refs":[]}`)
		rec := injectAutoRecorder(t, orch)

		_, err := orch.EnrichBoard(context.Background(), 8822)
		require.NoError(t, err)
		require.Zero(t, rec.count(), "all-sparse brief must not enqueue")
	})
}

// TestAutoDiscoveryEnqueuesBudgetedSources: 开启 + 预算 2 → 恰好前 2 个
// observation（稳定顺序），parent 指向新简报，EnrichBoard 不被批次拖慢。
func TestAutoDiscoveryEnqueuesBudgetedSources(t *testing.T) {
	setRelationAutoConfig(t, 2)
	orch, router, repo := newEnrichBoardOrch(t, true)
	enableAutoRelations(orch)
	seedBoardLane(t, repo, 901, 8822, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容：一期产能落地", time.Now())
	router.addResponse(`{"summary":"三条观察。","observations":[
		{"id":"o1","lane_id":901,"statement":"观察一","basis":"周摘要","as_of_date":"2026-08-26"},
		{"id":"o2","lane_id":901,"statement":"观察二","basis":"周摘要","as_of_date":"2026-08-26"},
		{"id":"o3","lane_id":901,"statement":"观察三","basis":"周摘要","as_of_date":"2026-08-26"}],
		"relationships":[],"uncertainties":[],"research_questions":[],"lane_refs":[]}`)
	rec := injectAutoRecorder(t, orch)

	out, err := orch.EnrichBoard(context.Background(), 8822)
	require.NoError(t, err)
	// The batch dispatch is async — wait for the recorder to see it.
	require.Eventually(t, func() bool { return rec.count() == 1 }, 5*time.Second, 50*time.Millisecond)
	batch := rec.batches[0]
	require.Len(t, batch, 2)
	require.Equal(t, "o1", batch[0].SourceKey)
	require.Equal(t, "o2", batch[1].SourceKey)
	require.Equal(t, "observation", batch[0].SourceKind)
	require.Equal(t, out.Result.ID, rec.parentIDs[0])
	require.Equal(t, uint(8822), rec.boards[0])
}

// TestAutoDiscoveryBatchFailureNeverRollsBackBrief: 生产 executor（默认
// goroutine 串行批）在流水线整体失败（LLM 无响应）时简报已持久化、无 panic。
func TestAutoDiscoveryBatchFailureNeverRollsBackBrief(t *testing.T) {
	setRelationAutoConfig(t, 1)
	orch, router, repo := newEnrichBoardOrch(t, true)
	enableAutoRelations(orch)
	seedBoardLane(t, repo, 901, 8822, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容：一期产能落地", time.Now())
	// 1) 简报 LLM 输出合法；2) 之后 scout 的 LLM 调用拿不到合法响应 → 流水线失败。
	router.addResponse(validBriefLLM)
	router.addResponse(`not-json`)

	out, err := orch.EnrichBoard(context.Background(), 8822)
	require.NoError(t, err)
	briefID := out.Result.ID
	// 批次在后台跑完（失败），简报仍在库、状态不变。
	require.Eventually(t, func() bool {
		var n int
		if err := repo.DB().Raw(`SELECT COUNT(*) FROM topic_enrichment_result WHERE id = ?`, briefID).Scan(&n).Error; err != nil {
			return false
		}
		return n == 1
	}, 10*time.Second, 100*time.Millisecond)
	// 失败 run 留痕（audit 行存在）。
	require.Eventually(t, func() bool {
		var n int
		_ = repo.DB().Raw(`SELECT COUNT(*) FROM cross_board_relation_runs WHERE source_board_id = 8822`).Scan(&n).Error
		return n >= 1
	}, 15*time.Second, 200*time.Millisecond)
}

// TestSelectAutoRelationSources: 纯函数预算变体 0 / 1 / 上限 / 上限+1。
func TestSelectAutoRelationSources(t *testing.T) {
	obs := []service.BoardBriefObservation{
		{ID: "o1"}, {ID: "o2"}, {ID: "o3"},
	}
	cases := []struct {
		name string
		max  int
		want []string
	}{
		{"budget 0 disables", 0, nil},
		{"budget 1", 1, []string{"o1"}},
		{"budget equals sources", 3, []string{"o1", "o2", "o3"}},
		{"budget exceeds sources", 4, []string{"o1", "o2", "o3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := service.SelectAutoRelationSources(obs, tc.max)
			if tc.want == nil {
				require.Empty(t, got)
				return
			}
			keys := make([]string, 0, len(got))
			for _, s := range got {
				keys = append(keys, s.SourceKey)
			}
			require.Equal(t, tc.want, keys)
		})
	}
	require.Nil(t, service.SelectAutoRelationSources(nil, 3))
}

// TestAutoDiscoveryConflictBatchSkipped: 同 board 已有批次在跑时新批次跳过
// （dispatch 层进程内 guard，留痕不叠加；不同 board 互不阻塞）。
func TestAutoDiscoveryConflictBatchSkipped(t *testing.T) {
	setRelationAutoConfig(t, 2)
	orch, _, _ := newEnrichBoardOrch(t, true)
	release := make(chan struct{})
	var ran []uint
	var mu sync.Mutex
	orch.SetAutoDiscoveryExec(func(ctx context.Context, boardID, parentID uint, sources []service.RelationSourceRef) {
		mu.Lock()
		ran = append(ran, boardID)
		mu.Unlock()
		<-release // hold the batch open after recording
	})
	t.Cleanup(func() { orch.SetAutoDiscoveryExec(nil) })

	cfg := &service.BoardEnrichmentConfig{RelationAutoDiscoveryEnabled: true}
	brief := &service.BoardBriefPayload{Observations: []service.BoardBriefObservation{{ID: "o1"}}}
	// Batch 1 (board 8822) holds the slot; batch 2 same board must skip;
	// batch 3 different board must run.
	orch.MaybeAutoDiscoverRelationsForTest(cfg, 8822, 1, brief)
	require.Eventually(t, func() bool {
		return autoBatchStarted(&mu, &ran)
	}, 5*time.Second, 50*time.Millisecond)
	orch.MaybeAutoDiscoverRelationsForTest(cfg, 8822, 2, brief)
	orch.MaybeAutoDiscoverRelationsForTest(cfg, 8802, 3, brief)
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(ran) == 2
	}, 5*time.Second, 50*time.Millisecond, "same-board conflict must skip, other board must run")
	mu.Lock()
	require.Equal(t, uint(8822), ran[0])
	require.Equal(t, uint(8802), ran[1])
	mu.Unlock()
	close(release)
}

func autoBatchStarted(mu *sync.Mutex, ran *[]uint) bool {
	mu.Lock()
	defer mu.Unlock()
	return len(*ran) == 1
}

var _ = json.Marshal
var _ = repository.RelationStatusProposed
