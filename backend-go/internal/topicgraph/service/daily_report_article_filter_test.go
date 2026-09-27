package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/topicgraph/repository"
)

// unreachableGormDB returns a handle whose pool was never connected: the first
// query fails, which is how the write path's degrade branch is exercised without
// breaking the shared test container connection.
func unreachableGormDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 port=1 user=nobody password=nobody dbname=none sslmode=disable connect_timeout=1",
	}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Discard})
	require.NoError(t, err)
	return db
}

// TestApplyExistingArticleRefs is the pure core: references without a live row
// are dropped, the surviving order is untouched, a thread that loses every
// reference ends up with [] (a JSON null scalar would break
// jsonb_array_elements_text for every later query), any non-array value reads as
// "no references" and is normalized, and a value that cannot be parsed at all is
// kept instead of silently truncating the thread's sources.
func TestApplyExistingArticleRefs(t *testing.T) {
	threads := []repository.DailyReportThread{
		{Title: "keeps-order", RelatedArticleIDs: marshalJSONArray([]uint{11, 12, 13})},
		{Title: "loses-all", RelatedArticleIDs: marshalJSONArray([]uint{21})},
		{Title: "no-refs"},
		{Title: "json-null", RelatedArticleIDs: repository.JSON("null")},
		{Title: "json-object", RelatedArticleIDs: repository.JSON(`{"article":1}`)},
		{Title: "unparseable", RelatedArticleIDs: repository.JSON(`[1,`)},
	}
	existing := map[uint]bool{11: true, 13: true}

	dropped := applyExistingArticleRefs(threads, existing)

	require.Equal(t, 2, dropped)
	require.Equal(t, "[11,13]", string(threads[0].RelatedArticleIDs))
	require.Equal(t, "[]", string(threads[1].RelatedArticleIDs),
		"an emptied list must be stored as [] rather than null")
	require.Equal(t, "[]", string(threads[2].RelatedArticleIDs),
		"a missing value is normalized to [] so the column stays readable")
	require.Equal(t, "[]", string(threads[3].RelatedArticleIDs),
		"the JSON null scalar is normalized, not preserved")
	require.Equal(t, "[]", string(threads[4].RelatedArticleIDs),
		"a non-array value carries no references and is normalized")
	require.Equal(t, `[1,`, string(threads[5].RelatedArticleIDs),
		"an unparseable value is left as stored rather than dropping its sources")
}

// TestFilterVanishedArticleRefsDropsDeletedCandidates covers the TOCTOU window
// the write path closes: the candidate was collected while the article existed,
// and a delete path removed it before the report was written.
func TestFilterVanishedArticleRefsDropsDeletedCandidates(t *testing.T) {
	db := testutil.SetupTestDB(t)
	feed := models.Feed{Title: "filter-feed", URL: "https://example.com/filter-feed"}
	require.NoError(t, db.Create(&feed).Error)
	alive := models.Article{FeedID: feed.ID, Title: "alive", Link: "https://example.com/filter-feed/alive"}
	require.NoError(t, db.Create(&alive).Error)

	batches := [][]repository.DailyReportThread{
		{
			{Title: "partly-vanished", RelatedArticleIDs: marshalJSONArray([]uint{alive.ID, 999001})},
			{Title: "all-vanished", RelatedArticleIDs: marshalJSONArray([]uint{999002})},
		},
	}

	dropped := filterVanishedArticleRefs(db, batches)

	require.Equal(t, 2, dropped)
	require.Equal(t, string(marshalJSONArray([]uint{alive.ID})), string(batches[0][0].RelatedArticleIDs),
		"the surviving reference keeps its position")
	require.Equal(t, "[]", string(batches[0][1].RelatedArticleIDs),
		"a thread whose candidates are all gone still lands in the report")
}

// TestFilterVanishedArticleRefsCoversMaterializedBatches pins the review gap:
// watch materialization appends its own batches (Step 7.5) after an LLM round
// trip, so the guard must run on the final batch list rather than on the
// clustered threads alone — a materialized thread whose article disappeared
// during adjudication used to reach the database unverified.
func TestFilterVanishedArticleRefsCoversMaterializedBatches(t *testing.T) {
	db := testutil.SetupTestDB(t)
	feed := models.Feed{Title: "materialized-feed", URL: "https://example.com/materialized-feed"}
	require.NoError(t, db.Create(&feed).Error)
	alive := models.Article{FeedID: feed.ID, Title: "alive", Link: "https://example.com/materialized-feed/alive"}
	require.NoError(t, db.Create(&alive).Error)

	// A keyword-materialized section: its threads carry one article id each,
	// encoded by the same helper the materialization path uses.
	materialized := []repository.DailyReportThread{
		{Title: "keyword hit kept", RelatedArticleIDs: mustMarshalUintArray([]uint{alive.ID})},
		{Title: "keyword hit deleted", RelatedArticleIDs: mustMarshalUintArray([]uint{999003})},
	}
	batches := [][]repository.DailyReportThread{{}, materialized}

	dropped := filterVanishedArticleRefs(db, batches)

	require.Equal(t, 1, dropped)
	require.Equal(t, string(mustMarshalUintArray([]uint{alive.ID})), string(batches[1][0].RelatedArticleIDs))
	require.Equal(t, "[]", string(batches[1][1].RelatedArticleIDs),
		"the vanished materialized reference is dropped, not stored dangling")
}

// TestFilterVanishedArticleRefsKeepsCandidatesWhenCheckFails pins the degrade
// branch: an unusable existence probe must not truncate the report's sources.
func TestFilterVanishedArticleRefsKeepsCandidatesWhenCheckFails(t *testing.T) {
	db := unreachableGormDB(t)
	batches := [][]repository.DailyReportThread{
		{{Title: "degrade", RelatedArticleIDs: marshalJSONArray([]uint{1, 2})}},
	}

	dropped := filterVanishedArticleRefs(db, batches)

	require.Zero(t, dropped)
	require.Equal(t, "[1,2]", string(batches[0][0].RelatedArticleIDs),
		"references are kept as collected when existence cannot be resolved")
}

// TestFilterVanishedArticleRefsNoCandidatesSkipsProbe covers the empty-input
// branch: a report without thread references must not issue a probe at all (the
// unreachable handle proves no query is attempted).
func TestFilterVanishedArticleRefsNoCandidatesSkipsProbe(t *testing.T) {
	db := unreachableGormDB(t)

	require.Zero(t, filterVanishedArticleRefs(db, nil))
	require.Zero(t, filterVanishedArticleRefs(db, [][]repository.DailyReportThread{
		{{Title: "no-refs"}},
		{{Title: "empty-refs", RelatedArticleIDs: repository.JSON("[]")}},
	}))
}
