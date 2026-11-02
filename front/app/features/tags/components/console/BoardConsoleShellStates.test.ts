/**
 * BoardConsoleShell — 版块切换/归档/失败态（8C G4b 从 1094 行巨型壳测试按组拆出；用例逐字保留，
 * fullName/断言不变）。通用 mock/挂载/hooks 见 `boardConsoleShellTestHarness.ts`。
 */
import { describe, expect, it } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import {
  installShellTestHooks,
  mountShell,
  routeState,
  router,
  getLaneDynamics,
  getBoards,
  getBoardHealthCheckup,
  TODAY,
} from './boardConsoleShellTestHarness'

installShellTestHooks()

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
})

