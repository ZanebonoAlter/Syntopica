/**
 * observationLanes 纯模型测试（矩阵阶段 G3，test-cases-matrix-stage.md MX/EV 组纯逻辑半）。
 *
 *  - source 全轴行序/休眠唯一基准（OX-11 口径）：clueTotal 降序 → sectionTotal 降序 → ID 升序
 *  - B 池独有 ID 不入名单；观察轴宽度变化永不重算名单/休眠/候选
 *  - 矩阵格视图三态（content 峰值/回退/未知、gap '—'、unavailable '?'）
 *  - 演进点径公式 / 节点投影 / series 值（gap=已知 0、未知=null 不补 0）
 */
import { describe, expect, it } from 'vitest'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'
import { normalizeLaneDynamicsWindow, type LaneDynamicsWindowData, type WindowLaneDay, type WindowSection } from '~/api/normalizers/laneDynamicsWindow'
import { buildDayAxis } from '~/features/tags/composables/laneMatrixModel'
import {
  buildObservationRows,
  compareTopicId,
  evolutionDotSize,
  evolutionSeriesValue,
  firstThreadTitle,
  matrixCellView,
  projectEvolutionNodes,
  projectMatrixCell,
  splitObservationRows,
  type LaneDayResolver,
} from './observationLanes'

const ANCHOR = '2026-10-23'
const AXIS14 = buildDayAxis(ANCHOR, 14)
const TIER_START = '2026-10-10'

// ── fixtures：raw 响应 → 真实 normalizer 归一（链路与 G1 对齐） ──

function sectionRaw(id: number, overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return { section_id: id, label: `桶${id}`, events: [`事件${id}`], ...overrides }
}

function laneRaw(topicId: number, label: string, timeline: { date: string, sections: Record<string, unknown>[] }[]): Record<string, unknown> {
  return { topic_id: topicId, label, watch_linked: false, section_count_14d: 0, snapshot: null, timeline }
}

function normalize(lanes: Record<string, unknown>[], days = 14): LaneDynamicsWindowData {
  return normalizeLaneDynamicsWindow({ window_days: days, has_reports: true, lanes, candidates: [] } as unknown as LaneDynamicsResponse)
}

function windowDay(date: string, sections: Partial<WindowSection>[]): WindowLaneDay {
  return {
    date,
    sections: sections.map((section, index) => ({
      sectionId: section.sectionId ?? String(index + 1),
      label: section.label ?? `桶${index + 1}`,
      threadCount: section.threadCount ?? null,
      fallbackClueCount: section.fallbackClueCount ?? null,
      foldedCount: section.foldedCount ?? 0,
      eventTitles: section.eventTitles ?? [],
    })),
  }
}

/** 档/池双索引 resolver（对齐 G1 resolveLaneDay 的三态语义；池未传 = unavailable）。 */
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

describe('observationLanes — source 全轴行模型', () => {
  it('行序 = clueTotal 降序 → sectionTotal 降序 → ID 升序（字符串 ID 按数值比，OX-11）', () => {
    const source = normalize([
      // 线索 10 / 分桶 2；线索 10 / 分桶 4（并列线索 → 分桶多在前）；线索 10 / 分桶 4 / ID 9 与 10（并列 → ID 升序）
      laneRaw(11, 'B', [{ date: '2026-10-23', sections: [sectionRaw(1, { thread_count: 10 })] }]),
      laneRaw(10, 'A', [
        { date: '2026-10-22', sections: [sectionRaw(2, { thread_count: 3 }), sectionRaw(3, { thread_count: 3 })] },
        { date: '2026-10-23', sections: [sectionRaw(4, { thread_count: 2 }), sectionRaw(5, { thread_count: 2 })] },
      ]),
      laneRaw(9, 'C', [
        { date: '2026-10-22', sections: [sectionRaw(6, { thread_count: 2 }), sectionRaw(7, { thread_count: 2 })] },
        { date: '2026-10-23', sections: [sectionRaw(8, { thread_count: 3 }), sectionRaw(9, { thread_count: 3 })] },
      ]),
      laneRaw(12, 'D', [{ date: '2026-10-21', sections: [sectionRaw(10, { thread_count: 1 })] }]),
    ])
    const rows = buildObservationRows(source, AXIS14)
    expect(rows.map(row => row.topicId)).toEqual(['9', '10', '11', '12'])
  })

  it('休眠边界：source 全窗分区 ≤2 休眠、=3 活跃（阈值含等号）', () => {
    const source = normalize([
      laneRaw(1, '两桶', [{ date: '2026-10-23', sections: [sectionRaw(1), sectionRaw(2)] }]),
      laneRaw(2, '三桶', [
        { date: '2026-10-22', sections: [sectionRaw(3)] },
        { date: '2026-10-23', sections: [sectionRaw(4), sectionRaw(5)] },
      ]),
    ])
    const { active, dormant } = splitObservationRows(buildObservationRows(source, AXIS14))
    expect(dormant.map(row => row.topicId)).toEqual(['1'])
    expect(active.map(row => row.topicId)).toEqual(['2'])
  })

  it('池独有 ID 不入名单；观察轴缩放（恰 7/30/365）不重算行序/休眠/候选（OX-1/OX-2）', () => {
    const source = normalize([
      laneRaw(10, 'A', [{ date: '2026-10-23', sections: [sectionRaw(1, { thread_count: 5 })] }]),
      laneRaw(11, 'B', [{ date: '2026-10-20', sections: [sectionRaw(2, { thread_count: 1 })] }]),
    ])
    const pool = normalize([
      laneRaw(10, 'A', [{ date: '2026-06-15', sections: [sectionRaw(3, { thread_count: 6 })] }]),
      laneRaw(99, '池独有', [{ date: '2026-06-15', sections: [sectionRaw(4, { thread_count: 9 })] }]),
    ], 365)
    const resolve = makeResolver(source, pool)
    const snapshot = buildObservationRows(source, AXIS14)
    const candidatesBefore = source.candidates.map(candidate => candidate.topicId)
    for (const width of [7, 14, 30, 365]) {
      const axis = buildDayAxis(ANCHOR, width)
      // 观察投影只作用可见轴；名单/行序/休眠与轴宽无关
      for (const row of snapshot) {
        for (const date of axis) projectMatrixCell(resolve, row.topicId, date)
      }
      const rows = buildObservationRows(source, AXIS14)
      expect(rows.map(row => [row.topicId, row.clueTotal, row.sectionTotal, row.dormant])).toEqual(
        snapshot.map(row => [row.topicId, row.clueTotal, row.sectionTotal, row.dormant]),
      )
      expect(rows.some(row => row.topicId === '99')).toBe(false)
    }
    expect(source.candidates.map(candidate => candidate.topicId)).toEqual(candidatesBefore)
  })

  it('thread_count 缺省回退 events+folded，再缺按 0 计（与 legacy 排序口径对齐）', () => {
    const source = normalize([
      laneRaw(1, '回退', [{ date: '2026-10-23', sections: [
        sectionRaw(1, { events: ['a', 'b'], folded_count: 1 }),
        sectionRaw(2, { thread_count: 2 }),
      ] }]),
      laneRaw(2, '更大', [{ date: '2026-10-23', sections: [sectionRaw(3, { thread_count: 6 })] }]),
    ])
    const rows = buildObservationRows(source, AXIS14)
    expect(rows[0]).toMatchObject({ topicId: '2', clueTotal: 6 })
    expect(rows[1]).toMatchObject({ topicId: '1', clueTotal: 5 })
  })

  it('compareTopicId：数字串按数值升序（9 < 10），非数字回退字典序', () => {
    expect(compareTopicId('9', '10')).toBeLessThan(0)
    expect(compareTopicId('10', '9')).toBeGreaterThan(0)
    expect(compareTopicId('10', '10')).toBe(0)
    expect(compareTopicId('a', 'b')).toBeLessThan(0)
  })
})

describe('observationLanes — 矩阵格视图（MX 组）', () => {
  const resolve: LaneDayResolver = (topicId, date) =>
    topicId === '10' && date === '2026-10-23'
      ? { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 3 }, { threadCount: 7 }, { threadCount: 1 }]) }
      : topicId === '10' && date === '2026-10-22'
        ? { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: null, fallbackClueCount: 3 }]) }
        : date >= TIER_START
          ? { kind: 'gap' }
          : { kind: 'unavailable' }

  it('MX-1：格值 = 同日各桶线索 max（3/7/1 → 7，非合计非桶数）；三指标 aria', () => {
    const view = matrixCellView(projectMatrixCell(resolve, '10', '2026-10-23'), { laneLabel: '主线', date: '2026-10-23' })
    expect(view.text).toBe('7')
    expect(view.openable).toBe(true)
    expect(view.bucket).toBe('is-b4')
    expect(view.aria).toContain('桶峰值 7 条')
    expect(view.aria).toContain('3 个主题桶')
    expect(view.aria).toContain('当日合计 11 条')
  })

  it('MX-2：thread_count 缺省回退 events+folded；两者皆缺显示 ?（未知不补数、仍可开）', () => {
    const fallback = matrixCellView(projectMatrixCell(resolve, '10', '2026-10-22'), { laneLabel: '主线', date: '2026-10-22' })
    expect(fallback.text).toBe('3')
    const bare = matrixCellView(
      { kind: 'content', metrics: { peak: null, total: 0, sectionCount: 2, hasUnknown: true } },
      { laneLabel: '主线', date: '2026-10-22' },
    )
    expect(bare.text).toBe('?')
    expect(bare.openable).toBe(true)
    expect(bare.aria).toContain('未知')
    expect(bare.bucket).toBe('')
  })

  it('MX-3/MX-4：有分区 0 线索 = 已知 0 可开；gap = 「—」 禁用；unavailable = 「?」 禁用', () => {
    const zero = matrixCellView(
      { kind: 'content', metrics: { peak: 0, total: 0, sectionCount: 1, hasUnknown: false } },
      { laneLabel: '主线', date: '2026-10-21' },
    )
    expect(zero.text).toBe('0')
    expect(zero.openable).toBe(true)

    const gap = matrixCellView(projectMatrixCell(resolve, '10', '2026-10-15'), { laneLabel: '主线', date: '2026-10-15' })
    expect(gap.text).toBe('—')
    expect(gap.openable).toBe(false)
    expect(gap.aria).toContain('无分区')

    const unknown = matrixCellView(projectMatrixCell(resolve, '10', '2026-10-01'), { laneLabel: '主线', date: '2026-10-01' })
    expect(unknown.text).toBe('?')
    expect(unknown.openable).toBe(false)
    expect(unknown.aria).toContain('未取到')
  })
})

describe('observationLanes — 演进投影（EV 组）', () => {
  it('EV-1：点径公式 min(24, 6+√total×5)：0→6、1→11、4→16、16→封顶 24、≥36→24', () => {
    expect(evolutionDotSize(0)).toBe(6)
    expect(evolutionDotSize(1)).toBeCloseTo(11, 5)
    expect(evolutionDotSize(2)).toBeCloseTo(13.071, 2)
    expect(evolutionDotSize(3)).toBeCloseTo(14.66, 1)
    expect(evolutionDotSize(4)).toBe(16)
    expect(evolutionDotSize(16)).toBe(24)
    expect(evolutionDotSize(36)).toBe(24)
  })

  it('EV-2/EV-3：节点合计 = 当日 total（非峰值）；标签阈值 ≥3；全未知合计为 null', () => {
    const resolve: LaneDayResolver = (topicId, date) => {
      if (topicId !== '10') return { kind: 'gap' }
      if (date === '2026-10-23') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 3 }, { threadCount: 1 }]) }
      if (date === '2026-10-22') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 2 }]) }
      if (date === '2026-10-21') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: null, fallbackClueCount: null }]) }
      if (date === '2026-10-20') return { kind: 'gap' }
      return { kind: 'unavailable' }
    }
    const nodes = projectEvolutionNodes('10', ['2026-10-23', '2026-10-22', '2026-10-21', '2026-10-20', '2026-10-19'], resolve, '2026-10-23')
    expect(nodes[0]).toMatchObject({ kind: 'content', total: 4, showLabel: true, isToday: true })
    expect(nodes[1]).toMatchObject({ kind: 'content', total: 2, showLabel: false })
    expect(nodes[2]).toMatchObject({ kind: 'content', total: null, showLabel: false })
    expect(nodes[3]).toMatchObject({ kind: 'gap', total: 0 })
    expect(nodes[4]).toMatchObject({ kind: 'unavailable', total: null })
  })

  it('首线索标题 = 第一桶首条事件标题，无事件回退桶 label', () => {
    expect(firstThreadTitle(windowDay('d', [{ eventTitles: ['甲', '乙'] }, { eventTitles: ['丙'] }]))).toBe('甲')
    expect(firstThreadTitle(windowDay('d', [{ eventTitles: [], label: '分区题' }]))).toBe('分区题')
    expect(firstThreadTitle(windowDay('d', []))).toBe('')
  })

  it('series 值：content=已知合计（全未知 null）、gap=已知 0、unavailable=null（不补 0）', () => {
    expect(evolutionSeriesValue({ kind: 'content', source: 'tier', day: windowDay('d', [{ threadCount: 3 }, { threadCount: 4 }]) })).toBe(7)
    expect(evolutionSeriesValue({ kind: 'content', source: 'tier', day: windowDay('d', [{ threadCount: null, fallbackClueCount: null }]) })).toBeNull()
    expect(evolutionSeriesValue({ kind: 'content', source: 'pool', day: windowDay('d', [{ threadCount: 0 }]) })).toBe(0)
    expect(evolutionSeriesValue({ kind: 'gap' })).toBe(0)
    expect(evolutionSeriesValue({ kind: 'unavailable' })).toBeNull()
  })
})
