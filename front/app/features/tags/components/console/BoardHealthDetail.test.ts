/**
 * 治理明细区 · 未收编候选（simplify-workshop-matrix-copy）：
 * 「历史归属不自动回填」这句从右轨常驻注脚移回**实际收编处**（候选清单上方），
 * 候选为空时不写这句（避免无对象时的空说明）。其余收编动作语义不变。
 */
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { BoardHealthSnapshotDTO } from '~/api/semanticBoards'
import BoardHealthDetail from './BoardHealthDetail.vue'

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return {
    ...original,
    useSemanticBoardsApi: () => ({
      addComposition: vi.fn(),
      removeFromComposition: vi.fn(),
    }),
  }
})

vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ success: vi.fn(), error: vi.fn(), warn: vi.fn() }),
}))

vi.mock('@iconify/vue', () => ({ Icon: { name: 'Icon', template: '<span />' } }))

function snapshot(candidates: number): BoardHealthSnapshotDTO {
  return {
    id: 1,
    semantic_board_id: 1974,
    checked_at: '2026-10-23T23:00:00+08:00',
    metrics: { board: { matched_tags: 128, reason_dist: {}, ghost_matches: 0, vector_missing: 0 }, labels: [] },
    uncaptured_candidates: Array.from({ length: candidates }, (_, index) => ({
      aux_id: index + 1,
      label: `候选标签 ${index + 1}`,
      member_tags: 20 - index,
      member_articles: 30 - index,
    })),
    llm_score: 85,
    llm_comment: '',
    created_at: '',
  }
}

function mountDetail(overrides: Record<string, unknown> = {}) {
  return mount(BoardHealthDetail, {
    props: {
      boardId: 1974,
      labels: [],
      loading: false,
      snapshot: snapshot(2),
      composites: [],
      mode: 'candidates',
      ...overrides,
    },
  })
}

describe('未收编候选 · 实际收编处说明', () => {
  it('有候选：收编处在清单旁写明只挂本版块、历史归属不自动回填', () => {
    const wrapper = mountDetail()
    const note = wrapper.find('[data-testid="board-health-candidates-note"]')
    expect(note.exists()).toBe(true)
    expect(note.text()).toContain('历史归属不自动回填')
    expect(note.text()).toContain('只挂到本版块构成')
  })

  it('无候选：不写回填说明；收编按钮按候选逐条渲染并外抛', async () => {
    const empty = mountDetail({ snapshot: snapshot(0) })
    expect(empty.find('[data-testid="board-health-candidates-note"]').exists()).toBe(false)

    const wrapper = mountDetail()
    expect(wrapper.findAll('[data-testid^="board-health-capture-"]')).toHaveLength(2)
    await wrapper.find('[data-testid="board-health-capture-2"]').trigger('click')
    expect(wrapper.emitted('capture')).toHaveLength(1)
    expect((wrapper.emitted('capture')![0] as [{ aux_id: number }])[0].aux_id).toBe(2)
  })

  it('非候选页签（quality）不渲染该说明', () => {
    const wrapper = mountDetail({ mode: 'quality' })
    expect(wrapper.find('[data-testid="board-health-candidates-note"]').exists()).toBe(false)
  })
})
