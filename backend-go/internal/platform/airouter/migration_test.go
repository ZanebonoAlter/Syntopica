package airouter

import (
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/aisettings"
)

func TestEnsureLegacySummaryConfigMigratedCreatesDefaultProviderAndRoutes(t *testing.T) {
	db := setupAIRouterTestDB(t)
	require.NoError(t, aisettings.SaveSummaryConfig(map[string]interface{}{
		"base_url": "https://api.example/v1",
		"api_key":  "token",
		"model":    "gpt-test",
	}, "legacy summary config"))

	require.NoError(t, EnsureLegacySummaryConfigMigrated())

	var provider models.AIProvider
	require.NoError(t, db.Where("name = ?", DefaultProviderName).First(&provider).Error)
	require.Equal(t, "https://api.example/v1", provider.BaseURL)

	var routeCount int64
	require.NoError(t, db.Model(&models.AIRoute{}).Count(&routeCount).Error)
	require.GreaterOrEqual(t, routeCount, int64(3))
}
