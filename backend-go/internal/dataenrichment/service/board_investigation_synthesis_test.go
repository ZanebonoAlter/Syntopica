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

// ── board_synthesize 调查综合（tasks 4.5，design D4/D7，test-cases M6）────────
//
// 用例清单（任务 4.5 + test-cases M6）：
//   M6.1/M6.4 H0 最可信 / 全非零 insufficient 合法 | M6.3 显眼假设 refuted 且
//   counter 保留 | M6.5 拆分/合并 derived_from 可追溯
//   非法 assessment / 坏 JSON 重试一次；仍失败 error 不编造
//   web/page quote 不在工具原文剔除 | method 绝不作 source | 幽灵 lane 剔除
//   悬空极性剔除 | 同向新闻 high supported 机械降级 + boundary/gap
//   无 argument/depth/system_reframe | prompt 契约（方法留痕/研究结果入，
//   旧论文结构出）| meta 稳定原因码不泄内部 err
//   M12：只缺根对象最后一个 } 可证明修复；内部截断/错配/未闭字符串/
//   尾随正文/缺 lane_refs 仍严格拒绝，修复后结构校验不放宽
//   M13：lane 证据缺 ref 时读数值 lane_id 别名（float64 整数 (0,2^53) 归一
//   十进制，显式 ref 优先、非法 ref 不被掩盖）；存活证据 supports/counters
//   极性按 first-seen 并集回填假设引用（受 max refs cap）；supported 缺存活
//   support、refuted/weakened 缺存活 counter = 清洗后结构失败（重试→0 行），
//   plausible/insufficient 不强制

// ── 测试素材 ────────────────────────────────────────────────────────────────

func synthesisTestStage() *boardHypothesisStageResult {
	return &boardHypothesisStageResult{
		Question:    investigationTestQuestion(),
		Methods:     boardMethodSelection{Candidates: []boardMethodCandidate{}, Selected: []boardMethodSelected{}, Dropped: []DroppedAnalysisMethod{}},
		MethodRefs:  []AnalysisMethodRef{},
		MethodCards: []AnalysisMethodCardTrace{},
		Hypotheses: boardHypothesisGeneration{
			Hypotheses: []boardHypothesis{
				{ID: "h0", Label: "无统一机制，可分别解释", IsNull: true, SupportNeeded: []string{"共同机制"}, DisconfirmNeeded: []string{"独立解释"}, Scope: "板块"},
				{ID: "h1", Label: "同一产业基金推动产能与招标", IsNull: false, SupportNeeded: []string{"基金公告"}, DisconfirmNeeded: []string{"独立资金明细"}, Scope: "近三个月"},
				{ID: "h2", Label: "政策补贴周期同步带动", IsNull: false, SupportNeeded: []string{"补贴文本"}, DisconfirmNeeded: []string{"时间线不重合"}, Scope: "政策周期"},
			},
		},
	}
}

func synthesisTestResearch() *BoardInvestigationResearchResult {
	loop := &AgentLoopResult{
		Topic: "两条泳道是否由同一资金驱动",
		ToolCalls: []ToolCallRecord{
			{Step: 1, Tool: "get_lane_detail", Args: map[string]any{"lane_id": 1}, ResultFull: "泳道1近期演进：招标公告与产能进展详情", Purpose: ResearchPurposeNeutral, Outcome: toolCallOutcomeOK},
			{Step: 2, Tool: "web_search", Args: map[string]any{"query": "产业基金 公告"}, ResultFull: `{"query":"产业基金 公告","hit_count":1,"results":[{"title":"基金公告","url":"https://example.com/a","snippet":"基金公告原文摘录ABC"}]}`, Purpose: ResearchPurposeSupport, HypothesisIDs: []string{"h1"}, Outcome: toolCallOutcomeOK},
		},
		FinalData: "按假设分组：h1 有基金公告线索；h2 无补贴时间线证据；反证检索未发现独立资金来源明细。",
	}
	return &BoardInvestigationResearchResult{
		Loop:     loop,
		Coverage: boardResearchCoverage{NeutralAttempted: true, CounterAttemptedByHypothesis: []string{"h1", "h2"}},
		Gaps:     []boardResearchGap{{Reason: researchGapToolUnavailable, Tool: "fetch_page"}},
	}
}

// cannedSynthesisJSON：H0 最可信（plausible/medium）、非零全 insufficient/refuted。
const cannedSynthesisJSON = `{"hypotheses":[
 {"id":"h0","label":"无统一机制，变化可分别解释","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块全域","support_evidence":["e2"],"counter_evidence":[],"gaps":[]},
 {"id":"h1","label":"同一产业基金推动两泳道","is_null":false,"assessment":"insufficient","confidence":"low","scope":"近三个月","support_evidence":[],"counter_evidence":[],"gaps":["资金来源明细未取得"]},
 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,"assessment":"refuted","confidence":"medium","scope":"政策周期","support_evidence":[],"counter_evidence":["e1"],"gaps":[]}],
 "conclusion":{"summary":"目前各泳道变化应分别理解","confidence":"medium","scope":"两条泳道","boundary":"资金数据缺失，无法区分共同驱动"},
 "evidence_chain":[
  {"id":"e1","source_type":"web","url":"https://example.com/a","quote":"基金公告原文摘录ABC","institution":"示例研究所","date":"2026-08-20","supports":[],"counters":["h2"]},
  {"id":"e2","source_type":"lane","ref":"1","lane_note":"招标与产能详情","supports":["h0"],"counters":[]}],
 "lane_refs":[{"lane_id":1,"note":"主泳道"}]}`

func synthesisTestQuestion() BoardInvestigationQuestion { return investigationTestQuestion() }

// parseSynthesis is the common parse entry with test-standard inputs.
func parseSynthesis(t *testing.T, llm string) (*boardInvestigationPayload, error) {
	t.Helper()
	return parseSynthesisWithResearch(t, llm, synthesisTestResearch())
}

// parseSynthesisWithResearch parses llm against a caller-provided research
// record（同源核对需要构造特定的工具调用集）。
func parseSynthesisWithResearch(t *testing.T, llm string, res *BoardInvestigationResearchResult) (*boardInvestigationPayload, error) {
	t.Helper()
	parsed, err := ParseJSONResponse(llm)
	if err != nil {
		t.Fatalf("parse llm json: %v", err)
	}
	stage := synthesisTestStage()
	initial := map[string]bool{}
	for _, h := range stage.Hypotheses.Hypotheses {
		initial[h.ID] = true
	}
	toolCalls := []ToolCallRecord{}
	if res.Loop != nil {
		toolCalls = append(toolCalls, res.Loop.ToolCalls...)
	}
	return parseBoardInvestigationSynthesis(parsed, initial, []uint{1, 2}, 0, nil, toolCalls)
}

func mustParseSynthesis(t *testing.T, llm string) *boardInvestigationPayload {
	t.Helper()
	payload, err := parseSynthesis(t, llm)
	if err != nil {
		t.Fatalf("parseBoardInvestigationSynthesis: %v", err)
	}
	return payload
}

// ── M6.1/M6.4/M6.3：H0 最可信、全非零不足、refuted 反证保留、无论文结构 ──────

func TestBoardSynthesis_ParseH0MostCredibleAndNoThesisSchema(t *testing.T) {
	p := mustParseSynthesis(t, cannedSynthesisJSON)
	if len(p.Hypotheses) != 3 {
		t.Fatalf("three hypotheses expected: %+v", p.Hypotheses)
	}
	h0, h1, h2 := p.Hypotheses[0], p.Hypotheses[1], p.Hypotheses[2]
	if h0.Assessment != HypothesisPlausible || !h0.IsNull {
		t.Fatalf("H0 plausible allowed as most credible: %+v", h0)
	}
	if h1.Assessment != HypothesisInsufficient || h2.Assessment != HypothesisRefuted {
		t.Fatalf("all-non-zero insufficient/refuted is legal: %+v %+v", h1, h2)
	}
	// M6.3：反证证据保留在 refuted 假设上。
	if len(h2.CounterEvidence) != 1 || h2.CounterEvidence[0] != "e1" {
		t.Fatalf("counter evidence must survive on refuted hypothesis: %+v", h2)
	}
	// conclusion 四字段齐全。
	if p.Conclusion.Summary == "" || p.Conclusion.Confidence != "medium" || p.Conclusion.Scope == "" || p.Conclusion.Boundary == "" {
		t.Fatalf("conclusion must carry summary/confidence/scope/boundary: %+v", p.Conclusion)
	}
	// 调查 schema 无旧论文字段。
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var keys map[string]any
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for banned := range map[string]bool{"thesis": true, "argument": true, "depth": true, "system_reframe": true, "mechanism_layers": true, "historical_analogy": true} {
		if _, ok := keys[banned]; ok {
			t.Fatalf("investigation schema must not carry legacy field %q: %s", banned, data)
		}
	}
	if p.Scope != "board" || p.ResultKind != "board_investigation" {
		t.Fatalf("scope/kind must be fixed: %+v", p)
	}
	// 方法引用不由 LLM 决定（parser 永不读 method_refs）。
	if len(p.MethodRefs) != 0 {
		t.Fatalf("parser must not adopt LLM method_refs: %+v", p.MethodRefs)
	}
}

// ── M6.5：拆分/合并 derived_from 追溯 + 未知引用清洗 ────────────────────────

func TestBoardSynthesis_SplitMergeDerivedFromScrubbed(t *testing.T) {
	p := mustParseSynthesis(t, `{"hypotheses":[
	 {"id":"h1a","label":"拆分A：基金推动产能","is_null":false,"derived_from":["h1"],"assessment":"plausible","confidence":"low","scope":"产能","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"hm","label":"合并：基金与补贴同源推动","is_null":false,"derived_from":["h1","h2"],"assessment":"insufficient","confidence":"low","scope":"两泳道","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h0","label":"无统一机制","is_null":true,"derived_from":["h9","幽灵"],"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":[],"counter_evidence":[],"gaps":[]}],
	 "conclusion":{"summary":"合并解释证据不足","confidence":"low","scope":"两条泳道","boundary":"尚无法区分"},
	 "evidence_chain":[],"lane_refs":[]}`)
	byID := map[string]boardInvestigationHypothesis{}
	for _, h := range p.Hypotheses {
		byID[h.ID] = h
	}
	if len(byID["h1a"].DerivedFrom) != 1 || byID["h1a"].DerivedFrom[0] != "h1" {
		t.Fatalf("split trace kept: %+v", byID["h1a"])
	}
	if len(byID["hm"].DerivedFrom) != 2 {
		t.Fatalf("merge trace kept: %+v", byID["hm"])
	}
	if len(byID["h0"].DerivedFrom) != 0 {
		t.Fatalf("unknown derived_from refs scrubbed: %+v", byID["h0"])
	}
}

// ── 非法 assessment：结构失败 → 纠错重试一次 → 成功 ─────────────────────────

func TestBoardSynthesis_IllegalAssessmentRetriesThenSucceeds(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: strings.Replace(cannedSynthesisJSON, `"assessment":"insufficient"`, `"assessment":"likely"`, 1)},
		{Content: cannedSynthesisJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	payload, meta, err := orch.synthesizeBoardInvestigation(context.Background(), "inv-sess",
		synthesisTestQuestion(), investigationTestBrief(), synthesisTestStage(), synthesisTestResearch(), []uint{1, 2}, 0, nil)
	if err != nil {
		t.Fatalf("retry must recover illegal assessment: %v", err)
	}
	if len(router.calls) != 2 {
		t.Fatalf("exactly two attempts expected, got %d", len(router.calls))
	}
	for i, c := range router.calls {
		if c.Operation != boardSynthesizeOperation {
			t.Fatalf("call %d operation: %s", i, c.Operation)
		}
		if c.SessionID != "inv-sess" {
			t.Fatalf("call %d session: %q", i, c.SessionID)
		}
		if !c.JSONMode {
			t.Fatalf("call %d must use JSON mode", i)
		}
	}
	if !strings.Contains(router.calls[1].Messages[0].Content, boardSynthesizeRetryLead) {
		t.Fatal("retry prompt must carry the corrective note")
	}
	if meta.Attempts != 2 || meta.RetryReason != synthesisRetryStructure {
		t.Fatalf("meta must trace the stable structure code: %+v", meta)
	}
	if payload.RetryReason != synthesisRetryStructure {
		t.Fatalf("payload retry reason must mirror meta: %+v", payload)
	}
	if payload.Hypotheses[1].Assessment != HypothesisInsufficient {
		t.Fatalf("second attempt payload used: %+v", payload.Hypotheses[1])
	}
}

// ── 坏 JSON 两次 → error，绝不机械编造调查结论 ────────────────────────────────

func TestBoardSynthesis_BadJSONTwiceErrorsNoFabrication(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: "这不是JSON"},
		{Content: `{"hypotheses": 还是坏`},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	payload, _, err := orch.synthesizeBoardInvestigation(context.Background(), "inv-sess",
		synthesisTestQuestion(), investigationTestBrief(), synthesisTestStage(), synthesisTestResearch(), []uint{1, 2}, 0, nil)
	if err == nil || payload != nil {
		t.Fatalf("two unusable attempts must error without fabrication, got %+v / %v", payload, err)
	}
	if len(router.calls) != 2 {
		t.Fatalf("no third attempt, got %d calls", len(router.calls))
	}
}

// ── M12：只修复可证明的单一根终止符 ─────────────────────────────────────────

func TestParseBoardSynthesisJSONResponse_TerminalRootRepairBoundary(t *testing.T) {
	missingRoot := strings.TrimSuffix(cannedSynthesisJSON, "}")
	missingLaneRefs := `{"hypotheses":[]`
	// Ends in `]` so the cheap suffix gate passes; the scanner itself must
	// reject `}` while `[` is on top of the delimiter stack.
	mismatched := `{"hypotheses":[},"lane_refs":[]`
	// The embedded invalid token makes the normal extractor fail. The scanner
	// then closes the root before seeing the final `]` and must reject an
	// empty-stack pop rather than panic or accept trailing delimiters.
	emptyStackPop := `{"hypotheses":tru,"lane_refs":[]} ]`
	escapedContent := `{"note":"含\"引号与\\反斜杠","lane_refs":[]`
	fencedMissingRoot := "```json\n" + missingRoot + "\n```"

	tests := []struct {
		name       string
		content    string
		wantRepair bool
		wantError  bool
	}{
		{name: "clean JSON", content: cannedSynthesisJSON},
		{name: "single missing root delimiter", content: missingRoot, wantRepair: true},
		{name: "fenced single missing root delimiter", content: fencedMissingRoot, wantRepair: true},
		{name: "escaped quote and backslash stay inside string", content: escapedContent, wantRepair: true},
		{name: "two missing delimiters", content: strings.TrimSuffix(missingRoot, "]"), wantError: true},
		{name: "internal delimiter mismatch after suffix gate", content: mismatched, wantError: true},
		{name: "empty stack closing delimiter", content: emptyStackPop, wantError: true},
		{name: "unterminated string", content: `{"hypotheses":[],"lane_refs":[{"note":"未闭合`, wantError: true},
		{name: "dangling escape inside string", content: `{"lane_refs":[{"note":"dangling\]`, wantError: true},
		{name: "trailing prose", content: missingRoot + " trailing", wantError: true},
		{name: "trailing prose ending in bracket", content: missingRoot + " trailing]", wantError: true},
		{name: "missing top level lane refs", content: missingLaneRefs, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, repaired, err := parseBoardSynthesisJSONResponse(tt.content)
			if tt.wantError {
				if err == nil || parsed != nil || repaired {
					t.Fatalf("unsafe candidate must stay rejected: parsed=%v repaired=%v err=%v", parsed, repaired, err)
				}
				return
			}
			if err != nil || parsed == nil {
				t.Fatalf("parse: %v", err)
			}
			if repaired != tt.wantRepair {
				t.Fatalf("repaired=%v want %v", repaired, tt.wantRepair)
			}
			if _, ok := parsed["lane_refs"].([]any); !ok {
				t.Fatalf("lane_refs must remain a complete top-level array: %#v", parsed["lane_refs"])
			}
		})
	}
}

func TestBoardSynthesis_CleanJSONDoesNotRecordRepair(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{{Content: cannedSynthesisJSON}}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	payload, meta, err := orch.synthesizeBoardInvestigation(context.Background(), "inv-sess",
		synthesisTestQuestion(), investigationTestBrief(), synthesisTestStage(), synthesisTestResearch(), []uint{1, 2}, 0, nil)
	if err != nil || payload == nil {
		t.Fatalf("clean synthesis must succeed: payload=%+v err=%v", payload, err)
	}
	if len(router.calls) != 1 || meta.Attempts != 1 || meta.RetryReason != "" || meta.RepairReason != "" {
		t.Fatalf("clean first attempt must not record retry or repair: calls=%d meta=%+v", len(router.calls), meta)
	}
}

func TestBoardSynthesis_SingleMissingRootDelimiterRepairedWithoutRetry(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{{Content: strings.TrimSuffix(cannedSynthesisJSON, "}")}}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	payload, meta, err := orch.synthesizeBoardInvestigation(context.Background(), "inv-sess",
		synthesisTestQuestion(), investigationTestBrief(), synthesisTestStage(), synthesisTestResearch(), []uint{1, 2}, 0, nil)
	if err != nil || payload == nil {
		t.Fatalf("single terminal root delimiter must repair safely: payload=%+v err=%v", payload, err)
	}
	if len(router.calls) != 1 || meta.Attempts != 1 {
		t.Fatalf("safe repair must not waste a corrective retry: calls=%d meta=%+v", len(router.calls), meta)
	}
	if meta.RepairReason != synthesisRepairTerminalRootDelimiter || meta.RetryReason != "" {
		t.Fatalf("generation meta must distinguish repair from retry: %+v", meta)
	}
}

func TestBoardSynthesis_RepairDoesNotBypassStructureValidation(t *testing.T) {
	illegal := strings.TrimSuffix(strings.Replace(cannedSynthesisJSON,
		`"assessment":"insufficient"`, `"assessment":"likely"`, 1), "}")
	router := &internalMockRouter{responses: []*airouter.ChatResult{{Content: illegal}, {Content: illegal}}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	payload, meta, err := orch.synthesizeBoardInvestigation(context.Background(), "inv-sess",
		synthesisTestQuestion(), investigationTestBrief(), synthesisTestStage(), synthesisTestResearch(), []uint{1, 2}, 0, nil)
	if err == nil || payload != nil {
		t.Fatalf("JSON repair must not weaken schema validation: payload=%+v err=%v", payload, err)
	}
	if len(router.calls) != 2 || meta.RetryReason != synthesisRetryStructure || meta.RepairReason != "" {
		t.Fatalf("invalid repaired candidates must use existing retry/failure path: calls=%d meta=%+v", len(router.calls), meta)
	}
}

// ── web/page 证据：quote 必须能在工具 ResultFull 中核到 ─────────────────────

func TestBoardSynthesis_WebPageQuoteVerification(t *testing.T) {
	p := mustParseSynthesis(t, `{"hypotheses":[
	 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":["e3"],"counter_evidence":[],"gaps":[]},
	 {"id":"h1","label":"基金推动","is_null":false,"assessment":"plausible","confidence":"low","scope":"近三月","support_evidence":["e1","e2"],"counter_evidence":[],"gaps":[]},
	 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策周期","support_evidence":[],"counter_evidence":[],"gaps":[]}],
	 "conclusion":{"summary":"基金线索存在但未定论","confidence":"low","scope":"两条泳道","boundary":"摘录可核查性不足"},
	 "evidence_chain":[
	  {"id":"e1","source_type":"web","url":"https://example.com/a","quote":"这段摘录不存在于任何工具结果","institution":"示例","date":"2026-08-20","supports":["h1"],"counters":[]},
	  {"id":"e2","source_type":"page","url":"https://example.com/a","quote":"基金公告原文摘录ABC","date":"2026-08-20","supports":["h1"],"counters":[]},
	  {"id":"e3","source_type":"news","ref":"ctx1","supports":["h0"],"counters":[]}],
	 "lane_refs":[]}`)
	// e1（quote 不可核）与 e2（缺 institution）剔除；e3 news 保留。
	if len(p.EvidenceChain) != 1 || p.EvidenceChain[0].ID != "e3" {
		t.Fatalf("unverifiable web/page evidence dropped item-wise: %+v", p.EvidenceChain)
	}
	// 假设引用同步清理（悬空引用不得残留）。
	h1 := p.Hypotheses[1]
	if len(h1.SupportEvidence) != 0 {
		t.Fatalf("hypothesis refs to dropped evidence must be cleaned: %+v", h1)
	}
	if len(p.Hypotheses[0].SupportEvidence) != 1 || p.Hypotheses[0].SupportEvidence[0] != "e3" {
		t.Fatalf("valid refs kept: %+v", p.Hypotheses[0])
	}
}

// ── 方法卡绝不可作证据来源 ──────────────────────────────────────────────────

func TestBoardSynthesis_MethodSourceNeverEvidence(t *testing.T) {
	p := mustParseSynthesis(t, `{"hypotheses":[
	 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":["e1","e2"],"counter_evidence":[],"gaps":[]},
	 {"id":"h1","label":"基金推动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策周期","support_evidence":[],"counter_evidence":[],"gaps":[]}],
	 "conclusion":{"summary":"证据不足","confidence":"low","scope":"板块","boundary":"无可用证据"},
	 "evidence_chain":[{"id":"e1","source_type":"method","ref":"11","supports":["h0"],"counters":[]},
	  {"id":"e2","source_type":"news","ref":"ctx1","supports":["h0"],"counters":[]}],
	 "lane_refs":[]}`)
	if len(p.EvidenceChain) != 1 || p.EvidenceChain[0].SourceType != "news" {
		t.Fatalf("method source must be dropped, news kept: %+v", p.EvidenceChain)
	}
	if p.Hypotheses[0].SupportEvidence[0] != "e2" {
		t.Fatalf("hypothesis refs must point at surviving evidence: %+v", p.Hypotheses[0])
	}
}

// ── 幽灵 lane：证据 ref 与 lane_refs 白名单外剔除 ────────────────────────────

func TestBoardSynthesis_GhostLaneDropped(t *testing.T) {
	p := mustParseSynthesis(t, `{"hypotheses":[
	 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":["e1"],"counter_evidence":[],"gaps":[]},
	 {"id":"h1","label":"基金推动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策周期","support_evidence":[],"counter_evidence":[],"gaps":[]}],
	 "conclusion":{"summary":"内部事实支持分别理解","confidence":"medium","scope":"两条泳道","boundary":"无"},
	 "evidence_chain":[
	  {"id":"e1","source_type":"lane","ref":"2","lane_note":"泳道二事实","supports":["h0"],"counters":[]},
	  {"id":"e2","source_type":"lane","ref":"77","lane_note":"幽灵泳道","supports":["h0"],"counters":[]}],
	 "lane_refs":[{"lane_id":77,"note":"幽灵"},{"lane_id":2,"note":"合法"}]}`)
	if len(p.EvidenceChain) != 1 || p.EvidenceChain[0].Ref != "2" {
		t.Fatalf("ghost lane evidence dropped, whitelisted kept: %+v", p.EvidenceChain)
	}
	if len(p.LaneRefs) != 1 || p.LaneRefs[0].LaneID != 2 {
		t.Fatalf("ghost lane_refs scrubbed: %+v", p.LaneRefs)
	}
}

// ── 悬空极性：supports/counters 指向不存在假设 → 证据剔除 ───────────────────

func TestBoardSynthesis_DanglingPolarityDropped(t *testing.T) {
	p := mustParseSynthesis(t, `{"hypotheses":[
	 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h1","label":"基金推动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策周期","support_evidence":[],"counter_evidence":[],"gaps":[]}],
	 "conclusion":{"summary":"极性不明证据被剔除","confidence":"low","scope":"板块","boundary":"无"},
	 "evidence_chain":[
	  {"id":"e1","source_type":"news","ref":"ctx1","supports":["hX"],"counters":[]},
	  {"id":"e2","source_type":"news","ref":"ctx2","supports":[],"counters":["h0"]}],
	 "lane_refs":[]}`)
	if len(p.EvidenceChain) != 1 || p.EvidenceChain[0].ID != "e2" {
		t.Fatalf("evidence without valid polarity dropped: %+v", p.EvidenceChain)
	}
}

// ── 机械质量护栏：同向新闻转述不得 high supported ───────────────────────────

func TestBoardSynthesis_SameDirectionNewsHighDowngraded(t *testing.T) {
	res := synthesisTestResearch()

	// 情形 A：只有 news 证据支持（research 有 counter）→ 降级。
	newsOnly := `{"hypotheses":[
	 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h1","label":"基金推动","is_null":false,"assessment":"supported","confidence":"high","scope":"近三月","support_evidence":["e1"],"counter_evidence":[],"gaps":[]},
	 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策周期","support_evidence":[],"counter_evidence":[],"gaps":[]}],
	 "conclusion":{"summary":"同向转述较多","confidence":"high","scope":"两泳道","boundary":"原始边界"},
	 "evidence_chain":[{"id":"e1","source_type":"news","ref":"ctx1","supports":["h1"],"counters":[]}],
	 "lane_refs":[]}`
	p := mustParseSynthesis(t, newsOnly)
	applyInvestigationQualityGuard(p, res)
	h1 := p.Hypotheses[1]
	if h1.Confidence != "medium" {
		t.Fatalf("news-only high supported must downgrade to medium: %+v", h1)
	}
	if len(h1.Gaps) == 0 || !strings.Contains(h1.Gaps[len(h1.Gaps)-1], "护栏") {
		t.Fatalf("downgrade must be traceable in gaps: %+v", h1)
	}
	if !strings.Contains(p.Conclusion.Boundary, "质量护栏") {
		t.Fatalf("downgrade must be noted in boundary: %q", p.Conclusion.Boundary)
	}

	// 情形 B：有可核查 web 证据但 research 未对该假设做过 counter → 降级。
	noCounter := *res
	noCounter.Coverage = boardResearchCoverage{NeutralAttempted: true, CounterAttemptedByHypothesis: []string{"h2"}}
	webNoCounter := strings.Replace(newsOnly,
		`{"id":"e1","source_type":"news","ref":"ctx1","supports":["h1"],"counters":[]}`,
		`{"id":"e1","source_type":"web","url":"https://example.com/a","quote":"基金公告原文摘录ABC","institution":"示例","date":"2026-08-20","supports":["h1"],"counters":[]}`, 1)
	p = mustParseSynthesis(t, webNoCounter)
	applyInvestigationQualityGuard(p, &noCounter)
	if p.Hypotheses[1].Confidence != "medium" {
		t.Fatalf("missing counter attempt must downgrade: %+v", p.Hypotheses[1])
	}

	// 情形 C（正控）：可核查 web 证据 + counter 已尝试 → high 保留。
	p = mustParseSynthesis(t, webNoCounter)
	applyInvestigationQualityGuard(p, res)
	if p.Hypotheses[1].Confidence != "high" {
		t.Fatalf("verifiable evidence + counter attempted keeps high: %+v", p.Hypotheses[1])
	}
}

// ── derived_from 命中 counter 覆盖也算做过反证（拆分后的假设）────────────────

func TestBoardSynthesis_QualityGuardFollowsDerivedFrom(t *testing.T) {
	res := synthesisTestResearch() // counter 覆盖 h1、h2
	p := mustParseSynthesis(t, `{"hypotheses":[
	 {"id":"h1a","label":"拆分：基金只推动产能","is_null":false,"derived_from":["h1"],"assessment":"supported","confidence":"high","scope":"产能","support_evidence":["e1"],"counter_evidence":[],"gaps":[]},
	 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策周期","support_evidence":[],"counter_evidence":[],"gaps":[]}],
	 "conclusion":{"summary":"拆分后基金线索可核查","confidence":"medium","scope":"产能","boundary":"仅覆盖产能泳道"},
	 "evidence_chain":[{"id":"e1","source_type":"web","url":"https://example.com/a","quote":"基金公告原文摘录ABC","institution":"示例","date":"2026-08-20","supports":["h1a"],"counters":[]}],
	 "lane_refs":[]}`)
	applyInvestigationQualityGuard(p, res)
	if p.Hypotheses[0].Confidence != "high" {
		t.Fatalf("counter coverage traced via derived_from: %+v", p.Hypotheses[0])
	}
}

// ── prompt 契约：研究结果与方法留痕入，旧论文结构出 ─────────────────────────

func TestBoardSynthesis_PromptContract(t *testing.T) {
	stage := synthesisTestStage()
	stage.MethodPrompt = "CONTENT-CAUSAL-STEP 检查每一环传导证据"
	stage.MethodRefs = []AnalysisMethodRef{{ID: 11, Title: "因果链检验", ContentHash: "abc123"}}
	stage.Methods.Selected = []boardMethodSelected{{ID: 11, Title: "因果链检验", Reason: "适配"}}
	stage.Methods.Dropped = []DroppedAnalysisMethod{
		{ID: 22, Title: "历史对照", Reason: "budget_exceeded"},
		{ID: 33, Title: "修辞卡", Reason: "content_noncompliant"},
	}
	prompt := assembleBoardSynthesizePrompt(synthesisTestQuestion(), investigationTestBrief(), stage, synthesisTestResearch(), []uint{1, 2})
	for _, want := range []string{
		"两条泳道是否由同一资金驱动",    // 问题
		"三条泳道各有进展",         // 父简报投影 summary
		"同一产业基金推动产能与招标",    // 初始假设 label（来自 stage）
		"按假设分组：h1 有基金公告线索", // research FinalData
		"web_search", "support", "h1", // 工具调用注记
		"基金公告原文摘录ABC",                    // 工具结果原文（供 quote 逐字核对）
		"tool_unavailable", "fetch_page", // gap 留痕
		"counter_attempted", "neutral", // coverage
		"CONTENT-CAUSAL-STEP", "因果链检验", // 实际注入方法正文
		"budget_exceeded", "content_noncompliant", // 舍弃机码
		"白名单", // lane 白名单
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("synthesize prompt missing %q", want)
		}
	}
	for _, banned := range []string{
		"system_reframe", "mechanism_layers", "historical_analogy", // 旧论文 schema 键
		"内部看美国", "reference_roles", // 旧作者画像
		`"thesis"`, `"argument"`, // 旧 schema 键（带引号，避免误伤 hypothesis 子串）
	} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("synthesize prompt must not require %q", banned)
		}
	}
}

// ── meta 稳定原因码：完整 err 不进 snapshot ──────────────────────────────────

func TestBoardSynthesis_MetaStableReasonCodeNoLeak(t *testing.T) {
	leaky := errors.New("dial tcp 10.0.0.7:5432: connection refused; pq: SELECT * FROM ai_call_logs")
	router := &chatErrRouter{internalMockRouter: &internalMockRouter{responses: []*airouter.ChatResult{{Content: cannedSynthesisJSON}}}, err: leaky}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	payload, meta, err := orch.synthesizeBoardInvestigation(context.Background(), "inv-sess",
		synthesisTestQuestion(), investigationTestBrief(), synthesisTestStage(), synthesisTestResearch(), []uint{1, 2}, 0, nil)
	if err != nil {
		t.Fatalf("chat error must recover on retry: %v", err)
	}
	if meta.RetryReason != synthesisRetryChat {
		t.Fatalf("chat error must trace stable code %q: %+v", synthesisRetryChat, meta)
	}
	metaJSON, _ := json.Marshal(meta)
	for _, banned := range []string{"10.0.0.7", "pq:", "SELECT", "dial tcp"} {
		if strings.Contains(string(metaJSON), banned) {
			t.Fatalf("synthesis meta must not leak internal error detail %q: %s", banned, metaJSON)
		}
	}
	if payload == nil || len(payload.Hypotheses) != 3 {
		t.Fatalf("payload recovered on second attempt: %+v", payload)
	}
}

// ── 修复上一 review Minor：hypothesize retry snapshot 用稳定原因码 ────────────

func TestBoardHypothesis_RetryReasonStableCodeNoLeak(t *testing.T) {
	// 第一次坏 JSON → 第二次合法：RetryReason 必须是稳定码 parse_error。
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: "坏JSON导致 pq: internal 10.0.0.9 泄漏文本不在快照"},
		{Content: cannedHypothesesJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	gen, err := orch.generateBoardHypotheses(context.Background(), "inv-sess", investigationTestQuestion(), investigationTestBrief(), "")
	if err != nil {
		t.Fatalf("parse failure must recover on retry: %v", err)
	}
	if gen.RetryReason != hypothesisRetryParse {
		t.Fatalf("retry reason must be stable code %q, got %q", hypothesisRetryParse, gen.RetryReason)
	}
	genJSON, _ := json.Marshal(gen)
	for _, banned := range []string{"pq:", "10.0.0.9", "坏JSON"} {
		if strings.Contains(string(genJSON), banned) {
			t.Fatalf("hypothesis generation meta must not carry raw error text %q: %s", banned, genJSON)
		}
	}

	// 传输错误 → chat_error；无 H0 → no_null_hypothesis。
	leaky := fmt.Errorf("dial tcp 10.0.0.7:5432 refused")
	errRouter := &chatErrRouter{internalMockRouter: &internalMockRouter{responses: []*airouter.ChatResult{{Content: cannedHypothesesJSON}}}, err: leaky}
	orch2 := &OrchestratorService{airouter: errRouter, capability: internalTestCap}
	gen2, err := orch2.generateBoardHypotheses(context.Background(), "inv-sess", investigationTestQuestion(), investigationTestBrief(), "")
	if err != nil {
		t.Fatalf("chat error must recover on retry: %v", err)
	}
	if gen2.RetryReason != hypothesisRetryChat {
		t.Fatalf("chat retry reason = %q, want %q", gen2.RetryReason, hypothesisRetryChat)
	}

	noH0Router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: noH0HypothesesJSON},
		{Content: noH0HypothesesJSON},
	}}
	orch3 := &OrchestratorService{airouter: noH0Router, capability: internalTestCap}
	gen3, err := orch3.generateBoardHypotheses(context.Background(), "inv-sess", investigationTestQuestion(), investigationTestBrief(), "")
	if err != nil {
		t.Fatalf("mechanical H0 path: %v", err)
	}
	if gen3.RetryReason != hypothesisRetryNoH0 {
		t.Fatalf("no-H0 retry reason = %q, want %q", gen3.RetryReason, hypothesisRetryNoH0)
	}
}

// ── 同源核对（review 4.5 Important）：quote 与 url 必须绑定同一工具来源 ──────

// crossSourceResearch 提供四条边界分明的工具调用：
// A=web_search（对象 url=a.example/x，snippet 含「来源A对象里的原文摘录」）
// B=web_search（对象 url=b.example/y，snippet 是另一段文本）
// C=fetch_page（args.url=c.example/doc，main_text 含「文档正文里的一段原文」）
// D=fetch_page（args.url=err.example/z，错误 JSON）
func crossSourceResearch() *BoardInvestigationResearchResult {
	loop := &AgentLoopResult{
		Topic: "两条泳道是否由同一资金驱动",
		ToolCalls: []ToolCallRecord{
			{Step: 1, Tool: "web_search", Args: map[string]any{"query": "甲"}, ResultFull: `{"query":"甲","hit_count":1,"results":[{"title":"来源A","url":"https://a.example/x","snippet":"来源A对象里的原文摘录"}]}`, Outcome: toolCallOutcomeOK},
			{Step: 2, Tool: "web_search", Args: map[string]any{"query": "乙"}, ResultFull: `{"query":"乙","hit_count":1,"results":[{"title":"来源B","url":"https://b.example/y","snippet":"来源B对象里的另一段文本"}]}`, Outcome: toolCallOutcomeOK},
			{Step: 3, Tool: "fetch_page", Args: map[string]any{"url": "https://c.example/doc"}, ResultFull: `{"title":"文档","url":"https://c.example/doc","main_text":"文档正文里的一段原文"}`, Outcome: toolCallOutcomeOK},
			{Step: 4, Tool: "fetch_page", Args: map[string]any{"url": "https://err.example/z"}, ResultFull: `{"error":"fetch_page 失败: timeout"}`, Outcome: toolCallOutcomeError},
		},
		FinalData: "素材已汇总。",
	}
	return &BoardInvestigationResearchResult{
		Loop:     loop,
		Coverage: boardResearchCoverage{NeutralAttempted: true, CounterAttemptedByHypothesis: []string{"h1", "h2"}},
	}
}

func TestBoardSynthesis_EvidenceQuoteBoundToSameToolSource(t *testing.T) {
	p, err := parseSynthesisWithResearch(t, `{"hypotheses":[
	 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h1","label":"基金推动","is_null":false,"assessment":"plausible","confidence":"low","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策周期","support_evidence":[],"counter_evidence":[],"gaps":[]}],
	 "conclusion":{"summary":"可核查来源有限","confidence":"low","scope":"两条泳道","boundary":"部分摘录无法绑定来源"},
	 "evidence_chain":[
	  {"id":"e_mixed","source_type":"web","url":"https://b.example/y","quote":"来源A对象里的原文摘录","institution":"示例","date":"2026-08-20","supports":["h1"],"counters":[]},
	  {"id":"e_web_ok","source_type":"web","url":"https://a.example/x","quote":"来源A对象里的原文摘录","institution":"示例","date":"2026-08-20","supports":["h1"],"counters":[]},
	  {"id":"e_page_wrong_args","source_type":"page","url":"https://other.example/doc","quote":"文档正文里的一段原文","institution":"示例","date":"2026-08-20","supports":["h1"],"counters":[]},
	  {"id":"e_page_err_call","source_type":"page","url":"https://err.example/z","quote":"timeout","institution":"示例","date":"2026-08-20","supports":["h1"],"counters":[]},
	  {"id":"e_page_ok","source_type":"page","url":"https://c.example/doc","quote":"文档正文里的一段原文","institution":"示例","date":"2026-08-20","supports":["h1"],"counters":[]}],
	 "lane_refs":[]}`, crossSourceResearch())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	kept := map[string]bool{}
	for _, e := range p.EvidenceChain {
		kept[e.ID] = true
	}
	// A quote + B URL：两条 ResultFull 并集里都出现过，但不在同一对象 → 拒绝。
	if kept["e_mixed"] {
		t.Fatal("web evidence with URL from object B and quote from object A must be rejected")
	}
	// 同对象 web：url 与 quote 同在来源 A 的结果对象 → 通过。
	if !kept["e_web_ok"] {
		t.Fatal("web evidence with url+quote in the SAME result object must pass")
	}
	// fetch_page args.url 与 evidence.url 不一致（quote 文本确实在 C 的结果里）→ 拒绝。
	if kept["e_page_wrong_args"] {
		t.Fatal("page evidence whose url mismatches the fetch_page args.url must be rejected")
	}
	// 错误调用（error JSON）永不作证据来源 → 拒绝。
	if kept["e_page_err_call"] {
		t.Fatal("evidence bound to an errored tool call must be rejected")
	}
	// 正确 page：args.url 一致 + quote 是该调用结果子串 → 通过。
	if !kept["e_page_ok"] {
		t.Fatal("page evidence with matching args.url and in-result quote must pass")
	}
}

func TestBoardSynthesis_URLNormalizationSafeEquivalences(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"host case + default port + trailing slash", "HTTPS://A.Example.COM:443/x/", "https://a.example.com/x", true},
		{"http default port dropped", "http://example.com:80", "http://example.com", true},
		{"trailing slash only", "https://example.com/a/", "https://example.com/a", true},
		{"different query never merges", "https://example.com/a?x=1", "https://example.com/a?x=2", false},
		{"query vs no query never merges", "https://example.com/a?x=1", "https://example.com/a", false},
		{"different path", "https://example.com/a", "https://example.com/b", false},
		{"different host", "https://a.example/x", "https://b.example/x", false},
		{"non-default port kept", "https://example.com:8443/x", "https://example.com/x", false},
	}
	for _, tc := range cases {
		na, nb := normalizeToolURL(tc.a), normalizeToolURL(tc.b)
		if got := na != "" && na == nb; got != tc.want {
			t.Fatalf("%s: normalizeToolURL(%q)=%q vs (%q)=%q, want equal=%v", tc.name, tc.a, na, tc.b, nb, tc.want)
		}
	}
	if normalizeToolURL("not a url") != "" || normalizeToolURL("example.com/x") != "" || normalizeToolURL("") != "" {
		t.Fatal("unparsable/relative URLs must normalize to empty (never bind evidence)")
	}

	// 端到端：大小写/默认端口/尾斜杠差异不阻断合法绑定。
	p, err := parseSynthesisWithResearch(t, `{"hypotheses":[
	 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h1","label":"基金推动","is_null":false,"assessment":"plausible","confidence":"low","scope":"近三月","support_evidence":["e1"],"counter_evidence":[],"gaps":[]},
	 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策周期","support_evidence":[],"counter_evidence":[],"gaps":[]}],
	 "conclusion":{"summary":"来源可核查","confidence":"low","scope":"两条泳道","boundary":"无"},
	 "evidence_chain":[{"id":"e1","source_type":"web","url":"https://A.Example:443/x/","quote":"来源A对象里的原文摘录","institution":"示例","date":"2026-08-20","supports":["h1"],"counters":[]}],
	 "lane_refs":[]}`, crossSourceResearch())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(p.EvidenceChain) != 1 || p.EvidenceChain[0].ID != "e1" {
		t.Fatalf("safe URL equivalences must not block legitimate binding: %+v", p.EvidenceChain)
	}
}

// ── 初始假设覆盖：无痕消失 = 结构失败 → 重试；两次仍失败 = error ─────────────

func TestBoardSynthesis_InitialHypothesisCoverageRetryThenError(t *testing.T) {
	// h1 无痕消失（既无同 id 也无 derived_from）→ 结构失败。
	vanishH1 := strings.Replace(cannedSynthesisJSON,
		` {"id":"h1","label":"同一产业基金推动两泳道","is_null":false,"assessment":"insufficient","confidence":"low","scope":"近三个月","support_evidence":[],"counter_evidence":[],"gaps":["资金来源明细未取得"]},`, "", 1)
	_, err := parseSynthesis(t, vanishH1)
	if err == nil || !strings.Contains(err.Error(), "h1") {
		t.Fatalf("vanished initial hypothesis must be a structural failure, got %v", err)
	}

	// H0 同样不可无痕消失。
	vanishH0 := strings.Replace(cannedSynthesisJSON,
		` {"id":"h0","label":"无统一机制，变化可分别解释","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块全域","support_evidence":["e2"],"counter_evidence":[],"gaps":[]},`, "", 1)
	_, err = parseSynthesis(t, vanishH0)
	if err == nil || !strings.Contains(err.Error(), "h0") {
		t.Fatalf("vanished H0 must be a structural failure, got %v", err)
	}

	// 重试恢复：第一次 h1 消失，第二次完整 → 成功且 meta 记录 invalid_structure。
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: vanishH1},
		{Content: cannedSynthesisJSON},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	payload, meta, err := orch.synthesizeBoardInvestigation(context.Background(), "inv-sess",
		synthesisTestQuestion(), investigationTestBrief(), synthesisTestStage(), synthesisTestResearch(), []uint{1, 2}, 0, nil)
	if err != nil {
		t.Fatalf("retry must recover coverage failure: %v", err)
	}
	if meta.Attempts != 2 || meta.RetryReason != synthesisRetryStructure {
		t.Fatalf("meta must trace the coverage structural retry: %+v", meta)
	}
	if !strings.Contains(router.calls[1].Messages[0].Content, boardSynthesizeRetryLead) {
		t.Fatal("retry prompt must carry the corrective note")
	}
	if payload == nil || len(payload.Hypotheses) != 3 {
		t.Fatalf("second attempt payload used: %+v", payload)
	}

	// 两次都消失 → error（0 行），不静默丢假设。
	router2 := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: vanishH1},
		{Content: vanishH1},
	}}
	orch2 := &OrchestratorService{airouter: router2, capability: internalTestCap}
	payload2, _, err := orch2.synthesizeBoardInvestigation(context.Background(), "inv-sess",
		synthesisTestQuestion(), investigationTestBrief(), synthesisTestStage(), synthesisTestResearch(), []uint{1, 2}, 0, nil)
	if err == nil || payload2 != nil {
		t.Fatalf("two coverage failures must error, got %+v / %v", payload2, err)
	}
}

// ── lane ref 溢出：2^64+1 不得回绕命中合法 lane ─────────────────────────────

func TestBoardSynthesis_LaneRefOverflowRejected(t *testing.T) {
	// 白名单 []uint{1,2}；2^64+1 经旧手写数位循环会回绕成 1（合法 lane）。
	p, err := parseSynthesis(t, `{"hypotheses":[
	 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":["e1"],"counter_evidence":[],"gaps":[]},
	 {"id":"h1","label":"基金推动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策周期","support_evidence":[],"counter_evidence":[],"gaps":[]}],
	 "conclusion":{"summary":"内部事实核查","confidence":"low","scope":"两条泳道","boundary":"无"},
	 "evidence_chain":[
	  {"id":"e1","source_type":"lane","ref":"1","lane_note":"合法泳道","supports":["h0"],"counters":[]},
	  {"id":"e2","source_type":"lane","ref":"18446744073709551617","lane_note":"2^64+1 溢出","supports":["h0"],"counters":[]},
	  {"id":"e3","source_type":"lane","ref":"`+strings.Repeat("9", 40)+`","lane_note":"超长数字","supports":["h0"],"counters":[]}],
	 "lane_refs":[{"lane_id":18446744073709551617,"note":"溢出float"},{"lane_id":1.5,"note":"非整数"},{"lane_id":-3,"note":"负数"},{"lane_id":2,"note":"合法"}]}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(p.EvidenceChain) != 1 || p.EvidenceChain[0].Ref != "1" {
		t.Fatalf("overflow/overlong lane refs must be dropped (no wraparound): %+v", p.EvidenceChain)
	}
	if len(p.LaneRefs) != 1 || p.LaneRefs[0].LaneID != 2 {
		t.Fatalf("illegal lane_ids must be dropped, legal kept: %+v", p.LaneRefs)
	}
}

// ── 列表统一非 nil []：JSON 永不出 null ─────────────────────────────────────

func TestBoardSynthesis_ListsNeverNullInJSON(t *testing.T) {
	// LLM 完全省略 gaps/support_evidence/counter_evidence/derived_from 键。
	p := mustParseSynthesis(t, `{"hypotheses":[
	 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块"},
	 {"id":"h1","label":"基金推动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"近三月"},
	 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策周期"}],
	 "conclusion":{"summary":"证据不足","confidence":"low","scope":"板块","boundary":"无"},
	 "evidence_chain":[],"lane_refs":[]}`)
	for i, h := range p.Hypotheses {
		if h.Gaps == nil || h.SupportEvidence == nil || h.CounterEvidence == nil || h.DerivedFrom == nil {
			t.Fatalf("hypothesis %d lists must be non-nil", i)
		}
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	raw := string(data)
	for _, banned := range []string{`"gaps":null`, `"support_evidence":null`, `"counter_evidence":null`, `"derived_from":null`, `"evidence_chain":null`, `"lane_refs":null`} {
		if strings.Contains(raw, banned) {
			t.Fatalf("persisted JSON must never carry %s: %s", banned, raw)
		}
	}
	for _, want := range []string{`"gaps":[]`, `"support_evidence":[]`, `"counter_evidence":[]`, `"derived_from":[]`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("persisted JSON must carry empty list %s: %s", want, raw)
		}
	}
}

// ── M13：lane_id 别名归一 + 极性并集回填 + 终局一致性门 ─────────────────────
//
// 根因（result12 真实 job）：qwen 偶发漏发 lane 证据的 ref、把泳道编号放进
// 数值 lane_id 字段——旧 parser 按幽灵 ref 逐条 drop，两条 support 证据全灭，
// 假设引用归零后 supported/refuted 定论仍照常落库（矛盾快照）。

// parseSynthesisLaneAlias parses with a caller-provided lane whitelist
// （别名归一用例需要白名单覆盖 5 / 2^53 等特定编号；无工具调用——纯 lane 用例）。
func parseSynthesisLaneAlias(t *testing.T, llm string, lanes []uint) *boardInvestigationPayload {
	t.Helper()
	parsed, err := ParseJSONResponse(llm)
	if err != nil {
		t.Fatalf("parse llm json: %v", err)
	}
	stage := synthesisTestStage()
	initial := map[string]bool{}
	for _, h := range stage.Hypotheses.Hypotheses {
		initial[h.ID] = true
	}
	p, err := parseBoardInvestigationSynthesis(parsed, initial, lanes, 0, nil, nil)
	if err != nil {
		t.Fatalf("parseBoardInvestigationSynthesis: %v", err)
	}
	return p
}

// aliasHyps：h1 评估与引用占位；证据链占位（h0/h2 固定非定局评估）。
const aliasHyps = `{"hypotheses":[
 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":[],"counter_evidence":[],"gaps":[]},
 {"id":"h1","label":"基金推动","is_null":false,"assessment":"%s","confidence":"medium","scope":"近三月","support_evidence":%s,"counter_evidence":[],"gaps":[]},
 {"id":"h2","label":"政策补贴同步","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策","support_evidence":[],"counter_evidence":[],"gaps":[]}],
 "conclusion":{"summary":"别名归一核对","confidence":"low","scope":"两条泳道","boundary":"无"},
 "evidence_chain":[%s],
 "lane_refs":[]}`

func TestBoardSynthesis_LaneIDAliasFillsMissingRef(t *testing.T) {
	t.Run("missing ref normalized from whitelisted lane_id", func(t *testing.T) {
		// result12 原形：ref 整字段缺失、lane_id=5、h1 supported 且已引用 e1——
		// 旧行为证据被 drop → supported 引用归零仍落库；修复后别名归一存活。
		llm := fmt.Sprintf(aliasHyps, "supported", `["e1"]`,
			`{"id":"e1","source_type":"lane","lane_id":5,"lane_note":"泳道5事实","supports":["h1"],"counters":[]}`)
		p := parseSynthesisLaneAlias(t, llm, []uint{1, 2, 5})
		if len(p.EvidenceChain) != 1 || p.EvidenceChain[0].Ref != "5" {
			t.Fatalf("lane_id alias must normalize to decimal ref and survive: %+v", p.EvidenceChain)
		}
		h1 := p.Hypotheses[1]
		if len(h1.SupportEvidence) != 1 || h1.SupportEvidence[0] != "e1" {
			t.Fatalf("supported hypothesis keeps its evidence ref: %+v", h1)
		}
	})

	t.Run("empty ref filled by alias", func(t *testing.T) {
		llm := fmt.Sprintf(aliasHyps, "supported", "[]",
			`{"id":"e1","source_type":"lane","ref":"","lane_id":1,"supports":["h1"],"counters":[]}`)
		p := parseSynthesisLaneAlias(t, llm, []uint{1, 2, 5})
		if len(p.EvidenceChain) != 1 || p.EvidenceChain[0].Ref != "1" {
			t.Fatalf("empty ref must be filled from lane_id alias: %+v", p.EvidenceChain)
		}
	})

	t.Run("whitespace ref treated as missing", func(t *testing.T) {
		llm := fmt.Sprintf(aliasHyps, "insufficient", "[]",
			`{"id":"e1","source_type":"lane","ref":"   ","lane_id":2,"supports":["h0"],"counters":[]}`)
		p := parseSynthesisLaneAlias(t, llm, []uint{1, 2, 5})
		if len(p.EvidenceChain) != 1 || p.EvidenceChain[0].Ref != "2" {
			t.Fatalf("whitespace-only ref must fall back to lane_id alias: %+v", p.EvidenceChain)
		}
	})

	t.Run("explicit valid ref wins over conflicting lane_id", func(t *testing.T) {
		llm := fmt.Sprintf(aliasHyps, "insufficient", "[]",
			`{"id":"e1","source_type":"lane","ref":"1","lane_id":2,"supports":["h0"],"counters":[]}`)
		p := parseSynthesisLaneAlias(t, llm, []uint{1, 2, 5})
		if len(p.EvidenceChain) != 1 || p.EvidenceChain[0].Ref != "1" {
			t.Fatalf("non-empty ref must never be overridden by the alias: %+v", p.EvidenceChain)
		}
	})

	// 非法别名/显式幽灵 ref 一律 drop（含会截断/回绕命中合法 lane 的形态）。
	rejections := []struct {
		name  string
		ev    string
		lanes []uint
	}{
		{"explicit ghost ref not masked by valid lane_id",
			`{"id":"e1","source_type":"lane","ref":"99","lane_id":1,"supports":["h0"],"counters":[]}`, []uint{1, 2, 5}},
		{"lane_id zero",
			`{"id":"e1","source_type":"lane","lane_id":0,"supports":["h0"],"counters":[]}`, []uint{1, 2, 5}},
		{"lane_id negative",
			`{"id":"e1","source_type":"lane","lane_id":-3,"supports":["h0"],"counters":[]}`, []uint{1, 2, 5}},
		{"lane_id fractional must not floor",
			`{"id":"e1","source_type":"lane","lane_id":1.5,"supports":["h0"],"counters":[]}`, []uint{1, 2, 5}},
		{"lane_id at 2^53 boundary",
			`{"id":"e1","source_type":"lane","lane_id":9007199254740992,"supports":["h0"],"counters":[]}`, []uint{1, 2, 5, 9007199254740992}},
		{"lane_id string form rejected",
			`{"id":"e1","source_type":"lane","lane_id":"5","supports":["h0"],"counters":[]}`, []uint{1, 2, 5}},
		{"lane_id null rejected",
			`{"id":"e1","source_type":"lane","lane_id":null,"supports":["h0"],"counters":[]}`, []uint{1, 2, 5}},
	}
	for _, tt := range rejections {
		t.Run(tt.name, func(t *testing.T) {
			llm := fmt.Sprintf(aliasHyps, "insufficient", "[]", tt.ev)
			p := parseSynthesisLaneAlias(t, llm, tt.lanes)
			if len(p.EvidenceChain) != 0 {
				t.Fatalf("illegal alias/ref must be dropped item-wise: %+v", p.EvidenceChain)
			}
		})
	}
}

func TestBoardSynthesis_PolarityMergeFillsHypothesisRefs(t *testing.T) {
	p := mustParseSynthesis(t, `{"hypotheses":[
	 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":["e2"],"counter_evidence":[],"gaps":[]},
	 {"id":"h1","label":"基金推动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h2","label":"政策补贴同步","is_null":false,"assessment":"refuted","confidence":"medium","scope":"政策","support_evidence":[],"counter_evidence":["e3"],"gaps":[]}],
	 "conclusion":{"summary":"极性并集回填","confidence":"low","scope":"两条泳道","boundary":"无"},
	 "evidence_chain":[
	  {"id":"e1","source_type":"lane","ref":"1","supports":["h1"],"counters":[]},
	  {"id":"e2","source_type":"lane","ref":"2","supports":["h0"],"counters":[]},
	  {"id":"e3","source_type":"lane","ref":"1","supports":[],"counters":["h2"]},
	  {"id":"e4","source_type":"lane","ref":"2","supports":[],"counters":["h1"]}],
	 "lane_refs":[]}`)
	h0, h1, h2 := p.Hypotheses[0], p.Hypotheses[1], p.Hypotheses[2]
	if len(h0.SupportEvidence) != 1 || h0.SupportEvidence[0] != "e2" {
		t.Fatalf("declared refs stay single after first-seen union: %+v", h0)
	}
	if len(h1.SupportEvidence) != 1 || h1.SupportEvidence[0] != "e1" {
		t.Fatalf("support refs filled from evidence polarity: %+v", h1)
	}
	if len(h1.CounterEvidence) != 1 || h1.CounterEvidence[0] != "e4" {
		t.Fatalf("counter refs filled from evidence polarity: %+v", h1)
	}
	if len(h2.CounterEvidence) != 1 || h2.CounterEvidence[0] != "e3" {
		t.Fatalf("declared counter refs stay single after union: %+v", h2)
	}
	// 证据极性本身不被回填改写：e4 只 counters h1。
	var e4 *boardInvestigationEvidence
	for i := range p.EvidenceChain {
		if p.EvidenceChain[i].ID == "e4" {
			e4 = &p.EvidenceChain[i]
		}
	}
	if e4 == nil || len(e4.Supports) != 0 || len(e4.Counters) != 1 || e4.Counters[0] != "h1" {
		t.Fatalf("merge must not rewrite evidence polarity: %+v", e4)
	}

	t.Run("duplicate evidence ids are renamed and polarity refs follow", func(t *testing.T) {
		p := mustParseSynthesis(t, `{"hypotheses":[
		 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":[],"counter_evidence":[],"gaps":[]},
		 {"id":"h1","label":"基金推动","is_null":false,"assessment":"supported","confidence":"medium","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]},
		 {"id":"h2","label":"政策补贴同步","is_null":false,"assessment":"refuted","confidence":"medium","scope":"政策","support_evidence":[],"counter_evidence":[],"gaps":[]}],
		 "conclusion":{"summary":"重复 id 仍可追溯","confidence":"medium","scope":"板块","boundary":"无"},
		 "evidence_chain":[
		  {"id":"e1","source_type":"lane","ref":"1","supports":["h1"],"counters":[]},
		  {"id":"e1","source_type":"lane","ref":"2","supports":[],"counters":["h2"]}],
		 "lane_refs":[]}`)
		if len(p.EvidenceChain) != 2 || p.EvidenceChain[0].ID != "e1" || p.EvidenceChain[1].ID != "e1_2" {
			t.Fatalf("duplicate evidence ids must be deterministically unique: %+v", p.EvidenceChain)
		}
		if got := p.Hypotheses[1].SupportEvidence; len(got) != 1 || got[0] != "e1" {
			t.Fatalf("first evidence polarity must bind the first canonical id: %+v", got)
		}
		if got := p.Hypotheses[2].CounterEvidence; len(got) != 1 || got[0] != "e1_2" {
			t.Fatalf("renamed evidence polarity must bind the renamed id: %+v", got)
		}
	})

	t.Run("max refs cap respected", func(t *testing.T) {
		var declared []string
		var evsJSON []string
		for i := 1; i <= boardSynthesisMaxEvidenceRefs+1; i++ {
			id := fmt.Sprintf("e%d", i)
			evsJSON = append(evsJSON, fmt.Sprintf(`{"id":%q,"source_type":"lane","ref":"1","supports":["h0"],"counters":[]}`, id))
			if i <= boardSynthesisMaxEvidenceRefs {
				declared = append(declared, `"`+id+`"`)
			}
		}
		llm := `{"hypotheses":[
		 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":[` + strings.Join(declared, ",") + `],"counter_evidence":[],"gaps":[]},
		 {"id":"h1","label":"基金推动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]},
		 {"id":"h2","label":"政策补贴同步","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策","support_evidence":[],"counter_evidence":[],"gaps":[]}],
		 "conclusion":{"summary":"上限核对","confidence":"low","scope":"板块","boundary":"无"},
		 "evidence_chain":[` + strings.Join(evsJSON, ",") + `],"lane_refs":[]}`
		p := mustParseSynthesis(t, llm)
		h0 := p.Hypotheses[0]
		if len(h0.SupportEvidence) != boardSynthesisMaxEvidenceRefs {
			t.Fatalf("merged refs must respect the cap: %+v", h0.SupportEvidence)
		}
		overflow := fmt.Sprintf("e%d", boardSynthesisMaxEvidenceRefs+1)
		for _, id := range h0.SupportEvidence {
			if id == overflow {
				t.Fatalf("evidence past the cap must not be appended: %+v", h0.SupportEvidence)
			}
		}
	})
}

func TestBoardSynthesis_DefinitiveAssessmentConsistencyGate(t *testing.T) {
	cases := []struct {
		name        string
		h1          string
		evidence    string
		wantErr     string // 空 = 必须解析成功
		wantSupport []string
		wantCounter []string
	}{
		{
			name:     "supported without surviving support fails",
			h1:       `{"id":"h1","label":"基金推动两泳道","is_null":false,"assessment":"supported","confidence":"medium","scope":"近三月","support_evidence":["e1"],"counter_evidence":[],"gaps":[]}`,
			evidence: `{"id":"e1","source_type":"lane","ref":"77","supports":["h1"],"counters":[]}`,
			wantErr:  "supported",
		},
		{
			name:     "refuted without surviving counter fails",
			h1:       `{"id":"h1","label":"基金推动两泳道","is_null":false,"assessment":"refuted","confidence":"medium","scope":"近三月","support_evidence":[],"counter_evidence":["e1"],"gaps":[]}`,
			evidence: `{"id":"e1","source_type":"news","ref":"ctx1","supports":["hX"],"counters":["hY"]}`,
			wantErr:  "refuted",
		},
		{
			name:     "weakened without surviving counter fails",
			h1:       `{"id":"h1","label":"基金推动两泳道","is_null":false,"assessment":"weakened","confidence":"medium","scope":"近三月","support_evidence":[],"counter_evidence":["e1"],"gaps":[]}`,
			evidence: `{"id":"e1","source_type":"news","ref":"ctx1","supports":["hX"],"counters":["hY"]}`,
			wantErr:  "weakened",
		},
		{
			name:        "supported rescued by polarity merge",
			h1:          `{"id":"h1","label":"基金推动两泳道","is_null":false,"assessment":"supported","confidence":"medium","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]}`,
			evidence:    `{"id":"e1","source_type":"lane","ref":"1","supports":["h1"],"counters":[]}`,
			wantSupport: []string{"e1"},
		},
		{
			name:        "weakened rescued by polarity merge",
			h1:          `{"id":"h1","label":"基金推动两泳道","is_null":false,"assessment":"weakened","confidence":"medium","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]}`,
			evidence:    `{"id":"e1","source_type":"news","ref":"ctx1","supports":[],"counters":["h1"]}`,
			wantCounter: []string{"e1"},
		},
		{
			name:     "plausible without direct evidence allowed",
			h1:       `{"id":"h1","label":"基金推动两泳道","is_null":false,"assessment":"plausible","confidence":"medium","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]}`,
			evidence: ``,
		},
		{
			name:     "insufficient without direct evidence allowed",
			h1:       `{"id":"h1","label":"基金推动两泳道","is_null":false,"assessment":"insufficient","confidence":"low","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]}`,
			evidence: ``,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			llm := `{"hypotheses":[
			 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":[],"counter_evidence":[],"gaps":[]},
			 ` + tt.h1 + `,
			 {"id":"h2","label":"政策补贴同步","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策","support_evidence":[],"counter_evidence":[],"gaps":[]}],
			 "conclusion":{"summary":"一致性门","confidence":"low","scope":"两条泳道","boundary":"无"},
			 "evidence_chain":[` + tt.evidence + `],"lane_refs":[]}`
			payload, err := parseSynthesis(t, llm)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "基金推动两泳道") {
					t.Fatalf("definitive contradiction must be a structural failure naming the hypothesis, got %+v / %v", payload, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("non-definitive or evidence-backed assessments must parse: %v", err)
			}
			if tt.wantSupport != nil && (len(payload.Hypotheses[1].SupportEvidence) != len(tt.wantSupport) || payload.Hypotheses[1].SupportEvidence[0] != tt.wantSupport[0]) {
				t.Fatalf("merged support refs expected %v, got %+v", tt.wantSupport, payload.Hypotheses[1].SupportEvidence)
			}
			if tt.wantCounter != nil && (len(payload.Hypotheses[1].CounterEvidence) != len(tt.wantCounter) || payload.Hypotheses[1].CounterEvidence[0] != tt.wantCounter[0]) {
				t.Fatalf("merged counter refs expected %v, got %+v", tt.wantCounter, payload.Hypotheses[1].CounterEvidence)
			}
		})
	}
}

func TestBoardSynthesis_DefinitiveConsistencyTwiceErrors(t *testing.T) {
	// h1 supported 但无任何存活证据 → 两次一致性失败 → synthesisRetryStructure、
	// 2 calls、nil payload（0 行落库，不机械编造）。
	contradictory := `{"hypotheses":[
	 {"id":"h0","label":"无统一机制","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h1","label":"基金推动两泳道","is_null":false,"assessment":"supported","confidence":"medium","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":[]},
	 {"id":"h2","label":"政策补贴同步","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策","support_evidence":[],"counter_evidence":[],"gaps":[]}],
	 "conclusion":{"summary":"矛盾快照","confidence":"low","scope":"两条泳道","boundary":"无"},
	 "evidence_chain":[],"lane_refs":[]}`
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: contradictory},
		{Content: contradictory},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	payload, meta, err := orch.synthesizeBoardInvestigation(context.Background(), "inv-sess",
		synthesisTestQuestion(), investigationTestBrief(), synthesisTestStage(), synthesisTestResearch(), []uint{1, 2}, 0, nil)
	if err == nil || payload != nil {
		t.Fatalf("two definitive contradictions must error with nil payload, got %+v / %v", payload, err)
	}
	if len(router.calls) != 2 {
		t.Fatalf("exactly two attempts expected, got %d", len(router.calls))
	}
	if meta.Attempts != 2 || meta.RetryReason != synthesisRetryStructure {
		t.Fatalf("meta must trace the stable structure code: %+v", meta)
	}
	if !strings.Contains(router.calls[1].Messages[0].Content, boardSynthesizeRetryLead) {
		t.Fatal("retry prompt must carry the corrective note")
	}
	if !strings.Contains(err.Error(), "基金推动两泳道") {
		t.Fatalf("failure reason must name the contradicted hypothesis for the corrective retry: %v", err)
	}
}
