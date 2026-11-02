/**
 * 版块态势状态源测试（rework-workshop-into-radar-and-desk tasks 1.3/2.1/2.2/2.5）。
 *
 * 覆盖 test-cases.md 白盒分支：
 *  - R-4/R-5/R-6/R-7：?lane= 恢复、非法回退、选中写 query、窗口切换保持
 *  - M-1..M-9：日期轴、格值、断档、合计、密度分档、窗口切换、轴外数据
 *  - MD-1..MD-3/MD-7：休眠阈值边界与窗口相关
 *  - 状态矩阵：loading/error/保留旧数据（stale）/空态两分支/旧请求不覆盖新版块
 */
import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest'
import { effectScope, ref, type EffectScope, type Ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'
import {
  buildDayAxis,
  buildLaneRow,
  buildLaneRows,
  hasReportDeepLink,
  normalizeWindowDays,
  parseBoardDeepLink,
  parseLaneParam,
  resolveLaneSelection,
  splitDormantRows,
  todayInShanghai,
  useBoardConsole,
} from './useBoardConsole'

const getLaneDynamics = vi.fn()

vi.mock('~/api/laneDynamics', () => ({
  useLaneDynamicsApi: () => ({ getLaneDynamics }),
}))

function makeLane(topicId: number, label: string, days: Record<string, number>, watchLinked = false) {
  return {
    topic_id: topicId,
    label,
    watch_linked: watchLinked,
    section_count_14d: Object.values(days).reduce((a, b) => a + b, 0),
    snapshot: null,
    timeline: Object.entries(days).map(([date, count]) => ({
      date,
      sections: Array.from({ length: count }, (_, i) => ({
        section_id: topicId * 100 + i,
        label: `${label} ${date} 分区${i}`,
        events: [`${label} ${date} 事件${i}`],
      })),
    })),
  }
}

function makeResponse(overrides: Partial<LaneDynamicsResponse> = {}): LaneDynamicsResponse {
  return {
    window_days: 14,
    has_reports: true,
    lanes: [
      makeLane(10, '主线 A', { '2026-10-22': 2, '2026-10-23': 3 }),
      makeLane(11, '主线 B', { '2026-10-23': 3 }),
      makeLane(12, '休眠线', { '2026-10-20': 2 }),
    ],
    candidates: [],
    ...overrides,
  }
}

/** 固定「今天」= 2026-10-23（Asia/Shanghai）。 */
const FIXED_NOW = () => new Date('2026-10-23T04:00:00Z')

interface Harness {
  scope: EffectScope
  boardId: Ref<number>
  routeState: { query: Record<string, unknown> }
  router: { replace: ReturnType<typeof vi.fn> }
  state: ReturnType<typeof useBoardConsole>
}

function mountConsole(initialBoard = 1974): Harness {
  const boardId = ref(initialBoard)
  const routeState: { query: Record<string, unknown> } = { query: {} }
  const router = {
    replace: vi.fn((to: { query: Record<string, unknown> }) => {
      routeState.query = { ...to.query }
      return Promise.resolve()
    }),
  }
  const scope = effectScope()
  const state = scope.run(() => useBoardConsole(boardId as never, {
    route: () => routeState as never,
    router: () => router as never,
    now: FIXED_NOW,
  }))!
  return { scope, boardId, routeState, router, state }
}

describe('useBoardConsole — 纯函数', () => {
  it('todayInShanghai：跨零点按 Asia/Shanghai 日历日', () => {
    expect(todayInShanghai(new Date('2026-10-23T15:59:59Z'))).toBe('2026-10-23')
    expect(todayInShanghai(new Date('2026-10-23T16:00:00Z'))).toBe('2026-10-24')
  })

  it('buildDayAxis：长度 = 窗口天数、升序、末位为 endDate（M-1/M-2）', () => {
    const axis = buildDayAxis('2026-10-23', 14)
    expect(axis).toHaveLength(14)
    expect(axis[0]).toBe('2026-10-10')
    expect(axis[13]).toBe('2026-10-23')
    expect([...axis].sort()).toEqual(axis)
    expect(buildDayAxis('2026-11-03', 14)[0]).toBe('2026-10-21')
  })

  it('buildDayAxis：非法输入返回空轴；天数下限 1（M-1 边界）', () => {
    expect(buildDayAxis('not-a-date', 14)).toEqual([])
    expect(buildDayAxis('2026-10-23', 0)).toEqual(['2026-10-23'])
  })

  it('buildLaneRow：格值 = 当日 section 数、轴外数据忽略、合计与最后活跃日（M-4/M-6/M-9）', () => {
    const axis = buildDayAxis('2026-10-23', 14)
    const row = buildLaneRow(makeLane(10, 'A', { '2026-10-23': 3, '2026-09-01': 5 }), axis)
    expect(row.counts['2026-10-23']).toBe(3)
    expect(row.counts['2026-09-01']).toBeUndefined()
    expect(row.total).toBe(3)
    expect(row.lastActiveIndex).toBe(13)
  })

  it('热度按线索数而非 section 数：同日分桶多但线索少的泳道排在后面（M-4）', () => {
    const axis = buildDayAxis('2026-10-23', 14)
    const manyBucketsFewClues = {
      ...makeLane(1, '分桶多', { '2026-10-23': 3 }),
      timeline: [{
        date: '2026-10-23',
        sections: [
          { section_id: 1, label: 'a', events: [], thread_count: 1 },
          { section_id: 2, label: 'b', events: [], thread_count: 0 },
          { section_id: 3, label: 'c', events: [], thread_count: 0 },
        ],
      }],
    }
    const fewBucketsManyClues = {
      ...makeLane(2, '线索多', { '2026-10-23': 1 }),
      timeline: [{
        date: '2026-10-23',
        sections: [{ section_id: 4, label: 'd', events: [], thread_count: 8 }],
      }],
    }
    const rows = buildLaneRows([manyBucketsFewClues, fewBucketsManyClues], axis)
    expect(rows.map(row => row.topicId)).toEqual([2, 1])
    expect(rows[0]!.clueTotal).toBe(8)
  })

  it('splitDormantRows：阈值 0/2 归休眠、3 留主矩阵（MD-1..MD-3）', () => {
    const axis = buildDayAxis('2026-10-23', 14)
    const rows = [
      buildLaneRow(makeLane(1, '零', {}), axis),
      buildLaneRow(makeLane(2, '两', { '2026-10-23': 2 }), axis),
      buildLaneRow(makeLane(3, '三', { '2026-10-23': 3 }), axis),
    ]
    const { active, dormant } = splitDormantRows(rows)
    expect(dormant.map((row) => row.topicId)).toEqual([1, 2])
    expect(active.map((row) => row.topicId)).toEqual([3])
  })

  it('buildLaneRows：按窗口内总数降序，并列按 topic_id 升序', () => {
    const axis = buildDayAxis('2026-10-23', 14)
    const rows = buildLaneRows([
      makeLane(30, 'C', { '2026-10-23': 2 }),
      makeLane(20, 'B', { '2026-10-23': 2 }),
      makeLane(10, 'A', { '2026-10-23': 5 }),
    ], axis)
    expect(rows.map((row) => row.topicId)).toEqual([10, 20, 30])
  })

  it('parseLaneParam：空串/空白/负数/非数字/数组 → null；正整数 → id（R-5）', () => {
    expect(parseLaneParam('55')).toBe(55)
    expect(parseLaneParam(' 55 ')).toBe(55)
    expect(parseLaneParam('')).toBeNull()
    expect(parseLaneParam('   ')).toBeNull()
    expect(parseLaneParam('-3')).toBeNull()
    expect(parseLaneParam('abc')).toBeNull()
    expect(parseLaneParam(55)).toBeNull()
    expect(parseLaneParam(['7'])).toBe(7)
  })

  it('resolveLaneSelection：query 命中优先；非法/缺失回退分区最多者，并列取小 id（R-4/R-5）', () => {
    const axis = buildDayAxis('2026-10-23', 14)
    const rows = buildLaneRows([
      makeLane(11, 'B', { '2026-10-23': 1 }),
      makeLane(10, 'A', { '2026-10-22': 1, '2026-10-23': 1 }),
    ], axis)
    expect(resolveLaneSelection(rows, '11')).toBe(11)
    expect(resolveLaneSelection(rows, '9999')).toBe(10)
    expect(resolveLaneSelection(rows, undefined)).toBe(10)
    expect(resolveLaneSelection([], '11')).toBeNull()
  })

  it('normalizeWindowDays：只接受 7/14/30/365，其余回落 14（M-8）', () => {
    expect(normalizeWindowDays(30)).toBe(30)
    expect(normalizeWindowDays('7')).toBe(7)
    expect(normalizeWindowDays(365)).toBe(365)
    expect(normalizeWindowDays(0)).toBe(14)
    expect(normalizeWindowDays('abc')).toBe(14)
    expect(normalizeWindowDays(15)).toBe(14)
  })

  it('parseBoardDeepLink：合法 board 去键保留其余 query；非法不映射（R-1/R-3）', () => {
    expect(parseBoardDeepLink({ board: '1974', annotation: '8' })).toEqual({
      boardId: 1974,
      query: { annotation: '8' },
    })
    expect(parseBoardDeepLink({ board: 'abc' })).toBeNull()
    expect(parseBoardDeepLink({ board: '0' })).toBeNull()
    expect(parseBoardDeepLink({ board: '-3' })).toBeNull()
    expect(parseBoardDeepLink({})).toBeNull()
    expect(parseBoardDeepLink(null)).toBeNull()
  })

  it('hasReportDeepLink：有非空 report 才视为日报深链（R-2）', () => {
    expect(hasReportDeepLink({ report: '1211' })).toBe(true)
    expect(hasReportDeepLink({ report: '' })).toBe(false)
    expect(hasReportDeepLink({ report: '   ' })).toBe(false)
    expect(hasReportDeepLink({})).toBe(false)
  })
})

describe('useBoardConsole — 组合状态', () => {
  let harness: Harness

  beforeEach(() => {
    getLaneDynamics.mockReset()
    getLaneDynamics.mockResolvedValue({ success: true, data: makeResponse() })
  })

  afterEach(() => {
    harness?.scope.stop()
  })

  it('加载完成后默认选中窗口内分区最多的泳道（R-5）', async () => {
    harness = mountConsole()
    await flushPromises()
    expect(getLaneDynamics).toHaveBeenCalledWith(1974, 14)
    expect(harness.state.rows.value).toHaveLength(3)
    expect(harness.state.selectedLaneId.value).toBe(10)
    expect(harness.state.activeRows.value.map((row) => row.topicId)).toEqual([10, 11])
    expect(harness.state.dormantRows.value.map((row) => row.topicId)).toEqual([12])
    expect(harness.state.expandedDormant.value).toBe(false)
  })

  it('query ?lane= 命中时选中它；指向休眠泳道自动展开休眠组（R-4/MD-7）', async () => {
    harness = mountConsole()
    harness.routeState.query = { lane: '12' }
    await flushPromises()
    expect(harness.state.selectedLaneId.value).toBe(12)
    expect(harness.state.expandedDormant.value).toBe(true)
  })

  it('selectLane 写 query 并保留其它键（R-6）', async () => {
    harness = mountConsole()
    harness.routeState.query = { annotation: '8' }
    await flushPromises()
    harness.state.selectLane(11)
    await flushPromises()
    expect(harness.router.replace).toHaveBeenCalled()
    expect(harness.routeState.query).toMatchObject({ lane: '11', annotation: '8' })
    expect(harness.state.selectedLane.value?.topicId).toBe(11)
  })

  it('非法 lane 回退默认，且不写坏 query（R-5）', async () => {
    harness = mountConsole()
    harness.routeState.query = { lane: '9999' }
    await flushPromises()
    expect(harness.state.selectedLaneId.value).toBe(10)
  })

  it('窗口切换：请求新 days、清空旧数据走骨架、选择保持或回落（M-8）', async () => {
    harness = mountConsole()
    await flushPromises()
    harness.state.selectLane(11)
    harness.state.setWindow(30)
    expect(harness.state.windowDays.value).toBe(30)
    expect(harness.state.data.value).toBeNull()
    expect(harness.state.selectedLaneId.value).toBeNull()
    await flushPromises()
    expect(getLaneDynamics).toHaveBeenLastCalledWith(1974, 30)
    // 窗口切换不清除 query 中的 lane：泳道仍在窗口内 → 选择保持
    expect(harness.state.selectedLaneId.value).toBe(11)
  })

  it('切版块：清数据、复位休眠展开与视图、按新版块重取（R-7/1.4）', async () => {
    harness = mountConsole()
    harness.routeState.query = { lane: '12' }
    await flushPromises()
    harness.state.expandedDormant.value = true
    // 切版块走 router.push('/tags/1975')：旧 query 不带过去
    harness.routeState.query = {}
    harness.boardId.value = 1975
    await flushPromises()
    expect(harness.state.expandedDormant.value).toBe(false)
    expect(harness.state.windowDays.value).toBe(14)
    expect(harness.state.activeView.value).toBe('matrix')
    // 新板块 A 档重取（末位还有首屏背景曲线预取的 365 池请求，故不用「最后一次调用」断言）
    expect(getLaneDynamics).toHaveBeenCalledWith(1975, 14)
  })

  it('刷新失败保留旧数据并给 stale 提示；重试成功清提示（2.5）', async () => {
    harness = mountConsole()
    await flushPromises()
    getLaneDynamics.mockResolvedValueOnce({ success: false, error: '网络错误' })
    await harness.state.load()
    expect(harness.state.error.value).toBe('网络错误')
    expect(harness.state.staleNotice.value).toBe(true)
    expect(harness.state.data.value).not.toBeNull()
  })

  it('首次加载失败无旧数据：error + 不置 stale（2.5）', async () => {
    getLaneDynamics.mockResolvedValueOnce({ success: false, error: '挂了' })
    harness = mountConsole()
    await flushPromises()
    expect(harness.state.error.value).toBe('挂了')
    expect(harness.state.staleNotice.value).toBe(false)
    expect(harness.state.data.value).toBeNull()
  })

  it('空态两分支：无日报 / 有日报无泳道（2.5）', async () => {
    getLaneDynamics.mockResolvedValueOnce({
      success: true,
      data: makeResponse({ has_reports: false, lanes: [], candidates: [] }),
    })
    harness = mountConsole()
    await flushPromises()
    expect(harness.state.emptyState.value).toBe('no-reports')
    harness.scope.stop()

    getLaneDynamics.mockResolvedValueOnce({
      success: true,
      data: makeResponse({ has_reports: true, lanes: [] }),
    })
    harness = mountConsole()
    await flushPromises()
    expect(harness.state.emptyState.value).toBe('no-lanes')
  })

  it('旧版块迟到响应不覆盖新版块（并发守卫）', async () => {
    let resolveFirst: (value: unknown) => void = () => {}
    getLaneDynamics.mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve }))
    harness = mountConsole()
    harness.boardId.value = 1975
    await flushPromises()
    // 每板块各发一次 A 档请求（另有首屏背景曲线预取的 365 池请求，不计入 A 档断言）
    expect(getLaneDynamics.mock.calls.filter(([, days]) => days === 14)).toHaveLength(2)
    // 新版块先返回
    getLaneDynamics.mockResolvedValueOnce({ success: true, data: makeResponse({ window_days: 14 }) })
    resolveFirst({ success: true, data: makeResponse({ window_days: 14 }) })
    await flushPromises()
    // 结果仍属于最后一次请求（新版块），不因旧响应重复渲染
    expect(harness.state.rows.value.length).toBe(3)
  })
})
