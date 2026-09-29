package airouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
)

// fakeChatReply configures fakeProviderClient.Chat for one vendor name.
type fakeChatReply struct {
	content string
	usage   *TokenUsage
	err     error
	// nilResponse makes Chat return (nil, nil): transport success with no
	// usable payload — the shape that must still trigger ordered fallback.
	nilResponse bool
	// attempts 透传模拟 client 内降级链产物（Router 须逐失败档写审计行）。
	attempts []StructuredAttempt
	// embedUsage feeds the fake Embed result so cache/log tests can exercise
	// usage-bearing embedding calls.
	embedUsage *TokenUsage
}

// fakeChatCall records one Chat invocation's binding identity + effective
// thinking level, so thinking-level resolution can be asserted end-to-end.
type fakeChatCall struct {
	vendorName string
	modelName  string
	effLevel   string
}

type fakeProviderClient struct {
	responses map[string]fakeChatReply
	chatCalls []fakeChatCall
}

func (f *fakeProviderClient) Chat(_ context.Context, vendor models.AIVendor, model models.AIModel, effLevel string, _ ChatRequest) (*ChatResponse, []StructuredAttempt, error) {
	f.chatCalls = append(f.chatCalls, fakeChatCall{vendorName: vendor.Name, modelName: model.Name, effLevel: effLevel})
	res := f.responses[vendor.Name]
	if res.err != nil {
		return nil, res.attempts, res.err
	}
	if res.nilResponse {
		return nil, res.attempts, nil
	}
	return &ChatResponse{Content: res.content, Usage: res.usage}, res.attempts, nil
}

func (f *fakeProviderClient) Embed(_ context.Context, vendor models.AIVendor, model models.AIModel, _ EmbeddingRequest) (*EmbeddingResult, error) {
	res := f.responses[vendor.Name]
	if res.err != nil {
		return nil, res.err
	}
	return &EmbeddingResult{Embeddings: [][]float64{{0.1, 0.2}}, Model: model.Name, Dimensions: 2, Provider: vendor.Name, Usage: res.embedUsage}, nil
}

// chatFixture 组装一份 vendor+model 绑定夹具（Router 数据面已换 LoadRouteWithModels）。
type chatFixture struct {
	vendor models.AIVendor
	model  models.AIModel
}

// seedChatBinding 建 vendor+model 并把绑定追加到 default summary 路由（路由不存在则建）。
func seedChatBinding(t *testing.T, db *gorm.DB, route *models.AIRoute, vendorName, modelName string,
	mutateVendor func(*models.AIVendor), mutateModel func(*models.AIModel), thinkingLevel string, priority int) chatFixture {
	t.Helper()
	vendor := models.AIVendor{Name: vendorName, ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "https://" + vendorName + ".example/v1", APIKey: "k", Enabled: true,
		ThinkingDialect: "none", StructuredOutput: StructOutputJSONSchema}
	if mutateVendor != nil {
		mutateVendor(&vendor)
	}
	require.NoError(t, db.Create(&vendor).Error)

	model := models.AIModel{VendorID: vendor.ID, Name: modelName, ModelKind: ModelKindLLM,
		Enabled: true, Status: ModelStatusOK}
	if mutateModel != nil {
		mutateModel(&model)
	}
	require.NoError(t, db.Create(&model).Error)

	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: model.ID,
		Priority: priority, Enabled: true, ThinkingLevel: levelPtr(thinkingLevel)}).Error)
	return chatFixture{vendor: vendor, model: model}
}

// seedDefaultSummaryRoute 建 default summary 路由（幂等：调用方每测试建一次）。
func seedDefaultSummaryRoute(t *testing.T, db *gorm.DB) models.AIRoute {
	t.Helper()
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true}
	require.NoError(t, db.Create(&route).Error)
	return route
}

func TestRouterFallsBackOnRetryableProviderError(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	route := seedDefaultSummaryRoute(t, db)
	seedChatBinding(t, db, &route, "primary", "m1", nil, nil, "", 1)
	backup := seedChatBinding(t, db, &route, "backup", "m2", nil, nil, "", 2)

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"primary": {err: &ProviderError{Message: "rate limited", Code: "rate_limit", Retryable: true}},
		"backup":  {content: "ok from backup"},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.op",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "ok from backup", result.Content)
	require.True(t, result.UsedFallback)
	require.Equal(t, 2, result.AttemptCount)
	// vendor/model 新来源语义：ProviderName/ProviderID 承载 vendor，ModelName 承载模型。
	require.Equal(t, "backup", result.ProviderName)
	require.Equal(t, backup.vendor.ID, result.ProviderID)
	require.Equal(t, backup.model.Name, result.ModelName)
}

func TestRouterFallsBackOnTerminalError(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	route := seedDefaultSummaryRoute(t, db)
	seedChatBinding(t, db, &route, "primary-terminal", "m1", nil, nil, "", 1)
	seedChatBinding(t, db, &route, "backup", "m2", nil, nil, "", 2)

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"primary-terminal": {err: &ProviderError{Message: "invalid key", Code: "unauthorized", Retryable: false}},
		"backup":           {content: "ok from backup"},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.op",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "ok from backup", result.Content)
	require.True(t, result.UsedFallback)
	require.Equal(t, 2, result.AttemptCount)
}

func TestRouterChatRejectsEmptyOperation(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	route := seedDefaultSummaryRoute(t, db)
	seedChatBinding(t, db, &route, "p", "m", nil, nil, "", 1)

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"p": {content: "ok"},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Operation is required")
	require.Nil(t, result)
}

func TestRouterEmbedRejectsEmptyOperation(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	setupEmbedRoute(t, store, "emb-m")

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"emb-vendor": {content: "ok"},
	}})

	result, err := router.Embed(context.Background(), EmbeddingRequest{Input: []string{"test"}}, CapabilityEmbedding)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Operation is required")
	require.Nil(t, result)
}

func TestFormatMessages(t *testing.T) {
	t.Run("simple messages", func(t *testing.T) {
		result := formatMessages([]Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		})
		require.Equal(t, "[system]\nYou are helpful.\n\n[user]\nHello", result)
	})

	t.Run("single message", func(t *testing.T) {
		result := formatMessages([]Message{
			{Role: "user", Content: "hi"},
		})
		require.Equal(t, "[user]\nhi", result)
	})

	t.Run("truncation over 20000 runes", func(t *testing.T) {
		bigContent := make([]rune, 21000)
		for i := range bigContent {
			bigContent[i] = 'a'
		}
		result := formatMessages([]Message{
			{Role: "user", Content: string(bigContent)},
		})
		require.Contains(t, result, "[truncated")
		require.Contains(t, result, "[user]\n")
		// Result should be shorter than raw input (21000 + marker overhead),
		// but can be slightly above 20000 due to truncation marker.
		runes := []rune(result)
		require.Less(t, len(runes), 21000, "truncated result should be shorter than raw input")
	})

	t.Run("under limit no truncation", func(t *testing.T) {
		result := formatMessages([]Message{
			{Role: "user", Content: "short"},
		})
		require.NotContains(t, result, "[truncated")
	})
}

func TestTokenUsageInChatResult(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	usage := &TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}

	route := seedDefaultSummaryRoute(t, db)
	seedChatBinding(t, db, &route, "p-usage", "m", nil, nil, "", 1)

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"p-usage": {content: "response", usage: usage},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.op",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Usage)
	require.Equal(t, 100, result.Usage.PromptTokens)
	require.Equal(t, 50, result.Usage.CompletionTokens)
	require.Equal(t, 150, result.Usage.TotalTokens)
}

func TestEncodeTokenUsage(t *testing.T) {
	require.Equal(t, "", encodeTokenUsage(nil))

	u := &TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}
	encoded := encodeTokenUsage(u)
	require.Contains(t, encoded, "\"prompt\":10")
	require.Contains(t, encoded, "\"completion\":20")
	require.Contains(t, encoded, "\"total\":30")
}

// seedSingleChatBinding 建单个 vendor+model 的 default summary 路由（旧 seedSingleProvider 等价物）。
func seedSingleChatBinding(t *testing.T, db *gorm.DB, vendorName string) {
	t.Helper()
	route := seedDefaultSummaryRoute(t, db)
	seedChatBinding(t, db, &route, vendorName, "m", nil, nil, "", 1)
}

// tokenUsageIsNull asserts the stored row's token_usage column is SQL NULL
// (not an empty string, not an all-zero jsonb).
func tokenUsageIsNull(t *testing.T, db *gorm.DB, logID uint) {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.AICallLog{}).Where("id = ? AND token_usage IS NULL", logID).Count(&n).Error)
	require.Equal(t, int64(1), n, "token_usage must be SQL NULL")
}

func TestTokenUsageRecordedInCallLog(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	seedSingleChatBinding(t, db, "p-log-usage")

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"p-log-usage": {content: "ok", usage: &TokenUsage{PromptTokens: 9, CompletionTokens: 12, TotalTokens: 21}},
	}})

	_, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.log_usage",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)

	var log models.AICallLog
	require.NoError(t, db.First(&log, "operation = ? AND success = true", "test.log_usage").Error)
	// Stored shape stays prompt/completion/total (consumer contract), values real.
	require.Contains(t, log.TokenUsage, "\"prompt\":9")
	require.Contains(t, log.TokenUsage, "\"completion\":12")
	require.Contains(t, log.TokenUsage, "\"total\":21")
}

func TestChatWithoutUsageLogsNullTokenUsage(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	seedSingleChatBinding(t, db, "p-no-usage")

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"p-no-usage": {content: "ok, but provider sent no usage block"},
	}})

	_, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.no_usage",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)

	var log models.AICallLog
	require.NoError(t, db.First(&log, "operation = ? AND success = true", "test.no_usage").Error)
	require.Empty(t, log.TokenUsage)
	tokenUsageIsNull(t, db, log.ID)
}

func TestChatFailureLogsNullTokenUsage(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	seedSingleChatBinding(t, db, "p-fail")

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"p-fail": {err: &ProviderError{Message: "boom", Code: "boom", Retryable: false}},
	}})

	_, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.fail_usage",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	require.Error(t, err)

	var log models.AICallLog
	require.NoError(t, db.First(&log, "operation = ? AND success = false", "test.fail_usage").Error)
	require.Empty(t, log.TokenUsage)
	tokenUsageIsNull(t, db, log.ID)
}

func TestEmbedUsageRecordedInCallLog(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	setupEmbedRoute(t, store, "m1")

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"emb-vendor": {embedUsage: &TokenUsage{PromptTokens: 7, TotalTokens: 7}},
	}})

	_, err := router.Embed(context.Background(), EmbeddingRequest{
		Input:     []string{"hello"},
		Operation: "test.embed_usage",
	}, CapabilityEmbedding)
	require.NoError(t, err)

	var log models.AICallLog
	require.NoError(t, db.First(&log, "operation = ? AND success = true", "test.embed_usage").Error)
	require.Contains(t, log.TokenUsage, "\"prompt\":7")
	require.Contains(t, log.TokenUsage, "\"total\":7")
}

// seedChatFailover seeds a two-binding ordered_failover route for the summary
// capability (primary-empty → backup) and returns the router plus the db
// handle for ai_call_logs assertions.
func seedChatFailover(t *testing.T) (*Router, *gorm.DB) {
	t.Helper()
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	route := seedDefaultSummaryRoute(t, db)
	seedChatBinding(t, db, &route, "primary-empty", "m1", nil, nil, "", 1)
	seedChatBinding(t, db, &route, "backup", "m2", nil, nil, "", 2)
	return NewRouterWithStore(store), db
}

// chatCallLogs loads all ai_call_logs rows in insert order.
func chatCallLogs(t *testing.T, db *gorm.DB) []models.AICallLog {
	t.Helper()
	var logs []models.AICallLog
	require.NoError(t, db.Order("id ASC").Find(&logs).Error)
	return logs
}

// TestRouterChatFallsBackOnEmptyResponse covers the real-world shape observed
// on glm board_synthesize: HTTP 200 but assistant content "" (or blank). The
// router must log the primary attempt as an empty_response failure (never a
// success row), then fall through ordered fallback to the backup binding,
// keeping the existing prompt/session/operation logging contract.
func TestRouterChatFallsBackOnEmptyResponse(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "empty_string", content: ""},
		{name: "whitespace_only", content: " \n\t "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, db := seedChatFailover(t)
			router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
				"primary-empty": {content: tc.content},
				"backup":        {content: "ok from backup"},
			}})

			result, err := router.Chat(context.Background(), ChatRequest{
				Operation:  "test.op",
				Capability: CapabilitySummary,
				SessionID:  "sess-empty",
				Messages:   []Message{{Role: "user", Content: "hi"}},
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "ok from backup", result.Content)
			require.True(t, result.UsedFallback)
			require.Equal(t, 2, result.AttemptCount)

			logs := chatCallLogs(t, db)
			require.Len(t, logs, 2)

			primary := logs[0]
			require.Equal(t, "primary-empty", primary.ProviderName)
			require.Equal(t, "m1", primary.Model)
			require.False(t, primary.Success)
			require.False(t, primary.IsFallback)
			require.Equal(t, "empty_response", primary.ErrorCode)
			require.NotEmpty(t, primary.ErrorMessage)
			// Existing failure-row logging contract must not regress.
			require.Equal(t, "test.op", primary.Operation)
			require.Equal(t, string(CapabilitySummary), primary.Capability)
			require.Equal(t, "[user]\nhi", primary.Prompt)
			require.Equal(t, "sess-empty", primary.SessionID)

			backup := logs[1]
			require.Equal(t, "backup", backup.ProviderName)
			require.Equal(t, "m2", backup.Model)
			require.True(t, backup.Success)
			require.True(t, backup.IsFallback)
			require.Equal(t, "ok from backup", backup.ResponseSnippet)
			require.Equal(t, "test.op", backup.Operation)
			require.Equal(t, "sess-empty", backup.SessionID)
		})
	}
}

// TestRouterChatNilResponseFallsBack: a provider client returning (nil, nil)
// must not panic and must still be treated as empty_response + fallback.
func TestRouterChatNilResponseFallsBack(t *testing.T) {
	router, db := seedChatFailover(t)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"primary-empty": {nilResponse: true},
		"backup":        {content: "ok from backup"},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.op",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "ok from backup", result.Content)
	require.True(t, result.UsedFallback)
	require.Equal(t, 2, result.AttemptCount)

	logs := chatCallLogs(t, db)
	require.Len(t, logs, 2)
	require.False(t, logs[0].Success)
	require.Equal(t, "empty_response", logs[0].ErrorCode)
	require.True(t, logs[1].Success)
}

// TestRouterChatAllBindingsEmptyReturnsError: when every binding yields an
// empty response, Chat must return an error naming each vendor plus the
// empty-response cause, with a nil result and no success log rows.
func TestRouterChatAllProvidersEmptyReturnsError(t *testing.T) {
	router, db := seedChatFailover(t)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"primary-empty": {content: " "},
		"backup":        {content: ""},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.op",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "primary-empty")
	require.Contains(t, err.Error(), "backup")
	require.Contains(t, err.Error(), "empty response")

	logs := chatCallLogs(t, db)
	require.Len(t, logs, 2)
	for _, row := range logs {
		require.False(t, row.Success, "no provider may log success on empty content")
		require.Equal(t, "empty_response", row.ErrorCode)
	}
}

// TestRouterChatPaddedContentStillSucceeds: whitespace around non-empty
// content must not count as an empty response. 非 off 生效档（默认 medium）按
// specs/ai-thinking-levels「输出清理跟随思考档」剥离 <think> 标签后 TrimSpace，
// 故返回内容为首尾空白剥除后的文本。
func TestRouterChatPaddedContentStillSucceeds(t *testing.T) {
	router, db := seedChatFailover(t)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"primary-empty": {content: "  padded answer  "},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.op",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "padded answer", result.Content)
	require.False(t, result.UsedFallback)
	require.Equal(t, 1, result.AttemptCount)

	logs := chatCallLogs(t, db)
	require.Len(t, logs, 1)
	require.True(t, logs[0].Success)
	require.Equal(t, "", logs[0].ErrorCode)
}

// ─── 思考档四级解析端到端（W2）───────────────────────────────────────────────

// TestRouterChatThinkingLevelResolution: 请求级 > 绑定级 > 模型默认 > medium，
// xhigh/max 按模型支持度钳制。fake client 记录每次调用的 effLevel 断言。
func TestRouterChatThinkingLevelResolution(t *testing.T) {
	cases := []struct {
		name       string
		reqLevel   string
		bindLevel  string
		modelLevel string // 空=模型默认档 NULL
		xhighMax   bool
		want       string
	}{
		{name: "request wins over all", reqLevel: "high", bindLevel: "low", modelLevel: "off", want: "high"},
		{name: "binding beats model default", reqLevel: "", bindLevel: "high", modelLevel: "medium", want: "high"},
		{name: "model default when no overrides", reqLevel: "", bindLevel: "", modelLevel: "off", want: "off"},
		{name: "global default medium", reqLevel: "", bindLevel: "", modelLevel: "", want: "medium"},
		{name: "xhigh clamped to high when unsupported", reqLevel: "xhigh", bindLevel: "low", modelLevel: "", xhighMax: false, want: "high"},
		{name: "xhigh kept when declared", reqLevel: "xhigh", bindLevel: "", modelLevel: "", xhighMax: true, want: "xhigh"},
		{name: "binding xhigh clamped too", reqLevel: "", bindLevel: "max", modelLevel: "", xhighMax: false, want: "high"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAIRouterTestDB(t)
			store := NewStore(db)
			route := seedDefaultSummaryRoute(t, db)
			seedChatBinding(t, db, &route, "lvl-vendor", "lvl-model",
				nil,
				func(m *models.AIModel) {
					m.ThinkingLevel = levelPtr(tc.modelLevel)
					m.SupportsXhighMax = tc.xhighMax
				},
				tc.bindLevel, 1)

			client := &fakeProviderClient{responses: map[string]fakeChatReply{
				"lvl-vendor": {content: "ok"},
			}}
			router := NewRouterWithStore(store)
			router.RegisterClient(ProviderTypeOpenAICompatible, client)

			_, err := router.Chat(context.Background(), ChatRequest{
				Operation:     "test.thinking",
				Capability:    CapabilitySummary,
				Messages:      []Message{{Role: "user", Content: "hi"}},
				ThinkingLevel: tc.reqLevel,
			})
			require.NoError(t, err)
			require.Len(t, client.chatCalls, 1)
			require.Equal(t, tc.want, client.chatCalls[0].effLevel)
			require.Equal(t, "lvl-model", client.chatCalls[0].modelName)
		})
	}
}

// ─── 输出清理跟随思考档（specs/ai-thinking-levels）───────────────────────────

// TestRouterChatOffLevelKeepsThinkTags: 生效档 off 保留原始内容（含 <think> 标签）。
func TestRouterChatOffLevelKeepsThinkTags(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	route := seedDefaultSummaryRoute(t, db)
	seedChatBinding(t, db, &route, "off-vendor", "off-model", nil,
		func(m *models.AIModel) { m.ThinkingLevel = levelPtr("off") }, "", 1)

	raw := "<think\n>reasoning</think\n>The answer is 42"
	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"off-vendor": {content: raw},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.off_keep",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	require.Equal(t, raw, result.Content, "off 档必须原样返回（含标签）")
}

// TestRouterChatNonOffStripsThinkTags: 非 off 档剥离 <think>...</think> 并 TrimSpace。
func TestRouterChatNonOffStripsThinkTags(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	route := seedDefaultSummaryRoute(t, db)
	seedChatBinding(t, db, &route, "strip-vendor", "strip-model", nil, nil, "", 1)

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"strip-vendor": {content: "<think>reasoning</think>\nThe answer is 42"},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.strip",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
	require.Equal(t, "The answer is 42", result.Content)
	require.NotContains(t, result.Content, "<think>")
}

// ─── 结构化解析纠错（W5 Router 侧）────────────────────────────────────────────

// TestRouterChatStructuredCorrectionExtractsWrappedJSON: 带包裹文本的响应经平衡
// 花括号纠错提取后，ChatResult.Content = 提取后的纯 JSON 文本（调用方直接 Unmarshal）。
func TestRouterChatStructuredCorrectionExtractsWrappedJSON(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	route := seedDefaultSummaryRoute(t, db)
	fix := seedChatBinding(t, db, &route, "corr-vendor", "corr-model", nil, nil, "", 1)

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"corr-vendor": {content: "以下是结果：{\"a\":1}\n完毕"},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.corr",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
		JSONMode:   true,
		JSONSchema: &JSONSchema{Type: "object"},
	})
	require.NoError(t, err)
	require.Equal(t, `{"a":1}`, result.Content)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Content), &parsed))
	require.Equal(t, fix.vendor.Name, result.ProviderName)
	require.Equal(t, "corr-model", result.ModelName)
}

// TestRouterChatCorrectionFailureFallsToNextBinding: 响应不含可提取 JSON 时该
// binding 按 parse_error 尝试失败处理（不得把原文当结构化结果返回），继续下一绑定。
func TestRouterChatCorrectionFailureFallsToNextBinding(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	route := seedDefaultSummaryRoute(t, db)
	seedChatBinding(t, db, &route, "bad-json", "m1", nil, nil, "", 1)
	seedChatBinding(t, db, &route, "good-json", "m2", nil, nil, "", 2)

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"bad-json":  {content: "完全没有任何花括号"},
		"good-json": {content: `{"ok":true}`},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.corr_fail",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
		JSONMode:   true,
		JSONSchema: &JSONSchema{Type: "object"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, `{"ok":true}`, result.Content)
	require.True(t, result.UsedFallback)
	require.Equal(t, 2, result.AttemptCount)

	logs := chatCallLogs(t, db)
	require.Len(t, logs, 2)
	require.False(t, logs[0].Success)
	require.Equal(t, "parse_error", logs[0].ErrorCode)
	require.NotContains(t, logs[0].ResponseSnippet, "完全没有任何花括号", "非 JSON 原文不得冒充结构化结果回传")
	require.True(t, logs[1].Success)
	require.Equal(t, `{"ok":true}`, logs[1].ResponseSnippet)
}

// ─── 降级链日志行数与 structured_stage（W4 Router 侧，真实 client + fake server）──

// TestRouterChatDegradeChainLogsStructuredStages: json_schema 400 → json_object
// 成功，Router 必须写两行：失败档（error_code=http_400、structured_stage=json_schema）
// + 成功档（structured_stage=json_object），vendor 档位不被降级链修改。
func TestRouterChatDegradeChainLogsStructuredStages(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"schema not supported","code":"http_400"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"a\":2}"}}]}`))
	}))
	t.Cleanup(srv.Close)

	route := seedDefaultSummaryRoute(t, db)
	seedChatBinding(t, db, &route, "ds-vendor", "ds-chat",
		func(v *models.AIVendor) {
			v.BaseURL = srv.URL
			v.StructuredOutput = StructOutputJSONSchema
		}, nil, "", 1)

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, NewOpenAICompatibleClient())

	result, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "test.chain",
		Capability: CapabilitySummary,
		Messages:   []Message{{Role: "user", Content: "hi"}},
		JSONMode:   true,
		JSONSchema: &JSONSchema{Type: "object"},
	})
	require.NoError(t, err)
	require.Equal(t, `{"a":2}`, result.Content)
	require.Equal(t, int32(2), calls.Load())

	logs := chatCallLogs(t, db)
	require.Len(t, logs, 2, "每档真实 HTTP 尝试各写一行（D5/红线5）")

	failed := logs[0]
	require.False(t, failed.Success)
	require.Equal(t, "http_400", failed.ErrorCode)
	require.Equal(t, "ds-vendor", failed.ProviderName)
	require.Equal(t, "ds-chat", failed.Model)
	require.Contains(t, failed.RequestMeta, `"structured_stage":"json_schema"`)
	require.Contains(t, failed.ResponseSnippet, "schema not supported")

	succeeded := logs[1]
	require.True(t, succeeded.Success)
	require.False(t, succeeded.IsFallback)
	require.Contains(t, succeeded.RequestMeta, `"structured_stage":"json_object"`)
	require.Equal(t, `{"a":2}`, succeeded.ResponseSnippet)
}
