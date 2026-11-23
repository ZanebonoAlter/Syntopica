/**
 * BoardConsoleShell — 右轨选中后 **URL 与实际视图一致**（add-key-event-matrix-view A-34 回归）。
 *
 * 抓的坑（2026-11-23 复验 A-34）：右轨「话题追踪」条目选中后画面已切回矩阵，URL 却是
 * `?view=events&lane=1804`——刷新该 URL 又回到关键事件视图（违反 design D6「视图状态走
 * URL、刷新保持」）。根因是同一动作里两次**独立** `router.replace` 争抢同一组 query key：
 * `handleSetView('matrix')` 删掉 `query.view`，紧接着的 `selectLaneFromStage` 又基于**尚未
 * 更新**的 `route.query` 重建 query，把 `view=events` 写回去。
 *
 * 为什么单开一个文件：既有 `BoardConsoleShellRailViewSwitch.test.ts` 的路由桩是
 * **同步落地**的（`replace` 立刻改 `route.query`），第二次写入自然能读到第一次的结果 →
 * 竞争被测试桩藏起来，上一轮就是这么漏网的。本文件把桩换成真实 vue-router 的时序
 * （**异步落地**：导航完成前 `route.query` 仍是旧快照），竞争才会显形。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import {
  installShellTestHooks,
  routeState,
  router,
  listWatches,
} from './boardConsoleShellTestHarness'
import { mature, mountReady } from './boardConsoleShellOtherFixtures'

// 关键事件批量读：本套件只关心视图与 URL 的一致性，事件内容一律空（不伪造节点）。
const getBoardKeyEvents = vi.hoisted(() => vi.fn())
vi.mock('~/api/boardKeyEvents', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/boardKeyEvents')>()
  return {
    ...original,
    useBoardKeyEventsApi: () => ({ getBoardKeyEvents }),
  }
})

installShellTestHooks()

/** 待落地的 query（真实 router.replace 是异步的：导航落地前 route.query 仍是旧快照）。 */
let pendingQuery: Record<string, unknown> | null = null

beforeEach(() => {
  pendingQuery = null
  // 延迟落地：replace 只记账，路由真正变化在 flushRoute() 里发生（模拟 vue-router 异步导航）。
  router.replace.mockImplementation((to: { query: Record<string, unknown> }) => {
    pendingQuery = { ...to.query }
    return Promise.resolve()
  })
})

/** 让待落地的 query 真正生效（等价于 vue-router 导航完成）。 */
async function flushRoute(): Promise<void> {
  await flushPromises()
  if (pendingQuery) {
    routeState.query = pendingQuery
    pendingQuery = null
  }
  await flushPromises()
}

/** 关注记录：persistentTopicId = 11 才能定位到 A 名单里的「主线 B」泳道。 */
function watchFixture() {
  return [{
    id: '5',
    semanticBoardId: '1974',
    label: '主线 B',
    type: 'label',
    persistentTopicId: '11',
    status: 'active',
    createdAt: '',
    updatedAt: '',
  }]
}

describe('BoardConsoleShell — 右轨选中后 URL 与实际视图一致（A-34）', () => {
  it('右轨「话题追踪」选中：URL 不残留 view=events，且用该 URL 重新挂载仍是矩阵视图', async () => {
    mature()
    listWatches.mockResolvedValue({ success: true, data: watchFixture() })
    getBoardKeyEvents.mockResolvedValue({ success: true, data: { events: [] } })
    routeState.query = { view: 'events' }
    const wrapper = await mountReady()

    // 前提：确实站在关键事件视图（否则本案测不到「切回来」）
    expect(wrapper.find('[data-testid="key-event-view"]').exists()).toBe(true)

    const replaceCallsBefore = router.replace.mock.calls.length
    await wrapper.find('[data-testid="rail-watch-locate-5"]').trigger('click')
    await flushRoute()

    // 画面已切回矩阵
    expect(wrapper.find('[data-testid="board-matrix"]').exists(), '右轨选中后应切回矩阵视图').toBe(true)
    expect(wrapper.find('[data-testid="key-event-view"]').exists()).toBe(false)

    // URL 与画面一致：矩阵是默认视图 → query 不带 view；选中的泳道带 lane
    expect(routeState.query.view, 'URL 不得残留 view=events（否则刷新该 URL 又回到关键事件视图）').toBeUndefined()
    expect(routeState.query.lane).toBe('11')

    // 同一动作只发一次 replace：两次独立 replace 会基于同一份旧快照重建 query → 后写覆盖前写
    const replaceCalls = router.replace.mock.calls.slice(replaceCallsBefore)
    expect(replaceCalls.length, '同一动作里的 query 写入必须合并成一次 replace').toBe(1)
    expect(replaceCalls[0]![0].query.view, '唯一一次 replace 也不得带 view=events').toBeUndefined()

    // 用该 URL 重新挂载 → 仍是矩阵视图（刷新保持，design D6）
    const remounted = await mountReady()
    expect(remounted.find('[data-testid="board-matrix"]').exists(), '用该 URL 重挂载应仍是矩阵视图').toBe(true)
    expect(remounted.find('[data-testid="key-event-view"]').exists()).toBe(false)
  })

  it('同类路径检查：右轨「当日其他话题」选中后 URL 同样不带 view（该路径只写一次 query）', async () => {
    mature()
    getBoardKeyEvents.mockResolvedValue({ success: true, data: { events: [] } })
    routeState.query = { view: 'events' }
    const wrapper = await mountReady()
    expect(wrapper.find('[data-testid="key-event-view"]').exists()).toBe(true)

    const replaceCallsBefore = router.replace.mock.calls.length
    await wrapper.find('[data-testid="rail-other-topic-item-1200"] button').trigger('click')
    await flushRoute()

    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(true)
    expect(routeState.query.view, 'URL 不得残留 view=events').toBeUndefined()
    expect(routeState.query.lane, '「当日其他话题」不写 lane（既有语义不变）').toBeUndefined()
    const replaceCalls = router.replace.mock.calls.slice(replaceCallsBefore)
    expect(replaceCalls.length, '该路径只写一次 query（无第二次 replace 竞争）').toBe(1)
  })
})
