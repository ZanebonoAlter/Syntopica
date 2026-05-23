package airouter

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

func setupAIRouterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AISettings{}, &models.AIProvider{}, &models.AIRoute{}, &models.AIRouteProvider{}, &models.AICallLog{}))
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
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilityArticleCompletion), Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p2.ID, Priority: 2, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: p1.ID, Priority: 1, Enabled: true}).Error)

	loadedRoute, providers, err := store.LoadRouteWithProviders(CapabilityArticleCompletion)
	require.NoError(t, err)
	require.NotNil(t, loadedRoute)
	require.Len(t, providers, 2)
	require.Equal(t, "primary", providers[0].Name)
	require.Equal(t, "backup", providers[1].Name)
}

func TestStoreLoadRouteWithProvidersReturnsErrorWhenMissing(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	loadedRoute, providers, err := store.LoadRouteWithProviders(CapabilityArticleCompletion)
	require.ErrorIs(t, err, ErrRouteNotFound)
	require.Nil(t, loadedRoute)
	require.Nil(t, providers)
}
