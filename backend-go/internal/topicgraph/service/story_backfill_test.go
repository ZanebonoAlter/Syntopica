package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/topicgraph/repository"
)

func setupStoryBackfillDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:backfill-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TopicTag{}, &models.Article{},
		&repository.BoardDailyReport{}, &repository.DailyReportSection{},
		&repository.DailyReportThread{}, &repository.BoardStory{}, &repository.StorySignal{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	oldDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = oldDB
		_ = sqlDB.Close()
	})
	return db
}

func backfillTestDay(t *testing.T, date string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	day, err := time.ParseInLocation(time.DateOnly, date, loc)
	require.NoError(t, err)
	return day
}

func TestBackfillDryRunVerdicts(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		ids                       []uint
		created, attached, merged int
	}{
		{"create", nil, 1, 0, 0},
		{"attach", []uint{101}, 0, 1, 0},
		{"merge-and-attach", []uint{101, 102}, 0, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, _ := storyAttributeFixture(t)
			for _, id := range tc.ids {
				store.candidates = append(store.candidates, storyAttributeCandidate(id))
			}
			stats, err := attributeStoriesForReportWithStats(context.Background(), store, 7, 21, storyAttributeDay(), true)
			require.NoError(t, err)
			require.Equal(t, tc.created, stats.Created)
			require.Equal(t, tc.attached, stats.Attached)
			require.Equal(t, tc.merged, stats.Merged)
			require.Zero(t, stats.Failed)
			assertStoryAttributeNoWrites(t, store)
			require.Empty(t, store.trusted, "dry-run must not persist fingerprint trust either")
		})
	}
}

func TestBackfillSkipsAndFailures(t *testing.T) {
	store, _ := storyAttributeFixture(t)
	thread := store.threads[0]
	assigned := thread
	id := uint(9)
	assigned.StoryID = &id
	manual := thread
	manual.StoryManual = true
	composite := thread
	composite.Scope = "composite"
	store.threads = []repository.DailyReportThread{assigned, manual, composite, thread}
	store.recallErr = errors.New("recall unavailable")
	stats, err := attributeStoriesForReportWithStats(context.Background(), store, 7, 21, storyAttributeDay(), true)
	require.NoError(t, err)
	require.Equal(t, 4, stats.Threads)
	require.Equal(t, 3, stats.Skipped)
	require.Equal(t, 1, stats.Failed)
	require.Zero(t, stats.Created+stats.Attached+stats.Merged)
	require.Empty(t, store.created)
	require.Empty(t, store.attached)
	require.Empty(t, store.merged)
	require.Equal(t, &id, store.threads[0].StoryID)
}

func TestBackfillCompositeTagsAndAttachRace(t *testing.T) {
	store, _ := storyAttributeFixture(t)
	store.tags = append(store.tags, models.TopicTag{ID: 12, Category: models.TagCategoryEvent})
	stats, err := attributeStoriesForReportWithStats(context.Background(), store, 7, 21, storyAttributeDay(), true)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Skipped)
	assertStoryAttributeNoWrites(t, store)
	store.tags = store.tags[:1]
	store.candidates = []repository.StoryCandidate{storyAttributeCandidate(101)}
	store.attachErr = repository.ErrStoryThreadNotAttachable
	stats, err = attributeStoriesForReportWithStats(context.Background(), store, 7, 21, storyAttributeDay(), false)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Skipped)
	require.Zero(t, stats.Attached+stats.Failed)
}

func TestBackfillSQLiteDryRunAndIdempotency(t *testing.T) {
	db := setupStoryBackfillDB(t)
	_, _ = storyAttributeFixture(t) // disable all real AI calls
	storyStoreForTest = nil         // exercise real portable repository persistence
	day := backfillTestDay(t, "2026-11-05")
	report := repository.BoardDailyReport{SemanticBoardID: 7, PeriodDate: day}
	require.NoError(t, db.Create(&report).Error)
	section := repository.DailyReportSection{ReportID: report.ID}
	require.NoError(t, db.Omit("embedding").Create(&section).Error)
	article := models.Article{ID: 101, FeedID: 1, Title: "evidence", Link: "https://example.test/backfill", PubDate: &day}
	require.NoError(t, db.Create(&article).Error)
	threads := []repository.DailyReportThread{
		{ReportID: report.ID, SectionID: section.ID, Title: "single", RelatedArticleIDs: repository.JSON(`[101]`), DisplayTier: "flash"},
		{ReportID: report.ID, SectionID: section.ID, Title: "manual", StoryManual: true},
		{ReportID: report.ID, SectionID: section.ID, Title: "composite", Scope: "composite"},
	}
	require.NoError(t, db.Omit("embedding").Create(&threads).Error)
	opts := BackfillOptions{From: day, To: day, DryRun: true}
	preview, err := PreviewStoryBackfill(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, 1, preview.Boards)
	require.Equal(t, 1, preview.Reports)
	for i := 0; i < 2; i++ {
		stats, err := BackfillStories(context.Background(), opts)
		require.NoError(t, err)
		require.Equal(t, BackfillStats{Boards: 1, Reports: 1, Threads: 3, Created: 1, Skipped: 2, DryRun: true}, stats)
		for _, model := range []any{&repository.BoardStory{}, &repository.StorySignal{}} {
			var count int64
			require.NoError(t, db.Model(model).Count(&count).Error)
			require.Zero(t, count)
		}
	}
	opts.DryRun = false
	stats, err := BackfillStories(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Created)
	require.Zero(t, stats.Failed)
	stats, err = BackfillStories(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, BackfillStats{Boards: 1, Reports: 1, Threads: 3, Skipped: 3}, stats)
	for _, model := range []any{&repository.BoardStory{}, &repository.StorySignal{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}
	var after []repository.DailyReportThread
	require.NoError(t, db.Order("id").Find(&after).Error)
	require.NotNil(t, after[0].StoryID)
	require.Equal(t, "flash", after[0].DisplayTier, "backfill must not run display dedup")
	require.True(t, after[1].StoryManual)
	require.Nil(t, after[1].StoryID)
	require.Nil(t, after[2].StoryID)
}

func TestBackfillBoardDateOrderBoundariesAndFailureContinuation(t *testing.T) {
	db := setupStoryBackfillDB(t)
	store, _ := storyAttributeFixture(t)
	from, to := backfillTestDay(t, "2026-11-01"), backfillTestDay(t, "2026-11-03")
	for _, report := range []repository.BoardDailyReport{
		{ID: 3, SemanticBoardID: 7, PeriodDate: to},
		{ID: 2, SemanticBoardID: 7, PeriodDate: from.AddDate(0, 0, 1)},
		{ID: 1, SemanticBoardID: 7, PeriodDate: from},
		{ID: 4, SemanticBoardID: 8, PeriodDate: from},
		{ID: 5, SemanticBoardID: 8, PeriodDate: to.AddDate(0, 0, 1)},
		{ID: 6, SemanticBoardID: 7, PeriodDate: from.AddDate(0, 0, -1)},
	} {
		require.NoError(t, db.Create(&report).Error)
	}
	opts := BackfillOptions{From: from, To: to, DryRun: true}
	preview, err := PreviewStoryBackfill(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, 2, preview.Boards)
	require.Equal(t, 4, preview.Reports)
	stats, err := BackfillStories(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, []uint{1, 2, 3, 4}, store.listReports)
	require.Equal(t, 4, stats.Created)
	assertStoryAttributeNoWrites(t, store)
	store.listReports = nil
	store.listErr = errors.New("load failed")
	stats, err = BackfillStories(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, 4, stats.Failed)
	require.Equal(t, []uint{1, 2, 3, 4}, store.listReports, "failed dates must not stop later boards")
	boardID := uint(8)
	opts.BoardID = &boardID
	store.listReports = nil
	store.listErr = nil
	_, err = BackfillStories(context.Background(), opts)
	require.NoError(t, err)
	require.Equal(t, []uint{4}, store.listReports)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = BackfillStories(ctx, opts)
	require.ErrorIs(t, err, context.Canceled)
	opts.From, opts.To = to, from
	_, err = BackfillStories(context.Background(), opts)
	require.Error(t, err)
}
