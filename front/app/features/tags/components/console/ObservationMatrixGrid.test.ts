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
        compositeCount: 0,
        persistentTopicId: null,
        statusKind: 'unattributed',
        statusText: '当次未归属',
      },
    ],
    clueCount: 2,
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
        { sectionId: '1', label: '桶1', threadCount: 3, compositeCount: 0, clueCount: 3, foldedCount: 0, eventTitles: ['事件一'] },
        { sectionId: '2', label: '桶2', threadCount: 1, compositeCount: 0, clueCount: 1, foldedCount: 0, eventTitles: [] },
      ],
      clueCount: 7,
    },
  }
  const FALLBACK_DAY: LaneDayResolution = {
    kind: 'content',
    source: 'tier',
    day: {
      date: '2026-10-22',
      sections: [{ sectionId: '3', label: '桶3', threadCount: 3, compositeCount: 0, clueCount: 3, foldedCount: 0, eventTitles: ['a', 'b', 'c'] }],
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
        sections: [{ sectionId: '9', label: '桶9', threadCount: 0, compositeCount: 0, clueCount: 0, foldedCount: 0, eventTitles: [] }],
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

/**
 * 「其他话题（未入泳道）」整行（`lane-matrix-coverage-and-others-row` 4.3）：
 *  逐日格 = 当日未入泳道分区的非综合稿线索数；无日报日「—」不可点；旧后端缺字段整行隐藏。
 */
describe('ObservationMatrixGrid — 其他话题整行', () => {
  const otherDays = {
    '2026-10-21': { date: '2026-10-21', sections: 3, clues: 7 },
    '2026-10-22': { date: '2026-10-22', sections: 0, clues: 0 },
  }

  it('DOM 顺序：活跃行 → 其他话题整行 → 当日其他话题组', () => {
    const wrapper = mountGrid({
      otherTopicDays: otherDays,
      otherTopicDaysAvailable: true,
      otherTopics: otherView(),
      othersOpen: true,
    })
    const grid = wrapper.find('[data-testid="matrix-grid"]').element
    const order = [...grid.querySelectorAll(
      '[data-testid^="matrix-lane-"], [data-testid="matrix-other-row"], [data-testid="other-topics-toggle"]',
    )].map(node => node.getAttribute('data-testid'))
    expect(order).toEqual(['matrix-lane-10', 'matrix-lane-11', 'matrix-other-row', 'other-topics-toggle'])
  })

  it('有日报日显示线索数（0 合法、可点、title 带分区数）；无日报日「—」禁用', async () => {
    const wrapper = mountGrid({ otherTopicDays: otherDays, otherTopicDaysAvailable: true })

    const withClues = wrapper.find('[data-testid="matrix-other-cell-2026-10-21"]')
    expect(withClues.text()).toBe('7')
    expect(withClues.attributes('title')).toBe('2026-10-21 · 3 个分区 · 7 条线索')
    expect(withClues.attributes('aria-label')).toBe('2026-10-21 · 3 个分区 · 7 条线索')
    expect(withClues.attributes('disabled')).toBeUndefined()
    await withClues.trigger('click')
    expect(wrapper.emitted('selectOthersDay')![0]).toEqual(['2026-10-21'])

    const zero = wrapper.find('[data-testid="matrix-other-cell-2026-10-22"]')
    expect(zero.text()).toBe('0')
    expect(zero.attributes('disabled')).toBeUndefined()

    const noReport = wrapper.find('[data-testid="matrix-other-cell-2026-10-23"]')
    expect(noReport.text()).toBe('—')
    expect(noReport.attributes('disabled')).toBeDefined()
    expect(noReport.attributes('aria-label')).toContain('无日报')
  })

  it('旧后端无 other_topics（未接线/字段缺省）→ 整行不渲染', () => {
    expect(mountGrid().find('[data-testid="matrix-other-row"]').exists()).toBe(false)
    expect(mountGrid({ otherTopicDays: otherDays }).find('[data-testid="matrix-other-row"]').exists()).toBe(false)
    expect(mountGrid({ otherTopicDays: otherDays, otherTopicDaysAvailable: false })
      .find('[data-testid="matrix-other-row"]').exists()).toBe(false)
  })

  it('available=true 但窗口内无任何读数（后端恒发空数组）→ 整行不渲染（不铺一整行「—」）', () => {
    expect(mountGrid({ otherTopicDays: {}, otherTopicDaysAvailable: true })
      .find('[data-testid="matrix-other-row"]').exists()).toBe(false)
    expect(mountGrid({ otherTopicDays: otherDays, otherTopicDaysAvailable: true })
      .find('[data-testid="matrix-other-row"]').exists()).toBe(true)
  })

  it('档窗外日期 title 写「档窗外，其他话题读数未取到」（不误说「无日报」）；档窗内无日报仍是「无日报」', () => {
    const wrapper = mountGrid({
      otherTopicDays: { '2026-10-22': { date: '2026-10-22', sections: 1, clues: 2 } },
      otherTopicDaysAvailable: true,
      otherTopicsSince: '2026-10-22',
    })

    const outside = wrapper.find('[data-testid="matrix-other-cell-2026-10-21"]')
    expect(outside.text()).toBe('—')
    expect(outside.attributes('disabled')).toBeDefined()
    expect(outside.attributes('title')).toContain('档窗外')
    expect(outside.attributes('title')).not.toContain('无日报')

    const inside = wrapper.find('[data-testid="matrix-other-cell-2026-10-23"]')
    expect(inside.attributes('title')).toContain('无日报')
    expect(inside.attributes('title')).not.toContain('档窗外')
  })

  it('行头无泳道生命周期菜单（不是泳道）', () => {
    const wrapper = mountGrid({ otherTopicDays: otherDays, otherTopicDaysAvailable: true })
    expect(wrapper.find('[data-testid="matrix-other-row"]').text()).toContain('其他话题')
    expect(wrapper.find('[data-testid="matrix-other-row"]').text()).toContain('未入泳道')
    expect(wrapper.find('[data-testid="matrix-other-row"]').find('button').exists()).toBe(false)
  })
})

/**
 * 覆盖组「窗口内未进日报」（`lane-matrix-coverage-and-others-row` 4.1）：
 *  活跃但当前窗口无分区的泳道进底部独立组，默认展开，组标题列出名字与最近一次；
 *  清单失败只出一行提示 + 重试；成功且为空则整组不渲染。
 */
describe('ObservationMatrixGrid — 覆盖组（窗口内未进日报）', () => {
  const coverage = [{ topicId: '20', label: '以色列内部选举', lastSeenDate: '2026-11-09' }]

  it('默认展开：组 summary 带数量 + 名字 + 最近一次，组内行格全为「—」', () => {
    const wrapper = mountGrid({ coverageLanes: coverage })
    const summary = wrapper.find('[data-testid="coverage-group-summary"]')
    expect(summary.exists()).toBe(true)
    expect(summary.text()).toContain('窗口内未进日报')
    expect(summary.text()).toContain('1')
    expect(summary.text()).toContain('以色列内部选举')
    expect(summary.text()).toContain('最近一次 11-09')
    expect(wrapper.find('[data-testid="coverage-group-lanes"]').attributes('title')).toContain('以色列内部选举')

    const row = wrapper.find('[data-testid="matrix-lane-20"]')
    expect(row.exists()).toBe(true)
    // 覆盖行名字是静态标签（不在本窗名单，不可选中）；行容器沿用休眠行弱化样式
    expect(row.element.tagName).toBe('SPAN')
    expect(row.element.parentElement!.className).toContain('is-dormant')
    // 身份由组标题表达，行内不重复小标（不挤占窄标签列的名字）
    expect(row.text()).toContain('以色列内部选举')
    expect(row.text()).not.toContain('未进日报')
    // 覆盖行是静态标签（非 button）：身份与「未进日报」只在 aria-label，不挂原生 title。
    expect(row.attributes('title')).toBeUndefined()
    expect(row.attributes('aria-label')).toContain('窗口内未进日报')
    expect(wrapper.find('[data-testid="matrix-cell-20-2026-10-23"]').text()).toBe('—')
  })

  it('最近一次缺失 → 写「无记录」（不拿今天冒充）', () => {
    const wrapper = mountGrid({ coverageLanes: [{ topicId: '21', label: '无记录线', lastSeenDate: null }] })
    expect(wrapper.find('[data-testid="coverage-group-lanes"]').text()).toContain('无记录')
  })

  it('清单失败：只出一行提示 + 重试（不把失败渲染成「没有窗口外泳道」）', async () => {
    const wrapper = mountGrid({ coverageLanes: [], coverageError: '接口挂了' })
    expect(wrapper.find('[data-testid="coverage-group-summary"]').exists()).toBe(false)
    const errorRow = wrapper.find('[data-testid="coverage-error-row"]')
    expect(errorRow.exists()).toBe(true)
    expect(errorRow.text()).toContain('窗口外泳道清单未取到')
    await wrapper.find('[data-testid="coverage-retry"]').trigger('click')
    expect(wrapper.emitted('retryCoverage')).toHaveLength(1)
  })

  it('读取中 / 成功且为空 → 不渲染组（不占位、不冒「0 条」）', () => {
    expect(mountGrid({ coverageLanes: [], coverageLoading: true }).find('[data-testid="coverage-group-summary"]').exists()).toBe(false)
    expect(mountGrid({ coverageLanes: [] }).find('[data-testid="coverage-group-summary"]').exists()).toBe(false)
  })

  it('覆盖行有泳道维护菜单（重命名/编辑/归档），且不提供行选中', () => {
    const wrapper = mountGrid({ coverageLanes: coverage, renameLane: vi.fn().mockResolvedValue(true) })
    const row = wrapper.find('[data-testid="matrix-lane-20"]')
    expect(row.element.tagName).toBe('SPAN')
    expect(row.find('button').exists()).toBe(false)
    expect(document.body.querySelector('[data-testid="lane-menu-trigger-20"]')).toBeNull()
  })
})

describe('ObservationMatrixGrid — 休眠折叠条列名字', () => {
  it('折叠条显示泳道名（数量 + 名字），超长由 CSS 截断但 title/aria 保留全文', () => {
    const wrapper = mountGrid({ dormantRows: [lane('12', '休眠线 C', true), lane('13', '休眠线 D', true)] })
    const names = wrapper.find('[data-testid="dormant-names"]')
    expect(names.text()).toBe('休眠线 C · 休眠线 D')
    expect(names.attributes('title')).toBe('休眠线 C · 休眠线 D')
  })
})
