/**
 * 左轨（版块栏）组件测试（console-rails-live-signals tasks 2.6）。
 *
 * 锁住已批准版面的行为契约：
 *  - 每行 = 名称 + **14 格**逐日线索条码（行内无数字、无口径说明文字）；
 *  - 未取到 = 斜纹占位（`is-missing`），与「安静」的真实 0（空槽）**形态不同**；
 *  - 选中行有选中态（左缘强调线）；只有选中行的当日格带脉动标记；
 *  - 治理数据（标签挂载数）只进 aria-label / 名称看全卡片；
 *  - `allowAddBoard` 入口行为不变。
 */
import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RailBoardRow } from './railClueAxis'
import BoardRail from './BoardRail.vue'

vi.mock('@iconify/vue', () => ({ Icon: { name: 'Icon', template: '<span class="icon-stub" />' } }))

const mounted: Array<{ unmount: () => void }> = []

function row(overrides: Partial<RailBoardRow> = {}): RailBoardRow {
  return {
    boardId: 1974,
    label: '中东地缘政治与美伊关系',
    cells: Array.from({ length: 14 }, () => 0),
    peak: 0,
    intensity: 1,
    tagCount: 409,
    anchorDate: '2026-11-18',
    ...overrides,
  }
}

function mountRail(props: Record<string, unknown> = {}) {
  const wrapper = mount(BoardRail, {
    props: {
      rows: [row()],
      axisDates: Array.from({ length: 14 }, (_, index) => `2026-11-${String(index + 5).padStart(2, '0')}`),
      selectedBoardId: 1974,
      loading: false,
      error: null,
      ...props,
    },
  })
  mounted.push(wrapper)
  return wrapper
}

afterEach(() => {
  for (const wrapper of mounted.splice(0)) wrapper.unmount()
})

describe('BoardRail — 逐日线索条码', () => {
  it('每行 = 名称 + 14 格条码（行内不出现数字与口径说明文字）', () => {
    const wrapper = mountRail({ rows: [row({ cells: Array.from({ length: 14 }, (_, i) => i) })] })
    expect(wrapper.find('[data-testid="board-rail-item-1974"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid^="board-rail-cell-1974-"]')).toHaveLength(14)
    const item = wrapper.find('[data-testid="board-rail-item-1974"]')
    expect(item.text()).toContain('中东地缘政治与美伊关系')
    expect(item.text()).not.toContain('409')
    expect(item.text()).not.toContain('每格')
  })

  it('未取到的格是斜纹占位（is-missing），与真实 0 的空槽形态不同', () => {
    const cells: (number | null)[] = Array.from({ length: 14 }, () => 0)
    cells[13] = null
    const wrapper = mountRail({ rows: [row({ cells })] })
    const missing = wrapper.find('[data-testid="board-rail-cell-1974-13"]')
    const quiet = wrapper.find('[data-testid="board-rail-cell-1974-0"]')
    expect(missing.classes()).toContain('is-missing')
    expect(quiet.classes()).not.toContain('is-missing')
    expect(quiet.classes()).not.toContain('is-val')
  })

  it('有线索的格按行内峰值着色（is-val）', () => {
    const cells = Array.from({ length: 14 }, () => 0)
    cells[3] = 4
    const wrapper = mountRail({ rows: [row({ cells, peak: 4 })] })
    expect(wrapper.find('[data-testid="board-rail-cell-1974-3"]').classes()).toContain('is-val')
  })
})

describe('BoardRail — 选中态与动效标记', () => {
  it('选中行有选中态；只有选中行的当日格带脉动标记', () => {
    const wrapper = mountRail({
      rows: [row({ boardId: 1974, cells: Array.from({ length: 14 }, () => 1) }), row({ boardId: 1975, cells: Array.from({ length: 14 }, () => 1) })],
      selectedBoardId: 1974,
    })
    expect(wrapper.find('[data-testid="board-rail-item-1974"]').classes()).toContain('is-active')
    expect(wrapper.find('[data-testid="board-rail-item-1975"]').classes()).not.toContain('is-active')
    // 当日格 = 共同轴末日（index 13）；只有选中行带脉动
    expect(wrapper.find('[data-testid="board-rail-cell-1974-13"]').classes()).toContain('is-now')
    expect(wrapper.find('[data-testid="board-rail-cell-1975-13"]').classes()).not.toContain('is-now')
  })

  it('未取到的当日格不脉动（没有值可强调）', () => {
    const cells: (number | null)[] = Array.from({ length: 14 }, () => 1)
    cells[13] = null
    const wrapper = mountRail({ rows: [row({ cells })] })
    expect(wrapper.find('[data-testid="board-rail-cell-1974-13"]').classes()).not.toContain('is-now')
  })
})

describe('BoardRail — 名称看全与入口', () => {
  it('治理数据退到 aria-label（不再挂原生 title）', () => {
    const wrapper = mountRail()
    const item = wrapper.find('[data-testid="board-rail-item-1974"]')
    const label = item.attributes('aria-label') ?? ''
    expect(label).toContain('标签挂载 409')
    expect(label).toContain('数据截至 2026-11-18')
    expect(item.attributes('title')).toBeUndefined()
  })

  it('名称被截断时悬停/聚焦出全名卡片（放得下则不打扰）', async () => {
    vi.useFakeTimers()
    const wrapper = mountRail()
    const item = wrapper.find('[data-testid="board-rail-item-1974"]')
    const name = item.find('.rail-item-name').element as HTMLElement

    // 放得下（scrollWidth ≤ clientWidth）：不出卡
    Object.defineProperty(name, 'scrollWidth', { value: 140, configurable: true })
    Object.defineProperty(name, 'clientWidth', { value: 140, configurable: true })
    await item.trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(document.body.querySelector('[data-testid="board-rail-name-card"]')).toBeNull()

    // 被省略号截断：键盘聚焦同样出卡（键鼠同权）
    Object.defineProperty(name, 'scrollWidth', { value: 320, configurable: true })
    await item.trigger('focus')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    const card = document.body.querySelector('[data-testid="board-rail-name-card"]')
    expect(card).not.toBeNull()
    expect(card!.textContent).toContain(row().label)
    expect(card!.textContent).toContain('数据截至 2026-11-18')
    // 卡片是视觉冗余：全名已在 aria-label 里，不重复播报
    expect(card!.getAttribute('aria-hidden')).toBe('true')

    await item.trigger('blur')
    await flushPromises()
    expect(document.body.querySelector('[data-testid="board-rail-name-card"]')).toBeNull()
    vi.useRealTimers()
  })

  it('点行 → 上抛 select；allowAddBoard 入口行为不变', async () => {
    const wrapper = mountRail({ allowAddBoard: true })
    await wrapper.find('[data-testid="board-rail-item-1974"]').trigger('click')
    expect(wrapper.emitted('select')).toEqual([[1974]])
    expect(wrapper.find('[data-testid="board-rail-add"]').exists()).toBe(true)
    await wrapper.find('[data-testid="board-rail-add"]').trigger('click')
    expect(wrapper.emitted('add-board')).toHaveLength(1)
  })

  it('默认不显示新建版块入口（唯一入口在总览）', () => {
    const wrapper = mountRail()
    expect(wrapper.find('[data-testid="board-rail-add"]').exists()).toBe(false)
  })

  it('overview 未取到 → 错误条 + 重试上抛', async () => {
    const wrapper = mountRail({ signalError: '版块信号未取到' })
    expect(wrapper.find('[data-testid="board-rail-retry"]').exists()).toBe(true)
    await wrapper.find('[data-testid="board-rail-retry"]').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })
})
