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
}

/**
 * 编排态取数窗口参数。
 *
 * - `days`：快捷档（7/14/30/90），以最新已完成日报为末日、含首末报告日。
 * - `from`/`to`：自选起止（闭区间），给出时 `days` 被忽略；必须成对给出。
 */
export interface ComposeCandidatesQuery {
  days?: number
  from?: string
  to?: string
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
  embedding: number[]
  persistent_topic_id?: number
  topic_match_confidence?: string
  persistent_topic?: { id: number, label: string, status: string, color: string }
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

interface ComposeCandidatesPayload {
  sections: ComposeCandidatePayload[]
  relations?: ComposeRelationPayload[]
  candidate_chains?: ComposeCandidateChainPayload[]
  match_threshold: number
  window?: ComposeWindowPayload
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
    embedding: p.embedding,
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
   * - 对象 = 新编排态窗口参数 `{days?, from?, to?}`；`from`/`to` 必须成对给出，
   *   给出时 `days` 被忽略；都不给按 `days=30`。
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
    } else if (query.from && query.to) {
      params.set('from', query.from)
      params.set('to', query.to)
    } else if (query.days != null) {
      params.set('days', String(query.days))
    } else {
      params.set('days', String(DEFAULT_COMPOSE_DAYS))
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
