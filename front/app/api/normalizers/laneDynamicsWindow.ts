/**
 * 泳道动态观察窗 normalizer（rework-workshop-into-radar-and-desk 矩阵阶段 G1；
 * retire-story-adopt-event-metrics 1.4/3.1 换口径）。
 *
 * 后端 `GET /semantic-boards/:id/lane-dynamics?days=N` 的 snake_case 协议**完全不改**
 * （api/laneDynamics.ts 既有约定）；仅在新数据面（观察窗 A/B 双通道）客户端边界归一为
 * camelCase 读模型并把数字 ID 字符串化（code-style §9：转换只在 API 层，组件/状态层
 * 不散落 raw 映射）。
 *
 * 量纲（D4）：日格 / 演进节点 / 顶部曲线统一为「当日线索合计」= 该泳道该日各主题桶
 * 非综合稿 thread 数之和（后端 `clue_count`）。事件线量纲（story_count / 最热事件线
 * 身份 / 未归组事件数 / 来源数颜色通道）随 story 退役整体退场；旧后端缺 `clue_count`
 * 时回退各桶 `thread_count`（缺省再用 events+folded）合计，全未知仍是 null（不造 0）。
 *
 * 零值语义：
 *  - `thread_count` 缺省（旧响应）→ `threadCount: null`，展示回退 `fallbackClueCount`
 *    （= events 数 + folded_count）；连 events/folded 都缺 → `fallbackClueCount: null`
 *    （未知，显示 '?'，不得拿桶数补数）。
 *  - `snapshot: null` = 待结算（合法态，不影响 timeline）。
 *  - 窗内零分区的日期后端不返回 → timeline 无该日 = 断档（'gap'），与「历史未取到」
 *    （'unavailable'，池未加载/失败）由 useObservationWindow 的 resolveLaneDay 区分。
 */
import type { LaneDynamicsResponse, LaneTimelineSection } from '../laneDynamics'

/** 归一后的单日主题桶（section）。 */
export interface WindowSection {
  sectionId: string
  label: string
  /** 桶线索数（raw thread_count）；null = 该桶线索数未知。 */
  threadCount: number | null
  /** threadCount 未知时的展示回退（events.length + folded_count）；null = 无法推导。 */
  fallbackClueCount: number | null
  foldedCount: number
  eventTitles: string[]
}

/** 归一后的单日节点：该日全部主题桶 + 当日线索合计。 */
export interface WindowLaneDay {
  date: string
  sections: WindowSection[]
  /** 当日线索合计（raw clue_count，已排除综合稿）；null/缺省 = 旧响应（按各桶合计派生）。 */
  clueCount?: number | null
}

export interface WindowLaneSnapshot {
  summary: string
  /** 汇总截止日（raw as_of，YYYY-MM-DD）。 */
  asOf: string
  /** 长版叙述；null = 长版缺失（与快照整体缺失两级可区分）。 */
  detail: string | null
}

/** 归一后的泳道（A 档响应或 B 池响应中的成员）。 */
export interface WindowLane {
  topicId: string
  label: string
  watchLinked: boolean
  /** 窗口内分区数（raw section_count_14d；字段名带 14d 但随 days 档变化）。 */
  sectionCountWindow: number
  /** 窗口内线索数（raw thread_count_14d）；null = 旧响应缺省。 */
  threadCountWindow: number | null
  /** 泳道当天热度（raw lane_heat，heat-v2-lane-clues-1d 当天口径）；缺省按 0。 */
  laneHeat?: number
  snapshot: WindowLaneSnapshot | null
  timeline: WindowLaneDay[]
}

export interface WindowCandidate {
  topicId: string
  label: string
  lastSeenDate: string
  recentHint: string
  /** 候选泳道的锚点日（raw anchor_date）；缺省 null。 */
  anchorDate: string | null
  /** 锚点日当天线索合计（raw clue_count）；缺省 null（未知，不造 0）。 */
  clueCount: number | null
  /** 候选加权热度（raw lane_heat）；缺省 null。 */
  laneHeat: number | null
}

/** 归一后的 lane-dynamics 读模型（新数据面消费）。 */
export interface LaneDynamicsWindowData {
  windowDays: number
  hasReports: boolean
  lanes: WindowLane[]
  candidates: WindowCandidate[]
  /** 窗内最新有分区日（= 数据轴锚点候选；与报告锚点分清，见 useBoardConsole anchorDate）。 */
  anchorDate: string | null
  /** 热度口径版本（heat-v2-lane-clues-1d）；缺省空串。 */
  heatRuleVersion?: string
}

function normalizeSection(section: LaneTimelineSection): WindowSection {
  const events = section.events ?? []
  const foldedCount = section.folded_count ?? 0
  const canFallback = section.events != null || section.folded_count != null
  return {
    sectionId: String(section.section_id),
    label: section.label,
    threadCount: section.thread_count ?? null,
    fallbackClueCount: canFallback ? events.length + foldedCount : null,
    foldedCount,
    eventTitles: events,
  }
}

export function normalizeLaneDynamicsWindow(raw: LaneDynamicsResponse): LaneDynamicsWindowData {
  let anchorDate: string | null = null
  const lanes: WindowLane[] = (raw.lanes ?? []).map(lane => {
    const timeline = (lane.timeline ?? []).map(day => ({
      date: day.date,
      sections: (day.sections ?? []).map(normalizeSection),
      clueCount: day.clue_count ?? null,
    }))
    for (const day of timeline) {
      if (anchorDate === null || day.date > anchorDate) anchorDate = day.date
    }
    return {
      topicId: String(lane.topic_id),
      label: lane.label,
      watchLinked: lane.watch_linked,
      sectionCountWindow: lane.section_count_14d,
      threadCountWindow: lane.thread_count_14d ?? null,
      laneHeat: lane.lane_heat ?? 0,
      snapshot: lane.snapshot
        ? { summary: lane.snapshot.summary, asOf: lane.snapshot.as_of, detail: lane.snapshot.detail ?? null }
        : null,
      timeline,
    }
  })
  return {
    windowDays: raw.window_days,
    hasReports: raw.has_reports,
    lanes,
    candidates: (raw.candidates ?? []).map(candidate => ({
      topicId: String(candidate.topic_id),
      label: candidate.label,
      lastSeenDate: candidate.last_seen_date,
      recentHint: candidate.recent_hint,
      anchorDate: candidate.anchor_date ?? null,
      clueCount: candidate.clue_count ?? null,
      laneHeat: candidate.lane_heat ?? null,
    })),
    anchorDate,
    heatRuleVersion: raw.heat_rule_version ?? '',
  }
}

/**
 * 日格派生（D4）：展示值 = 当日线索合计（raw clue_count 优先；旧响应回退各桶
 * 已知线索合计）。峰值只留作 title/aria 的辅助说明，不再作展示值；事件线数、
 * 最热事件线身份、未归组事件数、来源数分档全部退役。
 */
export interface DayCellMetrics {
  /** 展示值：当日线索合计；null = 全部桶未知（显示 '?'，不造 0）。 */
  clueCount: number | null
  /** 各桶已知线索数合计（clue_count 缺省时的回退源，同时是 aria 的辅助数）。 */
  total: number
  /** 同日各桶线索数最大值；null = 全部未知。 */
  peak: number | null
  sectionCount: number
  hasUnknown: boolean
}

export function dayCellMetrics(day: WindowLaneDay | null | undefined): DayCellMetrics {
  if (!day || day.sections.length === 0) {
    return { clueCount: 0, total: 0, peak: null, sectionCount: 0, hasUnknown: false }
  }
  let peak: number | null = null
  let total = 0
  let hasUnknown = false
  for (const section of day.sections) {
    const value = section.threadCount ?? section.fallbackClueCount
    if (value === null) {
      hasUnknown = true
      continue
    }
    total += value
    if (peak === null || value > peak) peak = value
  }
  // 后端 clue_count 是唯一权威的「当日线索合计」（已排除综合稿）；旧响应缺省时
  // 回退各桶已知合计：全未知 → null（不造 0），否则用已知部分求和。
  const clueCount = day.clueCount ?? (hasUnknown && peak === null ? null : total)
  return { clueCount, total, peak, sectionCount: day.sections.length, hasUnknown }
}
