import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { useDailyReportReader } from './useDailyReportReader'
import type { DailyReport } from '~/api/dailyReports'

const api = vi.hoisted(() => ({
  getBoardDailyReports: vi.fn(),
  getDailyReportDetail: vi.fn(),
  getTopicLifeline: vi.fn(),
  getArticle: vi.fn(),
  getReportKeyEvents: vi.fn(),
  getTopicKeyEvents: vi.fn(),
  putThreadKeyEvent: vi.fn(),
  patchKeyEventMarker: vi.fn(),
}))

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({
    getBoardDailyReports: api.getBoardDailyReports,
    getDailyReportDetail: api.getDailyReportDetail,
    getTopicLifeline: api.getTopicLifeline,
    getReportKeyEvents: api.getReportKeyEvents,
    getTopicKeyEvents: api.getTopicKeyEvents,
    putThreadKeyEvent: api.putThreadKeyEvent,
    patchKeyEventMarker: api.patchKeyEventMarker,
  }),
}))

vi.mock('~/api/articles', () => ({
  useArticlesApi: () => ({ getArticle: api.getArticle }),
}))

describe('useDailyReportReader', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getReportKeyEvents.mockResolvedValue({ success: true, data: { status: 'idle', markers: [] } })
    api.getTopicKeyEvents.mockResolvedValue({ success: true, data: { topic_id: 1, days: 14, markers: [] } })
    api.getBoardDailyReports.mockResolvedValue({ success: true, data: { reports: [] } })
    api.getTopicLifeline.mockResolvedValue({ success: true, data: { sections: [], relations: [] } })
    api.getArticle.mockImplementation(async (id: number) => ({ success: true, data: { id: String(id), title: `article-${id}` } }))
  })

  it('deduplicates lifeline and article requests', async () => {
    const reader = useDailyReportReader(ref(1974))
    await flushPromises()

    await Promise.all([reader.ensureLifeline(5), reader.ensureLifeline(5)])
    await reader.ensureLifeline(5)
    await reader.ensureArticleTitles([99, 99])
    await reader.ensureArticleTitles([99])

    expect(api.getTopicLifeline).toHaveBeenCalledTimes(1)
    expect(api.getArticle).toHaveBeenCalledTimes(1)
    expect(reader.getArticleTitle(99).data?.title).toBe('article-99')
  })

  it('keeps a failed item local and supports retry', async () => {
    let article99Attempts = 0
    api.getArticle.mockImplementation(async (id: number) => {
      if (id === 99 && article99Attempts++ === 0) return { success: false }
      return { success: true, data: { id: String(id), title: id === 99 ? 'recovered' : `article-${id}` } }
    })
    const reader = useDailyReportReader(ref(1974))
    await flushPromises()

    await reader.ensureArticleTitles([99, 100])
    expect(reader.getArticleTitle(99).status).toBe('error')
    expect(reader.getArticleTitle(100).status).toBe('success')

    await reader.retryArticle(99)
    expect(reader.getArticleTitle(99).data?.title).toBe('recovered')
  })

  it('clears board-scoped caches when the board changes', async () => {
    const boardId = ref(1974)
    const reader = useDailyReportReader(boardId)
    await flushPromises()
    await reader.ensureLifeline(5)
    await reader.ensureArticleTitles([99])

    boardId.value = 1980
    await flushPromises()

    expect(reader.days.value).toBe(7)
    expect(reader.getLifeline(5).status).toBe('idle')
    expect(reader.getArticleTitle(99).status).toBe('idle')
    expect(api.getBoardDailyReports).toHaveBeenLastCalledWith(1980, { days: 7 })
  })

  // 回归：mini 话题泳道点击历史时间点（loadHistorical → ensureHistoricalDetail）
  // 不得污染当前选中日报的 detailLoading / detailError，否则整个日报详情会被
  // 骨架屏替换，表现为“点击时间点就刷新日报”。
  it('ensureHistoricalDetail fills the cache without setting detailLoading/detailError', async () => {
    api.getDailyReportDetail.mockResolvedValue({
      success: true,
      data: { report: { id: 42 } as DailyReport },
    })

    const reader = useDailyReportReader(ref(1974))
    await flushPromises()

    const pending = reader.ensureHistoricalDetail(42)
    // 加载进行中也不应触碰 detailLoading（这正是 bug 的触发条件）。
    expect(reader.detailLoading.value).toBeNull()
    expect(reader.detailError.value).toBe('')
    await pending

    expect(api.getDailyReportDetail).toHaveBeenCalledWith(42)
    expect(reader.detailLoading.value).toBeNull()
    expect(reader.detailError.value).toBe('')
    expect(reader.detailCache.value.get(42)).toEqual({ id: 42 })

    // 重复加载命中缓存，不再发请求。
    await reader.ensureHistoricalDetail(42)
    expect(api.getDailyReportDetail).toHaveBeenCalledTimes(1)
  })

  it('ensureHistoricalDetail swallows failures without surfacing detailError', async () => {
    api.getDailyReportDetail.mockResolvedValue({ success: false })

    const reader = useDailyReportReader(ref(1974))
    await flushPromises()

    const result = await reader.ensureHistoricalDetail(7)
    expect(result).toBeUndefined()
    expect(reader.detailLoading.value).toBeNull()
    expect(reader.detailError.value).toBe('')
    expect(reader.detailCache.value.has(7)).toBe(false)
  })

  it('loads key events read-only alongside each cached report detail (打开页面只读拉取)', async () => {
    api.getDailyReportDetail.mockResolvedValue({
      success: true,
      data: { id: 55, board_id: 1974, report_date: '2026-10-09T12:00:00Z', sections: [], highlights: [] },
    })
    const reader = useDailyReportReader(ref(1974))
    await flushPromises()

    await reader.ensureDetail(55)
    await reader.ensureDetail(55) // 重复读走缓存
    expect(api.getReportKeyEvents).toHaveBeenCalledTimes(1)
    expect(api.getReportKeyEvents).toHaveBeenCalledWith(55)
    expect(api.putThreadKeyEvent).not.toHaveBeenCalled()
    expect(api.patchKeyEventMarker).not.toHaveBeenCalled()

    await reader.ensureHistoricalDetail(56)
    expect(api.getReportKeyEvents).toHaveBeenCalledWith(56)
  })

})
