package airouter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
)

// countingEmbedClient wraps fakeProviderClient behavior with an Embed call
// counter so cache-hit tests can assert no HTTP call was made.
type countingEmbedClient struct {
	embedCalls atomic.Int32
	vectors    [][]float64
}

func (c *countingEmbedClient) Chat(_ context.Context, _ models.AIProvider, _ ChatRequest) (*ChatResponse, error) {
	return &ChatResponse{Content: "unused"}, nil
}

func (c *countingEmbedClient) Embed(_ context.Context, provider models.AIProvider, _ EmbeddingRequest) (*EmbeddingResult, error) {
	c.embedCalls.Add(1)
	if c.vectors == nil {
		c.vectors = [][]float64{{0.5, 0.25}}
	}
	return &EmbeddingResult{Embeddings: c.vectors, Model: provider.Model, Dimensions: len(c.vectors[0]), Provider: provider.Name}, nil
}

func newEmbedCacheRouter(t *testing.T) (*Router, *Store, *countingEmbedClient) {
	t.Helper()
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	setupEmbedRoute(t, store, "m1")
	router := NewRouterWithStore(store)
	client := &countingEmbedClient{}
	router.RegisterClient(ProviderTypeOpenAICompatible, client)
	return router, store, client
}

func TestEmbedCacheHitSkipsProviderCall(t *testing.T) {
	router, _, client := newEmbedCacheRouter(t)

	req := EmbeddingRequest{Input: []string{"OpenAI"}, Operation: "tagmanagement.embedding"}
	first, err := router.Embed(context.Background(), req, CapabilityEmbedding)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, int32(1), client.embedCalls.Load())

	second, err := router.Embed(context.Background(), req, CapabilityEmbedding)
	require.NoError(t, err)
	require.NotNil(t, second)
	// Second identical call must be served from cache: no extra provider call.
	require.Equal(t, int32(1), client.embedCalls.Load())
	require.Equal(t, first.Embeddings, second.Embeddings)
	require.Equal(t, first.Model, second.Model)
	require.Equal(t, first.Dimensions, second.Dimensions)
	require.Equal(t, first.Provider, second.Provider)
}

func TestEmbedCacheHitDoesNotTakeSemaphore(t *testing.T) {
	router, store, client := newEmbedCacheRouter(t)

	req := EmbeddingRequest{Input: []string{"cached-label"}, Operation: "tagmanagement.embedding"}
	_, err := router.Embed(context.Background(), req, CapabilityEmbedding)
	require.NoError(t, err)
	require.Equal(t, int32(1), client.embedCalls.Load())

	// Fill every embedding semaphore slot (default capacity 5).
	route, _, err := store.LoadRouteWithProviders(CapabilityEmbedding)
	require.NoError(t, err)
	sem := router.getSemaphore(CapabilityEmbedding, route)
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}

	// Cache hit must return immediately without waiting on the semaphore.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, err := router.Embed(ctx, req, CapabilityEmbedding)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, int32(1), client.embedCalls.Load(), "cache hit must not call provider")
}

func TestEmbedCacheDifferentModelDoesNotCrossHit(t *testing.T) {
	router, store, client := newEmbedCacheRouter(t)

	req := EmbeddingRequest{Input: []string{"same text"}, Operation: "tagmanagement.embedding"}
	_, err := router.Embed(context.Background(), req, CapabilityEmbedding)
	require.NoError(t, err)
	require.Equal(t, int32(1), client.embedCalls.Load())

	// Switch the provider's model: same input must miss the cache.
	var p models.AIProvider
	require.NoError(t, store.db.First(&p, "name = ?", "emb-provider").Error)
	p.Model = "m2"
	require.NoError(t, store.db.Save(&p).Error)

	_, err = router.Embed(context.Background(), req, CapabilityEmbedding)
	require.NoError(t, err)
	require.Equal(t, int32(2), client.embedCalls.Load(), "different model must not hit m1 cache")

	var count int64
	require.NoError(t, store.db.Model(&models.AIEmbeddingCache{}).Count(&count).Error)
	require.Equal(t, int64(2), count, "one cache row per model")
}

func TestEmbedCacheWriteFailureDoesNotAffectResult(t *testing.T) {
	router, store, client := newEmbedCacheRouter(t)
	require.NoError(t, store.db.Migrator().DropTable(&models.AIEmbeddingCache{}))

	res, err := router.Embed(context.Background(), EmbeddingRequest{Input: []string{"no table"}, Operation: "tagmanagement.embedding"}, CapabilityEmbedding)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, int32(1), client.embedCalls.Load())
	require.Equal(t, "emb-provider", res.Provider)
}

func TestEmbedCacheLookupFailureFallsThroughToProvider(t *testing.T) {
	router, store, client := newEmbedCacheRouter(t)
	require.NoError(t, store.db.Migrator().DropTable(&models.AIEmbeddingCache{}))

	req := EmbeddingRequest{Input: []string{"no table"}, Operation: "tagmanagement.embedding"}

	// First call: lookup fails, real provider call runs, save failure only warns.
	first, err := router.Embed(context.Background(), req, CapabilityEmbedding)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, int32(1), client.embedCalls.Load())
	require.Equal(t, "emb-provider", first.Provider)

	// Second call: lookup fails again and must still fall through to the
	// provider instead of erroring out.
	second, err := router.Embed(context.Background(), req, CapabilityEmbedding)
	require.NoError(t, err)
	require.NotNil(t, second)
	require.Equal(t, int32(2), client.embedCalls.Load())
	require.Equal(t, first.Embeddings, second.Embeddings)
	require.Equal(t, first.Model, second.Model)
}

func TestEmbedCacheConcurrentSameKeySingleRow(t *testing.T) {
	router, store, _ := newEmbedCacheRouter(t)

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = router.Embed(context.Background(), EmbeddingRequest{Input: []string{"race"}, Operation: "tagmanagement.embedding"}, CapabilityEmbedding)
		}(i)
	}
	wg.Wait()
	for i := range errs {
		require.NoError(t, errs[i])
	}

	var count int64
	require.NoError(t, store.db.Model(&models.AIEmbeddingCache{}).Count(&count).Error)
	require.Equal(t, int64(1), count, "ON CONFLICT DO NOTHING keeps one row per key")
}

func TestEmbedCacheHitLogsCallWithCacheHitMeta(t *testing.T) {
	router, store, _ := newEmbedCacheRouter(t)

	req := EmbeddingRequest{Input: []string{"logged"}, Operation: "tagmanagement.embedding", Metadata: map[string]any{"article_id": 42}}
	_, err := router.Embed(context.Background(), req, CapabilityEmbedding)
	require.NoError(t, err)
	_, err = router.Embed(context.Background(), req, CapabilityEmbedding)
	require.NoError(t, err)

	var logs []models.AICallLog
	require.NoError(t, store.db.Order("id ASC").Find(&logs, "capability = ?", string(CapabilityEmbedding)).Error)
	require.Len(t, logs, 2)

	first, second := logs[0], logs[1]
	require.NotContains(t, first.RequestMeta, "cache_hit", "miss log must not carry cache_hit")
	require.True(t, second.Success)
	require.Contains(t, second.RequestMeta, `"cache_hit":true`)
	require.Contains(t, second.RequestMeta, `"article_id":42`)
}

// TestEmbedCacheExcludedOperationBypassesCache verifies the allowlist:
// operations with one-shot inputs (section/auxlabel embeddings) neither
// write cache rows nor consult the cache on later calls.
func TestEmbedCacheExcludedOperationBypassesCache(t *testing.T) {
	router, store, client := newEmbedCacheRouter(t)

	req := EmbeddingRequest{Input: []string{"article body text"}, Operation: "section.embedding"}
	for i := 0; i < 2; i++ {
		res, err := router.Embed(context.Background(), req, CapabilityEmbedding)
		require.NoError(t, err)
		require.NotNil(t, res)
	}
	// Every call goes to the provider; nothing is cached.
	require.Equal(t, int32(2), client.embedCalls.Load(), "excluded operation must always call provider")

	var count int64
	require.NoError(t, store.db.Model(&models.AIEmbeddingCache{}).Count(&count).Error)
	require.Equal(t, int64(0), count, "excluded operation must not write cache rows")

	var logs []models.AICallLog
	require.NoError(t, store.db.Find(&logs, "operation = ?", "section.embedding").Error)
	require.Len(t, logs, 2)
	for _, l := range logs {
		require.NotContains(t, l.RequestMeta, "cache_hit", "excluded operations never log cache_hit")
	}
}

// TestEmbedCacheAllowlistCoversProductionOperations pins the allowlist to
// the operations whose inputs recur across articles; one-shot content
// operations must stay out.
func TestEmbedCacheAllowlistCoversProductionOperations(t *testing.T) {
	require.True(t, embeddingCacheable("tagmanagement.embedding"))
	for _, op := range []string{
		"tagmanagement.auxlabel_embedding",
		"section.embedding",
		"section.embedding_backfill",
		"discovery.route_embedding",
	} {
		require.False(t, embeddingCacheable(op), "%s must be excluded", op)
	}
}
