/**
 * 今日雷达总览 normalizer（rework-workshop-into-radar-and-desk 8A/R-18；
 * retire-story-adopt-event-metrics 3.2 换当天线索口径）。
 *
 * 后端 `GET /semantic-boards/overview` 的 snake_case JSON 协议**完全不改**；
 * 仅在 Radar 客户端边界归一为 camelCase 读模型并把数字 ID 字符串化
 * （code-style §9：转换只在 API/store 层，组件不散落映射）。raw 载荷类型
 * 与后端 handler 输出一一对应（零值语义：无日报版块 report_* 为空/0，
 * anchor_date 可缺省，lanes 恒为数组）。
 *
 * 量纲（D4/D5/D10）：版块行关注度 = Σ 泳道当天热度（后端算好，一位小数）；
 * 泳道小矩阵与 3 日窗口统一为「当日线索合计」（`clue_counts` / `today_clues`），
 * 事件线数、分区数、来源数与颜色分档整体退役。旧后端缺 `clue_counts` /
 * `today_clues` 时归一为 `null`（未知，不造 0），由展示层标「线索数未知」。
 * 顶层清单段由事件线换成「当天线索最多的泳道」（`top_lanes`）；旧后端缺该段时
 * `topLanes = null`，右栏回退版块级日报标题列表（不渲染成业务空态）。
 */
import type { ApiResponse, PaginationMeta } from '~/types'

/** 后端原始响应行（snake_case wire 契约，勿在前端消费）。 */
export interface BoardRadarRawLane {
  topic_id: number
  label: string
  /** 14 天逐日线索合计（oldest→newest）；缺省 = 旧后端（未知，不造 0）。 */
  clue_counts?: number[]
  /** 全窗线索合计（次级排序键，后端已排序）。 */
  thread_count_14d?: number
  /** 退役字段（旧后端仍可能下发，前端不再消费）：分区数与事件线数逐日轴。 */
  section_count_14d?: number
  counts?: number[]
  story_counts?: number[]
}

export interface BoardRadarRawBoard {
  board_id: number
  label: string
  tag_count: number
  report_date: string
  report_title: string
  report_article_count: number
  report_event_count: number
  /** 版块自身最新数据日；缺省表示后端未提供（UI 不标延迟）。 */
  anchor_date?: string
  /** 当日 / 近 3 日 / 前 3 日线索合计；缺省 = 旧后端（未知，不造 0）。 */
  today_clues?: number
  last3_clues?: number
  prev3_clues?: number
  /** 关注度 = Σ 泳道当天热度（含候选权重，一位小数）。 */
  attention: number
  /** 热度口径版本（heat-v2-lane-clues-1d）。 */
  heat_rule_version?: string
  /** 14 天逐日线索轴（oldest→newest）；缺省 = 旧后端。 */
  clue_days?: number[]
  data_delayed: boolean
  lanes: BoardRadarRawLane[]

  /**
   * 退役字段（旧后端仍可能下发，前端不再消费——留着只为响应类型兼容）：
   * `today_sections`/`last3_sections`/`prev3_sections`/`days`（分区口径）、
   * `today_stories`/`last3_stories`/`prev3_stories`（事件线口径）、
   * `today_heat`/`last3_heat`/`prev3_heat`/`heat_days`（旧半衰期热度）、
   * `stories_backfilled`/`story_identity_state`/`heat_incomplete`/`heat_stale_feed_ids`。
   */
  today_sections?: number
  last3_sections?: number
  prev3_sections?: number
  days?: number[]
  today_stories?: number
  last3_stories?: number
  prev3_stories?: number
  today_heat?: number
  last3_heat?: number
  prev3_heat?: number
  heat_days?: number[]
  stories_backfilled?: boolean
  story_identity_state?: string
  heat_incomplete?: boolean
  heat_stale_feed_ids?: number[]
}

export interface BoardRadarRawOverview {
  date: string
  boards: BoardRadarRawBoard[]
  /**
   * 泳道清单段（D10）：跨版块归并后的「当天线索最多的泳道」（上限 8，服务端截断）
   * + 不完整/滞后标记。缺省 = 旧后端（客户端回退版块级日报标题列表）。
   */
  top_lanes?: BoardRadarRawTopLanes
}

/** 后端原始泳道清单条目（snake_case wire 契约，勿在前端消费）。 */
export interface BoardRadarRawTopLaneItem {
  board_id: number
  board_label: string
  topic_id: number
  topic_label: string
  /** 该泳道在**它自己版块锚点日**的线索合计。 */
  clue_count: number
  /** 条目所属版块自己的锚点日（滞后版块 ≠ 全局日）。 */
  anchor_date: string
}

/** 后端原始泳道清单段。 */
export interface BoardRadarRawTopLanes {
  anchor_date: string
  lanes: BoardRadarRawTopLaneItem[]
  /** true = 部分版块延迟或降级，本次排序不完整。 */
  partial: boolean
  degraded_board_ids: number[]
  lagging_board_ids: number[]
}

/** 归一化后的客户端读模型（camelCase + 字符串 ID，组件只消费此形态）。 */
export interface RadarLane {
  topicId: string
  label: string
  /** 14 天逐日线索合计；null = 旧后端缺该字段（未知，不造 0）。 */
  clueCounts: number[] | null
  /** 全窗线索合计（raw thread_count_14d）。 */
  threadCount14d: number
}

export interface RadarBoard {
  boardId: string
  label: string
  tagCount: number
  reportDate: string
  reportTitle: string
  reportArticleCount: number
  reportEventCount: number
  anchorDate?: string
  /** 当日 / 近 3 日 / 前 3 日线索合计；null = 旧后端缺字段（未知，不造 0）。 */
  todayClues: number | null
  last3Clues: number | null
  prev3Clues: number | null
  /** 关注度 = Σ 泳道当天热度（含候选权重，一位小数）。 */
  attention: number
  heatRuleVersion: string
  /** 14 天逐日线索轴；null = 旧后端缺字段。 */
  clueDays: number[] | null
  dataDelayed: boolean
  lanes: RadarLane[]
}

export interface RadarTopLaneItem {
  boardId: string
  boardLabel: string
  topicId: string
  topicLabel: string
  /** 该泳道在自己版块锚点日的线索合计。 */
  clueCount: number
  /** 条目所属版块自己的锚点日。 */
  anchorDate: string
}

/** 归一化后的泳道清单段（服务端已归并/排序/截断，前端不改口径）。 */
export interface RadarTopLanes {
  anchorDate: string
  lanes: RadarTopLaneItem[]
  partial: boolean
  degradedBoardIds: string[]
  laggingBoardIds: string[]
}

export interface RadarOverview {
  date: string
  boards: RadarBoard[]
  /** 泳道清单段；null = 响应没有该段（旧后端）→ 回退版块级日报标题列表。 */
  topLanes: RadarTopLanes | null
}

function num(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

/** 数值或 null（缺字段 ≠ 真实 0：旧后端缺口径时展示「未知」而不是 0）。 */
function numOrNull(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

/** 数字数组或 null（缺字段 ≠ 全 0 轴）。 */
function numArrayOrNull(value: unknown): number[] | null {
  return Array.isArray(value) ? value.filter((item): item is number => typeof item === 'number' && Number.isFinite(item)) : null
}

function str(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function id(value: unknown): string {
  return value === null || value === undefined ? '' : String(value)
}

export function normalizeRadarLane(lane: BoardRadarRawLane): RadarLane {
  return {
    topicId: id(lane?.topic_id),
    label: str(lane?.label),
    // clue_counts 缺省（旧后端）→ null：小矩阵逐格标「线索数未知」，不拿分区数冒充。
    clueCounts: numArrayOrNull(lane?.clue_counts),
    threadCount14d: num(lane?.thread_count_14d),
  }
}

export function normalizeRadarBoard(board: BoardRadarRawBoard): RadarBoard {
  return {
    boardId: id(board?.board_id),
    label: str(board?.label),
    tagCount: num(board?.tag_count),
    reportDate: str(board?.report_date),
    reportTitle: str(board?.report_title),
    reportArticleCount: num(board?.report_article_count),
    reportEventCount: num(board?.report_event_count),
    anchorDate: board?.anchor_date ? str(board.anchor_date) : undefined,
    todayClues: numOrNull(board?.today_clues),
    last3Clues: numOrNull(board?.last3_clues),
    prev3Clues: numOrNull(board?.prev3_clues),
    attention: num(board?.attention),
    heatRuleVersion: str(board?.heat_rule_version),
    clueDays: numArrayOrNull(board?.clue_days),
    dataDelayed: board?.data_delayed === true,
    lanes: Array.isArray(board?.lanes) ? board.lanes.map(normalizeRadarLane) : [],
  }
}

export function normalizeRadarTopLaneItem(item: BoardRadarRawTopLaneItem): RadarTopLaneItem {
  return {
    boardId: id(item?.board_id),
    boardLabel: str(item?.board_label),
    topicId: id(item?.topic_id),
    topicLabel: str(item?.topic_label),
    clueCount: num(item?.clue_count),
    anchorDate: str(item?.anchor_date),
  }
}

/** 泳道清单段归一：缺字段安全默认（旧响应无该段时返回 null 而非空清单）。 */
export function normalizeRadarTopLanes(raw?: BoardRadarRawTopLanes): RadarTopLanes {
  return {
    anchorDate: str(raw?.anchor_date),
    lanes: Array.isArray(raw?.lanes) ? raw.lanes.map(normalizeRadarTopLaneItem) : [],
    partial: raw?.partial === true,
    degradedBoardIds: Array.isArray(raw?.degraded_board_ids) ? raw.degraded_board_ids.map(id) : [],
    laggingBoardIds: Array.isArray(raw?.lagging_board_ids) ? raw.lagging_board_ids.map(id) : [],
  }
}

export function normalizeRadarOverview(raw: BoardRadarRawOverview): RadarOverview {
  return {
    date: str(raw?.date),
    boards: Array.isArray(raw?.boards) ? raw.boards.map(normalizeRadarBoard) : [],
    // 该段缺席 = 旧后端：交回 null 让调用方回退版块级日报列表（旧响应 ≠ 没有泳道）。
    topLanes: raw?.top_lanes ? normalizeRadarTopLanes(raw.top_lanes) : null,
  }
}

/**
 * Radar 客户端分支判别返回：成功 = 归一化读模型；失败/无数据 = raw 字段
 * （error/status/message/pagination 及错误体 data 原始形态）透传，不归一。
 * 局部类型仅限本分支，不改全局 ApiResponse 或其它 API 的返回形状。
 */
export type RadarOverviewResponse
  = { success: true, data: RadarOverview, pagination?: PaginationMeta, message?: string }
  | { success: false, data?: BoardRadarRawOverview, pagination?: PaginationMeta, message?: string, error?: string, status?: number }

/** Radar 客户端分支接线：raw 响应 → 归一化读模型；失败/无数据 raw 字段原样透传。 */
export function mapRadarOverviewResponse(res: ApiResponse<BoardRadarRawOverview>): RadarOverviewResponse {
  if (res.success && res.data) {
    return {
      success: true,
      data: normalizeRadarOverview(res.data),
      pagination: res.pagination,
      message: res.message,
    }
  }
  // 失败（含后端异常的 success 无 data）：error/status/message/pagination 与错误体
  // raw data 逐字段透传；调用方（useBoardRadarOverview）按 success=false 或无 data
  // 走失败路径，与旧实现的消费行为一致。
  return {
    success: false,
    data: res.data,
    pagination: res.pagination,
    message: res.message,
    error: res.error,
    status: res.status,
  }
}
