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
  /** 14 天逐日事件线数（add-event-story-heat 3b）；缺省 = 旧后端（mini-matrix 回退 counts）。 */
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
  today_sections: number
  last3_sections: number
  prev3_sections: number
  attention: number
  data_delayed: boolean
  days: number[]
  lanes: BoardRadarRawLane[]
  /** 当日/近3日/前3日事件线数（add-event-story-heat 3b）；缺省 = 旧后端（回退 sections 口径）。 */
  today_stories?: number
  last3_stories?: number
  prev3_stories?: number
  /** 当日/近3日/前3日事件线热度合计；缺省 = 旧后端（回退 sections 口径）。 */
  today_heat?: number
  last3_heat?: number
  prev3_heat?: number
  /** 14 天逐日热度（供后续曲线消费；本批仅透传）。 */
  heat_days?: number[]
  heat_rule_version?: string
  heat_incomplete?: boolean
  /** 窗口内是否有 thread 携带 story_id（回填标记，与 lane-dynamics 同源）；false ⇒ *_stories 为非综合稿事件数上界；缺省 = 旧后端（未知）。 */
  stories_backfilled?: boolean
}

export interface BoardRadarRawOverview {
  date: string
  boards: BoardRadarRawBoard[]
  /**
   * 事件线清单段（surface-story-events-in-radar）：跨版块归并后的今日该看 /
   * 升温 / 新起三档 + 未回填 / 滞后 / 降级标记。缺省 = 旧后端（客户端回退
   * 版块级日报标题列表与版块级升温清单，不渲染成业务空态）。
   */
  stories?: BoardRadarRawStoryFeed
}

/** 后端原始事件线条目（snake_case wire 契约，勿在前端消费）。 */
export interface BoardRadarRawStoryItem {
  board_id: number
  board_label: string
  story_id: number
  story_title: string
  /** 锚点日日终 48h 窗内去重来源数；缺字段 → null（前端省略数字，不伪造 0）。 */
  participants?: number
  heat: number
  baseline_heat: number
  ratio: number
  has_baseline: boolean
  has_today_evidence: boolean
  /** 该条目所属版块自己的锚点日（滞后版块 ≠ 全局日）。 */
  anchor_date: string
  /** 落点报告日期；空串 = 无正文落点（条目不可点）。 */
  report_date: string
}

/** 后端原始事件线清单段。 */
export interface BoardRadarRawStoryFeed {
  anchor_date: string
  heat_rule_version: string
  top_stories: BoardRadarRawStoryItem[]
  /** 升温事件线**全量**计数（非截断数）——报头「N 处出现升温」用它。
   *  独立 warming 清单数组已随右栏徽标化移除（matrix-clue-fallback-and-rail-merge）。 */
  warming_count: number
  /** 既有锚点日之前 3 日基线窗口内热度证据、又有锚点日当天证据的事件线条数（全部候选，不受截断）；
   *  0 = 无跨天延续事件线（右栏脚注出现）。旧后端无此字段 → 归一为 null（区分「0」与「无法判定」）。 */
  cross_day_story_count?: number
  unbackfilled_board_ids: number[]
  unbackfilled_board_count: number
  /** 无可回填对象的版块 id（本期没有任何事件身份可回填——不计入未回填数，也不产生条目）。 */
  no_story_board_ids?: number[]
  no_story_board_count?: number
  lagging_board_ids: number[]
  /** true = 部分版块降级，本次排序不完整。 */
  partial: boolean
  degraded_board_ids: number[]

  /**
   * 退役字段（旧后端仍可能下发，前端不再消费——留着只为响应类型兼容）：
   * `warming`/`new_stories` 清单与 `new_count`/`new_total_count` 去 Top 计数
   * 随单清单徽标化移除（matrix-clue-fallback-and-rail-merge D4）。
   */
  warming?: BoardRadarRawStoryItem[]
  new_stories?: BoardRadarRawStoryItem[]
  new_count?: number
  new_total_count?: number
}

/** 归一化后的客户端读模型（camelCase + 字符串 ID，组件只消费此形态）。 */
export interface RadarLane {
  topicId: string
  label: string
  sectionCount14d: number
  threadCount14d: number
  counts: number[]
  /** 14 天逐日事件线数（raw story_counts）；raw 缺省时回退 counts 兜底（mini-matrix 口径）。
   *  注：计数为 0 时前端逐格按 counts 降级（见 BoardRadarStory laneCells）。
   *  `storyCountsKnown=false` 表示旧后端未下发 story_counts——此时回退的是**分区数**，
   *  展示必须按「N 个分区」措辞，不得写成条事件线（matrix-clue R-2）。 */
  storyCounts: number[]
  storyCountsKnown: boolean
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
  /** 当日/近3日/前3日事件线数；raw 缺省按 0（消费方按 hasHeatData 回退 sections 口径）。 */
  todayStories: number
  last3Stories: number
  prev3Stories: number
  /** 事件线窗口是否已回填：false ⇒ today/last3/prev3Stories 为非综合稿事件数上界（「今日该读」须标注降级）；null = 老 payload 缺字段（未知，回落日报事件数）。 */
  storiesBackfilled: boolean | null
  /** 热度字段完整可用；区分真实全零热度与旧响应缺字段。 */
  hasHeatData?: boolean
  /** 当日/近3日/前3日事件线热度合计；raw 缺省按 0，是否可用由 hasHeatData 判定。 */
  todayHeat: number
  last3Heat: number
  prev3Heat: number
  /** 14 天逐日热度（透传，缺省空数组）。 */
  heatDays: number[]
  heatRuleVersion: string
  heatIncomplete: boolean
}

export interface RadarOverview {
  date: string
  boards: RadarBoard[]
  /** 事件线清单段；null = 响应没有该段（旧后端）→ 回退版块级列表。 */
  stories: RadarStoryFeed | null
}

/** 归一化后的事件线条目（camelCase + 字符串 ID）。 */
export interface RadarStoryItem {
  boardId: string
  boardLabel: string
  storyId: string
  /** 事件线标题（可能为空串——前端回退「事件线 #ID」，身份不用标题）。 */
  storyTitle: string
  /** 来源数；null = 字段缺失（省略数字，不伪造 0）。 */
  participants: number | null
  heat: number
  baselineHeat: number
  ratio: number
  hasBaseline: boolean
  hasTodayEvidence: boolean
  anchorDate: string
  /** 落点报告日期；空串 = 无正文落点（条目不可点）。 */
  reportDate: string
}

/** 归一化后的事件线清单段（单清单 + 行内徽标；matrix-clue-fallback-and-rail-merge D4）。 */
export interface RadarStoryFeed {
  anchorDate: string
  heatRuleVersion: string
  /** 「今日该读」单清单（服务端已截断到上限 8；条目自带徽标判定字段）。 */
  topStories: RadarStoryItem[]
  /** 升温事件线全量计数（报头消费）。 */
  warmingCount: number
  /** 既有锚点日之前 3 日基线窗口内证据、又有锚点日当天证据的事件线条数；
   *  null = 旧响应缺该字段，无法判定（不得当成 0 断言）。0 = 无跨天线（脚注出现）。 */
  crossDayStoryCount: number | null
  unbackfilledBoardIds: string[]
  unbackfilledBoardCount: number
  /** 无可回填对象的版块 id（不计入未回填数；客户端不展示，只作诊断）。 */
  noStoryBoardIds: string[]
  noStoryBoardCount: number
  laggingBoardIds: string[]
  partial: boolean
  degradedBoardIds: string[]
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
  const counts = Array.isArray(lane?.counts) ? lane.counts : []
  return {
    topicId: id(lane?.topic_id),
    label: str(lane?.label),
    sectionCount14d: num(lane?.section_count_14d),
    threadCount14d: num(lane?.thread_count_14d),
    counts,
    // story_counts 缺省（旧后端）→ counts 兜底（同为逐日计数口径；不拿桶数冒充）；
    // storyCountsKnown 记住「这是分区数不是事件线数」，展示层据此改用分区措辞。
    storyCounts: Array.isArray(lane?.story_counts) ? lane.story_counts : counts,
    storyCountsKnown: Array.isArray(lane?.story_counts),
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
    todayStories: num(board?.today_stories),
    last3Stories: num(board?.last3_stories),
    prev3Stories: num(board?.prev3_stories),
    storiesBackfilled: typeof board?.stories_backfilled === 'boolean' ? board.stories_backfilled : null,
    hasHeatData: [board?.today_heat, board?.last3_heat, board?.prev3_heat, board?.today_stories].every(value => typeof value === 'number' && Number.isFinite(value)),
    todayHeat: num(board?.today_heat),
    last3Heat: num(board?.last3_heat),
    prev3Heat: num(board?.prev3_heat),
    heatDays: Array.isArray(board?.heat_days) ? board.heat_days : [],
    heatRuleVersion: str(board?.heat_rule_version),
    heatIncomplete: board?.heat_incomplete === true,
  }
}

export function normalizeRadarStoryItem(item: BoardRadarRawStoryItem): RadarStoryItem {
  return {
    boardId: id(item?.board_id),
    boardLabel: str(item?.board_label),
    storyId: id(item?.story_id),
    storyTitle: str(item?.story_title),
    participants: typeof item?.participants === 'number' && Number.isFinite(item.participants) ? item.participants : null,
    heat: num(item?.heat),
    baselineHeat: num(item?.baseline_heat),
    ratio: num(item?.ratio),
    hasBaseline: item?.has_baseline === true,
    hasTodayEvidence: item?.has_today_evidence === true,
    anchorDate: str(item?.anchor_date),
    reportDate: str(item?.report_date),
  }
}

/** 事件线清单段归一：缺字段安全默认（旧响应无该段时返回空清单而非报错）。
 *  退役字段（warming/new_stories/new_count/new_total_count）不读——旧后端下发了也忽略。 */
export function normalizeRadarStoryFeed(raw?: BoardRadarRawStoryFeed): RadarStoryFeed {
  return {
    anchorDate: str(raw?.anchor_date),
    heatRuleVersion: str(raw?.heat_rule_version),
    topStories: Array.isArray(raw?.top_stories) ? raw.top_stories.map(normalizeRadarStoryItem) : [],
    warmingCount: num(raw?.warming_count),
    // 0 是有效值（= 没有跨天线），缺失才是「无法判定」——两者语义不同，
    // 归一成同一个值会让新前端 + 旧后端时断言一句没依据的话。
    crossDayStoryCount:
      typeof raw?.cross_day_story_count === 'number' && Number.isFinite(raw.cross_day_story_count)
        ? raw.cross_day_story_count
        : null,
    unbackfilledBoardIds: Array.isArray(raw?.unbackfilled_board_ids) ? raw.unbackfilled_board_ids.map(id) : [],
    unbackfilledBoardCount: num(raw?.unbackfilled_board_count),
    noStoryBoardIds: Array.isArray(raw?.no_story_board_ids) ? raw.no_story_board_ids.map(id) : [],
    noStoryBoardCount: num(raw?.no_story_board_count),
    laggingBoardIds: Array.isArray(raw?.lagging_board_ids) ? raw.lagging_board_ids.map(id) : [],
    partial: raw?.partial === true,
    degradedBoardIds: Array.isArray(raw?.degraded_board_ids) ? raw.degraded_board_ids.map(id) : [],
  }
}

export function normalizeRadarOverview(raw: BoardRadarRawOverview): RadarOverview {
  return {
    date: str(raw?.date),
    boards: Array.isArray(raw?.boards) ? raw.boards.map(normalizeRadarBoard) : [],
    // 该段缺席 = 旧后端：交回 null 让调用方回退版块级列表（旧响应 ≠ 没有事件）。
    stories: raw?.stories ? normalizeRadarStoryFeed(raw.stories) : null,
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
