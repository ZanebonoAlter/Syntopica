package airouter

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

func setupAIRouterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AISettings{}, &models.AIProvider{}, &models.AIRoute{}, &models.AIRouteProvider{}, &models.AICallLog{}, &models.AIEmbeddingCache{}))
	database.DB = db
	return db
}

func TestStoreLoadRouteWithProvidersOrdersByPriority(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	p1 := models.AIProvider{Name: "primary", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://a.example/v1", APIKey: "a", Model: "m1", Enabled: true}
	p2 := models.AIProvider{Name: "backup", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://b.example/v1", APIKey: "b", Model: "m2", Enabled: true}
	require.NoError(t, db.Create(&p1).Error)
	require.NoError(t, db.Create(&p2).Error)
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p2.ID, Priority: 2, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p1.ID, Priority: 1, Enabled: true}).Error)

	loadedRoute, providers, err := store.LoadRouteWithProviders(CapabilitySummary)
	require.NoError(t, err)
	require.NotNil(t, loadedRoute)
	require.Len(t, providers, 2)
	require.Equal(t, "primary", providers[0].Name)
	require.Equal(t, "backup", providers[1].Name)
}

func TestStoreLoadRouteWithProvidersReturnsErrorWhenMissing(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	loadedRoute, providers, err := store.LoadRouteWithProviders(CapabilitySummary)
	require.ErrorIs(t, err, ErrRouteNotFound)
	require.Nil(t, loadedRoute)
	require.Nil(t, providers)
}

func TestUpsertProviderOpenAICompatibleEmptyKeyAllowed(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	provider := &models.AIProvider{
		Name:         "local-llama",
		ProviderType: ProviderTypeOpenAICompatible,
		BaseURL:      "http://localhost:8080/v1",
		APIKey:       "",
		Model:        "qwen3-8b",
		Enabled:      true,
	}
	err := store.UpsertProvider(provider)
	require.NoError(t, err)
	require.NotZero(t, provider.ID)

	var loaded models.AIProvider
	require.NoError(t, db.First(&loaded, provider.ID).Error)
	require.Equal(t, "", loaded.APIKey)
}

func TestUpsertProviderModelKindDefaultsToLLM(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	provider := &models.AIProvider{
		Name:         "default-kind",
		ProviderType: ProviderTypeOpenAICompatible,
		BaseURL:      "http://localhost:8080/v1",
		Model:        "qwen3-8b",
		Enabled:      true,
	}
	require.NoError(t, store.UpsertProvider(provider))

	var loaded models.AIProvider
	require.NoError(t, db.First(&loaded, provider.ID).Error)
	require.Equal(t, "llm", loaded.ModelKind, "empty model_kind must normalize to llm")
}

func TestUpsertProviderModelKindAcceptsEmbedding(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	provider := &models.AIProvider{
		Name:         "emb-kind",
		ProviderType: ProviderTypeOpenAICompatible,
		BaseURL:      "http://localhost:8080/v1",
		Model:        "bge-m3",
		ModelKind:    "embedding",
		Enabled:      true,
	}
	require.NoError(t, store.UpsertProvider(provider))

	var loaded models.AIProvider
	require.NoError(t, db.First(&loaded, provider.ID).Error)
	require.Equal(t, "embedding", loaded.ModelKind)
}

func TestUpsertProviderModelKindRejectsInvalid(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	provider := &models.AIProvider{
		Name:         "bad-kind",
		ProviderType: ProviderTypeOpenAICompatible,
		BaseURL:      "http://localhost:8080/v1",
		Model:        "m",
		ModelKind:    "vision",
		Enabled:      true,
	}
	err := store.UpsertProvider(provider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid model_kind")
	require.Contains(t, err.Error(), "vision")
}

func TestUpsertProviderRenameUpdatesInPlace(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	created := &models.AIProvider{
		Name: "alpha", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "https://alpha.example/v1", Model: "m1", Enabled: true,
	}
	require.NoError(t, store.UpsertProvider(created))
	require.NotZero(t, created.ID)

	created.Name = "beta"
	created.BaseURL = "https://beta.example/v1"
	require.NoError(t, store.UpsertProvider(created))

	var count int64
	require.NoError(t, db.Model(&models.AIProvider{}).Count(&count).Error)
	require.EqualValues(t, 1, count, "rename must not create a second row")

	var loaded models.AIProvider
	require.NoError(t, db.First(&loaded, created.ID).Error)
	require.Equal(t, "beta", loaded.Name)
	require.Equal(t, "https://beta.example/v1", loaded.BaseURL)
}

func TestUpsertProviderRenameToExistingNameRejected(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	alpha := &models.AIProvider{Name: "alpha", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://alpha.example/v1", Model: "m1", Enabled: true}
	beta := &models.AIProvider{Name: "beta", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://beta.example/v1", Model: "m2", Enabled: true}
	require.NoError(t, store.UpsertProvider(alpha))
	require.NoError(t, store.UpsertProvider(beta))

	alpha.Name = "beta"
	err := store.UpsertProvider(alpha)
	require.Error(t, err, "renaming onto another provider's name must be rejected, not silently overwrite it")
	require.Contains(t, err.Error(), "beta")

	var loaded models.AIProvider
	require.NoError(t, db.First(&loaded, beta.ID).Error)
	require.Equal(t, "https://beta.example/v1", loaded.BaseURL, "the conflicting provider must stay untouched")
}

func TestUpsertProviderCreateDuplicateNameRejected(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	first := &models.AIProvider{Name: "alpha", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://alpha.example/v1", Model: "m1", Enabled: true}
	require.NoError(t, store.UpsertProvider(first))

	dup := &models.AIProvider{Name: "alpha", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://other.example/v1", Model: "m2", Enabled: true}
	err := store.UpsertProvider(dup)
	require.Error(t, err, "creating a provider with an existing name must be rejected, not silently overwrite it")
	require.Contains(t, err.Error(), "alpha")
}

// fix-provider-delete-route-deadlock：provider 删除级联解绑后线路可能被摘空，
// 查询该能力时返回既有的 ErrNoProviders，而非 panic 或不一致状态。
func TestStoreLoadRouteWithProvidersEmptyRoute(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&route).Error)

	loadedRoute, providers, err := store.LoadRouteWithProviders(CapabilitySummary)
	require.ErrorIs(t, err, ErrNoProviders)
	require.NotNil(t, loadedRoute, "被摘空的线路本身应保留")
	require.Empty(t, providers)
}
