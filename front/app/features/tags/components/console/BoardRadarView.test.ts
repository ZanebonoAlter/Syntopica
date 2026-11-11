/**
 * BoardRadarView 测试（rework-workshop-into-radar-and-desk 8.1/8.3 + 8A.1/8A.2 + R-17/R-18；
 * retire-story-adopt-event-metrics 3.2 换当天线索口径）。
 * 覆盖：加载骨架 / 分档标记 / 常温紧凑索引与完整横比 / 右栏「当天线索最多的泳道」清单 /
 * 空态 / 错误重试 / 点行进 L1 / 泳道小矩阵进 ?lane= / 无泳道有日报 / 数据延迟可辨 /
 * 首次失败不混业务空态。8A.1 报刊层级继承自已批准总览原型（ui-design approved），
 * 数据喂归一化读模型（camelCase/字符串 ID），raw→读模型接线在
 * api/normalizers/boardRadar.test.ts 与 semanticBoards.test.ts 验证。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BoardRadarView from './BoardRadarView.vue'
import type { RadarBoard, RadarTopLanes } from '~/api/semanticBoards'
import type { SchedulerStatus } from '~/types/scheduler'

const getOverview = vi.fn()
const createBoard = vi.fn()
const push = vi.fn()
const router = { push }

vi.mock('vue-router', () => ({ useRouter: () => router }))
vi.mock('~/api/semanticBoards', () => ({
  useBoardRadarApi: () => ({ getOverview }),
  useSemanticBoardsApi: () => ({ createBoard }),
}))
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ error: vi.fn(), warn: vi.fn(), success: vi.fn() }),
}))
vi.mock('~/features/tags/composables/useTagsPage', async () => {
  const { ref } = await import('vue')
  return {
    useTagsPage: () => ({
      boards: ref([]),
      auxiliaryLabels: ref([]),
      auxClusters: ref([]),
      auxUnclusteredCount: ref(0),
      auxLoading: ref(false),
      auxSearchQuery: ref(''),
      auxStatusFilter: ref(''),
      auxPagination: ref(null),
      upgradeBackfillNotice: ref(false),
      upgradePersistedSuggestions: ref([]),
      upgradePersistedLoading: ref(false),
      upgradePersistedGenerating: ref(false),
      showUpgradeDialog: ref(false),
      showMatchingConfigDialog: ref(false),
      showMergePreview: ref(false),
      showBackfillDialog: ref(false),
      showLabelPool: ref(false),
      showCreateBoard: ref(false),
      showCompositePool: ref(false),
      upgradeParams: ref({ direction: 'create', source: 'aux' }),
      setPage: vi.fn(),
      loadBoards: vi.fn(),
      loadAuxiliaryLabels: vi.fn(),
      loadClusters: vi.fn(),
      handleUpdatePage: vi.fn(),
      handleDisableAuxLabel: vi.fn(),
      handleMergeAuxLabel: vi.fn(),
    }),
  }
})
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))
// 右栏状态块（add-radar-pipeline-status）会启动常驻对账与 WS 订阅：
// 两者都 stub 掉，避免单测里发真实请求 / 留下长定时器。
vi.mock('~/composables/usePollBundle', () => ({
  usePollBundle: () => ({
    schedulers: useState<SchedulerStatus[]>('poll:schedulers', () => []),
    ensureStarted: vi.fn(),
    reconcile: vi.fn(),
    reconcileNow: vi.fn(),
  }),
}))
vi.mock('~/composables/useEventStream', () => ({
  useEventStream: () => ({
    on: () => () => {},
    off: () => {},
    get connected() { return false },
  }),
}))

function row(overrides: Partial<RadarBoard> = {}): RadarBoard {
  return {
    boardId: '1974',
    label: '中东地缘政治与美伊关系',
    tagCount: 144,
    reportDate: '2026-10-24',
    reportTitle: '伊朗警告打击海湾海底电缆',
    reportArticleCount: 14,
    reportEventCount: 13,
    todayClues: 8,
    last3Clues: 13,
    prev3Clues: 10,
    attention: 3.5,
    heatRuleVersion: 'heat-v2-lane-clues-1d',
    clueDays: Array.from({ length: 14 }, (_, i) => (i === 13 ? 8 : 1)),
    dataDelayed: false,
    lanes: [
      { topicId: '1', label: '美伊博弈', threadCount14d: 40, clueCounts: Array.from({ length: 14 }, (_, i) => (i === 13 ? 3 : 0)) },
      { topicId: '2', label: '沙特', threadCount14d: 27, clueCounts: Array.from({ length: 14 }, (_, i) => (i === 13 ? 2 : 0)) },
      { topicId: '3', label: '能源', threadCount14d: 17, clueCounts: Array.from({ length: 14 }, (_, i) => (i === 13 ? 1 : 0)) },
    ],
    ...overrides,
  }
}

async function mountRadar(boards: RadarBoard[], success = true, topLanes?: RadarTopLanes | null) {
  getOverview.mockResolvedValue({
    success,
    data: success ? { date: '2026-10-24', boards, topLanes: topLanes ?? null } : undefined,
    error: success ? undefined : '接口挂了',
  })
  const wrapper = mount(BoardRadarView, {
    attachTo: document.body,
    global: {
      stubs: {
        AddSemanticBoardDialog: { name: 'AddSemanticBoardDialog', props: ['visible'], template: '<div data-testid="add-board-stub" />' },
        UpgradeSuggestionPanel: true,
        MatchingConfigDialog: true,
        TagMergePreview: true,
        BackfillProgress: true,
        AuxiliaryLabelPool: true,
        CompositeLabelPool: true,
      },
    },
  })
  await flushPromises()
  return wrapper
}

/** 泳道清单段夹具（新版后端响应形状，服务端已归并/排序/截断）。 */
function laneFeed(over: Partial<RadarTopLanes> = {}): RadarTopLanes {
  return {
    anchorDate: '2026-10-24',
    lanes: [
      {
        boardId: '1974',
        boardLabel: '中东地缘政治与美伊关系',
        topicId: '1204',
        topicLabel: '美伊博弈',
        clueCount: 12,
        anchorDate: '2026-10-24',
      },
      {
        boardId: '3122',
        boardLabel: '能源价格',
        topicId: '88',
        topicLabel: '原油出口',
        clueCount: 5,
        anchorDate: '2026-10-23',
      },
    ],
    partial: false,
    degradedBoardIds: [],
    laggingBoardIds: ['3122'],
    ...over,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
})

describe('BoardRadarView — 当天线索最多的泳道清单右栏（D10）', () => {
  it('渲染泳道清单（版块名 + 泳道名 + 当天线索数 + 锚点日），不再拿日报标题当条目', async () => {
    const wrapper = await mountRadar([row()], true, laneFeed())
    const item = wrapper.find('[data-testid="radar-lane-1974-1204"]')
    expect(item.exists()).toBe(true)
    expect(item.text()).toContain('中东地缘政治与美伊关系')
    expect(item.text()).toContain('美伊博弈')
    expect(item.text()).toContain('12 条线索')
    expect(item.text()).toContain('10/24')
    // 滞后版块条目按自己的锚点日计（不冒充全局日）
    expect(wrapper.find('[data-testid="radar-lane-3122-88"]').text()).toContain('10/23')
    // 版块行的「最新日报标题」入口保持不变；右栏不再列日报标题
    expect(wrapper.find('[data-testid="radar-read-1974"]').exists()).toBe(false)
  })

  it('泳道条目点击 → 进既有控制台并带 ?lane= 参数', async () => {
    const wrapper = await mountRadar([row()], true, laneFeed())
    await wrapper.find('[data-testid="radar-lane-1974-1204"]').trigger('click')
    expect(push).toHaveBeenCalledWith({ path: '/tags/1974', query: { lane: '1204' } })
  })

  it('徽标与跨天脚注整体退役：无「新起」「升温 N×」、无升温卡片、无「跨天延续」', async () => {
    const wrapper = await mountRadar([row()], true, laneFeed({ partial: true }))
    const rail = wrapper.find('.rd-rail')
    expect(rail.text()).not.toContain('新起')
    expect(rail.text()).not.toContain('升温')
    expect(rail.text()).not.toContain('×')
    expect(rail.text()).not.toContain('跨天延续')
    expect(wrapper.find('[data-testid="radar-badge-legend"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-crossday-note"]').exists()).toBe(false)
    expect(wrapper.find('.rd-warm').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('暂未触发升温条件')
  })

  it('部分版块降级 / 滞后明示（本次排序不完整 + 各条目按自己的锚点日）', async () => {
    const wrapper = await mountRadar([row()], true, laneFeed({ partial: true, degradedBoardIds: ['9', '10'] }))
    const partial = wrapper.find('[data-testid="radar-partial"]')
    expect(partial.text()).toContain('本次泳道排序不完整')
    expect(partial.text()).toContain('2 个版块')
    expect(wrapper.find('[data-testid="radar-lagging"]').text()).toContain('1 个版块数据滞后')
  })

  it('清单为空渲染空态（不渲染成空白，也不说成「今天没有事件」）', async () => {
    const wrapper = await mountRadar([row()], true, laneFeed({ lanes: [] }))
    expect(wrapper.find('[data-testid="radar-no-lane"]').text()).toContain('今天没有可排的泳道')
    expect(wrapper.text()).not.toContain('今天没有事件')
  })

  it('旧响应（无 top_lanes 段）回退版块级日报标题列表，不渲染业务空态', async () => {
    const wrapper = await mountRadar([row()], true, null)
    expect(wrapper.find('[data-testid="radar-read-1974"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-read-1974"]').text()).toContain('伊朗警告打击海湾海底电缆')
    expect(wrapper.find('[data-testid="radar-no-lane"]').exists()).toBe(false)
  })
})

describe('BoardRadarView — 雷达列表', () => {
  it('热点行分「热点」档（新标定阈值 ≥14.5），常温折叠不展开；点行进 L1', async () => {
    const wrapper = await mountRadar([
      row({ boardId: '1', label: '热点版块', attention: 20 }),
      row({ boardId: '2', label: '关注版块', attention: 5 }),
      row({ boardId: '3', label: '常温版块', attention: 0.6, todayClues: 0, last3Clues: 0, prev3Clues: 0 }),
    ])
    expect(wrapper.find('[data-testid="radar-row-1"]').classes()).toContain('is-hot')
    expect(wrapper.find('[data-testid="radar-row-1"]').text()).toContain('热点')
    expect(wrapper.find('[data-testid="radar-row-3"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('常温 · 1 个版块')

    await wrapper.find('[data-testid="radar-row-1"]').trigger('click')
    expect(push).toHaveBeenCalledWith('/tags/1')
  })

  it('常温紧凑索引可展开完整横比，紧凑/展开两种去向都进版块', async () => {
    const wrapper = await mountRadar([row({ boardId: '3', label: '常温版块', attention: 0.6 })])
    expect(wrapper.text()).toContain('常温版块')
    expect(wrapper.find('.rd-cold-list').exists()).toBe(false)
    // 紧凑索引：常温行直接进版块
    await wrapper.find('.rd-cold-row').trigger('click')
    expect(push).toHaveBeenCalledWith('/tags/3')
    // 展开后的常温行与关注行同为完整横比（含小矩阵与分档）
    await wrapper.find('.rd-cold-toggle').trigger('click')
    expect(wrapper.find('.rd-cold-list').exists()).toBe(true)
    expect(wrapper.find('.rd-cold-list [data-testid="radar-row-3"]').exists()).toBe(true)
    expect(wrapper.find('.rd-cold-list [data-testid="radar-row-3"]').text()).toContain('关注强度 · 常温')
    expect(wrapper.find('.rd-cold-list .rd-mini-lane').exists()).toBe(true)
    await wrapper.find('.rd-cold-list [data-testid="radar-row-3"]').trigger('click')
    expect(push).toHaveBeenLastCalledWith('/tags/3')
  })

  it('加载骨架与错误重试', async () => {
    getOverview.mockReturnValue(new Promise(() => {}))
    const loading = mount(BoardRadarView, { attachTo: document.body })
    expect(loading.find('[data-testid="radar-loading"]').exists()).toBe(true)
    loading.unmount()
    getOverview.mockReset()

    const wrapper = await mountRadar([], false)
    expect(wrapper.find('[role="alert"]').text()).toContain('接口挂了')
    await wrapper.find('[data-testid="radar-retry"]').trigger('click')
    expect(getOverview).toHaveBeenCalledTimes(2)
  })

  it('全部常温：空态提示而不是空白', async () => {
    const wrapper = await mountRadar([row({ boardId: '5', attention: 0.6, todayClues: 0, last3Clues: 0, prev3Clues: 0, lanes: [] })])
    expect(wrapper.text()).toContain('今天没有需要看的版块')
    expect(wrapper.text()).toContain('常温 · 1 个版块')
  })

  it('冷启动/正常总览都能新建版块，创建后重拉总览并进新版块', async () => {
    createBoard.mockResolvedValue({ success: true, data: { id: 77 } })
    const wrapper = await mountRadar([])
    expect(wrapper.find('[data-testid="radar-home"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-add-board"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-empty-add"]').exists()).toBe(true)

    await wrapper.find('[data-testid="radar-add-board"]').trigger('click')
    expect(wrapper.findComponent({ name: 'AddSemanticBoardDialog' }).props('visible')).toBe(true)
    getOverview.mockResolvedValue({ success: true, data: { date: '2026-10-24', boards: [row()], topLanes: null } })
    await wrapper.findComponent({ name: 'AddSemanticBoardDialog' }).vm.$emit('confirm', {
      label: '新板块', description: '', display_order: 0, protected: false, auxiliary_labels: [],
    })
    await flushPromises()
    expect(createBoard).toHaveBeenCalledWith(expect.objectContaining({ label: '新板块' }))
    // 创建成功回程：先重拉总览（第 2 次 overview）再进新版块
    expect(getOverview).toHaveBeenCalledTimes(2)
    expect(push).toHaveBeenCalledWith('/tags/77')
  })
})

describe('BoardRadarView — 右栏「上滚下驻」两段容器（add-radar-pipeline-status D9）', () => {
  it('上段内容区独立滚动，状态块常驻底部（不在滚动容器内）', async () => {
    const wrapper = await mountRadar([row()])
    const column = wrapper.find('.rd-rail-col')
    const scroll = column.find('.rd-rail-scroll')
    const statusBlock = wrapper.find('[data-testid="radar-status-block"]')

    expect(column.exists()).toBe(true)
    expect(scroll.exists()).toBe(true)
    expect(statusBlock.exists()).toBe(true)
    // 内容区（泳道清单）在滚动段里
    expect(scroll.find('[aria-label="今日阅读与线索"]').exists()).toBe(true)
    // 状态块在滚动段**之外**，是滚动段的兄弟节点 → 不随内容滚动
    expect(scroll.find('[data-testid="radar-status-block"]').exists()).toBe(false)
    expect(scroll.element.nextElementSibling).toBe(statusBlock.element)
  })

  it('首次取数失败：阅读内容段不渲染，状态块仍常驻（默认折叠，摘要行用「读取中」表达未知态）', async () => {
    const wrapper = await mountRadar([], false)
    expect(wrapper.find('[aria-label="今日阅读与线索"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-status-block"]').exists()).toBe(true)
    // 默认折叠：五项事实不渲染，摘要行不把未知当业务结论（R-17）
    expect(wrapper.find('[data-testid="radar-status-queue"]').exists()).toBe(false)
    const summary = wrapper.find('[data-testid="radar-status-summary"]')
    expect(summary.exists()).toBe(true)
    expect(summary.text()).toContain('读取中')
    expect(wrapper.text()).not.toContain('今日日报未生成')
  })
})

describe('BoardRadarView — 8A.1 报刊式总览（已批准原型）', () => {
  it('关注板块横比一行：序号/标题/日报元信息/小矩阵/关注强度/3 日线索变化/进入', async () => {
    const wrapper = await mountRadar([row()])
    const story = wrapper.find('[data-testid="radar-row-1974"]')
    expect(story.classes()).toContain('is-warm')
    expect(story.find('.rd-rank').text()).toBe('01')
    expect(story.find('.rd-story-title').text()).toBe('中东地缘政治与美伊关系')
    expect(story.find('.rd-story-report').text()).toContain('伊朗警告打击海湾海底电缆')
    expect(story.find('.rd-story-meta').text()).toContain('10/24')
    expect(story.find('.rd-story-meta').text()).toContain('14 篇文章 · 13 个事件')
    expect(story.find('.rd-attention').text()).toBe('3.5')
    expect(story.find('.rd-tier').text()).toBe('关注强度 · 关注')
    expect(story.find('.rd-trend').text()).toContain('+30%')
    expect(story.find('.rd-trend').text()).toContain('近 3 日 13 / 前 3 日 10')
    expect(story.find('.rd-trend').text()).toContain('今日 · 当日线索 8')
    expect(story.find('.rd-enter').text()).toContain('矩阵')
    expect(wrapper.find('.rd-labels').text()).toContain('泳道活跃 · 每格线索数')
  })

  it('逐泳道 14 日小矩阵：格值 = 当日线索合计，title 带「N 条线索」；泳道名/格子进 ?lane=', async () => {
    const wrapper = await mountRadar([row()])
    const cells = wrapper.findAll('[data-testid="radar-row-1974"] .rd-mini-lane:first-child .rd-mini-cell')
    expect(cells).toHaveLength(14)
    // 今天格 = 3 条线索（clue_counts[13]）
    expect(cells[13]!.attributes('title')).toContain('3 条线索')
    expect(cells[13]!.attributes('title')).not.toContain('事件线')
    expect(cells[13]!.attributes('title')).not.toContain('个分区')
    await cells[13]!.trigger('click')
    expect(push).toHaveBeenCalledWith({ path: '/tags/1974', query: { lane: '1' } })

    await wrapper.find('[data-testid="radar-row-1974"] .rd-mini-name').trigger('click')
    expect(push).toHaveBeenLastCalledWith({ path: '/tags/1974', query: { lane: '1' } })
  })

  it('小矩阵颜色通道退役（D5）：只有「有线索/无线索」两态底色，不构成数值色阶', async () => {
    const wrapper = await mountRadar([row()])
    const cells = wrapper.findAll('[data-testid="radar-row-1974"] .rd-mini-cell')
    // 今天有线索 → is-active；其余 0 → 无档位类
    expect(cells[13]!.classes()).toContain('is-active')
    expect(cells[12]!.classes()).not.toContain('is-active')
    for (const retired of ['b1', 'b2', 'b3', 'b4', 'd1', 'd2', 'd3', 'd4', 'is-degraded']) {
      expect(cells[13]!.classes()).not.toContain(retired)
      expect(cells[12]!.classes()).not.toContain(retired)
    }
  })

  it('旧后端缺 clue_counts：格子标「线索数未知」，不造 0、不拿分区数冒充', async () => {
    const wrapper = await mountRadar([row({
      lanes: [{ topicId: '1', label: '美伊博弈', threadCount14d: 40, clueCounts: null }],
    })])
    const cells = wrapper.findAll('[data-testid="radar-row-1974"] .rd-mini-lane:first-child .rd-mini-cell')
    expect(cells).toHaveLength(14)
    for (const cell of cells) {
      expect(cell.attributes('title')).toContain('线索数未知')
      expect(cell.attributes('title')).not.toContain('0 条线索')
      expect(cell.attributes('title')).not.toContain('个分区')
      expect(cell.attributes('data-known')).toBe('false')
    }
  })

  it('排序不因分区/线索数反转：attention（Σ 泳道当天热度）高的在前', async () => {
    const wrapper = await mountRadar([
      row({ boardId: 'a', label: '线索多热度低', attention: 5, todayClues: 40 }),
      row({ boardId: 'b', label: '线索少热度高', attention: 20, todayClues: 2 }),
    ])
    expect(wrapper.findAll('[data-testid^="radar-row-"]').map(node => node.attributes('data-testid')))
      .toEqual(['radar-row-b', 'radar-row-a'])
  })

  it('数据延迟与锚日截止可辨：标注「数据延迟 · 截至 MM/DD」与当日线索口径', async () => {
    const wrapper = await mountRadar([row({ anchorDate: '2026-10-22' })])
    const story = wrapper.find('[data-testid="radar-row-1974"]')
    expect(story.find('.rd-story-meta').text()).toContain('数据延迟 · 截至 10/22')
    expect(story.find('.rd-trend').text()).toContain('10/22 · 当日线索 8')
  })

  it('3 日线索环比：last3Clues / prev3Clues 决定百分比；缺字段（旧后端）→ 「—」不造 0', async () => {
    const wrapper = await mountRadar([
      row({ boardId: '1', last3Clues: 4, prev3Clues: 2 }),
      row({ boardId: '2', last3Clues: null, prev3Clues: null }),
    ])
    expect(wrapper.find('[data-testid="radar-row-1"] .rd-trend').text()).toContain('+100%')
    expect(wrapper.find('[data-testid="radar-row-1"] .rd-trend').text()).toContain('近 3 日 4 / 前 3 日 2')
    const legacy = wrapper.find('[data-testid="radar-row-2"] .rd-trend')
    expect(legacy.text()).toContain('—')
    expect(legacy.text()).toContain('近 3 日 — / 前 3 日 —')
    expect(legacy.text()).not.toContain('NaN')
  })

  it('无活跃泳道但有日报：行内说明仍可读，日报按钮进阅读台', async () => {
    const wrapper = await mountRadar([row({ lanes: [], clueDays: [] })])
    const story = wrapper.find('[data-testid="radar-row-1974"]')
    expect(story.find('.rd-mini').text()).toContain('无活跃泳道 · 已有日报仍可读')
    await story.find('.rd-story-report').trigger('click')
    expect(push).toHaveBeenCalledWith('/tags/1974/2026-10-24')
  })

  it('一次请求渲染雷达：挂载只调一次 overview（RA-8）', async () => {
    await mountRadar([row()])
    expect(getOverview).toHaveBeenCalledTimes(1)
  })

  it('有旧数据时刷新失败：错误条与旧数据并存不混，不白屏', async () => {
    const wrapper = await mountRadar([row()])
    expect(wrapper.find('[data-testid="radar-row-1974"]').exists()).toBe(true)
    // 新建版块后的重拉失败：真实二次加载路径
    createBoard.mockResolvedValue({ success: true, data: { id: 78 } })
    getOverview.mockResolvedValue({ success: false, data: undefined, error: '刷新失败' })
    await wrapper.find('[data-testid="radar-add-board"]').trigger('click')
    await wrapper.findComponent({ name: 'AddSemanticBoardDialog' }).vm.$emit('confirm', {
      label: '新板块', description: '', display_order: 0, protected: false, auxiliary_labels: [],
    })
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('刷新失败')
    expect(wrapper.find('[data-testid="radar-row-1974"]').exists()).toBe(true)
  })

  it('超长标签不硬裁剪：标题 title 携带全文供悬浮/辅助读全', async () => {
    const longLabel = '超长版块名称测试'.repeat(12)
    const wrapper = await mountRadar([row({ label: longLabel })])
    expect(wrapper.find('.rd-story-title').attributes('title')).toBe(longLabel)
  })

  it('报头：标题句带关注数与热点数 + 今日线索统计，全局治理菜单保留', async () => {
    const wrapper = await mountRadar([
      row({ attention: 20, todayClues: 8 }),
      row({ boardId: '4', label: '关注版块', attention: 5, todayClues: 4 }),
    ])
    expect(wrapper.find('.rd-headline').text()).toBe('2 个板块值得关注，其中 1 个达到热点阈值')
    expect(wrapper.find('.rd-meta').text()).toContain('今日线索 12')
    expect(wrapper.find('.rd-meta').text()).not.toContain('升温')
    for (const id of ['radar-more-upgrade', 'radar-more-backfill', 'radar-more-matching', 'radar-more-merge', 'radar-more-labels']) {
      expect(wrapper.find(`[data-testid="${id}"]`).exists()).toBe(true)
    }
  })
})

describe('BoardRadarView — 首次失败不混业务空态（R-17）', () => {
  it('首次取数失败：错误条 + 中性报头，不出现业务空态与初始化 0 结论', async () => {
    const wrapper = await mountRadar([], false)
    expect(wrapper.find('[role="alert"]').text()).toContain('接口挂了')
    // 业务空态三要素不得出现
    expect(wrapper.text()).not.toContain('今天没有需要看的版块')
    expect(wrapper.text()).not.toContain('日报生成后')
    expect(wrapper.text()).not.toContain('新建第一个版块')
    expect(wrapper.find('[data-testid="radar-empty"]').exists()).toBe(false)
    // 报头不把初始化 0 呈现为业务结论
    expect(wrapper.find('.rd-headline').text()).toBe('今日雷达')
    expect(wrapper.text()).not.toContain('0 个板块值得关注')
    expect(wrapper.find('.rd-meta').exists()).toBe(false)
    expect(wrapper.text()).toContain('总览读取失败，重试后恢复今日雷达')
    // 阅读右栏不渲染：失败态不混「今天没有可排的泳道 / 今天还没有日报」业务空结论
    expect(wrapper.find('[aria-label="今日阅读与线索"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('今天还没有日报')
    expect(wrapper.text()).not.toContain('今天没有可排的泳道')
  })

  it('成功返回空数组：真实 0 才显示业务空态、统计与阅读右栏空态文案', async () => {
    const wrapper = await mountRadar([])
    expect(wrapper.find('[data-testid="radar-empty"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('今天没有需要看的版块')
    expect(wrapper.text()).toContain('0 个板块值得关注')
    expect(wrapper.find('.rd-meta').exists()).toBe(true)
    // 真实空集：右栏照常渲染（旧响应无 top_lanes → 版块级列表空态是诚实结论）
    expect(wrapper.find('[aria-label="今日阅读与线索"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('今天还没有日报')
  })

  it('有旧数据刷新失败：阅读右栏保留旧值可读（R-17 State Matrix）', async () => {
    const wrapper = await mountRadar([row()])
    createBoard.mockResolvedValue({ success: true, data: { id: 78 } })
    getOverview.mockResolvedValue({ success: false, data: undefined, error: '刷新失败' })
    await wrapper.find('[data-testid="radar-add-board"]').trigger('click')
    await wrapper.findComponent({ name: 'AddSemanticBoardDialog' }).vm.$emit('confirm', {
      label: '新板块', description: '', display_order: 0, protected: false, auxiliary_labels: [],
    })
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('刷新失败')
    expect(wrapper.find('[aria-label="今日阅读与线索"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-read-1974"]').exists()).toBe(true)
  })
})
