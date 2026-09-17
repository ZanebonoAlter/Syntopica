package articlerefs_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/articlerefs"
	"syntopica-backend/internal/platform/testutil"
)

// TestRewireArticleRefsPointsReferencesAtKeeper covers the duplicate-merge case:
// the loser copy disappears, so every thread citing it must cite the keeper
// instead, in place and without duplicating a keeper that was already there.
func TestRewireArticleRefsPointsReferencesAtKeeper(t *testing.T) {
	db := testutil.SetupTestDB(t)
	keeper := seedArticleRow(t, db, "rewire-keeper")
	loser := seedArticleRow(t, db, "rewire-loser")
	other := seedArticleRow(t, db, "rewire-other")

	// Loser cited between two foreign references: the keeper takes its slot.
	seedThreadRow(t, db, 1, fmt.Sprintf("[%d, %d, 4001]", other.ID, loser.ID))
	// Loser cited first and the keeper already present later: only dedupe.
	seedThreadRow(t, db, 2, fmt.Sprintf("[%d, %d, 4002]", loser.ID, keeper.ID))
	// No loser reference at all: must not be loaded, let alone written.
	seedThreadRow(t, db, 3, fmt.Sprintf("[%d]", keeper.ID))

	updated, err := articlerefs.RewireArticleRefs(db, loser.ID, keeper.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, updated, "only the rows citing the loser are rewritten")
	require.Equal(t, []int64{int64(other.ID), int64(keeper.ID), 4001}, refsIDs(t, db, 1))
	require.Equal(t, []int64{int64(keeper.ID), 4002}, refsIDs(t, db, 2),
		"the keeper keeps its first occurrence position, the duplicate is dropped")
	require.Equal(t, fmt.Sprintf("[%d]", keeper.ID), refsText(t, db, 3))
}

// TestRewireArticleRefsIsIdempotentAndNoOp covers the no-match branches: an id
// nobody references, an identical pair, and a second run after the first one
// already rewrote everything. All of them must report zero writes.
func TestRewireArticleRefsIsIdempotentAndNoOp(t *testing.T) {
	db := testutil.SetupTestDB(t)
	keeper := seedArticleRow(t, db, "idem-keeper")
	loser := seedArticleRow(t, db, "idem-loser")
	unreferenced := seedArticleRow(t, db, "idem-unreferenced")

	seedThreadRow(t, db, 1, fmt.Sprintf("[%d, %d]", loser.ID, keeper.ID))

	updated, err := articlerefs.RewireArticleRefs(db, unreferenced.ID, keeper.ID)
	require.NoError(t, err)
	require.Zero(t, updated, "an unreferenced loser produces no UPDATE")

	updated, err = articlerefs.RewireArticleRefs(db, loser.ID, loser.ID)
	require.NoError(t, err)
	require.Zero(t, updated, "an identical pair is a no-op by definition")

	updated, err = articlerefs.RewireArticleRefs(db, loser.ID, keeper.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)

	before := refsText(t, db, 1)
	updated, err = articlerefs.RewireArticleRefs(db, loser.ID, keeper.ID)
	require.NoError(t, err)
	require.Zero(t, updated, "second run has nothing left to rewrite")
	require.Equal(t, before, refsText(t, db, 1))
}

// TestRewireArticleRefsPreservesOrderOnLongArrays guards the reader contract:
// the frontend slices the first ten references, so order must survive a rewrite
// even when the duplicate sits deep in the array.
func TestRewireArticleRefsPreservesOrderOnLongArrays(t *testing.T) {
	db := testutil.SetupTestDB(t)
	keeper := seedArticleRow(t, db, "long-keeper")
	loser := seedArticleRow(t, db, "long-loser")

	refs := fmt.Sprintf("[%d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d]",
		101, 102, 103, loser.ID, 104, 105, 106, 107, 108, 109, 110, 111)
	seedThreadRow(t, db, 1, refs)

	updated, err := articlerefs.RewireArticleRefs(db, loser.ID, keeper.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
	require.Equal(t,
		[]int64{101, 102, 103, int64(keeper.ID), 104, 105, 106, 107, 108, 109, 110, 111},
		refsIDs(t, db, 1))
}

// TestPruneArticleRefsDropsDeletedIDs covers the feed-deletion case: the article
// rows are gone for good, so the references are dropped — and an array left with
// nothing must be [] rather than a JSON null.
func TestPruneArticleRefsDropsDeletedIDs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	keepA := seedArticleRow(t, db, "prune-a")
	keepB := seedArticleRow(t, db, "prune-b")
	removed := seedArticleRow(t, db, "prune-removed")

	seedThreadRow(t, db, 1, fmt.Sprintf("[%d, %d, %d]", keepA.ID, removed.ID, keepB.ID))
	seedThreadRow(t, db, 2, fmt.Sprintf("[%d]", removed.ID))
	seedThreadRow(t, db, 3, fmt.Sprintf("[%d, %d]", keepA.ID, keepB.ID))

	updated, err := articlerefs.PruneArticleRefs(db, []uint{removed.ID})
	require.NoError(t, err)
	require.EqualValues(t, 2, updated)
	require.Equal(t, []int64{int64(keepA.ID), int64(keepB.ID)}, refsIDs(t, db, 1), "order survives the drop")
	require.Empty(t, refsIDs(t, db, 2), "the only reference is dropped")
	require.Equal(t, "array", refsType(t, db, 2), "an emptied array must stay [] and never become null")
	require.Equal(t, "[]", refsText(t, db, 2))
	require.Equal(t, fmt.Sprintf("[%d, %d]", keepA.ID, keepB.ID), refsText(t, db, 3))
}

// TestPruneArticleRefsEmptyIDsIsNoOp covers the guard: no ids means no query and
// no write.
func TestPruneArticleRefsEmptyIDsIsNoOp(t *testing.T) {
	db := testutil.SetupTestDB(t)
	article := seedArticleRow(t, db, "empty-ids")
	seedThreadRow(t, db, 1, fmt.Sprintf("[%d]", article.ID))
	before := refsText(t, db, 1)

	updated, err := articlerefs.PruneArticleRefs(db, nil)
	require.NoError(t, err)
	require.Zero(t, updated)

	updated, err = articlerefs.PruneArticleRefs(db, []uint{0})
	require.NoError(t, err)
	require.Zero(t, updated, "id 0 is not a valid article key")

	require.Equal(t, before, refsText(t, db, 1))
}

// TestPruneArticleRefsDropsDirtyElements covers the dirty-data branch: a
// non-numeric element cannot reference an article, so maintaining the array
// removes it instead of perpetuating it (and no ::bigint cast may blow up).
func TestPruneArticleRefsDropsDirtyElements(t *testing.T) {
	db := testutil.SetupTestDB(t)
	alive := seedArticleRow(t, db, "dirty-alive")
	removed := seedArticleRow(t, db, "dirty-removed")

	seedThreadRow(t, db, 1, fmt.Sprintf("[%d, %d, \"abc\", \"\"]", alive.ID, removed.ID))

	updated, err := articlerefs.PruneArticleRefs(db, []uint{removed.ID})
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
	require.Equal(t, []int64{int64(alive.ID)}, refsIDs(t, db, 1))
}

// TestPruneArticleRefsToleratesScalarRefs proves the reference filter never
// trips on the JSON null scalar historical rows carry: the guarded read maps it
// to zero references instead of raising SQLSTATE 22023 for the whole statement.
// Normalizing such a row is NormalizeThreadRefs'/PruneDanglingRefs' job — this
// call targets specific ids and must not touch unrelated rows.
func TestPruneArticleRefsToleratesScalarRefs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	alive := seedArticleRow(t, db, "scalar-alive")
	removed := seedArticleRow(t, db, "scalar-removed")

	seedThreadRow(t, db, 1, "null")
	seedThreadRow(t, db, 2, fmt.Sprintf("[%d, %d]", alive.ID, removed.ID))

	updated, err := articlerefs.PruneArticleRefs(db, []uint{removed.ID})
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
	require.Equal(t, "null", refsType(t, db, 1), "a scalar row is skipped, not repaired, by an id-targeted prune")
	require.Equal(t, []int64{int64(alive.ID)}, refsIDs(t, db, 2))
}
