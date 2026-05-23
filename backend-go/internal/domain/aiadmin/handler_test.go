package aiadmin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

func setupAIAdminTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AIProvider{}, &models.AIRoute{}, &models.AIRouteProvider{}))
	database.DB = db
	return db
}

func TestDeleteProviderBlocksLinkedProvider(t *testing.T) {
	db := setupAIAdminTestDB(t)
	provider := models.AIProvider{Name: "linked", ProviderType: "openai_compatible", BaseURL: "https://api.example.com/v1", APIKey: "token", Model: "gpt", Enabled: true, TimeoutSeconds: 120}
	require.NoError(t, db.Create(&provider).Error)
	route := models.AIRoute{Name: "default", Capability: "summary", Enabled: true, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: provider.ID, Priority: 1, Enabled: true}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "provider_id", Value: fmt.Sprintf("%d", provider.ID)}}

	DeleteProvider(ctx)
	require.Equal(t, http.StatusConflict, recorder.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Contains(t, body["error"], "still used")
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
