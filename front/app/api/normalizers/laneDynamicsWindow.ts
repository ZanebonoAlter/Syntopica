/**
 * 泳道动态观察窗 normalizer（rework-workshop-into-radar-and-desk 矩阵阶段 G1）。
 *
 * 后端 `GET /semantic-boards/:id/lane-dynamics?days=N` 的 snake_case 协议**完全不改**
 * （api/laneDynamics.ts 既有约定）；仅在新数据面（观察窗 A/B 双通道）客户端边界归一为
 * camelCase 读模型并把数字 ID 字符串化（code-style §9：转换只在 API 层，组件/状态层
 * 不散落 raw 映射）。未迁移的旧矩阵/演进/抽屉消费者继续消费 `legacy` 原始响应
 * （api/laneDynamicsWindow.ts 配对携带），G5 收口后再移除。
 *
 * 零值语义（矩阵阶段合同 §1）：
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

/** 归一后的单日节点：该日全部主题桶。 */
export interface WindowLaneDay {
  date: string
  sections: WindowSection[]
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
  snapshot: WindowLaneSnapshot | null
  timeline: WindowLaneDay[]
}

export interface WindowCandidate {
  topicId: string
  label: string
  lastSeenDate: string
  recentHint: string
}

/** 归一后的 lane-dynamics 读模型（新数据面消费）。 */
export interface LaneDynamicsWindowData {
  windowDays: number
  hasReports: boolean
  lanes: WindowLane[]
  candidates: WindowCandidate[]
  /** 窗内最新有分区日（= 数据轴锚点候选；与报告锚点分清，见 useBoardConsole anchorDate）。 */
  anchorDate: string | null
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
    })),
    anchorDate,
  }
}

/** 日格三口径派生（MX 组）：桶峰值 / 当日合计 / 主题桶数 + 是否存在未知值。 */
export interface DayCellMetrics {
  /** 同日各桶线索数最大值；null = 全部未知（显示 '?'，不拿桶数补数）。 */
  peak: number | null
  /** 当日各桶线索数之和（已知值合计，演进节点/趋势图口径）。 */
  total: number
  sectionCount: number
  hasUnknown: boolean
}

export function dayCellMetrics(day: WindowLaneDay | null | undefined): DayCellMetrics {
  if (!day || day.sections.length === 0) {
    return { peak: null, total: 0, sectionCount: 0, hasUnknown: false }
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
  return { peak, total, sectionCount: day.sections.length, hasUnknown }
}
