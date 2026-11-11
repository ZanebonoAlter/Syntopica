package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/topicgraph/repository"
)

func TestHighlightStoryWhitelistAndRetry(t *testing.T) {
	old := highlightsChatFn
	defer func() { highlightsChatFn = old }()
	candidates := []repository.ReportStoryCandidate{{StoryID: 2, TagIDs: []uint{20}, ParticipantCount: 4, StoryHeat: 3.24}}
	tags := []repository.TagInput{{ID: 20, Label: "fact"}}
	calls := 0
	highlightsChatFn = func(_ context.Context, _, _ string, schema *airouter.JSONSchema, operation string) (string, error) {
		calls++
		require.Equal(t, highlightsOperation, operation)
		require.Contains(t, schema.Properties["highlights"].Items.Required, "story_id")
		if calls == 1 {
			return `{"highlights":[{"story_id":99,"title":"bad","reason":"bad","tag_ids":[20]}]}`, nil
		}
		return `{"highlights":[{"story_id":2,"title":" fact ","reason":" reason ","tag_ids":[20,99]},{"story_id":2,"title":"duplicate","reason":"r","tag_ids":[20]}]}`, nil
	}
	got, err := GenerateStoryHighlights(context.Background(), candidates, tags)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 2, calls)
	require.Equal(t, uint(2), *got[0].StoryID)
	require.Equal(t, []uint{20}, got[0].TagIDs)
	require.Equal(t, 3.2, *got[0].StoryHeat)
	require.Equal(t, "fact", got[0].Title)
	calls = 0
	highlightsChatFn = func(context.Context, string, string, *airouter.JSONSchema, string) (string, error) {
		calls++
		return `{"highlights":[]}`, nil
	}
	got, err = GenerateStoryHighlights(context.Background(), candidates, tags)
	require.NoError(t, err)
	require.Empty(t, got)
	require.Equal(t, 1, calls)
}

func TestHighlightStoryRejectsInvalidFactsAndKeepsHeatOrder(t *testing.T) {
	candidates := []repository.ReportStoryCandidate{{StoryID: 2, TagIDs: []uint{20}}, {StoryID: 1, TagIDs: []uint{10}}}
	for _, body := range []string{
		`{"highlights":null}`, `{"highlights":[{"story_id":"2","title":"t","reason":"r","tag_ids":[20]}]}`,
		`{"highlights":[{"story_id":2,"title":"t","reason":"r","tag_ids":[10]}]}`,
		`{"highlights":[{"story_id":2,"title":" ","reason":"r","tag_ids":[20]}]}`,
		`{"highlights":[{"story_id":2,"title":"t","reason":"r","tag_ids":[20],"story_heat":99}]}`,
		`{"highlights":[]} {"highlights":[]}`,
	} {
		_, err := parseStoryHighlightsResponse(body, candidates)
		require.Error(t, err, body)
	}
	got, err := parseStoryHighlightsResponse(`{"highlights":[{"story_id":1,"title":"one","reason":"r","tag_ids":[10]},{"story_id":2,"title":"two","reason":"r","tag_ids":[20]}]}`, candidates)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, uint(2), *got[0].StoryID)
	require.Equal(t, 60*time.Second, reportAttributionBudget)
}

func TestStoryHighlightCandidatesCoverMaterializedFacts(t *testing.T) {
	// inputs.tags 在 watch 物化之前固定（常规聚类输入）；物化线程的标签
	// 另行检索，不在其中——候选事实必须按线程标签批量补齐，物化故事才
	// 能入选而不是被 eligibleStoryFacts 误剔除。
	tags := []repository.TagInput{{ID: 10, Label: "聚类标签", Description: "常规事实"}}
	candidates := []repository.ReportStoryCandidate{
		{StoryID: 1, TagIDs: []uint{10}},
		{StoryID: 2, TagIDs: []uint{77}},
	}
	var fetched []uint
	fetch := func(ids []uint) ([]models.TopicTag, error) {
		fetched = append(fetched, ids...)
		return []models.TopicTag{{ID: 77, Label: "物化事件", Category: "event", Description: "物化事实"}}, nil
	}
	storyTags, eligible, err := storyHighlightCandidates(tags, candidates, fetch)
	require.NoError(t, err)
	require.Equal(t, []uint{77}, fetched, "只批量补缺失的事实标签")
	require.Len(t, eligible, 2, "有正文的物化故事必须能入选")
	require.Equal(t, uint(2), eligible[1].StoryID)
	byID := map[uint]repository.TagInput{}
	for _, tag := range storyTags {
		byID[tag.ID] = tag
	}
	require.Equal(t, "物化事件", byID[77].Label, "物化标签的事实进入 prompt 事实集")
}

func TestStoryHighlightCandidatesBackfillBeyondTop8(t *testing.T) {
	// 热度前 8 的候选标签都取不到合法事实，第 9 条有事实：截断必须发生在
	// 事实确认之后——第 9 条补入 top 8 槽位，而不是误回退标签模式。
	candidates := make([]repository.ReportStoryCandidate, 0, 9)
	for i := 0; i < 8; i++ {
		candidates = append(candidates, repository.ReportStoryCandidate{
			StoryID: uint(100 + i), StoryHeat: float64(100 - i), TagIDs: []uint{uint(1000 + i)},
		})
	}
	candidates = append(candidates, repository.ReportStoryCandidate{StoryID: 200, StoryHeat: 1, TagIDs: []uint{2000}})
	fetch := func([]uint) ([]models.TopicTag, error) {
		return []models.TopicTag{{ID: 2000, Label: "第九条事实"}}, nil
	}
	_, eligible, err := storyHighlightCandidates(nil, candidates, fetch)
	require.NoError(t, err)
	require.Len(t, eligible, 1)
	require.Equal(t, uint(200), eligible[0].StoryID)
}

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
