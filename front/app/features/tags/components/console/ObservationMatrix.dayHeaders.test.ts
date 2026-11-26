/**
 * ObservationMatrix 报日列头子套件（8C G2，test-cases-matrix-day-others.md RDY/GRP-1；
 * 8C.4S 结构规范包从 ObservationMatrix.test.ts 拆出）。
 *
 * 三态：未接线（reportDates 省略）列头全不可点、无提示；已接线未就绪/失败（null）不可点且
 * 不冒「该日没有日报」；Set 时只有报日是 button 且点它 emit `selectDayHeader`。另覆盖索引陈旧
 * 仍保留已知报日可点、0 正式泳道仍可点选日。fixture 见 `observationMatrixTestHarness.ts`。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { readFileSync } from 'node:fs'
import { mountMatrix, otherView } from './observationMatrixTestHarness'

const notifyWarn = vi.fn()
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ warn: notifyWarn, success: vi.fn(), error: vi.fn() }),
}))
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))

describe('ObservationMatrix — 报日列头三态（RDY/GRP-1）', () => {
  beforeEach(() => notifyWarn.mockReset())

  it('未接线（reportDates 省略）：列头全不可点、无提示，title 保持旧口径', () => {
    const wrapper = mountMatrix()
    expect(wrapper.find('[data-testid="matrix-report-index-hint"]').exists()).toBe(false)
    expect(wrapper.findAll('.omx-dayhead.is-report')).toHaveLength(0)
    const head = wrapper.find('[data-testid="matrix-dayhead-2026-10-23"]')
    expect(head.element.tagName).toBe('DIV')
    expect(head.attributes('title')).toBe('2026-10-23')
  })

  it('已接线未就绪（null + loading）：不可点 + 「读取报日…」，不冒「该日没有日报」', () => {
    const wrapper = mountMatrix({ reportDates: null, indexLoading: true })
    expect(wrapper.find('[data-testid="matrix-report-index-hint"]').text()).toBe('读取报日…')
    expect(wrapper.findAll('.omx-dayhead.is-report')).toHaveLength(0)
    const head = wrapper.find('[data-testid="matrix-dayhead-2026-10-23"]')
    expect(head.element.tagName).toBe('DIV')
    expect(head.attributes('title')).toContain('报日索引未就绪')
    expect(head.attributes('title')).not.toContain('该日没有日报')
  })

  it('已接线失败（null + error）：不可点 + 内联「暂不可选」提示', () => {
    const wrapper = mountMatrix({ reportDates: null, indexError: '列表挂了' })
    expect(wrapper.find('[data-testid="matrix-report-index-hint"]').text()).toContain('日期列头暂不可选')
    expect(wrapper.find('[data-testid="matrix-dayhead-2026-10-23"]').element.tagName).toBe('DIV')
  })

  it('Set：只有报日是 button 且点它 emit selectDayHeader；非报日不可点且说明', async () => {
    const wrapper = mountMatrix({ reportDates: new Set(['2026-10-22', '2026-10-23']) })
    const reportHead = wrapper.find('[data-testid="matrix-dayhead-2026-10-22"]')
    expect(reportHead.element.tagName).toBe('BUTTON')
    expect(reportHead.attributes('title')).toContain('选这一天')
    await reportHead.trigger('click')
    expect(wrapper.emitted('selectDayHeader')![0]).toEqual(['2026-10-22'])

    const plainHead = wrapper.find('[data-testid="matrix-dayhead-2026-10-21"]')
    expect(plainHead.element.tagName).toBe('DIV')
    expect(plainHead.attributes('title')).toContain('该日没有日报')
    await plainHead.trigger('click')
    expect(wrapper.emitted('selectDayHeader')).toHaveLength(1)
    // 与日格共用同一 selectedDate：列头高亮不另立状态
    expect(mountMatrix({ reportDates: new Set(['2026-10-22']), selectedDate: '2026-10-22' })
      .find('[data-testid="matrix-dayhead-2026-10-22"]').classes()).toContain('is-selected')
  })

  it('索引陈旧但已有旧 Set：已知报日仍可点，不一刀禁全部', async () => {
    const wrapper = mountMatrix({ reportDates: new Set(['2026-10-22']), indexError: '刷新失败' })
    expect(wrapper.find('[data-testid="matrix-report-index-hint"]').text()).toContain('刷新失败')
    const known = wrapper.find('[data-testid="matrix-dayhead-2026-10-22"]')
    expect(known.element.tagName).toBe('BUTTON')
    await known.trigger('click')
    expect(wrapper.emitted('selectDayHeader')![0]).toEqual(['2026-10-22'])
    expect(wrapper.find('[data-testid="matrix-dayhead-2026-10-21"]').attributes('title')).toContain('可能已陈旧')
  })

  it('0 正式泳道：轴保持连续 14 列，报日列头仍可点（不因 0 行禁用选日）', async () => {
    const wrapper = mountMatrix({
      emptyState: 'no-lanes',
      activeRows: [],
      dormantRows: [],
      reportDates: new Set(['2026-10-23']),
      selectedDate: '2026-10-23',
      otherTopics: otherView(),
    })
    expect(wrapper.findAll('.omx-dayhead')).toHaveLength(14)
    expect(wrapper.find('[data-testid="matrix-empty-no-lanes"]').text()).toContain('0 正式泳道')
    await wrapper.find('[data-testid="matrix-dayhead-2026-10-23"]').trigger('click')
    expect(wrapper.emitted('selectDayHeader')![0]).toEqual(['2026-10-23'])
  })
})


describe('ObservationMatrix — 8E 同归属关联日', () => {
  const node = {
    date: '2026-10-22', reportId: '900', sectionId: '101',
    sectionIds: ['101', '102'], sectionTitles: ['当天原题甲', '当天原题乙'], threadCount: 5,
  }

  it('同日只一个点、提示保留全部真实原题，hover/focus 浮到 body 不被横滚容器裁掉', async () => {
    const wrapper = mountMatrix({ observedDays: [node], reportDates: new Set(['2026-10-22']) })
    const head = wrapper.find('[data-testid="matrix-dayhead-2026-10-22"]')
    expect(head.findAll('[data-testid="date-association-marker"]')).toHaveLength(1)
    expect(head.attributes('title')).toContain('当天原题甲；当天原题乙')
    expect(head.attributes('aria-label')).toContain('同归属记录')
    await head.trigger('focus')
    expect(document.body.querySelector('[role="tooltip"]')?.textContent).toContain('当天原题乙')
    expect(head.find('[role="tooltip"]').exists()).toBe(false)
    const markerSource = readFileSync(`${import.meta.dirname}/DateAssociationMarker.vue`, 'utf8')
    expect(markerSource).toContain("strategy: 'fixed'")
    expect(markerSource).toMatch(/position:\s*fixed/)
    expect(markerSource).toMatch(/z-index:\s*1000/)
    await head.trigger('blur')
    expect(document.body.querySelector('[role="tooltip"]')).toBeNull()
    await head.trigger('mouseenter')
    expect(document.body.querySelector('[role="tooltip"]')?.textContent).toContain('当天原题甲')
    await head.trigger('mouseleave')
    wrapper.unmount()
  })

  it('关联日只发稳定首 section 节点事件，普通报日仍走旧事件；清空/换投影不留点', async () => {
    const wrapper = mountMatrix({ observedDays: [node], reportDates: new Set(['2026-10-22', '2026-10-23']) })
    await wrapper.find('[data-testid="matrix-dayhead-2026-10-22"]').trigger('click')
    expect(wrapper.emitted('selectObservedDay')).toEqual([[node]])
    expect(wrapper.emitted('selectDayHeader')).toBeUndefined()
    await wrapper.find('[data-testid="matrix-dayhead-2026-10-23"]').trigger('click')
    expect(wrapper.emitted('selectDayHeader')).toEqual([['2026-10-23']])
    await wrapper.setProps({ observedDays: [{ ...node, date: '2026-10-23' }] })
    expect(wrapper.find('[data-testid="matrix-dayhead-2026-10-22"] [data-testid="date-association-marker"]').exists()).toBe(false)
    await wrapper.setProps({ observedDays: [] })
    expect(wrapper.findAll('[data-testid="date-association-marker"]')).toHaveLength(0)
    wrapper.unmount()
  })
})
