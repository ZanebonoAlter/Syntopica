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
import { describe, expect, it } from 'vitest'
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

// ── 曲线口径标注（matrix-clue-fallback-and-rail-merge D2）────────────────────
// 曲线值与矩阵格同口径（事件线数 > 0 用事件线数，否则线索合计兜底）——
// 用户可见标注与 Y 轴单位文案须表达该口径，不再称「线索合计」。
describe('ObservationRangeControls — 曲线口径标注', () => {
  it('曲线标注表达事件线数口径与线索兜底；Y 轴单位为「事件线数（条）」', async () => {
    const { wrapper } = mountControls()
    const caption = wrapper.find('.orc-caption')
    expect(caption.exists()).toBe(true)
    expect(caption.text()).toContain('日事件线数')
    expect(caption.text()).toContain('按线索合计兜底')
    expect(caption.text()).not.toContain('日线索合计')
    const axes = wrapper.findComponent({ name: 'TrendAxes' })
    expect(axes.exists()).toBe(true)
    expect(axes.props('unitLabel')).toBe('事件线数（条）')
  })
})
