import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import DiscoveryWorkspace from './DiscoveryWorkspace.vue'

/**
 * S15 三页签导航（ui-design IA：三同级页签 + URL query 惯例 + 键盘可达）。
 * 「为你推荐」沿用 DiscoveryPanel、「兴趣记录」为 5.2 真实 InterestRecords（此处 stub 隔离 store 依赖）；
 * go-ask 冒泡验证：切回为你推荐并聚焦常驻查询输入框（R1 入口常驻，不藏在兴趣页签）。
 */

const routeQuery = reactive<Record<string, string>>({})
const replaceMock = vi.fn((to: { query?: Record<string, string> }) => {
  for (const k of Object.keys(routeQuery)) delete routeQuery[k]
  if (to.query) Object.assign(routeQuery, to.query)
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: routeQuery }),
  useRouter: () => ({ replace: replaceMock }),
}))

vi.mock('./DiscoveryPanel.vue', () => ({
  // 带真实 id 的输入框：验证 go-ask 聚焦目标（#discovery-query-input）存在
  default: { name: 'DiscoveryPanel', template: '<div data-testid="stub-panel"><input id="discovery-query-input" data-testid="stub-query-input" /></div>' },
}))

vi.mock('./CandidateLibrary.vue', () => ({
  default: { name: 'CandidateLibrary', template: '<div data-testid="stub-library" />' },
}))

vi.mock('./InterestRecords.vue', () => ({
  default: {
    name: 'InterestRecords',
    emits: ['go-ask'],
    template: '<div data-testid="stub-interest" @click="$emit(\'go-ask\')" />',
  },
}))

function mountWorkspace() {
  return mount(DiscoveryWorkspace, { attachTo: document.body })
}

beforeEach(() => {
  replaceMock.mockClear()
  for (const k of Object.keys(routeQuery)) delete routeQuery[k]
  document.body.innerHTML = ''
})

describe('DiscoveryWorkspace — 三页签结构', () => {
  it('渲染 tablist 与三个同级页签，默认选中「为你推荐」', () => {
    const wrapper = mountWorkspace()
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs.map(t => t.text())).toEqual(['为你推荐', '候选源库', '兴趣记录'])
    expect(tabs[0]!.attributes('aria-selected')).toBe('true')
    expect(tabs[1]!.attributes('aria-selected')).toBe('false')
    // roving tabindex：仅活动页签可 Tab 聚焦
    expect(tabs[0]!.attributes('tabindex')).toBe('0')
    expect(tabs[1]!.attributes('tabindex')).toBe('-1')
    wrapper.unmount()
  })

  it('默认显示现有 DiscoveryPanel（不破坏现有功能）', () => {
    const wrapper = mountWorkspace()
    expect(wrapper.find('[data-testid="stub-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="stub-library"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('?tab=library 直达候选源库', async () => {
    routeQuery.tab = 'library'
    const wrapper = mountWorkspace()
    await nextTick()
    expect(wrapper.find('[data-testid="stub-library"]').exists()).toBe(true)
    expect(wrapper.find('[role="tab"][data-testid="tab-library"]').attributes('aria-selected')).toBe('true')
    wrapper.unmount()
  })

  it('非法 tab 值回退为你推荐', () => {
    routeQuery.tab = 'nonsense'
    const wrapper = mountWorkspace()
    expect(wrapper.find('[data-testid="stub-panel"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('兴趣记录页签挂载 InterestRecords（5.2 替换占位）', async () => {
    routeQuery.tab = 'interest'
    const wrapper = mountWorkspace()
    await nextTick()
    expect(wrapper.find('[data-testid="stub-interest"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="stub-panel"]').exists()).toBe(false)
    wrapper.unmount()
  })
})

describe('DiscoveryWorkspace — 键盘导航（可达性）', () => {
  it('ArrowRight 依次切换：recommend → library → interest → 回环', async () => {
    const wrapper = mountWorkspace()
    const nav = wrapper.find('[role="tablist"]')
    await nav.trigger('keydown', { key: 'ArrowRight' })
    await nextTick()
    expect(replaceMock).toHaveBeenCalledWith({ query: { tab: 'library' } })
    expect(wrapper.find('[data-testid="stub-library"]').exists()).toBe(true)

    await nav.trigger('keydown', { key: 'ArrowRight' })
    await nextTick()
    expect(wrapper.find('[data-testid="stub-interest"]').exists()).toBe(true)

    await nav.trigger('keydown', { key: 'ArrowRight' })
    await nextTick()
    // 回环回默认页签：query 移除 tab（兼容旧链接形态）
    expect(replaceMock).toHaveBeenLastCalledWith({ query: {} })
    expect(wrapper.find('[data-testid="stub-panel"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('Home 直达第一个页签', async () => {
    routeQuery.tab = 'interest'
    const wrapper = mountWorkspace()
    await nextTick()
    await wrapper.find('[role="tablist"]').trigger('keydown', { key: 'Home' })
    await nextTick()
    expect(wrapper.find('[data-testid="stub-panel"]').exists()).toBe(true)
    wrapper.unmount()
  })
})

describe('DiscoveryWorkspace — 点击切换走 router.replace', () => {
  it('点击候选源库页签更新 query 且不产生多余历史', async () => {
    const wrapper = mountWorkspace()
    await wrapper.find('[data-testid="tab-library"]').trigger('click')
    await nextTick()
    expect(replaceMock).toHaveBeenCalledTimes(1)
    expect(replaceMock).toHaveBeenCalledWith({ query: { tab: 'library' } })
    expect(routeQuery.tab).toBe('library')
    wrapper.unmount()
  })
})

describe('DiscoveryWorkspace — go-ask 冒泡（兴趣空态找源入口）', () => {
  it('InterestRecords 发出 go-ask：切回为你推荐并聚焦常驻查询框', async () => {
    routeQuery.tab = 'interest'
    const wrapper = mountWorkspace()
    await nextTick()
    expect(wrapper.find('[data-testid="stub-interest"]').exists()).toBe(true)

    await wrapper.find('[data-testid="stub-interest"]').trigger('click') // stub 以 click 代 emit go-ask
    await nextTick()
    await nextTick() // switchTab 内部 nextTick 后才 focus
    expect(replaceMock).toHaveBeenLastCalledWith({ query: {} })
    expect(wrapper.find('[data-testid="stub-panel"]').exists()).toBe(true)
    expect(document.activeElement?.id).toBe('discovery-query-input') // 聚焦查询输入框
    wrapper.unmount()
  })
})
