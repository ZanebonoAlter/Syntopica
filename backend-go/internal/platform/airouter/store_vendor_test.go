package airouter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
)

// ─── Vendor CRUD 与 key 语义（W8）────────────────────────────────────────────

// W8-1：openai_compatible vendor 允许空 api_key 创建成功（无 "api_key is required"）。
func TestCreateVendorEmptyAPIKeyAllowed(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	vendor := &models.AIVendor{
		Name:         "local-llama",
		ProviderType: ProviderTypeOpenAICompatible,
		BaseURL:      "http://localhost:8080/v1",
		APIKey:       "",
		Enabled:      true,
	}
	require.NoError(t, store.CreateVendor(vendor))
	require.NotZero(t, vendor.ID)

	var loaded models.AIVendor
	require.NoError(t, db.First(&loaded, vendor.ID).Error)
	require.Equal(t, "", loaded.APIKey)
	// 默认值补齐
	require.Equal(t, ProviderTypeOpenAICompatible, loaded.ProviderType)
	require.Equal(t, 120, loaded.TimeoutSeconds)
	require.Equal(t, "none", loaded.ThinkingDialect)
	require.Equal(t, "json_schema", loaded.StructuredOutput)
}

// W8-2：更新携带 clearAPIKey=true → key 置空。
func TestUpdateVendorClearAPIKey(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	vendor := &models.AIVendor{Name: "deepseek", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-secret", Enabled: true}
	require.NoError(t, store.CreateVendor(vendor))

	vendor.APIKey = "" // 前端回显不含 key，请求侧 key 为空
	require.NoError(t, store.UpdateVendor(vendor, true))

	var loaded models.AIVendor
	require.NoError(t, db.First(&loaded, vendor.ID).Error)
	require.Equal(t, "", loaded.APIKey)
}

// W8-3：更新 api_key 空、无清除标记 → 沿用已存值（key 不丢）。
func TestUpdateVendorEmptyKeyKeepsStoredValue(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	vendor := &models.AIVendor{Name: "deepseek", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-keep-me", Enabled: true}
	require.NoError(t, store.CreateVendor(vendor))

	// 变更其他字段、key 留空、不带清除标记
	vendor.APIKey = ""
	vendor.BaseURL = "https://api.deepseek.com/v2"
	require.NoError(t, store.UpdateVendor(vendor, false))

	var loaded models.AIVendor
	require.NoError(t, db.First(&loaded, vendor.ID).Error)
	require.Equal(t, "sk-keep-me", loaded.APIKey, "空 key + 无 clear 标记必须沿用原值")
	require.Equal(t, "https://api.deepseek.com/v2", loaded.BaseURL)
}

// W8-5：start_command 保存与回显（原文持久化；云端留空=外部托管）。
func TestVendorStartCommandPersisted(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	local := &models.AIVendor{Name: "llamacpp", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "http://127.0.0.1:8080/v1", StartCommand: "llama-server -m qwythos.gguf --port 8080", Enabled: true}
	require.NoError(t, store.CreateVendor(local))

	var loadedLocal models.AIVendor
	require.NoError(t, db.First(&loadedLocal, local.ID).Error)
	require.Equal(t, "llama-server -m qwythos.gguf --port 8080", loadedLocal.StartCommand)

	cloud := &models.AIVendor{Name: "deepseek", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "https://api.deepseek.com/v1", Enabled: true}
	require.NoError(t, store.CreateVendor(cloud))

	var loadedCloud models.AIVendor
	require.NoError(t, db.First(&loadedCloud, cloud.ID).Error)
	require.Equal(t, "", loadedCloud.StartCommand, "云端留空=不被视为本地进程")
}

// W8-6：vendor.name 唯一——重名创建拒绝；改名撞到他人名下拒绝（排除自身）。
func TestVendorNameUnique(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	alpha := &models.AIVendor{Name: "alpha", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "https://alpha.example/v1", Enabled: true}
	require.NoError(t, store.CreateVendor(alpha))

	dup := &models.AIVendor{Name: "alpha", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "https://other.example/v1", Enabled: true}
	err := store.CreateVendor(dup)
	require.Error(t, err)
	require.Contains(t, err.Error(), "vendor name \"alpha\" already exists")

	// 改名到他人名下：beta 想叫 alpha
	beta := &models.AIVendor{Name: "beta", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "https://beta.example/v1", Enabled: true}
	require.NoError(t, store.CreateVendor(beta))
	beta.Name = "alpha"
	err = store.UpdateVendor(beta, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "vendor name \"alpha\" already exists")

	// 改名到自己的名字（排除自身）→ 放行
	alpha.Name = "alpha"
	alpha.BaseURL = "https://alpha2.example/v1"
	require.NoError(t, store.UpdateVendor(alpha, false))

	var count int64
	require.NoError(t, db.Model(&models.AIVendor{}).Count(&count).Error)
	require.EqualValues(t, 2, count, "撞名拒绝不得产生新行")
}

// W8-4：云端 vendor 不能配置 embedding 模型；本地 vendor 允许（hostIsLocal 正向）。
func TestUpsertModelCloudVendorRejectsEmbedding(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	cloud := &models.AIVendor{Name: "deepseek", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "https://api.deepseek.com/v1", Enabled: true}
	require.NoError(t, store.CreateVendor(cloud))

	emb := &models.AIModel{VendorID: cloud.ID, Name: "bge-m3", ModelKind: ModelKindEmbedding, Enabled: true}
	err := store.UpsertModel(emb)
	require.Error(t, err)
	require.Contains(t, err.Error(), "云端 vendor 不能配置 embedding 模型「bge-m3」")

	// 本地 vendor 允许 embedding（llama.cpp embedding 服务）
	local := &models.AIVendor{Name: "llamacpp-emb", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "http://10.11.12.111:8081/v1", Enabled: true}
	require.NoError(t, store.CreateVendor(local))
	embLocal := &models.AIModel{VendorID: local.ID, Name: "bge-m3", ModelKind: ModelKindEmbedding, Enabled: true}
	require.NoError(t, store.UpsertModel(embLocal))
}

// UpsertModel 校验：embedding 模型不能带思考档；thinking_level 必须合法七档；
// model_kind 空→llm。
func TestUpsertModelValidation(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	vendor := &models.AIVendor{Name: "local", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "http://127.0.0.1:8080/v1", Enabled: true}
	require.NoError(t, store.CreateVendor(vendor))

	// embedding 模型携带思考档 → 拒绝
	embWithLevel := &models.AIModel{VendorID: vendor.ID, Name: "bge-m3",
		ModelKind: ModelKindEmbedding, ThinkingLevel: strPtr("high"), Enabled: true}
	err := store.UpsertModel(embWithLevel)
	require.Error(t, err)
	require.Contains(t, err.Error(), "embedding 模型不能配置思考档")

	// 非法七档 → 拒绝，文案含合法值列表
	badLevel := &models.AIModel{VendorID: vendor.ID, Name: "qwythos",
		ModelKind: ModelKindLLM, ThinkingLevel: strPtr("ultra"), Enabled: true}
	err = store.UpsertModel(badLevel)
	require.Error(t, err)
	require.Contains(t, err.Error(), "thinking_level 非法")
	require.Contains(t, err.Error(), "off/minimal/low/medium/high/xhigh/max")

	// model_kind 缺省 → llm
	m := &models.AIModel{VendorID: vendor.ID, Name: "qwythos", Enabled: true,
		ThinkingLevel: strPtr("high")}
	require.NoError(t, store.UpsertModel(m))
	var loaded models.AIModel
	require.NoError(t, db.First(&loaded, m.ID).Error)
	require.Equal(t, ModelKindLLM, loaded.ModelKind)
	require.Equal(t, "high", *loaded.ThinkingLevel)

	// 同 vendor 下撞名 → 拒绝
	dup := &models.AIModel{VendorID: vendor.ID, Name: "qwythos", Enabled: true}
	err = store.UpsertModel(dup)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

// hostIsLocal 表驱动（W8-4 判定列）：loopback/RFC1918=本地，其余（公网/域名/解析失败）=云端。
func TestHostIsLocal(t *testing.T) {
	cases := []struct {
		baseURL string
		want    bool
	}{
		{"http://127.0.0.1:8080/v1", true},
		{"http://localhost:8080/v1", true},
		{"http://localhost/v1", true},
		{"http://[::1]:8080/v1", true},
		{"localhost", true},
		{"http://10.0.0.5:11434", true},
		{"http://172.16.0.1:8080/v1", true},
		{"http://172.31.255.255:8080/v1", true},
		{"http://192.168.1.1:8080/v1", true},
		{"10.11.12.111:8080", true},
		// 云端侧
		{"https://api.deepseek.com/v1", false},
		{"http://172.32.0.1:8080/v1", false},  // 172.16-31 之外
		{"http://172.15.0.1:8080/v1", false},  // 同上
		{"http://192.169.1.1:8080/v1", false}, // 同上
		{"http://11.0.0.1:8080/v1", false},    // 10/8 之外
		{"http://8.8.8.8/v1", false},
		{"http://example.internal:8080/v1", false}, // 裸域名不解析 → 云端
		{"", false},
		{"://bad url", false},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, hostIsLocal(tc.baseURL), "hostIsLocal(%q)", tc.baseURL)
	}
}

// ─── 模型清单刷新对账（W7）───────────────────────────────────────────────────

// W7-1：清单新出现的模型 INSERT，默认未启用、kind=llm、status=ok、last_seen_at=now。
func TestRefreshVendorModelsAddsNewDisabled(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	vendor := seedVendor(t, db, "deepseek", "https://api.deepseek.com/v1")

	result, err := store.RefreshVendorModels(context.Background(), vendor.ID,
		func(ctx context.Context, v models.AIVendor) ([]string, error) {
			return []string{"deepseek-chat", "deepseek-reasoner"}, nil
		})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"deepseek-chat", "deepseek-reasoner"}, result.Added)
	require.Empty(t, result.Stale)
	require.ElementsMatch(t, []string{"deepseek-chat", "deepseek-reasoner"}, result.Visible)

	var list []models.AIModel
	require.NoError(t, db.Where("vendor_id = ?", vendor.ID).Order("name").Find(&list).Error)
	require.Len(t, list, 2)
	for _, m := range list {
		require.False(t, m.Enabled, "新拉取模型默认未启用")
		require.Equal(t, ModelKindLLM, m.ModelKind, "刷新新增的模型 kind 一律 llm（W7-5）")
		require.Equal(t, ModelStatusOK, m.Status)
		require.NotNil(t, m.LastSeenAt)
	}
}

// W7-2 + W7-4：库内有但清单无 → 标 stale 不删（绑定保留）；刷新后重新出现 → 复活 ok。
func TestRefreshVendorModelsStaleLifecycle(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	vendor := seedVendor(t, db, "deepseek", "https://api.deepseek.com/v1")
	chat := seedModel(t, db, vendor.ID, "deepseek-chat", ModelKindLLM)
	reasoner := seedModel(t, db, vendor.ID, "deepseek-reasoner", ModelKindLLM)

	// 第一次刷新：清单只剩 chat → reasoner 标 stale 不删
	result, err := store.RefreshVendorModels(context.Background(), vendor.ID,
		func(ctx context.Context, v models.AIVendor) ([]string, error) {
			return []string{"deepseek-chat"}, nil
		})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"deepseek-reasoner"}, result.Stale)

	var staleRow models.AIModel
	require.NoError(t, db.First(&staleRow, reasoner.ID).Error)
	require.Equal(t, ModelStatusStale, staleRow.Status, "上游消失标 stale，不删除")

	var chatRow models.AIModel
	require.NoError(t, db.First(&chatRow, chat.ID).Error)
	require.Equal(t, ModelStatusOK, chatRow.Status)

	// 第二次刷新：reasoner 重新出现在清单 → 复活为 ok
	result, err = store.RefreshVendorModels(context.Background(), vendor.ID,
		func(ctx context.Context, v models.AIVendor) ([]string, error) {
			return []string{"deepseek-chat", "deepseek-reasoner"}, nil
		})
	require.NoError(t, err)
	require.Empty(t, result.Stale)

	var revived models.AIModel
	require.NoError(t, db.First(&revived, reasoner.ID).Error)
	require.Equal(t, ModelStatusOK, revived.Status, "stale 模型重新可见必须复活")
	require.NotNil(t, revived.LastSeenAt)
}

// W7-3：fetch 失败（网络/超时/非 2xx 由 fetch 包装）→ 返回错误，库内零改动（含 stale 标记不变）。
func TestRefreshVendorModelsFailureKeepsOldState(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	vendor := seedVendor(t, db, "deepseek", "https://api.deepseek.com/v1")
	seedModel(t, db, vendor.ID, "deepseek-chat", ModelKindLLM)
	reasoner := seedModel(t, db, vendor.ID, "deepseek-reasoner", ModelKindLLM)
	// 预置 reasoner 已是 stale
	require.NoError(t, db.Model(&models.AIModel{}).Where("id = ?", reasoner.ID).
		Update("status", ModelStatusStale).Error)

	_, err := store.RefreshVendorModels(context.Background(), vendor.ID,
		func(ctx context.Context, v models.AIVendor) ([]string, error) {
			return nil, errors.New("upstream timeout")
		})
	require.Error(t, err)

	var list []models.AIModel
	require.NoError(t, db.Where("vendor_id = ?", vendor.ID).Order("name").Find(&list).Error)
	require.Len(t, list, 2, "刷新失败库内清单零改动")
	for _, m := range list {
		switch m.Name {
		case "deepseek-chat":
			require.Equal(t, ModelStatusOK, m.Status)
		case "deepseek-reasoner":
			require.Equal(t, ModelStatusStale, m.Status, "stale 标记保持刷新前状态")
		}
	}
}

// ─── 删除级联与被摘空线路（W8-7 / W8-8）─────────────────────────────────────

// W8-7：删除被 7 条路由绑定、含 2 模型的 vendor → 单事务解绑 7、删模型 2、删 vendor；
// 不因引用拒绝；两个计数在删除前取。
func TestDeleteVendorCascadeCounts(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	vendor := seedVendor(t, db, "qwythos", "http://127.0.0.1:8080/v1")
	m1 := seedModel(t, db, vendor.ID, "qwythos", ModelKindLLM)
	m2 := seedModel(t, db, vendor.ID, "bge-m3", ModelKindEmbedding)

	// 7 条绑定：2 条挂 m1、5 条挂 m2（跨能力路由，模拟被多能力引用）
	capabilities := []Capability{CapabilitySummary, CapabilityTopicTagging, CapabilityDigestPolish,
		CapabilityOpenNotebook, CapabilityEmbedding, CapabilityFeedDiscovery, CapabilitySummary}
	names := []string{"r1", "r2", "r3", "r4", "r5", "r6", "r7"}
	for i, capab := range capabilities {
		route := models.AIRoute{Name: names[i], Capability: string(capab), Enabled: true}
		require.NoError(t, db.Create(&route).Error)
		modelID := m1.ID
		if i >= 2 {
			modelID = m2.ID
		}
		require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: modelID,
			Priority: 1, Enabled: true}).Error)
	}

	deletedModels, detachedBindings, err := store.DeleteVendor(vendor.ID)
	require.NoError(t, err)
	require.Equal(t, 2, deletedModels)
	require.Equal(t, 7, detachedBindings)

	var vendorCount, modelCount, bindingCount int64
	require.NoError(t, db.Model(&models.AIVendor{}).Where("id = ?", vendor.ID).Count(&vendorCount).Error)
	require.NoError(t, db.Model(&models.AIModel{}).Where("vendor_id = ?", vendor.ID).Count(&modelCount).Error)
	require.NoError(t, db.Model(&models.AIRouteModel{}).Count(&bindingCount).Error)
	require.EqualValues(t, 0, vendorCount)
	require.EqualValues(t, 0, modelCount)
	require.EqualValues(t, 0, bindingCount, "路由绑定全部解除")

	// 删除无模型的 vendor → 计数 0，不报错
	empty := seedVendor(t, db, "empty", "https://api.example.com/v1")
	deletedModels, detachedBindings, err = store.DeleteVendor(empty.ID)
	require.NoError(t, err)
	require.Equal(t, 0, deletedModels)
	require.Equal(t, 0, detachedBindings)
}

// W8-8：绑定被级联摘空后线路保留，调用该能力返回既有 ErrNoProviders（不 panic）。
func TestLoadRouteWithModelsEmptyAfterDetachReturnsNoProviders(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	vendor := seedVendor(t, db, "deepseek", "https://api.deepseek.com/v1")
	m := seedModel(t, db, vendor.ID, "deepseek-chat", ModelKindLLM)
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: m.ID,
		Priority: 1, Enabled: true}).Error)

	// 级联解绑（模拟 vendor 删除的解绑步骤，路由行保留）
	require.NoError(t, db.Where("route_id = ?", route.ID).Delete(&models.AIRouteModel{}).Error)

	loadedRoute, bindings, err := store.LoadRouteWithModels(CapabilitySummary)
	require.ErrorIs(t, err, ErrNoProviders)
	require.NotNil(t, loadedRoute, "被摘空的线路允许存在（路由行保留）")
	require.Nil(t, bindings)
}

// ─── LoadRouteWithModels 绑定过滤（W9-7 候选过滤半边）────────────────────────

// W9-7：只返回 link.enabled && model.enabled && model.status=ok && vendor.enabled 的绑定；
// disabled link / disabled model / stale model / disabled vendor 各一个夹具全部被滤掉。
func TestLoadRouteWithModelsFiltersUnavailableBindings(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true}
	require.NoError(t, db.Create(&route).Error)

	// 可用链：vendor + model 全绿
	goodVendor := seedVendor(t, db, "good-vendor", "https://api.good.example/v1")
	goodModel := seedModel(t, db, goodVendor.ID, "good-model", ModelKindLLM)

	// 夹具 1：disabled model（vendor 正常）
	disabledModelVendor := seedVendor(t, db, "dm-vendor", "https://api.dm.example/v1")
	disabledModel := seedModel(t, db, disabledModelVendor.ID, "disabled-model", ModelKindLLM)
	require.NoError(t, db.Model(&models.AIModel{}).Where("id = ?", disabledModel.ID).Update("enabled", false).Error)

	// 夹具 2：stale model
	staleVendor := seedVendor(t, db, "stale-vendor", "https://api.stale.example/v1")
	staleModel := seedModel(t, db, staleVendor.ID, "stale-model", ModelKindLLM)
	require.NoError(t, db.Model(&models.AIModel{}).Where("id = ?", staleModel.ID).Update("status", ModelStatusStale).Error)

	// 夹具 3：disabled vendor（model 本身正常）
	badVendor := &models.AIVendor{Name: "bad-vendor", ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: "https://api.bad.example/v1", Enabled: false}
	require.NoError(t, db.Create(badVendor).Error)
	modelOfBadVendor := seedModel(t, db, badVendor.ID, "bad-vendor-model", ModelKindLLM)

	// 夹具 4：disabled link（model 与 vendor 都正常）
	linkOnlyVendor := seedVendor(t, db, "link-vendor", "https://api.link.example/v1")
	linkOnlyModel := seedModel(t, db, linkOnlyVendor.ID, "link-only-model", ModelKindLLM)

	type bindFixture struct {
		modelID  uint
		priority int
		enabled  bool
	}
	fixtures := []bindFixture{
		{goodModel.ID, 1, true},
		{disabledModel.ID, 2, true},
		{staleModel.ID, 3, true},
		{modelOfBadVendor.ID, 4, true},
		{linkOnlyModel.ID, 5, false}, // link disabled
	}
	for _, f := range fixtures {
		require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: f.modelID,
			Priority: f.priority, Enabled: f.enabled}).Error)
	}

	loadedRoute, bindings, err := store.LoadRouteWithModels(CapabilitySummary)
	require.NoError(t, err)
	require.NotNil(t, loadedRoute)
	require.Len(t, bindings, 1, "只应剩全绿可用绑定")
	require.Equal(t, "good-model", bindings[0].Model.Name)
	require.Equal(t, "good-vendor", bindings[0].Vendor.Name)
	require.Equal(t, "", bindings[0].ThinkingLevel, "NULL 覆盖档读出为空串")
}

// 绑定覆盖档读出：非空 thinking_level 从 NULL 之外原样带出（空串存库归一为 NULL 的读侧验证）。
func TestLoadRouteWithModelsCarriesBindingThinkingLevel(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	vendor := seedVendor(t, db, "deepseek", "https://api.deepseek.com/v1")
	m := seedModel(t, db, vendor.ID, "deepseek-chat", ModelKindLLM)

	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true}
	require.NoError(t, db.Create(&route).Error)
	level := "high"
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: m.ID,
		Priority: 1, Enabled: true, ThinkingLevel: &level}).Error)

	_, bindings, err := store.LoadRouteWithModels(CapabilitySummary)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, "high", bindings[0].ThinkingLevel)
}

// ResolvePrimaryModel 返回首位绑定（model + vendor + route）。
func TestResolvePrimaryModelReturnsFirstBinding(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	vendor := seedVendor(t, db, "deepseek", "https://api.deepseek.com/v1")
	first := seedModel(t, db, vendor.ID, "deepseek-chat", ModelKindLLM)
	second := seedModel(t, db, vendor.ID, "deepseek-reasoner", ModelKindLLM)

	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: second.ID, Priority: 2, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: first.ID, Priority: 1, Enabled: true}).Error)

	model, gotVendor, gotRoute, err := store.ResolvePrimaryModel(CapabilitySummary)
	require.NoError(t, err)
	require.Equal(t, "deepseek-chat", model.Name)
	require.Equal(t, vendor.ID, gotVendor.ID)
	require.Equal(t, route.ID, gotRoute.ID)

	// 无路由 → ErrRouteNotFound
	_, _, _, err = store.ResolvePrimaryModel(CapabilityDigestPolish)
	require.ErrorIs(t, err, ErrRouteNotFound)
}

// ListRoutesWithModels：nested preload 装载 RouteModels → Model → Vendor，不影响 ListRoutes。
func TestListRoutesWithModelsPreloadsChain(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	vendor := seedVendor(t, db, "deepseek", "https://api.deepseek.com/v1")
	m := seedModel(t, db, vendor.ID, "deepseek-chat", ModelKindLLM)
	route := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilitySummary), Enabled: true}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: m.ID, Priority: 1, Enabled: true}).Error)

	routes, err := store.ListRoutesWithModels()
	require.NoError(t, err)
	require.Len(t, routes, 1)
	require.Len(t, routes[0].RouteModels, 1)
	require.Equal(t, "deepseek-chat", routes[0].RouteModels[0].Model.Name)
	require.Equal(t, "deepseek", routes[0].RouteModels[0].Model.Vendor.Name)
}

// ListVendors：enabled DESC, name ASC 排序。
func TestListVendorsOrdering(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	require.NoError(t, store.CreateVendor(&models.AIVendor{Name: "charlie", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://c.example/v1"}))
	require.NoError(t, store.CreateVendor(&models.AIVendor{Name: "alpha", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://a.example/v1", Enabled: true}))
	require.NoError(t, store.CreateVendor(&models.AIVendor{Name: "bravo", ProviderType: ProviderTypeOpenAICompatible, BaseURL: "https://b.example/v1", Enabled: true}))

	vendors, err := store.ListVendors()
	require.NoError(t, err)
	require.Len(t, vendors, 3)
	require.Equal(t, []string{"alpha", "bravo", "charlie"},
		[]string{vendors[0].Name, vendors[1].Name, vendors[2].Name}, "enabled DESC（true 在前），同组内 name ASC")
}

// ListModels：按 vendor_id, name 排序。
func TestListModelsOrdering(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)
	v1 := seedVendor(t, db, "v1", "https://v1.example/v1")
	v2 := seedVendor(t, db, "v2", "https://v2.example/v1")
	seedModel(t, db, v2.ID, "a-model", ModelKindLLM)
	seedModel(t, db, v1.ID, "z-model", ModelKindLLM)
	seedModel(t, db, v1.ID, "b-model", ModelKindLLM)

	list, err := store.ListModels()
	require.NoError(t, err)
	require.Len(t, list, 3)
	require.Equal(t, "b-model", list[0].Name) // v1 组内 name ASC
	require.Equal(t, "z-model", list[1].Name)
	require.Equal(t, "a-model", list[2].Name) // v2 组在前（vendor_id ASC）
}

// ─── 夹具辅助 ────────────────────────────────────────────────────────────────

func seedVendor(t *testing.T, db *gorm.DB, name, baseURL string) models.AIVendor {
	t.Helper()
	vendor := models.AIVendor{Name: name, ProviderType: ProviderTypeOpenAICompatible,
		BaseURL: baseURL, Enabled: true}
	require.NoError(t, db.Create(&vendor).Error)
	return vendor
}

func seedModel(t *testing.T, db *gorm.DB, vendorID uint, name, kind string) models.AIModel {
	t.Helper()
	m := models.AIModel{VendorID: vendorID, Name: name, ModelKind: kind, Enabled: true}
	require.NoError(t, db.Create(&m).Error)
	return m
}
