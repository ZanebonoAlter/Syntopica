package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
)

// ── 竞争假设（方法卡体系已移除，restructure-settings-navigation）──────────────
//
// 用例清单（保留假设生成核心）：
//   generated/custom 同链 | 无 H0 重试→机械补 H0 | 全宏大叙事重试
//   必填字段缺失不进研究 | >4 截断到 2-4 | 不预选赢家 | 未知/重复 id 剔除

// ── 测试素材 ────────────────────────────────────────────────────────────────

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
	gen, err := orch.generateBoardHypotheses(context.Background(), "investigation-sess", investigationTestQuestion(), investigationTestBrief())
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
	gen, err := orch.generateBoardHypotheses(context.Background(), "investigation-sess", investigationTestQuestion(), investigationTestBrief())
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
	gen, err := orch.generateBoardHypotheses(context.Background(), "investigation-sess", investigationTestQuestion(), investigationTestBrief())
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
	gen, err := orch.generateBoardHypotheses(context.Background(), "investigation-sess", investigationTestQuestion(), investigationTestBrief())
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
	gen, err := orch.generateBoardHypotheses(context.Background(), "investigation-sess", investigationTestQuestion(), investigationTestBrief())
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
	res, err := orch.prepareBoardHypotheses(context.Background(), "investigation-sess", custom, investigationTestBrief())
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
	if _, err := orch2.prepareBoardHypotheses(context.Background(), "s", rejected, investigationTestBrief()); err == nil {
		t.Fatal("illegal question source must be rejected")
	}
	if len(router2.calls) != 0 {
		t.Fatalf("illegal question must fail before any LLM call, got %d", len(router2.calls))
	}
}

// ── 不得预选赢家：prompt 不索取评估；结构无 assessment/confidence 字段 ───────

func TestBoardHypothesis_NoWinnerOrAssessmentInStage(t *testing.T) {
	prompt := assembleBoardHypothesizePrompt(investigationTestQuestion(), investigationTestBrief())
	for _, want := range []string{
		"is_null", "support_needed", "disconfirm_needed", "scope", // schema 字段
		"零假设",      // H0 纪律
		"不要输出任何评估", // 显式禁止预判
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
		{Content: cannedHypothesesJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	res, err := orch.prepareBoardHypotheses(context.Background(), "investigation-sess", investigationTestQuestion(), investigationTestBrief())
	if err != nil {
		t.Fatalf("prepareBoardHypotheses: %v", err)
	}
	if len(router.calls) != 1 {
		t.Fatalf("happy path = exactly 1 hypothesize call, got %d", len(router.calls))
	}
	c := router.calls[0]
	if c.SessionID != "investigation-sess" {
		t.Fatalf("session id must pass through: %q", c.SessionID)
	}
	if !c.JSONMode {
		t.Fatalf("must use JSON mode")
	}
	if len(res.Hypotheses.Hypotheses) != 3 || res.Hypotheses.Attempts != 1 || res.Hypotheses.H0Injected {
		t.Fatalf("happy path hypotheses: %+v", res.Hypotheses)
	}
}
