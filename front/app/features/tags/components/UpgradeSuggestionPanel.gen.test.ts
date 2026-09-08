import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import UpgradeSuggestionPanel from './UpgradeSuggestionPanel.vue'
import type { UpgradeSuggestionRow, SemanticBoard } from '~/api/semanticBoards'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

/**
 * 生成入口模式选择 — split-board-upgrade-directions（spec: 生成入口模式选择）：
 *  - 两步选择（方向 create/expand × 来源 aux/composite）
 *  - 扩充方向必须选定版块才能生成；选 create 不需要
 *  - 生成调用参数四格正确（含 target_board_id / days）
 *  - watch tab 不存在（观察池退役）
 *  - 扩充建议卡片展示锁定版块徽标；行内「合并到...」下拉不存在
 *  - 旧内存探索区不存在（候选列表/簇/「获取 LLM 建议」）
 */

const boards: SemanticBoard[] = [
  { id: 42, label: '美债', description: '美国国债主题', enrichment_enabled: false, aliases: [] } as unknown as SemanticBoard,
  { id: 43, label: 'AI', description: '人工智能', enrichment_enabled: false, aliases: [] } as unknown as SemanticBoard,
]

function mergeRow(): UpgradeSuggestionRow {
  return {
    id: 7,
    batch_id: 'b1',
    mode: 'expand:aux',
    decision: 'merge_into_existing',
    board_label: '',
    description: '挂载进美债',
    target_board_id: 42,
    target_board_label: '美债',
    auxiliary_label_ids: [10],
    auxiliary_labels: [{ id: 10, label: '美债拍卖' }],
    confidence: 'llm',
    status: 'pending',
    created_at: '2026-09-01T00:00:00Z',
  }
}

function mountPanel(rows: UpgradeSuggestionRow[] = []) {
  return mount(UpgradeSuggestionPanel, {
    props: {
      visible: true,
      backfillNotice: false,
      persistedSuggestions: rows,
      persistedLoading: false,
      persistedGenerating: false,
      boards,
    },
    global: { stubs: { teleport: true } },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('UpgradeSuggestionPanel — 生成入口模式选择（split-board-upgrade-directions 6.4）', () => {
  it('默认 create×aux：直接生成 emit 正确四格参数（含 days）', async () => {
    const w = mountPanel()
    await flushPromises()
    await w.find('[data-testid="gen-submit"]').trigger('click')
    const events = w.emitted('generate')
    expect(events).toBeTruthy()
    expect(events![0]).toEqual([{ direction: 'create', source: 'aux', days: 1 }])
  })

  it('选「创建版块+组合标签」：不带 days（组合路无时间窗）', async () => {
    const w = mountPanel()
    await flushPromises()
    await w.find('[data-testid="gen-source-composite"]').setValue(true)
    await w.find('[data-testid="gen-submit"]').trigger('click')
    const events = w.emitted('generate')
    expect(events).toBeTruthy()
    expect(events![0]).toEqual([{ direction: 'create', source: 'composite' }])
  })

  it('扩充方向未选版块时生成禁用；选定后 emit 带 target_board_id', async () => {
    const w = mountPanel()
    await flushPromises()
    await w.find('[data-testid="gen-direction-expand"]').setValue(true)
    await flushPromises()
    expect((w.find('[data-testid="gen-submit"]').element as HTMLButtonElement).disabled).toBe(true)

    await w.find('[data-testid="gen-board-search"]').setValue('美债')
    await flushPromises()
    await w.find('[data-testid="gen-board-option-42"]').trigger('click')
    await flushPromises()
    expect((w.find('[data-testid="gen-submit"]').element as HTMLButtonElement).disabled).toBe(false)
    await w.find('[data-testid="gen-submit"]').trigger('click')
    const events = w.emitted('generate')
    expect(events).toBeTruthy()
    expect(events![events!.length - 1]).toEqual([{ direction: 'expand', source: 'aux', target_board_id: 42 }])
  })

  it('扩充×组合：emit 同样带 target（二分类 compose 携带目标）', async () => {
    const w = mountPanel()
    await flushPromises()
    await w.find('[data-testid="gen-direction-expand"]').setValue(true)
    await w.find('[data-testid="gen-source-composite"]').setValue(true)
    await w.find('[data-testid="gen-board-option-43"]').trigger('click')
    await w.find('[data-testid="gen-submit"]').trigger('click')
    const events = w.emitted('generate')
    expect(events).toBeTruthy()
    expect(events![0]).toEqual([{ direction: 'expand', source: 'composite', target_board_id: 43 }])
  })

  it('watch 过滤 tab 不存在（观察池退役）', async () => {
    const w = mountPanel()
    await flushPromises()
    const tabs = w.findAll('.usp-filter-tab').map(t => t.text())
    expect(tabs).toContain('组合')
    expect(tabs).not.toContain('观察池')
  })

  it('旧内存探索区不存在：无候选列表/簇/「获取 LLM 建议」', async () => {
    const w = mountPanel()
    await flushPromises()
    expect(w.find('.usp-manual-title').exists()).toBe(false)
    expect(w.text()).not.toContain('获取 LLM 建议')
    expect(w.text()).not.toContain('重新分析中')
  })

  it('扩充 merge 建议卡片：锁定版块徽标 + 确认合并按钮；无「合并到...」下拉', async () => {
    const w = mountPanel([mergeRow()])
    await flushPromises()
    const row = w.find('[data-decision="merge_into_existing"]')
    expect(row.exists()).toBe(true)
    const badge = row.find('[data-testid="row-target-badge"]')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toContain('美债')
    expect(row.text()).not.toContain('合并到...')
    const confirm = row.find('[data-testid="merge-confirm"]')
    expect(confirm.exists()).toBe(true)
    expect(confirm.text()).toContain('「美债」')
    await confirm.trigger('click')
    const events = w.emitted('confirmRow')
    expect(events).toBeTruthy()
    expect(events![0]![0]).toMatchObject({ decision: 'merge_into_existing', target_board_id: 42 })
  })

  it('生成错误行内提示（gen-error）', async () => {
    const w = mountPanel()
    await w.setProps({ generateError: 'target board 99 not found' })
    await flushPromises()
    const err = w.find('[data-testid="gen-error"]')
    expect(err.exists()).toBe(true)
    expect(err.text()).toContain('target board 99 not found')
  })

  it('空态区分：未生成过显示引导；hasGenerated 后显示无建议', async () => {
    const w = mountPanel()
    await flushPromises()
    expect(w.find('.usp-persisted-empty').text()).toContain('选择方向与来源后生成建议')
    await w.find('[data-testid="gen-submit"]').trigger('click')
    await flushPromises()
    expect(w.find('.usp-persisted-empty').text()).toContain('本轮无建议')
  })
})

/**
 * 弹窗浮层展示锚（split-board-upgrade-directions 补修）：
 * 重写 <style scoped> 时误删 .usp-overlay 定位规则，弹窗变成流内 div 追加到页面下方。
 * vitest 未启用 css:true，SFC 样式不注入 DOM，getComputedStyle 测不到 —— 改用两层锚：
 *  1) 源码规则锚：样式表里 .usp-overlay 必须含 fixed/inset/z-index（防“样式定义被删”）
 *  2) Teleport 挂载锚：overlay 必须挂在 body 而非组件原地（防“浮层挂载点回退”）
 * 弹窗/浮层类组件展示合理性的机械锚模板，详见 skill ui-verify 与 standard/frontend/layout.md。
 */
describe('UpgradeSuggestionPanel — 弹窗浮层展示锚（split-board-upgrade-directions 补修）', () => {
  it('样式表含 .usp-overlay 定位规则：position:fixed + inset:0 + z-index（防重写 style 时误删）', () => {
    const src = readFileSync(`${import.meta.dirname}/UpgradeSuggestionPanel.vue`, 'utf8')
    const style = src.slice(src.indexOf('<style'))
    expect(style).toMatch(/\.usp-overlay\s*\{[^}]*position:\s*fixed/s)
    expect(style).toMatch(/\.usp-overlay\s*\{[^}]*inset:\s*0/s)
    expect(style).toMatch(/\.usp-overlay\s*\{[^}]*z-index/s)
  })

  it('overlay 渲染挂载到 body（Teleport 生效，非流内 div）', async () => {
    const w = mount(UpgradeSuggestionPanel, {
      props: {
        visible: true,
        backfillNotice: false,
        persistedSuggestions: [],
        persistedLoading: false,
        persistedGenerating: false,
        boards,
      },
    })
    await flushPromises()
    const overlay = document.body.querySelector('.usp-overlay')
    expect(overlay).toBeTruthy()
    expect(w.element.contains(overlay)).toBe(false)
    w.unmount()
    expect(document.body.querySelector('.usp-overlay')).toBeNull()
  })
})
