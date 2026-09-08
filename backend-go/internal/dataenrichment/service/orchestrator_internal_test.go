package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"syntopica-backend/internal/platform/airouter"
)

// White-box unit tests for the causal-analysis-agent schema (stage 2a-i):
// analyzeOutput parsing, form classification, lens candidates, JSON round-trip.
// These target unexported symbols, hence package service (not service_test).

var internalTestCap = airouter.Capability("data_enrichment_analysis")

// internalMockRouter returns canned ChatResults in order, then "{}".
type internalMockRouter struct {
	responses []*airouter.ChatResult
	idx       int
	calls     []airouter.ChatRequest
}

func (m *internalMockRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	m.calls = append(m.calls, req)
	if m.idx >= len(m.responses) {
		return &airouter.ChatResult{Content: "{}"}, nil
	}
	r := m.responses[m.idx]
	m.idx++
	return r, nil
}

// ── parseAnalyzeOutput: layered insight (event_chain) ───────────────────────

func TestParseAnalyzeOutput_EventChainLayered(t *testing.T) {
	parsed, err := ParseJSONResponse(`{
		"form": "event_chain",
		"lens": "油价这轮上涨能不能持续",
		"analysis": {
			"fact_layer": [
				{"claim": "产油国设施遭袭", "evidence": [{"source_type":"news","ref":"ctx1","quote":"设施遭袭"}], "verified": true}
			],
			"timeline": [
				{"date": "2026-07-01", "event": "遭袭", "ref": {"source_type":"news","ref":"ctx1"}}
			],
			"insight_layer": [
				{"cert": "medium", "title": "油价短期承压", "logic": "供应收紧→价格上涨", "evidence": [{"source_type":"news","ref":"ctx1","quote":"油价飙升"}]}
			],
			"depth": {
				"system_reframe": "放进全球能源供应链系统看",
				"mechanism_layers": [{"layer":"供给冲击","deep_logic":"产能骤降→供需缺口","basis":"遭袭报道"}],
				"historical_analogy": [{"case":"2019沙特油田遭袭","mechanism":"短期供应中断→油价跳涨","diff":"本次波及范围待证实"}],
				"regime_shift": null,
				"boundary": "目前还不能确认补产时序，不宜外推长期趋势",
				"evidence_chain": [{"source_type":"news","ref":"ctx1","quote":"设施遭袭","date":"2026-07-01"}]
			}
		}
	}`)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}

	out, err := parseAnalyzeOutput(parsed)
	if err != nil {
		t.Fatalf("parseAnalyzeOutput: %v", err)
	}
	if out.Form != FormEventChain {
		t.Fatalf("form: want event_chain, got %s", out.Form)
	}
	if out.Lens == "" {
		t.Fatal("lens should not be empty")
	}

	body, ok := out.Analysis.(EventChainAnalysis)
	if !ok {
		t.Fatalf("analysis body type: want EventChainAnalysis, got %T", out.Analysis)
	}
	// 事实层与见解层分离
	if len(body.FactLayer) != 1 {
		t.Fatalf("fact_layer: want 1, got %d", len(body.FactLayer))
	}
	if !body.FactLayer[0].Verified {
		t.Fatal("fact_layer[0] should be verified")
	}
	if len(body.InsightLayer) != 1 {
		t.Fatalf("insight_layer: want 1, got %d", len(body.InsightLayer))
	}
	if body.InsightLayer[0].Title != "油价短期承压" {
		t.Fatalf("insight title: got %q", body.InsightLayer[0].Title)
	}
	if len(body.Timeline) != 1 {
		t.Fatalf("timeline: want 1, got %d", len(body.Timeline))
	}
}

// ── parseAnalyzeOutput: insight without evidence is dropped ─────────────────

func TestParseAnalyzeOutput_InsightMustHaveEvidence(t *testing.T) {
	parsed, err := ParseJSONResponse(`{
		"form": "event_chain",
		"lens": "L",
		"analysis": {
			"fact_layer": [],
			"insight_layer": [
				{"cert": "medium", "title": "有据见解", "logic": "x", "evidence": [{"source_type":"news","ref":"r1"}]},
				{"cert": "medium", "title": "悬空见解A", "logic": "x", "evidence": []},
				{"cert": "medium", "title": "悬空见解B", "logic": "x"},
				{"cert": "medium", "title": "只有web验证也算有据", "logic": "x", "web_verified": [{"source_type":"tool","ref":"w1"}]}
			],
			"depth": {"system_reframe":"sr","mechanism_layers":[{"layer":"L","deep_logic":"d","basis":"b"}],"boundary":"不可外推","evidence_chain":[{"source_type":"news","ref":"r1"}]}
		}
	}`)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	out, err := parseAnalyzeOutput(parsed)
	if err != nil {
		t.Fatalf("parseAnalyzeOutput: %v", err)
	}
	body := out.Analysis.(EventChainAnalysis)
	if len(body.InsightLayer) != 2 {
		t.Fatalf("insight_layer: want 2 (drop 2 evidence-less), got %d", len(body.InsightLayer))
	}
	titles := []string{body.InsightLayer[0].Title, body.InsightLayer[1].Title}
	for _, ttl := range titles {
		if strings.Contains(ttl, "悬空") {
			t.Fatalf("evidence-less insight should be dropped, got %q", ttl)
		}
	}
}

// ── parseAnalyzeOutput: theme_vein cross_insight evidence enforcement ───────

func TestParseAnalyzeOutput_ThemeVeinCrossInsightEvidence(t *testing.T) {
	parsed, err := ParseJSONResponse(`{
		"form": "theme_vein",
		"lens": "L",
		"analysis": {
			"veins": [{"name": "AI算力", "desc": "算力线索", "evidence": [{"source_type":"news","ref":"r1"}]}],
			"cross_insight": [
				{"cert": "high", "title": "有据跨线索", "logic": "x", "evidence": [{"source_type":"news","ref":"r1"}]},
				{"cert": "low", "title": "悬空跨线索", "logic": "x", "evidence": []}
			],
			"depth": {"system_reframe":"sr","mechanism_layers":[{"layer":"L","deep_logic":"d","basis":"b"}],"boundary":"不可外推","evidence_chain":[{"source_type":"news","ref":"r1"}]}
		}
	}`)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	out, _ := parseAnalyzeOutput(parsed)
	body := out.Analysis.(ThemeVeinAnalysis)
	if len(body.Veins) != 1 {
		t.Fatalf("veins: want 1, got %d", len(body.Veins))
	}
	if len(body.CrossInsight) != 1 {
		t.Fatalf("cross_insight: want 1 (drop 1 evidence-less), got %d", len(body.CrossInsight))
	}
}

// ── parseAnalyzeOutput: certainty grading (4 levels) ────────────────────────

func TestParseAnalyzeOutput_CertaintyGrading(t *testing.T) {
	parsed, err := ParseJSONResponse(`{
		"form": "event_chain",
		"lens": "L",
		"analysis": {
			"insight_layer": [
				{"cert": "high", "title": "已验证", "logic": "x", "evidence": [{"source_type":"news","ref":"r1"}]},
				{"cert": "medium", "title": "推演有据", "logic": "x", "evidence": [{"source_type":"news","ref":"r1"}]},
				{"cert": "low", "title": "假设情景", "logic": "x", "evidence": [{"source_type":"news","ref":"r1"}]},
				{"cert": "question", "title": "指出条件", "logic": "x", "evidence": [{"source_type":"news","ref":"r1"}]}
			],
			"depth": {"system_reframe":"sr","mechanism_layers":[{"layer":"L","deep_logic":"d","basis":"b"}],"boundary":"不可外推","evidence_chain":[{"source_type":"news","ref":"r1"}]}
		}
	}`)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	out, _ := parseAnalyzeOutput(parsed)
	body := out.Analysis.(EventChainAnalysis)
	if len(body.InsightLayer) != 4 {
		t.Fatalf("want all 4 cert levels kept, got %d", len(body.InsightLayer))
	}
	wantCerts := map[string]bool{"high": false, "medium": false, "low": false, "question": false}
	for _, ins := range body.InsightLayer {
		if _, ok := wantCerts[ins.Cert]; ok {
			wantCerts[ins.Cert] = true
		}
	}
	for cert, seen := range wantCerts {
		if !seen {
			t.Fatalf("cert %q not present in insight_layer", cert)
		}
	}
}

// ── parseAnalyzeOutput: sparse has no insight_layer ─────────────────────────

func TestParseAnalyzeOutput_SparseNoInsight(t *testing.T) {
	parsed, err := ParseJSONResponse(`{
		"form": "sparse",
		"lens": "L",
		"analysis": {"notice": "命中仅1次，脉络单薄", "summary": "仅能确认事件发生"}
	}`)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	out, err := parseAnalyzeOutput(parsed)
	if err != nil {
		t.Fatalf("parseAnalyzeOutput: %v", err)
	}
	if out.Form != FormSparse {
		t.Fatalf("form: want sparse, got %s", out.Form)
	}
	body, ok := out.Analysis.(SparseAnalysis)
	if !ok {
		t.Fatalf("analysis body type: want SparseAnalysis, got %T", out.Analysis)
	}
	if body.Notice == "" {
		t.Fatal("sparse notice should not be empty")
	}
	// SparseAnalysis struct has no insight field by construction — the type
	// itself enforces "no insight_layer". We assert the type to prove it.
}

// ── parseAnalyzeOutput: single_point body ───────────────────────────────────

func TestParseAnalyzeOutput_SinglePoint(t *testing.T) {
	parsed, err := ParseJSONResponse(`{
		"form": "single_point",
		"lens": "L",
		"analysis": {
			"impact": {"implication": "直接提价", "ripple": "下游成本上升", "benchmark": "2022同期"},
			"evidence": [{"source_type":"news","ref":"r1"}],
			"depth": {"system_reframe":"sr","mechanism_layers":[{"layer":"L","deep_logic":"d","basis":"b"}],"boundary":"不可外推","evidence_chain":[{"source_type":"news","ref":"r1"}]}
		}
	}`)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	out, _ := parseAnalyzeOutput(parsed)
	body, ok := out.Analysis.(SinglePointAnalysis)
	if !ok {
		t.Fatalf("want SinglePointAnalysis, got %T", out.Analysis)
	}
	if body.Impact.Implication != "直接提价" {
		t.Fatalf("impact.implication: got %q", body.Impact.Implication)
	}
	if len(body.Evidence) != 1 {
		t.Fatalf("evidence: want 1, got %d", len(body.Evidence))
	}
}

// ── parseAnalyzeOutput: rejects invalid/missing form ────────────────────────

func TestParseAnalyzeOutput_RejectsInvalidForm(t *testing.T) {
	cases := []string{
		`{"form":"bogus","lens":"L","analysis":{}}`,
		`{"lens":"L","analysis":{}}`,
		`{"form":"event_chain","lens":"L"}`, // missing analysis
	}
	for i, c := range cases {
		parsed, err := ParseJSONResponse(c)
		if err != nil {
			t.Fatalf("case %d parse json: %v", i, err)
		}
		if _, err := parseAnalyzeOutput(parsed); err == nil {
			t.Fatalf("case %d: expected error for invalid analyze output", i)
		}
	}
}

// ── analyzeOutput JSON round-trip (marshal → unmarshal per form) ────────────

func TestAnalyzeOutput_JSONRoundTrip(t *testing.T) {
	validDepth := Depth{
		SystemReframe:   "sr",
		MechanismLayers: []MechanismLayer{{Layer: "L", DeepLogic: "d", Basis: "b"}},
		Boundary:        "不可外推",
		EvidenceChain:   []EvidenceChainItem{{SourceType: "news", Ref: "r"}},
	}
	cases := []analyzeOutput{
		{Form: FormEventChain, Lens: "L1", Analysis: EventChainAnalysis{
			FactLayer:    []FactClaim{{Claim: "c", Evidence: []Ref{{SourceType: "news", Ref: "r"}}, Verified: true}},
			Timeline:     []TimelineNode{{Date: "2026-07-01", Event: "e", Ref: &Ref{SourceType: "news", Ref: "r"}}},
			InsightLayer: []Insight{{Cert: CertMedium, Title: "t", Logic: "l", Evidence: []Ref{{SourceType: "news", Ref: "r"}}}},
			Depth:        validDepth,
		}},
		{Form: FormThemeVein, Lens: "L2", Analysis: ThemeVeinAnalysis{
			Veins:        []Vein{{Name: "v", Desc: "d", Evidence: []Ref{{SourceType: "news", Ref: "r"}}}},
			CrossInsight: []Insight{{Cert: CertHigh, Title: "c", Logic: "l", Evidence: []Ref{{SourceType: "news", Ref: "r"}}}},
			Depth:        validDepth,
		}},
		{Form: FormSinglePoint, Lens: "L3", Analysis: SinglePointAnalysis{
			Impact:   ImpactAssessment{Implication: "i", Ripple: "r", Benchmark: "b"},
			Evidence: []Ref{{SourceType: "tool", Ref: "r"}},
			Depth:    validDepth,
		}},
		{Form: FormStructural, Lens: "L5", Analysis: StructuralAnalysis{
			EvolutionNarrative: "人民币国际化进程的阶段性演进",
			Phases:             []Phase{{Period: "2009", Event: "跨境贸易试点"}},
			Depth:              validDepth,
		}},
		{Form: FormSparse, Lens: "L4", Analysis: SparseAnalysis{Notice: "n", Summary: "s"}},
	}
	for i, original := range cases {
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("case %d marshal: %v", i, err)
		}
		var roundtrip analyzeOutput
		if err := json.Unmarshal(data, &roundtrip); err != nil {
			t.Fatalf("case %d unmarshal: %v", i, err)
		}
		if roundtrip.Form != original.Form {
			t.Fatalf("case %d form: want %s, got %s", i, original.Form, roundtrip.Form)
		}
		if roundtrip.Lens != original.Lens {
			t.Fatalf("case %d lens: want %s, got %s", i, original.Lens, roundtrip.Lens)
		}
		if roundtrip.Analysis == nil {
			t.Fatalf("case %d analysis body should not be nil after round-trip", i)
		}
		// Body type must match original concrete type.
		switch original.Analysis.(type) {
		case EventChainAnalysis:
			if _, ok := roundtrip.Analysis.(EventChainAnalysis); !ok {
				t.Fatalf("case %d body type mismatch", i)
			}
		case ThemeVeinAnalysis:
			if _, ok := roundtrip.Analysis.(ThemeVeinAnalysis); !ok {
				t.Fatalf("case %d body type mismatch", i)
			}
		case SinglePointAnalysis:
			if _, ok := roundtrip.Analysis.(SinglePointAnalysis); !ok {
				t.Fatalf("case %d body type mismatch", i)
			}
		case StructuralAnalysis:
			if _, ok := roundtrip.Analysis.(StructuralAnalysis); !ok {
				t.Fatalf("case %d body type mismatch", i)
			}
		case SparseAnalysis:
			if _, ok := roundtrip.Analysis.(SparseAnalysis); !ok {
				t.Fatalf("case %d body type mismatch", i)
			}
		}
	}
}

// ── ReviewJudgeOutput JSON round-trip ───────────────────────────────────────

func TestReviewJudgeOutput_JSONRoundTrip(t *testing.T) {
	original := ReviewJudgeOutput{
		ShouldReview:    true,
		Reason:          "r",
		NewFindings:     []string{"new1", "new2"},
		Overturned:      []string{"old1"},
		ConfidenceShift: []map[string]any{{"insight": "i", "from": "medium", "to": "high"}},
		AffectedContext: "week",
		Confidence:      0.8,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ReviewJudgeOutput
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.NewFindings) != 2 || got.NewFindings[0] != "new1" {
		t.Fatalf("new_findings round-trip: %v", got.NewFindings)
	}
	if len(got.Overturned) != 1 {
		t.Fatalf("overturned round-trip: %v", got.Overturned)
	}
	if len(got.ConfidenceShift) != 1 {
		t.Fatalf("confidence_shift round-trip: %v", got.ConfidenceShift)
	}
	if !got.ShouldReview || got.Confidence != 0.8 {
		t.Fatalf("scalar round-trip mismatch: %+v", got)
	}
}

// ── interpret: form classification (4 forms) ───────────────────────────────

func TestInterpret_FormClassification(t *testing.T) {
	cases := []struct {
		name string
		form string
		resp string
	}{
		{"event_chain", FormEventChain, `{"form":"event_chain","form_reason":"高频线性因果","topics":[{"topic":"石油","reason":"油价"}]}`},
		{"theme_vein", FormThemeVein, `{"form":"theme_vein","form_reason":"多线并行发散","topics":[{"topic":"AI算力","reason":"芯片"}]}`},
		{"single_point", FormSinglePoint, `{"form":"single_point","form_reason":"单一事件","topics":[{"topic":"石油","reason":"油价"}]}`},
		{"structural", FormStructural, `{"form":"structural","form_reason":"长时段结构演化","topics":[{"topic":"货币互换机制","reason":"历史机制"}]}`},
		{"sparse", FormSparse, `{"form":"sparse","form_reason":"命中极少","topics":[]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			router := &internalMockRouter{responses: []*airouter.ChatResult{{Content: c.resp}}}
			orch := &OrchestratorService{airouter: router, capability: internalTestCap}
			res, err := orch.interpret(context.Background(), interpretContext{SessionID: "s", LifelineText: "L"})
			if err != nil {
				t.Fatalf("interpret(%s): %v", c.name, err)
			}
			if res.Form != c.form {
				t.Fatalf("form: want %s, got %s", c.form, res.Form)
			}
			// sparse 允许零主题；其它形态至少一个
			if c.form != FormSparse && len(res.Topics) == 0 {
				t.Fatalf("%s: expected >=1 topic", c.name)
			}
		})
	}
}

// ── interpret: rejects invalid form ─────────────────────────────────────────

func TestInterpret_RejectsInvalidForm(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: `{"form":"bogus","topics":[{"topic":"x","reason":"y"}]}`},
	}}
	orch := &OrchestratorService{airouter: router, capability: internalTestCap}
	_, err := orch.interpret(context.Background(), interpretContext{SessionID: "s", LifelineText: "L"})
	if err == nil {
		t.Fatal("interpret should reject invalid form")
	}
}

// ── LensSource: AgentLensSource proposes >=2 concrete problem-style lenses ─

func TestAgentLensSource_ConcreteLensCandidates(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{{Content: `{
		"lens_candidates": [
			{"name": "美国为何在对华芯片政策上反复横跳", "description": "看政策博弈"},
			{"name": "国产替代能否补位", "description": "看本土供应链"}
		]
	}`}}}
	src := NewAgentLensSource(router, internalTestCap)
	lenses, err := src.Propose(context.Background(), interpretContext{SessionID: "s", LifelineText: "L"}, FormEventChain)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if len(lenses) < 2 {
		t.Fatalf("need >=2 lens candidates, got %d", len(lenses))
	}
	// Each lens must be concrete (name non-empty), not an abstract tag.
	for _, l := range lenses {
		if l.Name == "" {
			t.Fatal("lens name should not be empty")
		}
	}
	// The lens prompt must carry the classified form (proves form flowed in).
	if len(router.calls) != 1 {
		t.Fatalf("want 1 LLM call, got %d", len(router.calls))
	}
	prompt := router.calls[0].Messages[0].Content
	if !strings.Contains(prompt, FormEventChain) {
		t.Fatalf("lens prompt should contain the form %q", FormEventChain)
	}
}

// ── LensSource: rejects <2 candidates ───────────────────────────────────────

func TestAgentLensSource_RejectsTooFew(t *testing.T) {
	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: `{"lens_candidates":[{"name":"唯一视角","description":"x"}]}`},
	}}
	src := NewAgentLensSource(router, internalTestCap)
	_, err := src.Propose(context.Background(), interpretContext{SessionID: "s", LifelineText: "L"}, FormThemeVein)
	if err == nil {
		t.Fatal("Propose should reject <2 lens candidates")
	}
}

// ── parseAnalyzeOutput: structural form + depth layer ───────────────────────

func TestParseAnalyzeOutput_Structural(t *testing.T) {
	parsed, err := ParseJSONResponse(`{
		"form": "structural",
		"lens": "人民币国际化走到哪一步了",
		"analysis": {
			"evolution_narrative": "从跨境贸易试点到本币互换网络的渐进结构演化",
			"phases": [
				{"period": "2009", "event": "跨境贸易人民币结算试点", "ref": {"source_type":"news","ref":"ctx1"}},
				{"period": "2016", "event": "加入 SDR 篮子"}
			],
			"depth": {
				"system_reframe": "放进全球货币体系多极化的大系统看",
				"mechanism_layers": [{"layer":"清算网络","deep_logic":"本币互换绕开美元清算","basis":"央行互换额度数据"}],
				"historical_analogy": [{"case":"欧元崛起","mechanism":"区域结算货币替代","diff":"人民币更侧重贸易结算而非金融"}],
				"regime_shift": null,
				"boundary": "目前还不能下结论说美元主导地位已被实质性动摇",
				"evidence_chain": [
					{"source_type":"web","url":"https://imf.org/x","quote":"SDR 权重调整","institution":"IMF","date":"2016-10-01"}
				]
			}
		}
	}`)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	out, err := parseAnalyzeOutput(parsed)
	if err != nil {
		t.Fatalf("parseAnalyzeOutput: %v", err)
	}
	body, ok := out.Analysis.(StructuralAnalysis)
	if !ok {
		t.Fatalf("want StructuralAnalysis, got %T", out.Analysis)
	}
	if body.EvolutionNarrative == "" {
		t.Fatal("evolution_narrative should not be empty")
	}
	if len(body.Phases) != 2 {
		t.Fatalf("phases: want 2, got %d", len(body.Phases))
	}
	if len(body.Depth.MechanismLayers) != 1 {
		t.Fatalf("mechanism_layers: want 1, got %d", len(body.Depth.MechanismLayers))
	}
	if len(body.Depth.EvidenceChain) != 1 || body.Depth.EvidenceChain[0].URL == "" {
		t.Fatal("evidence_chain web entry must carry url")
	}
}

// ── parseAnalyzeOutput: non-sparse without depth is rejected ────────────────

func TestParseAnalyzeOutput_DepthRequired_NonSparse(t *testing.T) {
	// event_chain with NO depth block → error (caller retries).
	noDepth, err := ParseJSONResponse(`{
		"form": "event_chain", "lens": "L",
		"analysis": {"insight_layer": [{"cert":"medium","title":"t","logic":"x","evidence":[{"source_type":"news","ref":"r1"}]}]}
	}`)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if _, err := parseAnalyzeOutput(noDepth); err == nil {
		t.Fatal("event_chain without depth should be rejected")
	}

	// event_chain with empty boundary → error.
	emptyBoundary, err := ParseJSONResponse(`{
		"form": "event_chain", "lens": "L",
		"analysis": {
			"insight_layer": [{"cert":"medium","title":"t","logic":"x","evidence":[{"source_type":"news","ref":"r1"}]}],
			"depth": {"system_reframe":"sr","mechanism_layers":[{"layer":"L","deep_logic":"d","basis":"b"}],"boundary":"  ","evidence_chain":[{"source_type":"news","ref":"r1"}]}
		}
	}`)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	if _, err := parseAnalyzeOutput(emptyBoundary); err == nil {
		t.Fatal("empty boundary should be rejected (反过度解读强制)")
	}
}

// ── parseAnalyzeOutput: sparse forbids depth (ignored if present) ───────────

func TestParseAnalyzeOutput_SparseForbidsDepth(t *testing.T) {
	parsed, err := ParseJSONResponse(`{
		"form": "sparse", "lens": "L",
		"analysis": {"notice": "命中仅1次", "summary": "仅确认发生",
			"depth": {"system_reframe":"不该有","boundary":"x","mechanism_layers":[{"layer":"L","deep_logic":"d","basis":"b"}],"evidence_chain":[{"source_type":"news","ref":"r1"}]}}
	}`)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	out, err := parseAnalyzeOutput(parsed)
	if err != nil {
		t.Fatalf("sparse must parse without depth-validation error: %v", err)
	}
	body, ok := out.Analysis.(SparseAnalysis)
	if !ok {
		t.Fatalf("want SparseAnalysis, got %T", out.Analysis)
	}
	// SparseAnalysis has no Depth field by construction — proving the type
	// asserts the depth block was ignored, not honoured.
	if body.Notice == "" {
		t.Fatal("notice should be set")
	}
}

// ── parseAnalyzeOutput: evidence_chain web/page entries carry url+quote ─────

func TestParseAnalyzeOutput_EvidenceChainWebPage(t *testing.T) {
	parsed, err := ParseJSONResponse(`{
		"form": "single_point", "lens": "L",
		"analysis": {
			"impact": {"implication": "i", "ripple": "r", "benchmark": "b"},
			"evidence": [{"source_type":"news","ref":"r1"}],
			"depth": {
				"system_reframe": "sr",
				"mechanism_layers": [{"layer":"L","deep_logic":"d","basis":"b"}],
				"boundary": "不可外推",
				"evidence_chain": [
					{"source_type":"web","url":"https://a.example.com","quote":"原文摘录A","institution":"机构A","date":"2026-07-01"},
					{"source_type":"page","url":"https://b.example.com/doc","quote":"原文摘录B","institution":"机构B","date":"2026-06-01"},
					{"source_type":"news","ref":"ctx2","quote":"新闻原话"}
				]
			}
		}
	}`)
	if err != nil {
		t.Fatalf("parse json: %v", err)
	}
	out, err := parseAnalyzeOutput(parsed)
	if err != nil {
		t.Fatalf("parseAnalyzeOutput: %v", err)
	}
	body := out.Analysis.(SinglePointAnalysis)
	if len(body.Depth.EvidenceChain) != 3 {
		t.Fatalf("evidence_chain: want 3, got %d", len(body.Depth.EvidenceChain))
	}
	for i, ec := range body.Depth.EvidenceChain {
		if (ec.SourceType == "web" || ec.SourceType == "page") && ec.URL == "" {
			t.Fatalf("evidence_chain[%d] web/page entry must carry url", i)
		}
		if ec.Quote == "" {
			t.Fatalf("evidence_chain[%d] must carry a direct quote", i)
		}
	}
}

// ── 4.8 单泳道三段 prompt 哨兵：0 作者画像 / 无 board 调查 schema / 无证据配额 ──

// TestTopicPrompts_NoAuthorProfileNoBoardSchema asserts the three single-lane
// prompt constants (interpret / agent loop / analyze) stay free of retired
// author-profile injection and board investigation schema (task 4.8 contracts
// ①/③), and that the analyze prompt carries no evidence type-count quota.
func TestTopicPrompts_NoAuthorProfileNoBoardSchema(t *testing.T) {
	analyze := fmt.Sprintf(analyzePrompt, "event_chain", "测试视角", "event_chain", "测试视角")
	agentLoop := fmt.Sprintf(agentLoopSystemPrompt, "测试视角", "工具列表", "脉络")
	prompts := map[string]string{
		"analyze":    analyze,
		"interpret":  interpretPrompt,
		"agent_loop": agentLoop,
	}

	for name, prompt := range prompts {
		// 契约①：旧作者画像/方法论画像全局注入已退役，任何入口不得回流。
		for _, bad := range []string{"作者画像", "方法论画像", "reference_role", "ReferenceRole"} {
			if strings.Contains(prompt, bad) {
				t.Errorf("%s prompt must not inject retired author profile, found %q", name, bad)
			}
		}
		// 契约③：board 调查 schema 不进单泳道 prompt。
		for _, bad := range []string{"hypotheses", "question_key", "parent_briefing_id"} {
			if strings.Contains(prompt, bad) {
				t.Errorf("%s prompt must not contain board investigation schema field %q", name, bad)
			}
		}
	}

	// 契约②：证据类型数量配额不得回流（含任何“固定数量”变体措辞）。
	for _, bad := range []string{"≥3 类", "至少三类", "至少 3 类", "三类证据", "3 类证据", "证据多样性"} {
		if strings.Contains(analyze, bad) {
			t.Errorf("analyze prompt must not carry evidence type quota %q", bad)
		}
	}
}
