package repository

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/platform/testutil"
)

func TestStoryHighlightsYesterdayAndRevision(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	board := seedTestBoard(t, db)
	day := NormalizeReportDate(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC))
	a := BoardStory{BoardID: board, Title: "a", FirstSeenDate: day, LastSeenDate: day}
	b := BoardStory{BoardID: board, Title: "b", FirstSeenDate: day, LastSeenDate: day}
	require.NoError(t, db.Omit("Embedding", "SubjectEmbedding").Create(&a).Error)
	require.NoError(t, db.Omit("Embedding", "SubjectEmbedding").Create(&b).Error)
	require.NoError(t, db.Model(&a).Updates(map[string]any{"merged_into": b.ID, "status": StoryStatusMerged}).Error)
	payload, _ := json.Marshal([]Highlight{{StoryID: &a.ID, Title: "yesterday", Reason: "r"}})
	yesterday := BoardDailyReport{SemanticBoardID: board, PeriodDate: day.AddDate(0, 0, -1), Status: "completed", Highlights: payload}
	require.NoError(t, repo.SaveReport(&yesterday, nil, nil))
	ids, err := repo.YesterdayHighlightStoryIDs(board, day)
	require.NoError(t, err)
	require.Equal(t, map[uint]bool{b.ID: true}, ids)
	ids, err = repo.YesterdayHighlightStoryIDs(board, day.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Empty(t, ids, "must not borrow the nearest older report")
	report := BoardDailyReport{SemanticBoardID: board, PeriodDate: day, Title: "temporary", Status: "completed", Highlights: []byte("[]")}
	require.NoError(t, repo.SaveReport(&report, nil, nil))
	stale := report
	require.NoError(t, repo.SaveReport(&report, nil, nil))
	updated, err := repo.FinalizeReportHighlights(&stale, []Highlight{{Title: "late", Reason: "late"}})
	require.NoError(t, err)
	require.False(t, updated, "stale model results cannot overwrite a rebuild")
	updated, err = repo.FinalizeReportHighlights(&report, []Highlight{{StoryID: &a.ID, Title: "today", Reason: "r", TagIDs: []uint{1}}})
	require.NoError(t, err)
	require.True(t, updated)
	got, err := repo.GetReportByID(report.ID)
	require.NoError(t, err)
	var highlights []Highlight
	require.NoError(t, json.Unmarshal(got.Highlights, &highlights))
	require.Len(t, highlights, 1)
	require.Equal(t, &b.ID, highlights[0].StoryID)
	require.True(t, highlights[0].IsProgress)
	var stored BoardDailyReport
	require.NoError(t, db.First(&stored, report.ID).Error)
	require.NoError(t, json.Unmarshal(stored.Highlights, &highlights))
	require.Equal(t, &a.ID, highlights[0].StoryID, "read normalization must not rewrite history")
}

func TestStoryHighlightCandidatesUseSignalsNotDisplayTier(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	board := seedTestBoard(t, db)
	day := NormalizeReportDate(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC))
	at := laneHeatAnchorEnd(day)
	reportID := seedTestReport(t, db, board, day)
	sectionID := seedTestSection(t, db, reportID, "facts")
	var stories []uint
	for i := 0; i < 10; i++ {
		s := BoardStory{BoardID: board, Title: "story", FirstSeenDate: day, LastSeenDate: day}
		require.NoError(t, db.Omit("Embedding", "SubjectEmbedding").Create(&s).Error)
		stories = append(stories, s.ID)
		thread := DailyReportThread{ReportID: reportID, SectionID: sectionID, Title: "facts", Scope: ThreadScopeSingle, StoryID: &s.ID, TagIDs: []byte("[123]"), DisplayTier: ThreadDisplayTierBody}
		require.NoError(t, db.Omit("Embedding").Create(&thread).Error)
	}
	// One hot line has evidence even when its evidence thread is hidden.
	feed := seedCoverageFeed(t, db, "report-highlight", 60, "ok", &at)
	seedHeatSignal(t, db, stories[9], feed, at)
	hidden := DailyReportThread{ReportID: reportID, SectionID: sectionID, Title: "hidden", Scope: ThreadScopeSingle, StoryID: &stories[9], TagIDs: []byte("[123]"), DisplayTier: ThreadDisplayTierHidden}
	require.NoError(t, db.Omit("Embedding").Create(&hidden).Error)
	candidates, err := repo.ReportStoryCandidates(board, reportID, day)
	require.NoError(t, err)
	// 仓储只排序不截：top 8 截断在服务层事实确认之后。
	require.Len(t, candidates, 10)
	require.Equal(t, stories[9], candidates[0].StoryID)
	require.Equal(t, 1, candidates[0].ParticipantCount)
	require.Equal(t, 1.0, candidates[0].StoryHeat)
	require.Equal(t, stories[0], candidates[1].StoryID)
	require.NoError(t, db.Model(&DailyReportThread{}).Where("report_id = ? AND story_id = ?", reportID, stories[9]).Update("display_tier", ThreadDisplayTierFlash).Error)
	candidates, err = repo.ReportStoryCandidates(board, reportID, day)
	require.NoError(t, err)
	require.Equal(t, stories[0], candidates[0].StoryID, "no body means not locatable, without rewriting signals")
	heat, _, err := repo.ComputeBoardHeat(board, at)
	require.NoError(t, err)
	require.Equal(t, 1.0, heat[stories[9]])
}

func TestStorySignalsSaveReportRebuildRemovesOldEvidence(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	// SaveReport 内部（assignAndUpdateTopics）读包级 Repo，与其它 SaveReport
	// 集成测试同样需要设置单例。
	Repo = repo
	t.Cleanup(func() { Repo = nil })
	board := seedTestBoard(t, db)
	day := NormalizeReportDate(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC))
	at := laneHeatAnchorEnd(day)
	story := BoardStory{BoardID: board, Title: "rebuild", FirstSeenDate: day, LastSeenDate: day}
	require.NoError(t, db.Omit("Embedding", "SubjectEmbedding").Create(&story).Error)
	feedA := seedCoverageFeed(t, db, "rebuild-a", 60, "ok", &at)
	feedB := seedCoverageFeed(t, db, "rebuild-b", 60, "ok", &at)
	articleA := seedSignalArticle(t, db, feedA, at, &at, false)
	articleB := seedSignalArticle(t, db, feedB, at, &at, false)
	// A previous report still covers A; only the rebuilt report covers B.
	previous := seedTestReport(t, db, board, day.AddDate(0, 0, -1))
	section := seedTestSection(t, db, previous, "retained")
	thread := DailyReportThread{ReportID: previous, SectionID: section, Title: "A", StoryID: &story.ID, RelatedArticleIDs: mustStoryJSON([]uint{articleA})}
	require.NoError(t, db.Omit("Embedding").Create(&thread).Error)
	report := BoardDailyReport{SemanticBoardID: board, PeriodDate: day, Status: "completed", Highlights: []byte("[]")}
	require.NoError(t, repo.SaveReport(&report, []DailyReportSection{{ClusterLabel: "old"}}, [][]DailyReportThread{{{Title: "B", StoryID: &story.ID, RelatedArticleIDs: mustStoryJSON([]uint{articleB})}}}))
	require.NoError(t, repo.RecomputeSignalsForStory(story.ID))
	heat, _, err := repo.ComputeBoardHeat(board, at)
	require.NoError(t, err)
	require.InDelta(t, 2, heat[story.ID], 1e-6)
	assertStoryParticipants(t, db, story.ID, 2)

	// Rebuild no longer covers B. No attribution pass is needed to retract it.
	require.NoError(t, repo.SaveReport(&report, nil, nil))
	var signals []StorySignal
	require.NoError(t, db.Where("story_id = ?", story.ID).Find(&signals).Error)
	require.Len(t, signals, 1)
	require.Equal(t, articleA, signals[0].ArticleID)
	assertStoryParticipants(t, db, story.ID, 1)
	heat, _, err = repo.ComputeBoardHeat(board, at)
	require.NoError(t, err)
	require.InDelta(t, 1, heat[story.ID], 1e-6)
	var stored BoardStory
	require.NoError(t, db.First(&stored, story.ID).Error)
	require.Equal(t, 1, stored.ThreadCount)
	require.Equal(t, 1, stored.ArticleCount)
}

// TestStoryHighlightSourceCountMatchesParticipantCount（fix-duplicate-article-heat
// 2.4）：同一篇内容被两个 feed 各抓一份 → 高亮的「来源数」与
// board_stories.participant_count 同值（都是 1，不是 2）——消费面不得出现
// 「热度按内容计、来源数按 feed 计」的混用。
func TestStoryHighlightSourceCountMatchesParticipantCount(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	board := seedTestBoard(t, db)
	day := NormalizeReportDate(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC))
	at := laneHeatAnchorEnd(day)
	reportID := seedTestReport(t, db, board, day)
	sectionID := seedTestSection(t, db, reportID, "facts")
	story := BoardStory{BoardID: board, Title: "dup", FirstSeenDate: day, LastSeenDate: day}
	require.NoError(t, db.Omit("Embedding", "SubjectEmbedding").Create(&story).Error)
	feedA := seedCoverageFeed(t, db, "highlight-dup-a", 60, "ok", &at)
	feedB := seedCoverageFeed(t, db, "highlight-dup-b", 60, "ok", &at)
	articleA := seedSignalArticleWithLink(t, db, feedA, "https://example.com/p/1?utm=x", at)
	articleB := seedSignalArticleWithLink(t, db, feedB, "https://example.com/p/1#comments", at)
	thread := DailyReportThread{
		ReportID: reportID, SectionID: sectionID, Title: "dup", Scope: ThreadScopeSingle,
		StoryID: &story.ID, TagIDs: []byte("[123]"), DisplayTier: ThreadDisplayTierBody,
		RelatedArticleIDs: mustStoryJSON([]uint{articleA, articleB}),
	}
	require.NoError(t, db.Omit("Embedding").Create(&thread).Error)
	require.NoError(t, repo.RecomputeSignalsForStory(story.ID))
	assertStoryParticipants(t, db, story.ID, 1)

	candidates, err := repo.ReportStoryCandidates(board, reportID, day)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, 1, candidates[0].ParticipantCount, "高亮来源数 = participant_count = 1")
	require.Equal(t, 1.0, candidates[0].StoryHeat, "热度只按 1 个参与者计")
}
