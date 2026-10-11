import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AppToggle from '~/components/ui/AppToggle.vue'

const healthMock = vi.hoisted(() => ({
  getHealth: vi.fn(),
  setAutoStartModels: vi.fn(),
  getDegradations: vi.fn(),
}))

vi.mock('~/api', () => ({
  useAIAdminApi: () => healthMock,
}))

const routerPush = vi.hoisted(() => vi.fn())
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush }),
  useRoute: () => ({ query: {} }),
}))

import SettingsSectionAiHealth from './SettingsSectionAiHealth.vue'

const snapshotFixture = {
  healthy: false,
  checked_at: '2026-08-02T10:00:00Z',
  auto_start_models: false,
  routes: [
    {
      route_name: 'default', capability: 'summary', primary_model: 'p-llm', vendor: 'v-llm',
      model_kind: 'llm', reachable: true, launched_by_backend: false,
      last_checked: '2026-08-02T10:00:00Z', error: '',
    },
    {
      route_name: 'default', capability: 'embedding', primary_model: 'p-emb', vendor: 'v-emb',
      model_kind: 'embedding', reachable: false, launched_by_backend: true,
      last_checked: '2026-08-02T10:00:00Z', error: 'connection refused',
    },
  ],
}

function mountPanel() {
  return mount(SettingsSectionAiHealth, {
    global: { stubs: { Icon: true } },
  })
}

describe('SettingsSectionAiHealth', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    healthMock.getHealth.mockResolvedValue({ success: true, data: snapshotFixture })
    healthMock.setAutoStartModels.mockResolvedValue({ success: true, data: { enabled: true } })
    healthMock.getDegradations.mockResolvedValue({
      success: true,
      data: { window: '24h', stats: { calls: 3, degradations: 1, failures: 0 }, events: [] },
    })
  })

  it('展示整体健康徽标与各路由通断/拉起明细', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.text()).toContain('未就绪')
    expect(wrapper.text()).toContain('p-llm')
    expect(wrapper.text()).toContain('p-emb')
    expect(wrapper.text()).toContain('通')
    expect(wrapper.text()).toContain('断')
    expect(wrapper.text()).toContain('已拉起')
    expect(wrapper.text()).toContain('connection refused')
  })

  it('checked_at 为 null 时显示检测中', async () => {
    healthMock.getHealth.mockResolvedValue({
      success: true,
      data: { ...snapshotFixture, checked_at: null },
    })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.text()).toContain('检测中')
  })

  it('拨动 auto_start_models 开关调用 PUT 接口', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.findComponent(AppToggle).find('.app-toggle__track').trigger('click')
    await flushPromises()

    expect(healthMock.setAutoStartModels).toHaveBeenCalledWith(true)
  })

  it('开关保存失败时回滚并提示', async () => {
    healthMock.setAutoStartModels.mockResolvedValue({ success: false, error: '保存失败' })
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.findComponent(AppToggle).find('.app-toggle__track').trigger('click')
    await flushPromises()

    expect(wrapper.findComponent(AppToggle).props('modelValue')).toBe(false)
    expect(wrapper.text()).toContain('保存失败')
  })

  it('主模型+供应商两列对齐 DTO（primary_model+vendor），无「主 provider」残留文案', async () => {
    const wrapper = mountPanel()
    await flushPromises()

    const headers = wrapper.findAll('th').map(th => th.text())
    expect(headers).toContain('主模型')
    expect(headers).toContain('供应商')
    expect(headers).not.toContain('主 provider')
    expect(wrapper.text()).toContain('p-llm')
    expect(wrapper.text()).toContain('v-llm')
  })

  it('降级观测块挂在健康快照下方；事件「定位」跳能力路由 section 并携带 focus_capability', async () => {
    healthMock.getDegradations.mockResolvedValue({
      success: true,
      data: {
        window: '24h',
        stats: { calls: 3, degradations: 1, failures: 0 },
        events: [{
          occurred_at: '2026-09-28T02:14:33Z',
          capability: 'topic_tagging',
          from: 0,
          to: 1,
          model: 'qwythos',
          type: 'fallback',
          code: 'timeout',
        }],
      },
    })
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.text()).toContain('降级观测')
    expect(wrapper.text()).not.toContain('窗口内无记录') // 事件非空，不显空态

    await wrapper.find('.evt-row__locate').trigger('click')
    expect(routerPush).toHaveBeenCalledWith({ query: { section: 'capability-routes', focus_capability: 'topic_tagging' } })
  })
})
