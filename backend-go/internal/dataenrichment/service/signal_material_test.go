package service

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tgrepo "syntopica-backend/internal/topicgraph/repository"
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

// ── add-topic-key-event-markers 2.3：信号发现材料纳入同 owner 合窗 confirmed ──

// stubKeyEventLifelineReader serves one lane's in-window sections via the
// LifelineReader interface (the assembler's only lane-data dependency).
type stubKeyEventLifelineReader struct {
	sectionsByTopic map[uint]SectionTimelineData
}

func (s *stubKeyEventLifelineReader) GetTopicLifeline(topicID uint) (SectionTimelineData, error) {
	if d, ok := s.sectionsByTopic[topicID]; ok {
		return d, nil
	}
	return SectionTimelineData{}, nil
}

func (s *stubKeyEventLifelineReader) GetTopicLifelineArchive(topicID uint) ([]LifelineArchiveRow, error) {
	return nil, nil
}

// seedSignalKeyEventWorld provisions board/lane/report/section/thread and
// returns ids. inWindow controls the section's period_date.
func seedSignalKeyEventWorld(t *testing.T, db *gorm.DB, inWindow bool, threadTitle string, tagIDs, artIDs string) (boardID, laneID, sectionID, threadID uint) {
	t.Helper()
	board := models.SemanticLabel{Label: "sig-ke-" + threadTitle, Slug: "sig-ke-" + threadTitle, LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	lane := topicgraphRepoBoard(t, db, board.ID)
	date := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	if !inWindow {
		date = time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	}
	report := topicgraphRepoReport(t, db, board.ID, date)
	section := topicgraphRepoSection(t, db, report, lane)
	thread := topicgraphRepoThread(t, db, report, section, threadTitle, tagIDs, artIDs)
	return board.ID, lane, section, thread
}

func topicgraphRepoBoard(t *testing.T, db *gorm.DB, boardID uint) uint {
	t.Helper()
	topic := tgrepo.BoardPersistentTopic{
		SemanticBoardID: boardID, Label: "信号泳道", Status: tgrepo.TopicStatusActive,
		FirstSeenDate: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		LastSeenDate:  time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Omit("embedding", "centroid").Create(&topic).Error)
	// Backdate created_at so the lane predates the historical cutoff (the
	// assembler skips lanes created after cutoff — correct behavior, wrong
	// for a fresh test row seeded "now").
	require.NoError(t, db.Exec(`UPDATE board_persistent_topics SET created_at = ? WHERE id = ?`,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), topic.ID).Error)
	return topic.ID
}

func topicgraphRepoReport(t *testing.T, db *gorm.DB, boardID uint, date time.Time) uint {
	t.Helper()
	report := tgrepo.BoardDailyReport{
		SemanticBoardID: boardID, PeriodDate: tgrepo.NormalizeReportDate(date),
		Status: "completed", Title: "t",
	}
	require.NoError(t, db.Create(&report).Error)
	return report.ID
}

func topicgraphRepoSection(t *testing.T, db *gorm.DB, reportID, laneID uint) uint {
	t.Helper()
	section := tgrepo.DailyReportSection{
		ReportID: reportID, ClusterLabel: "信号章节", ArticleCount: 1,
		Embedding: tgrepo.FloatsToPgVector([]float64{0}), PersistentTopicID: &laneID,
	}
	require.NoError(t, db.Create(&section).Error)
	return section.ID
}

func topicgraphRepoThread(t *testing.T, db *gorm.DB, reportID, sectionID uint, title, tags, arts string) uint {
	t.Helper()
	thread := tgrepo.DailyReportThread{
		ReportID: reportID, SectionID: sectionID, Title: title, Summary: "s-" + title,
		TagIDs: tgrepo.JSON(tags), RelatedArticleIDs: tgrepo.JSON(arts),
		Embedding: tgrepo.FloatsToPgVector([]float64{0}),
	}
	require.NoError(t, db.Create(&thread).Error)
	return thread.ID
}

// kePeriod is a fixed month window: 2026-08 in Shanghai TZ.
func kePeriod(t *testing.T) SignalPeriod {
	t.Helper()
	p, err := ParseSignalPeriod("month", "2026-08", time.Date(2026, 9, 1, 0, 0, 0, 0, models.ShanghaiTZ))
	require.NoError(t, err)
	return p
}

// TestSignalMaterialConfirmedKeyEvents_OnlyInWindowSameOwnerConfirmed covers
// spec scenarios 只冻结合窗人工关注 / 事后选择不冒充当时预测 / 历史材料隔离.
func TestSignalMaterialConfirmedKeyEvents_OnlyInWindowSameOwnerConfirmed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	boardID, laneID, sectionID, threadID := seedSignalKeyEventWorld(t, db, true, "在窗事件", "[31]", "[51,52]")
	otherBoard, otherLane, _, otherThread := seedSignalKeyEventWorld(t, db, true, "他板事件", "[31]", "[51]")

	cutoff := time.Date(2026, 8, 20, 0, 0, 0, 0, models.ShanghaiTZ)

	// Confirmed, confirmed BEFORE cutoff (当时已知).
	m1, err := tgrepoApplyThread(t, db, laneID, threadID, cutoff.AddDate(0, 0, -2))
	require.NoError(t, err)
	_ = m1
	// Confirmed AFTER cutoff → retrospective attention (事后选择).
	m2, err := tgrepoApplyThread(t, db, otherLane, otherThread, cutoff.AddDate(0, 0, +5))
	require.NoError(t, err)
	_ = m2
	// Suggested (unconfirmed) on a second thread → excluded from attention.
	_, thread2 := seedSignalKeyEventThread2(t, db, boardID, laneID)
	require.NoError(t, db.Exec(`INSERT INTO topic_key_event_markers
		(semantic_board_id, source_key, persistent_topic_id, source_thread_id, source_section_id, source_period_date,
		 title_snapshot, summary_snapshot, state, last_action, revision, created_at, updated_at)
		VALUES (?, 'k-sugg', ?, ?, ?, DATE '2026-08-12', '未确认建议', 's', 'suggested', 'ai_suggest', 1, now(), now())`,
		boardID, laneID, thread2, sectionID).Error)
	// Out-of-window confirmed (July) on the same lane.
	_, outSection := seedSignalJulySection(t, db, boardID, laneID)

	reader := &stubKeyEventLifelineReader{sectionsByTopic: map[uint]SectionTimelineData{
		laneID: {Sections: []TimelineSectionNode{{
			SectionID: sectionID, PeriodDate: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
			ClusterLabel: "信号章节", ArticleCount: 1, ThreadCount: 1,
			ThreadTitles: []string{"在窗事件"},
		}, {
			SectionID: outSection, PeriodDate: time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
			ClusterLabel: "七月旧闻", ArticleCount: 1,
		}}},
	}}
	builder := NewSignalMaterialBuilder(db, reader)
	p := kePeriod(t)
	material, err := builder.AssembleSignalMaterial(t.Context(), boardID, p, cutoff)
	require.NoError(t, err)

	// Lane survives with its slice.
	var lane *SignalLaneMaterial
	for i := range material.Lanes {
		if material.Lanes[i].LaneID == laneID {
			lane = &material.Lanes[i]
		}
	}
	require.NotNil(t, lane, "lane material must exist")
	// Only the in-window confirmed marker enters the attention list.
	require.Len(t, lane.ConfirmedKeyEvents, 1, "只冻结合窗同owner confirmed：他板/未确认/窗外一律剔除")
	att := lane.ConfirmedKeyEvents[0]
	assert.Equal(t, "在窗事件", att.Title)
	assert.True(t, att.EvidenceVerified, "source section 在白名单 → 证据可核")
	assert.False(t, att.CurrentRetrospectiveAttention, "cutoff 前确认 → 当时已知，非回顾标记")
	_, _ = otherBoard, otherLane

	// Retrospective flag: re-confirm after cutoff on a fresh source.
	boardID2, laneID2, sectionID2, threadID2 := seedSignalKeyEventWorld(t, db, true, "回顾事件", "[31]", "[53]")
	_, err = tgrepoApplyThread(t, db, laneID2, threadID2, cutoff.AddDate(0, 0, 3))
	require.NoError(t, err)
	m2b, _ := builder.collectConfirmedKeyEvents(t.Context(), boardID2, laneID2, "x", []SignalArticleSlice{{
		SectionID: sectionID2, PeriodDate: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
	}}, p, cutoff)
	require.NoError(t, err)
	require.Len(t, m2b, 1)
	assert.True(t, m2b[0].CurrentRetrospectiveAttention, "cutoff 后确认 → 本次回顾关注，不冒充当时预测")
}

// tgrepoApplyThread applies a manual mark via the real repository (needs the
// topicgraph repo singleton bound to the same DB).
func tgrepoApplyThread(t *testing.T, db *gorm.DB, topicID, threadID uint, decidedAt time.Time) (*tgrepo.TopicKeyEventMarker, error) {
	t.Helper()
	tgrepo.InitRepository(db)
	m, err := tgrepo.Repo.ApplyThreadKeyEvent(topicID, threadID, tgrepo.KeyEventActionMark, nil)
	if err != nil {
		return nil, err
	}
	// Pin decision timestamps deterministically for cutoff assertions.
	require.NoError(t, db.Exec(`UPDATE topic_key_event_markers SET confirmed_at = ?, decided_at = ? WHERE id = ?`,
		decidedAt, decidedAt, m.ID).Error)
	return m, nil
}

func seedSignalKeyEventThread2(t *testing.T, db *gorm.DB, boardID, laneID uint) (sectionID, threadID uint) {
	t.Helper()
	report := topicgraphRepoReport(t, db, boardID, time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC))
	section := topicgraphRepoSection(t, db, report, laneID)
	thread := topicgraphRepoThread(t, db, report, section, "第二事件", "[31]", "[54]")
	return section, thread
}

func seedSignalJulySection(t *testing.T, db *gorm.DB, boardID, laneID uint) (threadID, sectionID uint) {
	t.Helper()
	report := topicgraphRepoReport(t, db, boardID, time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC))
	section := topicgraphRepoSection(t, db, report, laneID)
	thread := topicgraphRepoThread(t, db, report, section, "七月确认事件", "[31]", "[55]")
	m, err := tgrepoApplyThread(t, db, laneID, thread, time.Date(2026, 7, 6, 0, 0, 0, 0, models.ShanghaiTZ))
	require.NoError(t, err)
	_ = m
	return thread, section
}

// TestSignalMaterialConfirmedKeyEvents_InvalidEvidenceRecordsGap covers
// spec scenario 失效依据不升级为有效信号：detached snapshot keeps attention
// but never verifies as evidence; exact-fingerprint threads DO verify.
func TestSignalMaterialConfirmedKeyEvents_InvalidEvidenceRecordsGap(t *testing.T) {
	db := testutil.SetupTestDB(t)
	boardID, laneID, sectionID, threadID := seedSignalKeyEventWorld(t, db, true, "会失效的事件", "[31]", "[56,57]")
	cutoff := time.Date(2026, 8, 20, 0, 0, 0, 0, models.ShanghaiTZ)
	p := kePeriod(t)

	_, err := tgrepoApplyThread(t, db, laneID, threadID, cutoff.AddDate(0, 0, -1))
	require.NoError(t, err)

	articles := []SignalArticleSlice{{
		SectionID: sectionID, PeriodDate: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
	}}
	builder := NewSignalMaterialBuilder(db, &stubKeyEventLifelineReader{})

	// ① Live source: verified, no gap.
	att, gaps := builder.collectConfirmedKeyEvents(t.Context(), boardID, laneID, "l", articles, p, cutoff)
	require.Len(t, att, 1)
	assert.True(t, att[0].EvidenceVerified)
	assert.Empty(t, gaps)

	// ② Detached source (rebuild replaced the thread) + NO fingerprint match
	// in whitelist → gap recorded, evidence NOT verified (快照不冒充有效原文).
	// A real rebuild DELETES the thread row; the FK SET NULLs the marker.
	require.NoError(t, db.Exec(`DELETE FROM daily_report_threads WHERE id = ?`, threadID).Error)
	att, gaps = builder.collectConfirmedKeyEvents(t.Context(), boardID, laneID, "l", articles, p, cutoff)
	require.Len(t, att, 1)
	assert.False(t, att[0].EvidenceVerified, "失效依据不升级为有效信号")
	require.Len(t, gaps, 1)
	assert.Contains(t, gaps[0].Reason, "无有效新闻切片")

	// ③ Exact fingerprint match on a whitelisted thread re-verifies — the
	// rebuilt thread carries identical title/tags/articles/date (reuse the
	// existing report row: same board+date has a unique slot).
	var reportID uint
	require.NoError(t, db.Raw(`SELECT id FROM board_daily_reports WHERE semantic_board_id = ? ORDER BY id LIMIT 1`, boardID).Scan(&reportID).Error)
	newThread := topicgraphRepoThread(t, db, reportID, sectionID, "会失效的事件", "[31]", "[57,56]")
	_ = newThread
	att, gaps = builder.collectConfirmedKeyEvents(t.Context(), boardID, laneID, "l", articles, p, cutoff)
	require.Len(t, att, 1)
	assert.True(t, att[0].EvidenceVerified, "指纹严格一致的存续来源可重新映射")
	assert.Empty(t, gaps)
}

// TestSignalMaterialLegacyPayloadCompat: lanes without key events serialize
// WITHOUT the field (omitempty) so old research snapshots stay readable.
func TestSignalMaterialLegacyPayloadCompat(t *testing.T) {
	raw, err := json.Marshal(&SignalMaterial{Lanes: []SignalLaneMaterial{{LaneID: 1, Articles: []SignalArticleSlice{}}}})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("confirmed_key_events")) {
		t.Fatalf("legacy payload must omit the attention field: %s", raw)
	}
	// And the field round-trips when present.
	with := &SignalMaterial{Lanes: []SignalLaneMaterial{{LaneID: 1, ConfirmedKeyEvents: []SignalKeyEventAttention{{MarkerID: 9, Title: "x", EvidenceVerified: true}}}}}
	raw2, _ := json.Marshal(with)
	var back SignalMaterial
	if err := json.Unmarshal(raw2, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Lanes) != 1 || back.Lanes[0].ConfirmedKeyEvents[0].MarkerID != 9 || !back.Lanes[0].ConfirmedKeyEvents[0].EvidenceVerified {
		t.Fatalf("attention field must round-trip: %+v", back)
	}
}
