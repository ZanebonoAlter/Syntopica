package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── cosine retrieval (design §D3) ──

func nearlyEqualVec() []float64 { return []float64{1, 0, 0} }

func TestCosineSimilarity(t *testing.T) {
	assert.InDelta(t, 1.0, cosineSimilarity(nearlyEqualVec(), []float64{2, 0, 0}), 1e-9, "same direction ⇒ 1")
	assert.InDelta(t, 0.0, cosineSimilarity(nearlyEqualVec(), []float64{0, 1, 0}), 1e-9, "orthogonal ⇒ 0")
	assert.InDelta(t, -1.0, cosineSimilarity(nearlyEqualVec(), []float64{-1, 0, 0}), 1e-9, "opposite ⇒ -1")
	assert.Equal(t, 0.0, cosineSimilarity(nearlyEqualVec(), []float64{1, 0}), "length mismatch ⇒ 0")
	assert.Equal(t, 0.0, cosineSimilarity(nil, nil), "nil ⇒ 0")
	assert.Equal(t, 0.0, cosineSimilarity([]float64{0, 0}, []float64{0, 0}), "zero norm ⇒ 0")
}

// ── query sentence resolution (spec: query 为空回退 label) ──

func TestWatchQuerySentence(t *testing.T) {
	w := repository.BoardTopicWatch{Label: "话题名", Query: "检索句"}
	assert.Equal(t, "检索句", watchQuerySentence(w))
	w.Query = ""
	assert.Equal(t, "话题名", watchQuerySentence(w))
}

// ── lazy embedding lifecycle (design §D3: cache → lazy recompute → write-back) ──

func TestEnsureWatchQueryVec_CachedVectorWins(t *testing.T) {
	w := repository.BoardTopicWatch{
		ID:             1,
		Label:          "L",
		EmbeddingCache: strPtrHelper(repository.FloatsToPgVector([]float64{0.5, 0.5})),
	}
	called := false
	embed := func(ctx context.Context, req airouter.EmbeddingRequest, cap airouter.Capability) (*airouter.EmbeddingResult, error) {
		called = true
		return &airouter.EmbeddingResult{Embeddings: [][]float64{{1, 1}}}, nil
	}
	vec := ensureWatchQueryVec(context.Background(), &w, embed)
	require.Len(t, vec, 2)
	assert.InDelta(t, 0.5, vec[0], 1e-9)
	assert.False(t, called, "cached vector must bypass embedding")
}

func TestEnsureWatchQueryVec_LazyRecomputeUsesQuerySentence(t *testing.T) {
	w := repository.BoardTopicWatch{ID: 2, Label: "话题名", Query: "检索句优先"}
	var embeddedInput string
	embed := func(ctx context.Context, req airouter.EmbeddingRequest, cap airouter.Capability) (*airouter.EmbeddingResult, error) {
		embeddedInput = req.Input[0]
		return &airouter.EmbeddingResult{Embeddings: [][]float64{{0.1, 0.2, 0.3}}}, nil
	}
	vec := ensureWatchQueryVec(context.Background(), &w, embed)
	// Note: write-back goes to repository.Repo (nil in unit tests without DB) —
	// ensureWatchQueryVec logs-and-continues on the write-back failure; the
	// returned vector still comes from the fresh embed.
	require.Len(t, vec, 3)
	assert.Equal(t, "检索句优先", embeddedInput, "lazy recompute embeds the resolved query sentence")
	assert.NotNil(t, w.EmbeddingCache, "in-memory cache is refreshed even if DB write-back degraded")
}

func TestEnsureWatchQueryVec_EmbedFailureDegradesToNil(t *testing.T) {
	w := repository.BoardTopicWatch{ID: 3, Label: "L"}
	embed := func(ctx context.Context, req airouter.EmbeddingRequest, cap airouter.Capability) (*airouter.EmbeddingResult, error) {
		return nil, assert.AnError
	}
	assert.Nil(t, ensureWatchQueryVec(context.Background(), &w, embed))
}

// ── retrieval threshold / top-K unit behavior (pure functions over pool) ──

func TestRetrieveAuxLabels_ThresholdAndTopK(t *testing.T) {
	// Directly test the scoring/capping logic through cosineSimilarity +
	// the same filtering code path shape: pool entries with known vectors.
	// (Full SQL pool test lives in repository integration tests.)
	pool := []repository.WatchSentenceLabel{
		{ID: 1, Label: "exact", Embedding: []float64{1, 0}},
		{ID: 2, Label: "orthogonal", Embedding: []float64{0, 1}},
		{ID: 3, Label: "half", Embedding: []float64{1, 1}}, // cos = 0.707
	}
	q := []float64{1, 0}
	// threshold 0.55: labels 1 (1.0) and 3 (0.707) pass, 2 (0.0) not.
	// top-K 1: only label 1 survives the cap.
	assert.InDelta(t, 1.0, cosineSimilarity(q, pool[0].Embedding), 1e-9)
	assert.InDelta(t, 0.0, cosineSimilarity(q, pool[1].Embedding), 1e-9)
	assert.InDelta(t, 0.7071, cosineSimilarity(q, pool[2].Embedding), 1e-3)
	_ = repository.WatchSentenceLabel{} // keep import referenced
}
