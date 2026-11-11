/**
 * 观察矩阵/泳道演进纯模型（rework-workshop-into-radar-and-desk 矩阵阶段 G3）。
 *
 * 职责拆两半（阶段合同 §2.1 三轴分工）：
 *  - **source 统计**（buildObservationRows）：名单/行序/休眠唯一基准 = A 档响应自身全轴
 *    （泳道当天热度降序 → 全窗线索合计降序 → topicId 升序，与后端 lane-dynamics 行序一致；
 *    休眠分组仍用 sectionTotal 阈值）。只表达当天线索热度，事件线 / 来源数不再参与。
 *    与观察轴/365 池无关——观察裁切只作用于下方日投影，永不回流重算本表。
 *  - **观察日投影**（projectMatrixCell/projectEvolutionNodes）：给定可见轴逐日把
 *    G1 `resolveLaneDay(topicId, date)` 的三态翻译成展示格/演进节点——
 *    content=有分区（已知 0 线索合法、可打开）；gap=请求覆盖但无分区（曲线值 0 但格/节点不可开）；
 *    unavailable=历史未取到（'?' / null，不补 0、不假定无日报）。
 *
 * 量纲（retire-story-adopt-event-metrics D4/D5）：格值、演进节点、顶部曲线统一为
 * 「当日线索合计」（非综合稿 thread 数之和）；颜色通道整体退役——数值 + title/aria
 * 承载全部信息。
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
  /** source 全轴线索总数（当日线索合计求和；行序次键，与后端一致）。 */
  clueTotal: number
  /** 泳道当天热度（lane_heat，heat-v2-lane-clues-1d）；缺省 0（行序主键，与后端一致）。 */
  laneHeat?: number
  /** date → 当日线索合计（各主题桶非综合稿 thread 数之和）。 */
  clueCounts?: Record<string, number>
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
 *  行序 = 泳道当天热度降序 → 全窗线索合计降序 → topicId 升序（与后端 lane-dynamics 一致）。 */
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
      const clueCounts: Record<string, number> = {}
      for (const day of lane.timeline) {
        if (!axisSet.has(day.date)) continue
        sectionTotal += day.sections.length
        // 当日线索合计：后端 clue_count 优先；旧响应回退各桶已知 thread 数合计。
        let dayClues = 0
        let unknown = false
        for (const section of day.sections) {
          const value = section.threadCount ?? section.fallbackClueCount
          if (value === null) {
            unknown = true
            continue
          }
          dayClues += value
        }
        const total = day.clueCount ?? (unknown && day.sections.every(s => (s.threadCount ?? s.fallbackClueCount) === null) ? 0 : dayClues)
        clueCounts[day.date] = total
        clueTotal += total
      }
      return {
        topicId: lane.topicId,
        label: lane.label,
        watchLinked: lane.watchLinked,
        sectionTotal,
        clueTotal,
        laneHeat: lane.laneHeat ?? 0,
        clueCounts,
        dormant: sectionTotal <= DORMANT_SECTION_THRESHOLD,
        snapshot: lane.snapshot,
      }
    })
    .sort((a, b) => (b.laneHeat ?? 0) - (a.laneHeat ?? 0) || b.clueTotal - a.clueTotal || compareTopicId(a.topicId, b.topicId))
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

/** 矩阵格展示视图（D4/D5）：格值 = 当日线索合计；颜色通道退役（数值 + title/aria 承载全部信息）；
 *  '—'=gap；'?'=未知。aria/title 给出「N 条线索 · M 个主题桶」。 */
export interface MatrixCellView {
  kind: 'content' | 'gap' | 'unavailable'
  /** 格内数字：当日线索合计 / '—' / '?'。 */
  text: string
  /** 有分区即可开（已知 0 线索合法可读）；gap/unavailable 不可开。 */
  openable: boolean
  aria: string
  title: string
}

function contentLabel(metrics: DayCellMetrics): string {
  const clueText = metrics.clueCount === null ? '线索数未知' : `${metrics.clueCount} 条线索`
  const peakNote = metrics.peak === null ? '' : `（最多一天桶 ${metrics.peak} 条）`
  return `${clueText} · ${metrics.sectionCount} 个主题桶${peakNote}`
}

export function matrixCellView(cell: MatrixCell, context: { laneLabel: string, date: string }): MatrixCellView {
  if (cell.kind === 'gap') {
    return {
      kind: 'gap',
      text: '—',
      openable: false,
      aria: `${context.laneLabel} ${context.date}：无分区`,
      title: '这一天这条泳道没有分区',
    }
  }
  if (cell.kind === 'unavailable') {
    return {
      kind: 'unavailable',
      text: '?',
      openable: false,
      aria: `${context.laneLabel} ${context.date}：未取到（历史数据未加载或加载失败）`,
      title: '该日历史数据未取到，不补 0',
    }
  }
  const { metrics } = cell
  const label = `${context.laneLabel} ${context.date}：${contentLabel(metrics)}`
  return {
    kind: 'content',
    text: metrics.clueCount === null ? '?' : String(metrics.clueCount),
    openable: true,
    aria: label,
    title: label,
  }
}

// ── 演进带投影（EV 组） ─────────────────────────────────────────────────────

/** 节点带形态切换阈值：可见日 ≤31 逐日节点，>31 趋势图 + 折叠逐日索引。 */
export const EVOLUTION_NODE_MAX_DAYS = 31
/** 密集日标签阈值：当日线索合计 ≥3 且可见日 ≤31 才显示首线索标题。 */
export const EVOLUTION_LABEL_THRESHOLD = 3

/** 点径公式（批准原型）：min(24, 6 + √线索数 × 5)；已知 0 → 最小 6px，≥36 封顶 24px。 */
export function evolutionDotSize(total: number): number {
  return Math.min(24, 6 + Math.sqrt(Math.max(0, total)) * 5)
}

/** 趋势与节点统一用当日线索合计；全部桶未知时仍为 null，不补 0。 */
export function knownDayTotal(metrics: DayCellMetrics): number | null {
  return metrics.clueCount
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
  /** 当日线索合计：content=已知；gap=已知 0（曲线合法但不可开）；unavailable=null。 */
  total: number | null
  metrics: DayCellMetrics | null
  firstTitle: string
  isToday: boolean
  /** 标签可见（EV-3）：content 且线索合计 ≥ 阈值（宽度条件由调用方按可见日数控制）。 */
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
      const total = metrics.clueCount
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

/** 趋势图 series 值：content=当日线索合计（全未知 null）、gap=已知 0、unavailable=null。 */
export function evolutionSeriesValue(resolution: LaneDayResolution): number | null {
  if (resolution.kind === 'gap') return 0
  if (resolution.kind === 'unavailable') return null
  return knownDayTotal(dayCellMetrics(resolution.day))
}
