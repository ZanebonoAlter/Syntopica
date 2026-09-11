import { apiClient } from './client'
import type { ApiResponse } from '~/types'

/**
 * 泳道动态聚合 API（overview-lane-dynamics design §D4）。
 *
 * - GET /semantic-boards/:id/lane-dynamics?days=N：单请求返回主卡区 + 候选栏全部数据。
 * - 响应为后端 snake_case DTO（沿用 semanticBoards.ts 的既有约定），
 *   前端不做 key 归一——字段形状是 tasks 2.x 后端批次的实现契约。
 * - 日期→事件的对应关系由后端聚合响应显式携带，前端 SHALL NOT 自行推断（spec）。
 */

/** 滚动窗口态势快照；null = 待结算（卡片降级显示占位条，时间线照常）。 */
export interface LaneSnapshot {
  /** ≤100 字中文态势句。 */
  summary: string
  /** 汇总截止日（= 最近一份已完成报告期，YYYY-MM-DD）。 */
  as_of: string
}

/** 时间线单日内的一个锚定 section（事件源）。 */
export interface LaneTimelineSection {
  section_id: number
  label: string
  /** thread 级标题列表（后端每 section 截前 5 条，D4）。 */
  events: string[]
  /** 被后端截断未载入的事件数（「数量可截断但 SHALL 提示被折叠数」，spec）。 */
  folded_count?: number
}

/** 时间线单日节点：该日全部锚定 section 分组。 */
export interface LaneTimelineDay {
  /** YYYY-MM-DD。 */
  date: string
  sections: LaneTimelineSection[]
}

/** 主卡区单条泳道（active ∪ watch 关联，按 section_count_14d DESC 排序）。 */
export interface LaneDynamicsLane {
  topic_id: number
  label: string
  /** watch 关联（该板块 active 的 sentence_topic watch 命中该话题）。 */
  watch_linked: boolean
  section_count_14d: number
  snapshot: LaneSnapshot | null
  timeline: LaneTimelineDay[]
}

/** 候选栏条目（FilterVisibleTopics 口径的 candidate，只读提示）。 */
export interface LaneDynamicsCandidate {
  topic_id: number
  label: string
  last_seen_date: string
  /** 最近动向摘要（最新 section 标题）。 */
  recent_hint: string
}

/** GET /lane-dynamics 响应 data。 */
export interface LaneDynamicsResponse {
  window_days: number
  /** 板块是否存在日报（tasks 2.2 空数据两态契约：无日报引导生成 vs 有日报无泳道文案）。 */
  has_reports: boolean
  lanes: LaneDynamicsLane[]
  candidates: LaneDynamicsCandidate[]
}

export function useLaneDynamicsApi() {
  /** 泳道动态聚合（只读）：GET /semantic-boards/:id/lane-dynamics?days=N。 */
  async function getLaneDynamics(boardId: number, days?: number): Promise<ApiResponse<LaneDynamicsResponse>> {
    const query = days ? apiClient.buildQueryParams({ days }) : ''
    return apiClient.get(`/semantic-boards/${boardId}/lane-dynamics${query ? `?${query}` : ''}`)
  }

  return {
    getLaneDynamics,
  }
}
