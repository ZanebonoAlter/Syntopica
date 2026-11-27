import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ObservationMatrixLaneRow from './ObservationMatrixLaneRow.vue'
import type { MatrixCellView, ObservationLaneRow } from './observationLanes'

// 图标组件在单测里走网络取图标（本地子集插件不在 vitest 管线内）→ 换成空壳，避免外网超时噪声
vi.mock('@iconify/vue', () => ({ Icon: { name: 'Icon', template: '<span />' } }))

const notifyWarn = vi.fn()
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ warn: notifyWarn, success: vi.fn(), error: vi.fn() }),
}))

/**
 * 矩阵行头「⋯」入口（lane-manual-correction-loop 6.3）：
 *  点「⋯」**不触发行选中 / 选中日变更**（阻止事件冒泡）；菜单项动作以 emit 上抛
 * （「编辑泳道」→ 修正工作面，「归档泳道」→ 归档弹窗）。
 *
 * 「展示合理性」机械锚：菜单 Teleport 到 body（不落在行 DOM 内），层级取自 `Z_INDEX` 档位表
 * （`<style>` 内不写死 z-index）。
 */
const ROW: ObservationLaneRow = {
  topicId: '10',
  label: 'Claude Code 工具链',
  watchLinked: false,
  sectionTotal: 3,
  clueTotal: 11,
  dormant: false,
  snapshot: null,
}

const CELLS: MatrixCellView[] = [
  { kind: 'content', text: '4', openable: true, depth: 0.5, aria: '4 条线索', title: '2026-10-23 · 4 条线索' },
]

let wrapper: VueWrapper | null = null

function mountRow(overrides: Record<string, unknown> = {}) {
  wrapper = mount(ObservationMatrixLaneRow, {
    props: {
      row: ROW,
      cells: CELLS,
      axis: ['2026-10-23'],
      today: '2026-10-23',
      selectedLaneId: null,
      selectedDate: null,
      dormant: false,
      ...overrides,
    },
    attachTo: document.body,
  })
  return wrapper
}

/** 行头「⋯」触发器（testid 前缀刻意避开 `matrix-lane-`，免得被行序断言误收）。 */
function trigger(): HTMLButtonElement {
  return document.body.querySelector('[data-testid="lane-menu-trigger-10"]') as HTMLButtonElement
}

/** 选中列淡底用例的行格（content / gap / unavailable 三态）。 */
function mountCells(cells: MatrixCellView[], selectedDate: string | null, axis = ['2026-10-22', '2026-10-23']) {
  wrapper = mount(ObservationMatrixLaneRow, {
    props: {
      row: ROW,
      cells,
      axis,
      today: '2026-10-23',
      selectedLaneId: null,
      selectedDate,
      dormant: false,
    },
  })
  return wrapper
}

function cellView(kind: MatrixCellView['kind']): MatrixCellView {
  if (kind === 'gap') return { kind: 'gap', text: '—', openable: false, depth: 0, aria: '无分区', title: '无分区' }
  if (kind === 'unavailable') {
    return { kind: 'unavailable', text: '?', openable: false, depth: 0, aria: '未取到', title: '未取到' }
  }
  return { kind: 'content', text: '4', openable: true, depth: 0.5, aria: '4 条线索', title: '2026-10-23 · 4 条线索' }
}

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('ObservationMatrixLaneRow — 行头「⋯」入口', () => {
  it('点「⋯」不 emit 行选中 / 选中日', async () => {
    const w = mountRow()
    trigger().click()
    await flushPromises()

    expect(w.emitted('selectLane')).toBeUndefined()
    expect(w.emitted('selectCell')).toBeUndefined()
    // 菜单已展开（点一次即开）
    expect(document.body.querySelector('[data-testid="row-action-menu"]')).not.toBeNull()
  })

  it('菜单含「编辑泳道」「归档泳道」两项', async () => {
    mountRow()
    trigger().click()
    await flushPromises()

    const items = [...document.body.querySelectorAll('.row-menu-item')].map(el => el.textContent?.trim())
    expect(items).toEqual(['编辑泳道', '归档泳道'])
  })

  it('点「编辑泳道」→ emit editLane(topicId)', async () => {
    const w = mountRow()
    trigger().click()
    await flushPromises()
    ;(document.body.querySelector('[data-testid="lane-menu-trigger-10-edit"]') as HTMLButtonElement).click()
    await flushPromises()

    expect(w.emitted('editLane')).toEqual([['10']])
    expect(w.emitted('selectLane')).toBeUndefined()
    // 选完即收起
    expect(document.body.querySelector('[data-testid="row-action-menu"]')).toBeNull()
  })

  it('点「归档泳道」→ emit archiveLane(topicId)', async () => {
    const w = mountRow()
    trigger().click()
    await flushPromises()
    ;(document.body.querySelector('[data-testid="lane-menu-trigger-10-archive"]') as HTMLButtonElement).click()
    await flushPromises()

    expect(w.emitted('archiveLane')).toEqual([['10']])
    expect(w.emitted('selectLane')).toBeUndefined()
  })

  it('名称按钮仍可点选行（选中态与 data 属性在按钮上）', async () => {
    const w = mountRow()
    const nameBtn = document.body.querySelector('[data-testid="matrix-lane-10"]') as HTMLButtonElement
    expect(nameBtn.getAttribute('aria-pressed')).toBe('false')
    expect(nameBtn.getAttribute('data-lane-id')).toBe('10')
    nameBtn.click()
    await flushPromises()
    expect(w.emitted('selectLane')).toEqual([['10']])
  })

  it('菜单 Teleport 到 body（不落在行 DOM 内），层级取自 Z_INDEX 档位表', async () => {
    mountRow()
    trigger().click()
    await flushPromises()

    const menu = document.body.querySelector('[data-testid="row-action-menu"]') as HTMLElement
    expect(menu.parentElement).toBe(document.body)
    expect(menu.style.zIndex).toBe('9999')
  })
})

/**
 * 选中列淡底（console-rails-live-signals D7）：选中日**整列**加一层淡底，
 * 只铺在**有分区**的那天；无分区的历史日不铺（不盖过「— 无分区」的诚实表达）。
 */
describe('ObservationMatrixLaneRow — 选中列淡底', () => {
  it('选中日的有分区格带淡底标记；其它日期的格不带', () => {
    const w = mountCells([cellView('content'), cellView('content')], '2026-10-23')
    expect(w.find('[data-testid="matrix-cell-10-2026-10-23"]').classes()).toContain('is-col-selected')
    expect(w.find('[data-testid="matrix-cell-10-2026-10-22"]').classes()).not.toContain('is-col-selected')
  })

  it('选中日无分区（gap / 未取到）的格不铺淡底', () => {
    const w = mountCells([cellView('gap'), cellView('unavailable')], '2026-10-22')
    expect(w.find('[data-testid="matrix-cell-10-2026-10-22"]').classes()).not.toContain('is-col-selected')
    expect(w.find('[data-testid="matrix-cell-10-2026-10-23"]').classes()).not.toContain('is-col-selected')
  })

  it('未选中日时整列不铺淡底', () => {
    const w = mountCells([cellView('content'), cellView('content')], null)
    expect(w.find('[data-testid="matrix-cell-10-2026-10-22"]').classes()).not.toContain('is-col-selected')
    expect(w.find('[data-testid="matrix-cell-10-2026-10-23"]').classes()).not.toContain('is-col-selected')
  })
})

/**
 * 行头就地改名（`lane-matrix-coverage-and-others-row` 4.2 / spec「矩阵行头就地改名」）：
 *  ⋯ 菜单第一项「重命名」→ 行内输入框（Enter 保存 / Esc·失焦取消 / 空名拒收）。
 *  写入复用既有 `PATCH /daily-reports/topics/:id`（`renameLane` 注入）；失败保留编辑态。
 */
describe('ObservationMatrixLaneRow — 行头就地改名', () => {
  const renameInput = (w: VueWrapper) => w.find('[data-testid="matrix-lane-rename-10"]')

  /** 点开 ⋯ 菜单并选「重命名」（菜单 Teleport 到 body）。 */
  async function openRename(): Promise<void> {
    trigger().click()
    await flushPromises()
    ;(document.body.querySelector('[data-testid="lane-menu-trigger-10-rename"]') as HTMLButtonElement).click()
    await flushPromises()
  }

  it('注入改名函数时菜单首项是「重命名」（重命名 / 编辑泳道 / 归档泳道）', async () => {
    mountRow({ renameLane: vi.fn().mockResolvedValue(true) })
    trigger().click()
    await flushPromises()

    const items = [...document.body.querySelectorAll('.row-menu-item')].map(el => el.textContent?.trim())
    expect(items).toEqual(['重命名', '编辑泳道', '归档泳道'])
  })

  it('未注入改名函数时菜单不出现「重命名」（旧调用零改动）', async () => {
    mountRow()
    trigger().click()
    await flushPromises()

    const items = [...document.body.querySelectorAll('.row-menu-item')].map(el => el.textContent?.trim())
    expect(items).toEqual(['编辑泳道', '归档泳道'])
  })

  it('点「重命名」→ 行内变输入框（值 = 当前名）且不触发行选中', async () => {
    const w = mountRow({ renameLane: vi.fn().mockResolvedValue(true) })
    await openRename()

    const input = renameInput(w)
    expect(input.exists()).toBe(true)
    expect((input.element as HTMLInputElement).value).toBe(ROW.label)
    expect(w.emitted('selectLane')).toBeUndefined()
  })

  it('Enter 提交：调 renameLane(topicId, 新名)，成功 → 退出编辑', async () => {
    const renameLane = vi.fn().mockResolvedValue(true)
    const w = mountRow({ renameLane })
    await openRename()

    await renameInput(w).setValue('新名字')
    await renameInput(w).trigger('keydown.enter')
    await flushPromises()

    expect(renameLane).toHaveBeenCalledWith('10', '新名字')
    expect(renameInput(w).exists()).toBe(false)
  })

  it('改名失败（返回 false）→ 保留输入框，不伪装成功', async () => {
    const renameLane = vi.fn().mockResolvedValue(false)
    const w = mountRow({ renameLane })
    await openRename()

    await renameInput(w).setValue('新名字')
    await renameInput(w).trigger('keydown.enter')
    await flushPromises()

    expect(renameLane).toHaveBeenCalledTimes(1)
    expect(renameInput(w).exists()).toBe(true)
  })

  it('Esc 取消：恢复原名并退出编辑（不提交）', async () => {
    const renameLane = vi.fn().mockResolvedValue(true)
    const w = mountRow({ renameLane })
    await openRename()

    await renameInput(w).setValue('新名字')
    await renameInput(w).trigger('keydown.esc')
    await flushPromises()

    expect(renameLane).not.toHaveBeenCalled()
    expect(renameInput(w).exists()).toBe(false)
    expect(w.find('[data-testid="matrix-lane-10"]').text()).toContain(ROW.label)
  })

  it('失焦取消：退出编辑（不提交）', async () => {
    const renameLane = vi.fn().mockResolvedValue(true)
    const w = mountRow({ renameLane })
    await openRename()

    await renameInput(w).setValue('新名字')
    await renameInput(w).trigger('blur')
    await flushPromises()

    expect(renameLane).not.toHaveBeenCalled()
    expect(renameInput(w).exists()).toBe(false)
  })

  it('空名/纯空白：提示「泳道名不能为空」并保持编辑态（不提交）', async () => {
    const renameLane = vi.fn().mockResolvedValue(true)
    const w = mountRow({ renameLane })
    await openRename()

    await renameInput(w).setValue('   ')
    await renameInput(w).trigger('keydown.enter')
    await flushPromises()

    expect(renameLane).not.toHaveBeenCalled()
    expect(renameInput(w).exists()).toBe(true)
    expect(notifyWarn).toHaveBeenCalledWith('泳道名不能为空')
  })
})

/**
 * 覆盖行（`lane-coverage-entry-list`）：矩阵底部列表项复用本组件的 `coverage` 分支，
 * 传 `cells=[]` 时只剩行头——名字静态 + 「最近命中 MM-DD」/「无记录」+ ⋯，无任何日期格。
 * 「最近命中」是话题最近一次被线索命中的日期，不是进日报的日期（不得写回「最近一次」）。
 */
describe('ObservationMatrixLaneRow — 覆盖行（无格形态）', () => {
  function mountCoverage(overrides: Record<string, unknown> = {}) {
    return mountRow({ cells: [], axis: [], coverage: true, ...overrides })
  }

  it('cells=[] → 不渲染任何日期格', () => {
    const w = mountCoverage({ coverageLastSeen: '2026-11-09' })
    expect(w.findAll('.omx-cell')).toHaveLength(0)
    expect(w.find('[data-testid^="matrix-cell-"]').exists()).toBe(false)
  })

  it('行头写「最近命中 MM-DD」，全文（含窗口内未进日报）在 aria-label', () => {
    const w = mountCoverage({ coverageLastSeen: '2026-11-09' })
    const name = w.find('[data-testid="matrix-lane-10"]')
    expect(name.element.tagName).toBe('SPAN')
    expect(name.text()).toContain(ROW.label)
    expect(name.text()).toContain('最近命中 11-09')
    expect(name.text()).not.toContain('最近一次')
    expect(name.attributes('aria-label')).toContain('窗口内未进日报 · 最近命中 11-09')
  })

  it('缺日期 → 写「无记录」（不拿今天冒充）', () => {
    const w = mountCoverage({ coverageLastSeen: null })
    expect(w.find('[data-testid="matrix-lane-10"]').text()).toContain('无记录')
  })

  it('名字是静态标签（点不动）；⋯ 仍提供重命名 / 编辑 / 归档', async () => {
    const w = mountCoverage({ coverageLastSeen: '2026-11-09', renameLane: vi.fn().mockResolvedValue(true) })
    ;(w.find('[data-testid="matrix-lane-10"]').element as HTMLElement).click()
    await flushPromises()
    expect(w.emitted('selectLane')).toBeUndefined()

    trigger().click()
    await flushPromises()
    const items = [...document.body.querySelectorAll('.row-menu-item')].map(el => el.textContent?.trim())
    expect(items).toEqual(['重命名', '编辑泳道', '归档泳道'])
  })
})

/**
 * 名称看全卡片（2026-11-26 用户反馈「泳道名展示不全」）：标签列放不下全名时，星停/聚焦出卡；
 * 名字放得下时零噪声；卡片只是视觉冗余（全名已在 `aria-label`，卡片 `aria-hidden`）。
 */
describe('ObservationMatrixLaneRow — 名称看全', () => {
  function laneNameEl(): HTMLElement {
    return document.body.querySelector('.omx-lane-name') as HTMLElement
  }

  function stubWidth(el: HTMLElement, scrollWidth: number, clientWidth: number): void {
    Object.defineProperty(el, 'scrollWidth', { value: scrollWidth, configurable: true })
    Object.defineProperty(el, 'clientWidth', { value: clientWidth, configurable: true })
  }

  it('不再挂原生 title：全名只在 aria-label 与卡片里', () => {
    mountRow()
    const btn = document.body.querySelector('[data-testid="matrix-lane-10"]') as HTMLElement
    expect(btn.getAttribute('title')).toBeNull()
    expect(btn.getAttribute('aria-label')).toContain(ROW.label)
  })

  it('悬停被截断的名 → 出卡（全名 + 本行指标）；放大字号后键盘聚焦同权', async () => {
    vi.useFakeTimers()
    mountRow()
    const btn = document.body.querySelector('[data-testid="matrix-lane-10"]') as HTMLButtonElement
    stubWidth(laneNameEl(), 300, 120)

    btn.dispatchEvent(new Event('mouseenter'))
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    const card = document.body.querySelector('[data-testid="matrix-lane-name-card"]')
    expect(card).not.toBeNull()
    expect(card!.textContent).toContain(ROW.label)
    expect(card!.textContent).toContain(`${ROW.clueTotal} 条线索`)
    expect(card!.textContent).toContain(`${ROW.sectionTotal} 个主题桶`)
    expect(card!.getAttribute('aria-hidden')).toBe('true')

    btn.dispatchEvent(new Event('focus'))
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(document.body.querySelector('[data-testid="matrix-lane-name-card"]')).not.toBeNull()

    btn.dispatchEvent(new Event('blur'))
    await flushPromises()
    expect(document.body.querySelector('[data-testid="matrix-lane-name-card"]')).toBeNull()
    vi.useRealTimers()
  })

  it('名字放得下（无截断）→ 不出卡', async () => {
    vi.useFakeTimers()
    mountRow()
    const btn = document.body.querySelector('[data-testid="matrix-lane-10"]') as HTMLButtonElement
    stubWidth(laneNameEl(), 120, 120)

    btn.dispatchEvent(new Event('mouseenter'))
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(document.body.querySelector('[data-testid="matrix-lane-name-card"]')).toBeNull()
    vi.useRealTimers()
  })
})
