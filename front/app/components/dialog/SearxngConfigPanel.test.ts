import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { mount } from '@vue/test-utils'
import SearxngConfigPanel from './SearxngConfigPanel.vue'
import AppToggle from '~/components/ui/AppToggle.vue'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const api = vi.hoisted(() => ({
  getConfig: vi.fn(),
  saveSettings: vi.fn(),
}))

vi.mock('~/api', () => ({
  useSearxngApi: () => api,
}))

function mountSection() {
  return mount(SearxngConfigPanel)
}

function findSaveButton(wrapper: ReturnType<typeof mountSection>) {
  const btn = wrapper.findAll('button').find(b => b.text().includes('保存设置'))
  expect(btn, 'save button must exist').toBeDefined()
  return btn!
}

describe('SearxngConfigPanel（D7 联网后端配置）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('回显：GET 配置填充 endpoint 与 enabled 开关（WS-6 回显分支）', async () => {
    api.getConfig.mockResolvedValue({
      success: true,
      data: { endpoint: 'http://localhost:8889', enabled: true },
    })
    const wrapper = mountSection()
    await flushPromises()

    const input = wrapper.find('input[type="text"]')
    expect((input.element as HTMLInputElement).value).toBe('http://localhost:8889')
    // AppToggle 开着 → .is-active（范式：findComponent，见 SettingsSectionAiHealth.test）。
    expect(wrapper.findComponent(AppToggle).classes()).toContain('is-active')
  })

  it('保存：携带当前输入（endpoint trim + enabled），成功提示且即时生效文案（WS-6）', async () => {
    api.getConfig.mockResolvedValue({ success: true, data: { endpoint: '', enabled: false } })
    api.saveSettings.mockResolvedValue({ success: true, data: null })
    const wrapper = mountSection()
    await flushPromises()

    await wrapper.find('input[type="text"]').setValue('  http://localhost:8889/  ')
    // 打开开关（默认关）：点 toggle 轨道。
    await wrapper.findComponent(AppToggle).find('.app-toggle__track').trigger('click')
    await findSaveButton(wrapper).trigger('click')
    await flushPromises()

    expect(api.saveSettings).toHaveBeenCalledWith({ endpoint: 'http://localhost:8889/', enabled: true })
    expect(wrapper.text()).toContain('即时生效')
  })

  it('保存失败：行内错误提示，不抛错', async () => {
    api.getConfig.mockResolvedValue({ success: true, data: { endpoint: '', enabled: false } })
    api.saveSettings.mockResolvedValue({ success: false, error: 'boom' })
    const wrapper = mountSection()
    await flushPromises()

    await findSaveButton(wrapper).trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('保存 SearXNG 设置失败')
  })
})
