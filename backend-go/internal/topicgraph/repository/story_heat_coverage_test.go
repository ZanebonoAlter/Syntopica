package repository

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// Task 4.4 — stale-feed coverage marking and recovery recompute, on
// in-memory SQLite (same style as story_heat_test.go).

func setupCoverageTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupStoryTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Feed{}))
	return db
}

func seedCoverageFeed(t *testing.T, db *gorm.DB, slug string, interval int, status string, lastRefresh *time.Time) uint {
	t.Helper()
	feed := models.Feed{
		Title:           "feed-" + slug,
		URL:             "https://example.com/feeds/" + slug,
		RefreshInterval: interval,
		RefreshStatus:   status,
		LastRefreshAt:   lastRefresh,
	}
	require.NoError(t, db.Create(&feed).Error)
	return feed.ID
}

func TestBoardHeatCoverageNeverRefreshedFeedIsIncomplete(t *testing.T) {
	db := setupCoverageTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
	story := seedStoryLine(t, db, 1, "a", "", "", now)
	fresh := seedCoverageFeed(t, db, "fresh", 60, "ok", ptrTime(now.Add(-30*time.Minute)))
	never := seedCoverageFeed(t, db, "never", 60, "idle", nil)
	seedHeatSignal(t, db, story, fresh, now.Add(-time.Hour))
	seedHeatSignal(t, db, story, never, now.Add(-time.Hour))
	coverage, err := repo.BoardHeatCoverage(1, now)
	require.NoError(t, err)
	assert.True(t, coverage.Incomplete)
	assert.Equal(t, []uint{never}, coverage.StaleFeedIDs)
	assert.True(t, coverage.CheckedAt.Equal(now))
}

func TestBoardHeatCoverageStaleThresholds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		interval int
		status   string
		age      time.Duration
		stale    bool
	}{
		// interval=10min → 3×10=30 < 90 → threshold is the 90min floor.
		{"interval10 within floor", 10, "ok", 89 * time.Minute, false},
		{"interval10 beyond floor", 10, "ok", 91 * time.Minute, true},
		// interval=60min → 3×60=180 > 90 → threshold is 180min.
		{"interval60 within 3x", 60, "ok", 179 * time.Minute, false},
		{"interval60 beyond 3x", 60, "ok", 181 * time.Minute, true},
		// Recent failures are not stale (avoid noise); beyond the floor they are.
		{"error recent not stale", 120, "error", 89 * time.Minute, false},
		{"error beyond floor stale", 120, "error", 91 * time.Minute, true},
		{"refreshing recent not stale", 120, "refreshing", 89 * time.Minute, false},
		{"refreshing stuck stale", 120, "refreshing", 91 * time.Minute, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupCoverageTestDB(t)
			repo := NewTopicGraphRepository(db)
			now := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
			story := seedStoryLine(t, db, 1, "a", "", "", now)
			feed := seedCoverageFeed(t, db, tc.name, tc.interval, tc.status, ptrTime(now.Add(-tc.age)))
			seedHeatSignal(t, db, story, feed, now.Add(-time.Hour))
			coverage, err := repo.BoardHeatCoverage(1, now)
			require.NoError(t, err)
			assert.Equal(t, tc.stale, coverage.Incomplete, "age %s interval %dm status %s", tc.age, tc.interval, tc.status)
			if tc.stale {
				assert.Equal(t, []uint{feed}, coverage.StaleFeedIDs)
			} else {
				assert.Empty(t, coverage.StaleFeedIDs)
			}
		})
	}
	// A board whose stories have no contributing feeds counts as complete.
	db := setupCoverageTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
	seedStoryLine(t, db, 1, "a", "", "", now)
	coverage, err := repo.BoardHeatCoverage(1, now)
	require.NoError(t, err)
	assert.False(t, coverage.Incomplete)
	assert.Empty(t, coverage.StaleFeedIDs)
}

func TestRecomputeHeatForBoardMatchesDirectComputeAfterRecovery(t *testing.T) {
	db := setupCoverageTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
	story := seedStoryLine(t, db, 1, "a", "", "", now)
	// interval=60 → threshold 180min；error at 120min 只踩 error 规则。
	feed := seedCoverageFeed(t, db, "stalled", 60, "error", ptrTime(now.Add(-120*time.Minute)))
	seedHeatSignal(t, db, story, feed, now.Add(-24*time.Hour))
	coverage, err := repo.BoardHeatCoverage(1, now)
	require.NoError(t, err)
	assert.True(t, coverage.Incomplete)
	assert.Equal(t, []uint{feed}, coverage.StaleFeedIDs)

	// Recovery: refresh succeeds and the catch-up backfills a newer signal.
	require.NoError(t, db.Model(&models.Feed{}).Where("id = ?", feed).
		Updates(map[string]any{"refresh_status": "ok", "last_refresh_at": now}).Error)
	seedHeatSignal(t, db, story, feed, now.Add(-12*time.Hour))
	coverage, err = repo.BoardHeatCoverage(1, now)
	require.NoError(t, err)
	assert.False(t, coverage.Incomplete)
	assert.Empty(t, coverage.StaleFeedIDs)

	heat, lanes, coverage, err := repo.RecomputeHeatForBoard(1, now, now)
	require.NoError(t, err)
	assert.False(t, coverage.Incomplete)
	directHeat, directLanes, err := repo.ComputeBoardHeat(1, now)
	require.NoError(t, err)
	require.Len(t, heat, len(directHeat))
	require.Len(t, lanes, len(directLanes))
	for id, want := range directHeat {
		assert.InDelta(t, want, heat[id], 1e-6)
	}
	for id, want := range directLanes {
		assert.InDelta(t, want, lanes[id], 1e-6)
	}
	// 补抓后最新证据生效：feed1 的 24h 前报道被 12h 前报道取代（同参与者不叠加）。
	assert.InDelta(t, math.Pow(0.5, 12.0/24), heat[story], 1e-6)
}

// Task 4.4 fix C — a laned story merged into a laneless survivor keeps its
// lane_heat via chain inheritance.
func TestLaneHeatInheritsLaneFromMergedFromStory(t *testing.T) {
	db := setupCoverageTestDB(t)
	repo := NewTopicGraphRepository(db)
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
	from := seedStoryLine(t, db, 1, "a", "", "", at)
	survivor := seedStoryLine(t, db, 1, "b", "", "", at)
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", from).Update("lane_id", 5).Error)
	seedHeatSignal(t, db, from, 1, at.Add(-24*time.Hour))
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", from).
		Updates(map[string]any{"merged_into": survivor, "status": StoryStatusMerged}).Error)
	heat, lanes, err := repo.ComputeBoardHeat(1, at)
	require.NoError(t, err)
	// A 的证据归 survivor 计热度，lane 5 继承自 A，lane_heat 不丢。
	assert.InDelta(t, 0.5, heat[survivor], 1e-6)
	assert.NotContains(t, heat, from)
	assert.InDelta(t, math.Pow(0.5, 24.0/72), lanes[5], 1e-6)
}

func ptrTime(t time.Time) *time.Time { return &t }
