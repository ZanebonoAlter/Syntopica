/**
 * ObservationLaneEvolution 测试（矩阵阶段 G3，test-cases-matrix-stage.md EV 组）。
 *
 *  - EV-1 点径公式 / EV-2 节点数字=日合计 / EV-3 标签阈值与宽度 / EV-4 无最密三日·今日事件
 *  - EV-5 态势句最新快照折叠+asOf 标注、待结算占位 / EV-6 长窗趋势图+折叠逐日索引
 *  - EV-7 空心禁用节点；G2 图已知 0（gap）事件由本组守 content 门槛
 *  - 选日只 emit（不弹层/不强滚）；loading/error/empty/stale 状态分层
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ObservationLaneEvolution from './ObservationLaneEvolution.vue'
import type { ObservationLaneRow } from './observationLanes'
import { buildDayAxis } from '~/features/tags/composables/laneMatrixModel'
import type { WindowLaneDay, WindowSection } from '~/api/normalizers/laneDynamicsWindow'
import type { LaneDayResolution } from '~/features/tags/composables/useObservationWindow'

const notifyWarn = vi.fn()
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ warn: notifyWarn, success: vi.fn(), error: vi.fn() }),
}))
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))

const TODAY = '2026-10-23'
const AXIS14 = buildDayAxis(TODAY, 14)

function laneRow(overrides: Partial<ObservationLaneRow> = {}): ObservationLaneRow {
  return {
    topicId: '10',
    label: '主线 A',
    watchLinked: true,
    sectionTotal: 5,
    clueTotal: 12,
    dormant: false,
    snapshot: { summary: '该线近两周持续升温。', asOf: '2026-10-22', detail: null },
    ...overrides,
  }
}

function windowDay(date: string, sections: Partial<WindowSection>[]): WindowLaneDay {
  return {
    date,
    sections: sections.map((section, index) => ({
      sectionId: section.sectionId ?? String(index + 1),
      label: section.label ?? `桶${index + 1}`,
      threadCount: section.threadCount ?? null,
      fallbackClueCount: section.fallbackClueCount ?? null,
      foldedCount: section.foldedCount ?? 0,
      eventTitles: section.eventTitles ?? [],
    })),
  }
}

/**
 * 三态 fixture resolver：
 *  - 10-23：2 桶 3+1（total 4，峰值 3）；10-22：已知 0；10-21：全未知；10-20：gap；10-19：unavailable
 */
function fixtureResolver(lane: ObservationLaneRow = laneRow()) {
  return (topicId: string, date: string): LaneDayResolution => {
    if (topicId !== lane.topicId) return { kind: 'gap' }
    if (date === '2026-10-23') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 3, eventTitles: ['甲事件标题'] }, { threadCount: 1, eventTitles: [] }]) }
    if (date === '2026-10-22') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 0, eventTitles: ['零线索日标题'] }]) }
    if (date === '2026-10-21') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: null, fallbackClueCount: null }]) }
    if (date === '2026-10-20') return { kind: 'gap' }
    if (date === '2026-10-19') return { kind: 'unavailable' }
    return { kind: 'gap' }
  }
}

function mountEvolution(overrides: Record<string, unknown> = {}) {
  return mount(ObservationLaneEvolution, {
    props: {
      lane: laneRow(),
      axis: AXIS14,
      today: TODAY,
      selectedDate: null,
      resolveDay: fixtureResolver(),
      loading: false,
      error: null,
      staleNotice: false,
      sourceDays: 14,
      ...overrides,
    },
  })
}

describe('ObservationLaneEvolution — 节点形态（EV-1/2/3/7）', () => {
  beforeEach(() => notifyWarn.mockReset())

  it('EV-1/EV-2：点径 = min(24, 6+√日合计×5)、节点数字 = 日合计（非桶峰值）；已知 0 合法', () => {
    const resolver = (topicId: string, date: string): LaneDayResolution => {
      if (date === '2026-10-23') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 16 }]) }
      if (date === '2026-10-22') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 3 }, { threadCount: 1 }]) }
      if (date === '2026-10-21') return { kind: 'content', source: 'tier', day: windowDay(date, [{ threadCount: 0 }]) }
      return { kind: 'gap' }
    }
    const axis = ['2026-10-21', '2026-10-22', '2026-10-23']
    const wrapper = mountEvolution({ axis, resolveDay: resolver })
    const capped = wrapper.find('[data-testid="evolution-node-2026-10-23"]')
    expect(capped.find('.oev-node-count').text()).toBe('16')
    expect(capped.find('.oev-dot').attributes('style')).toContain('width: 24px')
    expect(wrapper.find('[data-testid="evolution-node-2026-10-22"]').find('.oev-node-count').text()).toBe('4')
    expect(wrapper.find('[data-testid="evolution-node-2026-10-22"]').find('.oev-dot').attributes('style')).toContain('width: 16px')
    const zero = wrapper.find('[data-testid="evolution-node-2026-10-21"]')
    expect(zero.find('.oev-node-count').text()).toBe('0')
    expect(zero.find('.oev-dot').attributes('style')).toContain('width: 6px')
    expect(zero.attributes('disabled')).toBeUndefined()
  })

  it('EV-3：标签阈值 ≥3 才显示首线索标题；全未知合计显示 ? 不显示标签', () => {
    const wrapper = mountEvolution({ axis: ['2026-10-19', '2026-10-20', '2026-10-21', '2026-10-22', '2026-10-23'] })
    // total 4（≥3）→ 显示首线索标题；total 0 / 全未知 → 不显示
    const dense = wrapper.find('[data-testid="evolution-node-2026-10-23"]')
    expect(dense.find('.oev-node-label').text()).toBe('甲事件标题')
    expect(wrapper.find('[data-testid="evolution-node-2026-10-22"]').find('.oev-node-label').exists()).toBe(false)
    const unknown = wrapper.find('[data-testid="evolution-node-2026-10-21"]')
    expect(unknown.find('.oev-node-count').text()).toBe('?')
    expect(unknown.find('.oev-node-label').exists()).toBe(false)
  })

  it('EV-7：无分区节点空心禁用无标签；未取到节点 「?」 禁用；点击不 emit、不弹层', async () => {
    const wrapper = mountEvolution({ axis: ['2026-10-19', '2026-10-20', '2026-10-23'] })
    const gapNode = wrapper.find('[data-testid="evolution-node-2026-10-20"]')
    expect(gapNode.classes()).toContain('gap')
    expect(gapNode.attributes('disabled')).toBeDefined()
    expect(gapNode.find('.oev-node-label').exists()).toBe(false)
    await gapNode.trigger('click')
    expect(wrapper.emitted('selectDate')).toBeUndefined()

    const unknownNode = wrapper.find('[data-testid="evolution-node-2026-10-19"]')
    expect(unknownNode.find('.oev-node-count').text()).toBe('?')
    expect(unknownNode.attributes('disabled')).toBeDefined()
    await unknownNode.trigger('click')
    expect(wrapper.emitted('selectDate')).toBeUndefined()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })

  it('EV-4：无「最密集三天」「今日事件」区块，无泳道全文章/追踪管理等入口（DOM 反向断言）', () => {
    const wrapper = mountEvolution()
    expect(wrapper.find('[data-testid="lane-evolution-peaks"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="lane-evolution-today-events"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('最密集')
    expect(wrapper.text()).not.toContain('全部文章')
    expect(wrapper.text()).not.toContain('追踪管理')
  })
})

describe('ObservationLaneEvolution — 态势句与长窗（EV-5/6）', () => {
  beforeEach(() => notifyWarn.mockReset())

  it('EV-5：最新快照默认折叠、asOf 标注非本视窗总结；展开逐字；detail 可展开全文', async () => {
    const wrapper = mountEvolution({ lane: laneRow({ snapshot: { summary: '短版态势。', asOf: '2026-10-22', detail: '这是五百字长版全文。' } }) })
    const stance = wrapper.find('[data-testid="lane-evolution-stance"]')
    expect((stance.element as HTMLDetailsElement).open).toBe(false)
    expect(stance.find('summary').text()).toContain('最近结算态势 · 2026-10-22（非本视窗总结）')
    expect(wrapper.find('[data-testid="lane-evolution-stance-text"]').text()).toBe('短版态势。')

    const toggle = wrapper.find('[data-testid="lane-evolution-detail-toggle"]')
    expect(wrapper.find('[data-testid="lane-evolution-detail"]').exists()).toBe(false)
    await toggle.trigger('click')
    expect(wrapper.find('[data-testid="lane-evolution-detail"]').text()).toBe('这是五百字长版全文。')
    await toggle.trigger('click')
    expect(wrapper.find('[data-testid="lane-evolution-detail"]').exists()).toBe(false)
  })

  it('EV-5：snapshot=null 待结算占位，时间线照常渲染', () => {
    const wrapper = mountEvolution({ lane: laneRow({ snapshot: null }) })
    expect(wrapper.find('[data-testid="lane-evolution-pending"]').text()).toContain('态势待结算')
    expect(wrapper.find('[data-testid="lane-evolution-stance"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid^="evolution-node-"]').length).toBeGreaterThan(0)
  })

  it('EV-6：可见日 >31 切趋势图（gap 画 0 点、unavailable 无点不补 0）+ 折叠逐日索引', async () => {
    const axis = buildDayAxis(TODAY, 365)
    const resolver = fixtureResolver()
    const wrapper = mountEvolution({ axis, resolveDay: resolver })
    expect(wrapper.find('[data-testid="lane-evolution-ribbon"]').exists()).toBe(false)
    const chart = wrapper.find('[data-testid="lane-trend-chart"]')
    expect(chart.exists()).toBe(true)
    expect(chart.text()).toContain('所选泳道 · 所示日期内当日线索数')
    // gap 日（10-20）series 值 0 → 图上有 data-value=0 的点；unavailable 日（10-19）无点
    expect(chart.find('[data-date="2026-10-20"]').attributes('data-value')).toBe('0')
    expect(chart.find('[data-date="2026-10-19"]').exists()).toBe(false)

    const index = wrapper.find('[data-testid="lane-evolution-day-index"]')
    expect((index.element as HTMLDetailsElement).open).toBe(false)
    expect(index.find('summary').text()).toContain('展开逐日索引')
    // 折叠内容在 DOM（浏览器默认不显示），逐日行 content 可点选日
    await wrapper.find('[data-testid="evolution-index-day-2026-10-23"]').trigger('click')
    expect(wrapper.emitted('selectDate')![0]).toEqual(['2026-10-23'])
  })

  it('EV-6 守门：趋势图对 gap 已知 0 点发 select-date，本组过滤为提示不 emit', async () => {
    const axis = buildDayAxis(TODAY, 365)
    const wrapper = mountEvolution({ axis, resolveDay: fixtureResolver() })
    const gapDot = wrapper.find('[data-testid="lane-trend-chart"] [data-date="2026-10-20"]')
    expect(gapDot.exists()).toBe(true)
    await gapDot.trigger('click')
    expect(wrapper.emitted('selectDate')).toBeUndefined()
    expect(notifyWarn).toHaveBeenCalledWith(expect.stringContaining('没有分区'))

    await wrapper.find('[data-testid="lane-trend-chart"] [data-date="2026-10-23"]').trigger('click')
    expect(wrapper.emitted('selectDate')![0]).toEqual(['2026-10-23'])
  })
})

describe('ObservationLaneEvolution — 交互与状态', () => {
  beforeEach(() => notifyWarn.mockReset())

  it('选日只 emit：不弹层、不抢滚动；selectedDate 受控高亮透传图与节点', async () => {
    const wrapper = mountEvolution({ selectedDate: '2026-10-23' })
    expect(wrapper.find('[data-testid="evolution-node-2026-10-23"]').classes()).toContain('is-selected')
    expect(wrapper.find('[data-testid="lane-trend-chart"]').exists()).toBe(false) // 短窗无图
    await wrapper.find('[data-testid="evolution-node-2026-10-23"]').trigger('click')
    expect(wrapper.emitted('selectDate')![0]).toEqual(['2026-10-23'])
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)

    const long = mountEvolution({ axis: buildDayAxis(TODAY, 365), selectedDate: '2026-10-23' })
    expect(long.find('[data-testid="lane-trend-chart"]').find('.ltc-cursor').exists()).toBe(true)
  })

  it('头部：选中泳道标题 + 追踪徽标 + 可见天数/档全窗口径；今天节点标注', () => {
    const wrapper = mountEvolution()
    expect(wrapper.find('.oev-name').text()).toBe('主线 A')
    expect(wrapper.find('.oev-watch').text()).toBe('追踪')
    expect(wrapper.find('.oev-meta').text()).toContain('可见 14 日')
    expect(wrapper.find('.oev-meta').text()).toContain('14 日档全窗 12 条线索')
    expect(wrapper.find('[data-testid="evolution-node-2026-10-23"]').find('.oev-node-date').text()).toBe('今天')
  })

  it('状态分层：loading 骨架 / 首次错误+重试 / 未选泳道空态 / 刷新失败保留旧内容', async () => {
    const loading = mountEvolution({ lane: null, loading: true })
    expect(loading.find('[data-testid="lane-evolution-loading"]').exists()).toBe(true)
    expect(loading.find('[data-testid="evolution-node-2026-10-23"]').exists()).toBe(false)

    const error = mountEvolution({ lane: null, error: '超时' })
    expect(error.find('[data-testid="lane-evolution-error"]').text()).toContain('超时')
    await error.find('[data-testid="lane-evolution-retry"]').trigger('click')
    expect(error.emitted('retry')).toHaveLength(1)

    const empty = mountEvolution({ lane: null })
    expect(empty.find('[data-testid="lane-evolution-empty"]').exists()).toBe(true)

    const stale = mountEvolution({ error: '网络错误', staleNotice: true })
    expect(stale.find('[data-testid="lane-evolution-stale"]').exists()).toBe(true)
    expect(stale.find('[data-testid="evolution-node-2026-10-23"]').exists()).toBe(true)
  })
})
