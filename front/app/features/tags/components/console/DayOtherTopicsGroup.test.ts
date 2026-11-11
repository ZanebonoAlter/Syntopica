/**
 * DayOtherTopicsGroup 测试（rework-workshop-into-radar-and-desk 8C G2，test-cases-matrix-day-others.md GRP/OTH-12）。
 *
 * 本组件纯受控：行/计数/状态文案来自 G1 `buildOtherTopics` 视图，展开态来自壳层 props。
 *  - GRP-2 默认态：省略 `expanded` = 收起；展开/收起只由 prop 决定，点 toggle 只 emit（不自持第二份状态）
 *  - GRP-4 折叠：行离 DOM、summary 仍显示真实计数/状态
 *  - GRP-5 空/非 ready：summary 与状态文案在场，不冒 0 行、不给计数
 *  - GRP-6 行密度：两行 clamp（样式规则锚）+ 完整 title/aria（含当时状态词）
 *  - OTH-12 只标所选日：只有所选日列有计数格（0 合法可点），其余空白 aria-hidden 不可聚焦
 *  - 选行/选格发同一 `selectOther` 事件 `{ sectionId, date, topicId }`
 */
import { readFileSync } from 'node:fs'
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import DayOtherTopicsGroup from './DayOtherTopicsGroup.vue'
import {
  OTHER_TOPICS_EMPTY_TEXT,
  OTHER_TOPICS_ERROR_TEXT,
  OTHER_TOPICS_INDEX_ERROR_TEXT,
  OTHER_TOPICS_INDEX_LOADING_TEXT,
  OTHER_TOPICS_LOADING_TEXT,
  OTHER_TOPICS_NO_DATE_TEXT,
  OTHER_TOPICS_NO_REPORT_TEXT,
  OTHER_TOPICS_UNAVAILABLE_NOTE,
  type OtherTopicsView,
} from './dayOtherTopics'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))

const AXIS = ['2026-10-21', '2026-10-22', '2026-10-23']
const SELECTED = '2026-10-23'

function readyView(overrides: Partial<OtherTopicsView> = {}): OtherTopicsView {
  return {
    state: 'ready',
    rows: [
      {
        sectionId: '701',
        label: '乌干达因过境旅客确诊加德满都口岸加强防疫',
        clusterIndex: 0,
        threadCount: 1,
        persistentTopicId: null,
        statusKind: 'unattributed',
        statusText: '当次未归属',
      },
      {
        sectionId: '702',
        label: '第二条其他分区（零线索）',
        clusterIndex: 1,
        threadCount: 0,
        persistentTopicId: '88',
        statusKind: 'active',
        statusText: '当时已转正',
      },
    ],
    clueCount: 1,
    statusText: null,
    note: null,
    retryable: false,
    ...overrides,
  }
}

function passiveView(state: OtherTopicsView['state'], statusText: string, extra: Partial<OtherTopicsView> = {}): OtherTopicsView {
  return { state, rows: [], clueCount: null, statusText, note: null, retryable: false, ...extra }
}

function mountGroup(overrides: Record<string, unknown> = {}) {
  return mount(DayOtherTopicsGroup, {
    props: {
      axis: AXIS,
      selectedDate: SELECTED,
      view: readyView(),
      expanded: false,
      selectedSectionId: null,
      ...overrides,
    },
  })
}

describe('DayOtherTopicsGroup — summary 与展开态（GRP-2/4）', () => {
  it('收起（默认，省略 expanded）：summary 在场且显示真实计数，自身行不在 DOM', () => {
    const wrapper = mountGroup()
    const summary = wrapper.find('[data-testid="other-topics-toggle"]')
    expect(summary.exists()).toBe(true)
    expect(summary.classes()).toContain('omx-group-summary')
    expect(wrapper.find('[data-testid="other-topics-count"]').text()).toContain('1 条线索')
    expect(wrapper.find('[data-testid="other-topics-note"]').text()).toBe('2 个分区未入泳道')
    expect(summary.find('button').attributes('aria-expanded')).toBe('false')
    expect(summary.find('button').attributes('disabled')).toBeUndefined()
    expect(wrapper.find('[data-testid^="other-topic-row-"]').exists()).toBe(false)
  })

  it('组计数 = 组内线索合计（一个分区 3 条线索 → 3 条线索，不是分区数 1；2026-11-11 用户反馈）', () => {
    const wrapper = mountGroup({
      view: readyView({
        rows: [{
          sectionId: '8312',
          label: '财政部发布十五五会计规划，强化监督与造假治理',
          clusterIndex: 4,
          threadCount: 3,
          persistentTopicId: '2250',
          statusKind: 'candidate',
          statusText: '当时待观察',
        }],
        clueCount: 3,
      }),
    })
    expect(wrapper.find('[data-testid="other-topics-count"]').text()).toContain('3 条线索')
    expect(wrapper.find('[data-testid="other-topics-count"]').text()).not.toContain('1 条线索')
    // 分区数只在说明文案里（1 个分区不放徽标冒充线索数）
    expect(wrapper.find('[data-testid="other-topics-note"]').text()).toBe('1 个分区未入泳道')
  })

  it('展开（expanded=true）：summary 在前、自身行紧随其后；aria-expanded=true', () => {
    const wrapper = mountGroup({ expanded: true })
    expect(wrapper.find('[data-testid="other-topics-toggle"]').find('button').attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[data-testid="other-topics-note"]').text()).toBe('只标所选日')
    const ids = wrapper.findAll('[data-testid^="other-topic-row-"]').map(node => node.attributes('data-testid'))
    expect(ids).toEqual(['other-topic-row-701', 'other-topic-row-702'])
    // summary 在行之前（DOM 顺序）
    const html = wrapper.html()
    expect(html.indexOf('other-topics-toggle')).toBeLessThan(html.indexOf('other-topic-row-701'))
  })

  it('纯受控：点 toggle 只 emit toggleOthers，自身渲染态不变（展开/收起由壳层决定）', async () => {
    const collapsed = mountGroup()
    await collapsed.find('[data-testid="other-topics-toggle"]').find('button').trigger('click')
    expect(collapsed.emitted('toggleOthers')).toHaveLength(1)
    expect(collapsed.find('[data-testid^="other-topic-row-"]').exists()).toBe(false)

    const opened = mountGroup({ expanded: true })
    await opened.find('[data-testid="other-topics-toggle"]').find('button').trigger('click')
    expect(opened.emitted('toggleOthers')).toHaveLength(1)
    expect(opened.find('[data-testid="other-topic-row-701"]').exists()).toBe(true)
  })

  it('ready + 0 行：0 是合法计数（可展开看「都已可在泳道读到」），不是加载失败', () => {
    const wrapper = mountGroup({ view: readyView({ rows: [], clueCount: 0, statusText: OTHER_TOPICS_EMPTY_TEXT }), expanded: true })
    expect(wrapper.find('[data-testid="other-topics-count"]').text()).toBe('· 0 条线索')
    expect(wrapper.find('[data-testid="other-topics-note"]').text()).toBe(OTHER_TOPICS_EMPTY_TEXT)
    expect(wrapper.find('[data-testid="other-topics-note-row"]').text()).toBe(OTHER_TOPICS_EMPTY_TEXT)
    expect(wrapper.find('[data-testid^="other-topic-row-"]').exists()).toBe(false)
  })

  it('部分泳道当日未取到：组说明追加一句（不改行集、不补 0）', () => {
    const wrapper = mountGroup({ view: readyView({ note: OTHER_TOPICS_UNAVAILABLE_NOTE }) })
    expect(wrapper.find('[data-testid="other-topics-extra"]').text()).toBe(OTHER_TOPICS_UNAVAILABLE_NOTE)
    expect(wrapper.findAll('[data-testid^="other-topic-row-"]')).toHaveLength(0)
  })
})

describe('DayOtherTopicsGroup — 状态分流不冒 0（GRP-5）', () => {
  const cases: [OtherTopicsView, string][] = [
    [passiveView('no-date', OTHER_TOPICS_NO_DATE_TEXT), OTHER_TOPICS_NO_DATE_TEXT],
    [passiveView('index-loading', OTHER_TOPICS_INDEX_LOADING_TEXT), OTHER_TOPICS_INDEX_LOADING_TEXT],
    [passiveView('index-error', OTHER_TOPICS_INDEX_ERROR_TEXT, { retryable: true }), OTHER_TOPICS_INDEX_ERROR_TEXT],
    [passiveView('loading', OTHER_TOPICS_LOADING_TEXT), OTHER_TOPICS_LOADING_TEXT],
    [passiveView('no-report', OTHER_TOPICS_NO_REPORT_TEXT), OTHER_TOPICS_NO_REPORT_TEXT],
    [passiveView('error', OTHER_TOPICS_ERROR_TEXT, { retryable: true }), OTHER_TOPICS_ERROR_TEXT],
  ]

  it('非 ready 一律不给计数（不写 0）、summary 给状态文案、不可展开', () => {
    for (const [view, text] of cases) {
      const wrapper = mountGroup({ view, expanded: true })
      expect(wrapper.find('[data-testid="other-topics-toggle"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="other-topics-count"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="other-topics-note"]').text()).toBe(text)
      expect(wrapper.find('[data-testid="other-topics-toggle"]').find('button').attributes('disabled')).toBeDefined()
      expect(wrapper.find('[data-testid^="other-topic-row-"]').exists()).toBe(false)
    }
  })

  it('无报日文案写明「不是 0 条其他话题」；未接线（view=null）按未选日降级', () => {
    expect(mountGroup({ view: passiveView('no-report', OTHER_TOPICS_NO_REPORT_TEXT) })
      .find('[data-testid="other-topics-note"]').text()).toContain('不是 0 条其他话题')
    const unwired = mountGroup({ view: null })
    expect(unwired.find('[data-testid="other-topics-note"]').text()).toBe(OTHER_TOPICS_NO_DATE_TEXT)
    expect(unwired.find('[data-testid="other-topics-count"]').exists()).toBe(false)
    expect(unwired.find('[data-testid="other-topics-toggle"]').find('button').attributes('disabled')).toBeDefined()
  })
})

describe('DayOtherTopicsGroup — 只标所选日与就地下读（OTH-12/GRP-6）', () => {
  it('只有所选日列有计数格，其余日期空白 aria-hidden 且不可聚焦', () => {
    const wrapper = mountGroup({ expanded: true })
    const cells = wrapper.findAll('[data-testid^="other-topic-cell-"]')
    expect(cells).toHaveLength(2)
    expect(cells.map(cell => cell.attributes('data-date'))).toEqual([SELECTED, SELECTED])

    const blanks = wrapper.findAll('.omx-other-blank')
    expect(blanks).toHaveLength(2 * (AXIS.length - 1))
    for (const blank of blanks) {
      expect(blank.element.tagName).toBe('DIV')
      expect(blank.attributes('aria-hidden')).toBe('true')
      expect(blank.attributes('tabindex')).toBeUndefined()
    }
  })

  it('0 线索可点：选格与选行发同一稳定事件 { sectionId, date, topicId }', async () => {
    const wrapper = mountGroup({ expanded: true })
    const zeroCell = wrapper.find('[data-testid="other-topic-cell-702-2026-10-23"]')
    expect(zeroCell.text()).toBe('0')
    expect(zeroCell.attributes('disabled')).toBeUndefined()
    await zeroCell.trigger('click')
    await wrapper.find('[data-testid="other-topic-row-701"]').trigger('click')
    expect(wrapper.emitted('selectOther')).toEqual([
      [{ sectionId: '702', date: SELECTED, topicId: '88' }],
      [{ sectionId: '701', date: SELECTED, topicId: null }],
    ])
    // 不弹层、不强滚、不抢焦点（只 emit）
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })

  it('行 title/aria = 完整标题 + 当时状态词 + 点下方读全文（不截断）', () => {
    const wrapper = mountGroup({ expanded: true })
    const row = wrapper.find('[data-testid="other-topic-row-701"]')
    expect(row.attributes('title')).toBe('乌干达因过境旅客确诊加德满都口岸加强防疫 · 当次未归属 · 点下方读全文')
    expect(row.attributes('aria-label')).toBe(row.attributes('title'))
    expect(row.find('.omx-other-name').text()).toBe('乌干达因过境旅客确诊加德满都口岸加强防疫')
  })

  it('选中其他分区：行/格 aria-pressed 与选中态（不冒充泳道选中）', () => {
    const wrapper = mountGroup({ expanded: true, selectedSectionId: '702' })
    expect(wrapper.find('[data-testid="other-topic-row-702"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('[data-testid="other-topic-row-701"]').attributes('aria-pressed')).toBe('false')
    expect(wrapper.find('[data-testid="other-topic-cell-702-2026-10-23"]').classes()).toContain('is-selected')
    expect(wrapper.find('[data-testid="other-topic-cell-701-2026-10-23"]').classes()).not.toContain('is-selected')
  })
})

describe('DayOtherTopicsGroup — 布局锚（全宽/共享泳道视觉/参与父网格）', () => {
  it('组根参与父网格；行/格与正式泳道共用样式，单行标题及实底热度不另立规则', () => {
    const groupSource = readFileSync(`${import.meta.dirname}/DayOtherTopicsGroup.vue`, 'utf8')
    expect(groupSource).toMatch(/\.otg\s*\{[^}]*display:\s*contents/)
    const gridSource = readFileSync(`${import.meta.dirname}/ObservationMatrixGrid.vue`, 'utf8')
    expect(gridSource).toMatch(/\.omx-group-summary[^{}]*\{[^}]*grid-column:\s*1\s*\/\s*-1/)
    const laneSource = readFileSync(`${import.meta.dirname}/ObservationMatrixLaneRow.vue`, 'utf8')
    const rowStyles = readFileSync(`${import.meta.dirname}/observationMatrixRows.css`, 'utf8')
    for (const source of [groupSource, laneSource]) {
      expect(source).toContain('src="./observationMatrixRows.css"')
    }
    expect(groupSource).toContain('class="omx-lane omx-other-lane"')
    expect(groupSource).toContain('class="omx-cell omx-other-cell"')
    expect(rowStyles).toMatch(/height:\s*2\.9rem/)
    expect(rowStyles).toMatch(/white-space:\s*nowrap/)
    expect(rowStyles).toContain('var(--color-accent) 66%, var(--color-bg-base)')
    expect(rowStyles).not.toContain('dashed')
    expect(gridSource).not.toContain('.omx-other-cell.is-selected')
  })
})
