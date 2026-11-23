/**
 * BoardConsoleShell — 右轨发起的话题选中 → 切回「矩阵 / 追踪」视图（add-key-event-matrix-view 3.4，
 * design D14 / ui-design Interaction Decisions D4 / delta spec Scenario「右轨选中切回矩阵视图」）。
 *
 * 抓的坑（2026-11-23 用户实机：「我选了右侧的泳道等应该跳转回矩阵追踪视角」）：候选话题在关键
 * 事件视图里**不占行**（红线：候选不占行），泳道行只存在于矩阵 / 追踪视图——右轨选中若不切视图，
 * 就是一个没有任何视觉落点的死选择。判据必须在**发起处**区分：
 *  - 右轨发起（话题追踪条目 / 当日其他话题）→ 切矩阵 + 选中 + 滚入；
 *  - 本视图内部发起（点行头 / 点事件节点）→ 仍留在关键事件视图，不把用户踢出时间轴上下文。
 */
import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { flushPromises } from '@vue/test-utils'
import {
  installShellTestHooks,
  mountShell,
  routeState,
  listWatches,
} from './boardConsoleShellTestHarness'
import { mature, mountReady } from './boardConsoleShellOtherFixtures'

// 关键事件批量读：本套件只关心视图切换与选中落点，事件内容一律空（不伪造节点）。
const getBoardKeyEvents = vi.hoisted(() => vi.fn())
vi.mock('~/api/boardKeyEvents', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/boardKeyEvents')>()
  return {
    ...original,
    useBoardKeyEventsApi: () => ({ getBoardKeyEvents }),
  }
})

installShellTestHooks()

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

function spyScrollIntoView(): ReturnType<typeof vi.spyOn> {
  if (typeof HTMLElement.prototype.scrollIntoView !== 'function') {
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
      configurable: true, writable: true, value: () => {},
    })
  }
  return vi.spyOn(HTMLElement.prototype, 'scrollIntoView').mockImplementation(() => {})
}

describe('BoardConsoleShell — 右轨选中切回矩阵 / 追踪（design D14）', () => {
  it('关键事件视图里点右轨「话题追踪」条目：切回矩阵 + 该泳道被选中 + 滚入视野', async () => {
    mature()
    listWatches.mockResolvedValue({ success: true, data: watchFixture() })
    getBoardKeyEvents.mockResolvedValue({ success: true, data: { events: [] } })
    routeState.query = { view: 'events' }
    const wrapper = await mountReady()

    // 前提：确实站在关键事件视图（否则本案测不到「切回来」）
    expect(wrapper.find('[data-testid="key-event-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(false)
    const scrollSpy = spyScrollIntoView()

    await wrapper.find('[data-testid="rail-watch-locate-5"]').trigger('click')
    await flushPromises()
    await nextTick()

    expect(wrapper.find('[data-testid="key-event-view"]').exists(), '右轨选中后不得留在关键事件视图').toBe(false)
    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(true)
    // matrix 是默认视图，不占 query（D6 写入口径不变）
    expect(routeState.query.view).toBeUndefined()
    const lane = wrapper.find('[data-testid="matrix-lane-11"]')
    expect(lane.classes(), '选中泳道要高亮').toContain('is-selected')
    expect(scrollSpy.mock.instances, '选中泳道要滚入视野').toContain(lane.element)
    scrollSpy.mockRestore()
  })

  it('关键事件视图里点右轨「当日其他话题」：切回矩阵并落到既有阅读槽（不是死选择）', async () => {
    mature()
    getBoardKeyEvents.mockResolvedValue({ success: true, data: { events: [] } })
    routeState.query = { view: 'events' }
    const wrapper = await mountReady()
    expect(wrapper.find('[data-testid="key-event-view"]').exists()).toBe(true)

    await wrapper.find('[data-testid="rail-other-topic-item-1200"] button').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="key-event-view"]').exists()).toBe(false)
    expect(routeState.query.view).toBeUndefined()
    // 落点是既有单一阅读槽（其他分区阅读），不新造第二套
    const reading = wrapper.find('[data-testid="other-topic-reading"]')
    expect(reading.exists()).toBe(true)
    expect(reading.text()).toContain('别的分区甲')
  })

  it('视图内部选中（点行头）仍留在关键事件视图：不把用户踢出时间轴上下文', async () => {
    mature()
    listWatches.mockResolvedValue({ success: true, data: [] })
    getBoardKeyEvents.mockResolvedValue({ success: true, data: { events: [] } })
    routeState.query = { view: 'events' }
    const wrapper = await mountReady()

    const head = wrapper.find('[data-testid="key-event-row-10"]').find('.kev-row-head')
    await head.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="key-event-view"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(false)
    expect(routeState.query.view).toBe('events')
    expect(wrapper.find('[data-testid="key-event-row-10"]').find('.kev-row-head').classes()).toContain('is-selected')
  })

  it('矩阵里的「当日其他话题」选行不切换视图（舞台内发起，仍留在矩阵）', async () => {
    mature()
    getBoardKeyEvents.mockResolvedValue({ success: true, data: { events: [] } })
    const wrapper = mountShell()
    await flushPromises()
    await flushPromises()
    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(true)

    await wrapper.find('[data-testid="other-topics-toggle"] button').trigger('click')
    await wrapper.find('[data-testid="other-topic-row-1200"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(true)
    expect(routeState.query.view).toBeUndefined()
    expect(wrapper.find('[data-testid="other-topic-reading"]').exists()).toBe(true)
  })

  // ── 作用域收窄（review R-1）：切视图只发生在**关键事件视图**里 ──────────────

  it('归档视图里点右轨「话题追踪」：不切视图（归档上下文不丢）', async () => {
    mature()
    listWatches.mockResolvedValue({ success: true, data: watchFixture() })
    getBoardKeyEvents.mockResolvedValue({ success: true, data: { events: [] } })
    routeState.query = { view: 'archive' }
    const wrapper = await mountReady()
    expect(wrapper.find('[data-testid="board-archive"]').exists()).toBe(true)

    await wrapper.find('[data-testid="rail-watch-locate-5"]').trigger('click')
    await flushPromises()
    await nextTick()

    expect(wrapper.find('[data-testid="board-archive"]').exists(), '归档视图里的右轨选中不得静默切走视图').toBe(true)
    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(false)
    expect(routeState.query.view).toBe('archive')
  })

  it('归档视图里点右轨「当日其他话题」：不切视图（与舞台内选中同落点）', async () => {
    mature()
    getBoardKeyEvents.mockResolvedValue({ success: true, data: { events: [] } })
    routeState.query = { view: 'archive' }
    const wrapper = await mountReady()
    expect(wrapper.find('[data-testid="board-archive"]').exists()).toBe(true)

    await wrapper.find('[data-testid="rail-other-topic-item-1200"] button').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="board-archive"]').exists(), '归档视图里的右轨选中不得静默切走视图').toBe(true)
    expect(routeState.query.view).toBe('archive')
  })
})
