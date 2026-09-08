package airouter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
)

// fakeChatReply configures fakeProviderClient.Chat for one provider name.
type fakeChatReply struct {
	content string
	usage   *TokenUsage
	err     error
	// nilResponse makes Chat return (nil, nil): transport success with no
	// usable payload — the shape that must still trigger ordered fallback.
	nilResponse bool
}

type fakeProviderClient struct {
	responses map[string]fakeChatReply
}

func (f *fakeProviderClient) Chat(_ context.Context, provider models.AIProvider, _ ChatRequest) (*ChatResponse, error) {
	res := f.responses[provider.Name]
	if res.err != nil {
		return nil, res.err
	}
	if res.nilResponse {
		return nil, nil
	}
	return &ChatResponse{Content: res.content, Usage: res.usage}, nil
}

func (f *fakeProviderClient) Embed(_ context.Context, provider models.AIProvider, _ EmbeddingRequest) (*EmbeddingResult, error) {
	res := f.responses[provider.Name]
	if res.err != nil {
		return nil, res.err
	}
	return &EmbeddingResult{Embeddings: [][]float64{{0.1, 0.2}}, Model: "test", Dimensions: 2, Provider: provider.Name}, nil
}

func TestRouterFallsBackOnRetryableProviderError(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	p1 := models.AIProvider{Name: "primary", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://a.example/v1", APIKey: "a", Model: "m1", Enabled: true}
	p2 := models.AIProvider{Name: "backup", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://b.example/v1", APIKey: "b", Model: "m2", Enabled: true}
	require.NoError(t, db.Create(&p1).Error)
	require.NoError(t, db.Create(&p2).Error)
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p1.ID, Priority: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p2.ID, Priority: 2, Enabled: true}).Error)

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
}

func TestRouterFallsBackOnTerminalError(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	p1 := models.AIProvider{Name: "primary-terminal", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://a.example/v1", APIKey: "a", Model: "m1", Enabled: true}
	p2 := models.AIProvider{Name: "backup", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://b.example/v1", APIKey: "b", Model: "m2", Enabled: true}
	require.NoError(t, db.Create(&p1).Error)
	require.NoError(t, db.Create(&p2).Error)
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p1.ID, Priority: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p2.ID, Priority: 2, Enabled: true}).Error)

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

	p := models.AIProvider{Name: "p", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://a.example/v1", APIKey: "k", Model: "m", Enabled: true}
	require.NoError(t, db.Create(&p).Error)
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p.ID, Priority: 1, Enabled: true}).Error)

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

	p := models.AIProvider{Name: "emb-p", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://a.example/v1", APIKey: "k", Model: "m", Enabled: true}
	require.NoError(t, db.Create(&p).Error)
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilityEmbedding), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p.ID, Priority: 1, Enabled: true}).Error)

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"emb-p": {content: "ok"},
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

	p := models.AIProvider{Name: "p-usage", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://a.example/v1", APIKey: "k", Model: "m", Enabled: true}
	require.NoError(t, db.Create(&p).Error)
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p.ID, Priority: 1, Enabled: true}).Error)

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

// seedChatFailover seeds a two-provider ordered_failover route for the summary
// capability (primary-empty → backup) and returns the router plus the db
// handle for ai_call_logs assertions.
func seedChatFailover(t *testing.T) (*Router, *gorm.DB) {
	t.Helper()
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	p1 := models.AIProvider{Name: "primary-empty", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://a.example/v1", APIKey: "a", Model: "m1", Enabled: true}
	p2 := models.AIProvider{Name: "backup", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://b.example/v1", APIKey: "b", Model: "m2", Enabled: true}
	require.NoError(t, db.Create(&p1).Error)
	require.NoError(t, db.Create(&p2).Error)
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p1.ID, Priority: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p2.ID, Priority: 2, Enabled: true}).Error)

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
// success row), then fall through ordered fallback to the backup provider,
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

// TestRouterChatAllProvidersEmptyReturnsError: when every provider yields an
// empty response, Chat must return an error naming each provider plus the
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
// content must not count as an empty response; content is returned verbatim
// (no trimming) so caller contracts stay unchanged.
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
	require.Equal(t, "  padded answer  ", result.Content)
	require.False(t, result.UsedFallback)
	require.Equal(t, 1, result.AttemptCount)

	logs := chatCallLogs(t, db)
	require.Len(t, logs, 1)
	require.True(t, logs[0].Success)
	require.Equal(t, "", logs[0].ErrorCode)
}
