package airouter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
)

// setupEmbedRoute creates one enabled embedding model on an "emb-vendor" bound
// to the default embedding route, mirroring the fixture style of router_test.go
// （Router 数据面已换 vendor/model 绑定链）。
func setupEmbedRoute(t *testing.T, store *Store, modelName string) (models.AIVendor, models.AIModel) {
	t.Helper()
	db := store.db
	vendor := models.AIVendor{Name: "emb-vendor", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "https://e.example/v1", APIKey: "k", Enabled: true}
	require.NoError(t, db.Create(&vendor).Error)
	m := models.AIModel{VendorID: vendor.ID, Name: modelName, ModelKind: ModelKindEmbedding,
		Enabled: true, Status: ModelStatusOK}
	require.NoError(t, db.Create(&m).Error)
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilityEmbedding), Enabled: true}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: m.ID, Priority: 1, Enabled: true}).Error)
	return vendor, m
}

func TestSaveEmbeddingCacheUpsertIdempotent(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	rec := &models.AIEmbeddingCache{
		CacheKey:     "key-1",
		Model:        "m1",
		Operation:    "test.op",
		Embedding:    []byte("payload-a"),
		Dimensions:   2,
		InputPreview: "hello",
		CreatedAt:    time.Now(),
	}
	require.NoError(t, store.SaveEmbeddingCache(context.Background(), rec))

	// Same cache_key again (concurrent writer raced us to it): no error, still one row.
	dup := &models.AIEmbeddingCache{
		CacheKey:     "key-1",
		Model:        "m1",
		Operation:    "test.op",
		Embedding:    []byte("payload-b"),
		Dimensions:   2,
		InputPreview: "hello",
		CreatedAt:    time.Now(),
	}
	require.NoError(t, store.SaveEmbeddingCache(context.Background(), dup))

	var count int64
	require.NoError(t, db.Model(&models.AIEmbeddingCache{}).Count(&count).Error)
	require.Equal(t, int64(1), count)

	// The first write wins: DO NOTHING must not overwrite the stored payload.
	var stored models.AIEmbeddingCache
	require.NoError(t, db.First(&stored, "cache_key = ?", "key-1").Error)
	require.Equal(t, []byte("payload-a"), stored.Embedding)
}

func TestLookupEmbeddingCacheMissReturnsNil(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	rec, err := store.LookupEmbeddingCache(context.Background(), "missing")
	require.NoError(t, err)
	require.Nil(t, rec)
}

func TestLookupEmbeddingCacheRoundTrip(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	require.NoError(t, store.SaveEmbeddingCache(context.Background(), &models.AIEmbeddingCache{
		CacheKey:   "key-rt",
		Model:      "m1",
		Embedding:  models.EncodeEmbeddingVectors([][]float64{{0.5, 0.75}}),
		Dimensions: 2,
		CreatedAt:  time.Now(),
	}))

	rec, err := store.LookupEmbeddingCache(context.Background(), "key-rt")
	require.NoError(t, err)
	require.NotNil(t, rec)
	require.Equal(t, "m1", rec.Model)
	vectors, err := models.DecodeEmbeddingVectors(rec.Embedding)
	require.NoError(t, err)
	require.Equal(t, [][]float64{{0.5, 0.75}}, vectors)
}
