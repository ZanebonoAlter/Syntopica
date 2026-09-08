package service

import (
	"encoding/json"
	"strings"
	"testing"
)

// ── boardBriefComparableFields 纯函数（tasks 3.5 review 修复）───────────────
//
// 契约：judge prompt 的唯一载荷构造器——只放行
// summary/observations/relationships/uncertainties/lane_refs 五字段；
// 坏 JSON / 缺 summary 返回 nil（judge 跳过而非比较空壳）；
// stray thesis/argument/depth（legacy 论文形态）一律丢弃，永不进 prompt。

// 坏 JSON：不可解析 → nil，不 panic。
func TestBoardBriefComparableFields_BadJSON(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"truncated":  json.RawMessage(`{"summary":"s","observations":[`),
		"not object": json.RawMessage(`["summary"]`),
		"empty":      json.RawMessage(``),
	} {
		if got := boardBriefComparableFields(raw); got != nil {
			t.Fatalf("%s: bad JSON must yield nil, got %s", name, got)
		}
	}
}

// 缺 summary：不是 brief 形态 → nil（拒绝比较空壳，legacy/调查载荷在此被挡）。
func TestBoardBriefComparableFields_MissingSummary(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"thesis only":  json.RawMessage(`{"thesis":"X 不是 A 而是 B","argument":{},"depth":{}}`),
		"empty object": json.RawMessage(`{"observations":[],"relationships":[]}`),
	} {
		if got := boardBriefComparableFields(raw); got != nil {
			t.Fatalf("%s: payload without summary must yield nil, got %s", name, got)
		}
	}
}

// stray 字段丢弃：合法 brief 字段保留，thesis/argument/depth/research_questions
// 等其余键一律不出现（research_questions 是生成侧字段，不进比较载荷）。
func TestBoardBriefComparableFields_StrayFieldsDropped(t *testing.T) {
	raw := json.RawMessage(`{"summary":"概览","observations":[{"id":"o1"}],"relationships":[],
		"uncertainties":[],"lane_refs":[{"lane_id":1}],
		"thesis":"幽灵命题","argument":{"layers":[]},"depth":{"system_reframe":"x"},
		"research_questions":[{"id":"q1"}],"result_kind":"board_brief","degraded":false}`)
	got := boardBriefComparableFields(raw)
	if got == nil {
		t.Fatal("brief-shaped payload must project, got nil")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("projected payload not JSON: %v", err)
	}
	for _, keep := range []string{"summary", "observations", "relationships", "uncertainties", "lane_refs"} {
		if _, ok := m[keep]; !ok {
			t.Fatalf("projected payload missing %q: %s", keep, got)
		}
	}
	for _, drop := range []string{"thesis", "argument", "depth", "research_questions", "result_kind", "degraded"} {
		if _, ok := m[drop]; ok {
			t.Fatalf("stray key %q must be dropped: %s", drop, got)
		}
	}
	// 载荷内容本身不允许携带 legacy 词汇（hard guard 的字符串级验证）。
	if s := string(got); strings.Contains(s, "thesis") || strings.Contains(s, "argument") || strings.Contains(s, "depth") {
		t.Fatalf("legacy thesis vocabulary leaked into comparable payload: %s", s)
	}
}

// 只有 summary 也合法：其余四字段缺省不补空键。
func TestBoardBriefComparableFields_SummaryOnly(t *testing.T) {
	got := boardBriefComparableFields(json.RawMessage(`{"summary":"只有概览"}`))
	if got == nil {
		t.Fatal("summary-only payload is brief-shaped")
	}
	if string(got) != `{"summary":"只有概览"}` {
		t.Fatalf("unexpected projection: %s", got)
	}
}
