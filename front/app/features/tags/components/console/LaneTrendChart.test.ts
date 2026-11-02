/**
 * LaneTrendChart 组件测试（矩阵阶段 G2，test-cases-matrix-stage.md EV-6 + 零值语义）。
 *
 *  - 轴：Y = max/⌊max/2⌋/0 真实比例；X = 首/中/末（小跨度全标）。
 *  - series：null 不补 0 不桥接（分段折线）；已知 0 画基线点且可点选；单点段只画点。
 *  - 受控选择：selectedDate 高亮；不在序列内不高亮；点已知日 emit select-date、未知日不发。
 *  - 状态：loading / 空序列 / 全未知（不伪造曲线与量级）。
 */
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import LaneTrendChart from './LaneTrendChart.vue'
import type { LaneTrendPoint } from './laneTrendSeries'

function seriesOf(values: (number | null)[], start = '2026-10-01'): LaneTrendPoint[] {
  // 日期连续构造：跨月自然推进（2026-10-31 后到 11 月）
  const dates: string[] = []
  let cursor = new Date(Date.parse(`${start}T00:00:00Z`))
  for (let i = 0; i < values.length; i++) {
    dates.push(cursor.toISOString().slice(0, 10))
    cursor = new Date(cursor.getTime() + 86_400_000)
  }
  return values.map((value, index) => ({ date: dates[index] ?? '', value }))
}

const GEOMETRY = { viewWidth: 480, viewHeight: 74, baselineY: 64, topY: 8 }

function mountChart(props: Partial<InstanceType<typeof LaneTrendChart>['$props']> = {}) {
  return mount(LaneTrendChart, {
    props: { series: seriesOf([3, 5, 1]), ...props },
  })
}

describe('LaneTrendChart — 轴与刻度（EV-6）', () => {
  it('Y 刻度 = max/⌊max/2⌋/0；X = 首/中/末日期（>5 抽样）', () => {
    const wrapper = mountChart({ series: seriesOf([0, 5, 1, 9, 2, 4]) })
    const yTexts = wrapper.findAll('[data-testid="trend-axis-y"] span').map(span => span.text())
    expect(yTexts).toEqual(['9', '4', '0'])
    const xTexts = wrapper.findAll('[data-testid="trend-axis-x"] span').map(span => span.text())
    expect(xTexts).toEqual(['2026-10-01', '2026-10-03', '2026-10-06'])
  })

  it('小跨度（≤5）逐日全标；max ≤ 1 → [max, 0]', () => {
    const wrapper = mountChart({ series: seriesOf([1, 0]) })
    expect(wrapper.findAll('[data-testid="trend-axis-x"] span')).toHaveLength(2)
    const yTexts = wrapper.findAll('[data-testid="trend-axis-y"] span').map(span => span.text())
    expect(yTexts).toEqual(['1', '0'])
  })

  it('点坐标真实比例：满值到顶、0 落基线', () => {
    const wrapper = mountChart({ series: seriesOf([8, 0]) })
    const dots = wrapper.findAll('.ltc-dot')
    expect(Number(dots[0]!.attributes('cy'))).toBeCloseTo(GEOMETRY.topY, 5)
    expect(Number(dots[1]!.attributes('cy'))).toBeCloseTo(GEOMETRY.baselineY, 5)
  })
})

describe('LaneTrendChart — series 零值语义', () => {
  it('null 不补 0 不桥接：未知日无点，折线按已知段分段', () => {
    const wrapper = mountChart({ series: seriesOf([3, null, 5, 2, null, 4]) })
    expect(wrapper.findAll('.ltc-dot')).toHaveLength(4) // 未知两日无点
    // 已知段 [3] [5,2] [4] → 仅 [5,2] ≥2 点画线
    expect(wrapper.findAll('.ltc-line')).toHaveLength(1)
    expect(wrapper.find('[data-testid="lane-trend-plot"]').attributes('aria-label')).toContain('2 日未知不补 0')
  })

  it('全未知：不画线、无 Y 刻度，明示未知不补 0', () => {
    const wrapper = mountChart({ series: seriesOf([null, null]) })
    expect(wrapper.find('[data-testid="lane-trend-unknown"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-trend-plot"]').exists()).toBe(false)
  })

  it('单点段只画点不画线', () => {
    const wrapper = mountChart({ series: seriesOf([4]) })
    expect(wrapper.findAll('.ltc-dot')).toHaveLength(1)
    expect(wrapper.findAll('.ltc-line')).toHaveLength(0)
  })
})

describe('LaneTrendChart — 受控选择与点选', () => {
  it('点已知日（含 0）emit select-date；未知日无点不可点', async () => {
    const wrapper = mountChart({ series: seriesOf([3, null, 0]) })
    const dots = wrapper.findAll('.ltc-dot') // 未知日无点：仅 10-01（3 条）与 10-03（已知 0）
    expect(dots).toHaveLength(2)
    await dots[0]!.trigger('click')
    await dots[1]!.trigger('click') // 已知 0 是合法结论，可点选
    expect(wrapper.emitted('select-date')).toEqual([['2026-10-01'], ['2026-10-03']])
  })

  it('selectedDate 高亮选中点与游标线；不在序列内不高亮', () => {
    const wrapper = mountChart({ series: seriesOf([3, 5]), selectedDate: '2026-10-02' })
    const dots = wrapper.findAll('.ltc-dot')
    expect(dots[1]!.classes()).toContain('is-selected')
    expect(wrapper.find('.ltc-cursor').exists()).toBe(true)

    const absent = mountChart({ series: seriesOf([3, 5]), selectedDate: '2025-01-01' })
    expect(absent.find('.ltc-cursor').exists()).toBe(false)
  })

  it('点带 title 读日期/实际数；不进 Tab 序（无 tabindex）', () => {
    const wrapper = mountChart({ series: seriesOf([7]) })
    expect(wrapper.find('.ltc-dot').element.textContent).toContain('2026-10-01 · 当日合计 7 条')
    expect(wrapper.find('.ltc-dot').attributes('tabindex')).toBeUndefined()
  })
})

describe('LaneTrendChart — 状态', () => {
  it('loading：态文案优先，无绘图区', () => {
    const wrapper = mountChart({ loading: true })
    expect(wrapper.find('[data-testid="lane-trend-loading"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="lane-trend-plot"]').exists()).toBe(false)
  })

  it('空序列：空态文案（可自定义）', () => {
    const wrapper = mountChart({ series: [], emptyText: '该泳道在此区间无逐日数据。' })
    expect(wrapper.find('[data-testid="lane-trend-empty"]').text()).toBe('该泳道在此区间无逐日数据。')
  })

  it('label 由宿主给出（口径说明）', () => {
    const wrapper = mountChart({ label: '所选泳道 · 所示日期内日合计（非矩阵桶峰值）' })
    expect(wrapper.find('.ltc-label').text()).toContain('非矩阵桶峰值')
  })
})
