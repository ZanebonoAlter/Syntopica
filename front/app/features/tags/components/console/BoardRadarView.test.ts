/**
 * BoardRadarView 测试（rework-workshop-into-radar-and-desk 8.1/8.3 + 8A.1/8A.2 + R-17/R-18）。
 * 覆盖：加载骨架 / 分档标记 / 常温紧凑索引与完整横比 / 今日该读 / 升温判定 /
 * 空态 / 错误重试 / 点行进 L1 / 泳道小矩阵进 ?lane= / 无泳道有日报 / 数据延迟可辨 /
 * 首次失败不混业务空态。8A.1 报刊层级继承自已批准总览原型（ui-design approved），
 * 旧阈值/排序/边界断言全部保留；数据喂归一化读模型（camelCase/字符串 ID），
 * raw→读模型接线在 api/normalizers/boardRadar.test.ts 与 semanticBoards.test.ts 验证。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BoardRadarView from './BoardRadarView.vue'
import type { RadarBoard, RadarStoryFeed, RadarStoryItem } from '~/api/semanticBoards'
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
      matchingConfig: ref(null),
      matchingConfigLoading: ref(false),
      backfillTask: ref(null),
      handleUpgradeSuggest: vi.fn(),
      loadPersistedSuggestions: vi.fn(),
      handleGenerateUpgradeSuggestions: vi.fn(),
      handleDismissUpgradeRow: vi.fn(),
      handleConfirmUpgradeRow: vi.fn(),
      handleTriggerBackfill: vi.fn(),
      handleOpenMatchingConfig: vi.fn(),
      handleSaveMatchingConfig: vi.fn(),
      handleMergeComplete: vi.fn(),
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
    todaySections: 8,
    last3Sections: 13,
    prev3Sections: 10,
    attention: 2,
    dataDelayed: false,
    days: Array.from({ length: 14 }, (_, i) => (i === 13 ? 8 : 1)),
    todayStories: 8,
    last3Stories: 13,
    prev3Stories: 10,
    storiesBackfilled: true,
    todayHeat: 0,
    last3Heat: 0,
    prev3Heat: 0,
    heatDays: [],
    heatRuleVersion: '',
    heatIncomplete: false,
    lanes: [
      { topicId: '1', label: '美伊博弈', sectionCount14d: 15, threadCount14d: 40, counts: Array.from({ length: 14 }, () => 1), storyCounts: Array.from({ length: 14 }, () => 1), storyCountsKnown: true },
      { topicId: '2', label: '沙特', sectionCount14d: 14, threadCount14d: 27, counts: Array.from({ length: 14 }, () => 1), storyCounts: Array.from({ length: 14 }, () => 1), storyCountsKnown: true },
      { topicId: '3', label: '能源', sectionCount14d: 9, threadCount14d: 17, counts: Array.from({ length: 14 }, () => 1), storyCounts: Array.from({ length: 14 }, () => 1), storyCountsKnown: true },
    ],
    ...overrides,
  }
}

async function mountRadar(boards: RadarBoard[], success = true, stories?: RadarStoryFeed | null) {
  getOverview.mockResolvedValue({ success, data: success ? { date: '2026-10-24', boards, stories: stories ?? null } : undefined, error: success ? undefined : '接口挂了' })
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

/** 新起条目夹具（与后端 RadarStoryItem 同形状，可批量造）。 */
function newStoryItem(storyId: string): RadarStoryItem {
  return {
    boardId: '3000',
    boardLabel: '能源价格',
    storyId,
    storyTitle: `原油出口中断 ${storyId}`,
    participants: 1,
    heat: 2,
    baselineHeat: 0,
    ratio: 0,
    hasBaseline: false,
    hasTodayEvidence: true,
    anchorDate: '2026-10-24',
    reportDate: '2026-10-24',
  }
}

/** 事件线清单段夹具（新版后端响应形状）。 */
function storyFeed(over: Partial<RadarStoryFeed> = {}): RadarStoryFeed {
  return {
    anchorDate: '2026-10-24',
    heatRuleVersion: 'heat-v1-48h-hl24h',
    topStories: [{
      boardId: '1974',
      boardLabel: '中东地缘政治与美伊关系',
      storyId: '4211',
      storyTitle: '美伊博弈升级',
      participants: 4,
      heat: 5.0123,
      baselineHeat: 1.2,
      ratio: 4.1769,
      hasBaseline: true,
      hasTodayEvidence: true,
      anchorDate: '2026-10-23',
      reportDate: '2026-10-24',
    }],
    // matrix-clue-fallback-and-rail-merge：单清单 + 徽标——升温/新起卡片与
    // new_count/new_total_count 随徽标化移除；夹具的 topStories[0] 自带
    // has_baseline/ratio（升温徽标形态），newStoryItem() 造无基线形态。
    warmingCount: 6,
    crossDayStoryCount: 1,
    unbackfilledBoardIds: ['12'],
    unbackfilledBoardCount: 1,
    noStoryBoardIds: ['77'],
    noStoryBoardCount: 1,
    laggingBoardIds: ['3122'],
    partial: false,
    degradedBoardIds: [],
    ...over,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
})

describe('BoardRadarView — 事件线清单右栏（surface-story-events-in-radar）', () => {
  it('「今日该读」渲染事件线（版块名 + 事件线标题 + 来源数 + 热度 + 锚点日），不再拿日报标题当条目', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed())
    const item = wrapper.find('[data-testid="radar-story-4211"]')
    expect(item.exists()).toBe(true)
    expect(item.text()).toContain('中东地缘政治与美伊关系')
    expect(item.text()).toContain('美伊博弈升级')
    expect(item.text()).toContain('4 个来源')
    expect(item.text()).toContain('热度 5.0')
    // 条目显式带该版块自己的锚点日（滞后版块不得冒充全局日）
    expect(item.text()).toContain('10/23')
    // 旧版块级日报标题条目不再出现在右栏（版块行的「最新日报标题」入口保持不变）
    expect(wrapper.find('[data-testid="radar-read-1974"]').exists()).toBe(false)
    expect(wrapper.find('.rd-rail').text()).not.toContain('伊朗警告打击海湾海底电缆')
  })

  it('事件线条目点击 → 跳落点报告日并带 ?story= 参数', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed())
    await wrapper.find('[data-testid="radar-story-4211"]').trigger('click')
    expect(push).toHaveBeenCalledWith({ path: '/tags/1974/2026-10-24', query: { story: '4211' } })
  })

  it('单清单徽标化：升温倍率挂事件线行内（不挂版块名）；升温/新起独立卡片不再渲染', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed({
      topStories: [storyFeed().topStories[0]!, newStoryItem('4300')],
    }))
    // 升温徽标挂在事件线条目行内（红线 29：倍率挂事件线、不挂版块名）
    const warmBadge = wrapper.find('[data-testid="radar-badge-warm-4211"]')
    expect(warmBadge.exists()).toBe(true)
    expect(warmBadge.text()).toContain('升温')
    expect(warmBadge.text()).toContain('4.2×')
    expect(warmBadge.text()).not.toContain('中东地缘政治与美伊关系')
    expect(warmBadge.text()).not.toContain('版块')
    // 新起徽标：无基线条目行内「新起」，不显示倍率
    const newBadge = wrapper.find('[data-testid="radar-badge-new-4300"]')
    expect(newBadge.exists()).toBe(true)
    expect(newBadge.text()).toContain('新起')
    expect(newBadge.text()).not.toContain('×')
    // 独立升温卡片与「今日新起」卡片不再渲染（单清单结构）
    expect(wrapper.find('[data-testid="radar-new-block"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-new-4300"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-warm-4211"]').exists()).toBe(false)
  })

  it('徽标条目保持完整可读：无基线条目带来源数与热度 + 版块名 + 锚点日（红线 29），不显示倍率', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed({
      topStories: [newStoryItem('4300')],
    }))
    const item = wrapper.find('[data-testid="radar-story-4300"]')
    expect(item.exists()).toBe(true)
    expect(item.text()).toContain('能源价格')
    expect(item.text()).toContain('1 个来源')
    expect(item.text()).toContain('热度 2.0')
    expect(item.text()).toContain('10/24')
    // 无基线 → 不得出现倍率（0.0× / ∞ 都不行）
    expect(item.find('[data-testid="radar-badge-new-4300"]').text()).not.toContain('×')
    expect(item.text()).not.toContain('倍率')
  })

  it('无徽标 = 普通当日事件线：有基线但 ratio < 2 的条目行内无任何徽标', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed({
      topStories: [{
        boardId: '1974', boardLabel: '中东地缘政治与美伊关系', storyId: '5000',
        storyTitle: '平稳延续的事件线', participants: 3, heat: 3.0, baselineHeat: 2.0,
        ratio: 1.5, hasBaseline: true, hasTodayEvidence: true,
        anchorDate: '2026-10-24', reportDate: '2026-10-24',
      }],
    }))
    expect(wrapper.find('[data-testid="radar-story-5000"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-badge-new-5000"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-badge-warm-5000"]').exists()).toBe(false)
    // 普通当日事件线不带残热标记（有当日报道证据）
    expect(wrapper.find('[data-testid="radar-no-evidence-5000"]').exists()).toBe(false)
  })

  it('残热旧线（锚点日无报道证据）：两徽标都不出，meta 行标「今日无报道证据」（review R-7）', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed({
      topStories: [{
        boardId: '1974', boardLabel: '中东地缘政治与美伊关系', storyId: '5100',
        storyTitle: '只剩 48h 残热的旧线', participants: 2, heat: 1.8, baselineHeat: 1.5,
        ratio: 1.2, hasBaseline: true, hasTodayEvidence: false,
        anchorDate: '2026-10-24', reportDate: '2026-10-24',
      }],
    }))
    expect(wrapper.find('[data-testid="radar-story-5100"]').exists()).toBe(true)
    // 判据未变：无当日证据 → 既不「新起」也不「升温」（红线 29）
    expect(wrapper.find('[data-testid="radar-badge-new-5100"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-badge-warm-5100"]').exists()).toBe(false)
    // 但行内如实标一句：与「普通当日事件线」区分开
    const mark = wrapper.find('[data-testid="radar-no-evidence-5100"]')
    expect(mark.exists()).toBe(true)
    expect(mark.text()).toContain('今日无报道证据')
  })

  it('单清单 8 条一次呈现（上限由后端截断，前端不另设屏高逻辑），无「另有 N 条」类摘要', async () => {
    const top = Array.from({ length: 8 }, (_, i) => newStoryItem(String(4301 + i)))
    const wrapper = await mountRadar([row()], true, storyFeed({ topStories: top }))
    const items = wrapper.findAll('[data-testid^="radar-story-"]')
    expect(items).toHaveLength(8)
    expect(wrapper.text()).not.toContain('另有')
    expect(wrapper.text()).not.toContain('均已列入')
    expect(wrapper.text()).not.toContain('今天没有新起的事件线')
  })

  it('滞后版块与降级计数明示（design D2：N 个版块滞后；降级提示带上数量）', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed({ partial: true, degradedBoardIds: ['9', '10'] }))
    const lagging = wrapper.find('[data-testid="radar-lagging"]')
    expect(lagging.exists()).toBe(true)
    expect(lagging.text()).toContain('1 个版块数据滞后')
    expect(lagging.text()).toContain('锚点日')
    const partial = wrapper.find('[data-testid="radar-partial"]')
    expect(partial.text()).toContain('排序不完整')
    expect(partial.text()).toContain('2 个版块')
  })

  it('未回填版块明示「事件身份未回填，未纳入事件线排序」（不得写成「今天没有事件」）', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed({ unbackfilledBoardCount: 1 }))
    expect(wrapper.find('[data-testid="radar-unbackfilled"]').text()).toContain('事件身份未回填')
    expect(wrapper.find('[data-testid="radar-unbackfilled"]').text()).toContain('未纳入事件线排序')
    expect(wrapper.text()).not.toContain('今天没有事件')
  })

  it('部分版块延迟明示「本次事件线排序不完整」', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed({ partial: true }))
    expect(wrapper.find('[data-testid="radar-partial"]').text()).toContain('部分版块数据延迟')
    expect(wrapper.find('[data-testid="radar-partial"]').text()).toContain('排序不完整')
  })

  it('无正文落点的条目不渲染为可点（静态行 + 提示）', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed({
      topStories: [{ ...storyFeed().topStories[0]!, reportDate: '' }],
    }))
    const item = wrapper.find('[data-testid="radar-story-4211"]')
    expect(item.element.tagName).toBe('DIV')
    expect(item.text()).toContain('该事件线暂无正文落点')
    await item.trigger('click')
    expect(push).not.toHaveBeenCalled()
  })

  it('旧响应（无 stories 段）回退版块级日报标题列表与版块级升温清单，不渲染业务空态', async () => {
    const wrapper = await mountRadar([row({ todayStories: 3, todayHeat: 6, last3Heat: 6, last3Stories: 6 })], true, null)
    expect(wrapper.find('[data-testid="radar-read-1974"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-read-1974"]').text()).toContain('伊朗警告打击海湾海底电缆')
    expect(wrapper.find('.rd-warm').text()).toContain('3.0×')
    expect(wrapper.find('[data-testid="radar-no-story"]').exists()).toBe(false)
  })
})

describe('BoardRadarView — 雷达列表', () => {
  it('热点行分「热点」档（lane_heat 标定阈值 ≥3.3），常温折叠不展开；点行进 L1', async () => {
    const wrapper = await mountRadar([
      row({ boardId: '1', label: '热点版块', attention: 12 }),
      row({ boardId: '2', label: '关注版块', attention: 2 }),
      row({ boardId: '3', label: '常温版块', attention: 0.6, todaySections: 0, last3Sections: 0, prev3Sections: 0 }),
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

  it('今日该读取最新日报元信息；无升温显示明确文案（8.3）', async () => {
    const wrapper = await mountRadar([row()])
    const read = wrapper.find('[data-testid="radar-read-1974"]')
    expect(read.text()).toContain('伊朗警告打击海湾海底电缆')
    expect(read.text()).toContain('10/24')
    // workshop-radar §28：事件数 = 当日事件线数（todayStories），非 reportEventCount
    expect(read.text()).toContain('14 篇文章 · 8 个事件')
    expect(read.text()).not.toContain('未回填上界')
    expect(wrapper.find('[data-testid="radar-no-warm"]').text()).toContain('暂未触发升温条件')
  })

  // ── 跨天线脚注与单清单不重复（matrix-clue-fallback-and-rail-merge D3）────

  it('无跨天延续事件线时：清单下方一行脚注「跨天延续建设中」，不占卡片、不说「暂未触发升温条件」', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed({ warmingCount: 0, crossDayStoryCount: 0 }))
    const note = wrapper.find('[data-testid="radar-crossday-note"]')
    expect(note.exists()).toBe(true)
    expect(note.text()).toContain('跨天延续')
    expect(note.text()).toContain('建设中')
    // 旧升温卡片的空态措辞与卡片本身都不再出现
    expect(wrapper.find('[data-testid="radar-no-warm"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('暂未触发升温条件')
  })

  it('有跨天线但未达 2 倍：无任何升温空态或脚注（清单徽标已表达）', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed({ warmingCount: 0, crossDayStoryCount: 3 }))
    expect(wrapper.find('[data-testid="radar-crossday-note"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-no-warm"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('暂未触发升温条件')
    expect(wrapper.text()).not.toContain('没有跨天延续的事件线')
  })

  it('旧响应缺 cross_day_story_count：不出脚注、不断言成因（无法判定 ≠ 没有跨天线）', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed({ crossDayStoryCount: null }))
    expect(wrapper.find('[data-testid="radar-crossday-note"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('没有跨天延续的事件线')
    expect(wrapper.text()).not.toContain('暂未触发升温条件')
  })

  it('右栏不重复条目：同一事件线只出现一次（徽标承载分档，无双档重复渲染）', async () => {
    const wrapper = await mountRadar([row()], true, storyFeed({
      topStories: [storyFeed().topStories[0]!, newStoryItem('4300')],
    }))
    // 4211 带升温徽标、4300 带新起徽标，各只出现一次；右栏无第二块清单
    expect(wrapper.findAll('[data-testid="radar-story-4211"]')).toHaveLength(1)
    expect(wrapper.findAll('[data-testid="radar-story-4300"]')).toHaveLength(1)
    expect(wrapper.find('[data-testid="radar-new-block"]').exists()).toBe(false)
    // 该读条目总数 = 服务端 top_stories 数（无第二档叠加）
    expect(wrapper.findAll('[data-testid^="radar-story-"]')).toHaveLength(2)
  })

  it('今日该读事件数三态（workshop-radar §28）：未回填展示事件线上界+标注降级；老 payload 缺字段回落日报事件数且无标注', async () => {
    const wrapper = await mountRadar([
      row({ boardId: '2', label: '未回填版块', attention: 20, storiesBackfilled: false, todayStories: 6 }),
      row({ boardId: '3', label: '老载荷版块', attention: 10, storiesBackfilled: null, todayStories: 8 }),
    ])
    const degraded = wrapper.find('[data-testid="radar-read-2"]')
    expect(degraded.text()).toContain('14 篇文章 · 6 个事件（未回填上界）')
    const legacy = wrapper.find('[data-testid="radar-read-3"]')
    expect(legacy.text()).toContain('14 篇文章 · 13 个事件')
    expect(legacy.text()).not.toContain('未回填上界')
  })

  it('升温：热度口径（todayHeat ≥ 近3日日均热度 2 倍且 todayStories ≥3）显示倍数与事件线文案', async () => {
    const wrapper = await mountRadar([
      row({ boardId: '4', label: '升温版块', todayStories: 8, todayHeat: 8, last3Stories: 9, last3Heat: 9, prev3Heat: 3 }),
    ])
    expect(wrapper.find('.rd-warm').text()).toContain('2.7×')
    expect(wrapper.text()).toContain('仍属关注档，不是热点判定')
    expect(wrapper.text()).toContain('8 个当日事件线 / 近 3 日均值 3.0')
    expect(wrapper.text()).toContain('当日事件线 ≥3，且事件线热度达到近 3 日均值的 2 倍')
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
    const wrapper = await mountRadar([row({ boardId: '5', attention: 0.6, todaySections: 0, last3Sections: 0, prev3Sections: 0, lanes: [] })])
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
    getOverview.mockResolvedValue({ success: true, data: { date: '2026-10-24', boards: [row()] } })
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
    // 内容区（今日该读 / 今天可能升温）在滚动段里
    expect(scroll.find('[aria-label="今日阅读与升温"]').exists()).toBe(true)
    // 状态块在滚动段**之外**，是滚动段的兄弟节点 → 不随内容滚动
    expect(scroll.find('[data-testid="radar-status-block"]').exists()).toBe(false)
    expect(scroll.element.nextElementSibling).toBe(statusBlock.element)
  })

  it('首次取数失败：阅读内容段不渲染，状态块仍常驻（默认折叠，摘要行用「读取中」表达未知态）', async () => {
    const wrapper = await mountRadar([], false)
    expect(wrapper.find('[aria-label="今日阅读与升温"]').exists()).toBe(false)
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
  it('关注板块横比一行：序号/标题/日报元信息/小矩阵/关注强度/3 日变化/进入', async () => {
    const wrapper = await mountRadar([row()])
    const story = wrapper.find('[data-testid="radar-row-1974"]')
    expect(story.classes()).toContain('is-warm')
    expect(story.find('.rd-rank').text()).toBe('01')
    expect(story.find('.rd-story-title').text()).toBe('中东地缘政治与美伊关系')
    expect(story.find('.rd-story-report').text()).toContain('伊朗警告打击海湾海底电缆')
    expect(story.find('.rd-story-meta').text()).toContain('10/24')
    expect(story.find('.rd-story-meta').text()).toContain('14 篇文章 · 13 个事件')
    expect(story.find('.rd-attention').text()).toBe('2.0')
    expect(story.find('.rd-tier').text()).toBe('关注强度 · 关注')
    expect(story.find('.rd-trend').text()).toContain('+30%')
    expect(story.find('.rd-trend').text()).toContain('近 3 日 13 / 前 3 日 10')
    expect(story.find('.rd-trend').text()).toContain('今日 · 当日事件线数 8')
    expect(story.find('.rd-enter').text()).toContain('矩阵')
    expect(wrapper.find('.rd-labels').text()).toContain('泳道活跃 · 每格事件线数')
  })

  it('旧后端（未下发 story_counts）：回退的分区数按「N 个分区」措辞，不得冒充事件线数（R-2）', async () => {
    const counts = Array.from({ length: 14 }, () => 1)
    const wrapper = await mountRadar([row({
      lanes: [{
        topicId: '1',
        label: '美伊博弈',
        sectionCount14d: 15,
        threadCount14d: 40,
        counts,
        storyCounts: counts,
        // normalizer 在 raw 缺 story_counts 时置 false（回退值是分区数）
        storyCountsKnown: false,
      }],
    })])
    const cells = wrapper.findAll('[data-testid="radar-row-1974"] .rd-mini-cell')
    expect(cells).toHaveLength(14)
    expect(cells[0]!.attributes('title')).toContain('1 个分区')
    expect(cells[0]!.attributes('title')).not.toContain('条事件线')
    expect(cells[0]!.attributes('title')).not.toContain('无事件线')
    expect(cells[0]!.attributes('aria-label')).toContain('1 个分区')
  })

  it('逐泳道 14 日小矩阵：泳道名/格子进 ?lane=，格子 title 带事件线数（storyCounts 口径）', async () => {
    const wrapper = await mountRadar([row()])
    const laneButton = wrapper.find('[data-testid="radar-row-1974"] .rd-mini-name')
    expect(laneButton.attributes('title')).toBe('美伊博弈')
    await laneButton.trigger('click')
    expect(push).toHaveBeenCalledWith({ path: '/tags/1974', query: { lane: '1' } })

    const cell = wrapper.find('[data-testid="radar-row-1974"] .rd-mini-cell')
    expect(cell.attributes('title')).toContain('条事件线')
    await cell.trigger('click')
    expect(push).toHaveBeenLastCalledWith({ path: '/tags/1974', query: { lane: '1' } })
  })

  it('mini-matrix 逐格降级：事件线 0 而分区数 > 0 的格子按分区数兜底显示，title 明示降级（不冒充事件线数）', async () => {
    const wrapper = await mountRadar([row({
      lanes: [{
        topicId: '1',
        label: '美伊博弈',
        sectionCount14d: 15,
        threadCount14d: 40,
        // 历史未回填日：counts（分区数）= 1，storyCounts（事件线）= 0
        counts: Array.from({ length: 14 }, (_, i) => (i === 13 ? 4 : 1)),
        storyCounts: Array.from({ length: 14 }, (_, i) => (i === 13 ? 4 : 0)),
        storyCountsKnown: true,
      }],
    })])
    const cells = wrapper.findAll('[data-testid="radar-row-1974"] .rd-mini-cell')
    expect(cells).toHaveLength(14)
    // 末日 4 条事件线 → 最高档 b4、title 正常口径
    expect(cells[13]!.classes()).toContain('b4')
    expect(cells[13]!.attributes('title')).toContain('4 条事件线')
    // 历史未回填日：0 条事件线但分区数 1 → 格子不再空白（不再是 b0），改走降级专属
    // 代理色阶（d 档 + is-degraded 形态标记；基准 = 行内分区数最大值），title 明示降级
    expect(cells[12]!.classes()).not.toContain('b0')
    expect(cells[12]!.classes()).toContain('is-degraded')
    expect(cells[12]!.classes()).toContain('d4')
    expect(cells[12]!.classes()).not.toContain('b1')
    expect(cells[12]!.attributes('title')).toContain('无事件线')
    expect(cells[12]!.attributes('title')).toContain('按分区数 1 显示')
    expect(cells[12]!.attributes('title')).not.toContain('1 条事件线')
    expect(cells[12]!.attributes('aria-label')).toContain('无事件线')
  })

  it('mini-matrix 真断档（事件线 0 且分区 0）不触发降级标注（0 分区兜底仍是 0）', async () => {
    const wrapper = await mountRadar([row({
      lanes: [{
        topicId: '1',
        label: '美伊博弈',
        sectionCount14d: 15,
        threadCount14d: 40,
        counts: Array.from({ length: 14 }, () => 0),
        storyCounts: Array.from({ length: 14 }, () => 0),
        storyCountsKnown: true,
      }],
    })])
    const cells = wrapper.findAll('[data-testid="radar-row-1974"] .rd-mini-cell')
    expect(cells[0]!.classes()).toContain('b0')
    expect(cells[0]!.attributes('title')).not.toContain('按分区数')
    expect(cells[0]!.attributes('title')).not.toContain('降级')
  })

  it('mini-matrix 降级格走独立代理色阶：档位由兜底值决定、不占最深实心档、形态可辨', async () => {
    const wrapper = await mountRadar([row({
      lanes: [{
        topicId: '1',
        label: '美伊博弈',
        sectionCount14d: 15,
        threadCount14d: 40,
        // 前三天：事件线 0（未回填）而分区数 1/2/4 → 降级格；末日：10 条事件线（行内峰值）
        counts: [1, 2, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 10],
        storyCounts: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 10],
        storyCountsKnown: true,
      }],
    })])
    const cells = wrapper.findAll('[data-testid="radar-row-1974"] .rd-mini-cell')
    expect(cells).toHaveLength(14)
    // 降级格：d 档（独立代理色阶，基准 = 行内分区数最大值）+ is-degraded 形态标记；
    // 档位随兜底值 1/2/4 递增（不被行内事件线峰值 10 压平）
    expect(cells[0]!.classes()).toContain('is-degraded')
    expect(cells[0]!.classes()).toContain('d1')
    expect(cells[1]!.classes()).toContain('d2')
    expect(cells[2]!.classes()).toContain('d4')
    // 降级格不穿 b 档（不与已确证格同色阶），且永不占最深实心档 b4
    for (const index of [0, 1, 2]) {
      expect(cells[index]!.classes().some(name => /^b\d$/.test(name))).toBe(false)
      expect(cells[index]!.classes()).not.toContain('b4')
    }
    // 已确证格：按行内事件线最大值（10）分档 → b4，且不带降级形态标记
    expect(cells[13]!.classes()).toContain('b4')
    expect(cells[13]!.classes()).not.toContain('is-degraded')
    // 真断档日：仍是空档 b0（兜底值 0 不当降级）
    expect(cells[3]!.classes()).toContain('b0')
  })

  it('mini-matrix 降级格色阶除零边界：storyCounts 全 0 时不出现 NaN/Infinity', async () => {
    const wrapper = await mountRadar([row({
      lanes: [{
        topicId: '1',
        label: '美伊博弈',
        sectionCount14d: 15,
        threadCount14d: 40,
        // 事件线数全 0（未回填）而分区数有值：整行都是降级格
        counts: Array.from({ length: 14 }, (_, i) => (i === 13 ? 2 : 1)),
        storyCounts: Array.from({ length: 14 }, () => 0),
        storyCountsKnown: true,
      }],
    })])
    const cells = wrapper.findAll('[data-testid="radar-row-1974"] .rd-mini-cell')
    expect(cells).toHaveLength(14)
    for (const cell of cells) {
      expect(cell.classes().join(' ')).not.toMatch(/NaN|Infinity/)
      expect(cell.classes()).toContain('is-degraded')
    }
    // 代理值自己的行内最大值（2）定档：2 → 最深代理档，1 → 中档
    expect(cells[13]!.classes()).toContain('d4')
    expect(cells[0]!.classes()).toContain('d2')
  })

  it('mini-matrix 两数组全 0（整行真断档）：不崩、全部空档、无 NaN 色阶', async () => {
    const wrapper = await mountRadar([row({
      lanes: [{
        topicId: '1',
        label: '美伊博弈',
        sectionCount14d: 0,
        threadCount14d: 0,
        counts: Array.from({ length: 14 }, () => 0),
        storyCounts: Array.from({ length: 14 }, () => 0),
        storyCountsKnown: true,
      }],
    })])
    const cells = wrapper.findAll('[data-testid="radar-row-1974"] .rd-mini-cell')
    expect(cells).toHaveLength(14)
    for (const cell of cells) {
      expect(cell.classes()).toContain('b0')
      expect(cell.classes().join(' ')).not.toMatch(/NaN|Infinity/)
      expect(cell.attributes('title')).toContain('0 条事件线')
    }
  })

  it('排序不因 section 反转：attention（lane_heat）高的在前，分区多的不冒充（spec 场景）', async () => {
    const wrapper = await mountRadar([
      row({ boardId: 'a', label: '分区多热度低', attention: 2, todaySections: 8, todayStories: 8 }),
      row({ boardId: 'b', label: '分区少热度高', attention: 12, todaySections: 2, todayStories: 2 }),
    ])
    expect(wrapper.findAll('[data-testid^="radar-row-"]').map(node => node.attributes('data-testid')))
      .toEqual(['radar-row-b', 'radar-row-a'])
  })

  it('数据延迟与锚日截止可辨：标注「数据延迟 · 截至 MM/DD」与当日事件线口径', async () => {
    const wrapper = await mountRadar([row({ anchorDate: '2026-10-22' })])
    const story = wrapper.find('[data-testid="radar-row-1974"]')
    expect(story.find('.rd-story-meta').text()).toContain('数据延迟 · 截至 10/22')
    expect(story.find('.rd-trend').text()).toContain('10/22 · 当日事件线数 8')
  })

  it('热度环比：有 heat 字段时行内展示热度而非分区数', async () => {
    const wrapper = await mountRadar([row({
      todayHeat: 5.2,
      last3Heat: 4,
      prev3Heat: 2,
      todayStories: 6,
      last3Stories: 5,
    })])
    const story = wrapper.find('[data-testid="radar-row-1974"]')
    expect(story.find('.rd-trend').text()).toContain('近 3 日 4 / 前 3 日 2')
    expect(story.find('.rd-trend').text()).toContain('今日 · 当日事件线数 6')
    // 5.2 / (4/3) = 3.9 ≥2 且 todayStories 6 ≥3 → 升温行展示事件线均值与热度倍数
    expect(wrapper.find('.rd-warm').text()).toContain('3.9×')
    expect(wrapper.text()).toContain('6 个当日事件线 / 近 3 日均值 1.7')
  })

  it('无活跃泳道但有日报：行内说明仍可读，日报按钮进阅读台', async () => {
    const wrapper = await mountRadar([row({ lanes: [], days: [] })])
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

  it('报头：标题句带关注数与升温数，全局治理菜单保留', async () => {
    const wrapper = await mountRadar([
      row({ attention: 12 }),
      row({ boardId: '4', label: '升温版块', attention: 2, todayStories: 8, todayHeat: 8, last3Stories: 9, last3Heat: 9 }),
    ])
    expect(wrapper.find('.rd-headline').text()).toBe('2 个板块值得关注，1 处出现升温')
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
    // 阅读右栏不渲染：失败态不混「今天还没有日报/暂未触发升温条件」业务空结论（父实测 abort 场景）
    expect(wrapper.find('[aria-label="今日阅读与升温"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('今天还没有日报')
    expect(wrapper.text()).not.toContain('暂未触发升温条件')
  })

  it('成功返回空数组：真实 0 才显示业务空态、统计与阅读右栏空态文案', async () => {
    const wrapper = await mountRadar([])
    expect(wrapper.find('[data-testid="radar-empty"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('今天没有需要看的版块')
    expect(wrapper.text()).toContain('0 个板块值得关注')
    expect(wrapper.find('.rd-meta').exists()).toBe(true)
    // 真实空集：右栏照常渲染，空态文案是诚实结论
    expect(wrapper.find('[aria-label="今日阅读与升温"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('今天还没有日报')
    expect(wrapper.text()).toContain('暂未触发升温条件')
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
    expect(wrapper.find('[aria-label="今日阅读与升温"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-read-1974"]').exists()).toBe(true)
  })
})
