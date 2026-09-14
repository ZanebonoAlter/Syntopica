package core

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
	"syntopica-backend/internal/tagmanagement/repository"
)

// setupEdgeGCTestDB builds an isolated in-memory SQLite schema for the edge GC.
// SQLite (not testcontainers) keeps these pure-SQL-window cases runnable under
// -short; the edge GC only uses plain comparisons, no Postgres-specific SQL.
func setupEdgeGCTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:edgegc-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Feed{},
		&models.Article{},
		&models.TopicTag{},
		&models.ArticleTopicTag{},
		&models.TopicTagSemanticLabel{},
		&models.AISettings{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	repository.InitRepository(db)
	return db
}

// seedEdgeGCEdge creates an edge with an explicit created_at. GORM preserves a
// non-zero CreatedAt on insert, which is what makes the window boundary
// deterministic.
func seedEdgeGCEdge(t *testing.T, db *gorm.DB, articleID, tagID uint, createdAt time.Time) models.ArticleTopicTag {
	t.Helper()
	edge := models.ArticleTopicTag{
		ArticleID:  articleID,
		TopicTagID: tagID,
		Score:      0.7,
		Source:     "llm",
		CreatedAt:  createdAt,
	}
	require.NoError(t, db.Create(&edge).Error, "create edge")
	return edge
}

// seedArchivedArticle creates an article that has aged out of its feed's
// active window. Edge GC only reclaims edges of archived articles (review
// M5-B), so every deletion case needs the article archived.
func seedArchivedArticle(t *testing.T, db *gorm.DB, feedID uint) models.Article {
	t.Helper()
	article := seedArticle(t, db, feedID)
	require.NoError(t, db.Model(&models.Article{}).
		Where("id = ?", article.ID).
		Update("archived", true).Error, "archive article")
	article.Archived = true
	return article
}

func countEdgeGCEdges(t *testing.T, db *gorm.DB, tagID uint) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&models.ArticleTopicTag{}).Where("topic_tag_id = ?", tagID).Count(&count).Error)
	return count
}

func countEdgeGCTags(t *testing.T, db *gorm.DB, tagID uint) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&models.TopicTag{}).Where("id = ?", tagID).Count(&count).Error)
	return count
}

// startOfDayEdgeGC is the local-midnight anchor the edge GC cutoff is built
// from (calendar-day window, review H1).
func startOfDayEdgeGC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// Scenario: 超窗边被回收 — the expired edge of an ARCHIVED article goes away and
// the tag left without any edge is reclaimed by CleanupOrphanedTags.
func TestEdgeGCRemovesExpiredEdgesAndOrphanTags(t *testing.T) {
	db := setupEdgeGCTestDB(t)
	now := time.Now()
	feed := seedFeed(t, db)
	article := seedArchivedArticle(t, db, feed.ID)
	orphanTag := seedTag(t, db, "edge-gc-orphan", "llm")
	seedEdgeGCEdge(t, db, article.ID, orphanTag.ID, now.Add(-8*24*time.Hour))

	result, err := EdgeGC(context.Background(), EdgeGCRequest{RetentionDays: 7, Now: now})
	require.NoError(t, err)

	assert.EqualValues(t, 1, result.DeletedEdges)
	assert.Equal(t, 1, result.AffectedTags)
	assert.Equal(t, 1, result.OrphanedTags)
	assert.Equal(t, 7, result.RetentionDays)
	assert.Equal(t, startOfDayEdgeGC(now).AddDate(0, 0, -7), result.Cutoff, "cutoff is local midnight of today minus N days")
	assert.EqualValues(t, 0, countEdgeGCEdges(t, db, orphanTag.ID))
	assert.EqualValues(t, 0, countEdgeGCTags(t, db, orphanTag.ID), "orphan tag must be reclaimed")
}

// White-box matrix row: expired edge + tag with another in-window edge — the
// edge dies, the tag survives (mixed batch: one orphan, one survivor).
func TestEdgeGCKeepsTagWithRemainingEdge(t *testing.T) {
	db := setupEdgeGCTestDB(t)
	now := time.Now()
	feed := seedFeed(t, db)
	// One edge per (article, tag) pair — the unique index forbids duplicates,
	// so extra edges for the same tag come from extra articles.
	oldArticle := seedArchivedArticle(t, db, feed.ID)
	freshArticle := seedArchivedArticle(t, db, feed.ID)

	orphanTag := seedTag(t, db, "edge-gc-doomed", "llm")
	survivorTag := seedTag(t, db, "edge-gc-survivor", "llm")
	seedEdgeGCEdge(t, db, oldArticle.ID, orphanTag.ID, now.Add(-9*24*time.Hour))
	seedEdgeGCEdge(t, db, oldArticle.ID, survivorTag.ID, now.Add(-9*24*time.Hour))
	keptEdge := seedEdgeGCEdge(t, db, freshArticle.ID, survivorTag.ID, now.Add(-3*24*time.Hour))

	result, err := EdgeGC(context.Background(), EdgeGCRequest{RetentionDays: 7, Now: now})
	require.NoError(t, err)

	assert.EqualValues(t, 2, result.DeletedEdges)
	assert.Equal(t, 2, result.AffectedTags)
	assert.Equal(t, 1, result.OrphanedTags)
	assert.EqualValues(t, 0, countEdgeGCTags(t, db, orphanTag.ID))
	assert.EqualValues(t, 1, countEdgeGCTags(t, db, survivorTag.ID), "tag with a remaining edge must survive")
	assert.EqualValues(t, 1, countEdgeGCEdges(t, db, survivorTag.ID))

	var remaining models.ArticleTopicTag
	require.NoError(t, db.Where("topic_tag_id = ?", survivorTag.ID).First(&remaining).Error)
	assert.Equal(t, keptEdge.ID, remaining.ID)
}

// Scenario: 窗口内边保留供补档消费 + calendar-day boundary (review H1): the whole
// lower-bound day D = today-7d survives (any clock time on D), while D-1 is
// deleted — same window the rebuild guard and the backfill scan use.
func TestEdgeGCWindowBoundaryKeepsInWindowEdges(t *testing.T) {
	db := setupEdgeGCTestDB(t)
	now := time.Now()
	feed := seedFeed(t, db)
	tag := seedTag(t, db, "edge-gc-boundary", "llm")
	otherTag := seedTag(t, db, "edge-gc-anchor", "llm")

	// Distinct articles: one edge per (article, tag) pair.
	// cutoff = 00:00 of day D (D = today-7d); the window is [cutoff, now].
	// Every article is archived, which is the precondition for reclaiming.
	cutoff := startOfDayEdgeGC(now).AddDate(0, 0, -7)
	inWindowEarly := seedEdgeGCEdge(t, db, seedArchivedArticle(t, db, feed.ID).ID, tag.ID, cutoff.Add(time.Hour))
	inWindowSameClock := seedEdgeGCEdge(t, db, seedArchivedArticle(t, db, feed.ID).ID, tag.ID, now.AddDate(0, 0, -7))
	outOfWindowLate := seedEdgeGCEdge(t, db, seedArchivedArticle(t, db, feed.ID).ID, tag.ID, cutoff.Add(-time.Millisecond))
	outOfWindowSameClock := seedEdgeGCEdge(t, db, seedArchivedArticle(t, db, feed.ID).ID, tag.ID, now.Add(-8*24*time.Hour))
	fresh := seedEdgeGCEdge(t, db, seedArchivedArticle(t, db, feed.ID).ID, tag.ID, now.Add(-24*time.Hour))
	// Keep a second tag so the tag itself is not reclaimed and the edge set
	// stays inspectable.
	seedEdgeGCEdge(t, db, seedArchivedArticle(t, db, feed.ID).ID, otherTag.ID, now.Add(-8*24*time.Hour))

	result, err := EdgeGC(context.Background(), EdgeGCRequest{RetentionDays: 7, Now: now})
	require.NoError(t, err)

	// outOfWindowLate + outOfWindowSameClock + the anchor tag's expired edge.
	assert.EqualValues(t, 3, result.DeletedEdges, "only edges before the lower-bound day go")
	assert.EqualValues(t, 0, countEdgeGCEdges(t, db, otherTag.ID))

	var survivors []uint
	require.NoError(t, db.Model(&models.ArticleTopicTag{}).
		Where("topic_tag_id = ?", tag.ID).
		Order("id ASC").
		Pluck("id", &survivors).Error)
	assert.Equal(t, []uint{inWindowEarly.ID, inWindowSameClock.ID, fresh.ID}, survivors,
		"every edge created on day D or later must survive")
	assert.NotContains(t, survivors, outOfWindowLate.ID)
	assert.NotContains(t, survivors, outOfWindowSameClock.ID)
}

// Scenario: 配置非法回退默认 — missing / non-numeric / empty / 0 / negative all
// fall back to 7 without refusing to run.
func TestEdgeGCLoadTagEdgeRetentionDaysFallback(t *testing.T) {
	cases := []struct {
		name  string
		value *string
		want  int
	}{
		{name: "key missing", value: nil, want: DefaultTagEdgeRetentionDays},
		{name: "non numeric", value: strPtrEdgeGC("abc"), want: DefaultTagEdgeRetentionDays},
		{name: "empty", value: strPtrEdgeGC(""), want: DefaultTagEdgeRetentionDays},
		{name: "zero", value: strPtrEdgeGC("0"), want: DefaultTagEdgeRetentionDays},
		{name: "negative", value: strPtrEdgeGC("-3"), want: DefaultTagEdgeRetentionDays},
		{name: "valid override", value: strPtrEdgeGC("14"), want: 14},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupEdgeGCTestDB(t)
			if tc.value != nil {
				require.NoError(t, db.Create(&models.AISettings{
					Key:   TagEdgeRetentionDaysKey,
					Value: *tc.value,
				}).Error)
			}

			assert.Equal(t, tc.want, LoadTagEdgeRetentionDays(db))

			// The fallback must not stop the pass: an edge of an archived article
			// past the effective window still dies.
			now := time.Now()
			feed := seedFeed(t, db)
			article := seedArchivedArticle(t, db, feed.ID)
			tag := seedTag(t, db, "edge-gc-cfg-"+tc.name, "llm")
			seedEdgeGCEdge(t, db, article.ID, tag.ID, now.Add(-time.Duration(tc.want+1)*24*time.Hour))

			_, err := EdgeGC(context.Background(), EdgeGCRequest{
				RetentionDays: LoadTagEdgeRetentionDays(db),
				Now:           now,
			})
			require.NoError(t, err)
			assert.EqualValues(t, 0, countEdgeGCEdges(t, db, tag.ID))
		})
	}
}

// Idempotency: the second pass finds nothing left to reclaim.
func TestEdgeGCIdempotentSecondRunDeletesNothing(t *testing.T) {
	db := setupEdgeGCTestDB(t)
	now := time.Now()
	feed := seedFeed(t, db)
	article := seedArchivedArticle(t, db, feed.ID)
	doomed := seedTag(t, db, "edge-gc-idem-doomed", "llm")
	keeper := seedTag(t, db, "edge-gc-idem-keeper", "llm")
	seedEdgeGCEdge(t, db, article.ID, doomed.ID, now.Add(-30*24*time.Hour))
	seedEdgeGCEdge(t, db, article.ID, keeper.ID, now.Add(-time.Hour))

	first, err := EdgeGC(context.Background(), EdgeGCRequest{RetentionDays: 7, Now: now})
	require.NoError(t, err)
	assert.EqualValues(t, 1, first.DeletedEdges)
	assert.Equal(t, 1, first.OrphanedTags)

	second, err := EdgeGC(context.Background(), EdgeGCRequest{RetentionDays: 7, Now: now})
	require.NoError(t, err)
	assert.EqualValues(t, 0, second.DeletedEdges)
	assert.Equal(t, 0, second.AffectedTags)
	assert.Equal(t, 0, second.OrphanedTags)

	assert.EqualValues(t, 1, countEdgeGCEdges(t, db, keeper.ID))
	assert.EqualValues(t, 1, countEdgeGCTags(t, db, keeper.ID))
	assert.EqualValues(t, 0, countEdgeGCTags(t, db, doomed.ID))
}

func strPtrEdgeGC(s string) *string {
	return &s
}

// Scenario: 未归档文章的超窗边保留 (review M5-B) — the M5-B scope change. An edge
// past the window survives while its article is still unarchived, and the tag
// hanging only off that edge is not reclaimable either. The archived control
// tag proves the pass really ran (its expired edge dies) rather than no-oping.
func TestEdgeGCKeepsEdgesOfUnarchivedArticles(t *testing.T) {
	db := setupEdgeGCTestDB(t)
	now := time.Now()
	feed := seedFeed(t, db)

	// Live article: expired edge, but never archived → kept.
	liveArticle := seedArticle(t, db, feed.ID)
	liveTag := seedTag(t, db, "edge-gc-live", "llm")
	liveEdge := seedEdgeGCEdge(t, db, liveArticle.ID, liveTag.ID, now.Add(-8*24*time.Hour))

	// Archived control: same age, same window → reclaimed.
	archivedArticle := seedArchivedArticle(t, db, feed.ID)
	archivedTag := seedTag(t, db, "edge-gc-archived-control", "llm")
	seedEdgeGCEdge(t, db, archivedArticle.ID, archivedTag.ID, now.Add(-8*24*time.Hour))

	result, err := EdgeGC(context.Background(), EdgeGCRequest{RetentionDays: 7, Now: now})
	require.NoError(t, err)

	assert.Equal(t, 7, result.RetentionDays)
	assert.EqualValues(t, 1, result.DeletedEdges, "only the archived article's expired edge goes")
	assert.Equal(t, 1, result.AffectedTags)
	assert.Equal(t, 1, result.OrphanedTags)

	assert.EqualValues(t, 1, countEdgeGCEdges(t, db, liveTag.ID), "unarchived article's edge must survive the window")
	assert.EqualValues(t, 1, countEdgeGCTags(t, db, liveTag.ID), "tag kept alive by a surviving unarchived edge must not be reclaimed")

	var remaining models.ArticleTopicTag
	require.NoError(t, db.Where("topic_tag_id = ?", liveTag.ID).First(&remaining).Error)
	assert.Equal(t, liveEdge.ID, remaining.ID)

	assert.EqualValues(t, 0, countEdgeGCEdges(t, db, archivedTag.ID))
	assert.EqualValues(t, 0, countEdgeGCTags(t, db, archivedTag.ID))
}

// M5-B applies to the tag collection step as well: an edge whose article is
// unarchived must not be reported (or treated) as affected even when another
// archived article shares the tag — the tag is only "affected" when a live
// edge of its own is actually removed.
func TestEdgeGCUnarchivedEdgesDoNotAffectSharedTag(t *testing.T) {
	db := setupEdgeGCTestDB(t)
	now := time.Now()
	feed := seedFeed(t, db)
	sharedTag := seedTag(t, db, "edge-gc-shared", "llm")

	// Expired edges only, on one archived and one unarchived article.
	seedEdgeGCEdge(t, db, seedArchivedArticle(t, db, feed.ID).ID, sharedTag.ID, now.Add(-10*24*time.Hour))
	liveEdge := seedEdgeGCEdge(t, db, seedArticle(t, db, feed.ID).ID, sharedTag.ID, now.Add(-9*24*time.Hour))

	result, err := EdgeGC(context.Background(), EdgeGCRequest{RetentionDays: 7, Now: now})
	require.NoError(t, err)

	assert.EqualValues(t, 1, result.DeletedEdges)
	assert.Equal(t, 1, result.AffectedTags)
	assert.Equal(t, 0, result.OrphanedTags, "the surviving unarchived edge keeps the tag alive")
	assert.EqualValues(t, 1, countEdgeGCTags(t, db, sharedTag.ID))

	var remaining models.ArticleTopicTag
	require.NoError(t, db.Where("topic_tag_id = ?", sharedTag.ID).First(&remaining).Error)
	assert.Equal(t, liveEdge.ID, remaining.ID)
}
