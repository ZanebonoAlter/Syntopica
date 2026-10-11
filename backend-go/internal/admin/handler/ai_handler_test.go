package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
)

// ai-provider-model-overhaul 3.1/3.2/3.3：vendor/model/route/observability 端点
// 契约测试（specs/ai-vendor-model + ai-route-observability + ai-capability-routing）。

func setupAIAdminTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.AISettings{}, &models.AIVendor{}, &models.AIModel{},
		&models.AIRoute{}, &models.AIRouteModel{}, &models.AICallLog{}))
	database.DB = db
	repository.InitRepository(db)
	return db
}

func invokeHandler(t *testing.T, handler gin.HandlerFunc, method, path string, params gin.Params, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = params
	ctx.Request = newAIAdminJSONRequest(t, method, path, body)
	handler(ctx)
	return recorder
}

func newAIAdminJSONRequest(t *testing.T, method, target string, body any) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func decodeAIAdminBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func paramID(name string, id uint) gin.Params {
	return gin.Params{{Key: name, Value: fmt.Sprintf("%d", id)}}
}

// seedVendorWithModels 建一个本地 vendor + 若干 llm 模型（均 enabled/ok）。
func seedVendorWithModels(t *testing.T, db *gorm.DB, vendorName string, modelNames ...string) (models.AIVendor, []models.AIModel) {
	t.Helper()
	vendor := models.AIVendor{Name: vendorName, ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL: "http://10.11.12.111:8080/v1", APIKey: "sk-secret", StartCommand: "llama-server --port 8080", Enabled: true}
	require.NoError(t, db.Create(&vendor).Error)
	out := make([]models.AIModel, 0, len(modelNames))
	for _, name := range modelNames {
		m := models.AIModel{VendorID: vendor.ID, Name: name, ModelKind: airouter.ModelKindLLM,
			Enabled: true, Status: airouter.ModelStatusOK}
		require.NoError(t, db.Create(&m).Error)
		out = append(out, m)
	}
	return vendor, out
}

// ─── GET /ai/vendors：脱敏 + counts（W7/U2 扇出徽章数据源）─────────────────────

func TestListAIVendorsMasksSecretsAndCounts(t *testing.T) {
	db := setupAIAdminTestDB(t)
	vendor, _ := seedVendorWithModels(t, db, "llamacpp", "qwythos")
	empty := models.AIVendor{Name: "deepseek", ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL: "https://api.deepseek.com/v1", Enabled: true}
	require.NoError(t, db.Create(&empty).Error)

	// 模型被两条不同能力路由绑定 → capabilities_count=2。
	for _, cap := range []string{"summary", "topic_tagging"} {
		route := models.AIRoute{Name: "default", Capability: cap, Enabled: true}
		require.NoError(t, db.Create(&route).Error)
		require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID,
			ModelID: 0, Priority: 1, Enabled: true}).Error)
	}
	var bound models.AIModel
	require.NoError(t, db.Where("vendor_id = ?", vendor.ID).First(&bound).Error)
	require.NoError(t, db.Model(&models.AIRouteModel{}).Where("1 = 1").
		Update("model_id", bound.ID).Error)

	rec := invokeHandler(t, ListAIVendors, http.MethodGet, "/api/ai/vendors", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeAIAdminBody(t, rec)
	require.Equal(t, true, body["success"])
	raw := rec.Body.String()
	require.NotContains(t, raw, "sk-secret", "api_key 原文不得出现在响应")
	require.NotContains(t, raw, "llama-server", "start_command 原文不得出现在响应")

	data := body["data"].([]any)
	require.Len(t, data, 2)
	byName := map[string]map[string]any{}
	for _, item := range data {
		m := item.(map[string]any)
		byName[m["name"].(string)] = m
	}
	first := byName["llamacpp"]
	require.Equal(t, true, first["api_key_configured"])
	require.Equal(t, true, first["start_command_configured"])
	require.EqualValues(t, 1, first["models_count"])
	require.EqualValues(t, 2, first["capabilities_count"])
	require.NotContains(t, first, "api_key")
	second := byName["deepseek"]
	require.Equal(t, false, second["api_key_configured"])
	require.EqualValues(t, 0, second["models_count"])
	require.EqualValues(t, 0, second["capabilities_count"])
}

// ─── POST /ai/vendors：创建 + 自动清单刷新（成功/失败均不回滚）────────────────

func TestCreateAIVendorAutoRefreshesModels(t *testing.T) {
	db := setupAIAdminTestDB(t)

	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"new-model"}]}`))
	}))
	defer upstream.Close()

	rec := invokeHandler(t, CreateAIVendor, http.MethodPost, "/api/ai/vendors", nil, map[string]any{
		"name": "local", "provider_type": "openai_compatible", "base_url": upstream.URL,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeAIAdminBody(t, rec)
	require.Equal(t, true, body["success"])
	data := body["data"].(map[string]any)
	require.Equal(t, true, data["models_refreshed"])
	require.EqualValues(t, 1, data["added"])

	var count int64
	require.NoError(t, db.Model(&models.AIModel{}).Count(&count).Error)
	require.EqualValues(t, 1, count, "新拉取模型默认入库")
}

func TestCreateAIVendorRefreshFailureKeepsVendor(t *testing.T) {
	db := setupAIAdminTestDB(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	rec := invokeHandler(t, CreateAIVendor, http.MethodPost, "/api/ai/vendors", nil, map[string]any{
		"name": "local", "provider_type": "openai_compatible", "base_url": upstream.URL,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeAIAdminBody(t, rec)
	require.Equal(t, true, body["success"], "刷新失败不回滚 vendor 创建")
	data := body["data"].(map[string]any)
	require.Equal(t, false, data["models_refreshed"])
	require.NotEmpty(t, data["refresh_error"])

	var count int64
	require.NoError(t, db.Model(&models.AIVendor{}).Count(&count).Error)
	require.EqualValues(t, 1, count, "vendor 保留")
}

func TestCreateAIVendorRequiresNameAndBaseURL(t *testing.T) {
	setupAIAdminTestDB(t)
	rec := invokeHandler(t, CreateAIVendor, http.MethodPost, "/api/ai/vendors", nil, map[string]any{
		"name": "", "base_url": "",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeAIAdminBody(t, rec)
	require.Equal(t, false, body["success"])
	require.Contains(t, body["error"], "vendor fields are incomplete")
}

// ─── PUT /ai/vendors/:id：clear_api_key / clear_start_command / 沿用语义 ───────

func TestUpdateAIVendorClearAPIKeyAndKeep(t *testing.T) {
	db := setupAIAdminTestDB(t)
	vendor, _ := seedVendorWithModels(t, db, "deepseek")

	// 不带清除标记、api_key 空串 → 沿用已存 key。
	rec := invokeHandler(t, UpdateAIVendor, http.MethodPut, "/api/ai/vendors/:id", paramID("id", vendor.ID), map[string]any{
		"name": "deepseek", "provider_type": "openai_compatible",
		"base_url": "https://api.deepseek.com/v2", "api_key": "",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var kept models.AIVendor
	require.NoError(t, db.First(&kept, vendor.ID).Error)
	require.Equal(t, "sk-secret", kept.APIKey, "空 key + 无 clear 标记必须沿用原值")
	require.Equal(t, "https://api.deepseek.com/v2", kept.BaseURL)
	require.Equal(t, "llama-server --port 8080", kept.StartCommand, "空 start_command 且无清除标记沿用")

	// clear_api_key=true → 置空。
	rec = invokeHandler(t, UpdateAIVendor, http.MethodPut, "/api/ai/vendors/:id", paramID("id", vendor.ID), map[string]any{
		"name": "deepseek", "provider_type": "openai_compatible",
		"base_url": "https://api.deepseek.com/v2", "clear_api_key": true,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, db.First(&kept, vendor.ID).Error)
	require.Equal(t, "", kept.APIKey)
}

func TestUpdateAIVendorClearStartCommand(t *testing.T) {
	db := setupAIAdminTestDB(t)
	vendor, _ := seedVendorWithModels(t, db, "llamacpp")

	rec := invokeHandler(t, UpdateAIVendor, http.MethodPut, "/api/ai/vendors/:id", paramID("id", vendor.ID), map[string]any{
		"name": "llamacpp", "provider_type": "openai_compatible",
		"base_url": "http://10.11.12.111:8080/v1", "clear_start_command": true,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var loaded models.AIVendor
	require.NoError(t, db.First(&loaded, vendor.ID).Error)
	require.Equal(t, "", loaded.StartCommand)
}

// ─── DELETE /ai/vendors/:id：级联删除响应（W8-7）────────────────────────────

func TestDeleteAIVendorCascadeResponse(t *testing.T) {
	db := setupAIAdminTestDB(t)
	vendor, modelsSeeded := seedVendorWithModels(t, db, "llamacpp", "m1", "m2")

	// m1 挂 2 条路由、m2 挂 1 条 → detached_bindings=3；删除后路由行保留。
	routes := make([]models.AIRoute, 0, 3)
	for _, cap := range []string{"summary", "topic_tagging", "digest_polish"} {
		route := models.AIRoute{Name: "default", Capability: cap, Enabled: true}
		require.NoError(t, db.Create(&route).Error)
		routes = append(routes, route)
	}
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: routes[0].ID, ModelID: modelsSeeded[0].ID, Priority: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: routes[1].ID, ModelID: modelsSeeded[0].ID, Priority: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: routes[2].ID, ModelID: modelsSeeded[1].ID, Priority: 1, Enabled: true}).Error)

	rec := invokeHandler(t, DeleteAIVendor, http.MethodDelete, "/api/ai/vendors/:id", paramID("id", vendor.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeAIAdminBody(t, rec)
	require.Equal(t, true, body["success"])
	require.Contains(t, body["message"], "已删除 2 个模型、解除 3 条绑定")
	data := body["data"].(map[string]any)
	require.EqualValues(t, 2, data["deleted_models"])
	require.EqualValues(t, 3, data["detached_bindings"])

	var bindingCount, modelCount, vendorCount int64
	require.NoError(t, db.Model(&models.AIRouteModel{}).Count(&bindingCount).Error)
	require.NoError(t, db.Model(&models.AIModel{}).Count(&modelCount).Error)
	require.NoError(t, db.Model(&models.AIVendor{}).Count(&vendorCount).Error)
	require.EqualValues(t, 0, bindingCount)
	require.EqualValues(t, 0, modelCount)
	require.EqualValues(t, 0, vendorCount)
	var routeCount int64
	require.NoError(t, db.Model(&models.AIRoute{}).Count(&routeCount).Error)
	require.EqualValues(t, 3, routeCount, "被摘空的线路保留")
}

// ─── POST /ai/vendors/:id/models/refresh：diff 响应（W7）───────────────────────

func TestRefreshAIVendorModelsDiff(t *testing.T) {
	db := setupAIAdminTestDB(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"fresh"},{"id":"kept"}]}`))
	}))
	defer upstream.Close()

	vendor := models.AIVendor{Name: "local", ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL: upstream.URL, Enabled: true, TimeoutSeconds: 5}
	require.NoError(t, db.Create(&vendor).Error)
	existing := models.AIModel{VendorID: vendor.ID, Name: "kept", ModelKind: airouter.ModelKindLLM, Enabled: true}
	require.NoError(t, db.Create(&existing).Error)

	rec := invokeHandler(t, RefreshAIVendorModels, http.MethodPost, "/api/ai/vendors/:id/models/refresh",
		paramID("id", vendor.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeAIAdminBody(t, rec)
	data := body["data"].(map[string]any)
	require.ElementsMatch(t, []any{"fresh"}, data["added"].([]any))
	require.ElementsMatch(t, []any{}, data["stale"].([]any))
	require.EqualValues(t, 2, data["visible_count"])
}

func TestRefreshAIVendorModelsFailureReturns502(t *testing.T) {
	db := setupAIAdminTestDB(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	vendor := models.AIVendor{Name: "local", ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL: upstream.URL, Enabled: true, TimeoutSeconds: 5}
	require.NoError(t, db.Create(&vendor).Error)

	rec := invokeHandler(t, RefreshAIVendorModels, http.MethodPost, "/api/ai/vendors/:id/models/refresh",
		paramID("id", vendor.ID), nil)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	body := decodeAIAdminBody(t, rec)
	require.Equal(t, false, body["success"])
	require.NotEmpty(t, body["error"])
}

// ─── 测试连接端点（已存 / 内联）────────────────────────────────────────────

func TestTestAIVendorByIDAndInline(t *testing.T) {
	db := setupAIAdminTestDB(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"m-a"},{"id":"m-b"}]}`))
	}))
	defer upstream.Close()

	vendor := models.AIVendor{Name: "stored", ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL: upstream.URL, Enabled: true, TimeoutSeconds: 5}
	require.NoError(t, db.Create(&vendor).Error)

	rec := invokeHandler(t, TestAIVendor, http.MethodPost, "/api/ai/vendors/:id/test", paramID("id", vendor.ID), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeAIAdminBody(t, rec)
	require.Equal(t, true, body["success"])
	require.Contains(t, body["message"], "共 2 个可用模型")

	// 内联端点：未保存表单，不走库。
	rec = invokeHandler(t, TestAIVendorInline, http.MethodPost, "/api/ai/vendors/test", nil, map[string]any{
		"base_url": upstream.URL, "timeout_seconds": 5,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	body = decodeAIAdminBody(t, rec)
	require.Equal(t, true, body["success"])

	// 内联缺 base_url → 400。
	rec = invokeHandler(t, TestAIVendorInline, http.MethodPost, "/api/ai/vendors/test", nil, map[string]any{})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// 不存在的 vendor id → 404。
	rec = invokeHandler(t, TestAIVendor, http.MethodPost, "/api/ai/vendors/:id/test", paramID("id", 99999), nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ─── GET /ai/models + PUT /ai/models/:id（usable 标记 / 配置校验）──────────────

func TestListAIModelsUsableFlag(t *testing.T) {
	db := setupAIAdminTestDB(t)
	_, modelsSeeded := seedVendorWithModels(t, db, "local", "good")
	disabledVendor := models.AIVendor{Name: "off-vendor", ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL: "http://10.11.12.111:8081/v1", Enabled: false}
	require.NoError(t, db.Create(&disabledVendor).Error)
	offModel := models.AIModel{VendorID: disabledVendor.ID, Name: "m-off", ModelKind: airouter.ModelKindLLM, Enabled: true}
	require.NoError(t, db.Create(&offModel).Error)
	staleModel := models.AIModel{VendorID: modelsSeeded[0].VendorID, Name: "m-stale", ModelKind: airouter.ModelKindLLM,
		Enabled: true, Status: airouter.ModelStatusStale}
	require.NoError(t, db.Create(&staleModel).Error)

	rec := invokeHandler(t, ListAIModels, http.MethodGet, "/api/ai/models", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeAIAdminBody(t, rec)
	data := body["data"].([]any)
	byName := map[string]map[string]any{}
	for _, item := range data {
		m := item.(map[string]any)
		byName[m["name"].(string)] = m
	}
	require.Equal(t, true, byName["good"]["usable"])
	require.Equal(t, false, byName["m-off"]["usable"], "vendor 停用 → 不可绑定候选")
	require.Equal(t, false, byName["m-stale"]["usable"], "stale → 不可绑定候选（W9-7）")
	vendorInfo := byName["good"]["vendor"].(map[string]any)
	require.Equal(t, "local", vendorInfo["name"])
	require.Equal(t, "openai_compatible", vendorInfo["provider_type"])
}

func TestUpdateAIModelConfigAndValidation(t *testing.T) {
	db := setupAIAdminTestDB(t)
	_, modelsSeeded := seedVendorWithModels(t, db, "local", "qwythos")

	// 配置默认档 + 支持 xhigh/max。
	rec := invokeHandler(t, UpdateAIModel, http.MethodPut, "/api/ai/models/:id", paramID("id", modelsSeeded[0].ID), map[string]any{
		"enabled": true, "thinking_level": "high", "supports_xhigh_max": true,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var loaded models.AIModel
	require.NoError(t, db.First(&loaded, modelsSeeded[0].ID).Error)
	require.Equal(t, "high", *loaded.ThinkingLevel)
	require.True(t, loaded.SupportsXhighMax)

	// 空串清除思考档。
	rec = invokeHandler(t, UpdateAIModel, http.MethodPut, "/api/ai/models/:id", paramID("id", modelsSeeded[0].ID), map[string]any{
		"thinking_level": "",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, db.First(&loaded, modelsSeeded[0].ID).Error)
	require.Nil(t, loaded.ThinkingLevel)

	// 非法档位 → 400（store 文案透传，W9-4）。
	rec = invokeHandler(t, UpdateAIModel, http.MethodPut, "/api/ai/models/:id", paramID("id", modelsSeeded[0].ID), map[string]any{
		"thinking_level": "ultra",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeAIAdminBody(t, rec)
	require.Contains(t, body["error"], "thinking_level 非法")

	// 云端 vendor 不能配 embedding（W8-4）。
	cloud := models.AIVendor{Name: "deepseek", ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL: "https://api.deepseek.com/v1", Enabled: true}
	require.NoError(t, db.Create(&cloud).Error)
	cloudModel := models.AIModel{VendorID: cloud.ID, Name: "deepseek-chat", ModelKind: airouter.ModelKindLLM, Enabled: true}
	require.NoError(t, db.Create(&cloudModel).Error)
	rec = invokeHandler(t, UpdateAIModel, http.MethodPut, "/api/ai/models/:id", paramID("id", cloudModel.ID), map[string]any{
		"model_kind": "embedding",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	body = decodeAIAdminBody(t, rec)
	require.Contains(t, body["error"], "云端 vendor 不能配置 embedding 模型")
}

// ─── GET/PUT /ai/routes：绑定链视图 + 类型校验文案（W9）───────────────────────

func TestListAIRoutesReturnsModelChain(t *testing.T) {
	db := setupAIAdminTestDB(t)
	_, modelsSeeded := seedVendorWithModels(t, db, "local", "qwythos")
	route := models.AIRoute{Name: "default", Capability: "summary", Enabled: true}
	require.NoError(t, db.Create(&route).Error)
	override := "high"
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: modelsSeeded[0].ID,
		Priority: 1, Enabled: true, ThinkingLevel: &override}).Error)

	rec := invokeHandler(t, ListAIRoutes, http.MethodGet, "/api/ai/routes", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	body := decodeAIAdminBody(t, rec)
	data := body["data"].([]any)
	require.Len(t, data, 1)
	item := data[0].(map[string]any)
	require.Equal(t, "summary", item["capability"])
	bindings := item["route_models"].([]any)
	require.Len(t, bindings, 1)
	binding := bindings[0].(map[string]any)
	require.Equal(t, "high", binding["thinking_level"])
	m := binding["model"].(map[string]any)
	require.Equal(t, "qwythos", m["name"])
	require.Equal(t, "llm", m["model_kind"])
	v := binding["vendor"].(map[string]any)
	require.Equal(t, "local", v["name"])
}

func TestUpdateAIRouteValidatesModelKind(t *testing.T) {
	db := setupAIAdminTestDB(t)
	vendor, modelsSeeded := seedVendorWithModels(t, db, "local", "gpt")
	embModel := models.AIModel{VendorID: vendor.ID, Name: "bge-m3", ModelKind: airouter.ModelKindEmbedding,
		Enabled: true, Status: airouter.ModelStatusOK}
	require.NoError(t, db.Create(&embModel).Error)

	// W9-1：embedding 路由挂 llm 模型 → 拒绝，文案带模型名。
	rec := invokeHandler(t, UpdateAIRoute, http.MethodPut, "/api/ai/routes/:capability",
		gin.Params{{Key: "capability", Value: "embedding"}}, map[string]any{
			"bindings": []map[string]any{{"model_id": modelsSeeded[0].ID}},
		})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeAIAdminBody(t, rec)
	require.Contains(t, body["error"], "embedding 路由不能挂 llm 模型「gpt」")

	// W9-2：summary（llm 类）路由挂 embedding 模型 → 拒绝。
	rec = invokeHandler(t, UpdateAIRoute, http.MethodPut, "/api/ai/routes/:capability",
		gin.Params{{Key: "capability", Value: "summary"}}, map[string]any{
			"bindings": []map[string]any{{"model_id": embModel.ID}},
		})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	body = decodeAIAdminBody(t, rec)
	require.Contains(t, body["error"], "llm 路由不能挂 embedding 模型「bge-m3」")

	// W9-3：同类绑定放行，顺位与覆盖档一次落库（W9-6）。
	rec = invokeHandler(t, UpdateAIRoute, http.MethodPut, "/api/ai/routes/:capability",
		gin.Params{{Key: "capability", Value: "summary"}}, map[string]any{
			"name": "default", "max_concurrency": 2,
			"bindings": []map[string]any{
				{"model_id": modelsSeeded[0].ID, "thinking_level": "high"},
			},
		})
	require.Equal(t, http.StatusOK, rec.Code)
	var link models.AIRouteModel
	require.NoError(t, db.Where("route_id = ? AND model_id = ?", routeIDByName(t, db, "summary"), modelsSeeded[0].ID).First(&link).Error)
	require.Equal(t, 1, link.Priority)
	require.Equal(t, "high", *link.ThinkingLevel)
	var route models.AIRoute
	require.NoError(t, db.Where("capability = ? AND name = ?", "summary", "default").First(&route).Error)
	require.Equal(t, 2, route.MaxConcurrency, "max_concurrency 随保存落库")

	// 不存在的 model_id → ErrModelNotFound（W9-8）。
	rec = invokeHandler(t, UpdateAIRoute, http.MethodPut, "/api/ai/routes/:capability",
		gin.Params{{Key: "capability", Value: "summary"}}, map[string]any{
			"bindings": []map[string]any{{"model_id": 999999}},
		})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	body = decodeAIAdminBody(t, rec)
	require.Contains(t, body["error"], "ai model not found")
}

func routeIDByName(t *testing.T, db *gorm.DB, capability string) uint {
	t.Helper()
	var route models.AIRoute
	require.NoError(t, db.Where("capability = ?", capability).First(&route).Error)
	return route.ID
}

// ─── GET /ai/observability/degradations：window 校验 + 响应结构（薄层）──────────

func TestQueryDegradationsHandler(t *testing.T) {
	setupAIAdminTestDB(t)

	// 非法 window → 400 固定文案。
	rec := invokeHandler(t, QueryDegradations, http.MethodGet, "/api/ai/observability/degradations?window=12h", nil, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := decodeAIAdminBody(t, rec)
	require.Equal(t, "window must be 24h or 7d", body["error"])

	// 合法 window（空库）→ 结构完整：stats 全 0 + events 空数组。
	rec = invokeHandler(t, QueryDegradations, http.MethodGet, "/api/ai/observability/degradations?window=24h&capability=summary", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	body = decodeAIAdminBody(t, rec)
	require.Equal(t, true, body["success"])
	data := body["data"].(map[string]any)
	require.Equal(t, "24h", data["window"])
	stats := data["stats"].(map[string]any)
	require.EqualValues(t, 0, stats["calls"])
	require.EqualValues(t, 0, stats["degradations"])
	require.EqualValues(t, 0, stats["failures"])
	require.NotNil(t, data["events"])
	require.Len(t, data["events"].([]any), 0)
}
