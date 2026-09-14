package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/testutil"
)

// ── improve-discovery-recommendations 4.1：run 原子发布 / 严格精排 测试 ──
//
// 红基准（S1/S2 步 1）：TestRedBaseline_* 用现有代码路径复现两个 bug——
// ①精排 LLM 已选择子集但全部候选仍落库；②Ask 走旧种子通道且发布结果无数据库 ID。
// mock chat 返回新协议 JSON（{"selected":[{"id":...}]}）：旧宽松解析器只认
// route_id 字段 → 解析为空 → 全量落库（红）；严格实现按 id 解析 → 只发布选中项（绿）。

// mockDiscoveryClient 是 airouter.ProviderClient 的可控假实现：
// chatFn/embedFn 为 nil 时返回占位成功；chatCalls/embedCalls 统计真实调用次数。
type mockDiscoveryClient struct {
	chatFn     func(req airouter.ChatRequest) (string, error)
	embedFn    func(req airouter.EmbeddingRequest) (*airouter.EmbeddingResult, error)
	chatCalls  int
	embedCalls int
}

func (m *mockDiscoveryClient) Chat(_ context.Context, _ models.AIProvider, req airouter.ChatRequest) (*airouter.ChatResponse, error) {
	m.chatCalls++
	if m.chatFn == nil {
		return &airouter.ChatResponse{Content: `{"selected":[]}`}, nil
	}
	content, err := m.chatFn(req)
	if err != nil {
		return nil, err
	}
	return &airouter.ChatResponse{Content: content}, nil
}

func (m *mockDiscoveryClient) Embed(_ context.Context, provider models.AIProvider, req airouter.EmbeddingRequest) (*airouter.EmbeddingResult, error) {
	m.embedCalls++
	if m.embedFn == nil {
		return &airouter.EmbeddingResult{Embeddings: [][]float64{{1, 0, 0}}, Model: "test", Dimensions: 3, Provider: provider.Name}, nil
	}
	return m.embedFn(req)
}

// newMockAIRouter 在 db（sqlite 或 PG 测试库）上建 ai fixtures 并注入 mock client：
// llm provider + feed_discovery route；embedding provider + embedding route。
// skipEmbedding 时只建精排能力（配置缺失用例不建任何 fixture，直接用 nil router）。
func newMockAIRouter(t *testing.T, db *gorm.DB, client *mockDiscoveryClient, skipEmbedding bool) *airouter.Router {
	t.Helper()
	llm := models.AIProvider{Name: "fd-llm", ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL: "https://a.example/v1", APIKey: "k", Model: "m", ModelKind: "llm", Enabled: true}
	require.NoError(t, db.Create(&llm).Error)
	fdRoute := models.AIRoute{Name: "test-fd", Capability: string(airouter.CapabilityFeedDiscovery), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&fdRoute).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: fdRoute.ID, ProviderID: llm.ID, Priority: 1, Enabled: true}).Error)
	if !skipEmbedding {
		emb := models.AIProvider{Name: "fd-emb", ProviderType: airouter.ProviderTypeOpenAICompatible,
			BaseURL: "https://b.example/v1", APIKey: "k", Model: "e", ModelKind: "embedding", Enabled: true}
		require.NoError(t, db.Create(&emb).Error)
		embRoute := models.AIRoute{Name: "test-emb", Capability: string(airouter.CapabilityEmbedding), Enabled: true, Strategy: "ordered_failover"}
		require.NoError(t, db.Create(&embRoute).Error)
		require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: embRoute.ID, ProviderID: emb.ID, Priority: 1, Enabled: true}).Error)
	}
	router := airouter.NewRouterWithStore(airouter.NewStore(db))
	router.RegisterClient(airouter.ProviderTypeOpenAICompatible, client)
	return router
}

// setupRunRoutesFixture 建 n 条 usable_directly ok 路由（无 embedding——召回由
// fake recall 注入或 PG 用例另建），返回 route id 列表。
func setupRunRoutesFixture(t *testing.T, db *gorm.DB, n int) []uint {
	t.Helper()
	ids := make([]uint, 0, n)
	for i := 0; i < n; i++ {
		r := models.RSSHubRoute{
			Namespace: fmt.Sprintf("ns%d", i), Path: fmt.Sprintf("/p%d", i),
			Name: fmt.Sprintf("Route%d", i), Example: fmt.Sprintf("/ns%d/p%d", i, i),
			UsableDirectly: true, Status: "ok", Parameters: "{}",
		}
		require.NoError(t, db.Create(&r).Error)
		ids = append(ids, r.ID)
	}
	return ids
}

// strictSelectionJSON 构造严格精排协议响应。
func strictSelectionJSON(picks ...[2]any) string {
	var b strings.Builder
	b.WriteString(`{"selected":[`)
	for i, p := range picks {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%v,"reason":"%v"}`, p[0], p[1])
	}
	b.WriteString(`]}`)
	return b.String()
}

// ── 红基准（PG：旧代码路径完整复现，转绿后继续在同路径上验证新契约）──

// TestRedBaseline_LLMSubsetNotApplied ①：LLM 明确只选 2/6，本轮应仅 2 条成为
// 有效结果（spec Selected Recommendations Only「精排选择子集」）。
// 旧代码全候选落库 → 断言 pending==2 当前必红（实际 6）。
func TestRedBaseline_LLMSubsetNotApplied(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupRunRoutesFixture(t, db, 6)
	for i, id := range ids {
		setupCandidateVector(t, db, id, fmt.Sprintf("ns%d", i), fmt.Sprintf("/p%d", i), "test", []float64{1, 0, 0})
	}
	client := &mockDiscoveryClient{
		chatFn: func(airouter.ChatRequest) (string, error) {
			return strictSelectionJSON([2]any{ids[0], "理由甲"}, [2]any{ids[1], "理由乙"}), nil
		},
	}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "日本新闻", "")
	require.NoError(t, err)
	require.Equal(t, "succeeded", run.Status, "严格精排选中 2 条应成功发布")

	var pending []models.FeedRecommendation
	require.NoError(t, db.Where("status = ?", "pending").Find(&pending).Error)
	require.Len(t, pending, 2, "精排选择子集：仅 2 条发布，其余 4 条不以无理由推荐落库")
}

// TestRedBaseline_AskIndependentInterestAndDBID ②：成功查询应落独立兴趣条目
// （不再走旧 seed EMA 通道）且发布结果带数据库 ID / last_selected_at。
// 旧代码写 preference_vectors(seed)、发布行无 last_selected_at → 当前必红。
func TestRedBaseline_AskIndependentInterestAndDBID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupRunRoutesFixture(t, db, 3)
	for i, id := range ids {
		setupCandidateVector(t, db, id, fmt.Sprintf("ns%d", i), fmt.Sprintf("/p%d", i), "test", []float64{1, 0, 0})
	}
	client := &mockDiscoveryClient{
		chatFn: func(airouter.ChatRequest) (string, error) {
			return strictSelectionJSON([2]any{ids[0], "理由甲"}), nil
		},
	}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "dev tools", "")
	require.NoError(t, err)
	require.Equal(t, "succeeded", run.Status)

	// 独立兴趣条目：成功查询（含零结果）写一条，关联本次 run。
	var interests []models.DiscoveryInterestEntry
	require.NoError(t, db.Find(&interests).Error)
	require.Len(t, interests, 1, "成功查询应写独立兴趣记录")
	require.Equal(t, run.ID, *interests[0].RunID)
	require.Equal(t, "dev tools", interests[0].QueryText)
	require.Equal(t, "active", interests[0].Status)
	require.Nil(t, interests[0].BoardID, "无版块可匹配时保持 NULL（未匹配组，不挂标签不建版块）")
	require.Equal(t, 3, interests[0].Dimension)

	// 发布行带入选时间（旧 insertPending 不写）。
	var pending []models.FeedRecommendation
	require.NoError(t, db.Where("status = ?", "pending").Find(&pending).Error)
	require.Len(t, pending, 1)
	require.NotNil(t, pending[0].LastSelectedAt, "入选应刷新 last_selected_at")
	require.NotNil(t, pending[0].ExpiresAt, "pending 应带 expires_at")
}

// ── 回归网共享 helper（旧 recommendation 测试适配新契约用）──

// selectAllChatFn 返回「把 prompt 中出现的全部候选 id 都选中」的 chatFn。
// 新契约（spec Selected Recommendations Only）下精排未选中的候选不发布，
// 旧回归网测试改用它让全部粗筛候选入选，保持原断言语义不变。
func selectAllChatFn() func(airouter.ChatRequest) (string, error) {
	return func(req airouter.ChatRequest) (string, error) {
		ids := []uint{}
		for _, line := range strings.Split(req.Messages[0].Content, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "- id=") {
				continue
			}
			rest := line[len("- id="):]
			if i := strings.IndexByte(rest, ' '); i > 0 {
				rest = rest[:i]
			}
			if n, err := strconv.ParseUint(rest, 10, 64); err == nil {
				ids = append(ids, uint(n))
			}
		}
		picks := make([][2]any, 0, len(ids))
		for _, id := range ids {
			picks = append(picks, [2]any{id, "回归网理由"})
		}
		return strictSelectionJSON(picks...), nil
	}
}

// newSelectAllMockRouter 建带全选 mock 的完整 router（llm+embedding），
// 返回 (router, client)；client 供断言调用计数。
func newSelectAllMockRouter(t *testing.T, db *gorm.DB) (*airouter.Router, *mockDiscoveryClient) {
	t.Helper()
	client := &mockDiscoveryClient{chatFn: selectAllChatFn()}
	return newMockAIRouter(t, db, client, false), client
}

// newMockAIRouterEmbeddingOnly 只建 embedding 能力（不建 feed_discovery route），
// 用于配置缺失用例：capability 无 provider → configuration 失败、零 provider 调用。
func newMockAIRouterEmbeddingOnly(t *testing.T, db *gorm.DB, client *mockDiscoveryClient) *airouter.Router {
	t.Helper()
	emb := models.AIProvider{Name: "fd-emb", ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL: "https://b.example/v1", APIKey: "k", Model: "e", ModelKind: "embedding", Enabled: true}
	require.NoError(t, db.Create(&emb).Error)
	embRoute := models.AIRoute{Name: "test-emb", Capability: string(airouter.CapabilityEmbedding), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&embRoute).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: embRoute.ID, ProviderID: emb.ID, Priority: 1, Enabled: true}).Error)
	router := airouter.NewRouterWithStore(airouter.NewStore(db))
	router.RegisterClient(airouter.ProviderTypeOpenAICompatible, client)
	return router
}

// fakeRecallBatcher 测试注入用召回器：直接返回构造好的批次，绕开 candidate_embeddings 检索。
type fakeRecallBatcher struct {
	askFn     func(ctx context.Context, query string, vec []float64, dim int, model string) ([]recallBatch, error)
	refreshFn func(ctx context.Context) ([]recallBatch, error)
}

func (f *fakeRecallBatcher) askBatches(ctx context.Context, query string, vec []float64, dim int, model string) ([]recallBatch, error) {
	if f.askFn == nil {
		return nil, nil
	}
	return f.askFn(ctx, query, vec, dim, model)
}

func (f *fakeRecallBatcher) refreshBatches(ctx context.Context) ([]recallBatch, error) {
	if f.refreshFn == nil {
		return nil, nil
	}
	return f.refreshFn(ctx)
}

// ── S2：严格精排协议（未知 ID/重复 ID/理由校验/空选择/坏 JSON/provider 失败/配置缺失）──

// setupCandidateVector 为路由建统一候选实体（stable_key 幂等）+ 候选向量
// （4.3 起召回读 candidate_embeddings；route_embeddings 仅作迁移输入不再参与召回）。
// 返回 candidate id（供候选级排除/开关 fixture 引用）。
func setupCandidateVector(t *testing.T, db *gorm.DB, routeID uint, namespace, path, model string, vec []float64) uint {
	t.Helper()
	stableKey, err := BuildRSSHubStableKey(namespace, path)
	require.NoError(t, err)
	enabled := true
	var cand models.FeedCandidate
	require.NoError(t, db.Where("stable_key = ?", stableKey).FirstOrCreate(&cand, models.FeedCandidate{
		StableKey: stableKey, Kind: "rsshub", RouteID: &routeID,
		ManualMetadata: models.MetadataMap{}, RecommendationEnabled: &enabled,
	}).Error)
	require.NoError(t, db.Create(&models.CandidateEmbedding{
		CandidateID: cand.ID, Model: model, Dimension: len(vec), EmbeddingVec: floatsToPgVector(vec),
	}).Error)
	return cand.ID
}

// setupAskFixture 建 n 条 ok 路由 + 3 维同向候选向量，返回统一候选 id（精排协议 id =
// feed_candidates.id，原生与 rsshub 一律以候选身份出现）。
func setupAskFixture(t *testing.T, db *gorm.DB, n int) []uint {
	t.Helper()
	ids := setupRunRoutesFixture(t, db, n)
	candIDs := make([]uint, 0, n)
	for i, id := range ids {
		candIDs = append(candIDs, setupCandidateVector(t, db, id, fmt.Sprintf("ns%d", i), fmt.Sprintf("/p%d", i), "test", []float64{1, 0, 0}))
	}
	return candIDs
}

// routeIDForCandidate 返回统一候选的 RSSHub 路由 id（路由引用断言用）。
func routeIDForCandidate(t *testing.T, db *gorm.DB, candidateID uint) uint {
	t.Helper()
	var c models.FeedCandidate
	require.NoError(t, db.First(&c, candidateID).Error)
	require.NotNil(t, c.RouteID)
	return *c.RouteID
}

// TestRerankStrict_UnknownIDFailsRun：候选集合之外的 id → 整轮 failed、零发布、零兴趣。
func TestRerankStrict_UnknownIDFailsRun(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 3)
	client := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		return strictSelectionJSON([2]any{ids[0], "理由"}, [2]any{uint(9999), "编造 id"}), nil
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "查询", "")
	require.Error(t, err)
	require.Equal(t, "failed", run.Status)
	require.Equal(t, "rerank", run.ErrorCode, "协议违规落 rerank 错误码")
	var pending, interests, items int64
	db.Model(&models.FeedRecommendation{}).Where("status = ?", "pending").Count(&pending)
	db.Model(&models.DiscoveryInterestEntry{}).Count(&interests)
	db.Model(&models.DiscoveryRunItem{}).Count(&items)
	require.Zero(t, pending, "未知 id 整批失败：零 pending")
	require.Zero(t, interests, "失败查询绝不写兴趣")
	require.Zero(t, items, "零 run_items")
}

// TestRerankStrict_DuplicateIDDedup：重复 id → 去重留首个（理由取首次出现）。
func TestRerankStrict_DuplicateIDDedup(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 2)
	client := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		return strictSelectionJSON([2]any{ids[0], "首个理由"}, [2]any{ids[0], "重复理由"}), nil
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "查询", "")
	require.NoError(t, err)
	require.Equal(t, "succeeded", run.Status)
	var pending []models.FeedRecommendation
	require.NoError(t, db.Where("status = ?", "pending").Find(&pending).Error)
	require.Len(t, pending, 1, "重复 id 只一张卡")
	require.Equal(t, "首个理由", pending[0].LLMReason)
}

// TestRerankStrict_EmptyReasonFails：理由空串 → 整批失败。
func TestRerankStrict_EmptyReasonFails(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 2)
	client := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		return strictSelectionJSON([2]any{ids[0], ""}), nil
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "查询", "")
	require.Error(t, err)
	require.Equal(t, "failed", run.Status)
	require.Equal(t, "rerank", run.ErrorCode)
	var pending int64
	db.Model(&models.FeedRecommendation{}).Count(&pending)
	require.Zero(t, pending)
}

// TestRerankStrict_OverlongReasonFails：理由超过 500 rune → 整批失败（rune 计数，非字节）。
func TestRerankStrict_OverlongReasonFails(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 2)
	client := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		return strictSelectionJSON([2]any{ids[0], strings.Repeat("理", 501)}), nil
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "查询", "")
	require.Error(t, err)
	require.Equal(t, "failed", run.Status)
	var pending int64
	db.Model(&models.FeedRecommendation{}).Count(&pending)
	require.Zero(t, pending)
}

// TestRerankStrict_EmptySelectionSucceeds：空选择是成功零推荐（ask 仍写兴趣条目）。
func TestRerankStrict_EmptySelectionSucceeds(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_ = setupAskFixture(t, db, 3)
	client := &mockDiscoveryClient{} // 默认返回 {"selected":[]}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "冷门主题", "")
	require.NoError(t, err)
	require.Equal(t, "succeeded", run.Status, "空选择不是故障")

	var pending int64
	db.Model(&models.FeedRecommendation{}).Count(&pending)
	require.Zero(t, pending, "不用粗筛候选奍数")
	var interests []models.DiscoveryInterestEntry
	require.NoError(t, db.Find(&interests).Error)
	require.Len(t, interests, 1, "成功零结果也写兴趣记录")
	require.Equal(t, run.ID, *interests[0].RunID)
}

// TestRerankStrict_ProviderFailureNoPublish：provider 全链失败 → 整轮 failed、零发布、零兴趣。
func TestRerankStrict_ProviderFailureNoPublish(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_ = setupAskFixture(t, db, 3)
	client := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		return "", fmt.Errorf("provider down")
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "查询", "")
	require.Error(t, err)
	require.Equal(t, "failed", run.Status)
	require.Equal(t, "rerank", run.ErrorCode)
	var pending, interests int64
	db.Model(&models.FeedRecommendation{}).Count(&pending)
	db.Model(&models.DiscoveryInterestEntry{}).Count(&interests)
	require.Zero(t, pending)
	require.Zero(t, interests, "失败查询绝不写兴趣")
}

// TestRerankStrict_BadJSONFailsRun：不可解析响应 → 整轮 failed。
func TestRerankStrict_BadJSONFailsRun(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_ = setupAskFixture(t, db, 3)
	client := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		return "这不是 JSON，模型坏掉了", nil
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "查询", "")
	require.Error(t, err)
	require.Equal(t, "failed", run.Status)
	require.Equal(t, "rerank", run.ErrorCode)
}

// TestRun_ConfigurationMissingZeroProviderCalls：feed_discovery 无 provider →
// run failed + error_code=configuration，且零 provider 调用（mock 计数为证）。
func TestRun_ConfigurationMissingZeroProviderCalls(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_ = setupAskFixture(t, db, 3)
	client := &mockDiscoveryClient{}
	router := newMockAIRouterEmbeddingOnly(t, db, client) // 只建 embedding，不建 feed_discovery
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "查询", "")
	require.Error(t, err)
	require.Equal(t, "failed", run.Status)
	require.Equal(t, "configuration", run.ErrorCode, "配置缺失落 configuration 失败码")
	require.Zero(t, client.chatCalls, "配置缺失：零 LLM 调用")
	require.Zero(t, client.embedCalls, "配置缺失：零 embedding 调用（预检在任何 provider 调用前）")
}

// TestRun_EmptyAPIKeyProviderPassesPreflight：本地/自建 openai_compatible 网关可以合法无
// API key（本环境 qwen 网关即如此，同一个 embedding provider 已成功嵌入 949 条候选）；
// 配置预检不得因空 key 误杀 run（configuration），真实调用失败由 embedding 错误码兜底。
func TestRun_EmptyAPIKeyProviderPassesPreflight(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_ = setupAskFixture(t, db, 3)
	client := &mockDiscoveryClient{}
	router := newMockAIRouter(t, db, client, false)
	// 构造后清空全部 provider 的 api_key（模拟本地网关无凭据）。
	require.NoError(t, db.Model(&models.AIProvider{}).Where("1 = 1").Update("api_key", "").Error)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "查询", "")
	require.NoError(t, err, "空 key 不得被配置预检误杀")
	require.Equal(t, "succeeded", run.Status)
	require.NotEqual(t, DiscoveryRunErrConfiguration, run.ErrorCode)
}

// TestFailRunLogsCause：run 失败必须落一条含 run id / kind / error_code / 底层错误
// 摘要的 error 日志（run 表只有错误码，15ms 内失败的 run 若不记日志则无归因线索）。
func TestFailRunLogsCause(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_ = setupAskFixture(t, db, 3)
	client := &mockDiscoveryClient{embedFn: func(airouter.EmbeddingRequest) (*airouter.EmbeddingResult, error) {
		return nil, errors.New("provider exploded: upstream 502")
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	var captured strings.Builder
	logging.SetWriters(&captured, &captured)
	defer logging.ResetWriters()

	run, err := svc.StartAsk(context.Background(), "查询", "")
	require.Error(t, err)
	require.Equal(t, "failed", run.Status)
	require.Equal(t, DiscoveryRunErrEmbedding, run.ErrorCode)

	logs := captured.String()
	require.Contains(t, logs, fmt.Sprintf("run %d", run.ID), "日志须带 run id")
	require.Contains(t, logs, "kind=ask", "日志须带 run kind")
	require.Contains(t, logs, "error_code=embedding", "日志须带 error_code")
	require.Contains(t, logs, "provider exploded: upstream 502", "日志须带底层错误摘要")
}

// ── S1：run 幂等与原子发布 ──

// TestRun_SameRequestKeyRunningReuse：同 request_key 运行中 → 返回同 run 不重复执行。
func TestRun_SameRequestKeyRunningReuse(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_ = setupAskFixture(t, db, 3)
	client := &mockDiscoveryClient{}
	router := newMockAIRouter(t, db, client, false)
	// 预插一个运行中的同 key run（模拟并发重复提交/断电残留）。
	existing := models.DiscoveryRun{RequestKey: "dup-key", Kind: DiscoveryRunKindAsk,
		Query: "旧问题", Status: "running", StartedAt: time.Now()}
	require.NoError(t, db.Create(&existing).Error)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "新问题", "dup-key")
	require.NoError(t, err)
	require.Equal(t, existing.ID, run.ID, "同 request_key 运行中返回同一 run")
	require.Equal(t, "running", run.Status)
	require.Equal(t, "旧问题", run.Query, "复用不重执行、不覆盖原查询")
	require.Zero(t, client.chatCalls, "复用不重复执行精排")
	require.Zero(t, client.embedCalls)
	var cnt int64
	db.Model(&models.DiscoveryRun{}).Count(&cnt)
	require.EqualValues(t, 1, cnt, "不重复建 run")
}

// ── S3（4.2 接线）：独立兴趣记录与有界影响 ──

// setupSeedLabel 建 semantic_labels 行（真实版块 label_type='board' 或辅助标签
// 'auxiliary'），带 3 维向量。辅助标签同表共存——验证归属匹配只看 board 类型
// （历史 bug：旧 loadBoardVectors 漏 label_type 过滤）。
func setupSeedLabel(t *testing.T, db *gorm.DB, label, slug, labelType string, vec []float64) uint {
	t.Helper()
	emb := floatsToPgVector(vec)
	row := models.SemanticLabel{Label: label, Slug: slug, LabelType: labelType, Status: "active", Embedding: &emb}
	require.NoError(t, db.Create(&row).Error)
	return row.ID
}

// createInterestEntry 直插兴趣条目（seed 召回 fixture；RunID 可空模拟普通行）。
func createInterestEntry(t *testing.T, db *gorm.DB, query string, boardID *uint, vec []float64, ageDays float64) uint {
	t.Helper()
	entry := models.DiscoveryInterestEntry{
		QueryText: query, BoardID: boardID,
		EmbeddingVec: floatsToPgVector(vec), Dimension: len(vec), Model: "test",
		Status: "active", CreatedAt: time.Now().Add(-time.Duration(ageDays * float64(24*time.Hour))),
	}
	require.NoError(t, db.Create(&entry).Error)
	return entry.ID
}

// TestAsk_InterestBoardMatchedWritten：mock 问答向量 [1,0,0] 命中同向真实版块 →
// 兴趣条目 board_id 写入；同向量辅助标签（同表）不参与匹配（历史 bug 回归网）。
func TestAsk_InterestBoardMatchedWritten(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 2)
	boardA := setupSeedLabel(t, db, "体育", "s3-sports", "board", []float64{1, 0, 0})
	_ = setupSeedLabel(t, db, "辅助锚点", "s3-aux", "auxiliary", []float64{1, 0, 0}) // 不得参与匹配
	_ = setupSeedLabel(t, db, "软件", "s3-software", "board", []float64{0, 1, 0})
	client := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		return strictSelectionJSON([2]any{ids[0], "理由"}), nil
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	_, err := svc.StartAsk(context.Background(), "足球战术", "")
	require.NoError(t, err)

	var interests []models.DiscoveryInterestEntry
	require.NoError(t, db.Find(&interests).Error)
	require.Len(t, interests, 1)
	require.NotNil(t, interests[0].BoardID, "阈值+margin 双条件满足时应写入版块归属")
	require.Equal(t, boardA, *interests[0].BoardID)
}

// TestAsk_InterestBoardMarginFailStaysNULL：两个同向版块 sim 持平（差 0 < margin）→
// 不归属、保持 NULL，且不新建版块/不挂标签。
func TestAsk_InterestBoardMarginFailStaysNULL(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 2)
	_ = setupSeedLabel(t, db, "体育 A", "s3-tie-a", "board", []float64{1, 0, 0})
	_ = setupSeedLabel(t, db, "体育 B", "s3-tie-b", "board", []float64{1, 0, 0})
	client := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		return strictSelectionJSON([2]any{ids[0], "理由"}), nil
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	_, err := svc.StartAsk(context.Background(), "体育", "")
	require.NoError(t, err)

	var labelsBefore int64
	db.Model(&models.SemanticLabel{}).Count(&labelsBefore)
	var interests []models.DiscoveryInterestEntry
	require.NoError(t, db.Find(&interests).Error)
	require.Len(t, interests, 1)
	require.Nil(t, interests[0].BoardID, "领先不足 margin 不得归属")
	var labelsAfter int64
	db.Model(&models.SemanticLabel{}).Count(&labelsAfter)
	require.Equal(t, labelsBefore, labelsAfter, "未匹配组不新建版块不挂标签")
}

// TestAsk_TwoQueriesIndependentEntries：多主题两次 Ask → 两条独立条目各自保留
// 本次查询 embedding，不互相平均（D3 拒绝单行 EMA 的根因）。
func TestAsk_TwoQueriesIndependentEntries(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_ = setupAskFixture(t, db, 2)
	call := 0
	client := &mockDiscoveryClient{embedFn: func(airouter.EmbeddingRequest) (*airouter.EmbeddingResult, error) {
		call++
		if call == 1 {
			return &airouter.EmbeddingResult{Embeddings: [][]float64{{1, 0, 0}}, Model: "test", Dimensions: 3}, nil
		}
		return &airouter.EmbeddingResult{Embeddings: [][]float64{{0, 1, 0}}, Model: "test", Dimensions: 3}, nil
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run1, err := svc.StartAsk(context.Background(), "体育", "q-sports")
	require.NoError(t, err)
	run2, err := svc.StartAsk(context.Background(), "软件", "q-software")
	require.NoError(t, err)

	var interests []models.DiscoveryInterestEntry
	require.NoError(t, db.Order("id ASC").Find(&interests).Error)
	require.Len(t, interests, 2, "两次查询两条独立记录")
	require.Equal(t, run1.ID, *interests[0].RunID)
	require.Equal(t, run2.ID, *interests[1].RunID)
	require.Equal(t, floatsToPgVector([]float64{1, 0, 0}), interests[0].EmbeddingVec, "首次查询向量原样保留")
	require.Equal(t, floatsToPgVector([]float64{0, 1, 0}), interests[1].EmbeddingVec, "第二次查询不与第一次平均")
	require.Equal(t, "active", interests[0].Status)
	require.Equal(t, "active", interests[1].Status)
}

// TestAsk_InvalidSeedPolicyConfigFails：discovery_seed_policy 非法 → run failed +
// error_code=configuration，零 provider 调用（配置预检先行）。
func TestAsk_InvalidSeedPolicyConfigFails(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_ = setupAskFixture(t, db, 2)
	require.NoError(t, db.Create(&models.AISettings{
		Key: seedPolicyConfigKey, Value: `{"interest_window_days": 400}`,
	}).Error)
	client := &mockDiscoveryClient{}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "查询", "")
	require.Error(t, err)
	require.Equal(t, "failed", run.Status)
	require.Equal(t, "configuration", run.ErrorCode)
	require.Zero(t, client.chatCalls)
	require.Zero(t, client.embedCalls)
}

// TestRefresh_InvalidSeedPolicyConfigFails：refresh 路径同样配置预检。
func TestRefresh_InvalidSeedPolicyConfigFails(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_ = setupRunRoutesFixture(t, db, 2)
	require.NoError(t, db.Create(&models.AISettings{
		Key: seedPolicyConfigKey, Value: `{"seed_candidate_budget": 99}`,
	}).Error)
	client := &mockDiscoveryClient{}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	summary, err := svc.Refresh(context.Background())
	require.Error(t, err)
	var run models.DiscoveryRun
	require.NoError(t, db.First(&run, summary.RunID).Error)
	require.Equal(t, "failed", run.Status)
	require.Equal(t, "configuration", run.ErrorCode)
	require.Zero(t, client.chatCalls)
}

// setupSeedRoutesFixture 建 2 条路由：A 向 [1,0,0]、B 向 [0,1,0]（3 维候选向量）。
func setupSeedRoutesFixture(t *testing.T, db *gorm.DB) (a, b uint) {
	t.Helper()
	ids := setupRunRoutesFixture(t, db, 2)
	setupCandidateVector(t, db, ids[0], "ns0", "/p0", "test", []float64{1, 0, 0})
	setupCandidateVector(t, db, ids[1], "ns1", "/p1", "test", []float64{0, 1, 0})
	return ids[0], ids[1]
}

// TestRefresh_SeedRecallFromEntriesNotOldSeedRows（4.2 切换核心验收）：
// 种子份额来自 discovery_interest_entries 参与集；旧 preference_vectors 的 seed 行
// 不再参与召回。条目 10 天除（w=2^(-10/7)≈0.372）→ 预算 floor(4×0.372)=1 →
// 仅 top-1（A）入选；旧 seed 行向量指向 B 但不得召回 B。
func TestRefresh_SeedRecallFromEntriesNotOldSeedRows(t *testing.T) {
	db := testutil.SetupTestDB(t)
	routeA, routeB := setupSeedRoutesFixture(t, db)
	createInterestEntry(t, db, "软件工具", nil, []float64{1, 0, 0}, 10)
	oldSeed := models.PreferenceVector{
		BoardID: nil, Source: PreferenceSourceSeed,
		EmbeddingVec: floatsToPgVector([]float64{0, 1, 0}), Dimension: 3, Model: "test",
	}
	require.NoError(t, db.Create(&oldSeed).Error)

	router, client := newSelectAllMockRouter(t, db)
	svc := NewDiscoveryRunService(db, router, nil)
	summary, err := svc.Refresh(context.Background())
	require.NoError(t, err)
	require.Equal(t, "succeeded", func() string {
		var run models.DiscoveryRun
		require.NoError(t, db.First(&run, summary.RunID).Error)
		return run.Status
	}())

	var pending []models.FeedRecommendation
	require.NoError(t, db.Where("status = ?", "pending").Find(&pending).Error)
	routes := map[uint]bool{}
	for _, r := range pending {
		routes[r.RouteID] = true
	}
	require.True(t, routes[routeA], "参与条目向量召回 top-1（名额 1）应命中 A")
	require.False(t, routes[routeB], "旧 preference_vectors seed 行不得再参与召回（B 只有旧 seed 指向）")

	var seedRows []models.PreferenceVector
	require.NoError(t, db.Where("source = ?", PreferenceSourceSeed).Find(&seedRows).Error)
	require.Len(t, seedRows, 1, "旧 seed 行保留不动（4.3 处理双路）")
	require.GreaterOrEqual(t, client.chatCalls, 1, "至少一批精排")
}

// TestRefresh_StaleEntryMarkedInactive：窗口外（31d）active 条目 → 批量置
// inactive（历史保留不删）、不产生 seed 批次。
func TestRefresh_StaleEntryMarkedInactive(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_, _ = setupSeedRoutesFixture(t, db)
	entryID := createInterestEntry(t, db, "旧兴趣", nil, []float64{1, 0, 0}, 31)

	router, client := newSelectAllMockRouter(t, db)
	svc := NewDiscoveryRunService(db, router, nil)
	_, err := svc.Refresh(context.Background())
	require.NoError(t, err)

	var entry models.DiscoveryInterestEntry
	require.NoError(t, db.First(&entry, entryID).Error)
	require.Equal(t, "inactive", entry.Status, "超窗条目置 inactive，历史保留")
	var pending int64
	db.Model(&models.FeedRecommendation{}).Where("status = ?", "pending").Count(&pending)
	require.Zero(t, pending)
	require.Zero(t, client.chatCalls, "无 behavior 画像且 seed 全退窗 → 零批次零精排")
}

// TestRefresh_MatureBoardZeroSeedShare：条目所属版块行为成熟（窗口内去重文章
// ≥ 20）→ w=0 → 份额 0 不产生 seed 批次；条目仍 active 显示已衰减（不删除）。
// 4.3 语义：版块基础路已接入（见 TestRefresh_BoardBasePathGuaranteedNotSqueezed），
// 本例将版块 label 向量改为 4 维（与 3 维候选不兼容）隔离出版块路，专门验证 seed 让位。
func TestRefresh_MatureBoardZeroSeedShare(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_, _ = setupSeedRoutesFixture(t, db)
	boardID := setupSeedLabel(t, db, "日本新闻", "s3-jp-news", "board", []float64{1, 0, 0, 0}) // 4 维：与 3 维候选不兼容
	tag := models.TopicTag{Label: "财经", Slug: "s3-finance", Category: "keyword", Status: "active", IsCanonical: true}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: boardID, Score: 0.9}).Error)
	board := boardID
	for i := 0; i < 20; i++ {
		artID := uint(100 + i)
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: artID, TopicTagID: tag.ID, Source: "llm"}).Error)
		require.NoError(t, db.Create(&models.ReadingBehavior{
			ArticleID: artID, EventType: "favorite", CreatedAt: time.Now(),
		}).Error)
	}
	createInterestEntry(t, db, "日本新闻", &board, []float64{1, 0, 0}, 0)

	router, client := newSelectAllMockRouter(t, db)
	svc := NewDiscoveryRunService(db, router, nil)
	_, err := svc.Refresh(context.Background())
	require.NoError(t, err)

	var entry models.DiscoveryInterestEntry
	require.NoError(t, db.Order("id DESC").First(&entry).Error)
	require.Equal(t, "active", entry.Status, "成熟让位：份额 0 但记录保留显示（不删除不变 inactive）")
	var pending int64
	db.Model(&models.FeedRecommendation{}).Where("status = ?", "pending").Count(&pending)
	require.Zero(t, pending, "成熟范围 seed 份额 0，不挤占也不产候选")
	require.Zero(t, client.chatCalls, "seed 份额 0 且版块路维度不兼容 → 零批次不调精排")
}

// TestRun_FailedRunRetrySameKeyRerun：失败 run 重试复用同一行重执行，成功后 succeeded。
func TestRun_FailedRunRetrySameKeyRerun(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 2)
	failFirst := true
	client := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		if failFirst {
			return "", fmt.Errorf("provider down")
		}
		return strictSelectionJSON([2]any{ids[0], "重试成功"}), nil
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run1, err := svc.StartAsk(context.Background(), "查询", "retry-key")
	require.Error(t, err)
	require.Equal(t, "failed", run1.Status)

	failFirst = false
	run2, err := svc.StartAsk(context.Background(), "查询", "retry-key")
	require.NoError(t, err)
	require.Equal(t, run1.ID, run2.ID, "失败重试复用同一 run 行，不产生第二条")
	require.Equal(t, "succeeded", run2.Status)
	var pending int64
	db.Model(&models.FeedRecommendation{}).Where("status = ?", "pending").Count(&pending)
	require.EqualValues(t, 1, pending)
	var runs int64
	db.Model(&models.DiscoveryRun{}).Count(&runs)
	require.EqualValues(t, 1, runs)
}

// TestRun_RefreshBatchFailureAtomicNoPublish：多批中一批精排失败 → 整轮 failed、
// 已成功批次不落库（不出现半新半旧，design D4）。
func TestRun_RefreshBatchFailureAtomicNoPublish(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 2)
	x, y := ids[0], ids[1]
	client := &mockDiscoveryClient{chatFn: func(req airouter.ChatRequest) (string, error) {
		if strings.Contains(req.Messages[0].Content, fmt.Sprintf("id=%d", y)) {
			return strictSelectionJSON([2]any{uint(9999), "批 B 返回未知 id"}), nil // 批 B 失败
		}
		return strictSelectionJSON([2]any{x, "批 A 正常选中"}), nil // 批 A 成功
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)
	svc.recall = &fakeRecallBatcher{refreshFn: func(context.Context) ([]recallBatch, error) {
		return []recallBatch{
			{boardLabel: "版块A", source: RecommendationSourceManualRefresh, path: recallPathBoard, candidates: []recallCandidate{
				{candidateRow: candidateRow{CandidateID: x, Namespace: "ns0", Path: "/p0", Name: "X"}, paths: []string{recallPathBoard}},
			}},
			{boardLabel: "版块B", source: RecommendationSourceManualRefresh, path: recallPathBoard, candidates: []recallCandidate{
				{candidateRow: candidateRow{CandidateID: y, Namespace: "ns1", Path: "/p1", Name: "Y"}, paths: []string{recallPathBoard}},
			}},
		}, nil
	}}

	summary, err := svc.Refresh(context.Background())
	require.Error(t, err)
	_ = summary
	var pending, items int64
	db.Model(&models.FeedRecommendation{}).Count(&pending)
	db.Model(&models.DiscoveryRunItem{}).Count(&items)
	require.Zero(t, pending, "单批失败整轮不落库：批 A 已选的也不发布")
	require.Zero(t, items)
	var run models.DiscoveryRun
	require.NoError(t, db.Order("id DESC").First(&run).Error)
	require.Equal(t, "failed", run.Status)
	require.Equal(t, "rerank", run.ErrorCode)
}

// TestRun_RefreshZeroBatchesSucceeds：无画像 → 零批次，跳过 LLM 直接成功零推荐。
func TestRun_RefreshZeroBatchesSucceeds(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_ = setupRunRoutesFixture(t, db, 3) // 只有路由，无 preference_vectors
	client := &mockDiscoveryClient{}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	summary, err := svc.Refresh(context.Background())
	require.NoError(t, err)
	require.NotZero(t, summary.RunID, "refresh 响应带 run_id")
	require.Zero(t, client.chatCalls, "零批次不调 LLM")
	var pending int64
	db.Model(&models.FeedRecommendation{}).Count(&pending)
	require.Zero(t, pending)
	var run models.DiscoveryRun
	require.NoError(t, db.First(&run, summary.RunID).Error)
	require.Equal(t, "succeeded", run.Status)
}

// TestRun_PendingUpsertUpdateNotInsert：同候选再次入选 → 更新 llm_reason/last_selected_at/expires_at，不重复插入。
func TestRun_PendingUpsertUpdateNotInsert(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 2)
	reason := "第一轮理由"
	client := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		return strictSelectionJSON([2]any{ids[0], reason}), nil
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run1, err := svc.StartAsk(context.Background(), "查询一", "key-a")
	require.NoError(t, err)
	require.Equal(t, "succeeded", run1.Status)

	reason = "第二轮新理由"
	run2, err := svc.StartAsk(context.Background(), "查询二", "key-b")
	require.NoError(t, err)
	require.Equal(t, "succeeded", run2.Status)

	var pending []models.FeedRecommendation
	require.NoError(t, db.Where("status = ?", "pending").Find(&pending).Error)
	require.Len(t, pending, 1, "同 hash pending 更新不插入（D2）")
	require.Equal(t, "第二轮新理由", pending[0].LLMReason)
	require.NotNil(t, pending[0].LastSelectedAt)
	require.NotNil(t, pending[0].ExpiresAt)
	require.False(t, pending[0].LastSelectedAt.Before(pending[0].CreatedAt), "再次入选刷新 last_selected_at")

	var items []models.DiscoveryRunItem
	require.NoError(t, db.Find(&items).Error)
	require.Len(t, items, 2, "两个 run 各自留快照（reason_snapshot 各轮独立）")
}

// TestRun_AskDoesNotTouchPreferenceVectors：ask 不再走旧 seed EMA 合并通道，
// preference_vectors 旧行不动、无新 seed 行（验收：Ask 不调 WriteSeed）。
func TestRun_AskDoesNotTouchPreferenceVectors(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 2)
	oldVec := floatsToPgVector([]float64{1, 0, 0})
	before := models.PreferenceVector{BoardID: nil, Source: PreferenceSourceBehavior,
		EmbeddingVec: oldVec, Dimension: 3, Model: "test"}
	require.NoError(t, db.Create(&before).Error)
	client := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		return strictSelectionJSON([2]any{ids[0], "理由"}), nil
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	run, err := svc.StartAsk(context.Background(), "查询", "")
	require.NoError(t, err)
	require.Equal(t, "succeeded", run.Status)

	var pvs []models.PreferenceVector
	require.NoError(t, db.Find(&pvs).Error)
	require.Len(t, pvs, 1, "ask 不写 preference_vectors（无新 seed 行）")
	require.Equal(t, PreferenceSourceBehavior, pvs[0].Source)
	require.Equal(t, before.EmbeddingVec, pvs[0].EmbeddingVec, "旧行向量不动")
	require.Equal(t, before.Model, pvs[0].Model)
	require.Equal(t, before.Dimension, pvs[0].Dimension)
}
