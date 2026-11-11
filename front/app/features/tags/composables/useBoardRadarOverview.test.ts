/**
 * useBoardRadarOverview 测试（rework-workshop-into-radar-and-desk 8A/R-18 拆分）。
 * 取数/分档/今日该读/升温/报头文案/首次失败语义与去向导航在 composable 层验证；
 * API 归一在 api 层测试，本层 mock useBoardRadarApi 只喂归一后读模型。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useBoardRadarOverview } from './useBoardRadarOverview'
import type { RadarBoard, RadarStoryFeed } from '~/api/semanticBoards'

const getOverview = vi.fn()
const push = vi.fn()

vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
vi.mock('~/api/semanticBoards', () => ({
  useBoardRadarApi: () => ({ getOverview }),
}))

function board(over: Partial<RadarBoard> = {}): RadarBoard {
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
    attention: 26,
    dataDelayed: false,
    days: Array.from({ length: 14 }, () => 1),
    lanes: [],
    todayStories: 0,
    last3Stories: 0,
    prev3Stories: 0,
    storiesBackfilled: null,
    todayHeat: 0,
    last3Heat: 0,
    prev3Heat: 0,
    heatDays: [],
    heatRuleVersion: '',
    heatIncomplete: false,
    ...over,
  }
}

async function setup(data: { date?: string, boards: RadarBoard[], stories?: RadarStoryFeed | null }, success = true) {
  getOverview.mockResolvedValue({
    success,
    data: success ? { date: data.date ?? '2026-10-24', boards: data.boards, stories: data.stories ?? null } : undefined,
    error: success ? undefined : '接口挂了',
  })
  const overview = useBoardRadarOverview()
  await overview.load()
  return overview
}

/** 事件线清单段夹具（新版后端响应形状）。 */
function feed(over: Partial<RadarStoryFeed> = {}): RadarStoryFeed {
  return {
    anchorDate: '2026-11-06',
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
      anchorDate: '2026-11-05',
      reportDate: '2026-11-06',
    }],
    // matrix-clue-fallback-and-rail-merge：单清单化——warming/newStories 派生
    // 与 newCount/newTotalCount 透出随徽标化移除；topStories 条目自带徽标字段。
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
})

describe('useBoardRadarOverview — 取数与分档', () => {
  it('分档：≥3.3 热点、≥0.7 关注、<0.7 常温（lane_heat 标定阈值）；可见行热点在前', async () => {
    const overview = await setup({
      boards: [
        board({ boardId: '2', label: '关注', attention: 0.7 }),
        board({ boardId: '1', label: '热点', attention: 3.3 }),
        board({ boardId: '3', label: '常温', attention: 0.6 }),
      ],
    })
    expect(overview.hotRows.value.map(row => row.boardId)).toEqual(['1'])
    expect(overview.warmRows.value.map(row => row.boardId)).toEqual(['2'])
    expect(overview.coldRows.value.map(row => row.boardId)).toEqual(['3'])
    expect(overview.visibleRows.value.map(row => row.boardId)).toEqual(['1', '2'])
    expect(overview.loading.value).toBe(false)
  })

  it('升温（热度口径，workshop-radar 契约）：todayStories ≥3 且 todayHeat/last3Heat ≥2 倍才列入', async () => {
    const overview = await setup({
      boards: [
        // 3 条事件线、热度 6.0 vs 日均 2.0 → 3× 入选
        board({ boardId: '4', todayStories: 3, todayHeat: 6, last3Heat: 6, last3Stories: 6, todaySections: 1, last3Sections: 30 }),
        // 事件线 <3 → 不入选（分区数再多也不冒充）
        board({ boardId: '5', todayStories: 2, todayHeat: 6, last3Heat: 6, todaySections: 8 }),
        // 事件线够但热度不达 2 倍 → 不入选
        board({ boardId: '6', todayStories: 4, todayHeat: 3, last3Heat: 6, todaySections: 8 }),
      ],
    })
    expect(overview.warming.value).toHaveLength(1)
    expect(overview.warming.value[0]!.row.boardId).toBe('4')
    expect(overview.warming.value[0]!.ratio).toBeCloseTo(3, 5)
    expect(overview.warming.value[0]!.todayCount).toBe(3)
    expect(overview.warming.value[0]!.baselineAvg).toBeCloseTo(2, 5)
  })

  it('升温边界：3 条且恰好 2 倍入选；真实零热度与零基线不借分区数制造升温', async () => {
    const overview = await setup({ boards: [
      board({ boardId: '1', hasHeatData: true, todayStories: 3, todayHeat: 4, last3Heat: 6 }),
      board({ boardId: '2', hasHeatData: true, todayStories: 0, todayHeat: 0, last3Heat: 0, prev3Heat: 0, todaySections: 9, last3Sections: 3 }),
      board({ boardId: '3', hasHeatData: true, todayStories: 4, todayHeat: 8, last3Heat: 0 }),
    ] })
    expect(overview.warming.value.map(item => [item.row.boardId, item.ratio])).toEqual([['1', 2]])
  })

  it('升温（旧响应回退）：无 heat 字段时沿用当日分区 ≥3 且 ≥ 近 3 日均值 2 倍，按倍数降序', async () => {
    const overview = await setup({
      boards: [
        board({ boardId: '4', todaySections: 8, last3Sections: 9 }),
        board({ boardId: '5', todaySections: 3, last3Sections: 6, prev3Sections: 3 }),
        board({ boardId: '6', todaySections: 2, last3Sections: 1 }),
      ],
    })
    expect(overview.warming.value).toHaveLength(1)
    expect(overview.warming.value[0]!.row.boardId).toBe('4')
    expect(overview.warming.value[0]!.ratio).toBeCloseTo(8 / 3, 5)
    expect(overview.warming.value[0]!.todayCount).toBe(8)
    expect(overview.warming.value[0]!.baselineAvg).toBeCloseTo(3, 5)
  })

  it('今日该读：有标题才列、按关注度降序、最多 5 条', async () => {
    const overview = await setup({
      boards: [
        ...Array.from({ length: 6 }, (_, i) => board({ boardId: String(i + 1), attention: 20 + i })),
        board({ boardId: '9', reportTitle: '' }),
      ],
    })
    expect(overview.todayRead.value).toHaveLength(5)
    expect(overview.todayRead.value[0]!.boardId).toBe('6')
  })

  it('报头：正常标题句带关注数与升温数', async () => {
    const overview = await setup({
      boards: [
        board({ attention: 12 }),
        board({ boardId: '4', todaySections: 8, last3Sections: 9, attention: 5 }),
      ],
    })
    expect(overview.headline.value).toBe('2 个板块值得关注，1 处出现升温')
    expect(overview.subKind.value).toBe('hot')
    expect(overview.dateText.value).toContain('10月24日')
  })
})

describe('useBoardRadarOverview — 事件线清单（surface-story-events-in-radar）', () => {
  it('单清单直接消费服务端事件线段（前端不合并、不重排）；徽标字段随条目透出', async () => {
    const overview = await setup({ boards: [board()], stories: feed() })
    expect(overview.hasStoryFeed.value).toBe(true)
    expect(overview.topStories.value).toHaveLength(1)
    expect(overview.topStories.value[0]!.storyTitle).toBe('美伊博弈升级')
    // 徽标判定字段（D3）：无基线/倍率/当日证据由条目自带
    expect(overview.topStories.value[0]!.hasBaseline).toBe(true)
    expect(overview.topStories.value[0]!.ratio).toBeCloseTo(4.1769, 4)
    expect(overview.topStories.value[0]!.hasTodayEvidence).toBe(true)
  })

  it('降级标记透出：未回填版块数与排序不完整', async () => {
    const overview = await setup({ boards: [board()], stories: feed({ partial: true }) })
    expect(overview.unbackfilledBoardCount.value).toBe(1)
    expect(overview.partialStories.value).toBe(true)
  })

  it('滞后 / 降级版块数透出（右栏提示消费：N 个版块滞后、降级计数）', async () => {
    const overview = await setup({ boards: [board()], stories: feed({ partial: true, degradedBoardIds: ['9', '10'] }) })
    expect(overview.laggingBoardCount.value).toBe(1)
    expect(overview.degradedBoardCount.value).toBe(2)
  })

  it('报头升温计数用服务端全量计数（非截断数）', async () => {
    const overview = await setup({ boards: [board()], stories: feed({ warmingCount: 6 }) })
    expect(overview.warmingCount.value).toBe(6)
    expect(overview.headline.value).toContain('6 处出现升温')
  })

  it('旧响应回退：响应缺 stories 段 → 回退版块级列表（不渲染业务空态）', async () => {
    const overview = await setup({
      boards: [board({ boardId: '4', todayStories: 3, todayHeat: 6, last3Heat: 6, last3Stories: 6 })],
      stories: null,
    })
    expect(overview.hasStoryFeed.value).toBe(false)
    expect(overview.topStories.value).toEqual([])
    // 回退分支仍是版块级口径
    expect(overview.todayRead.value).toHaveLength(1)
    expect(overview.warming.value).toHaveLength(1)
    expect(overview.warmingCount.value).toBe(1)
  })

  it('openStory：跳落点报告日并带 ?story= 参数；无落点不跳', async () => {
    const overview = await setup({ boards: [board()], stories: feed() })
    overview.openStory(overview.topStories.value[0]!)
    expect(push).toHaveBeenCalledWith({
      path: '/tags/1974/2026-11-06',
      query: { story: '4211' },
    })
    push.mockClear()
    overview.openStory({ ...overview.topStories.value[0]!, reportDate: '' })
    expect(push).not.toHaveBeenCalled()
  })
})

describe('useBoardRadarOverview — 失败语义（R-17）', () => {
  it('首次取数失败：firstLoadFailed、中性标题、副注 failed，不把 0 当业务结论', async () => {
    const overview = await setup({ boards: [] }, false)
    expect(overview.error.value).toBe('接口挂了')
    expect(overview.rows.value).toEqual([])
    expect(overview.firstLoadFailed.value).toBe(true)
    expect(overview.subKind.value).toBe('failed')
    expect(overview.headline.value).toBe('今日雷达')
  })

  it('成功返回空数组：真实 0，业务空态形态', async () => {
    const overview = await setup({ boards: [] })
    expect(overview.error.value).toBe('')
    expect(overview.firstLoadFailed.value).toBe(false)
    expect(overview.subKind.value).toBe('empty')
    expect(overview.headline.value).toBe('0 个板块值得关注，暂未出现升温')
  })

  it('有旧数据时刷新失败：保留旧值可读 + 记录错误', async () => {
    getOverview.mockResolvedValue({ success: true, data: { date: '2026-10-24', boards: [board()] } })
    const overview = useBoardRadarOverview()
    await overview.load()
    getOverview.mockResolvedValue({ success: false, data: undefined, error: '刷新失败' })
    await overview.load()
    expect(overview.error.value).toBe('刷新失败')
    expect(overview.rows.value).toHaveLength(1)
    expect(overview.firstLoadFailed.value).toBe(false)
  })
})

describe('useBoardRadarOverview — 去向导航', () => {
  it('openBoard/openLane/openReport 的路由形状', async () => {
    const overview = await setup({ boards: [board()] })
    overview.openBoard('42')
    expect(push).toHaveBeenCalledWith('/tags/42')
    overview.openLane({ boardId: '1974', topicId: '1204' })
    expect(push).toHaveBeenCalledWith({ path: '/tags/1974', query: { lane: '1204' } })
    overview.openReport(board())
    expect(push).toHaveBeenCalledWith('/tags/1974/2026-10-24')
  })

  it('openReport 无日报日期时退回进版块', async () => {
    const overview = await setup({ boards: [board()] })
    overview.openReport(board({ reportDate: '' }))
    expect(push).toHaveBeenLastCalledWith('/tags/1974')
  })

  // ── 跨天线脚注判据（matrix-clue-fallback-and-rail-merge D3）────────────────
  // 升温卡片删除后，「无跨天线」的表达收敛为单行脚注：只在服务端判据为 0 时
  // 出现（crossDayBuilding）；>0（有跨天线但没升温）与 null（旧响应缺字段）
  // 都不出脚注——无法判定时不得断言成因。

  describe('crossDayBuilding：跨天线脚注判据三态', () => {
    it('crossDayStoryCount = 0 → true（脚注出现：跨天延续建设中）', async () => {
      const state = await setup({ boards: [board()], stories: feed({ crossDayStoryCount: 0 }) })
      expect(state.crossDayBuilding.value).toBe(true)
    })

    it('crossDayStoryCount > 0 → false（有跨天线但没升温：无任何空态，徽标已表达）', async () => {
      const state = await setup({ boards: [board()], stories: feed({ crossDayStoryCount: 2 }) })
      expect(state.crossDayBuilding.value).toBe(false)
    })

    it('缺字段 → false（无法判定，不出脚注、不断言成因）', async () => {
      const state = await setup({ boards: [board()], stories: feed({ crossDayStoryCount: null }) })
      expect(state.crossDayBuilding.value).toBe(false)
    })
  })
})
