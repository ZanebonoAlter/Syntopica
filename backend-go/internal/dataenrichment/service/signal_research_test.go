package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
)

// 研究闭环单元测试（board-signal-reports tasks 3.3/3.4，LP-1..LP-7 + OB-1/2 +
// NR-1/2）。全部 stub（LLM 路由 / 四源工具 / store）：不起真模型、不连外网、
// 无 DB。断言锚点：
//   - 预算与收束分类（finished / budget_exhausted / failed 三分，不伪装）；
//   - 每次执行必带非空 question；非法/重复动作不执行但计轮；
//   - cutoff 过滤先于 agent（agent 历史看不到晚于 cutoff 的观测）；
//   - 附录载筛选全集（不截 50 点）、工具日志载完整原响应；
//   - allowedTools：仅新 loop 四源，旧 explorationToolNames 零变化；
//   - 三防御继承：/no_think、ResultFull 完整、dedupKeyFor 重复拦截；
//   - NR：新链只产生 signal_research/signal_compose 两个 operation，store 面
//     上没有 review/lifeline 写路径（编译级）。

// ── stub 路由：按 operation 分流（默认分支直接失败 → NR 断言机械生效）────────

type srComposeReply struct {
	content string
	err     error
}

type srStubRouter struct {
	decisions []string         // signal_research 决策 JSON（按轮消费）
	errAt     map[int]error    // 第 N 次决策调用（1 基）强制报错
	compose   []srComposeReply // signal_compose 回复（按次消费）

	researchCalls []airouter.ChatRequest
	composeCalls  []airouter.ChatRequest
}

func (r *srStubRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch req.Operation {
	case signalResearchOperation:
		r.researchCalls = append(r.researchCalls, req)
		n := len(r.researchCalls)
		if n > len(r.decisions) {
			return nil, fmt.Errorf("srStubRouter: unexpected decision call #%d", n)
		}
		if err := r.errAt[n]; err != nil {
			return nil, err
		}
		return &airouter.ChatResult{Content: r.decisions[n-1]}, nil
	case signalComposeOperation:
		r.composeCalls = append(r.composeCalls, req)
		n := len(r.composeCalls)
		if n > len(r.compose) {
			return nil, fmt.Errorf("srStubRouter: unexpected compose call #%d", n)
		}
		rep := r.compose[n-1]
		if rep.err != nil {
			return nil, rep.err
		}
		return &airouter.ChatResult{Content: rep.content}, nil
	default:
		return nil, fmt.Errorf("srStubRouter: unexpected operation %q（新链不得产生其他 operation）", req.Operation)
	}
}

// ── stub store：仅研究需要的三个方法，无 review/lifeline 面（NR-2）────────────

type srStubStore struct {
	candidate *repository.BoardSignalCandidate
	discovery *repository.BoardSignalDiscovery

	getCandidateErr error
	getDiscoveryErr error
	createErr       error

	results []*repository.TopicEnrichmentResult
}

func (s *srStubStore) GetSignalCandidateByID(ctx context.Context, id uint) (*repository.BoardSignalCandidate, error) {
	if s.getCandidateErr != nil {
		return nil, s.getCandidateErr
	}
	if s.candidate == nil || s.candidate.ID != id {
		return nil, fmt.Errorf("candidate %d not found", id)
	}
	return s.candidate, nil
}

func (s *srStubStore) GetSignalDiscoveryByID(ctx context.Context, id uint) (*repository.BoardSignalDiscovery, error) {
	if s.getDiscoveryErr != nil {
		return nil, s.getDiscoveryErr
	}
	if s.discovery == nil || s.discovery.ID != id {
		return nil, fmt.Errorf("discovery %d not found", id)
	}
	return s.discovery, nil
}

func (s *srStubStore) CreateTopicEnrichmentResult(ctx context.Context, result *repository.TopicEnrichmentResult) error {
	if s.createErr != nil {
		return s.createErr
	}
	result.ID = uint(len(s.results) + 100)
	s.results = append(s.results, result)
	return nil
}

// ── stub 四源工具（直接构造 Registry，不注册探索工具）─────────────────────────

type srToolReply struct {
	body string
	err  error
}

type srToolStub struct {
	replies []srToolReply
	calls   []map[string]any
}

func (s *srToolStub) execute(ctx context.Context, args map[string]any) (string, error) {
	s.calls = append(s.calls, args)
	n := len(s.calls)
	if n > len(s.replies) {
		return "", fmt.Errorf("srToolStub: unexpected execute #%d", n)
	}
	rep := s.replies[n-1]
	if rep.err != nil {
		return "", rep.err
	}
	return rep.body, nil
}

func newSRRegistry(tools map[string]*srToolStub) *Registry {
	reg := &Registry{tools: map[string]*Tool{}}
	for name, stub := range tools {
		reg.tools[name] = &Tool{Name: name, Execute: stub.execute}
	}
	return reg
}

// ── fixtures ─────────────────────────────────────────────────────────────────

var srCutoff = time.Date(2026, 9, 10, 16, 0, 0, 0, time.FixedZone("CST", 8*3600))

func srFixtureMaterial() *SignalMaterial {
	return &SignalMaterial{
		Granularity:  repository.SignalGranularityMonth,
		Period:       "2026-08",
		AnalysisMode: repository.SignalAnalysisModeCurrent,
		Cutoff:       srCutoff,
		Lanes: []SignalLaneMaterial{{
			LaneID: 7, Label: "原油", Status: "active",
			Articles: []SignalArticleSlice{{SectionID: 31, ClusterLabel: "炼厂开工异动", ArticleCount: 5}},
		}},
	}
}

func srFixturePair(candidateID uint) (*repository.BoardSignalCandidate, *repository.BoardSignalDiscovery) {
	refs, _ := json.Marshal([]string{"31"})
	candidate := &repository.BoardSignalCandidate{
		ID: candidateID, DiscoveryID: 9, SemanticBoardID: 5,
		Granularity: repository.SignalGranularityMonth, Period: "2026-08",
		Signal: "炼厂开工异动", WhyItMatters: "它会改变供需判断",
		ResearchQuestion: "炼厂行为是否正在改变供需格局",
		EvidenceRefs:     refs, Score: 8, Rationale: "异动强度高",
	}
	materialJSON, _ := json.Marshal(srFixtureMaterial())
	discovery := &repository.BoardSignalDiscovery{
		ID: 9, SemanticBoardID: 5, Granularity: "month", Period: "2026-08",
		AnalysisMode: repository.SignalAnalysisModeCurrent, Cutoff: srCutoff,
		InputSnapshot: materialJSON,
	}
	return candidate, discovery
}

func srNewService(router AirRouter, reg *Registry, store *srStubStore) *SignalResearchService {
	return NewSignalResearchService(router, "data_enrichment_analysis", reg, store)
}

// srDecision 构造一轮决策 JSON。
func srDecision(action string, extra map[string]any) string {
	m := map[string]any{"action": action, "thought": "t"}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// srEiaBody 构造 eia_wpsr_table1 的完整原响应（period=周结日）。
func srEiaBody(observations ...map[string]any) string {
	periods := make([]string, 0, len(observations))
	for _, o := range observations {
		periods = append(periods, o["period"].(string))
	}
	b, _ := json.Marshal(map[string]any{
		"source": "EIA WPSR", "retrieved_at": "2026-09-22T02:00:00Z",
		"last_modified": "2026-09-16T12:00:00Z", "source_sha256": "abc123",
		"observations": observations, "periods": periods,
	})
	return string(b)
}

func srEiaObs(period string, value any) map[string]any {
	return map[string]any{
		"geo": "US", "product": "Crude Oil", "flow": "stocks",
		"label": "U.S. Commercial Crude Oil", "frequency": "weekly",
		"unit": "MMbbl", "period": period, "value": value, "raw_value": "x",
	}
}

// srComposeOK 构造一稿合格的 compose 回复：四段齐 + implication 结构化 +
// 时效声明 + 只引用给定的观测 id（无图）。期值 2026-08-01 的月粒度必须与
// 各自 fixture 账本的最新观测月一致（review M1 后校验会比对机械注入的
// 「数据可用性」行；月不对就改 fixture 观测期，不许放宽校验——零观测账本
// 用 srComposeOKNoData）。
func srComposeOK(refs ...string) string {
	return srComposeReplyWithAsOf("数据截至 2026-08-01。", refs...)
}

// srComposeOKNoData 是零观测账本唯一合法的时效写法（design §10.3）。
func srComposeOKNoData(refs ...string) string {
	return srComposeReplyWithAsOf("数据截至：无可用数据期。", refs...)
}

func srComposeReplyWithAsOf(factsAsOf string, refs ...string) string {
	token := func(ref string) string {
		if strings.HasPrefix(ref, "k") {
			return "[[calc:" + ref + "]]"
		}
		return "[[data:" + ref + "]]"
	}
	facts := "发生了什么，见引用。" + factsAsOf
	for _, r := range refs {
		facts += token(r)
	}
	thesis := "先说结论。"
	if len(refs) > 0 {
		thesis += "依据见 " + token(refs[0]) + "。"
	}
	b, _ := json.Marshal(map[string]any{
		"title": "库存变化还不能说明需求转向",
		"sections": []map[string]any{
			{"kind": "thesis", "text": thesis},
			{"kind": "facts", "text": facts},
			{"kind": "causal", "text": "这意味着什么：有另一种解释不能排除。"},
			{"kind": "implication", "text": "接下来怎么看，先维持判断。", "verdict": "维持原判断",
				"direction": "conditional", "horizon": "一个月", "trigger_condition": "若后续两周数据反向则修正", "self_doubt": "只看两周可能把偶发当趋势"},
		},
		"charts": []any{},
	})
	return string(b)
}

// runResearch 执行一次研究并解析落库 payload。
func runResearch(t *testing.T, svc *SignalResearchService, store *srStubStore, candidateID uint) (uint, *SignalReportPayload, []string) {
	t.Helper()
	var stages []string
	resultID, err := svc.ResearchCandidate(t.Context(), 5, candidateID, "", func(stage string) { stages = append(stages, stage) })
	if err != nil {
		t.Fatalf("ResearchCandidate: %v", err)
	}
	if len(store.results) != 1 {
		t.Fatalf("expected exactly one saved result, got %d", len(store.results))
	}
	var payload SignalReportPayload
	if err := json.Unmarshal(store.results[0].Sectors, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return resultID, &payload, stages
}

// signalToolCallRecords 解码落库行的 tool_calls（json.RawMessage → 记录切片）。
func signalToolCallRecords(t *testing.T, raw json.RawMessage) []ToolCallRecord {
	t.Helper()
	var records []ToolCallRecord
	if len(raw) == 0 {
		return records
	}
	if err := json.Unmarshal(raw, &records); err != nil {
		t.Fatalf("unmarshal tool calls: %v", err)
	}
	return records
}

// ── LP-1：第 17 轮 finish 正常收束（不再被旧 4/8 轮截停）──────────────────────

func TestSignalResearch_FinishAtSeventeenRounds(t *testing.T) {
	router := &srStubRouter{compose: []srComposeReply{{content: srComposeOK("c1:o1")}}}
	for i := 0; i < 16; i++ {
		router.decisions = append(router.decisions, srDecision("call_tool", map[string]any{
			"tool": "eia_wpsr_table1", "args": map[string]any{"section": fmt.Sprintf("s%d", i)},
			"question": fmt.Sprintf("问题%d", i),
		}))
	}
	router.decisions = append(router.decisions, srDecision("finish", map[string]any{"summary": "研究笔记：关键发现。"}))
	tool := &srToolStub{replies: []srToolReply{{body: srEiaBody(srEiaObs("2026-08-01", 420.0))}}}
	for i := 1; i < 16; i++ {
		tool.replies = append(tool.replies, srToolReply{body: srEiaBody(srEiaObs("2026-08-01", 420.0+float64(i)))})
	}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	resultID, payload, stages := runResearch(t, svc, store, 7)
	if resultID == 0 || len(stages) != 2 || stages[0] != "research" || stages[1] != "compose" {
		t.Fatalf("resultID=%d stages=%v", resultID, stages)
	}
	if len(router.researchCalls) != 17 || len(router.composeCalls) != 1 {
		t.Fatalf("research chats=%d compose chats=%d", len(router.researchCalls), len(router.composeCalls))
	}
	meta := payload.GenerationMeta
	if meta.StopReason != SignalStopReasonFinished || meta.Decisions != 17 ||
		meta.SourceCalls != 16 || meta.CalculationCalls != 0 || meta.Attempts != 1 || meta.Retries != 0 {
		t.Fatalf("generation_meta: %+v", meta)
	}
	if len(tool.calls) != 16 {
		t.Fatalf("tool executions=%d", len(tool.calls))
	}
	if payload.SchemaVersion != 2 || payload.Report.Title == "" || len(payload.Report.Sections) != 4 {
		t.Fatalf("payload shape: %+v", payload.Report)
	}
	// 独立 session + operation 区分。
	re := regexp.MustCompile(`^board_signal_report_7_[0-9a-f]{8}$`)
	for _, req := range router.researchCalls {
		if !re.MatchString(req.SessionID) {
			t.Fatalf("session id %q", req.SessionID)
		}
		if req.Operation != signalResearchOperation {
			t.Fatalf("operation %q", req.Operation)
		}
	}
	if router.composeCalls[0].SessionID != router.researchCalls[0].SessionID {
		t.Fatalf("compose must share the research session")
	}
	if payload.GenerationMeta.SessionID != router.researchCalls[0].SessionID {
		t.Fatalf("generation_meta session mismatch")
	}
	// 落库行形状：board scope + 周期 + 候选指向。
	row := store.results[0]
	if row.ResultKind != repository.ResultKindSignalReport || row.AnalysisScope != "board" ||
		row.SemanticBoardID == nil || *row.SemanticBoardID != 5 || row.PersistentTopicID != nil ||
		row.Granularity == nil || *row.Granularity != "month" || *row.Period != "2026-08" ||
		row.SourceSignalID == nil || *row.SourceSignalID != 7 {
		t.Fatalf("result row shape: %+v", row)
	}
}

// ── LP-2：39 可续 / 40 收束 / 41 不执行 ──────────────────────────────────────

func srCallDecision(i int) string {
	return srDecision("call_tool", map[string]any{
		"tool": "eia_wpsr_table1", "args": map[string]any{"section": fmt.Sprintf("s%d", i)},
		"question": fmt.Sprintf("问题%d", i),
	})
}

func srFillToolReplies(tool *srToolStub, n int) {
	for len(tool.replies) < n {
		// 期值向 srComposeOK 的 2026-08-01 对齐（月粒度）。
		tool.replies = append(tool.replies, srToolReply{body: srEiaBody(srEiaObs("2026-08-01", 420.0))})
	}
}

func TestSignalResearch_ThirtyNineRoundsThenFinishStillFinished(t *testing.T) {
	router := &srStubRouter{compose: []srComposeReply{{content: srComposeOK("c1:o1")}}}
	for i := 0; i < 39; i++ {
		router.decisions = append(router.decisions, srCallDecision(i))
	}
	router.decisions = append(router.decisions, srDecision("finish", map[string]any{"summary": "done"}))
	tool := &srToolStub{}
	srFillToolReplies(tool, 39)
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, payload, _ := runResearch(t, svc, store, 7)
	if payload.GenerationMeta.StopReason != SignalStopReasonFinished || payload.GenerationMeta.Decisions != 40 {
		t.Fatalf("39 calls + finish: stop=%s decisions=%d", payload.GenerationMeta.StopReason, payload.GenerationMeta.Decisions)
	}
}

func TestSignalResearch_FortyRoundsBudgetExhaustedNotFailure(t *testing.T) {
	router := &srStubRouter{compose: []srComposeReply{{content: srComposeOK()}}}
	for i := 0; i < 40; i++ {
		router.decisions = append(router.decisions, srCallDecision(i))
	}
	tool := &srToolStub{}
	srFillToolReplies(tool, 40)
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, payload, _ := runResearch(t, svc, store, 7)
	meta := payload.GenerationMeta
	if meta.StopReason != SignalStopReasonBudgetExhausted || meta.Decisions != 40 || meta.SourceCalls != 40 {
		t.Fatalf("budget exhausted meta: %+v", meta)
	}
	if len(router.researchCalls) != 40 {
		t.Fatalf("research chats=%d（第 41 轮不得发生）", len(router.researchCalls))
	}
	// 预算耗尽仍以已有证据成文：compose 恰好一次、result 落库。
	if len(router.composeCalls) != 1 || len(store.results) != 1 {
		t.Fatalf("compose=%d results=%d", len(router.composeCalls), len(store.results))
	}
	// 收束状态传给 compose（正文须披露缺口）。
	if !strings.Contains(router.composeCalls[0].Messages[1].Content, SignalStopReasonBudgetExhausted) {
		t.Fatalf("compose prompt must disclose budget exhaustion")
	}
}

// ── LP-3：20 取数 + 20 计算 = 总 40 执行，计算无独立额外预算 ──────────────────

func TestSignalResearch_MixedExecutionsShareBudget(t *testing.T) {
	router := &srStubRouter{compose: []srComposeReply{{content: srComposeOK()}}}
	tool := &srToolStub{}
	srFillToolReplies(tool, 20)
	for i := 0; i < 20; i++ {
		router.decisions = append(router.decisions, srCallDecision(i))
		// 每个计算请求输入不同 → 不构成 duplicate_calculation。
		router.decisions = append(router.decisions, srDecision("calculate", map[string]any{
			"op": "difference", "inputs": []string{"c1:o1", fmt.Sprintf("c%d:o1", i+1)}, "question": "q",
		}))
	}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, payload, _ := runResearch(t, svc, store, 7)
	meta := payload.GenerationMeta
	if meta.StopReason != SignalStopReasonBudgetExhausted || meta.Decisions != 40 ||
		meta.SourceCalls != 20 || meta.CalculationCalls != 20 {
		t.Fatalf("mixed budget meta: %+v", meta)
	}
	if len(router.researchCalls) != 40 || len(payload.Appendix.Calculations) != 20 {
		t.Fatalf("chats=%d calcs=%d", len(router.researchCalls), len(payload.Appendix.Calculations))
	}
}

// ── LP-4：非法动作连续 40 轮：不执行、计轮、有限结束 ──────────────────────────

func TestSignalResearch_InvalidActionsBurnRoundsButTerminate(t *testing.T) {
	router := &srStubRouter{compose: []srComposeReply{{content: srComposeOKNoData()}}}
	for i := 0; i < 40; i++ {
		router.decisions = append(router.decisions, srDecision("dance", nil))
	}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(nil), store)

	_, payload, _ := runResearch(t, svc, store, 7)
	meta := payload.GenerationMeta
	if meta.StopReason != SignalStopReasonBudgetExhausted || meta.Decisions != 40 ||
		meta.SourceCalls != 0 || meta.CalculationCalls != 0 {
		t.Fatalf("invalid actions meta: %+v", meta)
	}
	if len(router.researchCalls) != 40 {
		t.Fatalf("research chats=%d", len(router.researchCalls))
	}
	for _, rec := range signalToolCallRecords(t, store.results[0].ToolCalls) {
		if rec.Outcome != toolCallOutcomeBlocked {
			t.Fatalf("all rounds must be blocked records: %+v", rec)
		}
	}
	if len(payload.Appendix.Calls) != 0 || len(payload.Appendix.Calculations) != 0 {
		t.Fatalf("no calls/calcs expected: %+v", payload.Appendix)
	}
}

// ── LP-5：部分源失败 → 失败账本 + gap，可无数据成文 ──────────────────────────

func TestSignalResearch_SourceFailureRecordsGapAndComposes(t *testing.T) {
	router := &srStubRouter{
		decisions: []string{
			srDecision("call_tool", map[string]any{
				"tool": "eia_wpsr_table1", "args": map[string]any{"section": "stocks"}, "question": "库存是否下降",
			}),
			srDecision("finish", map[string]any{"summary": "源不可用，只有缺口。"}),
		},
		compose: []srComposeReply{{content: srComposeOKNoData()}},
	}
	tool := &srToolStub{replies: []srToolReply{{body: `{"error_code":"SOURCE_UNAVAILABLE","message":"EIA 不可达","detail":""}`}}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, payload, _ := runResearch(t, svc, store, 7)
	if payload.GenerationMeta.StopReason != SignalStopReasonFinished || payload.GenerationMeta.SourceCalls != 1 {
		t.Fatalf("meta: %+v", payload.GenerationMeta)
	}
	calls := payload.Appendix.Calls
	if len(calls) != 1 || calls[0].Status != "error" || calls[0].Error == "" {
		t.Fatalf("failed call record: %+v", calls)
	}
	// M2：类型化错误帧（error_code，无顶层 error 键）经包装层归一后，共享
	// 循环的 toolResultErrorText 才能识别——落库 tool_calls 记录的 Outcome
	// 必须记 error 而非 ok（与附录 status=error、gap 记录一致）。
	records := signalToolCallRecords(t, store.results[0].ToolCalls)
	if len(records) != 1 || records[0].Outcome != toolCallOutcomeError {
		t.Fatalf("typed error frame must stamp outcome=error on tool_calls: %+v", records)
	}
	foundGap := false
	for _, g := range payload.Appendix.Gaps {
		if g.CallID == "c1" && strings.Contains(g.Reason, "取数失败") {
			foundGap = true
		}
	}
	if !foundGap {
		t.Fatalf("gap missing: %+v", payload.Appendix.Gaps)
	}
}

// ── LP-6：取消/超时/LLM 异常 → failed，不写 result，不伪装 ────────────────────

func TestSignalResearch_ContextCancelFailsWithoutResult(t *testing.T) {
	router := &srStubRouter{decisions: []string{srCallDecision(0)}}
	tool := &srToolStub{replies: []srToolReply{{body: srEiaBody(srEiaObs("2026-07-31", 420.0))}}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	ctx, cancel := context.WithCancel(context.Background())
	toolExecutes := make(chan struct{})
	// 包装工具：真实执行后取消 context（模拟 30 分钟 job 超时强杀）。
	reg := newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool})
	wrapped := reg.tools["eia_wpsr_table1"]
	reg.tools["eia_wpsr_table1"] = &Tool{Name: "eia_wpsr_table1", Execute: func(ctx context.Context, args map[string]any) (string, error) {
		out, err := wrapped.Execute(ctx, args)
		cancel()
		close(toolExecutes)
		return out, err
	}}
	svc = srNewService(router, reg, store)

	var stages []string
	_, err := svc.ResearchCandidate(ctx, 5, 7, "", func(stage string) { stages = append(stages, stage) })
	if err == nil {
		t.Fatalf("canceled research must fail")
	}
	var stageErr *SignalStageError
	if !asSignalStageError(err, &stageErr) || stageErr.Stage != SignalStageResearch {
		t.Fatalf("stage error: %v", err)
	}
	if !strings.Contains(err.Error(), "取消或超时") {
		t.Fatalf("cancel reason must surface: %v", err)
	}
	if len(store.results) != 0 {
		t.Fatalf("canceled research must not save results")
	}
	<-toolExecutes
}

// LLM 中途异常（非取消）同样 failed，不伪装成预算收束。
func TestSignalResearch_LLMBreakIsFailedNotBudgetExhausted(t *testing.T) {
	router := &srStubRouter{
		decisions: []string{srCallDecision(0)},
		errAt:     map[int]error{2: fmt.Errorf("boom")},
	}
	tool := &srToolStub{replies: []srToolReply{{body: srEiaBody(srEiaObs("2026-07-31", 420.0))}}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, err := svc.ResearchCandidate(t.Context(), 5, 7, "", nil)
	if err == nil {
		t.Fatalf("LLM break must fail")
	}
	if strings.Contains(err.Error(), SignalStopReasonBudgetExhausted) || len(store.results) != 0 {
		t.Fatalf("failed run must not save results or fake budget stop: %v", err)
	}
}

// ── LP-7 + 三防御：question 必填、重复拦截、/no_think、ResultFull 不截断 ──────

func TestSignalResearch_QuestionRequiredDedupAndDefenses(t *testing.T) {
	// 单观测完整响应 >300 字符（防御②断言用）。
	body := srEiaBody(srEiaObs("2026-08-07", 421.5))
	if len(body) <= 300 {
		t.Fatalf("fixture body must exceed the 300-char preview cut: %d", len(body))
	}
	router := &srStubRouter{
		decisions: []string{
			// 第 1 轮：缺 question → 拦截（不执行）。
			srDecision("call_tool", map[string]any{"tool": "eia_wpsr_table1", "args": map[string]any{"section": "stocks"}}),
			// 第 2 轮：带 question → 执行。
			srDecision("call_tool", map[string]any{"tool": "eia_wpsr_table1", "args": map[string]any{"section": "stocks"}, "question": "库存是否下降"}),
			// 第 3 轮：同 tool 同 args → 循环去重拦截（不执行）。
			srDecision("call_tool", map[string]any{"tool": "eia_wpsr_table1", "args": map[string]any{"section": "stocks"}, "question": "库存是否下降"}),
			srDecision("finish", map[string]any{"summary": "done"}),
		},
		compose: []srComposeReply{{content: srComposeOK("c1:o1")}},
	}
	tool := &srToolStub{replies: []srToolReply{{body: body}}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, _, _ = runResearch(t, svc, store, 7)
	if len(tool.calls) != 1 {
		t.Fatalf("tool must execute exactly once, got %d", len(tool.calls))
	}
	if len(router.researchCalls) != 4 {
		t.Fatalf("research chats=%d（被拦也计轮）", len(router.researchCalls))
	}
	records := signalToolCallRecords(t, store.results[0].ToolCalls)
	if len(records) != 3 {
		t.Fatalf("tool call records=%d", len(records))
	}
	if records[0].Outcome != toolCallOutcomeBlocked || records[0].BlockedReason != "missing_question" {
		t.Fatalf("round1 must be blocked for missing question: %+v", records[0])
	}
	if records[1].Outcome != toolCallOutcomeOK {
		t.Fatalf("round2 must execute: %+v", records[1])
	}
	if records[2].Outcome != toolCallOutcomeBlocked || records[2].BlockedReason != "duplicate_call" {
		t.Fatalf("round3 must be dedup-blocked: %+v", records[2])
	}
	// 防御① /no_think 前缀。
	for _, req := range router.researchCalls {
		if !strings.HasPrefix(req.Messages[len(req.Messages)-1].Content, "/no_think") {
			t.Fatalf("research user message must carry /no_think prefix")
		}
	}
	// 防御② ResultFull 完整不截断（>300 字符；工具日志回写后即完整原响应）。
	if len(records[1].ResultFull) <= 300 || records[1].ResultFull != body {
		t.Fatalf("ResultFull must keep the full result: len=%d", len(records[1].ResultFull))
	}
}

// ── OB-1/2：cutoff 过滤先于 agent；完整原响应进工具日志；附录不截 50 点 ───────

func TestSignalResearch_CutoffFilterBeforeAgent(t *testing.T) {
	// cutoff=2026-09-10T16:00+08：08-07 保留，09-25（周结日晚于 cutoff）剔除。
	body := srEiaBody(srEiaObs("2026-08-07", 421.5), srEiaObs("2026-09-25", 418.0))
	router := &srStubRouter{
		decisions: []string{
			srDecision("call_tool", map[string]any{"tool": "eia_wpsr_table1", "args": map[string]any{"section": "stocks"}, "question": "库存走势"}),
			srDecision("finish", map[string]any{"summary": "done"}),
		},
		compose: []srComposeReply{{content: srComposeOK("c1:o1")}},
	}
	tool := &srToolStub{replies: []srToolReply{{body: body}}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, payload, _ := runResearch(t, svc, store, 7)

	// agent 历史（第 2 轮的 user 消息）只看得到筛选后的观测与 observation_id。
	history := router.researchCalls[1].Messages[len(router.researchCalls[1].Messages)-1].Content
	if !strings.Contains(history, "c1:o1") || !strings.Contains(history, "2026-08-07") {
		t.Fatalf("agent history must contain the kept observation with its id")
	}
	if strings.Contains(history, "2026-09-25") {
		t.Fatalf("post-cutoff observation leaked into agent history")
	}

	// 附录：完整筛选观测（kept=1）+ filter_meta。
	calls := payload.Appendix.Calls
	if len(calls) != 1 || len(calls[0].Observations) != 1 {
		t.Fatalf("appendix call observations: %+v", calls)
	}
	if calls[0].Observations[0]["observation_id"] != "c1:o1" {
		t.Fatalf("observation id stamp: %+v", calls[0].Observations[0])
	}
	if calls[0].FilterMeta == nil || calls[0].FilterMeta["kept"] != float64(1) || calls[0].FilterMeta["dropped"] != float64(1) {
		t.Fatalf("filter_meta: %+v", calls[0].FilterMeta)
	}
	if calls[0].RetrievedAt == "" || calls[0].SourceSHA256 == "" {
		t.Fatalf("provenance must be recorded: %+v", calls[0])
	}

	// 工具日志（result.tool_calls）：完整原响应（含被剔除观测）。
	records := signalToolCallRecords(t, store.results[0].ToolCalls)
	var rawRecord *ToolCallRecord
	for i := range records {
		if records[i].Tool == "eia_wpsr_table1" && records[i].Outcome == toolCallOutcomeOK {
			rawRecord = &records[i]
		}
	}
	if rawRecord == nil || !strings.Contains(rawRecord.ResultFull, "2026-09-25") {
		t.Fatalf("tool log must keep the FULL raw response")
	}
}

func TestSignalResearch_AllObservationsAfterCutoffRecordsGap(t *testing.T) {
	body := srEiaBody(srEiaObs("2026-09-25", 418.0))
	router := &srStubRouter{
		decisions: []string{
			srDecision("call_tool", map[string]any{"tool": "eia_wpsr_table1", "args": map[string]any{"section": "stocks"}, "question": "库存走势"}),
			srDecision("finish", map[string]any{"summary": "没有 cutoff 内数据。"}),
		},
		compose: []srComposeReply{{content: srComposeOKNoData()}},
	}
	tool := &srToolStub{replies: []srToolReply{{body: body}}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, payload, _ := runResearch(t, svc, store, 7)
	calls := payload.Appendix.Calls
	if len(calls) != 1 || len(calls[0].Observations) != 0 {
		t.Fatalf("all-filtered call must carry zero observations: %+v", calls)
	}
	found := false
	for _, g := range payload.Appendix.Gaps {
		if strings.Contains(g.Reason, "无覆盖") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no-coverage gap must be recorded: %+v", payload.Appendix.Gaps)
	}
}

func TestSignalResearch_SixtyObservationsNotCutToFifty(t *testing.T) {
	obs := make([]map[string]any, 0, 60)
	for i := 0; i < 60; i++ {
		period := fmt.Sprintf("2026-%02d-07", i%8+1) // 全部早于 cutoff，最新月与 srComposeOK 对齐
		obs = append(obs, srEiaObs(period, 400.0+float64(i)))
	}
	router := &srStubRouter{
		decisions: []string{
			srDecision("call_tool", map[string]any{"tool": "eia_wpsr_table1", "args": map[string]any{"section": "stocks"}, "question": "库存"}),
			srDecision("finish", map[string]any{"summary": "done"}),
		},
		compose: []srComposeReply{{content: srComposeOK("c1:o1")}},
	}
	tool := &srToolStub{replies: []srToolReply{{body: srEiaBody(obs...)}}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, payload, _ := runResearch(t, svc, store, 7)
	if len(payload.Appendix.Calls) != 1 || len(payload.Appendix.Calls[0].Observations) != 60 {
		t.Fatalf("appendix must keep all %d observations, got %d", 60, len(payload.Appendix.Calls[0].Observations))
	}
}

// ── allowedTools / 工具面不变量（HR-7 新研究侧）───────────────────────────────

func TestSignalResearch_AllowedToolsPinnedAndLegacyUnchanged(t *testing.T) {
	want := []string{"eia_wpsr_table1", "jodi_oil_primary", "wb_wdi", "un_comtrade_trade"}
	if len(signalResearchToolNames) != len(want) {
		t.Fatalf("research tool whitelist changed: %v", signalResearchToolNames)
	}
	for i, n := range want {
		if signalResearchToolNames[i] != n {
			t.Fatalf("research tool whitelist changed: %v", signalResearchToolNames)
		}
	}
	// 旧 loop：explorationToolNames 逐字不变，且不含四源。
	legacy := []string{"list_boards", "list_lanes", "get_lane_detail", "web_search", "fetch_page", "search_internal_context"}
	if len(explorationToolNames) != len(legacy) {
		t.Fatalf("explorationToolNames must stay unchanged: %v", explorationToolNames)
	}
	for i, n := range legacy {
		if explorationToolNames[i] != n {
			t.Fatalf("explorationToolNames must stay unchanged: %v", explorationToolNames)
		}
	}
	base := (&OrchestratorService{}).buildAgentAllowedTools(nil)
	for _, n := range signalResearchToolNames {
		for _, b := range base {
			if b == n {
				t.Fatalf("legacy loop must not gain research tool %s", n)
			}
		}
	}

	// 新 loop 内：非白名单工具（web_search）在 CheckCall 即拦，不执行。
	router := &srStubRouter{
		decisions: []string{
			srDecision("call_tool", map[string]any{"tool": "web_search", "args": map[string]any{"query": "x"}, "question": "q"}),
			srDecision("finish", map[string]any{"summary": "done"}),
		},
		compose: []srComposeReply{{content: srComposeOKNoData()}},
	}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(nil), store)
	_, payload, _ := runResearch(t, svc, store, 7)
	records := signalToolCallRecords(t, store.results[0].ToolCalls)
	if len(records) != 1 || records[0].Outcome != toolCallOutcomeBlocked || records[0].BlockedReason != "tool_not_in_research_whitelist" {
		t.Fatalf("web_search must be blocked: %+v", records)
	}
	if payload.GenerationMeta.SourceCalls != 0 {
		t.Fatalf("blocked calls are not executions: %+v", payload.GenerationMeta)
	}
}

// ── calculate 动作（policy 层形状校验 + 预算/账本联动）────────────────────────

func TestSignalResearch_CalculateActionValidation(t *testing.T) {
	router := &srStubRouter{
		decisions: []string{
			srDecision("call_tool", map[string]any{"tool": "eia_wpsr_table1", "args": map[string]any{"section": "s"}, "question": "q"}),
			// 缺 question → 拦截。
			srDecision("calculate", map[string]any{"op": "difference", "inputs": []string{"c1:o1", "c1:o2"}}),
			// 模型提交 value → 拦截（模型不能提交计算值）。
			srDecision("calculate", map[string]any{"op": "difference", "inputs": []string{"c1:o1", "c1:o2"}, "question": "q", "value": 3.14}),
			// 未知 op → 拦截。
			srDecision("calculate", map[string]any{"op": "eval", "inputs": []string{"c1:o1"}, "question": "q"}),
			// 空输入 → 拦截。
			srDecision("calculate", map[string]any{"op": "mean", "inputs": []string{}, "question": "q"}),
			// 合法计算 → 执行并记录。
			srDecision("calculate", map[string]any{"op": "difference", "inputs": []string{"c1:o1", "c1:o2"}, "question": "变化多少"}),
			// 相同请求重复 → 拦截。
			srDecision("calculate", map[string]any{"op": "difference", "inputs": []string{"c1:o1", "c1:o2"}, "question": "变化多少"}),
			srDecision("finish", map[string]any{"summary": "done"}),
		},
		compose: []srComposeReply{{content: srComposeOK("c1:o1", "c1:o2", "k1")}},
	}
	tool := &srToolStub{replies: []srToolReply{{body: srEiaBody(srEiaObs("2026-08-07", 420.0), srEiaObs("2026-08-14", 419.0))}}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, payload, _ := runResearch(t, svc, store, 7)
	calcs := payload.Appendix.Calculations
	if len(calcs) != 1 {
		t.Fatalf("exactly one executed calculation expected: %d", len(calcs))
	}
	if calcs[0].CalcID != "k1" || calcs[0].Status != SignalCalcStatusOK || calcs[0].Value != "1" {
		t.Fatalf("calc record: %+v", calcs[0])
	}
	meta := payload.GenerationMeta
	if meta.CalculationCalls != 1 || meta.Decisions != 8 {
		t.Fatalf("meta: %+v", meta)
	}
	// 拦截轮：记录 outcome=blocked；重复轮 reason=duplicate_calculation。
	records := signalToolCallRecords(t, store.results[0].ToolCalls)
	blockedReasons := map[string]bool{}
	for _, rec := range records {
		if rec.Outcome == toolCallOutcomeBlocked {
			blockedReasons[rec.BlockedReason] = true
		}
	}
	for _, want := range []string{"missing_question", "value_forbidden", "invalid_op", "invalid_inputs", "duplicate_calculation"} {
		if !blockedReasons[want] {
			t.Fatalf("missing blocked reason %s: %+v", want, blockedReasons)
		}
	}
}

// ── NR-1/2：新链零 judge/digest/review、零新闻污染 ───────────────────────────

func TestSignalResearch_NR_NoReviewNoNewsPollution(t *testing.T) {
	router := &srStubRouter{
		decisions: []string{
			srDecision("call_tool", map[string]any{"tool": "eia_wpsr_table1", "args": map[string]any{"section": "s"}, "question": "q"}),
			srDecision("finish", map[string]any{"summary": "done"}),
		},
		compose: []srComposeReply{{content: srComposeOK("c1:o1")}},
	}
	tool := &srToolStub{replies: []srToolReply{{body: srEiaBody(srEiaObs("2026-08-01", 420.0))}}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, payload, _ := runResearch(t, svc, store, 7)
	// NR-1：全部 LLM 调用只有两个 operation；stub 默认分支对其它 operation 直接
	// 失败——judge/digest 类调用一旦发生测试即红。
	if len(router.researchCalls) != 2 || len(router.composeCalls) != 1 {
		t.Fatalf("operations: research=%d compose=%d", len(router.researchCalls), len(router.composeCalls))
	}
	// store 面无 review 写路径（接口无该方法）；落库行无 review 键。
	raw := string(store.results[0].Sectors)
	if strings.Contains(strings.ToLower(raw), "review") || strings.Contains(strings.ToLower(raw), "judge") {
		t.Fatalf("payload must not carry review fields: %s", raw)
	}
	// NR-2：报告不污染新闻/lifeline——store 接口无任何 lifeline 写方法
	// （编译级保证）；材料快照原样透传为 input_snapshot。
	if string(store.results[0].InputSnapshot) == "" {
		t.Fatalf("input snapshot must be preserved")
	}
	_ = payload
}

// compose 失败三次 → 整个研究 failed，无 result（SV-6 的服务级联动在 compose
// 测试里细化；这里断言错误透传与 stage 归属）。
func TestSignalResearch_ComposeFailureFailsWholeRun(t *testing.T) {
	router := &srStubRouter{
		decisions: []string{srDecision("finish", map[string]any{"summary": "done"})},
		compose: []srComposeReply{
			{content: `{"title":"","sections":[],"charts":[]}`},
			{content: `{"title":"","sections":[],"charts":[]}`},
			{content: `{"title":"","sections":[],"charts":[]}`},
		},
	}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(nil), store)

	_, err := svc.ResearchCandidate(t.Context(), 5, 7, "", nil)
	if err == nil {
		t.Fatalf("compose failure must fail the run")
	}
	var stageErr *SignalStageError
	if !asSignalStageError(err, &stageErr) || stageErr.Stage != SignalStageCompose {
		t.Fatalf("stage error: %v", err)
	}
	if len(store.results) != 0 || len(router.composeCalls) != 3 {
		t.Fatalf("results=%d compose=%d", len(store.results), len(router.composeCalls))
	}
}

func asSignalStageError(err error, target **SignalStageError) bool {
	return errors.As(err, target)
}

// ── 研究进展持久化（tasks 4.7「断了不能白跑」）───────────────────────────────
//
// 锚点：每轮结束后滚动 upsert（轮次/取数计数与账本同源）；超时终态写
// abandoned+timeout 且写入 ctx 脱离已死 job ctx；成功落库报告后 superseded；
// store 未接线 / jobID 为空时零进展调用（旧路径零影响）。

type srProgressUpsert struct {
	row      repository.BoardSignalResearchProgress
	ctxAlive bool // 写入时 ctx.Err() == nil（终态写入必须为 true）
}

type srProgressStore struct {
	upserts    []srProgressUpsert
	superseded []string
	upsertErr  error
	markErr    error
}

func (s *srProgressStore) UpsertSignalResearchProgress(ctx context.Context, p *repository.BoardSignalResearchProgress) error {
	if s.upsertErr != nil {
		return s.upsertErr
	}
	s.upserts = append(s.upserts, srProgressUpsert{row: *p, ctxAlive: ctx.Err() == nil})
	return nil
}

func (s *srProgressStore) MarkSignalResearchProgressSuperseded(ctx context.Context, jobID string) error {
	if s.markErr != nil {
		return s.markErr
	}
	_ = ctx
	s.superseded = append(s.superseded, jobID)
	return nil
}

// 每轮滚动 upsert：两次取数轮各写一次（轮次/取数计数递增、账本 calls 同源
// 增长、身份字段与候选一致），成功落库报告后 superseded 恰一次。
func TestSignalResearch_ProgressPerRoundUpsertAndSupersede(t *testing.T) {
	router := &srStubRouter{
		decisions: []string{
			srDecision("call_tool", map[string]any{
				"tool": "eia_wpsr_table1", "args": map[string]any{"section": "s1"}, "question": "库存现状如何",
			}),
			srDecision("call_tool", map[string]any{
				"tool": "eia_wpsr_table1", "args": map[string]any{"section": "s2"}, "question": "趋势是否延续",
			}),
			srDecision("finish", map[string]any{"summary": "研究笔记：关键发现。"}),
		},
		compose: []srComposeReply{{content: srComposeOK("c1:o1")}},
	}
	tool := &srToolStub{replies: []srToolReply{
		{body: srEiaBody(srEiaObs("2026-07-31", 420.0))},
		{body: srEiaBody(srEiaObs("2026-08-07", 419.0))},
	}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)
	progress := &srProgressStore{}
	svc.SetSignalProgressStore(progress)

	resultID, err := svc.ResearchCandidate(t.Context(), 5, 7, "job-p1", nil)
	if err != nil {
		t.Fatalf("ResearchCandidate: %v", err)
	}
	if resultID == 0 || len(store.results) != 1 {
		t.Fatalf("research must succeed: id=%d results=%d", resultID, len(store.results))
	}
	// 每个取数轮结束后各一次 upsert（finish 是终态轮，不触发轮内保存）。
	if len(progress.upserts) != 2 {
		t.Fatalf("want 2 per-round upserts, got %d", len(progress.upserts))
	}
	for i, want := range []struct{ rounds, calls int }{{1, 1}, {2, 2}} {
		up := progress.upserts[i]
		if up.row.RoundsDone != want.rounds || up.row.SourceCalls != want.calls {
			t.Fatalf("upsert[%d]: rounds=%d calls=%d, want %d/%d", i, up.row.RoundsDone, up.row.SourceCalls, want.rounds, want.calls)
		}
		if up.row.Status != repository.SignalResearchProgressRunning {
			t.Fatalf("per-round status must be running, got %q", up.row.Status)
		}
		if up.row.JobID != "job-p1" || up.row.CandidateID != 7 || up.row.SemanticBoardID != 5 ||
			up.row.Granularity != repository.SignalGranularityMonth || up.row.Period != "2026-08" {
			t.Fatalf("upsert[%d] identity mismatch: %+v", i, up.row)
		}
		// ledger 与 appendix 同源结构：calls/calculations/gaps 三段，calls 内
		// question 已注记、观测带 observation_id。
		var ledger struct {
			Calls []struct {
				CallID       string           `json:"call_id"`
				Question     string           `json:"question"`
				Observations []map[string]any `json:"observations"`
			} `json:"calls"`
			Calculations []any `json:"calculations"`
			Gaps         []any `json:"gaps"`
		}
		if err := json.Unmarshal(up.row.Ledger, &ledger); err != nil {
			t.Fatalf("unmarshal ledger: %v", err)
		}
		if len(ledger.Calls) != want.calls || ledger.Calculations == nil || ledger.Gaps == nil {
			t.Fatalf("upsert[%d] ledger shape: calls=%d", i, len(ledger.Calls))
		}
		if ledger.Calls[0].Question == "" || ledger.Calls[0].Observations[0]["observation_id"] == nil {
			t.Fatalf("upsert[%d] ledger call not annotated: %+v", i, ledger.Calls[0])
		}
	}
	// 成功落库报告后 superseded 恰一次（行保留，不删）。
	if len(progress.superseded) != 1 || progress.superseded[0] != "job-p1" {
		t.Fatalf("supersede marks: %v", progress.superseded)
	}
}

// job 超时（ctx 已死）后终态 abandoned+timeout 仍落库，且写入用的是脱离已死
// ctx 的后台 context（ctxAlive=true）；不写 result。
func TestSignalResearch_ProgressAbandonedOnTimeoutWithLiveCtx(t *testing.T) {
	router := &srStubRouter{
		decisions: []string{srDecision("call_tool", map[string]any{
			"tool": "eia_wpsr_table1", "args": map[string]any{"section": "s1"}, "question": "q",
		})},
		compose: []srComposeReply{{content: srComposeOK("c1:o1")}},
	}
	tool := &srToolStub{replies: []srToolReply{{body: srEiaBody(srEiaObs("2026-07-31", 420.0))}}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)
	progress := &srProgressStore{}
	svc.SetSignalProgressStore(progress)

	// 已过期 ctx：第一轮 LLM 决策前 job 已死——模拟 150 分钟截止掐断研究。
	ctx, cancel := context.WithTimeout(context.Background(), -time.Millisecond)
	defer cancel()
	_, err := svc.ResearchCandidate(ctx, 5, 7, "job-p2", nil)
	if err == nil {
		t.Fatalf("dead ctx must fail the run")
	}
	var stageErr *SignalStageError
	if !asSignalStageError(err, &stageErr) || stageErr.Stage != SignalStageResearch {
		t.Fatalf("stage error: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout must be recognizable: %v", err)
	}
	if len(store.results) != 0 {
		t.Fatalf("failed run must not save a result")
	}
	// 终态写：abandoned + timeout + error 文本；写入 ctx 存活（WithoutCancel 生效）。
	if len(progress.upserts) != 1 {
		t.Fatalf("want exactly 1 terminal upsert, got %d", len(progress.upserts))
	}
	up := progress.upserts[0]
	if up.row.Status != repository.SignalResearchProgressAbandoned || up.row.StopReason != SignalProgressStopReasonTimeout {
		t.Fatalf("terminal row: status=%q stop_reason=%q", up.row.Status, up.row.StopReason)
	}
	if up.row.Error == "" {
		t.Fatalf("terminal row must carry the error text")
	}
	if !up.ctxAlive {
		t.Fatalf("terminal write must use a live (detached) context — 已死 ctx 下终态也必须落库")
	}
	if len(progress.superseded) != 0 {
		t.Fatalf("failed run must not supersede")
	}
}

// 拦截轮同样推进 rounds_done（不执行不冒充取数）：非法工具被 CheckCall 拦，
// 该轮结束后仍滚动保存一次进展。
func TestSignalResearch_ProgressBlockedRoundStillCounts(t *testing.T) {
	router := &srStubRouter{
		decisions: []string{
			srDecision("call_tool", map[string]any{
				"tool": "web_search", "args": map[string]any{"query": "x"}, "question": "q",
			}),
			srDecision("finish", map[string]any{"summary": "无相关数据，收束。"}),
		},
		compose: []srComposeReply{{content: srComposeOKNoData()}},
	}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(nil), store)
	progress := &srProgressStore{}
	svc.SetSignalProgressStore(progress)

	if _, err := svc.ResearchCandidate(t.Context(), 5, 7, "job-p3", nil); err != nil {
		t.Fatalf("ResearchCandidate: %v", err)
	}
	if len(progress.upserts) != 1 {
		t.Fatalf("want 1 upsert after the blocked round, got %d", len(progress.upserts))
	}
	up := progress.upserts[0]
	if up.row.RoundsDone != 1 || up.row.SourceCalls != 0 {
		t.Fatalf("blocked round: rounds=%d calls=%d, want 1/0", up.row.RoundsDone, up.row.SourceCalls)
	}
}

// store 未接线或 jobID 为空：零进展调用，其余行为不变（旧直调路径零影响）。
func TestSignalResearch_ProgressDisabledLeavesNoTrace(t *testing.T) {
	router := &srStubRouter{
		decisions: []string{srDecision("finish", map[string]any{"summary": "done"})},
		compose:   []srComposeReply{{content: srComposeOKNoData()}},
	}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(nil), store)
	progress := &srProgressStore{}
	svc.SetSignalProgressStore(progress)

	if _, err := svc.ResearchCandidate(t.Context(), 5, 7, "", nil); err != nil {
		t.Fatalf("ResearchCandidate without jobID: %v", err)
	}
	// 不接线 store：jobID 有值也不写。
	candidate2, discovery2 := srFixturePair(8)
	store2 := &srStubStore{candidate: candidate2, discovery: discovery2}
	router2 := &srStubRouter{
		decisions: []string{srDecision("finish", map[string]any{"summary": "done"})},
		compose:   []srComposeReply{{content: srComposeOKNoData()}},
	}
	svc2 := srNewService(router2, newSRRegistry(nil), store2)
	if _, err := svc2.ResearchCandidate(t.Context(), 5, 8, "job-p4", nil); err != nil {
		t.Fatalf("ResearchCandidate without store: %v", err)
	}
	if len(progress.upserts) != 0 || len(progress.superseded) != 0 {
		t.Fatalf("progress must stay untouched: %+v", progress)
	}
}

// stop_reason 派生：截止 → timeout；stage 已知 → stage 值；其余 → failed。
func TestSignalResearch_ProgressStopReasonDerivation(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("研究被取消或超时: %w", context.DeadlineExceeded), SignalProgressStopReasonTimeout},
		{&SignalStageError{Stage: SignalStageCompose, Err: errors.New("bad draft")}, SignalStageCompose},
		{&SignalStageError{Stage: SignalStageSave, Err: errors.New("db down")}, SignalStageSave},
		{errors.New("研究循环异常终止"), "failed"},
	}
	for i, c := range cases {
		if got := signalProgressStopReason(c.err); got != c.want {
			t.Fatalf("case %d: got %q want %q", i, got, c.want)
		}
	}
}

// ── 无覆盖早停反馈（tasks 3.8 / design §10.2，spec「空手源提示早停」）─────────
//
// 用例名先于实现（§2 用例先行）：
//   ① TestSignalResearch_NoCoverageFeedbackAfterThirdEmptyCall —— 同源连续3次
//     空手（kept=0）→ 第3次轮后历史含反馈行；反馈不新增 ToolCallRecord、
//     decisions 计数不变；
//   ② TestSignalResearch_NoCoverageFeedbackOncePerSource —— 源错误两形状都计
//     空手；第4/5次连续空手不再重复注入（每源最多一次）；
//   ③ TestSignalResearch_NoCoverageStreakResetsOnSuccess —— 成功取数清零计数
//    （2空手+成功+1空手不触发），再3连空手才重新达到触发条件（反馈行每源仍
//     恰一次，design「每源最多注入一次」权威）；
//   ④ TestSignalResearch_NoCoverageFinaleOnceAfterAllFourSources —— 四源全触
//     发 → 终局提示恰一次；
//   ⑤ TestSignalResearch_NoFeedbackWithoutProviderOrPolicy —— policy=nil 与
//     未实现新接口的 policy：历史逐字节一致且零反馈行；
//   ⑥ calculate 与被拦轮不参与计数（TestSignalResearch_NoCoverageIgnores...）。

// srEmptyObsBody：筛选后 kept=0 的空手响应（no_data / 全部被剔共同形状）。
func srEmptyObsBody() string { return `{"observations":[]}` }

// srTypedErrorBody：wiring 层类型化错误帧（{"error_code","message"} 形状）。
func srTypedErrorBody() string {
	return `{"error_code":"SOURCE_UNAVAILABLE","message":"目标 host 不在核定白名单内"}`
}

// srHistoryOf 取一次决策请求的 history 块（最后一条 user 消息全文）。
func srHistoryOf(req airouter.ChatRequest) string {
	return req.Messages[len(req.Messages)-1].Content
}

func srCountInHistories(router *srStubRouter, needle string) int {
	if len(router.researchCalls) == 0 {
		return 0
	}
	// 历史逐轮累积（每轮 user 消息携带全部历史行）：最终消息含全量历史，
	// 其中匹配行数即累计注入次数，不能逐消息累加（同一行会被重复计数）。
	final := srHistoryOf(router.researchCalls[len(router.researchCalls)-1])
	n := 0
	for _, line := range strings.Split(final, "\n") {
		if strings.Contains(line, needle) {
			n++
		}
	}
	return n
}

// srFirstFeedbackRound 返回首次出现 needle 的决策请求下标（0 基；无则 -1）。
func srFirstHistoryContaining(router *srStubRouter, needle string) int {
	for i, req := range router.researchCalls {
		if strings.Contains(srHistoryOf(req), needle) {
			return i
		}
	}
	return -1
}

// ①⑥：同源连续3次空手 → 第3次轮后的历史含反馈行；反馈行不是工具结果
// （不新增 ToolCallRecord）、不消耗决策轮（decisions=3取数+1finish）。
func TestSignalResearch_NoCoverageFeedbackAfterThirdEmptyCall(t *testing.T) {
	router := &srStubRouter{compose: []srComposeReply{{content: srComposeOKNoData()}}}
	for i := 0; i < 3; i++ {
		router.decisions = append(router.decisions, srDecision("call_tool", map[string]any{
			"tool": "eia_wpsr_table1", "args": map[string]any{"section": fmt.Sprintf("s%d", i)},
			"question": fmt.Sprintf("库存问题%d", i),
		}))
	}
	router.decisions = append(router.decisions, srDecision("finish", map[string]any{"summary": "四源无数据，如实列缺口。"}))
	tool := &srToolStub{replies: []srToolReply{{body: srEmptyObsBody()}, {body: srEmptyObsBody()}, {body: srEmptyObsBody()}}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, payload, _ := runResearch(t, svc, store, 7)

	if len(router.researchCalls) != 4 {
		t.Fatalf("decisions=3取数+1finish，research chats=%d", len(router.researchCalls))
	}
	for i := 0; i < 3; i++ {
		if strings.Contains(srHistoryOf(router.researchCalls[i]), "系统提示（源覆盖）") {
			t.Fatalf("round %d must not carry feedback yet", i+1)
		}
	}
	final := srHistoryOf(router.researchCalls[3])
	if got := strings.Count(final, "系统提示（源覆盖）"); got != 1 {
		t.Fatalf("feedback line must appear exactly once after round 3, got %d", got)
	}
	wantLine := "系统提示（源覆盖）：eia_wpsr_table1：该源在本研究问题上已连续3次无覆盖，换参数也不会有数据；若四源皆无覆盖请直接 finish（summary 里如实列缺口）"
	if !strings.Contains(final, wantLine) {
		t.Fatalf("feedback text mismatch, history tail: %q", final)
	}
	meta := payload.GenerationMeta
	if meta.Decisions != 4 || meta.SourceCalls != 3 || meta.CalculationCalls != 0 || meta.StopReason != SignalStopReasonFinished {
		t.Fatalf("feedback must not consume rounds or calls: %+v", meta)
	}
	// 反馈不新增 ToolCallRecord：恰3条取数记录，且没有一条记录携带反馈文本。
	records := signalToolCallRecords(t, store.results[0].ToolCalls)
	if len(records) != 3 {
		t.Fatalf("exactly 3 tool call records expected, got %d", len(records))
	}
	for _, rec := range records {
		if strings.Contains(rec.ResultFull, "系统提示（源覆盖）") || strings.Contains(rec.ResultPreview, "系统提示（源覆盖）") {
			t.Fatalf("feedback must not masquerade as a tool result: %+v", rec)
		}
	}
}

// ②：源错误两形状（{"error_code"} 类型化帧 / Execute error）都计空手；连续
// 第4/5次空手不再重复注入（每源最多一次）。
func TestSignalResearch_NoCoverageFeedbackOncePerSource(t *testing.T) {
	router := &srStubRouter{compose: []srComposeReply{{content: srComposeOKNoData()}}}
	for i := 0; i < 5; i++ {
		router.decisions = append(router.decisions, srDecision("call_tool", map[string]any{
			"tool": "eia_wpsr_table1", "args": map[string]any{"section": fmt.Sprintf("s%d", i)},
			"question": fmt.Sprintf("问题%d", i),
		}))
	}
	router.decisions = append(router.decisions, srDecision("finish", map[string]any{"summary": "源持续失败。"}))
	tool := &srToolStub{replies: []srToolReply{
		{body: srTypedErrorBody()},
		{err: errors.New("执行失败: 连接超时")},
		{body: srEmptyObsBody()},
		{err: errors.New("执行失败: 再超时")},
		{body: srTypedErrorBody()},
	}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, payload, _ := runResearch(t, svc, store, 7)

	if got := srCountInHistories(router, "已连续3次无覆盖"); got != 1 {
		t.Fatalf("per-source feedback must fire exactly once (4th/5th empty must not repeat), got %d", got)
	}
	if idx := srFirstHistoryContaining(router, "系统提示（源覆盖）"); idx != 3 {
		t.Fatalf("feedback must appear after the 3rd empty call, first seen at round %d", idx+1)
	}
	if payload.GenerationMeta.SourceCalls != 5 || payload.GenerationMeta.Decisions != 6 {
		t.Fatalf("counting must not alter budget accounting: %+v", payload.GenerationMeta)
	}
}

// ③：成功取数清零该源计数——2空手+成功+1空手不触发（若无重置，第4次调用即
// 累计第3次空手会提前触发）；成功后再3连空手才重新达到触发条件；反馈行每源
// 仍恰一次（design §10.2「每源最多注入一次」权威，不随成功重置）。
func TestSignalResearch_NoCoverageStreakResetsOnSuccess(t *testing.T) {
	router := &srStubRouter{compose: []srComposeReply{{content: srComposeOK("c3:o1")}}}
	for i := 0; i < 6; i++ {
		router.decisions = append(router.decisions, srDecision("call_tool", map[string]any{
			"tool": "eia_wpsr_table1", "args": map[string]any{"section": fmt.Sprintf("s%d", i)},
			"question": fmt.Sprintf("问题%d", i),
		}))
	}
	router.decisions = append(router.decisions, srDecision("finish", map[string]any{"summary": "done"}))
	tool := &srToolStub{replies: []srToolReply{
		{body: srEmptyObsBody()},
		{body: srEmptyObsBody()},
		{body: srEiaBody(srEiaObs("2026-08-07", 421.5))}, // 成功取数：kept=1
		{body: srEmptyObsBody()},
		{body: srEmptyObsBody()},
		{body: srEmptyObsBody()}, // 成功后第3次连续空手 → 此处触发
	}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, _, _ = runResearch(t, svc, store, 7)

	if idx := srFirstHistoryContaining(router, "系统提示（源覆盖）"); idx != 6 {
		t.Fatalf("streak must reset on success; feedback expected after round 6, got round %d", idx+1)
	}
	if got := srCountInHistories(router, "已连续3次无覆盖"); got != 1 {
		t.Fatalf("per-source feedback stays once-per-source across resets, got %d", got)
	}
}

// ④：四源全部触发过 → 终局提示恰一次（与第4个源的反馈同轮注入）。
func TestSignalResearch_NoCoverageFinaleOnceAfterAllFourSources(t *testing.T) {
	router := &srStubRouter{compose: []srComposeReply{{content: srComposeOKNoData()}}}
	type srcSpec struct {
		tool string
		args func(i int) map[string]any
	}
	specs := []srcSpec{
		{"eia_wpsr_table1", func(i int) map[string]any { return map[string]any{"section": fmt.Sprintf("s%d", i)} }},
		{"jodi_oil_primary", func(i int) map[string]any { return map[string]any{"geo": fmt.Sprintf("g%d", i)} }},
		{"wb_wdi", func(i int) map[string]any { return map[string]any{"indicator": fmt.Sprintf("i%d", i)} }},
		{"un_comtrade_trade", func(i int) map[string]any { return map[string]any{"period": fmt.Sprintf("p%d", i)} }},
	}
	for _, spec := range specs {
		for i := 0; i < 3; i++ {
			router.decisions = append(router.decisions, srDecision("call_tool", map[string]any{
				"tool": spec.tool, "args": spec.args(i), "question": spec.tool + "问题",
			}))
		}
	}
	router.decisions = append(router.decisions, srDecision("finish", map[string]any{"summary": "四源皆无覆盖，列缺口。"}))

	emptyReplies := func() []srToolReply {
		return []srToolReply{{body: srEmptyObsBody()}, {body: srEmptyObsBody()}, {body: srEmptyObsBody()}}
	}
	tools := map[string]*srToolStub{
		"eia_wpsr_table1":   {replies: emptyReplies()},
		"jodi_oil_primary":  {replies: emptyReplies()},
		"wb_wdi":            {replies: emptyReplies()},
		"un_comtrade_trade": {replies: []srToolReply{{body: srTypedErrorBody()}, {body: srTypedErrorBody()}, {body: srTypedErrorBody()}}},
	}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(tools), store)

	_, payload, _ := runResearch(t, svc, store, 7)

	// 每源反馈恰一次，触发点=该源第3次空手后的下一轮历史。
	for round, spec := range specs {
		idx := srFirstHistoryContaining(router, "系统提示（源覆盖）："+spec.tool+"：")
		if idx != round*3+3 {
			t.Fatalf("%s feedback expected after its 3rd empty call, first seen at round %d", spec.tool, idx+1)
		}
		if got := srCountInHistories(router, "系统提示（源覆盖）："+spec.tool+"："); got != 1 {
			t.Fatalf("%s feedback must fire exactly once, got %d", spec.tool, got)
		}
	}
	// 终局提示恰一次，与第4源反馈同轮（round 12）之后注入。
	if idx := srFirstHistoryContaining(router, "四个数据源均确认无覆盖"); idx != 12 {
		t.Fatalf("finale expected after round 12, first seen at round %d", idx+1)
	}
	if got := srCountInHistories(router, "四个数据源均确认无覆盖"); got != 1 {
		t.Fatalf("finale must fire exactly once, got %d", got)
	}
	if payload.GenerationMeta.Decisions != 13 || payload.GenerationMeta.SourceCalls != 12 {
		t.Fatalf("finale must not alter accounting: %+v", payload.GenerationMeta)
	}
}

// ⑤：policy=nil 与未实现 toolLoopFeedbackProvider 的 policy——历史逐字节一致
// 且零反馈行（既有 LP-1~LP-7 / policy=nil 回归全绿为主证据，此为显式断言）。
type srLoopProbeRouter struct {
	decisions []string
	requests  []airouter.ChatRequest
}

func (r *srLoopProbeRouter) Chat(_ context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	r.requests = append(r.requests, req)
	n := len(r.requests)
	if n > len(r.decisions) {
		return nil, fmt.Errorf("srLoopProbeRouter: unexpected call #%d", n)
	}
	return &airouter.ChatResult{Content: r.decisions[n-1]}, nil
}

// srNoFeedbackPolicy 只实现 toolLoopPolicy 三方法（如 investigationPolicy）。
type srNoFeedbackPolicy struct{}

func (srNoFeedbackPolicy) CheckCall(int, map[string]any) toolCallVerdict                     { return toolCallVerdict{} }
func (srNoFeedbackPolicy) ObserveCall(int, string, map[string]any, string, string, []string) {}
func (srNoFeedbackPolicy) CheckFinish(int, string) toolFinishVerdict                         { return toolFinishVerdict{} }

func TestSignalResearch_NoFeedbackWithoutProviderOrPolicy(t *testing.T) {
	// 编译期实现、运行期确认不实现反馈扩展（type assertion 缺席的契约）。
	var p any = srNoFeedbackPolicy{}
	if _, ok := p.(toolLoopPolicy); !ok {
		t.Fatalf("stub must implement toolLoopPolicy")
	}
	if _, ok := p.(toolLoopFeedbackProvider); ok {
		t.Fatalf("stub must NOT implement toolLoopFeedbackProvider")
	}

	run := func(policy toolLoopPolicy) []string {
		t.Helper()
		router := &srLoopProbeRouter{decisions: []string{
			`{"action":"call_tool","thought":"t","tool":"probe","args":{"a":1},"question":"q1"}`,
			`{"action":"call_tool","thought":"t","tool":"probe","args":{"a":2},"question":"q2"}`,
			`{"action":"finish","thought":"t","summary":"done"}`,
		}}
		stub := &srToolStub{replies: []srToolReply{{body: srEiaBody(srEiaObs("2026-08-07", 420.0))}, {body: srEiaBody(srEiaObs("2026-08-14", 421.0))}}}
		reg := newSRRegistry(map[string]*srToolStub{"probe": stub})
		loop, err := runToolLoop(t.Context(), router, reg, "data_enrichment_analysis", toolLoopParams{
			sessionID: "probe", systemPrompt: "s", taskLine: "l",
			operation: "data_enrichment.tool_use", allowedTools: []string{"probe"},
			maxLoops: 5, policy: policy,
		})
		if err != nil || loop.FinalData != "done" {
			t.Fatalf("probe loop: err=%v final=%q", err, loop.FinalData)
		}
		histories := make([]string, 0, len(router.requests))
		for _, req := range router.requests {
			h := srHistoryOf(req)
			if strings.Contains(h, "系统提示（源覆盖）") {
				t.Fatalf("no feedback line may appear without the provider: %q", h)
			}
			histories = append(histories, h)
		}
		return histories
	}

	nilHistory := run(nil)
	noProviderHistory := run(srNoFeedbackPolicy{})
	if len(nilHistory) != len(noProviderHistory) {
		t.Fatalf("history length diverged: %d vs %d", len(nilHistory), len(noProviderHistory))
	}
	for i := range nilHistory {
		if nilHistory[i] != noProviderHistory[i] {
			t.Fatalf("history byte-diverged at round %d:\nnil: %q\npol: %q", i+1, nilHistory[i], noProviderHistory[i])
		}
	}
}

// ⑥补：calculate 与被拦轮不参与计数——空手×2 → 计算（被拒）→ 白名单外被拦
// → 第3次空手才触发；若 calculate/被拦轮误入计数或清零，触发点都会漂移。
func TestSignalResearch_NoCoverageIgnoresCalculateAndBlockedRounds(t *testing.T) {
	router := &srStubRouter{compose: []srComposeReply{{content: srComposeOKNoData()}}}
	router.decisions = append(router.decisions,
		srDecision("call_tool", map[string]any{"tool": "eia_wpsr_table1", "args": map[string]any{"section": "s1"}, "question": "q1"}),
		srDecision("call_tool", map[string]any{"tool": "eia_wpsr_table1", "args": map[string]any{"section": "s2"}, "question": "q2"}),
		srDecision("calculate", map[string]any{"op": "difference", "inputs": []string{"c9:o9"}, "question": "算一下"}),
		srDecision("call_tool", map[string]any{"tool": "web_search", "args": map[string]any{"query": "x"}, "question": "q3"}),
		srDecision("call_tool", map[string]any{"tool": "eia_wpsr_table1", "args": map[string]any{"section": "s3"}, "question": "q4"}),
		srDecision("finish", map[string]any{"summary": "done"}),
	)
	tool := &srToolStub{replies: []srToolReply{{body: srEmptyObsBody()}, {body: srEmptyObsBody()}, {body: srEmptyObsBody()}}}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, newSRRegistry(map[string]*srToolStub{"eia_wpsr_table1": tool}), store)

	_, payload, _ := runResearch(t, svc, store, 7)

	if idx := srFirstHistoryContaining(router, "系统提示（源覆盖）"); idx != 5 {
		t.Fatalf("calculate/blocked rounds must not count; feedback expected after round 5, got round %d", idx+1)
	}
	if got := srCountInHistories(router, "系统提示（源覆盖）"); got != 1 {
		t.Fatalf("exactly one feedback line expected, got %d", got)
	}
	if payload.GenerationMeta.Decisions != 6 || payload.GenerationMeta.SourceCalls != 3 || payload.GenerationMeta.CalculationCalls != 1 {
		t.Fatalf("accounting unchanged: %+v", payload.GenerationMeta)
	}
}

// ── 源可用性现算（review M2 / design §10.4）：研究克隆期按 live probe 重贴标注 ──

// ① probe 报不可用 → 克隆后的 Description 与 prompt toolsDesc 都含标注；
// 不 RequiresKey 的源不受影响。
func TestSignalResearch_AvailabilityProbeAppliedAtCloneTime(t *testing.T) {
	const notice = SignalUnavailableNoticeAnchor + "COMTRADE_API_KEY】调用会返回配置缺失错误帧"
	probe := func(toolName string) string {
		if toolName == "un_comtrade_trade" {
			return notice
		}
		return ""
	}
	base := newSRRegistry(map[string]*srToolStub{
		"un_comtrade_trade": {},
		"eia_wpsr_table1":   {},
	})
	// 克隆期现算：不可用源带标注，其余不受影响。
	sub := buildSignalResearchRegistry(base, srCutoff, newSignalResearchLedger(srCutoff), probe)
	if !strings.Contains(sub.tools["un_comtrade_trade"].Description, "当前不可用：未配置 COMTRADE_API_KEY") {
		t.Fatalf("clone must carry the live unavailability notice: %s", sub.tools["un_comtrade_trade"].Description)
	}
	if strings.Contains(sub.tools["eia_wpsr_table1"].Description, "当前不可用") {
		t.Fatalf("keyless source must stay unmarked: %s", sub.tools["eia_wpsr_table1"].Description)
	}
	// prompt toolsDesc 同样含标注。
	router := &srStubRouter{
		decisions: []string{srDecision("finish", map[string]any{"summary": "done"})},
		compose:   []srComposeReply{{content: srComposeOKNoData()}},
	}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, base, store)
	svc.SetToolAvailabilityProbe(probe)
	runResearch(t, svc, store, 7)
	system := router.researchCalls[0].Messages[0].Content
	if !strings.Contains(system, "当前不可用：未配置 COMTRADE_API_KEY") {
		t.Fatalf("research prompt toolsDesc must carry the live notice")
	}
}

// ② probe 报可用（模拟重启前配好 key）→ 启动期旧标注被移除（克隆与 prompt
// 两侧）；静态目录元数据保留。
func TestSignalResearch_AvailabilityProbeRemovesStaleNotice(t *testing.T) {
	base := newSRRegistry(map[string]*srToolStub{"un_comtrade_trade": {}})
	base.tools["un_comtrade_trade"].Description = "UN Comtrade 目录元数据" + SignalUnavailableNoticeAnchor + "COMTRADE_API_KEY】启动期旧标注"
	sub := buildSignalResearchRegistry(base, srCutoff, newSignalResearchLedger(srCutoff), func(string) string { return "" })
	if strings.Contains(sub.tools["un_comtrade_trade"].Description, "当前不可用") {
		t.Fatalf("stale notice must be stripped when the live probe says available: %s", sub.tools["un_comtrade_trade"].Description)
	}
	if !strings.Contains(sub.tools["un_comtrade_trade"].Description, "UN Comtrade 目录元数据") {
		t.Fatalf("static catalog metadata must survive the strip: %s", sub.tools["un_comtrade_trade"].Description)
	}
	// prompt 侧同样无旧标注（与克隆同源）。
	router := &srStubRouter{
		decisions: []string{srDecision("finish", map[string]any{"summary": "done"})},
		compose:   []srComposeReply{{content: srComposeOKNoData()}},
	}
	candidate, discovery := srFixturePair(7)
	store := &srStubStore{candidate: candidate, discovery: discovery}
	svc := srNewService(router, base, store)
	svc.SetToolAvailabilityProbe(func(string) string { return "" })
	runResearch(t, svc, store, 7)
	system := router.researchCalls[0].Messages[0].Content
	if strings.Contains(system, "当前不可用") {
		t.Fatalf("prompt must not advertise a stale unavailability snapshot")
	}
}

// ③ probe 注入前后旧流程 buildToolsDesc 输出零变化（旧 allowedTools 不含四源；
// 克隆也不原地改写共享 base registry）。
func TestSignalResearch_AvailabilityProbeLeavesLegacyToolsDescUnchanged(t *testing.T) {
	base := newSRRegistry(map[string]*srToolStub{
		"web_search":        {},
		"un_comtrade_trade": {},
	})
	base.tools["web_search"].Description = "旧流程 web_search 描述"
	before := buildToolsDesc(base, explorationToolNames)
	if !strings.Contains(before, "旧流程 web_search 描述") {
		t.Fatalf("legacy toolsDesc fixture must be non-empty: %q", before)
	}
	probe := func(string) string { return SignalUnavailableNoticeAnchor + "COMTRADE_API_KEY】probe 标注" }
	_ = buildSignalResearchRegistry(base, srCutoff, newSignalResearchLedger(srCutoff), probe)
	after := buildToolsDesc(base, explorationToolNames)
	if after != before {
		t.Fatalf("legacy toolsDesc must be byte-identical:\nbefore: %q\nafter:  %q", before, after)
	}
	if strings.Contains(base.tools["un_comtrade_trade"].Description, "当前不可用") {
		t.Fatalf("clone must not mutate the shared base registry description")
	}
	if strings.Contains(after, "un_comtrade_trade") {
		t.Fatalf("legacy toolsDesc must not surface research tools")
	}
}
