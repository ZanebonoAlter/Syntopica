/**
 * OtherTopicReading 同归属跨日观察带子套件（8C G3，OBV 组；8C.4S 结构规范包从
 * OtherTopicReading.test.ts 拆出）。
 *
 *  - OBV-2/3 无可靠 topicId 不发请求并给缺身份说明；本窗 0/1 日只给说明
 *  - OBV-4 >1 日画节点：计数取峰值、缺失未知、不补 0；点节点 emit 稳定目标
 *  - OBV-5 观察失败可重试且不阻塞当前日报正文；旧成功保留 stale；settled 等观察终态
 *
 * fixture 与 mount 装配见 `otherTopicReadingTestHarness.ts`。
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import type { ObservationResponse, ObservationTimelineData } from '~/api/otherTopicObservation'
import {
  DATE,
  deferred,
  mountReading,
  obsRecord,
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

describe('同归属跨日观察带（OBV）', () => {
  it('无可靠 topicId：不发请求、缺身份说明（不是无记录）', async () => {
    const timeline = vi.fn(async () => ({
      success: true as const,
      data: { records: [obsRecord('1', '2026-10-20', 2, { topicId: null })], coverage: 'all' as const },
    }))
    const { wrapper } = mountReading({
      topicId: null,
      reportDay: readyDay(DATE, [section('501', '未归属分区', [thread('t1', '线索')], null)]),
      timeline,
    })
    await flushPromises()
    expect(timeline).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="other-observe-note"]').text()).toContain('当次未归属')
    // 正文照常可读
    expect(wrapper.text()).toContain('未归属分区')
  })

  it('本窗 0/1/多日保留状态说明，但阅读区不再重复绘制日期药丸', async () => {
    const outside = mountReading({
      timeline: vi.fn(async () => ({
        success: true as const,
        data: { records: [obsRecord('1', '2026-10-01', 7)], coverage: 'all' as const },
      })),
    })
    await flushPromises()
    expect(outside.wrapper.find('[data-testid="other-observe-note"]').text()).toContain('本窗没有该归属的其他记录日')
    expect(outside.wrapper.find('[data-testid="other-observe-nodes"]').exists()).toBe(false)

    const single = mountReading({
      timeline: vi.fn(async () => ({
        success: true as const,
        data: { records: [obsRecord('1', '2026-10-20', 3)], coverage: 'all' as const },
      })),
    })
    await flushPromises()
    expect(single.wrapper.find('[data-testid="other-observe-note"]').text()).toContain('仅 1 个同归属记录日')
    expect(single.wrapper.find('[data-testid="other-observe-node-2026-10-20"]').exists()).toBe(false)

    const multi = mountReading({
      timeline: vi.fn(async () => ({
        success: true as const,
        data: {
          records: [
            obsRecord('1', '2026-10-20', 2),
            obsRecord('2', '2026-10-20', 5),
            obsRecord('3', '2026-10-21', null),
            obsRecord('4', '2026-10-22', 0),
          ],
          coverage: 'all' as const,
        },
      })),
    })
    await flushPromises()
    expect(multi.wrapper.find('[data-testid="other-observe-note"]').text()).toContain('同归属记录日 · 3 天')
    expect(multi.wrapper.find('[data-testid="other-observe-note"]').text()).toContain('不称同一事件')
    expect(multi.wrapper.find('[data-testid="other-observe-note"]').text()).toContain('关联日已标在日期表头')
    expect(multi.wrapper.find('[data-testid="other-observe-nodes"]').exists()).toBe(false)
    expect(multi.wrapper.emitted('selectObservedDay')).toBeUndefined()

  })

  it('观察失败可重试且不阻塞正文；旧成功保留 stale；settled 等观察终态', async () => {
    const timeline = vi.fn()
      .mockResolvedValueOnce({ success: false, error: 'timeline 500' })
      .mockResolvedValueOnce({ success: true, data: { records: [obsRecord('1', '2026-10-20', 2), obsRecord('2', '2026-10-21', 4)], coverage: 'all' } })
    const { wrapper } = mountReading({ timeline })
    await flushPromises()
    expect(wrapper.text()).toContain('海峡局势')
    expect(wrapper.find('[data-testid="other-observe-note"]').text()).toContain('读取失败')
    await wrapper.find('[data-testid="other-observe-retry"]').trigger('click')
    await flushPromises()
    expect(timeline).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="other-observe-note"]').text()).toContain('关联日已标在日期表头')
    expect(wrapper.find('[data-testid="other-observe-nodes"]').exists()).toBe(false)

    // 观察在途时 settled 不提前发；观察终态后才发
    const pending = deferred<ObservationResponse<ObservationTimelineData>>()
    const pendingTimeline = vi.fn(() => pending.promise)
    const settledHarness = mountReading({ timeline: pendingTimeline })
    await flushPromises()
    expect(settledHarness.wrapper.emitted('readingSettled')).toBeUndefined()
    pending.resolve({ success: true, data: { records: [], coverage: 'all' } })
    await flushPromises()
    expect(settledHarness.wrapper.emitted('readingSettled')?.at(-1)?.[0]).toMatchObject({
      laneKey: 'other:501',
      date: DATE,
      status: 'ready',
      sectionId: '501',
      topicId: '10',
    })
  })
})
