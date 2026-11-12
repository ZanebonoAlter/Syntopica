import { apiClient } from './client'
import type { ApiResponse, BoardSourceBreakdown } from '~/types'

export interface SemanticBoard {
  id: number
  label: string
  slug: string
  aliases: string[]
  ref_count: number
  tag_count: number
  description: string
  display_order: number
  source: string
  status: string
  protected: boolean
  /** 循环 B 增强开关（板块级，默认 false）。 */
  enrichment_enabled: boolean
  /** 跨版块关系自动发现开关（add-evidence-backed-cross-board-relations；旧数据缺省 false）。 */
  relation_auto_discovery_enabled?: boolean
  /** 循环 B 实时详情窗口（默认 14）。 */
  window_days: number
  /** 解读员读取的上下文层（默认 week/month/year/all）。 */
  context_layers: string[]
  created_at: string
  updated_at: string
}

export interface AuxiliaryLabelItem {
  id: number
  label: string
  slug: string
  aliases: string[]
  ref_count: number
  description: string
  display_order: number
  source: string
  status: string
  protected: boolean
}

/** 版块挂载的组合标签条目（add-composite-labels：composition 列复用）。 */
export interface BoardCompositeMount {
  id: number
  label: string
  slug: string
  status: string
  ref_count: number
  components: string[]
}

// ── 版块体检（add-board-health-checkup）：GET /semantic-boards/:id/health-checkup ──

/** 体检快照·标签级纯净样本归因行（样本不足时分布字段为 null）。 */
export interface BoardHealthLabelMetric {
  aux_id: number
  label: string
  /** 构成标签类型（auxiliary | composite）；缺字段 = 旧快照（类型未知）。 */
  label_type?: string
  /**
   * 核心主体标记（fix-board-health-magnet-signal D3）：标签名出现在版块标题 /
   * description / aliases 任一文本中。核心主体是版块自身要谈论的对象，
   * MUST NOT 作为摘除或重写对象。缺字段 = 旧快照（按「非核心主体」处理，不臆造）。
   */
  core_subject?: boolean
  samples: number
  cosine_mean: number | null
  cosine_median: number | null
  cosine_p25: number | null
  cosine_p75: number | null
  low_sim_ratio: number | null
  insufficient: boolean
  /** 体检时刻的描述快照（病C 描述污染证据源；空 = 该标签无描述）。 */
  description?: string
  /** 磁铁信号（磁铁污染嫌疑）；缺字段/null = 未算或信号不足。 */
  magnet?: BoardHealthMagnetSignal | null
  /** pgvector top-3 最近邻（排除自身）；null = 自身向量缺失（信号不足）。 */
  neighbors?: BoardHealthNeighbor[] | null
  /** 命中的文章式词根（事件性预筛）；缺字段/空 = 未命中。 */
  desc_flags?: string[] | null
}

/**
 * 体检快照·磁铁信号（构成标签对失败挂载子集 vs 健康挂载子集的贴合度差）。
 *
 * 判定口径（fix-board-health-magnet-signal D1）：`failed_mean - good_mean ≥ 阈值`
 * 才成立——「磁铁」的语义是「把跑题的挂载吸过来」，失败侧贴合均值必须**高于**健康侧。
 *
 * `good_mean` / `good_samples` 是判定用的健康侧；`all_mean` / `all_samples` **保留
 * 但退出嫌疑判定**（历史快照解析 + 旧口径回退展示）。
 *
 * 口径标记 `caliber`（fix-board-health-magnet-signal D12 / review R-4）：
 *  - `'healthy_subset'` = 新口径（本轮由后端写入；**与样本是否充足无关**——
 *    健康侧不可算时照样写这个值）；
 *  - 空串 / 缺字段 = 旧口径快照（历史快照或该字段写入之前的快照）。
 *
 * 消费方 MUST 按 `caliber` 区分「旧口径快照」与「新快照但健康侧不可算」——
 * `good_mean` 无 `omitempty`，新快照在健康侧不可算时同样是 null，两者不可区分。
 */
export interface BoardHealthMagnetSignal {
  /** 健康挂载子集（direction_mismatch=false）贴合均值；缺字段/null = 旧快照或该侧无样本。 */
  good_mean?: number | null
  /**
   * 磁铁口径标记：`healthy_subset` = 新口径（失败子集 vs 健康子集）；
   * 空串 / 缺字段 = 旧口径快照（全体口径）。
   */
  caliber?: string
  failed_mean: number | null
  all_mean: number | null
  /** 健康挂载子集样本数；缺字段 = 旧快照（按 0 处理，不臆造）。 */
  good_samples?: number
  failed_samples: number
  all_samples: number
  suspicious: boolean
  insufficient: boolean
}

/**
 * 体检快照·分诊合理性闸门结论（fix-board-health-magnet-signal D6/G3）。
 *
 * `actionable` / `ratio` 描述的是**降级前**的状态（= 闸门触发原因），
 * 不是降级后的条数；`degraded` = 本轮可执行项是否被整轮降级为证据不足。
 */
export interface BoardHealthTriageGuard {
  /** 降级前的可执行项条数（remove + rewrite）。 */
  actionable: number
  /** 分诊清单总条数。 */
  total: number
  /** 可执行占比（0-1）；降级前的状态。 */
  ratio: number
  /** 闸门阈值（0-1，默认 0.8）。 */
  threshold: number
  /** 本轮是否被闸门降级。 */
  degraded: boolean
  /** 降级原因（服务端文案）。 */
  reason: string
}

/** 体检快照·最近邻证据（active 辅助标签 top-3，排除自身）。 */
export interface BoardHealthNeighbor {
  aux_id: number
  label: string
  cosine: number
}

/** 体检快照·失败挂载主要流向（top_board 为 null = 未知，不臆造目标）。 */
export interface BoardHealthMagnetFlow {
  failed_tags: number
  flow_tags: number
  top_board: string | null
  top_tags: number
}

/** 体检快照·标签级分诊裁决（category 四值：remove|rewrite|keep|insufficient）。 */
export interface BoardHealthTriageItem {
  aux_id: number
  category: string
  evidence: string
  suggestion?: string
}

/** 体检快照·版块级指标（全部匹配行口径）。 */
export interface BoardHealthBoardMetric {
  matched_tags: number
  reason_dist: Record<string, number>
  ghost_matches: number
  vector_missing: number
  /** 失败挂载主要流向证据；缺字段/null = 本轮无失败挂载或未知。 */
  magnet_flow?: BoardHealthMagnetFlow | null
}

export interface BoardHealthMetrics {
  board: BoardHealthBoardMetric
  labels: BoardHealthLabelMetric[]
  /** 标签级分诊清单；缺字段/null = 历史快照/降级轮次（按旧形态展示）。 */
  triage?: BoardHealthTriageItem[] | null
  /**
   * 分诊合理性闸门结论；缺字段/null = 无闸门记录（旧快照，或本轮未产生闸门结论）。
   */
  triage_guard?: BoardHealthTriageGuard | null
}

/** 未收编候选（事实画像 diff：成员 tags 高频携带但不在版块构成中）。 */
export interface BoardHealthCandidate {
  aux_id: number
  label: string
  member_tags: number
  member_articles: number
}

export interface BoardHealthSnapshotDTO {
  id: number
  semantic_board_id: number
  checked_at: string
  metrics: BoardHealthMetrics
  uncaptured_candidates: BoardHealthCandidate[]
  llm_score: number | null
  llm_comment: string | null
  created_at: string
}

export interface BoardHealthTrendPoint {
  checked_at: string
  llm_score: number | null
}

export interface BoardHealthCheckupResponse {
  latest: BoardHealthSnapshotDTO | null
  trend: BoardHealthTrendPoint[]
}

export interface BoardCompositionResponse {
  items: AuxiliaryLabelItem[]
  composites?: BoardCompositeMount[]
  total: number
}

/** 四格生成参数（split-board-upgrade-directions）：方向 × 来源 + 扩充锁定版块 + 时间窗。 */
export interface UpgradeGenerateParams {
  direction: 'create' | 'expand'
  source: 'aux' | 'composite'
  /** direction=expand 必填：生成前锁定单版块，本轮建议 target 恒等于它。 */
  target_board_id?: number
  /** 仅 create×aux 生效（候选时间窗；0=不过滤）。 */
  days?: number
}

/** 持久化建议行（GET /upgrade-suggestions）。字段对齐后端 boardUpgradeSuggestionRowDTO。 */
export interface UpgradeSuggestionRow {
  id: number
  batch_id: string
  mode: string
  decision: string
  board_label: string
  description: string
  target_board_id?: number
  target_board_label?: string
  auxiliary_label_ids: number[]
  auxiliary_labels: { id: number; label: string; status?: string }[]
  confidence: string
  /** 证据快照 {shortlist, margins, cotag_events, lane_briefs, ...}，按 key 安全读取，缺 key 降级。 */
  evidence?: Record<string, unknown>
  status: string
  dismiss_reason?: string
  created_at: string
  resolved_at?: string
}

/** POST /upgrade-suggestions/generate 返回的计数。 */
export interface GenerateSuggestionsResponse {
  inserted: number
  skipped: number
  cooldown_blocked: number
}

export interface UpgradeSuggestionsListResponse {
  suggestions: UpgradeSuggestionRow[]
}

export interface BackfillTask {
  id: string
  mode: string
  board_id?: number
  total: number
  processed: number
  failed: number
  status: 'pending' | 'running' | 'completed' | 'failed'
  failures: string[]
  created_at: string
}

export interface MatchingConfig {
  semantic_board_match_sim_threshold: number
  semantic_board_match_direct_hit_rate: number
  semantic_board_match_direct_max_sim: number
  semantic_board_match_direct_max_sim_min_hits: number
  semantic_board_match_direct_max_sim_min_hit_rate: number
  semantic_board_match_min_effective_sample: number
  semantic_board_match_hit_rate_sim_blend: number
  semantic_board_match_weight_sim: number
  semantic_board_match_weight_density: number
  semantic_board_match_weighted_threshold: number
  semantic_board_match_max_boards: number
  semantic_board_match_direct_hit_min_overlap: number
  semantic_board_match_direction_sim_threshold: number
  semantic_board_upgrade_ref_count_threshold: number
  semantic_board_upgrade_cluster_distance_threshold: number
  semantic_board_upgrade_cotag_window_days: number
  semantic_board_upgrade_cotag_top_n: number
  semantic_board_upgrade_cotag_dedupe_sim_threshold: number
  semantic_board_upgrade_cotag_hard_limit: number
  semantic_board_upgrade_cluster_method: string
  /**
   * 相关事件相似度阈值（enable-story-cross-day-continuation；默认 0.86、范围 [0,1]）。
   * 与入板匹配参数彼此独立：只影响「相关事件」列表，不重算向量、不参与命中即停顺序。
   */
  related_event_similarity_threshold: number
  /**
   * 候选泳道在版块关注度里的权重（lane-heat-metric D8；默认 0.5、范围 [0,1] 有限数）。
   * 只进「当天线索热度」计算，SHALL NOT 参与话题生命周期与入板匹配。
   * 缺省 = 旧后端未下发（前端回退默认 0.5）。
   */
  lane_heat_candidate_weight?: number
}

/** 相关事件条目的可读来源（候选标签在该版块的最新文章；无来源如实为 null）。 */
export interface RelatedEventSourceArticle {
  id: number
  title: string
}

export interface RelatedEventItem {
  tag_id: number
  label: string
  description: string
  /** 语义相似度（cosine），不是「同一事件概率」。 */
  similarity: number
  source_article: RelatedEventSourceArticle | null
}

/**
 * 相关事件只读响应（GET /semantic-boards/:id/tags/:tagId/related-events）。
 * status 三态如实上报索引覆盖：ready / partial / not_ready —— 缺索引不得当作空结果。
 */
export interface RelatedEventsResponse {
  board_id: number
  tag_id: number
  status: 'ready' | 'partial' | 'not_ready'
  reason?: string
  threshold: number
  window_days: number
  window_start: string
  window_end: string
  candidate_count: number
  indexed_count: number
  representation_version: string
  space_key?: string
  items: RelatedEventItem[]
}

export interface SuggestedAuxiliaryLabel extends AuxiliaryLabelItem {
  similarity: number
}

export interface SuggestAuxiliariesResponse {
  items: SuggestedAuxiliaryLabel[]
  total: number
  page: number
  page_size: number
}

export interface BoardArticleTag {
  id: number
  label: string
  category: string
  /** 标签描述（topic_tags.description；后端 8G.3a 起随 filtered_tags 返回）。 */
  description?: string
  match_reason: string
  score: number
  direction_mismatch: boolean
}

export interface MatchDetailConfig {
  sim_threshold: number
  hit_rate_sim_blend: number
  min_effective_sample: number
  direct_hit_rate: number
  direct_max_sim: number
  direct_max_sim_min_hits: number
  direct_max_sim_min_hit_rate: number
  direct_hit_min_overlap: number
  direction_sim_threshold: number
  direct_hit_score_factor?: number
  weight_sim: number
  weight_density: number
  weighted_threshold: number
}

export interface DirectHitAuxiliary {
  tag_auxiliary_id: number
  tag_label: string
  board_auxiliary_id: number
  board_label: string
}

export interface MatchDetailPair {
  tag_auxiliary_id: number
  tag_auxiliary_label: string
  board_auxiliary_id: number
  board_auxiliary_label: string
  similarity: number
  is_hit: boolean
}

export interface CompositeHitComponent {
  id: number
  label: string
  position: number
}

/** composite_hit 命中的组合标签（label + 有序组件序列）。 */
export interface CompositeHit {
  id: number
  label: string
  components: CompositeHitComponent[]
}

export interface MatchDetailResponse {
  topic_tag_id: number
  topic_tag_label: string
  semantic_board_id: number
  match_reason: string
  score: number
  direction_mismatch?: boolean
  direction_sim: number | null
  effective_min_hits: number
  config: MatchDetailConfig
  direct_hit_auxiliaries: DirectHitAuxiliary[]
  composite_hits?: CompositeHit[]
  tag_auxiliary_count: number
  hits: number
  hit_rate: number
  max_similarity: number
  pairs: MatchDetailPair[]
}

export interface BoardArticle {
  id: number
  title: string
  url: string
  /** 原文链接（后端 Article.ToDict 的 `link`，归档「打开原文」用）。 */
  link?: string
  pub_date: string
  feed_id: number
  feed_name: string
  filtered_tags: BoardArticleTag[]
  /** 归档口径（D5）：是否出现在查询窗口内任一分区线索的 related_article_ids。 */
  in_report?: boolean
  /** 弱证据：无强匹配（无标签 / max_sim），后端统一判定。 */
  weak_evidence?: boolean
  [key: string]: unknown
}


export function useSemanticBoardsApi() {
  async function getBoards(params?: { search?: string; status?: string }): Promise<ApiResponse<{ items: SemanticBoard[]; total: number }>> {
    const query = apiClient.buildQueryParams(params)
    return apiClient.get(`/semantic-boards${query ? `?${query}` : ''}`)
  }

  async function createBoard(data: {
    label: string
    description?: string
    display_order?: number
    protected?: boolean
    auxiliary_labels?: number[]
  }): Promise<ApiResponse<{ id: number }>> {
    return apiClient.post('/semantic-boards', data)
  }

  async function updateBoard(id: number, data: {
    label?: string
    description?: string
    display_order?: number
    protected?: boolean
    status?: string
    enrichment_enabled?: boolean
    relation_auto_discovery_enabled?: boolean
    window_days?: number
    context_layers?: string[]
  }): Promise<ApiResponse<{ id: number }>> {
    return apiClient.put(`/semantic-boards/${id}`, data)
  }

  async function deleteBoard(id: number): Promise<ApiResponse<{ id: number }>> {
    return apiClient.delete(`/semantic-boards/${id}`)
  }

  async function getComposition(id: number): Promise<ApiResponse<BoardCompositionResponse>> {
    return apiClient.get(`/semantic-boards/${id}/composition`)
  }

  async function removeFromComposition(boardId: number, auxiliaryLabelId: number): Promise<ApiResponse<{ board_id: number; auxiliary_label_id: number }>> {
    return apiClient.delete(`/semantic-boards/${boardId}/composition/${auxiliaryLabelId}`)
  }

  /** 四格建议生成（内存响应，调试用；面板主链路走 generateUpgradeSuggestions）。 */
  async function suggestUpgrade(params: UpgradeGenerateParams): Promise<ApiResponse<{ suggestions: UpgradeSuggestionRow[] }>> {
    const query = apiClient.buildQueryParams(params as unknown as Record<string, string | number | undefined>)
    return apiClient.post(`/semantic-boards/upgrade-suggest${query ? `?${query}` : ''}`)
  }

  async function executeUpgrade(data: {
    decision: 'create_new' | 'merge_into_existing' | 'compose'
    board_label?: string
    description?: string
    target_board_id?: number
    auxiliary_label_ids: number[]
    /** 携带持久化建议 id：后端在同一事务内置为 confirmed（spec: confirm 联动）。 */
    suggestion_id?: number
  }): Promise<ApiResponse<{ semantic_board_id: number; auxiliary_label_ids: number[]; composite_label_id?: number }>> {
    return apiClient.post('/semantic-boards/upgrade-execute', data)
  }

  /** 读持久化建议表。status 默认 pending；decision 空=默认列表（排除 watch），`watch`=观察池，其它=精确匹配。 */
  async function getUpgradeSuggestions(params?: { status?: string; decision?: string }): Promise<ApiResponse<UpgradeSuggestionsListResponse>> {
    const query = apiClient.buildQueryParams(params)
    return apiClient.get(`/semantic-boards/upgrade-suggestions${query ? `?${query}` : ''}`)
  }

  /** 将 pending 建议置为 dismissed（写冷却记录）。 */
  async function dismissUpgradeSuggestion(id: number, reason?: string): Promise<ApiResponse<{ id: number; status: string }>> {
    return apiClient.post(`/semantic-boards/upgrade-suggestions/${id}/dismiss`, reason ? { reason } : undefined)
  }

  /** 同步执行一轮四格生成入表（持久化主链路），返回新增/跳过/冷却拦截计数。 */
  async function generateUpgradeSuggestions(params: UpgradeGenerateParams): Promise<ApiResponse<GenerateSuggestionsResponse>> {
    const query = apiClient.buildQueryParams(params as unknown as Record<string, string | number | undefined>)
    return apiClient.post(`/semantic-boards/upgrade-suggestions/generate${query ? `?${query}` : ''}`)
  }

  async function triggerBackfill(data: { mode: string; board_id?: number }): Promise<ApiResponse<BackfillTask>> {
    return apiClient.post('/semantic-boards/backfill', data)
  }

  async function getBackfillStatus(id: string): Promise<ApiResponse<BackfillTask>> {
    return apiClient.get(`/semantic-boards/backfill/${id}`)
  }

  async function getMatchingConfig(): Promise<ApiResponse<MatchingConfig>> {
    return apiClient.get('/semantic-boards/matching-config')
  }

  async function updateMatchingConfig(data: Partial<MatchingConfig>): Promise<ApiResponse<MatchingConfig>> {
    return apiClient.put('/semantic-boards/matching-config', data)
  }

  async function suggestAuxiliaries(params: {
    label: string
    description?: string
    search?: string
    exclude_board_id?: number
    page?: number
    page_size?: number
  }): Promise<ApiResponse<SuggestAuxiliariesResponse>> {
    const query = apiClient.buildQueryParams(params)
    return apiClient.get(`/semantic-boards/suggest-auxiliaries${query ? `?${query}` : ''}`)
  }

  /**
   * 板块来源构成（add-source-board-hit-rate，只读）：该板块窗口内供血源列表。
   * 后端默认按篇数降序；板块不存在/非 board 类型 → 404。
   */
  async function getBoardSourceBreakdown(id: number, windowDays: number): Promise<ApiResponse<BoardSourceBreakdown>> {
    return apiClient.get<BoardSourceBreakdown>(`/semantic-boards/${id}/source-breakdown?window=${windowDays}`)
  }

  /** 版块体检数据（add-board-health-checkup）：只读 latest+趋势；trend 白名单 1-90 默认 7。 */
  async function getBoardHealthCheckup(id: number, trend = 7): Promise<ApiResponse<BoardHealthCheckupResponse>> {
    return apiClient.get<BoardHealthCheckupResponse>(`/semantic-boards/${id}/health-checkup?trend=${trend}`)
  }

  async function getBoardArticles(id: number, params?: Record<string, unknown>): Promise<ApiResponse<BoardArticle[]>> {
    const query = params ? apiClient.buildQueryParams(params) : ''
    return apiClient.get(`/semantic-boards/${id}/articles${query ? `?${query}` : ''}`)
  }

  async function getMatchDetail(boardId: number, tagId: number): Promise<ApiResponse<MatchDetailResponse>> {
    return apiClient.get(`/semantic-boards/${boardId}/match-detail/${tagId}`)
  }

  /**
   * 同版块「相关事件」（按需展开才请求；服务端只读，不调模型不写库）。
   * 阈值取 ai_settings.related_event_similarity_threshold，返回实际生效值。
   */
  async function getRelatedEvents(boardId: number, tagId: number): Promise<ApiResponse<RelatedEventsResponse>> {
    return apiClient.get(`/semantic-boards/${boardId}/tags/${tagId}/related-events`)
  }

  /**
   * 标签的辅助标签全集（GET /api/tags/:id/auxiliary-labels）。
   * 层级视图「事件辅助标签 × 版块收编」逐对核对用；字段对齐后端 semanticBoardAuxiliaryDTO。
   */
  async function getTagAuxiliaryLabels(tagId: number): Promise<ApiResponse<{ items: AuxiliaryLabelItem[]; total: number }>> {
    return apiClient.get(`/tags/${tagId}/auxiliary-labels`)
  }

async function suggestAuxiliariesForBoard(boardId: number, params?: {
    search?: string
    page?: number
    page_size?: number
  }): Promise<ApiResponse<SuggestAuxiliariesResponse>> {
    const query = apiClient.buildQueryParams(params)
    return apiClient.get(`/semantic-boards/${boardId}/suggest-auxiliaries${query ? `?${query}` : ''}`)
  }

  async function addComposition(boardId: number, auxiliaryLabelId: number): Promise<ApiResponse<{ board_id: number; auxiliary_label_id: number }>> {
    return apiClient.post(`/semantic-boards/${boardId}/composition`, { auxiliary_label_id: auxiliaryLabelId })
  }

  return {
    getBoards,
    createBoard,
    updateBoard,
    deleteBoard,
    getComposition,
    getBoardSourceBreakdown,
    getBoardHealthCheckup,
    removeFromComposition,
    addComposition,
    suggestUpgrade,
    executeUpgrade,
    getUpgradeSuggestions,
    dismissUpgradeSuggestion,
    generateUpgradeSuggestions,
    suggestAuxiliaries,
    suggestAuxiliariesForBoard,
    getBoardArticles,
    getMatchDetail,
    getRelatedEvents,
    getTagAuxiliaryLabels,
    triggerBackfill,
    getBackfillStatus,
    getMatchingConfig,
    updateMatchingConfig,
  }
}

// ── 今日雷达（rework-workshop-into-radar-and-desk 8.2 + 8A/R-18）─────────────
// 后端 snake_case JSON 协议不变；仅 Radar 客户端分支在边界归一为 camelCase/
// 字符串 ID 读模型（raw 契约与转换唯一实现在 api/normalizers/boardRadar.ts，
// 组件层不散落映射，也不扩展到 semanticBoards 其它 API）。

export type { RadarBoard, RadarLane, RadarOverview, RadarTopLaneItem, RadarTopLanes, BoardRadarRawOverview, BoardRadarRawTopLanes, BoardRadarRawTopLaneItem, RadarOverviewResponse } from '~/api/normalizers/boardRadar'

import { mapRadarOverviewResponse, type BoardRadarRawOverview, type RadarOverviewResponse } from '~/api/normalizers/boardRadar'

export function useBoardRadarApi() {
  async function getOverview(): Promise<RadarOverviewResponse> {
    const res = await apiClient.get<BoardRadarRawOverview>('/semantic-boards/overview')
    return mapRadarOverviewResponse(res)
  }
  return { getOverview }
}
