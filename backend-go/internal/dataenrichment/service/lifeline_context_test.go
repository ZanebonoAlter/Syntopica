package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/dataenrichment"
	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ── Test helpers ────────────────────────────────────────────────────────────

func setupLifelineTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database.DB = db
	if err := database.RunAutoMigrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	repository.InitRepo(db)
}

// mockAirouter records Chat calls and returns canned responses.
type mockAirouter struct {
	calls   []airouter.ChatRequest
	results map[string]string // keyed by operation+first message snippet, returns content
}

func newMockAirouter() *mockAirouter {
	return &mockAirouter{
		results: make(map[string]string),
	}
}

func (m *mockAirouter) add(operation string, msgPrefix string, content string) {
	m.results[operation+"|"+msgPrefix] = content
}

func (m *mockAirouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	m.calls = append(m.calls, req)

	// Try exact match first
	firstMsg := ""
	if len(req.Messages) > 0 {
		firstMsg = req.Messages[0].Content
	}
	key := req.Operation + "|" + firstMsg

	// Fall back to prefix match
	for k, v := range m.results {
		if strings.HasPrefix(key, k) || strings.HasPrefix(k, key) {
			return &airouter.ChatResult{Content: v}, nil
		}
	}

	// Default: return the operation as content so tests can verify.
	return &airouter.ChatResult{Content: fmt.Sprintf("summarized: %s", req.Operation)}, nil
}

// mockSectionReader returns canned section text for topic+range queries.
type mockSectionReader struct {
	byRange      map[string]string    // key: "topicID|from|to"
	datesByTopic map[uint][]time.Time // topicID → distinct section dates
}

func newMockSectionReader() *mockSectionReader {
	return &mockSectionReader{
		byRange:      make(map[string]string),
		datesByTopic: make(map[uint][]time.Time),
	}
}

func (m *mockSectionReader) add(topicID uint, from, to time.Time, text string) {
	m.byRange[fmt.Sprintf("%d|%s|%s", topicID, from.Format("2006-01-02"), to.Format("2006-01-02"))] = text
}

func (m *mockSectionReader) SectionDates(ctx context.Context, topicID uint) ([]time.Time, error) {
	if dates, ok := m.datesByTopic[topicID]; ok {
		return dates, nil
	}
	return nil, nil
}

func (m *mockSectionReader) ReadSections(ctx context.Context, topicID uint, from, to time.Time) (string, error) {
	key := fmt.Sprintf("%d|%s|%s", topicID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if text, ok := m.byRange[key]; ok {
		return text, nil
	}
	// Default: return a placeholder
	return fmt.Sprintf("sections for topic %d from %s to %s", topicID, from.Format("2006-01-02"), to.Format("2006-01-02")), nil
}

func newTestService(airouter service.AirRouter, sectionReader service.SectionReader) *service.LifelineContextService {
	return service.NewLifelineContextService(
		airouter,
		repository.Repo,
		sectionReader,
		dataenrichment.CapabilityNews,
	)
}

// mockTopicLister returns a fixed list of active topic IDs.
type mockTopicLister struct {
	ids []uint
}

func (m *mockTopicLister) ListActiveTopicIDs(ctx context.Context) ([]uint, error) {
	return m.ids, nil
}

// fixedMonday returns a time that is a known Monday.
// 2026-07-06 is a Monday (verified: July 2026 calendar).
func fixedMonday() time.Time {
	return time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
}

// ── Tests: RefreshWeek ──────────────────────────────────────────────────────

func TestRefreshWeek_ReadsCurrentWeekSections(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	now := fixedMonday() // Monday Jul 6, 2026
	// Current week: Mon Jul 6 → Mon Jul 13
	topicID := uint(1)

	// Populate a section in this week's range
	sr.add(topicID,
		time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		"周一 芯片供不应求：thread1; thread2\n周二 产能扩张：thread3")

	err := svc.RefreshWeek(context.Background(), topicID, now)
	if err != nil {
		t.Fatalf("RefreshWeek: %v", err)
	}

	// Verify: as_of clamped to now (Jul 6) — the write happened mid-period;
	// a future-dated as_of (Jul 13 boundary) would blind the freshness gate
	// (fix-board-analysis-material 7.2).
	ctx, err := repository.Repo.GetTopicLifelineContext(context.Background(), topicID, string(repository.GranularityWeek), service.FormatWeek(now))
	if err != nil {
		t.Fatalf("get context: %v", err)
	}
	if ctx.AsOfDate.Format("2006-01-02") != "2026-07-06" {
		t.Fatalf("expected clamped as_of_date 2026-07-06, got %s", ctx.AsOfDate.Format("2006-01-02"))
	}
	if ctx.Content == "" {
		t.Fatal("expected non-empty content")
	}
	if ctx.Source != "llm_assisted" {
		t.Fatalf("expected source llm_assisted, got %s", ctx.Source)
	}
}

func TestRefreshWeek_LLMCallParams(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	now := fixedMonday()
	topicID := uint(2)

	err := svc.RefreshWeek(context.Background(), topicID, now)
	if err != nil {
		t.Fatalf("RefreshWeek: %v", err)
	}

	if len(ar.calls) == 0 {
		t.Fatal("expected at least one Chat call")
	}

	req := ar.calls[0]
	// Verify Capability
	if req.Capability != dataenrichment.CapabilityNews {
		t.Fatalf("expected capability %s, got %s", dataenrichment.CapabilityNews, req.Capability)
	}
	// Verify Operation
	if req.Operation != "data_enrichment.summarize_context" {
		t.Fatalf("expected operation summarize_context, got %s", req.Operation)
	}
	// Verify SessionID format
	if !strings.HasPrefix(req.SessionID, "lifeline_context_") {
		t.Fatalf("expected session_id prefix lifeline_context_, got %s", req.SessionID)
	}
	if !strings.Contains(req.SessionID, "_week_") {
		t.Fatalf("expected session_id to contain _week_, got %s", req.SessionID)
	}
	// Verify temperature ~0.3
	if req.Temperature == nil || *req.Temperature < 0.2 || *req.Temperature > 0.4 {
		t.Fatalf("expected temperature ~0.3, got %v", req.Temperature)
	}
}

func TestRefreshWeek_NoOldContext(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	now := fixedMonday()
	topicID := uint(3)

	// No old week context exists — should still work (direct summary)
	err := svc.RefreshWeek(context.Background(), topicID, now)
	if err != nil {
		t.Fatalf("RefreshWeek: %v", err)
	}

	// LLM should receive only sections (no old context in prompt)
	if len(ar.calls) == 0 {
		t.Fatal("expected Chat call")
	}
	prompt := ar.calls[0].Messages[0].Content
	if strings.Contains(prompt, "已有汇总") || strings.Contains(prompt, "旧汇总") {
		t.Fatal("week prompt should not contain old context reference")
	}
	if strings.Contains(prompt, "本周") {
		t.Fatal("week prompt should NOT contain hardcoded 本周; should use period-specific label")
	}
	if !strings.Contains(prompt, "2026年第28周") {
		t.Errorf("week prompt should contain period-specific label 2026年第28周, got: %s", prompt)
	}
}

// ── Tests: RefreshMonth (standalone, not incremental) ──────────────────────

func TestRefreshMonth_StandaloneSummary(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	// Pre-populate old month context (June) — should NOT be merged into July summary.
	oldCtx := &repository.TopicLifelineContext{
		PersistentTopicID: 4,
		Granularity:       string(repository.GranularityMonth),
		Period:            "2026-06",
		Content:           "6月摘要：芯片板块整理为主，存储芯片价格回升",
		AsOfDate:          time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Source:            "llm_assisted",
	}
	if err := repository.Repo.UpsertTopicLifelineContext(context.Background(), oldCtx); err != nil {
		t.Fatalf("insert old context: %v", err)
	}

	// Mock sections for July only (standalone period).
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	topicID := uint(4)
	sr.add(topicID,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		"7月初突发，AI芯片出口管制")

	err := svc.RefreshMonth(context.Background(), topicID, now)
	if err != nil {
		t.Fatalf("RefreshMonth: %v", err)
	}

	// Verify: standalone summary (no old context merge).
	if len(ar.calls) == 0 {
		t.Fatal("expected Chat call")
	}
	prompt := ar.calls[0].Messages[0].Content
	if strings.Contains(prompt, "6月摘要") || strings.Contains(prompt, "已有汇总") {
		t.Fatal("month prompt should NOT contain old summary (standalone period)")
	}
	if strings.Contains(prompt, "本月") {
		t.Fatal("month prompt should NOT contain hardcoded 本月; should use period-specific label")
	}
	if !strings.Contains(prompt, "2026年7月") {
		t.Errorf("month prompt should contain period-specific label 2026年7月, got: %s", prompt)
	}

	// Verify: new row with period 2026-07 created (June row still exists).
	ctx, err := repository.Repo.GetTopicLifelineContext(context.Background(), topicID, string(repository.GranularityMonth), "2026-07")
	if err != nil {
		t.Fatalf("get new month context: %v", err)
	}
	if ctx.Period != "2026-07" {
		t.Fatalf("expected period 2026-07, got %s", ctx.Period)
	}

	// Verify: old June row still exists (not overwritten).
	old, err := repository.Repo.GetTopicLifelineContext(context.Background(), topicID, string(repository.GranularityMonth), "2026-06")
	if err != nil {
		t.Fatalf("old June context should still exist: %v", err)
	}
	if old.Period != "2026-06" {
		t.Fatalf("expected old period 2026-06, got %s", old.Period)
	}
}

// ── Tests: HealMissing (self-healing) ───────────────────────────────────────

func TestHealMissing_SkipOneWeekThenCatchUp(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	now := fixedMonday() // Monday Jul 6, 2026
	topicID := uint(5)

	// Mock SectionDates: data exists for W26, W27, W28 (oldWeek=W26, now=W28).
	var dates []time.Time
	for _, wp := range []string{"2026-W26", "2026-W27", "2026-W28"} {
		from, _, _ := service.ParsePeriodRange(wp, "week")
		dates = append(dates, from.AddDate(0, 0, 3))
	}
	sr.datesByTopic[topicID] = dates

	// Seed an old week context (2 weeks ago — "2026-W25", which is June 15-21 2026)
	// Actually let's use fixedMonday minus 14 days which gives a period that's 2 weeks before W27
	oldWeek := now.AddDate(0, 0, -14)
	oldPeriod := service.FormatWeek(oldWeek)
	oldCtx := &repository.TopicLifelineContext{
		PersistentTopicID: topicID,
		Granularity:       string(repository.GranularityWeek),
		Period:            oldPeriod,
		Content:           "两周前的内容",
		AsOfDate:          oldWeek,
		Source:            "llm_assisted",
	}
	if err := repository.Repo.UpsertTopicLifelineContext(context.Background(), oldCtx); err != nil {
		t.Fatalf("insert old: %v", err)
	}

	// Mock sections for the missing period and current period.
	// HealMissing will compute: latest=oldPeriod, current=FormatWeek(now).
	// Missing: all periods between oldPeriod+1 and current (inclusive).
	currentPeriod := service.FormatWeek(now)

	// Mock sections for each missing period.
	missing := service.PeriodsBetween(oldPeriod, currentPeriod, "week")
	for _, p := range missing {
		from, to, _ := service.ParsePeriodRange(p, "week")
		sr.add(topicID, from, to, fmt.Sprintf("period %s news", p))
	}

	lister := &mockTopicLister{ids: []uint{topicID}}
	err := svc.HealMissing(context.Background(), string(repository.GranularityWeek), now, lister)
	if err != nil {
		t.Fatalf("HealMissing: %v", err)
	}

	// Should have made at least 1 LLM call (if there are missing periods).
	if len(ar.calls) < 1 {
		t.Fatalf("expected at least 1 Chat call, got %d", len(ar.calls))
	}

	// Verify: latest period row exists with current period.
	ctx, err := repository.Repo.GetTopicLifelineContext(context.Background(), topicID, string(repository.GranularityWeek), currentPeriod)
	if err != nil {
		t.Fatalf("get current week context: %v", err)
	}
	if ctx.Period != currentPeriod {
		t.Fatalf("expected period %s, got %s", currentPeriod, ctx.Period)
	}
}

func TestHealMissing_NoStaleContexts(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	now := fixedMonday()
	currentPeriod := service.FormatWeek(now)
	topicID := uint(6)

	// Mock SectionDates: only dates within current period.
	sr.datesByTopic[topicID] = []time.Time{now}

	// Seed current week context.
	freshCtx := &repository.TopicLifelineContext{
		PersistentTopicID: topicID,
		Granularity:       string(repository.GranularityWeek),
		Period:            currentPeriod,
		Content:           "本周最新",
		AsOfDate:          now,
		Source:            "llm_assisted",
	}
	repository.Repo.UpsertTopicLifelineContext(context.Background(), freshCtx)

	lister := &mockTopicLister{ids: []uint{topicID}}
	err := svc.HealMissing(context.Background(), string(repository.GranularityWeek), now, lister)
	if err != nil {
		t.Fatalf("HealMissing: %v", err)
	}

	// Should be no-op — context is already at current period.
	if len(ar.calls) > 0 {
		t.Fatalf("expected 0 Chat calls for current context, got %d", len(ar.calls))
	}
}

func TestHealMissing_AsOfDateAdvancesSequentially(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	topicID := uint(7)

	// Mock SectionDates: data exists for months 4-7.
	sr.datesByTopic[topicID] = []time.Time{
		time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
	}

	// Seed old month context (April 2026).
	oldPeriod := "2026-04"
	oldCtx := &repository.TopicLifelineContext{
		PersistentTopicID: topicID,
		Granularity:       string(repository.GranularityMonth),
		Period:            oldPeriod,
		Content:           "旧摘要",
		AsOfDate:          time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Source:            "llm_assisted",
	}
	repository.Repo.UpsertTopicLifelineContext(context.Background(), oldCtx)

	// Mock sections for each missing month: 2026-05, 2026-06, 2026-07
	for _, p := range []string{"2026-05", "2026-06", "2026-07"} {
		from, to, _ := service.ParsePeriodRange(p, "month")
		sr.add(topicID, from, to, fmt.Sprintf("month %s news", p))
	}

	lister := &mockTopicLister{ids: []uint{topicID}}
	err := svc.HealMissing(context.Background(), string(repository.GranularityMonth), now, lister)
	if err != nil {
		t.Fatalf("HealMissing: %v", err)
	}

	// Should have made 3 LLM calls (May, June, July).
	if len(ar.calls) != 3 {
		t.Fatalf("expected 3 Chat calls for three months, got %d", len(ar.calls))
	}

	// Verify: current month period (2026-07) exists.
	ctx, err := repository.Repo.GetTopicLifelineContext(context.Background(), topicID, string(repository.GranularityMonth), "2026-07")
	if err != nil {
		t.Fatalf("get current month context: %v", err)
	}
	if ctx.Period != "2026-07" {
		t.Fatalf("expected period 2026-07, got %s", ctx.Period)
	}
}

// ── Tests: RefreshGranularity dispatch ─────────────────────────────────────

func TestRefreshGranularity_DispatchesCorrectly(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	now := fixedMonday()
	topicID := uint(8)

	tests := []struct {
		granularity string
		expectOp    string
	}{
		{string(repository.GranularityWeek), "data_enrichment.summarize_context"},
		{string(repository.GranularityMonth), "data_enrichment.summarize_context"},
		{string(repository.GranularityYear), "data_enrichment.summarize_context"},
		{string(repository.GranularityAll), "data_enrichment.summarize_context"},
	}

	for _, tt := range tests {
		t.Run(tt.granularity, func(t *testing.T) {
			ar.calls = nil // reset
			err := svc.RefreshGranularity(context.Background(), topicID, tt.granularity, now)
			if err != nil {
				t.Fatalf("RefreshGranularity(%s): %v", tt.granularity, err)
			}
			if len(ar.calls) == 0 {
				t.Fatalf("expected Chat call for %s", tt.granularity)
			}
			if ar.calls[0].Operation != tt.expectOp {
				t.Fatalf("expected %s, got %s", tt.expectOp, ar.calls[0].Operation)
			}
		})
	}
}

func TestRefreshGranularity_InvalidGranularity(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	err := svc.RefreshGranularity(context.Background(), 1, "daily", fixedMonday())
	if err == nil {
		t.Fatal("expected error for invalid granularity")
	}
	if !strings.Contains(err.Error(), "unknown granularity") {
		t.Fatalf("expected 'unknown granularity' error, got: %v", err)
	}
}

// ── Tests: RefreshAll ──────────────────────────────────────────────────────

func TestRefreshAll_IncrementalMerge(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	// Pre-populate old 'all' context
	oldCtx := &repository.TopicLifelineContext{
		PersistentTopicID: 9,
		Granularity:       string(repository.GranularityAll),
		Period:            "all",
		Content:           "历史全貌：芯片板块5年综述",
		AsOfDate:          time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Source:            "llm_assisted",
	}
	repository.Repo.UpsertTopicLifelineContext(context.Background(), oldCtx)

	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	topicID := uint(9)
	sr.add(topicID,
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		now,
		"增量：7月AI芯片管制新动态")

	err := svc.RefreshAll(context.Background(), topicID, now)
	if err != nil {
		t.Fatalf("RefreshAll: %v", err)
	}

	prompt := ar.calls[0].Messages[0].Content
	if !strings.Contains(prompt, "历史全貌") {
		t.Fatal("all prompt should contain old summary")
	}
	if !strings.Contains(prompt, "增量") {
		t.Fatal("all prompt should contain incremental sections")
	}
}

// ── Tests: HealMissing backward fill (historical backfill) ─────────────────

func TestHealMissing_BackfillsHistoricalPeriods(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	now := fixedMonday() // Monday Jul 6, 2026 = W28
	topicID := uint(10)

	// No seeded context at all — simulating a topic that has W23-W28 data
	// but never had lifeline_context rows generated.
	var dates []time.Time
	for w := 23; w <= 28; w++ {
		from, _, _ := service.ParsePeriodRange(fmt.Sprintf("2026-W%02d", w), "week")
		dates = append(dates, from.AddDate(0, 0, 3))
	}
	sr.datesByTopic[topicID] = dates

	lister := &mockTopicLister{ids: []uint{topicID}}
	err := svc.HealMissing(context.Background(), string(repository.GranularityWeek), now, lister)
	if err != nil {
		t.Fatalf("HealMissing: %v", err)
	}

	// Should have 6 LLM calls (W23-W28), all ≤ current W28.
	if len(ar.calls) != 6 {
		t.Fatalf("expected 6 Chat calls for W23-W28 backfill, got %d", len(ar.calls))
	}

	// Verify all 6 periods exist in DB.
	for w := 23; w <= 28; w++ {
		period := fmt.Sprintf("2026-W%02d", w)
		ctx, err := repository.Repo.GetTopicLifelineContext(context.Background(), topicID, string(repository.GranularityWeek), period)
		if err != nil {
			t.Fatalf("get context for %s: %v", period, err)
		}
		if ctx.Period != period {
			t.Fatalf("expected period %s, got %s", period, ctx.Period)
		}
	}
}

func TestHealMissing_BackfillsIncludesOldThreshold(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	now := fixedMonday() // W28
	topicID := uint(11)

	// Seed only W28 (current) context.
	currentPeriod := service.FormatWeek(now)
	freshCtx := &repository.TopicLifelineContext{
		PersistentTopicID: topicID,
		Granularity:       string(repository.GranularityWeek),
		Period:            currentPeriod,
		Content:           "current",
		AsOfDate:          now,
		Source:            "llm_assisted",
	}
	repository.Repo.UpsertTopicLifelineContext(context.Background(), freshCtx)

	// SectionDates: data exists for W24-W28 (older weeks were never summarized).
	var dates []time.Time
	for w := 24; w <= 28; w++ {
		from, _, _ := service.ParsePeriodRange(fmt.Sprintf("2026-W%02d", w), "week")
		dates = append(dates, from.AddDate(0, 0, 3))
	}
	sr.datesByTopic[topicID] = dates

	lister := &mockTopicLister{ids: []uint{topicID}}
	err := svc.HealMissing(context.Background(), string(repository.GranularityWeek), now, lister)
	if err != nil {
		t.Fatalf("HealMissing: %v", err)
	}

	// Should backfill W24-W27 (4 periods), skip W28 (already exists).
	if len(ar.calls) != 4 {
		t.Fatalf("expected 4 Chat calls for W24-W27 backfill, got %d", len(ar.calls))
	}

	// Verify all 5 periods (W24-W28) exist.
	for w := 24; w <= 28; w++ {
		period := fmt.Sprintf("2026-W%02d", w)
		ctx, err := repository.Repo.GetTopicLifelineContext(context.Background(), topicID, string(repository.GranularityWeek), period)
		if err != nil {
			t.Fatalf("get context for %s: %v", period, err)
		}
		if ctx.Period != period {
			t.Fatalf("expected period %s, got %s", period, ctx.Period)
		}
	}
}

// ── Tests: summarizeArchive period-aware prompt ────────────────────────────

func TestSummarizeArchive_PeriodAwarePrompt(t *testing.T) {
	setupLifelineTestDB(t)
	ar := newMockAirouter()
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	topicID := uint(12)
	// Refresh a specific historical period: 2026-W25 (June 15-21).
	err := svc.RefreshPeriod(context.Background(), topicID, string(repository.GranularityWeek), "2026-W25", fixedMonday())
	if err != nil {
		t.Fatalf("RefreshPeriod: %v", err)
	}

	if len(ar.calls) == 0 {
		t.Fatal("expected Chat call")
	}
	prompt := ar.calls[0].Messages[0].Content

	// Must contain the real period label (not hardcoded "本周").
	if !strings.Contains(prompt, "2026年第25周") {
		t.Errorf("prompt should contain 2026年第25周, got: %s", prompt)
	}
	if strings.Contains(prompt, "本周") {
		t.Fatal("prompt should NOT contain hardcoded 本周")
	}

	// Must contain new prompt structure.
	if !strings.Contains(prompt, "关键动向") {
		t.Error("prompt should contain 关键动向 section")
	}
	if !strings.Contains(prompt, "涉及主体与领域") {
		t.Error("prompt should contain 涉及主体与领域 section")
	}
	if !strings.Contains(prompt, "事件脉络") {
		t.Error("prompt should contain 事件脉络 section")
	}
	if !strings.Contains(prompt, "不做事态预测") {
		t.Error("prompt should contain 不做事态预测 constraint")
	}
}

// ── Tests: FormatPeriodLabel ───────────────────────────────────────────────

func TestFormatPeriodLabel(t *testing.T) {
	tests := []struct {
		name        string
		period      string
		granularity string
		want        string
	}{
		{
			name:        "week",
			period:      "2026-W27",
			granularity: "week",
			// W27 = Mon Jun 29 – Sun Jul 5
			want: "2026年第27周（6月29日-7月5日）",
		},
		{
			name:        "month",
			period:      "2026-06",
			granularity: "month",
			want:        "2026年6月",
		},
		{
			name:        "year",
			period:      "2026",
			granularity: "year",
			want:        "2026年",
		},
		{
			name:        "all",
			period:      "all",
			granularity: "all",
			want:        "全部周期（滚动汇总）",
		},
		{
			name:        "invalid week format",
			period:      "bad-week",
			granularity: "week",
			want:        "本周期",
		},
		{
			name:        "invalid month format",
			period:      "bad-month",
			granularity: "month",
			want:        "本周期",
		},
		{
			name:        "invalid year format",
			period:      "bad-year",
			granularity: "year",
			want:        "本周期",
		},
		{
			name:        "unknown granularity",
			period:      "2026-W01",
			granularity: "unknown",
			want:        "本周期",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.FormatPeriodLabel(tt.period, tt.granularity)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

// weekBoundaries returns (Monday 00:00, Monday 00:00 next week) for t.
func weekBoundaries(t time.Time) (mon, nextMon time.Time) {
	return weekRange(t)
}

// weekRange helper for test use.
func weekRange(t time.Time) (from, to time.Time) {
	weekday := t.Weekday()
	daysFromMonday := int(weekday) - int(time.Monday)
	if daysFromMonday < 0 {
		daysFromMonday += 7
	}
	monday := t.AddDate(0, 0, -daysFromMonday)
	from = time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, t.Location())
	to = from.AddDate(0, 0, 7)
	return
}
