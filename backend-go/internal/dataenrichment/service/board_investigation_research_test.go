package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"syntopica-backend/internal/platform/airouter"
)

// ── 共享研究循环（tasks 4.2/4.4，design D4/D7，test-cases M5）────────────────
//
// 用例清单（任务 4.2 + 4.4 review 修复 + test-cases M5）：
//   M5.1 单一共享 loop 服务全部假设（非每假设一套）
//   M5.2 研究计划含中性查询+每个非零假设反证；提前 finish 被拒继续
//   M5.3 非零假设至少一次反证尝试；无结果记 gap 不伪造
//   M5.4 同 tool+args 重复拦截；重复拦截不冒充完成纪律
//   M5.5 web_search/fetch_page 未配置 nonfatal + gap
//   中性查询照抄假设 label 被拒（保守判定）
//   幽灵 lane_id 执行前拦截留痕；非数字类型反馈分开（不误称合法 id 不存在）
//   counter 声明纪律：多目标/空目标/零假设目标拦截不计 attempt
//   四假设（H0+3）在 1 neutral + 3 counter + finish = 5 轮内完成
//   显式 AllowedTools 缺外部工具 → 预置 tool_unavailable gap（去重、不设 finish 门）
//   lane 白名单从父简报推导；输入只能收窄/确认；prompt/guard 同源
//   invalid action → llm_error gap（非 max_loops）
//   工具 Execute 错误文本含引号/换行 → ResultFull 仍可解析（policy=nil 与 investigation 双路径）
//   max_loops=6 partial + gaps；LLM 失败 partial + gap；ctx 取消返回 error
//   full result 不截断；prompt 契约（无方法卡正文/无赢家）
//   旧 runToolLoop policy=nil 行为回归（结构化注记不出现）

// ── 测试 mock ───────────────────────────────────────────────────────────────

// researchRouterStep is one canned Chat outcome (content or error).
type researchRouterStep struct {
	content string
	err     error
}

// researchMockRouter returns canned Chat results in order (honors ctx
// cancellation like a real router), then "{}".
type researchMockRouter struct {
	steps []researchRouterStep
	idx   int
	calls []airouter.ChatRequest
}

func (m *researchMockRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	m.calls = append(m.calls, req)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if m.idx >= len(m.steps) {
		return &airouter.ChatResult{Content: "{}"}, nil
	}
	s := m.steps[m.idx]
	m.idx++
	if s.err != nil {
		return nil, s.err
	}
	return &airouter.ChatResult{Content: s.content}, nil
}

// researchLaneRenderer records get_lane_detail calls (ghost-lane assertion).
type researchLaneRenderer struct {
	calls []uint
}

func (r *researchLaneRenderer) RenderLaneDetail(_ context.Context, laneID uint, _ int) (string, error) {
	r.calls = append(r.calls, laneID)
	return fmt.Sprintf("泳道%d 近期演进详情：两篇文章、一次招标公告", laneID), nil
}

// researchWebSearcher records queries and returns one fixed hit.
type researchWebSearcher struct {
	queries []string
	long    bool
}

func (s *researchWebSearcher) Search(_ context.Context, query string) ([]WebSearchResult, error) {
	s.queries = append(s.queries, query)
	snippet := "固定摘要"
	if s.long {
		snippet = strings.Repeat("长", 600)
	}
	return []WebSearchResult{{Title: "命中", URL: "https://example.com/a", Snippet: snippet}}, nil
}

// ── 测试素材 ────────────────────────────────────────────────────────────────

const researchTestSession = "data_enrichment_board_7_ab12cd34"

func researchHypotheses() []boardHypothesis {
	return []boardHypothesis{
		{ID: "h0", Label: "零假设：两条泳道变化没有统一机制，可由各自独立的普通因素分别解释", IsNull: true,
			SupportNeeded: []string{"共同机制一手证据"}, DisconfirmNeeded: []string{"各自独立解释成立"}, Scope: "本板块两条泳道"},
		{ID: "h1", Label: "同一产业基金同时推动产能与招标", IsNull: false,
			SupportNeeded: []string{"基金公告同时提及两泳道"}, DisconfirmNeeded: []string{"资金来源明细互相独立"}, Scope: "近三个月"},
		{ID: "h2", Label: "政策补贴周期同步带动", IsNull: false,
			SupportNeeded: []string{"补贴文本覆盖两泳道"}, DisconfirmNeeded: []string{"补贴时间线与变化不重合"}, Scope: "政策周期"},
	}
}

func researchTestInput(hyps []boardHypothesis) BoardInvestigationResearchInput {
	return BoardInvestigationResearchInput{
		SessionID:     researchTestSession,
		Question:      BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: QuestionSourceGenerated},
		Brief:         investigationTestBrief(),
		LaneWhitelist: []uint{1, 2},
		Hypotheses:    hyps,
		EvidenceNeeds: []string{"资金来源的一手公告", "两条泳道时间线的独立对照"},
	}
}

// researchCallDecision builds a call_tool decision JSON (json.Marshal keeps
// CJK/quotes safe).
func researchCallDecision(tool string, args map[string]any, purpose string, hyps ...string) string {
	b, _ := json.Marshal(map[string]any{
		"action": "call_tool", "thought": "查证", "tool": tool, "args": args,
		"purpose": purpose, "hypothesis_ids": hyps,
	})
	return string(b)
}

func researchFinishDecision() string {
	return `{"action":"finish","thought":"纪律已补齐","summary":"按假设分组的素材汇总：h0/h1/h2 各自支持、反证与缺口。"}`
}

// researchOrch builds an orchestrator with the given registry (repo/lifeline
// not needed by the research loop).
func researchOrch(router AirRouter, registry *Registry) *OrchestratorService {
	return NewOrchestratorService(router, nil, nil, NewLifelineRenderer(), registry, nil, internalTestCap)
}

// researchRegistry is a registry with stub lane renderer + web searcher.
func researchRegistry(lane *researchLaneRenderer, ws *researchWebSearcher) *Registry {
	return NewRegistry(&nilFetcherHTTP{},
		WithLaneDetailRenderer(lane),
		WithWebSearcher(ws),
	)
}

// ── M5.1：单一共享 loop 覆盖全部假设 ────────────────────────────────────────

func TestBoardInvestigationResearch_SingleLoopCoversAllHypotheses(t *testing.T) {
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "招标公告 资金来源 产能"}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "产业基金 独立资金来源明细"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "补贴政策 时间线 对照"}, ResearchPurposeCounter, "h2")},
		{content: researchCallDecision("web_search", map[string]any{"query": "基金公告 产能 招标"}, ResearchPurposeSupport, "h1")},
		{content: researchFinishDecision()},
	}}
	lane := &researchLaneRenderer{}
	ws := &researchWebSearcher{}
	orch := researchOrch(router, researchRegistry(lane, ws))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("runBoardInvestigationResearch: %v", err)
	}

	// M5.1：一个 loop（一次 runToolLoop），LLM 调用数=决策数，无每假设额外循环。
	if res.Loop == nil {
		t.Fatal("expected one shared loop result")
	}
	if len(router.calls) != 6 {
		t.Fatalf("expected exactly 6 LLM calls (one shared loop), got %d", len(router.calls))
	}
	for i, c := range router.calls {
		if c.Operation != "data_enrichment.tool_use" {
			t.Fatalf("call %d operation: got %s, want data_enrichment.tool_use", i, c.Operation)
		}
		if c.SessionID != researchTestSession {
			t.Fatalf("call %d session: got %s", i, c.SessionID)
		}
	}

	// 纪律：中性 + 每个非零假设 counter（h0 零假设无配额）。
	if !res.Coverage.NeutralAttempted {
		t.Fatal("coverage.NeutralAttempted should be true")
	}
	got := strings.Join(res.Coverage.CounterAttemptedByHypothesis, ",")
	if got != "h1,h2" {
		t.Fatalf("coverage.CounterAttemptedByHypothesis: got %q, want h1,h2", got)
	}

	// 纪律齐备：无 missing_* / max_loops / llm_error gap。
	for _, g := range res.Gaps {
		switch g.Reason {
		case researchGapMissingNeutral, researchGapMissingCounter, researchGapMaxLoops, researchGapLLMError:
			t.Fatalf("unexpected discipline gap %+v", g)
		}
	}

	// finish 成功，完整工具调用顺序带注记。
	if res.Loop.FinalData == "" {
		t.Fatal("finish should set FinalData")
	}
	if len(res.Loop.ToolCalls) != 5 {
		t.Fatalf("tool calls: got %d, want 5", len(res.Loop.ToolCalls))
	}
	for i, tc := range res.Loop.ToolCalls {
		if tc.Purpose == "" || tc.Outcome != toolCallOutcomeOK {
			t.Fatalf("tool call %d missing annotations: %+v", i, tc)
		}
	}
	if res.Loop.ToolCalls[0].Purpose != ResearchPurposeNeutral || len(res.Loop.ToolCalls[0].HypothesisIDs) != 0 {
		t.Fatalf("first call should be neutral without targets: %+v", res.Loop.ToolCalls[0])
	}
}

// ── M5.2/M5.3：提前 finish 被拒，反馈进历史，补齐后成功 ────────────────────

func TestBoardInvestigationResearch_EarlyFinishRejectedThenContinues(t *testing.T) {
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchFinishDecision()}, // 过早 finish：无 neutral、无 counter
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 2}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "资金来源独立证据"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "补贴时间线"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	orch := researchOrch(router, researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{}))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("runBoardInvestigationResearch: %v", err)
	}

	if len(res.FinishRejections) != 1 {
		t.Fatalf("finish rejections: got %d, want 1 (%v)", len(res.FinishRejections), res.FinishRejections)
	}
	if !strings.Contains(res.FinishRejections[0], "中性") || !strings.Contains(res.FinishRejections[0], "h1") {
		t.Fatalf("rejection feedback should name neutral + missing counters: %s", res.FinishRejections[0])
	}
	// 反馈进入下一轮 user 消息（agent 能看到并改写）。
	if len(router.calls) < 2 {
		t.Fatal("expected continued rounds after rejected finish")
	}
	second := router.calls[1]
	userMsg := second.Messages[len(second.Messages)-1].Content
	if !strings.Contains(userMsg, "宣布完成被拦") {
		t.Fatalf("feedback missing from follow-up history: %s", userMsg)
	}
	if res.Loop.FinalData == "" {
		t.Fatal("second finish should be accepted")
	}
	if res.Coverage.NeutralAttempted != true || len(res.Coverage.CounterAttemptedByHypothesis) != 2 {
		t.Fatalf("coverage after continuation: %+v", res.Coverage)
	}
}

// ── 中性查询照抄假设 label 被机械拒绝（保守判定）────────────────────────────

func TestBoardInvestigationResearch_NeutralQueryLabelCopyBlocked(t *testing.T) {
	hyps := researchHypotheses()
	label := hyps[1].Label // "同一产业基金同时推动产能与招标"
	router := &researchMockRouter{steps: []researchRouterStep{
		// 1) neutral 查询=照抄 h1 label → 拒
		{content: researchCallDecision("web_search", map[string]any{"query": label}, ResearchPurposeNeutral)},
		// 2) 改写为事实性查询 → 执行
		{content: researchCallDecision("web_search", map[string]any{"query": "招标公告 资金来源明细"}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "独立资金来源"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "补贴时间线"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	ws := &researchWebSearcher{}
	orch := researchOrch(router, researchRegistry(&researchLaneRenderer{}, ws))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(hyps))
	if err != nil {
		t.Fatalf("runBoardInvestigationResearch: %v", err)
	}

	blocked := res.Loop.ToolCalls[0]
	if blocked.BlockedReason != "neutral_query_conclusion_copy" || blocked.Outcome != toolCallOutcomeBlocked {
		t.Fatalf("first call should be blocked as conclusion copy: %+v", blocked)
	}
	// 工具未执行：照抄 label 的查询从未到达搜索器，只有改写后的中性查询与两次反证。
	if len(ws.queries) != 3 || ws.queries[0] != "招标公告 资金来源明细" || strings.Contains(strings.Join(ws.queries, "|"), label) {
		t.Fatalf("web_searcher queries: %v", ws.queries)
	}
	// 拒绝反馈进历史。
	if len(router.calls) < 2 {
		t.Fatal("expected continuation after blocked query")
	}
	userMsg := router.calls[1].Messages[len(router.calls[1].Messages)-1].Content
	if !strings.Contains(userMsg, "中性查询照抄") && !strings.Contains(userMsg, "neutral_query_conclusion_copy") {
		t.Fatalf("blocked feedback should reach the agent: %s", userMsg)
	}
	// 改写后的中性查询计入 coverage。
	if !res.Coverage.NeutralAttempted {
		t.Fatal("rewritten neutral query should count")
	}
}

// label 加少量垫字（“查一下<label>”）同样算照抄；复杂事实查询放行。
func TestBoardInvestigationResearch_NeutralQueryLabelCopyWithPadding(t *testing.T) {
	hyps := researchHypotheses()
	label := hyps[1].Label
	pol := newInvestigationPolicy(hyps, []uint{1, 2})
	if !pol.isConclusionCopy("查一下" + label) {
		t.Fatal("label with trivial padding should be rejected as conclusion copy")
	}
	// 复杂正常查询（含 label 词但带实质事实限定）应放行。
	if pol.isConclusionCopy(label + " 的招标编号与开标时间是什么") {
		t.Fatal("complex factual query containing label words should be allowed")
	}
}

// ── M5.4：duplicate 拦截不冒充完成纪律 ─────────────────────────────────────

func TestBoardInvestigationResearch_DuplicateBlockedNotNewAttempt(t *testing.T) {
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("web_search", map[string]any{"query": "Q1"}, ResearchPurposeCounter, "h1")}, // 执行
		{content: researchCallDecision("web_search", map[string]any{"query": "Q1"}, ResearchPurposeCounter, "h2")}, // 同参重复 → dedup 拦截，h2 不计
		{content: researchFinishDecision()}, // h2 缺 counter + 缺 neutral → 拒
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "Q2"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	ws := &researchWebSearcher{}
	orch := researchOrch(router, researchRegistry(&researchLaneRenderer{}, ws))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("runBoardInvestigationResearch: %v", err)
	}

	dup := res.Loop.ToolCalls[1]
	if dup.BlockedReason != "duplicate_call" || dup.Outcome != toolCallOutcomeBlocked {
		t.Fatalf("second same-args call should be dedup-blocked with structured reason: %+v", dup)
	}
	// finish 被拒过一次（h2 counter 与 neutral 缺失）。
	if len(res.FinishRejections) != 1 || !strings.Contains(res.FinishRejections[0], "h2") {
		t.Fatalf("finish rejection should mention h2: %v", res.FinishRejections)
	}
	// 搜索器只执行了 Q1 与 Q2。
	if strings.Join(ws.queries, "|") != "Q1|Q2" {
		t.Fatalf("executed queries: %v", ws.queries)
	}
	if res.Loop.FinalData == "" || len(res.Coverage.CounterAttemptedByHypothesis) != 2 {
		t.Fatalf("final state wrong: coverage=%+v", res.Coverage)
	}
}

// ── M5.5：web/fetch 未配置 nonfatal + gap ───────────────────────────────────

func TestBoardInvestigationResearch_ExternalToolsUnavailableNonfatal(t *testing.T) {
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("web_search", map[string]any{"query": "外部核查"}, ResearchPurposeNeutral)},
		{content: researchCallDecision("fetch_page", map[string]any{"url": "https://example.com/doc"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "补贴时间线"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	// 无 WithWebSearcher（Noop 默认）也无 WithPageFetcher。
	orch := researchOrch(router, NewRegistry(&nilFetcherHTTP{}, WithLaneDetailRenderer(&researchLaneRenderer{})))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("unavailable external tools must be non-fatal: %v", err)
	}

	if res.Loop.FinalData == "" {
		t.Fatal("loop should finish despite unavailable tools")
	}
	byTool := map[string]int{}
	for _, g := range res.Gaps {
		if g.Reason != researchGapToolUnavailable {
			t.Fatalf("want tool_unavailable gap, got %+v", g)
		}
		byTool[g.Tool]++
	}
	if byTool["web_search"] != 2 || byTool["fetch_page"] != 1 {
		t.Fatalf("gap tools: %v", byTool)
	}
	// 未配置/失败算尝试：纪律 coverage 仍齐。
	if !res.Coverage.NeutralAttempted || len(res.Coverage.CounterAttemptedByHypothesis) != 2 {
		t.Fatalf("coverage should count failed attempts: %+v", res.Coverage)
	}
	// 失败工具记录 outcome=error，完整错误在 ResultFull。
	for _, tc := range res.Loop.ToolCalls {
		if (tc.Tool == "web_search" || tc.Tool == "fetch_page") && tc.Outcome != toolCallOutcomeError {
			t.Fatalf("unavailable tool call should be outcome=error: %+v", tc)
		}
	}
}

// ── 幽灵 lane：执行前拦截留痕 ───────────────────────────────────────────────

func TestBoardInvestigationResearch_GhostLaneInterceptedBeforeExecution(t *testing.T) {
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 999}, ResearchPurposeNeutral)},
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证 h1"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证 h2"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	lane := &researchLaneRenderer{}
	orch := researchOrch(router, researchRegistry(lane, &researchWebSearcher{}))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("runBoardInvestigationResearch: %v", err)
	}

	ghost := res.Loop.ToolCalls[0]
	if ghost.BlockedReason != "ghost_lane" || ghost.Outcome != toolCallOutcomeBlocked {
		t.Fatalf("ghost lane call should be blocked: %+v", ghost)
	}
	if len(lane.calls) != 1 || lane.calls[0] != 1 {
		t.Fatalf("renderer must never see ghost lane: %v", lane.calls)
	}
	// 白名单内正常执行。
	if res.Loop.ToolCalls[1].Outcome != toolCallOutcomeOK || res.Loop.ToolCalls[1].Tool != "get_lane_detail" {
		t.Fatalf("whitelisted lane call should execute: %+v", res.Loop.ToolCalls[1])
	}
}

// ── max_loops=6：partial result + gaps，不抛错 ──────────────────────────────

func TestBoardInvestigationResearch_MaxLoopsPartialWithGaps(t *testing.T) {
	steps := make([]researchRouterStep, 0, 7)
	for i := 0; i < 6; i++ {
		steps = append(steps, researchRouterStep{
			content: researchCallDecision("web_search", map[string]any{"query": fmt.Sprintf("支持查询%d", i)}, ResearchPurposeSupport, "h1"),
		})
	}
	steps = append(steps, researchRouterStep{content: researchFinishDecision()}) // 永远轮不到
	router := &researchMockRouter{steps: steps}
	orch := researchOrch(router, researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{}))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("max loops must return partial, not error: %v", err)
	}

	if res.Loop.Loops != 6 {
		t.Fatalf("loops: got %d, want 6", res.Loop.Loops)
	}
	if res.Loop.FinalData != "" {
		t.Fatal("no finish should have been accepted")
	}
	reasons := map[string]bool{}
	for _, g := range res.Gaps {
		reasons[g.Reason] = true
	}
	if !reasons[researchGapMaxLoops] {
		t.Fatalf("gaps should include max_loops: %+v", res.Gaps)
	}
	// 全程只有 support(h1)：neutral 与 h2 counter 缺失要如实记录。
	if !reasons[researchGapMissingNeutral] || !reasons[researchGapMissingCounter] {
		t.Fatalf("gaps should include missing_neutral/missing_counter: %+v", res.Gaps)
	}
}

// ── LLM 失败：partial + llm_error gap；ctx 取消：返回 error ─────────────────

func TestBoardInvestigationResearch_LLMErrorPartialWithGap(t *testing.T) {
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{err: errors.New("upstream 503: internal host=x.y")}, // 完整错误只进 ResultFull/日志
	}}
	orch := researchOrch(router, researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{}))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("LLM failure must return partial, not error: %v", err)
	}
	found := false
	for _, g := range res.Gaps {
		if g.Reason == researchGapLLMError {
			found = true
		}
		if strings.Contains(g.Tool, "internal") { // Tool 为空，此处防未来塞敏感串
			t.Fatalf("gap must not carry internal error detail: %+v", g)
		}
	}
	if !found {
		t.Fatalf("gaps should include llm_error: %+v", res.Gaps)
	}
	// 敏感内部错误不进 gap：逐字段扫一遍。
	for _, g := range res.Gaps {
		b, _ := json.Marshal(g)
		if strings.Contains(string(b), "internal host") {
			t.Fatalf("gap leaked internal error: %s", b)
		}
	}
}

func TestBoardInvestigationResearch_CtxCancelledReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchFinishDecision()},
	}}
	orch := researchOrch(router, researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{}))

	res, err := orch.RunBoardInvestigationResearch(ctx, researchTestInput(researchHypotheses()))
	if err == nil {
		t.Fatal("canceled context should surface as error")
	}
	if res != nil {
		t.Fatalf("canceled run should not return a usable result: %+v", res)
	}
}

// ── 防御②：full result 不截断（policy 路径同样成立）─────────────────────────

func TestBoardInvestigationResearch_FullResultNotTruncated(t *testing.T) {
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("web_search", map[string]any{"query": "长结果"}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证1"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证2"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	ws := &researchWebSearcher{long: true}
	orch := researchOrch(router, researchRegistry(&researchLaneRenderer{}, ws))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("runBoardInvestigationResearch: %v", err)
	}
	tc := res.Loop.ToolCalls[0]
	if !strings.Contains(tc.ResultFull, strings.Repeat("长", 600)) {
		t.Fatalf("ResultFull must keep the complete tool result (len=%d)", len(tc.ResultFull))
	}
	// 预览沿用既有截断（字节级 ≤300），只断言确实短于完整结果且不超限。
	if len(tc.ResultPreview) >= len(tc.ResultFull) || len([]rune(tc.ResultPreview)) > 300 {
		t.Fatalf("ResultPreview should stay a capped preview: rune len=%d", len([]rune(tc.ResultPreview)))
	}
}

// ── purpose/目标假设非法：拦截并反馈 ────────────────────────────────────────

func TestBoardInvestigationResearch_InvalidPurposeAndUnknownTargetBlocked(t *testing.T) {
	noPurpose := `{"action":"call_tool","thought":"忘声明","tool":"web_search","args":{"query":"事实"},"hypothesis_ids":[]}`
	unknownTarget := researchCallDecision("web_search", map[string]any{"query": "反证"}, ResearchPurposeCounter, "hX")
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: noPurpose},
		{content: unknownTarget},
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证1"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证2"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	orch := researchOrch(router, researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{}))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("runBoardInvestigationResearch: %v", err)
	}
	if res.Loop.ToolCalls[0].BlockedReason != "invalid_purpose" {
		t.Fatalf("missing purpose should be blocked: %+v", res.Loop.ToolCalls[0])
	}
	if res.Loop.ToolCalls[1].BlockedReason != "invalid_hypothesis_target" {
		t.Fatalf("unknown hypothesis target should be blocked: %+v", res.Loop.ToolCalls[1])
	}
	if res.Loop.FinalData == "" {
		t.Fatal("loop should recover and finish")
	}
}

// ── prompt 契约：问题/假设/证据需求/泳道白名单；无方法卡正文、无赢家 ─────────

func TestBoardInvestigationResearch_PromptContract(t *testing.T) {
	in := researchTestInput(researchHypotheses())
	registry := researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{})
	desc := buildToolsDesc(registry, explorationToolNames)
	prompt := assembleBoardInvestigationResearchPrompt(in, desc)

	for _, want := range []string{
		in.Question.Text,
		"h0", "h1", "h2",
		"同一产业基金同时推动产能与招标", // h1 label
		"资金来源的一手公告",       // evidence need
		"政策补贴周期同步带动",
		ResearchPurposeNeutral, ResearchPurposeSupport, ResearchPurposeCounter,
		"web_search",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	// 泳道白名单明示。
	if !strings.Contains(prompt, "1") || !strings.Contains(prompt, "2") {
		t.Error("prompt should list lane whitelist ids")
	}
	// 纪律要求明示。
	for _, want := range []string{"中性", "反证", "零假设", "质量"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing discipline keyword %q", want)
		}
	}
	// 方法卡正文/作者文风/赢家不入 prompt：输入结构上没有该方法参数，
	// 再对哨兵词兜底断言（AssembleSelectedAnalysisMethods 的装配产物不会出现）。
	for _, banned := range []string{"分析方法参考", "CONTENT-", "作者", "模仿"} {
		if strings.Contains(prompt, banned) {
			t.Errorf("prompt must not contain %q", banned)
		}
	}
	// 输入校验：空假设集合拒绝。
	bad := researchTestInput(nil)
	if _, err := researchOrch(&researchMockRouter{}, researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{})).RunBoardInvestigationResearch(context.Background(), bad); err == nil {
		t.Error("empty hypotheses should be rejected")
	}
}

// ── counter 声明纪律：恰好一个非零假设（review 4.2）─────────────────────

// 多目标/零假设目标/空目标均在执行前拦截不计 attempt；拦截后循环能恢复完成。
func TestBoardInvestigationResearch_CounterDeclarationEnforced(t *testing.T) {
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("web_search", map[string]any{"query": "批量反证"}, ResearchPurposeCounter, "h1", "h2")}, // 多目标 → 拦
		{content: researchCallDecision("web_search", map[string]any{"query": "反证零假设"}, ResearchPurposeCounter, "h0")},      // 只 target H0 → 拦
		{content: researchCallDecision("web_search", map[string]any{"query": "事实核查 对象与时间"}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证1"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证2"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	ws := &researchWebSearcher{}
	orch := researchOrch(router, researchRegistry(&researchLaneRenderer{}, ws))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("runBoardInvestigationResearch: %v", err)
	}
	for i := 0; i < 2; i++ {
		tc := res.Loop.ToolCalls[i]
		if tc.BlockedReason != "invalid_counter_target" || tc.Outcome != toolCallOutcomeBlocked {
			t.Fatalf("call %d should be blocked invalid_counter_target: %+v", i, tc)
		}
	}
	// 拦截的调用从未执行（搜索器只见过合法三条），不计 attempt。
	if strings.Join(ws.queries, "|") != "事实核查 对象与时间|反证1|反证2" {
		t.Fatalf("executed queries: %v", ws.queries)
	}
	if res.Loop.FinalData == "" {
		t.Fatal("loop should recover and finish")
	}
	if got := strings.Join(res.Coverage.CounterAttemptedByHypothesis, ","); got != "h1,h2" {
		t.Fatalf("coverage: got %q, want h1,h2", got)
	}
	// 反馈进 agent 可见历史。
	userMsg := router.calls[2].Messages[len(router.calls[2].Messages)-1].Content
	if !strings.Contains(userMsg, "一次只能反证一个") && !strings.Contains(userMsg, "不能以零假设") {
		t.Fatalf("counter-blocked feedback should reach the agent: %s", userMsg)
	}

	// 空目标 counter 同类别拦截（单独一轮，避免撞 max_loops 预算）。
	router2 := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("web_search", map[string]any{"query": "空目标反证"}, ResearchPurposeCounter)},
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证1"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证2"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	res2, err := researchOrch(router2, researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{})).
		RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if tc := res2.Loop.ToolCalls[0]; tc.BlockedReason != "invalid_counter_target" || tc.Outcome != toolCallOutcomeBlocked {
		t.Fatalf("empty counter target should be blocked invalid_counter_target: %+v", tc)
	}
	if res2.Loop.FinalData == "" {
		t.Fatal("second run should finish")
	}
}

// 四假设（H0+3）在 1 neutral + 3 counter + finish = 5 轮内完成，仍一个共享
// loop（maxAgentLoops=6 不变，业务红线）。
func TestBoardInvestigationResearch_FourHypothesesLoopBudget(t *testing.T) {
	hyps := append(researchHypotheses(), boardHypothesis{
		ID: "h3", Label: "供应链交付周期同步带动", IsNull: false,
		SupportNeeded: []string{"交付周期数据同步"}, DisconfirmNeeded: []string{"交付周期与变化不重合"}, Scope: "交付周期",
	})
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证 h1"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证 h2"}, ResearchPurposeCounter, "h2")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证 h3"}, ResearchPurposeCounter, "h3")},
		{content: researchFinishDecision()},
	}}
	orch := researchOrch(router, researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{}))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(hyps))
	if err != nil {
		t.Fatalf("runBoardInvestigationResearch: %v", err)
	}
	if len(router.calls) != 5 || res.Loop.Loops != 5 {
		t.Fatalf("expected 5 rounds (1 neutral + 3 counter + finish), got calls=%d loops=%d", len(router.calls), res.Loop.Loops)
	}
	if res.Loop.FinalData == "" {
		t.Fatal("finish should be accepted within budget")
	}
	if got := strings.Join(res.Coverage.CounterAttemptedByHypothesis, ","); got != "h1,h2,h3" {
		t.Fatalf("coverage: got %q, want h1,h2,h3", got)
	}
	for _, g := range res.Gaps {
		switch g.Reason {
		case researchGapMissingNeutral, researchGapMissingCounter, researchGapMaxLoops, researchGapLLMError:
			t.Fatalf("unexpected discipline gap %+v", g)
		}
	}
}

// ── 显式 AllowedTools 缺外部工具 → 预置 tool_unavailable gap（review 4.4）──

func TestBoardInvestigationResearch_MissingExternalToolsPresetGap(t *testing.T) {
	// ① 显式缺 fetch_page：即使 agent 从未调用也预置一条 gap；finish 纪律不受影响。
	in := researchTestInput(researchHypotheses())
	in.AllowedTools = []string{"get_lane_detail", "web_search"}
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证1"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证2"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	res, err := researchOrch(router, researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{})).
		RunBoardInvestigationResearch(context.Background(), in)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Loop.FinalData == "" {
		t.Fatal("preset gap must not gate finish")
	}
	if len(res.Gaps) != 1 || res.Gaps[0].Reason != researchGapToolUnavailable || res.Gaps[0].Tool != "fetch_page" || res.Gaps[0].Step != 0 {
		t.Fatalf("want exactly one preset fetch_page gap, got %+v", res.Gaps)
	}

	// ② 双缺（只留内部工具）：全程不碰外部工具也各预置一条，去重每工具一条。
	in2 := researchTestInput(researchHypotheses())
	in2.AllowedTools = []string{"get_lane_detail"}
	router2 := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 2}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1, "window_days": 7}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	res2, err := researchOrch(router2, researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{})).
		RunBoardInvestigationResearch(context.Background(), in2)
	if err != nil {
		t.Fatalf("run2: %v", err)
	}
	byTool := map[string]int{}
	for _, g := range res2.Gaps {
		if g.Reason != researchGapToolUnavailable {
			t.Fatalf("run2 want only preset gaps, got %+v", g)
		}
		byTool[g.Tool]++
	}
	if byTool["web_search"] != 1 || byTool["fetch_page"] != 1 || len(res2.Gaps) != 2 {
		t.Fatalf("run2 presets should be one per tool, got %v (all=%+v)", byTool, res2.Gaps)
	}
	if res2.Loop.FinalData == "" || !res2.Coverage.NeutralAttempted {
		t.Fatalf("run2 finish discipline unaffected: %+v", res2.Coverage)
	}
}

// ── lane 白名单从父简报推导（review 4.4）：输入只能收窄/确认，同源入 prompt 与 guard ──

func TestBoardInvestigationResearch_LaneWhitelistDerivedFromBrief(t *testing.T) {
	// 纯函数：lane_refs/observations/relationships/research_questions 全部入集，
	// 去重升序；输入只能收窄；父简报为 nil → 空。
	b := investigationTestBrief() // lanes 1,2
	b.ResearchQuestions = append(b.ResearchQuestions, boardBriefQuestion{ID: "q2", Question: "q", RelatedLaneIDs: []uint{3}})
	b.Relationships = append(b.Relationships, boardBriefRelationship{LaneIDs: []uint{4, 1}, Type: RelationContextOnly, Confidence: "low"})
	if got := deriveLaneWhitelistFromBrief(b); !equalUintSlice(got, []uint{1, 2, 3, 4}) {
		t.Fatalf("deriveLaneWhitelistFromBrief: got %v, want [1 2 3 4]", got)
	}
	if got := effectiveInvestigationLaneWhitelist(b, []uint{4, 7, 2, 2}); !equalUintSlice(got, []uint{2, 4}) {
		t.Fatalf("narrow/confirm: got %v, want [2 4]（7 在父简报外被剔）", got)
	}
	if got := effectiveInvestigationLaneWhitelist(b, nil); !equalUintSlice(got, []uint{1, 2, 3, 4}) {
		t.Fatalf("empty input should auto-use brief set: got %v", got)
	}
	if got := effectiveInvestigationLaneWhitelist(nil, []uint{1, 2}); len(got) != 0 {
		t.Fatalf("nil brief must yield empty whitelist（幽灵进不来）: got %v", got)
	}

	// 运行时：输入含父简报外 lane 7 → 收窄到 {1,2}，7 被当幽灵拦下，
	// prompt 白名单与 guard 同源（不含 #7）。
	in := researchTestInput(researchHypotheses())
	in.LaneWhitelist = []uint{1, 2, 7}
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 7}, ResearchPurposeNeutral)}, // 幽灵 → 拦
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证1"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证2"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	lane := &researchLaneRenderer{}
	res, err := researchOrch(router, researchRegistry(lane, &researchWebSearcher{})).
		RunBoardInvestigationResearch(context.Background(), in)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if tc := res.Loop.ToolCalls[0]; tc.BlockedReason != "ghost_lane" || tc.Outcome != toolCallOutcomeBlocked {
		t.Fatalf("caller lane outside brief must be ghost-blocked: %+v", tc)
	}
	systemPrompt := router.calls[0].Messages[0].Content
	if !strings.Contains(systemPrompt, "泳道白名单（get_lane_detail 只允许这些 lane_id）：#1、#2") {
		t.Fatalf("prompt whitelist should render the effective set: %s", systemPrompt)
	}
	if strings.Contains(systemPrompt, "#7") {
		t.Fatal("prompt whitelist must not contain lanes outside the parent brief")
	}

	// 输入空 + 父简报有 lane → 自动使用父简报集合（lane 1 可执行）。
	in2 := researchTestInput(researchHypotheses())
	in2.LaneWhitelist = nil
	router2 := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证1"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证2"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	lane2 := &researchLaneRenderer{}
	res2, err := researchOrch(router2, researchRegistry(lane2, &researchWebSearcher{})).
		RunBoardInvestigationResearch(context.Background(), in2)
	if err != nil {
		t.Fatalf("run2: %v", err)
	}
	if res2.Loop.ToolCalls[0].Outcome != toolCallOutcomeOK || len(lane2.calls) != 1 || lane2.calls[0] != 1 {
		t.Fatalf("auto whitelist should allow brief lanes: calls=%v tc=%+v", lane2.calls, res2.Loop.ToolCalls[0])
	}

	// 输入 {2} 只收窄：父简报内的 lane 1 也被拦。
	in3 := researchTestInput(researchHypotheses())
	in3.LaneWhitelist = []uint{2}
	router3 := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)}, // 被收窄拦
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 2}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证1"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证2"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	lane3 := &researchLaneRenderer{}
	res3, err := researchOrch(router3, researchRegistry(lane3, &researchWebSearcher{})).
		RunBoardInvestigationResearch(context.Background(), in3)
	if err != nil {
		t.Fatalf("run3: %v", err)
	}
	if tc := res3.Loop.ToolCalls[0]; tc.BlockedReason != "ghost_lane" {
		t.Fatalf("narrowed whitelist should block lane 1: %+v", tc)
	}
	systemPrompt3 := router3.calls[0].Messages[0].Content
	if !strings.Contains(systemPrompt3, "泳道白名单（get_lane_detail 只允许这些 lane_id）：#2") {
		t.Fatalf("prompt should render the narrowed set: %s", systemPrompt3)
	}
}

func equalUintSlice(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── lane_id 非数字类型：反馈指类型错误，不误称合法数字 id 不存在（review 4.4）──

func TestBoardInvestigationResearch_LaneIDTypeErrorFeedback(t *testing.T) {
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": "1"}, ResearchPurposeNeutral)}, // 字符串类型 → 拦
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证1"}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证2"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	lane := &researchLaneRenderer{}
	orch := researchOrch(router, researchRegistry(lane, &researchWebSearcher{}))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("runBoardInvestigationResearch: %v", err)
	}
	tc := res.Loop.ToolCalls[0]
	if tc.BlockedReason != "ghost_lane" || tc.Outcome != toolCallOutcomeBlocked {
		t.Fatalf("string lane_id should be blocked: %+v", tc)
	}
	// 反馈说「必须是白名单内数字」，而非误称 lane_id=1 不在白名单内（1 明明在）。
	if !strings.Contains(tc.ResultFull, "必须是白名单内的数字编号") {
		t.Fatalf("type-error feedback should name the numeric requirement: %s", tc.ResultFull)
	}
	if strings.Contains(tc.ResultFull, "不在本次调查的泳道白名单内") {
		t.Fatalf("type error must not be reported as whitelist miss: %s", tc.ResultFull)
	}
	if len(lane.calls) != 1 || lane.calls[0] != 1 {
		t.Fatalf("renderer must only see the numeric call: %v", lane.calls)
	}
}

// ── invalid action → llm_error gap（非 max_loops，review 4.4）──────────────

func TestBoardInvestigationResearch_InvalidActionGapIsLLMError(t *testing.T) {
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: `{"action":"explode","thought":"非法动作"}`},
	}}
	orch := researchOrch(router, researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{}))

	res, err := orch.RunBoardInvestigationResearch(context.Background(), researchTestInput(researchHypotheses()))
	if err != nil {
		t.Fatalf("invalid action must return partial, not error: %v", err)
	}
	if res.Loop.FinalData != "" || !strings.Contains(res.Loop.Error, "action 不合法") {
		t.Fatalf("loop should record the invalid-action error: %+v", res.Loop)
	}
	reasons := map[string]bool{}
	for _, g := range res.Gaps {
		reasons[g.Reason] = true
	}
	if !reasons[researchGapLLMError] {
		t.Fatalf("gaps should include llm_error: %+v", res.Gaps)
	}
	if reasons[researchGapMaxLoops] {
		t.Fatalf("invalid action is an LLM output problem, not max_loops: %+v", res.Gaps)
	}
}

// ── 工具 Execute 错误文本含引号/换行 → ResultFull 仍可解析（review 4.4）───

// boomTestTool returns an Execute error whose message carries quotes/newlines/
// backslashes — exactly the payload that used to break the hand-rolled JSON.
func boomTestTool() *Tool {
	return &Tool{
		Name:        "boom",
		Description: "测试工具：总是失败且错误文本含引号/换行",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Execute: func(_ context.Context, _ map[string]any) (string, error) {
			return "", errors.New("执行失败: \"引号\"\n换行与\\反斜杠")
		},
	}
}

// policy=nil 旧路径：错误 ResultFull 必须是合法 JSON，且旧记录形状不变。
func TestRunToolLoop_ExecuteErrorLegalJSON(t *testing.T) {
	registry := NewRegistry(&nilFetcherHTTP{})
	registry.tools["boom"] = boomTestTool()
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: `{"action":"call_tool","thought":"触发错误","tool":"boom","args":{}}`},
		{content: `{"action":"finish","thought":"done","summary":"完成"}`},
	}}

	loop, err := runToolLoop(context.Background(), router, registry, internalTestCap, toolLoopParams{
		sessionID:    "legacy-sess",
		systemPrompt: "s",
		taskLine:     "l",
		operation:    "data_enrichment.tool_use",
		allowedTools: []string{"boom"},
		maxLoops:     maxAgentLoops,
	})
	if err != nil {
		t.Fatalf("runToolLoop: %v", err)
	}
	tc := loop.ToolCalls[0]
	var m map[string]any
	if jErr := json.Unmarshal([]byte(tc.ResultFull), &m); jErr != nil {
		t.Fatalf("ResultFull must be legal JSON even with quotes/newlines: %v (raw=%s)", jErr, tc.ResultFull)
	}
	if msg, _ := m["error"].(string); !strings.Contains(msg, "引号") || !strings.Contains(msg, "换行") {
		t.Fatalf("error text should survive escaping intact: %q", msg)
	}
	// policy=nil：旧记录不带结构化注记。
	if tc.Purpose != "" || tc.Outcome != "" || tc.BlockedReason != "" || tc.HypothesisIDs != nil {
		t.Fatalf("legacy record must not carry policy annotations: %+v", tc)
	}
	if loop.FinalData != "完成" {
		t.Fatalf("finish should still work: %q", loop.FinalData)
	}
}

// investigation 路径：同样可解析 → outcome=error + tool_error gap 不泄漏内部细节。
func TestBoardInvestigationResearch_ExecuteErrorOutcomeAndGap(t *testing.T) {
	registry := researchRegistry(&researchLaneRenderer{}, &researchWebSearcher{})
	registry.tools["boom"] = boomTestTool()
	in := researchTestInput(researchHypotheses())
	in.AllowedTools = []string{"get_lane_detail", "web_search", "fetch_page", "boom"}
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: researchCallDecision("get_lane_detail", map[string]any{"lane_id": 1}, ResearchPurposeNeutral)},
		{content: researchCallDecision("boom", map[string]any{}, ResearchPurposeCounter, "h1")},
		{content: researchCallDecision("web_search", map[string]any{"query": "反证2"}, ResearchPurposeCounter, "h2")},
		{content: researchFinishDecision()},
	}}
	orch := researchOrch(router, registry)

	res, err := orch.RunBoardInvestigationResearch(context.Background(), in)
	if err != nil {
		t.Fatalf("runBoardInvestigationResearch: %v", err)
	}
	boom := res.Loop.ToolCalls[1]
	if boom.Outcome != toolCallOutcomeError {
		t.Fatalf("legal error JSON should stamp outcome=error: %+v", boom)
	}
	var m map[string]any
	if jErr := json.Unmarshal([]byte(boom.ResultFull), &m); jErr != nil {
		t.Fatalf("investigation ResultFull must parse: %v (raw=%s)", jErr, boom.ResultFull)
	}
	found := false
	for _, g := range res.Gaps {
		b, _ := json.Marshal(g)
		if strings.Contains(string(b), "引号") || strings.Contains(string(b), "换行") {
			t.Fatalf("gap must not carry raw error text: %s", b)
		}
		if g.Reason == researchGapToolError && g.Tool == "boom" && g.Step == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("gaps should include tool_error for boom at step 2: %+v", res.Gaps)
	}
	// 外部工具都在 AllowedTools 里 → 无预置 gap；纪律齐备完成。
	if res.Loop.FinalData == "" || !res.Coverage.NeutralAttempted {
		t.Fatalf("run should finish: %+v", res.Coverage)
	}
}

// ── 旧 runToolLoop policy=nil 回归：行为与记录形状不变 ───────────────────────

func TestRunToolLoop_NilPolicyLegacyRegression(t *testing.T) {
	router := &researchMockRouter{steps: []researchRouterStep{
		{content: `{"action":"call_tool","thought":"禁用工具","tool":"list_etf_by_keyword","args":{"keyword":"x"}}`},
		{content: `{"action":"call_tool","thought":"搜索","tool":"web_search","args":{"query":"q1"}}`},
		{content: `{"action":"call_tool","thought":"重复搜索","tool":"web_search","args":{"query":"q1"}}`},
		{content: `{"action":"finish","thought":"done","summary":"完成"}`},
	}}
	// 无 WithWebSearcher → Noop 降级错误 JSON（执行了但返回错误）。
	registry := NewRegistry(&nilFetcherHTTP{})

	loop, err := runToolLoop(context.Background(), router, registry, internalTestCap, toolLoopParams{
		sessionID:    "legacy-sess",
		systemPrompt: "s",
		taskLine:     "l",
		operation:    "data_enrichment.tool_use",
		allowedTools: explorationToolNames,
		maxLoops:     maxAgentLoops,
	})
	if err != nil {
		t.Fatalf("runToolLoop: %v", err)
	}

	if len(loop.ToolCalls) != 3 {
		t.Fatalf("tool calls: got %d, want 3", len(loop.ToolCalls))
	}
	// 拦截标记仍走 Thought 后缀。
	if !strings.Contains(loop.ToolCalls[0].Thought, "被拦:工具不可用") {
		t.Fatalf("legacy guard marker missing: %s", loop.ToolCalls[0].Thought)
	}
	if !strings.Contains(loop.ToolCalls[2].Thought, "被拦:重复") {
		t.Fatalf("legacy dedup marker missing: %s", loop.ToolCalls[2].Thought)
	}
	// 结构化新字段不出现（零值）。
	for i, tc := range loop.ToolCalls {
		if tc.Purpose != "" || tc.Outcome != "" || tc.BlockedReason != "" || tc.HypothesisIDs != nil {
			t.Fatalf("legacy record %d must not carry policy annotations: %+v", i, tc)
		}
	}
	// 执行记录 ResultFull 完整（Noop 错误 JSON）。
	if !strings.Contains(loop.ToolCalls[1].ResultFull, "not configured") {
		t.Fatalf("executed call result: %s", loop.ToolCalls[1].ResultFull)
	}
	if loop.FinalData != "完成" {
		t.Fatalf("finish summary: %q", loop.FinalData)
	}
	// 原始 JSON 序列化不含新键（老消费者形状稳定）。
	b, _ := json.Marshal(loop.ToolCalls[0])
	for _, banned := range []string{"purpose", "outcome", "blocked_reason", "hypothesis_ids"} {
		if strings.Contains(string(b), banned) {
			t.Fatalf("legacy record JSON must not contain %q: %s", banned, b)
		}
	}
}
