/**
 * brush 背景曲线几何派生（rework-workshop-into-radar-and-desk 矩阵阶段 G2，ObservationRangeControls 专用）。
 *
 * 输入 = 池边界 + 上层给定的 series（{date,value|null}），输出 = 模板直接消费的
 * 视图模型（分段折线/面积/刻度/端点坐标）。日期→下标、值→y 全部经 laneTrendSeries
 * 纯函数（真实比例、null 不补 0 不桥接、已知 0 落基线）；本模块不做业务口径。
 */
import type { DateBounds } from './observationRange'
import {
  buildPoolAxis,
  dateTickIndexes,
  indexToX,
  knownMaxValue,
  knownSegments,
  tickTopPercent,
  trendTickValues,
  valueToY,
  xFraction,
  type LaneTrendPoint,
} from './laneTrendSeries'

/** 批准原型 brush 绘图区：viewBox 1000×80，基线 y=68、满值 y=10（显示高 88px 与 TrendAxes 对齐）。 */
export const BRUSH_GEOMETRY = { viewWidth: 1000, viewHeight: 80, baselineY: 68, topY: 10 }

export interface BrushCurveModel {
  /** 池全轴（含两端），刷选/日期轴共用。 */
  axis: string[]
  /** 已知值连续段（null 断开）。 */
  segments: LaneTrendPoint[][]
  /** 已知值最大值（0 = 全未知，不画线不造刻度）。 */
  max: number
  /** 外置 Y 刻度（TrendAxes）。 */
  yTicks: { value: number, topPercent: number }[]
  /** 外置 X 日期刻度（首/中/末抽样或小跨度全标）。 */
  xTicks: { label: string, leftPercent: number, align: 'start' | 'center' | 'end' }[]
  /** date → 池轴下标。 */
  indexOf: Map<string, number>
  /** 折线 points 串（单段）。 */
  segmentPoints: (segment: LaneTrendPoint[]) => string
  /** 面积 path d（单段，已知段 ≥2 才有）。 */
  segmentAreaPath: (segment: LaneTrendPoint[]) => string
  /** 池轴下标 → viewBox x。 */
  xAt: (index: number) => number
}

export function buildBrushCurve(bounds: DateBounds, series: LaneTrendPoint[] | null): BrushCurveModel {
  const axis = buildPoolAxis(bounds)
  const count = axis.length
  const indexOf = new Map(axis.map((date, index) => [date, index]))
  const valuesByDate = new Map(series?.map(point => [point.date, point.value]) ?? [])
  const axisSeries: LaneTrendPoint[] = axis.map(date => ({ date, value: valuesByDate.get(date) ?? null }))
  const segments = knownSegments(axisSeries)
  const max = knownMaxValue(axisSeries)
  const geometry = BRUSH_GEOMETRY
  const xAt = (index: number): number => indexToX(index, count, geometry)
  const yAt = (value: number): number => valueToY(value, max, geometry)
  return {
    axis,
    segments,
    max,
    yTicks: trendTickValues(max).map(value => ({ value, topPercent: tickTopPercent(value, max, geometry) })),
    xTicks: dateTickIndexes(count).map((index) => {
      const align = index === 0 ? 'start' as const : index === count - 1 ? 'end' as const : 'center' as const
      return { label: axis[index] ?? '', leftPercent: xFraction(index, count) * 100, align }
    }),
    indexOf,
    segmentPoints: segment =>
      segment.map(point => `${xAt(indexOf.get(point.date) ?? 0)},${yAt(point.value ?? 0)}`).join(' '),
    segmentAreaPath: (segment) => {
      const points = segment.map(point => `${xAt(indexOf.get(point.date) ?? 0)},${yAt(point.value ?? 0)}`)
      const firstX = xAt(indexOf.get(segment[0]?.date ?? '') ?? 0)
      const lastX = xAt(indexOf.get(segment[segment.length - 1]?.date ?? '') ?? 0)
      return `M${points.join(' L')} L${lastX},${geometry.baselineY} L${firstX},${geometry.baselineY} Z`
    },
    xAt,
  }
}
