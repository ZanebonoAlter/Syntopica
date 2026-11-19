/**
 * ArchiveHistoryStrip 组件测试（archive-time-chart-range）。
 *
 * 覆盖：拖动 emit preview 连续 + commit 恰一次、窗外按下先居中、Esc / pointercancel
 * 走 cancel、键盘 ←/Shift+←/Home/End 每次按键 emit 一次 commit。
 * mock：SVG getBoundingClientRect 打桩为 width=1000（像素↔日期线性换算的输入）。
 */
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ArchiveHistoryStrip from './ArchiveHistoryStrip.vue'
import { shiftDate } from './observationRange'

const EARLIEST = '2026-01-01'
const LATEST = '2026-12-31'
/** 全历史 365 天；视窗取 11 月（index 304..333）。 */
const VIEWPORT = { start: '2026-11-01', end: '2026-11-30' }
const RECT = { left: 0, top: 0, width: 1000, height: 28, right: 1000, bottom: 28, x: 0, y: 0 }

/** 视窗内一天（2026-11-15）对应的像素 x。 */
function xOf(date: string): number {
  const index = Math.round((Date.parse(`${date}T00:00:00Z`) - Date.parse(`${EARLIEST}T00:00:00Z`)) / 86_400_000)
  return (index / 364) * RECT.width
}

function mountStrip(props: Partial<InstanceType<typeof ArchiveHistoryStrip>['$props']> = {}) {
  const wrapper = mount(ArchiveHistoryStrip, {
    props: {
      earliest: EARLIEST,
      latest: LATEST,
      byDate: new Map(),
      viewport: VIEWPORT,
      ...props,
    },
  })
  const svg = wrapper.find('svg')
  ;(svg.element as SVGSVGElement).getBoundingClientRect = () => RECT as DOMRect
  return { wrapper, svg }
}

describe('ArchiveHistoryStrip — 拖动平移视窗', () => {
  it('窗内按下拖动：preview 连续发出，松手 commit 恰一次', async () => {
    const { wrapper, svg } = mountStrip()
    await svg.trigger('pointerdown', { clientX: xOf('2026-11-06'), button: 0, pointerId: 1 })
    await svg.trigger('pointermove', { clientX: xOf('2026-11-25'), button: 0, pointerId: 1 })

    const previews = wrapper.emitted('viewport-preview')!
    expect(previews.length).toBeGreaterThanOrEqual(2)
    const last = previews.at(-1)![0] as { start: string, end: string }
    // 拖动保持窗口宽度（30 天）：光标 11-25 → 右端 11-25+24 天 = 12-19
    expect(last.end).toBe('2026-12-19')
    expect(last.start).toBe('2026-11-20')

    await svg.trigger('pointerup', { clientX: xOf('2026-11-25'), button: 0, pointerId: 1 })
    const commits = wrapper.emitted('viewport-commit')!
    expect(commits).toHaveLength(1)
    expect(commits[0]![0]).toEqual(last)
    expect(wrapper.emitted('viewport-cancel')).toBeUndefined()
  })

  it('窗外按下：先把点击日居中再继续拖（窗口宽度保持）', async () => {
    const { wrapper, svg } = mountStrip()
    await svg.trigger('pointerdown', { clientX: xOf('2026-02-06'), button: 0, pointerId: 1 })
    const first = wrapper.emitted('viewport-preview')![0]![0] as { start: string, end: string }
    // 点击日 2026-02-06 落在视窗中部：窗口 = [点击日-15, 点击日+14]
    expect(first.start).toBe('2026-01-22')
    expect(first.end).toBe('2026-02-20')
    expect(new Date(`${first.end}T00:00:00Z`).getTime() - new Date(`${first.start}T00:00:00Z`).getTime())
      .toBe(29 * 86_400_000)
  })

  it('Esc：取消手势，不提交', async () => {
    const { wrapper, svg } = mountStrip()
    await svg.trigger('pointerdown', { clientX: xOf('2026-11-06'), button: 0, pointerId: 1 })
    await svg.trigger('pointermove', { clientX: xOf('2026-11-25'), button: 0, pointerId: 1 })
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('viewport-cancel')).toHaveLength(1)
    await svg.trigger('pointerup', { clientX: xOf('2026-11-25'), button: 0, pointerId: 1 })
    expect(wrapper.emitted('viewport-commit')).toBeUndefined()
  })

  it('pointercancel：取消手势，不提交', async () => {
    const { wrapper, svg } = mountStrip()
    await svg.trigger('pointerdown', { clientX: xOf('2026-11-06'), button: 0, pointerId: 1 })
    await svg.trigger('pointercancel', { clientX: xOf('2026-11-25'), button: 0, pointerId: 1 })
    expect(wrapper.emitted('viewport-cancel')).toHaveLength(1)
    expect(wrapper.emitted('viewport-commit')).toBeUndefined()
  })
})

describe('ArchiveHistoryStrip — 键盘步进', () => {
  it('← 平移 1 天、Shift+← 平移 7 天：每次按键 emit 一次 commit', async () => {
    const { wrapper } = mountStrip()
    const window = wrapper.find('[data-testid="archive-history-window"]')
    await window.trigger('keydown', { key: 'ArrowLeft' })
    expect(wrapper.emitted('viewport-commit')).toHaveLength(1)
    expect(wrapper.emitted('viewport-commit')![0]![0]).toEqual({ start: '2026-10-31', end: '2026-11-29' })

    await window.trigger('keydown', { key: 'ArrowLeft', shiftKey: true })
    const commits = wrapper.emitted('viewport-commit')!
    expect(commits).toHaveLength(2)
    // 受控组件：第二次按键仍从同一 viewport 派生（右端 11-30 再退 7 天）
    expect(commits[1]![0]).toEqual({ start: '2026-10-25', end: '2026-11-23' })
  })

  it('Home / End：到历史最早 / 最新', async () => {
    const { wrapper } = mountStrip()
    const window = wrapper.find('[data-testid="archive-history-window"]')
    await window.trigger('keydown', { key: 'Home' })
    expect(wrapper.emitted('viewport-commit')![0]![0]).toEqual({ start: '2026-01-01', end: '2026-01-30' })
    await window.trigger('keydown', { key: 'End' })
    expect(wrapper.emitted('viewport-commit')![1]![0]).toEqual({ start: '2026-12-02', end: '2026-12-31' })
  })

  it('视窗窗口是 slider：aria-valuetext 给出起止（MM-DD）', () => {
    const { wrapper } = mountStrip()
    const window = wrapper.find('[data-testid="archive-history-window"]')
    expect(window.attributes('role')).toBe('slider')
    expect(window.attributes('aria-valuetext')).toBe('11-01 ~ 11-30')
  })
})

describe('ArchiveHistoryStrip — 无数据与禁用态', () => {
  it('earliest 为 null（版块无命中文章）：不画曲线，无手势响应', async () => {
    const { wrapper, svg } = mountStrip({ earliest: null })
    expect(wrapper.find('.ahs-line').exists()).toBe(false)
    await svg.trigger('pointerdown', { clientX: 500, button: 0, pointerId: 1 })
    expect(wrapper.emitted('viewport-preview')).toBeUndefined()
  })

  it('disabled：保留轮廓但不响应手势', async () => {
    const { wrapper, svg } = mountStrip({ disabled: true })
    expect(wrapper.find('[data-testid="archive-history-window"]').exists()).toBe(true)
    await svg.trigger('pointerdown', { clientX: xOf('2026-11-06'), button: 0, pointerId: 1 })
    expect(wrapper.emitted('viewport-preview')).toBeUndefined()
  })

  it('两端标签：最早日 / 最新日（MM-DD）', () => {
    const { wrapper } = mountStrip()
    const ends = wrapper.findAll('.ahs-end').map(node => node.text())
    expect(ends).toEqual(['01-01', '12-31'])
  })

  it('历史边界：平移到最早时窗口左端贴齐最早日', async () => {
    const { wrapper } = mountStrip()
    const window = wrapper.find('[data-testid="archive-history-window"]')
    await window.trigger('keydown', { key: 'Home' })
    const range = wrapper.emitted('viewport-commit')![0]![0] as { start: string, end: string }
    expect(range.start).toBe(shiftDate(EARLIEST, 0))
  })
})
