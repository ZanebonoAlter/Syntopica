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

// Scenario: 超窗边被回收 — the expired edge goes away and the tag left without
// any edge is reclaimed by CleanupOrphanedTags.
func TestEdgeGCRemovesExpiredEdgesAndOrphanTags(t *testing.T) {
	db := setupEdgeGCTestDB(t)
	now := time.Now()
	feed := seedFeed(t, db)
	article := seedArticle(t, db, feed.ID)
	orphanTag := seedTag(t, db, "edge-gc-orphan", "llm")
	seedEdgeGCEdge(t, db, article.ID, orphanTag.ID, now.Add(-8*24*time.Hour))

	result, err := EdgeGC(context.Background(), EdgeGCRequest{RetentionDays: 7, Now: now})
	require.NoError(t, err)

	assert.EqualValues(t, 1, result.DeletedEdges)
	assert.Equal(t, 1, result.AffectedTags)
	assert.Equal(t, 1, result.OrphanedTags)
	assert.Equal(t, 7, result.RetentionDays)
	assert.Equal(t, now.Add(-7*24*time.Hour), result.Cutoff)
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
	oldArticle := seedArticle(t, db, feed.ID)
	freshArticle := seedArticle(t, db, feed.ID)

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

// Scenario: 窗口内边保留供补档消费 + boundary: created_at == now-7d survives
// (strictly-less-than delete), created_at == now-7d-1ms is deleted, 8d is gone.
func TestEdgeGCWindowBoundaryKeepsInWindowEdges(t *testing.T) {
	db := setupEdgeGCTestDB(t)
	now := time.Now()
	feed := seedFeed(t, db)
	tag := seedTag(t, db, "edge-gc-boundary", "llm")
	otherTag := seedTag(t, db, "edge-gc-anchor", "llm")

	// Distinct articles: one edge per (article, tag) pair.
	cutoff := now.Add(-7 * 24 * time.Hour)
	exact := seedEdgeGCEdge(t, db, seedArticle(t, db, feed.ID).ID, tag.ID, cutoff)
	justOutside := seedEdgeGCEdge(t, db, seedArticle(t, db, feed.ID).ID, tag.ID, cutoff.Add(-time.Millisecond))
	wellOutside := seedEdgeGCEdge(t, db, seedArticle(t, db, feed.ID).ID, tag.ID, now.Add(-8*24*time.Hour))
	fresh := seedEdgeGCEdge(t, db, seedArticle(t, db, feed.ID).ID, tag.ID, now.Add(-24*time.Hour))
	// Keep a second tag so the tag itself is not reclaimed and the edge set
	// stays inspectable.
	seedEdgeGCEdge(t, db, seedArticle(t, db, feed.ID).ID, otherTag.ID, now.Add(-8*24*time.Hour))

	result, err := EdgeGC(context.Background(), EdgeGCRequest{RetentionDays: 7, Now: now})
	require.NoError(t, err)

	// justOutside + wellOutside + the anchor tag's expired edge.
	assert.EqualValues(t, 3, result.DeletedEdges, "only the strictly-older edges go")
	assert.EqualValues(t, 0, countEdgeGCEdges(t, db, otherTag.ID))

	var survivors []uint
	require.NoError(t, db.Model(&models.ArticleTopicTag{}).
		Where("topic_tag_id = ?", tag.ID).
		Order("id ASC").
		Pluck("id", &survivors).Error)
	assert.Equal(t, []uint{exact.ID, fresh.ID}, survivors)
	assert.NotContains(t, survivors, justOutside.ID)
	assert.NotContains(t, survivors, wellOutside.ID)
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

			// The fallback must not stop the pass: an edge past the effective
			// window still dies.
			now := time.Now()
			feed := seedFeed(t, db)
			article := seedArticle(t, db, feed.ID)
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
	article := seedArticle(t, db, feed.ID)
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
