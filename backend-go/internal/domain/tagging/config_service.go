package tagging

import (
	"fmt"
	"strconv"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
)

// EmbeddingConfigService manages embedding configuration stored in the database
type EmbeddingConfigService struct{}

// NewEmbeddingConfigService creates a new config service
func NewEmbeddingConfigService() *EmbeddingConfigService {
	return &EmbeddingConfigService{}
}

// LoadConfig loads all config rows into a map
func (s *EmbeddingConfigService) LoadConfig() (map[string]string, error) {
	var configs []models.EmbeddingConfig
	if err := database.DB.Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("failed to load embedding config: %w", err)
	}
	m := make(map[string]string, len(configs))
	for _, c := range configs {
		m[c.Key] = c.Value
	}
	return m, nil
}

// LoadThresholds loads high/low similarity thresholds from config
func (s *EmbeddingConfigService) LoadThresholds() (EmbeddingMatchThresholds, error) {
	config, err := s.LoadConfig()
	if err != nil {
		return DefaultThresholds, err
	}

	thresholds := DefaultThresholds

	if v, ok := config["high_similarity_threshold"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1.0 {
			thresholds.HighSimilarity = f
		}
	}

	if v, ok := config["low_similarity_threshold"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1.0 {
			thresholds.LowSimilarity = f
		}
	}

	return thresholds, nil
}

// UpdateConfig updates a single config value by key
func (s *EmbeddingConfigService) UpdateConfig(key, value string) error {
	// Validate threshold values
	if key == "high_similarity_threshold" || key == "low_similarity_threshold" {
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid threshold value %q: must be a number", value)
		}
		if f <= 0 || f > 1.0 {
			return fmt.Errorf("invalid threshold value %f: must be between 0 and 1.0", f)
		}
	}

	// Check for model change
	if key == "embedding_model" {
		var existing models.EmbeddingConfig
		if err := database.DB.Where("key = ?", key).First(&existing).Error; err == nil {
			if existing.Value != value && value != "" {
				logging.Warnf("WARNING: Embedding model changed from %q to %q. Existing embeddings may be stale.", existing.Value, value)
			}
		}
	}

	result := database.DB.Model(&models.EmbeddingConfig{}).Where("key = ?", key).Update("value", value)
	if result.Error != nil {
		return fmt.Errorf("failed to update config %s: %w", key, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("config key %q not found", key)
	}
	return nil
}

// GetAllConfig returns all config rows
func (s *EmbeddingConfigService) GetAllConfig() ([]models.EmbeddingConfig, error) {
	var configs []models.EmbeddingConfig
	if err := database.DB.Order("key ASC").Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("failed to load embedding configs: %w", err)
	}
	return configs, nil
}
