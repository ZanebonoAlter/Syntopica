package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
)

// 成文环节单元测试（board-signal-reports tasks 4.1，SV-1..SV-7 + appendix 代码
// 生成）。stub LLM：不起真模型。断言锚点：
//   - 四段唯一序 / implication 结构化字段与 direction 枚举；
//   - 可见篇幅 <3000 非空白字符（2999 过 / 3000 拒，附录不计）；
//   - 引用可解析：悬空观测 / 未知计算 / 失败计算引用一律拒绝；
//   - 图表：0~3 张、line 同系列同单位 ≥2 非空点按期间排序、null 断点保留、
//     comparison 同源同单位、禁跨源/混单位；
//   - attempts≤3（初次+2 重试），耗尽 failed 无 result；
//   - appendix 由代码生成：draft 里塞 appendix 字段也必须被忽略；
//   - conditional（证据不足维持判断）合法，不强迫方向。

// ── stub：只接受 compose operation 的路由 ────────────────────────────────────

type scStubRouter struct {
	replies []srComposeReply
	calls   []airouter.ChatRequest
}

func (r *scStubRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Operation != signalComposeOperation {
		return nil, airflowOperationError(req.Operation)
	}
	r.calls = append(r.calls, req)
	n := len(r.calls)
	if n > len(r.replies) {
		return nil, &scOutOfRepliesError{n}
	}
	rep := r.replies[n-1]
	if rep.err != nil {
		return nil, rep.err
	}
	return &airouter.ChatResult{Content: rep.content}, nil
}

type scOutOfRepliesError struct{ n int }

func (e *scOutOfRepliesError) Error() string { return "scStubRouter: unexpected compose call" }

func airflowOperationError(op string) error {
	return &scWrongOperationError{op}
}

type scWrongOperationError struct{ op string }

func (e *scWrongOperationError) Error() string { return "unexpected operation " + e.op }

// ── fixtures ─────────────────────────────────────────────────────────────────

// scFixtureLedger 构造研究账本：c1=库存系列（含一个 null 观测），c2=进出口
// （imports/exports 同期间），k1=合格计算，k2=被拒计算。
func scFixtureLedger() *signalResearchLedger {
	l := newSignalResearchLedger(srCutoff)
	l.nextCall = 2
	mkObs := func(id, label, period string, unit string, value any) map[string]any {
		return map[string]any{"observation_id": id, "label": label, "period": period, "unit": unit, "geo": "US", "value": value}
	}
	l.calls = []*signalResearchCallRecord{
		{
			CallID: "c1", Tool: "eia_wpsr_table1", Question: "库存走势?", Status: "ok",
			RetrievedAt: "2026-09-22T02:00:00Z", SourceSHA256: "abc",
			Observations: []map[string]any{
				mkObs("c1:o1", "U.S. Commercial Crude Oil", "2026-08-01", "MMbbl", 420.0),
				mkObs("c1:o2", "U.S. Commercial Crude Oil", "2026-08-02", "MMbbl", 419.0),
				mkObs("c1:o3", "U.S. Commercial Crude Oil", "2026-08-03", "MMbbl", nil),
			},
			FilterMeta: map[string]any{"cutoff": "x", "kept": 3, "dropped": 0},
		},
		{
			CallID: "c2", Tool: "eia_wpsr_table1", Question: "净进出口?", Status: "ok",
			Observations: []map[string]any{
				mkObs("c2:o1", "U.S. Crude Oil Imports", "2026-08-01", "Mb/d", 6000.0),
				mkObs("c2:o2", "U.S. Crude Oil Exports", "2026-08-01", "Mb/d", 3600.0),
			},
		},
	}
	obs := func(ref, series, unit, period, flow string, value *float64) *SignalCalcObservation {
		return &SignalCalcObservation{Ref: ref, CallID: ref[:2], Tool: "eia_wpsr_table1", SeriesID: series, Unit: unit, Period: period, Flow: flow, Value: value}
	}
	l.obsIndex = map[string]*SignalCalcObservation{
		"c1:o1": obs("c1:o1", "eia|stock", "MMbbl", "2026-08-01", "stocks", f64(420)),
		"c1:o2": obs("c1:o2", "eia|stock", "MMbbl", "2026-08-02", "stocks", f64(419)),
		"c1:o3": obs("c1:o3", "eia|stock", "MMbbl", "2026-08-03", "stocks", nil),
		"c2:o1": obs("c2:o1", "eia|imports", "Mb/d", "2026-08-01", "imports", f64(6000)),
		"c2:o2": obs("c2:o2", "eia|exports", "Mb/d", "2026-08-01", "exports", f64(3600)),
	}
	l.calcs = []*SignalCalculationResult{
		runSignalCalculation(l.obsIndex, SignalCalculationRequest{Op: "difference", Inputs: []string{"c1:o1", "c1:o2"}, Question: "q"}, "k1", calcNow),     // ok: -1 MMbbl
		runSignalCalculation(l.obsIndex, SignalCalculationRequest{Op: "difference", Inputs: []string{"c1:o1", "c2:o1"}, Question: "q"}, "k2", calcNow),     // rejected: 混单位
		runSignalCalculation(l.obsIndex, SignalCalculationRequest{Op: "difference", Inputs: []string{"c2:o1", "c2:o2"}, Question: "q"}, "k3", calcNow),     // ok: 2400 Mb/d
		runSignalCalculation(l.obsIndex, SignalCalculationRequest{Op: "percent_change", Inputs: []string{"c1:o2", "c1:o3"}, Question: "q"}, "k4", calcNow), // missing: base null
	}
	return l
}

func scInput(ledger *signalResearchLedger) SignalComposeInput {
	candidate, discovery := srFixturePair(7)
	_ = discovery
	material := srFixtureMaterial()
	material.Gaps = append(material.Gaps, SignalMaterialGap{
		Reason: "泳道无历史归属记录：材料按当前泳道清单近似，周期内的退出/迁移不可还原",
	})
	return SignalComposeInput{
		SessionID:    "board_signal_report_7_abcd1234",
		Candidate:    candidate,
		Material:     material,
		Ledger:       ledger,
		Loop:         &AgentLoopResult{FinalData: "研究笔记：库存微降。"},
		StopReason:   SignalStopReasonFinished,
		Decisions:    5,
		AnalysisMode: repository.SignalAnalysisModeCurrent,
		Cutoff:       srCutoff,
	}
}

// scSections 构造四段；每段可用 override 覆盖。facts 按时效纪律（design
// §10.3）带「数据截至」声明——fixture 账本最新观测期为 2026-08-03。
func scSections(factsRefs ...string) []map[string]any {
	facts := "发生了什么。数据截至 2026-08-03。"
	for _, r := range factsRefs {
		if strings.HasPrefix(r, "k") {
			facts += "[[calc:" + r + "]]"
		} else {
			facts += "[[data:" + r + "]]"
		}
	}
	return []map[string]any{
		{"kind": "thesis", "text": "先说结论。"},
		{"kind": "facts", "text": facts},
		{"kind": "causal", "text": "这意味着什么：另一种解释还没排除。"},
		{"kind": "implication", "text": "接下来怎么看，先维持判断。", "verdict": "维持原判断",
			"direction": "conditional", "horizon": "一个月", "trigger_condition": "若数据反向则修正", "self_doubt": "样本太短"},
	}
}

func scDraft(sections []map[string]any, charts []any) string {
	if charts == nil {
		charts = []any{}
	}
	b, _ := json.Marshal(map[string]any{
		"title":    "库存变化还不能说明需求转向",
		"sections": sections,
		"charts":   charts,
	})
	return string(b)
}

func scCompose(t *testing.T, router AirRouter, in SignalComposeInput) (json.RawMessage, error) {
	t.Helper()
	composer := &SignalComposer{router: router, capability: "data_enrichment_analysis"}
	return composer.Compose(context.Background(), in)
}

func scParsePayload(t *testing.T, raw json.RawMessage) SignalReportPayload {
	t.Helper()
	var payload SignalReportPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

// ── SV-1：四段齐 / 缺段 / 未知 kind ───────────────────────────────────────────

func TestSignalCompose_SV1_SectionContract(t *testing.T) {
	ledger := scFixtureLedger()
	// 合法稿：一次过。
	router := &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections("c1:o1"), nil)}}}
	raw, err := scCompose(t, router, scInput(ledger))
	if err != nil {
		t.Fatalf("valid draft rejected: %v", err)
	}
	payload := scParsePayload(t, raw)
	if len(payload.Report.Sections) != 4 {
		t.Fatalf("sections=%d", len(payload.Report.Sections))
	}
	for i, want := range signalReportSectionOrder {
		if payload.Report.Sections[i].Kind != want {
			t.Fatalf("section[%d]=%s want %s", i, payload.Report.Sections[i].Kind, want)
		}
	}
	if len(router.calls) != 1 {
		t.Fatalf("compose chats=%d", len(router.calls))
	}
	// 缺 causal + 未知 kind：各回注一次后成功，attempts=2。
	broken := []map[string]any{
		{"kind": "thesis", "text": "结论。"},
		{"kind": "facts", "text": "事实。"},
		{"kind": "vibes", "text": "不是合法 kind。"},
		{"kind": "implication", "text": "后续。", "verdict": "v", "direction": "up", "horizon": "h", "trigger_condition": "t", "self_doubt": "s"},
	}
	router2 := &scStubRouter{replies: []srComposeReply{
		{content: scDraft(broken, nil)},
		{content: scDraft(scSections(), nil)},
	}}
	raw2, err := scCompose(t, router2, scInput(ledger))
	if err != nil {
		t.Fatalf("retry path failed: %v", err)
	}
	payload2 := scParsePayload(t, raw2)
	if payload2.GenerationMeta.Attempts != 2 || payload2.GenerationMeta.Retries != 1 {
		t.Fatalf("attempts/retries: %+v", payload2.GenerationMeta)
	}
	// 回注反馈必须点明缺段。
	feedback := router2.calls[1].Messages[len(router2.calls[1].Messages)-1].Content
	if !strings.Contains(feedback, "sections") {
		t.Fatalf("retry feedback must mention sections: %s", feedback)
	}
}

// ── SV-2：implication 条件字段 / direction 枚举 ──────────────────────────────

func TestSignalCompose_SV2_ImplicationFields(t *testing.T) {
	ledger := scFixtureLedger()
	broken := scSections()
	broken[3] = map[string]any{
		"kind": "implication", "text": "后续。",
		"verdict": "v", "direction": "sideways", "horizon": "", "trigger_condition": "", "self_doubt": "",
	}
	router := &scStubRouter{replies: []srComposeReply{
		{content: scDraft(broken, nil)},
		{content: scDraft(scSections(), nil)},
	}}
	if _, err := scCompose(t, router, scInput(ledger)); err != nil {
		t.Fatalf("retry path failed: %v", err)
	}
	feedback := router.calls[1].Messages[len(router.calls[1].Messages)-1].Content
	for _, want := range []string{"direction", "horizon", "trigger_condition", "self_doubt"} {
		if !strings.Contains(feedback, want) {
			t.Fatalf("feedback must name %s: %s", want, feedback)
		}
	}
}

// ── SV-3：2999 过 / 3000 拒（非空白字符，附录/UI 不计）───────────────────────

func scDraftWithFactsLength(n int) (string, int) {
	sections := scSections()
	// 时效声明带可校验期值（review M1 后校验要求期值在账本最新期内；fixture
	// 账本最新观测期为 2026-08-03），篇幅边界计入其字符（other 由本函数重算）。
	sections[1] = map[string]any{"kind": "facts", "text": "数据截至 2026-08-03。" + strings.Repeat("实", n)}
	draft := &signalComposeDraft{Title: "库存变化还不能说明需求转向"}
	other := countNonSpaceRunes(draft.Title)
	for _, s := range sections {
		text, _ := s["text"].(string)
		other += countNonSpaceRunes(text)
		kind, _ := s["kind"].(string)
		_ = kind
		if kind == "implication" {
			for _, k := range []string{"verdict", "direction", "horizon", "trigger_condition", "self_doubt"} {
				v, _ := s[k].(string)
				other += countNonSpaceRunes(v)
			}
		}
	}
	return scDraft(sections, nil), other
}

func TestSignalCompose_SV3_VisibleLengthBoundary(t *testing.T) {
	ledger := scFixtureLedger()
	// 合计恰 2999 → 通过；再补 1 字 → 3000 → 拒绝。（other 由 helper 自身
	// 返回，避免两处字面量漂移。）
	_, other := scDraftWithFactsLength(0)
	pad := signalReportMaxVisibleRunes - 1 - other // 总长 = 2999
	ok2999, _ := scDraftWithFactsLength(pad)
	router := &scStubRouter{replies: []srComposeReply{{content: ok2999}}}
	if _, err := scCompose(t, router, scInput(ledger)); err != nil {
		t.Fatalf("2999 chars must pass: %v", err)
	}
	tooLong, total := scDraftWithFactsLength(pad + 1)
	if total != signalReportMaxVisibleRunes {
		t.Fatalf("fixture boundary wrong: total=%d", total)
	}
	router2 := &scStubRouter{replies: []srComposeReply{{content: tooLong}}}
	if _, err := scCompose(t, router2, scInput(ledger)); err == nil {
		t.Fatalf("3000 chars must be rejected")
	}
}

// ── SV-4：引用可解析（悬空观测 / 未知计算 / 失败计算拒绝）────────────────────

func TestSignalCompose_SV4_RefResolution(t *testing.T) {
	ledger := scFixtureLedger()
	cases := []struct {
		name    string
		refs    []string
		wantErr string
	}{
		{"dangling data", []string{"c9:o1"}, "c9:o1"},
		{"unknown calc", []string{"k9"}, "k9"},
		{"rejected calc", []string{"k2"}, "rejected"},
		{"missing calc", []string{"k4"}, "missing"},
	}
	for _, tc := range cases {
		router := &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections(tc.refs...), nil)}}}
		_, err := scCompose(t, router, scInput(ledger))
		if err == nil {
			t.Fatalf("%s: must be rejected", tc.name)
		}
		if !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("%s: error must mention %s: %v", tc.name, tc.wantErr, err)
		}
	}
	// 合法 data+calc 引用：token 原样保留在正文（前端按附录渲染数值）。
	router := &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections("c1:o1", "k1"), nil)}}}
	raw, err := scCompose(t, router, scInput(ledger))
	if err != nil {
		t.Fatalf("valid refs rejected: %v", err)
	}
	payload := scParsePayload(t, raw)
	if !strings.Contains(payload.Report.Sections[1].Text, "[[data:c1:o1]]") ||
		!strings.Contains(payload.Report.Sections[1].Text, "[[calc:k1]]") {
		t.Fatalf("citation tokens must be preserved: %s", payload.Report.Sections[1].Text)
	}
}

// ── SV-5：图表（0 图 / 折线 null 断点排序 / 混源混单位拒绝）──────────────────

func TestSignalCompose_SV5_Charts(t *testing.T) {
	ledger := scFixtureLedger()
	in := scInput(ledger)

	// 0 图合法（有数据也不强迫成图）。
	router := &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections(), nil)}}}
	if _, err := scCompose(t, router, in); err != nil {
		t.Fatalf("zero charts must pass: %v", err)
	}

	// 折线：refs 乱序给入 + 一个 null 断点 → 通过且按期间升序重排，null 保留。
	line := map[string]any{"chart_id": "ch1", "kind": "line", "claim": "库存走势",
		"refs": []any{"c1:o3", "c1:o1", "c1:o2"}}
	router = &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections(), []any{line})}}}
	raw, err := scCompose(t, router, in)
	if err != nil {
		t.Fatalf("line with null gap must pass: %v", err)
	}
	payload := scParsePayload(t, raw)
	if len(payload.Report.Charts) != 1 {
		t.Fatalf("charts=%d", len(payload.Report.Charts))
	}
	got := payload.Report.Charts[0].Refs
	if len(got) != 3 || got[0] != "c1:o1" || got[1] != "c1:o2" || got[2] != "c1:o3" {
		t.Fatalf("line refs must be period-sorted with null kept: %v", got)
	}

	// 计算点参与折线：k1 的期间取输入最大期（08-02）→ 排在 08-01 之后。
	lineCalc := map[string]any{"chart_id": "ch1", "kind": "line", "claim": "差值",
		"refs": []any{"k1", "c1:o1"}}
	router = &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections(), []any{lineCalc})}}}
	raw, err = scCompose(t, router, in)
	if err != nil {
		t.Fatalf("calc-in-line must pass: %v", err)
	}
	payload = scParsePayload(t, raw)
	if payload.Report.Charts[0].Refs[0] != "c1:o1" || payload.Report.Charts[0].Refs[1] != "k1" {
		t.Fatalf("calc point must sort by its as-of period: %v", payload.Report.Charts[0].Refs)
	}

	// 折线混系列（同单位）：拒绝。
	spr := &SignalCalcObservation{Ref: "c3:o1", CallID: "c3", Tool: "eia_wpsr_table1", SeriesID: "eia|spr", Unit: "MMbbl", Period: "2026-08-01", Flow: "stocks", Value: f64(400)}
	ledger.obsIndex["c3:o1"] = spr
	lineMixed := map[string]any{"chart_id": "ch1", "kind": "line", "claim": "x", "refs": []any{"c1:o1", "c3:o1"}}
	router = &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections(), []any{lineMixed})}}}
	if _, err := scCompose(t, router, in); err == nil || !strings.Contains(err.Error(), "系列") {
		t.Fatalf("mixed-series line must be rejected: %v", err)
	}

	// 折线只有一个非空点：拒绝。
	lineOne := map[string]any{"chart_id": "ch1", "kind": "line", "claim": "x", "refs": []any{"c1:o3"}}
	router = &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections(), []any{lineOne})}}}
	if _, err := scCompose(t, router, in); err == nil {
		t.Fatalf("one-non-null line must be rejected")
	}

	// 混单位（MMbbl vs Mb/d）：拒绝。
	lineUnits := map[string]any{"chart_id": "ch1", "kind": "comparison", "claim": "x", "refs": []any{"c1:o1", "c2:o1"}}
	router = &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections(), []any{lineUnits})}}}
	if _, err := scCompose(t, router, in); err == nil || !strings.Contains(err.Error(), "单位") {
		t.Fatalf("mixed-unit chart must be rejected: %v", err)
	}

	// comparison：同源同单位不同系列（imports vs exports）合法；null 拒绝。
	comp := map[string]any{"chart_id": "ch2", "kind": "comparison", "claim": "进出对比", "refs": []any{"c2:o1", "c2:o2"}}
	router = &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections(), []any{comp})}}}
	if _, err := scCompose(t, router, in); err != nil {
		t.Fatalf("same-unit comparison must pass: %v", err)
	}
	compNull := map[string]any{"chart_id": "ch2", "kind": "comparison", "claim": "x", "refs": []any{"c1:o3"}}
	router = &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections(), []any{compNull})}}}
	if _, err := scCompose(t, router, in); err == nil {
		t.Fatalf("comparison with missing observation must be rejected")
	}

	// 4 张图：拒绝。
	four := []any{line, comp, lineCalc, lineOne}
	router = &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections(), four)}}}
	if _, err := scCompose(t, router, in); err == nil || !strings.Contains(err.Error(), "3") {
		t.Fatalf("more than 3 charts must be rejected: %v", err)
	}

	// 图表引用失败计算：拒绝。
	lineK2 := map[string]any{"chart_id": "ch1", "kind": "line", "claim": "x", "refs": []any{"k1", "k2"}}
	router = &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections(), []any{lineK2})}}}
	if _, err := scCompose(t, router, in); err == nil {
		t.Fatalf("chart referencing rejected calc must fail")
	}
}

// ── SV-6：三次成文全失败 → attempts3/retries2，无 result ──────────────────────

func TestSignalCompose_SV6_BoundedRetriesExhausted(t *testing.T) {
	ledger := scFixtureLedger()
	bad := scDraft([]map[string]any{{"kind": "thesis", "text": "只有一段"}}, nil)
	router := &scStubRouter{replies: []srComposeReply{
		{content: bad}, {content: bad}, {content: bad},
	}}
	_, err := scCompose(t, router, scInput(ledger))
	if err == nil {
		t.Fatalf("exhausted retries must fail")
	}
	if len(router.calls) != signalComposeMaxAttempts {
		t.Fatalf("compose chats=%d", len(router.calls))
	}
	if !strings.Contains(err.Error(), "attempt 3") {
		t.Fatalf("error should report the last attempt: %v", err)
	}
}

// ── SV-7：证据不足维持判断（conditional）合法，不强迫方向 ─────────────────────

func TestSignalCompose_SV7_ConditionalVerdictAllowed(t *testing.T) {
	ledger := scFixtureLedger()
	raw, err := scCompose(t, &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections(), nil)}}}, scInput(ledger))
	if err != nil {
		t.Fatalf("conditional must pass: %v", err)
	}
	payload := scParsePayload(t, raw)
	impl := payload.Report.Sections[3]
	if impl.Direction != "conditional" || impl.Verdict == "" || impl.SelfDoubt == "" {
		t.Fatalf("implication: %+v", impl)
	}
}

// ── appendix 代码生成：LLM 不得生成/覆写；material gaps 继承 ─────────────────

func TestSignalCompose_AppendixIsCodeGenerated(t *testing.T) {
	ledger := scFixtureLedger()
	// draft 里塞一个伪造 appendix + 伪造 generation_meta —— 必须被忽略。
	sections := scSections("c1:o1")
	sections = append(sections, map[string]any{})
	forge, _ := json.Marshal(map[string]any{
		"title":           "库存变化还不能说明需求转向",
		"sections":        sections[:4],
		"charts":          []any{},
		"appendix":        map[string]any{"calls": "forged"},
		"generation_meta": map[string]any{"attempts": 99},
	})
	router := &scStubRouter{replies: []srComposeReply{{content: string(forge)}}}
	raw, err := scCompose(t, router, scInput(ledger))
	if err != nil {
		t.Fatalf("compose failed: %v", err)
	}
	payload := scParsePayload(t, raw)
	if len(payload.Appendix.Calls) != 2 || len(payload.Appendix.Calculations) != 4 {
		t.Fatalf("appendix must be code-generated from the ledger: calls=%d calcs=%d",
			len(payload.Appendix.Calls), len(payload.Appendix.Calculations))
	}
	if payload.Appendix.Calls[0].CallID != "c1" || len(payload.Appendix.Calls[0].Observations) != 3 {
		t.Fatalf("appendix call observations must be the complete filtered set: %+v", payload.Appendix.Calls[0])
	}
	// material gaps（历史归属不可还原）+ 工具 gaps 继承进 appendix。
	reasons := map[string]bool{}
	for _, g := range payload.Appendix.Gaps {
		reasons[g.Reason] = true
	}
	if !reasons["泳道无历史归属记录：材料按当前泳道清单近似，周期内的退出/迁移不可还原"] {
		t.Fatalf("material gap must be inherited: %+v", payload.Appendix.Gaps)
	}
	// generation_meta 真实计数。
	meta := payload.GenerationMeta
	if meta.Attempts != 1 || meta.Retries != 0 || meta.Decisions != 5 || meta.SourceCalls != 2 ||
		meta.CalculationCalls != 4 || meta.StopReason != SignalStopReasonFinished ||
		meta.AnalysisMode != "current" || meta.SessionID != "board_signal_report_7_abcd1234" {
		t.Fatalf("generation_meta: %+v", meta)
	}
	if _, err := time.Parse(time.RFC3339, meta.Cutoff); err != nil {
		t.Fatalf("cutoff must be RFC3339: %v", err)
	}
	if payload.SchemaVersion != 2 {
		t.Fatalf("schema_version: %d", payload.SchemaVersion)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(payload.SignalSnapshot, &snapshot); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot["candidate_id"] != float64(7) || snapshot["research_question"] == nil {
		t.Fatalf("signal snapshot: %+v", snapshot)
	}
}

// compose 用户消息必须携带研究笔记 / 观测清单（引用宇宙）/ 缺口。
func TestSignalCompose_UserMessageCarriesLedger(t *testing.T) {
	ledger := scFixtureLedger()
	router := &scStubRouter{replies: []srComposeReply{{content: scDraft(scSections("c1:o1"), nil)}}}
	if _, err := scCompose(t, router, scInput(ledger)); err != nil {
		t.Fatalf("compose failed: %v", err)
	}
	user := router.calls[0].Messages[len(router.calls[0].Messages)-1].Content
	for _, want := range []string{"研究笔记", "c1:o1", "c2:o2", "数据缺口", "历史归属"} {
		if !strings.Contains(user, want) {
			t.Fatalf("compose user message missing %q", want)
		}
	}
	if req := router.calls[0]; req.SessionID != "board_signal_report_7_abcd1234" || req.Operation != signalComposeOperation {
		t.Fatalf("compose request identity: %+v", req)
	}
}

// ── 数据时效声明（design §10.3，tasks 4.9）：注入可用性行 + facts「数据截至」校验 ──

// scAsOfFixtureLedger 构造跨源混形态 period 的账本：jodi 月度+月内日（2026-06、
// 2026-07、2026-07-20——同月内日破平局）、wdi 年度（2024）、comtrade 混形态
// （"202603" vs "2026-07"——字符串序与规范化序相反，专测同源内按形态取大）、
// eia 取数失败无观测。
func scAsOfFixtureLedger() *signalResearchLedger {
	l := newSignalResearchLedger(srCutoff)
	obs := func(ref, callID, tool, period string, value *float64) *SignalCalcObservation {
		return &SignalCalcObservation{Ref: ref, CallID: callID, Tool: tool, SeriesID: "s|" + ref, Period: period, Value: value}
	}
	l.obsIndex = map[string]*SignalCalcObservation{
		"c1:o1": obs("c1:o1", "c1", "jodi_oil_primary", "2026-06", f64(100)),
		"c1:o2": obs("c1:o2", "c1", "jodi_oil_primary", "2026-07", f64(101)),
		"c1:o3": obs("c1:o3", "c1", "jodi_oil_primary", "2026-07-20", f64(102)),
		"c2:o1": obs("c2:o1", "c2", "wb_wdi", "2024", f64(1)),
		"c3:o1": obs("c3:o1", "c3", "un_comtrade_trade", "202603", f64(2)),
		"c3:o2": obs("c3:o2", "c3", "un_comtrade_trade", "2026-07", f64(3)),
	}
	l.calls = []*signalResearchCallRecord{
		{CallID: "c1", Tool: "jodi_oil_primary", Question: "q", Status: "ok", Observations: []map[string]any{}},
		{CallID: "c2", Tool: "wb_wdi", Question: "q", Status: "ok", Observations: []map[string]any{}},
		{CallID: "c3", Tool: "un_comtrade_trade", Question: "q", Status: "ok", Observations: []map[string]any{}},
		{CallID: "c4", Tool: "eia_wpsr_table1", Question: "q", Status: "error", Error: "SOURCE_UNAVAILABLE host 不在白名单", Observations: []map[string]any{}},
	}
	return l
}

// ① 注入行：各源规范化后 max period + 研究时点 cutoff；取数失败源如实列「无观测」。
func TestSignalCompose_AsOfLineInjected(t *testing.T) {
	ledger := scAsOfFixtureLedger()
	// 工具名升序：eia 无观测 < jodi < comtrade < wdi；jodi 同月内日破平局取
	// "2026-07-20"；comtrade 的字符串序最大值是 "202603"，规范化后正确答案是
	// "2026-07"。
	want := "数据可用性：eia_wpsr_table1 无观测；jodi_oil_primary 最新 2026-07-20；un_comtrade_trade 最新 2026-07；wb_wdi 最新 2024。研究时点 2026-09-10（晚于此的数据已被剔除）"
	sections := scSections("c1:o2")
	sections[1] = map[string]any{"kind": "facts", "text": "发生了什么。数据截至 2026-07。[[data:c1:o2]]"}
	router := &scStubRouter{replies: []srComposeReply{{content: scDraft(sections, nil)}}}
	if _, err := scCompose(t, router, scInput(ledger)); err != nil {
		t.Fatalf("compose failed: %v", err)
	}
	user := router.calls[0].Messages[len(router.calls[0].Messages)-1].Content
	if !strings.Contains(user, want) {
		t.Fatalf("availability line missing:\nwant: %s\ngot:  %s", want, user)
	}
}

// ② facts 含「数据截至」→ 通过。
func TestSignalCompose_FactsAsOfPresentPasses(t *testing.T) {
	ledger := scFixtureLedger()
	sections := scSections("c1:o1")
	sections[1] = map[string]any{"kind": "facts", "text": "发生了什么。数据截至 2026-08-03。[[data:c1:o1]]"}
	router := &scStubRouter{replies: []srComposeReply{{content: scDraft(sections, nil)}}}
	raw, err := scCompose(t, router, scInput(ledger))
	if err != nil {
		t.Fatalf("facts with 数据截至 must pass: %v", err)
	}
	if payload := scParsePayload(t, raw); payload.GenerationMeta.Attempts != 1 {
		t.Fatalf("must pass on first attempt: %+v", payload.GenerationMeta)
	}
}

// ③ facts 缺「数据截至」→ 回注重试且反馈点名时效要求。
func TestSignalCompose_FactsAsOfMissingRetried(t *testing.T) {
	ledger := scFixtureLedger()
	missing := scSections("c1:o1")
	missing[1] = map[string]any{"kind": "facts", "text": "发生了什么。[[data:c1:o1]]"}
	router := &scStubRouter{replies: []srComposeReply{
		{content: scDraft(missing, nil)},
		{content: scDraft(scSections("c1:o1"), nil)},
	}}
	raw, err := scCompose(t, router, scInput(ledger))
	if err != nil {
		t.Fatalf("retry path failed: %v", err)
	}
	if payload := scParsePayload(t, raw); payload.GenerationMeta.Attempts != 2 || payload.GenerationMeta.Retries != 1 {
		t.Fatalf("attempts/retries: %+v", payload.GenerationMeta)
	}
	feedback := router.calls[1].Messages[len(router.calls[1].Messages)-1].Content
	if !strings.Contains(feedback, "数据截至") || !strings.Contains(feedback, "时效") {
		t.Fatalf("retry feedback must name the as-of requirement: %s", feedback)
	}
}

// ⑤ 期值月粒度校验（review M1）：各源最新期的同月更细写法/中文年月/斜杠/
// 紧凑写法都通过——真话的更细写法不算编造。
func TestSignalCompose_AsOfPeriodMatchesSourceLatestMonth(t *testing.T) {
	ledger := scAsOfFixtureLedger()
	for _, asOf := range []string{"2026-07", "2026-07-20", "2026年7月", "2026/07", "202607", "2024"} {
		sections := scSections()
		sections[1] = map[string]any{"kind": "facts", "text": "发生了什么。数据截至 " + asOf + "。"}
		router := &scStubRouter{replies: []srComposeReply{{content: scDraft(sections, nil)}}}
		if _, err := scCompose(t, router, scInput(ledger)); err != nil {
			t.Fatalf("as-of %q must pass (month-granular latest match): %v", asOf, err)
		}
		if len(router.calls) != 1 {
			t.Fatalf("as-of %q must pass on the first attempt", asOf)
		}
	}
}

// ⑥ 编造/不匹配期值（未来期、与年度最新期不同月）→ 回注，反馈点名期值纪律。
func TestSignalCompose_FabricatedAsOfPeriodRetried(t *testing.T) {
	ledger := scFixtureLedger() // 真实最新期 2026-08-03（月 2026-08）
	for _, bad := range []string{"2030-01", "2024-05"} {
		sections := scSections("c1:o1")
		sections[1] = map[string]any{"kind": "facts", "text": "发生了什么。数据截至 " + bad + "。[[data:c1:o1]]"}
		router := &scStubRouter{replies: []srComposeReply{
			{content: scDraft(sections, nil)},
			{content: scDraft(scSections("c1:o1"), nil)},
		}}
		raw, err := scCompose(t, router, scInput(ledger))
		if err != nil {
			t.Fatalf("corrected as-of must pass on retry: %v", err)
		}
		if payload := scParsePayload(t, raw); payload.GenerationMeta.Attempts != 2 || payload.GenerationMeta.Retries != 1 {
			t.Fatalf("as-of %q must be rejected on attempt 1: %+v", bad, payload.GenerationMeta)
		}
		feedback := router.calls[1].Messages[len(router.calls[1].Messages)-1].Content
		for _, want := range []string{bad, "期值", "数据可用性", "不得编造"} {
			if !strings.Contains(feedback, want) {
				t.Fatalf("feedback for %q must mention %q: %s", bad, want, feedback)
			}
		}
	}
}

// ⑦ 提取不到期值（只写「数据截至」）→ 回注，不得靠字样蒙过。
func TestSignalCompose_AsOfPeriodMissingRetried(t *testing.T) {
	ledger := scFixtureLedger()
	sections := scSections("c1:o1")
	sections[1] = map[string]any{"kind": "facts", "text": "发生了什么。数据截至。[[data:c1:o1]]"}
	router := &scStubRouter{replies: []srComposeReply{
		{content: scDraft(sections, nil)},
		{content: scDraft(scSections("c1:o1"), nil)},
	}}
	if _, err := scCompose(t, router, scInput(ledger)); err != nil {
		t.Fatalf("corrected as-of must pass on retry: %v", err)
	}
	feedback := router.calls[1].Messages[len(router.calls[1].Messages)-1].Content
	for _, want := range []string{"期值", "数据可用性", "不得编造"} {
		if !strings.Contains(feedback, want) {
			t.Fatalf("feedback must mention %q: %s", want, feedback)
		}
	}
}

// ⑧ 「无可用数据期」只在零观测账本合法：零观测写期值=编造，非零观测写它=非法。
func TestSignalCompose_NoDataPeriodOnlyForZeroObservations(t *testing.T) {
	// 零观测账本 + 期值 → 编造，回注。
	zero := newSignalResearchLedger(srCutoff)
	zero.calls = []*signalResearchCallRecord{
		{CallID: "c1", Tool: "jodi_oil_primary", Question: "q", Status: "error", Observations: []map[string]any{}},
	}
	fabricated := scSections()
	fabricated[1] = map[string]any{"kind": "facts", "text": "数据截至 2026-07。"}
	router := &scStubRouter{replies: []srComposeReply{{content: scDraft(fabricated, nil)}}}
	if _, err := scCompose(t, router, scInput(zero)); err == nil || !strings.Contains(err.Error(), "无可用数据观测") {
		t.Fatalf("zero-observation ledger must reject any period value: %v", err)
	}
	// 非零观测账本 + 「无可用数据期」→ 非法，回注。
	ledger := scFixtureLedger()
	sections := scSections("c1:o1")
	sections[1] = map[string]any{"kind": "facts", "text": "数据截至：无可用数据期。[[data:c1:o1]]"}
	router = &scStubRouter{replies: []srComposeReply{{content: scDraft(sections, nil)}}}
	if _, err := scCompose(t, router, scInput(ledger)); err == nil || !strings.Contains(err.Error(), "有可用观测") {
		t.Fatalf("non-zero ledger must reject 无可用数据期: %v", err)
	}
}

// ⑨ 零观测账本：注入「本研究无可用数据观测」，facts 按要求写「数据截至：无可用数据期」→ 通过。
func TestSignalCompose_ZeroObservationNoDataPeriod(t *testing.T) {
	ledger := newSignalResearchLedger(srCutoff)
	ledger.calls = []*signalResearchCallRecord{
		{CallID: "c1", Tool: "jodi_oil_primary", Question: "q", Status: "error", Error: "host 不在白名单", Observations: []map[string]any{}},
	}
	in := scInput(ledger)
	sections := scSections()
	sections[1] = map[string]any{"kind": "facts", "text": "研究员没有取得任何数据观测。数据截至：无可用数据期。"}
	router := &scStubRouter{replies: []srComposeReply{{content: scDraft(sections, nil)}}}
	raw, err := scCompose(t, router, in)
	if err != nil {
		t.Fatalf("zero-observation as-of wording must pass: %v", err)
	}
	if payload := scParsePayload(t, raw); payload.GenerationMeta.Attempts != 1 {
		t.Fatalf("must pass on first attempt: %+v", payload.GenerationMeta)
	}
	user := router.calls[0].Messages[len(router.calls[0].Messages)-1].Content
	if !strings.Contains(user, "本研究无可用数据观测") || !strings.Contains(user, "研究时点 2026-09-10") {
		t.Fatalf("zero-observation availability line missing: %s", user)
	}
}
