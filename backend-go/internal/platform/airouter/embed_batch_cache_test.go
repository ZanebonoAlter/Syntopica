package airouter

// speed-up-tag-embedding 任务 2.4（test-cases.md 步 6/7 + 白盒批量缓存部分命
// 中边界 + D4 Retryable 重试）：
//   - 批量请求按单条 key 批查缓存：预置历史（单条路径写入的）缓存行 → 批量
//     调用只把 miss 部分发给 provider，结果按单条 key 落缓存；
//   - 单条路径 key 口径不变（历史缓存兼容：同一输入单条调用写、批量调用命中）；
//   - ProviderError.Retryable=true 做 1 次 200ms 退避重试。

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
)

// inputRecorderEmbedClient 记录每次 Embed 收到的 Input（按顺序）。
type inputRecorderEmbedClient struct {
	mu         sync.Mutex
	inputs     [][]string
	failNTimes int
	failErr    error
}

func (c *inputRecorderEmbedClient) Chat(_ context.Context, _ models.AIVendor, _ models.AIModel, _ string, _ ChatRequest) (*ChatResponse, []StructuredAttempt, error) {
	return &ChatResponse{Content: "unused"}, nil, nil
}

func (c *inputRecorderEmbedClient) Embed(ctx context.Context, vendor models.AIVendor, model models.AIModel, req EmbeddingRequest) (*EmbeddingResult, error) {
	c.mu.Lock()
	c.inputs = append(c.inputs, req.Input)
	failNTimes := c.failNTimes
	c.failNTimes--
	c.mu.Unlock()
	if failNTimes > 0 && c.failErr != nil {
		return nil, c.failErr
	}
	vecs := make([][]float64, len(req.Input))
	for i := range vecs {
		vecs[i] = []float64{float64(i), 0.5}
	}
	return &EmbeddingResult{Embeddings: vecs, Model: model.Name, Dimensions: 2, Provider: vendor.Name}, nil
}

func (c *inputRecorderEmbedClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.inputs)
}

func (c *inputRecorderEmbedClient) lastInput() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.inputs) == 0 {
		return nil
	}
	return c.inputs[len(c.inputs)-1]
}

func newBatchCacheRouter(t *testing.T) (*Router, *Store, *inputRecorderEmbedClient) {
	t.Helper()
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	setupEmbedRoute(t, store, "m1")
	router := NewRouterWithStore(store)
	client := &inputRecorderEmbedClient{}
	router.RegisterClient(ProviderTypeOpenAICompatible, client)
	return router, store, client
}

// 步 7 兼容：单条路径写入的历史缓存行，批量调用必须按同口径命中。
func preseedSingleItemCache(t *testing.T, router *Router, text string) {
	t.Helper()
	_, err := router.Embed(context.Background(), EmbeddingRequest{
		Input:     []string{text},
		Operation: "tagmanagement.embedding",
	}, CapabilityEmbedding)
	require.NoError(t, err)
}

// TestEmbedBatchCachePartialHit 步 6：3 条输入、1 条已有缓存 → provider 只收
// 到 2 条 miss 输入；结果 3 条齐全；miss 结果按单条 key 落缓存。
func TestEmbedBatchCachePartialHit(t *testing.T) {
	router, store, client := newBatchCacheRouter(t)

	// 先用单条路径写一条历史缓存（key 口径与批量 per-item key 一致）。
	preseedSingleItemCache(t, router, "cached-label")

	res, err := router.Embed(context.Background(), EmbeddingRequest{
		Input:     []string{"cached-label", "miss-a", "miss-b"},
		Operation: "tagmanagement.embedding",
	}, CapabilityEmbedding)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Embeddings, 3)

	// provider 只收到 miss 的 2 条（批内 1 条缓存命中不发 provider）。
	got := client.lastInput()
	require.Equal(t, []string{"miss-a", "miss-b"}, got, "provider must receive only cache misses in batch order")

	// miss 部分已按单条 key 写缓存：同批重放全命中，零 provider 调用。
	before := client.callCount()
	res2, err := router.Embed(context.Background(), EmbeddingRequest{
		Input:     []string{"cached-label", "miss-a", "miss-b"},
		Operation: "tagmanagement.embedding",
	}, CapabilityEmbedding)
	require.NoError(t, err)
	require.NotNil(t, res2)
	require.Equal(t, before, client.callCount(), "fully-cached batch must not call provider")
	require.Len(t, res2.Embeddings, 3)

	// 单条 key 批写验证：miss-a 的单条 key 缓存行存在。
	route, bindings, err := store.LoadRouteWithModels(CapabilityEmbedding)
	require.NoError(t, err)
	_ = route
	key := embeddingCacheKey(bindings[0].Model.Name, []string{"miss-a"})
	cached, err := store.LookupEmbeddingCache(context.Background(), key)
	require.NoError(t, err)
	require.NotNil(t, cached, "miss result must be persisted under the per-item key")
}

// 白盒边界：全 miss（0 hit）与全 hit（N hit）。
func TestEmbedBatchCacheAllMissAndAllHit(t *testing.T) {
	router, _, client := newBatchCacheRouter(t)

	// 全 miss：3 条都进 provider。
	_, err := router.Embed(context.Background(), EmbeddingRequest{
		Input:     []string{"x1", "x2", "x3"},
		Operation: "tagmanagement.embedding",
	}, CapabilityEmbedding)
	require.NoError(t, err)
	require.Equal(t, []string{"x1", "x2", "x3"}, client.lastInput())

	// 全 hit：重放同批，零 provider。
	before := client.callCount()
	_, err = router.Embed(context.Background(), EmbeddingRequest{
		Input:     []string{"x1", "x2", "x3"},
		Operation: "tagmanagement.embedding",
	}, CapabilityEmbedding)
	require.NoError(t, err)
	require.Equal(t, before, client.callCount())
}

// D4：ProviderError.Retryable=true 退避一次重试成功；不可重试错误不重试。
func TestEmbedRetryableErrorRetriedOnce(t *testing.T) {
	router, _, client := newBatchCacheRouter(t)
	client.failNTimes = 1
	client.failErr = &ProviderError{Message: "rate limited", Code: "429", Retryable: true}

	res, err := router.Embed(context.Background(), EmbeddingRequest{
		Input:     []string{"retry-me"},
		Operation: "tagmanagement.embedding",
	}, CapabilityEmbedding)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 2, client.callCount(), "retryable error must trigger exactly one retry")
}

func TestEmbedNonRetryableErrorNoRetry(t *testing.T) {
	router, _, client := newBatchCacheRouter(t)
	client.failNTimes = 100
	client.failErr = &ProviderError{Message: "bad request", Code: "400"}

	_, err := router.Embed(context.Background(), EmbeddingRequest{
		Input:     []string{"no-retry"},
		Operation: "tagmanagement.embedding",
	}, CapabilityEmbedding)
	require.Error(t, err)
	require.Equal(t, 1, client.callCount(), "non-retryable error must not retry within a binding")
}
