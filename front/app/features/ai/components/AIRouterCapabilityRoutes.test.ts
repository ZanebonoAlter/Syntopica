import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { computed, ref } from 'vue'
import type { AIModel, AIRoute, AIRouteModelBinding, RouteSaveRequest, ThinkingLevel } from '~/types'

const composableMocks = vi.hoisted(() => ({
  useAIModels: vi.fn(),
  useAIRoutes: vi.fn(),
}))

// 常量（capabilityOrder/routeLabels）保留真实导出，仅替换 composable 函数
vi.mock('../composables/useAIModels', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../composables/useAIModels')>()
  return { ...actual, useAIModels: composableMocks.useAIModels }
})
vi.mock('../composables/useAIRoutes', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../composables/useAIRoutes')>()
  return { ...actual, useAIRoutes: composableMocks.useAIRoutes }
})

// focus_capability 深链（观测面板「定位」跳入）；route 需 reactive 以支撑挂载期间 watch 路径
const routerMocks = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
  route: null as { query: Record<string, unknown> } | null,
}))
vi.mock('vue-router', async () => {
  const { reactive } = await import('vue')
  const route = reactive({ query: {} as Record<string, unknown> })
  routerMocks.route = route
  return {
    useRoute: () => route,
    useRouter: () => ({ push: routerMocks.push, replace: routerMocks.replace }),
  }
})

import AIRouterCapabilityRoutes from './AIRouterCapabilityRoutes.vue'
import { useNotify } from '~/composables/useNotify'

const stubs = { Icon: true }

/** 挂载过的实例统一在 afterEach unmount：否则旧实例的 watch 仍存活，会响应后续用例的 query 变化 */
const mountedWrappers: Array<{ unmount: () => void }> = []

type VendorSummary = NonNullable<AIModel['vendor']>
type ModelWithVendor = AIModel & { vendor: VendorSummary }

const VENDOR_L: VendorSummary = { id: 1, name: 'llamacpp-qwen', provider_type: 'openai_compatible', base_url: 'http://10.11.12.111:8080/v1' }
const VENDOR_D: VendorSummary = { id: 2, name: 'DeepSeek', provider_type: 'openai_compatible', base_url: 'https://api.deepseek.com' }

function makeModel(partial: Partial<AIModel> & { id: number, name: string, vendor: VendorSummary }): ModelWithVendor {
  return {
    vendor_id: partial.vendor.id,
    model_kind: 'llm',
    thinking_level: null,
    supports_xhigh_max: false,
    enabled: true,
    status: 'ok',
    last_seen_at: null,
    usable: true,
    ...partial,
  }
}

const MODELS: ModelWithVendor[] = [
  makeModel({ id: 1, name: 'qwythos', vendor: VENDOR_L, thinking_level: 'medium' }),
  makeModel({ id: 2, name: 'deepseek-flash', vendor: VENDOR_D, thinking_level: 'high', supports_xhigh_max: true }),
  makeModel({ id: 3, name: 'qwen3-embedding', vendor: VENDOR_L, model_kind: 'embedding' }),
  makeModel({ id: 4, name: 'm-disabled', vendor: VENDOR_D, enabled: false, usable: false }),
]

function makeBinding(model: ModelWithVendor, priority: number, thinking_level: ThinkingLevel | null): AIRouteModelBinding {
  return {
    id: priority + 1,
    route_id: 10,
    model_id: model.id,
    priority,
    thinking_level,
    enabled: true,
    model: {
      id: model.id,
      name: model.name,
      model_kind: model.model_kind,
      thinking_level: model.thinking_level,
      supports_xhigh_max: model.supports_xhigh_max,
      enabled: model.enabled,
      status: model.status,
    },
    vendor: { id: model.vendor.id, name: model.vendor.name },
  }
}

function makeRoute(capability: string, bindings: AIRouteModelBinding[]): AIRoute {
  return { id: 10 + bindings.length, name: 'default', capability, enabled: true, description: '', max_concurrency: 0, route_models: bindings }
}

function buildRoutesFixture(): AIRoute[] {
  return [
    makeRoute('summary', [makeBinding(MODELS[0]!, 0, null), makeBinding(MODELS[1]!, 1, 'low')]),
    makeRoute('topic_tagging', [makeBinding(MODELS[0]!, 0, null)]),
    makeRoute('embedding', [makeBinding(MODELS[2]!, 0, null)]),
  ]
}

function buildCtx(overrides: { routes?: AIRoute[], models?: ModelWithVendor[] } = {}) {
  const routes = overrides.routes ?? buildRoutesFixture()
  const models = overrides.models ?? MODELS
  const modelsCtx = {
    models: ref<AIModel[]>(models),
    loading: ref(false),
    error: ref<string | null>(null),
    fetchModels: vi.fn(async () => models),
    usableModels: computed(() => models.filter(m => m.usable === true)),
    usableModelsForCapability: vi.fn((capability: string) =>
      models.filter(m => m.usable === true && (capability === 'embedding' ? m.model_kind === 'embedding' : m.model_kind === 'llm')),
    ),
    byVendor: computed(() => []),
    updateModel: vi.fn(),
  }
  const routesCtx = {
    routes: ref<AIRoute[]>(routes),
    loading: ref(false),
    saving: ref(false),
    error: ref<string | null>(null),
    fetchRoutes: vi.fn(async () => routes),
    routeByCapability: computed(() => Object.fromEntries(routes.map(r => [r.capability, r]))),
    saveRoute: vi.fn(async (_capability: string, _data: RouteSaveRequest) => true),
  }
  return { modelsCtx, routesCtx }
}

type TestCtx = ReturnType<typeof buildCtx>

let ctx: TestCtx

function installMocks(overrides: { routes?: AIRoute[], models?: ModelWithVendor[] } = {}): TestCtx {
  ctx = buildCtx(overrides)
  composableMocks.useAIModels.mockReturnValue(ctx.modelsCtx)
  composableMocks.useAIRoutes.mockReturnValue(ctx.routesCtx)
  return ctx
}

async function flushAsync() {
  await vi.advanceTimersByTimeAsync(0)
  await vi.advanceTimersByTimeAsync(0)
  await vi.advanceTimersByTimeAsync(0)
}

/** 挂载并等 onMounted 取数/hydrate 完成（后续交互只动本地编辑态，不再被覆盖） */
async function mountRoutes() {
  const wrapper = mount(AIRouterCapabilityRoutes, { global: { stubs } })
  mountedWrappers.push(wrapper)
  await flushAsync()
  return wrapper
}

function section(wrapper: Awaited<ReturnType<typeof mountRoutes>>, capability: string) {
  return wrapper.find(`[data-route-section="${capability}"]`)
}

function chainItems(capabilityRoot: ReturnType<typeof section>) {
  return capabilityRoot.findAll('.route-provider')
}

function saveButton(wrapper: Awaited<ReturnType<typeof mountRoutes>>) {
  return wrapper.findAll('button').find(b => b.text().includes('保存路由'))!
}

function lastToast() {
  const { toasts } = useNotify()
  return toasts.value.at(-1)
}

function setRouteQuery(next: Record<string, unknown>) {
  routerMocks.route!.query = { ...next }
}

function lastReplaceQuery(): Record<string, unknown> {
  const call = routerMocks.replace.mock.calls.at(-1)![0] as { query: Record<string, unknown> }
  return call.query
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  setRouteQuery({})
  installMocks()
})

afterEach(() => {
  vi.useRealTimers()
  for (const w of mountedWrappers) w.unmount()
  mountedWrappers.length = 0
})

describe('AIRouterCapabilityRoutes 链式卡片（U7）', () => {
  it('链项显示「模型名@供应商名」+ 计数；rank 1 高亮；本地编辑态初始 = route_models[].thinking_level', async () => {
    const wrapper = await mountRoutes()
    const summary = section(wrapper, 'summary')

    const items = chainItems(summary)
    expect(items).toHaveLength(2)
    expect(items[0]!.text()).toContain('qwythos@llamacpp-qwen')
    expect(items[1]!.text()).toContain('deepseek-flash@DeepSeek')
    expect(items[0]!.find('.route-provider__rank--first').exists()).toBe(true)
    expect(summary.text()).toContain('2 模型')
    // 初始继承/覆盖态来自 route_models
    expect((items[0]!.find('select').element as HTMLSelectElement).value).toBe('')
    expect((items[1]!.find('select').element as HTMLSelectElement).value).toBe('low')
    expect(items[1]!.find('.route-provider__override').exists()).toBe(true)
    expect(items[0]!.find('.route-provider__override').exists()).toBe(false)
  })

  it('hover 模型名扇出提示：其他能力数 N>0 显「另承载 N 个能力」，N=0 显「未承载其他能力」', async () => {
    const wrapper = await mountRoutes()
    const items = chainItems(section(wrapper, 'summary'))

    // qwythos 同挂 summary + topic_tagging → 另承载 1
    expect(items[0]!.find('.fanout-tip').text()).toBe('qwythos 另承载 1 个能力')
    // deepseek-flash 仅挂 summary → 未承载其他能力
    expect(items[1]!.find('.fanout-tip').text()).toBe('未承载其他能力')
  })

  it('调档即本地态变化：选中非继承档出现「覆盖」徽章，选回继承徽章消失', async () => {
    const wrapper = await mountRoutes()
    const firstSelect = chainItems(section(wrapper, 'summary'))[0]!.find('select')

    await firstSelect.setValue('low')
    let items = chainItems(section(wrapper, 'summary'))
    expect(items[0]!.find('.route-provider__override').exists()).toBe(true)
    // 未声明 xhigh/max 的模型（qwythos）下拉不含 xhigh/max 候选（spec 钳制）
    const values = items[0]!.findAll('option').map(o => o.element.value)
    expect(values).not.toContain('xhigh')
    expect(values).not.toContain('max')

    await items[0]!.find('select').setValue('')
    items = chainItems(section(wrapper, 'summary'))
    expect(items[0]!.find('.route-provider__override').exists()).toBe(false)
  })

  it('chips 点击追加链尾（✓ 标已在链中）；↑↓ 换位即时生效；× 移除', async () => {
    const wrapper = await mountRoutes()

    // topic_tagging 链当前只有 qwythos；点 deepseek-flash chip → 追加链尾
    const tagging = section(wrapper, 'topic_tagging')
    const chip = tagging.findAll('.route-chip').find(c => c.text().includes('deepseek-flash'))!
    expect(chip.text()).toContain('+')
    await chip.trigger('click')

    let items = chainItems(section(wrapper, 'topic_tagging'))
    expect(items).toHaveLength(2)
    expect(items[1]!.text()).toContain('deepseek-flash@DeepSeek')
    expect(section(wrapper, 'topic_tagging').findAll('.route-chip').find(c => c.text().includes('deepseek-flash'))!.text()).toContain('✓')

    // ↑ 换位：第二项上移到 rank 1
    await items[1]!.findAll('.route-icon-btn')[0]!.trigger('click')
    items = chainItems(section(wrapper, 'topic_tagging'))
    expect(items[0]!.text()).toContain('deepseek-flash@DeepSeek')
    expect(items[0]!.find('.route-provider__rank--first').exists()).toBe(true)

    // × 移除
    await items[0]!.find('.route-icon-btn--danger').trigger('click')
    items = chainItems(section(wrapper, 'topic_tagging'))
    expect(items).toHaveLength(1)
    expect(items[0]!.text()).toContain('qwythos@llamacpp-qwen')
  })

  it('chips 按 vendor 分组渲染；embedding 行只列 embedding 模型；非 usable 模型不出现', async () => {
    const wrapper = await mountRoutes()
    const summary = section(wrapper, 'summary')

    const groups = summary.findAll('.route-chip-group').map(g => g.text())
    expect(groups).toEqual(['llamacpp-qwen：', 'DeepSeek：'])
    const chipNames = summary.findAll('.route-chip').map(c => c.text())
    expect(chipNames.some(t => t.includes('qwythos'))).toBe(true)
    expect(chipNames.some(t => t.includes('deepseek-flash'))).toBe(true)
    // 停用模型不是绑定候选
    expect(chipNames.some(t => t.includes('m-disabled'))).toBe(false)
    // llm 行不列 embedding 模型
    expect(chipNames.some(t => t.includes('qwen3-embedding'))).toBe(false)

    const embeddingChips = section(wrapper, 'embedding').findAll('.route-chip').map(c => c.text())
    expect(embeddingChips.some(t => t.includes('qwen3-embedding'))).toBe(true)
    expect(embeddingChips.some(t => t.includes('qwythos'))).toBe(false)
  })

  it('embedding 链项不出现思考档下拉（绑定不携带覆盖值），显示 —', async () => {
    const wrapper = await mountRoutes()
    const item = chainItems(section(wrapper, 'embedding'))[0]!
    expect(item.find('select').exists()).toBe(false)
    expect(item.find('.route-provider__dash').exists()).toBe(true)
  })

  it('保存：逐能力 PUT bindings=[{model_id, thinking_level}]（null=继承），成功 toast', async () => {
    const wrapper = await mountRoutes()

    // topic_tagging 追加 deepseek-flash（继承档）后保存
    await section(wrapper, 'topic_tagging').findAll('.route-chip').find(c => c.text().includes('deepseek-flash'))!.trigger('click')
    await saveButton(wrapper).trigger('click')
    await flushAsync()

    // 逐能力全量 PUT（含 fixture 中无路由的 5 条空链 → bindings:[]）
    expect(ctx.routesCtx.saveRoute).toHaveBeenCalledTimes(8)
    expect(ctx.routesCtx.saveRoute).toHaveBeenCalledWith('digest_polish', { bindings: [] })
    expect(ctx.routesCtx.saveRoute).toHaveBeenCalledWith('summary', {
      bindings: [
        { model_id: 1, thinking_level: null },
        { model_id: 2, thinking_level: 'low' },
      ],
    })
    expect(ctx.routesCtx.saveRoute).toHaveBeenCalledWith('topic_tagging', {
      bindings: [
        { model_id: 1, thinking_level: null },
        { model_id: 2, thinking_level: null },
      ],
    })
    expect(ctx.routesCtx.saveRoute).toHaveBeenCalledWith('embedding', {
      bindings: [{ model_id: 3, thinking_level: null }],
    })
    expect(lastToast()?.type).toBe('success')
  })

  it('清空保存：移除某能力最后一个绑定后保存，该能力 PUT bindings=[] 且成功后本地态不复活', async () => {
    // 模拟真实 saveRoute 语义：后端接受 payload 后 fetchRoutes 刷新路由缓存
    ctx.routesCtx.saveRoute.mockImplementation(async (capability: string, data: RouteSaveRequest) => {
      const route = ctx.routesCtx.routes.value.find(r => r.capability === capability)
      if (route) {
        route.route_models = data.bindings.map((b, i) => ({
          model_id: b.model_id,
          thinking_level: b.thinking_level ?? null,
          priority: i + 1,
        })) as AIRouteModelBinding[]
      }
      return true
    })
    const wrapper = await mountRoutes()

    // × 移除 topic_tagging 唯一绑定 → 保存
    await section(wrapper, 'topic_tagging').find('.route-icon-btn--danger').trigger('click')
    await saveButton(wrapper).trigger('click')
    await flushAsync()

    // 空链也被持久化：PUT bindings=[]（后端 UpsertRouteModels 清空语义）
    expect(ctx.routesCtx.saveRoute).toHaveBeenCalledWith('topic_tagging', { bindings: [] })
    expect(lastToast()?.type).toBe('success')

    // 保存成功后 hydrate 自刷新后的路由：清空态不复活
    expect(section(wrapper, 'topic_tagging').find('.route-section__empty').text()).toContain('点击下方按钮添加模型')
    expect(chainItems(section(wrapper, 'topic_tagging'))).toHaveLength(0)
  })

  it('保存失败：toast 后端错误原文并停在失败能力（未保存能力本地态保留）', async () => {
    const wrapper = await mountRoutes()
    ctx.routesCtx.saveRoute.mockResolvedValueOnce(false)
    ctx.routesCtx.error.value = 'embedding 路由不能挂 llm 模型「X」'

    await saveButton(wrapper).trigger('click')
    await flushAsync()

    expect(ctx.routesCtx.saveRoute).toHaveBeenCalledTimes(1)
    expect(lastToast()?.type).toBe('error')
    expect(lastToast()?.message).toBe('embedding 路由不能挂 llm 模型「X」')
  })

  it('空链态：虚线引导；无可绑定候选时提示先启用对应类型模型', async () => {
    const wrapper = await mountRoutes()
    expect(section(wrapper, 'digest_polish').find('.route-section__empty').text()).toContain('点击下方按钮添加模型')

    // 全部模型停用 → 无候选 → hint 引导
    installMocks({ models: MODELS.map(m => ({ ...m, enabled: false, usable: false })) })
    const bare = await mountRoutes()
    expect(bare.find('.route-section__hint').text()).toContain('先在「AI 模型」里启用一个 llm 类型的模型')
  })
})

describe('AIRouterCapabilityRoutes 定位跳入（observability Scenario: 定位跳转能力路由）', () => {
  it('focus_capability 到达后对应 route-section 脉冲高亮一次，随后清掉 query 参数（保留 section）', async () => {
    setRouteQuery({ section: 'capability-routes', focus_capability: 'topic_tagging' })
    installMocks()
    const wrapper = await mountRoutes()

    const tagging = section(wrapper, 'topic_tagging')
    expect(tagging.classes()).toContain('route-section--pulse')
    expect(section(wrapper, 'summary').classes()).not.toContain('route-section--pulse')

    // 聚焦即清参：focus_capability 移除，section 保留（深链不丢）
    expect(routerMocks.replace).toHaveBeenCalledTimes(1)
    expect(lastReplaceQuery()).toEqual({ section: 'capability-routes' })

    await vi.advanceTimersByTimeAsync(1500)
    expect(tagging.classes()).not.toContain('route-section--pulse')
  })

  it('未知 focus_capability 忽略：不脉冲、不清参', async () => {
    setRouteQuery({ section: 'capability-routes', focus_capability: 'nope' })
    installMocks()
    const wrapper = await mountRoutes()

    for (const el of wrapper.findAll('.route-section')) {
      expect(el.classes()).not.toContain('route-section--pulse')
    }
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  it('挂载期间 focus_capability 变化（watch 路径）同样定位脉冲并清参', async () => {
    const wrapper = await mountRoutes()
    expect(section(wrapper, 'topic_tagging').classes()).not.toContain('route-section--pulse')

    setRouteQuery({ section: 'capability-routes', focus_capability: 'topic_tagging' })
    await flushAsync()

    const tagging = section(wrapper, 'topic_tagging')
    expect(tagging.classes()).toContain('route-section--pulse')
    expect(lastReplaceQuery()).toEqual({ section: 'capability-routes' })

    await vi.advanceTimersByTimeAsync(1500)
    expect(tagging.classes()).not.toContain('route-section--pulse')
  })
})
