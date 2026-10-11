/**
 * BoardHealthPanel — add-board-health-checkup test-cases 步 14b/14c/14d/14e
 * + 状态矩阵七态（loading/empty/error/success/insufficient/警告条上游/操作失败）。
 *
 * 组件层：体检四态独立于构成管理；评分徽章色档 class；质量表联查/待体检/数据不足；
 * 折叠长列表（质量 top10/候选 top5）；picker 双模式添加全流程（含手动指定路径）；
 * 行内移除 confirm；组合挂载移除；收编快捷动作（addComposition + toast + 刷新）。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BoardHealthPanel from './BoardHealthPanel.vue'
import type { BoardHealthCheckupResponse, BoardHealthSnapshotDTO } from '~/api/semanticBoards'

const { apiMock, notifySuccessMock } = vi.hoisted(() => ({
  apiMock: {
    getBoardHealthCheckup: vi.fn(),
    addComposition: vi.fn(),
    removeFromComposition: vi.fn(),
  },
  notifySuccessMock: vi.fn(),
}))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return { ...original, useSemanticBoardsApi: () => apiMock }
})
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ success: notifySuccessMock, error: vi.fn() }),
}))
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span />' },
}))
vi.mock('./AuxiliaryLabelPicker.vue', () => ({
  default: {
    name: 'AuxiliaryLabelPicker',
    props: ['mode', 'boardId', 'selectedIds'],
    emits: ['update:selectedIds'],
    template: `<div data-testid="alp-stub" :data-mode="mode" @click="$emit('update:selectedIds', [42])" />`,
  },
}))
vi.mock('./CompositeLabelEditDialog.vue', () => ({
  default: { name: 'CompositeLabelEditDialog', props: ['visible', 'boardId', 'boardLabel'], template: '<div data-testid="composite-dialog-stub" />' },
}))

function snapshotFixture(overrides: Partial<BoardHealthSnapshotDTO> = {}): BoardHealthSnapshotDTO {
  return {
    id: 1,
    semantic_board_id: 5,
    checked_at: '2026-10-02T08:00:00Z',
    metrics: {
      board: {
        matched_tags: 320,
        reason_dist: { direct_hit: 210, composite_hit: 30 },
        ghost_matches: 12,
        vector_missing: 3,
      },
      labels: [
        {
          aux_id: 101, label: '推理', samples: 18,
          cosine_mean: 0.81, cosine_median: 0.83, cosine_p25: 0.76, cosine_p75: 0.88,
          low_sim_ratio: 0.11, insufficient: false,
        },
        {
          aux_id: 102, label: 'AGI', samples: 1,
          cosine_mean: null, cosine_median: null, cosine_p25: null, cosine_p75: null,
          low_sim_ratio: null, insufficient: true,
        },
      ],
    },
    uncaptured_candidates: [
      { aux_id: 201, label: '算力', member_tags: 12, member_articles: 5 },
      { aux_id: 202, label: '半导体', member_tags: 9, member_articles: 4 },
      { aux_id: 203, label: '光模块', member_tags: 8, member_articles: 3 },
      { aux_id: 204, label: '数据中心', member_tags: 7, member_articles: 2 },
      { aux_id: 205, label: '液冷', member_tags: 6, member_articles: 2 },
      { aux_id: 206, label: '电力', member_tags: 5, member_articles: 1 },
      { aux_id: 207, label: '稀土', member_tags: 4, member_articles: 1 },
    ],
    llm_score: 78,
    llm_comment: '构成清晰，方向匹配整体健康。',
    created_at: '2026-10-02T08:00:01Z',
    ...overrides,
  }
}

function healthResponse(latest: BoardHealthSnapshotDTO | null): BoardHealthCheckupResponse {
  return { latest, trend: latest ? [
    { checked_at: '2026-10-01T08:00:00Z', llm_score: 70 },
    { checked_at: '2026-10-02T08:00:00Z', llm_score: 78 },
  ] : [] }
}

const LABELS = [
  { id: 101, label: '推理', slug: 'reasoning', aliases: [], ref_count: 10, description: '', display_order: 0, source: 'llm', status: 'active', protected: false },
  { id: 102, label: 'AGI', slug: 'agi', aliases: [], ref_count: 2, description: '', display_order: 0, source: 'llm', status: 'active', protected: false },
]

function mountPanel(props: Partial<InstanceType<typeof BoardHealthPanel>['$props']> = {}) {
  return mount(BoardHealthPanel, {
    props: {
      boardId: 5,
      labels: LABELS,
      loading: false,
      composites: [],
      boardLabel: 'AI 前沿',
      ...props,
    },
  })
}

beforeEach(() => {
  apiMock.getBoardHealthCheckup.mockReset()
  apiMock.addComposition.mockReset()
  apiMock.removeFromComposition.mockReset()
  notifySuccessMock.mockReset()
})

describe('BoardHealthPanel — 体检状态矩阵', () => {
  it('loading 态：骨架文案（14f 前置）', async () => {
    apiMock.getBoardHealthCheckup.mockReturnValue(new Promise(() => {})) // pending
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.find('[data-testid="board-health-loading"]').exists()).toBe(true)
  })

  it('empty 态（无快照）：等待首轮体检徽章 + 质量列待体检 + 构成管理照常（14b）', async () => {
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(null) })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.text()).toContain('等待首轮体检')
    // 质量列「待体检」
    const rows = wrapper.findAll('[data-testid="board-health-quality-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('待体检')
    // 构成管理照常：添加按钮可点、移除按钮存在
    expect(wrapper.find('[data-testid="board-health-add"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="board-health-remove-101"]').exists()).toBe(true)
    // 上报无快照
    expect(wrapper.emitted('healthLoaded')![0]).toEqual([{ hasSnapshot: false, score: null }])
  })

  it('error 态：错误文案 + 重试；构成管理不受影响', async () => {
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: false, error: '网络错误' })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.find('[data-testid="board-health-error"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('网络错误')
    expect(wrapper.find('[data-testid="board-health-quality-row"]').exists()).toBe(true)
    // 重试自愈
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(snapshotFixture()) })
    await wrapper.find('.bhp-retry').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="board-health-score"]').exists()).toBe(true)
  })

  it('success 态：徽章/评语/指标 chips/质量表/趋势/候选全量渲染', async () => {
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(snapshotFixture()) })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.find('[data-testid="board-health-score"]').text()).toBe('78')
    expect(wrapper.find('[data-testid="board-health-score"]').classes()).toContain('bhp-badge--good')
    expect(wrapper.find('[data-testid="board-health-comment"]').text()).toContain('构成清晰')
    const chips = wrapper.find('[data-testid="board-health-chips"]')
    expect(chips.text()).toContain('320')
    expect(chips.text()).toContain('幽灵匹配')
    expect(wrapper.find('[data-testid="board-health-trend"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid^="board-health-candidate-"]')).toHaveLength(5) // 候选默认折叠 top 5（展开在折叠用例断言）
    // 低分上报（警告条上游）
    expect(wrapper.emitted('healthLoaded')![0]).toEqual([{ hasSnapshot: true, score: 78 }])
  })

  it('低分徽章 low 档（警告条上游 score<60）', async () => {
    const low = snapshotFixture({ llm_score: 52, llm_comment: '方向漂移明显。' })
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(low) })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.find('[data-testid="board-health-score"]').classes()).toContain('bhp-badge--low')
    expect(wrapper.emitted('healthLoaded')![0]).toEqual([{ hasSnapshot: true, score: 52 }])
  })

  it('insufficient 行：数据不足 + 分布列 —（状态矩阵 insufficient）', async () => {
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(snapshotFixture()) })
    const wrapper = mountPanel()
    await flushPromises()
    const cells = wrapper.findAll('[data-testid="board-health-quality-row"]').map(r => r.findAll('td'))
    // 样本数降序：推理(18)在前，AGI(1)在后
    expect(cells[0]![1]!.text()).toBe('18')
    expect(cells[0]![2]!.text()).toBe('0.81')
    expect(cells[1]![1]!.text()).toBe('1')
    expect(cells[1]![2]!.text()).toBe('—')
    expect(cells[1]![4]!.text()).toContain('数据不足')
  })
})

describe('BoardHealthPanel — 构成管理平移（14d）', () => {
  it('行内移除：confirm 确认后 emit remove，取消不 emit（confirm 文案不变）', async () => {
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(snapshotFixture()) })
    const wrapper = mountPanel()
    await flushPromises()
    const confirmSpy = vi.spyOn(window, 'confirm')

    confirmSpy.mockReturnValue(false)
    await wrapper.find('[data-testid="board-health-remove-101"]').trigger('click')
    expect(confirmSpy).toHaveBeenCalledWith(expect.stringContaining('不会自动回填历史数据'))
    expect(wrapper.emitted('remove')).toBeUndefined()

    confirmSpy.mockReturnValue(true)
    await wrapper.find('[data-testid="board-health-remove-101"]').trigger('click')
    expect(wrapper.emitted('remove')![0]).toEqual([101])
    confirmSpy.mockRestore()
  })

  it('picker 添加全流程：版块推荐路径 + 确认调用 addComposition（14d）', async () => {
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(snapshotFixture()) })
    apiMock.addComposition.mockResolvedValue({ success: true, data: {} })
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.find('[data-testid="board-health-add"]').trigger('click')
    expect(wrapper.find('[data-testid="alp-stub"]').attributes('data-mode')).toBe('edit')
    // stub picker 回选 42 → 确认
    await wrapper.find('[data-testid="alp-stub"]').trigger('click')
    await wrapper.find('[data-testid="board-health-confirm-add"]').trigger('click')
    await flushPromises()
    expect(apiMock.addComposition).toHaveBeenCalledWith(5, 42)
    expect(wrapper.emitted('refresh')).toHaveLength(1)
    expect(wrapper.text()).toContain('历史标签归属不会自动回填')
  })

  it('picker 手动指定路径：切 create 模式后添加（14d）', async () => {
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(snapshotFixture()) })
    apiMock.addComposition.mockResolvedValue({ success: true, data: {} })
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.find('[data-testid="board-health-add"]').trigger('click')
    const manualBtn = wrapper.findAll('.bhp-mode-btn').find(b => b.text() === '手动指定')!
    await manualBtn.trigger('click')
    expect(wrapper.find('[data-testid="alp-stub"]').attributes('data-mode')).toBe('create')
    await wrapper.find('[data-testid="alp-stub"]').trigger('click')
    await wrapper.find('[data-testid="board-health-confirm-add"]').trigger('click')
    await flushPromises()
    expect(apiMock.addComposition).toHaveBeenCalledWith(5, 42)
  })

  it('添加失败：行内 notice 反馈（操作失败态）', async () => {
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(snapshotFixture()) })
    apiMock.addComposition.mockResolvedValue({ success: false, error: '已挂载' })
    const wrapper = mountPanel()
    await flushPromises()
    // 收编失败路径直接断 notice（addComposition fail）
    await wrapper.find('[data-testid="board-health-capture-201"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="board-health-notice"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('收编')
  })

  it('组合标签区：创建入口 + 移除挂载 confirm + removeFromComposition', async () => {
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(snapshotFixture()) })
    apiMock.removeFromComposition.mockResolvedValue({ success: true, data: {} })
    const wrapper = mountPanel({
      composites: [{ id: 301, label: '美债收益率', slug: 'us-yield', status: 'active', ref_count: 3, components: ['美国国债', '收益率'] }],
    })
    await flushPromises()
    expect(wrapper.find('[data-testid="board-composite-create"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="board-composite-item-301"]').exists()).toBe(true)

    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    await wrapper.find('[data-testid="board-composite-item-301"]').find('.bhp-remove').trigger('click')
    await flushPromises()
    expect(confirmSpy).toHaveBeenCalledWith(expect.stringContaining('不会自动回填历史归属'))
    expect(apiMock.removeFromComposition).toHaveBeenCalledWith(5, 301)
    expect(wrapper.emitted('refresh')).toHaveLength(1)
    confirmSpy.mockRestore()
  })
})

describe('BoardHealthPanel — 未收编候选收编（14c）', () => {
  it('一键收编：addComposition + toast + 本地剔除 + refresh，不走 LLM 管道', async () => {
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(snapshotFixture()) })
    apiMock.addComposition.mockResolvedValue({ success: true, data: {} })
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.find('[data-testid="board-health-capture-201"]').trigger('click')
    await flushPromises()
    expect(apiMock.addComposition).toHaveBeenCalledWith(5, 201)
    expect(notifySuccessMock).toHaveBeenCalledWith(expect.stringContaining('已收编「算力」'))
    // 本地剔除：201 从候选列表消失
    expect(wrapper.find('[data-testid="board-health-candidate-201"]').exists()).toBe(false)
    expect(wrapper.emitted('refresh')).toHaveLength(1)
  })

  it('收编防重复：请求中其它收编按钮 disabled', async () => {
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(snapshotFixture()) })
    apiMock.addComposition.mockReturnValue(new Promise(() => {})) // pending
    const wrapper = mountPanel()
    await flushPromises()

    await wrapper.find('[data-testid="board-health-capture-201"]').trigger('click')
    await flushPromises()
    const other = wrapper.find('[data-testid="board-health-capture-202"]')
    expect(other.attributes('disabled')).toBeDefined()
  })
})

describe('BoardHealthPanel — 长列表折叠（14e）', () => {
  it('质量表默认 top 10 + 展开全部切换', async () => {
    const many = Array.from({ length: 12 }, (_, i) => ({
      id: 1000 + i, label: `标签${i}`, slug: `l${i}`, aliases: [], ref_count: 1,
      description: '', display_order: 0, source: 'llm', status: 'active', protected: false,
    }))
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(snapshotFixture()) })
    const wrapper = mountPanel({ labels: many })
    await flushPromises()

    expect(wrapper.findAll('[data-testid="board-health-quality-row"]')).toHaveLength(10)
    const fold = wrapper.find('[data-testid="board-health-labels-fold"]')
    expect(fold.text()).toContain('展开全部 (12)')
    await fold.trigger('click')
    expect(wrapper.findAll('[data-testid="board-health-quality-row"]')).toHaveLength(12)
  })

  it('未收编候选默认 top 5 + 展开全部切换', async () => {
    apiMock.getBoardHealthCheckup.mockResolvedValue({ success: true, data: healthResponse(snapshotFixture()) })
    const wrapper = mountPanel()
    await flushPromises()

    expect(wrapper.findAll('[data-testid^="board-health-candidate-"]')).toHaveLength(5)
    const fold = wrapper.find('[data-testid="board-health-candidates-fold"]')
    expect(fold.text()).toContain('展开全部 (7)')
    await fold.trigger('click')
    expect(wrapper.findAll('[data-testid^="board-health-candidate-"]')).toHaveLength(7)
  })
})
