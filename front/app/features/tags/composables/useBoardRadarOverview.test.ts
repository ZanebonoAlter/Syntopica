/**
 * useBoardRadarOverview 测试（rework-workshop-into-radar-and-desk 8A/R-18 拆分）。
 * 取数/分档/今日该读/升温/报头文案/首次失败语义与去向导航在 composable 层验证；
 * API 归一在 api 层测试，本层 mock useBoardRadarApi 只喂归一后读模型。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useBoardRadarOverview } from './useBoardRadarOverview'
import type { RadarBoard } from '~/api/semanticBoards'

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
    ...over,
  }
}

async function setup(data: { date?: string, boards: RadarBoard[] }, success = true) {
  getOverview.mockResolvedValue({
    success,
    data: success ? { date: data.date ?? '2026-10-24', boards: data.boards } : undefined,
    error: success ? undefined : '接口挂了',
  })
  const overview = useBoardRadarOverview()
  await overview.load()
  return overview
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useBoardRadarOverview — 取数与分档', () => {
  it('分档：≥50 热点、≥15 关注、<15 常温；可见行热点在前', async () => {
    const overview = await setup({
      boards: [
        board({ boardId: '2', label: '关注', attention: 26 }),
        board({ boardId: '1', label: '热点', attention: 60 }),
        board({ boardId: '3', label: '常温', attention: 5 }),
      ],
    })
    expect(overview.hotRows.value.map(row => row.boardId)).toEqual(['1'])
    expect(overview.warmRows.value.map(row => row.boardId)).toEqual(['2'])
    expect(overview.coldRows.value.map(row => row.boardId)).toEqual(['3'])
    expect(overview.visibleRows.value.map(row => row.boardId)).toEqual(['1', '2'])
    expect(overview.loading.value).toBe(false)
  })

  it('升温：当日 ≥3 且 ≥ 近 3 日均值 2 倍才列入，按倍数降序', async () => {
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
        board({ attention: 60 }),
        board({ boardId: '4', todaySections: 8, last3Sections: 9 }),
      ],
    })
    expect(overview.headline.value).toBe('2 个板块值得关注，1 处出现升温')
    expect(overview.subKind.value).toBe('hot')
    expect(overview.dateText.value).toContain('10月24日')
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
})
