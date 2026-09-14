import { apiClient } from './client'
import type { ApiResponse } from '~/types'
import type {
  AcceptedFeed,
  AskStarted,
  CandidateDetail,
  CandidateFormInput,
  CandidateRouteInfo,
  CatalogApplyResult,
  CatalogExportFile,
  CatalogExportResult,
  CatalogImportPreview,
  CatalogStatus,
  CatalogSyncSummary,
  CandidateAvailability,
  CandidateFilters,
  CandidateKind,
  DiscoveryCandidate,
  DiscoveryHistoryItem,
  DiscoveryInterest,
  DiscoveryRecommendation,
  DiscoveryRun,
  DiscoveryRunItem,
  HistoryEntryStatus,
  InterestStatus,
  RecommendationStatus,
  RefreshSummary,
  RouteParamOption,
} from '~/types/discovery'

/** 候选列表分页默认值（与后端 CandidateListDefaultPageSize 一致）。 */
const CandidateDefaultPageSize = 30

/**
 * 前端筛选语义 → 后端 wire 布尔：all = 不筛选（不下发该参数）。
 * 后端 ListCandidates 只认 recommendation_enabled=true|false，没有 "all" 取值。
 */
function recommendationEnabledParam(
  participation?: CandidateFilters['participation'],
): boolean | undefined {
  if (participation === 'enabled') return true
  if (participation === 'disabled') return false
  return undefined
}

/** 后端推荐卡片 payload（snake_case；route/board 预加载字段此处不展开）。 */
interface RecommendationPayload {
  id: number
  route_id: number
  board_id?: number | null
  source: string
  score: number
  llm_reason: string
  status: string
  created_at: string
  route_namespace: string
  route_path: string
  route_name: string
  route_example: string
  usable_directly: boolean
  requires_parameters: boolean
  parameters: string
  /** 参数可选值字典（按 param_name 分组）；无字典数据为空对象 */
  param_options?: Record<string, RouteParamOption[]>
  board_label: string
}

interface AcceptedFeedPayload {
  id: number
  title: string
  url: string
}

function normalizeCard(p: RecommendationPayload): DiscoveryRecommendation {
  return {
    id: String(p.id),
    routeId: String(p.route_id),
    boardId: p.board_id != null ? String(p.board_id) : null,
    boardLabel: p.board_label || '',
    source: p.source === 'qa' ? 'qa' : 'manual_refresh',
    score: p.score ?? 0,
    llmReason: p.llm_reason || '',
    status: (p.status || 'pending') as RecommendationStatus,
    routeNamespace: p.route_namespace || '',
    routePath: p.route_path || '',
    routeName: p.route_name || '',
    routeExample: p.route_example || '',
    usableDirectly: Boolean(p.usable_directly),
    requiresParameters: Boolean(p.requires_parameters),
    parameters: p.parameters || '{}',
    paramOptions: p.param_options ?? {},
    createdAt: p.created_at || '',
  }
}

/**
 * 订阅源发现 API：推荐卡片 / 问答 / 目录同步。
 * 所有 HTTP 经 ApiClient；snake→camel 与 id 字符串化在 normalizer 完成。
 */
export function useDiscoveryApi() {
  /** GET /api/discovery/recommendations?status=pending — 推荐卡片列表。 */
  async function getRecommendations(status: RecommendationStatus = 'pending'): Promise<ApiResponse<DiscoveryRecommendation[]>> {
    const res = await apiClient.get<RecommendationPayload[]>(`/discovery/recommendations?status=${status}`)
    if (res.success && res.data) return { ...res, data: res.data.map(normalizeCard) }
    return { ...res, data: [] as DiscoveryRecommendation[] }
  }

  /** POST /api/discovery/recommendations/refresh — 换一批（粗筛+精排+幂等落库）。 */
  async function refreshRecommendations(): Promise<ApiResponse<RefreshSummary>> {
    return apiClient.post<RefreshSummary>('/discovery/recommendations/refresh', {})
  }

  /** POST /api/discovery/recommendations/:id/accept — 接受（直订 / 填参验证后订阅）。 */
  async function acceptRecommendation(
    id: string,
    opts: { categoryId?: string, parameters?: Record<string, string> } = {},
  ): Promise<ApiResponse<AcceptedFeed>> {
    const body: Record<string, unknown> = {}
    if (opts.categoryId) body.category_id = Number(opts.categoryId)
    if (opts.parameters && Object.keys(opts.parameters).length > 0) body.parameters = opts.parameters
    const res = await apiClient.post<AcceptedFeedPayload>(`/discovery/recommendations/${id}/accept`, body)
    if (res.success && res.data) {
      return { ...res, data: { id: String(res.data.id), title: res.data.title, url: res.data.url } }
    }
    return { success: false, error: res.error }
  }

  /** POST /api/discovery/recommendations/:id/dismiss — 拒绝（冷却期内不再推荐）。 */
  async function dismissRecommendation(id: string): Promise<ApiResponse<null>> {
    return apiClient.post<null>(`/discovery/recommendations/${id}/dismiss`, {})
  }

  /**
   * POST /api/discovery/ask — 发起手动查询，返回 { run_id }（design D2/D9 新契约）。
   * 查询结果经 GET /discovery/runs/:id 轮询获取，独立展示不覆盖个性化列表；
   * 旧「ask 直接返回推荐卡片」契约由本 change 替换，不再保留。
   */
  async function askDiscovery(question: string): Promise<ApiResponse<AskStarted>> {
    const res = await apiClient.post<{ run_id: number | string }>('/discovery/ask', { question })
    if (res.success && res.data) {
      return { ...res, data: { runId: String(res.data.run_id) } }
    }
    return { success: false, error: res.error, status: res.status }
  }

  /** GET /api/discovery/catalog/status — 目录状态统计。 */
  async function getCatalogStatus(): Promise<ApiResponse<CatalogStatus>> {
    return apiClient.get<CatalogStatus>('/discovery/catalog/status')
  }

  /** POST /api/discovery/catalog/sync — 手动触发目录同步。 */
  async function syncCatalog(): Promise<ApiResponse<CatalogSyncSummary>> {
    return apiClient.post<CatalogSyncSummary>('/discovery/catalog/sync', {})
  }

  /* —————— 候选源库（feed-candidate-catalog，D9：/api/discovery/candidates） —————— */

  /** 后端候选 payload（snake_case；manual/upstream 合成后的有效展示字段）。 */
  interface CandidatePayload {
    id: number
    kind: string
    name: string
    description: string
    language: string
    region: string
    /** 展示用地址；后端不直接下发时由 feed_url / route 推导（见 candidateAddress） */
    address?: string
    /** rss 条目的实际 feed 地址；rsshub 条目缺省 */
    feed_url?: string | null
    recommendation_enabled: boolean
    subscribed: boolean
    availability: string
    last_checked_at?: string | null
    /** rsshub 条目的上游原始资料（snake_case）；rss 条目缺省（omitempty） */
    route?: RoutePayload | null
  }

  /** 后端 rsshub_routes 序列化形状（snake_case）。 */
  interface RoutePayload {
    namespace?: string
    path?: string
    name?: string
    example?: string
    parameters?: string
    usable_directly?: boolean
    requires_parameters?: boolean
    /** unknown | ok | broken | gone */
    status?: string
  }

  /** 列表响应形状：后端为 { items, total }（兼容既有扁平数组 mock/契约）。 */
  interface CandidateListPayload {
    items?: CandidatePayload[]
    total?: number
  }

  /** 上游路由归一：列表与详情共用，必须保留 status（gone = 上游已下架，spec C2）。 */
  function normalizeRoute(r: RoutePayload): CandidateRouteInfo {
    return {
      namespace: r.namespace || '',
      path: r.path || '',
      name: r.name || '',
      example: r.example || '',
      parameters: r.parameters || '{}',
      usableDirectly: Boolean(r.usable_directly),
      requiresParameters: Boolean(r.requires_parameters),
      status: r.status || '',
    }
  }

  /** 展示地址推导：显式 address > rss 的 feed_url > rsshub 的 namespace+path（与后端候选视图字段对齐）。 */
  function candidateAddress(p: CandidatePayload): string {
    if (p.address) return p.address
    if (p.feed_url) return p.feed_url
    if (p.route) return `${p.route.namespace || ''}${p.route.path || ''}`
    return ''
  }

  function normalizeCandidate(p: CandidatePayload): DiscoveryCandidate {
    const kind: CandidateKind = p.kind === 'rsshub' ? 'rsshub' : 'rss'
    const availability: CandidateAvailability = (
      ['unknown', 'ok', 'broken', 'requires_parameters'] as const
    ).includes(p.availability as CandidateAvailability)
      ? (p.availability as CandidateAvailability)
      : 'unknown'
    return {
      id: String(p.id),
      kind,
      name: p.name || '',
      description: p.description || '',
      language: p.language || '',
      region: p.region || '',
      address: candidateAddress(p),
      recommendationEnabled: Boolean(p.recommendation_enabled),
      subscribed: Boolean(p.subscribed),
      availability,
      lastCheckedAt: p.last_checked_at ?? null,
      route: p.route ? normalizeRoute(p.route) : null,
    }
  }

  /**
   * GET /api/discovery/candidates — 分页默认 30、上限 100；q ≤ 500 rune（design D9）。
   *
   * 前端类型层保留语义命名（CandidateFilters），wire 名在此映射对齐后端 ListCandidates：
   * query→q（trim 后下发，空串不下发）、perPage→page_size、kind→kind（'all' 不下发，
   * 由后端取全集）、participation→recommendation_enabled（enabled=true / disabled=false /
   * all 不下发）。映射必须停在本层：发旧名（query/per_page/participation）后端会静默忽略，
   * 表现为「搜索与筛选点了没反应」。
   */
  async function getCandidates(
    params: Partial<CandidateFilters> & { page?: number, perPage?: number } = {},
  ): Promise<ApiResponse<DiscoveryCandidate[]>> {
    const query = apiClient.buildQueryParams({
      page: params.page,
      page_size: params.perPage,
      q: params.query?.trim() || undefined,
      kind: params.kind && params.kind !== 'all' ? params.kind : undefined,
      recommendation_enabled: recommendationEnabledParam(params.participation),
    })
    const res = await apiClient.get<CandidatePayload[] | CandidateListPayload>(
      `/discovery/candidates${query ? `?${query}` : ''}`,
    )
    if (res.success && res.data) {
      const raw = res.data
      const items = Array.isArray(raw) ? raw : (raw.items ?? [])
      const total = !Array.isArray(raw) && typeof raw.total === 'number' ? raw.total : undefined
      return {
        ...res,
        data: items.map(normalizeCandidate),
        pagination: res.pagination
          ?? (total === undefined
            ? undefined
            : {
                page: params.page ?? 1,
                pages: Math.max(1, Math.ceil(total / (params.perPage ?? CandidateDefaultPageSize))),
                per_page: params.perPage ?? CandidateDefaultPageSize,
                total,
              }),
      }
    }
    return { ...res, data: [] as DiscoveryCandidate[] }
  }

  /**
   * POST /api/discovery/candidates — 手动新增（仅入库，不订阅不抓取）。
   *
   * wire 名对齐后端 service.CandidateCreateInput：地址字段是 `feed_url`（不是 `url`）。
   * 发 `url` 会被 ShouldBindJSON 静默丢弃 → 后端报「feed url must not be empty」，
   * 表现为「填了地址还提示地址为空」。类型/组件层继续用语义名 `url`，映射只在本层。
   *
   * recommendation_enabled 同样对齐后端同名 tag（指针语义）：新建时「参与推荐」开关就能落库，
   * 不再依赖后端硬编码默认 true；缺省不发的语义仍是 true。
   */
  async function createCandidate(input: CandidateFormInput): Promise<ApiResponse<DiscoveryCandidate>> {
    const res = await apiClient.post<CandidatePayload>('/discovery/candidates', {
      name: input.name,
      feed_url: input.url,
      description: input.description,
      language: input.language,
      region: input.region,
      recommendation_enabled: input.recommendationEnabled,
    })
    if (res.success && res.data) return { ...res, data: normalizeCandidate(res.data) }
    // 409 冲突体携带已有条目（client 保留 data），供「已存在入口」提示
    if (res.status === 409 && res.data) {
      return { ...res, data: normalizeCandidate(res.data as unknown as CandidatePayload) }
    }
    return { success: false, error: res.error, status: res.status, data: undefined }
  }

  /**
   * PATCH /api/discovery/candidates/:id — 编辑人工字段 / 原生 RSS 地址 / 推荐启停（不触订阅）。
   * 地址字段同样是 `feed_url`（后端 CandidateUpdateInput.FeedURL，仅 kind=rss 接受）；
   * rsshub 的 route 地址由上游表提供，前端 urlEditable=false 不下发该字段。
   */
  async function updateCandidate(
    id: string,
    patch: Partial<CandidateFormInput>,
  ): Promise<ApiResponse<DiscoveryCandidate>> {
    const body: Record<string, unknown> = {}
    if (patch.name !== undefined) body.name = patch.name
    if (patch.url !== undefined) body.feed_url = patch.url
    if (patch.description !== undefined) body.description = patch.description
    if (patch.language !== undefined) body.language = patch.language
    if (patch.region !== undefined) body.region = patch.region
    if (patch.recommendationEnabled !== undefined) body.recommendation_enabled = patch.recommendationEnabled
    const res = await apiClient.patch<CandidatePayload>(`/discovery/candidates/${id}`, body)
    if (res.success && res.data) return { ...res, data: normalizeCandidate(res.data) }
    if (res.status === 409 && res.data) {
      return { ...res, data: normalizeCandidate(res.data as unknown as CandidatePayload) }
    }
    return { success: false, error: res.error, status: res.status, data: undefined }
  }

  /* —————— 兴趣记录 / 查询 run / 推荐历史（D9；列表 UI 在 5.2 切片接线） —————— */

  /** GET /api/discovery/interests 的后端 payload（snake_case）。 */
  interface InterestPayload {
    id: number
    query_text: string
    board_label?: string | null
    created_at: string
    status: string
  }

  function normalizeInterest(p: InterestPayload): DiscoveryInterest {
    const status: InterestStatus = p.status === 'active' || p.status === 'faded' ? p.status : 'legacy'
    return {
      id: String(p.id),
      queryText: p.query_text || '',
      boardLabel: p.board_label ?? null,
      createdAt: p.created_at || '',
      status,
    }
  }

  /** GET /api/discovery/interests — 逐条问答兴趣（不合成平均画像）。 */
  async function getInterests(): Promise<ApiResponse<DiscoveryInterest[]>> {
    const res = await apiClient.get<InterestPayload[]>('/discovery/interests')
    if (res.success && res.data) return { ...res, data: res.data.map(normalizeInterest) }
    return { ...res, data: [] as DiscoveryInterest[] }
  }

  /** GET /api/discovery/runs/:id 的后端 payload（snake_case）。 */
  interface RunItemPayload {
    candidate_id: number | string
    name: string
    description: string
    reason: string
    recall_origins?: string[]
    availability?: string
  }

  interface RunPayload {
    id: number | string
    kind: string
    query: string
    status: string
    started_at: string
    finished_at?: string | null
    items?: RunItemPayload[]
  }

  function normalizeRunItem(p: RunItemPayload): DiscoveryRunItem {
    const availability = (
      ['unknown', 'ok', 'broken', 'requires_parameters'] as const
    ).includes(p.availability as CandidateAvailability)
      ? (p.availability as CandidateAvailability)
      : 'unknown'
    return {
      candidateId: String(p.candidate_id),
      name: p.name || '',
      description: p.description || '',
      reason: p.reason || '',
      recallOrigins: Array.isArray(p.recall_origins) ? p.recall_origins : [],
      availability,
    }
  }

  function normalizeRun(p: RunPayload): DiscoveryRun {
    return {
      id: String(p.id),
      kind: p.kind || '',
      query: p.query || '',
      status: p.status || 'running',
      startedAt: p.started_at || '',
      finishedAt: p.finished_at ?? null,
      items: Array.isArray(p.items) ? p.items.map(normalizeRunItem) : [],
    }
  }

  /** GET /api/discovery/runs/:id — 手动查询的独立 run 结果（不覆盖个性化列表）。 */
  async function getRun(id: string): Promise<ApiResponse<DiscoveryRun>> {
    const res = await apiClient.get<RunPayload>(`/discovery/runs/${id}`)
    if (res.success && res.data) return { ...res, data: normalizeRun(res.data) }
    return { success: false, error: res.error, status: res.status }
  }

  /** GET /api/discovery/recommendations?scope=history 的后端 payload（snake_case）。 */
  interface HistoryPayload {
    id: number
    name: string
    status: string
    snoozed_until?: string | null
    last_selected_at?: string | null
    llm_reason?: string
  }

  function normalizeHistoryItem(p: HistoryPayload): DiscoveryHistoryItem {
    const known = ['accepted', 'expired', 'snoozed', 'excluded'] as const
    const status: HistoryEntryStatus = (known as readonly string[]).includes(p.status)
      ? (p.status as HistoryEntryStatus)
      : 'unknown'
    return {
      id: String(p.id),
      name: p.name || '未知来源',
      status,
      snoozedUntil: p.snoozed_until ?? null,
      lastSelectedAt: p.last_selected_at ?? null,
      reason: p.llm_reason || '',
    }
  }

  /**
   * GET /api/discovery/recommendations?scope=history — 推荐历史聚合视图（R5）。
   * 选 scope= 而非 status=：历史是跨 accepted/expired/snoozed/excluded 的聚合视图，
   * 不是单一状态值；沿用 recommendations 列表资源与既有 query 参数惯例（design D9）。
   */
  async function getRecommendationHistory(): Promise<ApiResponse<DiscoveryHistoryItem[]>> {
    const res = await apiClient.get<HistoryPayload[]>('/discovery/recommendations?scope=history')
    if (res.success && res.data) return { ...res, data: res.data.map(normalizeHistoryItem) }
    return { ...res, data: [] as DiscoveryHistoryItem[] }
  }

  /** POST /api/discovery/recommendations/:id/restore — 恢复长期排除（仅恢复资格，R5「恢复不是订阅」）。 */
  async function restoreRecommendation(id: string): Promise<ApiResponse<null>> {
    return apiClient.post<null>(`/discovery/recommendations/${id}/restore`, {})
  }

  /* —————— 目录导入导出（5.3，spec C3/C4；文件本体 api 层原样透传，snake_case 与后端同构） —————— */

  /** GET /api/discovery/candidates/export — 默认安全导出：文件本体 + 排除计数。永不带 include_private（v1 不支持，前端预拦截）。 */
  async function exportCandidates(): Promise<ApiResponse<CatalogExportResult>> {
    return apiClient.get<CatalogExportResult>('/discovery/candidates/export')
  }

  /** POST /api/discovery/candidates/import/preview — 请求体 = 导入文件 JSON 本体；文件级问题 400 就地展示。 */
  async function previewCatalogImport(file: CatalogExportFile): Promise<ApiResponse<CatalogImportPreview>> {
    return apiClient.post<CatalogImportPreview>('/discovery/candidates/import/preview', file)
  }

  /** POST /api/discovery/candidates/import/confirm — 回传预览凭据 + 文件本体；409 stale_preview 由调用方识别重新预览。 */
  async function confirmCatalogImport(input: {
    fingerprint: string
    localRevision: number
    file: CatalogExportFile
  }): Promise<ApiResponse<CatalogApplyResult>> {
    const res = await apiClient.post<CatalogApplyResult>('/discovery/candidates/import/confirm', {
      fingerprint: input.fingerprint,
      local_revision: input.localRevision,
      import: input.file,
    })
    if (res.success && res.data) return res
    return { ...res, data: undefined }
  }

  /* —————— 候选详情（订阅弹窗数据源；GET /api/discovery/candidates/:id） —————— */

  /** 后端 CandidateView 形状（snake_case；仅取订阅所需字段）。 */
  interface CandidateDetailPayload {
    id: number
    kind: string
    name: string
    feed_url?: string | null
    subscribed: boolean
    route?: RoutePayload | null
  }

  function normalizeCandidateDetail(p: CandidateDetailPayload): CandidateDetail {
    const kind: CandidateKind = p.kind === 'rsshub' ? 'rsshub' : 'rss'
    return {
      id: String(p.id),
      kind,
      name: p.name || '',
      feedUrl: p.feed_url ?? '',
      subscribed: Boolean(p.subscribed),
      route: p.route ? normalizeRoute(p.route) : null,
    }
  }

  /** GET /api/discovery/candidates/:id — 单条候选（rsshub 附上游 Route；订阅弹窗拉取）。 */
  async function getCandidateDetail(id: string): Promise<ApiResponse<CandidateDetail>> {
    const res = await apiClient.get<CandidateDetailPayload>(`/discovery/candidates/${id}`)
    if (res.success && res.data) return { ...res, data: normalizeCandidateDetail(res.data) }
    return { ...res, data: undefined }
  }

  return {
    getRecommendations,
    refreshRecommendations,
    acceptRecommendation,
    dismissRecommendation,
    getCatalogStatus,
    syncCatalog,
    getCandidates,
    createCandidate,
    updateCandidate,
    getInterests,
    getRun,
    askDiscovery,
    getRecommendationHistory,
    restoreRecommendation,
    exportCandidates,
    previewCatalogImport,
    confirmCatalogImport,
    getCandidateDetail,
  }
}
