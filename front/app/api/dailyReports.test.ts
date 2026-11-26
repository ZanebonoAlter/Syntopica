import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getMock, postMock, patchMock, deleteMock, buildQueryParamsMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  postMock: vi.fn(),
  patchMock: vi.fn(),
  deleteMock: vi.fn(),
  buildQueryParamsMock: vi.fn(() => 'days=7'),
}))

vi.mock('./client', () => ({
  apiClient: {
    get: getMock,
    post: postMock,
    patch: patchMock,
    delete: deleteMock,
    buildQueryParams: buildQueryParamsMock,
  },
}))

import { useDailyReportsApi } from './dailyReports'

describe('useDailyReportsApi — active watch summaries', () => {
  beforeEach(() => {
    getMock.mockReset()
    getMock.mockResolvedValue({
      success: true,
      data: {
        reports: [{
          id: 9,
          semantic_board_id: 1974,
          period_date: '2026-06-29',
          title: '日报',
          summary: '',
          status: 'done',
          cluster_count: 1,
          article_count: 2,
          event_tag_count: 1,
          created_at: '2026-06-29',
          active_watch_summaries: [{ watch_id: 7, label: 'ASML', type: 'keyword' }],
        }],
      },
    })
  })

  it('normalizes the batched activeWatchSummaries field without another request per report', async () => {
    const api = useDailyReportsApi()
    const response = await api.getBoardDailyReports(1974, { days: 7 })
    expect(response.data?.reports[0]?.activeWatchSummaries).toEqual([
      { watchId: 7, label: 'ASML', type: 'keyword' },
    ])
    expect(getMock).toHaveBeenCalledOnce()
  })
})

describe('useDailyReportsApi — getBoardSectionTimeline time-range params (task 2.5)', () => {
  beforeEach(() => {
    getMock.mockReset()
    getMock.mockResolvedValue({ success: true, data: { sections: [], relations: [] } })
  })

  it('sends ?days=7 for the 7-day window', async () => {
    const api = useDailyReportsApi()
    await api.getBoardSectionTimeline(1974, 7)
    expect(getMock).toHaveBeenCalledWith('/semantic-boards/1974/section-timeline?days=7')
  })

  it('sends ?days=14 for the default 14-day window', async () => {
    const api = useDailyReportsApi()
    await api.getBoardSectionTimeline(1974, 14)
    expect(getMock).toHaveBeenCalledWith('/semantic-boards/1974/section-timeline?days=14')
  })

  it('sends ?days=30 for the 30-day window', async () => {
    const api = useDailyReportsApi()
    await api.getBoardSectionTimeline(1974, 30)
    expect(getMock).toHaveBeenCalledWith('/semantic-boards/1974/section-timeline?days=30')
  })

  it('sends ?days=0 for 全部 (all history), not an empty query', async () => {
    // "全部" 约定：days=0 → 显式 ?days=0，后端 <=0 即不限天。
    const api = useDailyReportsApi()
    await api.getBoardSectionTimeline(1974, 0)
    expect(getMock).toHaveBeenCalledWith('/semantic-boards/1974/section-timeline?days=0')
  })

  it('omits the query when days is undefined (backend default)', async () => {
    const api = useDailyReportsApi()
    await api.getBoardSectionTimeline(1974)
    expect(getMock).toHaveBeenCalledWith('/semantic-boards/1974/section-timeline')
  })
})

describe('useDailyReportsApi — 人工修正三个写端点（lane-manual-correction-loop 6.7）', () => {
  beforeEach(() => {
    patchMock.mockReset()
    postMock.mockReset()
    patchMock.mockResolvedValue({ success: true, data: null })
    postMock.mockResolvedValue({ success: true, data: null })
  })

  it('updateTopicAnchor → PATCH /daily-reports/topics/:id/anchor，body 为 section_ids', async () => {
    const api = useDailyReportsApi()
    await api.updateTopicAnchor(42, [7, 9])
    expect(patchMock).toHaveBeenCalledWith('/daily-reports/topics/42/anchor', { section_ids: [7, 9] })
  })

  it('setSectionAssignment → PATCH /daily-reports/sections/:id/assignment，body 为 topic_id', async () => {
    const api = useDailyReportsApi()
    await api.setSectionAssignment(7149, 42)
    expect(patchMock).toHaveBeenCalledWith('/daily-reports/sections/7149/assignment', { topic_id: 42 })
  })

  it('setSectionAssignment(null) → 解除归属（内容不删，仍可再参与候选）', async () => {
    const api = useDailyReportsApi()
    await api.setSectionAssignment(7149, null)
    expect(patchMock).toHaveBeenCalledWith('/daily-reports/sections/7149/assignment', { topic_id: null })
  })

  it('detachTopicDate → POST /daily-reports/topics/:id/detach-date，body 为 period_date', async () => {
    const api = useDailyReportsApi()
    await api.detachTopicDate(42, '2026-10-23')
    expect(postMock).toHaveBeenCalledWith('/daily-reports/topics/42/detach-date', { period_date: '2026-10-23' })
  })

  it('getTopicAnchor → GET /daily-reports/topics/:id/anchor（当前首义：来源 + 人工节点集）', async () => {
    getMock.mockReset()
    getMock.mockResolvedValue({
      success: true,
      data: { topic_id: 42, source: 'manual', section_ids: [7, 9], override_count: 2, effective_from: 'next_report' },
    })
    const api = useDailyReportsApi()
    const res = await api.getTopicAnchor(42)
    expect(getMock).toHaveBeenCalledWith('/daily-reports/topics/42/anchor')
    expect(res.data?.source).toBe('manual')
    expect(res.data?.section_ids).toEqual([7, 9])
  })
})

describe('useDailyReportsApi — rebucketSection（add-intra-lane-theme-bucketing）', () => {
  beforeEach(() => {
    postMock.mockReset()
  })

  it('posts section_id to the rebucket endpoint and returns buckets + sections', async () => {
    postMock.mockResolvedValue({
      success: true,
      data: { buckets: 2, sections: [{ id: 201, cluster_index: 3, bucket_index: 0 }] },
    })

    const api = useDailyReportsApi()
    const response = await api.rebucketSection(100, 7149)

    expect(postMock).toHaveBeenCalledWith('/daily-reports/100/rebucket', { section_id: 7149 })
    expect(response.data?.buckets).toBe(2)
    expect(response.data?.sections[0]?.bucket_index).toBe(0)
  })

  it('propagates the no-op response (buckets=1) unchanged', async () => {
    postMock.mockResolvedValue({ success: true, data: { buckets: 1, sections: [] } })

    const api = useDailyReportsApi()
    const response = await api.rebucketSection(100, 7149)

    expect(response.data?.buckets).toBe(1)
  })
})
