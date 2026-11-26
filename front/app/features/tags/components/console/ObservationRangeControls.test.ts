/**
 * ObservationRangeControls 组件测试（矩阵阶段 G2，test-cases-matrix-stage.md
 * S9 步 11/13/14 + OX-3/4/5/6 的组件层落点）。
 *
 *  - 表单：合法提交 / 四类非法不误提交（文案可见、commit 不发）。
 *  - 刷选：preview→commit 序列（mock rect/PointerEvent 组件层）；反向拖合法；选区平移；
 *    双端手柄；池边界（0..364 含端点）；同值松手恰一次 commit。
 *  - 取消：Esc / pointercancel 恢复起始预览且不提交。
 *  - 键盘：端点 ±1 / Shift±7 / Home/End（aria 随受控 range 回流更新）。
 *  - 档位：只有独立档按钮 emit source-change；拖成档宽也不发（OX-1 组件面）。
 *  - series：null 不桥接不补 0（分段折线）、Y 量级刻度、X 首/中/末日期、空/加载态。
 * mock 说明：getBoundingClientRect 打桩为 left=0/width=400（组件层坐标映射），非生产 e2e。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import ObservationRangeControls from './ObservationRangeControls.vue'
import { shiftRange, shiftDate, zoomRange } from './observationRange'
import { indexToX, valueToY } from './laneTrendSeries'

/** 样本池：锚点 2026-10-23，池 = [2025-10-24, 2026-10-23]（365 日含两端，index 0..364）。 */
const BOUNDS = { start: '2025-10-24', end: '2026-10-23' }
const N = 365
const RANGE = { start: '2026-10-10', end: '2026-10-23' } // 14 日档默认（index 351..364）
const RECT = { left: 0, top: 0, width: 400, height: 88, right: 400, bottom: 88, x: 0, y: 0 }
// 组件 onMounted/nextTick 读取真实 SVG 宽度（命中区按像素生成）；happy-dom 无布局，统一打桩。
SVGSVGElement.prototype.getBoundingClientRect = () => RECT as DOMRect

function dateAt(index: number): string {
  return shiftDate(BOUNDS.start, index)
}

function clientXAt(index: number): number {
  return (index / (N - 1)) * RECT.width
}

function mountControls(props: Partial<InstanceType<typeof ObservationRangeControls>['$props']> = {}) {
  const wrapper = mount(ObservationRangeControls, {
    props: {
      bounds: BOUNDS,
      range: RANGE,
      sourceDays: 14,
      series: null,
      ...props,
    },
  })
  const plot = wrapper.find('[data-testid="observation-range-plot"]')
  if (plot.exists()) (plot.element as SVGSVGElement).getBoundingClientRect = () => RECT as DOMRect
  return { wrapper, plot }
}

async function drag(wrapper: ReturnType<typeof mountControls>['wrapper'], indexes: number[], target: 'plot' | 'handle-start' | 'handle-end' = 'plot') {
  const testid = target === 'plot' ? 'observation-range-plot' : `brush-handle-${target.split('-')[1]}`
  const area = wrapper.find(`[data-testid="${testid}"]`)
  await area.trigger('pointerdown', { clientX: clientXAt(indexes[0]!), button: 0, pointerId: 1 })
  for (const index of indexes.slice(1)) {
    await wrapper.find('[data-testid="observation-range-plot"]').trigger('pointermove', { clientX: clientXAt(index) })
  }
}

describe('ObservationRangeControls — 表单（OX-3）', () => {
  it('合法日期：提交 emit commit（trim 后的严格日期）', async () => {
    const { wrapper } = mountControls()
    await wrapper.find('#orc-start').setValue('  2026-10-06 ')
    await wrapper.find('#orc-end').setValue('2026-10-20')
    await wrapper.find('form').trigger('submit')
    expect(wrapper.emitted('commit')).toEqual([[{ start: '2026-10-06', end: '2026-10-20' }]])
  })

  it.each([
    ['2026/10/06', '2026-10-26', '日期须为 YYYY-MM-DD 格式'],
    ['2026-10-26', '2026-10-06', '起始日期不能晚于截止日期'],
    ['2024-01-01', '2026-10-06', '日期超出可取数范围'],
    ['', '2026-10-26', '请填写起止日期'],
  ])('非法输入 %s~%s：显示文案且不 emit commit', async (start, end, message) => {
    const { wrapper } = mountControls()
    await wrapper.find('#orc-start').setValue(start)
    await wrapper.find('#orc-end').setValue(end)
    await wrapper.find('form').trigger('submit')
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect(wrapper.find('[data-testid="observation-range-feedback"]').text()).toContain(message)
  })

  it('非法后改回合法：文案清除并可提交', async () => {
    const { wrapper } = mountControls()
    await wrapper.find('#orc-start').setValue('bad')
    await wrapper.find('form').trigger('submit')
    await wrapper.find('#orc-start').setValue('2026-10-06')
    await wrapper.find('#orc-end').setValue('2026-10-20')
    await wrapper.find('form').trigger('submit')
    expect(wrapper.emitted('commit')).toEqual([[{ start: '2026-10-06', end: '2026-10-20' }]])
    expect(wrapper.find('[data-testid="observation-range-feedback"]').text()).toBe('')
  })
})

describe('ObservationRangeControls — 刷选 preview→commit', () => {
  it('空白处拖选：preview 序列后恰一次 commit（含端点日期）', async () => {
    const { wrapper, plot } = mountControls()
    await drag(wrapper, [100, 110, 120])
    const previews = wrapper.emitted('preview')!
    expect(previews[0]).toEqual([{ start: dateAt(100), end: dateAt(100) }])
    expect(previews.at(-1)).toEqual([{ start: dateAt(100), end: dateAt(120) }])
    await plot.trigger('pointerup')
    expect(wrapper.emitted('commit')).toEqual([[{ start: dateAt(100), end: dateAt(120) }]])
  })

  it('反向拖仍合法区间：120 → 100 归一 [100,120]', async () => {
    const { wrapper, plot } = mountControls()
    await drag(wrapper, [120, 100])
    await plot.trigger('pointerup')
    expect(wrapper.emitted('commit')).toEqual([[{ start: dateAt(100), end: dateAt(120) }]])
  })

  it('选区平移：宽度不变整体位移并钳制池内', async () => {
    const { wrapper, plot } = mountControls()
    await drag(wrapper, [358, 350]) // 池内 pan：delta=-8
    await plot.trigger('pointerup')
    expect(wrapper.emitted('commit')).toEqual([[{ start: dateAt(343), end: dateAt(356) }]])
  })

  it('双端手柄：start 端拖动不越过 end 端', async () => {
    const { wrapper, plot } = mountControls()
    await drag(wrapper, [351, 364, 400], 'handle-start')
    await plot.trigger('pointerup')
    expect(wrapper.emitted('commit')).toEqual([[{ start: dateAt(364), end: dateAt(364) }]])
  })

  it('池边界：拖出左/右界钳制 index 0..364（含首尾端点）', async () => {
    const { wrapper, plot } = mountControls()
    await drag(wrapper, [5, -30])
    await plot.trigger('pointerup')
    expect(wrapper.emitted('commit')).toEqual([[{ start: dateAt(0), end: dateAt(5) }]])
    await drag(wrapper, [100, 999])
    await plot.trigger('pointerup')
    expect(wrapper.emitted('commit')!.at(-1)).toEqual([{ start: dateAt(100), end: dateAt(364) }])
  })

  it('同值松手：pan 手势无位移仍恰一次 commit（R-22 组件面）', async () => {
    const { wrapper, plot } = mountControls()
    await drag(wrapper, [358]) // 选区内 pan，不动
    await plot.trigger('pointerup')
    expect(wrapper.emitted('commit')).toHaveLength(1)
    expect(wrapper.emitted('commit')).toEqual([[{ start: RANGE.start, end: RANGE.end }]])
  })

  it('Esc：不提交错误范围并恢复起始预览', async () => {
    const { wrapper, plot } = mountControls()
    await drag(wrapper, [358, 100])
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await plot.trigger('pointerup') // Esc 后手势已结束，后续 up 不再 commit
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect(wrapper.emitted('preview')!.at(-1)).toEqual([{ start: RANGE.start, end: RANGE.end }])
  })

  it('pointercancel：恢复起始预览且不提交', async () => {
    const { wrapper, plot } = mountControls()
    await drag(wrapper, [358, 100])
    await plot.trigger('pointercancel')
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect(wrapper.emitted('preview')!.at(-1)).toEqual([{ start: RANGE.start, end: RANGE.end }])
  })
})

describe('ObservationRangeControls — 键盘（OX-6）', () => {
  it('端点 ±1 / Shift±7：每次按键从已提交 range 派生（受控），不越对端', async () => {
    const { wrapper } = mountControls()
    const handle = wrapper.find('[data-testid="brush-handle-start"]')
    await handle.trigger('keydown', { key: 'ArrowLeft' })
    expect(wrapper.emitted('commit')![0]).toEqual([{ start: '2026-10-09', end: '2026-10-23' }])
    // 受控：父回流后再次按键才从新起点累进
    await wrapper.setProps({ range: { start: '2026-10-09', end: '2026-10-23' } })
    await handle.trigger('keydown', { key: 'ArrowRight', shiftKey: true })
    expect(wrapper.emitted('commit')![1]).toEqual([{ start: '2026-10-16', end: '2026-10-23' }])
    // 连续 Shift+→ 不回流：每次仍从同一已提交 range 派生（受控幂等，不累积）
    for (let i = 0; i < 5; i++) await handle.trigger('keydown', { key: 'ArrowRight', shiftKey: true })
    expect(wrapper.emitted('commit')!.at(-1)).toEqual([{ start: '2026-10-16', end: '2026-10-23' }])
  })

  it('Home/End：送到池首/池尾；aria 随受控 range 回流更新', async () => {
    const { wrapper } = mountControls()
    const startHandle = wrapper.find('[data-testid="brush-handle-start"]')
    await startHandle.trigger('keydown', { key: 'Home' })
    expect(wrapper.emitted('commit')![0]).toEqual([{ start: BOUNDS.start, end: '2026-10-23' }])

    // 受控回流：父把 range 更新后 aria-valuetext/now 跟随（组件不自持第二份状态）
    await wrapper.setProps({ range: { start: BOUNDS.start, end: '2026-10-23' } })
    expect(startHandle.attributes('aria-valuetext')).toBe(BOUNDS.start)
    expect(startHandle.attributes('aria-valuenow')).toBe('0')

    const endHandle = wrapper.find('[data-testid="brush-handle-end"]')
    await endHandle.trigger('keydown', { key: 'End' })
    expect(wrapper.emitted('commit')!.at(-1)).toEqual([{ start: BOUNDS.start, end: BOUNDS.end }])
  })
})

describe('ObservationRangeControls — 首挂无数据，SVG 晚到（R37/R40 生产接线）', () => {
  it('初始 bounds/range=null 无 SVG；数据晚到后重新测宽并生成真实 hit 层', async () => {
    const { wrapper } = mountControls({ bounds: null, range: null })
    expect(wrapper.find('[data-testid="observation-range-plot"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="brush-hit-overlap"]').exists()).toBe(false)

    await wrapper.setProps({ bounds: BOUNDS, range: RANGE })
    await nextTick()
    await nextTick()
    const plot = wrapper.find('[data-testid="observation-range-plot"]')
    expect(plot.exists()).toBe(true)
    const hit = wrapper.find('[data-testid="brush-hit-overlap"]')
    expect(hit.exists()).toBe(true)
    const style = hit.attributes('style') ?? ''
    expect(style).toContain('width:')
    expect(style).not.toContain('width: 0px')
  })
})

describe('ObservationRangeControls — 手机手柄命中区（R37/R40）', () => {
  it('单日：合并区 24px，上/下半区分别抓 start/end；SVG 键盘手柄保留', async () => {
    const top = mountControls({ range: { start: dateAt(300), end: dateAt(300) } })
    await nextTick()
    const topHit = top.wrapper.find('[data-testid="brush-hit-overlap"]')
    expect(topHit.exists()).toBe(true)
    expect(topHit.attributes('style')).toContain('width: 24px')
    expect(top.wrapper.find('[data-testid="brush-handle-start"]').exists()).toBe(true)
    expect(top.wrapper.find('[data-testid="brush-handle-end"]').exists()).toBe(true)
    await topHit.trigger('pointerdown', { clientX: clientXAt(300), clientY: 10, button: 0, pointerId: 1 })
    await top.plot.trigger('pointermove', { clientX: clientXAt(296) })
    await top.plot.trigger('pointerup')
    expect(top.wrapper.emitted('commit')?.at(-1)?.[0]).toEqual({ start: dateAt(296), end: dateAt(300) })

    const bottom = mountControls({ range: { start: dateAt(300), end: dateAt(300) } })
    await nextTick()
    await bottom.wrapper.find('[data-testid="brush-hit-overlap"]')
      .trigger('pointerdown', { clientX: clientXAt(300), clientY: 70, button: 0, pointerId: 1 })
    await bottom.plot.trigger('pointermove', { clientX: clientXAt(303) })
    await bottom.plot.trigger('pointerup')
    expect(bottom.wrapper.emitted('commit')?.at(-1)?.[0]).toEqual({ start: dateAt(300), end: dateAt(303) })
  })

  it('7/14 日短窗：可见线中点解析各自端，不互相抢；外缘 1px 首动不跳', async () => {
    for (const span of [7, 14]) {
      const startIndex = 364 - span
      const { wrapper, plot } = mountControls({
        range: { start: dateAt(startIndex), end: dateAt(364) },
      })
      await nextTick()
      expect(wrapper.find('[data-testid="brush-hit-overlap"]').exists()).toBe(true)
      await wrapper.find('[data-testid="brush-hit-overlap"]').trigger('pointerdown', {
        clientX: clientXAt(startIndex), clientY: 10, button: 0, pointerId: 1,
      })
      await plot.trigger('pointermove', { clientX: clientXAt(startIndex - 1) })
      await plot.trigger('pointerup')
      expect(wrapper.emitted('commit')?.at(-1)?.[0]).toEqual({
        start: dateAt(startIndex - 1),
        end: dateAt(364),
      })
    }

    const startIndex = 357
    const { wrapper, plot } = mountControls({
      range: { start: dateAt(startIndex), end: dateAt(364) },
    })
    await nextTick()
    // 起点外缘 6px 在 24px 热区内按下，只移动 1px：首动仍只变化 ≤1 天
    await wrapper.find('[data-testid="brush-hit-overlap"]').trigger('pointerdown', {
      clientX: clientXAt(startIndex) - 6, clientY: 10, button: 0, pointerId: 1,
    })
    await plot.trigger('pointermove', { clientX: clientXAt(startIndex) - 5 })
    await plot.trigger('pointerup')
    const committed = wrapper.emitted('commit')?.at(-1)?.[0] as { start: string, end: string }
    expect([dateAt(startIndex - 1), dateAt(startIndex)]).toContain(committed.start)
    expect(committed.end).toBe(dateAt(364))
  })

  it('间隔≥24px：两端各自命中区；拖命中区仍按实际 plot 映射', async () => {
    const { wrapper, plot } = mountControls({
      range: { start: dateAt(100), end: dateAt(140) },
    })
    await nextTick()
    expect(wrapper.find('[data-testid="brush-hit-start"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="brush-hit-end"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="brush-hit-start"]').attributes('style')).toContain('width: 24px')

    await wrapper.find('[data-testid="brush-hit-start"]').trigger('pointerdown', {
      clientX: clientXAt(100), clientY: 40, button: 0, pointerId: 1,
    })
    await plot.trigger('pointermove', { clientX: clientXAt(90) })
    await plot.trigger('pointerup')
    expect(wrapper.emitted('commit')?.at(-1)?.[0]).toEqual({ start: dateAt(90), end: dateAt(140) })
  })

  it('池边界：起点 0 区域夹回 0px，仍可抓', async () => {
    const { wrapper, plot } = mountControls({
      range: { start: dateAt(0), end: dateAt(40) },
    })
    await nextTick()
    const startHit = wrapper.find('[data-testid="brush-hit-start"]')
    expect(startHit.attributes('style')).toContain('left: 0px')
    await startHit.trigger('pointerdown', { clientX: clientXAt(0), clientY: 40, button: 0, pointerId: 1 })
    await plot.trigger('pointermove', { clientX: clientXAt(1) })
    await plot.trigger('pointerup')
    expect(wrapper.emitted('commit')?.at(-1)?.[0]).toEqual({ start: dateAt(1), end: dateAt(40) })

    const tail = mountControls({ range: { start: dateAt(324), end: dateAt(364) } })
    await nextTick()
    const endHit = tail.wrapper.find('[data-testid="brush-hit-end"]')
    expect(endHit.attributes('style')).toContain('left: 376px')
    await endHit.trigger('pointerdown', { clientX: clientXAt(364), clientY: 40, button: 0, pointerId: 1 })
    await tail.plot.trigger('pointermove', { clientX: clientXAt(363) })
    await tail.plot.trigger('pointerup')
    expect(tail.wrapper.emitted('commit')?.at(-1)?.[0]).toEqual({ start: dateAt(324), end: dateAt(363) })
  })
})

describe('ObservationRangeControls — 档位（OX-1 组件面）', () => {
  it('只有独立档按钮 emit source-change（带档值），激活态随 sourceDays', async () => {
    const { wrapper } = mountControls()
    const buttons = wrapper.findAll('.orc-tier')
    expect(buttons.map(button => button.text())).toEqual(['7 天', '14 天', '30 天', '近 1 年'])
    expect(buttons[1]!.attributes('aria-pressed')).toBe('true')
    await buttons[0]!.trigger('click')
    expect(wrapper.emitted('source-change')).toEqual([[7]])
  })

  it('拖成恰 7/30/365 日宽也不 emit source-change（日期动作不切档）', async () => {
    const { wrapper, plot } = mountControls()
    await drag(wrapper, [0, 6]) // 恰 7 日宽
    await plot.trigger('pointerup')
    await drag(wrapper, [0, 29]) // 恰 30 日宽
    await plot.trigger('pointerup')
    await drag(wrapper, [0, 364]) // 全池 365
    await plot.trigger('pointerup')
    expect(wrapper.emitted('source-change')).toBeUndefined()
    expect(wrapper.emitted('commit')).toHaveLength(3)
  })
})

describe('ObservationRangeControls — series 与轴', () => {
  it('null 不桥接不补 0：分段折线（仅已知段 ≥2 画线），已知 0 落基线', async () => {
    const series = [9, null, 4, 0].map((value, index) => ({ date: dateAt(index), value }))
    const { wrapper } = mountControls({ series })
    const polylines = wrapper.findAll('.orc-line')
    expect(polylines).toHaveLength(1) // [9] 与 [4,0] 两段，仅后者 ≥2 点画线
    const [x, y] = polylines[0]!.attributes('points')!.split(' ')[0]!.split(',').map(Number)
    expect(x).toBeCloseTo(indexToX(2, N, { viewWidth: 1000, viewHeight: 80, baselineY: 68, topY: 10 }), 5)
    expect(y).toBeCloseTo(valueToY(4, 9, { viewWidth: 1000, viewHeight: 80, baselineY: 68, topY: 10 }), 5)
    const zero = polylines[0]!.attributes('points')!.split(' ')[1]!.split(',').map(Number)
    expect(zero[1]).toBeCloseTo(68, 5) // 已知 0 → 基线
  })

  it('Y 量级刻度 = max/⌊max/2⌋/0；X 日期刻度 = 池首/中/末', () => {
    const series = [9, 9, 9].map((value, index) => ({ date: dateAt(index), value }))
    const { wrapper } = mountControls({ series })
    const yTexts = wrapper.findAll('[data-testid="trend-axis-y"] span').map(span => span.text())
    expect(yTexts).toEqual(['9', '4', '0'])
    const xTexts = wrapper.findAll('[data-testid="trend-axis-x"] span').map(span => span.text())
    expect(xTexts).toEqual([BOUNDS.start, dateAt(182), BOUNDS.end])
  })

  it('全未知 series：不画线、Y 无刻度（不伪造量级）', () => {
    const series = [dateAt(0), dateAt(1)].map(date => ({ date, value: null }))
    const { wrapper } = mountControls({ series })
    expect(wrapper.findAll('.orc-line')).toHaveLength(0)
    expect(wrapper.findAll('[data-testid="trend-axis-y"] span')).toHaveLength(0)
  })
})

describe('ObservationRangeControls — 状态', () => {
  it('bounds/range 未就绪：整组禁用 + 占位文案，无绘图区', () => {
    const wrapper = mount(ObservationRangeControls, {
      props: { bounds: null, range: null, sourceDays: 14, series: null },
    })
    expect(wrapper.find('[data-testid="observation-range-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="observation-range-plot"]').exists()).toBe(false)
    expect(wrapper.find('.orc-tier[aria-pressed="true"]').attributes('disabled')).toBeDefined()
  })

  it('loading / series 缺失：池曲线态文案，刷选仍可用', async () => {
    const { wrapper, plot } = mountControls({ loading: true })
    expect(wrapper.find('[data-testid="observation-range-loading"]').exists()).toBe(true)
    await drag(wrapper, [100, 110])
    await plot.trigger('pointerup')
    expect(wrapper.emitted('commit')).toHaveLength(1)

    const noSeries = mountControls()
    expect(noSeries.wrapper.find('[data-testid="observation-range-no-series"]').exists()).toBe(true)
  })

  it('上层 notice（G1 rangeNotice）在反馈行透出', () => {
    const { wrapper } = mountControls({ notice: 'URL start 参数不合法，已忽略' })
    expect(wrapper.find('[data-testid="observation-range-feedback"]').text()).toContain('URL start 参数不合法')
  })

  it('平移/缩放按钮：离散动作直接 commit（复用 G1 纯函数语义）', async () => {
    const { wrapper } = mountControls()
    await wrapper.find('button[aria-label="向前平移一个视窗"]').trigger('click')
    expect(wrapper.emitted('commit')![0]).toEqual([shiftRange(RANGE, -1, BOUNDS)])
    await wrapper.find('button[aria-label="缩小视窗（宽度加倍）"]').trigger('click')
    expect(wrapper.emitted('commit')![1]).toEqual([zoomRange(RANGE, 2, BOUNDS)])
  })
})

// ── 曲线口径标注（fix-lane-metric-provenance / simplify-workshop-matrix-copy）───
// 曲线值与矩阵格同口径（非综合稿线索数）；轴单位只写一次「线索数」，矩阵默认不另写图注
// （图注归需要它的宿主：仅工作面传 `caption`）；MUST NOT 出现已退役的事件线量纲字样。
describe('ObservationRangeControls — 曲线口径标注', () => {
  it('轴单位标「线索数」且不重复单位；不出现已退役的事件线/兜底字样', async () => {
    const { wrapper } = mountControls()
    const axes = wrapper.findComponent({ name: 'TrendAxes' })
    expect(axes.exists()).toBe(true)
    expect(axes.props('unitLabel')).toBe('线索数')
    expect(wrapper.find('.ta-unit').text()).toBe('线索数')
    expect(wrapper.text()).not.toContain('事件线')
    expect(wrapper.text()).not.toContain('兜底')
    expect(wrapper.text()).not.toContain('非文章数')
  })

  it('矩阵默认不另写图注（caption 缺省不渲染）；日期格式只由空输入提示与校验承担', () => {
    const { wrapper } = mountControls()
    expect(wrapper.find('.orc-caption').exists()).toBe(false)
    expect(wrapper.find('#orc-start').attributes('placeholder')).toBe('YYYY-MM-DD')
    expect(wrapper.findAll('.orf-field').map(node => node.text())).toEqual(['起始日', '截止日'])
    // 头部不另写日期口径旁注（Asia/Shanghai · 含起止日 已退役）
    expect(wrapper.find('.orc-sub').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Asia/Shanghai')
    expect(wrapper.text()).not.toContain('含起止日')
  })
})

describe('ObservationRangeControls — 配置化扩展（unify-lane-workbench 6.1 / OQ-1）', () => {
  it('缺省档位 = 矩阵页 7/14/30/365（零变化）', () => {
    const { wrapper } = mountControls()
    const labels = wrapper.findAll('.orc-tier').map(b => b.text())
    expect(labels).toEqual(['7 天', '14 天', '30 天', '近 1 年'])
  })

  it('tiers 传工作面档位清单：按钮按清单渲染并 emit 对应档', async () => {
    const { wrapper } = mountControls({ tiers: [7, 14, 30, 365] })
    const labels = wrapper.findAll('.orc-tier').map(b => b.text())
    expect(labels).toEqual(['7 天', '14 天', '30 天', '近 1 年'])
    await wrapper.findAll('.orc-tier')[3]!.trigger('click')
    expect(wrapper.emitted('source-change')).toEqual([[365]])
  })

  it('maxSpanDays：手势跨度被限界（保末端前移起点）；不传时不受影响', async () => {
    // 池 365 日；限 30 天：从 index 0 拖到 100 → 起点被顶到 end-29
    const { wrapper } = mountControls({ maxSpanDays: 30 })
    await drag(wrapper, [0, 100])
    await wrapper.find('[data-testid="observation-range-plot"]').trigger('pointerup')
    const commits = wrapper.emitted('commit')
    expect(commits).toBeTruthy()
    const range = commits!.at(-1)![0] as { start: string, end: string }
    expect(range).toEqual({ start: dateAt(100 - 29), end: dateAt(100) })
    // 不传 maxSpanDays：同一手势全程可用（矩阵现状）
    const plain = mountControls()
    await drag(plain.wrapper, [0, 100])
    await plain.wrapper.find('[data-testid="observation-range-plot"]').trigger('pointerup')
    const plainCommits = plain.wrapper.emitted('commit')
    expect(plainCommits!.at(-1)![0]).toEqual({ start: dateAt(0), end: dateAt(100) })
  })

  it('caption 传工作面口径文案：专用图注按传入渲染（不被精减退删）；缺省不渲染', () => {
    const { wrapper } = mountControls({ caption: ['365 日 · 可选线索数（不含 active 泳道）', '线索数（非文章数）· 不含 active 泳道'] })
    const caption = wrapper.find('.orc-caption')
    expect(caption.text()).toContain('不含 active 泳道')
    const plain = mountControls()
    expect(plain.wrapper.find('.orc-caption').exists()).toBe(false)
  })

  it('selectableBounds：表单以可选界校验（不越可选界提交）', async () => {
    const selectable = { start: '2026-09-01', end: '2026-10-23' }
    const { wrapper } = mountControls({ selectableBounds: selectable })
    await wrapper.find('#orc-start').setValue('2026-08-01') // 在池内、在可选界外
    await wrapper.find('#orc-end').setValue('2026-10-20')
    await wrapper.find('form').trigger('submit')
    expect(wrapper.emitted('commit')).toBeUndefined()
    expect(wrapper.text()).toContain('日期超出可取数范围')
  })
})


// ── 折叠（make-range-controls-collapsible） ───────────────────────────────────
// 窄屏判定走 useIsNarrowViewport（内部基于 matchMedia）；happy-dom 无真实视口，
// 故以模块级开关驱动，其余 36 条用例保持默认 false（宽屏展开）不受影响。
const collapseState = vi.hoisted(() => ({ narrow: false }))
vi.mock('~/composables/useMediaQuery', async () => {
  const { computed } = await import('vue')
  return { useIsNarrowViewport: () => computed(() => collapseState.narrow) }
})

describe('ObservationRangeControls — 可选 prop 扩展（archive-time-chart-range）', () => {
  it('缺省：单位仍「线索数」、标题仍「时间视窗」、存储键仍 syntopica.orc.collapsed', async () => {
    localStorage.clear()
    const { wrapper } = mountControls()
    expect(wrapper.findComponent({ name: 'TrendAxes' }).props('unitLabel')).toBe('线索数')
    expect(wrapper.find('.orc-title').text()).toBe('时间视窗')
    await wrapper.find('[data-testid="observation-range-toggle"]').trigger('click')
    expect(localStorage.getItem('syntopica.orc.collapsed')).toBe('1')
  })

  it('无 footer 插槽：DOM 不多出任何节点', () => {
    const { wrapper } = mountControls()
    expect(wrapper.find('.orc-footer').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('slot')
  })

  it('tiers 传空数组：不渲染档位分组（不留空的无障碍分组）', () => {
    const { wrapper } = mountControls({ tiers: [] })
    expect(wrapper.find('.orc-tiers').exists()).toBe(false)
    expect(wrapper.findAll('.orc-tier')).toHaveLength(0)
  })

  it('传入自定义单位 / 标题 / 存储键：按传入渲染（归档口径）', async () => {
    localStorage.clear()
    const { wrapper } = mountControls({
      unitLabel: '篇',
      title: '时间范围',
      collapseStorageKey: 'syntopica.archive.time.collapsed',
    })
    expect(wrapper.findComponent({ name: 'TrendAxes' }).props('unitLabel')).toBe('篇')
    expect(wrapper.find('.orc-title').text()).toBe('时间范围')
    await wrapper.find('[data-testid="observation-range-toggle"]').trigger('click')
    expect(localStorage.getItem('syntopica.archive.time.collapsed')).toBe('1')
    expect(localStorage.getItem('syntopica.orc.collapsed')).toBeNull()
  })

  it('无曲线提示按标题生成（缺省矩阵读作「时间视窗趋势未取到」）', () => {
    const { wrapper } = mountControls()
    const text = wrapper.find('[data-testid="observation-range-no-series"]').text()
    expect(text).toContain('时间视窗趋势未取到')
    expect(text).not.toContain('365 日池')
  })
})

describe('ObservationRangeControls 折叠', () => {
  const toggleOf = (w: ReturnType<typeof mountControls>['wrapper']) => w.get('[data-testid="observation-range-toggle"]')
  const summaryOf = (w: ReturnType<typeof mountControls>['wrapper']) => w.find('[data-testid="observation-range-summary"]')

  beforeEach(() => {
    localStorage.clear()
    collapseState.narrow = false
  })

  it('C1 宽屏默认展开：aria-expanded=true 且无摘要行', () => {
    const w = mountControls().wrapper
    expect(toggleOf(w).attributes('aria-expanded')).toBe('true')
    expect(summaryOf(w).exists()).toBe(false)
  })

  it('C6 展开态仍有「时间视窗」标题，且开关按钮只写「收起」（不吞标题）', () => {
    const w = mountControls().wrapper
    expect(w.text()).toContain('时间视窗')
    const toggle = toggleOf(w)
    expect(toggle.text()).toContain('收起')
    expect(toggle.text()).not.toContain('时间视窗')
  })

  it('C2 点击收起源：摘要含档位与起止日（MM-DD），主体不可见', async () => {
    const w = mountControls().wrapper
    await toggleOf(w).trigger('click')
    expect(toggleOf(w).attributes('aria-expanded')).toBe('false')
    const summary = summaryOf(w)
    expect(summary.exists()).toBe(true)
    expect(summary.text()).toContain('14 天')
    expect(summary.text()).toContain('10-10')
    expect(summary.text()).toContain('10-23')
    // v-show 通过内联 display 收起主体（happy-dom 下 isVisible 对选择器不敏感，直读 style）
    expect((w.get('#orc-body').element as HTMLElement).style.display).toBe('none')
  })

  it('C3 窄屏默认收起', () => {
    collapseState.narrow = true
    const w = mountControls().wrapper
    expect(toggleOf(w).attributes('aria-expanded')).toBe('false')
  })

  it('C4 手动展开写 localStorage，重新挂载仍展开（手动优先于窄屏默认）', async () => {
    collapseState.narrow = true
    const w = mountControls().wrapper
    expect(toggleOf(w).attributes('aria-expanded')).toBe('false')
    await toggleOf(w).trigger('click')
    expect(localStorage.getItem('syntopica.orc.collapsed')).toBe('0')
    const remounted = mountControls().wrapper
    await nextTick()
    expect(toggleOf(remounted).attributes('aria-expanded')).toBe('true')
  })

  it('C5 折叠切换不触发 preview / commit / source-change', async () => {
    const w = mountControls().wrapper
    await toggleOf(w).trigger('click')
    await toggleOf(w).trigger('click')
    expect(w.emitted('preview')).toBeUndefined()
    expect(w.emitted('commit')).toBeUndefined()
    expect(w.emitted('source-change')).toBeUndefined()
  })
})
