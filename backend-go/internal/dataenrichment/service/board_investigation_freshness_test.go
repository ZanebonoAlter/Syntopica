package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/airouter"
)

// ── InvestigateBoardQuestion 的 D10 补全门（tasks 4.6 补验收）────────────────
//
// 契约：调查链在 enabled/父简报/question 预检全部通过后、方法选择/假设/研究
// 任何 LLM 之前，对父简报不可变快照推导的 lane 集（与研究白名单同源 helper）
// 跑一次 month/year 补全门；无 lane 安全跳过；失败/限额溢出 nonfatal；不重跑
// brief/cards、不改写父简报；报告固化进 input_snapshot（phase/lanes/report）。
// 预检失败 = 0 freshness LLM + 0 其它 LLM；gate 之后的 ctx 取消仍走原子边界
// 0 行。复用 freshness_gate_test 的 mockFreshnessRefresher seam，不新增
// test-only 生产字段。

// invFreshnessOrch builds an investigation orchestrator with the D10 gate
// wired to the shared mock refresher seam (freshness_gate_test.go).
func invFreshnessOrch(t *testing.T, enabled bool, router service.AirRouter, refresher *mockFreshnessRefresher) (*service.OrchestratorService, *repository.Repository) {
	t.Helper()
	orch, repo := newInvestigationOrchWithRouter(t, enabled, router)
	orch.SetFreshnessRefresher(refresher)
	return orch, repo
}

// invCleanupLaneRows removes the brief's lane lifeline rows (shared PG
// container; keeps tests order-independent).
func invCleanupLaneRows(t *testing.T, repo *repository.Repository, laneIDs ...uint) {
	t.Helper()
	t.Cleanup(func() {
		_ = repo.DB().Exec(`DELETE FROM topic_lifeline_context WHERE persistent_topic_id IN ?`, laneIDs).Error
	})
}

// invOrderLog records cross-mock ordering (gate refreshes vs LLM calls) with
// one shared, mutex-guarded event list.
type invOrderLog struct {
	mu     sync.Mutex
	events []string
}

func (l *invOrderLog) record(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, s)
}

func (l *invOrderLog) snapshotEvents() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

// invOrderRouter records every LLM call into the shared order log.
type invOrderRouter struct {
	service.AirRouter
	log *invOrderLog
}

func (r *invOrderRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	r.log.record("llm:" + req.Operation)
	return r.AirRouter.Chat(ctx, req)
}

// invSeedFreshAndStaleLanes seeds lane 901 fully fresh and lane 902 stale
// (last written 6 days ago) for the periods derivable from monthDates(1, now):
// 2 month periods + 1 year period per lane.
func invSeedFreshAndStaleLanes(t *testing.T, repo *repository.Repository, now time.Time) {
	t.Helper()
	lastMonth := service.FormatMonth(now.AddDate(0, -1, 0))
	thisMonth := service.FormatMonth(now)
	thisYear := service.FormatYear(now)
	for _, p := range []struct{ gran, period string }{
		{"month", lastMonth}, {"month", thisMonth}, {"year", thisYear},
	} {
		seedGranRow(t, repo, 901, p.gran, p.period, now)                   // fresh
		seedGranRow(t, repo, 902, p.gran, p.period, now.AddDate(0, 0, -6)) // stale
	}
}

// ── 调用顺序：gate 先于 method_select/hypothesize，lane 集来自父简报 ─────────

func TestInvestigateBoardFreshness_GateRunsBeforeMethodSelectAndHypothesize(t *testing.T) {
	inner := newMockAirRouter()
	log := &invOrderLog{}
	router := &invOrderRouter{AirRouter: inner, log: log}
	refresher := &mockFreshnessRefresher{}
	orch, repo := invFreshnessOrch(t, true, router, refresher)
	const boardID = uint(95201)
	brief := seedInvBrief(t, repo, boardID)
	invCleanupLaneRows(t, repo, 901, 902)

	now := time.Now()
	refresher.dates = monthDates(1, now)
	invSeedFreshAndStaleLanes(t, repo, now)
	// 成功刷新即进 order log（本用例无失败）。
	refresher.refresh = func(topicID uint, gran, period string) {
		log.record(fmt.Sprintf("refresh:%d/%s/%s", topicID, gran, period))
	}

	m := seedInvMethod(t, repo, "inv-freshness-order-method", "因果链检验", "逐环核对时间先后与独立来源。")
	addInvChainWithSelector(inner, invSynthesisLLM, [2]string{fmt.Sprintf("%d", m.ID), "适配"})

	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	out, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)

	events := log.snapshotEvents()
	firstLLM := -1
	for i, e := range events {
		if strings.HasPrefix(e, "llm:") {
			firstLLM = i
			break
		}
	}
	require.NotEqual(t, -1, firstLLM, "chain must make LLM calls")
	refreshes := 0
	for i, e := range events[:firstLLM] {
		require.True(t, strings.HasPrefix(e, "refresh:"),
			"event %d %q must be a gate refresh — the gate precedes every LLM call", i, e)
		refreshes++
	}
	// 只有陈旧的 902 被重算（2 month + 1 year）；新鲜的 901 零调用。
	require.Equal(t, 3, refreshes, "stale lane 902 only: got events %v", events)
	for i := 0; i < firstLLM; i++ {
		require.True(t, strings.HasPrefix(events[i], "refresh:902/"), "fresh lane 901 must not be refreshed: %v", events)
	}
	// 链上首个 LLM 是方法选择，其次是假设生成（gate 在两者之前）。
	require.Equal(t, "llm:data_enrichment.board_method_select", events[firstLLM])
	require.Equal(t, "llm:data_enrichment.board_hypothesize", events[firstLLM+1])

	// 快照固化：phase=pre_hypothesize、lanes=2（父简报推导）、report.refreshed=3。
	fresh := invSectorsMap(t, out.Result.InputSnapshot)["freshness"].(map[string]any)
	require.Equal(t, "pre_hypothesize", fresh["phase"])
	require.Equal(t, float64(2), fresh["lanes"])
	report := fresh["report"].(map[string]any)
	require.Equal(t, float64(3), report["refreshed"])
}

// ── 陈旧触发补全、新鲜不重复（同日第二次调查 0 补全）───────────────────────

func TestInvestigateBoardFreshness_StaleRefreshedFreshNotRepeated(t *testing.T) {
	inner := newMockAirRouter()
	refresher := &mockFreshnessRefresher{}
	orch, repo := invFreshnessOrch(t, true, inner, refresher)
	const boardID = uint(95211)
	brief := seedInvBrief(t, repo, boardID)
	invCleanupLaneRows(t, repo, 901, 902)

	now := time.Now()
	refresher.dates = monthDates(1, now)
	invSeedFreshAndStaleLanes(t, repo, now)
	// cycle-A 语义：成功重算即写行（UpdatedAt=now → 新鲜）。
	refresher.refresh = func(topicID uint, gran, period string) {
		seedGranRow(t, repo, topicID, gran, period, now)
	}

	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	addInvChain(inner, invSynthesisLLM)
	out1, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)
	first := refresher.callCount()
	require.Equal(t, 3, first, "stale lane 902 only: %v", refresher.callsView())
	for _, c := range refresher.callsView() {
		require.True(t, strings.HasPrefix(c, "902/"), "fresh lane 901 must not be refreshed: %v", refresher.callsView())
	}

	// 同日第二次调查：全部行新鲜 → 0 次补全（幂等，spec「补齐幂等」场景）。
	addInvChain(inner, invSynthesisLLM)
	out2, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)
	require.Equal(t, first, refresher.callCount(), "second same-day investigation must add zero refreshes")
	require.NotEqual(t, out1.Result.ID, out2.Result.ID, "same question re-run is allowed")
	report2 := invSectorsMap(t, out2.Result.InputSnapshot)["freshness"].(map[string]any)["report"].(map[string]any)
	require.Equal(t, float64(0), report2["refreshed"])
}

// callsView snapshots the mock's recorded refresh calls (lock-safe).
func (m *mockFreshnessRefresher) callsView() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.calls...)
}

// ── gate 失败 nonfatal：调查照常完成、快照带 failure、父简报/行数正确 ─────────

func TestInvestigateBoardFreshness_GateFailureNonfatalAndSnapshotCarriesIt(t *testing.T) {
	inner := newMockAirRouter()
	refresher := &mockFreshnessRefresher{}
	orch, repo := invFreshnessOrch(t, true, inner, refresher)
	const boardID = uint(95221)
	brief := seedInvBrief(t, repo, boardID)
	parentBefore := string(brief.Sectors)
	invCleanupLaneRows(t, repo, 901, 902)

	now := time.Now()
	refresher.dates = monthDates(1, now)
	invSeedFreshAndStaleLanes(t, repo, now)
	lastMonth := service.FormatMonth(now.AddDate(0, -1, 0))
	refresher.failOn = map[string]bool{fmt.Sprintf("902/month/%s", lastMonth): true}

	addInvChain(inner, invSynthesisLLM)
	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	out, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err, "gate failure must be nonfatal")
	require.NotNil(t, out.Result)

	// 快照固化失败：failed=1、refreshed=2、details 含 refresh_failed + period。
	fresh := invSectorsMap(t, out.Result.InputSnapshot)["freshness"].(map[string]any)
	require.Equal(t, "pre_hypothesize", fresh["phase"])
	report := fresh["report"].(map[string]any)
	require.Equal(t, float64(1), report["failed"])
	require.Equal(t, float64(2), report["refreshed"])
	var failedDetail map[string]any
	for _, d := range report["details"].([]any) {
		dm := d.(map[string]any)
		if dm["action"] == "refresh_failed" {
			failedDetail = dm
		}
	}
	require.NotNil(t, failedDetail, "details must carry the refresh_failed entry")
	require.Equal(t, lastMonth, failedDetail["period"])
	require.NotEmpty(t, failedDetail["error"])

	// 调查行正常落库；父简报不被 gate 或调查链改写。
	require.Equal(t, int64(1), invCountChildren(t, repo, brief.ID))
	invAssertParentUnchanged(t, repo, brief.ID, parentBefore)
}

// ── 无 lane：安全跳过，不 panic，调查照常 ───────────────────────────────────

// invNoLaneBriefSectors：合法 board_brief 但零泳道引用（observations/
// relationships/research_questions/lane_refs 均无 lane id）。
const invNoLaneBriefSectors = `{"scope":"board","result_kind":"board_brief","summary":"版块观察不足，暂无可引用泳道。","observations":[],"relationships":[],"uncertainties":[{"question":"素材是否足以支撑调查","why_uncertain":"无活跃泳道","needed_evidence":"更多事实"}],"research_questions":[],"lane_refs":[]}`

// invNoLaneSynthesisLLM：无 lane 证据的调查结论（e1 web 来自 stub 检索）。
const invNoLaneSynthesisLLM = `{"hypotheses":[
 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"insufficient","confidence":"low","scope":"板块","support_evidence":[],"counter_evidence":[],"gaps":["无泳道证据"]},
 {"id":"h1","label":"产业基金推动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"近三月","support_evidence":["e1"],"counter_evidence":[],"gaps":[]},
 {"id":"h2","label":"政策补贴同步","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策","support_evidence":[],"counter_evidence":[],"gaps":[]}],
 "conclusion":{"summary":"仅有网页检索材料，证据不足","confidence":"low","scope":"无泳道","boundary":"无可下结论材料"},
 "evidence_chain":[
  {"id":"e1","source_type":"web","url":"https://example.com/a","quote":"基金公告原文摘录ABC","institution":"示例研究所","date":"2026-08-20","supports":["h1"],"counters":[]}],
 "lane_refs":[]}`

// addInvNoLaneChain：无 lane 白名单的研究链——neutral/counter 全走 web_search
// （get_lane_detail 对空白名单必被 ghost_lane 拦截）。
func addInvNoLaneChain(router *mockAirRouter, synthResp string) {
	router.addResponse(invHypothesesLLM)
	router.addResponse(`{"action":"call_tool","thought":"中性事实核查","tool":"web_search","args":{"query":"近三个月该板块公开事件时间线与数据"},"purpose":"neutral","hypothesis_ids":[]}`)
	router.addResponse(`{"action":"call_tool","thought":"反证h1","tool":"web_search","args":{"query":"产业基金 独立资金来源明细"},"purpose":"counter","hypothesis_ids":["h1"]}`)
	router.addResponse(`{"action":"call_tool","thought":"反证h2","tool":"web_search","args":{"query":"补贴政策 时间线对照"},"purpose":"counter","hypothesis_ids":["h2"]}`)
	router.addResponse(`{"action":"finish","thought":"纪律已补齐","summary":"仅有网页检索材料。"}`)
	router.addResponse(synthResp)
}

func seedInvBriefRaw(t *testing.T, repo *repository.Repository, boardID uint, sectors string) *repository.TopicEnrichmentResult {
	t.Helper()
	res := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief,
		Sectors:    json.RawMessage(sectors), SessionID: fmt.Sprintf("inv-seed-brief-%d", boardID),
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(context.Background(), res))
	t.Cleanup(func() {
		_ = repo.DB().Exec(`DELETE FROM topic_enrichment_result WHERE parent_result_id = ?`, res.ID).Error
		_ = repo.DB().Exec(`DELETE FROM topic_enrichment_result WHERE id = ?`, res.ID).Error
	})
	return res
}

func TestInvestigateBoardFreshness_NoLanesSkipsGateSafely(t *testing.T) {
	inner := newMockAirRouter()
	// dates 非空：若 gate 真的对非空 lane 集运行会产生调用——0 调用即证明
	// 空集安全跳过，而非 mock 缺数据。
	refresher := &mockFreshnessRefresher{dates: monthDates(1, time.Now())}
	orch, repo := invFreshnessOrch(t, true, inner, refresher)
	const boardID = uint(95231)
	brief := seedInvBriefRaw(t, repo, boardID, invNoLaneBriefSectors)

	addInvNoLaneChain(inner, invNoLaneSynthesisLLM)
	q := service.BoardInvestigationQuestion{Text: "该板块是否已形成可核实的共同驱动", Source: "custom"}
	out, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q) // MUST NOT panic
	require.NoError(t, err)
	require.NotNil(t, out.Result)
	require.Zero(t, refresher.callCount(), "empty lane set must skip the gate entirely")

	fresh := invSectorsMap(t, out.Result.InputSnapshot)["freshness"].(map[string]any)
	require.Equal(t, float64(0), fresh["lanes"])
	report := fresh["report"].(map[string]any)
	require.Equal(t, float64(0), report["refreshed"])
	require.Empty(t, report["details"].([]any))
}

// ── 预检失败：0 freshness LLM + 0 其它 LLM ─────────────────────────────────

func TestInvestigateBoardFreshness_PreFlightFailuresZeroGateZeroLLM(t *testing.T) {
	inner := newMockAirRouter()
	refresher := &mockFreshnessRefresher{dates: monthDates(1, time.Now())}
	orch, repo := invFreshnessOrch(t, true, inner, refresher)
	const boardID = uint(95241)
	const otherBoard = uint(95242)
	brief := seedInvBrief(t, repo, boardID)

	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	cases := []struct {
		name     string
		boardID  uint
		parentID uint
		question service.BoardInvestigationQuestion
	}{
		{"cross-board parent", otherBoard, brief.ID, q},
		{"missing parent", boardID, 99999999, q},
		{"illegal question source", boardID, brief.ID, service.BoardInvestigationQuestion{Text: "x", Source: "bogus"}},
	}
	for _, tc := range cases {
		_, err := orch.InvestigateBoardQuestion(context.Background(), tc.boardID, tc.parentID, tc.question)
		require.Error(t, err, tc.name)
	}
	require.Empty(t, inner.Calls, "pre-flight failures must make zero LLM calls")
	require.Zero(t, refresher.callCount(), "pre-flight failures must make zero gate refreshes")

	// 停用板块：同样 0 gate / 0 LLM。
	inner2 := newMockAirRouter()
	refresher2 := &mockFreshnessRefresher{dates: monthDates(1, time.Now())}
	orchDisabled, _ := invFreshnessOrch(t, false, inner2, refresher2)
	brief2 := seedInvBrief(t, repo, 95243)
	_, err := orchDisabled.InvestigateBoardQuestion(context.Background(), 95243, brief2.ID, q)
	require.Error(t, err)
	require.Empty(t, inner2.Calls)
	require.Zero(t, refresher2.callCount())
}

// ── gate 之后的 ctx 取消：gate 已跑、原子边界仍 0 行、父简报不动 ─────────────

func TestInvestigateBoardFreshness_CtxCancelAfterGateKeepsZeroRowBoundary(t *testing.T) {
	inner := newMockAirRouter()
	inner.addResponse(invHypothesesLLM) // 取消点在 hypothesize（gate 之后）
	refresher := &mockFreshnessRefresher{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	router := &invCtxCancelRouter{inner: inner, cancelOn: "data_enrichment.board_hypothesize", cancel: cancel}
	orch, repo := invFreshnessOrch(t, true, router, refresher)
	const boardID = uint(95251)
	brief := seedInvBrief(t, repo, boardID)
	parentBefore := string(brief.Sectors)
	invCleanupLaneRows(t, repo, 901, 902)

	// 无既有行 → gate 对两条 lane 各补 3 档（先于取消点执行，ctx 仍存活）。
	refresher.dates = monthDates(1, time.Now())

	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	out, err := orch.InvestigateBoardQuestion(ctx, boardID, brief.ID, q)
	require.Error(t, err)
	require.Nil(t, out)
	require.True(t, errors.Is(err, context.Canceled), "canceled chain must surface context.Canceled, got: %v", err)
	require.Positive(t, refresher.callCount(), "gate must have run (and completed) before the cancel point")
	require.Zero(t, invCountChildren(t, repo, brief.ID), "canceled investigation must leave zero rows")
	invAssertParentUnchanged(t, repo, brief.ID, parentBefore)
}
