package airouter

import (
	"encoding/json"
	"strings"

	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/database"
)

func EnsureLegacySummaryConfigMigrated() error {
	db := database.DB
	store := NewStore(db)

	legacyConfig, _, err := aisettings.LoadSummaryConfig()
	if err != nil {
		return err
	}
	baseURL, _ := legacyConfig["base_url"].(string)
	apiKey, _ := legacyConfig["api_key"].(string)
	model, _ := legacyConfig["model"].(string)
	if strings.TrimSpace(baseURL) != "" && strings.TrimSpace(apiKey) != "" && strings.TrimSpace(model) != "" {
		if _, err := store.EnsureLegacyProviderAndRoutes(baseURL, apiKey, model); err != nil {
			return err
		}
	}

	if firecrawlData, ok := legacyConfig["firecrawl"].(map[string]any); ok {
		existingFirecrawlConfig, _, loadErr := aisettings.LoadFirecrawlConfig()
		if loadErr == nil && len(existingFirecrawlConfig) == 0 {
			_ = aisettings.SaveFirecrawlConfig(firecrawlData, "Firecrawl configuration")
		}
	}

	return nil
}

func MarshalMetadata(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

func EnsureSchemaMigrated(db *gorm.DB) error {
	return db.AutoMigrate(&models.AIProvider{}, &models.AIRoute{}, &models.AIRouteProvider{}, &models.AICallLog{})
}
