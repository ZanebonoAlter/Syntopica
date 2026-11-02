/**
 * LaneChronicle 测试（矩阵阶段 G4，test-cases-matrix-stage.md CH 组）。
 *
 *  - CH-2 稳定 sectionId join 全量 threads（>5 条可读）/ 同名不同 threadId 各自摘要
 *  - CH-3 报告已更新/section 缺失 → base 标题 + 折叠提示 + 刷新；CH-4 当日无报告
 *  - CH-5 标题默认收束、单条互斥展开；切日收束
 *  - CH-6 相关文章按需（不预取/空列表/单项失败重试不改总数）；文章主点击 emit open-article
 *  - CH-7 上一/下一有内容日（跳过无分区与未取到日；边界禁用）只 emit 日期
 *  - CH-8 历史日报动作带完整日期保留；CH-9 三指标 caption 口径各注；CH-10 只读
 *  - 状态分层：error+重试 / stale 保留旧内容 / gap 与 unavailable 区分
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import LaneChronicle from './LaneChronicle.vue'
import type { ObservationLaneRow } from './observationLanes'
import type { WindowLaneDay, WindowSection } from '~/api/normalizers/laneDynamicsWindow'
import type { LaneDayResolution } from '~/features/tags/composables/useObservationWindow'
import type { LaneDayReportDetail, LaneDayReportIndexItem } from '~/api/normalizers/laneDayReading'

const api = vi.hoisted(() => ({
  loadReportIndex: vi.fn(),
  loadReportDetail: vi.fn(),
  loadArticleMeta: vi.fn(),
}))
vi.mock('~/api/laneDayReading', async (importOriginal) => {
  const actual = await importOriginal<typeof import('~/api/laneDayReading')>()
  return { ...actual, useLaneDayReadingApi: () => api }
})
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))

const TODAY = '2026-10-23'
const ANCHOR = '2026-10-22'
const AXIS = ['2026-10-18', '2026-10-19', '2026-10-20', '2026-10-21', '2026-10-22']

const LANE: ObservationLaneRow = {
  topicId: '10', label: '主线 A', watchLinked: false, sectionTotal: 9, clueTotal: 12, dormant: false,
  snapshot: null,
}

function section(id: string, overrides: Partial<WindowSection> = {}): WindowSection {
  return {
    sectionId: id,
    label: `桶${id}`,
    threadCount: 'threadCount' in overrides ? overrides.threadCount! : 1,
    fallbackClueCount: 'fallbackClueCount' in overrides ? overrides.fallbackClueCount! : null,
    foldedCount: overrides.foldedCount ?? 0,
    eventTitles: overrides.eventTitles ?? ['聚合标题'],
  }
}

function resolveDay(topicId: string, date: string): LaneDayResolution {
  if (topicId !== '10') return { kind: 'gap' }
  const days: Record<string, WindowLaneDay> = {
    '2026-10-22': { date, sections: [section('501', { threadCount: 3, eventTitles: ['聚合甲', '聚合乙'] }), section('502', { threadCount: 1, eventTitles: [] })] },
    '2026-10-20': { date, sections: [section('503', { threadCount: 2, foldedCount: 2, eventTitles: ['聚合丙'] }), section('505', { threadCount: 1, eventTitles: ['聚合丁'] })] },
    '2026-10-18': { date, sections: [section('504', { threadCount: 1 })] },
  }
  const day = days[date]
  if (day) return { kind: 'content', source: 'tier', day }
  return date === '2026-10-19' ? { kind: 'unavailable' } : { kind: 'gap' }
}

const INDEX: LaneDayReportIndexItem[] = [
  { reportId: '1211', periodDate: '2026-10-22', title: '日报', createdAt: '' },
  { reportId: '1210', periodDate: '2026-10-20', title: '日报', createdAt: '' },
  { reportId: '1209', periodDate: '2026-10-18', title: '日报', createdAt: '' },
]

function threads(count: number, sectionId: string) {
  return Array.from({ length: count }, (_, i) => ({
    threadId: `t${i + 1}`, sectionId, title: i < 2 ? '同名线索标题' : `线索${i + 1}`,
    summary: i < 2 ? (i === 0 ? '第一条的独立摘要' : '同名但不同 ID 的摘要') : `第${i + 1}条摘要`,
    confidence: 0.8, relatedArticleIds: i === 0 ? ['11', '12', '13'] : [], fitDistance: null, createdAt: '',
  }))
}

const REPORT_1211: LaneDayReportDetail = {
  reportId: '1211', boardId: '1974', periodDate: '2026-10-22',
  sections: [
    { sectionId: '501', clusterLabel: '桶501', bucketTitle: null, threads: threads(6, '501') },
    { sectionId: '502', clusterLabel: '桶502', bucketTitle: null, threads: [] },
  ],
}
const REPORT_1210_MISSING: LaneDayReportDetail = {
  reportId: '1210', boardId: '1974', periodDate: '2026-10-20',
  sections: [{ sectionId: '999', clusterLabel: '别的桶', bucketTitle: null, threads: [] }],
}
const REPORT_1209: LaneDayReportDetail = {
  reportId: '1209', boardId: '1974', periodDate: '2026-10-18',
  sections: [{ sectionId: '504', clusterLabel: '桶504', bucketTitle: null, threads: [{ threadId: 'u1', sectionId: '504', title: '早期线索', summary: '早期摘要', confidence: 0.7, relatedArticleIds: [], fitDistance: null, createdAt: '' }] }],
}

function mockHappyPath(): void {
  api.loadReportIndex.mockResolvedValue({ success: true, data: INDEX })
  api.loadReportDetail.mockImplementation((reportId: string) => {
    const detail = reportId === '1211' ? REPORT_1211 : reportId === '1210' ? REPORT_1210_MISSING : REPORT_1209
    return Promise.resolve({ success: true, data: detail })
  })
  api.loadArticleMeta.mockImplementation((articleId: string) =>
    articleId === '12'
      ? Promise.resolve({ success: false, error: '404' })
      : Promise.resolve({ success: true, data: { articleId, title: `文章${articleId}`, pubDate: '2026-10-22T09:00:00+08:00', read: false, article: { id: articleId } } }))
}

function mountChronicle(overrides: Record<string, unknown> = {}) {
  return mount(LaneChronicle, {
    props: {
      boardId: 1974,
      lane: LANE,
      selectedDate: '2026-10-22',
      axis: AXIS,
      today: TODAY,
      anchorDate: ANCHOR,
      resolveDay,
      ...overrides,
    },
  })
}

beforeEach(() => {
  api.loadReportIndex.mockReset()
  api.loadReportDetail.mockReset()
  api.loadArticleMeta.mockReset()
  mockHappyPath()
})
afterEach(() => {
  vi.clearAllMocks()
})

describe('CH-2 — 阅读终态信号（R34）', () => {
  it('详情就绪后 emit 当前 lane/date/status；gap 为终态也发', async () => {
    const ready = mountChronicle()
    await flushPromises()
    const payload = ready.emitted('readingSettled')?.at(-1)?.[0] as { laneKey: string, date: string, status: string }
    expect(payload.laneKey).toBe('10')
    expect(payload.date).toBe('2026-10-22')
    expect(typeof payload.status).toBe('string')

    const gap = mountChronicle({ selectedDate: '2026-10-21' })
    await flushPromises()
    expect(gap.emitted('readingSettled')?.at(-1)?.[0]).toMatchObject({
      laneKey: '10',
      date: '2026-10-21',
      status: 'gap',
    })
  })
})

describe('CH-2 — 空摘要文案（N3）', () => {
  it('loaded 真实 thread.summary="" 展开显示「暂无摘要记录」；未取到报告不冒空态', async () => {
    const emptyThread = {
      threadId: 'empty1', sectionId: '501', title: '空摘要线索', summary: '',
      confidence: 1, relatedArticleIds: [], fitDistance: null, createdAt: '',
    }
    api.loadReportDetail.mockResolvedValue({
      success: true,
      data: {
        ...REPORT_1211,
        sections: [{ ...REPORT_1211.sections[0]!, threads: [emptyThread] }],
      },
    })
    const loaded = mountChronicle()
    await flushPromises()
    await loaded.find('[data-testid="lane-chronicle-thread-toggle-empty1"]').trigger('click')
    const row = loaded.find('[data-testid="lane-chronicle-thread-empty1"]')
    expect(row.text()).toContain('暂无摘要记录')
    expect(row.text()).not.toContain('原始摘要为空')

    const unavailable = mountChronicle({ selectedDate: '2026-10-19' })
    await flushPromises()
    expect(unavailable.text()).not.toContain('暂无摘要记录')
  })
})

describe('CH-2 — 业务大小写（R39）', () => {
  it('编年 kicker 业务泳道名保留 API 大小写，仅系统 kicker 维持大写', async () => {
    const wrapper = mountChronicle({ lane: { ...LANE, label: 'Agent 架构重构与标准化协作生态' } })
    await flushPromises()
    expect(wrapper.find('.lch-kicker-system').text()).toBe('当日线索')
    expect(wrapper.find('.lch-lane-label').text()).toBe('Agent 架构重构与标准化协作生态')
    expect(wrapper.find('.lch-kicker').text()).toContain('Agent 架构重构与标准化协作生态')
  })
})

describe('CH-2 — 稳定 ID join 全量标题', () => {
  it('>5 条可读：6 条全量渲染、标题逐字、默认全部收束', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    const section501 = wrapper.find('[data-testid="lane-chronicle-section-501"]')!
    const rows = section501.findAll('.lct')
    expect(rows).toHaveLength(6)
    expect(section501.find('.lct-body').exists()).toBe(false)
    expect(section501.text()).not.toContain('第5条摘要') // 收束：摘要不可见
    expect(section501.text()).toContain('同名线索标题')
    expect(section501.text()).toContain('线索6')
  })

  it('0 线索桶合法：当日 0 条线索文案，不误报为失败', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-section-502"]').text()).toContain('当日 0 条线索')
    expect(wrapper.find('[data-testid="lane-chronicle-degraded"]').exists()).toBe(false)
  })
})

describe('CH-5 — 单条互斥展开与收束', () => {
  it('点一条就地展开真实 summary；点另一条前一条收束；再点收起', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t1"]').trigger('click')
    expect(wrapper.find('[data-testid="lane-chronicle-thread-t1"]').text()).toContain('第一条的独立摘要')
    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t2"]').trigger('click')
    expect(wrapper.find('[data-testid="lane-chronicle-thread-t2"]').text()).toContain('同名但不同 ID 的摘要')
    expect(wrapper.find('[data-testid="lane-chronicle-thread-t1"]').find('.lct-body').exists()).toBe(false)
    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t2"]').trigger('click')
    expect(wrapper.find('.lct-body').exists()).toBe(false)
  })

  it('切日默认收束', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t1"]').trigger('click')
    expect(wrapper.find('.lct-body').exists()).toBe(true)
    await wrapper.setProps({ selectedDate: '2026-10-20' })
    await flushPromises()
    expect(wrapper.find('.lct-body').exists()).toBe(false)
  })
})

describe('CH-6 — 相关文章按需', () => {
  it('展开前不请求；点开按真实 relatedArticleIds 读取；主点击 emit open-article', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t1"]').trigger('click')
    expect(api.loadArticleMeta).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="lane-chronicle-related-toggle-t1"]').text()).toContain('相关文章 · 3 篇')
    await wrapper.find('[data-testid="lane-chronicle-related-toggle-t1"]').trigger('click')
    await flushPromises()
    expect(api.loadArticleMeta).toHaveBeenCalledTimes(3)
    expect(wrapper.find('[data-testid="lane-chronicle-article-11"]').text()).toContain('文章11')
    await wrapper.find('[data-testid="lane-chronicle-article-11"]').trigger('click')
    expect(wrapper.emitted('openArticle')![0]).toEqual(['11'])
  })

  it('单项失败：局部错误 + 重试不阻塞其他条目；总数不变', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t1"]').trigger('click')
    await wrapper.find('[data-testid="lane-chronicle-related-toggle-t1"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-related-toggle-t1"]').text()).toContain('3 篇')
    expect(wrapper.find('[data-testid="lane-chronicle-article-12"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('文章 #12 加载失败')
    expect(wrapper.find('[data-testid="lane-chronicle-article-11"]').exists()).toBe(true)
    api.loadArticleMeta.mockImplementation((articleId: string) =>
      Promise.resolve({ success: true, data: { articleId, title: `文章${articleId}`, pubDate: '', read: false, article: { id: articleId } } }))
    await wrapper.find('[data-testid="lane-chronicle-article-retry-12"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-article-12"]').text()).toContain('文章12')
    expect(wrapper.find('[data-testid="lane-chronicle-related-toggle-t1"]').text()).toContain('3 篇')
  })

  it('related_article_ids 为空：明确空文案，不出按钮', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-t3"]').trigger('click')
    expect(wrapper.find('[data-testid="lane-chronicle-thread-t3"]').text()).toContain('相关文章列表为空')
    expect(wrapper.find('[data-testid="lane-chronicle-related-toggle-t3"]').exists()).toBe(false)
    expect(api.loadArticleMeta).not.toHaveBeenCalled()
  })
})

describe('CH-7 — 上一/下一有内容日', () => {
  it('跳过无分区与未取到日，就地 emit 日期（不 router/滚动）', async () => {
    const wrapper = mountChronicle({ selectedDate: '2026-10-20' })
    await flushPromises()
    const prev = wrapper.find('[data-testid="lane-chronicle-prev"]')
    const next = wrapper.find('[data-testid="lane-chronicle-next"]')
    // 内容日：10-22、10-20、10-18；10-21 gap、10-19 unavailable 被跳过
    expect(prev.attributes('title')).toBe('上一有内容日 2026-10-18')
    expect(next.attributes('title')).toBe('下一有内容日 2026-10-22')
    await prev.trigger('click')
    expect(wrapper.emitted('selectDate')![0]).toEqual(['2026-10-18'])
    expect(wrapper.emitted('selectDate')).toHaveLength(1)
  })

  it('边界：最早/最新有内容日按钮禁用 + 明确 title', async () => {
    const earliest = mountChronicle({ selectedDate: '2026-10-18' })
    await flushPromises()
    expect((earliest.find('[data-testid="lane-chronicle-prev"]').element as HTMLButtonElement).disabled).toBe(true)
    expect(earliest.find('[data-testid="lane-chronicle-prev"]').attributes('title')).toBe('已到最早有内容日')
    const latest = mountChronicle()
    await flushPromises()
    expect((latest.find('[data-testid="lane-chronicle-next"]').element as HTMLButtonElement).disabled).toBe(true)
    expect(latest.find('[data-testid="lane-chronicle-next"]').attributes('title')).toBe('已到最新有内容日')
  })
})

describe('CH-8/9/10 — 保留动作、caption 口径、只读', () => {
  it('历史日报动作带完整日期并 emit；归档动作 emit 日期', async () => {
    const wrapper = mountChronicle({ selectedDate: '2026-10-20' })
    await flushPromises()
    const open = wrapper.find('[data-testid="lane-chronicle-open-report"]')
    expect(open.text()).toContain('打开 10-20 日报')
    // 1210 缺 503 → degraded 但 reportId 仍在 → 可开（完整日期历史日报动作保留）
    expect((open.element as HTMLButtonElement).disabled).toBe(false)
    await open.trigger('click')
    expect(wrapper.emitted('openReport')![0]).toEqual([{ date: '2026-10-20', reportId: '1210' }])
    await wrapper.find('[data-testid="lane-chronicle-archive"]').trigger('click')
    expect(wrapper.emitted('viewArchive')![0]).toEqual(['2026-10-20'])
  })

  it('CH-9 三指标：默认只简短真实合计，口径收进可展开计数说明（R26）', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    // 10-22：桶 3+1 → 峰值 3、合计 4、桶数 2；默认只显当日合计，details 默认折叠
    expect(wrapper.find('[data-testid="lane-chronicle-metrics"]').text()).toBe('当日合计 4 条')
    const details = wrapper.find('[data-testid="lane-chronicle-metrics-details"]')
    expect(details.exists()).toBe(true)
    expect(details.attributes('open')).toBeUndefined()
    expect(details.find('.lch-metrics-detail').text()).toContain('桶峰值 3 条')
    expect(details.find('.lch-metrics-detail').text()).toContain('主题桶数 2 个')
    // 键盘可展开（summary 原生可聚焦；点击展开后 open 属性在场）
    await details.find('summary').trigger('click')
    expect(wrapper.find('[data-testid="lane-chronicle-metrics-details"]').attributes('open')).toBeDefined()
  })

  it('R26：未知不能冒完整总量——全未知标 ?，部分未知标 ≥已知部分', async () => {
    const allUnknown: LaneDayResolution = {
      kind: 'content', source: 'tier',
      day: { date: '2026-10-22', sections: [section('501', { threadCount: null, fallbackClueCount: null, eventTitles: [] })] },
    }
    const wrapper = mountChronicle({ resolveDay: () => allUnknown })
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-metrics"]').text()).toBe('当日合计 ? 条')
    expect(wrapper.find('[data-testid="lane-chronicle-metrics-details"]').find('.lch-metrics-detail').text()).toContain('桶峰值 ? 条')
    expect(wrapper.find('[data-testid="lane-chronicle-metrics-details"]').find('.lch-metrics-detail').text()).toContain('部分桶线索数未知')

    const partialUnknown: LaneDayResolution = {
      kind: 'content', source: 'tier',
      day: { date: '2026-10-22', sections: [section('501', { threadCount: 3 }), section('502', { threadCount: null, fallbackClueCount: null, eventTitles: [] })] },
    }
    const wrapper2 = mountChronicle({ resolveDay: () => partialUnknown })
    await flushPromises()
    expect(wrapper2.find('[data-testid="lane-chronicle-metrics"]').text()).toBe('当日合计 ≥3 条')
  })

  it('CH-10 只读：只有 GET 型读取、无生命周期/写动作', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    const calledApis = [api.loadReportIndex.mock.calls.length, api.loadReportDetail.mock.calls.length, api.loadArticleMeta.mock.calls.length]
    expect(calledApis[0]).toBe(1) // 只列表
    expect(calledApis[1]).toBe(1) // 只详情
    expect(calledApis[2]).toBe(0) // 相关文章未展开不取
    const html = wrapper.html()
    expect(html).not.toContain('归档话题')
    expect(html).not.toContain('转正')
    expect(html).not.toContain('合并')
    expect(wrapper.find('[data-testid="section-drawer-compose"]').exists()).toBe(false)
  })

  it('页脚口径：已返回 N 桶 · M 条标题', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-footer"]').text()).toBe('已返回 2 桶 · 6 条标题')
  })
})

describe('CH-3/4 — 降级与无报告', () => {
  it('section 缺失：base 标题 + 折叠提示 + 刷新；不借相似标题/别日', async () => {
    const wrapper = mountChronicle({ selectedDate: '2026-10-20' })
    await flushPromises()
    const section503 = wrapper.find('[data-testid="lane-chronicle-section-503"]')!
    expect(section503.text()).toContain('聚合丙')
    expect(section503.text()).toContain('另有 2 条折叠未读取')
    expect(section503.find('.lct').exists()).toBe(false)
    const banner = wrapper.find('[data-testid="lane-chronicle-degraded"]')
    expect(banner.text()).toContain('2 个主题桶的完整线索未读取')
    expect(banner.text()).toContain('2 条折叠未读取')
    expect(wrapper.find('[data-testid="lane-chronicle-refresh"]').exists()).toBe(true)
  })

  it('当日无报告：中性「未匹配」文案（R25，不直称无日报）+ base 降级显示', async () => {
    api.loadReportIndex.mockResolvedValue({ success: true, data: INDEX.filter(item => item.periodDate !== '2026-10-20') })
    const wrapper = mountChronicle({ selectedDate: '2026-10-20' })
    await flushPromises()
    const banner = wrapper.find('[data-testid="lane-chronicle-no-report"]')
    expect(banner.text()).toContain('未匹配到当日日报详情')
    expect(banner.text()).not.toContain('这一天没有日报')
    expect(wrapper.find('[data-testid="lane-chronicle-section-503"]').text()).toContain('聚合丙')
    const open = wrapper.find('[data-testid="lane-chronicle-open-report"]')
    expect((open.element as HTMLButtonElement).disabled).toBe(true)
    expect(open.attributes('title')).toBe('未匹配到当日日报')
  })
})

describe('状态分层 — error/stale/gap/unavailable', () => {
  it('列表失败：错误横幅 + 重试恢复', async () => {
    api.loadReportIndex.mockResolvedValueOnce({ success: false, error: '日报列表挂了' })
    const wrapper = mountChronicle()
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-error"]').text()).toContain('日报列表挂了')
    await wrapper.find('[data-testid="lane-chronicle-retry"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-error"]').exists()).toBe(false)
    expect(wrapper.findAll('.lct').length).toBeGreaterThan(0)
  })

  it('同上下文刷新失败：stale 横幅 + 旧内容可读（部分命中后 force 失败）', async () => {
    // 首读：503 命中（degraded 但有已读全文），刷新时详情失败 → stale 保留
    api.loadReportDetail.mockImplementationOnce(() => Promise.resolve({
      success: true,
      data: {
        reportId: '1210', boardId: '1974', periodDate: '2026-10-20',
        sections: [{ sectionId: '503', clusterLabel: '桶503', bucketTitle: null, threads: [{ threadId: 'd1', sectionId: '503', title: '降级日的命中线索', summary: '命中摘要', confidence: 0.7, relatedArticleIds: [], fitDistance: null, createdAt: '' }] }],
      } as LaneDayReportDetail,
    }))
    const wrapper = mountChronicle({ selectedDate: '2026-10-20' })
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-section-503"]').text()).toContain('降级日的命中线索')
    api.loadReportDetail.mockRejectedValueOnce(new Error('刷新失败'))
    await wrapper.find('[data-testid="lane-chronicle-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-stale"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-chronicle-stale"]').text()).toContain('刷新失败，当前展示上次数据')
    expect(wrapper.find('[data-testid="lane-chronicle-section-503"]').text()).toContain('降级日的命中线索')
  })

  it('gap 与 unavailable 区分提示；不误称无分区为未取到', async () => {
    const gap = mountChronicle({ selectedDate: '2026-10-21' })
    await flushPromises()
    expect(gap.find('[data-testid="lane-chronicle-gap"]').text()).toContain('这一天这条泳道没有分区')
    expect(gap.find('[data-testid="lane-chronicle-unavailable"]').exists()).toBe(false)

    const unavailable = mountChronicle({ selectedDate: '2026-10-19' })
    await flushPromises()
    expect(unavailable.find('[data-testid="lane-chronicle-unavailable"]').text()).toContain('未取到')
    expect(api.loadReportDetail).not.toHaveBeenCalled()
  })

  it('未选日期/未选泳道：不渲染主体、不发请求', async () => {
    const noLane = mountChronicle({ lane: null })
    await flushPromises()
    expect(noLane.find('[data-testid="lane-chronicle"]').exists()).toBe(false)
    expect(api.loadReportIndex).not.toHaveBeenCalled()
    const noDate = mountChronicle({ selectedDate: null })
    await flushPromises()
    expect(noDate.text()).toContain('尚未选择日期')
    expect(api.loadReportIndex).not.toHaveBeenCalled()
  })
})

describe('R24 — 离开可读上下文后旧 current 不进入新上下文', () => {
  function assertNoStaleLeak(wrapper: ReturnType<typeof mountChronicle>): void {
    // 页头动作：旧 reportId 不得重新启用
    const open = wrapper.find('[data-testid="lane-chronicle-open-report"]')
    expect((open.element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.emitted('openReport')).toBeUndefined()
    // 横幅/正文：旧日横幅与正文不进新上下文
    for (const testid of ['lane-chronicle-error', 'lane-chronicle-no-report', 'lane-chronicle-degraded', 'lane-chronicle-stale']) {
      expect(wrapper.find(`[data-testid="${testid}"]`).exists()).toBe(false)
    }
    expect(wrapper.find('.lch-body').exists()).toBe(false)
  }

  it('旧成功 → gap：旧报告禁用、旧横幅/正文隐藏，不发新请求', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-open-report"]').attributes('disabled')).toBeUndefined()
    await wrapper.setProps({ selectedDate: '2026-10-21' }) // gap
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-gap"]').exists()).toBe(true)
    assertNoStaleLeak(wrapper)
    expect(wrapper.find('[data-testid="lane-chronicle-open-report"]').attributes('title')).toBe('这一天这条泳道没有分区')
    expect(api.loadReportIndex).toHaveBeenCalledTimes(1)
  })

  it('旧成功 → unavailable / 未选日期：同样不泄漏', async () => {
    const unavailable = mountChronicle()
    await flushPromises()
    await unavailable.setProps({ selectedDate: '2026-10-19' }) // unavailable
    await flushPromises()
    expect(unavailable.find('[data-testid="lane-chronicle-unavailable"]').exists()).toBe(true)
    assertNoStaleLeak(unavailable)

    const noDate = mountChronicle()
    await flushPromises()
    await noDate.setProps({ selectedDate: null })
    await flushPromises()
    expect(noDate.text()).toContain('尚未选择日期')
    assertNoStaleLeak(noDate)
  })

  it('同日换无分区泳道：laneKey 不匹配即失效，旧日内容不挂新泳道', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    expect(wrapper.findAll('.lct').length).toBeGreaterThan(0)
    await wrapper.setProps({ lane: { ...LANE, topicId: '20', label: '无分区泳道' } })
    await flushPromises()
    // resolver 对非 '10' 泳道一律 gap
    expect(wrapper.find('[data-testid="lane-chronicle-gap"]').exists()).toBe(true)
    assertNoStaleLeak(wrapper)
  })

  it('旧请求晚到：不能重新启用旧报告；重入 content 正常读（缓存命中）', async () => {
    let releaseDetail!: (value: { success: boolean, data?: typeof REPORT_1211 }) => void
    api.loadReportDetail.mockImplementationOnce(() => new Promise(resolve => { releaseDetail = resolve }))
    const wrapper = mountChronicle()
    await flushPromises()
    // 在途期间切到 gap：显示失效
    await wrapper.setProps({ selectedDate: '2026-10-21' })
    await flushPromises()
    assertNoStaleLeak(wrapper)
    // 旧日详情晚到完成（只入缓存，display 仍被 gate 拦住）
    releaseDetail({ success: true, data: REPORT_1211 })
    await flushPromises()
    assertNoStaleLeak(wrapper)
    // 重入 content：缓存命中正常读，动作携带正确 date/reportId 对
    await wrapper.setProps({ selectedDate: '2026-10-22' })
    await flushPromises()
    const open = wrapper.find('[data-testid="lane-chronicle-open-report"]')
    expect((open.element as HTMLButtonElement).disabled).toBe(false)
    await open.trigger('click')
    expect(wrapper.emitted('openReport')![0]).toEqual([{ date: '2026-10-22', reportId: '1211' }])
    expect(api.loadReportDetail).toHaveBeenCalledTimes(1)
  })

  it('同日同泳道 sections 变化：旧分区内容不冒新分区（重读后按新分区渲染）', async () => {
    const wrapper = mountChronicle()
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle-section-502"]').exists()).toBe(true)
    const changedResolver = (topicId: string, date: string): LaneDayResolution => {
      if (topicId !== '10' || date !== '2026-10-22') return resolveDay(topicId, date)
      return {
        kind: 'content', source: 'tier',
        day: { date, sections: [section('501', { threadCount: 3, eventTitles: ['聚合甲'] }), section('506', { threadCount: 1 })] },
      }
    }
    await wrapper.setProps({ resolveDay: changedResolver })
    await flushPromises()
    // 旧 502 桶不冒充新上下文；重读后 501 命中、506 不在详情 → degraded 降级
    expect(wrapper.find('[data-testid="lane-chronicle-section-502"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="lane-chronicle-section-506"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-chronicle-degraded"]').text()).toContain('1 个主题桶的完整线索未读取')
    expect(wrapper.find('[data-testid="lane-chronicle-section-501"]').text()).not.toContain('桶502')
  })
})
