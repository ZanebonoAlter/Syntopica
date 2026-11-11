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
  /** 当日事件线数（raw story_count）；null = 旧响应缺省（展示回退当日非综合稿线程数合计）。 */
  storyCount?: number | null
  /** 当日最热事件线独立来源数（raw max_participant_count）；null = 缺省（按 0，颜色不深）。 */
  maxParticipantCount?: number | null
  /** 当日最热事件线存活 id（raw hottest_story_id，与 max 同一获胜线）；null = 缺省/无身份（不点名）。 */
  hottestStoryId?: number | null
  /** 当日最热事件线标题（raw hottest_story_title）；缺省空串（点名时空标题回退 #ID）。 */
  hottestStoryTitle?: string
  /** 当日未归组事件数（raw unassigned_event_count）；null = 缺省（按 0）。 */
  unassignedEventCount?: number | null
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
  /** 全窗事件线热度（raw lane_heat，半衰期求和）；缺省/旧后端按 0（排序回退事件线数）。 */
  laneHeat?: number
  /** 全窗事件线数（raw story_count_14d）；null/缺省 = 旧响应（回退逐日 story_count 合计）。 */
  storyCountWindow?: number | null
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
  /** 事件身份是否已回填（raw stories_backfilled）；null/缺省 = 旧响应（未知，不提示降级）。 */
  storiesBackfilled?: boolean | null
  /** 热度公式版本；缺省空串。 */
  heatRuleVersion?: string
  /** 落后源热度证据不全；缺省按 false。 */
  heatIncomplete?: boolean
  /** 落后 feed id 列表；缺省空数组。 */
  heatStaleFeedIds?: number[]
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
      storyCount: day.story_count ?? null,
      maxParticipantCount: day.max_participant_count ?? null,
      hottestStoryId: day.hottest_story_id ?? null,
      hottestStoryTitle: day.hottest_story_title ?? '',
      unassignedEventCount: day.unassigned_event_count ?? null,
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
      storyCountWindow: lane.story_count_14d ?? null,
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
    storiesBackfilled: raw.stories_backfilled ?? null,
    heatRuleVersion: raw.heat_rule_version ?? '',
    heatIncomplete: raw.heat_incomplete === true,
    heatStaleFeedIds: Array.isArray(raw.heat_stale_feed_ids) ? raw.heat_stale_feed_ids : [],
  }
}

/** 日格多口径派生（MX 组）：事件线数 / 桶峰值 / 当日合计 / 主题桶数 + 是否存在未知值。 */
export interface DayCellMetrics {
  /** 当日事件线数（raw story_count；缺省回退当日非综合稿线程数合计，不用桶数冒充）。 */
  storyCount: number | null
  /** 展示降级口径（matrix-clue-fallback-and-rail-merge D2）：矩阵格值与顶部时间视窗曲线共用
   *  ——事件线数 > 0 用事件线数，否则线索合计兜底；null = 事件线数未知且合计不可得（'?'，不造 0）。 */
  displayCount: number | null
  /** displayCount 是否走了线索合计兜底（事件线 0 且线索 > 0）：title/aria 降级标注的数据源。 */
  degradedFromClue: boolean
  /** 当日最热事件线独立来源数（颜色分档口径）；缺省 0。 */
  maxParticipantCount: number
  /** 当日最热事件线存活 id（surface-story-heat-in-report-top 点名用）；null = 无身份/旧响应，不点名。 */
  hottestStoryId: number | null
  /** 当日最热事件线标题；缺省空串（matrixCellView 空标题回退「事件线 #ID」）。 */
  hottestStoryTitle: string
  /** 当日未归组事件数；缺省 0。 */
  unassignedEventCount: number
  /** 同日各桶线索数最大值；null = 全部未知（显示 '?'，不拿桶数补数）。 */
  peak: number | null
  /** 当日各桶线索数之和（已知值合计，演进节点/趋势图口径）。 */
  total: number
  sectionCount: number
  hasUnknown: boolean
}

export function dayCellMetrics(day: WindowLaneDay | null | undefined): DayCellMetrics {
  if (!day || day.sections.length === 0) {
    return { storyCount: 0, displayCount: 0, degradedFromClue: false, maxParticipantCount: 0, hottestStoryId: null, hottestStoryTitle: '', unassignedEventCount: 0, peak: null, total: 0, sectionCount: 0, hasUnknown: false }
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
  // story_count 缺省（旧后端/未回填）→ 回退非综合稿线程数合计（thread_count ?? events+folded）
  const storyCount = day.storyCount ?? (peak === null ? null : total)
  // 展示降级口径（D2）：事件线数 > 0 用事件线数；事件线 0（历史未回填日）而线索 > 0
  // 时按线索合计兜底（真零事件 0 是合法已知值）；未知维持 null 不造 0——事件线 0
  // 且**线索全未知**时无法区分「真零」与「未回填空白」，归未知（hasUnknown 同时标出）。
  const displayCount = storyCount === null
    ? null
    : storyCount > 0
      ? storyCount
      : (total > 0 ? total : (hasUnknown ? null : 0))
  const degradedFromClue = storyCount === 0 && total > 0
  return {
    storyCount,
    displayCount,
    degradedFromClue,
    maxParticipantCount: day.maxParticipantCount ?? 0,
    hottestStoryId: day.hottestStoryId ?? null,
    hottestStoryTitle: day.hottestStoryTitle ?? '',
    unassignedEventCount: day.unassignedEventCount ?? 0,
    peak,
    total,
    sectionCount: day.sections.length,
    hasUnknown,
  }
}
