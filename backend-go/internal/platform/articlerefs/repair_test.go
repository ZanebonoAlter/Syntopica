package articlerefs_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"syntopica-backend/internal/platform/articlerefs"
	"syntopica-backend/internal/platform/testutil"
)

// TestNormalizeThreadRefsRewritesNonArrays covers both scalar shapes the schema
// accumulated — the JSON null scalar and SQL NULL — which otherwise make every
// jsonb_array_elements_text query raise SQLSTATE 22023.
func TestNormalizeThreadRefsRewritesNonArrays(t *testing.T) {
	db := testutil.SetupTestDB(t)
	alive := seedArticleRow(t, db, "normalize-alive")
	seedThreadRow(t, db, 1, "null")
	seedThreadNullRefs(t, db, 2)
	seedThreadRow(t, db, 3, fmt.Sprintf("[%d]", alive.ID))
	untouched := refsText(t, db, 3)

	normalized, err := articlerefs.NormalizeThreadRefs(db)
	require.NoError(t, err)
	require.EqualValues(t, 2, normalized)
	require.Equal(t, "array", refsType(t, db, 1))
	require.Equal(t, "[]", refsText(t, db, 1))
	require.Equal(t, "array", refsType(t, db, 2))
	require.Equal(t, "[]", refsText(t, db, 2))
	require.Equal(t, untouched, refsText(t, db, 3), "an array row is left alone")

	normalized, err = articlerefs.NormalizeThreadRefs(db)
	require.NoError(t, err)
	require.Zero(t, normalized, "normalization is idempotent")
}

// TestPruneDanglingRefsRemovesOrphanedReferences covers the one-shot repair: the
// walk drops every reference without a live article, normalizes scalar rows on
// the way, leaves clean rows byte-identical, and reports what it touched.
func TestPruneDanglingRefsRemovesOrphanedReferences(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aliveA := seedArticleRow(t, db, "repair-a")
	aliveB := seedArticleRow(t, db, "repair-b")

	seedThreadRow(t, db, 1, fmt.Sprintf("[%d, 999001, %d]", aliveA.ID, aliveB.ID))
	seedThreadRow(t, db, 2, "[999002]")
	seedThreadRow(t, db, 3, fmt.Sprintf("[%d]", aliveA.ID))
	seedThreadRow(t, db, 4, "null")
	clean := refsText(t, db, 3)

	rows, refs, err := articlerefs.PruneDanglingRefs(db, 0)
	require.NoError(t, err)
	require.EqualValues(t, 3, rows, "the two orphaned rows plus the scalar row")
	require.EqualValues(t, 2, refs, "one orphaned reference per affected array")
	require.Equal(t, []int64{int64(aliveA.ID), int64(aliveB.ID)}, refsIDs(t, db, 1), "order survives the drop")
	require.Empty(t, refsIDs(t, db, 2))
	require.Equal(t, "array", refsType(t, db, 2))
	require.Equal(t, "[]", refsText(t, db, 2))
	require.Equal(t, clean, refsText(t, db, 3), "a clean row must not be rewritten")
	require.Equal(t, "array", refsType(t, db, 4))
	require.Equal(t, "[]", refsText(t, db, 4))

	rows, refs, err = articlerefs.PruneDanglingRefs(db, 0)
	require.NoError(t, err)
	require.Zero(t, rows)
	require.Zero(t, refs, "the repaired data has nothing left to fix")
}

// TestPruneDanglingRefsBatchBoundaries covers the window cursor: a one-row batch
// must still reach every row (no infinite loop, no skipped window), and a
// non-positive batch must fall back to the default size instead of scanning
// nothing.
func TestPruneDanglingRefsBatchBoundaries(t *testing.T) {
	db := testutil.SetupTestDB(t)
	for i := uint(1); i <= 5; i++ {
		seedThreadRow(t, db, i, fmt.Sprintf("[%d]", 990000+i))
	}

	rows, refs, err := articlerefs.PruneDanglingRefs(db, 1)
	require.NoError(t, err)
	require.EqualValues(t, 5, rows, "a batch of 1 must still walk the whole table")
	require.EqualValues(t, 5, refs)
	for i := uint(1); i <= 5; i++ {
		require.Equal(t, "[]", refsText(t, db, i))
	}

	seedThreadRow(t, db, 6, "[990006]")
	seedThreadRow(t, db, 7, "[990007]")
	rows, refs, err = articlerefs.PruneDanglingRefs(db, 0)
	require.NoError(t, err)
	require.EqualValues(t, 2, rows, "batch <= 0 falls back to DefaultBatchSize")
	require.EqualValues(t, 2, refs)
}

// TestCountDanglingArticleRefs covers the read-only daily-report probe: zero on
// clean data, the exact reference count otherwise, and an error (never a silent
// zero) when the handle is unusable. Scope note: this is the counter itself — the
// job glue that consumes it (log-only, and a failure that must not fail the job)
// is covered by TestDailyReportJobSucceedsWhenDanglingRefProbeFails in
// internal/admin/scheduler, which is why no assertion here touches the job.
func TestCountDanglingArticleRefs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	alive := seedArticleRow(t, db, "count-alive")
	seedThreadRow(t, db, 1, fmt.Sprintf("[%d]", alive.ID))
	seedThreadRow(t, db, 2, "null")

	count, err := articlerefs.CountDanglingArticleRefs(db)
	require.NoError(t, err)
	require.Zero(t, count, "a scalar row contributes no references and must not raise")

	seedThreadRow(t, db, 3, fmt.Sprintf("[%d, 999003]", alive.ID))
	seedThreadRow(t, db, 4, "[999004, 999005]")

	count, err = articlerefs.CountDanglingArticleRefs(db)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)

	_, err = articlerefs.CountDanglingArticleRefs(nil)
	require.Error(t, err, "a nil handle must surface as an error, not as a count of zero")

	_, err = articlerefs.CountDanglingArticleRefs(unreachableRefDB(t))
	require.Error(t, err, "a failed probe must surface as an error so the job logs a warning")
}

// unreachableRefDB returns a handle whose pool was never connected: the probe
// fails instead of returning a misleading zero.
func unreachableRefDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 port=1 user=nobody password=nobody dbname=none sslmode=disable connect_timeout=1",
	}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Discard})
	require.NoError(t, err)
	return db
}
