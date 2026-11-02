import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BoardGovernanceDrawer from './BoardGovernanceDrawer.vue'
import type {
  AuxiliaryLabelItem,
  BoardHealthSnapshotDTO,
  SemanticBoard,
} from '~/api/semanticBoards'
import type { BoardSourceBreakdown } from '~/types'

/**
 * 治理抽屉（3.2）：五页签编排 + mode 拆分复用 BoardHealthDetail + 来源全量 + 设置；
 * 构成加载失败只影响抽屉内容（局部错误 + 重试）。
 */
const mounted: Array<{ unmount: () => void }> = []

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    getBoardSourceBreakdown: vi.fn(),
    updateBoard: vi.fn(),
    removeFromComposition: vi.fn(),
  },
}))
const notifyMocks = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return { ...original, useSemanticBoardsApi: () => apiMock }
})
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => notifyMocks,
}))
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span />' },
}))

const BOARD: SemanticBoard = {
  id: 1974,
  label: '中东地缘政治与美伊关系',
  slug: 'middle-east',
  aliases: [],
  ref_count: 0,
  tag_count: 120,
  description: '',
  display_order: 0,
  source: 'manual',
  status: 'active',
  protected: false,
  enrichment_enabled: false,
  window_days: 14,
  context_layers: ['week', 'month', 'year', 'all'],
  created_at: '',
  updated_at: '',
}

function label(id: number, text: string): AuxiliaryLabelItem {
  return {
    id,
    label: text,
    slug: `l-${id}`,
    aliases: [],
    ref_count: 3,
    description: '',
    display_order: 0,
    source: 'manual',
    status: 'active',
    protected: false,
  }
}

function snapshot(): BoardHealthSnapshotDTO {
  return {
    id: 1,
    semantic_board_id: 1974,
    checked_at: '2026-10-23T23:00:00+08:00',
    metrics: {
      board: { matched_tags: 128, reason_dist: {}, ghost_matches: 5, vector_missing: 0 },
      labels: [
        {
          aux_id: 7,
          label: '美伊关系',
          samples: 40,
          cosine_mean: 0.62,
          cosine_median: 0.61,
          cosine_p25: 0.5,
          cosine_p75: 0.7,
          low_sim_ratio: 0.2,
          insufficient: false,
        },
      ],
    },
    uncaptured_candidates: [
      { aux_id: 2, label: '胡塞武装', member_tags: 25, member_articles: 26 },
    ],
    llm_score: 85,
    llm_comment: '',
    created_at: '',
  }
}

function breakdown(): BoardSourceBreakdown {
  return {
    total_articles: 65,
    source_count: 2,
    sources: [
      { feed_id: 12, title: '沙特通讯社', articles: 30, share: 30 / 65, feed_articles: 60, feed_hit_rate: 0.5 },
      { feed_id: 11, title: '美国新闻', articles: 10, share: 10 / 65, feed_articles: 50, feed_hit_rate: 0.2 },
    ],
  }
}

function mountDrawer(overrides: Record<string, unknown> = {}) {
  const onClose = vi.fn()
  const onUpdateTab = vi.fn()
  const onRemove = vi.fn()
  const onCapture = vi.fn()
  const onRefresh = vi.fn()
  const onSaved = vi.fn()
  const wrapper = mount(BoardGovernanceDrawer, {
    props: {
      open: true,
      boardId: BOARD.id,
      boardLabel: BOARD.label,
      board: BOARD,
      tab: 'quality',
      labels: [label(7, '美伊关系'), label(8, '霍尔木兹海峡')],
      composites: [{ id: 31, label: '美伊 × 霍尔木兹', slug: 'c-31', status: 'active', ref_count: 6, components: ['美伊关系', '霍尔木兹海峡'] }],
      loading: false,
      error: null,
      snapshot: snapshot(),
      capturingId: null,
      onClose,
      'onUpdate:tab': onUpdateTab,
      onRemove,
      onCapture,
      onRefresh,
      onSaved,
      ...overrides,
    },
    attachTo: document.body,
  })
  mounted.push(wrapper)
  return { wrapper, onClose, onUpdateTab, onRemove, onCapture, onRefresh, onSaved }
}

function bodyEl() {
  return document.body.querySelector('[data-testid="board-governance-drawer"]') as HTMLElement
}

beforeEach(() => {
  apiMock.getBoardSourceBreakdown.mockReset()
  apiMock.getBoardSourceBreakdown.mockResolvedValue({ success: true, data: breakdown() })
  apiMock.updateBoard.mockReset()
  apiMock.updateBoard.mockResolvedValue({ success: true, data: { id: 1974 } })
  notifyMocks.success.mockReset()
  notifyMocks.error.mockReset()
})

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
  document.body.innerHTML = ''
  document.body.style.overflow = ''
})

describe('BoardGovernanceDrawer — 页签与内容', () => {
  it('默认构成页签：只渲染质量表（组合/候选区不重复出现）', async () => {
    mountDrawer({ score: 85, checkedAtText: '10-24 03:00', trendSummary: '7 轮 · 62 → 85' })
    await flushPromises()
    expect(bodyEl()).not.toBeNull()
    expect(document.body.querySelector('[data-testid="gov-body-quality"]')).not.toBeNull()
    expect(document.body.querySelector('[data-testid="board-health-quality-table"]')).not.toBeNull()
    expect(document.body.querySelector('[data-testid="board-health-composite-section"]')).toBeNull()
    expect(document.body.querySelector('[data-testid="board-health-candidates"]')).toBeNull()
  })

  it('点击页签发 update:tab（五个页签齐全）', async () => {
    const { onUpdateTab } = mountDrawer()
    await flushPromises()
    const tabs = document.body.querySelectorAll('[data-testid^="gov-tab-"]')
    expect(tabs).toHaveLength(5)
    ;(document.body.querySelector('[data-testid="gov-tab-composites"]') as HTMLButtonElement).click()
    expect(onUpdateTab).toHaveBeenCalledWith('composites')
  })

  it('组合页签只渲染组合区，候选页签只渲染候选区并可收编', async () => {
    const { wrapper, onCapture } = mountDrawer({ tab: 'composites' })
    await flushPromises()
    expect(document.body.querySelector('[data-testid="board-health-composite-section"]')).not.toBeNull()
    expect(document.body.querySelector('[data-testid="board-health-quality-table"]')).toBeNull()

    await wrapper.setProps({ tab: 'candidates' })
    await flushPromises()
    expect(document.body.querySelector('[data-testid="board-health-candidates"]')).not.toBeNull()
    ;(document.body.querySelector('[data-testid="board-health-capture-2"]') as HTMLButtonElement).click()
    expect(onCapture).toHaveBeenCalledWith(expect.objectContaining({ aux_id: 2 }))
  })

  it('来源页签渲染全量来源表（窗口/排序在面板内）', async () => {
    mountDrawer({ tab: 'sources' })
    await flushPromises()
    expect(document.body.querySelector('[data-testid="board-source-panel"]')).not.toBeNull()
    expect(document.body.querySelectorAll('[data-testid="board-source-row"]')).toHaveLength(2)
    // spec 必列 5 列全在（不得为塞进 32rem 删列）
    const heads = [...document.body.querySelectorAll('.bsp-table thead th')].map((th) => th.textContent?.trim())
    expect(heads).toHaveLength(5)
    expect(heads.some((head) => head?.includes('该源窗口内总量'))).toBe(true)
    expect(apiMock.getBoardSourceBreakdown).toHaveBeenCalledWith(1974, 7)
  })

  it('设置页签渲染版块设置表单，保存调 updateBoard 并上报 saved', async () => {
    const { onSaved } = mountDrawer({ tab: 'settings' })
    await flushPromises()
    expect(document.body.querySelector('[data-testid="gov-settings-form"]')).not.toBeNull()
    ;(document.body.querySelector('[data-testid="gov-settings-save"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(apiMock.updateBoard).toHaveBeenCalledWith(1974, expect.objectContaining({
      label: '中东地缘政治与美伊关系',
      window_days: 14,
    }))
    expect(onSaved).toHaveBeenCalledTimes(1)
  })

  it('构成加载失败 → 局部错误 + 重试（不阻塞页签切换）', async () => {
    const { onRefresh } = mountDrawer({ error: '构成加载失败' })
    await flushPromises()
    expect(document.body.querySelector('[data-testid="gov-composition-error"]')!.textContent).toContain('构成加载失败')
    ;(document.body.querySelector('.gvd-retry') as HTMLButtonElement).click()
    expect(onRefresh).toHaveBeenCalledTimes(1)
    // 错误不阻塞页签切换
    ;(document.body.querySelector('[data-testid="gov-tab-sources"]') as HTMLButtonElement).click()
    expect(document.body.querySelector('[data-testid="gov-tab-sources"]')).not.toBeNull()
  })
})
