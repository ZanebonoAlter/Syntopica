/**
 * BoardConsoleShell — 入口/管理/真实路由（含返回上下文与滚动）（8C G4b 从 1094 行巨型壳测试按组拆出；用例逐字保留，
 * fullName/断言不变）。通用 mock/挂载/hooks 见 `boardConsoleShellTestHarness.ts`。
 */
import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { flushPromises } from '@vue/test-utils'
import {
  installShellTestHooks,
  mountShell,
  mounted,
  routeState,
  router,
  applyQuery,
  mediaControl,
  openArticlePreviewMock,
  getLaneDynamics,
  getDailyReportDetail,
  reportDetail,
  TODAY,
} from './boardConsoleShellTestHarness'
import LaneChronicle from './LaneChronicle.vue'

installShellTestHooks()

describe('BoardConsoleShell — 入口/管理/真实路由', () => {
  it('撤旧窗口 select 与更多菜单；右轨无全局六项，无重复新建版块', async () => {
    routeState.query = { days: '7' }
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="console-window-select"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="console-more"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="gov-advanced"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="board-rail-add"]').exists()).toBe(false)
  })

  it('研究工作台走真实路由，并保存返回上下文（泳道/日期/档/范围/滚动）', async () => {
    const wrapper = mountShell()
    await flushPromises()
    await wrapper.find('[data-testid="console-view-research"]').trigger('click')
    expect(router.push).toHaveBeenCalledWith('/tags/1974/research')
    const stored = window.sessionStorage.getItem('syntopica:board-console:return:1974')
    expect(stored).not.toBeNull()
    const parsed = JSON.parse(stored!) as Record<string, unknown>
    expect(Object.keys(parsed)).toEqual(['laneId', 'date', 'days', 'start', 'end', 'scrollTop'])
    expect(parsed).not.toHaveProperty('report')
    expect(parsed).not.toHaveProperty('annotation')
  })

  it('板块管理菜单：设置/构成进入单级宽工作面；泳道管理打开既有面板', async () => {
    const wrapper = mountShell()
    await flushPromises()
    await wrapper.find('[data-testid="console-manage-settings"]').trigger('click')
    expect(wrapper.find('[data-testid="workface-stub"]').attributes('data-open')).toBe('true')
    expect(wrapper.find('[data-testid="workface-stub"]').attributes('data-tab')).toBe('settings')

    await wrapper.find('[data-testid="console-manage-composition"]').trigger('click')
    expect(wrapper.find('[data-testid="workface-stub"]').attributes('data-tab')).toBe('quality')

    // 泳道实体管理项显示为「泳道管理」（lane-manual-correction-loop 6.4）
    const manageTopics = wrapper.find('[data-testid="console-manage-topics"]')
    expect(manageTopics.text()).toBe('泳道管理')
    await manageTopics.trigger('click')
    expect(wrapper.find('[data-testid="topic-manage-stub"]').attributes('data-open')).toBe('true')
  })

  it('中等宽度上下文入口打开同一工作面 context 页（不是空抽屉）', async () => {
    const wrapper = mountShell()
    await flushPromises()
    await wrapper.find('[data-testid="console-gov-toggle"]').trigger('click')
    expect(wrapper.find('[data-testid="workface-stub"]').attributes('data-open')).toBe('true')
    expect(wrapper.find('[data-testid="workface-stub"]').attributes('data-tab')).toBe('context')
  })

  it('从阅读台返回：URL 显式 lane 优先，缓存恢复范围/日期/滚动，焦点回选中泳道', async () => {
    window.sessionStorage.setItem('syntopica:board-console:return:1974', JSON.stringify({
      laneId: '10',
      date: TODAY,
      days: 14,
      start: '2026-10-10',
      end: '2026-10-23',
      scrollTop: 120,
    }))
    routeState.query = { focus: 'report', lane: '11' }
    const wrapper = mountShell()
    await flushPromises()
    await flushPromises()

    // URL 显式 lane 不被缓存覆盖；范围来自缓存并写回 URL；focus 消费后移除
    expect(routeState.query.lane).toBe('11')
    expect(routeState.query.start).toBe('2026-10-10')
    expect(routeState.query.end).toBe('2026-10-23')
    expect(routeState.query.focus).toBeUndefined()
    expect(wrapper.find('[data-testid="lane-evolution"]').text()).toContain('主线 B')
    expect(document.activeElement).toBe(wrapper.find('[data-testid="matrix-lane-11"]').element)
  })

  it('R30：异步 router 下返回上下文合并为一次完整 query，start/end/lane/days 不互相取消', async () => {
    window.sessionStorage.setItem('syntopica:board-console:return:1974', JSON.stringify({
      laneId: '10',
      date: TODAY,
      days: 14,
      start: '2026-10-10',
      end: '2026-10-23',
      scrollTop: 0,
    }))
    const applied: Array<Record<string, unknown>> = []
    router.replace.mockImplementation(async (to: { query: Record<string, unknown> }) => {
      await Promise.resolve()
      routeState.query = { ...to.query }
      applied.push({ ...to.query })
    })

    const wrapper = mountShell()
    await flushPromises()
    await flushPromises()
    expect(applied.length).toBe(1)
    expect(applied[0]).toMatchObject({
      lane: '10',
      days: '14',
      start: '2026-10-10',
      end: '2026-10-23',
    })
    expect(routeState.query).toMatchObject({
      lane: '10',
      days: '14',
      start: '2026-10-10',
      end: '2026-10-23',
    })
    wrapper.unmount()
    mounted.pop()
    router.replace.mockImplementation(applyQuery)
  })

  it('R30：URL 显式范围/lane 与非法 days 优先，不被返回缓存覆盖', async () => {
    window.sessionStorage.setItem('syntopica:board-console:return:1974', JSON.stringify({
      laneId: '10',
      date: TODAY,
      days: 30,
      start: '2026-10-01',
      end: '2026-10-05',
      scrollTop: 0,
    }))
    routeState.query = { days: '8', start: '2026-10-10', end: '2026-10-23', lane: '11' }
    const wrapper = mountShell()
    await flushPromises()
    await flushPromises()
    expect(getLaneDynamics).toHaveBeenCalledWith(1974, 14)
    expect(routeState.query.days).toBe('8')
    expect(routeState.query.start).toBe('2026-10-10')
    expect(routeState.query.end).toBe('2026-10-23')
    expect(routeState.query.lane).toBe('11')
    expect(wrapper.find('[data-testid="observation-range-feedback"]').text()).toContain('days 参数不合法')
  })

  it('R34：阅读 pending 不耗布局帧；详情就绪且内容增高后恢复深滚动', async () => {
    window.sessionStorage.setItem('syntopica:board-console:return:1974', JSON.stringify({
      laneId: '10',
      date: TODAY,
      days: 14,
      start: '2026-10-10',
      end: TODAY,
      scrollTop: 382,
    }))
    let resolveDetail!: (value: unknown) => void
    getDailyReportDetail.mockReturnValue(new Promise((resolve) => { resolveDetail = resolve }))
    const frames: FrameRequestCallback[] = []
    const rafSpy = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback: FrameRequestCallback) => {
      frames.push(callback)
      return frames.length
    })
    const cafSpy = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
    const removeSpy = vi.spyOn(window, 'removeEventListener')

    const wrapper = mountShell()
    await flushPromises()
    await flushPromises()
    const stage = wrapper.find('.console-stage').element as HTMLElement
    let scrollHeight = 402 // clientHeight 100 → max 302（旧失败位置）
    Object.defineProperty(stage, 'clientHeight', { value: 100, configurable: true })
    Object.defineProperty(stage, 'scrollHeight', { get: () => scrollHeight, configurable: true })
    stage.scrollTop = 0

    // 详情仍未返回：不得开始布局恢复，也不得消耗任何帧
    for (let i = 0; i < 250 && frames.length > 0; i += 1) frames.shift()!(0)
    expect(frames).toHaveLength(0)
    expect(stage.scrollTop).toBe(0)

    resolveDetail(reportDetail())
    await flushPromises()
    await flushPromises()
    expect(frames.length).toBeGreaterThan(0)
    frames.shift()!(0)
    await nextTick()
    expect(stage.scrollTop).toBe(302)

    scrollHeight = 482 // 高度终态 → max 382
    frames.shift()!(0)
    await nextTick()
    expect(stage.scrollTop).toBe(382)
    expect(removeSpy.mock.calls.some(call => String(call[0]) === 'wheel')).toBe(true)

    rafSpy.mockRestore()
    removeSpy.mockRestore()
    cafSpy.mockRestore()
  })

  it('R34：pending 等详情期间 wheel 取消并摘监听，详情回来后不得强拉', async () => {
    window.sessionStorage.setItem('syntopica:board-console:return:1974', JSON.stringify({
      laneId: '10',
      date: TODAY,
      days: 14,
      start: '2026-10-10',
      end: TODAY,
      scrollTop: 382,
    }))
    let resolveDetail!: (value: unknown) => void
    getDailyReportDetail.mockReturnValue(new Promise((resolve) => { resolveDetail = resolve }))
    const addSpy = vi.spyOn(window, 'addEventListener')
    const removeSpy = vi.spyOn(window, 'removeEventListener')

    const wrapper = mountShell()
    await flushPromises()
    await flushPromises()
    const stage = wrapper.find('.console-stage').element as HTMLElement
    Object.defineProperty(stage, 'clientHeight', { value: 100, configurable: true })
    Object.defineProperty(stage, 'scrollHeight', { value: 402, configurable: true })
    stage.scrollTop = 0

    // pending 建立即挂监听（不是等 settled 才挂）
    expect(addSpy.mock.calls.some(call => String(call[0]) === 'wheel')).toBe(true)
    window.dispatchEvent(new Event('wheel'))
    expect(removeSpy.mock.calls.some(call => String(call[0]) === 'wheel')).toBe(true)

    resolveDetail(reportDetail())
    await flushPromises()
    await flushPromises()
    expect(stage.scrollTop).toBe(0)

    addSpy.mockRestore()
    removeSpy.mockRestore()
  })

  it('reader 关闭回到打开前的焦点，不清空矩阵/日期/范围', async () => {
    const wrapper = mountShell()
    await flushPromises()
    const laneButton = wrapper.find('[data-testid="matrix-lane-10"]').element as HTMLElement
    laneButton.focus()
    wrapper.findComponent(LaneChronicle).vm.$emit('openArticle', '42')
    expect(openArticlePreviewMock).toHaveBeenCalledWith(42)
    wrapper.findComponent({ name: 'ArticlePreviewModal' }).vm.$emit('close')
    await flushPromises()
    expect(document.activeElement).toBe(laneButton)
    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(true)
  })

  it('窄屏：不渲染矩阵/右轨，泳道摘要仍可换泳道并进入同一上下文工作面', async () => {
    mediaControl.set(true)
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="board-narrow-summary"]').exists()).toBe(true)

    await wrapper.find('[data-testid="narrow-lane-11"]').trigger('click')
    expect(routeState.query.lane).toBe('11')

    await wrapper.find('[data-testid="narrow-open-context"]').trigger('click')
    expect(wrapper.find('[data-testid="workface-stub"]').attributes('data-open')).toBe('true')
    expect(wrapper.find('[data-testid="workface-stub"]').attributes('data-tab')).toBe('context')
  })

  it('R31：非滚动 stage（手机/1280）保存实际 windowY，而不是 stage.scrollTop=0', async () => {
    Object.defineProperty(window, 'scrollY', { value: 120, configurable: true })
    const wrapper = mountShell()
    await flushPromises()
    const stage = wrapper.find('.console-stage').element as HTMLElement
    Object.defineProperty(stage, 'scrollHeight', { value: 100, configurable: true })
    Object.defineProperty(stage, 'clientHeight', { value: 200, configurable: true })
    await wrapper.find('[data-testid="console-view-research"]').trigger('click')
    const stored = JSON.parse(window.sessionStorage.getItem('syntopica:board-console:return:1974')!) as Record<string, unknown>
    expect(stored.scrollTop).toBe(120)
    wrapper.unmount()
    mounted.pop()
    Object.defineProperty(window, 'scrollY', { value: 0, configurable: true })
  })

  it('R31：非滚动 stage 恢复用 window.scrollTo（单一宿主）', async () => {
    window.sessionStorage.setItem('syntopica:board-console:return:1974', JSON.stringify({
      laneId: '10',
      date: TODAY,
      days: 14,
      start: '2026-10-10',
      end: '2026-10-23',
      scrollTop: 120,
    }))
    Object.defineProperty(document.documentElement, 'scrollHeight', { value: 1000, configurable: true })
    Object.defineProperty(window, 'innerHeight', { value: 800, configurable: true })
    const scrollSpy = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
    const wrapper = mountShell()
    await flushPromises()
    await flushPromises()
    expect(scrollSpy).toHaveBeenCalledWith(0, 120)
    scrollSpy.mockRestore()
    wrapper.unmount()
    mounted.pop()
    Object.defineProperty(document.documentElement, 'scrollHeight', { value: 0, configurable: true })
    Object.defineProperty(window, 'innerHeight', { value: 768, configurable: true })
  })

  it('R31：桌面真实滚动 stage 保存 stage.scrollTop（不混用 windowY）', async () => {
    Object.defineProperty(window, 'scrollY', { value: 120, configurable: true })
    const wrapper = mountShell()
    await flushPromises()
    const stage = wrapper.find('.console-stage').element as HTMLElement
    Object.defineProperty(stage, 'scrollHeight', { value: 500, configurable: true })
    Object.defineProperty(stage, 'clientHeight', { value: 200, configurable: true })
    stage.scrollTop = 80
    await wrapper.find('[data-testid="console-view-research"]').trigger('click')
    const stored = JSON.parse(window.sessionStorage.getItem('syntopica:board-console:return:1974')!) as Record<string, unknown>
    expect(stored.scrollTop).toBe(80)
    wrapper.unmount()
    mounted.pop()
    Object.defineProperty(window, 'scrollY', { value: 0, configurable: true })
  })

  it('追踪管理只以 boardId 打开（rail 管理入口）', async () => {
    const wrapper = mountShell()
    await flushPromises()
    await wrapper.find('[data-testid="rail-watch-manage"]').trigger('click')
    const panel = wrapper.find('[data-testid="watch-panel-stub"]')
    expect(panel.attributes('data-open')).toBe('true')
    expect(panel.attributes('data-board')).toBe('1974')

    // R33：话题候选接本板块话题管理，不误开新建泳道编排
    await wrapper.find('[data-testid="rail-topic-candidates"]').trigger('click')
    expect(wrapper.find('[data-testid="topic-manage-stub"]').attributes('data-open')).toBe('true')
    expect(wrapper.find('[data-testid="topic-manage-stub"]').attributes('data-board')).toBe('1974')
    expect(wrapper.find('[data-testid="compose-stub"]').exists()).toBe(false)
  })

  it('新建泳道：打开编排态；保存后关闭并重取泳道（5.4）', async () => {
    const wrapper = mountShell()
    await flushPromises()
    const callsBefore = getLaneDynamics.mock.calls.length
    await wrapper.find('[data-testid="console-new-lane"]').trigger('click')
    expect(wrapper.find('[data-testid="compose-stub"]').exists()).toBe(true)

    wrapper.findComponent({ name: 'BoardComposeOverlay' }).vm.$emit('saved', { id: 300 })
    await flushPromises()
    expect(wrapper.find('[data-testid="compose-stub"]').exists()).toBe(false)
    expect(getLaneDynamics.mock.calls.length).toBe(callsBefore + 1)
  })
})

