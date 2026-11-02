/**
 * BoardMatrixView 测试（rework-workshop-into-radar-and-desk tasks 2.1/2.2/2.5）。
 *
 * 覆盖 test-cases.md：
 *  - M-1..M-9：轴列/格值/断档/合计/今天列/密度分档（结构性断言，不断颜色）
 *  - MD-4/MD-5：休眠默认折叠、展开渲染弱化行
 *  - 2.5：骨架/错误重试/保留旧数据提示/空态两分支
 *  - X-1（矩阵侧）：点格只 emit，不写数据
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import BoardMatrixView from './BoardMatrixView.vue'
import { buildDayAxis, buildLaneRows, splitDormantRows } from '~/features/tags/composables/useBoardConsole'
import type { LaneDynamicsLane } from '~/api/laneDynamics'

const notifyWarn = vi.fn()
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ warn: notifyWarn, success: vi.fn(), error: vi.fn() }),
}))
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))

const TODAY = '2026-10-23'
const AXIS = buildDayAxis(TODAY, 14)

function lane(topicId: number, label: string, days: Record<string, number>): LaneDynamicsLane {
  return {
    topic_id: topicId,
    label,
    watch_linked: false,
    section_count_14d: 0,
    snapshot: null,
    timeline: Object.entries(days).map(([date, count]) => ({
      date,
      sections: Array.from({ length: count }, (_, i) => ({
        section_id: topicId * 100 + i,
        label: `${label} ${date} 分区${i}`,
        events: [`${label} 事件${i}`],
      })),
    })),
  }
}

function rowsFixture() {
  const rows = buildLaneRows([
    lane(10, '主线 A', { '2026-10-21': 1, '2026-10-23': 4 }),
    lane(11, '主线 B', { '2026-10-22': 3 }),
    lane(12, '休眠线 C', { '2026-10-20': 2 }),
  ], AXIS)
  return { rows, ...splitDormantRows(rows) }
}

function mountMatrix(overrides: Record<string, unknown> = {}) {
  const { active, dormant } = rowsFixture()
  return mount(BoardMatrixView, {
    props: {
      boardId: 1974,
      axis: AXIS,
      activeRows: active,
      dormantRows: dormant,
      selectedLaneId: 10,
      today: TODAY,
      windowDays: 14,
      expandedDormant: false,
      loading: false,
      error: null,
      staleNotice: false,
      emptyState: null,
      ...overrides,
    },
  })
}

describe('BoardMatrixView — 矩阵派生', () => {
  beforeEach(() => notifyWarn.mockReset())

  it('渲染色道 × 日期网格：列数 = 轴长，行 = 非休眠泳道，行尾合计（M-1/M-6）', () => {
    const wrapper = mountMatrix()
    expect(wrapper.findAll('.mx-dayhead')).toHaveLength(14)
    expect(wrapper.find('[data-testid="matrix-lane-10"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="matrix-lane-11"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="matrix-lane-12"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="matrix-total-10"]').text()).toBe('5')
    expect(wrapper.find('[data-testid="matrix-total-11"]').text()).toBe('3')
  })

  it('格值 = 当日 section 数；空格不渲染 0 且带 is-empty（M-4/M-5）', () => {
    const wrapper = mountMatrix()
    expect(wrapper.find(`[data-testid="matrix-cell-10-${TODAY}"]`).text()).toBe('4')
    const gap = wrapper.find('[data-testid="matrix-cell-10-2026-10-22"]')
    expect(gap.text()).toBe('')
    expect(gap.classes()).toContain('is-empty')
  })

  it('今天列高亮（M-3）', () => {
    const wrapper = mountMatrix()
    expect(wrapper.find(`[data-testid="matrix-cell-10-${TODAY}"]`).classes()).toContain('is-today')
    expect(wrapper.find('[data-testid="matrix-cell-10-2026-10-22"]').classes()).not.toContain('is-today')
  })

  it('密度分档：1/2/3/≥4（M-7）', () => {
    const rows = buildLaneRows([
      lane(20, '档', { '2026-10-20': 1, '2026-10-21': 2, '2026-10-22': 3, '2026-10-23': 5 }),
    ], AXIS)
    const wrapper = mountMatrix({ activeRows: rows, dormantRows: [], selectedLaneId: 20 })
    expect(wrapper.find('[data-testid="matrix-cell-20-2026-10-20"]').classes()).toContain('is-b1')
    expect(wrapper.find('[data-testid="matrix-cell-20-2026-10-21"]').classes()).toContain('is-b2')
    expect(wrapper.find('[data-testid="matrix-cell-20-2026-10-22"]').classes()).toContain('is-b3')
    expect(wrapper.find('[data-testid="matrix-cell-20-2026-10-23"]').classes()).toContain('is-b4')
  })

  it('轴外数据不计入格值与合计（M-9）', () => {
    const rows = buildLaneRows([lane(30, '轴外', { '2026-09-01': 9, '2026-10-23': 1 })], AXIS)
    const wrapper = mountMatrix({ activeRows: rows, dormantRows: [], selectedLaneId: 30 })
    expect(wrapper.find('[data-testid="matrix-total-30"]').text()).toBe('1')
    expect(wrapper.findAll('[data-testid^="matrix-cell-30-"]')).toHaveLength(14)
  })
})

describe('BoardMatrixView — 休眠与交互', () => {
  beforeEach(() => notifyWarn.mockReset())

  it('休眠默认折叠：渲染「休眠泳道 N」条，展开后以弱化行出现（MD-4/MD-5）', async () => {
    const wrapper = mountMatrix()
    const toggle = wrapper.find('[data-testid="dormant-toggle"]')
    expect(toggle.text()).toContain('休眠泳道')
    expect(toggle.text()).toContain('1')
    expect(wrapper.find('[data-testid="matrix-lane-12"]').exists()).toBe(false)
    await wrapper.setProps({ expandedDormant: true })
    expect(wrapper.find('[data-testid="matrix-lane-12"]').classes()).toContain('is-dormant')
  })

  it('点折叠条 emit toggleDormant（不自己改状态）', async () => {
    const wrapper = mountMatrix()
    await wrapper.find('.mx-dormant-btn').trigger('click')
    expect(wrapper.emitted('toggleDormant')).toHaveLength(1)
  })

  it('点命中格 emit openCell；点空格只提示、不 emit（D-1/D-2）', async () => {
    const wrapper = mountMatrix()
    await wrapper.find(`[data-testid="matrix-cell-10-${TODAY}"]`).trigger('click')
    expect(wrapper.emitted('openCell')![0]).toEqual([{ topicId: 10, date: TODAY }])

    await wrapper.find('[data-testid="matrix-cell-10-2026-10-22"]').trigger('click')
    expect(wrapper.emitted('openCell')).toHaveLength(1)
    expect(notifyWarn).toHaveBeenCalledWith(expect.stringContaining('没有分区'))
  })

  it('点泳道名 emit selectLane（7）', async () => {
    const wrapper = mountMatrix()
    await wrapper.find('[data-testid="matrix-lane-11"]').trigger('click')
    expect(wrapper.emitted('selectLane')![0]).toEqual([11])
  })
})

describe('BoardMatrixView — 状态矩阵（2.5）', () => {
  it('loading：骨架占位，不渲染网格', () => {
    const wrapper = mountMatrix({ loading: true, activeRows: [], dormantRows: [] })
    expect(wrapper.find('[data-testid="matrix-skeleton"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="matrix-grid"]').exists()).toBe(false)
  })

  it('error（无旧数据）：错误文案 + 重试 emit', async () => {
    const wrapper = mountMatrix({ loading: false, error: '网络错误', activeRows: [], dormantRows: [] })
    expect(wrapper.find('[data-testid="matrix-error"]').text()).toContain('网络错误')
    await wrapper.find('[data-testid="matrix-retry"]').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })

  it('行尾用线索总数、title 保留分区合计；排序键可见（热度口径）', () => {
    const rows = buildLaneRows([
      { ...lane(1, '分桶多', { '2026-10-23': 3 }), timeline: [{ date: '2026-10-23', sections: [
        { section_id: 1, label: 'a', events: [], thread_count: 1 },
        { section_id: 2, label: 'b', events: [], thread_count: 0 },
        { section_id: 3, label: 'c', events: [], thread_count: 0 },
      ] }] },
      { ...lane(2, '线索多', { '2026-10-23': 1 }), timeline: [{ date: '2026-10-23', sections: [
        { section_id: 4, label: 'd', events: [], thread_count: 8 },
      ] }] },
    ], AXIS)
    const wrapper = mountMatrix({ activeRows: rows, dormantRows: [], selectedLaneId: 2 })
    const total = wrapper.find('[data-testid="matrix-total-2"]')
    expect(total.text()).toBe('8')
    expect(total.attributes('title')).toContain('1 个分区')
    expect(wrapper.find('[data-testid="matrix-total-1"]').text()).toBe('1')
  })

  it('刷新失败但保留旧数据：提示条 + 旧网格仍在（2.5）', () => {
    const wrapper = mountMatrix({ error: '超时', staleNotice: true })
    expect(wrapper.find('[data-testid="matrix-stale-notice"]').text()).toContain('超时')
    expect(wrapper.find('[data-testid="matrix-grid"]').exists()).toBe(true)
  })

  it('空态：无日报 / 有日报无泳道（2.5）', async () => {
    const noReports = mountMatrix({ emptyState: 'no-reports', activeRows: [], dormantRows: [] })
    expect(noReports.find('[data-testid="matrix-empty-no-reports"]').exists()).toBe(true)
    expect(noReports.text()).toContain('还没有日报')
    // 空态不是死胡同：生成入口回旧版工坊（阶段④由阅读台接管）
    expect(noReports.find('[data-testid="matrix-empty-generate"]').exists()).toBe(true)

    const noLanes = mountMatrix({ emptyState: 'no-lanes', activeRows: [], dormantRows: [] })
    expect(noLanes.find('[data-testid="matrix-empty-no-lanes"]').exists()).toBe(true)
    await noLanes.setProps({ emptyState: null })
  })
})
