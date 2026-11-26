/**
 * 壳层视图模型（rework-workshop-into-radar-and-desk 矩阵阶段 G5，批准版面收口）。
 *
 * 只做三件事，全部为纯函数（G1/G3 语义不在此重算）：
 *  - **名单分组**：直接用 G3 `buildObservationRows`（A 档 source 全轴唯一基准）+
 *    `splitObservationRows`，壳层只缓存 computed 结果，不另算排序/休眠。
 *  - **选中态边界**：G1 legacy 数字 `selectedLaneId` ↔ G3 字符串 `topicId` 的换算各收在
 *    一处（组件层不散落 `String()/Number()`）；CH-1 默认选日只看 content（有分区，0 线索合法）。
 *  - **刷选区系列**：365 池逐日「A 名单同 ID 泳道的当日线索合计之和」（与矩阵格同口径；
 *    G2 `LaneTrendPoint` 契约；unknown=null 不补 0，断档=已知 0）。
 */
import type { LaneDynamicsWindowData, WindowLaneDay } from '~/api/normalizers/laneDynamicsWindow'
import { dayCellMetrics } from '~/api/normalizers/laneDynamicsWindow'
import { buildPoolAxis, type LaneTrendPoint } from './laneTrendSeries'
import type { DateBounds } from './observationRange'
import {
  buildObservationRows,
  splitObservationRows,
  type LaneDayResolver,
  type ObservationLaneRow,
} from './observationLanes'

/** 名单三分（保持 G3 行序）：rows = active + dormant 的原始顺序。 */
export interface ConsoleLaneGroups {
  rows: ObservationLaneRow[]
  active: ObservationLaneRow[]
  dormant: ObservationLaneRow[]
}

/**
 * A 档名单/行序/休眠分组（G5 computed 唯一入口）。
 * `sourceAxis` = A 档自身全轴（useBoardConsole.axis），与观察裁切/B 池无关。
 */
export function buildConsoleLaneGroups(
  source: LaneDynamicsWindowData | null | undefined,
  sourceAxis: readonly string[],
): ConsoleLaneGroups {
  const rows = buildObservationRows(source, sourceAxis)
  const groups = splitObservationRows(rows)
  return { rows, active: groups.active, dormant: groups.dormant }
}

/** legacy 数字选中 ID → G3 字符串行（组件层唯一的「旧选中态取新行」入口）。 */
export function findSelectedRow(
  rows: readonly ObservationLaneRow[],
  selectedLaneId: number | null | undefined,
): ObservationLaneRow | null {
  if (selectedLaneId === null || selectedLaneId === undefined) return null
  const key = String(selectedLaneId)
  return rows.find(row => row.topicId === key) ?? null
}

/** G3 字符串 topicId → G1 `selectLane` 数字入参（唯一 Number 转换点；非法返回 null）。 */
export function topicIdToNumber(topicId: string): number | null {
  const parsed = Number(topicId)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null
}

function isContent(resolve: LaneDayResolver, topicId: string, date: string): boolean {
  return resolve(topicId, date).kind === 'content'
}

/**
 * CH-1：可见观察区内「最后有分区日」（content=有分区，已知 0 线索同样合法）。
 * gap/unavailable 均不算内容日；无内容日返回 null（不补造日期）。
 */
export function lastContentDate(
  lane: ObservationLaneRow | null | undefined,
  axis: readonly string[],
  resolve: LaneDayResolver,
): string | null {
  if (!lane) return null
  for (let i = axis.length - 1; i >= 0; i -= 1) {
    const date = axis[i]!
    if (isContent(resolve, lane.topicId, date)) return date
  }
  return null
}

/**
 * 可见轴内报日集合（列头/窄屏报日条的可选集）。轴始终连续，**不**压缩为仅报日；
 * 索引未接线/未知 → 空数组（可选集为空，不是「窗内无日报」的结论）。
 */
export function reportDaysInAxis(
  axis: readonly string[],
  reportDates: ReadonlySet<string> | null | undefined,
): string[] {
  if (!reportDates) return []
  return axis.filter(date => reportDates.has(date))
}

/** 窗内最新报日（轴裁切后取末尾命中；未知/空集 → null，不回退 today 或内容日）。 */
export function latestReportDay(
  axis: readonly string[],
  reportDates: ReadonlySet<string> | null | undefined,
): string | null {
  if (!reportDates || reportDates.size === 0) return null
  for (let i = axis.length - 1; i >= 0; i -= 1) {
    const date = axis[i]!
    if (reportDates.has(date)) return date
  }
  return null
}

/**
 * 选中日校正（受控：只在必要时改写，避免把用户选定日无端搬走）。优先级固定（workshop-board-console
 * 「日报日驱动的日期轴」）：
 *  1. 当前日在轴内且（该泳道当日有分区 或 是报日 或 是**已验证记录日**）→ 保留（切泳道不搬走合法报日）；
 *  2. 无当前日（进入/切板块）→ 既有 CH-1「最后有分区日」优先，无泳道/无内容日 → 窗内**最新报日**；
 *  3. 当前日被范围排出 → 新窗内**最新报日**（不按离旧选日距离另算），窗内无报日 → null。
 *
 * `reportDates` 三态语义：
 *  - `undefined` = 未接线（旧调用）：完全保持旧口径（非内容日回最后内容日，无日期推导）；
 *  - `null` = 已接线但索引未就绪/失败：不把可能合法的报日选择搬走（不误判成「无日报」）；
 *  - `Set`（可为空集）= 索引已就绪：按真实报日集合校正。
 *
 * `verifiedDay` 语义（8E review R-41）：**已验证记录日** = 当前观察目标（同 persistentTopicId）
 * 在观察带里有真实分区记录的日子（仍须在可见轴内）。它是比报日索引更强的证据（记录自带真实
 * reportId/sectionId），索引缺该日（陈旧/不全）时不得把它当普通无报日搬走——否则黄点点击会被
 * 静默撤销。三态：`undefined` = 未接线（旧调用口径不变）；`null` = 已接线但当前无已验证目标
 * （无选择/已清场/观察未就绪）→ 走普通回落；字符串 = 该日受保护（仅保护当前这一个真实观察目标，
 * 不放开所有无报日）。
 */
export function reconcileSelectedDate(
  lane: ObservationLaneRow | null | undefined,
  axis: readonly string[],
  resolve: LaneDayResolver,
  current: string | null,
  reportDates?: ReadonlySet<string> | null,
  verifiedDay?: string | null,
): string | null {
  const fallbackToLatestReport = latestReportDay(axis, reportDates)
  if (current === null) {
    return (lane ? lastContentDate(lane, axis, resolve) : null) ?? fallbackToLatestReport
  }
  if (!axis.includes(current)) return fallbackToLatestReport
  if (lane && isContent(resolve, lane.topicId, current)) return current
  if (verifiedDay !== undefined && verifiedDay !== null && verifiedDay === current) return current
  if (reportDates === undefined) return lane ? lastContentDate(lane, axis, resolve) : null
  if (reportDates === null) return current
  if (reportDates.has(current)) return current
  return (lane ? lastContentDate(lane, axis, resolve) : null) ?? fallbackToLatestReport
}

/**
 * 刷选区（G2 `series`）：池界逐日 = A 名单同 ID 泳道当日线索合计之和
 * （与矩阵格同一条规则：曲线与格子数字同口径，retire-story-adopt-event-metrics D4）。
 *  - 该日值已知（clueCount ≠ null）→ 求和（0 是合法已知值，画基线）；
 *  - 该日全部 A 泳道均无分区（断档）→ 0（池窗口已知，无需补造）；
 *  - 池未取到（任一泳道该日 unavailable 且无其它泳道内容）→ null，不补 0；
 *  - 部分泳道日值未知 → 只用已知部分求和，全未知（合计无法成立）→ null。
 */
export function buildPoolSeries(
  bounds: DateBounds,
  laneIds: readonly string[],
  resolve: LaneDayResolver,
): LaneTrendPoint[] {
  return buildPoolAxis(bounds).map((date) => ({ date, value: poolDayTotal(date, laneIds, resolve) }))
}

function poolDayTotal(date: string, laneIds: readonly string[], resolve: LaneDayResolver): number | null {
  let total = 0
  let knownBuckets = 0
  let contentLanes = 0
  let unknownLanes = 0
  for (const topicId of laneIds) {
    const resolution = resolve(topicId, date)
    if (resolution.kind === 'unavailable') {
      unknownLanes += 1
      continue
    }
    if (resolution.kind !== 'content') continue
    contentLanes += 1
    const metrics = dayCellMetrics(resolution.day)
    if (metrics.clueCount === null) continue
    total += metrics.clueCount
    knownBuckets += metrics.sectionCount
  }
  // 断档（有窗口数据但该日谁都没分区）= 已知 0；否则只要还有泳道未取到就不冒 0
  if (contentLanes === 0) return unknownLanes > 0 ? null : 0
  if (knownBuckets === 0) return null
  return total
}

/** 单日分区数（窄屏迷你条口径；content 才有分区，其余 null 不补 0）。 */
export function narrowDaySectionCount(resolve: LaneDayResolver, topicId: string, date: string): number | null {
  const resolution = resolve(topicId, date)
  if (resolution.kind !== 'content') return null
  return (resolution.day as WindowLaneDay).sections.length
}
