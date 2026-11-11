package service

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/topicgraph/repository"
)

// Cross-period story dedup tests (add-event-story-heat tasks 6.1-6.3) on
// in-memory SQLite — same style as repository's story_repository_test.go.
// The verdicts under test are pure set arithmetic over related_article_ids +
// feeds; the pgvector paths are not involved.

var dedupNow = time.Date(2026, 11, 5, 8, 0, 0, 0, time.UTC)

func storyDedupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:story-dedup-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Article{},
		&repository.BoardDailyReport{},
		&repository.DailyReportSection{},
		&repository.DailyReportThread{},
		&repository.BoardStory{},
		&repository.StorySignal{},
	))
	original := repository.Repo
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() {
		repository.Repo = original
		if sqlDB, derr := db.DB(); derr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedDedupArticle(t *testing.T, db *gorm.DB, id, feedID uint) {
	t.Helper()
	pub := dedupNow
	require.NoError(t, db.Create(&models.Article{
		ID: id, FeedID: feedID, Title: fmt.Sprintf("dedup-article-%d", id),
		Link: fmt.Sprintf("https://example.com/dedup/%d", id), PubDate: &pub,
	}).Error)
}

func seedDedupStory(t *testing.T, db *gorm.DB, boardID uint) uint {
	t.Helper()
	story := repository.BoardStory{
		BoardID: boardID, Title: "dedup-story",
		FirstSeenDate: repository.NormalizeReportDate(dedupNow.AddDate(0, 0, -1)),
		LastSeenDate:  repository.NormalizeReportDate(dedupNow),
	}
	require.NoError(t, db.Create(&story).Error)
	return story.ID
}

func seedDedupReport(t *testing.T, db *gorm.DB, boardID uint, day time.Time) (reportID, sectionID uint) {
	t.Helper()
	report := repository.BoardDailyReport{
		SemanticBoardID: boardID, PeriodDate: repository.NormalizeReportDate(day),
		Title: "dedup-report", Status: "completed",
	}
	require.NoError(t, db.Create(&report).Error)
	section := repository.DailyReportSection{ReportID: report.ID, ClusterIndex: 0, ClusterLabel: "dedup-section"}
	require.NoError(t, db.Create(&section).Error)
	return report.ID, section.ID
}

func seedDedupThread(t *testing.T, db *gorm.DB, reportID, sectionID uint, storyID *uint, articleIDs []uint, scope string, isDevelopment, storyManual bool) uint {
	t.Helper()
	raw, err := json.Marshal(articleIDs)
	require.NoError(t, err)
	thread := repository.DailyReportThread{
		ReportID: reportID, SectionID: sectionID, Title: "dedup-thread",
		StoryID: storyID, Scope: scope, IsDevelopment: isDevelopment, StoryManual: storyManual,
		RelatedArticleIDs: repository.JSON(raw),
	}
	require.NoError(t, db.Create(&thread).Error)
	return thread.ID
}

func threadDisplayTier(t *testing.T, db *gorm.DB, threadID uint) string {
	t.Helper()
	var tier string
	require.NoError(t, db.Raw(`SELECT display_tier FROM daily_report_threads WHERE id = ?`, threadID).Scan(&tier).Error)
	return tier
}

func dedupArticleRange(from, to uint) []uint {
	ids := make([]uint, 0, to-from+1)
	for id := from; id <= to; id++ {
		ids = append(ids, id)
	}
	return ids
}

// seedDedupHistory builds the classic fixture: one story, a YESTERDAY report
// whose body thread carries articleIDs, and a TODAY report with one
// participant thread (today's articles). Articles from..to are pre-created.
func seedDedupHistory(t *testing.T, db *gorm.DB, boardID uint, historyIDs, todayIDs []uint, todayFeed uint) (storyID, todayReportID, todayThreadID uint) {
	t.Helper()
	for id := uint(1); id <= 30; id++ {
		seedDedupArticle(t, db, id, 100)
	}
	storyID = seedDedupStory(t, db, boardID)
	yesterdayReport, yesterdaySection := seedDedupReport(t, db, boardID, dedupNow.AddDate(0, 0, -1))
	seedDedupThread(t, db, yesterdayReport, yesterdaySection, &storyID, historyIDs, repository.ThreadScopeSingle, false, false)
	todayReport, todaySection := seedDedupReport(t, db, boardID, dedupNow)
	todayThread := seedDedupThread(t, db, todayReport, todaySection, &storyID, todayIDs, repository.ThreadScopeSingle, false, false)
	return storyID, todayReport, todayThread
}

// TestApplyStoryDedup_RestatementDegradesToFlash：重叠 0.8 + 无新 feed →
// flash（specs/daily-report-system「重述降级为快讯」）。
func TestApplyStoryDedup_RestatementDegradesToFlash(t *testing.T) {
	db := storyDedupTestDB(t)
	_, todayReport, todayThread := seedDedupHistory(t, db, 1, dedupArticleRange(1, 10), dedupArticleRange(3, 12), 100)

	result, err := applyStoryDedupForReport(todayReport, dedupNow)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Flash)
	assert.Equal(t, 0, result.Body)
	assert.Equal(t, 0, result.Skipped)
	assert.Equal(t, repository.ThreadDisplayTierFlash, threadDisplayTier(t, db, todayThread))
}

// TestApplyStoryDedup_NewFeedCountsAsDevelopment：重叠 0.7 但引入新 feed →
// body（specs「新来源判为进展」）。
func TestApplyStoryDedup_NewFeedCountsAsDevelopment(t *testing.T) {
	db := storyDedupTestDB(t)
	// articles 1..7 → feed 100（历史可见），8..10 → feed 200（今日新来源）。
	for id := uint(1); id <= 7; id++ {
		seedDedupArticle(t, db, id, 100)
	}
	for id := uint(8); id <= 10; id++ {
		seedDedupArticle(t, db, id, 200)
	}
	storyID := seedDedupStory(t, db, 1)
	yesterdayReport, yesterdaySection := seedDedupReport(t, db, 1, dedupNow.AddDate(0, 0, -1))
	seedDedupThread(t, db, yesterdayReport, yesterdaySection, &storyID, dedupArticleRange(1, 7), repository.ThreadScopeSingle, false, false)
	todayReport, todaySection := seedDedupReport(t, db, 1, dedupNow)
	todayThread := seedDedupThread(t, db, todayReport, todaySection, &storyID, dedupArticleRange(1, 10), repository.ThreadScopeSingle, false, false)

	result, err := applyStoryDedupForReport(todayReport, dedupNow)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Flash)
	assert.Equal(t, 1, result.Body)
	assert.Equal(t, repository.ThreadDisplayTierBody, threadDisplayTier(t, db, todayThread))
}

// TestApplyStoryDedup_DevelopmentStaysBody：is_development=true 优先级最高，
// 重叠 1.0 也进正文（specs「当事方新动作判为进展」）。
func TestApplyStoryDedup_DevelopmentStaysBody(t *testing.T) {
	db := storyDedupTestDB(t)
	articles := dedupArticleRange(1, 10)
	for _, id := range articles {
		seedDedupArticle(t, db, id, 100)
	}
	storyID := seedDedupStory(t, db, 1)
	yesterdayReport, yesterdaySection := seedDedupReport(t, db, 1, dedupNow.AddDate(0, 0, -1))
	seedDedupThread(t, db, yesterdayReport, yesterdaySection, &storyID, articles, repository.ThreadScopeSingle, false, false)
	todayReport, todaySection := seedDedupReport(t, db, 1, dedupNow)
	todayThread := seedDedupThread(t, db, todayReport, todaySection, &storyID, articles, repository.ThreadScopeSingle, true, false)

	result, err := applyStoryDedupForReport(todayReport, dedupNow)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Body)
	assert.Equal(t, 0, result.Flash)
	assert.Equal(t, repository.ThreadDisplayTierBody, threadDisplayTier(t, db, todayThread))
}

// TestApplyStoryDedup_UnattributedAndCompositeSkipped：story_id 为空 /
// composite / 人工指派一律跳过、保持 body，即使文章集合与历史完全重叠。
func TestApplyStoryDedup_UnattributedAndCompositeSkipped(t *testing.T) {
	db := storyDedupTestDB(t)
	articles := dedupArticleRange(1, 10)
	for _, id := range articles {
		seedDedupArticle(t, db, id, 100)
	}
	storyID := seedDedupStory(t, db, 1)
	yesterdayReport, yesterdaySection := seedDedupReport(t, db, 1, dedupNow.AddDate(0, 0, -1))
	seedDedupThread(t, db, yesterdayReport, yesterdaySection, &storyID, articles, repository.ThreadScopeSingle, false, false)
	todayReport, todaySection := seedDedupReport(t, db, 1, dedupNow)
	unassigned := seedDedupThread(t, db, todayReport, todaySection, nil, articles, repository.ThreadScopeSingle, false, false)
	composite := seedDedupThread(t, db, todayReport, todaySection, &storyID, articles, repository.ThreadScopeComposite, false, false)
	manual := seedDedupThread(t, db, todayReport, todaySection, &storyID, articles, repository.ThreadScopeSingle, false, true)

	result, err := applyStoryDedupForReport(todayReport, dedupNow)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Skipped, "unattributed/composite/manual threads never participate")
	assert.Equal(t, 0, result.Body)
	assert.Equal(t, 0, result.Flash)
	for _, threadID := range []uint{unassigned, composite, manual} {
		assert.Equal(t, repository.ThreadDisplayTierBody, threadDisplayTier(t, db, threadID))
	}
}

// TestApplyStoryDedup_DowngradeKeepsHeatIntact：降级只改展示层——dedup 前后
// 同 story 热度一致（1e-6）且 story_signals 行数不变（specs「降级不改变
// 热量」红线）。
func TestApplyStoryDedup_DowngradeKeepsHeatIntact(t *testing.T) {
	db := storyDedupTestDB(t)
	storyID, todayReport, todayThread := seedDedupHistory(t, db, 1, dedupArticleRange(1, 10), dedupArticleRange(3, 12), 100)

	// 热度证据：3 个参与者（feed 101/102/103）今天各报过一次。
	for _, feed := range []uint{101, 102, 103} {
		seedDedupArticle(t, db, 20+feed, feed)
		require.NoError(t, db.Create(&repository.StorySignal{
			StoryID: storyID, ArticleID: 20 + feed, FeedID: feed,
			ParticipantKey: fmt.Sprintf("feed:%d", feed), Kind: repository.StorySignalKindEditorial,
			ObservedAt: dedupNow.Add(-2 * time.Hour), ObservedAtSource: "pub_date",
		}).Error)
	}
	at := dedupNow
	var signalsBefore int64
	require.NoError(t, db.Model(&repository.StorySignal{}).Where("story_id = ?", storyID).Count(&signalsBefore).Error)
	heatBefore, _, err := repository.Repo.ComputeBoardHeat(1, at)
	require.NoError(t, err)
	require.NotEmpty(t, heatBefore[storyID], "fixture must produce non-zero heat")

	result, err := applyStoryDedupForReport(todayReport, dedupNow)
	require.NoError(t, err)
	require.Equal(t, 1, result.Flash, "fixture thread must be downgraded")
	require.Equal(t, repository.ThreadDisplayTierFlash, threadDisplayTier(t, db, todayThread))

	heatAfter, _, err := repository.Repo.ComputeBoardHeat(1, at)
	require.NoError(t, err)
	assert.InDelta(t, heatBefore[storyID], heatAfter[storyID], 1e-6, "heat must be identical across the downgrade")
	var signalsAfter int64
	require.NoError(t, db.Model(&repository.StorySignal{}).Where("story_id = ?", storyID).Count(&signalsAfter).Error)
	assert.Equal(t, signalsBefore, signalsAfter, "no signal row may be created or dropped by dedup")
}

// TestApplyStoryDedup_Idempotent：连跑两次结果一致、落库 tier 不再变化。
func TestApplyStoryDedup_Idempotent(t *testing.T) {
	db := storyDedupTestDB(t)
	_, todayReport, todayThread := seedDedupHistory(t, db, 1, dedupArticleRange(1, 10), dedupArticleRange(3, 12), 100)

	first, err := applyStoryDedupForReport(todayReport, dedupNow)
	require.NoError(t, err)
	second, err := applyStoryDedupForReport(todayReport, dedupNow)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, repository.ThreadDisplayTierFlash, threadDisplayTier(t, db, todayThread))
}

// TestApplyStoryDedup_FlashRestoredWhenConditionsChange：已 flash 的线程在
// 新 feed 补入后重算恢复 body（幂等语义的一半：按当前集合重算，不是单调
// 降级）。
func TestApplyStoryDedup_FlashRestoredWhenConditionsChange(t *testing.T) {
	db := storyDedupTestDB(t)
	_, todayReport, todayThread := seedDedupHistory(t, db, 1, dedupArticleRange(1, 10), dedupArticleRange(3, 12), 100)

	first, err := applyStoryDedupForReport(todayReport, dedupNow)
	require.NoError(t, err)
	require.Equal(t, 1, first.Flash)

	// 新来源补入：article 31（feed 300）加入今日集合 → overlap 8/11 仍 ≥0.6
	// 但出现新 feed → 进展。
	seedDedupArticle(t, db, 31, 300)
	var thread repository.DailyReportThread
	require.NoError(t, db.First(&thread, todayThread).Error)
	raw, err := json.Marshal(append(dedupArticleRange(3, 12), 31))
	require.NoError(t, err)
	require.NoError(t, db.Model(&thread).Update("related_article_ids", repository.JSON(raw)).Error)

	second, err := applyStoryDedupForReport(todayReport, dedupNow)
	require.NoError(t, err)
	assert.Equal(t, 1, second.Body)
	assert.Equal(t, 0, second.Flash)
	assert.Equal(t, repository.ThreadDisplayTierBody, threadDisplayTier(t, db, todayThread))
}

// TestApplyStoryDedup_HistoryWindowIsExactlySevenDays（review R-2）：spec
// 「最近 7 天（含当天，不含当前 report）」= day-6..day 恰好 7 个日历日
// ——day-6 的 body 历史参与去重（完全重叠 → flash），day-7 的历史在窗口
// 外不参与（无历史证据不降级，宁进正文不误降级）。
func TestApplyStoryDedup_HistoryWindowIsExactlySevenDays(t *testing.T) {
	seedWindowFixture := func(daysAgo int) (*gorm.DB, uint, uint) {
		db := storyDedupTestDB(t)
		articles := dedupArticleRange(1, 10)
		for _, id := range articles {
			seedDedupArticle(t, db, id, 100)
		}
		storyID := seedDedupStory(t, db, 1)
		histReport, histSection := seedDedupReport(t, db, 1, dedupNow.AddDate(0, 0, -daysAgo))
		seedDedupThread(t, db, histReport, histSection, &storyID, articles, repository.ThreadScopeSingle, false, false)
		todayReport, todaySection := seedDedupReport(t, db, 1, dedupNow)
		todayThread := seedDedupThread(t, db, todayReport, todaySection, &storyID, articles, repository.ThreadScopeSingle, false, false)
		return db, todayReport, todayThread
	}

	t.Run("day-6 history participates", func(t *testing.T) {
		db, todayReport, todayThread := seedWindowFixture(6)
		result, err := applyStoryDedupForReport(todayReport, dedupNow)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Flash, "day-6 在 7 天窗口（day-6..day）内：完全重叠 → flash")
		assert.Equal(t, repository.ThreadDisplayTierFlash, threadDisplayTier(t, db, todayThread))
	})

	t.Run("day-7 history is outside the window", func(t *testing.T) {
		db, todayReport, todayThread := seedWindowFixture(7)
		result, err := applyStoryDedupForReport(todayReport, dedupNow)
		require.NoError(t, err)
		assert.Equal(t, 1, result.Body, "day-7 在 7 天窗口外：历史不参与，无证据不降级")
		assert.Equal(t, 0, result.Flash)
		assert.Equal(t, repository.ThreadDisplayTierBody, threadDisplayTier(t, db, todayThread))
	})
}

// TestGetReportByID_ReturnsOnlyBodyThreads：读路径只回 body 线程；flash/
// hidden 落库仍在（直接 DB 可见，未来快讯 UI 可用）。
func TestGetReportByID_ReturnsOnlyBodyThreads(t *testing.T) {
	db := storyDedupTestDB(t)
	reportID, sectionID := seedDedupReport(t, db, 1, dedupNow)
	body := seedDedupThread(t, db, reportID, sectionID, nil, dedupArticleRange(1, 3), repository.ThreadScopeSingle, false, false)
	flash := seedDedupThread(t, db, reportID, sectionID, nil, dedupArticleRange(4, 6), repository.ThreadScopeSingle, false, false)
	hidden := seedDedupThread(t, db, reportID, sectionID, nil, dedupArticleRange(7, 9), repository.ThreadScopeSingle, false, false)
	require.NoError(t, db.Exec(`UPDATE daily_report_threads SET display_tier = ? WHERE id = ?`, repository.ThreadDisplayTierFlash, flash).Error)
	require.NoError(t, db.Exec(`UPDATE daily_report_threads SET display_tier = ? WHERE id = ?`, repository.ThreadDisplayTierHidden, hidden).Error)

	report, err := repository.Repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, report.Sections, 1)
	threads := report.Sections[0].Threads
	require.Len(t, threads, 1, "only the body thread renders")
	assert.Equal(t, body, threads[0].ID)

	var total int64
	require.NoError(t, db.Model(&repository.DailyReportThread{}).Where("report_id = ?", reportID).Count(&total).Error)
	assert.Equal(t, int64(3), total, "flash/hidden rows stay in the table — direct reads still see them")
}
