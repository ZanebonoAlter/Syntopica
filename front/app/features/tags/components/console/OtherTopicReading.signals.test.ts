/**
 * OtherTopicReading 受控重试意图与阅读终态信号子套件（8C G3，M1/M2；
 * 8C.4S 结构规范包从 OtherTopicReading.test.ts 拆出）。
 *
 *  - M1 失败/缺分区只 emit `retryReport({date,sectionId})`，不绕索引刷新、不自读旧 reportId；
 *    payload 用当前稳定上下文（切分区后不带旧 sectionId）
 *  - M2 终态信号在 DOM 挂载 + 报告/观察依赖真实终态后才发：初始 cache-ready 不提前发；
 *    强刷在途不算终态；同 context 刷新不重复发；观察失败/无稳定 ID 仍是终态；
 *    卸载后旧排队不发、换上下文只发新 identity
 *
 * fixture 与 mount 装配见 `otherTopicReadingTestHarness.ts`。
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import type { ObservationResponse, ObservationTimelineData } from '~/api/otherTopicObservation'
import {
  DATE,
  createObservation,
  deferred,
  errorDay,
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

describe('受控报告重试意图（M1：不绕索引刷新、不自读旧 reportId）', () => {
  it('失败态点重试只发 retryReport({date,sectionId})：组件不直连 store 读详情', async () => {
    const { wrapper } = mountReading({ reportDay: errorDay(DATE, '详情 500') })
    await flushPromises()
    api.loadReportIndex.mockClear()
    api.loadReportDetail.mockClear()

    await wrapper.find('[data-testid="other-reading-retry"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('retryReport')).toEqual([[{ date: DATE, sectionId: '501' }]])
    expect(api.loadReportIndex).not.toHaveBeenCalled()
    expect(api.loadReportDetail).not.toHaveBeenCalled()
    // 观察带自身强刷走专用 observer，不复用日报读取路径
    expect(wrapper.find('[data-testid="other-observe-retry"]').exists()).toBe(false)
  })

  it('缺分区点刷新同样只发意图（同日重建须由父先 force 索引再读当日）', async () => {
    const { wrapper } = mountReading({ reportDay: readyDay(DATE, [section('502', '别的分区', [])]) })
    await flushPromises()
    api.loadReportIndex.mockClear()
    api.loadReportDetail.mockClear()

    await wrapper.find('[data-testid="other-reading-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('retryReport')).toEqual([[{ date: DATE, sectionId: '501' }]])
    expect(api.loadReportIndex).not.toHaveBeenCalled()
    expect(api.loadReportDetail).not.toHaveBeenCalled()
  })

  it('payload 用当前稳定上下文：切分区后重试不带旧 sectionId（更不携带旧 reportId）', async () => {
    const { wrapper } = mountReading()
    await flushPromises()
    await wrapper.setProps({ selectedSectionId: '777', topicId: null })
    await flushPromises()
    expect(wrapper.find('[data-testid="other-reading-missing-section"]').exists()).toBe(true)

    await wrapper.find('[data-testid="other-reading-refresh"]').trigger('click')
    await flushPromises()
    const events = wrapper.emitted('retryReport') ?? []
    expect(events.at(-1)).toEqual([{ date: DATE, sectionId: '777' }])
    expect(JSON.stringify(events)).not.toContain('900')
  })
})

describe('阅读终态信号（M2：DOM/依赖终态后才发）', () => {
  const MULTI = {
    records: [obsRecord('1', '2026-10-20', 2), obsRecord('2', '2026-10-21', 4)],
    coverage: 'all' as const,
  }

  it('初始 cache-ready 不在 setup/DOM 挂载前发；挂载 + DOM 更新后发一次，回调能看到当前 DOM 与 identity', async () => {
    const timeline = vi.fn(async () => ({ success: true as const, data: MULTI }))
    const observation = createObservation(timeline)
    await observation.ensureObservation('10') // 旧 dataset 已就绪（不等挂载）
    const captured: { payload: { laneKey: string, date: string, status: string }, domText: string | null }[] = []
    const { wrapper } = mountReading({
      observation,
      attach: true,
      onSettled: (payload) => {
        captured.push({
          payload,
          domText: document.querySelector('[data-testid="other-observe-note"]')?.textContent ?? null,
        })
      },
    })
    // 终态数据在 setup 就齐了——但 DOM 还没挂/没更新，不能发
    expect(wrapper.emitted('readingSettled')).toBeUndefined()

    await flushPromises()
    expect(wrapper.emitted('readingSettled')).toEqual([[
      { laneKey: 'other:501', date: DATE, status: 'ready', sectionId: '501', topicId: '10' },
    ]])
    expect(captured).toHaveLength(1)
    expect(captured[0]!.domText).toContain('同归属记录日 · 2 天')
    expect(wrapper.find('[data-testid="other-observe-nodes"]').exists()).toBe(false)
  })

  it('已有数据但强刷在途：ready+loading 不算终态，刷新终态后才发（不提前恢复滚动）', async () => {
    const timeline = vi.fn()
    timeline.mockResolvedValueOnce({ success: true as const, data: MULTI })
    const observation = createObservation(timeline)
    await observation.ensureObservation('10')
    expect(observation.view.value.status).toBe('ready')

    const pending = deferred<ObservationResponse<ObservationTimelineData>>()
    timeline.mockReturnValueOnce(pending.promise)
    void observation.retry()
    await nextTick()
    expect(observation.view.value.status).toBe('ready')
    expect(observation.view.value.refreshing).toBe(true)

    const { wrapper } = mountReading({ observation })
    expect(wrapper.emitted('readingSettled')).toBeUndefined()

    pending.resolve({ success: true, data: MULTI })
    await flushPromises()
    expect(wrapper.emitted('readingSettled')).toHaveLength(1)
    expect(wrapper.emitted('readingSettled')?.[0]?.[0]).toMatchObject({
      laneKey: 'other:501',
      date: DATE,
      status: 'ready',
      sectionId: '501',
      topicId: '10',
    })
  })

  it('同 context 刷新：在途不重复发；刷新终态后再发一次供父 pending 消费', async () => {
    const timeline = vi.fn(async (): Promise<ObservationResponse<ObservationTimelineData>> => ({
      success: true as const,
      data: MULTI,
    }))
    const { wrapper, observation } = mountReading({ timeline })
    await flushPromises()
    expect(wrapper.emitted('readingSettled')).toHaveLength(1)

    const pending = deferred<ObservationResponse<ObservationTimelineData>>()
    timeline.mockReturnValueOnce(pending.promise)
    void observation.retry()
    await nextTick()
    expect(wrapper.emitted('readingSettled')).toHaveLength(1)

    pending.resolve({ success: true, data: MULTI })
    await flushPromises()
    expect(wrapper.emitted('readingSettled')).toHaveLength(2)
    expect(wrapper.emitted('readingSettled')?.[1]?.[0]).toMatchObject({
      laneKey: 'other:501',
      date: DATE,
      status: 'ready',
    })
  })

  it('观察失败 / 无稳定 ID 都是终态：不挡合法阅读终态；相关文章未展开不进等待', async () => {
    const failed = mountReading({
      timeline: vi.fn(async () => ({ success: false as const, error: 'timeline 500' })),
    })
    await flushPromises()
    expect(failed.wrapper.emitted('readingSettled')?.at(-1)?.[0]).toMatchObject({
      laneKey: 'other:501',
      status: 'ready',
      topicId: '10',
    })

    const unattributed = mountReading({
      topicId: null,
      reportDay: readyDay(DATE, [section('501', '未归属分区', [thread('t1', '线索', { related: ['11'] })], null)]),
    })
    await flushPromises()
    expect(unattributed.wrapper.emitted('readingSettled')?.at(-1)?.[0]).toMatchObject({
      laneKey: 'other:501',
      status: 'ready',
      topicId: null,
    })

    // 展开线索并让相关文章请求悬着：终态不等用户尚未展开的相关文章
    const article = deferred<{ success: boolean, data?: unknown }>()
    api.loadArticleMeta.mockReturnValueOnce(article.promise)
    await unattributed.wrapper.find('[data-testid="lane-chronicle-thread-toggle-t1"]').trigger('click')
    await unattributed.wrapper.find('[data-testid="lane-chronicle-related-toggle-t1"]').trigger('click')
    await nextTick()
    expect(unattributed.wrapper.emitted('readingSettled')).toHaveLength(1)
    article.resolve({ success: true, data: {} })
    await flushPromises()
  })

  it('卸载后旧排队信号不发；换上下文只发新 date/section identity', async () => {
    const pending = deferred<ObservationResponse<ObservationTimelineData>>()
    const timeline = vi.fn(() => pending.promise)
    const settled: unknown[] = []
    const { wrapper } = mountReading({ timeline, onSettled: payload => settled.push(payload) })
    await flushPromises()
    expect(settled).toHaveLength(0)

    wrapper.unmount()
    pending.resolve({ success: true, data: { records: [], coverage: 'all' } })
    await flushPromises()
    expect(settled).toHaveLength(0)

    const switched = mountReading({
      timeline: vi.fn(async () => ({ success: true as const, data: { records: [], coverage: 'all' as const } })),
      reportDay: readyDay(DATE, [section('502', '别的分区', [thread('t9', '线索九')])]),
    })
    await flushPromises()
    const before = switched.wrapper.emitted('readingSettled')?.length ?? 0
    await switched.wrapper.setProps({ selectedSectionId: '502' })
    await flushPromises()
    const after = (switched.wrapper.emitted('readingSettled') ?? []).slice(before)
    expect(after.length).toBeGreaterThan(0)
    expect(after.every(event => (event[0] as { sectionId: string }).sectionId === '502')).toBe(true)
  })
})
