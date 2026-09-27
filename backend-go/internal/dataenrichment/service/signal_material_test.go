package service

import (
	"testing"
	"time"

	"syntopica-backend/internal/models"
)

// Signal period & material helpers (board-signal-reports PC-1..PC-5).
// Pure logic only — no DB, no SQLite: the DB-touching assembler is a thin
// composition over these tested helpers.

var pcNow = time.Date(2026, 9, 22, 10, 0, 0, 0, models.ShanghaiTZ)

// ── PC-1: period validation (legal passes; everything else 400, no job) ────

func TestParseSignalPeriodAcceptsLegal(t *testing.T) {
	p, err := ParseSignalPeriod("month", "2026-08", pcNow)
	if err != nil {
		t.Fatalf("legal month: %v", err)
	}
	if p.Granularity != "month" || p.Period != "2026-08" {
		t.Fatalf("parsed: %+v", p)
	}
	p, err = ParseSignalPeriod("year", "2026", pcNow)
	if err != nil {
		t.Fatalf("legal year: %v", err)
	}
	if p.Period != "2026" {
		t.Fatalf("parsed: %+v", p)
	}
	// The current, still-running month is legal (not a future period).
	if _, err := ParseSignalPeriod("month", "2026-09", pcNow); err != nil {
		t.Fatalf("current month must be legal: %v", err)
	}
}

func TestParseSignalPeriodRejectsIllegalBeforeAnyJob(t *testing.T) {
	cases := []struct{ granularity, period string }{
		{"week", "2026-W27"},                   // week is not a signal granularity
		{"all", "all"},                         // ditto
		{"", "2026-08"},                        // missing granularity
		{"monthx", "2026-08"},                  // unknown granularity
		{"month", "2026-13"},                   // month 13
		{"month", "2026-00"},                   // month 0
		{"month", "2026-8"},                    // no leading zero
		{"month", "2026-08-01"},                // day bleed-in
		{"month", "2026-08 extra"},             // trailing content
		{"month", ""},                          // empty
		{"month", "   "},                       // whitespace
		{"month", "2026-０８"},                   // full-width digits
		{"month", "2026-08\t"},                 // tab
		{"month", "2026-08-08-08-08-08-08-08"}, // overlong
		{"year", "26"},                         // not YYYY
		{"year", "2026-01"},                    // month bleed-in
		{"year", "1999"},                       // below the 2000..2100 bound
		{"year", "2101"},                       // above the bound
	}
	for _, c := range cases {
		if _, err := ParseSignalPeriod(c.granularity, c.period, pcNow); err == nil {
			t.Fatalf("ParseSignalPeriod(%q, %q) must fail", c.granularity, c.period)
		}
	}
}

// ── PC-2: business-timezone half-open boundaries + future rejection ─────────

func TestParseSignalPeriodHalfOpenBoundsInBusinessTZ(t *testing.T) {
	p, err := ParseSignalPeriod("month", "2026-08", pcNow)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	wantFrom := time.Date(2026, 8, 1, 0, 0, 0, 0, models.ShanghaiTZ)
	wantTo := time.Date(2026, 9, 1, 0, 0, 0, 0, models.ShanghaiTZ)
	if !p.From.Equal(wantFrom) || !p.To.Equal(wantTo) {
		t.Fatalf("bounds: from=%v to=%v, want %v..%v", p.From, p.To, wantFrom, wantTo)
	}
	// The Shanghai boundary is NOT the UTC month boundary: 2026-08-01T00:00+08
	// is 2026-07-31T16:00Z.
	if got := p.From.UTC().Format("2006-01-02T15:04Z07:00"); got != "2026-07-31T16:00Z" {
		t.Fatalf("from in UTC = %s, want 2026-07-31T16:00", got)
	}

	p, err = ParseSignalPeriod("year", "2026", pcNow)
	if err != nil {
		t.Fatalf("parse year: %v", err)
	}
	if !p.From.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, models.ShanghaiTZ)) ||
		!p.To.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, models.ShanghaiTZ)) {
		t.Fatalf("year bounds: %+v", p)
	}
}

func TestParseSignalPeriodRejectsFuture(t *testing.T) {
	for _, c := range []struct{ granularity, period string }{
		{"month", "2026-10"}, // next month
		{"month", "2027-01"}, // future year month
		{"year", "2027"},     // next year
	} {
		if _, err := ParseSignalPeriod(c.granularity, c.period, pcNow); err == nil {
			t.Fatalf("future period %s/%s must fail", c.granularity, c.period)
		}
	}
}

func TestSignalCutoffCurrentVsHistorical(t *testing.T) {
	// Current period: the cutoff is the discovery job start (now).
	p, _ := ParseSignalPeriod("month", "2026-09", pcNow)
	cutoff := SignalCutoff(p, pcNow)
	if !cutoff.Equal(pcNow.In(models.ShanghaiTZ)) {
		t.Fatalf("current-period cutoff must be the job start: %v", cutoff)
	}
	if mode := SignalAnalysisMode(p, cutoff); mode != "current" {
		t.Fatalf("current period mode = %s", mode)
	}

	// Ended historical period: the cutoff is the (exclusive) period end.
	hist, _ := ParseSignalPeriod("month", "2026-08", pcNow)
	cutoff = SignalCutoff(hist, pcNow)
	if !cutoff.Equal(hist.To) {
		t.Fatalf("historical cutoff must be the period end: %v", cutoff)
	}
	if mode := SignalAnalysisMode(hist, cutoff); mode != "retrospective" {
		t.Fatalf("historical period mode = %s", mode)
	}
}

// ── PC-3: historical material & post-period summary degradation ─────────────

func pcSection(id uint, date time.Time, label string, articles int) TimelineSectionNode {
	return TimelineSectionNode{SectionID: id, PeriodDate: date, ClusterLabel: label, ArticleCount: articles}
}

func TestSelectSignalPeriodSummaryDegradation(t *testing.T) {
	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, models.ShanghaiTZ)
	rows := []LifelineArchiveRow{
		{Granularity: "month", Period: "2026-08", AsOfDate: time.Date(2026, 8, 20, 0, 0, 0, 0, models.ShanghaiTZ), Content: "8月窗口内写入的摘要"},
		{Granularity: "month", Period: "2026-08", AsOfDate: time.Date(2026, 9, 10, 0, 0, 0, 0, models.ShanghaiTZ), Content: "9月才写入、可能混入后期事实的摘要"},
		{Granularity: "month", Period: "2026-07", AsOfDate: time.Date(2026, 7, 20, 0, 0, 0, 0, models.ShanghaiTZ), Content: "7月摘要（不是目标周期）"},
	}

	// Usable: target period + as-of within the cutoff.
	if got := selectSignalPeriodSummary(rows, "month", "2026-08", cutoff); got == nil || *got != "8月窗口内写入的摘要" {
		t.Fatalf("in-cutoff summary must be selected: %v", got)
	}
	// Degradation: a target-period summary whose as-of is AFTER the cutoff is
	// unusable — dated raw slices stand in (nil here; slices come separately).
	lateOnly := []LifelineArchiveRow{
		{Granularity: "month", Period: "2026-08", AsOfDate: time.Date(2026, 9, 10, 0, 0, 0, 0, models.ShanghaiTZ), Content: "9月才写入、可能混入后期事实的摘要"},
	}
	if got := selectSignalPeriodSummary(lateOnly, "month", "2026-08", cutoff); got != nil {
		t.Fatalf("summary written after cutoff must degrade to nil: %v", *got)
	}
	// A different period's summary is never passed off as the target's.
	if got := selectSignalPeriodSummary(rows, "month", "2026-09", cutoff); got != nil {
		t.Fatalf("non-target period summary must not match: %v", *got)
	}
	// Non-target granularity never matches.
	if got := selectSignalPeriodSummary(rows, "year", "2026", cutoff); got != nil {
		t.Fatalf("wrong granularity must not match: %v", *got)
	}
}

func TestSelectSignalBackgroundSummaryNeverPastCutoff(t *testing.T) {
	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, models.ShanghaiTZ)
	rows := []LifelineArchiveRow{
		{Granularity: "month", Period: "2026-05", AsOfDate: time.Date(2026, 5, 30, 0, 0, 0, 0, models.ShanghaiTZ), Content: "5月背景"},
		{Granularity: "month", Period: "2026-07", AsOfDate: time.Date(2026, 7, 30, 0, 0, 0, 0, models.ShanghaiTZ), Content: "7月背景"},
		{Granularity: "month", Period: "2026-08", AsOfDate: time.Date(2026, 9, 5, 0, 0, 0, 0, models.ShanghaiTZ), Content: "8月背景（as-of 晚于 cutoff）"},
		{Granularity: "month", Period: "2026-06", AsOfDate: time.Date(2026, 6, 30, 0, 0, 0, 0, models.ShanghaiTZ), Content: ""}, // empty content
	}
	bg := selectSignalBackgroundSummary(rows, "month", "2026-08", cutoff)
	if bg == nil || bg.Period != "2026-07" {
		t.Fatalf("newest strictly-earlier ≤ cutoff background expected, got %+v", bg)
	}
	// No earlier row at all → nil (无归档如实标注).
	if got := selectSignalBackgroundSummary(rows, "month", "2026-01", cutoff); got != nil {
		t.Fatalf("no strictly-earlier row must yield nil, got %+v", got)
	}
}

// L1：背景选择必须按 granularity 过滤——混粒度归档下，year 行不得充当
// month 周期背景（year\"2025\" 字符串序小于 \"2026-08\"，无粒度检查时会被误选）。
func TestSelectSignalBackgroundSummaryFiltersGranularity(t *testing.T) {
	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, models.ShanghaiTZ)
	rows := []LifelineArchiveRow{
		{Granularity: "year", Period: "2025", AsOfDate: time.Date(2026, 1, 1, 0, 0, 0, 0, models.ShanghaiTZ), Content: "2025 年度背景"},
		{Granularity: "month", Period: "2026-06", AsOfDate: time.Date(2026, 6, 30, 0, 0, 0, 0, models.ShanghaiTZ), Content: "6月背景"},
	}
	// month 周期只命中 month 归档，year 行即使 Period 字符串更小也不作背景。
	bg := selectSignalBackgroundSummary(rows, "month", "2026-08", cutoff)
	if bg == nil || bg.Granularity != "month" || bg.Period != "2026-06" {
		t.Fatalf("month background must ignore year rows: %+v", bg)
	}
	// year 周期同样只看 year 归档。
	bg = selectSignalBackgroundSummary(rows, "year", "2026", cutoff)
	if bg == nil || bg.Granularity != "year" || bg.Period != "2025" {
		t.Fatalf("year background must ignore month rows: %+v", bg)
	}
	// 同粒度无严格早于目标期的归档 → nil（不拿异粒度凑数）。
	if got := selectSignalBackgroundSummary(rows, "year", "2025", cutoff); got != nil {
		t.Fatalf("wrong-granularity row must not serve as background: %+v", got)
	}
}

// ── PC-5: cutoff freeze on slices (先于 agent 的边界过滤) ─────────────────────

func TestFilterSignalSectionsHalfOpenCutoffFreeze(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, models.ShanghaiTZ)
	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, models.ShanghaiTZ)
	sections := []TimelineSectionNode{
		pcSection(1, time.Date(2026, 7, 31, 15, 59, 0, 0, models.ShanghaiTZ), "窗口前", 3), // before from → out
		pcSection(2, time.Date(2026, 8, 1, 0, 0, 0, 0, models.ShanghaiTZ), "起点恰含", 5),   // = From → in (半开含起点)
		pcSection(3, time.Date(2026, 8, 15, 12, 0, 0, 0, models.ShanghaiTZ), "窗口中", 7),
		pcSection(4, time.Date(2026, 8, 31, 23, 59, 0, 0, models.ShanghaiTZ), "终点前一步", 2),     // in
		pcSection(5, time.Date(2026, 9, 1, 0, 0, 0, 0, models.ShanghaiTZ), "终点恰排", 9),         // = To → out (半开)
		pcSection(6, time.Date(2026, 9, 10, 8, 0, 0, 0, models.ShanghaiTZ), "cutoff 后新新闻", 4), // out (PC-5)
	}
	got := filterSignalSections(sections, from, cutoff)
	if len(got) != 3 {
		t.Fatalf("want 3 in-window slices, got %d: %+v", len(got), got)
	}
	if got[0].SectionID != 2 || got[1].SectionID != 3 || got[2].SectionID != 4 {
		t.Fatalf("slices must ascend by period: %+v", got)
	}
	// Empty window → empty, never padded.
	if empty := filterSignalSections(nil, from, cutoff); len(empty) != 0 {
		t.Fatalf("empty input must stay empty: %+v", empty)
	}
}

// ── PC-4: empty material & unknowable membership → honest gap ───────────────

func TestSignalMaterialEmptyAndMembershipGap(t *testing.T) {
	m := &SignalMaterial{Lanes: []SignalLaneMaterial{}, Gaps: []SignalMaterialGap{}}
	if !m.IsEmpty() {
		t.Fatal("no lanes = empty material")
	}
	m.Lanes = append(m.Lanes, SignalLaneMaterial{LaneID: 1})
	if m.IsEmpty() {
		t.Fatal("one lane = not empty")
	}
	before := len(m.Gaps)
	appendSignalMembershipGap(m)
	if len(m.Gaps) != before+1 || m.Gaps[len(m.Gaps)-1].Reason == "" {
		t.Fatalf("membership gap must be recorded honestly: %+v", m.Gaps)
	}
}

func TestFilterSignalSectionsIsStatusBlind(t *testing.T) {
	// PC-3: a lane that is non-active TODAY can still be the period's material
	// — the pure filter never sees status; slices are selected by date alone.
	// (The assembler queries lanes regardless of status; this test pins that
	// the date filter is the only gate.)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, models.ShanghaiTZ)
	cutoff := time.Date(2026, 2, 1, 0, 0, 0, 0, models.ShanghaiTZ)
	got := filterSignalSections([]TimelineSectionNode{pcSection(9, time.Date(2026, 1, 10, 0, 0, 0, 0, models.ShanghaiTZ), "inactive lane slice", 6)}, from, cutoff)
	if len(got) != 1 || got[0].ArticleCount != 6 {
		t.Fatalf("date-only filter must keep historical slices: %+v", got)
	}
}
