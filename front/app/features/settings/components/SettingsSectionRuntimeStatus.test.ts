import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, reactive } from 'vue'
import SettingsSectionRuntimeStatus from './SettingsSectionRuntimeStatus.vue'

/**
 * 复合 section「运行状态」（settings-workspace spec：复合 section 进入默认子 tab /
 * 子 tab 切换与 URL 承载 / 非法 tab 值回退）。子面板全部 stub 隔离。
 */

const routeQuery = reactive<Record<string, string>>({})
const replaceMock = vi.fn((to: { query?: Record<string, unknown> }) => {
  for (const k of Object.keys(routeQuery)) delete routeQuery[k]
  if (to.query) {
    for (const [k, v] of Object.entries(to.query)) {
      if (v !== undefined) routeQuery[k] = String(v)
    }
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: routeQuery }),
  useRouter: () => ({ replace: replaceMock }),
}))

vi.mock('./SettingsSectionAiHealth.vue', () => ({
  default: { name: 'AiHealthStub', template: '<div data-testid="panel-ai-health" />' },
}))
vi.mock('./SettingsSectionQueues.vue', () => ({
  default: { name: 'QueuesStub', template: '<div data-testid="panel-queues" />' },
}))
vi.mock('./SettingsSectionSchedulers.vue', () => ({
  default: { name: 'SchedulersStub', template: '<div data-testid="panel-schedulers" />' },
}))

beforeEach(() => {
  replaceMock.mockClear()
  for (const k of Object.keys(routeQuery)) delete routeQuery[k]
  routeQuery.section = 'runtime-status'
})

describe('SettingsSectionRuntimeStatus（复合 section 子 tab）', () => {
  it('渲染 3 个子 tab，默认落 AI 健康', () => {
    const wrapper = mount(SettingsSectionRuntimeStatus)
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs.map(t => t.text())).toEqual(['AI 健康', '队列', '定时任务'])
    expect(tabs[0]!.attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-testid="panel-ai-health"]').exists()).toBe(true)
  })

  it('切到定时任务：router.replace 携带 tab=schedulers，面板切换', async () => {
    const wrapper = mount(SettingsSectionRuntimeStatus)
    const tab = wrapper.findAll('[role="tab"]').find(t => t.text() === '定时任务')!
    await tab.trigger('click')

    expect(replaceMock).toHaveBeenCalledWith({ query: { section: 'runtime-status', tab: 'schedulers' } })
    await nextTick()
    expect(wrapper.find('[data-testid="panel-schedulers"]').exists()).toBe(true)
  })

  it('旧键 3 枚（ai-health/queues/schedulers）作为 tab 值全部合法', () => {
    for (const legacyKey of ['ai-health', 'queues', 'schedulers']) {
      for (const k of Object.keys(routeQuery)) delete routeQuery[k]
      routeQuery.section = 'runtime-status'
      routeQuery.tab = legacyKey
      const wrapper = mount(SettingsSectionRuntimeStatus)
      const selected = wrapper.findAll('[role="tab"]').find(t => t.attributes('aria-selected') === 'true')
      expect(selected, `旧键 ${legacyKey} 应命中子 tab`).toBeDefined()
      wrapper.unmount()
    }
  })

  it('非法 tab 值回默认（AI 健康）并清参数', async () => {
    routeQuery.tab = 'bogus'
    const wrapper = mount(SettingsSectionRuntimeStatus)
    await nextTick()

    expect(replaceMock).toHaveBeenCalledWith({ query: { section: 'runtime-status', tab: undefined } })
    expect(wrapper.find('[data-testid="panel-ai-health"]').exists()).toBe(true)
  })

  it('datasources-network 的 tab 值（如 proxy）对本 section 非法：回默认', async () => {
    routeQuery.tab = 'proxy'
    const wrapper = mount(SettingsSectionRuntimeStatus)
    await nextTick()

    expect(replaceMock).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="panel-ai-health"]').exists()).toBe(true)
  })
})
