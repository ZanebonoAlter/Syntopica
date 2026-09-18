/**
 * 订阅源发现 / 兴趣画像 类型定义（preference-vector-feed-discovery）
 *
 * API 响应为 snake_case，在 api 层 normalizer 转 camelCase，组件只用 camelCase。
 * 数字 id 在 API 边界转 string。
 */

/** 推荐卡片状态机：pending → accepted | dismissed */
export type RecommendationStatus = 'pending' | 'accepted' | 'dismissed'

/** 推荐来源：手动刷新 | 问答；二者共享幂等池与 dismiss 冷却池 */
export type RecommendationSource = 'manual_refresh' | 'qa'

/** 参数可选值字典条目（feed-param-options）。source 仅为 manual/scraped，绝不由 LLM 生成。 */
export interface RouteParamOption {
  value: string
  label: string
  source: string
}

/** 推荐卡片（GET /api/discovery/recommendations、POST /api/discovery/ask 的单条） */
export interface DiscoveryRecommendation {
  id: string
  routeId: string
  boardId: string | null
  /** 所属版块名；全局桶/无版块时后端返回空串，前端兜底「全局推荐」 */
  boardLabel: string
  source: RecommendationSource
  score: number
  llmReason: string
  status: RecommendationStatus
  routeNamespace: string
  routePath: string
  routeName: string
  routeExample: string
  /** true = 无必填参数，可一键订阅 */
  usableDirectly: boolean
  /** true = 有必填参数，需用户填写后验证订阅 */
  requiresParameters: boolean
  /** 目录自带的参数说明（原始 JSON 字符串，对象或数组），由 utils/routeParams 解析 */
  parameters: string
  /** 参数可选值字典（按 param_name 分组，只来自 manual/scraped 真实数据）；无字典数据为空对象 */
  paramOptions: Record<string, RouteParamOption[]>
  createdAt: string
}

/** POST /api/discovery/recommendations/refresh 的产出摘要 */
/** POST /api/discovery/recommendations/refresh 受理响应（异步契约：产出经 GET runs/:id 轮询；
 * 后端兼容占位的计数字段恒 0，前端不消费） */
export interface RefreshSummary {
  runId: string
  status: string
}

/** GET /api/discovery/catalog/status 的目录统计 */
export interface CatalogStatus {
  total: number
  ok: number
  broken: number
  unknown: number
  gone: number
  embedded: number
}

/**
 * POST /api/discovery/catalog/sync 的产出摘要。
 * 注意：后端 CatalogSyncSummary 未加 json tag，键为 PascalCase。
 */
export interface CatalogSyncSummary {
  Inserted: number
  Updated: number
  Gone: number
  Total: number
  NewToEmbed: number
}

/** 接受推荐成功后后端返回的 Feed（取展示所需字段） */
export interface AcceptedFeed {
  id: string
  title: string
  url: string
}

/** 偏好来源：行为重算 | 问答种子 */
export type PreferenceSource = 'behavior' | 'seed'

/* ————————————————— 候选源库（improve-discovery-recommendations） ————————————————— */

/** 候选来源类型：rss = 原生/手动 RSS；rsshub = RSSHub 目录路由 */
export type CandidateKind = 'rss' | 'rsshub'

/**
 * 候选可用性（C7「Availability Checks Reflect Actual Endpoints」）。
 * unknown = 未验证（仍可被推荐，不得硬过滤）；requires_parameters ≠ 失效。
 */
export type CandidateAvailability = 'unknown' | 'ok' | 'broken' | 'requires_parameters'

/** 候选源库单条（GET/POST/PATCH /api/discovery/candidates；snake_case 在 api 层归一） */
export interface DiscoveryCandidate {
  id: string
  kind: CandidateKind
  /** 有效名称（人工非空 → 上游 → 缺省），展示字段 */
  name: string
  /** 有效介绍 */
  description: string
  /** 语言 / 地区（人工补充，可为空串） */
  language: string
  region: string
  /** rss = feed URL；rsshub = 路由地址（namespace/path 模板，编辑时只读） */
  address: string
  /** 参与推荐开关（与订阅状态正交：关闭不取消订阅） */
  recommendationEnabled: boolean
  /** 是否已订阅（独立字段，与参与推荐分列展示） */
  subscribed: boolean
  /** 可用性状态 */
  availability: CandidateAvailability
  /** 最近检查时间；null = 无检查记录（明示未验证，不伪造时间） */
  lastCheckedAt: string | null
  /**
   * 上游路由原始资料（仅 rsshub 条目携带，rss 条目缺省/null）。
   * route.status === 'gone' = 上游已下架：条目保留并标明，订阅不取消但不再建议订阅（spec C2）。
   */
  route?: CandidateRouteInfo | null
  /** 人工元数据原文（后端 manual_metadata 原样透传；编辑回填用，避免把出口清洗值固化成人工覆盖） */
  manualMetadata?: Record<string, string>
}

/** 候选列表筛选（来源类型 / 参与推荐状态） */
export interface CandidateFilters {
  query: string
  kind: 'all' | CandidateKind
  participation: 'all' | 'enabled' | 'disabled'
}

/** 新增/编辑候选表单值（手动 RSS 语义；rsshub 仅人工字段可改） */
export interface CandidateFormInput {
  name: string
  url: string
  description: string
  language: string
  region: string
  recommendationEnabled: boolean
}

/** 保存候选的结果：成功 / 重复地址（带已有条目入口）/ 失败 */
export type SaveCandidateResult =
  | { status: 'saved', candidate: DiscoveryCandidate }
  | { status: 'duplicate', existing: DiscoveryCandidate | null, error?: string }
  | { status: 'error', error: string }

/* ————————— 目录导入导出（5.3；后端 candidate_catalog_transfer.go 同构契约） ————————— */

/** 导出/导入文件单条条目（snake_case 与后端 CatalogExportEntry 对齐；api 层原样透传） */
export interface CatalogExportEntry {
  stable_key: string
  kind: 'rss' | 'rsshub'
  name?: string
  description?: string
  language?: string
  region?: string
  feed_url?: string
  route_namespace?: string
  route_path?: string
  recommendation_enabled?: boolean
  manual_metadata?: Record<string, string>
}

/** 导出/导入文件整体（format=syntopica-candidate-catalog, version=1） */
export interface CatalogExportFile {
  format: string
  version: number
  entries: CatalogExportEntry[]
}

/** GET /api/discovery/candidates/export 响应：文件本体 + 默认安全导出排除计数（spec C4） */
export interface CatalogExportResult {
  export: CatalogExportFile
  /** access_scope != public 的私有条目数 */
  excluded_private: number
  /** rss 地址带 query（可能携带 token，保守排除） */
  excluded_query: number
  /** 敏感复核不通过：userinfo / 内网 IP 字面量 / 无法安全分类 */
  excluded_sensitive: number
}

/** 预览单条分类（new | duplicate | conflict | invalid，含原因） */
export interface CatalogPreviewItem {
  stable_key: string
  kind: string
  name: string
  classification: 'new' | 'duplicate' | 'conflict' | 'invalid'
  reason?: string
}

/** POST import/preview 响应：分类计数 + 逐项明细 + 确认回执凭据 */
export interface CatalogImportPreview {
  counts: { new: number, duplicate: number, conflict: number, invalid: number }
  items: CatalogPreviewItem[]
  fingerprint: string
  local_revision: number
}

/** POST import/confirm 响应：逐项结果（部分失败明确不是全部成功，spec C3） */
export interface CatalogApplyResult {
  applied: string[]
  skipped_duplicate: string[]
  skipped_conflict: Array<{ stable_key: string, reason: string }>
  failed: Array<{ stable_key: string, reason: string }>
  invalid: Array<{ stable_key: string, reason: string }>
}

/** 候选详情（GET /api/discovery/candidates/:id）：订阅弹窗的数据源 */
export interface CandidateDetail {
  id: string
  kind: CandidateKind
  /** 有效名称（人工非空 → 上游 → 缺省） */
  name: string
  /** rss 条目的规范化 feed 地址；rsshub 为空 */
  feedUrl: string
  subscribed: boolean
  /** rsshub 条目的上游路由原始资料；rss 为 null */
  route: CandidateRouteInfo | null
}

/** 候选详情附带的 RSSHub 上游路由（models.RSSHubRoute 序列化形状的 camelCase 归一） */
export interface CandidateRouteInfo {
  namespace: string
  path: string
  name: string
  /** 上游文档页原始 description（未经出口清洗的原文；编辑回填用，避免把清洗值固化成人工覆盖） */
  description: string
  example: string
  /** 目录自带参数说明（原始 JSON 字符串），由 utils/routeParams 解析 */
  parameters: string
  usableDirectly: boolean
  requiresParameters: boolean
  /**
   * 上游路由状态（models.RSSHubRoute.status）：ok | unknown | broken | gone。
   * gone = 上游已下架（RSSHub 目录同步确认消失）：保留条目与订阅，仅作展示与禁订阅依据；
   * broken 是端点可用性提示，不阻塞订阅（勿并入 gone 处理）。
   */
  status?: string
}

/* ————————— 兴趣记录 / 查询 run（5.2 消费，客户端契约） ————————— */

/** 兴趣记录状态：参与中 | 已衰减（窗口/成熟度让位，≠ 拒绝） | 旧数据迁移 */
export type InterestStatus = 'active' | 'faded' | 'legacy'

/** GET /api/discovery/interests 单条：逐条问答兴趣，不合成平均画像 */
export interface DiscoveryInterest {
  id: string
  /** 本次查询原句 */
  queryText: string
  /** 归属版块名；null = 未匹配版块（独立保留，不挂标签） */
  boardLabel: string | null
  /** 记录时间 */
  createdAt: string
  status: InterestStatus
}

/** 手动查询状态机（test-cases S1）：闲置 | 执行中 | 成功 | 失败 */
export type AskStatus = 'idle' | 'running' | 'succeeded' | 'failed'

/** POST /api/discovery/ask 新契约响应（design D2/D9）：返回启动的查询 run id，前端轮询 runs/:id */
export interface AskStarted {
  runId: string
}

/** GET /api/discovery/runs/:id 的单条结果项 */
export interface DiscoveryRunItem {
  candidateId: string
  name: string
  description: string
  /** 推荐理由快照 */
  reason: string
  /** 实际召回来源徽标（版块/行为/问答，可有多个） */
  recallOrigins: string[]
  /** 可用性（unknown = 未验证：明示不伪造，不冒称可用/失效） */
  availability: CandidateAvailability
}

/** GET /api/discovery/runs/:id：手动查询的独立结果，不覆盖个性化列表 */
export interface DiscoveryRun {
  id: string
  kind: string
  /** 本次查询输入 */
  query: string
  status: string
  startedAt: string
  finishedAt: string | null
  items: DiscoveryRunItem[]
}

/**
 * 历史条目生命周期状态（R5 Recommendation Lifecycle and Exclusion）：
 * accepted=已订阅 | expired=自动过期（≠ 拒绝） | snoozed=暂时不看（冷却中） | excluded=长期排除。
 * restored 为前端「恢复推荐」成功后的本地展示态，不由后端下发；
 * unknown = 后端字段缺失/未识别的降级态（显示「状态未知」，不崩溃）。
 */
export type HistoryEntryStatus = 'accepted' | 'expired' | 'snoozed' | 'excluded' | 'restored' | 'unknown'

/** GET /api/discovery/recommendations?scope=history 单条（历史区聚合视图） */
export interface DiscoveryHistoryItem {
  id: string
  /** 有效名称；缺失时归一为「未知来源」 */
  name: string
  status: HistoryEntryStatus
  /** 暂时不看的冷却到期时间；其余状态为 null */
  snoozedUntil: string | null
  /** 上次入选/展示时间；null = 无记录 */
  lastSelectedAt: string | null
  /** 当时推荐理由快照 */
  reason: string
}

/** GET /api/preference-profile 的单条画像 */
export interface PreferenceProfileItem {
  boardId: string | null
  /** 版块名；全局桶后端返回「全局」 */
  boardLabel: string
  source: PreferenceSource
  /** { 标签名: 权重 } top 列表 */
  tagWeights: Record<string, number>
  lastComputedAt: string | null
}

/**
 * POST /api/preference-profile/recompute 的产出摘要。
 * 注意：后端 RecomputeSummary 未加 json tag，键为 PascalCase。
 */
export interface RecomputeSummary {
  BoardsComputed: number
  TagsUsed: number
  ArticleCount: number
}
