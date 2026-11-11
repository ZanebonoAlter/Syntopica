package main

import (
	"context"
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

func TestRebuildStorySignalsRangeDryRunAndIdempotence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:rebuild-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.Article{}, &repository.BoardDailyReport{}, &repository.DailyReportThread{}, &repository.BoardStory{}, &repository.StorySignal{}))
	now, err := time.Parse(time.DateOnly, "2026-11-05")
	require.NoError(t, err)
	board := uint(1)
	opts, err := parseOptions([]string{"--board", "1", "--from", "2026-11-05", "--to", "2026-11-05", "--rebuild-signals", "--dry-run"}, now)
	require.NoError(t, err)
	require.True(t, opts.RebuildSignals)
	require.Equal(t, board, *opts.BoardID)
	// Story 1 has both in-range and older evidence; 2 is outside the date
	// range and 3 is in another board. Their deliberately stale counts stay put.
	for i := uint(1); i <= 3; i++ {
		story := repository.BoardStory{ID: i, BoardID: 1, Title: "test", FirstSeenDate: now, LastSeenDate: now, ThreadCount: 99, ParticipantCount: 99}
		report := repository.BoardDailyReport{ID: i, SemanticBoardID: 1, PeriodDate: now, Title: "test", Status: "completed"}
		if i == 2 {
			report.PeriodDate = now.AddDate(0, 0, -1)
		}
		if i == 3 {
			report.SemanticBoardID = 2
			story.BoardID = 2
		}
		require.NoError(t, db.Omit("embedding", "subject_embedding").Create(&story).Error)
		require.NoError(t, db.Create(&report).Error)
		article := models.Article{ID: i, FeedID: i, Title: "archived evidence", Archived: true, CreatedAt: now}
		require.NoError(t, db.Create(&article).Error)
		thread := repository.DailyReportThread{ID: i, ReportID: i, StoryID: &i, RelatedArticleIDs: repository.JSON(fmt.Sprintf("[%d]", i))}
		require.NoError(t, db.Omit("embedding").Create(&thread).Error)
	}
	one := uint(1)
	older := repository.DailyReportThread{ID: 4, ReportID: 2, StoryID: &one, RelatedArticleIDs: repository.JSON("[2]")}
	require.NoError(t, db.Omit("embedding").Create(&older).Error)
	stats, err := rebuildStorySignals(context.Background(), db, opts)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Selected)
	assert.Zero(t, stats.Rebuilt)
	var count int64
	require.NoError(t, db.Model(&repository.StorySignal{}).Count(&count).Error)
	assert.Zero(t, count, "dry-run must not write")
	opts.DryRun = false
	for range 2 {
		stats, err = rebuildStorySignals(context.Background(), db, opts)
		require.NoError(t, err)
		assert.Equal(t, 1, stats.Selected)
		assert.Equal(t, 1, stats.Rebuilt)
		assert.EqualValues(t, 2, stats.Signals)
		var stories []repository.BoardStory
		require.NoError(t, db.Order("id").Find(&stories).Error)
		assert.Equal(t, 2, stories[0].ParticipantCount)
		assert.Equal(t, 2, stories[0].ThreadCount)
		assert.Equal(t, 2, stories[0].ArticleCount)
		assert.Equal(t, 99, stories[1].ParticipantCount)
		assert.Equal(t, 99, stories[2].ParticipantCount)
		require.NoError(t, db.Model(&repository.StorySignal{}).Count(&count).Error)
		assert.EqualValues(t, 2, count)
	}
	var threads []repository.DailyReportThread
	require.NoError(t, db.Order("id").Find(&threads).Error)
	for i, thread := range threads {
		expected := uint(i + 1)
		if thread.ID == 4 {
			expected = 1
		}
		require.NotNil(t, thread.StoryID)
		assert.Equal(t, expected, *thread.StoryID, "rebuild never changes attribution")
	}
}
