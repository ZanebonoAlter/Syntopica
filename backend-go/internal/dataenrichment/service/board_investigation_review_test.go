package service

import (
	"encoding/json"
	"strings"
	"testing"
)

// ── boardInvestigationComparableFields 纯函数（task 4.7 调查 review 隔离）──
//
// 契约（test-cases M8.7/M8.8 + 调度契约 3）：调查 review judge prompt 的唯一
// 载荷构造器——只放行 question、hypotheses 的 id/label/is_null/assessment/
// confidence/support_evidence/counter_evidence/gaps/scope/derived_from、
// conclusion、evidence 元数据（quote 逐字摘录除外）。tool_calls/method 正文/
// parent brief 全文/thesis/argument/depth 一律不出现；坏 JSON / 非调查形态
// 返回 nil（judge 跳过而非比较空壳）。

// 坏 JSON：不可解析 → nil，不 panic。
func TestBoardInvestigationComparableFields_BadJSON(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"truncated":  json.RawMessage(`{"question":{"text":"x"},"hypotheses":[`),
		"not object": json.RawMessage(`["hypotheses"]`),
		"empty":      json.RawMessage(``),
	} {
		if got := boardInvestigationComparableFields(raw); got != nil {
			t.Fatalf("%s: bad JSON must yield nil, got %s", name, got)
		}
	}
}

// 非调查形态 → nil：legacy 论文（thesis/argument/depth 无 hypotheses）、
// 简报载荷（summary/observations）、缺 hypotheses 的残缺调查。
func TestBoardInvestigationComparableFields_NotInvestigationShaped(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"legacy thesis":  json.RawMessage(`{"thesis":"X 不是 A 而是 B","argument":{},"depth":{}}`),
		"brief payload":  json.RawMessage(`{"summary":"概览","observations":[{"id":"o1"}],"relationships":[],"uncertainties":[]}`),
		"no hypotheses":  json.RawMessage(`{"question":{"text":"q"},"conclusion":{"summary":"c"}}`),
		"hyp not array":  json.RawMessage(`{"question":{"text":"q"},"hypotheses":{"id":"h0"},"conclusion":{"summary":"c"}}`),
		"wrong kind tag": json.RawMessage(`{"result_kind":"legacy_board_analysis","question":{"text":"q"},"hypotheses":[{"id":"h0"}],"conclusion":{"summary":"c"}}`),
	} {
		if got := boardInvestigationComparableFields(raw); got != nil {
			t.Fatalf("%s: non-investigation payload must yield nil, got %s", name, got)
		}
	}
}

// 允许清单投影：顶层只出 question/hypotheses/conclusion/evidence_chain；
// 假设条目只出 11 个允许字段（stray 字段丢弃）；证据条目出元数据但
// quote（逐字摘录，比较状态用不上且撑爆预算）必须丢弃；
// method_refs/lane_refs/scope/result_kind/parent_briefing_id/retry_reason
// 一律不出现。
func TestBoardInvestigationComparableFields_AllowedProjectionOnly(t *testing.T) {
	raw := json.RawMessage(`{"scope":"board","result_kind":"board_investigation","parent_briefing_id":12,
		"question":{"id":"q1","text":"两条泳道是否由同一资金驱动","source":"generated"},
		"hypotheses":[
		 {"id":"h0","label":"无统一机制","is_null":true,"derived_from":[],"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":["e2"],"counter_evidence":[],"gaps":[],"rank":"stray-secret"},
		 {"id":"h1","label":"产业基金推动","is_null":false,"derived_from":["h1"],"assessment":"supported","confidence":"medium","scope":"近三月","support_evidence":["e1"],"counter_evidence":["e9"],"gaps":["资金明细未核实"]}],
		"conclusion":{"summary":"h1 获支持仍有缺口","confidence":"medium","scope":"两条泳道","boundary":"资金明细未核实"},
		"evidence_chain":[
		 {"id":"e1","source_type":"web","url":"https://example.com/a","quote":"QUOTE-MARKER-逐字摘录","institution":"示例研究所","date":"2026-08-20","supports":["h1"],"counters":[]},
		 {"id":"e2","source_type":"lane","ref":"901","lane_note":"产能与招标","supports":["h0"],"counters":[]}],
		"lane_refs":[{"lane_id":901,"note":"主泳道"}],
		"method_refs":[{"id":3,"title":"因果链检验","content_hash":"abc"}],
		"retry_reason":"structure"}`)
	got := boardInvestigationComparableFields(raw)
	if got == nil {
		t.Fatal("investigation-shaped payload must project, got nil")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(got, &top); err != nil {
		t.Fatalf("projected payload not JSON: %v", err)
	}
	// 顶层允许清单：恰好四键。
	wantKeys := []string{"question", "hypotheses", "conclusion", "evidence_chain"}
	if len(top) != len(wantKeys) {
		t.Fatalf("top-level keys must be exactly %v, got %d keys: %s", wantKeys, len(top), got)
	}
	for _, k := range wantKeys {
		if _, ok := top[k]; !ok {
			t.Fatalf("projected payload missing %q: %s", k, got)
		}
	}

	// 假设条目字段允许清单（恰好 11 字段，stray 丢弃）。
	var hyps []map[string]json.RawMessage
	if err := json.Unmarshal(top["hypotheses"], &hyps); err != nil {
		t.Fatalf("hypotheses not a list: %v", err)
	}
	if len(hyps) != 2 {
		t.Fatalf("want 2 hypotheses, got %d", len(hyps))
	}
	wantHypFields := []string{"id", "label", "is_null", "derived_from", "assessment",
		"confidence", "scope", "support_evidence", "counter_evidence", "gaps"}
	if len(hyps[0]) != len(wantHypFields) {
		t.Fatalf("hypothesis keys must be exactly %v, got %d: %s", wantHypFields, len(hyps[0]), top["hypotheses"])
	}
	for _, k := range wantHypFields {
		if _, ok := hyps[0][k]; !ok {
			t.Fatalf("hypothesis missing %q: %s", k, top["hypotheses"])
		}
	}

	// 证据条目：元数据保留、quote 丢弃。
	var evs []map[string]json.RawMessage
	if err := json.Unmarshal(top["evidence_chain"], &evs); err != nil {
		t.Fatalf("evidence_chain not a list: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("want 2 evidence items, got %d", len(evs))
	}
	for i, ev := range evs {
		if _, ok := ev["quote"]; ok {
			t.Fatalf("evidence[%d].quote must be dropped: %s", i, top["evidence_chain"])
		}
		for _, k := range []string{"id", "source_type", "supports", "counters"} {
			if _, ok := ev[k]; !ok {
				t.Fatalf("evidence[%d] missing metadata %q: %s", i, k, top["evidence_chain"])
			}
		}
	}
	if _, ok := evs[0]["url"]; !ok {
		t.Fatalf("web evidence must keep url: %s", top["evidence_chain"])
	}

	// 字符串级禁区验证：legacy 词汇、方法卡正文/标题、lane_refs 注记、
	// quote 摘录、快照杂键一个都不许出现。
	s := string(got)
	for _, banned := range []string{"thesis", "argument", "depth", "因果链检验", "主泳道", "QUOTE-MARKER", "retry_reason", "parent_briefing_id", "result_kind", "stray-secret"} {
		if strings.Contains(s, banned) {
			t.Fatalf("banned content %q leaked into comparable payload: %s", banned, s)
		}
	}
}

// 最小合法载荷：question + hypotheses + conclusion（无 evidence_chain）
// 不补空键，evidence_chain 缺省不出现。
func TestBoardInvestigationComparableFields_MinimalPayload(t *testing.T) {
	got := boardInvestigationComparableFields(json.RawMessage(
		`{"question":{"text":"q"},"hypotheses":[{"id":"h0","label":"零","is_null":true}],"conclusion":{"summary":"c"}}`))
	if got == nil {
		t.Fatal("minimal investigation payload must project")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(got, &top); err != nil {
		t.Fatalf("projected payload not JSON: %v", err)
	}
	if len(top) != 3 {
		t.Fatalf("minimal payload must keep exactly question/hypotheses/conclusion, got %d keys: %s", len(top), got)
	}
	if _, ok := top["evidence_chain"]; ok {
		t.Fatalf("missing evidence_chain must stay missing, got %s", got)
	}
}
