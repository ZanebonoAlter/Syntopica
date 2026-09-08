package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
)

// ── 方法选择 + 竞争假设（tasks 4.1/4.3，design D4/D6，test-cases M4/M7）─────
//
// 用例清单（test-cases M4/M7 + 任务 4.1/4.3）：
//   M4.1/M4.2 generated/custom 同链 | M4.3 无 H0 重试→机械补 H0
//   M4.4 全宏大叙事重试 | M4.5 必填字段缺失不进研究 | M4.6 >4 截断到 2-4
//   M7.1 空方法库正常 | M7.2 均 avoid → 0 张 | M7.3 最多 2 张+理由
//   M7.4 预算整卡舍弃 | M7.9 先选卡再生假设（严格顺序、无选择循环）
//   selector 坏 JSON 降级 0 张且调查继续 | 不预选赢家 | 未知/重复 id 剔除

// ── 测试素材 ────────────────────────────────────────────────────────────────

func investigationTestBrief() *BoardBriefPayload {
	return &BoardBriefPayload{
		Scope: "board", ResultKind: repository.ResultKindBoardBrief,
		Summary: "三条泳道各有进展，暂未发现统一关系。",
		Observations: []BoardBriefObservation{
			{ID: "o1", LaneID: 1, Statement: "一期产能落地", Basis: "周摘要", AsOfDate: "2026-08-26"},
			{ID: "o2", LaneID: 2, Statement: "二期招标启动", Basis: "月摘要", AsOfDate: "2026-08-25"},
		},
		Relationships: []boardBriefRelationship{
			{LaneIDs: []uint{1, 2}, Type: RelationUnclear, Explanation: "同期出现但传导未证实", Confidence: "low"},
		},
		Uncertainties:     []boardBriefUncertainty{{Question: "招标与产能是否共享驱动", WhyUncertain: "中间环节缺失", NeededEvidence: "资金来源数据"}},
		ResearchQuestions: []boardBriefQuestion{{ID: "q1", Question: "两条泳道是否由同一资金驱动", Rationale: "若同源将改变跟踪优先级", RelatedLaneIDs: []uint{1, 2}}},
		LaneRefs:          []laneRef{{LaneID: 1, Note: "泳道一"}, {LaneID: 2, Note: "泳道二"}},
	}
}

func investigationTestQuestion() BoardInvestigationQuestion {
	return BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: QuestionSourceGenerated}
}

func investigationTestSummaries() []repository.AnalysisMethod {
	return []repository.AnalysisMethod{
		{ID: 11, Name: "causal-chain", Title: "因果链检验", Summary: "检验传导链每一环",
			SelectionMeta: repository.AnalysisMethodSelectionMeta{
				ApplicableWhen: []string{"怀疑存在跨泳道传导"}, AvoidWhen: []string{"只有单条消息"},
				RequiredEvidence: []string{"两个独立来源"}, FailureModes: []string{"时间先后当因果"},
			}},
		{ID: 22, Name: "historical", Title: "历史对照", Summary: "找相似机制的历史案例",
			SelectionMeta: repository.AnalysisMethodSelectionMeta{
				ApplicableWhen: []string{"有相似机制先例"}, AvoidWhen: []string{"材料只有一周"},
			}},
		{ID: 33, Name: "broad", Title: "第三张卡", Summary: "广撒网方法",
			SelectionMeta: repository.AnalysisMethodSelectionMeta{ApplicableWhen: []string{"任何问题"}}},
	}
}

// investigationFullCards 是 loader 应返回的正文卡（内容哨兵用于断言注入边界）。
func investigationFullCards(ids ...uint) []repository.AnalysisMethod {
	byID := map[uint]repository.AnalysisMethod{
		11: {ID: 11, Name: "causal-chain", Title: "因果链检验", Content: "CONTENT-CAUSAL-STEP-检查每一环传导证据"},
		22: {ID: 22, Name: "historical", Title: "历史对照", Content: "CONTENT-HISTORICAL-STEP-对照相似机制"},
		33: {ID: 33, Name: "broad", Title: "第三张卡", Content: "CONTENT-THIRD-STEP"},
	}
	out := make([]repository.AnalysisMethod, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return out
}

// cannedHypothesesJSON：合法 2-4 假设（含 H0）的标准响应。
const cannedHypothesesJSON = `{"hypotheses":[
 {"id":"h0","label":"两条泳道变化没有统一机制，可由各自独立因素分别解释","is_null":true,
  "support_needed":["能同时解释两条泳道变化的可信共同机制"],"disconfirm_needed":["两条泳道各自的独立解释成立"],"scope":"本板块两条泳道"},
 {"id":"h1","label":"同一产业基金同时推动产能与招标","is_null":false,
  "support_needed":["基金公告同时提及两条泳道"],"disconfirm_needed":["资金来源明细互相独立"],"scope":"近三个月"},
 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,
  "support_needed":["补贴政策文本覆盖两条泳道"],"disconfirm_needed":["补贴时间线与变化不重合"],"scope":"政策周期"}]}`

// noH0HypothesesJSON：结构合法但无零假设（全宏大非零解释，M4.3/M4.4）。
const noH0HypothesesJSON = `{"hypotheses":[
 {"id":"h1","label":"产业资本深度重塑板块结构","is_null":false,
  "support_needed":["产业链股权数据"],"disconfirm_needed":["股权分散证据"],"scope":"板块全域"},
 {"id":"h2","label":"宏观周期驱动整体扩张","is_null":false,
  "support_needed":["宏观数据同向"],"disconfirm_needed":["逆周期证据"],"scope":"宏观维度"}]}`

// sequencingRouter 在 mock 路由外记录跨组件事件顺序（llm:<op> / load）。
type sequencingRouter struct {
	*internalMockRouter
	events *[]string
}

func (m *sequencingRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	*m.events = append(*m.events, "llm:"+req.Operation)
	return m.internalMockRouter.Chat(ctx, req)
}

func mustPrepare(t *testing.T, orch *OrchestratorService, summaries []repository.AnalysisMethod, load analysisMethodLoader) *boardHypothesisStageResult {
	t.Helper()
	res, err := orch.prepareBoardHypotheses(context.Background(), "investigation-sess", investigationTestQuestion(), investigationTestBrief(), summaries, nil, load)
	if err != nil {
		t.Fatalf("prepareBoardHypotheses: %v", err)
	}
	return res
}

// ── M7.1 空方法库：不调 selector，照常 hypothesize ───────────────────────────

func TestAnalysisMethodSelection_NoCandidatesSkipsSelector(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{{Content: cannedHypothesesJSON}}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	res := mustPrepare(t, orch, nil, func(context.Context, []uint) ([]repository.AnalysisMethod, error) {
		t.Fatal("loader must not be called when nothing is selected")
		return nil, nil
	})
	if len(router.calls) != 1 || router.calls[0].Operation != boardHypothesizeOperation {
		t.Fatalf("zero-method library: exactly one hypothesize call expected, got %+v", router.calls)
	}
	if len(res.Methods.Selected) != 0 || len(res.MethodRefs) != 0 || res.MethodPrompt != "" {
		t.Fatalf("zero-method library must yield empty methods: %+v", res.Methods)
	}
	if len(res.Hypotheses.Hypotheses) != 3 {
		t.Fatalf("hypotheses must still generate: %+v", res.Hypotheses)
	}
	// 0 方法时 hypothesize prompt 不得渲染方法注入区块（提示词常量中的条件
	// 提法不算，只有真实注入才会出现区块头）。
	if strings.Contains(router.calls[0].Messages[0].Content, "分析方法参考（仅约束") {
		t.Fatal("zero methods must not render a method section in the hypothesize prompt")
	}
}

// ── M7.2/M7.3 未知/avoid/重复剔除 + 最多 2 张 + 理由保留 ────────────────────

func TestAnalysisMethodSelection_FiltersUnknownDuplicateAvoidCapsTwo(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: `{"selected":[
			{"id":11,"reason":"传导怀疑与因果链检验适配"},
			{"id":99,"reason":"臆造的卡"},
			{"id":22,"reason":"本可适配但禁用条件命中","avoid_matched":true},
			{"id":33,"reason":"适配问题"},
			{"id":11,"reason":"重复提名"},
			{"id":22,"reason":"再次合法提名但已满"},
			{"id":33,"reason":"重复提名2"}]}`},
		{Content: cannedHypothesesJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	res := mustPrepare(t, orch, investigationTestSummaries(), func(_ context.Context, ids []uint) ([]repository.AnalysisMethod, error) {
		return investigationFullCards(ids...), nil
	})
	sel := res.Methods.Selected
	if len(sel) != 2 || sel[0].ID != 11 || sel[1].ID != 33 {
		t.Fatalf("selection must keep first two legal picks in order, got %+v", sel)
	}
	if !strings.Contains(sel[0].Reason, "因果链检验适配") {
		t.Fatalf("selection reason must be preserved: %+v", sel[0])
	}
	// dropped 按 (ID,Reason) 逐条对账：未知剔除、avoid 剔除、重复剔除、
	// 超上限剔除各就各位（同 ID 可多次提名、多次留痕）。
	droppedPairs := map[string]bool{}
	for _, d := range res.Methods.Dropped {
		droppedPairs[fmt.Sprintf("%d:%s", d.ID, d.Reason)] = true
	}
	for _, want := range []string{"99:unknown_id", "22:avoid_matched", "11:duplicate", "22:selection_limit", "33:duplicate"} {
		if !droppedPairs[want] {
			t.Fatalf("expected drop trace %q, got %+v", want, res.Methods.Dropped)
		}
	}
	// avoid 提名的 22 最终既不在 selected 也不在 refs（唯一提名被剔除）。
	for _, r := range res.MethodRefs {
		if r.ID == 22 {
			t.Fatalf("avoid-matched method must never be injected: %+v", res.MethodRefs)
		}
	}
}

// ── selector 坏 JSON：单次降级 0 张、无重试循环、调查继续 ────────────────────

func TestAnalysisMethodSelection_BadJSONDegradesToZeroAndStillHypothesizes(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: "这不是JSON"},
		{Content: cannedHypothesesJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	res := mustPrepare(t, orch, investigationTestSummaries(), func(context.Context, []uint) ([]repository.AnalysisMethod, error) {
		t.Fatal("loader must not run when selector degraded to zero")
		return nil, nil
	})
	if len(router.calls) != 2 {
		t.Fatalf("degraded selector must not retry (1 select + 1 hypothesize), got %d calls", len(router.calls))
	}
	if router.calls[0].Operation != boardMethodSelectOperation || router.calls[1].Operation != boardHypothesizeOperation {
		t.Fatalf("operation sequence wrong: %+v", router.calls)
	}
	if !res.Methods.Degraded || res.Methods.DegradedWhy != methodSelectionInvalidResponseWhy {
		t.Fatalf("selector degradation must carry the stable reason code %q, got %+v", methodSelectionInvalidResponseWhy, res.Methods)
	}
	if len(res.Methods.Selected) != 0 || len(res.MethodRefs) != 0 {
		t.Fatalf("degraded selector yields zero methods: %+v", res.Methods)
	}
	if len(res.Hypotheses.Hypotheses) != 3 {
		t.Fatalf("investigation must continue without methods: %+v", res.Hypotheses)
	}
}

// selector LLM 调用失败（传输/上游错误）：DegradedWhy 只能是稳定原因码，
// 完整 err（含内部地址/SQL/上游报文）不得进 trace（它会随 input_snapshot 固化）。
func TestAnalysisMethodSelection_ChatErrorUsesStableReasonCodeNoLeak(t *testing.T) {
	leaky := errors.New("dial tcp 10.0.0.7:5432: connection refused; pq: SELECT * FROM analysis_methods WHERE id IN (1)")
	router := &chatErrRouter{internalMockRouter: &internalMockRouter{responses: []*airouter.ChatResult{{Content: cannedHypothesesJSON}}}, err: leaky}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	res := mustPrepare(t, orch, investigationTestSummaries(), func(context.Context, []uint) ([]repository.AnalysisMethod, error) {
		t.Fatal("loader must not run when selector degraded to zero")
		return nil, nil
	})
	if !res.Methods.Degraded || res.Methods.DegradedWhy != methodSelectionChatUnavailableWhy {
		t.Fatalf("chat error must carry the stable reason code %q, got %+v", methodSelectionChatUnavailableWhy, res.Methods)
	}
	traceJSON, err := json.Marshal(res.Methods)
	if err != nil {
		t.Fatalf("marshal methods trace: %v", err)
	}
	for _, banned := range []string{"10.0.0.7", "pq:", "SELECT", "dial tcp"} {
		if strings.Contains(string(traceJSON), banned) {
			t.Fatalf("methods trace must not leak internal error detail %q: %s", banned, traceJSON)
		}
	}
	if len(res.Hypotheses.Hypotheses) != 3 {
		t.Fatalf("investigation must continue without methods: %+v", res.Hypotheses)
	}
}

// chatErrRouter 首个 Chat 调用返回注入错误，其余走内嵌 mock（模拟携带内部
// 细节的传输/上游失败）。
type chatErrRouter struct {
	*internalMockRouter
	err error
	hit int
}

func (m *chatErrRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	if m.hit == 0 {
		m.hit++
		m.calls = append(m.calls, req)
		return nil, m.err
	}
	return m.internalMockRouter.Chat(ctx, req)
}

// ── M7.9/M2.8 selector prompt 契约：只有元数据，无正文/无假设/无旧画像 ───────

func TestAnalysisMethodSelection_PromptContract(t *testing.T) {
	prompt := assembleBoardMethodSelectPrompt(investigationTestQuestion(), investigationTestBrief(), boardMethodCandidates(investigationTestSummaries()))
	for _, want := range []string{
		"两条泳道是否由同一资金驱动",         // 问题文本
		QuestionSourceGenerated, // 问题来源
		"三条泳道各有进展",              // 简报 summary
		"一期产能落地",                // 简报观察
		"因果链检验", "历史对照",         // 候选标题
		"检验传导链每一环",      // 候选摘要
		"怀疑存在跨泳道传导",     // applicable_when
		"只有单条消息",        // avoid_when
		`{"selected":`,  // 输出 schema
		"avoid_matched", // avoid 机制
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("method select prompt missing %q", want)
		}
	}
	for _, banned := range []string{
		"CONTENT-CAUSAL", "CONTENT-HISTORICAL", "CONTENT-THIRD", // 任何方法正文
		"同一产业基金同时推动产能与招标", // 尚未生成的假设内容
		"内部看美国", "reference_roles", // 旧作者画像
		"assessment", "winner", "最可信", // 不得让选择器预判赢家
		`"id":0`, // 示例 id 必须是正整数（0 会被 parser 拒绝）
	} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("method select prompt must not contain %q", banned)
		}
	}
	// required_evidence/failure_modes 也属选择元数据。
	if !strings.Contains(prompt, "两个独立来源") || !strings.Contains(prompt, "时间先后当因果") {
		t.Fatal("selection_meta must be fully projected (required_evidence/failure_modes)")
	}
}

// ── M7.4 只有选中卡注入；预算超限整卡舍弃且理由保留 ─────────────────────────

func TestAnalysisMethodSelection_SelectedOnlyInjectionAndWholeCardBudget(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: `{"selected":[{"id":11,"reason":"适配"},{"id":22,"reason":"次适配"}]}`},
		{Content: cannedHypothesesJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	oversized := strings.Repeat("乙", boardInvestigationMethodRuneBudget+100)
	res := mustPrepare(t, orch, investigationTestSummaries(), func(_ context.Context, ids []uint) ([]repository.AnalysisMethod, error) {
		cards := investigationFullCards(ids...)
		for i := range cards {
			if cards[i].ID == 22 {
				cards[i].Content = oversized // 第二张超预算 → 整卡舍弃
			}
		}
		return cards, nil
	})
	if len(res.MethodRefs) != 1 || res.MethodRefs[0].ID != 11 {
		t.Fatalf("budget-overflow card must be dropped whole: %+v", res.MethodRefs)
	}
	if !strings.Contains(res.MethodPrompt, "CONTENT-CAUSAL") {
		t.Fatalf("selected in-budget card content must be injected: %q", res.MethodPrompt)
	}
	if strings.Contains(res.MethodPrompt, "乙") {
		t.Fatal("oversized card content must not leak into the injection")
	}
	foundBudgetDrop := false
	for _, d := range res.Methods.Dropped {
		if d.ID == 22 && d.Reason == "budget_exceeded" {
			foundBudgetDrop = true
		}
	}
	if !foundBudgetDrop {
		t.Fatalf("budget drop must be traced: %+v", res.Methods.Dropped)
	}
	// 选择与舍弃理由都保留：selected 里 11/22 的 reason 仍在。
	if len(res.Methods.Selected) != 2 {
		t.Fatalf("selection reasons must survive budget drops: %+v", res.Methods.Selected)
	}
	// 方法正文只进 hypothesize 调用。
	hypoPrompt := router.calls[1].Messages[0].Content
	if !strings.Contains(hypoPrompt, "CONTENT-CAUSAL") {
		t.Fatal("hypothesize prompt must carry the injected method content")
	}
	if strings.Contains(hypoPrompt, "乙") {
		t.Fatal("budget-dropped card content must not reach hypothesize")
	}
}

// ── M7.9 严格顺序 select → load → hypothesize；无选择循环 ───────────────────

func TestAnalysisMethodSelection_StrictOrderNoSelectionLoop(t *testing.T) {
	events := []string{}
	base := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: `{"selected":[{"id":22,"reason":"更适配"},{"id":11,"reason":"次之"}]}`},
		{Content: cannedHypothesesJSON},
	}}
	router := &sequencingRouter{internalMockRouter: base, events: &events}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	var loadedIDs []uint
	res, err := orch.prepareBoardHypotheses(context.Background(), "investigation-sess",
		investigationTestQuestion(), investigationTestBrief(), investigationTestSummaries(), nil,
		func(_ context.Context, ids []uint) ([]repository.AnalysisMethod, error) {
			events = append(events, "load")
			loadedIDs = append([]uint{}, ids...)
			return investigationFullCards(ids...), nil
		})
	if err != nil {
		t.Fatalf("prepareBoardHypotheses: %v", err)
	}
	want := []string{"llm:" + boardMethodSelectOperation, "load", "llm:" + boardHypothesizeOperation}
	if fmt.Sprint(events) != fmt.Sprint(want) {
		t.Fatalf("stage order must be select→load→hypothesize, got %v", events)
	}
	// loader 按选择器返回顺序取卡（相关性顺序）。
	if fmt.Sprint(loadedIDs) != "[22 11]" {
		t.Fatalf("loader must follow selector order, got %v", loadedIDs)
	}
	if res.MethodRefs[0].ID != 22 || res.MethodRefs[1].ID != 11 {
		t.Fatalf("method refs keep selector order: %+v", res.MethodRefs)
	}
	selectCalls := 0
	for _, c := range base.calls {
		switch c.Operation {
		case boardMethodSelectOperation:
			selectCalls++
		case boardHypothesizeOperation:
		default:
			t.Fatalf("unexpected operation %q", c.Operation)
		}
	}
	if selectCalls != 1 {
		t.Fatalf("selector must run exactly once (no selection loop), got %d", selectCalls)
	}
}

// ── loader 失败：降级 0 方法继续 hypothesize，理由留痕 ────────────────────────

func TestAnalysisMethodSelection_LoadErrorDegradesToNoMethods(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: `{"selected":[{"id":11,"reason":"适配"},{"id":22,"reason":"次适配"}]}`},
		{Content: cannedHypothesesJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	res := mustPrepare(t, orch, investigationTestSummaries(), func(context.Context, []uint) ([]repository.AnalysisMethod, error) {
		return nil, fmt.Errorf("db down")
	})
	if len(res.MethodRefs) != 0 || res.MethodPrompt != "" {
		t.Fatalf("load failure must degrade to zero methods: %+v", res)
	}
	for _, d := range res.Methods.Dropped {
		if d.Reason != "load_failed" {
			t.Fatalf("load failure must be traced per selected card: %+v", res.Methods.Dropped)
		}
	}
	if len(res.Methods.Dropped) != 2 {
		t.Fatalf("both selected cards traced as load_failed: %+v", res.Methods.Dropped)
	}
	if len(res.Hypotheses.Hypotheses) != 3 {
		t.Fatalf("hypothesize must continue after load failure: %+v", res.Hypotheses)
	}
}

// ── summaries 读取失败：安全降级留痕，selector 0 次，hypothesize 照跑 ──────

func TestAnalysisMethodSelection_SummaryFailureDegradesSafelyAndContinues(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{{Content: cannedHypothesesJSON}}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	res, err := orch.prepareBoardHypotheses(context.Background(), "investigation-sess",
		investigationTestQuestion(), investigationTestBrief(), investigationTestSummaries(),
		fmt.Errorf("pq: connection refused at 10.0.0.7:5432 (internal detail)"), nil)
	if err != nil {
		t.Fatalf("summaries failure must degrade, not fail: %v", err)
	}
	// trace：Degraded=true + 安全稳定原因码；内部错误细节不进 trace/snapshot。
	if !res.Methods.Degraded || res.Methods.DegradedWhy != methodSummariesUnavailableWhy {
		t.Fatalf("summary failure must be traced with the stable why, got %+v", res.Methods)
	}
	if strings.Contains(res.Methods.DegradedWhy, "pq:") || strings.Contains(res.Methods.DegradedWhy, "10.0.0.7") {
		t.Fatalf("DegradedWhy must not leak internal error detail: %q", res.Methods.DegradedWhy)
	}
	traceJSON, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal stage result: %v", err)
	}
	if strings.Contains(string(traceJSON), "pq:") || strings.Contains(string(traceJSON), "10.0.0.7") {
		t.Fatal("stage result must not carry the internal summaries error")
	}
	// selector 0 次（Attempts=0 且无 select 调用），只有 hypothesize 跑了。
	if res.Methods.Attempts != 0 {
		t.Fatalf("selector must not run when summaries failed, attempts=%d", res.Methods.Attempts)
	}
	if len(router.calls) != 1 || router.calls[0].Operation != boardHypothesizeOperation {
		t.Fatalf("summaries failure: exactly one hypothesize call expected, got %+v", router.calls)
	}
	if len(res.Methods.Selected) != 0 || len(res.MethodRefs) != 0 || res.MethodPrompt != "" {
		t.Fatalf("summaries failure must yield zero methods: %+v", res)
	}
	if len(res.Hypotheses.Hypotheses) != 3 {
		t.Fatalf("hypothesize must still run: %+v", res.Hypotheses)
	}
}

// ── nil repo：装配错误明确拒绝，不偷偷走 0 方法 ────────────────────────────

func TestBoardHypothesis_NilRepoRejectedWithoutLLMCall(t *testing.T) {
	router := &internalMockRouter{}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap} // repo 未接线
	_, err := orch.HypothesizeBoardInvestigation(context.Background(), "s", investigationTestQuestion(), investigationTestBrief())
	if err == nil || !strings.Contains(err.Error(), "repository not wired") {
		t.Fatalf("nil repo must be an explicit error, got %v", err)
	}
	if len(router.calls) != 0 {
		t.Fatalf("nil repo must fail before any LLM call, got %d", len(router.calls))
	}
}

// ── selector id 只接受正整数：小数/负数/零/非数值一律忽略，不截断成其它卡 ────

func TestAnalysisMethodSelection_NonPositiveOrFractionalIDsIgnored(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: `{"selected":[
			{"id":11.5,"reason":"小数不得截断成11"},
			{"id":-1,"reason":"负数"},
			{"id":0,"reason":"零"},
			{"id":"11","reason":"字符串"},
			{"id":9007199254740993,"reason":"超JS safe integer，不得wrap成其它uint"},
			{"id":1e21,"reason":"超大数，uint溢出wrap风险"},
			{"id":11,"reason":"合法"}]}`},
		{Content: cannedHypothesesJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	res := mustPrepare(t, orch, investigationTestSummaries(), func(_ context.Context, ids []uint) ([]repository.AnalysisMethod, error) {
		return investigationFullCards(ids...), nil
	})
	if len(res.Methods.Selected) != 1 || res.Methods.Selected[0].ID != 11 {
		t.Fatalf("only the positive-integer nomination survives, got %+v", res.Methods.Selected)
	}
	// 非法提名既不选中也不落 dropped（无法用 uint 表达，只进日志）。
	if len(res.Methods.Dropped) != 0 {
		t.Fatalf("illegal ids are ignored pre-whitelist, not dropped: %+v", res.Methods.Dropped)
	}
	if len(res.MethodRefs) != 1 || res.MethodRefs[0].ID != 11 {
		t.Fatalf("fractional 11.5 must not inject card 11 via truncation: %+v", res.MethodRefs)
	}
}

// ── selector 空 reason：稳定占位，trace 不出现空串 ──────────────────────

func TestAnalysisMethodSelection_EmptyReasonGetsStablePlaceholder(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: `{"selected":[{"id":11},{"id":22,"reason":"   "}]}`},
		{Content: cannedHypothesesJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	res := mustPrepare(t, orch, investigationTestSummaries(), func(_ context.Context, ids []uint) ([]repository.AnalysisMethod, error) {
		return investigationFullCards(ids...), nil
	})
	if len(res.Methods.Selected) != 2 {
		t.Fatalf("both legal nominations selected: %+v", res.Methods.Selected)
	}
	for _, s := range res.Methods.Selected {
		if s.Reason != boardMethodSelectReasonPlaceholder {
			t.Fatalf("empty/blank reason must be the stable placeholder, got %q", s.Reason)
		}
	}
}

// ── loader 为 nil 但选中非空：逐卡 no_loader 留痕，不静默丢弃 ──────────────

func TestAnalysisMethodSelection_NilLoaderTracedAsNoLoader(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: `{"selected":[{"id":11,"reason":"适配"},{"id":22,"reason":"次适配"}]}`},
		{Content: cannedHypothesesJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	res := mustPrepare(t, orch, investigationTestSummaries(), nil) // loader 缺失
	if len(res.Methods.Dropped) != 2 {
		t.Fatalf("both selected cards must be traced, got %+v", res.Methods.Dropped)
	}
	for _, d := range res.Methods.Dropped {
		if d.Reason != "no_loader" {
			t.Fatalf("nil loader must be traced as no_loader, got %+v", d)
		}
	}
	if len(res.MethodRefs) != 0 || res.MethodPrompt != "" {
		t.Fatalf("nil loader yields zero injected methods: %+v", res)
	}
	if len(res.Hypotheses.Hypotheses) != 3 {
		t.Fatalf("hypothesize must continue after no_loader: %+v", res.Hypotheses)
	}
}

// ── M4.1 parser：合法 2-4、id 归一唯一、>4 截断 ─────────────────────────────

func TestBoardHypothesis_ParseLegalRangeAndUniqueIDs(t *testing.T) {
	two := `{"hypotheses":[
	 {"id":"h0","label":"零假设：无统一机制","is_null":true,"support_needed":["共同机制证据"],"disconfirm_needed":["各自独立解释"],"scope":"板块"},
	 {"label":"缺id自动补","is_null":false,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s"}]}`
	parsed, err := ParseJSONResponse(two)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	hs, err := parseBoardHypotheses(parsed)
	if err != nil {
		t.Fatalf("parseBoardHypotheses: %v", err)
	}
	if len(hs) != 2 || hs[0].IsNull != true || hs[1].IsNull != false {
		t.Fatalf("two-hypothesis payload must parse: %+v", hs)
	}
	if hs[1].ID == "" || hs[1].ID == hs[0].ID {
		t.Fatalf("missing ids must be auto-assigned uniquely: %+v", hs)
	}

	// 重复显式 id → 机械改名保唯一。
	dup := `{"hypotheses":[
	 {"id":"h","label":"甲","is_null":true,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s"},
	 {"id":"h","label":"乙","is_null":false,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s"},
	 {"id":"h","label":"丙","is_null":false,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s"}]}`
	parsed, _ = ParseJSONResponse(dup)
	hs, err = parseBoardHypotheses(parsed)
	if err != nil {
		t.Fatalf("duplicate ids must be normalized, not rejected: %v", err)
	}
	seen := map[string]bool{}
	for _, h := range hs {
		if seen[h.ID] {
			t.Fatalf("ids must be unique after normalization: %+v", hs)
		}
		seen[h.ID] = true
	}

	// M4.6 5 个假设 → 截断到 4。
	var sb strings.Builder
	sb.WriteString(`{"hypotheses":[`)
	for i := 0; i < 5; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"id":"h%d","label":"假设%d","is_null":%v,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s"}`, i, i, i == 0)
	}
	sb.WriteString(`]}`)
	parsed, _ = ParseJSONResponse(sb.String())
	hs, err = parseBoardHypotheses(parsed)
	if err != nil {
		t.Fatalf("5 hypotheses must truncate, not fail: %v", err)
	}
	if len(hs) != boardHypothesisMaxCount {
		t.Fatalf("count cap = %d, got %d", boardHypothesisMaxCount, len(hs))
	}
	if !hs[0].IsNull {
		t.Fatal("truncation keeps the first entries (H0 at head survives)")
	}
}

// ── M4.5 parser：必填字段缺失逐条剔除；<2 → 结构失败 ────────────────────────

func TestBoardHypothesis_ParseStrictRequiredFields(t *testing.T) {
	// 四条里两条缺必填（缺 support_needed / 缺 scope），两条合法。
	partial := `{"hypotheses":[
	 {"id":"h0","label":"零假设","is_null":true,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s"},
	 {"id":"bad1","label":"缺support","is_null":false,"disconfirm_needed":["b"],"scope":"s"},
	 {"id":"bad2","label":"缺scope","is_null":false,"support_needed":["a"],"disconfirm_needed":["b"]},
	 {"id":"bad3","label":"空label","is_null":false,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s2"},
	 {"id":"h1","label":"合法","is_null":false,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s"}]}`
	parsed, err := ParseJSONResponse(strings.Replace(partial, `"label":"空label"`, `"label":"  "`, 1))
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	hs, err := parseBoardHypotheses(parsed)
	if err != nil {
		t.Fatalf("per-item drops must not fail the payload: %v", err)
	}
	if len(hs) != 2 {
		t.Fatalf("invalid items dropped, valid kept: %+v", hs)
	}

	// 只剩 1 条合法 → 数量不合格（重试信号）。
	lone := `{"hypotheses":[
	 {"id":"h0","label":"唯一","is_null":true,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s"},
	 {"id":"bad","label":"缺disconfirm","is_null":false,"support_needed":["a"],"scope":"s"}]}`
	parsed, _ = ParseJSONResponse(lone)
	if _, err := parseBoardHypotheses(parsed); err == nil {
		t.Fatal("fewer than 2 valid hypotheses must fail structurally")
	}

	// hypotheses 字段缺失 → 结构失败。
	parsed, _ = ParseJSONResponse(`{"summary":"没有假设字段"}`)
	if _, err := parseBoardHypotheses(parsed); err == nil {
		t.Fatal("missing hypotheses array must fail")
	}
}

// ── M4.3 无 H0：重试一次 → 仍无 → 机械补入朴素 H0 ──────────────────────────

func TestBoardHypothesis_NoH0RetriesThenMechanicalH0(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: noH0HypothesesJSON},
		{Content: noH0HypothesesJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	gen, err := orch.generateBoardHypotheses(context.Background(), "investigation-sess", investigationTestQuestion(), investigationTestBrief(), "")
	if err != nil {
		t.Fatalf("second attempt without H0 must mechanically inject H0, got error: %v", err)
	}
	if len(router.calls) != 2 {
		t.Fatalf("no-H0 must retry exactly once, got %d calls", len(router.calls))
	}
	if !strings.Contains(router.calls[1].Messages[0].Content, boardHypothesizeRetryLead) {
		t.Fatal("retry prompt must carry the corrective note")
	}
	if !strings.Contains(router.calls[1].Messages[0].Content, "零假设") {
		t.Fatal("corrective note must name the missing zero hypothesis")
	}
	if !gen.H0Injected || gen.Attempts != 2 || gen.RetryReason == "" {
		t.Fatalf("generation meta must trace retry + injection: %+v", gen)
	}
	if len(gen.Hypotheses) != 3 { // 2 非零 + 1 机械 H0
		t.Fatalf("mechanical H0 appended to the 2 non-null, got %+v", gen.Hypotheses)
	}
	if !gen.Hypotheses[0].IsNull || !strings.Contains(gen.Hypotheses[0].Label, "没有统一机制") {
		t.Fatalf("injected H0 must lead the set as a plain explanation: %+v", gen.Hypotheses[0])
	}
	if len(gen.Hypotheses[0].SupportNeeded) == 0 || len(gen.Hypotheses[0].DisconfirmNeeded) == 0 || gen.Hypotheses[0].Scope == "" {
		t.Fatalf("mechanical H0 must carry its own evidence needs: %+v", gen.Hypotheses[0])
	}
	// id 唯一（LLM 用过 h1/h2，机械 H0 用 h0 不得冲突）。
	seen := map[string]bool{}
	for _, h := range gen.Hypotheses {
		if seen[h.ID] {
			t.Fatalf("injected H0 id collides: %+v", gen.Hypotheses)
		}
		seen[h.ID] = true
	}
}

// ── M4.4 全宏大：重试一次；第二次带 H0 → 用 LLM 结果、不注入 ─────────────────

func TestBoardHypothesis_AllGrandRetriesThenComplies(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: noH0HypothesesJSON},
		{Content: cannedHypothesesJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	gen, err := orch.generateBoardHypotheses(context.Background(), "investigation-sess", investigationTestQuestion(), investigationTestBrief(), "")
	if err != nil {
		t.Fatalf("compliant second attempt must be used: %v", err)
	}
	if len(router.calls) != 2 || gen.Attempts != 2 {
		t.Fatalf("exactly two attempts expected: calls=%d meta=%+v", len(router.calls), gen)
	}
	if gen.H0Injected {
		t.Fatal("second attempt carrying an H0 must not need mechanical injection")
	}
	if len(gen.Hypotheses) != 3 || !gen.Hypotheses[0].IsNull {
		t.Fatalf("LLM hypotheses used verbatim: %+v", gen.Hypotheses)
	}
}

// ── 坏 JSON 两次 → error，绝不机械编完整假设集 ────────────────────────────────

func TestBoardHypothesis_BadJSONTwiceErrors(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: "garbage"},
		{Content: `{"hypotheses": 还是坏`},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	gen, err := orch.generateBoardHypotheses(context.Background(), "investigation-sess", investigationTestQuestion(), investigationTestBrief(), "")
	if err == nil || gen != nil {
		t.Fatalf("two unusable attempts must return error, got %+v / %v", gen, err)
	}
	if len(router.calls) != 2 {
		t.Fatalf("exactly two attempts (no third), got %d", len(router.calls))
	}
}

// ── 第二次结构不可用（<2 合格）→ error，不凭空造非零假设 ─────────────────────

func TestBoardHypothesis_SecondAttemptStructurallyUnusableErrors(t *testing.T) {
	lone := `{"hypotheses":[
	 {"id":"h0","label":"唯一合格","is_null":true,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s"},
	 {"id":"bad","label":"缺disconfirm","is_null":false,"support_needed":["a"],"scope":"s"}]}`
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: noH0HypothesesJSON}, // 第一次：结构合法但无 H0 → 重试
		{Content: lone},               // 第二次：结构不可用 → error
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	gen, err := orch.generateBoardHypotheses(context.Background(), "investigation-sess", investigationTestQuestion(), investigationTestBrief(), "")
	if err == nil || gen != nil {
		t.Fatalf("structurally unusable second attempt must error (no fabricated hypotheses), got %+v", gen)
	}
}

// ── 机械补 H0 后超 4 → 裁到 4（H0 + 前 3 非零）──────────────────────────────

func TestBoardHypothesis_MechanicalH0TruncatesToFour(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"hypotheses":[`)
	for i := 1; i <= 4; i++ {
		if i > 1 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"id":"h%d","label":"非零假设%d","is_null":false,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s"}`, i, i)
	}
	sb.WriteString(`]}`)
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: noH0HypothesesJSON},
		{Content: sb.String()},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	gen, err := orch.generateBoardHypotheses(context.Background(), "investigation-sess", investigationTestQuestion(), investigationTestBrief(), "")
	if err != nil {
		t.Fatalf("injection + truncation path failed: %v", err)
	}
	if len(gen.Hypotheses) != boardHypothesisMaxCount {
		t.Fatalf("after mechanical H0 the set must cap at %d, got %d", boardHypothesisMaxCount, len(gen.Hypotheses))
	}
	if !gen.Hypotheses[0].IsNull {
		t.Fatalf("H0 must lead after injection: %+v", gen.Hypotheses)
	}
	nulls := 0
	for _, h := range gen.Hypotheses {
		if h.IsNull {
			nulls++
		}
	}
	if nulls != 1 {
		t.Fatalf("exactly one null expected, got %d", nulls)
	}
}

// ── M4.1/M4.2 generated/custom 同链 + question 校验 ─────────────────────────

func TestBoardHypothesis_QuestionSourceValidationAndCustomSameChain(t *testing.T) {
	// 校验：source 枚举 + trim 非空。
	bad := []BoardInvestigationQuestion{
		{Text: "问题", Source: "unknown"},
		{Text: "  ", Source: QuestionSourceCustom},
		{Text: "", Source: QuestionSourceGenerated},
	}
	for i, q := range bad {
		if err := q.Normalize(); err == nil {
			t.Fatalf("case %d: question must be rejected: %+v", i, q)
		}
	}
	custom := BoardInvestigationQuestion{Text: "  自填问题：资金是否同源？ ", Source: QuestionSourceCustom}
	if err := custom.Normalize(); err != nil {
		t.Fatalf("valid custom question rejected: %v", err)
	}
	if custom.Text != "自填问题：资金是否同源？" {
		t.Fatalf("question text must be trimmed: %q", custom.Text)
	}

	// custom 无显示 id 也走同一链路，prompt 携带原文与来源。
	router := &internalMockRouter{responses: []*airouter.ChatResult{{Content: cannedHypothesesJSON}}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	res, err := orch.prepareBoardHypotheses(context.Background(), "investigation-sess", custom, investigationTestBrief(), nil, nil, nil)
	if err != nil {
		t.Fatalf("custom question must run the same chain: %v", err)
	}
	if res.Question.Source != QuestionSourceCustom || res.Question.ID != "" {
		t.Fatalf("question echoed with source/id: %+v", res.Question)
	}
	prompt := router.calls[0].Messages[0].Content
	if !strings.Contains(prompt, "自填问题：资金是否同源？") || !strings.Contains(prompt, QuestionSourceCustom) {
		t.Fatalf("hypothesize prompt must carry custom question text + source: %q", prompt)
	}

	// 非法问题在任何 LLM 调用前被拒。
	rejected := BoardInvestigationQuestion{Text: "x", Source: "bogus"}
	router2 := &internalMockRouter{}
	orch2 := &OrchestratorService{airouter: router2, capability: internalTestCap}
	if _, err := orch2.prepareBoardHypotheses(context.Background(), "s", rejected, investigationTestBrief(), nil, nil, nil); err == nil {
		t.Fatal("illegal question source must be rejected")
	}
	if len(router2.calls) != 0 {
		t.Fatalf("illegal question must fail before any LLM call, got %d", len(router2.calls))
	}
}

// ── 不得预选赢家：prompt 不索取评估；结构无 assessment/confidence 字段 ───────

func TestBoardHypothesis_NoWinnerOrAssessmentInStage(t *testing.T) {
	prompt := assembleBoardHypothesizePrompt(investigationTestQuestion(), investigationTestBrief(), "CONTENT-CAUSAL-STEP")
	for _, want := range []string{
		"is_null", "support_needed", "disconfirm_needed", "scope", // schema 字段
		"零假设",                 // H0 纪律
		"CONTENT-CAUSAL-STEP", // 方法正文只进此调用
		"不要输出任何评估",            // 显式禁止预判
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("hypothesize prompt missing %q", want)
		}
	}
	for _, banned := range []string{"assessment", "winner", "最可信的假设", "选出赢家", "confidence"} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("hypothesize prompt must not request %q", banned)
		}
	}

	// LLM 顽抗返回 assessment/winner → parser 只取白名单字段。
	rogue := `{"winner":"h1","hypotheses":[
	 {"id":"h0","label":"零假设","is_null":true,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s","assessment":"supported","confidence":"high"},
	 {"id":"h1","label":"非零","is_null":false,"support_needed":["a"],"disconfirm_needed":["b"],"scope":"s","assessment":"refuted"}]}`
	parsed, err := ParseJSONResponse(rogue)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	hs, err := parseBoardHypotheses(parsed)
	if err != nil {
		t.Fatalf("rogue extra fields must be ignored, not rejected: %v", err)
	}
	data, err := json.Marshal(hs[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var keys map[string]any
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	allowed := map[string]bool{"id": true, "label": true, "is_null": true, "support_needed": true, "disconfirm_needed": true, "scope": true}
	for k := range keys {
		if !allowed[k] {
			t.Fatalf("hypothesis struct leaked stage-forbidden field %q: %s", k, data)
		}
	}
}

// ── 一次成功：单次 hypothesize 调用，session 透传 ─────────────────────────────

func TestBoardHypothesis_SingleCallOnSuccessWithSession(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: `{"selected":[{"id":11,"reason":"适配"}]}`},
		{Content: cannedHypothesesJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	res := mustPrepare(t, orch, investigationTestSummaries(), func(_ context.Context, ids []uint) ([]repository.AnalysisMethod, error) {
		return investigationFullCards(ids...), nil
	})
	if len(router.calls) != 2 {
		t.Fatalf("happy path = 1 select + 1 hypothesize, got %d", len(router.calls))
	}
	for i, c := range router.calls {
		if c.SessionID != "investigation-sess" {
			t.Fatalf("call %d session id must pass through: %q", i, c.SessionID)
		}
		if !c.JSONMode {
			t.Fatalf("call %d must use JSON mode", i)
		}
	}
	if !res.Methods.Degraded && len(res.Methods.Selected) != 1 {
		t.Fatalf("one method selected: %+v", res.Methods)
	}
	if len(res.Hypotheses.Hypotheses) != 3 || res.Hypotheses.Attempts != 1 || res.Hypotheses.H0Injected {
		t.Fatalf("happy path hypotheses: %+v", res.Hypotheses)
	}
	if len(res.MethodRefs) != 1 || res.MethodRefs[0].ContentHash == "" {
		t.Fatalf("method refs must carry content hash: %+v", res.MethodRefs)
	}
}
