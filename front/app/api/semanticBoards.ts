import { apiClient } from './client'
import type { ApiResponse } from '~/types'

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
  match_reason: string
  score: number
  downgraded: boolean
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
  downgraded: boolean
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
  pub_date: string
  feed_id: number
  feed_name: string
  filtered_tags: BoardArticleTag[]
  [key: string]: unknown
}

// ── 话题态势版图（identity 轨，design §3 契约） ────────────────────────────
/** 持久话题主态势标签（后端派生，前端不重复算）。 */
export type TopicStance = 'emerging' | 'pending' | 'active' | 'stalled' | 'archived'

/** mini-lifeline 单日格点（后端 generate_series 补空日，保证日期轴连续）。 */
export interface LifelinePoint {
  date: string
  section_count: number
}

/** 话题态势版图单行（对齐 GET /topic-landscape 响应 topics[]）。 */
export interface TopicLandscapeTopic {
  id: number
  label: string
  status: string
  source: string
  stance: TopicStance
  is_vacuum: boolean
  vacuum_strong: number
  hit_count: number
  consecutive_hits: number
  first_seen_date: string
  last_seen_date: string
  days_since_last: number
  can_activate: boolean
  lifeline: LifelinePoint[]
}

/** 活力顶栏指标（design §3 vitality）。feed_active MVP 可空。 */
export interface Vitality {
  days: number
  article_count: number
  section_count: number
  active_topic_count: number
  feed_active: number | null
  trend: number[]
}

/** GET /topic-landscape 响应 data。 */
export interface TopicLandscapeResponse {
  topics: TopicLandscapeTopic[]
  vitality: Vitality
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

  /** 话题态势版图（identity 轨，只读）：GET /semantic-boards/:id/topic-landscape?days=N。 */
  async function getTopicLandscape(boardId: number, days?: number): Promise<ApiResponse<TopicLandscapeResponse>> {
    const query = days ? apiClient.buildQueryParams({ days }) : ''
    return apiClient.get(`/semantic-boards/${boardId}/topic-landscape${query ? `?${query}` : ''}`)
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

  async function getBoardArticles(id: number, params?: Record<string, unknown>): Promise<ApiResponse<BoardArticle[]>> {
    const query = params ? apiClient.buildQueryParams(params) : ''
    return apiClient.get(`/semantic-boards/${id}/articles${query ? `?${query}` : ''}`)
  }

  async function getMatchDetail(boardId: number, tagId: number): Promise<ApiResponse<MatchDetailResponse>> {
    return apiClient.get(`/semantic-boards/${boardId}/match-detail/${tagId}`)
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
    getTopicLandscape,
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
    triggerBackfill,
    getBackfillStatus,
    getMatchingConfig,
    updateMatchingConfig,
  }
}
