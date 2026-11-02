/**
 * OtherTopicReading（8C G3 单一阅读槽）测试 —— 阅读正文（ORD 组）。
 *
 *  - ORD-1 分区原题逐字 + 全量线索标题；默认收束；单条互斥展开、摘要逐字
 *  - ORD-2/3 0 线索合法；reportDay 日期不匹配/加载中不挂旧日 sections；无日报与失败分层
 *  - ORD-4 详情已更新缺分区 → 中性提示 + 刷新（不借别区/别日标题）
 *  - ORD-5 相关文章按需、同文章跨线索复用同一注入 store 缓存、空列表/打开文章事件
 *  - ORD-7 动作带完整日期；组件不滚动/不改日期
 *
 * 8C.4S 结构规范包按内聚子套件拆分：观察带见 `OtherTopicReading.observation.test.ts`、
 * 重试意图与阅读终态信号见 `OtherTopicReading.signals.test.ts`；fixture 与 mount 装配在
 * `otherTopicReadingTestHarness.ts`（断言与用例名不变）。
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import {
  DATE,
  errorDay,
  mountReading,
  readyDay,
  section,
  thread,
} from './otherTopicReadingTestHarness'

const api = vi.hoisted(() => ({
  loadReportIndex: vi.fn(),
  loadReportDetail: vi.fn(),
  loadArticleMeta: vi.fn(),
}))
vi.mock('~/api/laneDayReading', async (importOriginal) => {
  const actual = await importOriginal<typeof import('~/api/laneDayReading')>()
  return { ...actual, useLaneDayReadingApi: () => api }
})
vi.mock('~/api/otherTopicObservation', async (importOriginal) => {
  const actual = await importOriginal<typeof import('~/api/otherTopicObservation')>()
  return { ...actual, useOtherTopicObservationApi: () => ({ loadBoardSectionTimeline: vi.fn() }) }
})
vi.mock('@iconify/vue', () => ({ Icon: { name: 'Icon', template: '<span />' } }))

beforeEach(() => {
  api.loadReportIndex.mockReset().mockResolvedValue({
    success: true,
    data: [{ reportId: '900', periodDate: DATE, title: '日报', createdAt: '' }],
  })
  api.loadReportDetail.mockReset().mockResolvedValue({
    success: true,
    data: { reportId: '900', boardId: '1974', periodDate: DATE, sections: [] },
  })
  api.loadArticleMeta.mockReset().mockImplementation(async (articleId: string) => ({
    success: true,
    data: {
      articleId,
      title: `文章${articleId}`,
      pubDate: '2026-10-20T09:00:00+08:00',
      read: false,
      article: { id: articleId },
    },
  }))
})
afterEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
})

describe('阅读正文（ORD）', () => {
  it('分区原题逐字 + 全量线索；默认收束；单条互斥展开摘要逐字', async () => {
    const threads = [
      thread('t1', '完整线索标题一', { summary: '第一条原始摘要' }),
      thread('t2', '完整线索标题二', { summary: '第二条原始摘要' }),
      thread('t3', '完整线索标题三', { summary: '' }),
    ]
    const { wrapper } = mountReading({ reportDay: readyDay(DATE, [section('501', '海峡局势与油运', threads)]) })
    await flushPromises()
    expect(wrapper.find('[data-testid="other-topic-reading"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('海峡局势与油运')
    expect(wrapper.find('[data-testid="other-reading-threads"]').findAll('.lct')).toHaveLength(3)
    expect(wrapper.text()).not.toContain('第一条原始摘要')

    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t1"]').trigger('click')
    expect(wrapper.text()).toContain('第一条原始摘要')
    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t2"]').trigger('click')
    expect(wrapper.text()).not.toContain('第一条原始摘要')
    expect(wrapper.text()).toContain('第二条原始摘要')
    // 空 summary 是合法态，不伪造
    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t3"]').trigger('click')
    expect(wrapper.text()).toContain('暂无摘要记录')
  })

  it('0 线索合法；reportDay 日期不匹配/加载中不挂旧日 sections；无日报与失败分层', async () => {
    const empty = mountReading({ reportDay: readyDay(DATE, [section('501', '空分区', [])]) })
    await flushPromises()
    expect(empty.wrapper.text()).toContain('0 条线索')

    const stale = mountReading({ reportDay: readyDay('2026-10-21', [section('501', '旧日分区', [thread('t1', '旧日线索')])]) })
    await flushPromises()
    expect(stale.wrapper.text()).toContain('正在读取当日报告')
    expect(stale.wrapper.text()).not.toContain('旧日分区')
    expect(stale.wrapper.find('[data-testid="other-reading-threads"]').exists()).toBe(false)

    const loading = mountReading({
      reportDay: { date: DATE, status: 'loading', error: null, errorSource: null, reportId: null, sections: [] },
    })
    await flushPromises()
    expect(loading.wrapper.text()).toContain('正在读取当日报告')

    const noReport = mountReading({
      reportDay: { date: DATE, status: 'no-report', error: null, errorSource: null, reportId: null, sections: [] },
    })
    await flushPromises()
    expect(noReport.wrapper.text()).toContain('所选日没有日报')
    expect(noReport.wrapper.text()).not.toContain('0 条线索')

    const failed = mountReading({ reportDay: errorDay(DATE, '详情 500') })
    await flushPromises()
    expect(failed.wrapper.text()).toContain('当日报告读取失败')
    expect(failed.wrapper.text()).toContain('详情 500')
    api.loadReportIndex.mockClear()
    await failed.wrapper.find('[data-testid="other-reading-retry"]').trigger('click')
    await flushPromises()
    expect(failed.wrapper.emitted('retryReport')?.[0]).toEqual([{ date: DATE, sectionId: '501' }])
    expect(api.loadReportIndex).not.toHaveBeenCalled()
  })

  it('报告已更新缺该分区：中性提示 + 刷新（force 重读详情），不借别区标题', async () => {
    const { wrapper } = mountReading({
      reportDay: readyDay(DATE, [section('502', '别的分区', [thread('x1', '别区线索')])]),
    })
    await flushPromises()
    expect(wrapper.text()).toContain('当日报告已更新，未找到该分区')
    expect(wrapper.text()).not.toContain('别的分区')
    expect(wrapper.find('[data-testid="other-reading-refresh"]').exists()).toBe(true)

    await wrapper.find('[data-testid="other-reading-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('retryReport')?.[0]).toEqual([{ date: DATE, sectionId: '501' }])
    expect(api.loadReportIndex).not.toHaveBeenCalled()
    expect(api.loadReportDetail).not.toHaveBeenCalled()
  })

  it('相关文章按需读取；同文章跨线索复用注入 store 缓存；空列表/打开文章事件', async () => {
    const threads = [
      thread('t1', '线索一', { related: ['11'] }),
      thread('t2', '线索二', { related: ['11', '12'] }),
      thread('t3', '无线索文章', { related: [] }),
    ]
    const { wrapper } = mountReading({ reportDay: readyDay(DATE, [section('501', '分区', threads)]) })
    await flushPromises()
    expect(api.loadArticleMeta).not.toHaveBeenCalled()

    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t1"]').trigger('click')
    await wrapper.find('[data-testid="lane-chronicle-related-toggle-t1"]').trigger('click')
    await flushPromises()
    expect(api.loadArticleMeta.mock.calls.map(call => call[0])).toEqual(['11'])

    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t2"]').trigger('click')
    await wrapper.find('[data-testid="lane-chronicle-related-toggle-t2"]').trigger('click')
    await flushPromises()
    // 11 已在同一注入 store 缓存，只新增 12
    expect(api.loadArticleMeta.mock.calls.map(call => call[0])).toEqual(['11', '12'])

    await wrapper.find('[data-testid="lane-chronicle-article-11"]').trigger('click')
    expect(wrapper.emitted('openArticle')?.[0]).toEqual(['11'])

    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t3"]').trigger('click')
    expect(wrapper.text()).toContain('相关文章列表为空')
  })

  it('动作带完整日期；组件不滚动/不改日期', async () => {
    const scrollIntoView = vi.fn()
    const original = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView')
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { value: scrollIntoView, configurable: true })
    try {
      const { wrapper } = mountReading()
      await flushPromises()
      await wrapper.find('[data-testid="other-reading-open-report"]').trigger('click')
      await wrapper.find('[data-testid="other-reading-archive"]').trigger('click')
      expect(wrapper.emitted('openReport')?.[0]).toEqual([{ date: DATE, reportId: '900' }])
      expect(wrapper.emitted('viewArchive')?.[0]).toEqual([DATE])
      expect(scrollIntoView).not.toHaveBeenCalled()
    } finally {
      if (original) Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', original)
      else delete (HTMLElement.prototype as { scrollIntoView?: unknown }).scrollIntoView
    }
  })
})
