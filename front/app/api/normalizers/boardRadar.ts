/**
 * 今日雷达总览 normalizer（rework-workshop-into-radar-and-desk 8A/R-18）。
 *
 * 后端 `GET /semantic-boards/overview` 的 snake_case JSON 协议**完全不改**；
 * 仅在 Radar 客户端边界归一为 camelCase 读模型并把数字 ID 字符串化
 * （code-style §9：转换只在 API/store 层，组件不散落映射）。raw 载荷类型
 * 与后端 handler 输出一一对应（零值语义：无日报版块 report_* 为空/0，
 * anchor_date 可缺省，days/lanes 恒为数组）。
 */
import type { ApiResponse, PaginationMeta } from '~/types'

/** 后端原始响应行（snake_case wire 契约，勿在前端消费）。 */
export interface BoardRadarRawLane {
  topic_id: number
  label: string
  section_count_14d: number
  thread_count_14d: number
  counts: number[]
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
  today_sections: number
  last3_sections: number
  prev3_sections: number
  attention: number
  data_delayed: boolean
  days: number[]
  lanes: BoardRadarRawLane[]
}

export interface BoardRadarRawOverview {
  date: string
  boards: BoardRadarRawBoard[]
}

/** 归一化后的客户端读模型（camelCase + 字符串 ID，组件只消费此形态）。 */
export interface RadarLane {
  topicId: string
  label: string
  sectionCount14d: number
  threadCount14d: number
  counts: number[]
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
  todaySections: number
  last3Sections: number
  prev3Sections: number
  attention: number
  dataDelayed: boolean
  days: number[]
  lanes: RadarLane[]
}

export interface RadarOverview {
  date: string
  boards: RadarBoard[]
}

function num(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
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
    sectionCount14d: num(lane?.section_count_14d),
    threadCount14d: num(lane?.thread_count_14d),
    counts: Array.isArray(lane?.counts) ? lane.counts : [],
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
    todaySections: num(board?.today_sections),
    last3Sections: num(board?.last3_sections),
    prev3Sections: num(board?.prev3_sections),
    attention: num(board?.attention),
    dataDelayed: board?.data_delayed === true,
    days: Array.isArray(board?.days) ? board.days : [],
    lanes: Array.isArray(board?.lanes) ? board.lanes.map(normalizeRadarLane) : [],
  }
}

export function normalizeRadarOverview(raw: BoardRadarRawOverview): RadarOverview {
  return {
    date: str(raw?.date),
    boards: Array.isArray(raw?.boards) ? raw.boards.map(normalizeRadarBoard) : [],
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
