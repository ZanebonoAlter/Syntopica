package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ── Fault-injection mocks (daily-report-lifeline-heal task 1.2) ─────────────

// funcAirouter delegates Chat to a test-controlled function and counts every
// call, so tests can assert zero-LLM steady state and inject per-topic
// failures (via req.SessionID = lifeline_context_<topicID>_<gran>_<hex>).
type funcAirouter struct {
	mu    sync.Mutex
	calls int
	fn    func(req airouter.ChatRequest) (*airouter.ChatResult, error)
}

func (m *funcAirouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	m.mu.Lock()
	m.calls++
	fn := m.fn
	m.mu.Unlock()
	if fn != nil {
		return fn(req)
	}
	return &airouter.ChatResult{Content: "summarized"}, nil
}

func (m *funcAirouter) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// failForTopic returns an fn that errors on every Chat for the given topic
// (session id prefix match) and succeeds otherwise.
func failForTopic(topicID uint, granularity string) func(req airouter.ChatRequest) (*airouter.ChatResult, error) {
	prefix := fmt.Sprintf("lifeline_context_%d_%s_", topicID, granularity)
	return func(req airouter.ChatRequest) (*airouter.ChatResult, error) {
		if strings.HasPrefix(req.SessionID, prefix) {
			return nil, errors.New("llm exploded for topic")
		}
		return &airouter.ChatResult{Content: "summarized"}, nil
	}
}

// errLister fails ListActiveTopicIDs (lister-failure branch).
type errLister struct {
	err error
}

func (l *errLister) ListActiveTopicIDs(ctx context.Context) ([]uint, error) {
	return nil, l.err
}

// failingDatesReader makes SectionDates error for selected topics while
// delegating everything else to the plain mock.
type failingDatesReader struct {
	*mockSectionReader
	failFor map[uint]error
}

func (m *failingDatesReader) SectionDates(ctx context.Context, topicID uint) ([]time.Time, error) {
	if err, ok := m.failFor[topicID]; ok {
		return nil, err
	}
	return m.mockSectionReader.SectionDates(ctx, topicID)
}

// addMonthData registers one section date (→ one month period of data) for a topic.
func addMonthData(sr *mockSectionReader, topicID uint, d time.Time) {
	sr.datesByTopic[topicID] = append(sr.datesByTopic[topicID], d)
}

// getLifelineRow reads one (topic, granularity, period) row; ok=false when absent.
func getLifelineRow(t *testing.T, topicID uint, gran, period string) (row *repository.TopicLifelineContext, ok bool) {
	t.Helper()
	ctxRow, err := repository.Repo.GetTopicLifelineContext(context.Background(), topicID, gran, period)
	if err != nil {
		require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "get lifeline context: %v", err)
		return nil, false
	}
	return ctxRow, true
}

// ── Tests ───────────────────────────────────────────────────────────────────

// 主链路节拍 2（test-cases.md）：缺行补齐 + 已有行不动，计数正确。
func TestHealMissingTolerant_FillsMissingRows(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday() // 2026-07-06 → current month period 2026-07

	ar := &funcAirouter{}
	sr := newMockSectionReader()
	addMonthData(sr, 1, time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)) // topic 1: gap
	addMonthData(sr, 2, time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)) // topic 2: covered
	require.NoError(t, repository.Repo.UpsertTopicLifelineContext(context.Background(), &repository.TopicLifelineContext{
		PersistentTopicID: 2, Granularity: "month", Period: "2026-07",
		Content: "covered", AsOfDate: now, Source: "llm_assisted",
	}))
	svc := newTestService(ar, sr)

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "month", now, &mockTopicLister{ids: []uint{1, 2}})
	require.NoError(t, err)
	require.Equal(t, 1, healed, "only topic 1's gap healed")
	require.Zero(t, failed)
	require.Equal(t, 1, ar.callCount(), "exactly one LLM call for the one gap")

	row, ok := getLifelineRow(t, 1, "month", "2026-07")
	require.True(t, ok, "topic 1 gap filled")
	require.NotEmpty(t, row.Content)
	covered, ok := getLifelineRow(t, 2, "month", "2026-07")
	require.True(t, ok)
	require.Equal(t, "covered", covered.Content, "existing row untouched")
}

// 白盒分支「单 period LLM 失败」：同 topic 第一个 period 失败后，
// 下一 period 仍继续补（period 级容错，非仅 topic 级）。
func TestHealMissingTolerant_PeriodFailureContinuesSameTopic(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday() // current month period 2026-07

	// 首个 LLM 调用失败（命中 2026-06 期），后续照常成功。
	var calls int
	ar := &funcAirouter{fn: func(req airouter.ChatRequest) (*airouter.ChatResult, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("first period llm boom")
		}
		return &airouter.ChatResult{Content: "summarized"}, nil
	}}
	sr := newMockSectionReader()
	addMonthData(sr, 1, time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)) // → 2026-06（失败）
	addMonthData(sr, 1, time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)) // → 2026-07（继续补）
	svc := newTestService(ar, sr)

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "month", now, &mockTopicLister{ids: []uint{1}})
	require.NoError(t, err)
	require.Equal(t, 1, healed, "2026-07 filled despite 2026-06 failure")
	require.Equal(t, 1, failed, "2026-06 counted as failed")

	_, ok := getLifelineRow(t, 1, "month", "2026-07")
	require.True(t, ok)
	_, ok = getLifelineRow(t, 1, "month", "2026-06")
	require.False(t, ok)
}

// 前置「越界引用」变体：素材存在但话题不在 lister 名单（lister 只回
// active，archived 不进名单）→ 不补不调用。
func TestHealMissingTolerant_TopicNotListedNotHealed(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday()

	ar := &funcAirouter{}
	sr := newMockSectionReader()
	addMonthData(sr, 42, time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)) // 有素材但不在名单（archived）
	svc := newTestService(ar, sr)

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "month", now, &mockTopicLister{ids: []uint{7}})
	require.NoError(t, err)
	require.Zero(t, healed)
	require.Zero(t, failed)
	require.Zero(t, ar.callCount(), "未列入名单的话题不补不调用")

	_, ok := getLifelineRow(t, 42, "month", "2026-07")
	require.False(t, ok)
}

// 时间窗口「空窗口」变体：listed 但零素材 → 「有素材但无行」判定天然排除。
func TestHealMissingTolerant_NoMaterialNoHeal(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday()

	ar := &funcAirouter{}
	svc := newTestService(ar, newMockSectionReader())

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "month", now, &mockTopicLister{ids: []uint{9}})
	require.NoError(t, err)
	require.Zero(t, healed)
	require.Zero(t, failed)
	require.Zero(t, ar.callCount(), "无素材话题零 LLM 调用")

	_, ok := getLifelineRow(t, 9, "month", "2026-07")
	require.False(t, ok)
}

// 时间窗口「边界两端」变体：跨月切换夜 23:59/00:00 分属两期，各自成行。
func TestHealMissingTolerant_MonthBoundaryDates(t *testing.T) {
	setupLifelineTestDB(t)
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC) // 当期 2026-08

	ar := &funcAirouter{}
	sr := newMockSectionReader()
	addMonthData(sr, 1, time.Date(2026, 7, 31, 23, 59, 0, 0, time.UTC)) // → 2026-07
	addMonthData(sr, 1, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))    // → 2026-08
	svc := newTestService(ar, sr)

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "month", now, &mockTopicLister{ids: []uint{1}})
	require.NoError(t, err)
	require.Equal(t, 2, healed, "边界日期分属两期，各自补齐")
	require.Zero(t, failed)
	require.Equal(t, 2, ar.callCount())

	_, ok := getLifelineRow(t, 1, "month", "2026-07")
	require.True(t, ok)
	_, ok = getLifelineRow(t, 1, "month", "2026-08")
	require.True(t, ok)
}

// 主链路节拍 5：单 topic LLM 失败 → failed++ 且后续 topic 照常补。
func TestHealMissingTolerant_TopicLLMFailureContinues(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday()

	ar := &funcAirouter{fn: failForTopic(2, "month")}
	sr := newMockSectionReader()
	for _, id := range []uint{1, 2, 3} {
		addMonthData(sr, id, time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC))
	}
	svc := newTestService(ar, sr)

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "month", now, &mockTopicLister{ids: []uint{1, 2, 3}})
	require.NoError(t, err, "single-topic LLM failure must not abort the sweep")
	require.Equal(t, 2, healed, "topics 1 and 3 still healed")
	require.Equal(t, 1, failed, "topic 2 counted as failed")

	_, ok := getLifelineRow(t, 1, "month", "2026-07")
	require.True(t, ok)
	_, ok = getLifelineRow(t, 3, "month", "2026-07")
	require.True(t, ok)
	_, ok = getLifelineRow(t, 2, "month", "2026-07")
	require.False(t, ok, "failed topic left unfilled (retried next sweep)")
}

// 补齐失败顺延不丢 Scenario：次日轮只重试仍缺的 (topic, period)，已补行不重写。
func TestHealMissingTolerant_PartialFailureRetryOnlyMissing(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday()

	failing := &funcAirouter{fn: failForTopic(2, "month")}
	sr := newMockSectionReader()
	for _, id := range []uint{1, 2} {
		addMonthData(sr, id, time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC))
	}
	svc := newTestService(failing, sr)
	lister := &mockTopicLister{ids: []uint{1, 2}}

	// Day 1: topic 1 healed, topic 2 fails.
	h1, f1, err := svc.HealMissingTolerant(context.Background(), "month", now, lister)
	require.NoError(t, err)
	require.Equal(t, 1, h1)
	require.Equal(t, 1, f1)
	first, ok := getLifelineRow(t, 1, "month", "2026-07")
	require.True(t, ok)
	firstContent := first.Content

	// Day 2 (LLM healthy again): only topic 2's gap remains → exactly 1 call.
	healthy := &funcAirouter{}
	svc = newTestService(healthy, sr)
	h2, f2, err := svc.HealMissingTolerant(context.Background(), "month", now, lister)
	require.NoError(t, err)
	require.Equal(t, 1, h2, "retry fills only the still-missing topic")
	require.Zero(t, f2)
	require.Equal(t, 1, healthy.callCount(), "healed rows are not rewritten (zero extra calls)")

	again, ok := getLifelineRow(t, 1, "month", "2026-07")
	require.True(t, ok)
	require.Equal(t, firstContent, again.Content, "day-1 row content byte-identical")
}

// 已存在行不被重写：内容逐字不变、零 LLM 调用。
func TestHealMissingTolerant_ExistingRowsNotRewritten(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday()

	ar := &funcAirouter{}
	sr := newMockSectionReader()
	addMonthData(sr, 7, time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC))
	require.NoError(t, repository.Repo.UpsertTopicLifelineContext(context.Background(), &repository.TopicLifelineContext{
		PersistentTopicID: 7, Granularity: "month", Period: "2026-07",
		Content: "original", AsOfDate: now, Source: "llm_assisted",
	}))
	svc := newTestService(ar, sr)

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "month", now, &mockTopicLister{ids: []uint{7}})
	require.NoError(t, err)
	require.Zero(t, healed)
	require.Zero(t, failed)
	require.Zero(t, ar.callCount(), "existing row must not trigger any LLM call")

	row, ok := getLifelineRow(t, 7, "month", "2026-07")
	require.True(t, ok)
	require.Equal(t, "original", row.Content)
}

// 稳态零成本 Scenario：全部行齐 → 零缺口零 LLM 调用。
func TestHealMissingTolerant_NoGapsZeroLLMCalls(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday()

	ar := &funcAirouter{}
	sr := newMockSectionReader()
	for _, id := range []uint{1, 2, 3} {
		addMonthData(sr, id, time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC))
		require.NoError(t, repository.Repo.UpsertTopicLifelineContext(context.Background(), &repository.TopicLifelineContext{
			PersistentTopicID: id, Granularity: "month", Period: "2026-07",
			Content: "have it", AsOfDate: now, Source: "llm_assisted",
		}))
	}
	svc := newTestService(ar, sr)

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "month", now, &mockTopicLister{ids: []uint{1, 2, 3}})
	require.NoError(t, err)
	require.Zero(t, healed)
	require.Zero(t, failed)
	require.Zero(t, ar.callCount(), "steady state must cost zero LLM calls")
}

// 白盒分支「lister 失败」：返回 error、零 LLM 调用。
func TestHealMissingTolerant_ListerErrorAborts(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday()

	ar := &funcAirouter{}
	sr := newMockSectionReader()
	svc := newTestService(ar, sr)

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "month", now, &errLister{err: errors.New("db down")})
	require.Error(t, err)
	require.Zero(t, healed)
	require.Zero(t, failed)
	require.Zero(t, ar.callCount(), "no LLM calls when topics cannot be listed")
}

// 白盒分支「单 topic 读取 sections 失败」：failed++ 继续 #2。
func TestHealMissingTolerant_SectionDatesFailureSkipsTopic(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday()

	ar := &funcAirouter{}
	sr := &failingDatesReader{
		mockSectionReader: newMockSectionReader(),
		failFor:           map[uint]error{1: errors.New("sections read boom")},
	}
	addMonthData(sr.mockSectionReader, 2, time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC))
	svc := newTestService(ar, sr)

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "month", now, &mockTopicLister{ids: []uint{1, 2}})
	require.NoError(t, err)
	require.Equal(t, 1, healed, "topic 2 healed despite topic 1 read failure")
	require.Equal(t, 1, failed, "topic 1 counted as failed")
}

// 前置空集变体：无 active 话题 → 零工作量零调用。
func TestHealMissingTolerant_EmptyTopicListZeroWork(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday()

	ar := &funcAirouter{}
	svc := newTestService(ar, newMockSectionReader())

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "month", now, &mockTopicLister{ids: nil})
	require.NoError(t, err)
	require.Zero(t, healed)
	require.Zero(t, failed)
	require.Zero(t, ar.callCount())
}

// 时间窗口「period 上界」分支：素材日期晚于当前期（时钟回拨）不补未来期。
func TestHealMissingTolerant_FuturePeriodSkipped(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday() // 2026-07

	ar := &funcAirouter{}
	sr := newMockSectionReader()
	addMonthData(sr, 1, time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC)) // period 2027-01 > current
	svc := newTestService(ar, sr)

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "month", now, &mockTopicLister{ids: []uint{1}})
	require.NoError(t, err)
	require.Zero(t, healed)
	require.Zero(t, failed)
	require.Zero(t, ar.callCount(), "no future-period generation")

	_, ok := getLifelineRow(t, 1, "month", "2027-01")
	require.False(t, ok)
}

// 跨窗口变体 + 「month 齐 year 缺」：year 轮独立补多年历史缺口。
func TestHealMissingTolerant_YearSweepFillsMultiYearGaps(t *testing.T) {
	setupLifelineTestDB(t)
	now := fixedMonday() // current year period 2026

	ar := &funcAirouter{}
	sr := newMockSectionReader()
	addMonthData(sr, 1, time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)) // → 2025
	addMonthData(sr, 1, time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)) // → 2026
	// Month rows pre-created for both periods so the month dimension is full —
	// only the year dimension carries gaps (「month 齐 year 缺」变体)。
	for _, p := range []string{"2025-03", "2026-03"} {
		require.NoError(t, repository.Repo.UpsertTopicLifelineContext(context.Background(), &repository.TopicLifelineContext{
			PersistentTopicID: 1, Granularity: "month", Period: p,
			Content: "month row present", AsOfDate: now, Source: "llm_assisted",
		}))
	}
	svc := newTestService(ar, sr)

	healed, failed, err := svc.HealMissingTolerant(context.Background(), "year", now, &mockTopicLister{ids: []uint{1}})
	require.NoError(t, err)
	require.Equal(t, 2, healed, "both 2025 and 2026 year rows backfilled")
	require.Zero(t, failed)
	require.Equal(t, 2, ar.callCount())

	_, ok := getLifelineRow(t, 1, "year", "2025")
	require.True(t, ok)
	_, ok = getLifelineRow(t, 1, "year", "2026")
	require.True(t, ok)

	// 月轮独立：month 行已存在 → 月扫描零调用（两粒度互不掺和）。
	monthAir := &funcAirouter{}
	monthSvc := newTestService(monthAir, sr)
	mh, mf, err := monthSvc.HealMissingTolerant(context.Background(), "month", now, &mockTopicLister{ids: []uint{1}})
	require.NoError(t, err)
	require.Zero(t, mh)
	require.Zero(t, mf)
	require.Zero(t, monthAir.callCount())
}
