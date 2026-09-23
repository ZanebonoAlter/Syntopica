package wiring

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"syntopica-backend/internal/datasources"
	"syntopica-backend/internal/datasources/sources"
)

// stubEIA implements the narrow fetcher interfaces with canned results.
type stubEIA struct {
	res   map[string]any
	err   error
	weeks []int // captured variadic window
}

func (s *stubEIA) Fetch(ctx context.Context, section string, weeks ...int) (map[string]any, error) {
	s.weeks = weeks
	return s.res, s.err
}

type stubJODI struct {
	res   map[string]any
	err   error
	years []int // captured variadic window
}

func (s *stubJODI) Fetch(ctx context.Context, geo, flow, unit, month string, years ...int) (map[string]any, error) {
	s.years = years
	s.res["_args"] = map[string]string{"geo": geo, "flow": flow, "unit": unit, "month": month}
	return s.res, s.err
}

type stubWDI struct {
	res map[string]any
	err error
}

func (s stubWDI) Fetch(ctx context.Context, indicator string, countries []string, from, to int) (map[string]any, error) {
	s.res["_args"] = map[string]any{"indicator": indicator, "countries": countries, "from": from, "to": to}
	return s.res, s.err
}

type stubComtrade struct {
	res map[string]any
	err error
}

func (s stubComtrade) Fetch(ctx context.Context, p sources.ComtradeParams) (map[string]any, error) {
	s.res["_args"] = p
	return s.res, s.err
}

func TestResearchToolsNoURLParameters(t *testing.T) {
	tools := ResearchTools(&stubEIA{}, &stubJODI{}, &stubWDI{}, &stubComtrade{})
	for _, tool := range tools {
		schema := tool.InputSchema
		b, _ := json.Marshal(schema)
		lower := strings.ToLower(string(b))
		if strings.Contains(lower, "url") || strings.Contains(lower, "path") {
			t.Fatalf("%s: InputSchema must not expose url/path parameters: %s", tool.Name, b)
		}
	}
	if len(tools) != 4 {
		t.Fatalf("want 4 tools, got %d", len(tools))
	}
}

func TestEiaToolSectionAndErrors(t *testing.T) {
	tools := ResearchTools(&stubEIA{res: map[string]any{"observations": []map[string]any{{"flow": "x"}}}}, &stubJODI{}, &stubWDI{}, &stubComtrade{})
	var eia = tools[0]
	out, err := eia.Execute(context.Background(), map[string]any{"section": "stocks"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "observations") {
		t.Fatalf("output should embed result: %s", out)
	}

	// Typed error → structured error_code JSON, NOT a Go error (the agent loop
	// consumes tool output).
	errTools := ResearchTools(&stubEIA{err: datasources.SchemaChanged("eia_wpsr", "上游结构漂移")}, &stubJODI{}, &stubWDI{}, &stubComtrade{})
	out, err = errTools[0].Execute(context.Background(), map[string]any{"section": "stocks"})
	if err != nil {
		t.Fatalf("typed errors are tool output, not Go errors: %v", err)
	}
	if !strings.Contains(out, "SCHEMA_CHANGED") {
		t.Fatalf("output should carry error_code: %s", out)
	}
}

func TestJodiToolArgPassing(t *testing.T) {
	tools := ResearchTools(&stubEIA{}, &stubJODI{res: map[string]any{}}, &stubWDI{}, &stubComtrade{})
	out, err := tools[1].Execute(context.Background(), map[string]any{
		"geo": "US", "flow": "production",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, `"unit":""`) && !strings.Contains(out, `"unit": ""`) {
		t.Fatalf("unit defaults to empty string (flow default resolved in fetcher): %s", out)
	}
	if !strings.Contains(out, `"month":""`) && !strings.Contains(out, `"month": ""`) {
		t.Fatalf("month defaults to empty string (latest period): %s", out)
	}
}

func TestWdiToolArgConversion(t *testing.T) {
	tools := ResearchTools(&stubEIA{}, &stubJODI{}, &stubWDI{res: map[string]any{}}, &stubComtrade{})
	out, err := tools[2].Execute(context.Background(), map[string]any{
		"indicator": "NE.EXP.GNFS.ZS",
		"countries": []any{"CHN", "JPN"},
		"from_year": float64(2020), // JSON numbers arrive as float64
		"to_year":   float64(2024),
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"CHN", "JPN", `"from":2020`, `"to":2024`} {
		if !strings.Contains(out, want) {
			t.Fatalf("args must convert correctly (missing %s): %s", want, out)
		}
	}
}

func TestComtradeToolPartnerOptional(t *testing.T) {
	// With partner → pointer set.
	tools := ResearchTools(&stubEIA{}, &stubJODI{}, &stubWDI{}, &stubComtrade{res: map[string]any{}})
	out, err := tools[3].Execute(context.Background(), map[string]any{
		"freq_code": "A", "reporter": float64(156), "partner": float64(682),
		"cmd_code": "2709", "flow_code": "M", "period": "2024",
	})
	if err != nil || !strings.Contains(out, `"Partner":682`) {
		t.Fatalf("partner conversion failed: err=%v out=%s", err, out)
	}
	// Without partner → nil (World aggregate + all partners returned).
	out, err = tools[3].Execute(context.Background(), map[string]any{
		"freq_code": "A", "reporter": float64(156),
		"cmd_code": "2709", "flow_code": "M", "period": "2024",
	})
	if err != nil || !strings.Contains(out, `"Partner":null`) {
		t.Fatalf("absent partner must stay nil: err=%v out=%s", err, out)
	}
}

// ── explicit window params (HR-1/3/6/7): optional, strict, old surface intact ──

// TestResearchToolsToolSurfaceUnchanged pins the registered tool registry:
// exactly the four research tools, names unchanged, and the new optional
// window params (weeks/years) are NOT required — old callers' argument sets
// stay valid (HR-7: existing tool surface zero change outside new research).
func TestResearchToolsToolSurfaceUnchanged(t *testing.T) {
	tools := ResearchTools(&stubEIA{}, &stubJODI{}, &stubWDI{}, &stubComtrade{})
	want := []string{"eia_wpsr_table1", "jodi_oil_primary", "wb_wdi", "un_comtrade_trade"}
	if len(tools) != len(want) {
		t.Fatalf("want %d tools, got %d", len(want), len(tools))
	}
	for i, name := range want {
		if tools[i].Name != name {
			t.Fatalf("tool[%d] = %s, want %s", i, tools[i].Name, name)
		}
	}
	for i, key := range map[int]string{0: "weeks", 1: "years"} {
		required, _ := tools[i].InputSchema["required"].([]string)
		for _, r := range required {
			if r == key {
				t.Fatalf("%s must not require %s (old callers unaffected)", tools[i].Name, key)
			}
		}
	}
}

func TestEiaToolWeeksPassThrough(t *testing.T) {
	eia := &stubEIA{res: map[string]any{}}
	tools := ResearchTools(eia, &stubJODI{}, &stubWDI{}, &stubComtrade{})

	// Absent → no window (default mode), zero-valued variadic.
	if _, err := tools[0].Execute(context.Background(), map[string]any{"section": "stocks"}); err != nil {
		t.Fatalf("absent weeks: %v", err)
	}
	if len(eia.weeks) != 0 {
		t.Fatalf("absent weeks must not pass a window, got %v", eia.weeks)
	}
	// Valid integer (JSON number) → passed through.
	if _, err := tools[0].Execute(context.Background(), map[string]any{"section": "stocks", "weeks": float64(12)}); err != nil {
		t.Fatalf("weeks=12: %v", err)
	}
	if len(eia.weeks) != 1 || eia.weeks[0] != 12 {
		t.Fatalf("weeks=12 must pass through, got %v", eia.weeks)
	}
	// Fractional / string / out-of-range → INVALID_ARGUMENT error JSON before
	// the fetcher is ever invoked (HR-1: rejected pre-network).
	for _, bad := range []any{float64(6.5), "6", float64(13), true} {
		out, err := tools[0].Execute(context.Background(), map[string]any{"section": "stocks", "weeks": bad})
		if err != nil {
			t.Fatalf("weeks=%v: typed errors are tool output, not Go errors: %v", bad, err)
		}
		if !strings.Contains(out, "INVALID_ARGUMENT") {
			t.Fatalf("weeks=%v: want INVALID_ARGUMENT, got %s", bad, out)
		}
	}
}

func TestJodiToolYearsPassThrough(t *testing.T) {
	jodi := &stubJODI{res: map[string]any{}}
	tools := ResearchTools(&stubEIA{}, jodi, &stubWDI{}, &stubComtrade{})

	if _, err := tools[1].Execute(context.Background(), map[string]any{"geo": "US", "flow": "production", "years": float64(5)}); err != nil {
		t.Fatalf("years=5: %v", err)
	}
	if len(jodi.years) != 1 || jodi.years[0] != 5 {
		t.Fatalf("years=5 must pass through, got %v", jodi.years)
	}
	for _, bad := range []any{float64(2.5), "3", float64(6)} {
		out, err := tools[1].Execute(context.Background(), map[string]any{"geo": "US", "flow": "production", "years": bad})
		if err != nil {
			t.Fatalf("years=%v: %v", bad, err)
		}
		if !strings.Contains(out, "INVALID_ARGUMENT") {
			t.Fatalf("years=%v: want INVALID_ARGUMENT, got %s", bad, out)
		}
	}
	// month + years both present still reaches the fetcher (the source owns
	// the mutual-exclusion ruling pre-network); wiring only guards types.
	jodi.years = nil
	if _, err := tools[1].Execute(context.Background(), map[string]any{"geo": "US", "flow": "production", "month": "2026-01", "years": float64(3)}); err != nil {
		t.Fatalf("month+years pass-through: %v", err)
	}
	if len(jodi.years) != 1 {
		t.Fatalf("mutex enforcement belongs to the source, got %v", jodi.years)
	}
}

// ── capability & availability declarations (design §10.1 research side) ──

// TestBuildToolsDescriptionsCarryCatalogMetadata: every tool description is
// enriched with catalog metadata derived from datasources.Definition at
// runtime (never a second hand-copied set), the ensemble hard limits, and the
// Comtrade measured-coverage caveat; a configured key must NOT trigger any
// unavailability wording.
func TestBuildToolsDescriptionsCarryCatalogMetadata(t *testing.T) {
	tools := BuildTools(func(configKeyName string) string {
		if configKeyName == "COMTRADE_API_KEY" {
			return "test-key"
		}
		return ""
	})
	codes := []string{"eia_wpsr", "jodi_oil_primary", "wb_wdi", "un_comtrade"}
	for i, tool := range tools {
		d, ok := datasources.Definition(codes[i])
		if !ok {
			t.Fatalf("catalog lost definition %s", codes[i])
		}
		for _, field := range []string{d.Coverage, d.Frequency, d.TypicalLag, d.UnitPolicy} {
			if !strings.Contains(tool.Description, field) {
				t.Fatalf("%s: description must carry catalog metadata %q: %s", tool.Name, field, tool.Description)
			}
		}
		if !strings.Contains(tool.Description, "无价格序列、裂解价差、运价") {
			t.Fatalf("%s: description must declare the ensemble hard limits: %s", tool.Name, tool.Description)
		}
		if strings.Contains(tool.Description, "当前不可用") {
			t.Fatalf("%s: must not claim unavailability while its key is configured: %s", tool.Name, tool.Description)
		}
	}
	if !strings.Contains(tools[3].Description, "count=0") || !strings.Contains(tools[3].Description, "不是调用失败") {
		t.Fatalf("comtrade description must declare the measured-coverage caveat: %s", tools[3].Description)
	}
}

// TestBuildToolsMarksUnconfiguredKeySourceUnavailable: with no key configured,
// exactly the RequiresKey source is marked「当前不可用：未配置
// COMTRADE_API_KEY」— but at RESEARCH time via UnavailableNoticeProbe, not
// baked into the BuildTools startup snapshot (design §10.4 live resolver；
// review M2）. Execute semantics are untouched by construction.
func TestBuildToolsMarksUnconfiguredKeySourceUnavailable(t *testing.T) {
	tools := BuildTools(func(string) string { return "" })
	for _, tool := range tools {
		if strings.Contains(tool.Description, "当前不可用") {
			t.Fatalf("%s: BuildTools must not bake a startup availability snapshot: %s", tool.Name, tool.Description)
		}
	}
	// 新路径等价行为：现算 probe 只标 RequiresKey 且 key 为空的源。
	probe := UnavailableNoticeProbe(func(string) string { return "" })
	for i, tool := range tools {
		has := strings.Contains(probe(tool.Name), "当前不可用：未配置 COMTRADE_API_KEY")
		if i == 3 && !has {
			t.Fatalf("un_comtrade_trade: key missing, research-time probe must mark it unavailable")
		}
		if i != 3 && has {
			t.Fatalf("%s: requires no key, probe must not mark it unavailable", tool.Name)
		}
	}
}

// TestUnavailableNoticeProbeMirrorsStatusFor: the research-time availability
// probe and the catalog status share one judgment (RequiresKey + empty key
// via the same live resolver)，UI 保存 key 后两者同步翻转、无需重启
// (review M2 / design §10.4).
func TestUnavailableNoticeProbeMirrorsStatusFor(t *testing.T) {
	codes := []string{"eia_wpsr", "jodi_oil_primary", "wb_wdi", "un_comtrade"}
	names := []string{"eia_wpsr_table1", "jodi_oil_primary", "wb_wdi", "un_comtrade_trade"}
	for _, keyConfigured := range []bool{false, true} {
		resolve := datasources.KeyResolver(func(string) string {
			if keyConfigured {
				return "test-key"
			}
			return ""
		})
		probe := UnavailableNoticeProbe(resolve)
		for i, code := range codes {
			d, ok := datasources.Definition(code)
			if !ok {
				t.Fatalf("catalog lost definition %s", code)
			}
			status, _ := datasources.StatusFor(d, resolve)
			notice := probe(names[i])
			wantUnavailable := status == datasources.StatusDisabled
			if wantUnavailable != (notice != "") {
				t.Fatalf("probe/StatusFor drift for %s (keyConfigured=%v): status=%s notice=%q", code, keyConfigured, status, notice)
			}
			if wantUnavailable && !strings.Contains(notice, d.ConfigKeyName) {
				t.Fatalf("%s: notice must name %s: %s", code, d.ConfigKeyName, notice)
			}
		}
		if got := probe("list_boards"); got != "" {
			t.Fatalf("non-catalog tool must stay untouched: %q", got)
		}
	}
	if got := UnavailableNoticeProbe(nil)("un_comtrade_trade"); got != "" {
		t.Fatalf("nil resolver must report available: %q", got)
	}
}
