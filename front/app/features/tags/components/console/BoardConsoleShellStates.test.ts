/**
 * BoardConsoleShell — 版块切换/归档/失败态（8C G4b 从 1094 行巨型壳测试按组拆出；用例逐字保留，
 * fullName/断言不变）。通用 mock/挂载/hooks 见 `boardConsoleShellTestHarness.ts`。
 */
import { describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import {
  installShellTestHooks,
  mountShell,
  routeState,
  router,
  mediaControl,
  getLaneDynamics,
  getBoards,
  getBoardHealthCheckup,
  TODAY,
} from './boardConsoleShellTestHarness'

installShellTestHooks()

// 关键事件视图数据源（add-key-event-matrix-view）：壳层集成测试不发真实请求。
const getBoardKeyEvents = vi.fn()
vi.mock('~/api/boardKeyEvents', () => ({
  useBoardKeyEventsApi: () => ({ getBoardKeyEvents }),
}))

describe('BoardConsoleShell — 版块切换/归档/失败态', () => {
  it('切版块：push 新路由、重取数据并清选中日', async () => {
    const wrapper = mountShell()
    await flushPromises()
    await wrapper.find(`[data-testid="matrix-cell-10-${TODAY}"]`).trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle"]').text()).toContain(TODAY)

    await wrapper.find('[data-testid="board-rail-item-1975"]').trigger('click')
    expect(router.push).toHaveBeenCalledWith('/tags/1975')

    routeState.params = { boardId: '1975' }
    routeState.path = '/tags/1975'
    await wrapper.setProps({ boardId: 1975 })
    await flushPromises()
    // 新板块 A 档重取（末位还有首屏背景曲线预取的 365 池请求，故不用「最后一次调用」断言）
    expect(getLaneDynamics).toHaveBeenCalledWith(1975, 14)
  })

  it('归档视图可切换：矩阵卸载、归档挂载，切回不重取泳道（R-9）', async () => {
    const wrapper = mountShell()
    await flushPromises()
    const callsBefore = getLaneDynamics.mock.calls.length
    await wrapper.find('[data-testid="console-view-archive"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="board-archive"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="lane-chronicle"]').exists()).toBe(false)
    expect(getLaneDynamics.mock.calls.length).toBe(callsBefore)

    await wrapper.find('[data-testid="console-view-matrix"]').trigger('click')
    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(true)
    expect(getLaneDynamics.mock.calls.length).toBe(callsBefore)
  })

  it('矩阵加载失败：内联错误但右轨照常（2.5）', async () => {
    getLaneDynamics.mockResolvedValue({ success: false, error: '泳道接口挂了' })
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="matrix-error"]').text()).toContain('泳道接口挂了')
    expect(wrapper.find('[data-testid="board-context-rail"]').exists()).toBe(true)
  })

  it('健康刷新失败保留上次快照（stale）而不是伪空 0', async () => {
    getBoardHealthCheckup
      .mockResolvedValueOnce({
        success: true,
        data: {
          latest: {
            id: 1,
            semantic_board_id: 1974,
            checked_at: '2026-10-23T23:00:00+08:00',
            metrics: { board: { matched_tags: 10, reason_dist: {}, ghost_matches: 0, vector_missing: 0 }, labels: [] },
            uncaptured_candidates: [],
            llm_score: 82,
            llm_comment: '构成贴合。',
            created_at: '',
          },
          trend: [],
        },
      })
      .mockResolvedValueOnce({ success: false, error: '刷新挂了' })
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-health-score"]').text()).toBe('82')

    // 成功后刷新失败：保留上次快照并标 stale（首错才是空态，不伪 0）
    await wrapper.find('[data-testid="rail-health-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="rail-health-stale"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="rail-health-score"]').text()).toBe('82')
  })

  it('版块栏加载失败：重试重新拉取', async () => {
    getBoards.mockReset()
      .mockResolvedValueOnce({ success: false, error: '版块列表挂了' })
      .mockResolvedValueOnce({ success: true, data: { items: [{ id: 1974, label: '中东', tag_count: 1 }], total: 1 } })
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="board-rail-retry"]').exists()).toBe(true)
    await wrapper.find('[data-testid="board-rail-retry"]').trigger('click')
    await flushPromises()
    expect(getBoards).toHaveBeenCalledTimes(2)
  })

  // ── 关键事件视图（add-key-event-matrix-view 3.x）──────────────────────────

  it('关键事件视图可切换：保留选中态与观察窗、URL 写 ?view=events、切回矩阵删除该键', async () => {
    getBoardKeyEvents.mockReset().mockResolvedValue({ success: true, data: { events: [] } })
    const wrapper = mountShell()
    await flushPromises()
    // 先在矩阵选中一个格（lane + date 落 query）
    await wrapper.find(`[data-testid="matrix-cell-10-${TODAY}"]`).trigger('click')
    await flushPromises()
    expect(routeState.query.lane).toBeDefined()

    await wrapper.find('[data-testid="console-view-events"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="key-event-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(false)
    // 分段按钮 aria-pressed 跟随当前视图
    expect(wrapper.find('[data-testid="console-view-events"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('[data-testid="console-view-matrix"]').attributes('aria-pressed')).toBe('false')
    // 切换 MUST NOT 重置选中泳道/选中日/观察窗（D6/spec）
    expect(routeState.query.lane).toBeDefined()
    expect(routeState.query.view).toBe('events')
    expect(routeState.query.days).toBeUndefined()
    // A 档不因视图切换重取
    const callsBefore = getLaneDynamics.mock.calls.length
    await wrapper.find('[data-testid="console-view-matrix"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="console-view-matrix"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('[data-testid="console-view-events"]').attributes('aria-pressed')).toBe('false')
    expect(routeState.query.view).toBeUndefined()
    expect(routeState.query.lane).toBeDefined()
    expect(getLaneDynamics.mock.calls.length).toBe(callsBefore)
  })

  it('URL ?view=events 直接进入事件视图；非法值回落 matrix', async () => {
    getBoardKeyEvents.mockReset().mockResolvedValue({ success: true, data: { events: [] } })
    routeState.query = { view: 'events' }
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="key-event-view"]').exists()).toBe(true)
    wrapper.unmount()

    routeState.query = { view: 'bogus' }
    const wrapper2 = mountShell()
    await flushPromises()
    expect(wrapper2.find('[data-testid="key-event-view"]').exists()).toBe(false)
    expect(wrapper2.find('[data-testid="board-matrix"]').exists()).toBe(true)
  })

  it('窄屏（<768px）不渲染关键事件按钮与视图，沿用版块摘要降级', async () => {
    getBoardKeyEvents.mockReset().mockResolvedValue({ success: true, data: { events: [] } })
    const wrapper = mountShell()
    await flushPromises()
    mediaControl.set(true)
    await flushPromises()
    // 窄屏：关键事件按钮与视图都不渲染（矩阵按钮是既有全宽入口，不在本 change 范围）
    expect(wrapper.find('[data-testid="console-view-events"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="key-event-view"]').exists()).toBe(false)
    // 窄屏摘要（BoardNarrowSummary）照常
    expect(wrapper.find('[data-testid="board-narrow-summary"]').exists()).toBe(true)
  })

  it('窄屏 + ?view=events 深链：回落矩阵（不渲染事件视图），阅读槽与窄屏摘要照常（review R-8）', async () => {
    getBoardKeyEvents.mockReset().mockResolvedValue({ success: true, data: { events: [] } })
    mediaControl.set(true)
    routeState.query = { view: 'events' }
    const wrapper = mountShell()
    await flushPromises()

    // 窄屏深链不得把用户卡在空舞台：不进事件视图形态
    expect(wrapper.find('[data-testid="key-event-view"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="board-narrow-summary"]').exists(), '窄屏摘要照常').toBe(true)
    // 阅读槽不得被连带隐藏（否则窄屏下无处可读）
    const hasReadingSlot = wrapper.find('[data-testid="console-reading-empty"]').exists()
      || wrapper.find('[data-testid="lane-chronicle"]').exists()
      || wrapper.find('[data-testid="other-topic-reading"]').exists()
    expect(hasReadingSlot, '窄屏深链不得把阅读槽一起隐藏').toBe(true)
    // URL 与画面一致（不再挂在 events 上）
    expect(routeState.query.view).toBeUndefined()
  })
})

