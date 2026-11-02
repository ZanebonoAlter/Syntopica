/**
 * useConsoleReturnContext 测试（8C G4a：稳定其他分区/阅读 key/旧 lane 兼容）。
 *
 * 覆盖：旧六字段 payload 兼容、非法 other 静默降级、other 身份不污染 lane/URL 四参、
 * 0 正式/allLanes 空恢复、同日多 section 精确身份、URL 显式 lane 优先、阅读 pending 不耗帧、
 * settled 命中当前阅读 key 才恢复、等候中真实用户意图取消、归档/研究往返接口与一次写参。
 * 滚动驱动用受控 rAF spy（同 Shell R34 测试），不依赖真实浏览器。
 */
import { afterEach, describe, expect, it, vi } from 'vitest'
import { computed, effectScope, nextTick, ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import type { LocationQuery, Router } from 'vue-router'
import type { LaneDynamicsWindowData } from '~/api/normalizers/laneDynamicsWindow'
import type { ConsoleOtherSelection } from './consoleReturnContext'
import type { ObservationRange } from './observationRange'
import type { ObservationLaneRow } from './observationLanes'
import { useConsoleReturnContext, type ReadingSettledState } from './useConsoleReturnContext'

const DATE = '2026-10-24'
const STORAGE_KEY = 'syntopica:board-console:return:1974'

afterEach(() => {
  vi.restoreAllMocks()
  window.sessionStorage.clear()
})

function laneRow(topicId: string): ObservationLaneRow {
  return {
    topicId,
    label: `泳道${topicId}`,
    watchLinked: false,
    sectionTotal: 1,
    clueTotal: 1,
    dormant: false,
    snapshot: null,
  }
}

function emptyWindow(): LaneDynamicsWindowData {
  return { windowDays: 14, hasReports: true, lanes: [], candidates: [], anchorDate: DATE }
}

function seed(payload: Record<string, unknown>): void {
  window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify(payload))
}

function makeStage(): { stage: HTMLElement, setScrollHeight: (value: number) => void } {
  const stage = document.createElement('div')
  let scrollHeight = 402
  Object.defineProperty(stage, 'clientHeight', { value: 100, configurable: true })
  Object.defineProperty(stage, 'scrollHeight', { get: () => scrollHeight, configurable: true })
  stage.scrollTop = 0
  return { stage, setScrollHeight: (value: number) => { scrollHeight = value } }
}

function captureFrames(): { frames: FrameRequestCallback[] } {
  const frames: FrameRequestCallback[] = []
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback: FrameRequestCallback) => {
    frames.push(callback)
    return frames.length
  })
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
  return { frames }
}

const OTHER: ConsoleOtherSelection = { sectionId: '502', date: DATE, reportId: '900', topicId: '77' }

function makeContext(options: {
  rows?: ObservationLaneRow[]
  selectedLaneKey?: string | null
  selectedOther?: ConsoleOtherSelection | null
  othersOpen?: boolean
  stage?: HTMLElement | null
  query?: LocationQuery
  path?: string
  selectedDate?: string | null
} = {}) {
  const boardId = ref(1974)
  const sourceWindow = ref<LaneDynamicsWindowData | null>(emptyWindow())
  const sourceDays = ref(14)
  const selectedLaneKey = ref<string | null>(options.selectedLaneKey === undefined ? '10' : options.selectedLaneKey)
  const selectedDate = ref<string | null>(options.selectedDate ?? null)
  const observation = ref<ObservationRange | null>({ start: '2026-10-10', end: '2026-10-23' })
  const selectedOther = ref<ConsoleOtherSelection | null>(options.selectedOther ?? null)
  const othersOpen = ref(options.othersOpen ?? false)
  const readingSettled = ref<ReadingSettledState | null>(null)
  const rows = options.rows ?? [laneRow('10'), laneRow('11')]
  // route-like 桩对象按真实 vue-router 形状（query 值为 string|null，非 unknown）；
  // 仍以断言用的具体类型声明（query 必选）保住 `Object.keys(h.route.query)` 类断言。
  const route: { query: LocationQuery, path: string } = {
    query: { ...(options.query ?? {}) },
    path: options.path ?? '/tags/1974',
  }
  const replace = vi.fn(async (location: { path: string, query: LocationQuery }) => {
    route.path = location.path
    route.query = { ...location.query }
  })
  const push = vi.fn(async () => {})
  const selectLane = vi.fn((topicId: string) => { selectedLaneKey.value = topicId })
  const setSourceDays = vi.fn((days: number) => { sourceDays.value = days })
  const applyObservationRange = vi.fn((start: string, end: string) => {
    observation.value = { start, end }
    return true
  })

  const scope = effectScope()
  const ctx = scope.run(() => useConsoleReturnContext({
    boardId,
    sourceWindow,
    sourceDays,
    allLanes: computed(() => rows),
    selectedLaneKey: computed(() => selectedLaneKey.value),
    selectedDate,
    observation,
    stageRef: () => options.stage ?? null,
    route: () => route,
    router: () => ({ replace, push } as unknown as Router),
    selectLane,
    setSourceDays,
    applyObservationRange,
    readingSettled,
    selectedOther,
    othersOpen,
  }))!

  return {
    ctx,
    scope,
    boardId,
    sourceWindow,
    sourceDays,
    selectedLaneKey,
    selectedDate,
    observation,
    selectedOther,
    othersOpen,
    readingSettled,
    route,
    replace,
    push,
    selectLane,
    setSourceDays,
    applyObservationRange,
  }
}

async function settleRestore(): Promise<void> {
  await flushPromises()
  await flushPromises()
}

describe('useConsoleReturnContext — 旧 payload 与 lane 回归', () => {
  it('旧六字段 payload 可读：恢复泳道/日期/档/范围并一次写回四参（无 date=）', async () => {
    seed({ laneId: '10', date: DATE, days: 14, start: '2026-10-10', end: '2026-10-23', scrollTop: 0 })
    const h = makeContext({ selectedLaneKey: null })
    await settleRestore()

    expect(h.selectLane).toHaveBeenCalledWith('10', { writeQuery: false })
    expect(h.selectedDate.value).toBe(DATE)
    expect(h.selectedOther.value).toBeNull()
    expect(h.route.query).toEqual({ days: '14', start: '2026-10-10', end: '2026-10-23', lane: '10' })
    expect(Object.keys(h.route.query).sort()).toEqual(['days', 'end', 'lane', 'start'])
    expect(h.replace).toHaveBeenCalledTimes(1)
    expect(window.sessionStorage.getItem(STORAGE_KEY)).toBeNull()
    h.scope.stop()
  })

  it('非法/日期不一致的 other 静默降级，不阻止旧字段恢复', async () => {
    seed({
      laneId: '10', date: DATE, days: 14, start: '2026-10-10', end: '2026-10-23', scrollTop: 0,
      other: { sectionId: '502', date: '2026-10-20', reportId: '900', topicId: '77' },
    })
    const h = makeContext()
    await settleRestore()

    expect(h.selectedOther.value).toBeNull()
    expect(h.selectedDate.value).toBe(DATE)
    expect(h.route.query.lane).toBe('10')
    h.scope.stop()
  })

  it('URL 显式 lane 优先：缓存 laneId 不覆盖显式值；显式 lane 不同则不恢复 other', async () => {
    seed({ laneId: '10', date: DATE, days: 14, start: '2026-10-10', end: '2026-10-23', scrollTop: 0, other: OTHER, othersOpen: true })
    const h = makeContext({ selectedLaneKey: '11', query: { lane: '11' } })
    await settleRestore()

    expect(h.selectLane).not.toHaveBeenCalled()
    expect(h.route.query.lane).toBe('11')
    expect(h.selectedOther.value).toBeNull()
    expect(h.othersOpen.value).toBe(false)
    // 范围/档仍按缓存恢复（不被 other 的显式冲突牵连）
    expect(h.route.query.start).toBe('2026-10-10')
    expect(h.route.query.end).toBe('2026-10-23')
    h.scope.stop()
  })
})

describe('useConsoleReturnContext — 其他分区阅读槽恢复', () => {
  it('恢复 stable board/report/section/date + 展开态；other 身份不写进 URL lane/date', async () => {
    seed({ laneId: '10', date: DATE, days: 14, start: '2026-10-10', end: '2026-10-23', scrollTop: 0, other: OTHER, othersOpen: true })
    const h = makeContext()
    await settleRestore()

    expect(h.selectedOther.value).toEqual(OTHER)
    expect(h.othersOpen.value).toBe(true)
    // URL 四参仍是泳道/档/范围；没有 date=/other= 键
    expect(Object.keys(h.route.query).sort()).toEqual(['days', 'end', 'lane', 'start'])
    expect(h.route.query.lane).toBe('10')
    expect(h.route.query.date).toBeUndefined()
    h.scope.stop()
  })

  it('显式 lane 与保存 lane 相同（从其他话题打开日报再回来）：恢复原阅读槽', async () => {
    seed({ laneId: '10', date: DATE, days: 14, start: '2026-10-10', end: '2026-10-23', scrollTop: 0, other: OTHER, othersOpen: true })
    const h = makeContext({ query: { focus: 'report', lane: '10' } })
    await settleRestore()

    expect(h.selectedOther.value).toEqual(OTHER)
    expect(h.othersOpen.value).toBe(true)
    expect(h.route.query.lane).toBe('10')
    expect(h.route.query.focus).toBeUndefined()
    h.scope.stop()
  })

  it('同日多个 section 精确恢复所选 ID（不按标题/当前 topic 猜配）', async () => {
    const other = { sectionId: '502', date: DATE, reportId: '900', topicId: '77' }
    seed({ laneId: null, date: DATE, days: 14, start: '2026-10-10', end: '2026-10-23', scrollTop: 0, other, othersOpen: true })
    const h = makeContext({ selectedLaneKey: null })
    await settleRestore()

    expect(h.selectedOther.value?.sectionId).toBe('502')
    expect(h.selectedOther.value?.reportId).toBe('900')
    expect(h.selectedOther.value?.topicId).toBe('77')
    h.scope.stop()
  })

  it('0 正式泳道/allLanes 空也恢复（不等 lane 非空）', async () => {
    const other = { sectionId: '502', date: DATE, reportId: '900', topicId: '77' }
    seed({ laneId: null, date: DATE, days: 14, start: '2026-10-10', end: '2026-10-23', scrollTop: 0, other, othersOpen: true })
    const h = makeContext({ rows: [], selectedLaneKey: null })
    await settleRestore()

    expect(h.selectLane).not.toHaveBeenCalled()
    expect(h.selectedDate.value).toBe(DATE)
    expect(h.selectedOther.value).toEqual(other)
    expect(h.othersOpen.value).toBe(true)
    expect(h.route.query.lane).toBeUndefined()
    h.scope.stop()
  })
})

describe('useConsoleReturnContext — 滚动恢复 gate（R34 继承）', () => {
  it('阅读 pending 不排帧；settled 命中当前阅读 key/date 才恢复，内容增高后追到目标', async () => {
    const { stage, setScrollHeight } = makeStage()
    const { frames } = captureFrames()
    seed({ laneId: null, date: DATE, days: 14, start: '2026-10-10', end: '2026-10-23', scrollTop: 382, other: OTHER, othersOpen: true })
    const h = makeContext({ stage, selectedLaneKey: null })
    await settleRestore()

    expect(frames).toHaveLength(0)
    expect(stage.scrollTop).toBe(0)

    // 不同阅读 key 的终态不兑现（防旧信号误恢复）
    h.readingSettled.value = { laneKey: 'other:599', date: DATE, status: 'ready' }
    await nextTick()
    await flushPromises()
    expect(frames).toHaveLength(0)

    h.readingSettled.value = { laneKey: 'other:502', date: DATE, status: 'ready' }
    await nextTick()
    await flushPromises()
    expect(frames.length).toBeGreaterThan(0)

    frames.shift()!(0)
    await nextTick()
    expect(stage.scrollTop).toBe(302)

    setScrollHeight(482)
    frames.shift()!(0)
    await nextTick()
    expect(stage.scrollTop).toBe(382)
    h.scope.stop()
  })

  it('等候中真实用户意图（wheel）取消，随后 settled 到达也不强拉', async () => {
    const { stage } = makeStage()
    const { frames } = captureFrames()
    seed({ laneId: '10', date: DATE, days: 14, start: '2026-10-10', end: '2026-10-23', scrollTop: 382 })
    const h = makeContext({ stage })
    await settleRestore()
    expect(frames).toHaveLength(0)

    window.dispatchEvent(new Event('wheel'))
    h.readingSettled.value = { laneKey: '10', date: DATE, status: 'ready' }
    await nextTick()
    await flushPromises()

    expect(frames).toHaveLength(0)
    expect(stage.scrollTop).toBe(0)
    h.scope.stop()
  })

  it('settled 日期不一致不恢复（同 key 旧日期信号作废）', async () => {
    const { stage } = makeStage()
    seed({ laneId: '10', date: DATE, days: 14, start: '2026-10-10', end: '2026-10-23', scrollTop: 200 })
    const h = makeContext({ stage })
    await settleRestore()

    h.readingSettled.value = { laneKey: '10', date: '2026-10-23', status: 'ready' }
    await nextTick()
    await flushPromises()
    expect(stage.scrollTop).toBe(0)

    // 目标小于当前最大可滚值：直接一次写到目标（不需额外帧）
    h.readingSettled.value = { laneKey: '10', date: DATE, status: 'ready' }
    await nextTick()
    await flushPromises()
    expect(stage.scrollTop).toBe(200)
    h.scope.stop()
  })
})

describe('useConsoleReturnContext — 保存/往返接口', () => {
  it('saveContext 保存 other/othersOpen 与滚动，不新增 date=；泳道身份保持独立', async () => {
    const { stage } = makeStage()
    stage.scrollTop = 80
    const h = makeContext({
      stage,
      selectedOther: OTHER,
      othersOpen: true,
      selectedDate: DATE,
    })
    await settleRestore()
    h.replace.mockClear()

    h.ctx.saveContext()
    const stored = JSON.parse(window.sessionStorage.getItem(STORAGE_KEY)!) as Record<string, unknown>
    expect(Object.keys(stored).sort()).toEqual(['date', 'days', 'end', 'laneId', 'other', 'othersOpen', 'scrollTop', 'start'])
    expect(stored.laneId).toBe('10')
    expect(stored.other).toEqual(OTHER)
    expect(stored.othersOpen).toBe(true)
    expect(stored.scrollTop).toBe(80)
    expect(h.replace).not.toHaveBeenCalled()
    // 保存不改当前选择/日期（归档内部往返靠状态保留）
    expect(h.selectedOther.value).toEqual(OTHER)
    expect(h.selectedDate.value).toBe(DATE)
    h.scope.stop()
  })

  it('无 other 选择时保持旧六字段形态（与既有 Shell 返回测试兼容）', async () => {
    const h = makeContext({ selectedDate: DATE })
    await settleRestore()
    h.ctx.saveContext()
    const stored = JSON.parse(window.sessionStorage.getItem(STORAGE_KEY)!) as Record<string, unknown>
    expect(Object.keys(stored)).toEqual(['laneId', 'date', 'days', 'start', 'end', 'scrollTop'])
    h.scope.stop()
  })

  it('openReport/openResearch 先保存上下文再路由（阅读台/研究往返）', async () => {
    const h = makeContext({ selectedOther: OTHER, othersOpen: true, selectedDate: DATE })
    await settleRestore()

    h.ctx.openReport(DATE)
    expect(h.push).toHaveBeenCalledWith({ path: `/tags/1974/${DATE}`, query: { lane: '10' } })
    const saved = JSON.parse(window.sessionStorage.getItem(STORAGE_KEY)!) as Record<string, unknown>
    expect(saved.other).toEqual(OTHER)

    h.ctx.openResearch()
    expect(h.push).toHaveBeenCalledWith('/tags/1974/research')
    h.scope.stop()
  })
})
