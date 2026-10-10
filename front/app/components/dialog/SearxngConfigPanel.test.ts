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
  checkHealth: vi.fn(),
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

function findProbeButton(wrapper: ReturnType<typeof mountSection>) {
  const btn = wrapper.find('[data-testid="searxng-health-probe"]')
  expect(btn.exists(), 'probe button must exist').toBe(true)
  return btn
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

describe('SearxngConfigPanel 连通性探测（add-searxng-health-indicator）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getConfig.mockResolvedValue({ success: true, data: { endpoint: 'http://localhost:8889', enabled: true } })
  })

  it('HP-1 idle：初始无状态行，仅「测试连接」按钮可用', async () => {
    const wrapper = mountSection()
    await flushPromises()
    expect(findProbeButton(wrapper).exists()).toBe(true)
    expect(wrapper.find('[data-testid="searxng-health-status"]').exists()).toBe(false)
    expect((findProbeButton(wrapper).element as HTMLButtonElement).disabled).toBe(false)
  })

  it('HP-2 点击测试连接：调用探测 API，四态渲染对应文案与色点类', async () => {
    const cases = [
      { status: 'ok' as const, latency_ms: 123, detail: '搜索正常，返回 3 条结果' },
      { status: 'degraded' as const, latency_ms: 5000, detail: '服务可达但搜索无结果（引擎可能故障）' },
      { status: 'unreachable' as const, latency_ms: 8000, detail: '无法连接：超时' },
      { status: 'disabled' as const, latency_ms: 0, detail: '未配置或未启用' },
    ]
    for (const health of cases) {
      api.checkHealth.mockReset()
      api.checkHealth.mockResolvedValue({ success: true, data: health })
      const wrapper = mountSection()
      await flushPromises()

      await findProbeButton(wrapper).trigger('click')
      await flushPromises()

      expect(api.checkHealth).toHaveBeenCalledTimes(1)
      const statusEl = wrapper.find('[data-testid="searxng-health-status"]')
      expect(statusEl.exists()).toBe(true)
      expect(statusEl.find('.searxng-health-dot').classes()).toContain(`searxng-health-dot--${health.status}`)
      if (health.status === 'ok') expect(statusEl.text()).toContain(`${health.latency_ms}ms`)
      if (health.status === 'degraded') expect(statusEl.text()).toContain('搜索无结果')
      if (health.status === 'unreachable') expect(statusEl.text()).toContain('无法连接')
      if (health.status === 'disabled') expect(statusEl.text()).toContain('未启用/未配置')
      wrapper.unmount()
    }
  })

  it('HP-3 loading 防重复：探测中按钮 disabled，重复点击不重复发请求', async () => {
    let resolveProbe: (v: unknown) => void = () => {}
    api.checkHealth.mockReturnValue(new Promise(r => { resolveProbe = r }))
    const wrapper = mountSection()
    await flushPromises()

    await findProbeButton(wrapper).trigger('click')
    await flushPromises()
    const btn = findProbeButton(wrapper).element as HTMLButtonElement
    expect(btn.disabled).toBe(true)
    expect(btn.textContent).toContain('测试中')
    await findProbeButton(wrapper).trigger('click') // loading 中重复点击
    expect(api.checkHealth).toHaveBeenCalledTimes(1)

    resolveProbe({ success: true, data: { status: 'ok', latency_ms: 1, detail: '' } })
    await flushPromises()
    expect((findProbeButton(wrapper).element as HTMLButtonElement).disabled).toBe(false)
  })

  it('HP-4 保存成功后自动探测一次；探测失败不影响保存成功反馈', async () => {
    api.saveSettings.mockResolvedValue({ success: true, data: null })
    api.checkHealth.mockResolvedValue({ success: false, error: '探测后端炸了' })
    const wrapper = mountSection()
    await flushPromises()

    await findSaveButton(wrapper).trigger('click')
    await flushPromises()

    expect(api.checkHealth).toHaveBeenCalledTimes(1) // 保存成功自动触发一次探测
    expect(wrapper.text()).toContain('即时生效') // 探测失败不吞保存成功反馈
    expect(wrapper.find('[data-testid="searxng-health-error"]').text()).toContain('探测后端炸了') // 探测失败落状态行弱提示（透传后端错误摘要）
  })

  it('HP-5 探测请求异常（网络层）：行内弱提示，不抛错', async () => {
    api.checkHealth.mockRejectedValue(new Error('network down'))
    const wrapper = mountSection()
    await flushPromises()

    await findProbeButton(wrapper).trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="searxng-health-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="searxng-health-status"]').exists()).toBe(false)
  })
})
