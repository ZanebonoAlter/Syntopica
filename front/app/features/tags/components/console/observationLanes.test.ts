/**
 * observationLanes 纯模型测试（矩阵阶段 G3 + add-event-story-heat 5.4 story 口径）。
 *
 *  - source 全轴行序/休眠唯一基准：lane_heat 降序 → 事件线数降序 → ID 升序（与后端一致）；
 *    休眠分组仍用 sectionTotal 阈值
 *  - B 池独有 ID 不入名单；观察轴宽度变化永不重算名单/休眠/候选
 *  - 矩阵格视图三态（content 事件线数/回退/0、gap '—'、unavailable '?'）与来源数颜色分档
 *  - 演进点径公式 / 节点投影（节点数字 = 当日事件线数）/ series 值（gap=已知 0、未知=null 不补 0）
 */
import { describe, expect, it } from 'vitest'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'
import { normalizeLaneDynamicsWindow, type DayCellMetrics, type LaneDynamicsWindowData, type WindowLaneDay, type WindowSection } from '~/api/normalizers/laneDynamicsWindow'
import { buildDayAxis } from '~/features/tags/composables/laneMatrixModel'
import {
  buildObservationRows,
  compareTopicId,
  evolutionDotSize,
  evolutionSeriesValue,
  firstThreadTitle,
  HOTTEST_STORY_TITLE_MAX,
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

function laneRaw(topicId: number, label: string, timeline: { date: string, sections: Record<string, unknown>[] }[], overrides: Record<string, unknown> = {}): Record<string, unknown> {
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
      fallbackClueCount: section.fallbackClueCount ?? null,
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
  it('行序 = lane_heat 降序 → 事件线数降序 → ID 升序（add-event-story-heat 5.4，与后端一致）', () => {
    const source = normalize([
      // 事件线 10 / 无 heat；事件线 10 / heat 2；事件线 4 / heat 7（heat 最高在前）；
      // heat 2 并列 → 事件线多者在前；再并列（10/10）→ ID 升序
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

  it('lane_heat 全缺（旧后端）：事件线数降序 → ID 升序；storyTotal 缺省回退非综合稿线程合计', () => {
    const source = normalize([
      laneRaw(1, '回退', [{ date: '2026-10-23', sections: [
        sectionRaw(1, { events: ['a', 'b'], folded_count: 1 }),
        sectionRaw(2, { thread_count: 2 }),
      ] }]),
      laneRaw(2, '更大', [{ date: '2026-10-23', sections: [sectionRaw(3, { thread_count: 6 })] }]),
    ])
    const rows = buildObservationRows(source, AXIS14)
    expect(rows[0]).toMatchObject({ topicId: '2', storyTotal: 6, laneHeat: 0 })
    expect(rows[1]).toMatchObject({ topicId: '1', storyTotal: 5 })
  })

  it('story_count 优先于线程合计；participant/unassigned 逐日映射', () => {
    const source = normalize([
      laneRaw(3, '显式', [{
        date: '2026-10-23',
        sections: [sectionRaw(1, { thread_count: 9 })],
      }]),
    ])
    // 直接在归一结果上注入 wire 字段重归一：用 raw 形态更直接
    const raw = {
      topic_id: 3, label: '显式', watch_linked: false, section_count_14d: 1, snapshot: null,
      lane_heat: 1.5,
      timeline: [{
        date: '2026-10-23',
        story_count: 2,
        max_participant_count: 6,
        unassigned_event_count: 3,
        sections: [sectionRaw(1, { thread_count: 9 })],
      }],
    }
    const rows = buildObservationRows(normalize([raw]), AXIS14)
    expect(rows[0]).toMatchObject({
      topicId: '3',
      laneHeat: 1.5,
      storyTotal: 2,
      storyCounts: { '2026-10-23': 2 },
      participantCounts: { '2026-10-23': 6 },
      unassignedCounts: { '2026-10-23': 3 },
    })
    expect(source.lanes[0]!.timeline[0]!.storyCount).toBeNull()
  })

  it('全窗事件线数优先消费 story_count_14d，显式 0 不被逐日线程合计覆盖', () => {
    const source = normalize([
      laneRaw(2, '零事件线', [{ date: ANCHOR, sections: [sectionRaw(1, { thread_count: 99 })] }], { lane_heat: 1, story_count_14d: 0 }),
      laneRaw(1, '两条', [{ date: ANCHOR, sections: [sectionRaw(2, { thread_count: 1 })] }], { lane_heat: 1, story_count_14d: 2 }),
    ])
    const rows = buildObservationRows(source, AXIS14)
    expect(rows.map(row => [row.topicId, row.storyTotal])).toEqual([['1', 2], ['2', 0]])
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

  it('thread_count 缺省回退 events+folded，再缺按 0 计（clueTotal 兼容口径保留）', () => {
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

describe('observationLanes — 矩阵格视图（MX 组，story 口径）', () => {
  const resolve: LaneDayResolver = (topicId, date) =>
    topicId === '10' && date === '2026-10-23'
      ? {
          kind: 'content', source: 'tier',
          day: windowDay(date, [{ threadCount: 3 }, { threadCount: 7 }, { threadCount: 1 }], {
            storyCount: 2, maxParticipantCount: 6, unassignedEventCount: 3,
          }),
        }
      : topicId === '10' && date === '2026-10-22'
        ? { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: null, fallbackClueCount: 3 }]) }
        : date >= TIER_START
          ? { kind: 'gap' }
          : { kind: 'unavailable' }

  it('MX-1：格值 = 当日事件线数（同日多桶合并，非桶峰值/合计）；四指标 aria；颜色档 = 来源数', () => {
    const view = matrixCellView(projectMatrixCell(resolve, '10', '2026-10-23'), { laneLabel: '主线', date: '2026-10-23' })
    expect(view.text).toBe('2')
    expect(view.openable).toBe(true)
    expect(view.bucket).toBe('is-p3')
    expect(view.aria).toContain('2 个事件线')
    expect(view.aria).toContain('最热 6 个来源')
    expect(view.aria).toContain('未归组 3 个事件')
    expect(view.aria).toContain('3 个主题桶')
  })

  // ── 展示降级口径（matrix-clue-fallback-and-rail-merge D2）─────────────────
  it('MX-5：历史未回填日按线索合计兜底——格值 132、aria 明示「0 条事件线，按线索合计兜底」', () => {
    const degradedResolve: LaneDayResolver = (topicId, date) =>
      topicId === '10' && date === '2026-10-23'
        ? {
            kind: 'content', source: 'tier',
            day: windowDay(date, [{ threadCount: 100 }, { threadCount: 32 }], {
              storyCount: 0, maxParticipantCount: 0, unassignedEventCount: 132,
            }),
          }
        : { kind: 'gap' }
    const view = matrixCellView(projectMatrixCell(degradedResolve, '10', '2026-10-23'), { laneLabel: '主线', date: '2026-10-23' })
    expect(view.text).toBe('132')
    expect(view.openable).toBe(true)
    expect(view.aria).toContain('132 条线索')
    expect(view.aria).toContain('0 条事件线')
    expect(view.aria).toContain('兜底')
    expect(view.aria).not.toContain('132 个事件线')
  })

  it('MX-6：真零事件（story 0 且线索 0）格值 0、无降级尾注，不与断档混淆', () => {
    const zeroResolve: LaneDayResolver = (topicId, date) =>
      topicId === '10' && date === '2026-10-23'
        ? { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 0 }], { storyCount: 0 }) }
        : { kind: 'gap' }
    const view = matrixCellView(projectMatrixCell(zeroResolve, '10', '2026-10-23'), { laneLabel: '主线', date: '2026-10-23' })
    expect(view.text).toBe('0')
    expect(view.aria).toContain('0 个事件线')
    expect(view.aria).not.toContain('兜底')
  })

  it('MX-7：事件线数 > 0 的格子格值仍为事件线数（线索再多不用兜底、不混加）', () => {
    const normalResolve: LaneDayResolver = (topicId, date) =>
      topicId === '10' && date === '2026-10-23'
        ? { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 132 }], { storyCount: 3 }) }
        : { kind: 'gap' }
    const view = matrixCellView(projectMatrixCell(normalResolve, '10', '2026-10-23'), { laneLabel: '主线', date: '2026-10-23' })
    expect(view.text).toBe('3')
    expect(view.aria).toContain('3 个事件线')
    expect(view.aria).not.toContain('降级')
  })

  it('MX-2：story_count 缺省回退当日非综合稿线程合计（不用桶数冒充）；线索全未知时 aria 注明', () => {
    const fallback = matrixCellView(projectMatrixCell(resolve, '10', '2026-10-22'), { laneLabel: '主线', date: '2026-10-22' })
    expect(fallback.text).toBe('3')
    const bare = matrixCellView(
      {
        kind: 'content',
        metrics: { storyCount: null, displayCount: null, degradedFromClue: false, maxParticipantCount: 0, hottestStoryId: null, hottestStoryTitle: '', unassignedEventCount: 0, peak: null, total: 0, sectionCount: 2, hasUnknown: true },
      },
      { laneLabel: '主线', date: '2026-10-22' },
    )
    // 旧响应连回退计数都未知时不冒充真实 0，但仍可进入已有分区。
    expect(bare.text).toBe('?')
    expect(bare.openable).toBe(true)
    expect(bare.aria).toContain('未知')
    expect(bare.bucket).toBe('')
  })

  it('MX-3/MX-4：有分区 0 事件线 = 已知 0 可开；gap = 「—」 禁用；unavailable = 「?」 禁用', () => {
    const zero = matrixCellView(
      { kind: 'content', metrics: { storyCount: 0, displayCount: 0, degradedFromClue: false, maxParticipantCount: 0, hottestStoryId: null, hottestStoryTitle: '', unassignedEventCount: 0, peak: 0, total: 0, sectionCount: 1, hasUnknown: false } },
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

  it('颜色档：来源数 0 无色 / 1-2 浅 / 3-5 中 / 6+ 深（颜色非唯一信息载体，数字在 title/aria）', () => {
    const view = (participants: number) => matrixCellView(
      {
        kind: 'content',
        metrics: { storyCount: 1, displayCount: 1, degradedFromClue: false, maxParticipantCount: participants, hottestStoryId: null, hottestStoryTitle: '', unassignedEventCount: 0, peak: 1, total: 1, sectionCount: 1, hasUnknown: false },
      },
      { laneLabel: '主线', date: '2026-10-23' },
    )
    expect(view(0).bucket).toBe('')
    expect(view(1).bucket).toBe('is-p1')
    expect(view(2).bucket).toBe('is-p1')
    expect(view(3).bucket).toBe('is-p2')
    expect(view(5).bucket).toBe('is-p2')
    expect(view(6).bucket).toBe('is-p3')
  })
})

// ── 最热事件线点名（surface-story-heat-in-report-top）─────────────────────

describe('observationLanes — 最热事件线点名（surface-story-heat-in-report-top）', () => {
  const ctx = { laneLabel: '主线', date: '2026-10-23' }
  const metrics = (over: Partial<DayCellMetrics> = {}): DayCellMetrics => ({
    storyCount: 2, displayCount: 2, degradedFromClue: false, maxParticipantCount: 4, unassignedEventCount: 1, peak: 2, total: 4, sectionCount: 3, hasUnknown: false,
    hottestStoryId: 55, hottestStoryTitle: '美伊博弈升级',
    ...over,
  } as DayCellMetrics)

  it('点名：id+title → 「最热：标题（N 个来源）」；title 与 aria 同源；四数字顺序不变', () => {
    const view = matrixCellView({ kind: 'content', metrics: metrics() }, ctx)
    expect(view.title).toBe('主线 2026-10-23：2 个事件线 · 最热：美伊博弈升级（4 个来源） · 未归组 1 个事件 · 3 个主题桶')
    expect(view.aria).toBe(view.title)
    const positions = ['2 个事件线', '4 个来源', '未归组 1 个事件', '3 个主题桶'].map(seg => view.title!.indexOf(seg))
    expect(positions.every(index => index >= 0)).toBe(true)
    expect(positions).toEqual([...positions].sort((a, b) => a - b))
  })

  it('无身份（id null / 旧响应缺字段）：不追加点名段、无空括号/undefined；标注「最热事件线暂不可识别」不冒名', () => {
    for (const over of [{ hottestStoryId: null, hottestStoryTitle: '' }, { hottestStoryId: undefined, hottestStoryTitle: undefined }]) {
      const view = matrixCellView({ kind: 'content', metrics: metrics(over) }, ctx)
      expect(view.title).toContain('最热 4 个来源')
      expect(view.title).toContain('最热事件线暂不可识别')
      expect(view.title).not.toContain('最热：')
      expect(view.title).not.toContain('（）')
      expect(view.title).not.toContain('undefined')
    }
  })

  it('空/纯空白标题回退「事件线 #ID」；0 来源照常点名（0 是已知合法值，不当断档）', () => {
    const blank = matrixCellView({ kind: 'content', metrics: metrics({ hottestStoryTitle: '　 ' }) }, ctx)
    expect(blank.title).toContain('最热：事件线 #55（4 个来源）')
    const zero = matrixCellView({ kind: 'content', metrics: metrics({ maxParticipantCount: 0 }) }, ctx)
    expect(zero.title).toContain('最热：美伊博弈升级（0 个来源）')
  })

  it(`超长标题截断 ≤${HOTTEST_STORY_TITLE_MAX} 字 + 省略号（防 title 属性爆炸）；恰满不截`, () => {
    const long = '甲'.repeat(HOTTEST_STORY_TITLE_MAX + 10)
    const view = matrixCellView({ kind: 'content', metrics: metrics({ hottestStoryTitle: long }) }, ctx)
    expect(view.title).toContain(`最热：${'甲'.repeat(HOTTEST_STORY_TITLE_MAX)}…（4 个来源）`)
    expect(view.title).not.toContain(long)
    const edge = matrixCellView({ kind: 'content', metrics: metrics({ hottestStoryTitle: '乙'.repeat(HOTTEST_STORY_TITLE_MAX) }) }, ctx)
    expect(edge.title).toContain(`最热：${'乙'.repeat(HOTTEST_STORY_TITLE_MAX)}（4 个来源）`)
  })

  it('resolver 链路：raw hottest_story_id/title 经 normalizer 透传 → projectMatrixCell 后同名段', () => {
    const raw = {
      topic_id: 10, label: '主线', watch_linked: false, section_count_14d: 1, snapshot: null,
      timeline: [{
        date: '2026-10-23', story_count: 2, max_participant_count: 4,
        hottest_story_id: 55, hottest_story_title: '美伊博弈升级',
        sections: [sectionRaw(1, { thread_count: 4 })],
      }],
    }
    const resolve = makeResolver(normalize([raw]))
    const view = matrixCellView(projectMatrixCell(resolve, '10', '2026-10-23'), ctx)
    expect(view.title).toContain('最热：美伊博弈升级（4 个来源）')
    // 显式 null（新后端无最热线）→ 同旧响应降级，不点名
    const nullRaw = { ...raw, timeline: [{ ...raw.timeline[0]!, hottest_story_id: null, hottest_story_title: '' }] }
    const nullView = matrixCellView(projectMatrixCell(makeResolver(normalize([nullRaw])), '10', '2026-10-23'), ctx)
    expect(nullView.title).not.toContain('最热：')
    expect(nullView.title).toContain('最热事件线暂不可识别')
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

  it('EV-2/EV-3：节点数字 = 当日事件线数（story_count 优先，缺省回退线程合计）；标签阈值 ≥3；gap=0/unavailable=null', () => {
    const resolve: LaneDayResolver = (topicId, date) => {
      if (topicId !== '10') return { kind: 'gap' }
      if (date === '2026-10-23') return {
        kind: 'content', source: 'tier',
        day: windowDay(date, [{ threadCount: 3 }, { threadCount: 1 }], { storyCount: 4, maxParticipantCount: 5 }),
      }
      if (date === '2026-10-22') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 2 }]) }
      if (date === '2026-10-21') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: null, fallbackClueCount: null }]) }
      if (date === '2026-10-20') return { kind: 'gap' }
      return { kind: 'unavailable' }
    }
    const nodes = projectEvolutionNodes('10', ['2026-10-23', '2026-10-22', '2026-10-21', '2026-10-20', '2026-10-19'], resolve, '2026-10-23')
    expect(nodes[0]).toMatchObject({ kind: 'content', total: 4, showLabel: true, isToday: true })
    expect(nodes[1]).toMatchObject({ kind: 'content', total: 2, showLabel: false })
    // 旧数据缺事件线及回退计数，保留未知。
    expect(nodes[2]).toMatchObject({ kind: 'content', total: null, showLabel: false })
    expect(nodes[3]).toMatchObject({ kind: 'gap', total: 0 })
    expect(nodes[4]).toMatchObject({ kind: 'unavailable', total: null })
  })

  it('首线索标题 = 第一桶首条事件标题，无事件回退桶 label', () => {
    expect(firstThreadTitle(windowDay('d', [{ eventTitles: ['甲', '乙'] }, { eventTitles: ['丙'] }]))).toBe('甲')
    expect(firstThreadTitle(windowDay('d', [{ eventTitles: [], label: '分区题' }]))).toBe('分区题')
    expect(firstThreadTitle(windowDay('d', []))).toBe('')
  })

  it('series 优先事件线数，不把线程合计用于长窗趋势', () => {
    const day = windowDay('d', [{ threadCount: 9 }, { threadCount: 8 }], { storyCount: 2 })
    expect(evolutionSeriesValue({ kind: 'content', source: 'tier', day })).toBe(2)
    const explicitZero = { ...day, storyCount: 0 }
    expect(evolutionSeriesValue({ kind: 'content', source: 'pool', day: explicitZero })).toBe(0)
  })

  it('series 值：content=已知合计（全未知 null）、gap=已知 0、unavailable=null（不补 0）', () => {
    expect(evolutionSeriesValue({ kind: 'content', source: 'tier', day: windowDay('d', [{ threadCount: 3 }, { threadCount: 4 }]) })).toBe(7)
    expect(evolutionSeriesValue({ kind: 'content', source: 'tier', day: windowDay('d', [{ threadCount: null, fallbackClueCount: null }]) })).toBeNull()
    expect(evolutionSeriesValue({ kind: 'content', source: 'pool', day: windowDay('d', [{ threadCount: 0 }]) })).toBe(0)
    expect(evolutionSeriesValue({ kind: 'gap' })).toBe(0)
    expect(evolutionSeriesValue({ kind: 'unavailable' })).toBeNull()
  })
})
