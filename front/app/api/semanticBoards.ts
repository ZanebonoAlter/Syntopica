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

export interface BoardCompositionResponse {
  items: AuxiliaryLabelItem[]
  total: number
}

export interface UpgradeCandidate {
  id: number
  label: string
  slug: string
  ref_count: number
}

export interface BoardAffinity {
  board_id: number
  board_label: string
  matching_candidates: number
  avg_distance: number
}

export interface UpgradeCluster {
  candidates: UpgradeCandidate[]
  board_affinities: BoardAffinity[]
}

export interface UpgradeConfig {
  semantic_board_upgrade_ref_count_threshold: number
  semantic_board_upgrade_cluster_distance_threshold: number
  semantic_board_upgrade_cotag_window_days: number
  semantic_board_upgrade_cotag_top_n: number
  semantic_board_upgrade_cotag_dedupe_sim_threshold: number
  semantic_board_upgrade_cotag_hard_limit: number
}

export interface UpgradeCandidatesResponse {
  candidates: UpgradeCandidate[]
  clusters: UpgradeCluster[]
  config: UpgradeConfig
}

export interface UpgradeSuggestion {
  decision: 'create_new' | 'merge_into_existing' | 'skip'
  board_label?: string
  description?: string
  target_board_id?: number
  auxiliary_label_ids: number[]
  auxiliary_labels: { id: number; label: string }[]
  target_board_label?: string
  reason: string
  board_affinities: BoardAffinity[]
}

export interface UpgradeSuggestResponse {
  suggestions: UpgradeSuggestion[]
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

export interface MatchDetailResponse {
  topic_tag_id: number
  topic_tag_label: string
  semantic_board_id: number
  match_reason: string
  score: number
  downgraded: boolean
  direction_sim: number | null
  effective_min_hits: number
  config: MatchDetailConfig
  direct_hit_auxiliaries: DirectHitAuxiliary[]
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

export interface BoardNarrativeTag {
  id: number
  label: string
}

export interface BoardNarrative {
  id: number
  title: string
  summary: string
  status: string
  related_tags: BoardNarrativeTag[]
  related_article_ids: number[]
  scope_type: string
  article_count: number
  period_date: string
}

export function useSemanticBoardsApi() {
  async function getBoards(params?: { search?: string; status?: string }): Promise<ApiResponse<{ items: SemanticBoard[]; total: number }>> {
    const query = apiClient.buildQueryParams(params)
    return apiClient.get(`/semantic-boards${query ? `?${query}` : ''}`)
  }

  async function getBoard(id: number): Promise<ApiResponse<SemanticBoard>> {
    return apiClient.get(`/semantic-boards/${id}`)
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

  async function getUpgradeCandidates(): Promise<ApiResponse<UpgradeCandidatesResponse>> {
    return apiClient.get('/semantic-boards/upgrade-candidates')
  }

  async function suggestUpgrade(): Promise<ApiResponse<UpgradeSuggestResponse>> {
    return apiClient.post('/semantic-boards/upgrade-suggest')
  }

  async function executeUpgrade(data: {
    decision: 'create_new' | 'merge_into_existing'
    board_label?: string
    description?: string
    target_board_id?: number
    auxiliary_label_ids: number[]
  }): Promise<ApiResponse<{ semantic_board_id: number; auxiliary_label_ids: number[] }>> {
    return apiClient.post('/semantic-boards/upgrade-execute', data)
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

  async function getBoardNarratives(id: number, params?: { days?: number }): Promise<ApiResponse<BoardNarrative[]>> {
    const query = params ? apiClient.buildQueryParams(params) : ''
    return apiClient.get(`/semantic-boards/${id}/narratives${query ? `?${query}` : ''}`)
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

  async function triggerNarrativeGeneration(params: { date: string; board_id?: number }) {
    return apiClient.post<{ success: boolean; data: { saved: number } }>('/narratives/boards/generate', params)
  }

  return {
    getBoards,
    getBoard,
    createBoard,
    updateBoard,
    deleteBoard,
    getComposition,
    removeFromComposition,
    addComposition,
    getUpgradeCandidates,
    suggestUpgrade,
    executeUpgrade,
    suggestAuxiliaries,
    suggestAuxiliariesForBoard,
    getBoardArticles,
    getMatchDetail,
    getBoardNarratives,
    triggerBackfill,
    getBackfillStatus,
    getMatchingConfig,
    updateMatchingConfig,
    triggerNarrativeGeneration,
  }
}
