import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UpgradeSuggestionPanel from './UpgradeSuggestionPanel.vue'
import type { UpgradeSuggestionRow, SemanticBoard } from '~/api/semanticBoards'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function composeRow(): UpgradeSuggestionRow {
  return {
    id: 42,
    batch_id: 'b1',
    mode: '',
    decision: 'compose',
    board_label: '美债收益率',
    description: '美国国债与收益率的组合',
    auxiliary_label_ids: [10, 11],
    auxiliary_labels: [
      { id: 10, label: '美国国债' },
      { id: 11, label: '收益率' },
    ],
    confidence: 'llm',
    evidence: {
      compose_cooccurrence: 15,
      compose_window_days: 30,
      compose_representative_titles: ['美债收益率破5%', '国债拍卖遇冷'],
    },
    status: 'pending',
    created_at: '2026-09-01T00:00:00Z',
  }
}

const boards: SemanticBoard[] = []

function mountPanel(rows: UpgradeSuggestionRow[]) {
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

describe('UpgradeSuggestionPanel — compose 建议（add-composite-labels 6.2）', () => {
  it('决策过滤 tab 含「组合」，切到组合 tab emit loadPersisted("compose")', async () => {
    const w = mountPanel([])
    await flushPromises()
    const tab = w.findAll('.usp-filter-tab').find(t => t.text() === '组合')
    expect(tab).toBeTruthy()
    await tab!.trigger('click')
    const events = w.emitted('loadPersisted')
    expect(events).toBeTruthy()
    expect(events![events!.length - 1]!).toEqual(['compose'])
  })

  it('compose 卡片渲染：组合名、组件序列、共现证据（频次+窗口+代表事件）', async () => {
    const w = mountPanel([composeRow()])
    await flushPromises()
    const row = w.find('[data-decision="compose"]')
    expect(row.exists()).toBe(true)
    expect(row.text()).toContain('创建组合标签')
    expect(row.text()).toContain('美债收益率')
    expect(row.text()).toContain('美国国债')
    expect(row.text()).toContain('收益率')
    // 共现证据
    const evidence = w.find('[data-testid="compose-evidence"]')
    expect(evidence.exists()).toBe(true)
    expect(w.find('[data-testid="compose-cooccurrence"]').text()).toContain('共现 15 篇')
    expect(w.find('[data-testid="compose-cooccurrence"]').text()).toContain('30 天窗口')
    expect(evidence.text()).toContain('美债收益率破5%')
    // 确认按钮存在
    expect(w.find('[data-testid="compose-confirm"]').exists()).toBe(true)
  })

  it('确认 compose 建议 emit confirmRow（decision=compose，组件带勾选子集）', async () => {
    const w = mountPanel([composeRow()])
    await flushPromises()
    await w.find('[data-testid="compose-confirm"]').trigger('click')
    const events = w.emitted('confirmRow')
    expect(events).toBeTruthy()
    const payload = events![0]![0] as UpgradeSuggestionRow & { decision: string }
    expect(payload.decision).toBe('compose')
    expect(payload.auxiliary_label_ids).toEqual([10, 11])
    expect(payload.id).toBe(42)
  })

  it('compose 行不显示「合并到...」入口（组合不进版块合并流程）', async () => {
    const w = mountPanel([composeRow()])
    await flushPromises()
    expect(w.text()).not.toContain('合并到...')
  })

  it('空态：无 compose 建议时不渲染 compose 卡片', async () => {
    const w = mountPanel([])
    await flushPromises()
    expect(w.find('[data-decision="compose"]').exists()).toBe(false)
    // 空态文案随四格入口改版（split-board-upgrade-directions）：未生成过显示引导。
    expect(w.find('.usp-persisted-empty').text()).toContain('选择方向与来源后生成建议')
  })
})
