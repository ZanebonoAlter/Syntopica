package airouter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/domain/models"
)

type fakeProviderClient struct {
	responses map[string]struct {
		content string
		err     error
	}
}

func (f *fakeProviderClient) Chat(_ context.Context, provider models.AIProvider, _ ChatRequest) (string, error) {
	res := f.responses[provider.Name]
	return res.content, res.err
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
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilityArticleCompletion), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p1.ID, Priority: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p2.ID, Priority: 2, Enabled: true}).Error)

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]struct {
		content string
		err     error
	}{
		"primary": {err: &ProviderError{Message: "rate limited", Code: "rate_limit", Retryable: true}},
		"backup":  {content: "ok from backup"},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{Capability: CapabilityArticleCompletion, Messages: []Message{{Role: "user", Content: "hi"}}})
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
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilityArticleCompletion), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p1.ID, Priority: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p2.ID, Priority: 2, Enabled: true}).Error)

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]struct {
		content string
		err     error
	}{
		"primary-terminal": {err: &ProviderError{Message: "invalid key", Code: "unauthorized", Retryable: false}},
		"backup":           {content: "ok from backup"},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{Capability: CapabilityArticleCompletion, Messages: []Message{{Role: "user", Content: "hi"}}})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "ok from backup", result.Content)
	require.True(t, result.UsedFallback)
	require.Equal(t, 2, result.AttemptCount)
}
