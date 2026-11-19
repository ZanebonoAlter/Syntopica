import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { useDailyReportReader } from './useDailyReportReader'
import { groupSectionsByTopic } from '~/features/tags/components/daily-report/dailyReportMagazine'
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
  rebucketSection: vi.fn(),
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
    rebucketSection: api.rebucketSection,
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
    api.getArticle.mockImplementation(async (id: number) => ({ success: true, data: { id: String(id), title: `article-${id}` } }))
  })

  it('deduplicates article requests (日报侧不再拉话题生命线)', async () => {
    const reader = useDailyReportReader(ref(1974))
    await flushPromises()

    await Promise.all([reader.ensureArticleTitles([99, 99])])
    await reader.ensureArticleTitles([99])

    expect(api.getArticle).toHaveBeenCalledTimes(1)
    expect(reader.getArticleTitle(99).data?.title).toBe('article-99')
    // 折线退役（replace-lane-lifeline-with-topic-events）：日报读端 MUST NOT 再调 lifeline 端点
    expect(api.getTopicLifeline).not.toHaveBeenCalled()
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
    await reader.ensureArticleTitles([99])

    boardId.value = 1980
    await flushPromises()

    expect(reader.days.value).toBe(7)
    expect(reader.getArticleTitle(99).status).toBe('idle')
    expect(api.getBoardDailyReports).toHaveBeenLastCalledWith(1980, { days: 7 })
  })

  // 回归：读详情时不得污染 detailLoading / detailError（否则整个日报详情会被骨架屏
  // 替换）；历史详情的静默加载入口（ensureHistoricalDetail）已随迷你生命线退役删除
  // （replace-lane-lifeline-with-topic-events review R-11），这里只锚定当期详情读取。
  it('ensureDetail 写入缓存且结算后 detailLoading 回到 null', async () => {
    api.getDailyReportDetail.mockResolvedValue({
      success: true,
      data: { report: { id: 42 } as DailyReport },
    })

    const reader = useDailyReportReader(ref(1974))
    await flushPromises()

    const pending = reader.ensureDetail(42)
    // 加载进行中：detailLoading 指向请求中的报告 id，但不置错误
    expect(reader.detailLoading.value).toBe(42)
    expect(reader.detailError.value).toBe('')
    await pending

    expect(api.getDailyReportDetail).toHaveBeenCalledWith(42)
    expect(reader.detailLoading.value).toBeNull()
    expect(reader.detailError.value).toBe('')
    expect(reader.detailCache.value.get(42)).toEqual({ id: 42 })

    // 重复加载命中缓存，不再发请求。
    await reader.ensureDetail(42)
    expect(api.getDailyReportDetail).toHaveBeenCalledTimes(1)
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
  })

})

describe('useDailyReportReader — 重新分桶（add-intra-lane-theme-bucketing）', () => {
  function readerReport() {
    return {
      id: 60,
      semantic_board_id: 1974,
      period_date: '2026-10-23',
      title: '日报',
      summary: '',
      status: 'completed',
      cluster_count: 2,
      article_count: 2,
      event_tag_count: 2,
      highlights: [],
      dynamics: '',
      sections: [
        {
          id: 100, cluster_index: 0, cluster_label: '域型泳道', cluster_tag_ids: [1, 2],
          threads: [{ id: 1000, report_id: 60, section_id: 100, title: 't0', summary: '', tag_ids: [1], confidence: 0.9, related_article_ids: [] }],
          article_count: 1, best_tier: 0, avg_score: 0.9, bucket_index: 0, lane_tier: 'l1_direct',
          persistent_topic_id: 7, persistent_topic: { id: 7, label: 'AGENT', status: 'active', color: '#a00', consecutive_hits: 3, can_activate: false },
        },
        {
          id: 500, cluster_index: 2, cluster_label: '另一个话题', cluster_tag_ids: [3],
          threads: [], article_count: 1, best_tier: 0, avg_score: 0.8, lane_tier: 'l1_direct',
          persistent_topic_id: 8, persistent_topic: { id: 8, label: '其他', status: 'active', color: '#0a0', consecutive_hits: 2, can_activate: false },
        },
      ],
    } as unknown as DailyReport
  }

  beforeEach(() => {
    vi.clearAllMocks()
    api.rebucketSection.mockReset()
    api.getBoardDailyReports.mockResolvedValue({ success: true, data: { reports: [] } })
    api.getReportKeyEvents.mockResolvedValue({ success: true, data: { status: 'idle', markers: [] } })
    api.getDailyReportDetail.mockResolvedValue({ success: true, data: { report: readerReport() } })
  })

  it('成功时只局部替换该桶组，并记录旧锚点别名', async () => {
    api.rebucketSection.mockResolvedValue({
      success: true,
      data: {
        buckets: 2,
        sections: [
          { id: 201, cluster_index: 0, cluster_label: '方向A', bucket_index: 0, bucket_title: '工具调用', threads: [], article_count: 1, best_tier: 0, avg_score: 0.9 },
          { id: 202, cluster_index: 0, cluster_label: '方向B', bucket_index: 1, bucket_title: 'Skill 生态', threads: [], article_count: 1, best_tier: 0, avg_score: 0.9 },
        ],
      },
    })

    const reader = useDailyReportReader(ref(1974))
    await flushPromises()
    await reader.ensureDetail(60)

    const outcome = await reader.rebucket(60, 100)
    expect(outcome).toEqual({ ok: true, buckets: 2 })
    expect(api.rebucketSection).toHaveBeenCalledWith(60, 100)

    const sections = reader.getReportDetail(60)!.sections
    expect(sections.map(section => section.id)).toEqual([500, 201, 202])
    expect(reader.rebucketSectionId.value).toBeNull()

    // 被删除的旧桶行 → 组头（深链回退）。
    expect(reader.sectionAliases.value.get(100)).toBe(201)
  })

  it('替换后兄弟桶仍归入同一话题分组（服务端带 persistent_topic brief）', async () => {
    const topic = { id: 7, label: 'AGENT', status: 'active', color: '#a00', consecutive_hits: 3, can_activate: false }
    api.rebucketSection.mockResolvedValue({
      success: true,
      data: {
        buckets: 2,
        sections: [
          { id: 201, cluster_index: 0, cluster_label: '方向A', bucket_index: 0, bucket_title: '工具调用', lanes: undefined, threads: [], article_count: 1, best_tier: 0, avg_score: 0.9, persistent_topic_id: 7, persistent_topic: topic, lane_tier: 'l1_direct' },
          { id: 202, cluster_index: 0, cluster_label: '方向B', bucket_index: 1, bucket_title: 'Skill 生态', threads: [], article_count: 1, best_tier: 0, avg_score: 0.9, persistent_topic_id: 7, persistent_topic: topic, lane_tier: 'l1_direct' },
        ],
      },
    })

    const reader = useDailyReportReader(ref(1974))
    await flushPromises()
    await reader.ensureDetail(60)
    await reader.rebucket(60, 100)

    const groups = groupSectionsByTopic({ key: 'active', label: '关心的话题', eyebrow: 'Following', sections: reader.getReportDetail(60)!.sections })
    const topicGroup = groups.find(group => group.topicId === 7)
    expect(topicGroup?.sections.map(section => section.id)).toEqual([201, 202])
    expect(topicGroup?.canonicalLabel).toBe('AGENT')
  })

  it('失败时回到 idle 且原数据不变', async () => {
    api.rebucketSection.mockResolvedValue({ success: false, error: 'rebucket failed' })

    const reader = useDailyReportReader(ref(1974))
    await flushPromises()
    await reader.ensureDetail(60)
    const before = reader.getReportDetail(60)!.sections

    const outcome = await reader.rebucket(60, 100)
    expect(outcome.ok).toBe(false)
    expect(outcome.error).toBe('rebucket failed')
    expect(reader.rebucketSectionId.value).toBeNull()
    expect(reader.getReportDetail(60)!.sections).toBe(before)
    expect(reader.sectionAliases.value.size).toBe(0)
  })

  it('拆不动（buckets=1）不改动本地数据', async () => {
    api.rebucketSection.mockResolvedValue({
      success: true,
      data: { buckets: 1, sections: [{ id: 100, cluster_index: 0, cluster_label: '域型泳道', threads: [], article_count: 1, best_tier: 0, avg_score: 0.9 }] },
    })

    const reader = useDailyReportReader(ref(1974))
    await flushPromises()
    await reader.ensureDetail(60)
    const before = reader.getReportDetail(60)!.sections

    const outcome = await reader.rebucket(60, 100)
    expect(outcome).toEqual({ ok: true, buckets: 1 })
    expect(reader.getReportDetail(60)!.sections).toBe(before)
  })

  it('请求进行中重复点击直接忽略', async () => {
    let resolveRequest: ((value: unknown) => void) | undefined
    api.rebucketSection.mockImplementation(() => new Promise((resolve) => { resolveRequest = resolve }))

    const reader = useDailyReportReader(ref(1974))
    await flushPromises()
    await reader.ensureDetail(60)

    const first = reader.rebucket(60, 100)
    expect(reader.rebucketSectionId.value).toBe(100)
    const second = await reader.rebucket(60, 500)
    expect(second.ok).toBe(false)
    expect(api.rebucketSection).toHaveBeenCalledTimes(1)

    resolveRequest?.({ success: true, data: { buckets: 1, sections: [] } })
    await first
    expect(reader.rebucketSectionId.value).toBeNull()
  })
})
