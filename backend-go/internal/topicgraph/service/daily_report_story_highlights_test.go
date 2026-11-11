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

// TestStoryAttributionHardStop covers retire-story-adopt-event-metrics review
// R-1（用户拍板：删开关改硬停写）：日报尾段永久不进入归属判定——即使 store
// 里备着一批可归属的 thread，也不新建 / 合并 / 挂载任何事件线，不写
// story_id，不产生 story_signals。回滚路径是 git revert，不是改配置键。
func TestStoryAttributionHardStop(t *testing.T) {
	store, _ := storyAttributeFixture(t)
	fx := newClusterFixture(t)

	finalizeDailyReportHighlights(context.Background(), fx.report, &reportHighlightInputs{})

	require.Empty(t, store.listReports, "硬停写后日报尾段不得再进入归属判定（连查询都不发生）")
	require.Empty(t, store.created, "不得新建 board_stories")
	require.Empty(t, store.attached, "不得把 thread 挂到事件线（即不写 story_id）")
	require.Empty(t, store.merged, "不得合并事件线")
	require.Empty(t, store.calls, "归属判定链的任何一步都不得被调用")

	var stories, signals int64
	require.NoError(t, fx.db.Model(&repository.BoardStory{}).Count(&stories).Error)
	require.NoError(t, fx.db.Model(&repository.StorySignal{}).Count(&signals).Error)
	require.Zero(t, stories, "新日报恒零 board_stories 写入")
	require.Zero(t, signals, "新日报恒零 story_signals 写入")
}
