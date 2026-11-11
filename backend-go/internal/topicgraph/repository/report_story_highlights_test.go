package repository

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestLaneHottestStoryIdentity(t *testing.T) {
	one, two := uint(1), uint(2)
	a := laneStoryAggregates{backfilled: true, survivors: map[uint]uint{1: 1, 2: 2}, participants: map[uint]int{1: 4, 2: 4}, titles: map[uint]string{1: "winner", 2: "other"}}
	sections := []laneSectionRow{{ID: 10}}
	stats := map[uint][]laneThreadStat{10: {{StoryID: &two}, {StoryID: &one}, {StoryID: &one}}}
	id, title := a.hottestStory(sections, stats)
	require.Equal(t, &one, id)
	require.Equal(t, "winner", title)
	count, max, missing := a.foldDay(sections, stats)
	require.Equal(t, 2, count)
	require.Equal(t, 4, max)
	require.Zero(t, missing)
	id, title = a.hottestStory(sections, nil)
	require.Nil(t, id)
	require.Empty(t, title)
	a.participants[1], a.participants[2] = 0, 0
	id, _ = a.hottestStory(sections, stats)
	require.Equal(t, &one, id)
}

func TestHighlightStorySurvivorsOneHop(t *testing.T) {
	two, three, four := uint(2), uint(3), uint(4)
	stories := []BoardStory{{ID: 1, MergedInto: &two}, {ID: 2}, {ID: 3, MergedInto: &four}, {ID: 4, MergedInto: &three}, {ID: 5, MergedInto: &three}}
	ids := highlightStorySurvivors(stories)
	require.Equal(t, uint(2), ids[1])
	require.Equal(t, uint(2), ids[2])
	require.Zero(t, ids[3])
	require.Zero(t, ids[4])
	require.Zero(t, ids[5])
}

func TestHighlightHeatCandidateRanking(t *testing.T) {
	at := time.Date(2026, 11, 6, 23, 59, 59, 0, storyHeatZone)
	e := boardHeatEvidence{survivors: map[uint]uint{1: 1, 2: 2, 3: 3}, signals: []storyHeatSignal{
		{StoryID: 1, ParticipantKey: "a", ObservedAt: at.Add(-24 * time.Hour)},
		{StoryID: 1, ParticipantKey: "b", ObservedAt: at.Add(-24 * time.Hour)},
		{StoryID: 2, ParticipantKey: "a", ObservedAt: at},
		{StoryID: 3, ParticipantKey: "a", ObservedAt: at.Add(time.Second)},
	}}
	candidates := rankReportStoryCandidates([]ReportStoryCandidate{{StoryID: 2}, {StoryID: 3}, {StoryID: 1}}, e, at)
	require.Equal(t, uint(1), candidates[0].StoryID)
	require.Equal(t, 2, candidates[0].ParticipantCount)
	require.Equal(t, uint(2), candidates[1].StoryID)
	require.Zero(t, candidates[2].StoryHeat)
}

// TestHighlightHeatCandidateRankingUsesContentIdentity（fix-duplicate-article-heat
// 2.4）：高亮的「来源数」按内容身份去重——同一篇内容的两个 feed 副本算 1 个
// 来源（与 board_stories.participant_count 同一口径）；content_key 为空的
// 证据退化为 feed 粒度，不与有内容身份的证据合并。
func TestHighlightHeatCandidateRankingUsesContentIdentity(t *testing.T) {
	at := time.Date(2026, 11, 6, 23, 59, 59, 0, storyHeatZone)
	e := boardHeatEvidence{survivors: map[uint]uint{1: 1, 2: 2}, signals: []storyHeatSignal{
		{StoryID: 1, ParticipantKey: "feed:10", ContentKey: "https://example.com/p/1", ObservedAt: at},
		{StoryID: 1, ParticipantKey: "feed:20", ContentKey: "https://example.com/p/1", ObservedAt: at},
		{StoryID: 2, ParticipantKey: "feed:30", ContentKey: "https://example.com/p/2", ObservedAt: at},
		{StoryID: 2, ParticipantKey: "feed:40", ContentKey: "", ObservedAt: at},
	}}
	candidates := rankReportStoryCandidates([]ReportStoryCandidate{{StoryID: 1}, {StoryID: 2}}, e, at)
	// 事件线 2 有 2 个参与者（1 份内容 + 1 个退化 feed）→ 热度更高，排在前。
	require.Equal(t, uint(2), candidates[0].StoryID)
	require.Equal(t, 2, candidates[0].ParticipantCount, "有内容身份 + 退化 feed = 2 个来源")
	require.Equal(t, uint(1), candidates[1].StoryID)
	require.Equal(t, 1, candidates[1].ParticipantCount, "同一篇内容的两个 feed 副本 = 1 个来源")
}
