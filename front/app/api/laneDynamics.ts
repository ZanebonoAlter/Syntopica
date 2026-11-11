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
  /**
   * ≤500 字长版叙述全文（lane-trend-overview design §D3）。
   * undefined/null = 长版缺失（存量快照或生成失败降级），趋势区回退短版并明示提示；
   * 非空 = 长版全文，趋势区完整展示不截断。与 snapshot=null（快照整体缺失）两级可区分。
   */
  detail?: string | null
}

/** 时间线单日内的一个锚定 section（事件源）。 */
export interface LaneTimelineSection {
  section_id: number
  label: string
  /** 该 section 的线索总数（后端权威口径；旧响应缺省时用 events+folded 兜底）。 */
  thread_count?: number
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
  /** 当日事件线数（add-event-story-heat 3b；缺省 = 旧后端，前端回退非综合稿线程数合计）。 */
  story_count?: number
  /** 当日最热事件线的独立来源数；缺省按 0（颜色不深）。 */
  max_participant_count?: number
  /** 当日最热事件线的存活 id（surface-story-heat-in-report-top；与 max 同一获胜线）；
 *  null/缺省 = 无可识别身份（旧后端/未回填），展示不点名、不冒名。 */
  hottest_story_id?: number | null
  /** 当日最热事件线标题（同 hottest_story_id 获胜线）；缺省空串，空标题用「事件线 #ID」回退。 */
  hottest_story_title?: string
  /** 当日未归组到泳道的事件数；缺省按 0。 */
  unassigned_event_count?: number
}

/** 主卡区单条泳道（active ∪ watch 关联，按 section_count_14d DESC 排序）。 */
export interface LaneDynamicsLane {
  topic_id: number
  label: string
  /** watch 关联（该板块 active 的 sentence_topic watch 命中该话题）。 */
  watch_linked: boolean
  section_count_14d: number
  /** 窗口内线索数（热度口径；与 section 分桶解耦）。旧响应缺省时前端按 events+folded 兜底。 */
  thread_count_14d?: number
  /** 全窗事件线热度（半衰期求和，add-event-story-heat 3b）；缺省按 0（旧后端）。 */
  lane_heat?: number
  /** 全窗事件线数；缺省时前端按窗口内逐日 story_count（再回退非综合稿线程数）合计兜底。 */
  story_count_14d?: number
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
  /** 事件身份是否已回填（add-event-story-heat 3b）；缺省 = 旧后端（未知，不提示降级）。 */
  stories_backfilled?: boolean
  /** 热度公式版本（如 heat-v1-48h-hl24h）；缺省 = 旧后端。 */
  heat_rule_version?: string
  /** 落后源导致热度证据不全；缺省 = 旧后端（按 false 处理）。 */
  heat_incomplete?: boolean
  /** 落后 feed id 列表；缺省 = 旧后端。 */
  heat_stale_feed_ids?: number[]
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
