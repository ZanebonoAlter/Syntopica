import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BoardDailyReportTimeline from './BoardDailyReportTimeline.vue'
import type { DailyReport, DailyReportListItem } from '~/api/dailyReports'

const api = vi.hoisted(() => ({
  getBoardDailyReports: vi.fn(),
  getDailyReportDetail: vi.fn(),
  getTopicLifeline: vi.fn(),
  getArticle: vi.fn(),
  getWatchHits: vi.fn(),
  getLaneDynamics: vi.fn(),
  listContexts: vi.fn(),
}))

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({
    getBoardDailyReports: api.getBoardDailyReports,
    getDailyReportDetail: api.getDailyReportDetail,
    getTopicLifeline: api.getTopicLifeline,
  }),
}))

vi.mock('~/api/articles', () => ({
  useArticlesApi: () => ({ getArticle: api.getArticle }),
}))

vi.mock('~/api/topicWatches', () => ({
  useTopicWatchesApi: () => ({ getWatchHits: api.getWatchHits }),
}))

vi.mock('~/api/laneDynamics', () => ({
  useLaneDynamicsApi: () => ({ getLaneDynamics: api.getLaneDynamics }),
}))

vi.mock('~/api/boardEnrichment', () => ({
  useBoardEnrichmentApi: () => ({ listContexts: api.listContexts }),
}))

vi.mock('@floating-ui/vue', () => ({
  useFloating: () => ({ floatingStyles: { value: {} } }),
}))

vi.mock('@floating-ui/dom', () => ({
  autoUpdate: vi.fn(),
  offset: vi.fn(),
  shift: vi.fn(),
  flip: vi.fn(),
}))

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" />' },
}))

vi.mock('./BoardThreadBrowser.vue', () => ({
  default: { name: 'BoardThreadBrowser', template: '<div data-testid="thread-browser" />' },
}))

vi.mock('./TopicDetectiveWall.client.vue', () => ({
  default: { name: 'TopicDetectiveWall', template: '<div data-testid="detective-wall" />' },
}))

const reports: DailyReportListItem[] = [
  {
    id: 60,
    semantic_board_id: 1974,
    period_date: '2026-06-21T12:00:00Z',
    title: '6 月 21 日日报',
    summary: '今日摘要',
    status: 'done',
    cluster_count: 1,
    article_count: 4,
    event_tag_count: 2,
    created_at: '2026-06-21T12:00:00Z',
    activeWatchSummaries: [
      { watchId: 1, label: 'ASML', type: 'keyword' },
      { watchId: 2, label: '中东局势', type: 'label' },
      { watchId: 3, label: '出口限制', type: 'keyword' },
    ],
  },
  {
    id: 52,
    semantic_board_id: 1974,
    period_date: '2026-06-20T12:00:00Z',
    title: '6 月 20 日日报',
    summary: '昨日摘要',
    status: 'done',
    cluster_count: 1,
    article_count: 2,
    event_tag_count: 1,
    created_at: '2026-06-20T12:00:00Z',
  },
]

function makeDetail(id: number): DailyReport {
  return {
    ...reports.find(report => report.id === id)!,
    highlights: [{ title: '头条', reason: '值得关注', tag_ids: [] }],
    dynamics: '',
    sections: [{
      id: 366,
      cluster_index: 0,
      cluster_label: '霍尔木兹海峡航运恢复',
      cluster_tag_ids: [],
      article_count: 2,
      best_tier: 0,
      avg_score: 0.9,
      persistent_topic_id: 5,
      persistent_topic: {
        id: 5,
        label: '霍尔木兹海峡航运恢复',
        status: 'active',
        color: '#b44f45',
        consecutive_hits: 3,
        can_activate: false,
      },
      threads: [{
        id: 10,
        report_id: id,
        section_id: 366,
        title: '通行量逐步恢复',
        summary: '航运风险开始回落。',
        tag_ids: [],
        confidence: 0.9,
        related_article_ids: [99],
        created_at: '2026-06-21T12:00:00Z',
      }],
    }],
  }
}

async function mountTimeline() {
  const wrapper = mount(BoardDailyReportTimeline, {
    attachTo: document.body,
    props: { boardId: 1974 },
  })
  await flushPromises()
  return wrapper
}

describe('BoardDailyReportTimeline preserved behavior', () => {
  beforeEach(() => {
    api.getBoardDailyReports.mockResolvedValue({ success: true, data: { reports } })
    api.getDailyReportDetail.mockImplementation(async (id: number) => ({ success: true, data: { report: makeDetail(id) } }))
    api.getArticle.mockResolvedValue({ success: true, data: { id: 99, title: '航运恢复观察' } })
    api.getWatchHits.mockResolvedValue({ success: true, data: [] })
    api.getLaneDynamics.mockResolvedValue({
      success: true,
      data: {
        window_days: 14,
        has_reports: true,
        lanes: [{
          topic_id: 5,
          label: '霍尔木兹海峡航运恢复',
          watch_linked: false,
          section_count_14d: 2,
          snapshot: { summary: '短版态势', detail: '长版全文态势叙述', as_of: '2026-06-21' },
          timeline: [{ date: '2026-06-21', sections: [{ section_id: 366, label: '航运', events: ['通行量逐步恢复'] }] }],
        }],
        candidates: [],
      },
    })
    api.listContexts.mockResolvedValue({ success: true, data: [] })
  })

  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('opens a report, navigates dates, and closes with Escape', async () => {
    const wrapper = await mountTimeline()
    const reportCard = wrapper.find('.drt-summary-card')
    ;(reportCard.element as HTMLElement).focus()
    await reportCard.trigger('click')
    await flushPromises()

    expect(document.body.querySelector('.drm-overlay')).not.toBeNull()
    expect(document.body.textContent).toContain('6 月 21 日')

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }))
    await flushPromises()
    expect(document.body.textContent).toContain('6 月 20 日')

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(document.body.querySelector('.drm-overlay')).toBeNull()
    expect(document.activeElement).toBe(reportCard.element)
  })

  it('renders at most two active watch previews and locates the tagged section without list N+1 requests', async () => {
    api.getWatchHits.mockResolvedValue({
      success: true,
      data: [{
        id: 'watch-hit-1',
        watchId: '1',
        sectionId: '366',
        reportId: '60',
        periodDate: '2026-06-21',
        reason: '含关键字『ASML』',
        watchLabel: 'ASML',
        watchType: 'keyword',
      }],
    })

    const wrapper = await mountTimeline()
    expect(wrapper.findAll('[data-testid="watch-preview"]')).toHaveLength(1)
    expect(wrapper.findAll('.drt-watch-preview__tag')).toHaveLength(2)
    expect(wrapper.find('.drt-watch-preview__more').text()).toBe('+1')
    expect(api.getBoardDailyReports).toHaveBeenCalledOnce()

    await wrapper.find('.drt-watch-preview__tag').trigger('click')
    await flushPromises()
    expect(api.getWatchHits).toHaveBeenCalledWith(60)
    expect(document.body.querySelector('#report-section-366')).not.toBeNull()

    const watchIndex = document.body.querySelector('[data-testid="watch-index"]')
    const contentColumn = document.body.querySelector('.drm-content')
    expect(watchIndex).not.toBeNull()
    expect(contentColumn?.contains(watchIndex)).toBe(true)
  })

  it('keeps topic overview entry point', async () => {
    const wrapper = await mountTimeline()

    await wrapper.find('.drt-browser-toggle').trigger('click')
    expect(wrapper.find('[data-testid="thread-browser"]').exists()).toBe(true)
    await wrapper.find('.drt-browser-toggle').trigger('click')

    await wrapper.find('.drt-summary-card').trigger('click')
    await flushPromises()
    const topicHeader = document.body.querySelector('.drm-topic__header') as HTMLElement
    if (topicHeader.getAttribute('aria-expanded') !== 'true') topicHeader.click()
    await nextTick()
    expect(document.body.querySelector('.drm-thread__header')).not.toBeNull()
  })

  it('emits openArticle from a thread article', async () => {
    const wrapper = await mountTimeline()
    await wrapper.find('.drt-summary-card').trigger('click')
    await flushPromises()

    const topicHeader = document.body.querySelector('.drm-topic__header') as HTMLElement
    if (topicHeader.getAttribute('aria-expanded') !== 'true') topicHeader.click()
    await nextTick()
    ;(document.body.querySelector('.drm-thread__header') as HTMLElement).click()
    await flushPromises()
    ;(document.body.querySelector('.drm-article') as HTMLElement).click()
    await nextTick()

    expect(wrapper.emitted('openArticle')).toEqual([[99]])
  })
})

describe('BoardDailyReportTimeline — 泳道趋势区宿主取数（lane-trend-overview）', () => {
  // 隔离：上一测试的组件实例（attachTo body）不自动卸载，其挂起 watcher 会在后续测试里
  // 继续发请求；这里逐测试 unmount + 重置 mock，保证 HD 断言的调用计数只属本测试。
  let mounted: Array<{ unmount: () => void }> = []

  /** active zone fixture：topic_status_at_report='active' 才进 active 泳道（自动展开 + 趋势区挂载条件）。 */
  function makeActiveDetail(id: number): DailyReport {
    const detail = makeDetail(id)
    return {
      ...detail,
      sections: detail.sections.map(section => ({ ...section, topic_status_at_report: 'active' as const })),
    }
  }

  beforeEach(() => {
    mounted = []
    vi.resetAllMocks()
    api.getBoardDailyReports.mockResolvedValue({ success: true, data: { reports } })
    api.getDailyReportDetail.mockImplementation(async (id: number) => ({ success: true, data: { report: makeActiveDetail(id) } }))
    api.getArticle.mockResolvedValue({ success: true, data: { id: 99, title: '航运恢复观察' } })
    api.getWatchHits.mockResolvedValue({ success: true, data: [] })
    api.getTopicLifeline.mockResolvedValue({ success: true, data: { sections: [], relations: [] } })
    api.getLaneDynamics.mockResolvedValue({
      success: true,
      data: {
        window_days: 14,
        has_reports: true,
        lanes: [{
          topic_id: 5,
          label: '霍尔木兹海峡航运恢复',
          watch_linked: false,
          section_count_14d: 2,
          snapshot: { summary: '短版态势', detail: '长版全文态势叙述', as_of: '2026-06-21' },
          timeline: [{ date: '2026-06-21', sections: [{ section_id: 366, label: '航运', events: ['通行量逐步恢复'] }] }],
        }],
        candidates: [],
      },
    })
    api.listContexts.mockResolvedValue({ success: true, data: [] })
  })

  afterEach(() => {
    mounted.splice(0).forEach(instance => instance.unmount())
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  it('HD-1/HD-2: 首个泳道展开触发一次板块级请求；翻期不重拉（趋势锚定最新报告期）', async () => {
    const wrapper = await mountTimeline()
    mounted.push(wrapper)
    await wrapper.find('.drt-summary-card').trigger('click')
    await flushPromises()

    // 自动展开首个泳道 → 一次板块级 lane-dynamics（days=14）
    expect(api.getLaneDynamics).toHaveBeenCalledTimes(1)
    expect(api.getLaneDynamics).toHaveBeenCalledWith(1974, 14)
    expect(document.body.querySelector('[data-testid="lane-trend-overview"]')).not.toBeNull()
    expect(document.body.querySelector('[data-testid="trend-detail"]')?.textContent).toContain('长版全文态势叙述')

    // 翻期（本实例工具栏「较早一期」按钮）：泳道组件重挂、信号再发，但宿主缓存命中不重拉。
    // 不用 document 级 keydown 广播：同文件更早测试的未卸载实例会一并响应（测试卫生）。
    const navButtons = document.body.querySelectorAll('.drm-toolbar button')
    ;(navButtons[0] as HTMLElement).click()
    await flushPromises()
    expect(document.body.textContent).toContain('6 月 20 日')
    expect(api.getLaneDynamics).toHaveBeenCalledTimes(1)
    // 趋势区仍渲染（缓存数据仍在，与所读报告日期解耦）
    expect(document.body.querySelector('[data-testid="lane-trend-overview"]')).not.toBeNull()
  })

  it('HD-3: lane-dynamics 失败只隔离在趋势区：展开体照常渲染，错误条可重试', async () => {
    api.getLaneDynamics.mockRejectedValueOnce(new Error('网络错误'))
    const wrapper = await mountTimeline()
    mounted.push(wrapper)
    await wrapper.find('.drt-summary-card').trigger('click')
    await flushPromises()

    // 泳道展开体既有内容不受影响
    expect(document.body.querySelector('.drm-section-card')).not.toBeNull()
    expect(document.body.querySelector('.drm-thread__header')).not.toBeNull()

    // 趋势区内联错误条
    const errorBar = document.body.querySelector('[data-testid="trend-error"]')
    expect(errorBar).not.toBeNull()
    expect(errorBar?.textContent).toContain('网络错误')

    // 重试 → force 重拉成功 → 趋势区渲染长版
    ;(document.body.querySelector('[data-testid="trend-retry"]') as HTMLElement).click()
    await flushPromises()
    expect(api.getLaneDynamics).toHaveBeenCalledTimes(2)
    expect(document.body.querySelector('[data-testid="trend-detail"]')?.textContent).toContain('长版全文态势叙述')
  })

  it('HD-4: contexts 按档位按需拉取且命中缓存：切档才发、回切不再发', async () => {
    const wrapper = await mountTimeline()
    mounted.push(wrapper)
    await wrapper.find('.drt-summary-card').trigger('click')
    await flushPromises()
    expect(api.listContexts).not.toHaveBeenCalled()

    // 切月档 → listContexts(5, 'month') 一次
    ;(document.body.querySelector('[data-testid="trend-tab-month"]') as HTMLElement).click()
    await flushPromises()
    expect(api.listContexts).toHaveBeenCalledTimes(1)
    expect(api.listContexts).toHaveBeenCalledWith(5, 'month')

    // 切年档 → 独立请求 year
    ;(document.body.querySelector('[data-testid="trend-tab-year"]') as HTMLElement).click()
    await flushPromises()
    expect(api.listContexts).toHaveBeenCalledTimes(2)
    expect(api.listContexts).toHaveBeenLastCalledWith(5, 'year')

    // 回切月档 → 命中缓存不再发
    ;(document.body.querySelector('[data-testid="trend-tab-month"]') as HTMLElement).click()
    await flushPromises()
    expect(api.listContexts).toHaveBeenCalledTimes(2)
  })
})
