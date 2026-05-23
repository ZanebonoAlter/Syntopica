package airouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

type Capability string

const (
	CapabilityArticleCompletion  Capability = "article_completion"
	CapabilityTopicTagging       Capability = "topic_tagging"
	CapabilityOpenNotebook       Capability = "open_notebook"
	CapabilityEmbedding          Capability = "embedding"
	DefaultRouteName             string     = "default"
	DefaultProviderName          string     = "default-primary"
	ProviderTypeOpenAICompatible string     = "openai_compatible"
	ProviderTypeOllama           string     = "ollama"
)

var defaultCapabilities = []Capability{
	CapabilityArticleCompletion,
	CapabilityTopicTagging,
	CapabilityEmbedding,
}

var (
	ErrRouteNotFound    = errors.New("ai route not found")
	ErrNoProviders      = errors.New("ai route has no enabled providers")
	ErrProviderNotFound = errors.New("ai provider not found")
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	if db == nil {
		db = database.DB
	}
	return &Store{db: db}
}

func (s *Store) ListProviders() ([]models.AIProvider, error) {
	var providers []models.AIProvider
	if err := s.db.Order("enabled DESC, name ASC").Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

func (s *Store) ListRoutes() ([]models.AIRoute, error) {
	var routes []models.AIRoute
	if err := s.db.Preload("RouteProviders", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("priority ASC").Preload("Provider")
	}).Order("capability ASC, name ASC").Find(&routes).Error; err != nil {
		return nil, err
	}
	return routes, nil
}

func (s *Store) LoadRouteWithProviders(capability Capability) (*models.AIRoute, []models.AIProvider, error) {
	var route models.AIRoute
	err := s.db.Where("capability = ? AND enabled = ?", string(capability), true).
		Order("CASE WHEN name = 'default' THEN 0 ELSE 1 END").
		Order("id ASC").
		Preload("RouteProviders", func(tx *gorm.DB) *gorm.DB {
			return tx.Where("enabled = ?", true).Order("priority ASC").Preload("Provider")
		}).
		First(&route).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrRouteNotFound
		}
		return nil, nil, err
	}

	providers := make([]models.AIProvider, 0, len(route.RouteProviders))
	for _, link := range route.RouteProviders {
		if !link.Enabled || !link.Provider.Enabled {
			continue
		}
		providers = append(providers, link.Provider)
	}
	if len(providers) == 0 {
		return &route, nil, ErrNoProviders
	}
	return &route, providers, nil
}

func (s *Store) UpsertProvider(provider *models.AIProvider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	provider.Name = strings.TrimSpace(provider.Name)
	provider.ProviderType = strings.TrimSpace(provider.ProviderType)
	provider.BaseURL = strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/")
	provider.Model = strings.TrimSpace(provider.Model)
	provider.APIKey = strings.TrimSpace(provider.APIKey)
	if provider.Name == "" || provider.BaseURL == "" || provider.Model == "" {
		return fmt.Errorf("provider fields are incomplete")
	}
	if provider.ProviderType == "" {
		provider.ProviderType = ProviderTypeOpenAICompatible
	}
	if provider.ProviderType != ProviderTypeOllama && provider.APIKey == "" {
		return fmt.Errorf("api_key is required for provider type %s", provider.ProviderType)
	}
	if provider.TimeoutSeconds <= 0 {
		provider.TimeoutSeconds = 120
	}

	var existing models.AIProvider
	err := s.db.Where("name = ?", provider.Name).First(&existing).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return s.db.Create(provider).Error
	}

	provider.ID = existing.ID
	provider.CreatedAt = existing.CreatedAt
	return s.db.Save(provider).Error
}

func (s *Store) UpsertRoute(route *models.AIRoute, providerIDs []uint) error {
	if route == nil {
		return fmt.Errorf("route is nil")
	}
	if strings.TrimSpace(route.Capability) == "" || strings.TrimSpace(route.Name) == "" {
		return fmt.Errorf("route fields are incomplete")
	}
	if route.Strategy == "" {
		route.Strategy = "ordered_failover"
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "capability"}, {Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"enabled", "strategy", "description", "updated_at"}),
		}).Create(route).Error; err != nil {
			return err
		}

		if err := tx.Where("capability = ? AND name = ?", route.Capability, route.Name).First(route).Error; err != nil {
			return err
		}

		if err := tx.Where("route_id = ?", route.ID).Delete(&models.AIRouteProvider{}).Error; err != nil {
			return err
		}

		for idx, providerID := range providerIDs {
			link := models.AIRouteProvider{RouteID: route.ID, ProviderID: providerID, Priority: idx + 1, Enabled: true}
			if err := tx.Create(&link).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ResolvePrimaryProvider(capability Capability) (*models.AIProvider, *models.AIRoute, error) {
	route, providers, err := s.LoadRouteWithProviders(capability)
	if err != nil {
		return nil, nil, err
	}
	provider := providers[0]
	return &provider, route, nil
}

func (s *Store) EnsureLegacyProviderAndRoutes(baseURL, apiKey, model string) (*models.AIProvider, error) {
	provider := &models.AIProvider{
		Name:           DefaultProviderName,
		ProviderType:   ProviderTypeOpenAICompatible,
		BaseURL:        strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:         strings.TrimSpace(apiKey),
		Model:          strings.TrimSpace(model),
		Enabled:        true,
		TimeoutSeconds: 120,
	}
	if err := s.UpsertProvider(provider); err != nil {
		return nil, err
	}

	for _, capability := range defaultCapabilities {
		if err := s.ensureDefaultRoute(capability, provider.ID); err != nil {
			return nil, err
		}
	}

	return provider, nil
}

func (s *Store) ensureDefaultRoute(capability Capability, providerID uint) error {
	var route models.AIRoute
	err := s.db.Where("capability = ? AND name = ?", string(capability), DefaultRouteName).First(&route).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		route = models.AIRoute{
			Capability:  string(capability),
			Name:        DefaultRouteName,
			Enabled:     true,
			Strategy:    "ordered_failover",
			Description: fmt.Sprintf("Default route for %s", capability),
		}
		if err := s.db.Create(&route).Error; err != nil {
			return err
		}
	}

	link := models.AIRouteProvider{RouteID: route.ID, ProviderID: providerID, Priority: 1, Enabled: true}
	var existing models.AIRouteProvider
	err = s.db.Where("route_id = ? AND provider_id = ?", route.ID, providerID).First(&existing).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return s.db.Create(&link).Error
	}
	return s.db.Model(&existing).Update("enabled", true).Error
}

func (s *Store) LogCall(ctx context.Context, logEntry *models.AICallLog) {
	if logEntry == nil || s.db == nil {
		return
	}
	if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.HasTraceID() {
		logEntry.TraceID = spanCtx.TraceID().String()
	}
	_ = s.db.Create(logEntry).Error
}

func encodeMeta(v any) string {
	if v == nil {
		return ""
	}
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}
