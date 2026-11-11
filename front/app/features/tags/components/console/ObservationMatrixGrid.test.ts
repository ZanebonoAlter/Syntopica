/**
 * ObservationMatrixGrid 测试（rework-workshop-into-radar-and-desk 8C G2）。
 *
 * 网格层契约（矩阵集成测试覆盖不到的网格自有行为）：
 *  - 日期列头三态：`reportDates` 省略 / null / Set；只有报日是原生 button，点它 emit
 *    `selectDayHeader(date)`；非报日不可点且 title 说明（索引未就绪不冒「无日报」）
 *  - 旧事件保留：`selectLane` / `selectDate` / `toggleDormant`（旧壳零改动）
 *  - 组顺序与独立性：正式行 → 其他组（summary + 行）→ 休眠 summary + 休眠行；
 *    0 休眠仍保留同级 summary（本窗无、禁用展开、不造行）
 *  - 新事件透传：`toggleOthers` / `selectOther`
 */
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ObservationMatrixGrid from './ObservationMatrixGrid.vue'
import type { OtherTopicsView } from './dayOtherTopics'
import type { LaneDayResolution, ObservationLaneRow } from './observationLanes'
import { buildDayAxis } from '~/features/tags/composables/laneMatrixModel'

vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ warn: vi.fn(), success: vi.fn(), error: vi.fn() }),
}))
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))

const TODAY = '2026-10-23'
const AXIS = ['2026-10-21', '2026-10-22', '2026-10-23']

function lane(topicId: string, label: string, dormant = false): ObservationLaneRow {
  return { topicId, label, watchLinked: false, sectionTotal: 1, clueTotal: 1, dormant, snapshot: null }
}

const ACTIVE = [lane('10', '主线 A'), lane('11', '主线 B')]
const DORMANT = [lane('12', '休眠线 C', true)]

/** 所有日都是 gap（本测试只关心列头/组结构，不关心格值）。 */
function gapResolver(): LaneDayResolution {
  return { kind: 'gap' }
}

function otherView(): OtherTopicsView {
  return {
    state: 'ready',
    rows: [
      {
        sectionId: '701',
        label: '其他分区一',
        clusterIndex: 0,
        threadCount: 2,
        persistentTopicId: null,
        statusKind: 'unattributed',
        statusText: '当次未归属',
      },
    ],
    count: 1,
    statusText: null,
    note: null,
    retryable: false,
  }
}

function mountGrid(overrides: Record<string, unknown> = {}) {
  return mount(ObservationMatrixGrid, {
    props: {
      axis: AXIS,
      activeRows: ACTIVE,
      dormantRows: DORMANT,
      selectedLaneId: '10',
      selectedDate: '2026-10-23',
      today: TODAY,
      resolveDay: gapResolver,
      expandedDormant: false,
      ...overrides,
    },
  })
}

describe('ObservationMatrixGrid — 报日列头三态', () => {
  it('Set：只有报日是 button；两行 MM/DD 标签保留在按钮内；点它 emit selectDayHeader', async () => {
    const wrapper = mountGrid({ reportDates: new Set(['2026-10-22']) })
    const reportHead = wrapper.find('[data-testid="matrix-dayhead-2026-10-22"]')
    expect(reportHead.element.tagName).toBe('BUTTON')
    expect(reportHead.attributes('title')).toBe('2026-10-22 · 选这一天')
    expect(reportHead.find('.omx-day-month').text()).toBe('10')
    expect(reportHead.find('.omx-day-day').text()).toBe('22')
    await reportHead.trigger('click')
    expect(wrapper.emitted('selectDayHeader')![0]).toEqual(['2026-10-22'])

    const plainHead = wrapper.find('[data-testid="matrix-dayhead-2026-10-21"]')
    expect(plainHead.element.tagName).toBe('DIV')
    expect(plainHead.attributes('title')).toBe('2026-10-21 · 该日没有日报')
    await plainHead.trigger('click')
    expect(wrapper.emitted('selectDayHeader')).toHaveLength(1)
  })

  it('省略（未接线）：无提示、全不可点、title 为裸日期（旧调用零改动）', () => {
    const wrapper = mountGrid()
    expect(wrapper.find('[data-testid="matrix-report-index-hint"]').exists()).toBe(false)
    expect(wrapper.findAll('.omx-dayhead.is-report')).toHaveLength(0)
    expect(wrapper.find('[data-testid="matrix-dayhead-2026-10-22"]').attributes('title')).toBe('2026-10-22')
  })

  it('null：已接线但未就绪/失败 → 不可点 + 提示，不冒「该日没有日报」', () => {
    const loading = mountGrid({ reportDates: null, indexLoading: true })
    expect(loading.find('[data-testid="matrix-report-index-hint"]').text()).toBe('读取报日…')
    expect(loading.find('[data-testid="matrix-dayhead-2026-10-22"]').attributes('title')).toContain('报日索引未就绪')

    const failed = mountGrid({ reportDates: null, indexError: '列表挂了' })
    expect(failed.find('[data-testid="matrix-report-index-hint"]').text()).toContain('日期列头暂不可选')
    expect(failed.find('[data-testid="matrix-dayhead-2026-10-22"]').attributes('title')).not.toContain('该日没有日报')
  })

  it('陈旧索引 + 旧 Set：已知报日仍可点，未报日 title 不硬断言无日报', async () => {
    const wrapper = mountGrid({ reportDates: new Set(['2026-10-22']), indexError: '刷新失败' })
    expect(wrapper.find('[data-testid="matrix-report-index-hint"]').text()).toContain('刷新失败')
    const known = wrapper.find('[data-testid="matrix-dayhead-2026-10-22"]')
    expect(known.element.tagName).toBe('BUTTON')
    await known.trigger('click')
    expect(wrapper.emitted('selectDayHeader')![0]).toEqual(['2026-10-22'])
    expect(wrapper.find('[data-testid="matrix-dayhead-2026-10-21"]').attributes('title')).toContain('可能已陈旧')
  })

  it('>31 列横滚分支保留：报日列头仍是 button，单行短标签', () => {
    const axis = buildDayAxis(TODAY, 32)
    const wrapper = mountGrid({ axis, reportDates: new Set([axis[0]!]) })
    expect(wrapper.find('[data-testid="matrix-grid"]').classes()).toContain('is-scrollable')
    const head = wrapper.find(`[data-testid="matrix-dayhead-${axis[0]}"]`)
    expect(head.element.tagName).toBe('BUTTON')
    expect(head.find('.omx-day-month').exists()).toBe(false)
    expect(wrapper.findAll('.omx-dayhead')).toHaveLength(32)
  })
})

describe('ObservationMatrixGrid — 格值与色深（线索口径，retire-story-adopt-event-metrics D4/D5 + 3.5 修订）', () => {
  const CLUE_DAY: LaneDayResolution = {
    kind: 'content',
    source: 'tier',
    day: {
      date: '2026-10-23',
      sections: [
        { sectionId: '1', label: '桶1', threadCount: 3, fallbackClueCount: null, foldedCount: 0, eventTitles: ['事件一'] },
        { sectionId: '2', label: '桶2', threadCount: 1, fallbackClueCount: null, foldedCount: 0, eventTitles: [] },
      ],
      clueCount: 7,
    },
  }
  const FALLBACK_DAY: LaneDayResolution = {
    kind: 'content',
    source: 'tier',
    day: {
      date: '2026-10-22',
      sections: [{ sectionId: '3', label: '桶3', threadCount: null, fallbackClueCount: 3, foldedCount: 0, eventTitles: ['a', 'b', 'c'] }],
    },
  }
  const resolver = (topicId: string, date: string): LaneDayResolution => {
    if (date === '2026-10-23') return CLUE_DAY
    if (date === '2026-10-22') return FALLBACK_DAY
    return { kind: 'unavailable' }
  }

  it('格值 = 当日线索合计（clue_count 优先，多桶合并）；色深行内归一；title/aria 给「N 条线索 · M 个主题桶」', () => {
    const wrapper = mountGrid({ resolveDay: resolver })
    const cell = wrapper.find('[data-testid="matrix-cell-10-2026-10-23"]')
    expect(cell.text()).toBe('7')
    // 本行可见窗最大值 = 7 → 峰值格 66%；无事件线来源数分档类
    expect(cell.attributes('style')).toContain('--omx-clue-mix: 66%')
    expect(cell.classes()).toContain('is-clue-depth')
    expect(cell.classes()).not.toContain('is-p1')
    expect(cell.classes()).not.toContain('is-p2')
    expect(cell.classes()).not.toContain('is-p3')
    expect(cell.attributes('title')).toContain('7 条线索')
    expect(cell.attributes('title')).toContain('2 个主题桶')
    expect(cell.attributes('title')).not.toContain('事件线')
    expect(cell.attributes('title')).not.toContain('个来源')
    expect(cell.attributes('title')).not.toContain('最热')
    expect(cell.attributes('aria-label')).toBe(cell.attributes('title'))
    expect(cell.attributes('disabled')).toBeUndefined()
  })

  it('旧响应缺 clue_count：回退各桶已知线索合计（3，非桶数 1），色深按 3/7 = 28%，无点名段', () => {
    const wrapper = mountGrid({ resolveDay: resolver })
    const cell = wrapper.find('[data-testid="matrix-cell-10-2026-10-22"]')
    expect(cell.text()).toBe('3')
    expect(cell.attributes('style')).toContain('--omx-clue-mix: 28%')
    expect(cell.classes()).not.toContain('is-p1')
    expect(cell.classes()).not.toContain('is-p2')
    expect(cell.classes()).not.toContain('is-p3')
    expect(cell.attributes('title')).toContain('3 条线索')
    expect(cell.attributes('title')).not.toContain('最热')
    expect(cell.attributes('title')).not.toContain('undefined')
  })

  it('三态保持：断档「—」禁用不可开；历史未取到「?」禁用；0 线索可开', async () => {
    const gapResolver = (_topicId: string, date: string): LaneDayResolution =>
      date === '2026-10-21' ? { kind: 'gap' } : resolver(_topicId, date)
    const zeroDay: LaneDayResolution = {
      kind: 'content',
      source: 'tier',
      day: {
        date: '2026-10-22',
        sections: [{ sectionId: '9', label: '桶9', threadCount: 0, fallbackClueCount: 0, foldedCount: 0, eventTitles: [] }],
        clueCount: 0,
      },
    }
    const mixed = (topicId: string, date: string): LaneDayResolution => {
      if (date === '2026-10-22') return zeroDay
      return gapResolver(topicId, date)
    }
    const wrapper = mountGrid({ resolveDay: mixed })

    const gap = wrapper.find('[data-testid="matrix-cell-10-2026-10-21"]')
    expect(gap.text()).toBe('—')
    expect(gap.attributes('disabled')).toBeDefined()
    expect(gap.attributes('aria-label')).toContain('无分区')
    await gap.trigger('click')
    expect(wrapper.emitted('selectDate')).toBeUndefined()

    const unavailableGrid = mountGrid({ resolveDay: () => ({ kind: 'unavailable' }) })
    const unavailable = unavailableGrid.find('[data-testid="matrix-cell-10-2026-10-23"]')
    expect(unavailable.text()).toBe('?')
    expect(unavailable.attributes('disabled')).toBeDefined()
    expect(unavailable.attributes('aria-label')).toContain('未取到')

    const zero = wrapper.find('[data-testid="matrix-cell-10-2026-10-22"]')
    expect(zero.text()).toBe('0')
    expect(zero.attributes('disabled')).toBeUndefined()
    await zero.trigger('click')
    expect(wrapper.emitted('selectDate')![0]).toEqual([{ topicId: '10', date: '2026-10-22' }])
  })
})

describe('ObservationMatrixGrid — 组顺序与旧事件保留', () => {
  it('DOM 顺序：正式行 → 其他组 summary/行 → 休眠 summary → 休眠行', () => {
    const wrapper = mountGrid({ otherTopics: otherView(), othersOpen: true, expandedDormant: true })
    const grid = wrapper.find('[data-testid="matrix-grid"]').element
    const order = [...grid.querySelectorAll(
      '[data-testid^="matrix-lane-"], [data-testid="other-topics-toggle"], [data-testid^="other-topic-row-"], [data-testid="dormant-toggle"]',
    )].map(node => node.getAttribute('data-testid'))
    expect(order).toEqual([
      'matrix-lane-10',
      'matrix-lane-11',
      'other-topics-toggle',
      'other-topic-row-701',
      'dormant-toggle',
      'matrix-lane-12',
    ])
    expect(wrapper.find('[data-testid="other-topics-group"]').element.closest('[data-testid="matrix-grid"]')).not.toBeNull()
  })

  it('0 休眠仍保留同级 summary（0/本窗无、禁用展开、不造行）', () => {
    const wrapper = mountGrid({ dormantRows: [], expandedDormant: true })
    const summary = wrapper.find('[data-testid="dormant-toggle"]')
    expect(summary.exists()).toBe(true)
    expect(summary.classes()).toContain('omx-group-summary')
    expect(summary.text()).toContain('0')
    expect(summary.text()).toContain('本窗无')
    expect(summary.find('button').attributes('disabled')).toBeDefined()
    expect(wrapper.findAll('[data-testid^="matrix-lane-"]')).toHaveLength(2)
  })

  it('旧事件保留：selectLane / selectDate / toggleDormant 照旧 emit', async () => {
    const wrapper = mountGrid()
    await wrapper.find('[data-testid="matrix-lane-11"]').trigger('click')
    expect(wrapper.emitted('selectLane')![0]).toEqual(['11'])
    await wrapper.find('[data-testid="matrix-cell-10-2026-10-22"]').trigger('click')
    expect(wrapper.emitted('selectDate')).toBeUndefined() // gap 格不可开：不 emit
    await wrapper.find('[data-testid="dormant-toggle"]').find('button').trigger('click')
    expect(wrapper.emitted('toggleDormant')).toHaveLength(1)
  })

  it('新事件透传：toggleOthers / selectOther 由组冒泡到网格', async () => {
    const wrapper = mountGrid({ otherTopics: otherView(), othersOpen: true })
    await wrapper.find('[data-testid="other-topics-toggle"]').find('button').trigger('click')
    expect(wrapper.emitted('toggleOthers')).toHaveLength(1)
    await wrapper.find('[data-testid="other-topic-row-701"]').trigger('click')
    expect(wrapper.emitted('selectOther')![0]).toEqual([{ sectionId: '701', date: '2026-10-23', topicId: null }])
  })
})
