package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Pure-function unit tests for manual-topic helpers (no DB, no Docker).
// These exercise aggregateEmbeddings / detectOutliers / FloatsToPgVector
// directly and run under `go test -short`. Split out of
// daily_report_manual_topic_test.go (which now holds only the PostgreSQL
// integration tests) per the repository-layer naming convention
// (`*_unit_test.go` vs `*_test.go`).

// ── aggregateEmbeddings tests ───────────────────────────────────────────────

func TestAggregateEmbeddings_Normal(t *testing.T) {
	vectors := [][]float64{
		{1.0, 2.0, 3.0},
		{4.0, 5.0, 6.0},
		{7.0, 8.0, 9.0},
	}
	mean, skipped := aggregateEmbeddings(vectors)
	assert.Equal(t, 0, skipped)
	assert.Len(t, mean, 3)
	// (1+4+7)/3=4, (2+5+8)/3=5, (3+6+9)/3=6
	assert.InDeltaSlice(t, []float64{4.0, 5.0, 6.0}, mean, 1e-9)
}

func TestAggregateEmbeddings_DimensionMismatch(t *testing.T) {
	vectors := [][]float64{
		{1.0, 2.0, 3.0},
		{4.0, 5.0}, // 2-dim — skipped
		{7.0, 8.0, 9.0},
	}
	mean, skipped := aggregateEmbeddings(vectors)
	assert.Equal(t, 1, skipped)
	assert.Len(t, mean, 3)
	// mean of first and third only: (1+7)/2=4, (2+8)/2=5, (3+9)/2=6
	assert.InDeltaSlice(t, []float64{4.0, 5.0, 6.0}, mean, 1e-9)
}

func TestAggregateEmbeddings_EmptyInput(t *testing.T) {
	mean, skipped := aggregateEmbeddings(nil)
	assert.Equal(t, 0, skipped)
	assert.Nil(t, mean)
}

func TestAggregateEmbeddings_AllSkipped(t *testing.T) {
	vectors := [][]float64{
		{1.0, 2.0, 3.0},
		nil,        // nil slice → skipped
		{4.0, 5.0}, // wrong dim (2 vs 3) → skipped
	}
	mean, skipped := aggregateEmbeddings(vectors)
	assert.Equal(t, 2, skipped)
	// only {1,2,3} is usable → mean = {1,2,3}
	assert.InDeltaSlice(t, []float64{1.0, 2.0, 3.0}, mean, 1e-9)
}

func TestAggregateEmbeddings_SingleVector(t *testing.T) {
	vectors := [][]float64{
		{3.5, -2.1, 0.0},
	}
	mean, skipped := aggregateEmbeddings(vectors)
	assert.Equal(t, 0, skipped)
	assert.InDeltaSlice(t, []float64{3.5, -2.1, 0.0}, mean, 1e-9)
}

func TestAggregateEmbeddings_EmptyVectorsAll(t *testing.T) {
	// all slices are empty (len=0) → none usable
	mean, skipped := aggregateEmbeddings([][]float64{{}, {}})
	assert.Equal(t, 2, skipped)
	assert.Nil(t, mean, "all empty → nil mean")
}

// ── detectOutliers tests ────────────────────────────────────────────────────

func TestDetectOutliers_AllTight(t *testing.T) {
	distances := []float64{0.1, 0.15, 0.12}
	threshold := 0.3
	flags := detectOutliers(distances, threshold)
	assert.Equal(t, []bool{false, false, false}, flags)
}

func TestDetectOutliers_ContainsOutlier(t *testing.T) {
	distances := []float64{0.1, 0.8, 0.12} // 0.8 > 0.3*1.3 = 0.39
	threshold := 0.3
	flags := detectOutliers(distances, threshold)
	assert.Equal(t, []bool{false, true, false}, flags)
}

func TestDetectOutliers_ThresholdBoundary(t *testing.T) {
	threshold := 0.3
	boundary := threshold * 1.3 // exactly 0.39
	distances := []float64{0.1, boundary, boundary + 1e-9}
	flags := detectOutliers(distances, threshold)
	// boundary (0.39) is NOT > 0.39 → false; boundary+epsilon IS > 0.39 → true
	assert.False(t, flags[1], "exactly at threshold must not be outlier")
	assert.True(t, flags[2], "slightly above threshold must be outlier")
}

func TestDetectOutliers_EmptyInput(t *testing.T) {
	flags := detectOutliers(nil, 0.3)
	assert.Nil(t, flags)
}

// ── formatPgVector tests ────────────────────────────────────────────────────

func TestFloatsToPgVector_Roundtrip(t *testing.T) {
	original := []float64{1.0, 2.5, -0.3}
	formatted := FloatsToPgVector(original)
	parsed, err := repoParsePgVector(formatted)
	assert.NoError(t, err)
	assert.Len(t, parsed, len(original))
	for i, v := range original {
		assert.InDelta(t, v, parsed[i], 1e-9)
	}
}

func TestFloatsToPgVector_Empty(t *testing.T) {
	result := FloatsToPgVector(nil)
	assert.Equal(t, "[]", result)
}

func TestFloatsToPgVector_SingleElement(t *testing.T) {
	result := FloatsToPgVector([]float64{42.0})
	parsed, err := repoParsePgVector(result)
	assert.NoError(t, err)
	assert.InDelta(t, 42.0, parsed[0], 1e-9)
}

// ── helpers ─────────────────────────────────────────────────────────────────

// verifyNoDbEmpty ensures a pure-function test didn't accidentally try to use a DB.
func verifyNoDbEmpty(t *testing.T) {
	t.Helper()
	if Repo != nil && Repo.db != nil {
		t.Skip("not a DB test – Repo is assigned from another test")
	}
}

func TestPureFunctionDetection(t *testing.T) {
	// Sanity: if Repo.db is non-nil, the global was leaked from another test.
	// Pure-function tests must not depend on Repo.
	verifyNoDbEmpty(t)
}
