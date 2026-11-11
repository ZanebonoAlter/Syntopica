package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/topicgraph/repository"
)

func TestAttributeOnlineStopsOnErrorAndTimeout(t *testing.T) {
	store := &fakeStoryAttributeStore{}
	// Production uses the same synchronous runner: a cancelled context is
	// returned directly, not raced against a detached writer.
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	_, err := attributeStoriesForReportWithStats(ctx, store, 1, 1, time.Now(), false, true)
	require.True(t, errors.Is(err, context.DeadlineExceeded))
	require.Empty(t, store.listReports)
}

func TestAttributeOnlineBlockingEmbeddingCancelsWithoutLateWrites(t *testing.T) {
	store, _ := storyAttributeFixture(t)
	old := storySubjectEmbedWithContext
	defer func() { storySubjectEmbedWithContext = old }()
	var observedDeadline bool
	storySubjectEmbedWithContext = func(ctx context.Context, _ string) ([]float64, error) {
		_, observedDeadline = ctx.Deadline()
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	stats, err := attributeStoriesForReportWithStats(ctx, store, 7, 21, storyAttributeDay(), false, true)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.True(t, observedDeadline)
	require.Equal(t, 1, stats.Failed)
	assertStoryAttributeNoWrites(t, store)
	// The runner has returned; no background worker remains to perform a write.
	time.Sleep(10 * time.Millisecond)
	assertStoryAttributeNoWrites(t, store)
}

func TestAttributeOnlineStopsAtFirstThreadError(t *testing.T) {
	store, _ := storyAttributeFixture(t)
	store.threads = append(store.threads, store.threads[0])
	store.recallErr = errors.New("recall unavailable")
	stats, err := attributeStoriesForReportWithStats(context.Background(), store, 7, 21, storyAttributeDay(), false, true)
	require.ErrorContains(t, err, "recall unavailable")
	require.Equal(t, 1, stats.Threads)
	require.Len(t, store.recalls, 1)
	assertStoryAttributeNoWrites(t, store)
}

// TestStoryAttributionSwitch covers retire-story-adopt-event-metrics task 1.1:
// 开关开 = 行为不变（归属判定照常进入）、开关关 = 零写入（不新建 / 合并 /
// 挂载任何事件线，也不写 story_id）、坏值 = 回退默认（行为不变）。
func TestStoryAttributionSwitch(t *testing.T) {
	oldSwitch, oldTimeout, oldOnline := storyAttributionEnabled, reportAttributionTimeout, attributeReportOnline
	t.Cleanup(func() {
		storyAttributionEnabled, reportAttributionTimeout, attributeReportOnline = oldSwitch, oldTimeout, oldOnline
	})
	reportAttributionTimeout = reportAttributionBudget

	// 开关开（缺项回退默认）：归属判定照常进入，行为不变。
	store, _ := storyAttributeFixture(t)
	attributeReportOnline = func(ctx context.Context, boardID, reportID uint, date time.Time) error {
		_, err := attributeStoriesForReportWithStats(ctx, store, boardID, reportID, date, false)
		return err
	}
	storyAttributionEnabled = func() bool { return repository.DefaultStoryAttributionEnabled }
	report := &repository.BoardDailyReport{ID: 21, SemanticBoardID: 7, PeriodDate: storyAttributeDay()}
	require.NoError(t, boundedReportAttribution(context.Background(), report))
	require.NotEmpty(t, store.listReports, "开关开时归属判定必须照常进入（行为不变）")

	// 开关关：零写入——连归属判定都不进入，更不会产生 board_stories /
	// story_signals / story_id。
	store, _ = storyAttributeFixture(t)
	storyAttributionEnabled = func() bool { return false }
	require.NoError(t, boundedReportAttribution(context.Background(), report))
	require.Empty(t, store.listReports, "开关关时不得进入归属判定")
	require.Empty(t, store.created)
	require.Empty(t, store.attached)
	require.Empty(t, store.merged)

	// 坏值：告警回退默认（写 story，行为不变）。
	enabled, ok := repository.ParseStoryAttributionEnabled("maybe")
	require.False(t, ok)
	require.Equal(t, repository.DefaultStoryAttributionEnabled, enabled)
	// 大小写 / 空白变体按布尔解析。
	enabled, ok = repository.ParseStoryAttributionEnabled(" FALSE ")
	require.True(t, ok)
	require.False(t, enabled)
	enabled, ok = repository.ParseStoryAttributionEnabled("true")
	require.True(t, ok)
	require.True(t, enabled)
}
