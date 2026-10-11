import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { computed, ref } from 'vue'
import type { AIModel, AIVendor } from '~/types'

const composableMocks = vi.hoisted(() => ({
  useAIVendors: vi.fn(),
  useAIModels: vi.fn(),
  useAIRoutes: vi.fn(),
}))

// 父层与卡片/弹窗共享同一组 composable 模块 mock（leaf 实例动作也落在这里）
vi.mock('../composables/useAIVendors', () => ({ useAIVendors: composableMocks.useAIVendors }))
vi.mock('../composables/useAIModels', () => ({ useAIModels: composableMocks.useAIModels }))
vi.mock('../composables/useAIRoutes', () => ({ useAIRoutes: composableMocks.useAIRoutes }))

import AIProviderManagement from './AIProviderManagement.vue'
import VendorCard from './VendorCard.vue'
import VendorDialog from './VendorDialog.vue'
import { useNotify } from '~/composables/useNotify'

const stubs = { Icon: true }

function makeVendor(partial: Partial<AIVendor> & { id: number; name: string }): AIVendor {
  return {
    provider_type: 'openai_compatible',
    base_url: 'http://10.11.12.111:8080/v1',
    timeout_seconds: 300,
    thinking_dialect: 'qwen-chat-template',
    structured_output: 'json_schema',
    start_command_configured: false,
    api_key_configured: false,
    enabled: true,
    models_count: 1,
    capabilities_count: 0,
    ...partial,
  }
}

function makeModel(partial: Partial<AIModel> & { id: number; vendor_id: number; name: string }): AIModel {
  return {
    model_kind: 'llm',
    thinking_level: null,
    supports_xhigh_max: false,
    enabled: false,
    status: 'ok',
    last_seen_at: null,
    ...partial,
  }
}

let ctx: ReturnType<typeof buildMocks>

function buildMocks(overrides: {
  vendors?: AIVendor[]
  models?: AIModel[]
  vendorsError?: string | null
  loading?: boolean
} = {}) {
  const vendorsCtx = {
    vendors: ref<AIVendor[]>(overrides.vendors ?? []),
    loading: ref(overrides.loading ?? false),
    error: ref<string | null>(overrides.vendorsError ?? null),
    fetchVendors: vi.fn().mockResolvedValue([]),
    createVendor: vi.fn(),
    updateVendor: vi.fn(),
    removeVendor: vi.fn(),
    refreshModels: vi.fn(),
    testConnection: vi.fn(),
    testInline: vi.fn(),
  }
  const modelsCtx = {
    models: ref<AIModel[]>(overrides.models ?? []),
    loading: ref(false),
    error: ref<string | null>(null),
    fetchModels: vi.fn().mockResolvedValue([]),
    updateModel: vi.fn().mockResolvedValue(true),
    usableModels: computed(() => [] as AIModel[]),
    usableModelsForCapability: vi.fn(() => [] as AIModel[]),
    byVendor: computed(() => []),
  }
  const routesCtx = {
    routes: ref([]),
    loading: ref(false),
    saving: ref(false),
    error: ref<string | null>(null),
    fetchRoutes: vi.fn().mockResolvedValue([]),
    routeByCapability: computed(() => ({})),
    saveRoute: vi.fn(),
  }
  composableMocks.useAIVendors.mockReturnValue(vendorsCtx)
  composableMocks.useAIModels.mockReturnValue(modelsCtx)
  composableMocks.useAIRoutes.mockReturnValue(routesCtx)
  return { vendorsCtx, modelsCtx, routesCtx }
}

/** 装配一组可控 composable 实例并写入模块级 ctx（断言须用 ctx 内的同一批 spy/ref） */
function installMocks(overrides: Parameters<typeof buildMocks>[0] = {}): void {
  ctx = buildMocks(overrides)
}

function mountSection() {
  return mount(AIProviderManagement, { global: { stubs } })
}

function lastToast() {
  const { toasts } = useNotify()
  return toasts.value.at(-1)
}

describe('AIProviderManagement section 容器', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    installMocks()
  })

  it('loading：清单加载中显示骨架转圈，不出卡片', () => {
    installMocks({ loading: true })
    const wrapper = mountSection()
    expect(wrapper.find('.animate-spin').exists()).toBe(true)
    expect(wrapper.findAllComponents(VendorCard)).toHaveLength(0)
  })

  it('empty：无供应商时显示「还没有供应商」+ 新增引导', () => {
    const wrapper = mountSection()
    expect(wrapper.text()).toContain('还没有供应商')
    expect(wrapper.text()).toContain('新增供应商')
    expect(wrapper.findAllComponents(VendorCard)).toHaveLength(0)
  })

  it('error：列表加载失败内联条 + 重试重拉 vendors 与 models', async () => {
    installMocks({ vendorsError: 'boom' })
    const wrapper = mountSection()

    expect(wrapper.find('.error-bar').text()).toContain('boom')
    await wrapper.find('.retry-btn').trigger('click')
    expect(ctx.vendorsCtx.fetchVendors).toHaveBeenCalledTimes(2) // mounted 1 次 + 重试
    expect(ctx.modelsCtx.fetchModels).toHaveBeenCalledTimes(2)
  })

  it('渲染每 vendor 一张卡片，models 按 vendor_id 过滤下发', () => {
    installMocks({
      vendors: [makeVendor({ id: 1, name: 'a' }), makeVendor({ id: 2, name: 'b' })],
      models: [
        makeModel({ id: 11, vendor_id: 1, name: 'm1' }),
        makeModel({ id: 12, vendor_id: 2, name: 'm2' }),
        makeModel({ id: 13, vendor_id: 2, name: 'm3' }),
      ],
    })
    const wrapper = mountSection()

    const cards = wrapper.findAllComponents(VendorCard)
    expect(cards).toHaveLength(2)
    expect(cards[0]!.props('models')).toHaveLength(1)
    expect(cards[1]!.props('models')).toHaveLength(2)
  })

  it('刷新成功：diff toast + models 重拉 + diff 下发卡片', async () => {
    installMocks({ vendors: [makeVendor({ id: 1, name: 'a' })], models: [makeModel({ id: 11, vendor_id: 1, name: 'old' })] })
    ctx.vendorsCtx.refreshModels.mockResolvedValue({ added: ['x'], stale: ['y'], visible_count: 2 })
    const wrapper = mountSection()

    wrapper.findAllComponents(VendorCard)[0]!.vm.$emit('refresh')
    await flushPromises()

    expect(ctx.vendorsCtx.refreshModels).toHaveBeenCalledWith(1)
    expect(ctx.modelsCtx.fetchModels).toHaveBeenCalled()
    expect(lastToast()?.type).toBe('success')
    expect(lastToast()?.message).toContain('新增 1 个 · stale 1 个')
    expect(wrapper.findAllComponents(VendorCard)[0]!.props('refreshDiff')).toEqual({ added: ['x'], stale: ['y'], visible_count: 2 })
  })

  it('刷新失败（U3）：error toast、models 不重拉（旧清单保留）、不下发 diff', async () => {
    installMocks({
      vendors: [makeVendor({ id: 1, name: 'a' })],
      models: [makeModel({ id: 11, vendor_id: 1, name: 'old' })],
    })
    ctx.vendorsCtx.refreshModels.mockResolvedValue(null)
    ctx.vendorsCtx.error.value = 'upstream timeout'
    const wrapper = mountSection()

    wrapper.findAllComponents(VendorCard)[0]!.vm.$emit('refresh')
    await flushPromises()

    expect(lastToast()?.type).toBe('error')
    expect(lastToast()?.message).toContain('upstream timeout')
    // 除 mounted 预拉外不额外重拉 models（旧清单缓存保留）
    expect(ctx.modelsCtx.fetchModels).toHaveBeenCalledTimes(1)
    expect(wrapper.findAllComponents(VendorCard)[0]!.props('refreshDiff')).toBeNull()
    // 旧清单缓存原样
    expect(ctx.modelsCtx.models.value.map(m => m.name)).toEqual(['old'])
  })

  it('清单行配置透传 updateModel；失败 toast', async () => {
    installMocks({
      vendors: [makeVendor({ id: 1, name: 'a' })],
      models: [makeModel({ id: 11, vendor_id: 1, name: 'm1' })],
    })
    const wrapper = mountSection()
    const model = makeModel({ id: 11, vendor_id: 1, name: 'm1' })

    wrapper.findAllComponents(VendorCard)[0]!.vm.$emit('update-model', model, { enabled: true })
    await flushPromises()
    expect(ctx.modelsCtx.updateModel).toHaveBeenCalledWith(11, { enabled: true })

    ctx.modelsCtx.updateModel.mockResolvedValue(false)
    ctx.modelsCtx.error.value = '更新模型失败'
    wrapper.findAllComponents(VendorCard)[0]!.vm.$emit('update-model', model, { enabled: true })
    await flushPromises()
    expect(lastToast()?.type).toBe('error')
  })

  it('删除成功：级联计数 toast + 路由链与模型缓存重载', async () => {
    installMocks({ vendors: [makeVendor({ id: 1, name: 'a' })], models: [makeModel({ id: 11, vendor_id: 1, name: 'm1' })] })
    ctx.vendorsCtx.removeVendor.mockResolvedValue({ deleted_models: 3, detached_bindings: 5 })
    const wrapper = mountSection()

    wrapper.findAllComponents(VendorCard)[0]!.vm.$emit('delete')
    await flushPromises()

    expect(ctx.vendorsCtx.removeVendor).toHaveBeenCalledWith(1)
    expect(lastToast()?.type).toBe('success')
    expect(lastToast()?.message).toContain('3 个模型')
    expect(lastToast()?.message).toContain('5 条绑定')
    expect(ctx.modelsCtx.fetchModels).toHaveBeenCalled()
    expect(ctx.routesCtx.fetchRoutes).toHaveBeenCalled()
  })

  it('保存（新增）：createVendor 成功后关弹窗、拉清单、自动展开、diff toast', async () => {
    installMocks({ vendors: [] })
    ctx.vendorsCtx.createVendor.mockResolvedValue({ id: 9, models_refreshed: true, added: 4 })
    const wrapper = mountSection()

    const dialog = wrapper.findComponent(VendorDialog)
    expect(dialog.props('vendor')).toBeNull()
    dialog.vm.$emit('save', { name: 'DeepSeek', base_url: 'https://api.deepseek.com/v1' })
    await flushPromises()

    expect(ctx.vendorsCtx.createVendor).toHaveBeenCalledTimes(1)
    expect(dialog.props('modelValue')).toBe(false)
    expect(lastToast()?.message).toBe('已保存，拉取到 4 个模型')
    expect(ctx.modelsCtx.fetchModels).toHaveBeenCalled()
  })

  it('保存（新增）：清单拉取失败不回滚 → warn toast 带原因', async () => {
    installMocks({ vendors: [] })
    ctx.vendorsCtx.createVendor.mockResolvedValue({ id: 9, models_refreshed: false, refresh_error: 'dns failure' })
    const wrapper = mountSection()

    wrapper.findComponent(VendorDialog).vm.$emit('save', { name: 'X', base_url: 'http://localhost:8080/v1' })
    await flushPromises()

    expect(lastToast()?.type).toBe('warn')
    expect(lastToast()?.message).toContain('清单拉取失败：dns failure')
  })

  it('保存（新增）失败：error toast 且弹窗不关', async () => {
    installMocks({ vendors: [] })
    ctx.vendorsCtx.createVendor.mockResolvedValue(null)
    ctx.vendorsCtx.error.value = 'vendor name conflict'
    const wrapper = mountSection()

    // 先打开弹窗再触发保存失败
    await wrapper.findAll('button').filter(b => b.text().includes('新增供应商'))[0]!.trigger('click')
    const dialog = wrapper.findComponent(VendorDialog)
    expect(dialog.props('modelValue')).toBe(true)

    dialog.vm.$emit('save', { name: 'X', base_url: 'http://localhost:8080/v1' })
    await flushPromises()

    expect(lastToast()?.type).toBe('error')
    expect(lastToast()?.message).toContain('vendor name conflict')
    expect(dialog.props('modelValue')).toBe(true)
  })

  it('保存（编辑）：卡片 edit 事件预填弹窗，保存走 updateVendor', async () => {
    installMocks({ vendors: [makeVendor({ id: 1, name: 'a' })] })
    ctx.vendorsCtx.updateVendor.mockResolvedValue(true)
    const wrapper = mountSection()

    wrapper.findAllComponents(VendorCard)[0]!.vm.$emit('edit')
    await flushPromises()
    const dialog = wrapper.findComponent(VendorDialog)
    expect(dialog.props('vendor') as AIVendor).toEqual(expect.objectContaining({ id: 1 }))

    dialog.vm.$emit('save', { name: 'a2', base_url: 'http://10.11.12.111:8080/v1' })
    await flushPromises()

    expect(ctx.vendorsCtx.updateVendor).toHaveBeenCalledWith(1, { name: 'a2', base_url: 'http://10.11.12.111:8080/v1' })
    expect(lastToast()?.message).toBe('已保存')
  })

  it('新增入口唯一在头部（FB-6）；空态只引导不重复按钮；点击打开弹窗（modelValue=true、vendor=null）', async () => {
    installMocks({ vendors: [] })
    const wrapper = mountSection()

    const addBtns = wrapper.findAll('button').filter(b => b.text().includes('新增供应商'))
    // FB-6：header 唯一入口；空态文案引导，不再放第二个按钮
    expect(addBtns).toHaveLength(1)
    expect(wrapper.find('.empty-hint').text()).toContain('右上角')
    await addBtns[0]!.trigger('click')
    expect(wrapper.findComponent(VendorDialog).props('modelValue')).toBe(true)
  })
})
