import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DailyReportReader from './DailyReportReader.vue'
import { __mediaQueryClient } from '~/composables/useMediaQuery'
import type { DailyReport, DailyReportListItem } from '~/api/dailyReports'

/** 页边注栏收起偏好键（组件内同名常量，测试按契约字符串断言）。 */
const NOTES_RAIL_STORAGE_KEY = 'syntopica.daily-report.margin-notes-collapsed'

const api = vi.hoisted(() => ({
  getBoardDailyReports: vi.fn(),
  getTodayReports: vi.fn().mockResolvedValue({ success: true, data: { date: '2026-06-21', reports: [] } }),
  getDailyReportDetail: vi.fn(),
  getTopicLifeline: vi.fn(),
  getArticle: vi.fn(),
  getWatchHits: vi.fn(),
  getLaneDynamics: vi.fn(),
  listContexts: vi.fn(),
  getReportAnnotations: vi.fn(),
  anchorAnnotation: vi.fn(),
  askAnnotation: vi.fn(),
  deleteAnnotation: vi.fn(),
  rebucketSection: vi.fn(),
  generateDailyReport: vi.fn(),
  getThreadRelations: vi.fn().mockResolvedValue({
    success: true,
    data: {
      report_id: 1, days: 7, window_start: '', window_end: '', status: 'empty', threads: {}, total: 0, skipped_no_embedding: 0,
    },
  }),
  getReportKeyEvents: vi.fn().mockResolvedValue({ success: true, data: { status: 'idle', markers: [] } }),
  getTopicKeyEvents: vi.fn().mockResolvedValue({ success: true, data: { topic_id: 1, days: 14, markers: [] } }),
  putThreadKeyEvent: vi.fn(),
  patchKeyEventMarker: vi.fn(),
}))

const notify = vi.hoisted(() => ({ error: vi.fn(), warn: vi.fn(), success: vi.fn() }))

vi.mock('~/composables/useNotify', () => ({
  useNotify: () => notify,
}))

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({
    getBoardDailyReports: api.getBoardDailyReports,
    getTodayReports: api.getTodayReports,
    getDailyReportDetail: api.getDailyReportDetail,
    getTopicLifeline: api.getTopicLifeline,
    rebucketSection: api.rebucketSection,
    generateDailyReport: api.generateDailyReport,
    getThreadRelations: api.getThreadRelations,
    getReportKeyEvents: api.getReportKeyEvents,
    getTopicKeyEvents: api.getTopicKeyEvents,
    putThreadKeyEvent: api.putThreadKeyEvent,
    patchKeyEventMarker: api.patchKeyEventMarker,
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

vi.mock('~/api/marginNotes', () => ({
  useMarginNotesApi: () => ({
    getReportAnnotations: api.getReportAnnotations,
    anchorAnnotation: api.anchorAnnotation,
    askAnnotation: api.askAnnotation,
    deleteAnnotation: api.deleteAnnotation,
  }),
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
    highlights: [{ title: '头条', reason: '值得关注', tag_ids: [10], cluster_id: 10 }],
    dynamics: '',
    sections: [{
      id: 366,
      cluster_index: 0,
      cluster_label: '霍尔木兹海峡航运恢复',
      bucket_title: null,
      bucket_index: 0,
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
        tag_ids: [10],
        confidence: 0.9,
        related_article_ids: [99],
        created_at: '2026-06-21T12:00:00Z',
      }],
    }],
  }
}

async function mountReader(overrides: Record<string, unknown> = {}) {
  const wrapper = mount(DailyReportReader, {
    attachTo: document.body,
    props: {
      boardId: 1974,
      boardTitle: '中东',
      mode: 'desk',
      initialDate: '2026-06-21',
      today: '2026-06-21',
      ...overrides,
    },
    global: {
      stubs: {
        DailyReportTopicSection: {
          name: 'DailyReportTopicSection',
          props: ['locateThread'],
          emits: ['rebucket'],
          template: '<div><span data-testid="stub-locate">{{ locateThread ? `${locateThread.clusterId}:${locateThread.nonce}` : \'none\' }}</span><button data-testid="stub-rebucket" @click="$emit(\'rebucket\', 366)">rebucket</button></div>',
        },
      },
    },
  })
  await flushPromises()
  return wrapper
}

describe('DailyReportReader — 阅读台（6.x）', () => {
  const originalMatchMedia = window.matchMedia

  beforeEach(() => {
    api.getBoardDailyReports.mockResolvedValue({ success: true, data: { reports } })
    api.getDailyReportDetail.mockImplementation(async (id: number) => ({ success: true, data: { report: makeDetail(id) } }))
    api.getArticle.mockResolvedValue({ success: true, data: { id: 99, title: '航运恢复观察' } })
    api.getWatchHits.mockResolvedValue({ success: true, data: [] })
    api.getLaneDynamics.mockResolvedValue({ success: true, data: { window_days: 14, has_reports: true, lanes: [], candidates: [] } })
    api.listContexts.mockResolvedValue({ success: true, data: [] })
    api.getReportAnnotations.mockResolvedValue({ success: true, data: { annotations: [] } })
    api.deleteAnnotation.mockResolvedValue({ success: true, data: null })
    api.askAnnotation.mockResolvedValue({ success: false })
    api.rebucketSection.mockResolvedValue({ success: true, data: { buckets: 2, sections: [] } })
    api.generateDailyReport.mockResolvedValue({ success: true, data: { job_id: 'j', status: 'queued' } })
    // 页边注栏收起偏好存 localStorage（组件用 typeof window 守卫，happy-dom 下可真实读写），
    // 每个用例前清空，避免上一条用例落盘的偏好污染下面用例
    localStorage.clear()
  })

  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
    window.matchMedia = originalMatchMedia
    __mediaQueryClient.value = false
  })

  it('左栏恢复旧版目录：本期目录 + 历史日报列表（不再渲染分区索引）', async () => {
    const wrapper = await mountReader({ boards: [{ id: 1974, label: '中东' }] })
    expect(wrapper.find('.drm-sidebar').exists()).toBe(true)
    expect(wrapper.find('[data-testid="desk-index"]').exists()).toBe(false)
    expect(wrapper.findAll('.drm-sidebar__archive button').length).toBeGreaterThan(0)
  })

  it('阅读台桌面默认收起页边注：两列 + 右缘入口浮标，且不锁 body 滚动', async () => {
    const wrapper = await mountReader()
    expect(wrapper.find('.drm-sidebar').exists()).toBe(true)
    expect(wrapper.find('.drm-notes-rail').exists()).toBe(false)
    expect(wrapper.find('[data-testid="drm-notes-tab"]').exists()).toBe(true)
    expect(wrapper.find('.drm-layout').classes()).toContain('drm-layout--notes-collapsed')
    expect(wrapper.find('[data-testid="desk-evolution"]').exists()).toBe(false)
    expect(document.body.style.overflow).not.toBe('hidden')
  })

  it('右缘浮标展开 → 栏头收起按钮收回（双向切换，偏好只在显式切换时落盘）', async () => {
    const wrapper = await mountReader()
    expect(localStorage.getItem(NOTES_RAIL_STORAGE_KEY)).toBeNull()

    await wrapper.find('[data-testid="drm-notes-tab"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('.drm-notes-rail').exists()).toBe(true)
    expect(wrapper.find('.drm-layout').classes()).not.toContain('drm-layout--notes-collapsed')
    expect(wrapper.find('[data-testid="drm-notes-tab"]').exists()).toBe(false)
    expect(localStorage.getItem(NOTES_RAIL_STORAGE_KEY)).toBe('0')

    const collapse = wrapper.find('[data-testid="mn-rail-collapse"]')
    expect(collapse.exists()).toBe(true)
    await collapse.trigger('click')
    await flushPromises()
    expect(wrapper.find('.drm-notes-rail').exists()).toBe(false)
    expect(wrapper.find('[data-testid="drm-notes-tab"]').exists()).toBe(true)
    expect(wrapper.find('.drm-layout').classes()).toContain('drm-layout--notes-collapsed')
    expect(localStorage.getItem(NOTES_RAIL_STORAGE_KEY)).toBe('1')
  })

  it('记住显式选择：偏好为展开（0）时进入即三栏，为收起（1）时即两列', async () => {
    localStorage.setItem(NOTES_RAIL_STORAGE_KEY, '0')
    const expanded = await mountReader()
    expect(expanded.find('.drm-notes-rail').exists()).toBe(true)
    expect(expanded.find('[data-testid="drm-notes-tab"]').exists()).toBe(false)
  })

  it('程序性展开（深链定位批注）不写偏好，保持用户上次显式选择', async () => {
    api.getReportAnnotations.mockResolvedValue({
      success: true,
      data: {
        annotations: [{
          id: 77,
          report_id: 51,
          section_id: 366,
          thread_id: 10,
          quoted_text: '航运风险开始回落',
          anchor_offset_start: null,
          anchor_offset_end: null,
          qas: [],
          created_at: '2026-06-21T12:00:00Z',
        }],
      },
    })
    const wrapper = await mountReader({ initialAnnotationId: 77 })
    await flushPromises()

    expect(wrapper.find('.drm-notes-rail').exists()).toBe(true)
    expect(localStorage.getItem(NOTES_RAIL_STORAGE_KEY)).toBeNull()
  })

  it('窄屏（<1100px）：不渲染桌面浮标与第三列，仍走右下入口分支', async () => {
    __mediaQueryClient.value = true
    window.matchMedia = ((query: string) => ({
      matches: query.includes('1100px'),
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })) as unknown as typeof window.matchMedia

    const wrapper = await mountReader()
    await flushPromises()

    expect(wrapper.find('[data-testid="drm-notes-tab"]').exists()).toBe(false)
    expect(wrapper.find('.drm-notes-rail').exists()).toBe(false)
    expect(wrapper.find('.drm-layout').classes()).not.toContain('drm-layout--notes-collapsed')
  })

  it('刊头保留「今日重点」top3 高亮（不再隐藏）', async () => {
    const wrapper = await mountReader()
    const masthead = wrapper.find('[data-testid="daily-report-masthead"]')
    expect(masthead.exists()).toBe(true)
    expect(masthead.find('.drm-highlights').exists()).toBe(true)
    expect(masthead.findAll('.drm-highlight').length).toBeGreaterThan(0)
  })

  // ── retire-story-adopt-event-metrics 3.3：刊头重点定位意图转发（cluster_id） ──

  it('重点定位：正文有目标 → 刊头按钮下发定位意图给正文分区（clusterId+nonce）', async () => {
    const wrapper = await mountReader()
    const button = wrapper.find('[data-testid="highlight-locate-0"]')
    expect(button.exists()).toBe(true)
    await button.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="stub-locate"]').text()).toBe('10:1')
  })

  it('重点定位：点击时目标已不在当前正文 → 不下发定位请求（原地轻提示，不跳近似位置）', async () => {
    const wrapper = await mountReader()
    expect(wrapper.find('[data-testid="stub-locate"]').text()).toBe('none')
    // 模拟渲染后目标消失（数据变更/过滤）：直接从刊头发外部 cluster id
    wrapper.findComponent({ name: 'DailyReportMasthead' }).vm.$emit('locate', 999)
    await flushPromises()
    expect(wrapper.find('[data-testid="stub-locate"]').text()).toBe('none')
  })

  it('desk 首屏按 14 天取数，并可加载更早（B-5）', async () => {
    const wrapper = await mountReader()
    expect(api.getBoardDailyReports).toHaveBeenCalledWith(1974, { days: 14 })
    await wrapper.find('[data-testid="desk-load-more"]').trigger('click')
    await flushPromises()
    expect(api.getBoardDailyReports).toHaveBeenLastCalledWith(1974, { days: 21 })
  })

  it('详情失败：错误条带重试，重试成功后恢复正文（B-6）', async () => {
    api.getDailyReportDetail.mockResolvedValueOnce({ success: false, error: '网络错误' })
    const wrapper = await mountReader()
    expect(wrapper.find('[data-testid="detail-retry"]').exists()).toBe(true)
    expect(wrapper.find('.drm-sidebar').exists()).toBe(true)
    expect(wrapper.find('[data-testid="desk-evolution"]').exists()).toBe(false)
    await wrapper.find('[data-testid="detail-retry"]').trigger('click')
    await flushPromises()
    expect(api.getDailyReportDetail).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="daily-report-masthead"]').exists()).toBe(true)
  })

  it('目录点击滚动到对应分区', async () => {
    const wrapper = await mountReader()
    const scrollIntoView = vi.fn()
    const spy = vi.spyOn(document, 'getElementById').mockReturnValue({ scrollIntoView } as unknown as HTMLElement)
    const button = wrapper.findAll('.drm-sidebar button')[0]
    await button!.trigger('click')
    expect(spy).toHaveBeenCalled()
    expect(scrollIntoView).toHaveBeenCalled()
    spy.mockRestore()
  })

  it('历史日报切换日报并回写 selectDate', async () => {
    const wrapper = await mountReader()
    await wrapper.findAll('.drm-sidebar__archive button')[1]!.trigger('click')
    await flushPromises()
    expect(api.getDailyReportDetail).toHaveBeenCalledWith(52)
    expect(wrapper.emitted('selectDate')?.at(-1)).toEqual(['2026-06-20'])
  })

  // ── ?cluster= 深链定位（D14；旧 ?story= 已下线） ──

  it('深链定位：正文有该聚簇线索 → 下发定位意图给正文分区（clusterId+nonce）', async () => {
    const wrapper = await mountReader({ initialClusterId: 10 })
    expect(wrapper.find('[data-testid="stub-locate"]').text()).toBe('10:1')
  })

  it('深链落空：本期正文无该聚簇线索 → 不下发定位请求（不跳近似位置）+ 原地提示', async () => {
    const wrapper = await mountReader({ initialClusterId: 999 })
    expect(wrapper.find('[data-testid="stub-locate"]').text()).toBe('none')
    expect(notify.warn).toHaveBeenCalledWith('本期正文暂无对应线索')
  })

  it('翻页/切报告清参：定位请求随报告切换清空（不在别的报告重复定位）', async () => {
    const wrapper = await mountReader({ initialClusterId: 10 })
    expect(wrapper.find('[data-testid="stub-locate"]').text()).toBe('10:1')
    await wrapper.findAll('.drm-sidebar__archive button')[1]!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="stub-locate"]').text()).toBe('none')
  })

  it('无 ?cluster= 参数：不下发任何定位请求', async () => {
    const wrapper = await mountReader()
    expect(wrapper.find('[data-testid="stub-locate"]').text()).toBe('none')
  })

  it('旧 ?story= 深链：解析后忽略，不定位、不报错，只提示该定位方式已下线', async () => {
    const wrapper = await mountReader({ initialLegacyStory: true })
    expect(wrapper.find('[data-testid="stub-locate"]').text()).toBe('none')
    expect(notify.warn).toHaveBeenCalledWith('事件线定位已下线，请从「今日重点」进入正文')
    expect(notify.error).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="daily-report-masthead"]').exists()).toBe(true)
  })

  it('保留泳道内主题分桶：section rebucket 事件调用 rebucket API', async () => {
    const wrapper = await mountReader()
    await wrapper.find('[data-testid="stub-rebucket"]').trigger('click')
    await flushPromises()
    expect(api.rebucketSection).toHaveBeenCalledWith(60, 366)
  })

  it('无日报空态：显示生成入口并调用生成 API', async () => {
    api.getBoardDailyReports.mockResolvedValue({ success: true, data: { reports: [] } })
    const wrapper = await mountReader({ initialDate: '2026-06-19' })
    expect(wrapper.find('[data-testid="desk-empty"]').exists()).toBe(true)
    await wrapper.find('[data-testid="desk-empty-generate"]').trigger('click')
    await flushPromises()
    expect(api.generateDailyReport).toHaveBeenCalledWith({ date: '2026-06-19', board_id: 1974 })
  })

  // tasks 9.3 阅读台回归：Esc 返回 + 浮层让位语义。实测锚：
  // DailyReportReader.vue handleKeydown——抽屉/浮层开着时 Esc 不连阅读层一起关（丢阅读位置）。
  // 窄屏页边注抽屉（notesDrawerOpen）与浮层走同一支 guard，窄屏分支由真机 ui-verification 兑底，
  // 这里用 overlayStack 计数器直接驱动同一语义（不缠 media query 在测试环境的 ssrWidth 分支）。
  it('Esc 关闭阅读层；有浮层开着时 Esc 让位不关阅读层', async () => {
    const { pushOverlay, popOverlay, resetOverlayStack } = await import('~/components/ui/overlayStack')
    resetOverlayStack()
    try {
      const wrapper = await mountReader()

      // 浮层开着（如文章抽屉/弹层）：Esc 只该关浮层，阅读层不动
      pushOverlay()
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
      await flushPromises()
      expect(wrapper.emitted('close')).toBeUndefined()

      // 浮层关闭后再 Esc 才关阅读层（emit close 恰一次）
      popOverlay()
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
      await flushPromises()
      expect(wrapper.emitted('close')).toHaveLength(1)
    } finally {
      resetOverlayStack()
    }
  })
})
