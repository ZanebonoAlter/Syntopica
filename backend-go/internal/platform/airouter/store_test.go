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
	require.NoError(t, db.AutoMigrate(&models.AISettings{}, &models.AIProvider{}, &models.AIRoute{}, &models.AIRouteProvider{},
		&models.AICallLog{}, &models.AIEmbeddingCache{},
		// vendor/model 两层新表（ai-provider-model-overhaul）：AIVendor 需在 AIModel 前，
		// AutoMigrate 建 AIModel.Vendor 外键约束时依赖它已存在
		&models.AIVendor{}, &models.AIModel{}, &models.AIRouteModel{}))
	database.DB = db
	return db
}

// TestStoreLoadRouteWithModelsOrdersByPriority 语义平移自旧
// LoadRouteWithProvidersOrdersByPriority：绑定按 ai_route_models.priority 升序返回
// （priority 2 先建、priority 1 后建，读取仍按顺位）。ErrRouteNotFound /
// ErrNoProviders / 过滤语义见 store_vendor_test.go。
func TestStoreLoadRouteWithModelsOrdersByPriority(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	vendor := &models.AIVendor{Name: "ord-vendor", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "https://ord.example/v1", Enabled: true}
	require.NoError(t, store.CreateVendor(vendor))
	primary := &models.AIModel{VendorID: vendor.ID, Name: "primary", ModelKind: ModelKindLLM, Enabled: true, Status: ModelStatusOK}
	backup := &models.AIModel{VendorID: vendor.ID, Name: "backup", ModelKind: ModelKindLLM, Enabled: true, Status: ModelStatusOK}
	require.NoError(t, store.UpsertModel(primary))
	require.NoError(t, store.UpsertModel(backup))

	route := &models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true}
	require.NoError(t, store.UpsertRouteModels(route, nil))
	// 语义平移旧测试：显式建两条绑定且 priority 与插入顺序相反（priority 2 先建、
	// priority 1 后建），验证读取按 priority 升序而非插入序。
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: backup.ID, Priority: 2, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: primary.ID, Priority: 1, Enabled: true}).Error)

	loadedRoute, bindings, err := store.LoadRouteWithModels(CapabilitySummary)
	require.NoError(t, err)
	require.NotNil(t, loadedRoute)
	require.Len(t, bindings, 2)
	require.Equal(t, "primary", bindings[0].Model.Name)
	require.Equal(t, "backup", bindings[1].Model.Name)
}
