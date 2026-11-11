/**
 * BoardLaneEvolution 测试（rework-workshop-into-radar-and-desk task 2.4）。
 *
 * 覆盖 test-cases.md E-1..E-7：
 *  - 节点大小按线索数（0 空心 / 1-2 mid / ≥3 big）与「≥3 条线索才显示标签」
 *  - 今天事件 / 无今天数据不伪造
 *  - 最密集三天（并列按日期降序）
 *  - 态势句：as_of 标注、长句折叠展开、无快照待结算占位
 *  - 点击节点 emit openCell；空格只提示
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import BoardLaneEvolution from './BoardLaneEvolution.vue'
import { buildDayAxis, buildLaneRow } from '~/features/tags/composables/useBoardConsole'
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

function makeLane(overrides: Partial<LaneDynamicsLane> = {}): LaneDynamicsLane {
  return {
    topic_id: 10,
    label: '主线 A',
    watch_linked: false,
    section_count_14d: 0,
    snapshot: { summary: '这条线最近在降温。', as_of: '2026-10-22' },
    timeline: [
      {
        date: '2026-10-18',
        sections: [
          { section_id: 1, label: '分区一', events: ['事件一'] },
          { section_id: 2, label: '分区二', events: ['事件二'] },
          { section_id: 3, label: '分区三', events: ['事件三'] },
        ],
      },
      {
        date: '2026-10-19',
        sections: [
          { section_id: 4, label: '分区四', events: ['事件四'] },
          { section_id: 5, label: '分区五', events: ['事件五'] },
          { section_id: 6, label: '分区六', events: ['事件六'] },
        ],
      },
      {
        date: '2026-10-20',
        sections: [
          { section_id: 7, label: '分区七', events: ['事件七'] },
          { section_id: 8, label: '分区八', events: ['事件八'] },
        ],
      },
      {
        date: TODAY,
        sections: [
          { section_id: 9, label: '今日分区一', events: ['今天事件一', '今天事件二'] },
          { section_id: 10, label: '今日分区二', events: ['今天事件三'] },
          { section_id: 11, label: '今日分区三', events: ['今天事件四'] },
        ],
      },
    ],
    ...overrides,
  }
}

function mountEvolution(rowOverrides: Partial<LaneDynamicsLane> = {}, expanded = false) {
  const row = buildLaneRow(makeLane(rowOverrides), AXIS)
  const wrapper = mount(BoardLaneEvolution, {
    props: { axis: AXIS, row, today: TODAY, windowDays: 14 },
  })
  if (expanded) {
    wrapper.find('[data-testid="lane-evolution-summary-toggle"]').trigger('click')
  }
  return wrapper
}

describe('BoardLaneEvolution — 节点与事件', () => {
  beforeEach(() => notifyWarn.mockReset())

  it('节点大小分档：0 空心 / 2 mid / 3 线索 big；只有 ≥3 条线索显示事件标签（E-1/E-2）', () => {
    const wrapper = mountEvolution()
    expect(wrapper.find(`[data-testid="evolution-node-${TODAY}"]`).classes()).toContain('is-big')
    expect(wrapper.find('[data-testid="evolution-node-2026-10-20"]').classes()).toContain('is-mid')
    expect(wrapper.find('[data-testid="evolution-node-2026-10-21"]').classes()).toContain('is-hollow')
    expect(wrapper.find(`[data-testid="evolution-node-${TODAY}"]`).find('.ev-node-label').exists()).toBe(true)
    expect(wrapper.find('[data-testid="evolution-node-2026-10-20"]').find('.ev-node-label').exists()).toBe(false)
  })

  it('节点大小按当日事件线数而非线索/section 数（story 口径，5.4）', () => {
    // thread 合计 6 但 story_count=2 → mid（非 big）；story_count=4 → big
    const wrapper = mountEvolution({
      timeline: [
        {
          date: TODAY,
          story_count: 2,
          max_participant_count: 5,
          sections: [{ section_id: 1, label: '一个大桶', events: [], thread_count: 6 }],
        },
        {
          date: '2026-10-22',
          story_count: 4,
          sections: [{ section_id: 2, label: '另一桶', events: [], thread_count: 1 }],
        },
      ],
    })
    expect(wrapper.find(`[data-testid="evolution-node-${TODAY}"]`).classes()).toContain('is-mid')
    expect(wrapper.find(`[data-testid="evolution-node-${TODAY}"]`).attributes('aria-label')).toContain('2 条事件线')
    expect(wrapper.find(`[data-testid="evolution-node-${TODAY}"]`).attributes('title')).toContain('最热 5 个来源')
    expect(wrapper.find(`[data-testid="evolution-node-${TODAY}"]`).find('.ev-node-count').text()).toBe('2')
    expect(wrapper.find('[data-testid="evolution-node-2026-10-22"]').classes()).toContain('is-big')
    expect(wrapper.find('[data-testid="evolution-node-2026-10-22"]').attributes('aria-label')).toContain('4 条事件线')
  })

  it('今天事件：列出今日全部（截 4 条）事件；无今天数据提示不伪造（E-4）', () => {
    const wrapper = mountEvolution()
    const todayEvents = wrapper.find('[data-testid="lane-evolution-today-events"]')
    expect(todayEvents.text()).toContain('今天事件一')
    expect(todayEvents.text()).toContain('今天事件四')

    const noToday = mountEvolution({
      timeline: [
        { date: '2026-10-20', sections: [{ section_id: 1, label: '旧分区', events: ['旧事件'] }] },
      ],
    })
    expect(noToday.find('[data-testid="lane-evolution-no-today"]').exists()).toBe(true)
  })

  it('最密集三天：计数降序，并列取更晚日期（E-3）', () => {
    const wrapper = mountEvolution()
    const peaks = wrapper.find('[data-testid="lane-evolution-peaks"]')
    const dates = peaks.findAll('.ev-peak-date').map((node) => node.text())
    expect(dates).toEqual(['10-23', '10-19', '10-18'])
  })

  it('加载态：骨架占位（不显示空态文案）', () => {
    const wrapper = mount(BoardLaneEvolution, {
      props: { axis: AXIS, row: null, today: TODAY, windowDays: 14, loading: true, error: null },
    })
    expect(wrapper.find('[data-testid="lane-evolution-loading"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-evolution-empty"]').exists()).toBe(false)
  })

  it('错误态：局部错误 + 重试（retry 事件）', async () => {
    const wrapper = mount(BoardLaneEvolution, {
      props: { axis: AXIS, row: null, today: TODAY, windowDays: 14, loading: false, error: '网络错误' },
      propsData: undefined,
    })
    expect(wrapper.find('[data-testid="lane-evolution-error"]').text()).toContain('网络错误')
    await wrapper.find('[data-testid="lane-evolution-retry"]').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })

  it('保留旧数据：staleNotice 提示带上次数据', () => {
    const row = buildLaneRow(makeLane(), AXIS)
    const wrapper = mount(BoardLaneEvolution, {
      props: { axis: AXIS, row, today: TODAY, windowDays: 14, loading: false, error: '超时', staleNotice: true },
    })
    expect(wrapper.find('[data-testid="lane-evolution-stale"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-evolution-ribbon"]').exists()).toBe(true)
  })

  it('点击节点 emit openCell；空格只提示不 emit（E-1 交互）', async () => {
    const wrapper = mountEvolution()
    await wrapper.find(`[data-testid="evolution-node-${TODAY}"]`).trigger('click')
    expect(wrapper.emitted('openCell')![0]).toEqual([{ topicId: 10, date: TODAY }])

    await wrapper.find('[data-testid="evolution-node-2026-10-21"]').trigger('click')
    expect(wrapper.emitted('openCell')).toHaveLength(1)
    expect(notifyWarn).toHaveBeenCalled()
  })
})

describe('BoardLaneEvolution — 态势句', () => {
  it('有快照：显示 summary + as_of；无快照：待结算占位且时间线照常（E-5）', () => {
    const wrapper = mountEvolution()
    expect(wrapper.find('[data-testid="lane-evolution-asof"]').text()).toContain('2026-10-22')
    expect(wrapper.text()).toContain('这条线最近在降温')

    const pending = mountEvolution({ snapshot: null })
    expect(pending.find('[data-testid="lane-evolution-pending"]').exists()).toBe(true)
    expect(pending.find('[data-testid="lane-evolution-ribbon"]').exists()).toBe(true)
  })

  it('长句默认折叠，展开后显示 detail 全文（E-6）', async () => {
    const longSummary = '这是一段超过八十个字符的态势句，用来验证折叠入口是否存在并且能够在展开后展示完整内容。'.repeat(2)
    const wrapper = mountEvolution({
      snapshot: { summary: longSummary, detail: `${longSummary} 这是长版叙述全文。`, as_of: '2026-10-22' },
    })
    const text = wrapper.find('.ev-stance-text')
    expect(text.classes()).toContain('is-clamped')
    await wrapper.find('[data-testid="lane-evolution-summary-toggle"]').trigger('click')
    expect(wrapper.find('.ev-stance-text').classes()).not.toContain('is-clamped')
    expect(wrapper.find('.ev-stance-text').text()).toContain('长版叙述全文')
  })

  it('无选中泳道：空提示（E 组合边界）', () => {
    const wrapper = mount(BoardLaneEvolution, {
      props: { axis: AXIS, row: null, today: TODAY, windowDays: 14 },
    })
    expect(wrapper.find('[data-testid="lane-evolution-empty"]').exists()).toBe(true)
  })
})
