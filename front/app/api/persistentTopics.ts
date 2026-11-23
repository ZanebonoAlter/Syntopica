import { apiClient } from './client'
import { camelizeKeys, mapApiResponse } from '~/utils/api-helpers'
import type { ApiResponse } from '~/types'

/**
 * 手动建泳道编排态 API（切片③）。
 *
 * - 候选端点 GET .../compose-candidates 回传带 embedding 的 section，前端实时算
 *   聚合锚点 / 离群（见 composeReport.ts），不在后端逐次勾选往返。
 * - 保存走切片①的 POST .../persistent-topics/manual。
 *
 * API 边界归一：snake_case → camelCase 在本层完成，数字 id 转字符串（与
 * topicWatches 约定一致）。embedding 数组原样透传（number[]）。
 */

/** 编排态候选 section 现有归属的轻量话题摘要（编排态判断移出/分组用 status）。 */
export interface CandidateTopicBrief {
  id: string
  label: string
  status: string
}

/** 编排态默认取数窗口天数（契约 §1.1：默认 30）。 */
const DEFAULT_COMPOSE_DAYS = 30
/** 服务端未回传 match_threshold 时的兜底阈值（契约 §2.2：默认 0.3）。 */
const DEFAULT_MATCH_THRESHOLD = 0.3

/**
 * 编排态 section 下一条「已确认关键线索」（★ 只读投影）。
 *
 * - title 是服务端冻结快照（title_snapshot），客户端不改写、不回填新闻正文；
 * - threadId / confirmedAt 可空：日报重建可能把来源指针置 NULL（脱离快照）；
 * - 不参与选入/接近度/质心/阈值任何计算。
 */
export interface ComposeConfirmedKeyEvent {
  markerId: string
  threadId?: string
  title: string
  confirmedAt?: string
}

/** 编排态候选 section（embedding 已展开为 number[]，供前端实时计算）。 */
export interface ComposeCandidate {
  id: string
  reportId?: string
  periodDate: string
  clusterLabel: string
  embedding: number[]
  persistentTopicId?: string
  topicMatchConfidence?: string
  persistentTopic?: CandidateTopicBrief
  /**
   * 该 section 的非综合稿（非 composite）线索数（unify-lane-workbench D2）。
   *
   * - 旧后端缺字段 / null / 非有限数 → null（未知，不冒充 0）；
   * - 0 是合法已知值，保留不吞；
   * - 只由 `clue_count` 归一化产生，不用 section 数 / thread 总数替代。
   */
  clueCount?: number | null
  /**
   * 该 section 下已确认的关键线索（★ 只读投影）。API 层恒为数组（无标记为
   * []），归一化保证存在；类型标可选只为兼容手工构造的旧 fixture。
   * 长度 > 0 才画星形。
   */
  confirmedKeyEvents?: ComposeConfirmedKeyEvent[]
}

/** 窗口内两端可见的原关系（identity / similarity 都保留，画布只画这批边）。 */
export interface ComposeRelation {
  fromId: string
  toId: string
  distance: number
  relationType: string
}

/**
 * 候选链（服务端唯一实现的候选片段口径）。
 *
 * 前端只渲染不重算：同一 candidate 话题按原 relations 的连通片段切分，
 * 合格条件是「节点 ≥2 且首末 UTC 日差 ≥3」，中间允许断档。
 */
export interface ComposeCandidateChain {
  /** 稳定键：`{topicId}:{分量内最小 section id}`。 */
  key: string
  topicId: string
  topicLabel: string
  topicStatus: string
  sectionIds: string[]
  firstDate: string
  lastDate: string
  /** 首末报告日期的 UTC 日差。 */
  spanDays: number
}

/** 实际生效的取数窗口（由服务端回显，前端不自行推算末日）。 */
export interface ComposeWindow {
  /** 生效窗口起始日；服务端返回 null（无锚点/全历史档）时归一化为空串。 */
  from: string
  /** 生效窗口结束日；服务端返回 null（无锚点/全历史档）时归一化为空串。 */
  to: string
  /** 实际跨度天数（from/to 生效时为实际跨度，否则为请求的 days）。 */
  days: number
  /** 该版块最新已完成日报日期；无已完成日报时为 null。 */
  latestCompletedDate: string | null
}

/** GET .../compose-candidates 的归一化响应。 */
export interface ComposeCandidatesData {
  sections: ComposeCandidate[]
  relations: ComposeRelation[]
  candidateChains: ComposeCandidateChain[]
  matchThreshold: number
  window: ComposeWindow | null
  /**
   * 「更早成员」：该话题里报告期早于窗口首日的全部成员（升序）。
   *
   * null / 缺省 = 后端未提供该字段（旧后端 / 编排画布不传参数）→ 计数回退只显示
   * 窗口数；数组（可为空）= 后端给了全量口径。
   */
  earlyMembers?: ComposeEarlyMember[] | null
}

/** 「更早成员」行（与候选 section 同形；更早成员不上画布，但向量仍随行下发）。 */
export interface ComposeEarlyMember {
  /**
   * 内容向量（缺省 = 后端未给/解析失败）：修正工作面的关联推荐质心需要窗口外
   * 成员参与，否则窗口内成员为 0 时质心为空、候选区全空。
   */
  embedding?: number[]
  id: string
  reportId?: string
  periodDate: string
  clusterLabel: string
  persistentTopicId?: string
  /** 锚定距离快照；接近度 = 1 − 该值。缺失 = 无距离口径（不伪造）。 */
  topicMatchDistance?: number
}

/**
 * 编排态取数窗口参数。
 *
 * - `days`：快捷档（7/14/30/365，OQ-2 后上限 365），以最新已完成日报为末日、含首末报告日。
 * - `from`/`to`：自选起止（闭区间），给出时 `days` 被忽略；必须成对给出。
 * - `allMembersTopicId`：泳道修正模式的全量成员口径（独立参数，不影响窗口）。
 * - `omitEmbeddings`：轻量取数（unify-lane-workbench 2.4）——曲线只需要
 *   `period_date + persistent_topic_id + clue_count`，跳过每条 10,244 B 的向量；
 *   该形态下 `sections[].embedding` 缺省，归一为 `[]`（无向量，不参与质心）。
 */
export interface ComposeCandidatesQuery {
  days?: number
  from?: string
  to?: string
  /**
   * 泳道修正模式：取该话题的**全量成员**（窗口外的成员一并返回）。
   * 不给 = 编排画布口径（行为不变）。
   */
  allMembersTopicId?: number
  /** 曲线路径专用（不画点就不拉向量）；不传 = 既有带向量行为不变。 */
  omitEmbeddings?: boolean
}

/** 手动建泳道保存后返回的新话题。 */
export interface ManualTopicResult {
  id: string
  label: string
  status: string
  source: string
}

/** createManualLane 的归一化响应。skipped 为无向量被跳过的 section id。 */
export interface ManualLaneResult {
  topic: ManualTopicResult
  skipped: string[]
}

/** POST .../embed-query 的归一化响应。query 文本→embedding 向量（与 section 同模型）。 */
export interface EmbedQueryData {
  embedding: number[]
}

// ── backend snake_case payloads ─────────────────────────────────────────────

interface ComposeCandidatePayload {
  id: number
  report_id: number
  period_date: string
  cluster_label: string
  /** 轻量取数（omit_embeddings=1）时缺省 → 归一为 []（无向量，不参与质心）。 */
  embedding?: number[]
  persistent_topic_id?: number
  topic_match_confidence?: string
  persistent_topic?: { id: number, label: string, status: string, color: string }
  /** 后端补字段（unify-lane-workbench）；旧后端缺省 → 归一化为 null（未知）。 */
  clue_count?: number | null
  /** ★ 只读投影；旧后端缺省 → 归一化为 []（无标记）。 */
  confirmed_key_events?: {
    marker_id: number | string
    thread_id?: number | string | null
    title?: string | null
    confirmed_at?: string | null
  }[]
}

interface ComposeRelationPayload {
  from_id: number | string
  to_id: number | string
  distance: number
  relation_type: string
}

interface ComposeCandidateChainPayload {
  key: string
  topic_id: number | string
  topic_label: string
  topic_status: string
  section_ids: (number | string)[]
  first_date: string
  last_date: string
  span_days: number
}

interface ComposeWindowPayload {
  /** 服务端可能返回 null（无锚点/全历史档），不是缺省也不是空串。 */
  from: string | null
  to: string | null
  days: number
  latest_completed_date: string | null
}

interface ComposeEarlyMemberPayload {
  id: number
  report_id?: number
  period_date: string
  cluster_label: string
  /** 内容向量：缺省（旧后端 / 无向量成员）→ 归一为 undefined，不塞 []。 */
  embedding?: number[]
  persistent_topic_id?: number
  topic_match_distance?: number | null
}

interface ComposeCandidatesPayload {
  sections: ComposeCandidatePayload[]
  relations?: ComposeRelationPayload[]
  candidate_chains?: ComposeCandidateChainPayload[]
  match_threshold: number
  window?: ComposeWindowPayload
  /** 缺失 = 后端未提供全量成员口径（与「空数组」不同）。 */
  early_members?: ComposeEarlyMemberPayload[]
}

interface ManualTopicPayload {
  id: number
  label: string
  status: string
  source: string
}

interface ManualLanePayload {
  topic: ManualTopicPayload
  skipped: number[]
}

interface EmbedQueryPayload {
  embedding: number[]
}

// ── normalizers ─────────────────────────────────────────────────────────────

function normalizeCandidate(p: ComposeCandidatePayload): ComposeCandidate {
  const out: ComposeCandidate = {
    id: String(p.id),
    reportId: String(p.report_id),
    periodDate: p.period_date,
    clusterLabel: p.cluster_label,
    embedding: p.embedding ?? [],
    // 0 保留；缺字段 / null / 非有限数（NaN、Infinity）/ 负数（非非负整数）→ null 未知。
    clueCount: typeof p.clue_count === 'number' && Number.isFinite(p.clue_count) && p.clue_count >= 0 ? p.clue_count : null,
    confirmedKeyEvents: (p.confirmed_key_events ?? []).map((event) => {
      const item: ComposeConfirmedKeyEvent = {
        markerId: String(event.marker_id),
        title: event.title ?? '',
      }
      if (event.thread_id != null) item.threadId = String(event.thread_id)
      if (event.confirmed_at) item.confirmedAt = event.confirmed_at
      return item
    }),
  }
  if (p.persistent_topic_id != null) out.persistentTopicId = String(p.persistent_topic_id)
  if (p.topic_match_confidence) out.topicMatchConfidence = p.topic_match_confidence
  if (p.persistent_topic) out.persistentTopic = { id: String(p.persistent_topic.id), label: p.persistent_topic.label, status: p.persistent_topic.status }
  return out
}

function normalizeRelation(p: ComposeRelationPayload): ComposeRelation {
  return {
    fromId: String(p.from_id),
    toId: String(p.to_id),
    distance: typeof p.distance === 'number' ? p.distance : Number(p.distance ?? 0),
    relationType: p.relation_type ?? 'similarity',
  }
}

function normalizeChain(p: ComposeCandidateChainPayload): ComposeCandidateChain {
  return {
    key: String(p.key),
    topicId: String(p.topic_id),
    topicLabel: p.topic_label ?? '',
    topicStatus: p.topic_status ?? 'candidate',
    sectionIds: (p.section_ids ?? []).map(id => String(id)),
    firstDate: p.first_date ?? '',
    lastDate: p.last_date ?? '',
    spanDays: typeof p.span_days === 'number' ? p.span_days : Number(p.span_days ?? 0),
  }
}

function normalizeWindow(p?: ComposeWindowPayload): ComposeWindow | null {
  if (!p) return null
  return {
    from: p.from ?? '',
    to: p.to ?? '',
    days: typeof p.days === 'number' ? p.days : Number(p.days ?? 0),
    latestCompletedDate: p.latest_completed_date ?? null,
  }
}

function normalizeEarlyMember(p: ComposeEarlyMemberPayload): ComposeEarlyMember {
  const out: ComposeEarlyMember = {
    id: String(p.id),
    periodDate: p.period_date,
    clusterLabel: p.cluster_label,
  }
  if (p.report_id != null) out.reportId = String(p.report_id)
  if (p.persistent_topic_id != null) out.persistentTopicId = String(p.persistent_topic_id)
  if (typeof p.topic_match_distance === 'number' && Number.isFinite(p.topic_match_distance)) {
    out.topicMatchDistance = p.topic_match_distance
  }
  // 与 sections 侧同一个向量透传口径：有值才拷贝（空数组不是向量，归一为 undefined）。
  if (p.embedding?.length) out.embedding = p.embedding
  return out
}

function normalizeManualLane(p: ManualLanePayload): ManualLaneResult {
  return {
    topic: {
      id: String(p.topic.id),
      label: p.topic.label,
      status: p.topic.status,
      source: p.topic.source,
    },
    skipped: (p.skipped ?? []).map(id => String(id)),
  }
}

// ── API ─────────────────────────────────────────────────────────────────────

export function usePersistentTopicsApi() {
  /**
   * GET /api/semantic-boards/:boardId/persistent-topics/compose-candidates
   *
   * `query` 兼容两种形态：
   * - 数字 = 旧的 `?days=N` 调用（`useInlineCompose` 仍在用，语义不变）；
   * - 对象 = 新编排态窗口参数 `{days?, from?, to?, allMembersTopicId?}`；
   *   `from`/`to` 必须成对给出，给出时 `days` 被忽略；都不给按 `days=30`；
   *   `allMembersTopicId` 走独立的 `all_members_topic_id` 参数（泳道修正模式
   *   的全量成员口径；不给 = 编排画布行为不变）。
   *
   * 归一化在本层完成：id 一律字符串化、字段一律 camelCase，`relations` /
   * `candidate_chains` / `window` 缺省时降级为空（后端未升级也不炸）。
   */
  async function getComposeCandidates(
    boardId: number | string,
    query: number | ComposeCandidatesQuery = {},
  ): Promise<ApiResponse<ComposeCandidatesData>> {
    const params = new URLSearchParams()
    if (typeof query === 'number') {
      params.set('days', String(query))
    } else {
      if (query.from && query.to) {
        params.set('from', query.from)
        params.set('to', query.to)
      } else if (query.days != null) {
        params.set('days', String(query.days))
      } else {
        params.set('days', String(DEFAULT_COMPOSE_DAYS))
      }
      if (query.allMembersTopicId != null) {
        params.set('all_members_topic_id', String(query.allMembersTopicId))
      }
      if (query.omitEmbeddings) {
        params.set('omit_embeddings', '1')
      }
    }
    const res = await apiClient.get<ComposeCandidatesPayload>(
      `/semantic-boards/${boardId}/persistent-topics/compose-candidates?${params.toString()}`,
    )
    if (!res.success || !res.data) return { success: false, error: res.error || '加载候选失败' }
    const data: ComposeCandidatesData = {
      sections: (res.data.sections ?? []).map(normalizeCandidate),
      relations: (res.data.relations ?? []).map(normalizeRelation),
      candidateChains: (res.data.candidate_chains ?? []).map(normalizeChain),
      matchThreshold: typeof res.data.match_threshold === 'number' ? res.data.match_threshold : DEFAULT_MATCH_THRESHOLD,
      window: normalizeWindow(res.data.window),
      // null = 后端没给这个字段（全量口径不可得 → 计数回退只显示窗口数）。
      earlyMembers: res.data.early_members ? res.data.early_members.map(normalizeEarlyMember) : null,
    }
    return mapApiResponse(res, data)
  }

  /**
   * POST /api/semantic-boards/:boardId/persistent-topics/manual
   * sectionIds 来自 ComposeCandidate.id（字符串），请求体转回数字（后端收 []uint）。
   */
  async function createManualLane(
    boardId: number | string,
    label: string,
    sectionIds: string[],
  ): Promise<ApiResponse<ManualLaneResult>> {
    const res = await apiClient.post<ManualLanePayload>(
      `/semantic-boards/${boardId}/persistent-topics/manual`,
      { label, section_ids: sectionIds.map(id => Number(id)) },
    )
    if (!res.success || !res.data) return { success: false, error: res.error || '保存失败', message: res.message }
    return mapApiResponse(res, normalizeManualLane(camelizeKeys<ManualLanePayload>(res.data)))
  }

  /**
   * POST /api/semantic-boards/:boardId/persistent-topics/embed-query
   * 文本 → embedding（与 section 同模型），供编排态候选池语义排序（task 3.12）。
   * 失败返回 success=false，调用方负责降级（回退默认序）。
   */
  async function embedQuery(
    boardId: number | string,
    query: string,
  ): Promise<ApiResponse<EmbedQueryData>> {
    const res = await apiClient.post<EmbedQueryPayload>(
      `/semantic-boards/${boardId}/persistent-topics/embed-query`,
      { query },
    )
    if (!res.success || !res.data) return { success: false, error: res.error || '搜索向量生成失败' }
    return mapApiResponse(res, { embedding: res.data.embedding ?? [] })
  }

  return { getComposeCandidates, createManualLane, embedQuery }
}
