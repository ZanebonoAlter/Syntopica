package airouter

import (
	"context"
	"encoding/json"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/tracing"
)

type Capability string

const (
	CapabilitySummary            Capability = "summary"
	CapabilityTopicTagging       Capability = "topic_tagging"
	CapabilityDigestPolish       Capability = "digest_polish"
	CapabilityOpenNotebook       Capability = "open_notebook"
	CapabilityEmbedding          Capability = "embedding"
	CapabilityFeedDiscovery      Capability = "feed_discovery"
	DefaultRouteName             string     = "default"
	ProviderTypeOpenAICompatible string     = "openai_compatible"
	ProviderTypeOllama           string     = "ollama"
)

var (
	ErrRouteNotFound = errors.New("ai route not found")
	ErrNoProviders   = errors.New("ai route has no enabled providers")
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

func (s *Store) LogCall(ctx context.Context, logEntry *models.AICallLog) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "Store.LogCall")
	defer span.End()
	if logEntry == nil || s.db == nil {
		return
	}
	if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.HasTraceID() {
		logEntry.TraceID = spanCtx.TraceID().String()
	}
	// token_usage 列是 JSONB，空串不是合法 JSON；空值时省略该列让 DB 置 NULL。
	if logEntry.TokenUsage == "" {
		_ = s.db.Omit("token_usage").Create(logEntry).Error
		return
	}
	_ = s.db.Create(logEntry).Error
}

// SaveEmbeddingCache persists one embedding result with ON CONFLICT DO NOTHING
// semantics: a concurrent writer that raced us to the same cache_key wins and
// our row is silently dropped. Cache writes are best-effort by design.
func (s *Store) SaveEmbeddingCache(_ context.Context, rec *models.AIEmbeddingCache) error {
	if s.db == nil || rec == nil {
		return nil
	}
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(rec).Error
}

// LookupEmbeddingCache fetches a cached embedding by cache_key. Returns
// (nil, nil) on miss or when no DB is configured.
func (s *Store) LookupEmbeddingCache(_ context.Context, key string) (*models.AIEmbeddingCache, error) {
	if s.db == nil {
		return nil, nil
	}
	var rec models.AIEmbeddingCache
	err := s.db.Where("cache_key = ?", key).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &rec, nil
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
