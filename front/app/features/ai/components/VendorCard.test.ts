import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { AIModel, AIVendor } from '~/types'

const apiMocks = vi.hoisted(() => ({
  testVendor: vi.fn(),
  testVendorInline: vi.fn(),
}))

vi.mock('~/api', () => ({
  useAIAdminApi: () => apiMocks,
}))

import VendorCard from './VendorCard.vue'
import AppConfirmDialog from '~/components/ui/AppConfirmDialog.vue'
import { useConfirm } from '~/composables/useConfirm'

const stubs = { Icon: true }

function makeVendor(partial: Partial<AIVendor> & { id: number; name: string }): AIVendor {
  return {
    provider_type: 'openai_compatible',
    base_url: 'http://10.11.12.111:8080/v1',
    timeout_seconds: 300,
    thinking_dialect: 'qwen-chat-template',
    structured_output: 'json_schema',
    start_command_configured: true,
    api_key_configured: false,
    enabled: true,
    models_count: 3,
    capabilities_count: 2,
    ...partial,
  }
}

let modelId = 10

function makeModel(partial: Partial<AIModel> & { name: string }): AIModel {
  modelId += 1
  return {
    id: modelId,
    vendor_id: 1,
    model_kind: 'llm',
    thinking_level: 'medium',
    supports_xhigh_max: true,
    enabled: true,
    status: 'ok',
    last_seen_at: new Date(Date.now() - 3600_000).toISOString(),
    ...partial,
  }
}

function mountCard(props: {
  vendor: AIVendor
  models: AIModel[]
  expanded?: boolean
  refreshing?: boolean
  refreshDiff?: { added: string[]; stale: string[] } | null
}) {
  return mount(VendorCard, { props, global: { stubs } })
}

/** 卡片 + 全局确认弹窗 harness（U5 删除确认走真实 AppConfirmDialog 管道） */
function mountCardWithConfirm(vendor: AIVendor, models: AIModel[], bindingsCount = 0) {
  return mount({
    components: { VendorCard, AppConfirmDialog },
    data: () => ({ vendor, models, bindingsCount, deleteCount: 0 }),
    template: `<div>
      <VendorCard :vendor="vendor" :models="models" :expanded="true" :bindings-count="bindingsCount" @delete="deleteCount++" />
      <AppConfirmDialog />
    </div>`,
  }, { global: { stubs } })
}

describe('VendorCard 卡片头（U2）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('卡片头渲染 名称/base_url/N 模型/承载 N 能力徽章，点击头 emit toggle', async () => {
    const wrapper = mountCard({ vendor: makeVendor({ id: 1, name: 'llamacpp-qwen' }), models: [] })

    expect(wrapper.text()).toContain('llamacpp-qwen')
    expect(wrapper.text()).toContain('http://10.11.12.111:8080/v1')
    expect(wrapper.text()).toContain('3 模型')
    expect(wrapper.find('.fanout-badge').classes()).not.toContain('fanout-badge--zero')
    expect(wrapper.find('.fanout-badge').text()).toBe('承载 2 能力')
    // 未展开：无清单面板
    expect(wrapper.find('.vendor-panel').exists()).toBe(false)

    await wrapper.find('.vendor-head').trigger('click')
    expect(wrapper.emitted('toggle')).toHaveLength(1)
  })

  it('零能力：徽章常显为「未接入路由」muted 形态', () => {
    const wrapper = mountCard({
      vendor: makeVendor({ id: 1, name: 'lonely', capabilities_count: 0 }),
      models: [],
    })
    const badge = wrapper.find('.fanout-badge')
    expect(badge.text()).toBe('未接入路由')
    expect(badge.classes()).toContain('fanout-badge--zero')
  })

  it('展开清单：行渲染 启用勾选/模型名/类型/默认档/xh 开关；models_count 缺省回退行数', async () => {
    const models = [makeModel({ name: 'qwythos' })]
    const wrapper = mountCard({
      vendor: makeVendor({ id: 1, name: 'v', models_count: undefined }),
      models,
      expanded: true,
    })

    expect(wrapper.find('.vendor-panel').exists()).toBe(true)
    expect(wrapper.text()).toContain('1 模型')
    const row = wrapper.find('.model-row:not(.model-row--head)')
    expect((row.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(true)
    expect(row.find('.col-name').text()).toContain('qwythos')
    expect((row.find('select.model-kind-select').element as HTMLSelectElement).value).toBe('llm')
    expect((row.find('select.model-level-select').element as HTMLSelectElement).value).toBe('medium')
    expect((row.find('input.xhigh-check').element as HTMLInputElement).checked).toBe(true)
  })

  it('默认档下拉钳制：supports_xhigh_max=false 无 xhigh/max 选项；勾掉开关且默认档在高两档时自动回落 high', async () => {
    const models = [
      makeModel({ name: 'plain', thinking_level: 'high', supports_xhigh_max: false }),
      makeModel({ name: 'peak', thinking_level: 'xhigh', supports_xhigh_max: true }),
      makeModel({ name: 'mid', thinking_level: 'high', supports_xhigh_max: true }),
    ]
    const wrapper = mountCard({ vendor: makeVendor({ id: 1, name: 'v' }), models, expanded: true })

    const rows = wrapper.findAll('.model-row:not(.model-row--head)')
    // 未声明 xh/max：下拉无高两档；声明了则两档都在
    const plainLevels = rows[0]!.findAll('select.model-level-select option').map(o => (o.element as HTMLOptionElement).value)
    expect(plainLevels).not.toContain('xhigh')
    expect(plainLevels).not.toContain('max')
    const peakLevels = rows[1]!.findAll('select.model-level-select option').map(o => (o.element as HTMLOptionElement).value)
    expect(peakLevels).toContain('xhigh')
    expect(peakLevels).toContain('max')

    // 勾掉 peak 的开关：默认档 xhigh → 一次 patch 同时落 supports_xhigh_max=false + thinking_level=high
    await rows[1]!.find('input.xhigh-check').setValue(false)
    const events = wrapper.emitted('update-model') ?? []
    expect(events).toHaveLength(1)
    const [model, patch] = events[0]!
    expect((model as AIModel).name).toBe('peak')
    expect(patch).toEqual({ supports_xhigh_max: false, thinking_level: 'high' })

    // 默认档不在高两档时勾掉开关：仅落 supports_xhigh_max，不牵动默认档
    await rows[2]!.find('input.xhigh-check').setValue(false)
    const plainEvents = (wrapper.emitted('update-model') ?? []).slice(1)
    expect(plainEvents).toHaveLength(1)
    expect((plainEvents[0]![0] as AIModel).name).toBe('mid')
    expect(plainEvents[0]![1]).toEqual({ supports_xhigh_max: false })
  })

  it('embedding 行：思考档显示 —，无 xh 开关', () => {
    const models = [makeModel({ name: 'qwen3-embedding', model_kind: 'embedding', thinking_level: null, supports_xhigh_max: false })]
    const wrapper = mountCard({ vendor: makeVendor({ id: 1, name: 'v' }), models, expanded: true })

    const row = wrapper.find('.model-row:not(.model-row--head)')
    expect(row.find('.dash').exists()).toBe(true)
    expect(row.find('select.model-level-select').exists()).toBe(false)
    expect(row.find('input.xhigh-check').exists()).toBe(false)
  })

  it('本地 vendor：类型下拉可选 embedding；云端 vendor：下拉禁用且无 embedding 选项', () => {
    const local = mountCard({ vendor: makeVendor({ id: 1, name: 'local' }), models: [makeModel({ name: 'm1' })], expanded: true })
    const localSelect = local.find('select.model-kind-select')
    expect(localSelect.attributes('disabled')).toBeUndefined()
    expect(localSelect.findAll('option').some(o => o.element.value === 'embedding')).toBe(true)

    const cloud = mountCard({
      vendor: makeVendor({ id: 2, name: 'cloud', base_url: 'https://api.deepseek.com/v1' }),
      models: [makeModel({ name: 'm2', vendor_id: 2 })],
      expanded: true,
    })
    const cloudSelect = cloud.find('select.model-kind-select')
    expect(cloudSelect.attributes('disabled')).toBeDefined()
    expect(cloudSelect.findAll('option').some(o => o.element.value === 'embedding')).toBe(false)
  })

  it('清单空态：无模型时显示拉取引导', () => {
    const wrapper = mountCard({ vendor: makeVendor({ id: 1, name: 'v', models_count: 0 }), models: [], expanded: true })
    expect(wrapper.text()).toContain('暂无模型')
  })
})

describe('VendorCard 清单行 diff 与刷新锁（U3/U2）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('refresh diff added 集合内 / last_seen_at 近 5 分钟内的模型带「新」标', () => {
    const models = [
      makeModel({ name: 'm-new-diff' }),
      makeModel({ name: 'm-new-seen', last_seen_at: new Date().toISOString() }),
      makeModel({ name: 'm-old' }),
    ]
    const wrapper = mountCard({
      vendor: makeVendor({ id: 1, name: 'v' }),
      models,
      expanded: true,
      refreshDiff: { added: ['m-new-diff'], stale: [] },
    })

    const names = wrapper.findAll('.model-row:not(.model-row--head)')
    expect(names[0]!.find('.new-tag').exists()).toBe(true)
    expect(names[1]!.find('.new-tag').exists()).toBe(true)
    expect(names[2]!.find('.new-tag').exists()).toBe(false)
  })

  it('stale 行：整行灰显 + stale 标 + 启用勾选禁用', () => {
    const models = [
      makeModel({ name: 'alive' }),
      makeModel({ name: 'gone', status: 'stale', enabled: false }),
    ]
    const wrapper = mountCard({ vendor: makeVendor({ id: 1, name: 'v' }), models, expanded: true })

    const rows = wrapper.findAll('.model-row:not(.model-row--head)')
    expect(rows[0]!.classes()).not.toContain('model-row--stale')
    expect(rows[1]!.classes()).toContain('model-row--stale')
    expect(rows[1]!.text()).toContain('stale')
    expect(rows[1]!.find('.app-toggle').classes()).toContain('is-disabled')
  })

  it('刷新 loading 锁：按钮禁用显示刷新中，emit refresh；清单行保持旧数据渲染', async () => {
    const models = [makeModel({ name: 'm1' })]
    const wrapper = mountCard({ vendor: makeVendor({ id: 1, name: 'v' }), models, expanded: true, refreshing: true })

    const refreshBtn = wrapper.findAll('button').find(b => b.text().includes('刷新中…'))!
    expect(refreshBtn.attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('正在刷新模型列表…')

    // 失败场景下父层不改 props：旧清单保持渲染（State Matrix：刷新失败保留旧清单）
    expect(wrapper.text()).toContain('m1')
    // 锁内 disabled 吞点击（happy-dom 语义）；解锁后可点且 emit refresh
    await wrapper.setProps({ refreshing: false })
    await wrapper.findAll('button').find(b => b.text().includes('刷新模型列表'))!.trigger('click')
    expect(wrapper.emitted('refresh')).toHaveLength(1)
  })

  it('清单行变更 emit update-model：启用/类型切 embedding 清思考档/思考档空串清除/xh 开关', async () => {
    const models = [makeModel({ name: 'm1' })]
    const wrapper = mountCard({ vendor: makeVendor({ id: 1, name: 'v' }), models, expanded: true })
    const row = wrapper.find('.model-row:not(.model-row--head)')

    await row.find('.app-toggle__track').trigger('click')
    expect(wrapper.emitted('update-model')![0]).toEqual([models[0], { enabled: false }])

    await row.find('select.model-kind-select').setValue('embedding')
    expect(wrapper.emitted('update-model')![1]).toEqual([models[0], { model_kind: 'embedding', thinking_level: '' }])

    await row.find('select.model-level-select').setValue('')
    expect(wrapper.emitted('update-model')![2]).toEqual([models[0], { thinking_level: '' }])

    await row.find('select.model-level-select').setValue('high')
    expect(wrapper.emitted('update-model')![3]).toEqual([models[0], { thinking_level: 'high' }])

    await row.find('input.xhigh-check').setValue(false)
    expect(wrapper.emitted('update-model')![4]).toEqual([models[0], { supports_xhigh_max: false }])
  })

  it('测试连接三态：按已存配置内联测，成功/失败内联展示', async () => {
    const wrapper = mountCard({ vendor: makeVendor({ id: 7, name: 'v' }), models: [], expanded: true })

    const testBtn = () => wrapper.findAll('button').find(b => b.text().includes('测试连接') || b.text().includes('测试中…'))!
    // 手动 resolve 的 deferred：确定性地观察「测试中」中间态
    let resolveTest!: (v: unknown) => void
    apiMocks.testVendor.mockImplementationOnce(() => new Promise(r => { resolveTest = r }))
    await testBtn().trigger('click')
    expect(testBtn().text()).toContain('测试中…')
    expect(testBtn().attributes('disabled')).toBeDefined()

    resolveTest({ success: true, message: '连接成功，共 3 个可用模型', data: { models: [] } })
    await flushPromises()
    expect(wrapper.find('.test-result--ok').text()).toContain('共 3 个可用模型')
    expect(apiMocks.testVendor).toHaveBeenCalledWith(7)

    apiMocks.testVendor.mockResolvedValueOnce({ success: false, error: 'dial timeout' })
    await testBtn().trigger('click')
    await flushPromises()
    expect(wrapper.find('.test-result--fail').text()).toContain('dial timeout')
  })
})

describe('VendorCard 删除级联确认（U5）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('点删除弹确认：正文列明「N 个模型 / M 条绑定」（M=真实绑定条数），danger 确认后 emit delete', async () => {
    // 2 模型×同 1 能力 → 去重能力数 1，真实绑定 2 条：确认文案 M 口径应为后者
    const wrapper = mountCardWithConfirm(
      makeVendor({ id: 1, name: 'llamacpp-qwen', models_count: 3, capabilities_count: 1 }),
      [makeModel({ name: 'm1' }), makeModel({ name: 'm2' })],
      2,
    )

    await wrapper.find('.panel-btn--danger').trigger('click')
    await flushPromises()

    const { state } = useConfirm()
    expect(state.value?.open).toBe(true)
    expect(state.value?.title).toContain('llamacpp-qwen')
    expect(state.value?.message).toContain('3 个模型')
    expect(state.value?.message).toContain('2 条')
    expect(state.value?.message).not.toContain('1 条')
    expect(state.value?.danger).toBe(true)

    // 确认按钮（AppButton danger variant）→ emit delete
    const confirmBtn = Array.from(document.body.querySelectorAll('button')).find(b => b.textContent?.includes('删除'))
    expect(confirmBtn).toBeTruthy()
    await confirmBtn!.click()
    await flushPromises()

    expect((wrapper.vm as unknown as { deleteCount: number }).deleteCount).toBe(1)
    expect(state.value?.open).toBe(false)
  })

  it('取消确认：不 emit delete', async () => {
    const wrapper = mountCardWithConfirm(
      makeVendor({ id: 1, name: 'v', models_count: 1, capabilities_count: 0 }),
      [],
    )

    await wrapper.find('.panel-btn--danger').trigger('click')
    await flushPromises()

    const cancelBtn = Array.from(document.body.querySelectorAll('button')).find(b => b.textContent?.includes('取消'))
    await cancelBtn!.click()
    await flushPromises()

    expect((wrapper.vm as unknown as { deleteCount: number }).deleteCount).toBe(0)
  })
})
