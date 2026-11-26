/**
 * 归档时间曲线纯函数（archive-time-chart-range）。
 *
 * 职责：把按日计数端点的稀疏结果派生成主图序列、视窗区间与导航条密度序列。
 * 日期算术一律复用 `observationRange.ts`（shiftDate / daysBetweenInclusive /
 * clampRangeToBounds），本模块不新写日期函数。
 *
 * **口径与 `laneTrendSeries.ts` 相反（勿"统一口径"）**：
 *  - laneTrendSeries：`value: null` = 该日**未取到/未知**，不补 0、不与相邻段桥接；
 *  - 本模块：一次请求已覆盖整段历史，**缺日即已知 0**，补 0 画基线（归档口径：
 *    每日进版块文章数，某日没有命中文章就是 0 篇，不是"没取到"）。
 */
import {
  clampRangeToBounds,
  daysBetweenInclusive,
  shiftDate,
  type DateBounds,
  type ObservationRange,
} from './observationRange'
import type { LaneTrendPoint } from './laneTrendSeries'

/** 按日计数端点的原始载荷（snake_case，与后端 DTO 一致）。 */
export interface DailyCountsPayload {
  earliest_date: string | null
  latest_date: string | null
  days: Array<{ date: string, count: number }>
}

/** 稀疏计数 → 日期 → 篇数 的查表（缺日不在表里，取值时按 0 处理）。 */
export interface ArchiveCounts {
  earliest: string | null
  latest: string | null
  byDate: Map<string, number>
}

/** 稀疏载荷 → 查表（端点原样，不补零、不改口径）。 */
export function toCountsMap(payload: DailyCountsPayload | null | undefined): ArchiveCounts {
  const byDate = new Map<string, number>()
  for (const day of payload?.days ?? []) {
    if (day && typeof day.date === 'string') byDate.set(day.date, day.count)
  }
  return {
    earliest: payload?.earliest_date ?? null,
    latest: payload?.latest_date ?? null,
    byDate,
  }
}

/**
 * 视窗区间：以 anchorEnd 为右端（含）、向前 windowDays 天。
 *
 * - 右端不超过 upperBound（视窗右端上限 = max(今天, 最新命中日)）；
 * - 左端贴齐 earliest（历史不足一窗时视窗变窄，不早于最早有文章日）；
 * - anchorEnd 早于 earliest（拖过头）时：历史够长则把右端抬到「左端恰好贴 earliest」
 *   的合法位置（宽度保持），历史本身不足一窗则右端贴历史末（整段可见）；
 * - earliest 晚于 anchorEnd（全部文章在未来日，极少）时不做左贴，区间仍非空。
 */
export function viewportFor(
  anchorEnd: string,
  earliest: string | null,
  upperBound: string,
  windowDays = 90,
): ObservationRange {
  const width = Math.max(1, Math.floor(windowDays) || 1)
  let end = anchorEnd > upperBound ? upperBound : anchorEnd
  if (earliest && end < earliest) {
    const minAnchor = shiftDate(earliest, width - 1)
    end = minAnchor <= upperBound ? minAnchor : upperBound
  }
  let start = shiftDate(end, -(width - 1))
  if (earliest && earliest > start) start = earliest
  if (start > end) start = end
  return { start, end }
}

/**
 * 视窗切片序列：逐日一个点，**缺日补 0**（已知 0，画基线）。
 * 与 laneTrendSeries 的 null 语义相反，注释见本文件头。
 */
export function sliceSeries(viewport: ObservationRange, byDate: Map<string, number>): LaneTrendPoint[] {
  const count = daysBetweenInclusive(viewport.start, viewport.end)
  if (count <= 0) return []
  const points: LaneTrendPoint[] = []
  for (let index = 0; index < count; index++) {
    const date = shiftDate(viewport.start, index)
    points.push({ date, value: byDate.get(date) ?? 0 })
  }
  return points
}

/** 视窗整体平移；越出目标界返回 null（调用方据此拒绝该位移，不静默截断）。 */
export function shiftViewport(
  viewport: ObservationRange,
  deltaDays: number,
  bounds: DateBounds,
): ObservationRange | null {
  const shifted = {
    start: shiftDate(viewport.start, deltaDays),
    end: shiftDate(viewport.end, deltaDays),
  }
  if (shifted.start < bounds.start || shifted.end > bounds.end) return null
  return shifted
}

/**
 * 已提交区间随视窗**同步平移同偏移**（保持相对位置、宽度不变），再钳制进目标界内。
 *
 * 钳制放在平移之后：先钳制再平移会把区间推到目标界边缘，破坏"相对位置不变"的
 * 产品规则（导航条平移时区间应保持相对位置同步平移）。
 */
export function shiftRangeWithViewport(
  range: ObservationRange,
  deltaDays: number,
  bounds: DateBounds,
): ObservationRange {
  const shifted = {
    start: shiftDate(range.start, deltaDays),
    end: shiftDate(range.end, deltaDays),
  }
  return clampRangeToBounds(shifted, bounds)
}

/**
 * 导航条密度序列（全历史 → 分桶）。
 *
 * 天数 ≤ maxBuckets 时逐日一点；超过则按桶降采样，**桶内取 sum**（导航条只表达密度，
 * 精确值看主图——桶和不是当天篇数，不得当作精确值解读）。
 */
export function buildStripSeries(
  earliest: string,
  latest: string,
  byDate: Map<string, number>,
  maxBuckets = 240,
): Array<{ date: string, value: number }> {
  const total = daysBetweenInclusive(earliest, latest)
  if (total <= 0) return []
  const buckets = Math.max(1, Math.min(total, Math.floor(maxBuckets) || 1))
  const series: Array<{ date: string, value: number }> = []
  for (let index = 0; index < buckets; index++) {
    const startIndex = Math.floor(index * total / buckets)
    const endIndex = Math.max(startIndex, Math.floor((index + 1) * total / buckets) - 1)
    let sum = 0
    for (let day = startIndex; day <= endIndex; day++) {
      sum += byDate.get(shiftDate(earliest, day)) ?? 0
    }
    series.push({ date: shiftDate(earliest, startIndex), value: sum })
  }
  return series
}
