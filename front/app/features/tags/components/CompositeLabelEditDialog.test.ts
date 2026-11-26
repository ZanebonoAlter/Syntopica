import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { defineComponent, h } from 'vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import { Z_INDEX, nestedOverlayZ } from '~/components/ui/layout-contract'
import CompositeLabelEditDialog from './CompositeLabelEditDialog.vue'
import type { CompositeLabel } from '~/api/compositeLabels'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const getLabelsMock = vi.fn()
const createLabelMock = vi.fn()
const getComponentOptionsMock = vi.fn()

vi.mock('~/api/auxiliaryLabels', () => ({
  useAuxiliaryLabelsApi: () => ({ getLabels: getLabelsMock }),
}))

const addCompositionMock = vi.fn()
vi.mock('~/api/semanticBoards', () => ({
  useSemanticBoardsApi: () => ({ addComposition: addCompositionMock }),
}))

vi.mock('~/api/compositeLabels', () => ({
  useCompositeLabelsApi: () => ({ createLabel: createLabelMock, getComponentOptions: getComponentOptionsMock }),
}))

function auxItem(id: number, label: string) {
  return { id, label, slug: `aux-${id}`, aliases: [], ref_count: 5, description: '', display_order: 0, source: 'llm', status: 'active', protected: false }
}

function optionItem(id: number, label: string, boardCount = 0, boardLabels: string[] = [], inBoard = false, cooccurrence = 0) {
  return {
    id,
    label,
    ref_count: 5,
    board_count: boardCount,
    in_board: inBoard,
    cooccurrence,
    mounted_boards: boardLabels.map((label, i) => ({ id: 100 + i, label })),
  }
}

function mountBoardDialog(composites: CompositeLabel[] = [], boardId?: number, boardLabel?: string) {
  return mount(CompositeLabelEditDialog, {
    props: { visible: true, composites, boardId, boardLabel },
    global: { stubs: { teleport: true } },
  })
}

function mountDialog(composites: CompositeLabel[] = []) {
  return mount(CompositeLabelEditDialog, {
    props: { visible: true, composites },
    global: { stubs: { teleport: true } },
  })
}

beforeEach(() => {
  // resetAllMocks：清掉上个用例残留的 Once 队列与 implementation 覆盖
  // （clearAllMocks 只清调用记录，mockResolvedValueOnce/RejectedValue 会跨用例泄漏）
  vi.resetAllMocks()
  getLabelsMock.mockResolvedValue({ success: true, data: { items: [auxItem(1, '美国国债'), auxItem(2, '收益率'), auxItem(3, 'CPI'), auxItem(4, '美联储'), auxItem(5, '加息'), auxItem(6, '通胀'), auxItem(7, '就业')], total: 7 } })
  getComponentOptionsMock.mockResolvedValue({ success: true, data: { items: [
    optionItem(1, '美国国债', 2, ['宏观版块', '债券版块']),
    optionItem(2, '收益率', 1, ['宏观版块']),
    optionItem(3, 'CPI'),
    optionItem(4, '美联储'),
    optionItem(5, '加息'),
    optionItem(6, '通胀'),
    optionItem(7, '就业'),
  ] } })
})

describe('CompositeLabelEditDialog — 手动创建组合标签', () => {
  it('渲染组件选择器（active 辅助标签）', async () => {
    const w = mountDialog()
    await flushPromises()
    const picker = w.find('[data-testid="composite-label-picker"]')
    expect(picker.exists()).toBe(true)
    expect(w.findAll('.cld-picker-item').length).toBe(7)
  })

  it('勾选少于 2 个组件时显示计数错误且提交禁用（变体1）', async () => {
    const w = mountDialog()
    await flushPromises()
    await w.find('[data-testid="composite-label-name"]').setValue('美债收益率')
    // 只选 1 个
    await w.findAll('.cld-picker-item')[0]!.trigger('click')
    expect(w.find('[data-testid="composite-label-count-error"]').text()).toContain('至少选择 2 个组件')
    expect((w.find('[data-testid="composite-label-submit"]').element as HTMLButtonElement).disabled).toBe(true)
  })

  it('选满 5 个后第 6 个不可再选（上限拦截，已选不丢）', async () => {
    const w = mountDialog()
    await flushPromises()
    const items = w.findAll('.cld-picker-item')
    for (let i = 0; i < 5; i++) await items[i]!.trigger('click')
    // 重新查询（点击后重渲染使旧 wrapper 失效）
    const refreshed = w.findAll('.cld-picker-item')
    // 第 6 个按钮呈禁用态且点击无效
    await refreshed[5]!.trigger('click')
    expect(w.findAll('[data-testid^="composite-label-component-"]').length).toBe(5)
    expect(w.findAll('.cld-picker-item')[5]!.classes()).toContain('is-disabled')
  })

  it('创建成功展示 outcome=created 提示并 emit confirm', async () => {
    createLabelMock.mockResolvedValue({ success: true, data: { id: 10, label: '美债收益率', outcome: 'created', message: '组合标签已创建' } })
    const w = mountDialog()
    await flushPromises()
    await w.find('[data-testid="composite-label-name"]').setValue('美债收益率')
    await w.findAll('.cld-picker-item')[0]!.trigger('click')
    await w.findAll('.cld-picker-item')[1]!.trigger('click')
    await w.find('[data-testid="composite-label-submit"]').trigger('click')
    await flushPromises()
    expect(createLabelMock).toHaveBeenCalledWith({ label: '美债收益率', description: undefined, component_label_ids: [1, 2] })
    expect(w.find('[data-testid="composite-label-result"]').text()).toContain('组合标签已创建')
    expect(w.emitted('confirm')).toBeTruthy()
  })

  it('去重命中（reused_l1）不视为错误——展示复用信息（变体5）', async () => {
    createLabelMock.mockResolvedValue({ success: true, data: { id: 9, label: '美债收益率', outcome: 'reused_l1', message: '组件集合与既有组合标签完全一致，已复用既有组合（ref_count+1）' } })
    const w = mountDialog()
    await flushPromises()
    await w.find('[data-testid="composite-label-name"]').setValue('美国国债收益率')
    await w.findAll('.cld-picker-item')[1]!.trigger('click')
    await w.findAll('.cld-picker-item')[0]!.trigger('click')
    await w.find('[data-testid="composite-label-submit"]').trigger('click')
    await flushPromises()
    // 提交按组件选择顺序（position 顺序语义）
    expect(createLabelMock).toHaveBeenCalledWith({ label: '美国国债收益率', description: undefined, component_label_ids: [2, 1] })
    const result = w.find('[data-testid="composite-label-result"]')
    expect(result.text()).toContain('复用')
    expect(w.find('[data-testid="composite-label-error"]').exists()).toBe(false)
  })

  it('API 失败展示错误提示（变体3）', async () => {
    createLabelMock.mockRejectedValue(new Error('组件不存在'))
    const w = mountDialog()
    await flushPromises()
    await w.find('[data-testid="composite-label-name"]').setValue('组合X')
    await w.findAll('.cld-picker-item')[0]!.trigger('click')
    await w.findAll('.cld-picker-item')[1]!.trigger('click')
    await w.find('[data-testid="composite-label-submit"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="composite-label-error"]').text()).toContain('组件不存在')
  })
})

describe('CompositeLabelEditDialog — 组件推荐交互（S12，design D7）', () => {
  it('默认候选来自 component-options 且带「挂 N 版块」推荐信号（主链路步2）', async () => {
    const w = mountDialog()
    await flushPromises()
    expect(getComponentOptionsMock).toHaveBeenCalled()
    expect(getLabelsMock).not.toHaveBeenCalled()
    expect(w.findAll('.cld-picker-item').length).toBe(7)
    const badges = w.findAll('[data-testid="composite-label-board-badge"]')
    expect(badges.length).toBe(2)
    expect(badges[0]!.text()).toBe('挂2版块')
    expect(badges[1]!.text()).toBe('挂1版块')
  })

  it('选中组件后展示相关现有组合，集合一致时预告复用（主链路步3）', async () => {
    const composites: CompositeLabel[] = [
      {
        id: 9, label: '美债收益率', slug: 'mei-zhai', description: '', source: 'manual', status: 'active', ref_count: 3, aliases: [], created_at: '2026-09-01',
        components: [
          { label_id: 1, label: '美国国债', position: 1 },
          { label_id: 2, label: '收益率', position: 2 },
        ],
      },
      {
        id: 10, label: '通胀就业', slug: 'tongzhang', description: '', source: 'manual', status: 'disabled', ref_count: 1, aliases: [], created_at: '2026-09-01',
        components: [
          { label_id: 6, label: '通胀', position: 1 },
          { label_id: 7, label: '就业', position: 2 },
        ],
      },
    ]
    const w = mountDialog(composites)
    await flushPromises()
    expect(w.find('[data-testid="composite-label-related"]').exists()).toBe(false)

    // 只选「美国国债」→ 相关组合含「美债收益率」
    await w.findAll('.cld-picker-item')[0]!.trigger('click')
    const related = w.find('[data-testid="composite-label-related"]')
    expect(related.exists()).toBe(true)
    expect(related.text()).toContain('美债收益率')
    expect(related.text()).not.toContain('通胀就业')
    expect(w.find('[data-testid="composite-label-reuse-hint"]').exists()).toBe(false)

    // 再选「收益率」→ 集合与「美债收益率」完全一致 → 复用预告
    await w.findAll('.cld-picker-item')[1]!.trigger('click')
    expect(w.find('[data-testid="composite-label-related"]').exists()).toBe(true)
    expect(w.find('[data-testid="composite-label-reuse-hint"]').exists()).toBe(true)
    expect(w.find('[data-testid="composite-label-reuse-hint"]').text()).toContain('创建将复用此组合')
  })

  it('搜索时降级全量模糊检索（主链路步2 搜索路径）', async () => {
    const w = mountDialog()
    await flushPromises()
    await w.find('.cld-input--search').setValue('收益')
    await flushPromises()
    expect(getLabelsMock).toHaveBeenCalledWith({ status: 'active', search: '收益', per_page: 100 })
    expect(w.findAll('.cld-picker-item').length).toBe(7)
    expect(w.findAll('[data-testid="composite-label-board-badge"]').length).toBe(0)
  })

  it('component-options 失败时降级回 aux 全量列表（变体3）', async () => {
    getComponentOptionsMock.mockRejectedValue(new Error('boom'))
    const w = mountDialog()
    await flushPromises()
    expect(getLabelsMock).toHaveBeenCalledWith({ status: 'active', per_page: 100 })
    expect(w.findAll('.cld-picker-item').length).toBe(7)
  })
})

describe('CompositeLabelEditDialog — 已选展示回归（联动重拉截断）', () => {
  it('选中组件触发联动重拉后，最新选中组件不在新列表（top50 截断）也必须在已选序列中展示', async () => {
    // 首次：完整列表含 1=美联储、2=加息
    getComponentOptionsMock.mockResolvedValueOnce({ success: true, data: { items: [
      optionItem(1, '美联储'), optionItem(2, '加息'),
    ] } })
    const w = mountBoardDialog([], 2197, '美国新闻')
    await flushPromises()
    // 联动重拉的返回值必须提前设置：trigger 内的 watcher 异步触发 loadOptions，
    // 点击后再 mock 就晚了（会吃到默认列表）。
    getComponentOptionsMock.mockResolvedValueOnce({ success: true, data: { items: [
      optionItem(2, '加息', 0, [], false, 11), optionItem(3, '沃什', 0, [], false, 4),
    ] } })
    await w.findAll('.cld-picker-item')[0]!.trigger('click') // 选「美联储」

    await flushPromises()

    const selectedItems = w.findAll('[data-testid^="composite-label-component-"]')
    expect(selectedItems.length).toBe(1)
    expect(selectedItems[0]!.text()).toContain('美联储')

    // 从截断列表里再选「沃什」，两个都已选且顺序正确（再次预设重拉返回，防默认列表同 id 覆盖缓存）
    getComponentOptionsMock.mockResolvedValueOnce({ success: true, data: { items: [
      optionItem(2, '加息', 0, [], false, 11), optionItem(3, '沃什', 0, [], false, 4),
    ] } })
    await w.findAll('.cld-picker-item')[1]!.trigger('click')
    await flushPromises()
    const after = w.findAll('[data-testid^="composite-label-component-"]')
    expect(after.length).toBe(2)
    expect(after[0]!.text()).toContain('美联储')
    expect(after[1]!.text()).toContain('沃什')
  })
})

describe('CompositeLabelEditDialog — 版块上下文与共现联动（design D7 升级）', () => {
  it('boardId 上下文：请求带 board_id、本版块徽标置顶展示', async () => {
    getComponentOptionsMock.mockResolvedValue({ success: true, data: { items: [
      optionItem(1, '美联储', 1, ['宏观版块'], true),
      optionItem(2, '全球热词', 2, ['其它版块A', '其它版块B']),
    ] } })
    const w = mountBoardDialog([], 2197, '美国新闻')
    await flushPromises()
    expect(getComponentOptionsMock).toHaveBeenCalledWith({ board_id: 2197, related_to: undefined })
    expect(w.find('[data-testid="composite-label-inboard-badge"]').text()).toBe('本版块')
    expect(w.find('[data-testid="composite-label-board-badge"]').text()).toBe('挂2版块')
  })

  it('选中组件后联动重拉：related_to=最新选中，共现徽标展示', async () => {
    getComponentOptionsMock.mockResolvedValue({ success: true, data: { items: [
      optionItem(1, '美联储'), optionItem(2, '加息', 0, [], false, 11),
    ] } })
    const w = mountBoardDialog([], 2197, '美国新闻')
    await flushPromises()
    getComponentOptionsMock.mockClear()

    await w.findAll('.cld-picker-item')[0]!.trigger('click')
    await flushPromises()
    expect(getComponentOptionsMock).toHaveBeenCalledWith({ board_id: 2197, related_to: 1 })
    // 重拉后的列表含共现徽标
    expect(w.find('[data-testid="composite-label-cooc-badge"]').text()).toBe('共现11')
  })

  it('创建成功自动挂载到版块上下文', async () => {
    createLabelMock.mockResolvedValue({ success: true, data: { id: 42, label: '美联储加息', outcome: 'created', message: '组合标签已创建' } })
    addCompositionMock.mockResolvedValue({ success: true, data: {} })
    const w = mountBoardDialog([], 2197, '美国新闻')
    await flushPromises()
    await w.find('[data-testid="composite-label-name"]').setValue('美联储加息')
    await w.findAll('.cld-picker-item')[0]!.trigger('click')
    await w.findAll('.cld-picker-item')[1]!.trigger('click')
    await w.find('[data-testid="composite-label-submit"]').trigger('click')
    await flushPromises()
    expect(addCompositionMock).toHaveBeenCalledWith(2197, 42)
    expect(w.find('[data-testid="composite-label-result"]').text()).toContain('已挂载到「美国新闻」')
  })
})

/**
 * 层级契约（standard/frontend/layout.md「浮层层级契约」）：
 * 组合标签创建弹窗是「弹窗内再开的嵌套浮层」——层级必须严格高于宿主弹窗遮罩，
 * 否则被宿主遮罩（连同 backdrop blur）整块盖住，用户点了「创建并挂载」完全看不到弹窗
 * （2026-10-30 用户实机报障）。
 *
 * 断言取**内联** z-index：vitest 未开启 css 处理，`<style scoped>` 不生效，
 * 样式表里写死的层级读不到（getComputedStyle 恒为空），只有内联绑定值可断言。
 */
const LayerHost = defineComponent({
  setup() {
    return () => h(AppDialog, { modelValue: true, size: 'xl' }, {
      default: () => h(CompositeLabelEditDialog, { visible: true }),
    })
  },
})

/** 宿主在更高档（阅读层）时的组合宿主：嵌套浮层按 nestedOverlayZ(hostZ) 抬升后显式传入。 */
const RaisedLayerHost = defineComponent({
  setup() {
    return () => h(AppDialog, { modelValue: true, size: 'xl', zIndex: Z_INDEX.readingDialog }, {
      default: () => h(CompositeLabelEditDialog, { visible: true, zIndex: nestedOverlayZ(Z_INDEX.readingDialog) }),
    })
  },
})

describe('CompositeLabelEditDialog — 层级契约（浮层层级契约）', () => {
  it('嵌套浮层内联层级严格大于宿主 AppDialog 遮罩内联层级（不被宿主遮罩盖住）', async () => {
    const host = mount(LayerHost, { global: { stubs: { teleport: true } } })
    await flushPromises()
    const hostOverlay = host.find('.app-dialog-overlay').element as HTMLElement
    const nestedOverlay = host.find('.cld-overlay').element as HTMLElement
    const hostZ = Number(hostOverlay.style.zIndex)
    const nestedZ = Number(nestedOverlay.style.zIndex)
    expect(hostZ).toBe(Z_INDEX.dialog)
    expect(nestedZ).toBe(Z_INDEX.dialogNested)
    expect(nestedZ).toBeGreaterThan(hostZ)
  })

  it('宿主在更高档时：嵌套浮层按 nestedOverlayZ(hostZ) 抬升后仍严格高于宿主', async () => {
    const host = mount(RaisedLayerHost, { global: { stubs: { teleport: true } } })
    await flushPromises()
    const hostOverlay = host.find('.app-dialog-overlay').element as HTMLElement
    const nestedOverlay = host.find('.cld-overlay').element as HTMLElement
    const hostZ = Number(hostOverlay.style.zIndex)
    const nestedZ = Number(nestedOverlay.style.zIndex)
    expect(hostZ).toBe(Z_INDEX.readingDialog)
    expect(nestedZ).toBe(nestedOverlayZ(Z_INDEX.readingDialog))
    expect(nestedZ).toBeGreaterThan(hostZ)
  })
})

/**
 * 浮层展示合理性锚（standard/frontend/layout.md「浮层组件展示合理性锚」）：
 * 该组件是自建浮层（Teleport + overlay），单测须带两条锚——
 *  1) 源码规则锚：<style> 里 .cld-overlay 必须含 position: fixed + inset: 0；
 *     层级已按「浮层层级契约」改为档位表内联注入（:style="{ zIndex }"），<style> 不得
 *     再写死 z-index——避免双源漂移（层级只有内联一处来源）。
 *  2) Teleport 挂载锚：不 stub teleport 挂载时 overlay 必须渲染在 document.body，
 *     不能退化成流内 div（浮层挂载点回退）。
 * vitest 未开启 css 处理，SFC 样式不注入 DOM，getComputedStyle 恒为空 —— 只能读源码文本。
 */
describe('CompositeLabelEditDialog — 浮层展示合理性锚', () => {
  it('样式表含 .cld-overlay 定位规则：position:fixed + inset:0，且不写死 z-index', () => {
    const src = readFileSync(`${import.meta.dirname}/CompositeLabelEditDialog.vue`, 'utf8')
    const style = src.slice(src.indexOf('<style'))
    expect(style).toMatch(/\.cld-overlay\s*\{[^}]*position:\s*fixed/s)
    expect(style).toMatch(/\.cld-overlay\s*\{[^}]*inset:\s*0/s)
    const rule = style.match(/\.cld-overlay\s*\{([^}]*)\}/s)
    expect(rule).toBeTruthy()
    expect(rule![1]).not.toMatch(/z-index/)
  })

  it('overlay 渲染挂载到 body（Teleport 生效，非流内 div）', async () => {
    const w = mount(CompositeLabelEditDialog, { props: { visible: true, composites: [] } })
    await flushPromises()
    const overlay = document.body.querySelector('.cld-overlay')
    expect(overlay).toBeTruthy()
    expect(overlay!.parentElement).toBe(document.body)
    w.unmount()
    expect(document.body.querySelector('.cld-overlay')).toBeNull()
  })
})
