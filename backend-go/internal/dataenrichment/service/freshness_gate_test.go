package service_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// ── 补全门用例（fix-board-analysis-material 7.1 / M4 重写）──────────────────
//
// 语义升级：gate 从「保鲜」变「补全」——mock 提供 section dates 推有料周期集，
// 无行→首建、行最后写于 72h 前→重算；week 不在检查集；限额溢出降级。

type mockFreshnessRefresher struct {
	mu     sync.Mutex
	calls  []string
	failOn map[string]bool
	dates  []time.Time // section dates returned per topic
	// optional side-effect: write a (topic, gran, period) row as cycle-A would
	refresh func(topicID uint, gran, period string)
}

func (m *mockFreshnessRefresher) RefreshGranularity(ctx context.Context, topicID uint, granularity string, now time.Time) error {
	return m.RefreshPeriod(ctx, topicID, granularity, service.PeriodForGranularity(now, granularity), now)
}

func (m *mockFreshnessRefresher) RefreshPeriod(ctx context.Context, topicID uint, granularity, period string, now time.Time) error {
	m.mu.Lock()
	m.calls = append(m.calls, fmt.Sprintf("%d/%s/%s", topicID, granularity, period))
	fail := m.failOn != nil && m.failOn[fmt.Sprintf("%d/%s/%s", topicID, granularity, period)]
	refresh := m.refresh
	m.mu.Unlock()
	if fail {
		return fmt.Errorf("mock refresh failure")
	}
	if refresh != nil {
		refresh(topicID, granularity, period)
	}
	return nil
}

func (m *mockFreshnessRefresher) SectionDates(ctx context.Context, topicID uint) ([]time.Time, error) {
	return m.dates, nil
}

func (m *mockFreshnessRefresher) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func newFreshnessOrch(t *testing.T) (*service.OrchestratorService, *repository.Repository, *mockFreshnessRefresher) {
	t.Helper()
	repo := setupOrchTestDB(t)
	refresher := &mockFreshnessRefresher{}
	orch := service.NewOrchestratorService(
		newMockAirRouter(), repo, &orchMockLifelineReader{}, service.NewLifelineRenderer(),
		service.NewRegistry(&nilFetcher{}), &mockBoardConfigReader{cfg: service.DefaultBoardConfig()}, testCap,
	)
	orch.SetFreshnessRefresher(refresher)
	return orch, repo, refresher
}

func seedGranRow(t *testing.T, repo *repository.Repository, topicID uint, gran, period string, asOf time.Time) {
	t.Helper()
	// Backdate UpdatedAt alongside AsOfDate: the gate reads "last written"
	// from UpdatedAt (production rows carry their true write time), and GORM
	// would otherwise auto-stamp now — masking staleness in fixtures.
	if err := repo.UpsertTopicLifelineContext(context.Background(), &repository.TopicLifelineContext{
		PersistentTopicID: topicID, Granularity: gran, Period: period,
		Content: "c", AsOfDate: asOf, Source: "manual", UpdatedAt: asOf,
	}); err != nil {
		t.Fatalf("seed %s row: %v", gran, err)
	}
}

// monthDates builds section dates spanning the last n months (day 10 of
// each). Anchoring to day 10 is load-bearing: time.AddDate normalizes
// month-end overflow (e.g. 2026-08-31 minus two months becomes 2026-07-01,
// not 2026-06-31), which would silently collapse the derived period set
// when the test happens to run on a 29th–31st.
func monthDates(n int, end time.Time) []time.Time {
	base := time.Date(end.Year(), end.Month(), 10, 0, 0, 0, 0, end.Location())
	out := make([]time.Time, 0, n+1)
	for i := n; i >= 0; i-- {
		out = append(out, base.AddDate(0, -i, 0))
	}
	return out
}

// M4.1 截断档重算：7 月行存在但最后写于 6 天前（半月档）→ 重算该周期。
func TestFreshnessGate_TruncatedRowRecomputed(t *testing.T) {
	orch, repo, refresher := newFreshnessOrch(t)
	now := time.Now()
	// Section data: this month + last month.
	refresher.dates = monthDates(1, now)
	// Last-month row exists but last written 6 days ago (mid-month snapshot).
	lastMonth := now.AddDate(0, -1, 0)
	seedGranRow(t, repo, 1, "month", service.FormatMonth(lastMonth), now.AddDate(0, 0, -6))
	// This-month row + year row fresh (written now).
	seedGranRow(t, repo, 1, "month", service.FormatMonth(now), now)
	seedGranRow(t, repo, 1, "year", service.FormatYear(now), now)

	report := orch.EnsureLaneFreshnessForTest(context.Background(), []uint{1})
	want := fmt.Sprintf("1/month/%s", service.FormatMonth(lastMonth))
	if len(refresher.calls) != 1 || refresher.calls[0] != want {
		t.Fatalf("want refresh %s only, got %v", want, refresher.calls)
	}
	if report.Refreshed != 1 {
		t.Fatalf("report.Refreshed=%d", report.Refreshed)
	}
}

// M4.2 无记录首建：新泳道 month 无行 → 装配前建首份（不再留给定时任务）。
func TestFreshnessGate_FirstRowCreated(t *testing.T) {
	orch, _, refresher := newFreshnessOrch(t)
	now := time.Now()
	refresher.dates = monthDates(2, now) // 3 data-bearing months, zero rows

	report := orch.EnsureLaneFreshnessForTest(context.Background(), []uint{1})
	if report.Refreshed != 4 { // 3 months + current year
		t.Fatalf("want 4 created rows (3 month + 1 year), got %d: %v", report.Refreshed, refresher.calls)
	}
	months, years := 0, 0
	for _, c := range refresher.calls {
		if strings.HasPrefix(c, "1/month/") {
			months++
		} else if strings.HasPrefix(c, "1/year/") {
			years++
		}
	}
	if months != 3 || years != 1 {
		t.Fatalf("want 3 month + 1 year creations, got %v", refresher.calls)
	}
}

// M4.3 无料周期不入集：本周出生的泳道无月级 section → skip_no_data，不建空档。
func TestFreshnessGate_NoDataSkipped(t *testing.T) {
	orch, _, refresher := newFreshnessOrch(t)
	refresher.dates = []time.Time{time.Now()} // only this month's data → month has 1 period, year has 1

	report := orch.EnsureLaneFreshnessForTest(context.Background(), []uint{1})
	// month 2026-08 + year 2026 both derivable from today's data → 2 creates.
	if report.Refreshed != 2 {
		t.Fatalf("want 2 (month+year current), got %d: %v", report.Refreshed, refresher.calls)
	}
}

// M4.4 week 不在检查集：有 week 数据、有新鲜 week 行 → 也不产生 week 调用。
func TestFreshnessGate_WeekOutOfCheckedSet(t *testing.T) {
	orch, repo, refresher := newFreshnessOrch(t)
	now := time.Now()
	refresher.dates = monthDates(1, now)
	seedGranRow(t, repo, 1, "month", service.FormatMonth(now), now)
	seedGranRow(t, repo, 1, "month", service.FormatMonth(now.AddDate(0, -1, 0)), now)
	seedGranRow(t, repo, 1, "year", service.FormatYear(now), now)

	report := orch.EnsureLaneFreshnessForTest(context.Background(), []uint{1})
	if refresher.callCount() != 0 {
		t.Fatalf("week/month/year all fresh → 0 calls, got %v", refresher.calls)
	}
	for _, d := range report.Details {
		if d.Granularity == "week" {
			t.Fatalf("week must never be checked: %+v", d)
		}
	}
}

// M4.5 限额溢出降级：补全需求 > 40 → 溢出部分 budget_exhausted，不报错不阻塞。
func TestFreshnessGate_BudgetExhausted(t *testing.T) {
	orch, _, refresher := newFreshnessOrch(t)
	now := time.Now()
	// 50 data-bearing months, zero rows → 50 needs > 40 cap.
	refresher.dates = monthDates(50, now)

	report := orch.EnsureLaneFreshnessForTest(context.Background(), []uint{1})
	if report.Refreshed != 40 {
		t.Fatalf("cap: want exactly 40 refreshes, got %d", report.Refreshed)
	}
	var exhausted int
	for _, d := range report.Details {
		if d.Action == "budget_exhausted" {
			exhausted++
		}
	}
	if exhausted == 0 {
		t.Fatal("expected budget_exhausted details")
	}
}

// M4.6 幂等：同日两次触发，第二次零调用（行新鲜）。
func TestFreshnessGate_IdempotentSameDay(t *testing.T) {
	orch, repo, refresher := newFreshnessOrch(t)
	now := time.Now()
	refresher.dates = monthDates(1, now)
	// Side-effect: writing a row via cycle-A updates UpdatedAt → fresh.
	refresher.refresh = func(topicID uint, gran, period string) {
		seedGranRow(t, repo, topicID, gran, period, now)
	}

	orch.EnsureLaneFreshnessForTest(context.Background(), []uint{1})
	first := refresher.callCount()
	if first == 0 {
		t.Fatal("first pass should have created rows")
	}
	orch.EnsureLaneFreshnessForTest(context.Background(), []uint{1})
	if refresher.callCount() != first {
		t.Fatalf("second pass same day must add 0 calls: %d → %d", first, refresher.callCount())
	}
}

// M4.7 补全失败降级：失败留痕（Failed + error detail），分析不中断。
func TestFreshnessGate_FailureDegrades(t *testing.T) {
	orch, _, refresher := newFreshnessOrch(t)
	now := time.Now()
	refresher.dates = monthDates(1, now)
	lastMonth := service.FormatMonth(now.AddDate(0, -1, 0))
	thisMonth := service.FormatMonth(now)
	refresher.failOn = map[string]bool{fmt.Sprintf("1/month/%s", lastMonth): true}

	report := orch.EnsureLaneFreshnessForTest(context.Background(), []uint{1})
	if report.Failed != 1 || report.Refreshed != 2 { // failed: last month; created: this month + year
		t.Fatalf("failed=%d refreshed=%d, want 1/2", report.Failed, report.Refreshed)
	}
	var failedDetail *service.FreshnessGateDetail
	for i := range report.Details {
		if report.Details[i].Action == "refresh_failed" {
			failedDetail = &report.Details[i]
		}
	}
	if failedDetail == nil || failedDetail.Error == "" || failedDetail.Period != lastMonth {
		t.Fatalf("failed detail must carry error and period %s: %+v", lastMonth, failedDetail)
	}
	_ = thisMonth
}

// M4.8 多泳道串行：调用顺序确定（per topic, month before year）。
func TestFreshnessGate_SerialMultiLane(t *testing.T) {
	orch, _, refresher := newFreshnessOrch(t)
	now := time.Now()
	refresher.dates = monthDates(1, now)

	report := orch.EnsureLaneFreshnessForTest(context.Background(), []uint{1, 2})
	if report.Refreshed != 6 { // 2 topics × (2 months + 1 year)
		t.Fatalf("want 6 refreshes (2 topics × month×2 + year), got %d", report.Refreshed)
	}
	want := []string{
		fmt.Sprintf("1/month/%s", service.FormatMonth(now.AddDate(0, -1, 0))),
		fmt.Sprintf("1/month/%s", service.FormatMonth(now)),
		fmt.Sprintf("1/year/%s", service.FormatYear(now)),
		fmt.Sprintf("2/month/%s", service.FormatMonth(now.AddDate(0, -1, 0))),
		fmt.Sprintf("2/month/%s", service.FormatMonth(now)),
		fmt.Sprintf("2/year/%s", service.FormatYear(now)),
	}
	for i, w := range want {
		if refresher.calls[i] != w {
			t.Fatalf("serial order broken at %d: want %q, got %q (all: %v)", i, w, refresher.calls[i], refresher.calls)
		}
	}
}
