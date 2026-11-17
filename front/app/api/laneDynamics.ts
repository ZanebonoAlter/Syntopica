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
  /**
   * 该 section 的 thread 总数（**含**综合稿）——不是线索数。
   * 线索数 = thread_count − composite_count（两者都已知时）；旧响应缺任一 → 未知。
   */
  thread_count?: number
  /**
   * 该 section 内的综合稿（多事件稿）条数（fix-lane-metric-provenance）。
   * 缺省 = 旧后端 → 桶内线索数不可数（不拿 thread_count 冒充线索数）。
   */
  composite_count?: number
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
  /**
   * 当日线索合计（retire-story-adopt-event-metrics D4）：该泳道该日各主题桶
   * 非综合稿 thread 数之和（矩阵日格 / 演进节点 / 顶部曲线唯一量纲）。
   * 缺省 = 旧后端，前端回退各桶 thread_count（再回退 events+folded）合计。
   */
  clue_count?: number
}

/** 主卡区单条泳道（active ∪ watch 关联）。 */
export interface LaneDynamicsLane {
  topic_id: number
  label: string
  /** watch 关联（该板块 active 的 sentence_topic watch 命中该话题）。 */
  watch_linked: boolean
  section_count_14d: number
  /** 窗口内线索数（全窗线索合计；次级排序键）。旧响应缺省时前端按各桶合计兜底。 */
  thread_count_14d?: number
  /** 泳道当天热度（heat-v2-lane-clues-1d：当天线索数按话题状态加权）；缺省按 0。 */
  lane_heat?: number
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
  /** 锚点日（该版块自己的最新日报日，YYYY-MM-DD）；缺省 = 旧后端。 */
  anchor_date?: string
  /** 候选泳道锚点日的线索合计；缺省 = 旧后端（未知，不造 0）。 */
  clue_count?: number
  /** 候选泳道热度（clue_count × lane_heat_candidate_weight）；缺省按 0。 */
  lane_heat?: number
}

/** GET /lane-dynamics 响应 data。 */
export interface LaneDynamicsResponse {
  window_days: number
  /** 板块是否存在日报（tasks 2.2 空数据两态契约：无日报引导生成 vs 有日报无泳道文案）。 */
  has_reports: boolean
  /**
   * 窗口锚点日 = 板块**最新 completed 日报**日（YYYY-MM-DD）；缺省 = 旧后端
   * （fix-lane-metric-provenance：矩阵窗口与泳道工作面窗口必须同一天）。
   */
  anchor_date?: string
  lanes: LaneDynamicsLane[]
  candidates: LaneDynamicsCandidate[]
  /** 热度口径版本（heat-v2-lane-clues-1d）；缺省 = 旧后端。 */
  heat_rule_version?: string
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
