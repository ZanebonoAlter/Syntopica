package repository

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/textutil"
)

func seedHeatSignal(t *testing.T, db *gorm.DB, story, feed uint, observed time.Time) uint {
	t.Helper()
	article := seedSignalArticle(t, db, feed, observed, &observed, false)
	require.NoError(t, db.Create(&StorySignal{
		StoryID: story, ArticleID: article, FeedID: feed,
		ParticipantKey: fmt.Sprintf("feed:%d", feed), Kind: StorySignalKindEditorial,
		ObservedAt: observed, ObservedAtSource: "pub_date",
	}).Error)
	return article
}

func TestStoryHeatHalfLifeWindowAndParticipantDedup(t *testing.T) {
	for _, tc := range []struct {
		hours float64
		want  float64
	}{{0, 1}, {24, 0.5}, {48, 0.25}, {49, 0}} {
		t.Run(fmt.Sprint(tc.hours), func(t *testing.T) {
			db := setupStoryTestDB(t)
			repo := NewTopicGraphRepository(db)
			at := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
			story := seedStoryLine(t, db, 1, "a", "", "", at)
			seedHeatSignal(t, db, story, 1, at.Add(-time.Duration(tc.hours)*time.Hour))
			heat, _, err := repo.ComputeBoardHeat(1, at)
			require.NoError(t, err)
			assert.InDelta(t, tc.want, heat[story], 1e-6)
		})
	}
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
	story := seedStoryLine(t, db, 1, "a", "", "", at)
	seedHeatSignal(t, db, story, 1, at.Add(-48*time.Hour))
	seedHeatSignal(t, db, story, 1, at.Add(-24*time.Hour))
	seedHeatSignal(t, db, story, 2, at)
	// Future reports cannot leak into historical heat.
	seedHeatSignal(t, db, story, 1, at.Add(time.Hour))
	archived := seedHeatSignal(t, db, story, 3, at)
	require.NoError(t, db.Model(&models.Article{}).Where("id = ?", archived).Update("archived", true).Error)
	heat, lanes, err := repo.ComputeBoardHeat(1, at)
	require.NoError(t, err)
	assert.InDelta(t, 2.5, heat[story], 1e-6)
	assert.Empty(t, lanes)
	again, _, err := repo.ComputeBoardHeat(1, at)
	require.NoError(t, err)
	assert.Equal(t, heat, again)
	other, otherLanes, err := repo.ComputeBoardHeat(99, at)
	require.NoError(t, err)
	assert.NotNil(t, other)
	assert.NotNil(t, otherLanes)
	assert.Empty(t, other)
}

func TestStoryHeatIncludesArchivedArticleEvidence(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
	story := seedStoryLine(t, db, 1, "archived-evidence", "", "", at)
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", story).Update("lane_id", 7).Error)
	article := seedHeatSignal(t, db, story, 1, at.Add(-24*time.Hour))
	require.NoError(t, db.Model(&models.Article{}).Where("id = ?", article).Update("archived", true).Error)

	heat, lanes, err := repo.ComputeBoardHeat(1, at)
	require.NoError(t, err)
	assert.InDelta(t, 0.5, heat[story], 1e-6)
	assert.InDelta(t, math.Pow(0.5, 24.0/72), lanes[7], 1e-6)
}

func TestLaneHeatIndependentWindowAndMergedSurvivor(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
	a := seedStoryLine(t, db, 1, "a", "", "", at)
	b := seedStoryLine(t, db, 1, "b", "", "", at)
	c := seedStoryLine(t, db, 1, "c", "", "", at)
	for _, story := range []uint{a, b, c} {
		require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", story).Update("lane_id", 7).Error)
	}
	seedHeatSignal(t, db, a, 1, at.Add(-72*time.Hour))
	seedHeatSignal(t, db, a, 1, at.Add(-96*time.Hour))
	seedHeatSignal(t, db, b, 1, at.Add(-72*time.Hour))
	seedHeatSignal(t, db, b, 2, at.Add(-14*24*time.Hour))
	seedHeatSignal(t, db, b, 3, at.Add(-14*24*time.Hour-time.Second))
	heat, lanes, err := repo.ComputeBoardHeat(1, at)
	require.NoError(t, err)
	assert.Empty(t, heat)
	assert.InDelta(t, 1+math.Pow(0.5, 14*24.0/72), lanes[7], 1e-6)

	// Source and survivor share feed 1; the newest evidence wins once.
	seedHeatSignal(t, db, a, 1, at.Add(-24*time.Hour))
	seedHeatSignal(t, db, b, 1, at.Add(-48*time.Hour))
	seedHeatSignal(t, db, b, 4, at)
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", a).
		Updates(map[string]any{"merged_into": b, "status": StoryStatusMerged, "lane_id": 8}).Error)
	heat, lanes, err = repo.ComputeBoardHeat(1, at)
	require.NoError(t, err)
	assert.InDelta(t, 1.5, heat[b], 1e-6)
	assert.NotContains(t, heat, a)
	assert.NotContains(t, lanes, uint(8))
	assert.InDelta(t, math.Pow(0.5, 24.0/72)+1+math.Pow(0.5, 14*24.0/72), lanes[7], 1e-6)

	// Legacy two-hop rows resolve to the final survivor without creating chains.
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", b).
		Updates(map[string]any{"merged_into": c, "status": StoryStatusMerged}).Error)
	heat, _, err = repo.ComputeBoardHeat(1, at)
	require.NoError(t, err)
	assert.InDelta(t, 1.5, heat[c], 1e-6)
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", c).
		Updates(map[string]any{"merged_into": a, "status": StoryStatusMerged}).Error)
	heat, _, err = repo.ComputeBoardHeat(1, at)
	require.NoError(t, err)
	assert.Empty(t, heat) // Corrupt cycles terminate and never count evidence.
}

func TestStoryHeatByDayMatchesDailyComputations(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	at := time.Date(2026, 11, 5, 23, 59, 59, 0, storyHeatZone)
	a := seedStoryLine(t, db, 1, "a", "", "", at)
	b := seedStoryLine(t, db, 1, "b", "", "", at)
	for _, story := range []uint{a, b} {
		for i := range 6 {
			seedHeatSignal(t, db, story, uint(i%2+1), at.Add(-time.Duration(i*24)*time.Hour))
		}
	}
	seedHeatSignal(t, db, a, 3, at.Add(-48*time.Hour))
	seedHeatSignal(t, db, a, 4, at.Add(time.Second))
	// Unsorted inputs in UTC must still use CST date keys and CST day ends.
	days := []time.Time{at.Add(-48 * time.Hour).UTC(), at.UTC(), at.Add(-72 * time.Hour).UTC(), at.Add(-24 * time.Hour).UTC()}
	totals, err := repo.ComputeBoardStoryHeatByDay(1, days)
	require.NoError(t, err)
	for _, day := range days {
		heat, _, err := repo.ComputeBoardHeat(1, day)
		require.NoError(t, err)
		var sum float64
		for _, value := range heat {
			sum += value
		}
		assert.InDelta(t, sum, totals[day.In(storyHeatZone).Format("2006-01-02")], 1e-6)
	}
	again, err := repo.ComputeBoardStoryHeatByDay(1, days)
	require.NoError(t, err)
	assert.Equal(t, totals, again)
	empty, err := repo.ComputeBoardStoryHeatByDay(1, nil)
	require.NoError(t, err)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}

// ── 参与者身份：内容身份优先、feed 兜底（fix-duplicate-article-heat 2.4）──

// seedHeatSignalWithLink 落一条带内容身份键的热度证据：content_key =
// 规范化链接（link 为空 → 空串 → 内容身份不可判定 → 退化为 feed 粒度）。
func seedHeatSignalWithLink(t *testing.T, db *gorm.DB, story, feed uint, link string, observed time.Time) uint {
	t.Helper()
	article := models.Article{FeedID: feed, Title: "evidence", Link: link, CreatedAt: observed, PubDate: &observed}
	require.NoError(t, db.Create(&article).Error)
	require.NoError(t, db.Create(&StorySignal{
		StoryID: story, ArticleID: article.ID, FeedID: feed,
		ParticipantKey: fmt.Sprintf("feed:%d", feed),
		ContentKey:     textutil.NormalizeArticleLink(link),
		Kind:           StorySignalKindEditorial,
		ObservedAt:     observed, ObservedAtSource: "pub_date",
	}).Error)
	return article.ID
}

// TestStoryHeatParticipantIdentityPrefersContent 覆盖 spec「参与者身份与去重」
// 的四个 Scenario（同链接多 feed 副本只算一个参与者 / 不同 feed 各计一次 /
// 链接缺失退化按 feed / 归档不影响参与者计入）：参与者去重键 =
// COALESCE(NULLIF(content_key,”), participant_key)，热度与 participant_count
// 同源。
func TestStoryHeatParticipantIdentityPrefersContent(t *testing.T) {
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
	// 12.5h 前的证据：单参与者热度 = 0.5^(12.5/24) ≈ 0.697（显示 0.70）。
	observed := at.Add(-12*time.Hour - 30*time.Minute)
	weight := math.Pow(0.5, 12.5/StoryHeatHalfLifeHours)

	t.Run("同链接多 feed 副本只算一个参与者", func(t *testing.T) {
		db := setupStoryTestDB(t)
		repo := NewTopicGraphRepository(db)
		story := seedStoryLine(t, db, 1, "dup", "", "", at)
		seedHeatSignalWithLink(t, db, story, 10, "https://example.com/p/1?utm=x", observed)
		seedHeatSignalWithLink(t, db, story, 20, "https://example.com/p/1#comments", observed)
		heat, _, err := repo.ComputeBoardHeat(1, at)
		require.NoError(t, err)
		assert.InDelta(t, weight, heat[story], 0.02, "同一篇内容只按 1 个参与者计（0.70）")
		assert.Greater(t, math.Abs(heat[story]-2*weight), 0.02, "不得按 2 个 feed 叠加（1.40）")
		require.NoError(t, repo.RecomputeBoardStoryCounts(story))
		assertStoryParticipants(t, db, story, 1)
	})

	t.Run("五个不同内容各计一次", func(t *testing.T) {
		db := setupStoryTestDB(t)
		repo := NewTopicGraphRepository(db)
		story := seedStoryLine(t, db, 1, "five", "", "", at)
		for i := range 5 {
			seedHeatSignalWithLink(t, db, story, uint(10+i), fmt.Sprintf("https://example.com/p/%d", i), observed)
		}
		heat, _, err := repo.ComputeBoardHeat(1, at)
		require.NoError(t, err)
		assert.InDelta(t, 5*weight, heat[story], 0.02, "五个独立内容 → 五个参与者")
		require.NoError(t, repo.RecomputeBoardStoryCounts(story))
		assertStoryParticipants(t, db, story, 5)
	})

	t.Run("同一 feed 多条不叠加（内容身份不可判定时退化按 feed）", func(t *testing.T) {
		db := setupStoryTestDB(t)
		repo := NewTopicGraphRepository(db)
		story := seedStoryLine(t, db, 1, "same-feed", "", "", at)
		seedHeatSignalWithLink(t, db, story, 10, "", observed)
		seedHeatSignalWithLink(t, db, story, 10, "", observed.Add(time.Hour))
		heat, _, err := repo.ComputeBoardHeat(1, at)
		require.NoError(t, err)
		// 同一 feed 的多条只占 1 个参与者键，热度取其中最新的一条证据（11.5h）。
		assert.InDelta(t, math.Pow(0.5, 11.5/StoryHeatHalfLifeHours), heat[story], 0.02, "同一 feed 多条只按 1 个参与者计")
		require.NoError(t, repo.RecomputeBoardStoryCounts(story))
		assertStoryParticipants(t, db, story, 1)
	})

	t.Run("链接缺失退化按 feed", func(t *testing.T) {
		db := setupStoryTestDB(t)
		repo := NewTopicGraphRepository(db)
		story := seedStoryLine(t, db, 1, "no-link", "", "", at)
		seedHeatSignalWithLink(t, db, story, 10, "", observed)
		seedHeatSignalWithLink(t, db, story, 20, "", observed)
		heat, _, err := repo.ComputeBoardHeat(1, at)
		require.NoError(t, err)
		assert.InDelta(t, 2*weight, heat[story], 0.02, "不同 feed 不因内容身份不可判定而合并")
		require.NoError(t, repo.RecomputeBoardStoryCounts(story))
		assertStoryParticipants(t, db, story, 2)
	})

	t.Run("空 link 不被同内容的非空副本吞并", func(t *testing.T) {
		db := setupStoryTestDB(t)
		repo := NewTopicGraphRepository(db)
		story := seedStoryLine(t, db, 1, "mixed", "", "", at)
		seedHeatSignalWithLink(t, db, story, 10, "", observed)
		seedHeatSignalWithLink(t, db, story, 20, "https://example.com/p/1", observed)
		heat, _, err := repo.ComputeBoardHeat(1, at)
		require.NoError(t, err)
		assert.InDelta(t, 2*weight, heat[story], 0.02, "空 link 单独按 feed 计")
		require.NoError(t, repo.RecomputeBoardStoryCounts(story))
		assertStoryParticipants(t, db, story, 2)
	})

	t.Run("归档文章照常计入", func(t *testing.T) {
		db := setupStoryTestDB(t)
		repo := NewTopicGraphRepository(db)
		story := seedStoryLine(t, db, 1, "archived", "", "", at)
		article := seedHeatSignalWithLink(t, db, story, 10, "https://example.com/p/1", observed)
		require.NoError(t, db.Model(&models.Article{}).Where("id = ?", article).Update("archived", true).Error)
		heat, _, err := repo.ComputeBoardHeat(1, at)
		require.NoError(t, err)
		assert.InDelta(t, weight, heat[story], 0.02, "归档只是 feed 窗口轮转，不减参与者")
		require.NoError(t, repo.RecomputeBoardStoryCounts(story))
		assertStoryParticipants(t, db, story, 1)
	})
}
