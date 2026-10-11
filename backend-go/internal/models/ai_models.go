package models

import (
	"encoding/json"
	"time"
)

type AISettings struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Key         string    `gorm:"size:100;unique;index" json:"key"`
	Value       string    `gorm:"type:text" json:"value"`
	Description string    `gorm:"size:200" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Deprecated: 旧 ai_providers 单层实体已随 ai-provider-model-overhaul 迁移拆解为
// AIVendor+AIModel（specs/ai-vendor-model），新代码一律走两层实体。结构体仅保留
// 供存量表（迁移停写、回滚保险窗口）与旧迁移测试编译，SHALL NOT 新增读写。
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
	ID             uint   `gorm:"primaryKey" json:"id"`
	Name           string `gorm:"size:100;index:idx_ai_routes_capability_name,unique" json:"name"`
	Capability     string `gorm:"size:50;index:idx_ai_routes_capability_name,unique;index" json:"capability"`
	Enabled        bool   `gorm:"index" json:"enabled"`
	Description    string `gorm:"size:255" json:"description"`
	MaxConcurrency int    `json:"max_concurrency"` // 0 means use default per capability
	// RouteModels 关联（ai-provider-model-overhaul D3）：绑定链为 model。
	// 旧 Strategy/Priority/RouteProviders（provider 直挂）已随 D2 死列收口删除
	// （DB 物理列保留一版不迁不建）。
	RouteModels []AIRouteModel `gorm:"foreignKey:RouteID" json:"route_models,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (AIRoute) TableName() string {
	return "ai_routes"
}

// Deprecated: 旧 ai_route_providers 关联已随 ai-provider-model-overhaul 迁移改挂
// ai_route_models（AIRouteModel），新代码一律走新关联。结构体仅保留供存量表
// （停写、回滚保险窗口）与旧迁移测试编译，SHALL NOT 新增读写。
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

// AIVendor 供应商：承载协议/认证/端点/进程拉起（ai-provider-model-overhaul design D1）。
// 旧 AIProvider 单层实体迁移拆解后停写；LegacyProviderID 保留原 ai_providers.id
// 作迁移判重锚点，自定义（非预设）供应商 PresetID 为空。
type AIVendor struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Name             string    `gorm:"size:100;uniqueIndex" json:"name"`
	PresetID         string    `gorm:"size:50" json:"preset_id,omitempty"` // 预设标识（deepseek/zai/…），自定义为空
	ProviderType     string    `gorm:"size:50;index" json:"provider_type"` // openai_compatible|ollama
	BaseURL          string    `gorm:"size:500" json:"base_url"`
	APIKey           string    `gorm:"type:text" json:"api_key"`
	TimeoutSeconds   int       `json:"timeout_seconds"`
	StartCommand     string    `gorm:"type:text" json:"start_command"`            // 空=外部托管
	ThinkingDialect  string    `gorm:"size:50" json:"thinking_dialect"`           // qwen-chat-template|deepseek|reasoning-effort|none
	StructuredOutput string    `gorm:"size:50" json:"structured_output"`          // json_schema|json_object|none
	LegacyProviderID *uint     `gorm:"index" json:"legacy_provider_id,omitempty"` // 迁移判重锚点，可空
	Enabled          bool      `gorm:"index" json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (AIVendor) TableName() string {
	return "ai_vendors"
}

// AIModel 模型：挂属于唯一 vendor，承载模型名/类别/思考默认档/启用状态与清单时间戳。
// ThinkingLevel 七档（off..max）为模型默认档；embedding 模型恒 NULL（无思考语义）。
type AIModel struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	VendorID         uint       `gorm:"index;uniqueIndex:idx_ai_models_vendor_name" json:"vendor_id"`
	Name             string     `gorm:"size:100;uniqueIndex:idx_ai_models_vendor_name" json:"name"`
	ModelKind        string     `gorm:"size:20;default:llm;index" json:"model_kind"` // llm|embedding
	ThinkingLevel    *string    `gorm:"size:20" json:"thinking_level,omitempty"`     // 七档 off..max；embedding 恒 null
	SupportsXhighMax bool       `json:"supports_xhigh_max"`
	Enabled          bool       `gorm:"index" json:"enabled"`
	Status           string     `gorm:"size:20;default:ok;index" json:"status"` // ok|stale
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty"`
	// Vendor 关联（belongs-to）：供 GORM nested preload（如 RouteModels.Model.Vendor）
	// 与 API 序列化装载。创建/更新模型时保持零值即可，GORM 不会为空关联写库
	// （与 AIRouteModel.Model 同模式）。
	Vendor    AIVendor  `gorm:"foreignKey:VendorID" json:"vendor,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (AIModel) TableName() string {
	return "ai_models"
}

// AIRouteModel 能力路由↔模型绑定：取代 ai_route_providers 的 provider 直挂。
// ThinkingLevel 为场景覆盖档（空=继承模型默认档）。
type AIRouteModel struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	RouteID       uint      `gorm:"index:idx_air_route_model_link,unique" json:"route_id"`
	ModelID       uint      `gorm:"index:idx_air_route_model_link,unique" json:"model_id"`
	Priority      int       `gorm:"index" json:"priority"`
	ThinkingLevel *string   `gorm:"size:20" json:"thinking_level,omitempty"` // 空=继承模型默认
	Enabled       bool      `gorm:"index" json:"enabled"`
	Model         AIModel   `gorm:"foreignKey:ModelID" json:"model"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (AIRouteModel) TableName() string {
	return "ai_route_models"
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
