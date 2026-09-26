package core

import (
	"fmt"
	"strconv"

	"syntopica-backend/internal/platform/logging"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

// EmbeddingConfigService manages embedding configuration stored in the database
type EmbeddingConfigService struct{}

// NewEmbeddingConfigService creates a new config service
func NewEmbeddingConfigService() *EmbeddingConfigService {
	return &EmbeddingConfigService{}
}

// LoadConfig loads all config rows into a map
func (s *EmbeddingConfigService) LoadConfig() (map[string]string, error) {
	var configs []tagmodels.EmbeddingConfig
	if err := repository.Repo.DB().Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("failed to load embedding config: %w", err)
	}
	m := make(map[string]string, len(configs))
	for _, c := range configs {
		m[c.Key] = c.Value
	}
	return m, nil
}

// ClusterConfig holds tuning parameters for tag clustering.
type ClusterConfig struct {
	MaxTags             int
	SimilarityThreshold float64
	MaxClusterSize      int
	KwMinOverlap        int
	SemThreshold        float64
}

// DefaultClusterConfig provides sensible defaults.
var DefaultClusterConfig = ClusterConfig{
	MaxTags:             500,
	SimilarityThreshold: 0.85,
	MaxClusterSize:      8,
	KwMinOverlap:        2,
	SemThreshold:        0.80,
}

// LoadClusterConfig loads cluster configuration from the database,
// falling back to defaults for any missing keys.
func (s *EmbeddingConfigService) LoadClusterConfig() ClusterConfig {
	cfg := DefaultClusterConfig
	config, err := s.LoadConfig()
	if err != nil {
		return cfg
	}
	if v, ok := config["cluster_max_tags"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxTags = n
		}
	}
	if v, ok := config["cluster_similarity_threshold"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1.0 {
			cfg.SimilarityThreshold = f
		}
	}
	if v, ok := config["cluster_max_size"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxClusterSize = n
		}
	}
	if v, ok := config["event_cluster_kw_min_overlap"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.KwMinOverlap = n
		}
	}
	if v, ok := config["event_cluster_sem_threshold"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1.0 {
			cfg.SemThreshold = f
		}
	}
	return cfg
}

// UpdateConfig updates a single config value by key
func (s *EmbeddingConfigService) UpdateConfig(key, value string) error {
	// Validate threshold values
	if key == "match_threshold" {
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
		var existing tagmodels.EmbeddingConfig
		if err := repository.Repo.DB().Where("key = ?", key).First(&existing).Error; err == nil {
			if existing.Value != value && value != "" {
				logging.Warnf("WARNING: Embedding model changed from %q to %q. Existing embeddings may be stale.", existing.Value, value)
			}
		}
	}

	result := repository.Repo.DB().Model(&tagmodels.EmbeddingConfig{}).Where("key = ?", key).Update("value", value)
	if result.Error != nil {
		return fmt.Errorf("failed to update config %s: %w", key, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("config key %q not found", key)
	}
	return nil
}

// GetAllConfig returns all config rows
func (s *EmbeddingConfigService) GetAllConfig() ([]tagmodels.EmbeddingConfig, error) {
	var configs []tagmodels.EmbeddingConfig
	if err := repository.Repo.DB().Order("key ASC").Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("failed to load embedding configs: %w", err)
	}
	return configs, nil
}
