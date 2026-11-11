/**
 * laneConsoleView 纯模型测试（矩阵阶段 G5；S9 主链 11/12/15/20、CH-1 默认选日、G2 series）。
 *
 *  - 名单分组只在 A 档全轴算（观察裁切/B 池不参与），行序/休眠沿用 G3；
 *  - legacy 数字选中 ID ↔ G3 字符串 topicId 各只有一处换算；
 *  - 默认选日 = 可见区间最后有分区日（0 线索合法，gap/unavailable 跳过）；
 *  - 刷选区 series：已知合计 / 断档 0 / 池未取到 null / 桶数未知不冒 0。
 */
import { describe, expect, it } from 'vitest'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'
import {
  normalizeLaneDynamicsWindow,
  type LaneDynamicsWindowData,
} from '~/api/normalizers/laneDynamicsWindow'
import { buildDayAxis } from '~/features/tags/composables/laneMatrixModel'
import type { LaneDayResolver } from './observationLanes'
import {
  buildConsoleLaneGroups,
  buildPoolSeries,
  findSelectedRow,
  lastContentDate,
  narrowDaySectionCount,
  reconcileSelectedDate,
  reportDaysInAxis,
  topicIdToNumber,
} from './laneConsoleView'

const ANCHOR = '2026-10-23'
const AXIS14 = buildDayAxis(ANCHOR, 14)
const TIER_START = AXIS14[0]!

function sectionRaw(id: number, overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return { section_id: id, label: `桶${id}`, events: [`事件${id}`], ...overrides }
}

function laneRaw(topicId: number, label: string, timeline: { date: string, sections: Record<string, unknown>[], story_count?: number }[]): Record<string, unknown> {
  return { topic_id: topicId, label, watch_linked: false, section_count_14d: 0, snapshot: null, timeline }
}

function normalize(lanes: Record<string, unknown>[], days = 14): LaneDynamicsWindowData {
  return normalizeLaneDynamicsWindow({
    window_days: days,
    has_reports: true,
    lanes,
    candidates: [],
  } as unknown as LaneDynamicsResponse)
}

/** 档/池双索引 resolver（与 G1 resolveLaneDay 三态同构；池未传 = unavailable）。 */
function makeResolver(tier: LaneDynamicsWindowData, pool?: LaneDynamicsWindowData | null): LaneDayResolver {
  const tierIndex = new Map(tier.lanes.map(lane => [lane.topicId, new Map(lane.timeline.map(day => [day.date, day]))]))
  const poolIndex = pool ? new Map(pool.lanes.map(lane => [lane.topicId, new Map(lane.timeline.map(day => [day.date, day]))])) : null
  return (topicId, date) => {
    if (date >= TIER_START) {
      const day = tierIndex.get(topicId)?.get(date)
      return day ? { kind: 'content', source: 'tier', day } : { kind: 'gap' }
    }
    if (!poolIndex) return { kind: 'unavailable' }
    const day = poolIndex.get(topicId)?.get(date)
    return day ? { kind: 'content', source: 'pool', day } : { kind: 'gap' }
  }
}

describe('laneConsoleView — 名单分组（A 档唯一基准）', () => {
  it('rows 由 A 档全轴排序与休眠分组派生；观察裁切不改名单', () => {
    const source = normalize([
      laneRaw(20, '活跃线', [
        { date: '2026-10-22', sections: [sectionRaw(1, { thread_count: 4 }), sectionRaw(2, { thread_count: 1 })] },
        { date: '2026-10-23', sections: [sectionRaw(3, { thread_count: 2 })] },
      ]),
      laneRaw(21, '休眠线', [{ date: '2026-10-23', sections: [sectionRaw(4, { thread_count: 9 })] }]),
    ])

    const groups = buildConsoleLaneGroups(source, AXIS14)
    expect(groups.rows.map(row => row.topicId)).toEqual(['21', '20'])
    expect(groups.dormant.map(row => row.topicId)).toEqual(['21'])
    expect(groups.active.map(row => row.topicId)).toEqual(['20'])

    // 统计只认传入的 source 全轴：轴外日期不累计（故壳层必须传档全轴而非可见裁切轴）
    const narrowed = buildConsoleLaneGroups(source, ['2026-10-23'])
    expect(narrowed.rows.find(row => row.topicId === '20')?.sectionTotal).toBe(1)
    expect(narrowed.rows.find(row => row.topicId === '20')?.clueTotal).toBe(2)
    expect(narrowed.dormant.map(row => row.topicId)).toEqual(['21', '20'])
  })

  it('无源数据：空名单（不补造行）', () => {
    expect(buildConsoleLaneGroups(null, AXIS14)).toEqual({ rows: [], active: [], dormant: [] })
  })
})

describe('laneConsoleView — 选中态边界换算', () => {
  const rows = buildConsoleLaneGroups(
    normalize([laneRaw(1204, '美伊博弈', [{ date: '2026-10-23', sections: [sectionRaw(1, { thread_count: 3 })] }])]),
    AXIS14,
  ).rows

  it('legacy 数字选中 ID 取 G3 字符串行；缺省/不存在返回 null', () => {
    expect(findSelectedRow(rows, 1204)?.topicId).toBe('1204')
    expect(findSelectedRow(rows, null)).toBeNull()
    expect(findSelectedRow(rows, 9999)).toBeNull()
  })

  it('字符串 topicId → selectLane 数字入参只在此处换算，非法返回 null', () => {
    expect(topicIdToNumber('1204')).toBe(1204)
    expect(topicIdToNumber('0')).toBeNull()
    expect(topicIdToNumber('-3')).toBeNull()
    expect(topicIdToNumber('12.5')).toBeNull()
    expect(topicIdToNumber('abc')).toBeNull()
  })
})

describe('laneConsoleView — 默认选日（CH-1）', () => {
  const tier = normalize([
    laneRaw(1204, '主线', [
      { date: '2026-10-15', sections: [sectionRaw(1, { thread_count: 0 })] },
      { date: '2026-10-20', sections: [sectionRaw(2, { thread_count: 2 })] },
    ]),
  ])
  const pool = normalize([
    laneRaw(1204, '主线', [
      { date: '2026-10-01', sections: [sectionRaw(9, { thread_count: 1 })] },
    ]),
  ], 365)
  const lane = buildConsoleLaneGroups(tier, AXIS14).rows[0]!
  const tierOnly = makeResolver(tier)

  it('最后有分区日取content（0 线索日合法，不跳过）', () => {
    const axis = ['2026-10-15', '2026-10-20', '2026-10-21', '2026-10-23']
    expect(lastContentDate(lane, axis, tierOnly)).toBe('2026-10-20')
    expect(lastContentDate(lane, ['2026-10-15'], tierOnly)).toBe('2026-10-15')
  })

  it('无分区/未取到不补造日期；无内容日返回 null', () => {
    expect(lastContentDate(lane, ['2026-10-21', '2026-10-22'], tierOnly)).toBeNull()
    expect(lastContentDate(null, AXIS14, tierOnly)).toBeNull()
  })

  it('池跨档历史：补到 content 即算（B 只补 A 同 ID 历史）', () => {
    const withPool = makeResolver(tier, pool)
    expect(lastContentDate(lane, ['2026-10-01', '2026-10-20'], withPool)).toBe('2026-10-20')
    expect(lastContentDate(lane, ['2026-10-01'], withPool)).toBe('2026-10-01')
    // 池未取到：该历史日不是内容日（不冒 0）
    expect(lastContentDate(lane, ['2026-10-01'], tierOnly)).toBeNull()
  })

  it('reconcile：当前日仍为内容日则保留，否则回最后有分区日', () => {
    const axis = ['2026-10-15', '2026-10-20', '2026-10-23']
    expect(reconcileSelectedDate(lane, axis, tierOnly, '2026-10-15')).toBe('2026-10-15')
    expect(reconcileSelectedDate(lane, axis, tierOnly, '2026-10-23')).toBe('2026-10-20')
    // 索引未接线（未知）：范围排出不能凭空造日期 → null（8C RDY-6 口径）
    expect(reconcileSelectedDate(lane, axis, tierOnly, '2026-10-01')).toBeNull()
    expect(reconcileSelectedDate(lane, axis, tierOnly, null)).toBe('2026-10-20')
    // 索引未接线（undefined）= 旧口径：无 lane 时非内容日不造日期
    expect(reconcileSelectedDate(null, axis, tierOnly, '2026-10-15')).toBeNull()
  })
})

describe('laneConsoleView — 刷选区 series（G2 LaneTrendPoint）', () => {
  it('逐日 = A 名单同 ID 桶线索合计；断档已知 0；池未取到 null', () => {
    const tier = normalize([
      laneRaw(1, 'A', [
        { date: '2026-10-22', sections: [sectionRaw(1, { thread_count: 3 }), sectionRaw(2, { thread_count: 2 })] },
        { date: '2026-10-23', sections: [sectionRaw(3, { thread_count: 4 })] },
      ]),
      laneRaw(2, 'B', [
        { date: '2026-10-22', sections: [sectionRaw(4, { thread_count: 1 })] },
      ]),
    ])
    const bounds = { start: '2026-10-01', end: '2026-10-23' }
    const series = buildPoolSeries(bounds, ['1', '2'], makeResolver(tier))
    expect(series).toHaveLength(23)
    expect(series[0]).toEqual({ date: '2026-10-01', value: null }) // 池未取到，不补 0
    expect(series.find(item => item.date === '2026-10-22')).toEqual({ date: '2026-10-22', value: 6 })
    expect(series.find(item => item.date === '2026-10-23')).toEqual({ date: '2026-10-23', value: 4 })
    expect(series.find(item => item.date === '2026-10-21')).toEqual({ date: '2026-10-21', value: 0 }) // 断档 = 已知 0
  })

  it('桶线索数未知：部分未知只用已知部分，全未知给 null 不冒 0', () => {
    const tier = normalize([
      laneRaw(1, 'A', [
        { date: '2026-10-23', sections: [sectionRaw(1, { thread_count: 5 }), sectionRaw(2, { events: null })] },
        { date: '2026-10-22', sections: [sectionRaw(3, { events: null })] },
      ]),
    ])
    const series = buildPoolSeries({ start: '2026-10-22', end: '2026-10-23' }, ['1'], makeResolver(tier))
    expect(series[0]).toEqual({ date: '2026-10-22', value: null })
    expect(series[1]).toEqual({ date: '2026-10-23', value: 5 })
  })

  // ── 展示降级口径（matrix-clue-fallback-and-rail-merge D2）：曲线与矩阵格同口径 ──
  it('曲线与矩阵格同口径：story_count=0 的日子用线索合计兜底，story_count>0 用事件线数（不混加）', () => {
    const tier = normalize([
      laneRaw(1, 'A', [
        // 历史未回填日：事件线 0、线索 132 → 曲线 132（与矩阵格一致）
        { date: '2026-10-22', sections: [sectionRaw(1, { thread_count: 100 }), sectionRaw(2, { thread_count: 32 })], story_count: 0 },
        // 已回填日：事件线 3、线索 132 → 曲线 3（不用线索合计）
        { date: '2026-10-23', sections: [sectionRaw(3, { thread_count: 132 })], story_count: 3 },
      ]),
    ])
    const series = buildPoolSeries({ start: '2026-10-22', end: '2026-10-23' }, ['1'], makeResolver(tier))
    expect(series[0]).toEqual({ date: '2026-10-22', value: 132 })
    expect(series[1]).toEqual({ date: '2026-10-23', value: 3 })
  })

  it('story_count=0 且线索全未知：曲线该日仍未知（null 不补 0，不造兜底数）', () => {
    const tier = normalize([
      laneRaw(1, 'A', [
        { date: '2026-10-23', sections: [sectionRaw(1, { events: null })], story_count: 0 },
      ]),
    ])
    const series = buildPoolSeries({ start: '2026-10-23', end: '2026-10-23' }, ['1'], makeResolver(tier))
    expect(series[0]).toEqual({ date: '2026-10-23', value: null })
  })

  it('story_count 已知而桶线索未知：该日照常计入（值可判即不算未知日）', () => {
    const tier = normalize([
      laneRaw(1, 'A', [
        { date: '2026-10-23', sections: [sectionRaw(1, { events: null })], story_count: 7 },
      ]),
    ])
    const series = buildPoolSeries({ start: '2026-10-23', end: '2026-10-23' }, ['1'], makeResolver(tier))
    expect(series[0]).toEqual({ date: '2026-10-23', value: 7 })
  })

  it('A 名单为空：逐日 0（窗口已知无活动），不产生 null', () => {
    const tier = normalize([])
    const series = buildPoolSeries({ start: '2026-10-22', end: '2026-10-23' }, [], makeResolver(tier))
    expect(series).toEqual([
      { date: '2026-10-22', value: 0 },
      { date: '2026-10-23', value: 0 },
    ])
  })
})

describe('laneConsoleView — 窄屏迷你条口径', () => {
  it('content 取分区数（含 0 线索桶），gap/unavailable 不补 0', () => {
    const tier = normalize([
      laneRaw(1, 'A', [{ date: '2026-10-23', sections: [sectionRaw(1, { thread_count: 0 }), sectionRaw(2, { thread_count: 0 })] }]),
    ])
    const resolve = makeResolver(tier)
    expect(narrowDaySectionCount(resolve, '1', '2026-10-23')).toBe(2)
    expect(narrowDaySectionCount(resolve, '1', '2026-10-22')).toBeNull()
    expect(narrowDaySectionCount(resolve, '1', '2026-10-01')).toBeNull()
  })
})

// ── 8C G1：日报日驱动的选中日校正（test-cases RDY 组） ─────────────────────

describe('laneConsoleView — 报日集合与选中日三级校正（RDY）', () => {
  const tier = normalize([
    laneRaw(1204, '主线', [
      { date: '2026-10-20', sections: [sectionRaw(1, { thread_count: 2 })] },
      { date: '2026-10-22', sections: [sectionRaw(2, { thread_count: 0 })] },
    ]),
  ])
  const lane = buildConsoleLaneGroups(tier, AXIS14).rows[0]!
  const resolve = makeResolver(tier)
  const axis = ['2026-10-15', '2026-10-16', '2026-10-20', '2026-10-22', '2026-10-23', '2026-10-24']
  const reports = new Set(['2026-10-16', '2026-10-22', '2026-10-23', '2026-10-24'])

  it('RDY-1 零泳道：选中窗内最新报日（不是 today、不是空态）', () => {
    expect(reconcileSelectedDate(null, axis, resolve, null, reports)).toBe('2026-10-24')
  })

  it('RDY-2 有泳道初始化：仍是既有 CH-1「最后有分区日」，不被报日抢走', () => {
    expect(reconcileSelectedDate(lane, axis, resolve, null, reports)).toBe('2026-10-22')
  })

  it('RDY-3 保留用户报日：泳道当日无分区也不搬回内容日', () => {
    expect(reconcileSelectedDate(lane, axis, resolve, '2026-10-24', reports)).toBe('2026-10-24')
  })

  it('RDY-4 保留内容日：索引 lookback 未含该日也不搬走', () => {
    const narrowIndex = new Set(['2026-10-23'])
    expect(reconcileSelectedDate(lane, axis, resolve, '2026-10-20', narrowIndex)).toBe('2026-10-20')
  })

  it('RDY-5 范围排出：回新窗内最新报日（24），不是最近/等距的 16', () => {
    const newAxis = ['2026-10-15', '2026-10-16', '2026-10-17', '2026-10-22', '2026-10-23', '2026-10-24']
    expect(reconcileSelectedDate(lane, newAxis, resolve, '2026-10-12', reports)).toBe('2026-10-24')
  })

  it('RDY-6 排出后窗内无报日：null 空态（不跳 today/active 日）', () => {
    const newAxis = ['2026-10-15', '2026-10-16', '2026-10-17']
    expect(reconcileSelectedDate(lane, newAxis, resolve, '2026-10-12', new Set(['2026-10-01']))).toBeNull()
    expect(reconcileSelectedDate(null, newAxis, resolve, '2026-10-12', new Set())).toBeNull()
  })

  it('RDY-7 索引未就绪：暂 null（不误报无日报）；索引到达后回 RDY-1', () => {
    expect(reconcileSelectedDate(null, axis, resolve, null, null)).toBeNull()
    expect(reconcileSelectedDate(null, axis, resolve, null, undefined)).toBeNull()
    expect(reconcileSelectedDate(null, axis, resolve, null, reports)).toBe('2026-10-24')
  })

  it('RDY-9 报日集合裁切到可见轴：窗外报日不当回退候选', () => {
    const windowAxis = ['2026-10-20', '2026-10-21', '2026-10-22']
    // 10/24 在索引内但不在轴内 → 只能回窗内 10/22
    expect(reconcileSelectedDate(null, windowAxis, resolve, null, reports)).toBe('2026-10-22')
    expect(reconcileSelectedDate(lane, windowAxis, resolve, '2026-10-25', reports)).toBe('2026-10-22')
  })

  it('RDY-10 切泳道：当前报日保留（编年空态不回跳）', () => {
    expect(reconcileSelectedDate(lane, axis, resolve, '2026-10-23', reports)).toBe('2026-10-23')
  })

  it('索引未知但当前日仍在轴内：不因暂未知把选择清掉', () => {
    expect(reconcileSelectedDate(lane, axis, resolve, '2026-10-23', null)).toBe('2026-10-23')
  })

  it('reportDaysInAxis：只取轴内报日、保持轴序；索引未知回空数组', () => {
    expect(reportDaysInAxis(axis, reports)).toEqual(['2026-10-16', '2026-10-22', '2026-10-23', '2026-10-24'])
    expect(reportDaysInAxis(['2026-10-20'], reports)).toEqual([])
    expect(reportDaysInAxis(axis, null)).toEqual([])
  })
})

// ── 8E review R-41：已验证记录日（观察带真实同 ID 记录）不被普通回落搬走 ──────

describe('laneConsoleView — 已验证记录日保护（R41）', () => {
  const tier = normalize([
    laneRaw(1204, '主线', [
      { date: '2026-10-20', sections: [sectionRaw(1, { thread_count: 2 })] },
      { date: '2026-10-22', sections: [sectionRaw(2, { thread_count: 0 })] },
    ]),
  ])
  const lane = buildConsoleLaneGroups(tier, AXIS14).rows[0]!
  const resolve = makeResolver(tier)
  const axis = ['2026-10-15', '2026-10-16', '2026-10-20', '2026-10-21', '2026-10-22', '2026-10-23']
  const reports = new Set(['2026-10-16', '2026-10-22', '2026-10-23'])

  it('R41-1 索引缺该日但它是已验证记录日：保留（不被回落到内容日/最新报日）', () => {
    expect(reconcileSelectedDate(lane, axis, resolve, '2026-10-21', reports)).toBe('2026-10-22')
    expect(reconcileSelectedDate(lane, axis, resolve, '2026-10-21', reports, '2026-10-21')).toBe('2026-10-21')
  })

  it('R41-2 未接线/无已验证目标/保护别的日子：普通回落语义不变', () => {
    expect(reconcileSelectedDate(lane, axis, resolve, '2026-10-21', reports, null)).toBe('2026-10-22')
    expect(reconcileSelectedDate(lane, axis, resolve, '2026-10-21', reports, '2026-10-20')).toBe('2026-10-22')
  })

  it('R41-3 范围排出（轴外）：保护不越过轴边界，仍回窗内最新报日', () => {
    expect(reconcileSelectedDate(lane, axis, resolve, '2026-10-12', reports, '2026-10-12')).toBe('2026-10-23')
  })

  it('R41-4 索引未就绪/失败时保护与旧口径一致（不误报无日报、不搬走）', () => {
    expect(reconcileSelectedDate(lane, axis, resolve, '2026-10-21', null, '2026-10-21')).toBe('2026-10-21')
    expect(reconcileSelectedDate(lane, axis, resolve, '2026-10-21', null)).toBe('2026-10-21')
  })
})
