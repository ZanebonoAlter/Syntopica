import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { computed, ref } from 'vue'
import type { AIModel, AIRoute, AIRouteModelBinding, RouteSaveRequest, ThinkingLevel } from '~/types'

const composableMocks = vi.hoisted(() => ({
  useAIModels: vi.fn(),
  useAIRoutes: vi.fn(),
  useAIVendors: vi.fn(),
  useRouteHealth: vi.fn(),
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
vi.mock('../composables/useAIVendors', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../composables/useAIVendors')>()
  return { ...actual, useAIVendors: composableMocks.useAIVendors }
})
vi.mock('../composables/useRouteHealth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../composables/useRouteHealth')>()
  return { ...actual, useRouteHealth: composableMocks.useRouteHealth }
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
  const vendorsCtx = {
    vendors: ref<Array<{ id: number, name: string, preset_id?: string }>>([]),
    fetchVendors: vi.fn(async () => []),
  }
  const healthCtx = {
    fetchHealth: vi.fn(async () => {}),
    cardHealth: vi.fn((_capability: string, _index: number, _name: string): { fail: number, lastErr: string | null, takeover: number } => ({ fail: 0, lastErr: null, takeover: 0 })),
    rowHealth: vi.fn((_capability: string, _hasBackup: boolean): { level: 'ok' | 'warn' | 'err', text: string, degradedCount: number } => ({ level: 'ok', text: '正常', degradedCount: 0 })),
    loaded: ref(false),
    error: ref<string | null>(null),
  }
  return { modelsCtx, routesCtx, vendorsCtx, healthCtx }
}

type TestCtx = ReturnType<typeof buildCtx>

let ctx: TestCtx

function installMocks(overrides: { routes?: AIRoute[], models?: ModelWithVendor[] } = {}): TestCtx {
  ctx = buildCtx(overrides)
  composableMocks.useAIModels.mockReturnValue(ctx.modelsCtx)
  composableMocks.useAIRoutes.mockReturnValue(ctx.routesCtx)
  composableMocks.useAIVendors.mockReturnValue(ctx.vendorsCtx)
  composableMocks.useRouteHealth.mockReturnValue(ctx.healthCtx)
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

function laneCards(capabilityRoot: ReturnType<typeof section>) {
  return capabilityRoot.findAll('.swim-card')
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

describe('AIRouterCapabilityRoutes 行式泳道（U7，tasks 8.3）', () => {
  it('泳道卡：模型名+供应商名分行（告别 model@vendor 文字串）；rank 1 高亮；初始继承/覆盖态来自 route_models', async () => {
    const wrapper = await mountRoutes()
    const summary = section(wrapper, 'summary')

    const cards = laneCards(summary)
    expect(cards).toHaveLength(2)
    expect(cards[0]!.text()).toContain('qwythos')
    expect(cards[0]!.text()).toContain('llamacpp-qwen')
    expect(cards[0]!.text()).not.toContain('@')
    expect(cards[1]!.text()).toContain('deepseek-flash')
    expect(cards[1]!.text()).toContain('DeepSeek')
    expect(cards[0]!.find('.swim-card__rank--first').exists()).toBe(true)
    expect(summary.find('.swimlane__name').text()).toBe('文章总结')
    // 初始继承/覆盖态
    expect((cards[0]!.find('select').element as HTMLSelectElement).value).toBe('')
    expect((cards[1]!.find('select').element as HTMLSelectElement).value).toBe('low')
    expect(cards[1]!.find('.swim-card__badge--override').exists()).toBe(true)
    expect(cards[0]!.find('.swim-card__badge--override').exists()).toBe(false)
  })

  it('泳道行头供血说明：八条能力均渲染非空说明小字；文案与真实供血一致', async () => {
    const wrapper = await mountRoutes()
    for (const cap of ['summary', 'topic_tagging', 'digest_polish', 'embedding', 'open_notebook', 'feed_discovery', 'data_enrichment_news', 'data_enrichment_analysis']) {
      const desc = section(wrapper, cap).find('.swimlane__desc')
      expect(desc.exists(), `${cap} 缺供血说明行`).toBe(true)
      expect(desc.text().trim()).not.toBe('')
    }
    // 关键分工（spec）：新闻总结=生命线月/年归档（不暗示日报）；日报润色=日报+近14天档
    expect(section(wrapper, 'data_enrichment_news').find('.swimlane__desc').text()).toContain('归档')
    expect(section(wrapper, 'data_enrichment_news').find('.swimlane__desc').text()).not.toContain('日报')
    expect(section(wrapper, 'digest_polish').find('.swimlane__desc').text()).toContain('14')
  })

  it('模型池横条：usable 模型按 vendor 分组渲染；停用模型不出现；行内计数正确', async () => {
    const wrapper = await mountRoutes()
    const groups = wrapper.findAll('.pool-strip__group')
    expect(groups).toHaveLength(2)
    const poolText = wrapper.find('.pool-strip').text()
    expect(poolText).toContain('qwythos')
    expect(poolText).toContain('deepseek-flash')
    expect(poolText).toContain('qwen3-embedding')
    expect(poolText).not.toContain('m-disabled')
    // 行头计数
    expect(section(wrapper, 'summary').find('.swimlane__count').text()).toBe('2')
  })

  it('hover 卡片扇出提示：另承载 N 个能力 / 未承载其他能力', async () => {
    const wrapper = await mountRoutes()
    const cards = laneCards(section(wrapper, 'summary'))
    expect(cards[0]!.find('.fanout-tip').text()).toContain('qwythos 另承载 1 个能力')
    expect(cards[1]!.find('.fanout-tip').text()).toContain('未承载其他能力')
  })

  it('调档即本地态变化：覆盖徽章出现/消失；未声明 xhigh/max 的模型下拉不含候选（钳制）', async () => {
    const wrapper = await mountRoutes()
    const firstSelect = laneCards(section(wrapper, 'summary'))[0]!.find('select')

    await firstSelect.setValue('low')
    let cards = laneCards(section(wrapper, 'summary'))
    expect(cards[0]!.find('.swim-card__badge--override').exists()).toBe(true)
    const values = cards[0]!.findAll('option').map(o => o.element.value)
    expect(values).not.toContain('xhigh')
    expect(values).not.toContain('max')

    await cards[0]!.find('select').setValue('')
    cards = laneCards(section(wrapper, 'summary'))
    expect(cards[0]!.find('.swim-card__badge--override').exists()).toBe(false)
  })

  it('点击池卡弹能力菜单（FB-7）：类型不匹配置灰、已在行中标出；选中追加行尾', async () => {
    const wrapper = await mountRoutes()

    // 点 deepseek-flash（llm）池卡 → 菜单展开
    const poolCard = wrapper.findAll('.pool-card').find(c => c.text().includes('deepseek-flash'))!
    await poolCard.trigger('click')
    const menu = document.body.querySelector('.cap-picker')
    expect(menu).toBeTruthy()
    const items = [...menu!.querySelectorAll('.cap-picker__item')]
    expect(items).toHaveLength(8)

    // embedding 行对 llm 卡置灰（仅 embedding）
    const embItem = items.find(el => el.textContent?.includes('向量嵌入'))!
    expect(embItem.className).toContain('cap-picker__item--disabled')
    expect(embItem.textContent).toContain('仅 embedding')
    // topic_tagging 可选且显示当前 1 卡
    const taggingItem = items.find(el => el.textContent?.includes('主题提取'))!
    expect(taggingItem.textContent).toContain('1 卡')
    taggingItem.dispatchEvent(new Event('click'))
    await flushAsync()

    // 追加行尾 + 菜单关闭
    const cards = laneCards(section(wrapper, 'topic_tagging'))
    expect(cards).toHaveLength(2)
    expect(cards[1]!.text()).toContain('deepseek-flash')
    expect(document.body.querySelector('.cap-picker')).toBeNull()

    // 已在行中：再点池卡菜单该项标「已在行中」
    await wrapper.findAll('.pool-card').find(c => c.text().includes('deepseek-flash'))!.trigger('click')
    const again = [...document.body.querySelectorAll('.cap-picker__item')]
    expect(again.find(el => el.textContent?.includes('主题提取'))!.textContent).toContain('已在行中')
  })

  it('←→ 换位即时生效；× 移除（FB-3 键盘/按钮双通道）', async () => {
    const wrapper = await mountRoutes()
    let cards = laneCards(section(wrapper, 'summary'))
    expect(cards).toHaveLength(2)

    // 第二张卡 ← 前移 → rank 1
    await cards[1]!.findAll('.swim-icon-btn').find(b => b.attributes('title') === '前移')!.trigger('click')
    cards = laneCards(section(wrapper, 'summary'))
    expect(cards[0]!.text()).toContain('deepseek-flash')
    expect(cards[0]!.find('.swim-card__rank--first').exists()).toBe(true)

    // 第一张卡 → 后移回去
    await cards[0]!.findAll('.swim-icon-btn').find(b => b.attributes('title') === '后移')!.trigger('click')
    cards = laneCards(section(wrapper, 'summary'))
    expect(cards[0]!.text()).toContain('qwythos')

    // × 移除第二张
    await cards[1]!.find('.swim-icon-btn--danger').trigger('click')
    cards = laneCards(section(wrapper, 'summary'))
    expect(cards).toHaveLength(1)
    expect(cards[0]!.text()).toContain('qwythos')
  })

  it('embedding 卡不出现思考档下拉（绑定不携带覆盖值），显示 —', async () => {
    const wrapper = await mountRoutes()
    const card = laneCards(section(wrapper, 'embedding'))[0]!
    expect(card.find('select').exists()).toBe(false)
    expect(card.find('.swim-card__dash').exists()).toBe(true)
  })

  it('降级内嵌标注（FB-5）：失败卡红边+徽章、接管徽章、行头健康点三态', async () => {
    // summary：顺位1失败2次（timeout），顺位2接管2次 → 行头 warn「降级中·已兜底」
    ctx.healthCtx.cardHealth.mockImplementation((_cap: string, index: number, _name: string) => {
      if (index === 0) return { fail: 2, lastErr: 'timeout 30s · 01:52', takeover: 0 }
      if (index === 1) return { fail: 0, lastErr: null, takeover: 2 }
      return { fail: 0, lastErr: null, takeover: 0 }
    })
    ctx.healthCtx.rowHealth.mockImplementation((_cap: string, hasBackup: boolean) => {
      if (_cap === 'summary' && hasBackup) return { level: 'warn', text: '24h 降级 2 次·已兜底', degradedCount: 2 }
      if (_cap === 'topic_tagging') return { level: 'err', text: '24h 失败 1 次·无兜底', degradedCount: 1 }
      return { level: 'ok', text: '正常', degradedCount: 0 }
    })
    const wrapper = await mountRoutes()

    const cards = laneCards(section(wrapper, 'summary'))
    expect(cards[0]!.classes()).toContain('swim-card--fail')
    expect(cards[0]!.find('.swim-card__err-dot').text()).toBe('2')
    expect(cards[0]!.find('.swim-card__badge--fail').text()).toContain('24h 失败 2')
    expect(cards[0]!.attributes('title')).toContain('timeout 30s')
    expect(cards[1]!.find('.swim-card__badge--takeover').text()).toContain('接管 2')
    // 行头三态
    expect(section(wrapper, 'summary').find('.swimlane__dot--warn').exists()).toBe(true)
    expect(section(wrapper, 'summary').text()).toContain('已兜底')
    expect(section(wrapper, 'topic_tagging').find('.swimlane__dot--err').exists()).toBe(true)
    expect(section(wrapper, 'topic_tagging').text()).toContain('无兜底')
    expect(section(wrapper, 'digest_polish').find('.swimlane__dot--ok').exists()).toBe(true)
  })

  it('保存：逐能力 PUT bindings=[{model_id, thinking_level}]（priority 由行序派生），成功 toast', async () => {
    const wrapper = await mountRoutes()

    // 点击菜单给 topic_tagging 追加 deepseek-flash（继承档）
    await wrapper.findAll('.pool-card').find(c => c.text().includes('deepseek-flash'))!.trigger('click')
    ;[...document.body.querySelectorAll('.cap-picker__item')]
      .find(el => el.textContent?.includes('主题提取'))!
      .dispatchEvent(new Event('click'))
    await flushAsync()

    await saveButton(wrapper).trigger('click')
    await flushAsync()

    // 逐能力全量 PUT（含 fixture 中无路由的 5 行空链 → bindings:[]）
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

  it('清空保存：移除某行最后一个绑定后保存，该能力 PUT bindings=[] 且成功后本地态不复活', async () => {
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

    await section(wrapper, 'topic_tagging').find('.swim-icon-btn--danger').trigger('click')
    await saveButton(wrapper).trigger('click')
    await flushAsync()

    expect(ctx.routesCtx.saveRoute).toHaveBeenCalledWith('topic_tagging', { bindings: [] })
    expect(lastToast()?.type).toBe('success')
    expect(section(wrapper, 'topic_tagging').find('.swimlane__empty').text()).toContain('拖入模型卡片')
    expect(laneCards(section(wrapper, 'topic_tagging'))).toHaveLength(0)
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

  it('空行引导；全部模型停用时池空态引导', async () => {
    const wrapper = await mountRoutes()
    expect(section(wrapper, 'digest_polish').find('.swimlane__empty').text()).toContain('拖入模型卡片')

    installMocks({ models: MODELS.map(m => ({ ...m, enabled: false, usable: false })) })
    const bare = await mountRoutes()
    expect(bare.find('.pool-strip').text()).toContain('先在「供应商与模型」里启用模型')
    expect(bare.findAll('.pool-card')).toHaveLength(0)
  })
})

describe('AIRouterCapabilityRoutes 定位跳入（observability Scenario: 定位跳转能力路由）', () => {
  it('focus_capability 到达后对应 route-section 脉冲高亮一次，随后清掉 query 参数（保留 section）', async () => {
    setRouteQuery({ section: 'capability-routes', focus_capability: 'topic_tagging' })
    installMocks()
    const wrapper = await mountRoutes()

    const tagging = section(wrapper, 'topic_tagging')
    expect(tagging.classes()).toContain('swimlane--pulse')
    expect(section(wrapper, 'summary').classes()).not.toContain('swimlane--pulse')

    // 聚焦即清参：focus_capability 移除，section 保留（深链不丢）
    expect(routerMocks.replace).toHaveBeenCalledTimes(1)
    expect(lastReplaceQuery()).toEqual({ section: 'capability-routes' })

    await vi.advanceTimersByTimeAsync(1500)
    expect(tagging.classes()).not.toContain('swimlane--pulse')
  })

  it('未知 focus_capability 忽略：不脉冲、不清参', async () => {
    setRouteQuery({ section: 'capability-routes', focus_capability: 'nope' })
    installMocks()
    const wrapper = await mountRoutes()

    for (const el of wrapper.findAll('.swimlane')) {
      expect(el.classes()).not.toContain('swimlane--pulse')
    }
    expect(routerMocks.replace).not.toHaveBeenCalled()
  })

  it('挂载期间 focus_capability 变化（watch 路径）同样定位脉冲并清参', async () => {
    const wrapper = await mountRoutes()
    expect(section(wrapper, 'topic_tagging').classes()).not.toContain('swimlane--pulse')

    setRouteQuery({ section: 'capability-routes', focus_capability: 'topic_tagging' })
    await flushAsync()

    const tagging = section(wrapper, 'topic_tagging')
    expect(tagging.classes()).toContain('swimlane--pulse')
    expect(lastReplaceQuery()).toEqual({ section: 'capability-routes' })

    await vi.advanceTimersByTimeAsync(1500)
    expect(tagging.classes()).not.toContain('swimlane--pulse')
  })
})

describe('CapabilitySwimlane 供血说明行（spec Scenario: 未收录能力不渲染空说明行）', () => {
  it('不传 description 时不渲染 .swimlane__desc（空占位不出现）', async () => {
    const { default: CapabilitySwimlane } = await import('./CapabilitySwimlane.vue')
    const wrapper = mount(CapabilitySwimlane, {
      global: { stubs },
      props: {
        capability: 'future_cap',
        label: '未来能力',
        cards: [],
        healthLevel: 'ok',
        healthText: '正常',
        draggingOver: false,
        rejectFlash: false,
        pulsing: false,
      },
    })
    mountedWrappers.push(wrapper)
    expect(wrapper.find('.swimlane__desc').exists()).toBe(false)

    await wrapper.setProps({ description: '供血某功能' })
    expect(wrapper.find('.swimlane__desc').text()).toBe('供血某功能')
  })
})
