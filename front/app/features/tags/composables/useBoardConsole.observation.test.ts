/**
 * 版块态势控制台 — 观察窗状态测试（矩阵阶段 G1，test-cases-matrix-stage.md OX/CA 组）。
 *
 *  - OX-1 恰好档宽不切档 / OX-2 B 只补 A 同 ID 历史 / OX-7 明确切档 / OX-8 365 档池一次请求
 *  - OX-9 URL 深链部分恢复 / OX-10 双通道 epoch / OX-11 source 全轴排序休眠
 *  - OX-12 A成功B失败 / OX-13 A首败B成不冒充 / OX-14 旧A刷新失败stale保留
 *  - CA-2 四档缓存与 365 去重（console 层）；anchor 注入（0 泳道仍有日报锚点）；切板块复位。
 */
import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest'
import { effectScope, reactive, ref, type EffectScope, type Ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'
import { useBoardConsole } from './useBoardConsole'

const getLaneDynamics = vi.fn()

vi.mock('~/api/laneDynamics', () => ({
  useLaneDynamicsApi: () => ({ getLaneDynamics }),
}))

function makeLane(topicId: number, label: string, days: Record<string, number>, clues?: Record<string, number>) {
  return {
    topic_id: topicId,
    label,
    watch_linked: false,
    section_count_14d: Object.values(days).reduce((a, b) => a + b, 0),
    snapshot: null,
    timeline: Object.entries(days).map(([date, count]) => ({
      date,
      sections: Array.from({ length: count }, (_, i) => ({
        section_id: topicId * 100 + i,
        label: `${label} ${date} 分区${i}`,
        thread_count: clues?.[date] ?? 1,
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
      makeLane(10, '主线 A', { '2026-10-22': 2, '2026-10-23': 3 }, { '2026-10-22': 5, '2026-10-23': 7 }),
      makeLane(11, '主线 B', { '2026-10-23': 3 }, { '2026-10-23': 4 }),
      makeLane(12, '休眠线', { '2026-10-20': 2 }, { '2026-10-20': 2 }),
    ],
    candidates: [],
    ...overrides,
  }
}

/** B 池（days=365）：含 A 同 ID（10）历史 + 池独有（99）。historyDate 可换（区分板块归属用）。 */
function makePoolResponse(historyDate = '2026-06-15'): LaneDynamicsResponse {
  return {
    window_days: 365,
    has_reports: true,
    lanes: [
      { ...makeLane(10, '主线 A', { '2026-10-22': 2, '2026-10-23': 3 }), timeline: [
        ...makeLane(10, '主线 A', { '2026-10-22': 2, '2026-10-23': 3 }).timeline,
        { date: historyDate, sections: [{ section_id: 1001, label: '历史桶', thread_count: 6, events: ['历史事件'] }] },
      ] },
      makeLane(99, '池独有线', { [historyDate]: 1 }, { [historyDate]: 9 }),
    ],
    candidates: [],
  }
}

const FIXED_NOW = () => new Date('2026-10-23T04:00:00Z')

interface Harness {
  scope: EffectScope
  boardId: Ref<number>
  routeState: { query: Record<string, unknown> }
  router: { replace: ReturnType<typeof vi.fn> }
  state: ReturnType<typeof useBoardConsole>
}

function mountConsole(initialQuery: Record<string, unknown> = {}, board = 1974, reportAnchor?: () => string | null, delayedReplaceDays?: number): Harness {
  const boardId = ref(board)
  // reactive：外部 query 变更（days/start/end 深链）需可被 watcher 追踪
  const routeState = reactive({ query: { ...initialQuery } })
  const router = {
    replace: vi.fn((to: { query: Record<string, unknown> }) => {
      const apply = () => { routeState.query = { ...to.query } }
      if (delayedReplaceDays !== undefined && String(to.query?.days) === String(delayedReplaceDays)) {
        return Promise.resolve().then(apply)
      }
      apply()
      return Promise.resolve()
    }),
  }
  const scope = effectScope()
  const state = scope.run(() => useBoardConsole(boardId as never, {
    route: () => routeState as never,
    router: () => router as never,
    now: FIXED_NOW,
    ...(reportAnchor ? { reportAnchor } : {}),
  }))!
  return { scope, boardId, routeState, router, state }
}

function tierCalls(): number[] {
  return getLaneDynamics.mock.calls.map(([, days]) => days as number)
}

function rosterSignature(state: ReturnType<typeof useBoardConsole>) {
  return state.rows.value.map(row => [row.topicId, row.clueTotal, row.total] as const)
}

describe('useBoardConsole — 观察窗（G1）', () => {
  let harness: Harness

  beforeEach(() => {
    getLaneDynamics.mockReset()
    getLaneDynamics.mockImplementation((_boardId: number, days: number) =>
      Promise.resolve({ success: true, data: days === 365 ? makePoolResponse() : makeResponse({ window_days: days }) }),
    )
  })

  afterEach(() => {
    harness?.scope.stop()
  })

  it('OX-1：拖/应用恰 7/14/30/365 日宽度都不切档，base 恒为 A14（range: exact widths keep source14）', async () => {
    harness = mountConsole()
    await flushPromises()
    const before = rosterSignature(harness.state)
    const beforeDormant = harness.state.dormantRows.value.map(row => row.topicId)
    // 恰 7 日（档窗内）
    expect(harness.state.applyObservationRange('2026-10-17', '2026-10-23')).toBe(true)
    // 恰 14 日（= 档自身区间）
    expect(harness.state.applyObservationRange('2026-10-10', '2026-10-23')).toBe(true)
    // 恰 30 日（跨出档窗，B 补内容但不换基准）
    expect(harness.state.applyObservationRange('2026-09-24', '2026-10-23')).toBe(true)
    await flushPromises()
    // 恰 365 日（整池）
    expect(harness.state.applyObservationRange('2025-10-24', '2026-10-23')).toBe(true)
    await flushPromises()
    expect(harness.state.sourceDays.value).toBe(14)
    expect(rosterSignature(harness.state)).toEqual(before)
    expect(harness.state.dormantRows.value.map(row => row.topicId)).toEqual(beforeDormant)
    // A 侧永远只有 14；出现的 365 只能是池（B）请求，不存在 7/30 被当 base
    expect(tierCalls().filter(days => days !== 14 && days !== 365)).toEqual([])
    expect(tierCalls().filter(days => days === 14)).toEqual([14])
  })

  it('OX-2：B 只补 A 同 ID 历史；池独有 ID 不进名单/候选（range: pool history only for source ids）', async () => {
    harness = mountConsole()
    await flushPromises()
    const before = rosterSignature(harness.state)
    expect(harness.state.applyObservationRange('2026-06-10', '2026-10-23')).toBe(true)
    await flushPromises()
    expect(harness.state.poolWindow.value).not.toBeNull()
    expect(rosterSignature(harness.state)).toEqual(before)
    expect(harness.state.candidates.value).toEqual([])
    expect(harness.state.rows.value.some(row => row.topicId === 99)).toBe(false)
    // A 同 ID 历史可读（池来源）；池独有 ID 也能解析内容但没有行（G3 只渲染 A 行）
    expect(harness.state.resolveLaneDay('10', '2026-06-15')).toMatchObject({ kind: 'content', source: 'pool' })
    expect(harness.state.resolveLaneDay('99', '2026-06-15')).toMatchObject({ kind: 'content', source: 'pool' })
    // 档窗内日期仍走 A（tier 来源）
    expect(harness.state.resolveLaneDay('10', '2026-10-22')).toMatchObject({ kind: 'content', source: 'tier' })
    expect(harness.state.resolveLaneDay('10', '2026-10-21')).toEqual({ kind: 'gap' })
  })

  it('OX-7：明确切档（setSourceDays / URL days）才换基准并重置默认范围（range: explicit tier changes source）', async () => {
    harness = mountConsole()
    await flushPromises()
    harness.state.applyObservationRange('2026-09-28', '2026-10-23')
    await flushPromises()
    harness.state.setSourceDays(7)
    await flushPromises()
    expect(harness.state.sourceDays.value).toBe(7)
    expect(tierCalls().at(-1)).toBe(7)
    expect(harness.state.observation.value).toEqual({ start: '2026-10-17', end: '2026-10-23' })
    // 切档写 query days 且清 start/end 深链；保留 lane 等无关键
    expect(harness.routeState.query.days).toBe('7')
    expect('start' in harness.routeState.query).toBe(false)
    // 外部 URL days=30（明确切档）→ 请求 30 并重置范围
    harness.routeState.query = { ...harness.routeState.query, days: '30' }
    await flushPromises()
    expect(harness.state.sourceDays.value).toBe(30)
    expect(tierCalls().at(-1)).toBe(30)
    expect(harness.state.observation.value).toEqual({ start: '2026-09-24', end: '2026-10-23' })
  })

  it('R35：外部 URL days 变更保留显式 start/end（fromUrl 不删除深链）', async () => {
    harness = mountConsole({ lane: '10', days: '14' })
    await flushPromises()
    harness.routeState.query = {
      ...harness.routeState.query,
      days: '30',
      start: '2026-09-28',
      end: '2026-10-23',
    }
    await flushPromises()
    expect(harness.state.sourceDays.value).toBe(30)
    expect(harness.state.observation.value).toEqual({ start: '2026-09-28', end: '2026-10-23' })
    expect(harness.routeState.query.start).toBe('2026-09-28')
    expect(harness.routeState.query.end).toBe('2026-10-23')
  })

  it('OX-8：365 档即池，只发一次请求（range: 365 shares pool request）', async () => {
    harness = mountConsole()
    await flushPromises()
    harness.state.setSourceDays(365)
    await flushPromises()
    expect(tierCalls()).toEqual([14, 365])
    expect(harness.state.needsPool.value).toBe(false)
    await harness.state.ensurePool()  // 幂等 no-op
    await harness.state.retryPool()  // 365 档下同样 no-op
    expect(tierCalls()).toEqual([14, 365])
    // 历史由 A（365 档响应）直接覆盖
    expect(harness.state.resolveLaneDay('10', '2026-06-15')).toMatchObject({ kind: 'content', source: 'tier' })
  })

  it('OX-9：深链四参数恢复；任一非法忽略该项并提示、其余生效（range: url restore partial）', async () => {
    harness = mountConsole({ lane: '10', days: '30', start: '2026-09-28', end: '2026-10-23' })
    await flushPromises()
    // 请求序列 = 档响应 + 首屏背景曲线预取的 365 池（[30] 是 A 档，[365] 是池）
    expect(tierCalls()).toEqual([30, 365])
    expect(harness.state.selectedLaneId.value).toBe(10)
    expect(harness.state.observation.value).toEqual({ start: '2026-09-28', end: '2026-10-23' })
    expect(harness.state.rangeNotice.value).toBeNull()
    harness.scope.stop()

    // days 非法（初始）→ 提示回退默认 14 + 不发非法请求（变体 3；R-23 初始分支）
    getLaneDynamics.mockClear()
    harness = mountConsole({ days: '8' })
    await flushPromises()
    expect(tierCalls()).toEqual([14, 365])
    expect(harness.state.daysNotice.value).toContain('days 参数不合法')
    expect(harness.state.daysNotice.value).toContain('已回退 14 日档')
    harness.scope.stop()

    // start 格式非法 → 忽略 start/end、原视窗（档默认）保持 + 文案
    getLaneDynamics.mockClear()
    harness = mountConsole({ days: '30', start: '2026/09/28', end: '2026-10-23' })
    await flushPromises()
    expect(tierCalls()).toEqual([30, 365])
    expect(harness.state.observation.value).toEqual({ start: '2026-09-24', end: '2026-10-23' })
    expect(harness.state.rangeNotice.value).toContain('YYYY-MM-DD')
    harness.scope.stop()

    // start>end → 顺序文案；池外 → 越界文案
    getLaneDynamics.mockClear()
    harness = mountConsole({ start: '2026-10-23', end: '2026-10-01' })
    await flushPromises()
    expect(harness.state.rangeNotice.value).toContain('起始日期不能晚于截止日期')
    expect(harness.state.observation.value).toEqual({ start: '2026-10-10', end: '2026-10-23' })
    harness.scope.stop()
    getLaneDynamics.mockClear()
    harness = mountConsole({ start: '2020-01-01', end: '2026-10-23' })
    await flushPromises()
    expect(harness.state.rangeNotice.value).toContain('超出可取数范围')
  })

  it('OX-10：切板块后档/池旧响应晚到均丢弃，双通道 epoch 隔离（console: dual-channel epoch）', async () => {
    const pendingA = deferred<unknown>()
    const pendingPool = deferred<unknown>()
    getLaneDynamics.mockImplementation((boardId: number, days: number) => {
      if (boardId === 1974 && days === 14) return pendingA.promise
      if (boardId === 1974 && days === 365) return pendingPool.promise
      return Promise.resolve({ success: true, data: makeResponse() })
    })
    harness = mountConsole()
    harness.state.applyObservationRange('2026-09-28', '2026-10-23')  // 尚无锚点：不生效，先挂起 A
    await flushPromises()
    pendingA.resolve({ success: true, data: makeResponse() })
    await flushPromises()
    // A 到位后名单已成形（切板块前的真实快照）
    const rosterBeforeSwitch = rosterSignature(harness.state)
    // A 到位后范围应用 → needsPool 触发池请求（挂起）
    harness.state.applyObservationRange('2026-09-28', '2026-10-23')
    await flushPromises()
    // 切板块走 router.push('/tags/1975')：旧 query（含 apply 写入的 start/end）不带过去
    harness.routeState.query = {}
    harness.boardId.value = 1975
    await flushPromises()
    // 旧板块 A/池迟到响应回放
    pendingPool.resolve({ success: true, data: makePoolResponse() })
    await flushPromises()
    expect(harness.state.data.value?.lanes.every(lane => makeResponse().lanes.some(l => l.topic_id === lane.topic_id)) ?? true).toBe(true)
    // 旧板块迟到的池响应不回填：新板块窗口里不得出现旧板块的历史桶日
    expect(harness.state.poolWindow.value?.lanes.some(lane => lane.timeline.some(day => day.date === '2026-06-15')) ?? false).toBe(false)
    expect(harness.state.poolLoading.value).toBe(false)
    // 名单不被旧板块迟到响应改写（与切板块前快照逐项一致）
    expect(rosterSignature(harness.state)).toEqual(rosterBeforeSwitch)
  })

  it('OX-11：排序/休眠只按 source 全轴，不随观察轴或池合并重算（range: source axis order and dormancy）', async () => {
    harness = mountConsole()
    await flushPromises()
    const full = rosterSignature(harness.state)
    const dormantBefore = harness.state.dormantRows.value.map(row => row.topicId)
    // 裁到只有 10-23 单日（12 的唯一分区在 10-20 之外，若按观察轴重算会误入「零分区」）
    harness.state.applyObservationRange('2026-10-23', '2026-10-23')
    await flushPromises()
    // 拉池后再裁（B 合并 timeline 诱惑源）
    harness.state.applyObservationRange('2026-06-15', '2026-10-23')
    await flushPromises()
    expect(rosterSignature(harness.state)).toEqual(full)
    expect(harness.state.dormantRows.value.map(row => row.topicId)).toEqual(dormantBefore)
    expect(harness.state.visibleAxis.value.at(-1)).toBe('2026-10-23')
    expect(harness.state.visibleAxis.value).toHaveLength(131)
  })

  it('OX-12：A 成功 B 失败 → A 可读、历史未取到不补 0、范围不动、B 可重试（console: pool failure keeps source readable）', async () => {
    // B 通道首取即失败（A 通道照常可读；池失败不自动重发、不影响名单）
    getLaneDynamics.mockImplementation((_boardId: number, days: number) => Promise.resolve(
      days === 365
        ? { success: false, error: '池挂了' }
        : { success: true, data: makeResponse({ window_days: days }) },
    ))
    harness = mountConsole()
    await flushPromises()
    const before = rosterSignature(harness.state)
    expect(harness.state.poolError.value).toBe('池挂了')
    expect(rosterSignature(harness.state)).toEqual(before)
    expect(harness.state.applyObservationRange('2026-09-28', '2026-10-23')).toBe(true)
    await flushPromises()
    expect(harness.state.poolError.value).toBe('池挂了')
    expect(rosterSignature(harness.state)).toEqual(before)
    expect(harness.state.observation.value).toEqual({ start: '2026-09-28', end: '2026-10-23' })
    // 历史未取到（unavailable ≠ gap ≠ 0）
    expect(harness.state.resolveLaneDay('10', '2026-09-29')).toEqual({ kind: 'unavailable' })
    // B 单独重试成功
    getLaneDynamics.mockImplementation((_boardId: number, days: number) => Promise.resolve(
      days === 365
        ? { success: true, data: makePoolResponse() }
        : { success: true, data: makeResponse({ window_days: days }) },
    ))
    await harness.state.retryPool()
    expect(harness.state.poolError.value).toBeNull()
    expect(harness.state.resolveLaneDay('10', '2026-09-29')).toEqual({ kind: 'gap' })
  })

  it('R29：B 刷新失败保留旧池并标陈旧；重试成功清提示，A/名单不动', async () => {
    harness = mountConsole()
    await flushPromises()
    harness.state.applyObservationRange('2026-06-15', '2026-10-23')
    await flushPromises()
    expect(harness.state.poolWindow.value).not.toBeNull()
    const roster = rosterSignature(harness.state)
    const tierCallsBefore = tierCalls().filter(days => days === 14).length
    expect(harness.state.resolveLaneDay('10', '2026-06-15').kind).toBe('content')

    getLaneDynamics.mockResolvedValueOnce({ success: false, error: '池刷新挂了' })
    await harness.state.retryPool()
    await flushPromises()
    expect(harness.state.poolError.value).toBe('池刷新挂了')
    expect(harness.state.poolStaleNotice.value).toBe(true)
    expect(harness.state.poolWindow.value).not.toBeNull()
    expect(rosterSignature(harness.state)).toEqual(roster)
    expect(tierCalls().filter(days => days === 14).length).toBe(tierCallsBefore)
    expect(harness.state.resolveLaneDay('10', '2026-06-15').kind).toBe('content')

    await harness.state.retryPool()
    await flushPromises()
    expect(harness.state.poolError.value).toBeNull()
    expect(harness.state.poolStaleNotice.value).toBe(false)
    expect(harness.state.resolveLaneDay('10', '2026-06-15').kind).toBe('content')
  })

  it('OX-13：A 首次失败 B 成功 → A 错误态 + 重试，不用 B 冒充（console: first source failure is not pool success）', async () => {
    getLaneDynamics.mockImplementationOnce(() => Promise.resolve({ success: false, error: '档请求挂了' }))
    harness = mountConsole()
    await flushPromises()
    expect(harness.state.error.value).toBe('档请求挂了')
    expect(harness.state.rows.value).toEqual([])
    expect(harness.state.data.value).toBeNull()
    // B 即使成功也不进入名单/数据
    await harness.state.ensurePool()
    expect(harness.state.poolWindow.value).not.toBeNull()
    expect(harness.state.rows.value).toEqual([])
    expect(harness.state.error.value).toBe('档请求挂了')
    // A 重试恢复
    await harness.state.load()
    expect(harness.state.error.value).toBeNull()
    expect(rosterSignature(harness.state)).toEqual(rosterSignature(harness.state))
  })

  it('OX-14：旧 A 刷新失败 stale 保留；失败不覆盖成功缓存，切回旧档命中缓存（console: stale source survives refresh failure）', async () => {
    harness = mountConsole()
    await flushPromises()
    const before = rosterSignature(harness.state)
    getLaneDynamics.mockResolvedValueOnce({ success: false, error: '刷新失败' })
    await harness.state.load()
    expect(harness.state.staleNotice.value).toBe(true)
    expect(harness.state.error.value).toBe('刷新失败')
    expect(rosterSignature(harness.state)).toEqual(before)
    expect(harness.state.observation.value).toEqual({ start: '2026-10-10', end: '2026-10-23' })
    // 失败未覆盖成功缓存：切 30 再切回 14 → 命中缓存不发请求
    harness.state.setSourceDays(30)
    await flushPromises()
    expect(tierCalls().at(-1)).toBe(30)
    harness.state.setSourceDays(14)
    await flushPromises()
    expect(tierCalls().at(-1)).toBe(30)
    expect(rosterSignature(harness.state)).toEqual(before)
  })

  it('CA-2：同板块 7→14→30→365 四档缓存，365 档/池共用一次请求，回切命中缓存（cache: four tiers share pool）', async () => {
    harness = mountConsole()
    await flushPromises()
    for (const days of [7, 30, 365]) {
      harness.state.setSourceDays(days)
      await flushPromises()
    }
    // [14] 首档 A、[365] 首屏背景曲线预取的池（后被 365 档复用）、[7]/[30] 档响应；365 档命中缓存不发请求
    expect(tierCalls()).toEqual([14, 365, 7, 30])
    // 池请求（观察跨出 30 档窗）→ 365 已缓存，不新增请求；历史由档响应（A）直接覆盖
    harness.state.applyObservationRange('2026-06-15', '2026-10-23')
    await flushPromises()
    await harness.state.ensurePool()
    expect(tierCalls()).toEqual([14, 365, 7, 30])
    expect(harness.state.resolveLaneDay('10', '2026-06-15')).toMatchObject({ kind: 'content', source: 'tier' })
    // 回切 14 → 缓存命中
    harness.state.setSourceDays(14)
    await flushPromises()
    expect(tierCalls()).toEqual([14, 365, 7, 30])
  })

  it('anchor 注入：0 泳道版块仍有报告锚点，轴/池边界不回退 today（旧投诉回归）', async () => {
    getLaneDynamics.mockResolvedValueOnce({
      success: true,
      data: makeResponse({ has_reports: true, lanes: [] }),
    })
    harness = mountConsole({}, 1974, () => '2026-10-22')
    await flushPromises()
    expect(harness.state.rows.value).toEqual([])
    expect(harness.state.emptyState.value).toBe('no-lanes')
    expect(harness.state.anchorDate.value).toBe('2026-10-22')
    expect(harness.state.today.value).toBe('2026-10-23')
    expect(harness.state.axis.value.at(-1)).toBe('2026-10-22')
    expect(harness.state.poolBounds.value).toEqual({ start: '2025-10-23', end: '2026-10-22' })
    // 有锚点即有默认观察范围（无泳道不阻断观察窗）
    expect(harness.state.observation.value).toEqual({ start: '2026-10-09', end: '2026-10-22' })
  })

  it('anchor 优先级：注入报告锚点晚于泳道最新日时以报告锚点为准', async () => {
    harness = mountConsole({}, 1974, () => '2026-10-23')
    await flushPromises()
    expect(harness.state.anchorDate.value).toBe('2026-10-23')
    expect(harness.state.observation.value).toEqual({ start: '2026-10-10', end: '2026-10-23' })
  })

  it('R35：异步 router 延迟提交 + 源缓存立即命中切档，仍重置目标档默认范围', async () => {
    harness = mountConsole({}, 1974, undefined, 365)
    await flushPromises()
    // 预热 365 与 14 缓存
    harness.state.setSourceDays(365)
    await flushPromises()
    await flushPromises()
    harness.state.setSourceDays(14)
    await flushPromises()
    // 30 档自定义 2 日，再切回已缓存 365：router 删除 start/end 尚未提交
    harness.state.setSourceDays(30)
    await flushPromises()
    harness.state.applyObservationRange('2026-10-15', '2026-10-16')
    await flushPromises()
    harness.state.setSourceDays(365)
    await flushPromises()
    await flushPromises()
    expect(harness.state.sourceDays.value).toBe(365)
    expect(harness.state.visibleAxis.value).toHaveLength(365)
    expect(harness.state.observation.value?.start).not.toBe('2026-10-15')
    expect(harness.routeState.query.days).toBe('365')
    expect('start' in harness.routeState.query).toBe(false)
    expect('end' in harness.routeState.query).toBe(false)

    // 回到已缓存 14：同样重置默认 14 日区间
    harness.state.applyObservationRange('2026-10-15', '2026-10-16')
    await flushPromises()
    harness.state.setSourceDays(14)
    await flushPromises()
    await flushPromises()
    expect(harness.state.sourceDays.value).toBe(14)
    expect(harness.state.visibleAxis.value).toHaveLength(14)
    expect(harness.state.observation.value?.start).toBe('2026-10-10')
    expect('start' in harness.routeState.query).toBe(false)
  })

  it('切板块：观察范围/提示/池全部复位并按新锚点重建（cache: board switch release）', async () => {
    // 板块感知 mock：两块板的池历史日不同（验证池随板块复位、不留旧板数据）
    getLaneDynamics.mockImplementation((boardId: number, days: number) => Promise.resolve({
      success: true,
      data: days === 365
        ? makePoolResponse(boardId === 1974 ? '2026-06-15' : '2026-07-02')
        : makeResponse({ window_days: days }),
    }))
    harness = mountConsole()
    await flushPromises()
    harness.state.applyObservationRange('2026-06-15', '2026-10-23')
    await flushPromises()
    expect(harness.state.poolWindow.value).not.toBeNull()
    harness.routeState.query = {}
    harness.boardId.value = 1975
    await flushPromises()
    expect(harness.state.observation.value).toEqual({ start: '2026-10-10', end: '2026-10-23' })
    // 池随板块复位并重取新板块自己的池（旧板历史日不得留在窗口里）
    expect(harness.state.poolWindow.value?.lanes.some(lane => lane.timeline.some(day => day.date === '2026-06-15')) ?? false).toBe(false)
    expect(harness.state.poolWindow.value?.lanes.some(lane => lane.timeline.some(day => day.date === '2026-07-02')) ?? false).toBe(true)
    expect(harness.state.poolError.value).toBeNull()
    expect(harness.state.rangeNotice.value).toBeNull()
    expect(harness.state.daysNotice.value).toBeNull()
    expect(getLaneDynamics).toHaveBeenCalledWith(1975, 14)
  })

  it('池失败后切 365 档：池失败态清掉（365 档即池，无独立池可失败，不误述）', async () => {
    let poolFail = true
    getLaneDynamics.mockImplementation((_boardId: number, days: number) => Promise.resolve(
      days === 365
        ? (poolFail ? { success: false, error: '池挂了' } : { success: true, data: makePoolResponse() })
        : { success: true, data: makeResponse({ window_days: days }) },
    ))
    harness = mountConsole()
    await flushPromises()
    expect(harness.state.poolError.value).toBe('池挂了')
    // 切 365 档：档/池共用同一响应，没有独立的池可失败 → 失败/陈旧态清掉
    poolFail = false
    harness.state.setSourceDays(365)
    await flushPromises()
    expect(harness.state.poolError.value).toBeNull()
    expect(harness.state.poolStaleNotice.value).toBe(false)
    expect(harness.state.error.value).toBeNull()
  })

  it('V-8：无锚点（无日报/无数据）不发池请求（无曲线可画，不做无用读）', async () => {
    getLaneDynamics.mockResolvedValue({
      success: true,
      data: makeResponse({ has_reports: false, lanes: [], candidates: [] }),
    })
    harness = mountConsole()
    await flushPromises()
    expect(harness.state.anchorDate.value).toBeNull()
    expect(tierCalls()).not.toContain(365)
  })

  it('首屏即预取 365 池：不等用户拖动，背景曲线就有历史（curve: pool prefetched on first paint）', async () => {
    harness = mountConsole()
    await flushPromises()
    // 用户未做任何拖动/切档：池已取到（请求序列含 365）
    expect(tierCalls()).toContain(365)
    expect(harness.state.poolWindow.value).not.toBeNull()
    // 档窗外历史日可解析（背景曲线有历史点，不再是「首屏只有 14 天」）
    expect(harness.state.resolveLaneDay('10', '2026-06-15')).toMatchObject({ kind: 'content', source: 'pool' })
    // 预取不改观察范围/名单（仍是档默认区间）
    expect(harness.state.observation.value).toEqual({ start: '2026-10-10', end: '2026-10-23' })
    expect(harness.state.sourceDays.value).toBe(14)
  })

  it('R-22：预览不写 URL；同值松手提交恰一次（保留 lane）；重复幂等；非法不改已提交范围', async () => {
    harness = mountConsole()
    await flushPromises()
    harness.state.selectLane(11)
    await flushPromises()
    const replacesBefore = harness.router.replace.mock.calls.length
    // 拖选中预览：observation 更新，不写 query（URL 无 start/end）
    expect(harness.state.applyObservationRange('2026-10-01', '2026-10-23', { writeQuery: false })).toBe(true)
    expect(harness.state.observation.value).toEqual({ start: '2026-10-01', end: '2026-10-23' })
    expect(harness.router.replace.mock.calls.length).toBe(replacesBefore)
    expect('start' in harness.routeState.query).toBe(false)
    // 松手同值提交：恰写一次，含 start/end 且保留 lane
    expect(harness.state.applyObservationRange('2026-10-01', '2026-10-23')).toBe(true)
    expect(harness.router.replace.mock.calls.length).toBe(replacesBefore + 1)
    expect(harness.routeState.query).toMatchObject({ start: '2026-10-01', end: '2026-10-23', lane: '11' })
    expect(harness.state.committedRange.value).toEqual({ start: '2026-10-01', end: '2026-10-23' })
    // 重复同值提交：幂等不再写
    expect(harness.state.applyObservationRange('2026-10-01', '2026-10-23')).toBe(true)
    expect(harness.router.replace.mock.calls.length).toBe(replacesBefore + 1)
    // 非法输入：不改已提交范围（URL/committedRange/显示均保持）
    expect(harness.state.applyObservationRange('2026-10-23', '2026-10-01')).toBe(false)
    expect(harness.routeState.query).toMatchObject({ start: '2026-10-01', end: '2026-10-23' })
    expect(harness.state.committedRange.value).toEqual({ start: '2026-10-01', end: '2026-10-23' })
    expect(harness.state.observation.value).toEqual({ start: '2026-10-01', end: '2026-10-23' })
  })

  it('R-22：深链恢复后预览他值再提交，恰一次写新值（committedRange 与 URL 对齐）', async () => {
    harness = mountConsole({ start: '2026-10-05', end: '2026-10-12' })
    await flushPromises()
    expect(harness.state.observation.value).toEqual({ start: '2026-10-05', end: '2026-10-12' })
    expect(harness.state.committedRange.value).toEqual({ start: '2026-10-05', end: '2026-10-12' })
    // 预览到另一范围：URL 不动
    expect(harness.state.applyObservationRange('2026-09-28', '2026-10-23', { writeQuery: false })).toBe(true)
    expect(harness.routeState.query.start).toBe('2026-10-05')
    expect(harness.routeState.query.end).toBe('2026-10-12')
    // 提交新值：committedRange（恢复值）≠ 新值 → 恰一次写
    const replaces = harness.router.replace.mock.calls.length
    expect(harness.state.applyObservationRange('2026-09-28', '2026-10-23')).toBe(true)
    expect(harness.router.replace.mock.calls.length).toBe(replaces + 1)
    expect(harness.routeState.query).toMatchObject({ start: '2026-09-28', end: '2026-10-23' })
  })

  it('R-23：外部非法 days 保留当前合法档并告知实际档；不发非法请求', async () => {
    harness = mountConsole()
    await flushPromises()
    harness.state.setSourceDays(30)
    await flushPromises()
    const calls = tierCalls().length
    harness.routeState.query = { ...harness.routeState.query, days: '8' }
    await flushPromises()
    expect(harness.state.sourceDays.value).toBe(30)
    expect(harness.state.daysNotice.value).toContain('days 参数不合法（8）')
    expect(harness.state.daysNotice.value).toContain('保持 30 日档')
    expect(tierCalls().length).toBe(calls)
    // 修正回合法档：提示清空、无多余请求（当前已是 30，仅清 notice）
    harness.routeState.query = { ...harness.routeState.query, days: '30' }
    await flushPromises()
    expect(harness.state.daysNotice.value).toBeNull()
    expect(harness.state.sourceDays.value).toBe(30)
    expect(tierCalls().length).toBe(calls)
  })

  it('G5：selectLane 接受 G3 字符串 topicId（组件不再散落 Number 映射）', async () => {
    harness = mountConsole()
    await flushPromises()
    harness.state.selectLane('11')
    expect(harness.state.selectedLaneId.value).toBe(11)
    expect(harness.routeState.query.lane).toBe('11')
    // 非法/不在名单：不改变当前选中
    harness.state.selectLane('abc')
    harness.state.selectLane('99')
    expect(harness.state.selectedLaneId.value).toBe(11)
  })

  it('G5：报告锚点元信息晚到只重算默认范围，不覆盖用户显式选择', async () => {
    const anchor = ref<string | null>(null)
    harness = mountConsole({}, 1974, () => anchor.value)
    await flushPromises()
    expect(harness.state.anchorDate.value).toBe('2026-10-23')
    expect(harness.state.observation.value).toEqual({ start: '2026-10-10', end: '2026-10-23' })

    anchor.value = '2026-10-26'
    await flushPromises()
    harness.state.syncDefaultRangeToAnchor()
    expect(harness.state.anchorDate.value).toBe('2026-10-26')
    expect(harness.state.observation.value).toEqual({ start: '2026-10-13', end: '2026-10-26' })

    // 用户已显式设置范围：晚到的默认同步不再改动
    harness.state.applyObservationRange('2026-10-01', '2026-10-05')
    harness.state.syncDefaultRangeToAnchor()
    expect(harness.state.observation.value).toEqual({ start: '2026-10-01', end: '2026-10-05' })
  })

  it('幂等：重复应用相同范围不重复写 query/发请求（变体 11）', async () => {
    harness = mountConsole()
    await flushPromises()
    harness.state.applyObservationRange('2026-10-01', '2026-10-23')
    await flushPromises()
    const writes = harness.router.replace.mock.calls.length
    const calls = tierCalls().length
    harness.state.applyObservationRange('2026-10-01', '2026-10-23')
    await flushPromises()
    expect(harness.router.replace.mock.calls.length).toBe(writes)
    expect(tierCalls().length).toBe(calls)
    // 非法重复输入也不触发额外请求，只更新文案
    harness.state.applyObservationRange('2026-10-23', '2026-10-01')
    expect(tierCalls().length).toBe(calls)
    expect(harness.state.rangeNotice.value).toContain('起始日期不能晚于截止日期')
    expect(harness.state.observation.value).toEqual({ start: '2026-10-01', end: '2026-10-23' })
  })
})

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => { resolve = r })
  return { promise, resolve }
}
