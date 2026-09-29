/**
 * AI 相关类型定义（ai-provider-model-overhaul D7/D8：vendor / model / route 三层模型，
 * 字段对齐 backend-go/internal/admin/handler/ai_handler.go 响应，snake_case）
 */

/**
 * AI 设置数据
 */
export interface AISettings {
  baseURL?: string
  apiKey?: string
  model?: string
  summaryEnabled?: boolean
  providerId?: number
  providerName?: string
  routeName?: string
  timeRange?: number
  apiKeyConfigured?: boolean
}

/** 思考档位（七档 + 关） */
export type ThinkingLevel = 'off' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh' | 'max'

/** 思考方言（wire 映射方式） */
export type ThinkingDialect = 'qwen-chat-template' | 'deepseek' | 'reasoning-effort' | 'none'

/** 结构化输出模式 */
export type StructuredOutputMode = 'json_schema' | 'json_object' | 'none'

/** 模型功能类型（与 provider_type 协议维度正交） */
export type ModelKind = 'llm' | 'embedding'

/** provider 协议类型（仅两种 client 实现） */
export type ProviderType = 'openai_compatible' | 'ollama'

/** 供应商（脱敏视图：api_key/start_command 只回 configured 布尔，原文不下发） */
export interface AIVendor {
  id: number
  name: string
  preset_id?: string
  provider_type: ProviderType
  base_url: string
  timeout_seconds: number
  thinking_dialect: ThinkingDialect
  structured_output: StructuredOutputMode
  start_command_configured: boolean
  api_key_configured: boolean
  enabled: boolean
  /** 列表端点附加：该 vendor 模型数 */
  models_count?: number
  /** 列表端点附加：其模型被绑定的去重 capability 数（扇出徽章数据源） */
  capabilities_count?: number
}

export interface AIVendorUpsertRequest {
  name: string
  preset_id?: string
  provider_type?: ProviderType
  base_url: string
  api_key?: string
  timeout_seconds?: number
  /** 本地进程启动命令；留空=外部托管服务（更新时不改动已保存值） */
  start_command?: string
  /** true 时清除已保存的启动命令 */
  clear_start_command?: boolean
  thinking_dialect?: ThinkingDialect
  structured_output?: StructuredOutputMode
  enabled?: boolean
  /** true 时清除已保存密钥；false 且 api_key 空串=沿用已存值 */
  clear_api_key?: boolean
}

/** POST /ai/vendors 响应：保存后自动触发一次清单刷新（失败不回滚创建） */
export interface AIVendorCreateResult {
  id: number
  models_refreshed?: boolean
  refresh_error?: string
  added?: number
}

export interface AIModel {
  id: number
  vendor_id: number
  name: string
  model_kind: ModelKind
  /** null=继承全局默认档 */
  thinking_level: ThinkingLevel | null
  supports_xhigh_max: boolean
  enabled: boolean
  /** ok=最近探测可达；stale=上游清单已消失 */
  status: 'ok' | 'stale'
  last_seen_at: string | null
  /** GET /ai/models 附加：vendor 摘要 */
  vendor?: { id: number; name: string; provider_type: ProviderType; base_url: string }
  /** enabled && status=ok && vendor.enabled（绑定候选过滤，列表端点附加） */
  usable?: boolean
}

/** 路由绑定项（route_models 元素，按 priority 升序即降级顺序） */
export interface AIRouteModelBinding {
  id: number
  route_id: number
  model_id: number
  priority: number
  /** null=继承模型默认档 */
  thinking_level: ThinkingLevel | null
  enabled: boolean
  model: {
    id: number
    name: string
    model_kind: ModelKind
    thinking_level: ThinkingLevel | null
    supports_xhigh_max: boolean
    enabled: boolean
    status: 'ok' | 'stale'
  }
  vendor: { id: number; name: string }
}

export interface AIRoute {
  id: number
  name: string
  capability: string
  enabled: boolean
  description: string
  max_concurrency: number
  route_models: AIRouteModelBinding[]
}

/** PUT /ai/routes/:capability 保存体（全删全建语义） */
export interface RouteSaveRequest {
  name?: string
  enabled?: boolean
  description?: string
  max_concurrency?: number
  bindings: Array<{ model_id: number; thinking_level?: ThinkingLevel | null }>
}

/** PUT /ai/models/:id 请求体（thinking_level 空串=清除，继承全局默认档） */
export interface AIModelUpdateRequest {
  enabled?: boolean
  model_kind?: ModelKind
  thinking_level?: string | null
  supports_xhigh_max?: boolean
}

/** POST /ai/vendors/:id/models/refresh 响应：清单 diff */
export interface VendorModelsRefreshResult {
  added: string[]
  stale: string[]
  visible_count: number
}

/** DELETE /ai/vendors/:id 响应：级联计数（确认弹窗展示用） */
export interface VendorDeleteResult {
  deleted_models: number
  detached_bindings: number
}

/** GET /api/ai/health 的单条路由健康明细 */
export interface AIHealthRoute {
  route_name: string
  capability: string
  primary_model: string
  vendor: string
  model_kind: string
  reachable: boolean
  launched_by_backend: boolean
  last_checked: string
  error: string
}

/** GET /api/ai/health 的健康快照（checked_at 为 null 表示启动后首次检测未完成） */
export interface AIHealthSnapshot {
  healthy: boolean
  checked_at: string | null
  auto_start_models: boolean
  routes: AIHealthRoute[]
}

/** POST /api/ai/health/reprobe 的结果（triggered=已启动一次异步重探；skipped=已有探测在跑，未并发） */
export interface AIHealthReprobeResult {
  triggered: boolean
  skipped: boolean
}

/** 降级事件（specs/ai-route-observability）：from/to 为组内顺位（0-based）迁移；全链失败与 structured 事件两者为 null */
export interface DegradationEvent {
  occurred_at: string
  capability: string
  from: number | null
  to: number | null
  /** 降级后实际命中的模型（全链失败=最后尝试的模型） */
  model: string
  type: 'structured' | 'fallback'
  /** structured=降级方向（如 "json_schema→json_object"）；fallback=底层错误码 */
  code: string
}

/** 窗口统计：calls=调用组数（distinct trace），degradations=降级事件数（不含全链失败），failures=全链失败组数 */
export interface DegradationStats {
  calls: number
  degradations: number
  failures: number
}

/** GET /api/ai/observability/degradations 响应（events 按 occurred_at 降序） */
export interface DegradationReport {
  window: string
  stats: DegradationStats
  events: DegradationEvent[]
}
