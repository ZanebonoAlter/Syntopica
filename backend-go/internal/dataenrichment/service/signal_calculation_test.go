package service

import (
	"math/big"
	"strings"
	"testing"
	"time"
)

// 受限计算解释器单元测试（board-signal-reports tasks 3.5，CA-1..CA-7）。
// 纯逻辑、无 DB、无 SQLite、无 LLM：解释器只依赖观测索引。

var calcNow = time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)

func calcObs(ref, tool, series, unit, period, flow string, value *float64) *SignalCalcObservation {
	return &SignalCalcObservation{
		Ref: ref, CallID: strings.SplitN(ref, ":", 2)[0], Tool: tool,
		SeriesID: series, Unit: unit, Period: period, Flow: flow, Value: value,
	}
}

func f64(v float64) *float64 { return &v }

// calcEIAIndex 构造样张同款 EIA 索引：c1=stocks（MMbbl），c2=supply（Mb/d）。
func calcEIAIndex() map[string]*SignalCalcObservation {
	return map[string]*SignalCalcObservation{
		"c1:o1": calcObs("c1:o1", "eia_wpsr_table1", "eia:US commercial crude (excl SPR)", "MMbbl", "2026-09-04", "stocks", f64(420.0)),
		"c1:o2": calcObs("c1:o2", "eia_wpsr_table1", "eia:US commercial crude (excl SPR)", "MMbbl", "2026-09-11", "stocks", f64(419.0)),
		"c1:o3": calcObs("c1:o3", "eia_wpsr_table1", "eia:US SPR", "MMbbl", "2026-09-04", "stocks", f64(400.0)),
		"c2:o1": calcObs("c2:o1", "eia_wpsr_table1", "eia:US imports", "Mb/d", "2026-09-04", "imports", f64(6000)),
		"c2:o2": calcObs("c2:o2", "eia_wpsr_table1", "eia:US imports", "Mb/d", "2026-09-11", "imports", f64(6400)),
		"c2:o3": calcObs("c2:o3", "eia_wpsr_table1", "eia:US exports", "Mb/d", "2026-09-04", "exports", f64(3300)),
		"c2:o4": calcObs("c2:o4", "eia_wpsr_table1", "eia:US exports", "Mb/d", "2026-09-11", "exports", f64(3600)),
	}
}

func runCalc(t *testing.T, index map[string]*SignalCalcObservation, op string, inputs ...string) *SignalCalculationResult {
	t.Helper()
	return runSignalCalculation(index, SignalCalculationRequest{Op: op, Inputs: inputs, Question: "q"}, "k1", calcNow)
}

// CA-1：同系列相邻两周库存差 → −1 MMbbl，带公式与输入。
func TestSignalCalc_DifferenceSameSeriesAdjacentPeriods(t *testing.T) {
	res := runCalc(t, calcEIAIndex(), "difference", "c1:o2", "c1:o1")
	if res.Status != SignalCalcStatusOK {
		t.Fatalf("status=%s reason=%s", res.Status, res.Reason)
	}
	if res.Value != "-1" || res.Unit != "MMbbl" {
		t.Fatalf("want -1 MMbbl, got %s %s", res.Value, res.Unit)
	}
	if res.Expression != "c1:o2 - c1:o1" {
		t.Fatalf("expression: %q", res.Expression)
	}
	if res.Precision != 4 || res.CalcID != "k1" || res.ComputedAt == "" {
		t.Fatalf("record fields: %+v", res)
	}
}

// CA-2：百分比变化 (419−420)/420×100 → −0.2381%，half-up 4 位。
func TestSignalCalc_PercentChangeHalfUpFourDigits(t *testing.T) {
	res := runCalc(t, calcEIAIndex(), "percent_change", "c1:o2", "c1:o1")
	if res.Status != SignalCalcStatusOK {
		t.Fatalf("status=%s reason=%s", res.Status, res.Reason)
	}
	if res.Value != "-0.2381" || res.Unit != "%" {
		t.Fatalf("want -0.2381%%, got %s %s", res.Value, res.Unit)
	}
}

// CA-3：同源同期间批准流量对 imports−exports → 2800 Mb/d。
func TestSignalCalc_DifferenceApprovedTrafficPairSamePeriod(t *testing.T) {
	res := runCalc(t, calcEIAIndex(), "difference", "c2:o2", "c2:o4")
	if res.Status != SignalCalcStatusOK {
		t.Fatalf("status=%s reason=%s", res.Status, res.Reason)
	}
	if res.Value != "2800" || res.Unit != "Mb/d" {
		t.Fatalf("want 2800 Mb/d, got %s %s", res.Value, res.Unit)
	}
}

// CA-4：mean 明确同系列窗口 → 算术均值；缺期不称完整窗口。
func TestSignalCalc_MeanExplicitWindowAndMissingPeriod(t *testing.T) {
	index := calcEIAIndex()
	index["c1:o0"] = calcObs("c1:o0", "eia_wpsr_table1", "eia:US commercial crude (excl SPR)", "MMbbl", "2026-08-28", "stocks", f64(421.0))
	res := runCalc(t, index, "mean", "c1:o0", "c1:o1", "c1:o2")
	if res.Status != SignalCalcStatusOK || res.Value != "420" || res.Unit != "MMbbl" {
		t.Fatalf("mean 3 periods: status=%s value=%s unit=%s reason=%s", res.Status, res.Value, res.Unit, res.Reason)
	}

	index["c1:o9"] = calcObs("c1:o9", "eia_wpsr_table1", "eia:US commercial crude (excl SPR)", "MMbbl", "2026-08-21", "stocks", nil)
	missing := runCalc(t, index, "mean", "c1:o9", "c1:o0", "c1:o1", "c1:o2")
	if missing.Status != SignalCalcStatusMissing {
		t.Fatalf("mean with null member must be missing, got %s", missing.Status)
	}
	if missing.Value != "" || missing.Reason == "" || !strings.Contains(missing.Reason, "c1:o9") {
		t.Fatalf("missing record: %+v", missing)
	}
}

// CA-5：任一 null → missing 不填 0；percent_change 基数非正拒绝。
func TestSignalCalc_MissingInputsAndNonPositiveBase(t *testing.T) {
	index := calcEIAIndex()
	index["c1:o4"] = calcObs("c1:o4", "eia_wpsr_table1", "eia:US commercial crude (excl SPR)", "MMbbl", "2026-08-28", "stocks", nil)

	res := runCalc(t, index, "difference", "c1:o2", "c1:o4")
	if res.Status != SignalCalcStatusMissing || res.Value != "" || res.Reason == "" {
		t.Fatalf("null input must be missing with reason, got %+v", res)
	}

	zero := runCalc(t, index, "percent_change", "c1:o2", "c2:o1") // series 不同 + 单位不同，先撞兼容
	if zero.Status != SignalCalcStatusRejected {
		t.Fatalf("cross-series percent must be rejected first, got %s (%s)", zero.Status, zero.Reason)
	}
	index["c1:b0"] = calcObs("c1:b0", "eia_wpsr_table1", "eia:US commercial crude (excl SPR)", "MMbbl", "2026-08-28", "stocks", f64(0))
	res = runCalc(t, index, "percent_change", "c1:o2", "c1:b0")
	if res.Status != SignalCalcStatusRejected || !strings.Contains(res.Reason, "非正") {
		t.Fatalf("base=0 must be rejected: %+v", res)
	}
	index["c1:b1"] = calcObs("c1:b1", "eia_wpsr_table1", "eia:US commercial crude (excl SPR)", "MMbbl", "2026-08-28", "stocks", f64(-5))
	res = runCalc(t, index, "percent_change", "c1:o2", "c1:b1")
	if res.Status != SignalCalcStatusRejected {
		t.Fatalf("negative base must be rejected: %+v", res)
	}
}

// CA-6：混单位 / 库存减流量 / 跨源 / 未知算子（脚本类）一律拒绝。
func TestSignalCalc_CompatibilityRejections(t *testing.T) {
	index := calcEIAIndex()

	// 混单位（MMbbl − Mb/d，同系列不同也不成立）。
	res := runCalc(t, index, "difference", "c1:o2", "c2:o2")
	if res.Status != SignalCalcStatusRejected || res.Value != "" {
		t.Fatalf("mixed units must be rejected: %+v", res)
	}
	// 库存减流量：同期间非批准对（stocks vs imports）。
	res = runCalc(t, index, "difference", "c1:o2", "c2:o2") // 期间不同且非同系列
	if res.Status != SignalCalcStatusRejected {
		t.Fatalf("stock-flow must be rejected: %+v", res)
	}
	index["c1:p9"] = calcObs("c1:p9", "eia_wpsr_table1", "eia:US SPR", "MMbbl", "2026-09-11", "stocks", f64(400))
	res = runCalc(t, index, "difference", "c1:o2", "c1:p9") // 同期间、非批准流量对
	if res.Status != SignalCalcStatusRejected || !strings.Contains(res.Reason, "批准") {
		t.Fatalf("same-period non-approved pair must be rejected: %+v", res)
	}
	// 跨源：JODI imports − EIA imports（同单位同期间也不行）。
	index["j:o1"] = calcObs("j:o1", "jodi_oil_primary", "jodi:US:imports:CRUDEOIL:KBD", "Mb/d", "2026-09", "imports", f64(6000))
	res = runCalc(t, index, "difference", "j:o1", "c2:o2")
	if res.Status != SignalCalcStatusRejected || !strings.Contains(res.Reason, "跨源") {
		t.Fatalf("cross-source must be rejected: %+v", res)
	}
	// 未知算子（含脚本类输入）。
	res = runCalc(t, index, "eval", "c1:o1")
	if res.Status != SignalCalcStatusRejected || !strings.Contains(res.Reason, "未知算子") {
		t.Fatalf("unknown op must be rejected: %+v", res)
	}
}

// CA-7：forward 引用 / 计算结果作输入 → 拒绝（首版仅原始观测输入）。
func TestSignalCalc_ForwardAndCalcRefsRejected(t *testing.T) {
	index := calcEIAIndex()
	// forward：引用不存在（未来的 k1 不在观测索引）。
	res := runCalc(t, index, "difference", "c1:o2", "k1")
	if res.Status != SignalCalcStatusRejected || !strings.Contains(res.Reason, "k1") {
		t.Fatalf("forward ref must be rejected: %+v", res)
	}
	// 计算结果作输入：k1 永远不在观测索引（解释器只吃原始观测）。
	res = runCalc(t, index, "percent_change", "k1", "c1:o1")
	if res.Status != SignalCalcStatusRejected {
		t.Fatalf("calc-as-input must be rejected: %+v", res)
	}
}

// 舍入与格式：half-up（half away from zero）4 位、去末尾 0、无科学计数、无 -0。
func TestSignalCalc_DecimalFormatting(t *testing.T) {
	cases := []struct {
		r    string // 精确有理数 n/d
		want string
	}{
		{"2", "2"},
		{"2.00004", "2"},
		{"2.00005", "2.0001"}, // half away from zero
		{"-0.00005", "-0.0001"},
		{"-66.66666666666667", "-66.6667"},
		{"1/3", "0.3333"},
		{"-1/3", "-0.3333"},
		{"0", "0"},
		{"-0", "0"},
		{"2800", "2800"},
	}
	for _, tc := range cases {
		r, ok := new(big.Rat).SetString(tc.r)
		if !ok {
			t.Fatalf("parse rational %q", tc.r)
		}
		if got := formatSignalDecimal(r); got != tc.want {
			t.Fatalf("format %s: want %q got %q", tc.r, tc.want, got)
		}
	}
}

// 同系列同期间差值（a−a=0）必须拒绝，不能给出伪装成信息的 0。
func TestSignalCalc_SameSeriesSamePeriodRejected(t *testing.T) {
	res := runCalc(t, calcEIAIndex(), "difference", "c1:o2", "c1:o2")
	if res.Status != SignalCalcStatusRejected {
		t.Fatalf("identical refs must be rejected: %+v", res)
	}
	// 相同期间相同系列不同 ref 也是同期间同系列。
	index := calcEIAIndex()
	index["c1:o2b"] = calcObs("c1:o2b", "eia_wpsr_table1", "eia:US commercial crude (excl SPR)", "MMbbl", "2026-09-11", "stocks", f64(419))
	res = runCalc(t, index, "difference", "c1:o2", "c1:o2b")
	if res.Status != SignalCalcStatusRejected {
		t.Fatalf("same series same period must be rejected: %+v", res)
	}
}

// ── WDI 计算路径（review M1：期间键是 year，与其它源的 period 不同）───────────

// wdiObs 构造 wb_wdi 原始观测行（键与 WDIObservation 对齐：期间键是 year，
// 无 period；无单位列；value 与 JSON 反序列化后同型——float64 或 nil）。
func wdiObs(indicator, iso3, year string, value any) map[string]any {
	return map[string]any{
		"country_iso3": iso3, "country_name": "China",
		"indicator_id": indicator, "year": year, "value": value,
	}
}

// wdiIndex 经 signalCalcObservationFromMap 把 WDI 原始观测行入索引（覆盖
// 期间键读取路径，而非手工 calcObs 绕过）：同指标同国的三年观测。
func wdiIndex() map[string]*SignalCalcObservation {
	rows := map[string]map[string]any{
		"c1:o1": wdiObs("NE.EXP.GNFS.ZS", "CHN", "2023", 42.1),
		"c1:o2": wdiObs("NE.EXP.GNFS.ZS", "CHN", "2024", 43.5),
		"c1:o3": wdiObs("NE.EXP.GNFS.ZS", "CHN", "2025", 41.9),
	}
	index := make(map[string]*SignalCalcObservation, len(rows))
	for ref, obs := range rows {
		index[ref] = signalCalcObservationFromMap(ref, strings.SplitN(ref, ":", 2)[0], "wb_wdi", obs)
	}
	return index
}

// M1①：WDI 同系列不同年 difference（修复前期间恒空串被「同一期间」误拒）。
func TestSignalCalc_WDIDifferenceAcrossYears(t *testing.T) {
	res := runCalc(t, wdiIndex(), "difference", "c1:o2", "c1:o1")
	if res.Status != SignalCalcStatusOK {
		t.Fatalf("status=%s reason=%s", res.Status, res.Reason)
	}
	if res.Value != "1.4" {
		t.Fatalf("want 1.4, got %s", res.Value)
	}
	if res.Unit != "" {
		t.Fatalf("WDI 原生无单位列，unit 必须为空: %q", res.Unit)
	}
}

// M1②：WDI 多年 mean（修复前第二个输入即触发「重复期间空串」误拒）。
func TestSignalCalc_WDIMeanAcrossYears(t *testing.T) {
	res := runCalc(t, wdiIndex(), "mean", "c1:o1", "c1:o2", "c1:o3")
	if res.Status != SignalCalcStatusOK {
		t.Fatalf("status=%s reason=%s", res.Status, res.Reason)
	}
	// (42.1+43.5+41.9)/3 = 42.5。
	if res.Value != "42.5" {
		t.Fatalf("want 42.5, got %s", res.Value)
	}
}

// M1③：WDI 观测混入 EIA 系列计算 → 跨源拒绝（series/源身份判定不受 WDI
// 期间键修正影响）。
func TestSignalCalc_WDIMixedWithEIARejected(t *testing.T) {
	wdi := signalCalcObservationFromMap("c9:o1", "c9", "wb_wdi", wdiObs("NE.EXP.GNFS.ZS", "CHN", "2024", 43.5))
	if wdi.Period != "2024" || wdi.SeriesID != "wdi|NE.EXP.GNFS.ZS|CHN" || wdi.Tool != "wb_wdi" {
		t.Fatalf("WDI index entry: %+v", wdi)
	}
	index := calcEIAIndex()
	index["c9:o1"] = wdi
	res := runCalc(t, index, "difference", "c9:o1", "c1:o2")
	if res.Status != SignalCalcStatusRejected || !strings.Contains(res.Reason, "跨源") {
		t.Fatalf("WDI+EIA difference must be rejected: %+v", res)
	}
	res = runCalc(t, index, "percent_change", "c9:o1", "c1:o2")
	if res.Status != SignalCalcStatusRejected || !strings.Contains(res.Reason, "跨源") {
		t.Fatalf("WDI+EIA percent_change must be rejected: %+v", res)
	}
	res = runCalc(t, index, "mean", "c9:o1", "c1:o2")
	if res.Status != SignalCalcStatusRejected || !strings.Contains(res.Reason, "跨源") {
		t.Fatalf("WDI+EIA mean must be rejected: %+v", res)
	}
}
