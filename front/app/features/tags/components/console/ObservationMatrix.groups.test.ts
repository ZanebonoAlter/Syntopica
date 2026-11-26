/**
 * ObservationMatrix 组顺序与其他行子套件（8C G2，test-cases-matrix-day-others.md GRP-1/3/4、
 * OTH-12；8C.4S 结构规范包从 ObservationMatrix.test.ts 拆出）。
 *
 *  - DOM 顺序：正式行 → 当日其他话题 summary/行 → 休眠 summary → 休眠行；0 休眠仍有同级
 *    summary（本窗无、禁用展开、不造行）
 *  - 两 toggle 互不串联（渲染态只由 props 决定）；收起时组行离 DOM、summary 显示真实计数
 *  - 其他行只标所选日（其余日期空白 aria-hidden 不可聚焦）；选行/选格同一事件；
 *    选中高亮不污染正式行；长窗两组 summary 仍全宽
 *
 * fixture 见 `observationMatrixTestHarness.ts`。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { buildDayAxis } from '~/features/tags/composables/laneMatrixModel'
import { AXIS14, TODAY, mountMatrix, otherView } from './observationMatrixTestHarness'

const notifyWarn = vi.fn()
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ warn: notifyWarn, success: vi.fn(), error: vi.fn() }),
}))
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))

describe('ObservationMatrix — 组顺序与折叠独立性（GRP-1/3/4）', () => {
  beforeEach(() => notifyWarn.mockReset())

  it('DOM 顺序 = 正式行 → 其他 summary → 其他行 → 休眠 summary → 休眠行', () => {
    const wrapper = mountMatrix({
      selectedDate: '2026-10-23',
      otherTopics: otherView(),
      othersOpen: true,
      expandedDormant: true,
    })
    const grid = wrapper.find('[data-testid="matrix-grid"]').element
    const order = [...grid.querySelectorAll(
      '[data-testid^="matrix-lane-"], [data-testid="other-topics-toggle"], [data-testid^="other-topic-row-"], [data-testid="dormant-toggle"]',
    )].map(node => node.getAttribute('data-testid'))
    expect(order).toEqual([
      'matrix-lane-11',
      'matrix-lane-10',
      'other-topics-toggle',
      'other-topic-row-701',
      'other-topic-row-702',
      'dormant-toggle',
      'matrix-lane-12',
    ])
    // 两个 summary 都纵向全宽（同一 grid-column: 1/-1 类），均在 grid 容器内
    expect(wrapper.find('[data-testid="other-topics-toggle"]').classes()).toContain('omx-group-summary')
    expect(wrapper.find('[data-testid="dormant-toggle"]').classes()).toContain('omx-group-summary')
  })

  it('两 toggle 互不串联：各自 emit，渲染态只由 props 决定（受控 wrapper 模拟）', async () => {
    const wrapper = mountMatrix({ selectedDate: '2026-10-23', otherTopics: otherView(), othersOpen: true })
    await wrapper.find('[data-testid="other-topics-toggle"]').find('button').trigger('click')
    expect(wrapper.emitted('toggleOthers')).toHaveLength(1)
    expect(wrapper.emitted('toggleDormant')).toBeUndefined()
    await wrapper.find('[data-testid="dormant-toggle"]').find('button').trigger('click')
    expect(wrapper.emitted('toggleDormant')).toHaveLength(1)
    expect(wrapper.emitted('toggleOthers')).toHaveLength(1)
    // 组件不自持展开态：行仍按 othersOpen=true 在场，休眠行仍按 expandedDormant=false 不在 DOM
    expect(wrapper.find('[data-testid="other-topic-row-701"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="matrix-lane-12"]').exists()).toBe(false)
    // 两 toggle 都是原生 button + aria-expanded
    expect(wrapper.find('[data-testid="other-topics-toggle"]').find('button').attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[data-testid="dormant-toggle"]').find('button').attributes('aria-expanded')).toBe('false')
  })

  it('收起其他组：自身行离 DOM，summary 仍显示真实计数', () => {
    const wrapper = mountMatrix({ selectedDate: '2026-10-23', otherTopics: otherView(), othersOpen: false })
    expect(wrapper.find('[data-testid^="other-topic-row-"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="other-topics-count"]').text()).toContain('1 条线索')
    expect(wrapper.find('[data-testid="other-topics-note"]').text()).toContain('未入泳道')
  })

  it('0 休眠仍有 0/本窗无 summary，禁用展开且不造行', () => {
    const wrapper = mountMatrix({ dormantRows: [], expandedDormant: true })
    const summary = wrapper.find('[data-testid="dormant-toggle"]')
    expect(summary.exists()).toBe(true)
    expect(summary.text()).toContain('休眠泳道')
    expect(summary.text()).toContain('0')
    expect(summary.text()).toContain('本窗无')
    expect(summary.find('button').attributes('disabled')).toBeDefined()
    expect(wrapper.findAll('[data-testid^="matrix-lane-"]')).toHaveLength(2)
  })

  it('非 ready 不冒 0：不给计数、summary 给状态文案、不可展开', () => {
    const loadingView = otherView({ state: 'loading', rows: [], clueCount: null, statusText: '正在读取当日报告…' })
    const wrapper = mountMatrix({ selectedDate: '2026-10-23', otherTopics: loadingView, othersOpen: false })
    expect(wrapper.find('[data-testid="other-topics-count"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="other-topics-note"]').text()).toBe('正在读取当日报告…')
    expect(wrapper.find('[data-testid="other-topics-toggle"]').find('button').attributes('disabled')).toBeDefined()

    const noReport = otherView({ state: 'no-report', rows: [], clueCount: null, statusText: '所选日没有日报（不是 0 条其他话题）' })
    expect(mountMatrix({ selectedDate: '2026-10-23', otherTopics: noReport })
      .find('[data-testid="other-topics-note"]').text()).toContain('不是 0 条其他话题')
  })

  it('未接线（otherTopics 省略）：组按「未选日」诚实降级，不伪造 0 行', () => {
    const wrapper = mountMatrix()
    expect(wrapper.find('[data-testid="other-topics-note"]').text()).toBe('先选择一个有日报的日期')
    expect(wrapper.find('[data-testid="other-topics-count"]').exists()).toBe(false)
  })
})

describe('ObservationMatrix — 其他行只标所选日（GRP-4/OTH-12）', () => {
  beforeEach(() => notifyWarn.mockReset())

  it('只有所选日列有计数格（0 也可点），其余日期空白 aria-hidden 不可聚焦', async () => {
    const wrapper = mountMatrix({ selectedDate: '2026-10-23', otherTopics: otherView(), othersOpen: true })
    expect(wrapper.findAll('[data-testid^="other-topic-cell-"]')).toHaveLength(2)
    const blanks = wrapper.findAll('.omx-other-blank')
    expect(blanks).toHaveLength(2 * (AXIS14.length - 1))
    expect(blanks[0]!.attributes('aria-hidden')).toBe('true')
    expect(blanks[0]!.element.tagName).toBe('DIV')

    const zeroCell = wrapper.find('[data-testid="other-topic-cell-702-2026-10-23"]')
    expect(zeroCell.text()).toBe('0')
    expect(zeroCell.attributes('disabled')).toBeUndefined()
    await zeroCell.trigger('click')
    expect(wrapper.emitted('selectOther')![0]).toEqual([{ sectionId: '702', date: '2026-10-23', topicId: '88' }])
  })

  it('选行与选格同一事件：稳定 { sectionId, date, topicId }；行 title/aria 含状态词与「点下方读全文」', async () => {
    const wrapper = mountMatrix({ selectedDate: '2026-10-23', otherTopics: otherView(), othersOpen: true })
    const row = wrapper.find('[data-testid="other-topic-row-701"]')
    expect(row.attributes('title')).toContain('乌干达因过境旅客确诊加德满都口岸加强防疫')
    expect(row.attributes('title')).toContain('当次未归属')
    expect(row.attributes('title')).toContain('点下方读全文')
    expect(row.attributes('aria-label')).toBe(row.attributes('title'))
    await row.trigger('click')
    expect(wrapper.emitted('selectOther')![0]).toEqual([{ sectionId: '701', date: '2026-10-23', topicId: null }])
    // 不弹层、不新增第二组日期控件
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.find('select').exists()).toBe(false)
  })

  it('选中其他分区高亮（aria-pressed）不污染正式行选中', () => {
    const wrapper = mountMatrix({
      selectedDate: '2026-10-23',
      otherTopics: otherView(),
      othersOpen: true,
      selectedOtherSectionId: '702',
    })
    expect(wrapper.find('[data-testid="other-topic-row-702"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('[data-testid="other-topic-row-701"]').attributes('aria-pressed')).toBe('false')
    expect(wrapper.find('[data-testid="matrix-lane-10"]').attributes('aria-pressed')).toBe('true')
  })

  it('长窗口（365 列）：两组 summary 仍全宽且留在 grid 容器内', () => {
    const wrapper = mountMatrix({
      axis: buildDayAxis(TODAY, 365),
      selectedDate: '2026-10-23',
      otherTopics: otherView(),
      othersOpen: true,
    })
    const grid = wrapper.find('[data-testid="matrix-grid"]')
    expect(grid.classes()).toContain('is-scrollable')
    expect(wrapper.find('[data-testid="other-topics-toggle"]').element.closest('[data-testid="matrix-grid"]')).not.toBeNull()
    expect(wrapper.find('[data-testid="dormant-toggle"]').element.closest('[data-testid="matrix-grid"]')).not.toBeNull()
    expect(wrapper.find('[data-testid="other-topics-toggle"]').classes()).toContain('omx-group-summary')
  })
})
