package aisettings

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

const summaryConfigKey = "summary_config"
const openNotebookConfigKey = "open_notebook_config"
const firecrawlConfigKey = "firecrawl_config"

func loadConfigByKey(key string) (map[string]interface{}, *models.AISettings, error) {
	var settings models.AISettings
	err := database.DB.Where("key = ?", key).First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return map[string]interface{}{}, nil, nil
		}
		return nil, nil, err
	}

	config := map[string]interface{}{}
	if settings.Value == "" {
		return config, &settings, nil
	}

	if err := json.Unmarshal([]byte(settings.Value), &config); err != nil {
		return nil, nil, err
	}

	return config, &settings, nil

}

func saveConfigByKey(key string, config map[string]interface{}, description string) error {
	configJSON, err := models.ToJSONValue(config)
	if err != nil {
		return err
	}

	var settings models.AISettings
	dbErr := database.DB.Where("key = ?", key).First(&settings).Error
	if dbErr == nil {
		settings.Value = configJSON
		if description != "" {
			settings.Description = description
		}
		return database.DB.Save(&settings).Error
	}

	if !errors.Is(dbErr, gorm.ErrRecordNotFound) {
		return dbErr
	}

	settings = models.AISettings{
		Key:         key,
		Value:       configJSON,
		Description: description,
	}

	return database.DB.Create(&settings).Error
}

func LoadSummaryConfig() (map[string]interface{}, *models.AISettings, error) {
	return loadConfigByKey(summaryConfigKey)
}

func SaveSummaryConfig(config map[string]interface{}, description string) error {
	return saveConfigByKey(summaryConfigKey, config, description)
}

func LoadFirecrawlConfig() (map[string]interface{}, *models.AISettings, error) {
	return loadConfigByKey(firecrawlConfigKey)
}

func SaveFirecrawlConfig(config map[string]interface{}, description string) error {
	return saveConfigByKey(firecrawlConfigKey, config, description)
}

func LoadOpenNotebookConfig() (map[string]interface{}, *models.AISettings, error) {
	return loadConfigByKey(openNotebookConfigKey)
}

func SaveOpenNotebookConfig(config map[string]interface{}, description string) error {
	return saveConfigByKey(openNotebookConfigKey, config, description)
}
