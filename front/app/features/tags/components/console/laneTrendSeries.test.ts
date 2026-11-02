/**
 * laneTrendSeries 纯函数测试（矩阵阶段 G2，test-cases-matrix-stage.md EV-6 + 阶段合同零值语义）。
 *
 *  - null 不补 0、不桥接：knownSegments 按未知日切段。
 *  - 已知 0 是合法值：计入已知段、max 计算时与其他值同权。
 *  - Y 刻度 = max/⌊max/2⌋/0（max ≤ 1 退 [max,0]，无已知值空刻度）。
 *  - X 日期刻度：≤5 全标，>5 首/中/末去重；单点 xFraction 居中。
 *  - 池全轴含两端（365 边界 index 0..364 含首尾）。
 */
import { describe, expect, it } from 'vitest'
import {
  DATE_TICK_FULL_MAX,
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

const GEOMETRY = { viewWidth: 480, viewHeight: 74, baselineY: 64, topY: 8 }

function series(values: (number | null)[]): LaneTrendPoint[] {
  return values.map((value, index) => ({ date: `2026-10-${String(index + 1).padStart(2, '0')}`, value }))
}

describe('laneTrendSeries — null 不补 0、不桥接', () => {
  it('knownSegments：未知日切段，已知 0 保留在段内', () => {
    const segments = knownSegments(series([3, null, 0, 2, null, null, 5]))
    expect(segments.map(segment => segment.map(point => point.value))).toEqual([
      [3],
      [0, 2],
      [5],
    ])
  })

  it('knownSegments：全未知/空 series 返回空数组（不伪造段）', () => {
    expect(knownSegments(series([null, null]))).toEqual([])
    expect(knownSegments([])).toEqual([])
  })

  it('knownMaxValue：已知 0 与负缺失互不冒充；无已知值返回 0', () => {
    expect(knownMaxValue(series([0, 0, 1]))).toBe(1)
    expect(knownMaxValue(series([0, 0]))).toBe(0)
    expect(knownMaxValue(series([null, null]))).toBe(0)
  })
})

describe('laneTrendSeries — 刻度契约（EV-6）', () => {
  it('trendTickValues：max>1 → [max, ⌊max/2⌋, 0]；max≤1 → [max,0]；无已知值空', () => {
    expect(trendTickValues(9)).toEqual([9, 4, 0])
    expect(trendTickValues(2)).toEqual([2, 1, 0])
    expect(trendTickValues(1)).toEqual([1, 0])
    expect(trendTickValues(0)).toEqual([])
  })

  it('dateTickIndexes：≤5 全标；>5 首/中/末；两元素与单元素不重复', () => {
    expect(dateTickIndexes(4)).toEqual([0, 1, 2, 3])
    expect(dateTickIndexes(DATE_TICK_FULL_MAX)).toEqual([0, 1, 2, 3, 4])
    expect(dateTickIndexes(6)).toEqual([0, 2, 5])
    expect(dateTickIndexes(365)).toEqual([0, 182, 364])
    expect(dateTickIndexes(2)).toEqual([0, 1])
    expect(dateTickIndexes(1)).toEqual([0])
    expect(dateTickIndexes(0)).toEqual([])
  })

  it('xFraction：单点居中不除零；indexToX 随跨度线性', () => {
    expect(xFraction(0, 1)).toBe(0.5)
    expect(xFraction(0, 4)).toBe(0)
    expect(xFraction(3, 4)).toBe(1)
    expect(indexToX(2, 5, GEOMETRY)).toBe(240)
  })
})

describe('laneTrendSeries — 值→坐标（真实比例）', () => {
  it('valueToY：0 落基线、满值到顶、max≤0 全落基线', () => {
    expect(valueToY(0, 8, GEOMETRY)).toBe(64)
    expect(valueToY(8, 8, GEOMETRY)).toBe(8)
    expect(valueToY(4, 8, GEOMETRY)).toBe(36)
    expect(valueToY(5, 0, GEOMETRY)).toBe(64)
  })

  it('tickTopPercent：基线刻度贴底 0% 不越界', () => {
    expect(tickTopPercent(0, 8, GEOMETRY)).toBeCloseTo((64 / 74) * 100, 5)
    expect(tickTopPercent(8, 8, GEOMETRY)).toBeCloseTo((8 / 74) * 100, 5)
  })
})

describe('laneTrendSeries — 池全轴', () => {
  it('buildPoolAxis：含两端 365 日（2025-10-24 → 2026-10-23）', () => {
    const axis = buildPoolAxis({ start: '2025-10-24', end: '2026-10-23' })
    expect(axis).toHaveLength(365)
    expect(axis[0]).toBe('2025-10-24')
    expect(axis[364]).toBe('2026-10-23')
  })

  it('buildPoolAxis：单日池与跨月边界', () => {
    expect(buildPoolAxis({ start: '2026-10-23', end: '2026-10-23' })).toEqual(['2026-10-23'])
    expect(buildPoolAxis({ start: '2026-09-30', end: '2026-10-02' })).toEqual([
      '2026-09-30',
      '2026-10-01',
      '2026-10-02',
    ])
  })
})
