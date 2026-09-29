import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { AIVendor } from '~/types'

const apiMocks = vi.hoisted(() => ({
  testVendorInline: vi.fn(),
}))

vi.mock('~/api', () => ({
  useAIAdminApi: () => apiMocks,
}))

import VendorDialog from './VendorDialog.vue'

// AppDialog 以 stub 内联渲染插槽（Teleport/overlay 结构已由 AppDialog.test.ts 锁定，
// 这里聚焦两段式表单流转与校验）
const AppDialogStub = {
  props: { modelValue: Boolean, title: String, size: String },
  emits: ['update:modelValue'],
  template: `<div v-if="modelValue" class="app-dialog-stub" :data-size="size">
    <h2 class="stub-title">{{ title }}</h2>
    <slot />
    <slot name="footer" />
  </div>`,
}

const stubs = { AppDialog: AppDialogStub, Icon: true }

function mountDialog(props: Record<string, unknown> = {}) {
  return mount(VendorDialog, {
    props: { modelValue: true, ...props },
    global: { stubs },
  })
}

function findPresetCard(wrapper: ReturnType<typeof mountDialog>, label: string) {
  return wrapper.findAll('.preset-card').find(c => c.text().includes(label))!
}

async function selectPreset(wrapper: ReturnType<typeof mountDialog>, label: string) {
  await findPresetCard(wrapper, label).trigger('click')
}

function textInputs(wrapper: ReturnType<typeof mountDialog>) {
  return wrapper.findAll('input.app-input')
}

function saveButton(wrapper: ReturnType<typeof mountDialog>) {
  return wrapper.findAll('button').find(b => b.text().includes('保存'))!
}

function makeVendor(partial: Partial<AIVendor> & { id: number; name: string }): AIVendor {
  return {
    provider_type: 'openai_compatible',
    base_url: 'https://api.deepseek.com/v1',
    timeout_seconds: 120,
    thinking_dialect: 'deepseek',
    structured_output: 'json_object',
    start_command_configured: false,
    api_key_configured: false,
    enabled: true,
    ...partial,
  }
}

describe('VendorDialog 两段式（U1）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('打开默认停在第一段：未选预设时第二段提示先选择，无表单字段', () => {
    const wrapper = mountDialog()
    expect(wrapper.findAll('.preset-card')).toHaveLength(7)
    expect(wrapper.text()).toContain('先在上方选择供应商')
    expect(textInputs(wrapper)).toHaveLength(0)
  })

  it('选云端预设：只露 名称/Base URL/API Key，无类型下拉与启动命令，高级默认收起', async () => {
    const wrapper = mountDialog()
    await selectPreset(wrapper, 'DeepSeek')

    expect(findPresetCard(wrapper, 'DeepSeek').classes()).toContain('preset-card--selected')
    expect(wrapper.text()).toContain('接入 DeepSeek（只需 API Key）')
    // 第二段主表单三个输入（高级折叠内的超时输入不计，按非 advanced 网格过滤）
    const mainGrid = wrapper.findAll('.form-grid').find(g => !g.classes().includes('advanced-grid'))!
    expect(mainGrid.findAll('input.app-input')).toHaveLength(3)
    expect(wrapper.find('input[type="password"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('启动命令')
    expect(wrapper.text()).not.toContain('OpenAI Compatible')
    // 高级折叠默认收起
    const details = wrapper.find('details.advanced')
    expect(details.exists()).toBe(true)
    expect((details.element as HTMLDetailsElement).open).toBe(false)
  })

  it('选无 key 本地预设（Ollama）：显示「本地服务无需密钥」，无 API Key 输入', async () => {
    const wrapper = mountDialog()
    await selectPreset(wrapper, 'Ollama')

    expect(wrapper.text()).toContain('本地服务无需密钥')
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('接入 Ollama')
  })

  it('选 llama.cpp：另露启动命令字段与后端执行/跨主机留空说明', async () => {
    const wrapper = mountDialog()
    await selectPreset(wrapper, 'llama.cpp')

    const cmdInput = textInputs(wrapper).find(i => (i.attributes('placeholder') || '').includes('llama-server'))
    expect(cmdInput).toBeTruthy()
    expect(wrapper.text()).toContain('由后端执行的本地启动命令；跨主机部署请留空')
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
  })

  it('选自定义：露全量表单（类型下拉 + 启动命令），无预设摘要条', async () => {
    const wrapper = mountDialog()
    await selectPreset(wrapper, '自定义')

    expect(wrapper.text()).toContain('自定义接入（OpenAI 兼容）')
    expect(wrapper.find('.preset-summary').exists()).toBe(false)
    const typeSelect = wrapper.findAll('select').find(s =>
      s.findAll('option').some(o => o.element.value === 'openai_compatible'))
    expect(typeSelect).toBeTruthy()
    expect(textInputs(wrapper).some(i => (i.attributes('placeholder') || '').includes('llama-server'))).toBe(true)
  })

  it('高级展开：超时/方言/结构化输出可改，预设档位显示「跟随预设」', async () => {
    const wrapper = mountDialog()
    await selectPreset(wrapper, 'DeepSeek')

    const details = wrapper.find('details.advanced');
    (details.element as HTMLDetailsElement).open = true

    expect(wrapper.text()).toContain('超时（秒）')
    expect(wrapper.text()).toContain('思考方言覆盖')
    expect(wrapper.text()).toContain('结构化输出覆盖')
    const dialectSelect = wrapper.findAll('select').find(s => s.text().includes('跟随预设（deepseek）'))
    expect(dialectSelect).toBeTruthy()
    expect(dialectSelect!.findAll('option').some(o => o.element.value === 'reasoning-effort')).toBe(true)
  })

  it('测试连接三态：待测 → 测试中（锁）→ 成功/失败内联展示', async () => {
    const wrapper = mountDialog()
    await selectPreset(wrapper, 'DeepSeek')

    const testBtn = () => wrapper.findAll('button').find(b => b.text().includes('测试连接') || b.text().includes('测试中…'))!

    // 成功态（deferred 手动 resolve，确定性观察「测试中」中间态）
    let resolveTest!: (v: unknown) => void
    apiMocks.testVendorInline.mockImplementationOnce(() => new Promise(r => { resolveTest = r }))
    await testBtn().trigger('click')
    expect(testBtn().text()).toContain('测试中…')
    resolveTest({ success: true, message: '连接成功，共 2 个可用模型', data: { models: [] } })
    await flushPromises()
    expect(wrapper.find('.conn-result--ok').text()).toContain('共 2 个可用模型')

    // 失败态（业务失败 success=false）
    apiMocks.testVendorInline.mockResolvedValueOnce({ success: false, error: 'connection refused' })
    await wrapper.findAll('button').find(b => b.text().includes('测试连接'))!.trigger('click')
    await flushPromises()
    const fail = wrapper.find('.conn-result--fail')
    expect(fail.exists()).toBe(true)
    expect(fail.text()).toContain('connection refused')

    // 测试中发出请求体（未保存表单内联测：base_url 取表单值）
    apiMocks.testVendorInline.mockResolvedValueOnce({ success: true, message: 'ok', data: { models: [] } })
    await wrapper.findAll('button').find(b => b.text().includes('测试连接'))!.trigger('click')
    await flushPromises()
    expect(apiMocks.testVendorInline).toHaveBeenLastCalledWith(
      expect.objectContaining({ base_url: 'https://api.deepseek.com/v1' }),
    )
  })

  it('字段校验：名称/Base URL 必填，未过校验不发 save', async () => {
    const wrapper = mountDialog()
    await selectPreset(wrapper, 'DeepSeek')

    await textInputs(wrapper)[0]!.setValue('')
    await textInputs(wrapper)[1]!.setValue('not-a-url')
    await saveButton(wrapper).trigger('click')

    expect(wrapper.emitted('save')).toBeFalsy()
    expect(wrapper.text()).toContain('请填写名称')
    expect(wrapper.text()).toContain('请填写合法 URL')
  })

  it('保存载荷：预设档位默认注入（DeepSeek → deepseek/json_object），preset_id 携带', async () => {
    const wrapper = mountDialog()
    await selectPreset(wrapper, 'DeepSeek')
    await wrapper.find('input[type="password"]').setValue('sk-abc')

    await saveButton(wrapper).trigger('click')

    expect(wrapper.emitted('save')).toHaveLength(1)
    expect(wrapper.emitted('save')![0]![0]).toMatchObject({
      name: 'DeepSeek',
      preset_id: 'deepseek',
      base_url: 'https://api.deepseek.com/v1',
      thinking_dialect: 'deepseek',
      structured_output: 'json_object',
      api_key: 'sk-abc',
    })
  })

  it('编辑模式：预填名称/URL/档位覆盖值，key 只显示已配置占位不回显', () => {
    const wrapper = mountDialog({
      vendor: makeVendor({
        id: 5,
        name: 'my-deepseek',
        preset_id: 'deepseek',
        thinking_dialect: 'none', // 覆盖过预设档 → 预填具体值
        structured_output: 'json_object', // 与预设一致 → 跟随预设
        api_key_configured: true,
      }),
    })

    expect(wrapper.find('.stub-title').text()).toBe('编辑供应商：my-deepseek')
    const inputs = textInputs(wrapper)
    expect((inputs[0]!.element as HTMLInputElement).value).toBe('my-deepseek')
    expect((inputs[1]!.element as HTMLInputElement).value).toBe('https://api.deepseek.com/v1')
    expect((inputs[2]!.element as HTMLInputElement).value).toBe('')
    expect(inputs[2]!.attributes('placeholder')).toContain('已配置')
    // key 已配置 → 清除开关可见
    expect(wrapper.text()).toContain('清除已保存密钥')
    // 档位：方言覆盖预填 none；结构化输出与预设一致显示跟随预设
    const dialectSelect = wrapper.findAll('select').find(s => s.findAll('option').some(o => o.element.value === 'none'))
    expect((dialectSelect!.element as HTMLSelectElement).value).toBe('none')
  })

  it('编辑模式保存：未输入新 key 不发 api_key；勾选清除才带 clear_api_key', async () => {
    const wrapper = mountDialog({
      vendor: makeVendor({ id: 5, name: 'my-deepseek', preset_id: 'deepseek', api_key_configured: true }),
    })

    await saveButton(wrapper).trigger('click')
    let payload = wrapper.emitted('save')![0]![0] as Record<string, unknown>
    expect(payload.api_key).toBeUndefined()
    expect(payload.clear_api_key).toBeUndefined()

    await wrapper.find('.clear-check').setValue(true)
    await saveButton(wrapper).trigger('click')
    payload = wrapper.emitted('save')![1]![0] as Record<string, unknown>
    expect(payload.clear_api_key).toBe(true)
  })

  it('编辑模式：档位与预设不同 → 保存发覆盖值（spec「档位可改」）', async () => {
    const wrapper = mountDialog({
      vendor: makeVendor({ id: 5, name: 'my-deepseek', preset_id: 'deepseek', thinking_dialect: 'none' }),
    })
    const details = wrapper.find('details.advanced');
    (details.element as HTMLDetailsElement).open = true

    await saveButton(wrapper).trigger('click')
    const payload = wrapper.emitted('save')![0]![0] as Record<string, unknown>
    expect(payload.thinking_dialect).toBe('none')
    expect(payload.structured_output).toBe('json_object')
  })

  it('取消/关闭：emit update:modelValue(false)', async () => {
    const wrapper = mountDialog()
    await wrapper.findAll('button').find(b => b.text() === '取消')!.trigger('click')
    expect(wrapper.emitted('update:modelValue')![0]).toEqual([false])
  })
})
