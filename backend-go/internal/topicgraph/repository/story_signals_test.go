package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

func seedSignalArticle(t *testing.T, db *gorm.DB, feedID uint, created time.Time, pub *time.Time, archived bool) uint {
	t.Helper()
	article := models.Article{FeedID: feedID, Title: "evidence", CreatedAt: created, PubDate: pub, Archived: archived}
	require.NoError(t, db.Create(&article).Error)
	return article.ID
}

func assertStoryParticipants(t *testing.T, db *gorm.DB, storyID uint, count int) {
	t.Helper()
	var story BoardStory
	require.NoError(t, db.First(&story, storyID).Error)
	assert.Equal(t, count, story.ParticipantCount)
}

func TestStorySignalsRebuildTimeIdempotenceAndMigration(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
	pub := at.Add(-24 * time.Hour)
	a := seedStoryLine(t, db, 1, "a", "", "", at)
	b := seedStoryLine(t, db, 1, "b", "", "", at)
	published := seedSignalArticle(t, db, 10, at, &pub, false)
	fallback := seedSignalArticle(t, db, 10, at, nil, false)
	other := seedSignalArticle(t, db, 20, at, nil, false)
	archived := seedSignalArticle(t, db, 30, at, &pub, true)
	report := seedStoryReport(t, db, 1, at)
	section := seedStorySection(t, db, report, nil)
	thread := seedStoryThread(t, db, report, section, nil, []uint{published, published, fallback, other, archived}, false)
	require.NoError(t, repo.AttachThreadToBoardStory(report, thread, a, false))
	var signals []StorySignal
	require.NoError(t, db.Order("article_id").Find(&signals).Error)
	require.Len(t, signals, 4)
	assert.True(t, signals[0].ObservedAt.Equal(pub))
	assert.Equal(t, "pub_date", signals[0].ObservedAtSource)
	assert.True(t, signals[1].ObservedAt.Equal(at))
	assert.Equal(t, "fallback", signals[1].ObservedAtSource)
	assert.Equal(t, "feed:10", signals[0].ParticipantKey)
	assert.Equal(t, StorySignalKindEditorial, signals[0].Kind)
	assertStoryParticipants(t, db, a, 3)
	for range 2 {
		require.NoError(t, repo.RecomputeSignalsForStory(a))
	}
	var again []StorySignal
	require.NoError(t, db.Order("article_id").Find(&again).Error)
	assert.Equal(t, signals, again)

	// A manual reattribution rebuilds both ends, including an empty source.
	require.NoError(t, db.Model(&DailyReportThread{}).Where("id = ?", thread).Update("story_id", b).Error)
	require.NoError(t, repo.RecomputeSignalsForStory(a))
	require.NoError(t, repo.RecomputeSignalsForStory(b))
	var count int64
	require.NoError(t, db.Model(&StorySignal{}).Where("story_id = ?", a).Count(&count).Error)
	assert.Zero(t, count)
	assertStoryParticipants(t, db, a, 0)
	assertStoryParticipants(t, db, b, 3)

	// Feed-window rotation does not change reporting evidence or participants.
	require.NoError(t, db.Model(&models.Article{}).Where("id = ?", other).Update("archived", true).Error)
	require.NoError(t, repo.RecomputeBoardStoryCounts(b))
	assertStoryParticipants(t, db, b, 3)
	require.NoError(t, repo.RecomputeSignalsForStory(b))
	require.NoError(t, db.Model(&StorySignal{}).Where("story_id = ?", b).Count(&count).Error)
	assert.EqualValues(t, 4, count)
}

func TestStorySignalsMergePreservesSourceAndParticipants(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
	a := seedStoryLine(t, db, 1, "a", "", "", at)
	b := seedStoryLine(t, db, 1, "b", "", "", at)
	report := seedStoryReport(t, db, 1, at)
	section := seedStorySection(t, db, report, nil)
	for i, story := range []uint{a, b} {
		article := seedSignalArticle(t, db, uint(i+1), at, nil, false)
		thread := seedStoryThread(t, db, report, section, nil, []uint{article}, false)
		require.NoError(t, repo.AttachThreadToBoardStory(report, thread, story, false))
	}
	// A protected thread contributes a third feed without moving on merge.
	manualArticle := seedSignalArticle(t, db, 3, at, nil, false)
	manual := seedStoryThread(t, db, report, section, nil, []uint{manualArticle}, true)
	require.NoError(t, db.Model(&DailyReportThread{}).Where("id = ?", manual).Update("story_id", a).Error)
	require.NoError(t, repo.RecomputeSignalsForStory(a))
	survivor, err := repo.MergeBoardStories(a, b)
	require.NoError(t, err)
	assert.Equal(t, b, survivor)
	assertStoryParticipants(t, db, b, 3)
	var count int64
	require.NoError(t, db.Model(&StorySignal{}).Where("story_id = ?", a).Count(&count).Error)
	assert.EqualValues(t, 2, count)
	heat, _, err := repo.ComputeBoardHeat(1, at)
	require.NoError(t, err)
	assert.InDelta(t, 3, heat[b], 1e-6)
	assert.NotContains(t, heat, a)
}

// TestStorySignalsParticipantCountUsesContentIdentity（fix-duplicate-article-heat
// 2.4）：participant_count 按内容身份去重——同一篇内容被两个 feed 各抓一份只算
// 1 个参与者（spec「同链接多 feed 副本只算一个参与者」）；内容身份不可判定
// （link 为空）时退化为 feed 粒度，不同 feed 不合并。
func TestStorySignalsParticipantCountUsesContentIdentity(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	at := time.Date(2026, 11, 10, 12, 0, 0, 0, storyHeatZone)
	story := seedStoryLine(t, db, 1, "dup", "", "", at)
	// 同一篇内容的两个 feed 副本（只在 query / fragment 上不同）。
	feedA := seedSignalArticleWithLink(t, db, 10, "https://example.com/p/1?utm=x", at)
	feedB := seedSignalArticleWithLink(t, db, 20, "https://example.com/p/1#comments", at)
	report := seedStoryReport(t, db, 1, at)
	section := seedStorySection(t, db, report, nil)
	thread := seedStoryThread(t, db, report, section, nil, []uint{feedA, feedB}, false)
	require.NoError(t, repo.AttachThreadToBoardStory(report, thread, story, false))
	assertStoryParticipants(t, db, story, 1)

	// 追加一篇独立内容 → 2；再追加一篇空 link（退化按 feed）→ 3。
	other := seedSignalArticleWithLink(t, db, 30, "https://example.com/p/2", at)
	noLink := seedSignalArticleWithLink(t, db, 40, "", at)
	require.NoError(t, db.Model(&DailyReportThread{}).Where("id = ?", thread).
		Update("related_article_ids", mustStoryJSON([]uint{feedA, feedB, other, noLink})).Error)
	require.NoError(t, repo.RecomputeSignalsForStory(story))
	assertStoryParticipants(t, db, story, 3)
}
