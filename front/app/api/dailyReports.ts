import { apiClient } from './client'
import type { ApiResponse } from '~/types'

/**
 * 日报重点条目（retire-story-adopt-event-metrics 2.3/3.3：候选源换成当天同事件聚簇）。
 *
 * `cluster_id` 是聚簇代表 event 标签 id，也是本期正文的定位键（DOM 顺序第一条
 * `tag_ids` 含该 id 的 thread）；`article_count` / `cluster_mode` / `data_as_of` /
 * `research_at` 都是服务端快照（模型不输出数字）。标签回退/历史数据缺省这些字段——
 * 缺失就省略对应文案，不伪造 0；事件线字段（story_id、participant_count、story_heat、
 * heat_rule_version、heat_as_of、heat_incomplete、is_progress）已整体退役。
 */
export interface DailyReportHighlight {
  title: string
  reason: string
  tag_ids: number[]
  /** 聚簇代表 event 标签 id；缺省 = 旧标签回退重点（无定位/无聚簇快照）。 */
  cluster_id?: number | null
  /** 簇内文章数（内容身份去重后的篇数）；缺省省略（真实 0 是合法值）。 */
  article_count?: number | null
  /** 聚簇口径（中文：向量聚簇 / 语义向量兜底 / 主体近似）；缺省省略。 */
  cluster_mode?: string
  /** 数据截至日（YYYY-MM-DD）；缺省省略。 */
  data_as_of?: string
  /** 研究时点（RFC3339）；缺省省略。 */
  research_at?: string | null
}

export interface DailyReportThread {
  id: number
  report_id: number
  section_id: number
  title: string
  summary: string
  tag_ids: number[]
  confidence: number
  /** Thread 标题 ↔ 所属 section 内容 embedding（tag 事实文本，非 cluster_label 标题）的余弦贴合距离（observability System 3）。
   *  nil = 无信号；有值时为 0.0~2.0 的距离，越小越贴合（0.0 = 完美贴合）。
   *  embedding 字段绝不在此声明（后端 json:"-" 不外泄到前端）。 */
  fit_distance?: number
  related_article_ids: number[]
  created_at: string
}

export interface PersistentTopicBrief {
  id: number
  label: string
  status: string  // 'candidate' | 'active' | 'archived'
  /** Stable colour derived from the topic id by the backend; same topic → same colour across renders. */
  color: string
  hit_count?: number
  consecutive_hits: number
  can_activate: boolean
}

/** Full persistent topic row, as returned by the management API (merge/split/update). */
export interface PersistentTopic {
  id: number
  semantic_board_id: number
  label: string
  description: string
  status: string  // 'candidate' | 'active' | 'archived'
  first_seen_date: string
  last_seen_date: string
  hit_count: number
  consecutive_hits: number
}

/** Topic list item from GET /semantic-boards/:id/topics — adds a section count
 * and the stable colour, for the management UI. */
export interface BoardTopicListItem extends PersistentTopic {
  section_count: number
  color: string
  can_activate: boolean
}

export interface SectionTimelineNode {
  id: number
  report_id: number
  period_date: string
  cluster_label: string
  status: string  // emerging / continuing / split / merge / ending (dynamically derived)
  article_count: number
  thread_count: number
  image_url?: string
  imageUrl?: string
  /** Per-tag match-quality snapshot for timeline/lifecycle/lifeline sections.
   *  Mirrors `DailyReportSection.quality_breakdown`; null for historical
   *  sections (column added by the quality-scoring-observability change). */
  quality_breakdown?: DailyReportQualityEntry[] | null
  // Persistent topic assignment (optional — historical/unmatched sections may lack it).
  persistent_topic_id?: number
  topic_match_distance?: number
  topic_match_confidence?: string  // 'anchor_hit' | 'auto_new' | 'unmatched'
  persistent_topic?: PersistentTopicBrief
}

export interface SectionRelation {
  from_id: number
  to_id: number
  distance: number
  /** 'identity' = same persistent topic (solid line); 'similarity' = Hungarian match (dashed line). */
  relation_type?: string
}

// ── 话题关键事件注记（add-topic-key-event-markers）──────────────────────────

/** Marker state: suggested = AI 建议（未处理）；confirmed = 人工已确认；
 *  dismissed = 不采纳/已取消（仅表示不再选择，原新闻保留）。 */
export type KeyEventState = 'suggested' | 'confirmed' | 'dismissed'

/** 一次人工选择/建议的独立注记行。快照字段全部来自服务端来源条目；
 *  source_thread_id 为空表示来源已被重建替换（快照仍可回看）。 */
export interface TopicKeyEventMarker {
  id: number
  semantic_board_id: number
  persistent_topic_id?: number | null
  source_report_id?: number | null
  source_section_id?: number | null
  source_thread_id?: number | null
  /** 来源日报日期（非事件发生时间）。 */
  source_period_date: string
  topic_label_snapshot: string
  title_snapshot: string
  summary_snapshot: string
  tag_ids_snapshot: number[] | null
  article_ids_snapshot: number[] | null
  state: KeyEventState
  /** AI 建议理由（仅 suggested/被确认前；人工标记为空）。 */
  recommendation_reason?: string
  /** {"evidence_article_ids":[...],"new_progress":bool}。 */
  recommendation_basis?: { evidence_article_ids?: number[], new_progress?: boolean } | null
  last_action?: string
  confirmed_at?: string | null
  decided_at?: string | null
  revision: number
}

/** GET /daily-reports/:id/key-events 响应：当期注记 + 推荐任务状态。 */
export interface ReportKeyEventsData {
  status: 'idle' | 'running' | 'done' | 'failed'
  markers: TopicKeyEventMarker[]
}

// SectionLifecycleNode has the same shape as SectionTimelineNode
export type SectionLifecycleNode = SectionTimelineNode

/** Frozen per-tag match-quality lineage snapshot persisted on
 *  daily_report_sections.quality_breakdown. snake_case to match the rest of
 *  the daily-report API surface (the dailyReports client does not camelize).
 *  Historical sections return null for the whole array. */
export interface DailyReportQualityEntry {
  tag_id: number
  label: string
  match_reason: string
  score: number
  downgraded: boolean
}

export interface ActiveWatchSummary {
  watchId: number | string
  label: string
  type: 'label' | 'keyword'
}

export interface DailyReportWatchHit extends ActiveWatchSummary {
  sectionId: number
  reason?: string
}

export interface DailyReportSection {
  id: number
  cluster_index: number
  cluster_label: string
  cluster_tag_ids: number[]
  threads: DailyReportThread[]
  article_count: number
  best_tier: number
  avg_score: number
  /** Frozen match-quality lineage for this section's source tags, or null for
   *  historical sections generated before this field existed (DP-1 snake_case). */
  quality_breakdown?: DailyReportQualityEntry[] | null
  // Persistent topic assignment (optional; populated by the daily pipeline).
  persistent_topic_id?: number
  topic_match_distance?: number
  topic_match_confidence?: string
  persistent_topic?: PersistentTopicBrief
  /** 报告生成时的话题状态快照（active | candidate | null），用于阅读分区而非
   *  当前 topic.status。旧数据可能为 null；缺失时保守降级到"其他动态"。
   *  后端必须输出小写枚举值（active | candidate）；混合大小写将降级到"其他动态"。
   *  snake_case，与 daily-report API 约定一致。 */
  topic_status_at_report?: string | null
  /** 分桶来源（l1_direct / l2_llm / l3_new / watch_keyword / watch_sentence）。
   *  watch_* = 关注物化板块（watch-materialized-topic）；旧数据可能为 null。 */
  lane_tier?: string | null
  /** 物化板块归属 watch 名（读路径 transient 透出；LLM 当日标题之外的追踪源装饰）。 */
  watch_label?: string | null
  /** 泳道内主题分桶的方向名（add-intra-lane-theme-bucketing）；未分桶/历史 section 为 null。 */
  bucket_title?: string | null
  /** 同 cluster_index 内的桶序号（0 起、组内唯一）；未分桶为 0。 */
  bucket_index?: number
}

/** POST /daily-reports/:id/rebucket 响应：该桶组最新 sections + 桶数（buckets=1 = 未拆动/未达阈值）。 */
export interface RebucketSectionsResult {
  buckets: number
  sections: DailyReportSection[]
}

export interface DailyReport {
  id: number
  semantic_board_id: number
  period_date: string
  title: string
  summary: string
  status: string
  cluster_count: number
  article_count: number
  event_tag_count: number
  highlights: DailyReportHighlight[]
  dynamics: string
  sections: DailyReportSection[]
  created_at: string
  /** Active watch hits returned with the report detail when available. */
  activeWatchHits?: DailyReportWatchHit[]
}

export interface TodayReportBrief {
  report_id: number
  semantic_board_id: number
  period_date: string
  title: string
  status: string
}

export interface DailyReportListItem {
  id: number
  semantic_board_id: number
  period_date: string
  title: string
  summary: string
  status: string
  cluster_count: number
  article_count: number
  event_tag_count: number
  created_at: string
  /** One summary per active watch with at least one hit in this report. */
  activeWatchSummaries?: ActiveWatchSummary[]
}

interface ActiveWatchSummaryPayload {
  watch_id?: number
  watchId?: number
  label: string
  type?: string
  watch_type?: string
  watchType?: string
}

interface DailyReportListItemPayload extends Omit<DailyReportListItem, 'activeWatchSummaries'> {
  active_watch_summaries?: ActiveWatchSummaryPayload[]
  activeWatchSummaries?: ActiveWatchSummaryPayload[]
}

function normalizeWatchSummary(payload: ActiveWatchSummaryPayload): ActiveWatchSummary {
  return {
    watchId: payload.watchId ?? payload.watch_id ?? 0,
    label: payload.label,
    type: (payload.type ?? payload.watchType ?? payload.watch_type) === 'keyword' ? 'keyword' : 'label',
  }
}

function normalizeReportListItem(payload: DailyReportListItemPayload): DailyReportListItem {
  const summaries = payload.activeWatchSummaries ?? payload.active_watch_summaries
  return {
    ...payload,
    activeWatchSummaries: summaries?.map(normalizeWatchSummary),
  }
}

export function useDailyReportsApi() {
  /**
   * 触发日报生成（add-radar-pipeline-status D1）。
   * `board_id`（单板）与 `board_ids`（板块集合）**互斥**：同时给会被后端 400 拒绝
   * （board_id 与 board_ids 不能同时指定）。两者都不给 = 全量。
   */
  async function generateDailyReport(params: { date: string; board_id?: number; board_ids?: number[] }) {
    return apiClient.post<{ job_id: string; status: string }>('/daily-reports/generate', params)
  }

  async function getTodayReports(date?: string): Promise<ApiResponse<{ date: string, reports: TodayReportBrief[] }>> {
    const query = date ? `?date=${encodeURIComponent(date)}` : ''
    return apiClient.get<{ date: string, reports: TodayReportBrief[] }>(`/daily-reports/today${query}`)
  }

  async function getBoardDailyReports(boardId: number, params?: { days?: number }): Promise<ApiResponse<{ reports: DailyReportListItem[] }>> {
    const query = params ? apiClient.buildQueryParams(params) : ''
    const response = await apiClient.get<{ reports: DailyReportListItemPayload[] }>(`/semantic-boards/${boardId}/daily-reports${query ? `?${query}` : ''}`)
    if (!response.success || !response.data) return response as ApiResponse<{ reports: DailyReportListItem[] }>
    return {
      ...response,
      data: { ...response.data, reports: (response.data.reports ?? []).map(normalizeReportListItem) },
    }
  }

  async function getDailyReportDetail(id: number): Promise<ApiResponse<{ report: DailyReport }>> {
    return apiClient.get(`/daily-reports/${id}`)
  }

  async function getBoardSectionTimeline(boardId: number, days?: number): Promise<ApiResponse<{ sections: SectionTimelineNode[], relations: SectionRelation[] }>> {
    // days 传 0 表示"全部历史"（显式 ?days=0，后端 <=0 即不限天）；省略则走后端默认。
    const query = days != null ? `?days=${days}` : ''
    return apiClient.get(`/semantic-boards/${boardId}/section-timeline${query}`)
  }

  async function getSectionLifecycle(sectionId: number): Promise<ApiResponse<{ sections: SectionLifecycleNode[], relations: SectionRelation[] }>> {
    return apiClient.get(`/daily-reports/sections/${sectionId}/lifecycle`)
  }

  /** All sections of one persistent topic (no day limit), aggregated by topic id. Identity-key based — survives label drift.
   *  key_events：可选已确认关键节点（add-topic-key-event-markers），快照独立于真实 sections。 */
  async function getTopicLifeline(topicId: number): Promise<ApiResponse<{ sections: SectionTimelineNode[], relations: SectionRelation[], key_events?: TopicKeyEventMarker[] }>> {
    return apiClient.get(`/daily-reports/topics/${topicId}/lifeline`)
  }

  /** 当期日报的注记与推荐状态（日报读端：可含 suggested 待处理；打开页面只读不触发推荐）。 */
  async function getReportKeyEvents(reportId: number): Promise<ApiResponse<ReportKeyEventsData>> {
    return apiClient.get(`/daily-reports/${reportId}/key-events`)
  }

  /** 话题已确认关键节点（topic 读端：默认且仅 confirmed；days=0 全部，默认 14）。 */
  async function getTopicKeyEvents(topicId: number, days?: number): Promise<ApiResponse<{ topic_id: number, days: number, markers: TopicKeyEventMarker[] }>> {
    const query = days != null ? `?days=${days}` : ''
    return apiClient.get(`/daily-reports/topics/${topicId}/key-events${query}`)
  }

  /** 按来源线索动作（PUT）：首建可不带 expected_revision；同目标幂等，旧 revision 409 附最新状态。 */
  async function putThreadKeyEvent(topicId: number, threadId: number, action: 'mark' | 'dismiss', expectedRevision?: number): Promise<ApiResponse<TopicKeyEventMarker>> {
    return apiClient.put(`/daily-reports/topics/${topicId}/key-events/threads/${threadId}`, {
      action,
      ...(expectedRevision != null ? { expected_revision: expectedRevision } : {}),
    })
  }

  /** 按注记维护（PATCH）：unmark 可取消来源失效的 confirmed；重新 mark 需来源仍有效。 */
  async function patchKeyEventMarker(topicId: number, markerId: number, action: 'mark' | 'dismiss' | 'unmark', revision: number): Promise<ApiResponse<TopicKeyEventMarker>> {
    return apiClient.patch(`/daily-reports/topics/${topicId}/key-events/${markerId}`, {
      action,
      revision,
    })
  }

  /** 对某个 section 所在的桶组重跑泳道内主题分桶（add-intra-lane-theme-bucketing）。
   *  section id 变化时服务端复用组头行；未达阈值/拆不动返回 buckets=1 且不改数据。 */
  async function rebucketSection(reportId: number, sectionId: number): Promise<ApiResponse<RebucketSectionsResult>> {
    return apiClient.post(`/daily-reports/${reportId}/rebucket`, { section_id: sectionId })
  }

  /** Reconstruct persistent topics from historical sections lacking a topic. Optional boardId scopes to one board. */
  async function backfillPersistentTopics(boardId?: number): Promise<ApiResponse<{ status: string }>> {
    const query = boardId ? `?board_id=${boardId}` : ''
    return apiClient.post(`/daily-reports/backfill-topics${query}`, {})
  }

  /** Rename and/or change status (active|archived) of a topic. Omit a field to leave it unchanged. */
  async function updateTopic(topicId: number, params: { label?: string; status?: 'active' | 'archived' }): Promise<ApiResponse<PersistentTopic>> {
    return apiClient.patch(`/daily-reports/topics/${topicId}`, params)
  }

  /** Hard-delete a topic. Sections keep their content; only the topic assignment is cleared. Irreversible. */
  async function deleteTopic(topicId: number): Promise<ApiResponse<null>> {
    return apiClient.delete(`/daily-reports/topics/${topicId}`)
  }

  /** List EVERY persistent topic on a board (active + candidate + archived + orphans) with section counts. */
  async function listBoardTopics(boardId: number): Promise<ApiResponse<{ topics: BoardTopicListItem[] }>> {
    return apiClient.get(`/semantic-boards/${boardId}/topics`)
  }

  /** Merge source topics into the target; sources are archived and their sections reassigned to the target. */
  async function mergeTopics(targetTopicId: number, sourceTopicIds: number[]): Promise<ApiResponse<PersistentTopic>> {
    return apiClient.post(`/daily-reports/topics/${targetTopicId}/merge`, { source_topic_ids: sourceTopicIds })
  }

  /** Carve sections out of a topic into a freshly created topic. */
  async function splitTopic(sourceTopicId: number, sectionIds: number[], label: string): Promise<ApiResponse<PersistentTopic>> {
    return apiClient.post(`/daily-reports/topics/${sourceTopicId}/split`, { section_ids: sectionIds, label })
  }

  return {
    generateDailyReport,
    getTodayReports,
    getBoardDailyReports,
    getDailyReportDetail,
    getBoardSectionTimeline,
    getSectionLifecycle,
    getTopicLifeline,
    getReportKeyEvents,
    getTopicKeyEvents,
    putThreadKeyEvent,
    patchKeyEventMarker,
    rebucketSection,
    backfillPersistentTopics,
    updateTopic,
    deleteTopic,
    listBoardTopics,
    mergeTopics,
    splitTopic,
  }
}
