/**
 * BoardNarrowSummary（矩阵阶段 G5 + 8C G3）：窄屏保留全部可选泳道（活跃 + 休眠折叠）、
 * 迷你条分区数口径、选中泳道下方编年入口、本板块上下文入口、空/加载/错误态；
 * 8C G3 追加同源报日条（只滚条自身）、当日其他话题组、0 休眠仍有 summary、0 正式泳道不再整块空态。
 */
import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import BoardNarrowSummary from './BoardNarrowSummary.vue'
import type { LaneDayResolver, ObservationLaneRow } from './observationLanes'
import type { OtherTopicsView } from './dayOtherTopics'
import type { WindowLaneDay } from '~/api/normalizers/laneDynamicsWindow'

vi.mock('@iconify/vue', () => ({ Icon: { name: 'Icon', template: '<span />' } }))

const AXIS = Array.from({ length: 14 }, (_, i) => `2026-10-${String(10 + i).padStart(2, '0')}`)

function day(date: string, sections: number): WindowLaneDay {
  return {
    date,
    sections: Array.from({ length: sections }, (_, index) => ({
      sectionId: `${date}-${index}`,
      label: `桶 ${index}`,
      threadCount: 1,
      compositeCount: 0,
      clueCount: 1,
      foldedCount: 0,
      eventTitles: [],
    })),
  }
}

function row(topicId: string, label: string, options: Partial<ObservationLaneRow> = {}): ObservationLaneRow {
  return {
    topicId,
    label,
    watchLinked: false,
    sectionTotal: 3,
    clueTotal: 20,
    dormant: false,
    snapshot: null,
    ...options,
  }
}

const ACTIVE = row('1', '美伊博弈', { watchLinked: true })
const DORMANT = row('2', '休眠线', { sectionTotal: 1, clueTotal: 2, dormant: true })

const DAYS: Record<string, Record<string, WindowLaneDay>> = {
  '1': {
    '2026-10-20': day('2026-10-20', 2),
    '2026-10-23': day('2026-10-23', 1),
  },
  '2': {
    '2026-10-21': day('2026-10-21', 1),
  },
}

const resolveDay: LaneDayResolver = (topicId, date) => {
  const found = DAYS[topicId]?.[date]
  return found ? { kind: 'content', source: 'tier', day: found } : { kind: 'gap' }
}

function mountSummary(overrides: Record<string, unknown> = {}) {
  return mount(BoardNarrowSummary, {
    props: {
      boardLabel: '中东',
      rows: [ACTIVE, DORMANT],
      dormantRows: [DORMANT],
      axis: AXIS,
      today: '2026-10-23',
      sourceDays: 14,
      selectedLaneId: null,
      resolveDay,
      expandedDormant: false,
      loading: false,
      error: null,
      emptyState: null,
      ...overrides,
    },
  })
}

function otherView(overrides: Partial<OtherTopicsView> = {}): OtherTopicsView {
  return {
    state: 'ready',
    rows: [
      {
        sectionId: '901',
        label: '海峡局势与油运航线安全',
        clusterIndex: 0,
        threadCount: 3,
        compositeCount: 0,
        persistentTopicId: '77',
        statusKind: 'active',
        statusText: '当时已转正',
      },
      {
        sectionId: '902',
        label: '也门停火谈判',
        clusterIndex: 1,
        threadCount: 0,
        compositeCount: 0,
        persistentTopicId: null,
        statusKind: 'unattributed',
        statusText: '当次未归属',
      },
    ],
    clueCount: 3,
    statusText: null,
    note: null,
    retryable: false,
    ...overrides,
  }
}

function htmlIndex(wrapper: ReturnType<typeof mountSummary>, testid: string): number {
  return wrapper.html().indexOf(`data-testid="${testid}"`)
}

describe('BoardNarrowSummary', () => {
  it('渲染活跃泳道与 14 日迷你条；休眠泳道折叠可展开', async () => {
    const wrapper = mountSummary()
    expect(wrapper.text()).toContain('美伊博弈')
    expect(wrapper.text()).toContain('20 线索 · 3 分区')
    expect(wrapper.text()).toContain('追踪')
    expect(wrapper.findAll('.bns-cell')).toHaveLength(14)
    expect(wrapper.find('[data-testid="narrow-lane-2"]').exists()).toBe(false)

    await wrapper.find('[data-testid="narrow-dormant-toggle"]').trigger('click')
    expect(wrapper.emitted('toggleDormant')).toHaveLength(1)
    const expanded = mountSummary({ expandedDormant: true })
    expect(expanded.find('[data-testid="narrow-lane-2"]').text()).toContain('休眠线')
  })

  it('点泳道只切选中（不冒阅读台），选中后出现下方编年入口', async () => {
    const wrapper = mountSummary({ selectedLaneId: '1' })
    await wrapper.find('[data-testid="narrow-lane-1"]').trigger('click')
    expect(wrapper.emitted('selectLane')?.[0]).toEqual(['1'])
    expect(wrapper.find('[data-testid="narrow-goto-chronicle"]').exists()).toBe(true)
    await wrapper.find('[data-testid="narrow-goto-chronicle"]').trigger('click')
    expect(wrapper.emitted('gotoChronicle')).toHaveLength(1)
  })

  it('本板块上下文入口发 openContext（窄屏不是隐藏内容）', async () => {
    const wrapper = mountSummary()
    await wrapper.find('[data-testid="narrow-open-context"]').trigger('click')
    expect(wrapper.emitted('openContext')).toHaveLength(1)
  })

  it('无日报：生成入口；无泳道文案；加载/错误重试', async () => {
    const noReports = mountSummary({ rows: [], dormantRows: [], emptyState: 'no-reports' })
    expect(noReports.find('[data-testid="narrow-generate"]').exists()).toBe(true)

    const noLanes = mountSummary({ rows: [], dormantRows: [], emptyState: 'no-lanes' })
    expect(noLanes.text()).toContain('0 正式泳道')

    const loading = mountSummary({ rows: [], dormantRows: [], loading: true })
    expect(loading.find('.bns-skeleton').exists()).toBe(true)

    const error = mountSummary({ rows: [], dormantRows: [], error: '超时' })
    expect(error.text()).toContain('超时')
    await error.find('[data-testid="narrow-retry"]').trigger('click')
    expect(error.emitted('retry')).toHaveLength(1)
  })
})

describe('8C G3 — 同源报日条（连续轴，只滚条自身）', () => {
  it('同一连续轴只报日可点；非报日非交互；点报日发 selectDayHeader', async () => {
    const reports = new Set(['2026-10-11', '2026-10-15', '2026-10-20'])
    const wrapper = mountSummary({ reportDates: reports, selectedDate: '2026-10-15' })
    expect(wrapper.find('[data-testid="narrow-day-strip"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-strip-date]')).toHaveLength(14)
    expect(wrapper.findAll('[data-testid^="narrow-dayhead-"]')).toHaveLength(3)
    for (const date of reports) {
      expect(wrapper.find(`[data-testid="narrow-dayhead-${date}"]`).element.tagName).toBe('BUTTON')
    }
    // 非报日仍是连续轴上的非交互文本（不压轴、不可聚焦）
    const off = wrapper.find('[data-testid="narrow-day-2026-10-12"]')
    expect(off.element.tagName).toBe('SPAN')
    expect(off.attributes('aria-hidden')).toBe('true')
    expect(wrapper.find('[data-testid="narrow-dayhead-2026-10-15"]').attributes('aria-pressed')).toBe('true')

    await wrapper.find('[data-testid="narrow-dayhead-2026-10-20"]').trigger('click')
    expect(wrapper.emitted('selectDayHeader')?.[0]).toEqual(['2026-10-20'])
  })

  it('选中日只滚日期条自身 scrollLeft（nextTick 测量后 clamp），不 scrollIntoView 拉整页', async () => {
    const scrollIntoView = vi.fn()
    const scrollTo = vi.fn()
    const originalIntoView = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView')
    const originalScrollTo = Object.getOwnPropertyDescriptor(window, 'scrollTo')
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { value: scrollIntoView, configurable: true })
    Object.defineProperty(window, 'scrollTo', { value: scrollTo, configurable: true })
    try {
      const wrapper = mountSummary({ reportDates: new Set(AXIS), selectedDate: '2026-10-11' })
      const track = wrapper.find('[data-testid="narrow-day-strip-track"]').element as HTMLElement
      Object.defineProperty(track, 'scrollWidth', { value: 1200, configurable: true })
      Object.defineProperty(track, 'clientWidth', { value: 300, configurable: true })
      Object.defineProperty(track, 'scrollLeft', { value: 0, writable: true, configurable: true })
      const target = wrapper.find('[data-testid="narrow-dayhead-2026-10-20"]').element as HTMLElement
      Object.defineProperty(target, 'offsetLeft', { value: 900, configurable: true })
      Object.defineProperty(target, 'offsetWidth', { value: 60, configurable: true })

      await wrapper.setProps({ selectedDate: '2026-10-20' })
      await nextTick()
      await nextTick()
      // 居中 900 - (300-60)/2 = 780；clamp 上限 1200-300=900
      expect(track.scrollLeft).toBe(780)

      // 远端目标：居中 1100 - 120 = 980 → clamp 到 900
      const far = wrapper.find('[data-testid="narrow-dayhead-2026-10-23"]').element as HTMLElement
      Object.defineProperty(far, 'offsetLeft', { value: 1100, configurable: true })
      Object.defineProperty(far, 'offsetWidth', { value: 60, configurable: true })
      await wrapper.setProps({ selectedDate: '2026-10-23' })
      await nextTick()
      await nextTick()
      expect(track.scrollLeft).toBe(900)

      expect(scrollIntoView).not.toHaveBeenCalled()
      expect(scrollTo).not.toHaveBeenCalled()
    } finally {
      if (originalIntoView) Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', originalIntoView)
      else delete (HTMLElement.prototype as { scrollIntoView?: unknown }).scrollIntoView
      if (originalScrollTo) Object.defineProperty(window, 'scrollTo', originalScrollTo)
      else delete (window as { scrollTo?: unknown }).scrollTo
    }
  })

  it('报日索引加载/失败：条照画但只给说明，不冒「无日报」', () => {
    const loading = mountSummary({ reportDates: null, indexLoading: true })
    expect(loading.find('[data-testid="narrow-day-strip"]').exists()).toBe(true)
    expect(loading.findAll('[data-testid^="narrow-dayhead-"]')).toHaveLength(0)
    expect(loading.text()).toContain('读取报日')

    const failed = mountSummary({ reportDates: null, indexError: '列表 500' })
    expect(failed.text()).toContain('报日索引未取到')
    expect(failed.findAll('[data-testid^="narrow-dayhead-"]')).toHaveLength(0)
  })
})

describe('8C G3 — 其他话题组与 0 休眠 summary（独立折叠）', () => {
  it('顺序：正式行 → 其他组 → 休眠组；0 休眠仍有 summary（本窗无、禁用、不造行）', async () => {
    const wrapper = mountSummary({ reportDates: new Set(AXIS), otherTopics: otherView(), rows: [ACTIVE], dormantRows: [] })
    expect(htmlIndex(wrapper, 'narrow-lane-1')).toBeGreaterThan(-1)
    expect(htmlIndex(wrapper, 'narrow-lane-1')).toBeLessThan(htmlIndex(wrapper, 'narrow-other-group'))
    expect(htmlIndex(wrapper, 'narrow-other-group')).toBeLessThan(htmlIndex(wrapper, 'narrow-dormant-group'))

    const dormant = wrapper.find('[data-testid="narrow-dormant-toggle"]')
    expect(dormant.exists()).toBe(true)
    expect(dormant.text()).toContain('本窗无')
    expect(dormant.attributes('disabled')).toBeDefined()
    await dormant.trigger('click')
    expect(wrapper.emitted('toggleDormant')).toBeUndefined()
  })

  it('两 toggle 互不串联；其他行点行发 selectOther（0 条也可点）', async () => {
    const wrapper = mountSummary({
      reportDates: new Set(AXIS),
      otherTopics: otherView(),
      othersOpen: true,
      selectedDate: '2026-10-15',
      selectedOtherSectionId: '902',
      expandedDormant: false,
    })
    expect(wrapper.find('[data-testid="narrow-other-row-901"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="narrow-other-row-902"]').exists()).toBe(true)
    const row = wrapper.find('[data-testid="narrow-other-row-902"]')
    expect(row.attributes('aria-pressed')).toBe('true')
    const full = row.attributes('title') ?? ''
    expect(full).toContain('也门停火谈判')
    expect(full).toContain('当次未归属')
    expect(full).toContain('点下方读全文')
    expect(row.find('.bns-other-count').text()).toBe('0')
    await row.trigger('click')
    expect(wrapper.emitted('selectOther')?.[0]).toEqual([{ sectionId: '902', date: '2026-10-15', topicId: null }])

    await wrapper.find('[data-testid="narrow-other-toggle"]').trigger('click')
    expect(wrapper.emitted('toggleOthers')).toHaveLength(1)
    expect(wrapper.emitted('toggleDormant')).toBeUndefined()
    await wrapper.find('[data-testid="narrow-dormant-toggle"]').trigger('click')
    expect(wrapper.emitted('toggleDormant')).toHaveLength(1)
    expect(wrapper.emitted('toggleOthers')).toHaveLength(1)
  })

  it('组非 ready：summary 状态在场、无计数、toggle 禁用；收起的组行不进 DOM', () => {
    const failed = otherView({
      state: 'index-error',
      rows: [],
      clueCount: null,
      statusText: '报日索引未取到，暂不能读取当日分区',
      retryable: true,
      note: '列表 500',
    })
    const wrapper = mountSummary({ reportDates: null, indexError: '列表 500', otherTopics: failed, othersOpen: true })
    expect(wrapper.text()).toContain('报日索引未取到，暂不能读取当日分区')
    expect(wrapper.find('[data-testid="narrow-other-count"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="narrow-other-toggle"]').attributes('disabled')).toBeDefined()

    const collapsed = mountSummary({ reportDates: new Set(AXIS), otherTopics: otherView(), othersOpen: false })
    expect(collapsed.find('[data-testid="narrow-other-toggle"]').text()).toContain('2 个分区未入泳道')
    expect(collapsed.find('[data-testid="narrow-other-row-901"]').exists()).toBe(false)
  })

  it('0 正式泳道：不再只剩整块空态；报日条 + 其他组 + 旧空态文案并存', () => {
    const wrapper = mountSummary({
      rows: [],
      dormantRows: [],
      emptyState: 'no-lanes',
      reportDates: new Set(AXIS),
      otherTopics: otherView(),
      othersOpen: true,
    })
    expect(wrapper.find('[data-testid="narrow-day-strip"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="narrow-other-group"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('0 正式泳道')
    expect(wrapper.find('[data-testid="narrow-other-row-901"]').exists()).toBe(true)
  })

  it('旧 props/events 未接线：不渲染报日条/其他组，泳道卡与休眠入口照旧', () => {
    const wrapper = mountSummary()
    expect(wrapper.find('[data-testid="narrow-day-strip"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="narrow-other-group"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="narrow-lane-1"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="narrow-dormant-toggle"]').exists()).toBe(true)
  })
})
