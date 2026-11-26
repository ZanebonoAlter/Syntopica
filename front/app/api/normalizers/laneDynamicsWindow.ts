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
 * 身份 / 未归组事件数 / 来源数颜色通道）随 story 退役整体退场。
 *
 * 桶线索数的唯一公式（fix-lane-metric-provenance）：**桶线索数 = thread_count −
 * composite_count**（综合稿不计入线索数）。任一字段缺失 → 该桶线索数未知（null），
 * 不得拿含综合稿的 thread_count 或 events+folded 冒充线索数。旧后端缺
 * `composite_count` 时各桶线索数不可数 → 当日合计显示 `?`（诚实降级，不造 0）。
 *
 * 零值语义：
 *  - `thread_count` / `composite_count` 缺省（旧响应）→ 桶线索数 null（未知）。
 *  - `snapshot: null` = 待结算（合法态，不影响 timeline）。
 *  - 窗内零分区的日期后端不返回 → timeline 无该日 = 断档（'gap'），与「历史未取到」
 *    （'unavailable'，池未加载/失败）由 useObservationWindow 的 resolveLaneDay 区分。
 */
import type { LaneDynamicsResponse, LaneTimelineSection } from '../laneDynamics'

/**
 * 桶线索数（唯一公式）：`thread_count − composite_count`；任一未知 → null。
 *
 * 综合稿（thread 的 event 标签数 ≥ 2）不计入线索数，因此**不得**直接拿
 * `thread_count` 当线索数（那是含综合稿的 thread 总数）。
 */
export function laneSectionClueCount(section: LaneTimelineSection): number | null {
  if (typeof section.thread_count !== 'number' || !Number.isFinite(section.thread_count)) return null
  if (typeof section.composite_count !== 'number' || !Number.isFinite(section.composite_count)) return null
  const clue = section.thread_count - section.composite_count
  // 差值为负 = 后端统计异常（综合稿数 > thread 总数）：按「不可数」处理，不抹成合法的 0
  // （fix-lane-metric-provenance review R-6）。
  if (clue < 0) return null
  return clue
}

/** 归一后的单日主题桶（section）。 */
export interface WindowSection {
  sectionId: string
  label: string
  /** 桶内 thread 总数（**含**综合稿）；null = 未知。 */
  threadCount: number | null
  /** 桶内综合稿数（raw composite_count）；null = 未知（旧后端）。 */
  compositeCount: number | null
  /** 桶线索数 = threadCount − compositeCount；任一未知 → null（不可数）。 */
  clueCount: number | null
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

/**
 * 归一后的「其他话题（未入泳道）」逐日读数（`lane-matrix-coverage-and-others-row`）。
 *
 * 口径与泳道日格同量纲：`clues` = 当日未入泳道分区的**非综合稿线索数**；
 * `sections` = 当日未入泳道分区数（title/aria 用）。
 */
export interface WindowOtherTopicDay {
  date: string
  sections: number
  clues: number
}

/** 归一后的 lane-dynamics 读模型（新数据面消费）。 */
export interface LaneDynamicsWindowData {
  windowDays: number
  hasReports: boolean
  lanes: WindowLane[]
  candidates: WindowCandidate[]
  /**
   * 窗内最新有分区日（= 数据轴锚点候选；与报告锚点分清，见 useBoardConsole anchorDate）。
   * 后端显式下发 `anchor_date`（最新 completed 日报日）时以它为准——
   * 矩阵窗口末日与泳道工作面窗口末日必须同一天（fix-lane-metric-provenance）。
   */
  anchorDate: string | null
  /** 热度口径版本（heat-v2-lane-clues-1d）；缺省空串。 */
  heatRuleVersion?: string
  /**
   * 「其他话题（未入泳道）」逐日读数（date → 当日未入泳道分区数与非综合稿线索数）。
   * 空对象 = 该响应没有该字段（旧后端）；配合 `otherTopicsAvailable` 区分
   * 「旧后端（无字段）」与「有字段但窗口内没有其他话题（空）」两种态。
   */
  otherTopics: Record<string, WindowOtherTopicDay>
  /** 后端是否下发 `other_topics` 字段：false = 旧后端（矩阵整行不渲染）。 */
  otherTopicsAvailable: boolean
}

function normalizeSection(section: LaneTimelineSection): WindowSection {
  const events = section.events ?? []
  const foldedCount = section.folded_count ?? 0
  return {
    sectionId: String(section.section_id),
    label: section.label,
    threadCount: section.thread_count ?? null,
    compositeCount: section.composite_count ?? null,
    clueCount: laneSectionClueCount(section),
    foldedCount,
    eventTitles: events,
  }
}

/**
 * 「其他话题（未入泳道）」逐日条目归一（`lane-matrix-coverage-and-others-row`）。
 *
 * - 字段缺省（旧后端）→ available=false（前端整行不渲染，不显示全「—」空壳行）；
 * - 空数组 → available=true + 空对象（「有字段但窗口内无其他话题」，与旧后端可区分）；
 * - 数字不合法（非有限数 / 负数）→ **跳过该条**，不拿 0 冒充（不造数）。
 */
function normalizeOtherTopics(raw: LaneDynamicsResponse): {
  days: Record<string, WindowOtherTopicDay>
  available: boolean
} {
  const list = raw.other_topics
  if (!Array.isArray(list)) return { days: {}, available: false }
  const days: Record<string, WindowOtherTopicDay> = {}
  for (const item of list) {
    if (!item || typeof item.date !== 'string' || item.date.trim() === '') continue
    if (!Number.isFinite(item.sections) || !Number.isFinite(item.clues)) continue
    if (item.sections < 0 || item.clues < 0) continue
    days[item.date] = { date: item.date, sections: item.sections, clues: item.clues }
  }
  return { days, available: true }
}

export function normalizeLaneDynamicsWindow(raw: LaneDynamicsResponse): LaneDynamicsWindowData {
  let latestDay: string | null = null
  const lanes: WindowLane[] = (raw.lanes ?? []).map(lane => {
    const timeline = (lane.timeline ?? []).map(day => ({
      date: day.date,
      sections: (day.sections ?? []).map(normalizeSection),
      clueCount: day.clue_count ?? null,
    }))
    for (const day of timeline) {
      if (latestDay === null || day.date > latestDay) latestDay = day.date
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
  const otherTopics = normalizeOtherTopics(raw)
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
    // 后端锚点（最新 completed 日报日）优先；旧后端缺该字段时回退数据最新日。
    anchorDate: raw.anchor_date ?? latestDay,
    heatRuleVersion: raw.heat_rule_version ?? '',
    otherTopics: otherTopics.days,
    otherTopicsAvailable: otherTopics.available,
  }
}

/**
 * 日格派生（D4）：展示值 = 当日线索合计（raw clue_count 优先；旧响应回退各桶
 * 已知**非综合稿**线索合计）。桶峰值只作辅助说明（已是同口径）；事件线数、
 * 最热事件线身份、未归组事件数、来源数分档全部退役。
 */
export interface DayCellMetrics {
  /** 展示值：当日线索合计；null = 全部桶未知（显示 '?'，不造 0）。 */
  clueCount: number | null
  /** 当日合计（同口径）；未知时为已知部分之和（配合 hasUnknown 表达「≥N」）。 */
  total: number
  /** 同日各桶线索数最大值；null = 全部未知。 */
  peak: number | null
  sectionCount: number
  hasUnknown: boolean
  /** 当日综合稿条数（各桶之和）；null = 未知（某桶 composite_count 缺失）。 */
  compositeCount: number | null
  /** 各已知桶的非综合稿线索数（按桶序；供 title/aria 的分桶明细）。 */
  bucketClues: number[]
  /**
   * 当日线索合计的来源：
   *  - `backend` = 后端 `clue_count`（权威完整值，即使各桶明细有未知也照常用）；
   *  - `buckets` = 旧响应回退各桶合计（`hasUnknown` 时为下界，展示应写 `≥N`）；
   *  - `none` = 无法成立（显示 `?`）。
   */
  clueCountSource: 'backend' | 'buckets' | 'none'
  /**
   * `clueCount` 是否是**下界**（旧响应回退各桶合计且有桶不可数）。为 true 时展示层
   * SHALL 写 `≥N`，MUST NOT 把已知部分之和当成完整合计（review R-1）。
   */
  isLowerBound: boolean
}

export function dayCellMetrics(day: WindowLaneDay | null | undefined): DayCellMetrics {
  if (!day || day.sections.length === 0) {
    return {
      clueCount: 0, total: 0, peak: null, sectionCount: 0,
      hasUnknown: false, compositeCount: 0, bucketClues: [], clueCountSource: 'backend', isLowerBound: false,
    }
  }
  let peak: number | null = null
  let knownTotal = 0
  let composite = 0
  let compositeKnown = true
  let hasUnknown = false
  const bucketClues: number[] = []
  for (const section of day.sections) {
    const value = section.clueCount
    if (value === null) {
      hasUnknown = true
      compositeKnown = false
      continue
    }
    bucketClues.push(value)
    knownTotal += value
    if (peak === null || value > peak) peak = value
    if (section.compositeCount === null) compositeKnown = false
    else composite += section.compositeCount
  }
  // 后端 clue_count 是唯一权威的「当日线索合计」（已排除综合稿）；缺省时回退
  // 各桶已知合计（同一公式派生，不含综合稿）：全未知 → null（不造 0）。
  const backendCount = typeof day.clueCount === 'number' && Number.isFinite(day.clueCount)
    ? day.clueCount
    : null
  const derived = hasUnknown && peak === null ? null : knownTotal
  const clueCount = backendCount ?? derived
  const clueCountSource: DayCellMetrics['clueCountSource'] = backendCount !== null
    ? 'backend'
    : (derived === null ? 'none' : 'buckets')
  return {
    clueCount,
    total: clueCount ?? knownTotal,
    peak,
    sectionCount: day.sections.length,
    hasUnknown,
    compositeCount: compositeKnown ? composite : null,
    bucketClues,
    clueCountSource,
    isLowerBound: clueCountSource === 'buckets' && hasUnknown,
  }
}
