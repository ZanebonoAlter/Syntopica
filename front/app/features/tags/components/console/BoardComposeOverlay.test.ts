import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import BoardComposeOverlay from './BoardComposeOverlay.vue'
import { estimateTextWidth } from '~/features/tags/composables/laneComposeLogic'
import type { ComposeCandidatesData } from '~/api/persistentTopics'

/**
 * 编排态组件用例（白盒分支表 §3.5 V1–V16 + delta spec 场景）。
 * 判据来自主线程 2026-10-31 定稿的 test-cases-lane-compose-implementation.md。
 */

const { persistentMock } = vi.hoisted(() => ({
  persistentMock: {
    getComposeCandidates: vi.fn(),
    createManualLane: vi.fn(),
    embedQuery: vi.fn(),
  },
}))

const { reportsMock } = vi.hoisted(() => ({
  reportsMock: {
    listBoardTopics: vi.fn(),
    updateTopic: vi.fn(),
  },
}))

const { notifyMock } = vi.hoisted(() => ({
  notifyMock: { success: vi.fn(), error: vi.fn(), warn: vi.fn() },
}))

vi.mock('~/api/persistentTopics', () => ({ usePersistentTopicsApi: () => persistentMock }))
vi.mock('~/api/dailyReports', () => ({ useDailyReportsApi: () => reportsMock }))
vi.mock('~/composables/useNotify', () => ({ useNotify: () => notifyMock }))
const { narrowState } = vi.hoisted(() => ({ narrowState: { value: false } }))
vi.mock('~/composables/useMediaQuery', () => ({ useIsNarrowViewport: () => narrowState }))

function section(id: string, periodDate: string, clusterLabel: string, embedding: number[], topicId?: string, status?: string) {
  return {
    id,
    reportId: '1',
    periodDate,
    clusterLabel,
    embedding,
    persistentTopicId: topicId,
    persistentTopic: topicId ? { id: topicId, label: topicId === '1984' ? '候选泳道 A' : '已有泳道', status: status ?? 'candidate' } : undefined,
  }
}

const SECTIONS = [
  section('1', '2026-10-27', '候选一', [1, 0], '1984'),
  section('2', '2026-10-28', '候选二', [0.99, 0.1], '1984'),
  section('3', '2026-10-30', '候选三', [0.98, 0.2], '1984'),
  section('4', '2026-10-26', '散料近邻', [0.97, 0.15]),
  section('5', '2026-10-27', '已有泳道', [1, 0], '2000', 'active'),
  section('6', '2026-10-29', '归档来源事件', [0.5, 0.5], '3000', 'archived'),
]

const CHAIN = {
  key: '1984:1',
  topicId: '1984',
  topicLabel: '候选泳道 A',
  topicStatus: 'candidate',
  sectionIds: ['1', '2', '3'],
  firstDate: '2026-10-27',
  lastDate: '2026-10-30',
  spanDays: 3,
}

const RELATIONS = [
  { fromId: '1', toId: '2', distance: 0.1, relationType: 'identity' },
  { fromId: '2', toId: '3', distance: 0.12, relationType: 'identity' },
]

const WINDOW = { from: '2026-10-20', to: '2026-10-30', days: 11, latestCompletedDate: '2026-10-30' }

/** 服务端管理名单（snake_case 原样；可见性已由服务端按累计命中门槛过滤）。 */
const HISTORY_TOPICS = [
  {
    id: 1984,
    semantic_board_id: 1974,
    label: '候选泳道 A',
    description: '',
    status: 'candidate',
    first_seen_date: '2026-10-01',
    last_seen_date: '2026-10-30',
    hit_count: 5,
    consecutive_hits: 5,
    section_count: 3,
    color: '#abc',
    can_activate: true,
  },
  {
    id: 1985,
    semantic_board_id: 1974,
    label: '未达门槛话题',
    description: '',
    status: 'candidate',
    first_seen_date: '2026-10-05',
    last_seen_date: '2026-10-12',
    hit_count: 1,
    consecutive_hits: 1,
    section_count: 1,
    color: '#def',
    can_activate: false,
  },
]

function response(overrides: Partial<ComposeCandidatesData> = {}): ComposeCandidatesData {
  return {
    sections: SECTIONS,
    relations: RELATIONS,
    candidateChains: [CHAIN],
    matchThreshold: 0.3,
    window: WINDOW,
    ...overrides,
  }
}

let wrapper: ReturnType<typeof mount> | null = null

async function mountOverlay() {
  wrapper = mount(BoardComposeOverlay, {
    props: { boardId: 1974, boardLabel: '测试版块' },
    attachTo: document.body,
    global: { stubs: { teleport: true } },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}

/** 模拟容器真实宽度（画布 1:1 的前提）。 */
async function setCanvasWidth(w: number): Promise<void> {
  const el = wrapper!.find('[data-testid="lane-compose-canvas"]').element as HTMLElement
  Object.defineProperty(el, 'clientWidth', { value: w, configurable: true })
  window.dispatchEvent(new Event('resize'))
  await flushPromises()
}

/** 走右栏「按标题找」人工补选一个组外事件。 */
async function addManualPick(w: ReturnType<typeof mount>, id: string): Promise<void> {
  await w.find('[data-testid="lane-compose-tab-find"]').trigger('click')
  await flushPromises()
  await w.find(`[data-testid="lane-find-add-${id}"]`).trigger('click')
  await flushPromises()
}

beforeEach(() => {
  narrowState.value = false
  document.body.innerHTML = ''
  notifyMock.success.mockReset()
  notifyMock.error.mockReset()
  notifyMock.warn.mockReset()
  persistentMock.getComposeCandidates.mockReset().mockResolvedValue({ success: true, data: response() })
  persistentMock.createManualLane.mockReset().mockResolvedValue({
    success: true,
    data: { topic: { id: '300', label: '新泳道', status: 'active', source: 'manual' }, skipped: [] },
  })
  persistentMock.embedQuery.mockReset().mockResolvedValue({ success: true, data: { embedding: [1, 0] } })
  reportsMock.listBoardTopics.mockReset().mockResolvedValue({ success: true, data: { topics: HISTORY_TOPICS } })
  reportsMock.updateTopic.mockReset().mockResolvedValue({
    success: true,
    data: { id: 1984, semantic_board_id: 1974, label: '候选泳道 A', description: '', status: 'active', first_seen_date: '2026-10-01', last_seen_date: '2026-10-30', hit_count: 5, consecutive_hits: 5 },
  })
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('BoardComposeOverlay — 编排态入口与三栏（S1 / V16）', () => {
  it('进入编排态：左候选、中央画布、右栏三段都在；已有 active 泳道默认隐藏；无默认勾选', async () => {
    const w = await mountOverlay()
    expect(w.find('[data-testid="lane-compose-candidates"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-canvas"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-inspector"]').exists()).toBe(true)
    // 已有 active 泳道默认隐藏（点不画）
    expect(w.find('[data-testid="lane-node-5"]').exists()).toBe(false)
    // 无默认勾选
    expect(w.findAll('input[type="checkbox"]:checked')).toHaveLength(0)
    expect(w.find('[data-testid="compose-save"]').attributes('disabled')).toBeDefined()
  })

  it('V16 窄屏：只提示宽屏/横屏，不画工作台也不取数', async () => {
    narrowState.value = true
    const w = await mountOverlay()
    expect(w.find('[data-testid="board-compose-narrow"]').exists()).toBe(true)
    expect(w.find('[data-testid="board-compose"]').exists()).toBe(false)
    expect(persistentMock.getComposeCandidates).not.toHaveBeenCalled()
    expect(w.emitted('close')).toBeUndefined()
    narrowState.value = false
  })

  it('空窗口：候选栏与画布各有空态，保存不可用', async () => {
    persistentMock.getComposeCandidates.mockResolvedValue({
      success: true,
      data: response({ sections: [], relations: [], candidateChains: [] }),
    })
    const w = await mountOverlay()
    expect(w.find('[data-testid="lane-compose-candidates-empty"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-canvas-empty"]').exists()).toBe(true)
    expect(w.find('[data-testid="compose-save"]').attributes('disabled')).toBeDefined()
  })

  it('候选栏：每条候选展示原名、真实成员、首末日期、事件数与连接跨度', async () => {
    const w = await mountOverlay()
    const card = w.find('[data-testid="lane-candidate-1984:1"]')
    expect(card.exists()).toBe(true)
    expect(card.text()).toContain('候选泳道 A')
    expect(card.text()).toContain('3 个事件')
    expect(card.text()).toContain('连接跨度 3 天')
    expect(card.text()).toContain('10-27')
    expect(card.text()).toContain('10-30')
    expect(card.findAll('input[type="checkbox"]')).toHaveLength(4) // 1 组框 + 3 成员框
  })

  it('原生复选框三态：全选 / 半选 / 未选与成员计数同源', async () => {
    const w = await mountOverlay()
    const group = () => w.find('[data-testid="lane-group-1984:1"]').element as HTMLInputElement
    const member = () => w.find('[data-testid="lane-member-2"]').element as HTMLInputElement
    expect(group().checked).toBe(false)
    expect(group().indeterminate).toBe(false)

    await member().click()
    await flushPromises()
    expect(group().checked).toBe(false)
    expect(group().indeterminate).toBe(true)
    expect(w.find('[data-testid="lane-state-1984:1"]').text()).toContain('1 / 3 已选')

    await group().click()
    await flushPromises()
    expect(group().checked).toBe(true)
    expect(group().indeterminate).toBe(false)
    expect(w.find('[data-testid="lane-state-1984:1"]').text()).toContain('3 / 3 已选')
    expect(w.find('[data-testid="compose-save"]').attributes('disabled')).toBeUndefined()
  })

  it('点候选标题一次补齐全部成员；再点全选标题取消该组，组外人工补选保留', async () => {
    const w = await mountOverlay()
    await addManualPick(w, '4')
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    expect(w.findAll('[data-testid="lane-node-1"], [data-testid="lane-node-2"], [data-testid="lane-node-3"]')).toHaveLength(3)
    expect((w.find('[data-testid="lane-member-1"]').element as HTMLInputElement).checked).toBe(true)

    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    expect((w.find('[data-testid="lane-member-1"]').element as HTMLInputElement).checked).toBe(false)
    // 组外人工补选保留
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('1')
  })

  it('取消整组：撤销该组全部成员并保留组外选择', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    await addManualPick(w, '4')
    await w.find('[data-testid="lane-cancel-1984:1"]').trigger('click')
    await flushPromises()
    expect((w.find('[data-testid="lane-member-1"]').element as HTMLInputElement).checked).toBe(false)
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('1')
  })
})

describe('BoardComposeOverlay — 画布（V1–V7、V14、V15）', () => {
  it('V1/V2 画布按容器真实像素 1:1；宽度变化后重排重绘', async () => {
    const w = await mountOverlay()
    // 列宽锁 124 下限后，内容宽常大于容器宽 → viewBox 取内容宽（靠横向滚动），
    // 但 SVG 的 width 属性 = viewBox 宽 × 缩放，渲染仍 1:1（不缩放内容）。
    await setCanvasWidth(900)
    const vb900 = Number(w.find('[data-testid="lane-compose-canvas-svg"]').attributes('viewBox')!.split(' ')[2])
    expect(vb900).toBeGreaterThanOrEqual(900)
    await setCanvasWidth(2400)
    const vb2400 = Number(w.find('[data-testid="lane-compose-canvas-svg"]').attributes('viewBox')!.split(' ')[2])
    // 容器足够宽时 viewBox 回落到容器宽（内容宽不再撑开画布）
    expect(vb2400).toBe(2400)
  })

  it('V3 不越界：行标签 / 分隔线 / 节点标题都收在画布内（无横条胶囊）', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    const vbWidth = Number(w.find('[data-testid="lane-compose-canvas-svg"]').attributes('viewBox')!.split(' ')[2])
    expect(w.findAll('[data-role="band-capsule"], .lcc-canvas__band-bar')).toHaveLength(0)
    const rows = w.findAll('.lcc-canvas__row')
    expect(rows.length).toBeGreaterThanOrEqual(2)
    for (const separator of w.findAll('[data-role="row-separator"]')) {
      expect(Number(separator.attributes('x1'))).toBeGreaterThanOrEqual(0)
      expect(Number(separator.attributes('x2'))).toBeLessThanOrEqual(vbWidth)
    }
    for (const label of w.findAll('[data-role="row-label"]')) {
      const x = Number(label.attributes('x'))
      const textWidth = estimateTextWidth(label.text(), 11)
      expect(x).toBeGreaterThanOrEqual(0)
      expect(x + textWidth).toBeLessThanOrEqual(vbWidth)
    }
  })

  it('V4 半径下限：普通点 ≥3px', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    for (const node of w.findAll('.lcc-canvas__point')) {
      expect(Number(node.attributes('r'))).toBeGreaterThanOrEqual(3)
    }
  })

  it('V5 已选点半径恒严格大于推荐点半径', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    const selected = Number(w.find('[data-testid="lane-node-1"] .lcc-canvas__point').attributes('r'))
    const recommended = Number(w.find('[data-testid="lane-node-4"] .lcc-canvas__point').attributes('r'))
    expect(recommended).toBeGreaterThan(0)
    expect(selected).toBeGreaterThan(recommended)
    // 推荐点不等于已选（没有勾号）
    expect(w.find('[data-testid="lane-node-4"]').classes()).not.toContain('is-selected')
    expect(w.findAll('.lcc-canvas__check')).toHaveLength(3)
  })

  it('相关节点标题：选中话题后，相关节点在节点下显示标题缩略', async () => {
    const longLabel = '这是很长很长的一个事件标题需要缩略显示'
    const sections = SECTIONS.map(s => (s.id === '4' ? { ...s, clusterLabel: longLabel } : s))
    persistentMock.getComposeCandidates.mockResolvedValue({
      success: true,
      data: response({
        sections,
        relations: [...RELATIONS, { fromId: '1', toId: '4', distance: 0.2, relationType: 'similarity' }],
      }),
    })
    const w = await mountOverlay()
    await setCanvasWidth(900)
    // 未选中：不画相关节点标题（散料点保持干净）
    expect(w.findAll('.lcc-canvas__related-title')).toHaveLength(0)
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    const titles = w.findAll('.lcc-canvas__related-title')
    const ids = titles.map(t => t.attributes('data-testid'))
    // 相关 = 1 跳原关系 ∪ 质心较近的推荐：节点 4（近邻）、节点 6（归档来源，距离 0.225 ≤ 阈值）
    expect(ids).toContain('lane-related-title-4')
    expect(ids).toContain('lane-related-title-6')
    // 已选成员自己不当自己的相关节点；只读的节点 5 走提示行、不逐点画标题
    expect(ids).not.toContain('lane-related-title-1')
    expect(ids).not.toContain('lane-related-title-5')
    // 标题缩略：比原文短且带省略号，不超上限
    const t4 = w.find('[data-testid="lane-related-title-4"]')
    expect(t4.text().length).toBeLessThan(longLabel.length)
    expect(t4.text().endsWith('…')).toBe(true)
    expect(t4.text().length).toBeLessThanOrEqual(11) // 10 个汉字 + 省略号
    // 位置：在节点下方，且不越出画布
    const nodeY = Number(w.find('[data-testid="lane-node-4"] .lcc-canvas__point').attributes('cy'))
    const titleY = Number(t4.attributes('y'))
    expect(titleY).toBeGreaterThan(nodeY)
    const vbWidth = Number(/^0 0 (\d+)/.exec(w.find('[data-testid="lane-compose-canvas-svg"]').attributes('viewBox')!)![1])
    const titleX = Number(t4.attributes('x'))
    const textWidth = estimateTextWidth(t4.text(), 9)
    expect(titleX - textWidth / 2).toBeGreaterThanOrEqual(0)
    expect(titleX + textWidth / 2).toBeLessThanOrEqual(vbWidth)
  })

  it('相关节点在已有泳道里：只读参照区不压字、画布底部不再有说明行', async () => {
    persistentMock.getComposeCandidates.mockResolvedValue({
      success: true,
      data: response({
        relations: [...RELATIONS, { fromId: '1', toId: '5', distance: 0.2, relationType: 'similarity' }],
      }),
    })
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    // 节点 5 属于已有 active 泳道（只读）→ 不在节点下画标题
    expect(w.find('[data-testid="lane-related-title-5"]').exists()).toBe(false)
    // 2026-11-23 用户定档：底部那行说明整段删除（只读节点在只读梯子里可见）
    expect(w.find('[data-testid="lane-related-hint"]').exists()).toBe(false)
    // 开启只读参照后：仍不在节点下压字
    await (w.find('[data-testid="lane-compose-show-assigned"]').element as HTMLInputElement).click()
    await flushPromises()
    expect(w.find('[data-testid="lane-related-title-5"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-related-hint"]').exists()).toBe(false)
  })

  it('相关节点标题：选中话题后，相关节点在节点下显示标题缩略', async () => {
    const longLabel = '这是很长很长的一个事件标题需要缩略显示'
    const sections = SECTIONS.map(s => (s.id === '4' ? { ...s, clusterLabel: longLabel } : s))
    persistentMock.getComposeCandidates.mockResolvedValue({
      success: true,
      data: response({
        sections,
        relations: [...RELATIONS, { fromId: '1', toId: '4', distance: 0.2, relationType: 'similarity' }],
      }),
    })
    const w = await mountOverlay()
    await setCanvasWidth(900)
    // 未选中：不画相关节点标题（散料点保持干净）
    expect(w.findAll('.lcc-canvas__related-title')).toHaveLength(0)
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    const titles = w.findAll('.lcc-canvas__related-title')
    const ids = titles.map(t => t.attributes('data-testid'))
    // 相关 = 1 跳原关系 ∪ 质心较近的推荐：节点 4（近邻）、节点 6（归档来源，距离 0.225 ≤ 阈值）
    expect(ids).toContain('lane-related-title-4')
    expect(ids).toContain('lane-related-title-6')
    // 已选成员自己不当自己的相关节点；只读的节点 5 走提示行、不逐点画标题
    expect(ids).not.toContain('lane-related-title-1')
    expect(ids).not.toContain('lane-related-title-5')
    // 标题缩略：比原文短且带省略号，不超上限
    const t4 = w.find('[data-testid="lane-related-title-4"]')
    expect(t4.text().length).toBeLessThan(longLabel.length)
    expect(t4.text().endsWith('…')).toBe(true)
    expect(t4.text().length).toBeLessThanOrEqual(11) // 10 个汉字 + 省略号
    // 位置：在节点下方，且不越出画布
    const nodeY = Number(w.find('[data-testid="lane-node-4"] .lcc-canvas__point').attributes('cy'))
    const titleY = Number(t4.attributes('y'))
    expect(titleY).toBeGreaterThan(nodeY)
    const vbWidth = Number(/^0 0 (\d+)/.exec(w.find('[data-testid="lane-compose-canvas-svg"]').attributes('viewBox')!)![1])
    const titleX = Number(t4.attributes('x'))
    const textWidth = estimateTextWidth(t4.text(), 9)
    expect(titleX - textWidth / 2).toBeGreaterThanOrEqual(0)
    expect(titleX + textWidth / 2).toBeLessThanOrEqual(vbWidth)
  })

  it('相关节点标题：既无原关系又离质心远（超阈值）的节点不画标题', async () => {
    const sections = [...SECTIONS, section('7', '2026-10-21', '毫不相干的事件', [0, 1])]
    persistentMock.getComposeCandidates.mockResolvedValue({ success: true, data: response({ sections }) })
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-related-title-7"]').exists()).toBe(false)
  })


  it('V6 行式泳道：每个候选链一行（行标签 = 话题名 + 计数），无 tick 拖线', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    const chainRow = w.find('.lcc-canvas__row[data-row-kind="chain"]')
    expect(chainRow.exists()).toBe(true)
    expect(chainRow.find('[data-role="row-label"]').text()).toBe('候选泳道 A')
    expect(chainRow.text()).toContain('剩余 3 条')
    expect(chainRow.find('.lcc-canvas__row-tally').text()).toBe('0/3')
    expect(w.findAll('.lcc-canvas__band-tick')).toHaveLength(0)
  })

  it('V7 话题多于可读行数：明写「另有 N 个候选话题在左侧候选栏」', async () => {
    const sections = Array.from({ length: 12 }, (_, i) => section(String(i + 1), '2026-10-27', `事件 ${i + 1}`, [1, 0], `T${i + 1}`))
    const chains = Array.from({ length: 12 }, (_, i) => ({
      ...CHAIN,
      key: `T${i + 1}:${i + 1}`,
      topicId: `T${i + 1}`,
      topicLabel: `候选话题 ${i + 1}`,
      sectionIds: [String(i + 1)],
    }))
    persistentMock.getComposeCandidates.mockResolvedValue({
      success: true,
      data: response({ sections, candidateChains: chains, relations: [] }),
    })
    const w = await mountOverlay()
    await setCanvasWidth(900)
    expect(w.findAll('.lcc-canvas__row[data-row-kind="chain"]')).toHaveLength(10)
    expect(w.find('.lcc-canvas__row-more').text()).toContain('另有 2 个候选话题在左侧候选栏')
  })

  it('V14 已有 active 泳道默认隐藏，显式开启才显示且只读', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    expect(w.find('[data-testid="lane-node-5"]').exists()).toBe(false)
    await (w.find('[data-testid="lane-compose-show-assigned"]').element as HTMLInputElement).click()
    await flushPromises()
    const node = w.find('[data-testid="lane-node-5"]')
    expect(node.exists()).toBe(true)
    expect(node.classes()).toContain('is-readonly')
  })

  it('V15 取消聚焦不偷改已有泳道显示偏好', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await (w.find('[data-testid="lane-compose-show-assigned"]').element as HTMLInputElement).click()
    await flushPromises()
    await w.find('[data-testid="lane-node-5"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-compose-clear-focus"]').trigger('click')
    await flushPromises()
    expect((w.find('[data-testid="lane-compose-show-assigned"]').element as HTMLInputElement).checked).toBe(true)
    expect(w.find('[data-testid="lane-node-5"]').exists()).toBe(true)
  })

  it('V13 已有 active 泳道的点只读：节点卡不给选入动作', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await (w.find('[data-testid="lane-compose-show-assigned"]').element as HTMLInputElement).click()
    await flushPromises()
    await w.find('[data-testid="lane-node-5"]').trigger('click')
    await flushPromises()
    const card = w.find('[data-testid="lane-node-card"]')
    expect(card.exists()).toBe(true)
    expect(card.text()).toContain('已有泳道只读')
    expect(w.find('[data-testid="lane-node-card-action"]').exists()).toBe(false)
  })
})

describe('BoardComposeOverlay — 节点卡（V8–V12）', () => {
  it('V8 悬停出卡：完整标题、日期、估算接近度（与编辑工作面同向）、原关系条数', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-node-4"]').trigger('mouseenter')
    await flushPromises()
    const card = w.find('[data-testid="lane-node-card"]')
    expect(card.exists()).toBe(true)
    expect(card.text()).toContain('散料近邻')
    expect(card.text()).toContain('2026-10-26')
    // 两工作面接近度方向一致（S2.3）：不再显示「距离」，统一「估算接近度」（越大越近）。
    expect(card.text()).toContain('估算接近度')
    expect(card.text()).not.toContain('距离')
    expect(card.text()).toContain('原关系')
  })

  it('V9 选中后卡片钉住并跟随节点（移开鼠标不消失）', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await w.find('[data-testid="lane-node-1"]').trigger('click')
    await flushPromises()
    const card = w.find('[data-testid="lane-node-card"]')
    expect(card.exists()).toBe(true)
    expect(card.classes()).toContain('is-pinned')
    await w.find('[data-testid="lane-node-1"]').trigger('mouseleave')
    await flushPromises()
    expect(w.find('[data-testid="lane-node-card"]').exists()).toBe(true)
  })

  it('V10/V11 卡内「＋选入 / 移出」真实改成员并当场翻字', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    // 阶 0 散料默认折叠：先整组选中一条候选链激活质心，散料近邻才浮现
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-node-4"]').trigger('click')
    await flushPromises()
    const action = w.find('[data-testid="lane-node-card-action"]')
    expect(action.text()).toContain('选入本次泳道')
    await action.trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-node-card-action"]').text()).toContain('移出本次泳道')
    expect(w.find('[data-testid="lane-node-4"]').classes()).toContain('is-selected')

    await w.find('[data-testid="lane-node-card-action"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-node-card-action"]').text()).toContain('选入本次泳道')
    expect(w.find('[data-testid="lane-node-4"]').classes()).not.toContain('is-selected')
  })

  it('V12 卡上 × 与再点同一节点 = 取消选中（卡片收起、右栏线索清空）', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await w.find('[data-testid="lane-node-1"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-node-card"]').exists()).toBe(true)

    await w.find('[data-testid="lane-node-card-close"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-node-card"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-compose-focus"]').text()).toContain('点画布上的点看详情')

    // 再点一次同一节点同样取消选中
    await w.find('[data-testid="lane-node-1"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-node-card"]').exists()).toBe(true)
    await w.find('[data-testid="lane-node-1"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-node-card"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-compose-focus"]').text()).toContain('点画布上的点看详情')
  })
})

describe('BoardComposeOverlay — 右栏三段两 tab（S6）', () => {
  it('右栏只有三段：当前事件线索、可补充的事件、tab 区（已选事件 / 按标题找）', async () => {
    const w = await mountOverlay()
    expect(w.find('[data-testid="lane-compose-focus"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-suggestions"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-tab-chosen"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-tab-find"]').exists()).toBe(true)
    // 不得出现关联事件清单 / 计算依据 / 话题泳道参照
    expect(w.text()).not.toContain('查看关联事件')
    expect(w.text()).not.toContain('计算依据')
    expect(w.text()).not.toContain('已有泳道参照')
  })

  it('当前事件线索只叙述，不放选入按钮', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await w.find('[data-testid="lane-node-1"]').trigger('click')
    await flushPromises()
    const focus = w.find('[data-testid="lane-compose-focus"]')
    expect(focus.text()).toContain('候选一')
    expect(focus.find('button').exists()).toBe(false)
  })

  it('可补充的事件：逐条明确加入，加入后从待选移除并重算', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    const suggest = w.find('[data-testid="lane-compose-suggestions"]')
    expect(suggest.find('[data-testid="lane-suggest-4"]').exists()).toBe(true)
    await w.find('[data-testid="lane-suggest-add-4"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-suggest-4"]').exists()).toBe(false)
  })

  it('已选事件 tab：逐条真实复选框，取消勾选即移出本次', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    expect(w.findAll('[data-testid^="lane-chosen-row-"]')).toHaveLength(3)
    await (w.find('[data-testid="lane-chosen-check-2"]').element as HTMLInputElement).click()
    await flushPromises()
    expect(w.find('[data-testid="lane-chosen-row-2"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('2')
  })

  it('按标题找：只在本窗范围内按标题过滤，不自动勾选', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-compose-tab-find"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-compose-find-input"]').setValue('归档')
    await flushPromises()
    expect(w.find('[data-testid="lane-find-6"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-find-1"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('3')
  })

  it('按标题找：无匹配有空态', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-compose-tab-find"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-compose-find-input"]').setValue('不存在的事件')
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-panel-find"]').text()).toContain('没有匹配的事件')
  })
})

describe('BoardComposeOverlay — 日期共窗与草稿确认（S2）', () => {
  it('有草稿切日期先确认；取消保留成员与推荐，确认才清草稿重算', async () => {
    const w = await mountOverlay()
    await flushPromises()
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('3')

    // 时间视窗改为 ObservationRangeControls（档位按钮组；不再用 select）
    const tiers = w.findAll('.orc-tier')
    expect(tiers.map(b => b.text())).toEqual(['7 天', '14 天', '30 天', '近 1 年'])
    await tiers[0]!.trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="compose-confirm-ok"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('3')

    await w.find('[data-testid="compose-confirm-cancel"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('3')
    // 取消后档位激活态回到生效档（30 天）
    expect(w.findAll('.orc-tier')[2]!.classes()).toContain('is-active')

    await w.findAll('.orc-tier')[0]!.trigger('click')
    await flushPromises()
    await w.find('[data-testid="compose-confirm-ok"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('0')
    expect(persistentMock.getComposeCandidates).toHaveBeenLastCalledWith(1974, { mode: 7, days: 7 })
  })

  it('日期非法：行内报错、保留旧范围与草稿（表单就地拒绝，不静默截断）', async () => {
    const w = await mountOverlay()
    await flushPromises()
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    // 表单倒置：控件自己拦下（issue 反馈行），不提交 commit、不改成员
    await w.find('#orc-start').setValue('2026-10-30')
    await w.find('#orc-end').setValue('2026-10-01')
    await w.find('.orc form').trigger('submit')
    await flushPromises()
    expect(w.find('[data-testid="observation-range-feedback"]').text()).toContain('起始日期不能晚于截止日期')
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('3')
    const calls = persistentMock.getComposeCandidates.mock.calls.length
    await w.find('#orc-start').setValue('2026-01-01') // 池外
    await w.find('#orc-end').setValue('2026-01-20')
    await w.find('.orc form').trigger('submit')
    await flushPromises()
    expect(w.find('[data-testid="observation-range-feedback"]').text()).toContain('超出可取数范围')
    expect(persistentMock.getComposeCandidates.mock.calls.length).toBe(calls) // 非法不重取
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('3')
  })
})

describe('BoardComposeOverlay — 保存与命名（S7）', () => {
  it('缺名称：拒绝创建并保留成员', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="compose-save"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="compose-name-input"] input').setValue('   ')
    await w.find('[data-testid="compose-name-confirm"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="compose-name-error"]').exists()).toBe(true)
    expect(persistentMock.createManualLane).not.toHaveBeenCalled()
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('3')
  })

  it('保存成功：走既有 manual 端点 → saved + close', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="compose-save"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="compose-name-input"] input').setValue('新泳道')
    await w.find('[data-testid="compose-name-confirm"]').trigger('click')
    await flushPromises()
    expect(persistentMock.createManualLane).toHaveBeenCalledWith(1974, '新泳道', ['1', '2', '3'])
    expect(w.emitted('saved')?.[0]?.[0]).toMatchObject({ id: '300', label: '新泳道' })
    expect(w.emitted('close')).toHaveLength(1)
  })

  it('自动预填名跟随来源组；人工写过的名字不被覆盖', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="compose-save"]').trigger('click')
    await flushPromises()
    expect((w.find('[data-testid="compose-name-input"] input').element as HTMLInputElement).value).toBe('候选泳道 A')

    await w.find('[data-testid="compose-name-input"] input').setValue('我手动起的名字')
    await w.find('[data-testid="compose-name-close"]').trigger('click')
    await flushPromises()
    // 取消整组 → 重新整组勾回：人工名不得被任何预填覆盖
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="compose-save"]').trigger('click')
    await flushPromises()
    expect((w.find('[data-testid="compose-name-input"] input').element as HTMLInputElement).value).toBe('我手动起的名字')
  })

  it('取消本次：有草稿先确认，确认后才关闭', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="compose-cancel-draft"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="compose-confirm-ok"]').exists()).toBe(true)
    expect(w.emitted('close')).toBeUndefined()

    await w.find('[data-testid="compose-confirm-ok"]').trigger('click')
    await flushPromises()
    expect(w.emitted('close')).toHaveLength(1)
  })
})

describe('BoardComposeOverlay — 历史待启用话题入口（旧编排面能力回归）', () => {
  it('工作面内有明确入口；点击开弹窗并拉服务端管理名单', async () => {
    const w = await mountOverlay()
    expect(w.find('[data-testid="lane-compose-history-open"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-history-search"]').exists()).toBe(false)

    await w.find('[data-testid="lane-compose-history-open"]').trigger('click')
    await flushPromises()
    expect(reportsMock.listBoardTopics).toHaveBeenCalledWith(1974)
    const row = w.find('[data-testid="lane-history-1984"]')
    expect(row.exists()).toBe(true)
    expect(row.text()).toContain('候选泳道 A')
    expect(row.text()).toContain('累计命中 5 次')
    expect(row.text()).toContain('2026-10-30')
    // 说明段：管理资格名单 ≠ 本次推荐；未达门槛置灰但仍可带入草稿
    expect(w.find('.lchd__explainer').text()).toContain('管理资格名单')
    expect(w.find('.lchd__explainer').text()).toContain('不是本次推荐')
  })

  it('未达管理门槛：「直接启用原话题」置灰并说明原因，仍可带入草稿', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-compose-history-open"]').trigger('click')
    await flushPromises()
    const activate = w.find('[data-testid="lane-history-activate-1985"]')
    expect(activate.attributes('disabled')).toBeDefined()
    expect(w.find('[data-testid="lane-history-hint-1985"]').text()).toContain('累计命中未达管理门槛')
    // 置灰 ≠ 不可带入草稿
    expect(w.find('[data-testid="lane-history-adopt-1985"]').attributes('disabled')).toBeUndefined()
  })

  it('「带入草稿」：只带该话题在当前窗口内的 owned 成员，不补相似、不改人工名', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-compose-history-open"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-history-adopt-1984"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('3')
    expect((w.find('[data-testid="lane-member-1"]').element as HTMLInputElement).checked).toBe(true)
    // 只带 owned 成员：散料（无归属）不得被自动补选
    expect(w.findAll('[data-testid^="lane-chosen-row-"]')).toHaveLength(3)
    expect(notifyMock.success).toHaveBeenCalled()
    expect(String(notifyMock.success.mock.calls[0]![0])).toContain('新增预选 3 条')
  })

  it('人工写过的名字不被历史带入覆盖（提示「已保留你写的名称」）', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="compose-save"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="compose-name-input"] input').setValue('我手动起的名字')
    await w.find('[data-testid="compose-name-close"]').trigger('click')
    await flushPromises()

    await w.find('[data-testid="lane-compose-history-open"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-history-adopt-1984"]').trigger('click')
    await flushPromises()
    expect(String(notifyMock.success.mock.calls[0]![0])).toContain('已保留你写的名称，未覆盖')

    await w.find('[data-testid="compose-save"]').trigger('click')
    await flushPromises()
    expect((w.find('[data-testid="compose-name-input"] input').element as HTMLInputElement).value).toBe('我手动起的名字')
  })

  it('「直接启用原话题」：走既有 updateTopic(status=active)，并说清与本次草稿无关', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-compose-history-open"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-history-activate-1984"]').trigger('click')
    await flushPromises()
    expect(reportsMock.updateTopic).toHaveBeenCalledWith(1984, { status: 'active' })
    expect(String(notifyMock.success.mock.calls[0]![0])).toContain('与本次草稿的人工新建无关')
    // 不建新泳道、不碰编排态保存接口
    expect(persistentMock.createManualLane).not.toHaveBeenCalled()
  })

  it('「直接启用原话题」失败：报错、不假装成功', async () => {
    reportsMock.updateTopic.mockResolvedValueOnce({ success: false, error: '启用被拒' })
    const w = await mountOverlay()
    await w.find('[data-testid="lane-compose-history-open"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-history-activate-1984"]').trigger('click')
    await flushPromises()
    expect(notifyMock.error).toHaveBeenCalled()
    expect(String(notifyMock.error.mock.calls[0]![0])).toContain('启用被拒')
    expect(notifyMock.success).not.toHaveBeenCalled()
  })

  it('浮层展示锚：工作面遮罩 Teleport 到 body + fixed/inset 0 规则存在；历史名单弹窗走 AppDialog 尺寸档', async () => {
    // 样式规则锚（工作面是自建浮层容器）
    const source = readFileSync(`${import.meta.dirname}/BoardComposeOverlay.vue`, 'utf8')
    const style = source.slice(source.indexOf('<style'))
    expect(style).toMatch(/\.bco,\s*\.bco-narrow\s*\{[^}]*position:\s*fixed/s)
    expect(style).toMatch(/\.bco,\s*\.bco-narrow\s*\{[^}]*inset:\s*0/s)

    // 弹窗尺寸档锚：新弹窗走 AppDialog size 档，不写自由 width / 手写 z-index
    const dialogSource = readFileSync(`${import.meta.dirname}/../compose/LaneComposeHistoryDialog.vue`, 'utf8')
    expect(dialogSource).toContain('size="lg"')
    expect(dialogSource).not.toMatch(/<AppDialog[^>]*\bwidth=/)

    // Teleport 挂载锚：不 stub teleport 时遮罩渲染在 document.body 而非组件原地
    const anchor = mount(BoardComposeOverlay, {
      props: { boardId: 1974, boardLabel: '测试版块' },
      attachTo: document.body,
    })
    await flushPromises()
    expect(document.body.querySelector(':scope > [data-testid="board-compose"]')).not.toBeNull()
    anchor.unmount()
  })

  it('名单搜索：纯前端按名过滤（不调 AI / 不调 embed）', async () => {
    const w = await mountOverlay()
    await w.find('[data-testid="lane-compose-history-open"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-history-search"] input').setValue('门槛')
    await flushPromises()
    expect(w.find('[data-testid="lane-history-1985"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-history-1984"]').exists()).toBe(false)
    expect(persistentMock.embedQuery).not.toHaveBeenCalled()
  })
})

describe('BoardComposeOverlay — 时间视窗与可选池曲线（unify 7.1–7.4 / OQ-2）', () => {
  it('顶部时间视窗在场：档位 7/14/30/365 + 曲线口径明示「不含 active 泳道」', async () => {
    const w = await mountOverlay()
    await flushPromises()
    const range = w.find('[data-testid="lane-compose-range"]')
    expect(range.exists()).toBe(true)
    expect(w.findAll('.orc-tier').map(b => b.text())).toEqual(['7 天', '14 天', '30 天', '近 1 年'])
    expect(range.text()).toContain('不含 active 泳道')
    expect(range.text()).toContain('线索数（非文章数）')
  })

  it('可选池曲线走轻量取数（omit_embeddings=1），失败给单独重试入口', async () => {
    // 初始池取数失败（第二次调用 = 挂载时的 loadPoolCurve）：曲线错误在场但工作面不清空
    const real = persistentMock.getComposeCandidates.getMockImplementation()
    persistentMock.getComposeCandidates.mockImplementation(async (boardId: number, query: Record<string, unknown>) => {
      if (query && (query as { omitEmbeddings?: boolean }).omitEmbeddings) {
        return { success: false, error: '池未取到' }
      }
      return real!(boardId, query)
    })
    const w = await mountOverlay()
    await flushPromises()
    expect(persistentMock.getComposeCandidates).toHaveBeenCalledWith(1974, { days: 365, omitEmbeddings: true })
    expect(w.find('[data-testid="lane-compose-curve-error"]').text()).toContain('池未取到')
    expect(w.find('[data-testid="lane-compose-canvas"]').exists()).toBe(true)
    // 重试：恢复成功后错误清空
    persistentMock.getComposeCandidates.mockImplementation(real!)
    await w.find('[data-testid="lane-compose-curve-retry"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-curve-error"]').exists()).toBe(false)
  })

  it('档位切 365 可用（OQ-2 裁决）：无草稿直接取数', async () => {
    const w = await mountOverlay()
    await flushPromises()
    await w.findAll('.orc-tier')[3]!.trigger('click')
    await flushPromises()
    expect(persistentMock.getComposeCandidates).toHaveBeenLastCalledWith(1974, { mode: 365, days: 365 })
  })
})

// ── 泳道行 / 拖拽 / 阈值三档 / 检索态（rework-lane-compose-swimlane-canvas 2.x–6.x） ──

/** 指针拖拽（pointerdown → pointermove → pointerup），坐标即画布逻辑坐标（容器 rect 为 0）。 */
async function dragOn(w: ReturnType<typeof mount>, from: { x: number, y: number }, to: { x: number, y: number }, on: Element): Promise<void> {
  const surface = w.find('[data-testid="lane-compose-canvas"]').element
  const fire = (type: string, point: { x: number, y: number }): void => {
    const event = new Event(type, { bubbles: true, cancelable: true }) as PointerEvent
    Object.defineProperties(event, {
      clientX: { value: point.x },
      clientY: { value: point.y },
      pointerId: { value: 1 },
    })
    ;(type === 'pointerdown' ? on : surface).dispatchEvent(event)
  }
  fire('pointerdown', from)
  fire('pointermove', to)
  fire('pointerup', to)
  await flushPromises()
}

function rowHandleRect(w: ReturnType<typeof mount>, row: number): { x: number, y: number } {
  const handle = w.find(`[data-role="row-handle"][data-row="${row}"]`)
  return { x: Number(handle.attributes('x')) + 20, y: Number(handle.attributes('y')) + 12 }
}

function laneRowDropY(w: ReturnType<typeof mount>): number {
  const top = Number(w.find('.lcc-canvas__row[data-row="0"] [data-role="row-separator"]').attributes('y1'))
  return top + 40
}

describe('BoardComposeOverlay — 行式泳道与拖拽选入（A-2 / A-3 / A-13）', () => {
  it('A-13 拖行标签整组采纳：一次落位，左栏记 n/n，取空的链行从画布消失', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    expect(w.find('.lcc-canvas__row[data-row-kind="chain"]').exists()).toBe(true)
    await dragOn(w, rowHandleRect(w, 1), { x: 60, y: laneRowDropY(w) }, w.find('[data-role="row-handle"][data-row="1"]').element)
    expect(w.find('[data-testid="lane-state-1984:1"]').text()).toContain('3 / 3 已选')
    expect(w.find('.lcc-canvas__row[data-row-kind="chain"]').exists()).toBe(false)
    // 不出现「已采纳」摘要、不统计来源构成
    expect(w.text()).not.toContain('已采纳')
    expect(w.find('[data-testid="lane-compose-canvas"]').text()).not.toContain('来源构成')
  })

  it('A-2 拖单节点入本次泳道：成员数 +1 且原行留空位', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    const node = w.find('[data-testid="lane-node-1"]')
    const point = node.find('.lcc-canvas__point')
    await dragOn(
      w,
      { x: Number(point.attributes('cx')), y: Number(point.attributes('cy')) },
      { x: Number(point.attributes('cx')), y: laneRowDropY(w) },
      node.element,
    )
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('1')
    expect((w.find('[data-testid="lane-member-1"]').element as HTMLInputElement).checked).toBe(true)
    // 原链行仍在（还有 2 个未采纳成员），n/N 同步
    expect(w.find('[data-testid="lane-state-1984:1"]').text()).toContain('1 / 3 已选')
  })

  it('A-5 点击节点只改选中态：成员数不变（防回归到「点击选入」）', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await w.find('[data-testid="lane-node-1"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-node-card"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('0')
    expect(w.findAll('input[type="checkbox"]:checked')).toHaveLength(0)
  })
})

describe('BoardComposeOverlay — 阈值三档（A-6 / 4.3）', () => {
  it('工具条三档默认「标准」；切「仅高相关」只改散料，候选链成员/已选不变，右栏同批', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    expect(w.find('[data-testid="lane-compose-tier-standard"]').attributes('aria-pressed')).toBe('true')

    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-node-6"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-suggest-6"]').exists()).toBe(true)

    await w.find('[data-testid="lane-compose-tier-tight"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-tier-tight"]').attributes('aria-pressed')).toBe('true')
    // 候选链成员 + 已选逐个不变
    for (const id of ['1', '2', '3']) expect(w.find(`[data-testid="lane-node-${id}"]`).exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-member-count"]').text()).toContain('3')
    // 散料收紧：画布与右栏同批
    expect(w.find('[data-testid="lane-node-6"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-suggest-6"]').exists()).toBe(false)
  })
})

describe('BoardComposeOverlay — 检索态与右栏恒定骨架（A-7 / A-7b / A-14）', () => {
  it('A-14 空态 ③ 默认落「按标题找」；选入后回到「已选事件」', async () => {
    const w = await mountOverlay()
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-tab-find"]').attributes('aria-selected')).toBe('true')
    expect(w.find('[data-testid="lane-compose-find-input"]').exists()).toBe(true)
    // ①②③ 三段都在
    expect(w.find('[data-testid="lane-compose-focus"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-suggestions"]').exists()).toBe(true)

    await w.find('[data-testid="lane-title-1984:1"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-tab-chosen"]').attributes('aria-selected')).toBe('true')
    expect(w.find('[data-testid="lane-compose-focus"]').exists()).toBe(true)
  })

  it('A-7 检索态：画布降级 + 命中四角括号；①② 保持可见', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await w.find('[data-testid="lane-compose-tab-find"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-compose-find-input"]').setValue('归档')
    await flushPromises()

    expect(w.find('[data-testid="lane-compose-search-state"]').text()).toContain('检索态')
    expect(w.find('[data-testid="lane-node-6"] [data-role="search-hit"]').exists()).toBe(true)
    expect(w.find('.lcc-canvas__row[data-row-kind="chain"]').classes()).toContain('is-search-bg')
    expect(w.find('[data-testid="lane-compose-focus"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-suggestions"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-compose-find-scope"]').text()).toContain('未归类 · 命中 1 条')
  })

  it('A-7b 检索范围不含候选链成员：只出现在链成员标题里的词 → 命中 0', async () => {
    const w = await mountOverlay()
    await setCanvasWidth(900)
    await w.find('[data-testid="lane-compose-tab-find"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-compose-find-input"]').setValue('候选一')
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-find-scope"]').text()).toContain('命中 0 条')
    expect(w.findAll('[data-role="search-hit"]')).toHaveLength(0)
    // 候选链成员仍可由行标签整组采纳处置
    expect(w.find('[data-role="row-handle"][data-row="1"]').exists()).toBe(true)
  })
})

describe('BoardComposeOverlay — 工具条文案（6.1）', () => {
  it('工具条只给状态结论，不含「置信度」「不是」类解释句式', async () => {
    const w = await mountOverlay()
    const note = w.find('[data-testid="lane-compose-tool-note"]')
    expect(note.exists()).toBe(true)
    expect(note.text()).toContain('未归类')
    const tools = w.find('.bco__tools')
    expect(tools.text()).not.toContain('置信度')
    expect(tools.text()).not.toContain('不是')
    expect(tools.text()).not.toContain('推荐不自动选入')
  })
})

describe('BoardComposeOverlay — 检索态 Esc（interaction-table Esc 行为②）', () => {
  it('检索中按 Esc 只退出检索，不退出编排态', async () => {
    const w = await mountOverlay()
    await flushPromises()
    await w.find('[data-testid="lane-compose-tab-find"]').trigger('click')
    await flushPromises()
    await w.find('[data-testid="lane-compose-find-input"]').setValue('归档')
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-search-state"]').exists()).toBe(true)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(w.find('[data-testid="lane-compose-search-state"]').exists()).toBe(false)
    expect(w.emitted('close')).toBeUndefined()
  })
})
