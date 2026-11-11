/**
 * useBoardRadarOverview 测试（rework-workshop-into-radar-and-desk 8A/R-18 拆分；
 * retire-story-adopt-event-metrics 3.2 换当天线索口径）。
 * 取数/分档/旧响应回退/泳道清单段标记/报头文案/首次失败语义与去向导航在 composable 层验证；
 * API 归一在 api 层测试，本层 mock useBoardRadarApi 只喂归一后读模型。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useBoardRadarOverview } from './useBoardRadarOverview'
import type { RadarBoard, RadarTopLanes } from '~/api/semanticBoards'

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
    todayClues: 8,
    last3Clues: 13,
    prev3Clues: 10,
    attention: 26,
    heatRuleVersion: 'heat-v2-lane-clues-1d',
    clueDays: Array.from({ length: 14 }, () => 1),
    dataDelayed: false,
    lanes: [],
    ...over,
  }
}

async function setup(data: { date?: string, boards: RadarBoard[], topLanes?: RadarTopLanes | null }, success = true) {
  getOverview.mockResolvedValue({
    success,
    data: success ? { date: data.date ?? '2026-10-24', boards: data.boards, topLanes: data.topLanes ?? null } : undefined,
    error: success ? undefined : '接口挂了',
  })
  const overview = useBoardRadarOverview()
  await overview.load()
  return overview
}

/** 泳道清单段夹具（新版后端响应形状，服务端已排序/截断）。 */
function feed(over: Partial<RadarTopLanes> = {}): RadarTopLanes {
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
})

describe('useBoardRadarOverview — 取数与分档', () => {
  it('分档：≥14.5 热点、≥2.3 关注、<2.3 常温（版块关注度 = Σ 泳道当天热度）；可见行热点在前', async () => {
    const overview = await setup({
      boards: [
        board({ boardId: '2', label: '关注', attention: 2.3 }),
        board({ boardId: '1', label: '热点', attention: 14.5 }),
        board({ boardId: '3', label: '常温', attention: 2.2 }),
      ],
    })
    expect(overview.hotRows.value.map(row => row.boardId)).toEqual(['1'])
    expect(overview.warmRows.value.map(row => row.boardId)).toEqual(['2'])
    expect(overview.coldRows.value.map(row => row.boardId)).toEqual(['3'])
    expect(overview.visibleRows.value.map(row => row.boardId)).toEqual(['1', '2'])
    expect(overview.loading.value).toBe(false)
  })

  it('今日该读（旧响应回退）：有标题才列、按关注度降序、最多 5 条', async () => {
    const overview = await setup({
      boards: [
        ...Array.from({ length: 6 }, (_, i) => board({ boardId: String(i + 1), attention: 20 + i })),
        board({ boardId: '9', reportTitle: '' }),
      ],
    })
    expect(overview.todayRead.value).toHaveLength(5)
    expect(overview.todayRead.value[0]!.boardId).toBe('6')
  })

  it('报头：标题句带关注数与热点数（不再有升温口径）', async () => {
    const overview = await setup({
      boards: [
        board({ attention: 20 }),
        board({ boardId: '4', attention: 5 }),
      ],
    })
    expect(overview.headline.value).toBe('2 个板块值得关注，其中 1 个达到热点阈值')
    expect(overview.subKind.value).toBe('hot')
    expect(overview.dateText.value).toContain('10月24日')
  })

  it('报头：无热点时只给关注数，不造「0 处升温」类结论', async () => {
    const overview = await setup({ boards: [board({ attention: 5 })] })
    expect(overview.headline.value).toBe('1 个板块值得关注')
    expect(overview.subKind.value).toBe('no-hot')
  })

  it('当日线索合计：全行已知才给总数；任一行缺字段（旧后端）→ null，不造 0', async () => {
    const known = await setup({
      boards: [board({ todayClues: 8 }), board({ boardId: '2', todayClues: 5 })],
    })
    expect(known.todayClueTotal.value).toBe(13)
    const legacy = await setup({
      boards: [board({ todayClues: 8 }), board({ boardId: '2', todayClues: null })],
    })
    expect(legacy.todayClueTotal.value).toBeNull()
  })
})

describe('useBoardRadarOverview — 当天线索最多的泳道清单段', () => {
  it('直接消费服务端清单（前端不合并、不重排）：条目与锚点日透出', async () => {
    const overview = await setup({ boards: [board()], topLanes: feed() })
    expect(overview.hasLaneFeed.value).toBe(true)
    expect(overview.laneItems.value).toHaveLength(2)
    expect(overview.laneItems.value[0]!.topicLabel).toBe('美伊博弈')
    expect(overview.laneItems.value[0]!.clueCount).toBe(12)
    // 滞后版块按自己的锚点日计
    expect(overview.laneItems.value[1]!.anchorDate).toBe('2026-10-23')
  })

  it('降级 / 滞后标记透出（右栏提示消费：N 个版块滞后、降级计数）', async () => {
    const overview = await setup({ boards: [board()], topLanes: feed({ partial: true, degradedBoardIds: ['9', '10'] }) })
    expect(overview.partialLanes.value).toBe(true)
    expect(overview.laggingBoardCount.value).toBe(1)
    expect(overview.degradedBoardCount.value).toBe(2)
  })

  it('旧响应回退：响应缺 top_lanes 段 → hasLaneFeed=false，回退版块级列表（不渲染业务空态）', async () => {
    const overview = await setup({ boards: [board()], topLanes: null })
    expect(overview.hasLaneFeed.value).toBe(false)
    expect(overview.laneItems.value).toEqual([])
    expect(overview.todayRead.value).toHaveLength(1)
  })

  it('成功但清单为空：hasLaneFeed=true 且条目为空（真实空态，与旧响应区分）', async () => {
    const overview = await setup({ boards: [board()], topLanes: feed({ lanes: [] }) })
    expect(overview.hasLaneFeed.value).toBe(true)
    expect(overview.laneItems.value).toEqual([])
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
    expect(overview.headline.value).toBe('0 个板块值得关注')
  })

  it('有旧数据时刷新失败：保留旧值可读 + 记录错误', async () => {
    getOverview.mockResolvedValue({ success: true, data: { date: '2026-10-24', boards: [board()], topLanes: null } })
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

  it('openLane 缺 id 不跳（不产生假深链）', async () => {
    const overview = await setup({ boards: [board()] })
    overview.openLane({ boardId: '', topicId: '1204' })
    overview.openLane({ boardId: '1974', topicId: '' })
    expect(push).not.toHaveBeenCalled()
  })
})
