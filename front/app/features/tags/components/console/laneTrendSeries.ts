/**
 * 泳道趋势 series 纯函数（rework-workshop-into-radar-and-desk 矩阵阶段 G2，design D11 观察窗）。
 *
 * 本组共享数据契约：`{ date: string, value: number | null }[]`（date-only 字符串、升序由上层保证）。
 *  - `value: null` = 该日线索数未取到/未知（池未加载、断档），**不补 0、不与相邻段桥接**。
 *  - `value: 0` = 已知零（合法结论），正常画点、落在 0 基线。
 * 读取模型/统计（来源口径、窗口裁切、排序）全部在上层（G1/G3/G5）完成，本模块只做
 * 几何与刻度派生；组件层不得在此复制业务口径。
 *
 * 刻度契约（批准原型 matrix-graphics.js trendFrame / test-cases EV-6）：
 *  - Y 轴（事件线数）：max > 1 → [max, ⌊max/2⌋, 0]；max ≤ 1 → [max, 0]；无已知值 → 无刻度（不伪造量级）。
 *  - X 轴（日期）：跨度 ≤ DATE_TICK_FULL_MAX 逐日全标，否则首/中/末抽样（去重保序）。
 */
import { daysBetweenInclusive, shiftDate } from './observationRange'
import type { DateBounds } from './observationRange'

/** 单日序列点：value=null 表示未知（未取到），0 是已知合法值。 */
export interface LaneTrendPoint {
  date: string
  value: number | null
}

/** 日期轴逐日全标的跨度上限（超过则首/中/末抽样）。 */
export const DATE_TICK_FULL_MAX = 5

/** 把已知值切成连续段：遇 null 断开，返回各段（长度 ≥1，仅含已知值）。 */
export function knownSegments(series: readonly LaneTrendPoint[]): LaneTrendPoint[][] {
  const segments: LaneTrendPoint[][] = []
  let current: LaneTrendPoint[] = []
  for (const point of series) {
    if (point.value === null) {
      if (current.length > 0) segments.push(current)
      current = []
    } else {
      current.push(point)
    }
  }
  if (current.length > 0) segments.push(current)
  return segments
}

/** 已知值最大值；无已知值返回 0（调用方需先判断「全未知」再决定是否画线/刻度）。 */
export function knownMaxValue(series: readonly LaneTrendPoint[]): number {
  let max = 0
  for (const point of series) {
    if (point.value !== null && point.value > max) max = point.value
  }
  return max
}

/** Y 轴刻度值（真实比例，max=0 表示无已知值 → 空刻度）。 */
export function trendTickValues(max: number): number[] {
  if (max <= 0) return []
  return max > 1 ? [max, Math.floor(max / 2), 0] : [max, 0]
}

/** X 轴日期刻度下标：小跨度全标，大跨度首/中/末（去重保序）。 */
export function dateTickIndexes(count: number): number[] {
  if (count <= 0) return []
  if (count <= DATE_TICK_FULL_MAX) {
    return Array.from({ length: count }, (_, index) => index)
  }
  return [...new Set([0, Math.floor((count - 1) / 2), count - 1])]
}

/** 均匀 X 定位（0..1）：单点居中，避免除零。 */
export function xFraction(index: number, count: number): number {
  if (count <= 1) return 0.5
  return index / (count - 1)
}

/** 池全轴（含两端）：[bounds.start … bounds.end]，来自 G1 日期纯函数（UTC 数值日算术）。 */
export function buildPoolAxis(bounds: DateBounds): string[] {
  const count = daysBetweenInclusive(bounds.start, bounds.end)
  return Array.from({ length: count }, (_, index) => shiftDate(bounds.start, index))
}

/**
 * 绘图几何（与批准原型同构；TrendAxes 的刻度定位百分比重算自这里）：
 *  - baselineY：0 基线 y；topY：满值 y；viewHeight：viewBox 高。
 */
export interface TrendGeometry {
  viewWidth: number
  viewHeight: number
  baselineY: number
  topY: number
}

/** 数值 → y 坐标（真实比例，0 落在基线；max≤0 时全部落基线）。 */
export function valueToY(value: number, max: number, geometry: TrendGeometry): number {
  if (max <= 0) return geometry.baselineY
  const clamped = Math.max(0, value)
  return geometry.baselineY - (clamped / max) * (geometry.baselineY - geometry.topY)
}

/** Y 刻度 → TrendAxes 顶部定位百分比（0=0%，基线贴底对齐外置轴）。 */
export function tickTopPercent(value: number, max: number, geometry: TrendGeometry): number {
  return ((valueToY(value, max, geometry) / geometry.viewHeight) * 100)
}

/** x 下标 → viewBox x 坐标。 */
export function indexToX(index: number, count: number, geometry: TrendGeometry): number {
  return xFraction(index, count) * geometry.viewWidth
}
