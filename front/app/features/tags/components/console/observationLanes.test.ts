/**
 * observationLanes 纯模型测试（矩阵阶段 G3 + retire-story-adopt-event-metrics 3.1 线索口径）。
 *
 *  - source 全轴行序/休眠唯一基准：lane_heat 降序 → 全窗线索合计降序 → ID 升序（与后端一致）；
 *    休眠分组仍用 sectionTotal 阈值
 *  - B 池独有 ID 不入名单；观察轴宽度变化永不重算名单/休眠/候选
 *  - 矩阵格视图三态（content = 当日线索合计 / 未知 '?'、gap '—'、unavailable '?'）；
 *    事件线量纲与最热事件线点名整体退役（D5）
 *  - 矩阵色深（2026-11-11 用户看版后修订）：当日线索合计在本行可见窗内归一（0 = 不着色、
 *    行内峰值 = 1、线性插值）；跨行不归一，0/null/非正分母不着色；混入比例单调
 *  - 演进点径公式 / 节点投影（节点数字 = 当日线索合计）/ series 值（gap=已知 0、未知=null 不补 0）
 */
import { describe, expect, it } from 'vitest'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'
import { normalizeLaneDynamicsWindow, type LaneDynamicsWindowData, type WindowLaneDay, type WindowSection } from '~/api/normalizers/laneDynamicsWindow'
import { buildDayAxis } from '~/features/tags/composables/laneMatrixModel'
import {
  CLUE_DEPTH_MAX_MIX,
  buildObservationRows,
  clueDepth,
  clueMix,
  clueRowMax,
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
  // composite_count 默认 0（无综合稿）：桶线索数 = thread_count − composite_count = thread_count。
  return { section_id: id, label: `桶${id}`, events: [`事件${id}`], composite_count: 0, ...overrides }
}

function laneRaw(topicId: number, label: string, timeline: { date: string, sections: Record<string, unknown>[], clue_count?: number }[], overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return { topic_id: topicId, label, watch_linked: false, section_count_14d: 0, snapshot: null, timeline, ...overrides }
}

function normalize(lanes: Record<string, unknown>[], days = 14): LaneDynamicsWindowData {
  return normalizeLaneDynamicsWindow({ window_days: days, has_reports: true, lanes, candidates: [] } as unknown as LaneDynamicsResponse)
}

function windowDay(date: string, sections: Partial<WindowSection>[], day: Partial<WindowLaneDay> = {}): WindowLaneDay {
  return {
    date,
    sections: sections.map((section, index) => ({
      sectionId: section.sectionId ?? String(index + 1),
      label: section.label ?? `桶${index + 1}`,
      threadCount: section.threadCount ?? null,
      compositeCount: section.compositeCount ?? 0,
      // 桶线索数 = thread_count − composite_count（唯一公式）；threadCount 缺失 → 未知。
      clueCount: section.clueCount
        ?? (section.threadCount === undefined || section.threadCount === null
          ? null
          : Math.max(0, section.threadCount - (section.compositeCount ?? 0))),
      foldedCount: section.foldedCount ?? 0,
      eventTitles: section.eventTitles ?? [],
    })),
    ...day,
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
  it('行序 = lane_heat 降序 → 全窗线索合计降序 → ID 升序（与后端一致）', () => {
    const source = normalize([
      // 线索 10 / 无 heat；线索 10 / heat 2；线索 4 / heat 7（heat 最高在前）；
      // heat 2 并列 → 线索多者在前；再并列（线索 10/10）→ ID 升序
      laneRaw(11, 'B', [{ date: '2026-10-23', sections: [sectionRaw(1, { thread_count: 10 })] }]),
      laneRaw(10, 'A', [
        { date: '2026-10-22', sections: [sectionRaw(2, { thread_count: 3 }), sectionRaw(3, { thread_count: 3 })] },
        { date: '2026-10-23', sections: [sectionRaw(4, { thread_count: 2 }), sectionRaw(5, { thread_count: 2 })] },
      ], { lane_heat: 2 }),
      laneRaw(9, 'C', [
        { date: '2026-10-22', sections: [sectionRaw(6, { thread_count: 2 }), sectionRaw(7, { thread_count: 2 })] },
      ], { lane_heat: 2 }),
      laneRaw(12, 'D', [{ date: '2026-10-21', sections: [sectionRaw(8, { thread_count: 4 })] }], { lane_heat: 7 }),
    ])
    const rows = buildObservationRows(source, AXIS14)
    expect(rows.map(row => row.topicId)).toEqual(['12', '10', '9', '11'])
  })

  it('lane_heat 全缺（旧后端）：线索合计降序 → ID 升序；clueTotal 回退各桶已知非综合稿线索合计', () => {
    const source = normalize([
      laneRaw(1, '回退', [{ date: '2026-10-23', sections: [
        // 缺 thread_count 的桶不可数（不拿 events+folded 冒充线索数）
        sectionRaw(1, { events: ['a', 'b'], folded_count: 1, thread_count: undefined, composite_count: undefined }),
        sectionRaw(2, { thread_count: 2 }),
      ] }]),
      laneRaw(2, '更大', [{ date: '2026-10-23', sections: [sectionRaw(3, { thread_count: 6 })] }]),
    ])
    const rows = buildObservationRows(source, AXIS14)
    expect(rows[0]).toMatchObject({ topicId: '2', clueTotal: 6, laneHeat: 0 })
    expect(rows[1]).toMatchObject({ topicId: '1', clueTotal: 2 })
  })

  it('当日线索合计优先消费 clue_count（各桶 thread 合计不覆盖）；lane_heat 透传', () => {
    const raw = {
      topic_id: 3, label: '显式', watch_linked: false, section_count_14d: 1, snapshot: null,
      lane_heat: 1.5,
      timeline: [{
        date: '2026-10-23',
        clue_count: 2,
        sections: [sectionRaw(1, { thread_count: 9 })],
      }],
    }
    const source = normalize([raw])
    expect(source.lanes[0]!.timeline[0]!.clueCount).toBe(2)
    const rows = buildObservationRows(source, AXIS14)
    expect(rows[0]).toMatchObject({
      topicId: '3',
      laneHeat: 1.5,
      clueTotal: 2,
      clueCounts: { '2026-10-23': 2 },
    })
  })

  it('全窗线索合计优先消费 clue_count，显式 0 不被各桶线索合计覆盖', () => {
    const source = normalize([
      laneRaw(2, '零线索', [{ date: ANCHOR, clue_count: 0, sections: [sectionRaw(1, { thread_count: 99 })] }], { lane_heat: 1 }),
      laneRaw(1, '两条', [{ date: ANCHOR, clue_count: 2, sections: [sectionRaw(2, { thread_count: 1 })] }], { lane_heat: 1 }),
    ])
    const rows = buildObservationRows(source, AXIS14)
    expect(rows.map(row => [row.topicId, row.clueTotal])).toEqual([['1', 2], ['2', 0]])
    expect(rows[1]!.clueCounts).toEqual({ [ANCHOR]: 0 })
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

  it('桶线索数 = thread_count − composite_count（综合稿不计）；缺 composite_count → 不可数', () => {
    const source = normalize([
      laneRaw(1, '含综合稿', [{ date: '2026-10-23', sections: [
        sectionRaw(1, { thread_count: 5, composite_count: 2 }),
        sectionRaw(2, { thread_count: 2, composite_count: 0 }),
      ] }]),
      laneRaw(2, '更大', [{ date: '2026-10-23', sections: [sectionRaw(3, { thread_count: 6 })] }]),
    ])
    const rows = buildObservationRows(source, AXIS14)
    expect(rows[0]).toMatchObject({ topicId: '2', clueTotal: 6 })
    expect(rows[1]).toMatchObject({ topicId: '1', clueTotal: 5 })

    // 旧响应缺 composite_count → 桶线索数不可数（不拿 thread_count 冒充）。
    const legacy = normalize([
      laneRaw(1, '旧响应', [{ date: '2026-10-23', sections: [
        sectionRaw(1, { thread_count: 3, composite_count: undefined }),
      ] }]),
    ])
    const legacyRows = buildObservationRows(legacy, AXIS14)
    expect(legacyRows[0]!.clueTotal).toBe(0)
  })

  it('compareTopicId：数字串按数值升序（9 < 10），非数字回退字典序', () => {
    expect(compareTopicId('9', '10')).toBeLessThan(0)
    expect(compareTopicId('10', '9')).toBeGreaterThan(0)
    expect(compareTopicId('10', '10')).toBe(0)
    expect(compareTopicId('a', 'b')).toBeLessThan(0)
  })
})

describe('observationLanes — 矩阵格视图（MX 组，线索口径）', () => {
  const resolve: LaneDayResolver = (topicId, date) =>
    topicId === '10' && date === '2026-10-23'
      ? {
          kind: 'content', source: 'tier',
          day: windowDay(date, [{ threadCount: 3 }, { threadCount: 7 }, { threadCount: 1 }], { clueCount: 11 }),
        }
      : topicId === '10' && date === '2026-10-22'
        ? { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: null }]) }
        : date >= TIER_START
          ? { kind: 'gap' }
          : { kind: 'unavailable' }

  it('MX-1：格值 = 当日线索合计（同日多桶合并，非桶峰值）；aria 给「N 条线索 · M 个主题桶」', () => {
    const view = matrixCellView(projectMatrixCell(resolve, '10', '2026-10-23'), { laneLabel: '主线', date: '2026-10-23' })
    expect(view.text).toBe('11')
    expect(view.openable).toBe(true)
    expect(view.aria).toContain('11 条线索')
    expect(view.aria).toContain('3 个主题桶')
    expect(view.aria).not.toContain('事件线')
    expect(view.aria).not.toContain('个来源')
    expect(view.title).toBe(view.aria)
  })

  // ── 展示口径（retire-story-adopt-event-metrics D4/D5）────────────────────
  it('MX-5：历史日无 clue_count 时直接显示各桶线索合计，无「兜底」降级说明', () => {
    const degradedResolve: LaneDayResolver = (topicId, date) =>
      topicId === '10' && date === '2026-10-23'
        ? { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 100 }, { threadCount: 32 }]) }
        : { kind: 'gap' }
    const view = matrixCellView(projectMatrixCell(degradedResolve, '10', '2026-10-23'), { laneLabel: '主线', date: '2026-10-23' })
    expect(view.text).toBe('132')
    expect(view.openable).toBe(true)
    expect(view.aria).toContain('132 条线索')
    expect(view.aria).not.toContain('兜底')
    expect(view.aria).not.toContain('事件线')
  })

  it('MX-6：真零线索格值 0、可开、无降级尾注，不与断档混淆', () => {
    const zeroResolve: LaneDayResolver = (topicId, date) =>
      topicId === '10' && date === '2026-10-23'
        ? { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 0 }], { clueCount: 0 }) }
        : { kind: 'gap' }
    const view = matrixCellView(projectMatrixCell(zeroResolve, '10', '2026-10-23'), { laneLabel: '主线', date: '2026-10-23' })
    expect(view.text).toBe('0')
    expect(view.kind).toBe('content')
    expect(view.openable).toBe(true)
    expect(view.aria).toContain('0 条线索')
    expect(view.aria).not.toContain('兜底')
  })

  it('MX-7：伴生数字与格值同口径（综合稿只以差额出现，thread 总数不得当线索数）', () => {
    const peakResolve: LaneDayResolver = (topicId, date) =>
      topicId === '10' && date === '2026-10-23'
        ? {
            kind: 'content', source: 'tier',
            day: windowDay(date, [{ threadCount: 5, compositeCount: 2 }], { clueCount: 3 }),
          }
        : { kind: 'gap' }
    const view = matrixCellView(projectMatrixCell(peakResolve, '10', '2026-10-23'), { laneLabel: '主线', date: '2026-10-23' })
    expect(view.text).toBe('3')
    expect(view.aria).toContain('3 条线索')
    expect(view.aria).toContain('1 个主题桶')
    // 综合稿只以差额形式说明；含综合稿的 thread 总数 5 不得表述为「线索条数」。
    expect(view.aria).toContain('另有 2 条综合稿未计入')
    expect(view.aria).not.toContain('5 条线索')
    expect(view.aria).not.toContain('最多一天桶')
  })

  it('MX-2：线索数未知显示「?」不造 0；旧响应缺 composite_count 时桶线索数不可数', () => {
    const fallback = matrixCellView(projectMatrixCell(resolve, '10', '2026-10-22'), { laneLabel: '主线', date: '2026-10-22' })
    expect(fallback.text).toBe('?')
    expect(fallback.aria).toContain('线索数未知')

    const bare = matrixCellView(
      {
        kind: 'content',
        metrics: { clueCount: null, total: 0, peak: null, sectionCount: 2, hasUnknown: true, compositeCount: null, bucketClues: [], clueCountSource: 'none', isLowerBound: false },
      },
      { laneLabel: '主线', date: '2026-10-22' },
    )
    // 有分区即可开；但数字未知时不冒充真实 0。
    expect(bare.text).toBe('?')
    expect(bare.openable).toBe(true)
    expect(bare.aria).toContain('线索数未知')
    expect(bare.aria).not.toContain('0 条线索')
    expect(bare.aria).toContain('2 个主题桶')
  })

  it('MX-9：旧响应回退 + 部分桶不可数 → 格值与 aria 都写下界「≥N」，不冒充完整合计', () => {
    const partialResolve: LaneDayResolver = (topicId, date) =>
      topicId === '10' && date === '2026-10-23'
        ? {
            kind: 'content', source: 'tier',
            // 桶1 已知 3、桶2 缺 composite_count（不可数）
            day: windowDay(date, [{ threadCount: 3 }, { threadCount: null, clueCount: null }]),
          }
        : { kind: 'gap' }
    const view = matrixCellView(projectMatrixCell(partialResolve, '10', '2026-10-23'), { laneLabel: '主线', date: '2026-10-23' })
    expect(view.text).toBe('≥3')
    expect(view.aria).toContain('≥3 条线索')
    // 有未知桶时不列「各桶 …」（否则与合计/桶数对不上）
    expect(view.aria).not.toContain('各桶')
    expect(view.aria).toContain('2 个主题桶')
    expect(view.openable).toBe(true)
  })

  it('MX-3/MX-4：有分区 0 线索 = 已知 0 可开；gap = 「—」 禁用；unavailable = 「?」 禁用', () => {
    const zero = matrixCellView(
      { kind: 'content', metrics: { clueCount: 0, total: 0, peak: 0, sectionCount: 1, hasUnknown: false, compositeCount: 0, bucketClues: [0], clueCountSource: 'backend', isLowerBound: false } },
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

  it('格视图只带数值/深度/aria（无 bucket）：色深由 rowMax 行内归一，不在视图里塞归属类名', () => {
    const cell = projectMatrixCell(resolve, '10', '2026-10-23')
    const view = matrixCellView(cell, { laneLabel: '主线', date: '2026-10-23', rowMax: 11 })
    expect(Object.keys(view).sort()).toEqual(['aria', 'depth', 'kind', 'openable', 'text', 'title'])
    expect(view.depth).toBe(1)
    expect(matrixCellView(cell, { laneLabel: '主线', date: '2026-10-23', rowMax: 22 }).depth).toBe(0.5)
    expect(view.title).not.toContain('最热')
    expect(view.title).not.toContain('个来源')
    // 未知/断档格 depth 恒 0（不造 0、不着色），即使传了分母
    const unknown = matrixCellView({ kind: 'content', metrics: { clueCount: null, total: 0, peak: null, sectionCount: 2, hasUnknown: true, compositeCount: null, bucketClues: [], clueCountSource: 'none', isLowerBound: false } }, { laneLabel: '主线', date: '2026-10-22', rowMax: 11 })
    expect(unknown.depth).toBe(0)
    const gap = matrixCellView({ kind: 'gap' }, { laneLabel: '主线', date: '2026-10-15', rowMax: 11 })
    expect(gap.depth).toBe(0)
  })
})

// ── 矩阵色深（2026-11-11 用户看版后修订，tasks 3.5）───────────────────────

describe('observationLanes — 矩阵色深（行内归一连续深浅）', () => {
  it('线性单调：同格线索越多越深；行内峰值 = 1，其余按比例插值', () => {
    const rowMax = 10
    const depths = [1, 4, 7, 10].map(count => clueDepth(count, rowMax))
    expect(depths).toEqual([0.1, 0.4, 0.7, 1])
    for (let i = 1; i < depths.length; i++) expect(depths[i]!).toBeGreaterThan(depths[i - 1]!)
  })

  it('0 线索与 null（未知）都不着色，不造 0 也不拿未知拉低分母', () => {
    expect(clueDepth(0, 10)).toBe(0)
    expect(clueDepth(null, 10)).toBe(0)
    expect(clueDepth(undefined, 10)).toBe(0)
    expect(clueDepth(-3, 10)).toBe(0)
    expect(clueDepth(Number.NaN, 10)).toBe(0)
    expect(clueRowMax([null, undefined, 3, null])).toBe(3)
    expect(clueRowMax([null, null])).toBe(0)
    expect(clueDepth(3, 0)).toBe(0)
  })

  it('行内归一不受其它行影响：冷泳道的峰值仍是最深，不被热门泳道的绝对数压白', () => {
    const coldRow = [1, 2, 4]
    const hotRow = [50, 100, 200]
    const coldDepths = coldRow.map(count => clueDepth(count, clueRowMax(coldRow)))
    const hotDepths = hotRow.map(count => clueDepth(count, clueRowMax(hotRow)))
    expect(coldDepths).toEqual([0.25, 0.5, 1])
    expect(coldDepths).toEqual(hotDepths)
    // 同样的 4 条线索：冷行峰值、热行只有 2%，证明分母只取本行
    expect(clueDepth(4, clueRowMax(coldRow))).toBe(1)
    expect(clueDepth(4, clueRowMax(hotRow))).toBeCloseTo(0.02, 10)
  })

  it('混入比例：0 返回空串（不注入样式），峰值 = 66%，中间整数百分比且单调不减', () => {
    expect(clueMix(0)).toBe('')
    expect(clueMix(-0.5)).toBe('')
    expect(clueMix(1)).toBe('66%')
    expect(clueMix(0.5)).toBe('33%')
    expect(CLUE_DEPTH_MAX_MIX).toBe(66)
    const mixes = [0.1, 0.4, 0.7, 1].map(depth => Number(clueMix(depth).replace('%', '')))
    expect(mixes).toEqual([7, 26, 46, 66])
  })
})

// ── 最热事件线点名整体退役（负向断言，D4/D5）──────────────────────────────

describe('observationLanes — 事件线量纲与点名退役', () => {
  const ctx = { laneLabel: '主线', date: '2026-10-23' }
  const RETIRED_WORDS = ['最热', '个来源', '事件线', '未归组', '兜底']

  it('旧 story 响应字段即使出现也不进入格值或文案（格值只认当日线索合计）', () => {
    const raw = {
      topic_id: 10, label: '主线', watch_linked: false, section_count_14d: 1, snapshot: null,
      timeline: [{
        date: '2026-10-23', story_count: 2, max_participant_count: 6, unassigned_event_count: 3,
        hottest_story_id: 55, hottest_story_title: '美伊博弈升级',
        sections: [sectionRaw(1, { thread_count: 4 })],
      }],
    }
    const view = matrixCellView(projectMatrixCell(makeResolver(normalize([raw])), '10', '2026-10-23'), ctx)
    expect(view.text).toBe('4')
    for (const word of RETIRED_WORDS) {
      expect(view.aria).not.toContain(word)
      expect(view.title).not.toContain(word)
    }
  })

  it('空标题 / 零来源情形整体退役：不渲染「事件线 #ID（0 个来源）」，也不当作断档', () => {
    const raw = {
      topic_id: 10, label: '主线', watch_linked: false, section_count_14d: 1, snapshot: null,
      timeline: [{
        date: '2026-10-23', hottest_story_id: 10, hottest_story_title: '　 ', max_participant_count: 0,
        sections: [sectionRaw(1, { thread_count: 0 })],
      }],
    }
    const view = matrixCellView(projectMatrixCell(makeResolver(normalize([raw])), '10', '2026-10-23'), ctx)
    expect(view.text).toBe('0')
    expect(view.kind).toBe('content')
    expect(view.openable).toBe(true)
    expect(view.title).not.toContain('事件线 #')
    expect(view.title).not.toContain('个来源')
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

  it('EV-2/EV-3：节点数字 = 当日线索合计（clue_count 优先，缺省回退各桶合计）；标签阈值 ≥3；gap=0/unavailable=null', () => {
    const resolve: LaneDayResolver = (topicId, date) => {
      if (topicId !== '10') return { kind: 'gap' }
      if (date === '2026-10-23') return {
        kind: 'content', source: 'tier',
        day: windowDay(date, [{ threadCount: 3 }, { threadCount: 1 }], { clueCount: 4 }),
      }
      if (date === '2026-10-22') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 2 }]) }
      if (date === '2026-10-21') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: null }]) }
      if (date === '2026-10-20') return { kind: 'gap' }
      return { kind: 'unavailable' }
    }
    const nodes = projectEvolutionNodes('10', ['2026-10-23', '2026-10-22', '2026-10-21', '2026-10-20', '2026-10-19'], resolve, '2026-10-23')
    expect(nodes[0]).toMatchObject({ kind: 'content', total: 4, showLabel: true, isToday: true })
    expect(nodes[1]).toMatchObject({ kind: 'content', total: 2, showLabel: false })
    // 旧数据缺当日线索合计及回退计数，保留未知。
    expect(nodes[2]).toMatchObject({ kind: 'content', total: null, showLabel: false })
    expect(nodes[3]).toMatchObject({ kind: 'gap', total: 0 })
    expect(nodes[4]).toMatchObject({ kind: 'unavailable', total: null })
  })

  it('首线索标题 = 第一桶首条事件标题，无事件回退桶 label', () => {
    expect(firstThreadTitle(windowDay('d', [{ eventTitles: ['甲', '乙'] }, { eventTitles: ['丙'] }]))).toBe('甲')
    expect(firstThreadTitle(windowDay('d', [{ eventTitles: [], label: '分区题' }]))).toBe('分区题')
    expect(firstThreadTitle(windowDay('d', []))).toBe('')
  })

  it('series 优先当日线索合计（clue_count），不把桶峰值用于长窗趋势', () => {
    const day = windowDay('d', [{ threadCount: 9 }, { threadCount: 8 }], { clueCount: 2 })
    expect(evolutionSeriesValue({ kind: 'content', source: 'tier', day })).toBe(2)
    const explicitZero = { ...day, clueCount: 0 }
    expect(evolutionSeriesValue({ kind: 'content', source: 'pool', day: explicitZero })).toBe(0)
  })

  it('series 值：content=已知合计（全未知 null）、gap=已知 0、unavailable=null（不补 0）', () => {
    expect(evolutionSeriesValue({ kind: 'content', source: 'tier', day: windowDay('d', [{ threadCount: 3 }, { threadCount: 4 }]) })).toBe(7)
    expect(evolutionSeriesValue({ kind: 'content', source: 'tier', day: windowDay('d', [{ threadCount: null }]) })).toBeNull()
    expect(evolutionSeriesValue({ kind: 'content', source: 'pool', day: windowDay('d', [{ threadCount: 0 }]) })).toBe(0)
    expect(evolutionSeriesValue({ kind: 'gap' })).toBe(0)
    expect(evolutionSeriesValue({ kind: 'unavailable' })).toBeNull()
  })
})
