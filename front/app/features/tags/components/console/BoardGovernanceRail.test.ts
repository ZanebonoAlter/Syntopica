import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BoardGovernanceRail from './BoardGovernanceRail.vue'
import type { BoardHealthSnapshotDTO, SemanticBoard } from '~/api/semanticBoards'
import type { BoardSourceBreakdown } from '~/types'

/**
 * 治理右轨（3.1/3.3）：体检信号 + 候选 top3 + 来源 top3 + 入口；
 * 右轨失败不阻塞（局部 retry）；来源口径与全量面板同源（useBoardSources）。
 */
const mounted: Array<{ unmount: () => void }> = []

const { apiMock } = vi.hoisted(() => ({
  apiMock: { getBoardSourceBreakdown: vi.fn() },
}))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return { ...original, useSemanticBoardsApi: () => apiMock }
})
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

function snapshot(): BoardHealthSnapshotDTO {
  return {
    id: 1,
    semantic_board_id: 1974,
    checked_at: '2026-10-23T23:00:00+08:00',
    metrics: {
      board: { matched_tags: 128, reason_dist: {}, ghost_matches: 5, vector_missing: 0 },
      labels: [],
    },
    uncaptured_candidates: [
      { aux_id: 1, label: '沙特求援美拒打击胡塞武装', member_tags: 27, member_articles: 22 },
      { aux_id: 2, label: '胡塞武装', member_tags: 25, member_articles: 26 },
      { aux_id: 3, label: '特朗普', member_tags: 15, member_articles: 16 },
    ],
    llm_score: 85,
    llm_comment: '核心标签贴合度良好。',
    created_at: '',
  }
}

function breakdown(): BoardSourceBreakdown {
  return {
    total_articles: 65,
    source_count: 4,
    sources: [
      { feed_id: 11, title: '美国新闻', articles: 10, share: 10 / 65, feed_articles: 50, feed_hit_rate: 0.2 },
      { feed_id: 12, title: '沙特通讯社', articles: 30, share: 30 / 65, feed_articles: 60, feed_hit_rate: 0.5 },
      { feed_id: 13, title: '伊朗媒体', articles: 20, share: 20 / 65, feed_articles: 40, feed_hit_rate: 0.5 },
      { feed_id: 14, title: '半岛电视台', articles: 5, share: 5 / 65, feed_articles: 25, feed_hit_rate: 0.2 },
    ],
  }
}

function mountRail(overrides: Record<string, unknown> = {}) {
  const onOpenDrawer = vi.fn()
  const onOpenWatch = vi.fn()
  const onOpenResearch = vi.fn()
  const onCapture = vi.fn()
  const onRetry = vi.fn()
  const wrapper = mount(BoardGovernanceRail, {
    props: {
      boardId: BOARD.id,
      boardLabel: BOARD.label,
      snapshot: snapshot(),
      loading: false,
      error: null,
      score: 85,
      tier: 'good',
      checkedAtText: '10/23 23:00',
      trendPolyline: '0,10 100,4',
      trendSummary: '7 轮 · 当前 85（+23）',
      capturingId: null,
      watchCount: 3,
      onOpenDrawer,
      onOpenWatch,
      onOpenResearch,
      onCapture,
      onRetry,
      ...overrides,
    },
    attachTo: document.body,
  })
  mounted.push(wrapper)
  return { wrapper, onOpenDrawer, onOpenWatch, onOpenResearch, onCapture, onRetry }
}

beforeEach(() => {
  apiMock.getBoardSourceBreakdown.mockReset()
  apiMock.getBoardSourceBreakdown.mockResolvedValue({ success: true, data: breakdown() })
})

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
  document.body.innerHTML = ''
  document.body.style.overflow = ''
})

describe('BoardGovernanceRail — 信号与来源', () => {
  it('渲染体检分、幽灵匹配与候选 top3', async () => {
    const { wrapper } = mountRail()
    await flushPromises()
    expect(wrapper.find('[data-testid="board-health-score"]').text()).toBe('85')
    expect(wrapper.text()).toContain('幽灵匹配')
    const candidates = wrapper.findAll('[data-testid^="board-health-candidate-"]')
    expect(candidates).toHaveLength(3)
  })

  it('来源 top3 按篇数降序，显示占比条与总计', async () => {
    const { wrapper } = mountRail()
    await flushPromises()
    const rows = wrapper.findAll('[data-testid^="gov-source-row-"]')
    expect(rows.map((row) => row.attributes('data-testid'))).toEqual([
      'gov-source-row-12', // 30 篇
      'gov-source-row-13', // 20 篇
      'gov-source-row-11', // 10 篇
    ])
    expect(rows[0]!.text()).toContain('沙特通讯社')
    expect(rows[0]!.text()).toContain('46%')
    expect(wrapper.text()).toContain('近 7 天 65 篇 · 4 个来源')
    expect(apiMock.getBoardSourceBreakdown).toHaveBeenCalledWith(1974, 7)
  })

  it('来源加载失败只影响本块，重试按同窗口重取', async () => {
    apiMock.getBoardSourceBreakdown
      .mockResolvedValueOnce({ success: false, error: '后端 500' })
      .mockResolvedValueOnce({ success: true, data: breakdown() })
    const { wrapper } = mountRail()
    await flushPromises()
    expect(wrapper.find('[data-testid="gov-sources-error"]').text()).toContain('后端 500')
    // 体检信号不受影响
    expect(wrapper.find('[data-testid="board-health-score"]').exists()).toBe(true)

    await wrapper.find('[data-testid="gov-sources-retry"]').trigger('click')
    await flushPromises()
    expect(apiMock.getBoardSourceBreakdown).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('[data-testid^="gov-source-row-"]')).toHaveLength(3)
  })
})

describe('BoardGovernanceRail — 入口与动作', () => {
  it('来源区块头 / 查看全部治理 / 治理抽屉入口都打开对应页签', async () => {
    const { wrapper, onOpenDrawer, onOpenResearch } = mountRail()
    await flushPromises()

    await wrapper.find('.gvr-block-head').trigger('click')
    expect(onOpenDrawer).toHaveBeenLastCalledWith('sources')

    await wrapper.find('[data-testid="gov-entry-drawer"]').trigger('click')
    expect(onOpenDrawer).toHaveBeenLastCalledWith('quality')

    await wrapper.find('[data-testid="gov-entry-research"]').trigger('click')
    expect(onOpenResearch).toHaveBeenCalledTimes(1)

    await wrapper.find('[data-testid="gov-entry-settings"]').trigger('click')
    expect(onOpenDrawer).toHaveBeenLastCalledWith('settings')

    await wrapper.find('[data-testid="gov-entry-watch"]').trigger('click')
    expect(wrapper.text()).toContain('我在追踪')
  })

  it('追踪入口发 openWatch、候选收编发 capture', async () => {
    const { wrapper, onOpenWatch, onCapture } = mountRail()
    await flushPromises()

    await wrapper.find('[data-testid="gov-entry-watch"]').trigger('click')
    expect(onOpenWatch).toHaveBeenCalledTimes(1)

    await wrapper.find('[data-testid="board-health-capture-2"]').trigger('click')
    expect(onCapture).toHaveBeenCalledWith(
      expect.objectContaining({ aux_id: 2, label: '胡塞武装' }),
    )
  })
})
