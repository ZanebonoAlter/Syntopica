import { apiClient } from './client'
import type { AiDetectionStatus, ApiResponse } from '~/types'

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
  /** Thread 范围：'single' = 单事件稿；'composite' = 综合稿（event 标签数 ≥ 2）。
   *  综合稿不计入线索数/热度/行序，但可在展示层标出（fix-lane-metric-provenance）。 */
  scope?: string
  related_article_ids: number[]
  /**
   * 线索文章的读路径投影（add-ai-generated-article-detection 4.6）：只带列表行需要的
   * id/title/判定两字段，顺序跟随 related_article_ids（后端已把疑似文章稳定排至末尾），
   * 成员不增删——「N 篇」计数仍取 related_article_ids.length。
   * 旧响应无此字段（omitempty）：前端回退到标题缓存且不做任何排序。
   */
  articles?: Array<{
    id: number
    title: string
    ai_detection_status?: AiDetectionStatus
    ai_detection_score?: number
  }>
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

/**
 * 当前首义状态（GET /daily-reports/topics/:id/anchor）。
 *
 * `source`：`auto` = 创建话题时写入（**无节点集**）；`manual` = 人工选定的节点范围。
 * `section_ids` 只在人工首义下有值（人工选定的节点，可能含当前取数窗口之外的节点）。
 */
export interface TopicAnchorState {
  topic_id: number
  source: 'auto' | 'manual'
  section_ids: number[]
  /** 人工改写次数（留痕条数）。 */
  override_count: number
  /** 生效时机；人工改写为 `next_report`，自动首义为空串。 */
  effective_from: string
  /**
   * 首义来源节点（可考性三态）。
   *
   * `resolved=true` → 来源节点**可考**，`section_id` / `title` / `period_date`
   * 齐备；`resolved=false` → 来源**不可考**，三个节点字段为 `null`
   * （不得用「最早成员」等推测值顶替——虚构即编造）。
   *
   * 缺省 = 后端未升级（旧响应无此段）；前端按兜底口径处理，不崩。
   */
  anchor_origin?: TopicAnchorOrigin | null
  /**
   * 当前质心的口径与输入（滚动窗口）。
   *
   * 缺省 = 后端未升级；前端不造数（该块整体不渲染）。
   */
  centroid?: TopicAnchorCentroid | null
}

/**
 * 首义来源节点（与后端 `TopicAnchorOrigin` 的 json tag 逐字对齐）。
 */
export interface TopicAnchorOrigin {
  /** 来源是否可考；false = 不可考（节点字段为 null）。 */
  resolved: boolean
  /** 来源节点 id；不可考时为 null。 */
  section_id: number | null
  /** 来源节点标题（section 的 cluster_label）；不可考时为 null。 */
  title: string | null
  /** 来源节点报告期（YYYY-MM-DD）；不可考时为 null。 */
  period_date: string | null
}

/** 参与滚动计算的一个节点（质心滚动窗口的只读明细）。 */
export interface TopicAnchorCentroidMember {
  section_id: number
  title: string
  period_date: string
}

/**
 * 当前质心的滚动窗口口径（与后端 `TopicAnchorCentroid` 的 json tag 逐字对齐）。
 *
 * `degraded=true` 表示当前质心**不是正常混合口径**：可解析向量不足 2 条时退回首义
 * 向量，首义不可用时退化为纯滚动均值——两种分支落点不同，因此它只是**单一退化标记**，
 * 前端文案不得断言成其中一种。
 */
export interface TopicAnchorCentroid {
  /** 滚动窗口大小（配置默认 30）。 */
  window_size: number
  /** 首义权重 α（配置默认 0.5）。 */
  anchor_weight: number
  /** 参与滚动计算的 section 集合（与 `members` 同序）。 */
  section_ids: number[]
  /** 参与滚动计算的节点数（后端未给时按 `section_ids.length` 回退）。 */
  section_count?: number
  /** 参与滚动计算的节点明细（只读）。 */
  members: TopicAnchorCentroidMember[]
  /** 退化标记（单一布尔；两种退化分支的落点不同）。 */
  degraded: boolean
}

/**
 * 人工重设首义的写入结果（PATCH /daily-reports/topics/:id/anchor）。
 *
 * 与后端 `AnchorFromSectionsResult` 的 json tag 逐字对齐（review R-12）。
 */
export interface TopicAnchorWriteResult {
  topic_id: number
  /** 输入模式；当前恒为 `selected_sections`。 */
  source: string
  /** 去重后的选定节点 id。 */
  selected_sections: number[]
  /** 该话题首义被人工改写的累计次数（留痕行数）。 */
  override_count: number
  /** 生效时机；恒为 `next_report`。 */
  effective_from: string
}

/**
 * 泳道归属变更的结果（PATCH /daily-reports/sections/:id/assignment）。
 *
 * 与后端 `AssignmentChangeResult` 的 json tag 逐字对齐（review R-12）。
 */
export interface AssignmentChangeResult {
  section_id: number
  /** 变更后的归属；null = 游离节点（不归属任何泳道）。 */
  topic_id: number | null
  /** 变更前的归属；null = 此前无归属。 */
  previous_topic_id: number | null
  /** `detach`（移除）/ `attach`（加回）。 */
  action: 'detach' | 'attach'
  /** 是否真的写了行；幂等加回返回 false（不写列、不追加留痕）。 */
  changed: boolean
  /** 加回到目标锚点的实算距离；节点无可用向量时缺失。 */
  topic_match_distance?: number
  /** 被重建关系图的版块。 */
  board_id: number
}

/**
 * 批量移除某天的结果（POST /daily-reports/topics/:id/detach-date）。
 *
 * 与后端 `BatchDetachResult` 的 json tag 逐字对齐（review R-12）。
 */
export interface BatchDetachResult {
  topic_id: number
  /** 被释放的日期（`YYYY-MM-DD`）。 */
  period_date: string
  /** 被释放的节点数。 */
  detached: number
  /** 被释放的节点 id（升序）。 */
  section_ids: number[]
  /** 被重建关系图的版块。 */
  board_id: number
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

/**
 * 线索相关推荐（replace-lane-lifeline-with-topic-events D3/D4）。
 *
 * 一条历史线索 = 一个 chip，身份是 **thread 本身**（不做语义归并）；相似度是
 * 两条 thread 向量的余弦，不是概率、不掺时间衰减/篇数权重，小数只放进预览浮层。
 */
export interface ThreadRelationItem {
  thread_id: number
  title: string
  summary: string
  /** 该候选所属日报日期（YYYY-MM-DD）；窗口 [report_date-days, report_date-1]，不含当期。 */
  period_date: string
  /** 余弦相似度（未取整），只用于档位、进度条与浮层核对。 */
  score: number
  /** strong（≥0.85）/ related（≥0.75）；chip 上不显示小数。 */
  level: 'strong' | 'related'
  /** 人工确认过的关键事件命中该候选的 event 标签 → chip 带 ★（只读）。 */
  confirmed: boolean
  /** 候选线索的首篇来源文章；缺失 = 该线索无文章或行已删（不编造标题）。 */
  source_article: ThreadRelationSourceArticle | null
}

/** 候选线索的首篇来源文章（点击 chip 直接开它的站内阅读预览）。 */
export interface ThreadRelationSourceArticle {
  id: number
  title: string
  feed: string
  published_at: string | null
}

/**
 * `GET /daily-reports/:id/thread-relations` 响应（一次请求覆盖整份日报的所有线索）。
 *
 * `threads` 按**当前期** thread id 分组，键是字符串（JSON 对象）；没有推荐的线索
 * 不出现在这里。`status`：`partial` = 当期有线索缺向量被跳过，`empty` = 无推荐。
 */
export interface ThreadRelationsResponse {
  report_id: number
  /** 实际生效窗口（7 或 30），服务端回显，前端按它标注档位。 */
  days: number
  window_start: string
  window_end: string
  status: 'ready' | 'partial' | 'empty'
  threads: Record<string, ThreadRelationItem[]>
  total: number
  skipped_no_embedding: number
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

/**
 * 缺口条目（GET /daily-reports/gaps 的一格）。
 *
 * 一格 = 某天某个板块「有候选内容但没有报告」——服务端按生成 Step1 的严格口径
 * 判定（active + event + NOT direction_mismatch + 跨板排他），前端不自行推算。
 * `board_label` 可能为空（旧数据/无标签板块），展示侧回退成 `Board #N`。
 */
export interface DailyReportGap {
  /** 报告日（YYYY-MM-DD；窗口不含今天）。 */
  date: string
  board_id: number
  board_label: string
}

/** 补填运行的终态（四态；与日报生成终态互不抑制）。 */
export type DailyReportBackfillStatus = 'success' | 'partial' | 'failed' | 'budget_exhausted'

/**
 * 最近一次补填运行的摘要（缺口查询响应的 `last_run`）。
 *
 * 无运行记录时后端**省略**该字段——故这里是可选字段，不是 null 语义。
 */
export interface DailyReportBackfillRunSummary {
  run_id: string
  status: DailyReportBackfillStatus
  /** 已补出篇数。 */
  filled: number
  /** 真失败篇数。 */
  failed: number
  /** 剩余缺口篇数（预算耗尽时 > 0）。 */
  remaining: number
  window_start: string
  window_end: string
  started_at: string
  /** 收尾时刻；运行未收尾时后端省略该字段。 */
  finished_at?: string
}

/** 缺口查询响应（GET /daily-reports/gaps）。 */
export interface DailyReportGaps {
  window_start: string
  window_end: string
  /** 实际窗口天数（受保留窗口收窄）。 */
  days: number
  retention_days: number
  /** 平铺的缺口格（前端自己按 date 分组）。 */
  gaps: DailyReportGap[]
  /** 有缺口的日期数。 */
  missing_days: number
  /** 缺口格总数（= 篇数）。 */
  total_missing: number
  /** 最近一次补填运行摘要；无运行记录时缺省。 */
  last_run?: DailyReportBackfillRunSummary
}

/** 补填受理响应（POST /daily-reports/backfill-gaps）：受理即返回，异步执行。 */
export interface DailyReportBackfillAccepted {
  job_id: string
  run_id: string
  status: string
  window_start: string
  window_end: string
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

  /**
   * 查询近 N 天的日报缺口（只读；不写库、不触发生成）。
   *
   * 缺口判定口径在服务端（与生成 Step1 同谓词），前端**不自行推算**——
   * 只消费 `missing_days` / `total_missing` 与平铺的 `gaps`。
   */
  async function getDailyReportGaps(params?: { days?: number }): Promise<ApiResponse<DailyReportGaps>> {
    const query = params?.days != null ? `?days=${params.days}` : ''
    return apiClient.get<DailyReportGaps>(`/daily-reports/gaps${query}`)
  }

  /**
   * 触发缺口补填（异步：受理即返回 job_id，服务端后台旧到新串行补）。
   *
   * 请求体三选一：`{ date }` 补单日 / `{ days }` 收窄窗口 / `{}` 按默认窗口补齐。
   * `date` 与 `days` 互斥（同时给后端 400 `conflicting_scope`）。
   * `board_id` 可选：把范围收窄到单个板块。
   */
  async function triggerGapBackfill(
    params: { date?: string; days?: number; board_id?: number } = {},
  ): Promise<ApiResponse<DailyReportBackfillAccepted>> {
    return apiClient.post<DailyReportBackfillAccepted>('/daily-reports/backfill-gaps', params)
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

  /**
   * 整份日报的线索相关推荐（thread 向量口径）：一次请求覆盖所有线索，前端不为每条
   * 线索各发一次。`days` 只认 7/30，其它值服务端回落 7 并在响应 `days` 回显。
   */
  async function getThreadRelations(reportId: number, params?: { days?: number }): Promise<ApiResponse<ThreadRelationsResponse>> {
    const query = params?.days != null ? `?days=${params.days}` : ''
    return apiClient.get(`/daily-reports/${reportId}/thread-relations${query}`)
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

  /**
   * 人工重设首义范围（lane-manual-correction-loop）：写入该话题的**人工首义节点集**。
   *
   * 语义边界（契约见 `specs/lane-manual-correction`）：首义 = 一组被选中的节点的 embedding 均值；
   * 改首义改的是**事实**，派生的重心 / 真空标记 / 泳道快照由算法次日重算，前端不做立即重算。
   */
  async function updateTopicAnchor(topicId: number, sectionIds: number[]): Promise<ApiResponse<TopicAnchorWriteResult>> {
    return apiClient.patch(`/daily-reports/topics/${topicId}/anchor`, { section_ids: sectionIds })
  }

  /**
   * 读当前首义（lane-manual-correction-loop）：首义来源 + 人工首义的节点集。
   *
   * `source = "auto"` = 首义是创建话题时写入的（**无节点集**，界面不得虚构节点列表）；
   * `source = "manual"` = 首义由人工选定的节点范围定义，节点集在 `section_ids` 里
   * （可能含窗口外节点 → 画布「更早」折叠带）。
   *
   * 另带两段只读块：`anchor_origin`（首义来源节点，可考 / 不可考）与 `centroid`
   * （当前质心的滚动窗口口径与退化标记）。两者都是**只增不改**的扩展字段，
   * 旧后端缺失时前端按兜底口径处理（不崩、不造数）。
   */
  async function getTopicAnchor(topicId: number): Promise<ApiResponse<TopicAnchorState>> {
    return apiClient.get(`/daily-reports/topics/${topicId}/anchor`)
  }

  /**
   * 改归属（双向，lane-manual-correction-loop）：把某个 section 归到目标话题。
   *
   * `topicId = null` = **解除归属**（该内容不再归属任何泳道；内容仍在，以后仍可再参与候选），
   * 不是删除内容。
   */
  async function setSectionAssignment(sectionId: number, topicId: number | null): Promise<ApiResponse<AssignmentChangeResult>> {
    return apiClient.patch(`/daily-reports/sections/${sectionId}/assignment`, { topic_id: topicId })
  }

  /** 从某话题的某个报告期摘掉该日归属（lane-manual-correction-loop）：该日内容不再归属本泳道，内容仍在。 */
  async function detachTopicDate(topicId: number, periodDate: string): Promise<ApiResponse<BatchDetachResult>> {
    return apiClient.post(`/daily-reports/topics/${topicId}/detach-date`, { period_date: periodDate })
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
    getDailyReportGaps,
    triggerGapBackfill,
    getTodayReports,
    getBoardDailyReports,
    getDailyReportDetail,
    getBoardSectionTimeline,
    getSectionLifecycle,
    getTopicLifeline,
    getThreadRelations,
    getReportKeyEvents,
    getTopicKeyEvents,
    putThreadKeyEvent,
    patchKeyEventMarker,
    rebucketSection,
    backfillPersistentTopics,
    updateTopic,
    updateTopicAnchor,
    getTopicAnchor,
    setSectionAssignment,
    detachTopicDate,
    deleteTopic,
    listBoardTopics,
    mergeTopics,
    splitTopic,
  }
}
