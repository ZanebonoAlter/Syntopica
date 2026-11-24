import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AppToggle from '~/components/ui/AppToggle.vue'
import WebSearchConfigPanel from './WebSearchConfigPanel.vue'

/**
 * 网页搜索配置面板（switch-margin-note-search-to-deepseek / settings-workspace
 * spec「数据源连通性探测」）：无表单、无可编辑字段，只展示关联状态 + 按需探测。
 */

const api = vi.hoisted(() => ({
  getSettings: vi.fn(),
  getHealth: vi.fn(),
}))

vi.mock('~/api', () => ({
  useWebSearchApi: () => api,
}))

function mountPanel() {
  return mount(WebSearchConfigPanel)
}

function findProbeButton(wrapper: ReturnType<typeof mountPanel>) {
  const btn = wrapper.find('[data-testid="web-search-health-probe"]')
  expect(btn.exists(), 'probe button must exist').toBe(true)
  return btn
}

/** 通用探测准备：设置已关联供应商 → 点击「测试连接」→ 返回探测完成后的 wrapper。 */
async function probeWith(data: Record<string, unknown>) {
  api.getHealth.mockResolvedValue({ success: true, data })
  const wrapper = mountPanel()
  await flushPromises()
  await findProbeButton(wrapper).trigger('click')
  await flushPromises()
  return wrapper
}

describe('WebSearchConfigPanel 关联状态', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('已关联供应商：显示 vendor_name，测试连接可用', async () => {
    api.getSettings.mockResolvedValue({
      success: true,
      data: { vendor_configured: true, vendor_name: 'DeepSeek' },
    })
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.find('[data-testid="web-search-vendor"]').text()).toContain('已关联供应商：DeepSeek')
    expect(wrapper.find('[data-testid="web-search-vendor-missing"]').exists()).toBe(false)
    expect((findProbeButton(wrapper).element as HTMLButtonElement).disabled).toBe(false)
  })

  it('已关联但 vendor_name 为空：兜底显示「（未命名）」，不留悬空冒号', async () => {
    api.getSettings.mockResolvedValue({
      success: true,
      data: { vendor_configured: true, vendor_name: '' },
    })
    const wrapper = mountPanel()
    await flushPromises()

    const vendor = wrapper.find('[data-testid="web-search-vendor"]')
    expect(vendor.exists()).toBe(true)
    expect(vendor.text()).toContain('已关联供应商：（未命名）')
    expect(wrapper.find('[data-testid="web-search-vendor-missing"]').exists()).toBe(false)
  })

  it('未关联：中性空态 + 前往「AI 模型」指引，测试连接仍可点（探测给 disabled 中性态）', async () => {
    api.getSettings.mockResolvedValue({
      success: true,
      data: { vendor_configured: false, vendor_name: '' },
    })
    api.getHealth.mockResolvedValue({
      success: true,
      data: { status: 'disabled', latency_ms: 0, detail: '未关联可用的 DeepSeek 供应商' },
    })
    const wrapper = mountPanel()
    await flushPromises()

    const empty = wrapper.find('[data-testid="web-search-vendor-missing"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('「AI 模型」')
    expect(empty.text()).not.toContain('AI 路由')
    expect(wrapper.find('[data-testid="web-search-vendor"]').exists()).toBe(false)

    await findProbeButton(wrapper).trigger('click')
    await flushPromises()
    expect(api.getHealth).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="web-search-health-status"]').text()).toContain('未关联供应商')
  })

  it('读取中：测试连接禁用', async () => {
    let resolveSettings: (v: unknown) => void = () => {}
    api.getSettings.mockReturnValue(new Promise(r => { resolveSettings = r }))
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.find('[data-testid="web-search-settings-loading"]').exists()).toBe(true)
    expect((findProbeButton(wrapper).element as HTMLButtonElement).disabled).toBe(true)

    resolveSettings({ success: true, data: { vendor_configured: true, vendor_name: 'DeepSeek' } })
    await flushPromises()
    expect((findProbeButton(wrapper).element as HTMLButtonElement).disabled).toBe(false)
  })

  it('读取失败：行内错误 + 重试入口，重试重新请求', async () => {
    api.getSettings.mockResolvedValueOnce({ success: false, error: '读取后端炸了' })
    const wrapper = mountPanel()
    await flushPromises()

    const errorBox = wrapper.find('[data-testid="web-search-settings-error"]')
    expect(errorBox.exists()).toBe(true)
    expect(errorBox.text()).toContain('读取后端炸了')

    api.getSettings.mockResolvedValueOnce({
      success: true,
      data: { vendor_configured: true, vendor_name: 'DeepSeek' },
    })
    await wrapper.find('[data-testid="web-search-settings-retry"]').trigger('click')
    await flushPromises()

    expect(api.getSettings).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="web-search-settings-error"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="web-search-vendor"]').text()).toContain('DeepSeek')
  })
})

describe('WebSearchConfigPanel 连通性探测四态', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getSettings.mockResolvedValue({
      success: true,
      data: { vendor_configured: true, vendor_name: 'DeepSeek' },
    })
  })

  it('ok：绿色点 + 「连接正常 · N ms」+ detail 次级说明', async () => {
    const wrapper = await probeWith({
      status: 'ok',
      latency_ms: 460,
      detail: '搜索正常，返回 10 条结果',
    })

    const statusEl = wrapper.find('[data-testid="web-search-health-status"]')
    expect(statusEl.exists()).toBe(true)
    expect(statusEl.find('.web-search-health-dot').classes()).toContain('web-search-health-dot--ok')
    expect(statusEl.text()).toContain('连接正常 · 460ms')
    expect(wrapper.find('[data-testid="web-search-health-detail"]').text()).toBe('搜索正常，返回 10 条结果')
    expect(wrapper.find('[data-testid="web-search-health-hint"]').exists()).toBe(false)
  })

  it('degraded：黄色点 + 「服务可达但搜索无结果」', async () => {
    const wrapper = await probeWith({
      status: 'degraded',
      latency_ms: 900,
      detail: '服务可达但搜索无结果（搜索服务侧可能异常）',
    })

    const statusEl = wrapper.find('[data-testid="web-search-health-status"]')
    expect(statusEl.find('.web-search-health-dot').classes()).toContain('web-search-health-dot--degraded')
    expect(statusEl.text()).toContain('服务可达但搜索无结果')
    expect(wrapper.find('[data-testid="web-search-health-detail"]').text()).toBe('服务可达但搜索无结果（搜索服务侧可能异常）')
  })

  it('unreachable + auth：鉴权文案 + 去向提示指向「AI 模型」', async () => {
    const wrapper = await probeWith({
      status: 'unreachable',
      latency_ms: 1200,
      detail: '后端 detail 原文',
      error_kind: 'auth',
    })

    const statusEl = wrapper.find('[data-testid="web-search-health-status"]')
    expect(statusEl.find('.web-search-health-dot').classes()).toContain('web-search-health-dot--unreachable')
    expect(statusEl.text()).toContain('鉴权失败（检查 DeepSeek 供应商 API Key）')
    expect(wrapper.find('[data-testid="web-search-health-detail"]').text()).toBe('后端 detail 原文')

    const hint = wrapper.find('[data-testid="web-search-health-hint"]')
    expect(hint.exists()).toBe(true)
    expect(hint.text()).toBe('请到「AI 模型」分区检查 DeepSeek 供应商的 API Key')
  })

  it('unreachable + quota：额度文案 + 去向提示指向「AI 模型」', async () => {
    const wrapper = await probeWith({
      status: 'unreachable',
      latency_ms: 1300,
      detail: '后端 detail 原文',
      error_kind: 'quota',
    })

    expect(wrapper.find('[data-testid="web-search-health-status"]').text()).toContain('余额或额度不足')

    const hint = wrapper.find('[data-testid="web-search-health-hint"]')
    expect(hint.exists()).toBe(true)
    expect(hint.text()).toBe('请到「AI 模型」分区检查 DeepSeek 账户余额或套餐额度')
  })

  it('unreachable + timeout：文案为请求超时，不给去向提示', async () => {
    const wrapper = await probeWith({
      status: 'unreachable',
      latency_ms: 8000,
      detail: '后端 detail 原文',
      error_kind: 'timeout',
    })

    expect(wrapper.find('[data-testid="web-search-health-status"]').text()).toContain('请求超时')
    expect(wrapper.find('[data-testid="web-search-health-hint"]').exists()).toBe(false)
  })

  it('unreachable 且 error_kind 缺省：落到通用「搜索服务暂不可用」，不给去向提示', async () => {
    const wrapper = await probeWith({
      status: 'unreachable',
      latency_ms: 500,
      detail: 'connection refused',
    })

    expect(wrapper.find('[data-testid="web-search-health-status"]').text()).toContain('搜索服务暂不可用')
    expect(wrapper.find('[data-testid="web-search-health-detail"]').text()).toBe('connection refused')
    expect(wrapper.find('[data-testid="web-search-health-hint"]').exists()).toBe(false)
  })

  it('disabled：灰色点 + 「未关联供应商，联网参考已关闭」', async () => {
    const wrapper = await probeWith({
      status: 'disabled',
      latency_ms: 0,
      detail: '未关联可用的 DeepSeek 供应商',
    })

    const statusEl = wrapper.find('[data-testid="web-search-health-status"]')
    expect(statusEl.find('.web-search-health-dot').classes()).toContain('web-search-health-dot--disabled')
    expect(statusEl.text()).toContain('未关联供应商')
    expect(wrapper.find('[data-testid="web-search-health-detail"]').text()).toBe('未关联可用的 DeepSeek 供应商')
  })

  it('测试连接 loading 防重复：探测中按钮 disabled，重复点击不重复发请求', async () => {
    let resolveProbe: (v: unknown) => void = () => {}
    api.getHealth.mockReturnValue(new Promise(r => { resolveProbe = r }))
    const wrapper = mountPanel()
    await flushPromises()

    await findProbeButton(wrapper).trigger('click')
    await flushPromises()
    const btn = findProbeButton(wrapper).element as HTMLButtonElement
    expect(btn.disabled).toBe(true)
    expect(btn.textContent).toContain('测试中')
    await findProbeButton(wrapper).trigger('click') // loading 中重复点击
    expect(api.getHealth).toHaveBeenCalledTimes(1)

    resolveProbe({ success: true, data: { status: 'ok', latency_ms: 1, detail: '' } })
    await flushPromises()
    expect((findProbeButton(wrapper).element as HTMLButtonElement).disabled).toBe(false)
  })

  it('探测请求失败：弱提示不阻塞面板', async () => {
    api.getHealth.mockResolvedValue({ success: false, error: '探测后端炸了' })
    const wrapper = mountPanel()
    await flushPromises()

    await findProbeButton(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="web-search-health-error"]').text()).toContain('探测后端炸了')
    expect(wrapper.find('[data-testid="web-search-health-status"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="web-search-vendor"]').exists()).toBe(true)
  })
})

describe('WebSearchConfigPanel 契约守卫（design D6：面板无可编辑字段）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getSettings.mockResolvedValue({
      success: true,
      data: { vendor_configured: true, vendor_name: 'DeepSeek' },
    })
  })

  it('面板不含输入框 / 开关组件 / form：密钥只在「AI 模型」分区维护', async () => {
    api.getHealth.mockResolvedValue({
      success: true,
      data: { status: 'ok', latency_ms: 100, detail: '搜索正常，返回 10 条结果' },
    })
    const wrapper = mountPanel()
    await flushPromises()
    await findProbeButton(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.find('textarea').exists()).toBe(false)
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.findComponent(AppToggle).exists()).toBe(false)
  })
})
