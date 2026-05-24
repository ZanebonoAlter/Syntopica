package models

import (
	"encoding/json"
	"time"
)

type SchedulerTask struct {
	ID                    uint       `gorm:"primaryKey" json:"id"`
	Name                  string     `gorm:"size:50;unique;not null;index" json:"name"`
	Description           string     `gorm:"size:200" json:"description"`
	CheckInterval         int        `gorm:"default:60;not null" json:"check_interval"` // seconds
	LastExecutionTime     *time.Time `json:"last_execution_time"`
	NextExecutionTime     *time.Time `json:"next_execution_time"`
	Status                string     `gorm:"size:20;default:idle;index" json:"status"`
	LastError             string     `gorm:"type:text" json:"last_error"`
	LastErrorTime         *time.Time `json:"last_error_time"`
	TotalExecutions       int        `gorm:"default:0" json:"total_executions"`
	SuccessfulExecutions  int        `gorm:"default:0" json:"successful_executions"`
	FailedExecutions      int        `gorm:"default:0" json:"failed_executions"`
	ConsecutiveFailures   int        `gorm:"default:0" json:"consecutive_failures"`
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
	Key         string    `gorm:"size:100;unique;not null;index" json:"key"`
	Value       string    `gorm:"type:text" json:"value"`
	Description string    `gorm:"size:200" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AIProvider struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Name           string    `gorm:"size:100;unique;not null;index" json:"name"`
	ProviderType   string    `gorm:"size:50;not null;default:openai_compatible;index" json:"provider_type"`
	BaseURL        string    `gorm:"size:500;not null" json:"base_url"`
	APIKey         string    `gorm:"type:text;not null" json:"api_key"`
	Model          string    `gorm:"size:100;not null" json:"model"`
	Enabled        bool      `gorm:"not null;default:true;index" json:"enabled"`
	TimeoutSeconds int       `gorm:"not null;default:120" json:"timeout_seconds"`
	MaxTokens      *int      `json:"max_tokens,omitempty"`
	Temperature    *float64  `json:"temperature,omitempty"`
	EnableThinking bool      `gorm:"not null;default:false" json:"enable_thinking"`
	Metadata       string    `gorm:"type:text" json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (AIProvider) TableName() string {
	return "ai_providers"
}

type AIRoute struct {
	ID             uint              `gorm:"primaryKey" json:"id"`
	Name           string            `gorm:"size:100;not null;index:idx_ai_routes_capability_name,unique" json:"name"`
	Capability     string            `gorm:"size:50;not null;index:idx_ai_routes_capability_name,unique;index" json:"capability"`
	Enabled        bool              `gorm:"not null;default:true;index" json:"enabled"`
	Priority       int               `gorm:"not null;default:100;index" json:"priority"`
	Strategy       string            `gorm:"size:50;not null;default:ordered_failover" json:"strategy"`
	Description    string            `gorm:"size:255" json:"description"`
	MaxConcurrency int               `gorm:"not null;default:0" json:"max_concurrency"` // 0 means use default per capability
	RouteProviders []AIRouteProvider `gorm:"foreignKey:RouteID" json:"route_providers,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

func (AIRoute) TableName() string {
	return "ai_routes"
}

type AIRouteProvider struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	RouteID    uint       `gorm:"not null;index:idx_ai_route_provider_link,unique" json:"route_id"`
	ProviderID uint       `gorm:"not null;index:idx_ai_route_provider_link,unique" json:"provider_id"`
	Priority   int        `gorm:"not null;default:100;index" json:"priority"`
	Enabled    bool       `gorm:"not null;default:true;index" json:"enabled"`
	Provider   AIProvider `gorm:"foreignKey:ProviderID" json:"provider"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (AIRouteProvider) TableName() string {
	return "ai_route_providers"
}

type AICallLog struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Capability      string    `gorm:"size:50;not null;index" json:"capability"`
	RouteName       string    `gorm:"size:100;not null" json:"route_name"`
	ProviderName    string    `gorm:"size:100;not null" json:"provider_name"`
	Success         bool      `gorm:"not null;index" json:"success"`
	IsFallback      bool      `gorm:"not null;default:false" json:"is_fallback"`
	LatencyMs       int       `json:"latency_ms"`
	ErrorCode       string    `gorm:"size:100" json:"error_code"`
	ErrorMessage    string    `gorm:"type:text" json:"error_message"`
	RequestMeta     string    `gorm:"type:text" json:"request_meta"`
	ResponseSnippet string    `gorm:"type:text" json:"response_snippet"`
	TraceID         string    `gorm:"size:64" json:"trace_id,omitempty"`
	CreatedAt       time.Time `gorm:"index:idx_ai_call_logs_created_at" json:"created_at"`
}

func (AICallLog) TableName() string {
	return "ai_call_logs"
}

func (a *AISettings) ToDict() map[string]interface{} {
	var valueJSON interface{}
	if a.Value != "" {
		_ = json.Unmarshal([]byte(a.Value), &valueJSON)
	}

	return map[string]interface{}{
		"id":          a.ID,
		"key":         a.Key,
		"value":       valueJSON,
		"description": a.Description,
		"created_at":  FormatDatetimeCST(a.CreatedAt),
		"updated_at":  FormatDatetimeCST(a.UpdatedAt),
	}
}

func (a *AISettings) ParseValue(v interface{}) error {
	if a.Value == "" {
		return nil
	}
	return json.Unmarshal([]byte(a.Value), v)
}

func ToJSONValue(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
