package airouter

import (
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// TestUpsertRouteModelsValidation exercises UpsertRouteModels against a
// testcontainer Postgres: the binding validation does a real DB lookup
// (model existence / enabled / status / model_kind), so per testing.md it must
// run on real PG, not SQLite. Covers test-cases W9-1..6/7(提交拒绝)/8.
// Docker required. Skipped under -short.
func TestUpsertRouteModelsValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	store := NewStore(db)

	// 夹具：一个 llm 模型 + 一个 embedding 模型（本地 vendor，允许 embedding）。
	vendor := models.AIVendor{Name: "mk-binding-vendor", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "http://10.11.12.111:8080/v1", Enabled: true}
	require.NoError(t, db.Create(&vendor).Error)
	llmModel := models.AIModel{VendorID: vendor.ID, Name: "gpt", ModelKind: ModelKindLLM, Enabled: true}
	embModel := models.AIModel{VendorID: vendor.ID, Name: "bge-m3", ModelKind: ModelKindEmbedding, Enabled: true}
	require.NoError(t, db.Create(&llmModel).Error)
	require.NoError(t, db.Create(&embModel).Error)

	// W9-1：embedding 路由挂 llm 模型 → 拒绝，文案含模型名。
	embRouteLLM := &models.AIRoute{Name: "default", Capability: string(CapabilityEmbedding), Enabled: true}
	err := store.UpsertRouteModels(embRouteLLM, []RouteModelBindingInput{{ModelID: llmModel.ID}})
	require.Error(t, err, "embedding route must reject an llm model")
	require.Contains(t, err.Error(), "embedding 路由不能挂 llm 模型「gpt」")

	// W9-2：summary（llm 类）路由挂 embedding 模型 → 拒绝。
	sumRouteEmb := &models.AIRoute{Name: "default", Capability: string(CapabilitySummary), Enabled: true}
	err = store.UpsertRouteModels(sumRouteEmb, []RouteModelBindingInput{{ModelID: embModel.ID}})
	require.Error(t, err, "llm route must reject an embedding model")
	require.Contains(t, err.Error(), "llm 路由不能挂 embedding 模型「bge-m3」")

	// W9-3：同类绑定放行（embedding 路由+embedding 模型；summary 路由+llm 模型）。
	embRouteOK := &models.AIRoute{Name: "emb-ok", Capability: string(CapabilityEmbedding), Enabled: true}
	require.NoError(t, store.UpsertRouteModels(embRouteOK, []RouteModelBindingInput{{ModelID: embModel.ID}}),
		"embedding route must accept an embedding model")
	sumRouteOK := &models.AIRoute{Name: "sum-ok", Capability: string(CapabilitySummary), Enabled: true}
	require.NoError(t, store.UpsertRouteModels(sumRouteOK, []RouteModelBindingInput{{ModelID: llmModel.ID}}),
		"llm route must accept an llm model")

	// 空绑定 = 清空绑定，跳过校验。
	embRouteClear := &models.AIRoute{Name: "emb-clear", Capability: string(CapabilityEmbedding), Enabled: true}
	require.NoError(t, store.UpsertRouteModels(embRouteClear, nil), "empty bindings must skip validation (clear bindings)")
	var clearedCount int64
	require.NoError(t, db.Model(&models.AIRouteModel{}).Where("route_id = ?", embRouteClear.ID).Count(&clearedCount).Error)
	require.EqualValues(t, 0, clearedCount)

	// W9-4：绑定 thinking_level=ultra → 拒绝，文案含「thinking_level 非法」与合法七档列表。
	badLevel := &models.AIRoute{Name: "bad-level", Capability: string(CapabilitySummary), Enabled: true}
	err = store.UpsertRouteModels(badLevel, []RouteModelBindingInput{{ModelID: llmModel.ID, ThinkingLevel: "ultra"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "thinking_level 非法")
	require.Contains(t, err.Error(), "off/minimal/low/medium/high/xhigh/max")

	// W9-5：embedding 类绑定携带思考档覆盖 → 拒绝。
	embWithLevel := &models.AIRoute{Name: "emb-level", Capability: string(CapabilityEmbedding), Enabled: true}
	err = store.UpsertRouteModels(embWithLevel, []RouteModelBindingInput{{ModelID: embModel.ID, ThinkingLevel: "high"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "embedding 绑定不能携带思考档覆盖")

	// W9-8：不存在的 model_id → ErrModelNotFound。
	missing := &models.AIRoute{Name: "missing", Capability: string(CapabilityEmbedding), Enabled: true}
	err = store.UpsertRouteModels(missing, []RouteModelBindingInput{{ModelID: 9999999}})
	require.ErrorIs(t, err, ErrModelNotFound)

	// W9-7 提交拒绝：stale 与未启用的模型直接提交绑定 → 拒绝。
	disabledModel := models.AIModel{VendorID: vendor.ID, Name: "disabled-m", ModelKind: ModelKindLLM, Enabled: false}
	staleModel := models.AIModel{VendorID: vendor.ID, Name: "stale-m", ModelKind: ModelKindLLM, Enabled: true, Status: ModelStatusStale}
	require.NoError(t, db.Create(&disabledModel).Error)
	require.NoError(t, db.Create(&staleModel).Error)

	disabledRoute := &models.AIRoute{Name: "reject-disabled", Capability: string(CapabilitySummary), Enabled: true}
	err = store.UpsertRouteModels(disabledRoute, []RouteModelBindingInput{{ModelID: disabledModel.ID}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "模型「disabled-m」当前不可绑定（stale 或未启用）")

	staleRoute := &models.AIRoute{Name: "reject-stale", Capability: string(CapabilitySummary), Enabled: true}
	err = store.UpsertRouteModels(staleRoute, []RouteModelBindingInput{{ModelID: staleModel.ID}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "模型「stale-m」当前不可绑定（stale 或未启用）")
}

// W9-6：顺位调整与覆盖档同一次 UpsertRouteModels 原子落库——全删全建，新顺序
// （priority=idx+1）与新覆盖值同时生效；重复保存幂等覆盖，行数不增长。
func TestUpsertRouteModelsAtomicOrderAndOverride(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	store := NewStore(db)

	vendor := models.AIVendor{Name: "atomic-vendor", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "http://127.0.0.1:8080/v1", Enabled: true}
	require.NoError(t, db.Create(&vendor).Error)
	m1 := models.AIModel{VendorID: vendor.ID, Name: "primary", ModelKind: ModelKindLLM, Enabled: true}
	m2 := models.AIModel{VendorID: vendor.ID, Name: "backup", ModelKind: ModelKindLLM, Enabled: true}
	require.NoError(t, db.Create(&m1).Error)
	require.NoError(t, db.Create(&m2).Error)

	route := &models.AIRoute{Name: "default", Capability: string(CapabilitySummary), Enabled: true}
	// 第一次保存：m1(覆盖 high) → m2(继承)
	require.NoError(t, store.UpsertRouteModels(route, []RouteModelBindingInput{
		{ModelID: m1.ID, ThinkingLevel: "high"},
		{ModelID: m2.ID},
	}))

	type linkRow struct {
		ModelID       uint
		Priority      int
		ThinkingLevel *string
		Enabled       bool
	}
	var links []linkRow
	require.NoError(t, db.Table("ai_route_models").
		Select("model_id, priority, thinking_level, enabled").
		Where("route_id = ?", route.ID).Order("priority ASC").Scan(&links).Error)
	require.Len(t, links, 2)
	require.Equal(t, m1.ID, links[0].ModelID)
	require.Equal(t, 1, links[0].Priority)
	require.NotNil(t, links[0].ThinkingLevel)
	require.Equal(t, "high", *links[0].ThinkingLevel)
	require.True(t, links[0].Enabled)
	require.Equal(t, m2.ID, links[1].ModelID)
	require.Equal(t, 2, links[1].Priority)
	require.Nil(t, links[1].ThinkingLevel, "空覆盖档必须存 NULL（继承）")

	// 第二次保存（顺位对调 + m2 覆盖 low）：一次 UpsertRouteModels 原子生效。
	require.NoError(t, store.UpsertRouteModels(route, []RouteModelBindingInput{
		{ModelID: m2.ID, ThinkingLevel: "low"},
		{ModelID: m1.ID},
	}))

	links = nil
	require.NoError(t, db.Table("ai_route_models").
		Select("model_id, priority, thinking_level, enabled").
		Where("route_id = ?", route.ID).Order("priority ASC").Scan(&links).Error)
	require.Len(t, links, 2, "全删全建不产生重复行")
	require.Equal(t, m2.ID, links[0].ModelID)
	require.Equal(t, 1, links[0].Priority)
	require.NotNil(t, links[0].ThinkingLevel)
	require.Equal(t, "low", *links[0].ThinkingLevel)
	require.Equal(t, m1.ID, links[1].ModelID)
	require.Equal(t, 2, links[1].Priority)
	require.Nil(t, links[1].ThinkingLevel)
}
