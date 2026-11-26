/**
 * BoardConsoleShell — 骨架与最新日报独立锚（8C G4b 从 1094 行巨型壳测试按组拆出；用例逐字保留，
 * fullName/断言不变）。通用 mock/挂载/hooks 见 `boardConsoleShellTestHarness.ts`。
 */
import { describe, expect, it } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import {
  installShellTestHooks,
  mountShell,
  mounted,
  routeState,
  getLaneDynamics,
  getOverview,
  getBoardDailyReports,
  BOARD_LABEL,
  emptyResponse,
  overview,
  response,
  TODAY,
  router,
} from './boardConsoleShellTestHarness'

installShellTestHooks()

describe('BoardConsoleShell — 骨架与最新日报独立锚', () => {
  it('渲染顶栏/三区/版块栏；最新日报来自版块 report 元信息', async () => {
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="console-breadcrumb"]').text()).toContain('叙事工坊')
    expect(wrapper.text()).toContain(BOARD_LABEL)
    expect(wrapper.find('[data-testid="board-rail"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-evolution"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="board-context-rail"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="board-rail-item-1974"]').classes()).toContain('is-active')
    expect(wrapper.find('[data-testid="console-today-report"]').text()).toContain('10/23')
  })

  it('0 active 泳道但已有日报：报日入口仍可见；旧横幅不再出现，网格内联与其他话题组在场，且不被泳道最大日替代', async () => {
    getLaneDynamics.mockResolvedValue({ success: true, data: emptyResponse() })
    getOverview.mockResolvedValue(overview('2026-10-24'))
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="console-today-report"]').text()).toContain('10/24')
    expect(wrapper.find('[data-testid="console-no-lanes-fallback"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="matrix-empty-no-lanes"]').text()).toContain('0 正式泳道')
    expect(wrapper.find('[data-testid="other-topics-group"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="console-fallback-report"]').exists()).toBe(false)
    await wrapper.find('[data-testid="console-today-report"]').trigger('click')
    expect(router.push).toHaveBeenCalledWith({ path: '/tags/1974/2026-10-24', query: {} })
  })

  it('R28：生成完成先刷新独立元信息再刷新 A；新报告日进入头部与源 axis', async () => {
    const newDay = '2026-10-24'
    getOverview
      .mockResolvedValueOnce(overview(TODAY))
      .mockResolvedValue(overview(newDay))
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="console-today-report"]').text()).toContain('10/23')

    wrapper.findComponent({ name: 'DailyReportGenerateDialog' }).vm.$emit('completed')
    await flushPromises()
    await flushPromises()
    expect(getOverview).toHaveBeenCalledTimes(2)
    expect(getLaneDynamics).toHaveBeenLastCalledWith(1974, 14)
    expect(wrapper.find('[data-testid="console-today-report"]').text()).toContain('10/24')
    expect(wrapper.find(`[data-testid="matrix-cell-10-${newDay}"]`).exists()).toBe(true)
  })

  it('R28：生成对话框关闭也刷新元信息与 A（失败不丢旧值）', async () => {
    getOverview
      .mockResolvedValueOnce(overview(TODAY))
      .mockResolvedValue(overview('2026-10-25'))
    const wrapper = mountShell()
    await flushPromises()
    wrapper.findComponent({ name: 'DailyReportGenerateDialog' }).vm.$emit('cancel')
    await flushPromises()
    await flushPromises()
    expect(getOverview).toHaveBeenCalledTimes(2)
    expect(getLaneDynamics).toHaveBeenLastCalledWith(1974, 14)
    expect(wrapper.find('[data-testid="console-today-report"]').text()).toContain('10/25')
  })

  it('R28：0 active 泳道也随候选新日报更新头部；不靠 active 最大日', async () => {
    getOverview
      .mockResolvedValueOnce(overview(TODAY))
      .mockResolvedValue(overview('2026-10-25'))
    getLaneDynamics
      .mockResolvedValueOnce({ success: true, data: response() })
      .mockResolvedValue({ success: true, data: emptyResponse() })
    const wrapper = mountShell()
    await flushPromises()

    wrapper.findComponent({ name: 'DailyReportGenerateDialog' }).vm.$emit('completed')
    await flushPromises()
    await flushPromises()
    expect(wrapper.find('[data-testid="console-today-report"]').text()).toContain('10/25')
    expect(wrapper.find('[data-testid="console-no-lanes-fallback"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="matrix-empty-no-lanes"]').text()).toContain('0 正式泳道')
  })

  it('R28：显式刷新元信息失败保留上次成功值并标陈旧，重试按钮在场', async () => {
    getOverview.mockResolvedValueOnce(overview(TODAY))
    const wrapper = mountShell()
    await flushPromises()
    getOverview.mockResolvedValue({ success: false, error: 'overview 500' })
    getBoardDailyReports.mockResolvedValue({ success: false, error: '列表 500' })

    await wrapper.find('[data-testid="console-refresh"]').trigger('click')
    await flushPromises()
    await flushPromises()
    expect(wrapper.find('[data-testid="console-today-report"]').text()).toContain('10/23')
    expect(wrapper.find('[data-testid="console-refresh"]').text()).toContain('元信息陈旧')
    expect(wrapper.find('[data-testid="console-refresh"]').attributes('title')).toContain('保留上次成功元信息')

    getOverview.mockResolvedValue(overview('2026-10-25'))
    await wrapper.find('[data-testid="console-refresh"]').trigger('click')
    await flushPromises()
    await flushPromises()
    expect(wrapper.find('[data-testid="console-today-report"]').text()).toContain('10/25')
    expect(wrapper.find('[data-testid="console-refresh"]').text()).not.toContain('元信息陈旧')
  })

  it('日报元信息两个来源都失败：按钮禁用且明确「不代表没有历史日报」', async () => {
    getOverview.mockResolvedValue({ success: false, error: 'overview 500' })
    getBoardDailyReports.mockResolvedValue({ success: false, error: '列表 500' })
    const wrapper = mountShell()
    await flushPromises()
    const button = wrapper.find('[data-testid="console-today-report"]')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.text()).toContain('元信息未取到')
    expect(button.attributes('title')).toContain('不代表该版块没有历史日报')
  })

  it('R27：挂载/卸载不写全局 html 根字号（角色字号由组件局部承担）', async () => {
    const previous = document.documentElement.style.fontSize
    document.documentElement.style.fontSize = '17px'
    const wrapper = mountShell()
    await flushPromises()
    expect(document.documentElement.style.fontSize).toBe('17px')
    wrapper.unmount()
    mounted.pop()
    expect(document.documentElement.style.fontSize).toBe('17px')
    document.documentElement.style.fontSize = previous
  })

  it('?lane= 指向休眠泳道：自动展开并选中（1.3/MD-7）', async () => {
    routeState.query = { lane: '12' }
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="matrix-lane-12"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-evolution"]').text()).toContain('休眠线 C')
  })

  it('点泳道名切换选中并写 query（1.3）', async () => {
    const wrapper = mountShell()
    await flushPromises()
    await wrapper.find('[data-testid="matrix-lane-11"]').trigger('click')
    expect(routeState.query.lane).toBe('11')
    expect(wrapper.find('[data-testid="lane-evolution"]').text()).toContain('主线 B')
  })
})

