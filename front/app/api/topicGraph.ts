import { apiClient } from './client'
import type { TimelineFilters } from '~/types/timeline'
import type { PendingArticlesResponse } from '~/types/timeline'

export type TopicGraphType = 'daily' | 'weekly'
export type TopicCategory = 'event' | 'person' | 'keyword'
export type AnalysisType = 'event' | 'person' | 'keyword'
export type TopicAnalysisType = AnalysisType
export type TopicKind = 'topic' | 'entity' | 'keyword'

export interface AnalysisStatusResponse {
  status: 'pending' | 'processing' | 'completed' | 'failed'
  progress: number
  error: string | null
  result: Record<string, unknown> | null
}

export interface RebuildAnalysisRequest {
  windowType: string
  anchorDate: string
}

export interface TopicTag {
  id?: number
  label: string
  slug: string
  category: TopicCategory
  kind?: TopicKind
  icon?: string
  aliases?: string[]
  description?: string
  score: number
  quality_score?: number
  is_low_quality?: boolean
  is_abstract?: boolean
  child_slugs?: string[]
}

export interface AggregatedTopicTag {
  slug: string
  label: string
  category: TopicCategory
  kind?: TopicKind
  icon?: string
  score: number
  article_count: number
}

export interface GraphNode {
  id: string
  label: string
  slug?: string
  kind: 'topic' | 'feed'
  category?: TopicCategory
  icon?: string
  weight: number
  article_count?: number
  color?: string
  feed_name?: string
  category_name?: string
  is_abstract?: boolean
}

export type TopicGraphNode = GraphNode

export interface TopicGraphEdge {
  id: string
  source: string
  target: string
  kind: 'topic_topic' | 'topic_feed'
  weight: number
}

export interface TopicGraphPayload {
  type: TopicGraphType
  anchor_date: string
  period_label: string
  nodes: GraphNode[]
  edges: TopicGraphEdge[]
  topic_count: number
  article_count: number
  feed_count: number
  top_topics: TopicTag[]
}

export interface TopicsByCategoryPayload {
  events: TopicTag[]
  people: TopicTag[]
  keywords: TopicTag[]
}

export interface HotspotDigestCard {
  id: number
  title: string
  summary: string
  feed_name: string
  feed_icon: string
  feed_color: string
  category_name: string
  article_count: number
  created_at: string
  aggregated_tags: AggregatedTopicTag[]
  matched_articles?: Array<{
    id: number
    title: string
    feed_name?: string
    feed_icon?: string
    feed_color?: string
  }>
  matched_articles_tags?: AggregatedTopicTag[]
}

export interface HotspotDigestsResponse {
  digests: HotspotDigestCard[]
  total: number
}

export interface TopicGraphSummaryCard {
  id: number
  title: string
  summary: string
  feed_name: string
  feed_icon: string
  feed_color: string
  category_name: string
  article_count: number
  created_at: string
  topics: TopicTag[]
  aggregated_tags: AggregatedTopicTag[]
  articles: Array<{
    id: number
    title: string
    link: string
  }>
}

export interface TopicHistoryPoint {
  anchor_date: string
  count: number
  label: string
}

export interface RelatedTag {
  id: number
  label: string
  slug: string
  category: TopicCategory
  kind?: TopicKind
  cooccurrence: number
}

export interface TopicGraphDetailPayload {
  topic: TopicTag
  articles: Array<{
    id: number
    title: string
    link: string
  }>
  total_articles: number
  related_tags: RelatedTag[]
  summaries: TopicGraphSummaryCard[]
  history: TopicHistoryPoint[]
  related_topics: TopicTag[]
  search_links: Record<string, string>
  app_links: Record<string, string>
}

export interface GetTopicAnalysisParams {
  tagID: number
  analysisType: TopicAnalysisType
  windowType: TopicGraphType
  anchorDate: string
}

export interface TopicAnalysisRecord {
  id: number
  topic_tag_id: number
  analysis_type: TopicAnalysisType
  window_type: TopicGraphType
  anchor_date: string
  summary_count: number
  payload_json: string
  source: string
  version: number
  created_at: string
  updated_at: string
  // 支持后端PascalCase格式
  ID?: number
  TopicTagID?: number
  AnalysisType?: TopicAnalysisType
  WindowType?: TopicGraphType
  AnchorDate?: string
  SummaryCount?: number
  PayloadJSON?: string
  Source?: string
  Version?: number
  CreatedAt?: string
  UpdatedAt?: string
}

export interface RebuildAnalysisParams extends RebuildAnalysisRequest {
  tagID: number
  analysisType: TopicAnalysisType
}

export interface TopicAnalysisStatusRecord {
  status: AnalysisStatusResponse['status'] | 'missing' | 'ready'
  progress: number
  error: string | null
  result: Record<string, unknown> | null
}

export interface GetTopicArticlesParams {
  slug: string
  page?: number
  pageSize?: number
  dateRange?: TimelineFilters['dateRange']
  startDate?: string
  endDate?: string
  sources?: string[]
}

export interface TopicArticlesResponse {
  articles: Array<{
    id: string
    title: string
    link: string
    summary: string
    content?: string
    pub_date: string
    feed_name: string
    feed_id: string
    tags: Array<{
      slug: string
      label: string
      category: TopicCategory
    }>
    image_url?: string
  }>
  total: number
  page: number
  pageSize: number
}

function withQuery(endpoint: string, params: Record<string, string | undefined>) {
  const query = apiClient.buildQueryParams(params)
  return query ? `${endpoint}?${query}` : endpoint
}

export interface TopicGraphFilters {
  categoryId?: string
  feedId?: string
}

export function useTopicGraphApi() {
  return {
    async getGraph(type: TopicGraphType, date?: string, filters?: TopicGraphFilters) {
      return apiClient.get<TopicGraphPayload>(withQuery(`/topic-graph/${type}`, {
        date,
        category_id: filters?.categoryId,
        feed_id: filters?.feedId,
      }))
    },

    async getTopicDetail(slug: string, type?: TopicGraphType, date?: string, filters?: TopicGraphFilters) {
      return apiClient.get<TopicGraphDetailPayload>(withQuery(`/topic-graph/topic/${slug}`, {
        type,
        date,
        category_id: filters?.categoryId,
        feed_id: filters?.feedId,
      }))
    },

    async getTopicAnalysis(params: GetTopicAnalysisParams) {
      return apiClient.get<TopicAnalysisRecord>(withQuery('/topic-graph/analysis', {
        tag_id: String(params.tagID),
        analysis_type: params.analysisType,
        window_type: params.windowType,
        anchor_date: params.anchorDate,
      }))
    },

    async rebuildTopicAnalysis(params: RebuildAnalysisParams) {
      return apiClient.post<TopicAnalysisRecord>(withQuery('/topic-graph/analysis/rebuild', {
        tag_id: String(params.tagID),
        analysis_type: params.analysisType,
        window_type: params.windowType,
        anchor_date: params.anchorDate,
      }), {})
    },

    async getAnalysisStatus(params: GetTopicAnalysisParams) {
      return apiClient.get<TopicAnalysisStatusRecord>(withQuery('/topic-graph/analysis/status', {
        tag_id: String(params.tagID),
        analysis_type: params.analysisType,
        window_type: params.windowType,
        anchor_date: params.anchorDate,
      }))
    },

    async retryTopicAnalysis(params: RebuildAnalysisParams) {
      return apiClient.post<TopicAnalysisRecord>(withQuery('/topic-graph/analysis/retry', {
        tag_id: String(params.tagID),
        analysis_type: params.analysisType,
        window_type: params.windowType,
        anchor_date: params.anchorDate,
      }), {})
    },

    async getTopicsByCategory(type: TopicGraphType, date?: string, filters?: TopicGraphFilters) {
      return apiClient.get<TopicsByCategoryPayload>(withQuery('/topic-graph/by-category', {
        type,
        date,
        category_id: filters?.categoryId,
        feed_id: filters?.feedId,
      }))
    },

    async getDigestsByArticleTag(slug: string, type?: TopicGraphType, date?: string, limit?: number, kind?: TopicCategory) {
      return apiClient.get<HotspotDigestsResponse>(withQuery(`/topic-graph/tag/${slug}/digests`, {
        type,
        date,
        limit: limit ? String(limit) : undefined,
        kind,
      }))
    },

    async getTopicArticles(params: GetTopicArticlesParams) {
      const queryParams: Record<string, string | undefined> = {
        page: params.page ? String(params.page) : undefined,
        page_size: params.pageSize ? String(params.pageSize) : undefined,
        date_range: params.dateRange || undefined,
        start_date: params.startDate,
        end_date: params.endDate,
      }

      if (params.sources && params.sources.length > 0) {
        queryParams.sources = params.sources.join(',')
      }

      return apiClient.get<TopicArticlesResponse>(
        withQuery(`/topic-graph/topic/${params.slug}/articles`, queryParams)
      )
    },

    async getPendingArticlesByTag(slug: string, type?: TopicGraphType, date?: string) {
      return apiClient.get<PendingArticlesResponse>(withQuery(`/topic-graph/tag/${slug}/pending-articles`, {
        type,
        date,
      }))
    },

    async searchTags(query: string, category?: string, limit?: number) {
      return apiClient.get<{ id: number; label: string; slug: string; category: string; feed_count: number }[]>(withQuery('/topic-tags/search', {
        q: query,
        category,
        limit: limit ? String(limit) : undefined,
      }))
    },

    async mergeTags(sourceTagId: number, targetTagId: number) {
      return apiClient.post<{ source_id: number; target_id: number; target_label: string }>('/topic-tags/merge', {
        source_tag_id: sourceTagId,
        target_tag_id: targetTagId,
      })
    },
  }
}

export interface NarrativeItem {
  id: number
  title: string
  summary: string
  status: 'emerging' | 'continuing' | 'splitting' | 'merging' | 'ending'
  period: string
  period_date: string
  generation: number
  parent_ids: number[]
  related_tags: { id: number; slug: string; label: string; category: TopicCategory; kind?: TopicKind }[]
  child_ids: number[]
}

export interface NarrativeTimelineDay {
  date: string
  narratives: NarrativeItem[]
}

export interface NarrativeScopeCategory {
  category_id: number
  category_name: string
  category_icon: string
  category_color: string
  board_count: number
  last_generated_at: string
}

export interface NarrativeScopesResponse {
  date: string
  global_count: number
  categories: NarrativeScopeCategory[]
}

export interface BoardNarrativeItem extends NarrativeItem {
  source: 'llm' | 'abstract'
  board_id: number
}

export interface BoardItem {
  id: number
  name: string
  description: string
  scope_type: string
  scope_category_id: number | null
  narrative_count: number
  aggregate_status: 'emerging' | 'continuing' | 'splitting' | 'merging' | 'ending'
  narratives: BoardNarrativeItem[]
  prev_board_ids: number[]
  abstract_tag_id: number | null
  abstract_tag_slug: string
  board_concept_id: number | null
  concept_name: string
  is_system: boolean
  created_at: string
  event_tags: TagBrief[]
  abstract_tags: TagBrief[]
}

export interface TagBrief {
  id: number
  slug: string
  label: string
  category: string
  kind?: string
}

export interface BoardTimelineDay {
  date: string
  boards: BoardItem[]
}

export function useNarrativeApi() {
  const getNarratives = async (date: string, scopeType?: string, categoryId?: number) => {
    const params: Record<string, string | undefined> = { date }
    if (scopeType) params.scope_type = scopeType
    if (categoryId !== undefined) params.category_id = String(categoryId)
    return apiClient.get<NarrativeItem[]>(
      withQuery('/narratives', params)
    )
  }

  const getNarrativeTimeline = async (date: string, days = 7, scopeType?: string, categoryId?: number) => {
    const params: Record<string, string | undefined> = { date, days: String(days) }
    if (scopeType) params.scope_type = scopeType
    if (categoryId !== undefined) params.category_id = String(categoryId)
    return apiClient.get<NarrativeTimelineDay[]>(
      withQuery('/narratives/timeline', params)
    )
  }

  const getNarrativeHistory = async (id: number) => {
    return apiClient.get<NarrativeItem[]>(
      `/narratives/${id}/history`
    )
  }

  const deleteNarratives = async (date: string, scopeType?: string, categoryId?: number) => {
    const params: Record<string, string | undefined> = { date }
    if (scopeType) params.scope_type = scopeType
    if (categoryId !== undefined) params.category_id = String(categoryId)
    return apiClient.delete<{ deleted: number }>(
      withQuery('/narratives', params)
    )
  }

  const getNarrativeScopes = async (date: string, days = 7) => {
    return apiClient.get<NarrativeScopesResponse>(
      `/narratives/scopes?date=${date}&days=${days}`
    )
  }

  const regenerateNarratives = async (date: string, scopeType?: string, categoryId?: number) => {
    return apiClient.post<{ saved: number }>(
      '/narratives/regenerate',
      {
        date,
        scope_type: scopeType || undefined,
        category_id: categoryId !== undefined ? categoryId : undefined,
      }
    )
  }

  const getBoardTimeline = async (date: string, days = 7, scopeType?: string, categoryId?: number) => {
    const params: Record<string, string | undefined> = { date, days: String(days) }
    if (scopeType) params.scope_type = scopeType
    if (categoryId !== undefined) params.category_id = String(categoryId)
    return apiClient.get<BoardTimelineDay[]>(
      withQuery('/narratives/boards/timeline', params)
    )
  }

  const getBoardDetail = async (id: number) => {
    return apiClient.get<BoardItem>(
      `/narratives/boards/${id}`
    )
  }

  return { getNarratives, getNarrativeTimeline, getNarrativeHistory, deleteNarratives, getNarrativeScopes, regenerateNarratives, getBoardTimeline, getBoardDetail }
}
