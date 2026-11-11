/**
 * 观察矩阵/泳道演进纯模型（rework-workshop-into-radar-and-desk 矩阵阶段 G3）。
 *
 * 职责拆两半（阶段合同 §2.1 三轴分工）：
 *  - **source 统计**（buildObservationRows）：名单/行序/休眠唯一基准 = A 档响应自身全轴
 *    （lane_heat 降序 → 事件线数降序 → topicId 升序，add-event-story-heat 5.4 与后端一致；
 *    休眠分组仍用 sectionTotal 阈值）。
 *    与观察轴/365 池无关——观察裁切只作用于下方日投影，永不回流重算本表。
 *  - **观察日投影**（projectMatrixCell/projectEvolutionNodes）：给定可见轴逐日把
 *    G1 `resolveLaneDay(topicId, date)` 的三态翻译成展示格/演进节点——
 *    content=有分区（已知 0 线索合法、可打开）；gap=请求覆盖但无分区（曲线值 0 但格/节点不可开）；
 *    unavailable=历史未取到（'?' / null，不补 0、不假定无日报）。
 *
 * ID 契约：直接消费 API 边界归一后的 `sourceWindow.lanes`（topicId 已 string、camelCase），
 * 组件/本模型不再出现 raw snake_case 或散落 String(id) 映射。
 */
import {
  dayCellMetrics,
  type DayCellMetrics,
  type LaneDynamicsWindowData,
  type WindowLaneDay,
  type WindowLaneSnapshot,
} from '~/api/normalizers/laneDynamicsWindow'
import { DORMANT_SECTION_THRESHOLD } from '~/features/tags/composables/laneMatrixModel'
import type { LaneDayResolution } from '~/features/tags/composables/useObservationWindow'

/** 再导出 G1 分辨结果类型（定义仍在 `useObservationWindow`），下游/测试从本模块引用不用再抄路径。 */
export type { LaneDayResolution }

/** G1 resolveLaneDay 的受控注入形态（组件只调用、不自建索引/网络状态）。 */
export type LaneDayResolver = (topicId: string, date: string) => LaneDayResolution

/** source 档全轴行模型：排序键与休眠分组都在这里，观察裁切不参与。 */
export interface ObservationLaneRow {
  topicId: string
  label: string
  watchLinked: boolean
  /** source 全轴分区总数（休眠分组 + 兼容保留；不再参与行序）。 */
  sectionTotal: number
  /** source 全轴线索总数（thread 口径；兼容保留）。 */
  clueTotal: number
  /** 全窗事件线热度（lane_heat）；缺省 0（行序主键，与后端一致；buildObservationRows 恒填，测试字面量可省）。 */
  laneHeat?: number
  /** source 全轴事件线总数（story_count；缺省回退逐日非综合稿线程数合计；行序次键）。 */
  storyTotal?: number
  /** date → 当日事件线数。 */
  storyCounts?: Record<string, number>
  /** date → 当日最热事件线独立来源数；缺省 0。 */
  participantCounts?: Record<string, number>
  /** date → 当日未归组事件数；缺省 0。 */
  unassignedCounts?: Record<string, number>
  /** source 全窗分区 ≤ DORMANT_SECTION_THRESHOLD（含等号）→ 休眠展示分组（不写任何状态）。 */
  dormant: boolean
  snapshot: WindowLaneSnapshot | null
}

/** 字符串 topicId 的升序比较：数字串按数值（沿用 legacy 数字 ID 升序语义），非数字回退字典序。 */
export function compareTopicId(a: string, b: string): number {
  const na = Number(a)
  const nb = Number(b)
  if (Number.isFinite(na) && Number.isFinite(nb)) {
    if (na !== nb) return na - nb
    return 0
  }
  return a < b ? -1 : a > b ? 1 : 0
}

/** 名单/行序/休眠唯一基准（A 档响应 × source 全轴；B 池成员、观察轴均不进入）。
 *  行序 = lane_heat 降序 → 事件线数降序 → topicId 升序（add-event-story-heat 5.4，与后端一致）。 */
export function buildObservationRows(
  source: LaneDynamicsWindowData | null | undefined,
  sourceAxis: readonly string[],
): ObservationLaneRow[] {
  if (!source) return []
  const axisSet = new Set(sourceAxis)
  return source.lanes
    .map((lane) => {
      let sectionTotal = 0
      let clueTotal = 0
      let storyTotal = 0
      const storyCounts: Record<string, number> = {}
      const participantCounts: Record<string, number> = {}
      const unassignedCounts: Record<string, number> = {}
      for (const day of lane.timeline) {
        if (!axisSet.has(day.date)) continue
        sectionTotal += day.sections.length
        let dayClues = 0
        for (const section of day.sections) {
          clueTotal += section.threadCount ?? section.fallbackClueCount ?? 0
          dayClues += section.threadCount ?? section.fallbackClueCount ?? 0
        }
        // story_count 缺省（旧后端/未回填）→ 回退当日非综合稿线程数合计（不用桶数冒充）
        const dayStories = day.storyCount ?? dayClues
        storyCounts[day.date] = dayStories
        participantCounts[day.date] = day.maxParticipantCount ?? 0
        unassignedCounts[day.date] = day.unassignedEventCount ?? 0
        storyTotal += dayStories
      }
      return {
        topicId: lane.topicId,
        label: lane.label,
        watchLinked: lane.watchLinked,
        sectionTotal,
        clueTotal,
        laneHeat: lane.laneHeat ?? 0,
        storyTotal: lane.storyCountWindow ?? storyTotal,
        storyCounts,
        participantCounts,
        unassignedCounts,
        dormant: sectionTotal <= DORMANT_SECTION_THRESHOLD,
        snapshot: lane.snapshot,
      }
    })
    .sort((a, b) => (b.laneHeat ?? 0) - (a.laneHeat ?? 0) || (b.storyTotal ?? 0) - (a.storyTotal ?? 0) || compareTopicId(a.topicId, b.topicId))
}

/** 休眠分组（顺序保持；G5 可直接 splitObservationRows(rows) 喂矩阵 props）。 */
export function splitObservationRows(rows: readonly ObservationLaneRow[]): {
  active: ObservationLaneRow[]
  dormant: ObservationLaneRow[]
} {
  const active: ObservationLaneRow[] = []
  const dormant: ObservationLaneRow[] = []
  for (const row of rows) (row.dormant ? dormant : active).push(row)
  return { active, dormant }
}

// ── 矩阵日格投影 ────────────────────────────────────────────────────────────

export type MatrixCell =
  | { kind: 'content', metrics: DayCellMetrics }
  | { kind: 'gap' }
  | { kind: 'unavailable' }

export function projectMatrixCell(resolve: LaneDayResolver, topicId: string, date: string): MatrixCell {
  const resolution = resolve(topicId, date)
  if (resolution.kind === 'content') return { kind: 'content', metrics: dayCellMetrics(resolution.day) }
  return resolution.kind === 'gap' ? { kind: 'gap' } : { kind: 'unavailable' }
}

/** 矩阵格展示视图（MX 组）：格值 = 当日事件线数、颜色 = 最热事件线来源数分档；
 *  '—'=gap；'?'=未知。title/aria 同时给出事件线数/最热点名（或最热来源数）/未归组事件数/主题桶数
 *  （颜色不是唯一信息载体，workshop-board-console 矩阵格契约；surface-story-heat-in-report-top 点名同源）。 */
export interface MatrixCellView {
  kind: 'content' | 'gap' | 'unavailable'
  /** 格内数字：当日事件线数/'—'/'?'。 */
  text: string
  /** 有分区即可开（已知 0 事件线、事件线数未知都合法可读）；gap/unavailable 不可开。 */
  openable: boolean
  /** 来源数分档（颜色通道，0/1-2/3-5/6+；0 不着色）。 */
  bucket: '' | 'is-p1' | 'is-p2' | 'is-p3'
  aria: string
  title: string
}

/** 来源数分档界点（升序；participantBucket 分档与矩阵图例同一常量来源，0 档无色）。 */
export const PARTICIPANT_BUCKET_EDGES = [1, 3, 6] as const

/** 图例 4 档区间文本（0 / 1-2 / 3-5 / 6+）：由分档界点推导，阈值只定义一次
 *  （改界点，格子颜色与图例同步，不在图例里另写魔法数字）。 */
export const PARTICIPANT_TIER_LABELS: readonly string[] = [
  '0',
  `${PARTICIPANT_BUCKET_EDGES[0]}-${PARTICIPANT_BUCKET_EDGES[1] - 1}`,
  `${PARTICIPANT_BUCKET_EDGES[1]}-${PARTICIPANT_BUCKET_EDGES[2] - 1}`,
  `${PARTICIPANT_BUCKET_EDGES[2]}+`,
]

/** 来源数 → 颜色档（0 无色 / 1-2 浅 / 3-5 中 / 6+ 深；单一色相只改不透明度/明度）。 */
export function participantBucket(participants: number): MatrixCellView['bucket'] {
  if (participants >= PARTICIPANT_BUCKET_EDGES[2]) return 'is-p3'
  if (participants >= PARTICIPANT_BUCKET_EDGES[1]) return 'is-p2'
  if (participants >= PARTICIPANT_BUCKET_EDGES[0]) return 'is-p1'
  return ''
}

/** 点名标题截断上限：超长事件线标题截为 ≤30 字 + 省略号（防 title/aria 属性爆炸；spec 允许）。 */
export const HOTTEST_STORY_TITLE_MAX = 30

/** 最热线点名段的可读名字（surface-story-heat-in-report-top）：
 *  有存活 id 才点名；标题空白回退「事件线 #ID」；超长截断；无身份返回 null（不追加、不冒名，
 *  绝不拿泳道/section 标题代替——颜色口径与点名口径必须同源）。 */
export function hottestStoryName(metrics: DayCellMetrics): string | null {
  if (metrics.hottestStoryId == null) return null
  const title = metrics.hottestStoryTitle.trim()
  if (!title) return `事件线 #${metrics.hottestStoryId}`
  return title.length > HOTTEST_STORY_TITLE_MAX
    ? `${title.slice(0, HOTTEST_STORY_TITLE_MAX)}…`
    : title
}

export function matrixCellView(cell: MatrixCell, context: { laneLabel: string, date: string }): MatrixCellView {
  if (cell.kind === 'gap') {
    return {
      kind: 'gap',
      text: '—',
      openable: false,
      bucket: '',
      aria: `${context.laneLabel} ${context.date}：无分区`,
      title: '这一天这条泳道没有分区',
    }
  }
  if (cell.kind === 'unavailable') {
    return {
      kind: 'unavailable',
      text: '?',
      openable: false,
      bucket: '',
      aria: `${context.laneLabel} ${context.date}：未取到（历史数据未加载或加载失败）`,
      title: '该日历史数据未取到，不补 0',
    }
  }
  const { metrics } = cell
  const unknownNote = metrics.hasUnknown ? '（部分桶线索数未知）' : ''
  // 展示降级口径（matrix-clue-fallback-and-rail-merge D2）：格值 = displayCount
  // （事件线数 > 0 用事件线数，否则线索合计兜底）；降级格首段明示「N 条线索
  // （0 条事件线，按线索合计兜底显示）」不冒充事件线数，非降级格维持「N 个事件线」。
  const countSegment = metrics.displayCount === null
    ? '? 个事件线'
    : metrics.degradedFromClue
      ? `${metrics.displayCount} 条线索（0 条事件线，按线索合计兜底显示）`
      : `${metrics.displayCount} 个事件线`
  // 最热段：有身份 → 点名获胜线（id/title 与 max 同源）；无身份（旧响应/未回填）→ 保留数字并
  // 明示「暂不可识别」，不用别的标题冒充（spec「无身份不伪造标题」）。
  const hottestName = hottestStoryName(metrics)
  const heatSegment = hottestName !== null
    ? `最热：${hottestName}（${metrics.maxParticipantCount} 个来源）`
    : `最热 ${metrics.maxParticipantCount} 个来源 · 最热事件线暂不可识别`
  const label = `${context.laneLabel} ${context.date}：${countSegment} · ${heatSegment} · 未归组 ${metrics.unassignedEventCount} 个事件 · ${metrics.sectionCount} 个主题桶${unknownNote}`
  return {
    kind: 'content',
    text: metrics.displayCount === null ? '?' : String(metrics.displayCount),
    openable: true,
    bucket: participantBucket(metrics.maxParticipantCount),
    aria: label,
    title: label,
  }
}

// ── 演进带投影（EV 组） ─────────────────────────────────────────────────────

/** 节点带形态切换阈值：可见日 ≤31 逐日节点，>31 趋势图 + 折叠逐日索引。 */
export const EVOLUTION_NODE_MAX_DAYS = 31
/** 密集日标签阈值：当日事件线数 ≥3 且可见日 ≤31 才显示首线索标题。 */
export const EVOLUTION_LABEL_THRESHOLD = 3

/** 点径公式（批准原型）：min(24, 6 + √事件线数 × 5)；已知 0 → 最小 6px，≥36 封顶 24px。 */
export function evolutionDotSize(total: number): number {
  return Math.min(24, 6 + Math.sqrt(Math.max(0, total)) * 5)
}

/** 趋势与节点统一用事件线数；缺字段且所有回退计数未知时仍为 null，不补 0。 */
export function knownDayTotal(metrics: DayCellMetrics): number | null {
  return metrics.storyCount
}

/** 当日首线索标题：第一桶首条事件标题，兜底第一桶分区 label（展示引导，非断言事实）。 */
export function firstThreadTitle(day: WindowLaneDay): string {
  for (const section of day.sections) {
    if (section.eventTitles.length > 0) return section.eventTitles[0]!
    if (section.label) return section.label
  }
  return ''
}

export interface EvolutionNode {
  date: string
  kind: 'content' | 'gap' | 'unavailable'
  /** 当日事件线数：content=已知（story_count 缺省回退非综合稿线程合计）；gap=已知 0（曲线合法但不可开）；unavailable=null。 */
  total: number | null
  metrics: DayCellMetrics | null
  firstTitle: string
  isToday: boolean
  /** 标签可见（EV-3）：content 且事件线数 ≥ 阈值（宽度条件由调用方按可见日数控制）。 */
  showLabel: boolean
}

export function projectEvolutionNodes(
  topicId: string,
  axis: readonly string[],
  resolve: LaneDayResolver,
  today: string,
): EvolutionNode[] {
  return axis.map((date) => {
    const resolution = resolve(topicId, date)
    const isToday = date === today
    if (resolution.kind === 'content') {
      const metrics = dayCellMetrics(resolution.day)
      const total = metrics.storyCount
      return {
        date,
        kind: 'content',
        total,
        metrics,
        firstTitle: firstThreadTitle(resolution.day),
        isToday,
        showLabel: total !== null && total >= EVOLUTION_LABEL_THRESHOLD,
      }
    }
    if (resolution.kind === 'gap') {
      return { date, kind: 'gap', total: 0, metrics: null, firstTitle: '', isToday, showLabel: false }
    }
    return { date, kind: 'unavailable', total: null, metrics: null, firstTitle: '', isToday, showLabel: false }
  })
}

/** 趋势图 series 值：content=事件线数（全未知 null）、gap=已知 0、unavailable=null。 */
export function evolutionSeriesValue(resolution: LaneDayResolution): number | null {
  if (resolution.kind === 'gap') return 0
  if (resolution.kind === 'unavailable') return null
  return knownDayTotal(dayCellMetrics(resolution.day))
}
