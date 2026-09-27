import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive } from 'vue'
import AIRouterBackupProviders from './AIRouterBackupProviders.vue'

// 组件经 inject('ai-router-ctx') 取上下文（宿主 AIProviderManagement 提供），
// 测试直接 provide 一个可控 ctx，聚焦表单字段渲染与交互。
function makeCtx(overrides: Record<string, unknown> = {}) {
  return reactive({
    saving: false,
    backupProviders: [] as Record<string, unknown>[],
    showNewProviderForm: false,
    newProviderForm: {
      name: '', model: '', provider_type: 'openai_compatible', base_url: '', api_key: '',
      model_kind: 'llm', start_command: '', enable_thinking: false,
    },
    editingProviderId: null as number | null,
    editProviderForm: {
      name: '', model: '', provider_type: 'openai_compatible', base_url: '', api_key: '',
      model_kind: 'llm', start_command: '', clear_start_command: false,
      enabled: true, timeout_seconds: 120, enable_thinking: false, clear_api_key: false,
    },
    showEditProviderApiKey: false,
    showNewProviderApiKey: false,
    testingProviderId: null as number | null,
    saveNewProvider: () => {},
    startEditingProvider: () => {},
    cancelEditingProvider: () => {},
    saveEditedProvider: () => {},
    deleteBackupProvider: () => {},
    isProviderLinked: () => false,
    testBackupProvider: () => {},
    ...overrides,
  })
}

function makeProvider(partial: Record<string, unknown> = {}) {
  return {
    id: 2,
    name: 'backup-emb',
    model: 'bge-m3',
    base_url: 'http://localhost:8082/v1',
    provider_type: 'openai_compatible',
    model_kind: 'embedding',
    start_command_configured: true,
    api_key_configured: false,
    enabled: true,
    ...partial,
  }
}

const stubs = {
  Icon: true,
  AppSectionHeader: true,
  AppToggle: true,
  AppButton: { template: '<button class="app-btn"><slot /></button>' },
  AppInput: {
    props: ['modelValue', 'type', 'placeholder'],
    template: '<input class="app-input" :type="type || \'text\'" :placeholder="placeholder" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
  },
}

function mountWithCtx(ctx: ReturnType<typeof makeCtx>) {
  return mount(AIRouterBackupProviders, {
    global: {
      provide: { 'ai-router-ctx': ctx },
      stubs,
    },
  })
}

describe('AIRouterBackupProviders（model_kind/start_command 表单）', () => {
  it('新建表单渲染模型类型选择（默认 llm）与启动命令输入，选择后写回表单', async () => {
    const ctx = makeCtx({ showNewProviderForm: true })
    const wrapper = mountWithCtx(ctx)

    const selects = wrapper.findAll('select.ai-select')
    expect(selects.length).toBe(2)
    const kindSelect = selects[1]!
    expect((kindSelect.element as HTMLSelectElement).value).toBe('llm')
    expect(kindSelect.find('option[value="embedding"]').exists()).toBe(true)

    const startCommandInput = wrapper.findAll('input.app-input')
      .find(el => (el.attributes('placeholder') || '').includes('llama-server'))
    expect(startCommandInput).toBeTruthy()

    await kindSelect.setValue('embedding')
    expect(ctx.newProviderForm.model_kind).toBe('embedding')
  })

  it('provider 列表显示 model_kind 徽标', () => {
    const ctx = makeCtx({ backupProviders: [makeProvider()] })
    const wrapper = mountWithCtx(ctx)
    expect(wrapper.text()).toContain('Embedding')
  })

  it('编辑表单：已配置启动命令时可标记清除', async () => {
    const ctx = makeCtx({
      backupProviders: [makeProvider()],
      editingProviderId: 2,
    })
    const wrapper = mountWithCtx(ctx)

    const clearBtn = wrapper.findAll('button').find(b => b.text().includes('清除已配置的启动命令'))
    expect(clearBtn).toBeTruthy()
    await clearBtn!.trigger('click')
    expect(ctx.editProviderForm.clear_start_command).toBe(true)
    expect(wrapper.text()).toContain('保存后将清除已配置的启动命令')
  })

  it('编辑表单：未配置启动命令时不显示清除入口', () => {
    const ctx = makeCtx({
      backupProviders: [makeProvider({ start_command_configured: false })],
      editingProviderId: 2,
    })
    const wrapper = mountWithCtx(ctx)
    expect(wrapper.text()).not.toContain('清除已配置的启动命令')
  })

  it('provider 卡片渲染测试连接按钮，点击调用 testBackupProvider', async () => {
    const testBackupProvider = vi.fn()
    const ctx = makeCtx({
      backupProviders: [makeProvider()],
      testBackupProvider,
    })
    const wrapper = mountWithCtx(ctx)

    const testBtn = wrapper.findAll('button').find(b => (b.attributes('title') || '').includes('测试连接'))
    expect(testBtn).toBeTruthy()
    await testBtn!.trigger('click')
    expect(testBackupProvider).toHaveBeenCalledWith(ctx.backupProviders[0])
  })

  it('该 provider 测试中时按钮禁用且显示 loading 图标', () => {
    const ctx = makeCtx({
      backupProviders: [makeProvider()],
      testingProviderId: 2,
    })
    const wrapper = mountWithCtx(ctx)

    const testBtn = wrapper.findAll('button').find(b => (b.attributes('title') || '').includes('测试连接'))
    expect(testBtn).toBeTruthy()
    expect((testBtn!.element as HTMLButtonElement).disabled).toBe(true)
    expect(testBtn!.find('.animate-spin').exists()).toBe(true)
  })
})

describe('AIRouterBackupProviders 删除入口（挂线路不阻断，fix-provider-delete-route-deadlock 2b）', () => {
  // spec「删除入口不被引用状态阻断」：后端已级联解绑，前端不得再用 disabled
  // 阻断挂线路 provider 的删除按钮；行内提示必须是告知性文案（解绑后果），
  // 而非「先移除再删」的阻断式引导。
  it('provider 挂在线路上：删除按钮可点（无 disabled），行内提示为解绑告知', () => {
    const ctx = makeCtx({
      backupProviders: [makeProvider()],
      isProviderLinked: () => true,
    })
    const wrapper = mountWithCtx(ctx)

    const deleteBtn = wrapper.find('button.ai-icon-btn--danger')
    expect(deleteBtn.exists()).toBe(true)
    expect(deleteBtn.attributes('disabled')).toBeUndefined()

    const warn = wrapper.find('.ai-warn-text')
    expect(warn.exists()).toBe(true)
    expect(warn.text()).toContain('解绑')
    expect(warn.text()).not.toContain('先移除再删')
  })

  it('provider 未挂线路：删除按钮可点，无行内提示', () => {
    const ctx = makeCtx({
      backupProviders: [makeProvider()],
      isProviderLinked: () => false,
    })
    const wrapper = mountWithCtx(ctx)

    const deleteBtn = wrapper.find('button.ai-icon-btn--danger')
    expect(deleteBtn.attributes('disabled')).toBeUndefined()
    expect(wrapper.find('.ai-warn-text').exists()).toBe(false)
  })
})
