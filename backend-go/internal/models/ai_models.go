package models

import (
	"encoding/json"
	"time"
)

type SchedulerTask struct {
	ID                    uint       `gorm:"primaryKey" json:"id"`
	Name                  string     `gorm:"size:50;unique;index" json:"name"`
	Description           string     `gorm:"size:200" json:"description"`
	CheckInterval         int        `json:"check_interval"` // seconds
	LastExecutionTime     *time.Time `json:"last_execution_time"`
	NextExecutionTime     *time.Time `json:"next_execution_time"`
	Status                string     `gorm:"size:20;index" json:"status"`
	LastError             string     `gorm:"type:text" json:"last_error"`
	LastErrorTime         *time.Time `json:"last_error_time"`
	TotalExecutions       int        `json:"total_executions"`
	SuccessfulExecutions  int        `json:"successful_executions"`
	FailedExecutions      int        `json:"failed_executions"`
	ConsecutiveFailures   int        `json:"consecutive_failures"`
	LastExecutionDuration *float64   `json:"last_execution_duration"` // seconds
	LastExecutionResult   string     `gorm:"type:text" json:"last_execution_result"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (s *SchedulerTask) ToDict() map[string]interface{} {
	successRate := 0.0
	if s.TotalExecutions > 0 {
		successRate = float64(s.SuccessfulExecutions) / float64(s.TotalExecutions) * 100
	}

	return map[string]interface{}{
		"id":                      s.ID,
		"name":                    s.Name,
		"description":             s.Description,
		"check_interval":          s.CheckInterval,
		"last_execution_time":     FormatDatetimeCSTPtr(s.LastExecutionTime),
		"next_execution_time":     FormatDatetimeCSTPtr(s.NextExecutionTime),
		"status":                  s.Status,
		"last_error":              s.LastError,
		"last_error_time":         FormatDatetimeCSTPtr(s.LastErrorTime),
		"total_executions":        s.TotalExecutions,
		"successful_executions":   s.SuccessfulExecutions,
		"failed_executions":       s.FailedExecutions,
		"consecutive_failures":    s.ConsecutiveFailures,
		"last_execution_duration": s.LastExecutionDuration,
		"last_execution_result":   s.LastExecutionResult,
		"created_at":              FormatDatetimeCST(s.CreatedAt),
		"updated_at":              FormatDatetimeCST(s.UpdatedAt),
		"success_rate":            successRate,
	}
}

type AISettings struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Key         string    `gorm:"size:100;unique;index" json:"key"`
	Value       string    `gorm:"type:text" json:"value"`
	Description string    `gorm:"size:200" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AIProvider struct {
	ID             uint     `gorm:"primaryKey" json:"id"`
	Name           string   `gorm:"size:100;unique;index" json:"name"`
	ProviderType   string   `gorm:"size:50;index" json:"provider_type"`
	BaseURL        string   `gorm:"size:500" json:"base_url"`
	APIKey         string   `gorm:"type:text" json:"api_key"`
	Model          string   `gorm:"size:100" json:"model"`
	Enabled        bool     `gorm:"index" json:"enabled"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	MaxTokens      *int     `json:"max_tokens,omitempty"`
	Temperature    *float64 `json:"temperature,omitempty"`
	// EnableThinking controls whether the request propagates
	// chat_template_kwargs.enable_thinking=true, letting the model reason.
	// (Previously this flag only stripped <think> tags from responses after-the-fact.)
	EnableThinking bool `json:"enable_thinking"`
	// ModelKind declares the provider's functional model type: "llm" (default,
	// chat/inference) or "embedding" (vector embedding). It is orthogonal to
	// ProviderType, which expresses protocol (openai_compatible / ollama).
	ModelKind string `gorm:"size:20;default:llm;index" json:"model_kind"`
	// StartCommand, when non-empty, marks this provider as a locally-managed
	// process (e.g. a llama.cpp llama-server launch line) the runtime MAY attempt
	// to start. Empty means an externally-managed service.
	StartCommand string    `gorm:"type:text" json:"start_command"`
	Metadata     string    `gorm:"type:text" json:"metadata"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (AIProvider) TableName() string {
	return "ai_providers"
}

type AIRoute struct {
	ID             uint              `gorm:"primaryKey" json:"id"`
	Name           string            `gorm:"size:100;index:idx_ai_routes_capability_name,unique" json:"name"`
	Capability     string            `gorm:"size:50;index:idx_ai_routes_capability_name,unique;index" json:"capability"`
	Enabled        bool              `gorm:"index" json:"enabled"`
	Priority       int               `gorm:"index" json:"priority"`
	Strategy       string            `gorm:"size:50" json:"strategy"`
	Description    string            `gorm:"size:255" json:"description"`
	MaxConcurrency int               `json:"max_concurrency"` // 0 means use default per capability
	RouteProviders []AIRouteProvider `gorm:"foreignKey:RouteID" json:"route_providers,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

func (AIRoute) TableName() string {
	return "ai_routes"
}

type AIRouteProvider struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	RouteID    uint       `gorm:"index:idx_ai_route_provider_link,unique" json:"route_id"`
	ProviderID uint       `gorm:"index:idx_ai_route_provider_link,unique" json:"provider_id"`
	Priority   int        `gorm:"index" json:"priority"`
	Enabled    bool       `gorm:"index" json:"enabled"`
	Provider   AIProvider `gorm:"foreignKey:ProviderID" json:"provider"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (AIRouteProvider) TableName() string {
	return "ai_route_providers"
}

type AICallLog struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Operation       string    `gorm:"type:varchar(80);index:idx_call_logs_op_time,priority:1" json:"operation"`
	Capability      string    `gorm:"size:50;index" json:"capability"`
	RouteName       string    `gorm:"size:100" json:"route_name"`
	ProviderName    string    `gorm:"size:100" json:"provider_name"`
	Model           string    `gorm:"size:100" json:"model,omitempty"`
	Success         bool      `gorm:"index" json:"success"`
	IsFallback      bool      `json:"is_fallback"`
	LatencyMs       int       `json:"latency_ms"`
	ErrorCode       string    `gorm:"size:100" json:"error_code"`
	ErrorMessage    string    `gorm:"type:text" json:"error_message"`
	Prompt          string    `gorm:"type:text" json:"prompt,omitempty"`
	RequestMeta     string    `gorm:"type:text" json:"request_meta"`
	ResponseSnippet string    `gorm:"type:text" json:"response_snippet"`
	TokenUsage      string    `gorm:"type:jsonb" json:"token_usage,omitempty"`
	TraceID         string    `gorm:"size:64" json:"trace_id,omitempty"`
	SessionID       string    `gorm:"type:varchar(120);index:idx_call_logs_session" json:"session_id,omitempty"`
	CreatedAt       time.Time `gorm:"index:idx_ai_call_logs_created_at" json:"created_at"`
}

func (AICallLog) TableName() string {
	return "ai_call_logs"
}

// AIEmbeddingCache persists embedding results keyed by (model, input_hash) so
// repeated identical embedding requests skip the provider HTTP call entirely.
// The vector payload is stored as a little-endian float32 byte stream (bytea;
// see airouter/embedding_codec.go), not jsonb text and not pgvector: the hit
// path only needs byte-roundtrip, never similarity search (that is
// topic_tag_embeddings' job). Binary storage costs ~10KB per 2560-dim row
// versus ~31.5KB as jsonb floating point text.
type AIEmbeddingCache struct {
	CacheKey     string    `gorm:"primaryKey;size:64" json:"cache_key"`
	Model        string    `gorm:"size:100;index" json:"model"`
	Operation    string    `gorm:"type:varchar(80)" json:"operation"`
	Embedding    []byte    `gorm:"type:bytea" json:"embedding"`
	Dimensions   int       `json:"dimensions"`
	InputPreview string    `gorm:"size:200" json:"input_preview"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

func (AIEmbeddingCache) TableName() string {
	return "ai_embedding_cache"
}

func ToJSONValue(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
