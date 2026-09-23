package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

func setupAIAdminTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AIProvider{}, &models.AIRoute{}, &models.AIRouteProvider{}))
	database.DB = db
	repository.InitRepository(database.DB)
	return db
}

// 复现测试（fix-provider-delete-route-deadlock）：provider 挂在多条线路上时
// 删除应级联解绑成功，而非 409 拒绝（旧行为导致 provider 永远无法删除）。
func TestDeleteProviderDetachesLinkedProvider(t *testing.T) {
	db := setupAIAdminTestDB(t)
	provider := models.AIProvider{Name: "linked", ProviderType: "openai_compatible", BaseURL: "https://api.example.com/v1", APIKey: "token", Model: "gpt", Enabled: true, TimeoutSeconds: 120}
	require.NoError(t, db.Create(&provider).Error)
	summaryRoute := models.AIRoute{Name: "default", Capability: "summary", Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&summaryRoute).Error)
	tagRoute := models.AIRoute{Name: "default", Capability: "topic_tagging", Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&tagRoute).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: summaryRoute.ID, ProviderID: provider.ID, Priority: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: tagRoute.ID, ProviderID: provider.ID, Priority: 1, Enabled: true}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "provider_id", Value: fmt.Sprintf("%d", provider.ID)}}

	DeleteProvider(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Contains(t, body["message"], "detached from 2 route(s)")

	var linkCount int64
	require.NoError(t, db.Model(&models.AIRouteProvider{}).Where("provider_id = ?", provider.ID).Count(&linkCount).Error)
	require.EqualValues(t, 0, linkCount, "线路关联应被级联删除")
	var providerCount int64
	require.NoError(t, db.Model(&models.AIProvider{}).Where("id = ?", provider.ID).Count(&providerCount).Error)
	require.EqualValues(t, 0, providerCount, "provider 应被删除")
}

func TestDeleteProviderRemovesUnusedProvider(t *testing.T) {
	db := setupAIAdminTestDB(t)
	provider := models.AIProvider{Name: "unused", ProviderType: "openai_compatible", BaseURL: "https://api.example.com/v1", APIKey: "token", Model: "gpt", Enabled: true, TimeoutSeconds: 120}
	require.NoError(t, db.Create(&provider).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "provider_id", Value: fmt.Sprintf("%d", provider.ID)}}

	DeleteProvider(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var count int64
	require.NoError(t, db.Model(&models.AIProvider{}).Where("id = ?", provider.ID).Count(&count).Error)
	require.EqualValues(t, 0, count)
}

func TestUpdateProviderClearAPIKey(t *testing.T) {
	db := setupAIAdminTestDB(t)
	provider := models.AIProvider{Name: "cloud", ProviderType: "openai_compatible", BaseURL: "https://api.example.com/v1", APIKey: "secret-key", Model: "gpt-4o", Enabled: true, TimeoutSeconds: 120}
	require.NoError(t, db.Create(&provider).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "provider_id", Value: fmt.Sprintf("%d", provider.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	body := fmt.Sprintf(`{"name":"cloud","base_url":"https://api.example.com/v1","model":"gpt-4o","clear_api_key":true}`)
	ctx.Request.Body = io.NopCloser(strings.NewReader(body))

	UpdateProvider(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var loaded models.AIProvider
	require.NoError(t, db.First(&loaded, provider.ID).Error)
	require.Equal(t, "", loaded.APIKey)
}

func TestUpdateProviderKeepExistingKeyWhenNoClear(t *testing.T) {
	db := setupAIAdminTestDB(t)
	provider := models.AIProvider{Name: "cloud", ProviderType: "openai_compatible", BaseURL: "https://api.example.com/v1", APIKey: "secret-key", Model: "gpt-4o", Enabled: true, TimeoutSeconds: 120}
	require.NoError(t, db.Create(&provider).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "provider_id", Value: fmt.Sprintf("%d", provider.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	body := fmt.Sprintf(`{"name":"cloud","base_url":"https://api.example.com/v1","model":"gpt-4o","clear_api_key":false}`)
	ctx.Request.Body = io.NopCloser(strings.NewReader(body))

	UpdateProvider(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var loaded models.AIProvider
	require.NoError(t, db.First(&loaded, provider.ID).Error)
	require.Equal(t, "secret-key", loaded.APIKey)
}

func TestUpsertProviderAcceptsModelKindAndStartCommand(t *testing.T) {
	db := setupAIAdminTestDB(t)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	body := `{"name":"local","base_url":"http://localhost:8081/v1","model":"qwen","model_kind":"embedding","start_command":"llama-server -m qwen.gguf --port 8081"}`
	ctx.Request.Body = io.NopCloser(strings.NewReader(body))

	UpsertProvider(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var loaded models.AIProvider
	require.NoError(t, db.Where("name = ?", "local").First(&loaded).Error)
	require.Equal(t, "embedding", loaded.ModelKind)
	require.Equal(t, "llama-server -m qwen.gguf --port 8081", loaded.StartCommand)
}

func TestListProvidersReturnsModelKindAndStartCommandConfigured(t *testing.T) {
	db := setupAIAdminTestDB(t)
	provider := models.AIProvider{Name: "with-cmd", ProviderType: "openai_compatible", BaseURL: "https://api.example.com/v1", Model: "gpt", ModelKind: "embedding", StartCommand: "llama-server --port 8082", Enabled: true, TimeoutSeconds: 120}
	require.NoError(t, db.Create(&provider).Error)
	noCmd := models.AIProvider{Name: "no-cmd", ProviderType: "openai_compatible", BaseURL: "https://api.example.com/v1", Model: "gpt", Enabled: true, TimeoutSeconds: 120}
	require.NoError(t, db.Create(&noCmd).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	ListProviders(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	byName := map[string]map[string]any{}
	for _, p := range resp.Data {
		byName[p["name"].(string)] = p
	}
	require.Equal(t, "embedding", byName["with-cmd"]["model_kind"])
	require.Equal(t, true, byName["with-cmd"]["start_command_configured"])
	// start_command must NOT be echoed in the list response.
	_, echoed := byName["with-cmd"]["start_command"]
	require.False(t, echoed, "start_command must not be echoed in the list response")
	require.Equal(t, "llm", byName["no-cmd"]["model_kind"])
	require.Equal(t, false, byName["no-cmd"]["start_command_configured"])
}

func TestUpdateProviderClearStartCommand(t *testing.T) {
	db := setupAIAdminTestDB(t)
	provider := models.AIProvider{Name: "cloud", ProviderType: "openai_compatible", BaseURL: "https://api.example.com/v1", Model: "gpt-4o", StartCommand: "llama-server --port 8083", Enabled: true, TimeoutSeconds: 120}
	require.NoError(t, db.Create(&provider).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "provider_id", Value: fmt.Sprintf("%d", provider.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	body := `{"name":"cloud","base_url":"https://api.example.com/v1","model":"gpt-4o","clear_start_command":true}`
	ctx.Request.Body = io.NopCloser(strings.NewReader(body))

	UpdateProvider(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var loaded models.AIProvider
	require.NoError(t, db.First(&loaded, provider.ID).Error)
	require.Equal(t, "", loaded.StartCommand, "clear_start_command must empty the start_command")
}

func TestUpdateProviderRenameSucceeds(t *testing.T) {
	db := setupAIAdminTestDB(t)
	provider := models.AIProvider{Name: "old-name", ProviderType: "openai_compatible", BaseURL: "https://api.example.com/v1", Model: "gpt-4o", Enabled: true, TimeoutSeconds: 120}
	require.NoError(t, db.Create(&provider).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "provider_id", Value: fmt.Sprintf("%d", provider.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	body := `{"name":"new-name","base_url":"https://api.example.com/v1","model":"gpt-4o"}`
	ctx.Request.Body = io.NopCloser(strings.NewReader(body))

	UpdateProvider(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.AIProvider{}).Where("name = ?", "new-name").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestUpdateProviderRenameToExistingNameRejected(t *testing.T) {
	db := setupAIAdminTestDB(t)
	p1 := models.AIProvider{Name: "alpha", ProviderType: "openai_compatible", BaseURL: "https://a.example/v1", Model: "m1", Enabled: true, TimeoutSeconds: 120}
	p2 := models.AIProvider{Name: "beta", ProviderType: "openai_compatible", BaseURL: "https://b.example/v1", Model: "m2", Enabled: true, TimeoutSeconds: 120}
	require.NoError(t, db.Create(&p1).Error)
	require.NoError(t, db.Create(&p2).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "provider_id", Value: fmt.Sprintf("%d", p1.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	body := `{"name":"beta","base_url":"https://a.example/v1","model":"m1"}`
	ctx.Request.Body = io.NopCloser(strings.NewReader(body))

	UpdateProvider(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
}

func TestTestConnectionByProviderID(t *testing.T) {
	db := setupAIAdminTestDB(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" && r.Header.Get("Authorization") == "Bearer stored-key" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"m1"},{"id":"m2"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fake.Close()

	provider := models.AIProvider{Name: "backup", ProviderType: "openai_compatible", BaseURL: fake.URL, APIKey: "stored-key", Model: "m1", Enabled: true, TimeoutSeconds: 120}
	require.NoError(t, db.Create(&provider).Error)

	t.Run("resolves stored config by provider_id", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		ctx.Request.Body = io.NopCloser(strings.NewReader(fmt.Sprintf(`{"provider_id":%d}`, provider.ID)))

		TestConnection(ctx)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

		var resp struct {
			Success bool `json:"success"`
			Data    struct {
				Reachable bool     `json:"reachable"`
				Models    []string `json:"models"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
		require.True(t, resp.Success)
		require.True(t, resp.Data.Reachable)
		require.Equal(t, []string{"m1", "m2"}, resp.Data.Models)
	})

	t.Run("unknown provider_id returns 404", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		ctx.Request.Body = io.NopCloser(strings.NewReader(`{"provider_id":999999}`))

		TestConnection(ctx)
		require.Equal(t, http.StatusNotFound, recorder.Code)
	})
}
