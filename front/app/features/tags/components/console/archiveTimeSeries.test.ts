/**
 * archiveTimeSeries 纯函数测试（archive-time-chart-range）。
 *
 * 覆盖口径边界：视窗贴齐最早日 / 跨年 / 上界 clamp、缺日补 0（与 laneTrendSeries 的
 * null 语义相反）、平移越界返回 null、导航条分桶。
 */
import { describe, expect, it } from 'vitest'
import {
  buildStripSeries,
  shiftRangeWithViewport,
  shiftViewport,
  sliceSeries,
  toCountsMap,
  viewportFor,
} from './archiveTimeSeries'
import { daysBetweenInclusive, shiftDate } from './observationRange'

function mapOf(entries: Array<[string, number]>): Map<string, number> {
  return new Map(entries)
}

describe('archiveTimeSeries — toCountsMap', () => {
  it('稀疏 days → 查表；earliest/latest 原样透传', () => {
    const counts = toCountsMap({
      earliest_date: '2026-10-01',
      latest_date: '2026-10-05',
      days: [{ date: '2026-10-01', count: 3 }, { date: '2026-10-05', count: 1 }],
    })
    expect(counts.earliest).toBe('2026-10-01')
    expect(counts.latest).toBe('2026-10-05')
    expect(counts.byDate.get('2026-10-01')).toBe(3)
    expect(counts.byDate.has('2026-10-03')).toBe(false)
  })

  it('空载荷：earliest/latest 为 null，表为空', () => {
    const counts = toCountsMap({ earliest_date: null, latest_date: null, days: [] })
    expect(counts.earliest).toBeNull()
    expect(counts.latest).toBeNull()
    expect(counts.byDate.size).toBe(0)
  })
})

describe('archiveTimeSeries — viewportFor', () => {
  it('默认 90 天窗口：右端=锚点，左端前推 89 天', () => {
    const viewport = viewportFor('2026-11-18', '2026-01-01', '2026-11-18')
    expect(viewport.end).toBe('2026-11-18')
    expect(viewport.start).toBe('2026-08-21')
    expect(daysBetweenInclusive(viewport.start, viewport.end)).toBe(90)
  })

  it('跨年：窗口左端落到上一年', () => {
    const viewport = viewportFor('2026-01-05', '2025-01-01', '2026-01-05')
    expect(viewport.start).toBe('2025-10-08')
    expect(viewport.end).toBe('2026-01-05')
  })

  it('历史不足一窗：左端贴齐最早有文章日（视窗变窄）', () => {
    const viewport = viewportFor('2026-11-18', '2026-11-10', '2026-11-18')
    expect(viewport.start).toBe('2026-11-10')
    expect(viewport.end).toBe('2026-11-18')
    expect(daysBetweenInclusive(viewport.start, viewport.end)).toBe(9)
  })

  it('右端不超过上界（今天/最新命中日）', () => {
    const viewport = viewportFor('2026-12-31', '2026-01-01', '2026-11-18')
    expect(viewport.end).toBe('2026-11-18')
    expect(viewport.start).toBe('2026-08-21')
  })

  it('锚点早于最早命中日：抬到左端恰好贴 earliest（宽度保持）', () => {
    const viewport = viewportFor('2026-01-05', '2026-01-10', '2026-11-18')
    expect(viewport.start).toBe('2026-01-10')
    expect(viewport.end).toBe('2026-04-09')
    expect(daysBetweenInclusive(viewport.start, viewport.end)).toBe(90)
  })

  it('earliest 未就绪（null）：不贴齐，按锚点前推', () => {
    const viewport = viewportFor('2026-11-18', null, '2026-11-18')
    expect(viewport.start).toBe('2026-08-21')
    expect(viewport.end).toBe('2026-11-18')
  })
})

describe('archiveTimeSeries — sliceSeries', () => {
  it('缺日补 0（已知 0，画基线）——与 laneTrendSeries 的 null 语义相反', () => {
    const series = sliceSeries(
      { start: '2026-10-01', end: '2026-10-03' },
      mapOf([['2026-10-01', 4], ['2026-10-03', 2]]),
    )
    expect(series.map(point => point.value)).toEqual([4, 0, 2])
    expect(series.every(point => point.value !== null)).toBe(true)
  })

  it('逐日一点，天数 = 含两端天数', () => {
    const series = sliceSeries({ start: '2026-10-01', end: '2026-10-10' }, new Map())
    expect(series).toHaveLength(10)
    expect(series[0]!.date).toBe('2026-10-01')
    expect(series[9]!.date).toBe('2026-10-10')
  })
})

describe('archiveTimeSeries — shiftViewport / shiftRangeWithViewport', () => {
  const bounds = { start: '2026-01-01', end: '2026-12-31' }

  it('界内平移：整体位移，宽度不变', () => {
    const shifted = shiftViewport({ start: '2026-10-01', end: '2026-10-10' }, -5, bounds)
    expect(shifted).toEqual({ start: '2026-09-26', end: '2026-10-05' })
  })

  it('越界平移返回 null（不静默截断）', () => {
    expect(shiftViewport({ start: '2026-01-01', end: '2026-01-10' }, -1, bounds)).toBeNull()
    expect(shiftViewport({ start: '2026-12-22', end: '2026-12-31' }, 1, bounds)).toBeNull()
  })

  it('区间随视窗同步平移：相对位置与宽度不变', () => {
    const shifted = shiftRangeWithViewport({ start: '2026-10-05', end: '2026-10-09' }, -30, bounds)
    expect(shifted).toEqual({ start: '2026-09-05', end: '2026-09-09' })
  })

  it('平移后越出目标界：钳制进界内（宽度不增）', () => {
    const shifted = shiftRangeWithViewport({ start: '2026-01-02', end: '2026-01-06' }, -10, bounds)
    expect(shifted.start).toBe('2026-01-01')
    expect(shifted.end).toBe('2026-01-05')
  })
})

describe('archiveTimeSeries — buildStripSeries', () => {
  it('天数不超过桶数：逐日一点', () => {
    const series = buildStripSeries('2026-10-01', '2026-10-05', mapOf([['2026-10-02', 3]]), 10)
    expect(series).toHaveLength(5)
    expect(series.map(point => point.value)).toEqual([0, 3, 0, 0, 0])
  })

  it('超长历史：按桶降采样，桶内求和（密度示意）', () => {
    const byDate = new Map<string, number>()
    for (let index = 0; index < 10; index++) {
      byDate.set(shiftDate('2026-10-01', index), 2)
    }
    const series = buildStripSeries('2026-10-01', '2026-10-10', byDate, 5)
    expect(series).toHaveLength(5)
    // 每桶 2 天、每天 2 篇 → 桶和 4
    expect(series.map(point => point.value)).toEqual([4, 4, 4, 4, 4])
  })

  it('首尾同一天：只一个点', () => {
    const series = buildStripSeries('2026-10-01', '2026-10-01', mapOf([['2026-10-01', 7]]), 10)
    expect(series).toEqual([{ date: '2026-10-01', value: 7 }])
  })
})
