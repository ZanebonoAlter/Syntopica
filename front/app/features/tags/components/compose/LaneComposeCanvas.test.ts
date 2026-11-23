import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import LaneComposeCanvas from './LaneComposeCanvas.vue'
import { estimateTextWidth } from '~/features/tags/composables/laneComposeLogic'
import type { ComposeCandidate, ComposeCandidateChain, ComposeRelation } from '~/api/persistentTopics'

/**
 * 中央画布缩放用例（用户 2026-11-01 当场提的需求：滚轮放大 / 缩小）。
 *
 * 判据（机械断言，不看实现）：
 * - 滚轮 → 缩放读数变化（放大一次、缩小一次）；
 * - 100% 时 `viewBox` 宽 == 容器 `clientWidth`（1:1 契约，误差 <1px）；
 * - 缩放后**没有元素越出画布边界**；普通点**渲染**半径仍 ≥3px（缩小时不许缩到看不见）；
 * - 缩放后节点卡仍贴在节点旁（水平距离 < 200px，与既有 E2E 口径一致）；
 * - 重置入口能回到 100%。
 *
 * happy-dom 注意：不实现 `ResizeObserver`、不实现真实滚动；
 * `clientWidth` 用 `Object.defineProperty` 打桩，`WheelEvent` 不带 clientX/clientY（自行补）。
 */

function section(
  id: string,
  periodDate: string,
  clusterLabel: string,
  embedding: number[],
  topicId?: string,
  status?: string,
): ComposeCandidate {
  return {
    id,
    reportId: '1',
    periodDate,
    clusterLabel,
    embedding,
    persistentTopicId: topicId,
    persistentTopic: topicId
      ? { id: topicId, label: topicId === '1984' ? '候选泳道 A' : '已有泳道', status: status ?? 'candidate' }
      : undefined,
  }
}

const SECTIONS: ComposeCandidate[] = [
  section('1', '2026-10-26', '候选一', [1, 0], '1984'),
  section('2', '2026-10-27', '候选二', [0.99, 0.1], '1984'),
  section('3', '2026-10-28', '候选三', [0.98, 0.2], '1984'),
  section('4', '2026-10-26', '散料近邻', [0.97, 0.15]),
]

const RELATIONS: ComposeRelation[] = [
  { fromId: '1', toId: '2', distance: 0.1, relationType: 'identity' },
  { fromId: '2', toId: '3', distance: 0.12, relationType: 'identity' },
]

const CHAINS: ComposeCandidateChain[] = [
  {
    key: '1984:1',
    topicId: '1984',
    topicLabel: '候选泳道 A',
    topicStatus: 'candidate',
    sectionIds: ['1', '2', '3'],
    firstDate: '2026-10-26',
    lastDate: '2026-10-28',
    spanDays: 2,
  },
]

const DATES = ['2026-10-26', '2026-10-27', '2026-10-28']

let wrapper: ReturnType<typeof mount> | null = null

async function mountCanvas() {
  wrapper = mount(LaneComposeCanvas, {
    props: {
      sections: SECTIONS,
      relations: RELATIONS,
      chains: CHAINS,
      selectedIds: new Set(['1', '2', '3']),
      focusId: '2',
      matchThreshold: 0.3,
      showAssigned: false,
      dates: DATES,
    },
    attachTo: document.body,
  })
  await flushPromises()
  return wrapper
}

/** 模拟容器真实宽度（画布 1:1 的前提）。 */
async function setCanvasWidth(w: number): Promise<void> {
  const el = wrapper!.find('[data-testid="lane-compose-canvas"]').element as HTMLElement
  Object.defineProperty(el, 'clientWidth', { value: w, configurable: true })
  window.dispatchEvent(new Event('resize'))
  await flushPromises()
}

/** 在画布上滚一格（deltaY < 0 放大、> 0 缩小）。 */
async function wheel(deltaY: number, clientX = 120, clientY = 80): Promise<WheelEvent> {
  const el = wrapper!.find('[data-testid="lane-compose-canvas-svg"]').element as SVGElement
  const event = new WheelEvent('wheel', { deltaY, bubbles: true, cancelable: true })
  // happy-dom 的 WheelEvent 不带 clientX/clientY（真实浏览器原生带），这里补上
  Object.defineProperty(event, 'clientX', { value: clientX, configurable: true })
  Object.defineProperty(event, 'clientY', { value: clientY, configurable: true })
  el.dispatchEvent(event)
  await flushPromises()
  return event
}

function readout(): string {
  return wrapper!.find('[data-testid="lane-compose-zoom-readout"]').text()
}

function svgAttrs(): { viewBox: string, width: number, height: number } {
  const el = wrapper!.find('[data-testid="lane-compose-canvas-svg"]')
  const viewBox = el.attributes('viewBox') ?? ''
  return { viewBox, width: Number(el.attributes('width')), height: Number(el.attributes('height')) }
}

/** 节点卡的 left（px，解析 style 字符串）。 */
function cardLeft(): number {
  const style = wrapper!.find('[data-testid="lane-node-card"]').attributes('style') ?? ''
  return Number(/left:\s*([\d.]+)px/.exec(style)?.[1])
}


/** 在画布表面拖一段（pointerdown → pointermove → pointerup）。 */
async function drag(from: { x: number, y: number }, to: { x: number, y: number }, on?: Element): Promise<void> {
  const surface = wrapper!.find('[data-testid="lane-compose-canvas"]')
  const target = on ?? surface.element
  const down = new Event('pointerdown', { bubbles: true, cancelable: true }) as PointerEvent
  Object.defineProperty(down, 'clientX', { value: from.x, configurable: true })
  Object.defineProperty(down, 'clientY', { value: from.y, configurable: true })
  Object.defineProperty(down, 'pointerId', { value: 1, configurable: true })
  target.dispatchEvent(down)
  const move = new Event('pointermove', { bubbles: true, cancelable: true }) as PointerEvent
  Object.defineProperty(move, 'clientX', { value: to.x, configurable: true })
  Object.defineProperty(move, 'clientY', { value: to.y, configurable: true })
  Object.defineProperty(move, 'pointerId', { value: 1, configurable: true })
  target.dispatchEvent(move)
  const up = new Event('pointerup', { bubbles: true, cancelable: true }) as PointerEvent
  Object.defineProperty(up, 'clientX', { value: to.x, configurable: true })
  Object.defineProperty(up, 'clientY', { value: to.y, configurable: true })
  Object.defineProperty(up, 'pointerId', { value: 1, configurable: true })
  target.dispatchEvent(up)
  await flushPromises()
}

/** 画布表面的滚动位置（happy-dom 不做真实滚动，这里直接读写属性）。 */
function scrollPos(): { left: number, top: number } {
  const el = wrapper!.find('[data-testid="lane-compose-canvas"]').element as HTMLElement
  return { left: el.scrollLeft, top: el.scrollTop }
}

beforeEach(() => {
  document.body.innerHTML = ''
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('LaneComposeCanvas — 滚轮缩放（Z1–Z6）', () => {
  it('Z1 滚轮：向上滚放大、向下滚缩小，读数跟着变', async () => {
    await mountCanvas()
    await setCanvasWidth(900)
    expect(readout()).toBe('100%')

    await wheel(-100)
    expect(readout()).toBe('110%')

    await wheel(-100)
    expect(readout()).toBe('121%')

    await wheel(100)
    expect(readout()).toBe('110%')
  })

  it('Z2 1:1 契约：100% 时 viewBox 宽 == 容器 clientWidth；缩放只改呈现、不重排日期列', async () => {
    await mountCanvas()
    await setCanvasWidth(900)
    const before = svgAttrs()
    const vbWidth = Number(/^0 0 (\d+)/.exec(before.viewBox)![1])
    expect(Math.abs(vbWidth - 900)).toBeLessThan(1)

    await wheel(-100)
    const after = svgAttrs()
    // 逻辑坐标（viewBox）不动 → 日期列位置不漂移
    expect(after.viewBox).toBe(before.viewBox)
    // 等比缩放：SVG 元素尺寸按比例放大
    expect(after.width).toBeCloseTo(before.width * 1.1, 6)
    expect(after.height).toBeCloseTo(before.height * 1.1, 6)
  })

  it('Z3 缩放后没有元素越出画布边界；普通点渲染半径仍 ≥3px', async () => {
    const w = await mountCanvas()
    await setCanvasWidth(900)
    const vbWidth = 900
    const vbHeight = Number(/^0 0 \d+ (\d+)/.exec(svgAttrs().viewBox)![1])

    await wheel(-100)
    await wheel(-100)
    for (const point of w.findAll('.lcc-canvas__point')) {
      const cx = Number(point.attributes('cx'))
      const cy = Number(point.attributes('cy'))
      expect(cx).toBeGreaterThanOrEqual(0)
      expect(cx).toBeLessThanOrEqual(vbWidth)
      expect(cy).toBeGreaterThanOrEqual(0)
      expect(cy).toBeLessThanOrEqual(vbHeight)
    }
    // 行式泳道：没有横条胶囊（行分组只靠 y 区段 + 行标签 + 1px 分隔线）
    expect(w.findAll('[data-role="band-capsule"], .lcc-canvas__band-bar, .lcc-canvas__band-tick')).toHaveLength(0)

    // 缩到最小 0.5×：普通点渲染半径仍 ≥3px（缩小时不许把点缩到看不见）
    for (let i = 0; i < 10; i++) await wheel(100)
    expect(readout()).toBe('50%')
    for (const point of w.findAll('.lcc-canvas__point')) {
      expect(Number(point.attributes('r')) * 0.5).toBeGreaterThanOrEqual(3)
    }
  })

  it('Z4 缩放后节点卡仍贴在节点旁（水平距离 < 200px）', async () => {
    const w = await mountCanvas()
    await setCanvasWidth(900)
    const cx = Number(w.find('[data-testid="lane-node-2"] .lcc-canvas__point').attributes('cx'))
    const leftAt100 = cardLeft()

    await wheel(-100)
    const left = cardLeft()
    // 节点的屏幕位置 = 逻辑坐标 × 缩放；卡片必须跟着走（否则与节点错位）
    const screenX = cx * 1.1
    expect(Math.abs(left - screenX)).toBeLessThan(200)
    expect(left).toBeGreaterThan(leftAt100)
  })

  it('Z5 重置入口能回到 100%', async () => {
    const w = await mountCanvas()
    await setCanvasWidth(900)
    await wheel(-100)
    expect(readout()).toBe('110%')

    await w.find('[data-testid="lane-compose-zoom-reset"]').trigger('click')
    await flushPromises()
    expect(readout()).toBe('100%')
    expect(svgAttrs().width).toBeCloseTo(900, 6)
  })

  it('Z6 画布上滚轮不把整页滚走（preventDefault）', async () => {
    await mountCanvas()
    await setCanvasWidth(900)
    const event = await wheel(-100)
    expect(event.defaultPrevented).toBe(true)
  })

  it('Z7 缩放只影响呈现：选择与相关节点标题不受影响', async () => {
    const w = await mountCanvas()
    await setCanvasWidth(900)
    const selectedBefore = w.findAll('.lcc-canvas__node.is-selected').length
    const relatedBefore = w.findAll('.lcc-canvas__related-title').length

    await wheel(-100)
    await wheel(100)
    expect(w.findAll('.lcc-canvas__node.is-selected')).toHaveLength(selectedBefore)
    expect(w.findAll('.lcc-canvas__related-title')).toHaveLength(relatedBefore)
    expect(readout()).toBe('100%')
  })
})

describe('LaneComposeCanvas — 拖拽平移（P1–P3）', () => {
  it('P1 按住拖动：内容跟着指针走（滚动位置与位移反向）', async () => {
    const w = await mountCanvas()
    const el = w.find('[data-testid="lane-compose-canvas"]').element as HTMLElement
    el.scrollLeft = 0
    el.scrollTop = 0
    await drag({ x: 400, y: 300 }, { x: 300, y: 300 })
    expect(scrollPos().left).toBe(100)
    await drag({ x: 300, y: 300 }, { x: 200, y: 260 })
    expect(scrollPos().left).toBe(200)
    expect(scrollPos().top).toBe(40)
  })

  it('P2 位移没超阈值不算拖（不改变滚动位置）', async () => {
    const w = await mountCanvas()
    const el = w.find('[data-testid="lane-compose-canvas"]').element as HTMLElement
    el.scrollLeft = 0
    el.scrollTop = 0
    await drag({ x: 200, y: 200 }, { x: 202, y: 201 })
    expect(scrollPos().left).toBe(0)
    expect(scrollPos().top).toBe(0)
  })

  it('P3 拖拽刚结束的那一下 click 不算「点节点」（不误触发聚焦）', async () => {
    const w = await mountCanvas()
    const node = w.find('[data-node="1"]').element as Element
    await drag({ x: 100, y: 100 }, { x: 40, y: 100 }, node)
    // 拖拽后再点同一个节点：应当正常触发（拖拽那一刻被压掉的是那一下 click）
    await w.find('[data-node="1"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-node-card"]').exists()).toBe(true)
  })
})

describe('LaneComposeCanvas — 节点卡宽度固定（防塌成竖条）', () => {
  it('Z8 卡片宽度用常量、不用「量出来的宽度」算位置（避���塌缩反馈环）', () => {
    const src = readFileSync(`${import.meta.dirname}/LaneComposeCanvas.vue`, 'utf8')
    // 宽度常量存在，且定位用它
    expect(src).toMatch(/const CARD_WIDTH = \d+/)
    expect(src).toMatch(/const width = CARD_WIDTH/)
    // 不许把量出来的宽度喂回定位（反馈环 → 卡片塌成一条竖条、文字一字一行）
    expect(src).not.toMatch(/width: el\.offsetWidth/)
    // 样式上固定宽度 + 高度封顶（内容超高时卡内滚动）
    expect(src).toMatch(/\.lcc-card \{[^}]*width: 300px/)
    expect(src).toMatch(/\.lcc-card \{[^}]*max-height: calc\(100% - 12px\)/)
  })
})

describe('LaneComposeCanvas · 焦点相关高亮与接近度（unify-lane-workbench 5.2 / 4.3）', () => {
  it('焦点的一跳原关系相关高亮，非相关弱化；推荐命中也相关（D6）', async () => {
    const w = await mountCanvas()
    // 种子 = {focusId:'2'}：一跳 1/3 相关；4 是散料近邻（对质心近 → 推荐命中）也相关
    expect(w.find('[data-testid="lane-node-1"]').classes()).toContain('is-related')
    expect(w.find('[data-testid="lane-node-3"]').classes()).toContain('is-related')
    expect(w.find('[data-testid="lane-node-4"]').classes()).toContain('is-related')
    // 已选成员自己不进相关集合（种子/成员排除）；无其它可弱化节点时无断言对象
    expect(w.find('[data-testid="lane-node-2"]').classes()).not.toContain('is-related')
  })

  it('无焦点不弱化不残留；切换焦点重算、取消恢复默认', async () => {
    const w = await mountCanvas()
    await w.setProps({ focusId: null })
    for (const id of ['1', '2', '3', '4']) {
      expect(w.find(`[data-testid="lane-node-${id}"]`).classes()).not.toContain('is-related')
      expect(w.find(`[data-testid="lane-node-${id}"]`).classes()).not.toContain('is-dimmed')
    }
  })

  it('焦点不参与质心（推荐仍用已选成员均值，焦点向量不改变推荐集）', async () => {
    const w = await mountCanvas()
    // 4 与已选质心（≈[0.99,0.1]）距离小 → 推荐命中（与 focusId 无关）
    expect(w.find('[data-testid="lane-node-4"]').attributes('data-distance')).not.toBe('')
    await w.setProps({ focusId: '4' })
    // 焦点换到 4：4 仍是推荐（不因成为焦点而被排除/加入质心）
    expect(w.find('[data-testid="lane-node-4"]').classes()).not.toContain('is-related')
  })

  it('节点卡接近度方向统一：distance=0.2 显示估算接近度 0.80（不再显示距离）', async () => {
    const w = await mountCanvas()
    await w.setProps({ focusId: '4' })
    await flushPromises()
    const card = w.find('[data-testid="lane-node-card"]')
    expect(card.exists()).toBe(true)
    // 散料近邻 embedding [0.97,0.15] vs 质心 [0.99,0.1] → cos 距离 ≈ 0.0002 → 接近度 ≈ 1.00
    expect(card.text()).toContain('估算接近度')
    expect(card.text()).not.toContain('距离')
  })

  it('两工作面接近度同向（B4/S2.3）：distance=0.2 样本显示 0.80', async () => {
    // 直接构造距离样本：embedding 与质心夹角使 cos = 0.8（distance = 0.2）
    const sample = section('9', '2026-10-26', '方向样本', [0.8, 0.6])
    const w2 = mount(LaneComposeCanvas, {
      props: {
        sections: [section('m', '2026-10-26', '成员', [1, 0]), sample],
        relations: [],
        chains: [],
        selectedIds: new Set(['m']),
        focusId: null,
        matchThreshold: 0.3,
        showAssigned: false,
        dates: DATES,
      },
      attachTo: document.body,
    })
    await flushPromises()
    // 质心 = [1,0]；样本 [0.8,0.6] → cos = 0.8 → 距离 0.2 → 接近度 0.80
    await w2.setProps({ focusId: '9' })
    await flushPromises()
    expect(w2.find('[data-testid="lane-node-card"]').text()).toContain('估算接近度 0.80')
    w2.unmount()
  })
})

// ── 行式泳道 / 手势 / 门控 / 检索态（rework-lane-compose-swimlane-canvas 2.x–5.x） ──

/** 连续日期轴。 */
function days(count: number, start = '2026-11-06'): string[] {
  const out: string[] = []
  const startTs = Date.parse(`${start}T00:00:00Z`)
  for (let i = 0; i < count; i++) out.push(new Date(startTs + i * 86_400_000).toISOString().slice(0, 10))
  return out
}

async function mountRows(overrides: Record<string, unknown> = {}) {
  const w = mount(LaneComposeCanvas, {
    props: {
      sections: SECTIONS,
      relations: RELATIONS,
      chains: CHAINS,
      selectedIds: new Set<string>(['1', '2', '3']),
      focusId: null,
      matchThreshold: 0.3,
      showAssigned: false,
      dates: DATES,
      ...overrides,
    },
    attachTo: document.body,
  })
  wrapper = w
  await flushPromises()
  const el = w.find('[data-testid="lane-compose-canvas"]').element as HTMLElement
  Object.defineProperty(el, 'clientWidth', { value: 900, configurable: true })
  Object.defineProperty(el, 'clientHeight', { value: 600, configurable: true })
  window.dispatchEvent(new Event('resize'))
  await flushPromises()
  return w
}

function pointOf(w: ReturnType<typeof mount>, id: string): { x: number, y: number } {
  const circle = w.find(`[data-testid="lane-node-${id}"] .lcc-canvas__point`)
  return { x: Number(circle.attributes('cx')), y: Number(circle.attributes('cy')) }
}

function rowTop(w: ReturnType<typeof mount>, row: number): number {
  return Number(w.find(`.lcc-canvas__row[data-row="${row}"] [data-role="row-separator"]`).attributes('y1'))
}

function canvasWidth(w: ReturnType<typeof mount>): number {
  return Number(/^0 0 (\d+)/.exec(w.find('[data-testid="lane-compose-canvas-svg"]').attributes('viewBox')!)![1])
}

describe('LaneComposeCanvas — 行式泳道（2.1 / 2.2）', () => {
  it('行分组只靠「行标签 + 分隔线」：无横条胶囊、无 tick 拖线', async () => {
    const w = await mountRows()
    expect(w.element.querySelectorAll('[data-role="band-capsule"]').length).toBe(0)
    expect(w.findAll('.lcc-canvas__band-bar')).toHaveLength(0)
    expect(w.findAll('.lcc-canvas__band-tick')).toHaveLength(0)
    // 每行一条分隔线 + 行标签
    const rows = w.findAll('.lcc-canvas__row')
    expect(rows.length).toBeGreaterThanOrEqual(2)
    expect(w.findAll('[data-role="row-separator"]')).toHaveLength(rows.length)
    expect(w.findAll('[data-role="row-label"]')).toHaveLength(rows.length)
    // 绘制顺序不参与断言：只要求「本次泳道行唯一且存在」
    expect(w.findAll('[data-row-kind="lane"]')).toHaveLength(1)
  })

  it('链被取空 → 该行不渲染（左栏才保留 n/n）', async () => {
    const w = await mountRows({ selectedIds: new Set(['1', '2', '3']) })
    expect(w.findAll('.lcc-canvas__row[data-row-kind="chain"]')).toHaveLength(0)
    expect(w.find('.lcc-canvas__row[data-row-kind="lane"] [data-role="row-label"]').text()).toBe('本次泳道')

    const still = await mountRows({ selectedIds: new Set<string>() })
    expect(still.findAll('.lcc-canvas__row[data-row-kind="chain"]')).toHaveLength(1)
  })

  it('成员 y 按「行归属 + 槽位」排（同日排序键不是 id）', async () => {
    const w = await mountRows({ selectedIds: new Set<string>() })
    // 三个链成员在候选链行（row 1），三个不同日 → 同槽位不同列
    const ys = ['1', '2', '3'].map(id => pointOf(w, id).y)
    expect(new Set(ys).size).toBe(1)
    expect(ys[0]).toBeGreaterThan(rowTop(w, 1))
    // 散料（近邻，已被质心推荐命中）在地板行，比链行更靠下
    if (w.find('[data-testid="lane-node-4"]').exists()) {
      expect(pointOf(w, '4').y).toBeGreaterThan(ys[0]!)
    }
  })

  it('本次泳道成员优先归第 0 行（不改变其原链行身份）', async () => {
    const w = await mountRows({ selectedIds: new Set(['2']) })
    const laneTop = rowTop(w, 0)
    expect(pointOf(w, '2').y).toBeGreaterThan(laneTop)
    expect(pointOf(w, '2').y).toBeLessThan(rowTop(w, 1))
  })
})

describe('LaneComposeCanvas — 节点常显截断标题（2.3 / D6）', () => {
  it('14 天窗口：所有会画出来的节点都有 ≤10 字标题（含散料），一节点一条', async () => {
    const w = await mountRows({ dates: days(14, '2026-10-26') })
    const titles = w.findAll('[data-role="node-title"]')
    expect(titles.length).toBeGreaterThan(0)
    for (const title of titles) expect(title.text().length).toBeLessThanOrEqual(10)
    // 候选链成员 / 已选成员 / 散料都有标题（同一行不能有的有点、有的没有）
    expect(w.find('[data-testid="lane-node-1"] [data-role="node-title"]').text()).toBe('候选一')
    expect(w.find('[data-testid="lane-node-4"] [data-role="node-title"]').text()).toBe('散料近邻')
    // 一节点一条：标题数 == 会画出来的非只读节点数
    expect(titles).toHaveLength(w.findAll('.lcc-canvas__node:not(.is-readonly)').length)
  })

  it('30 天窗口：标题仍常显（列宽锁 124 → 10 字），不再整档关闭', async () => {
    const w = await mountRows({ dates: days(30, '2026-10-26') })
    const titles = w.findAll('[data-role="node-title"]')
    expect(titles.length).toBeGreaterThan(0)
    for (const title of titles) expect(title.text().length).toBeLessThanOrEqual(10)
    expect(w.find('[data-testid="lane-node-1"] [data-role="node-title"]').text()).toBe('候选一')
    // 字号来自样式表（happy-dom 不套 scoped CSS，按源码锚断言 11px 档）
    const src = readFileSync(`${import.meta.dirname}/LaneComposeCanvas.vue`, 'utf8')
    expect(src).toMatch(/\.lcc-canvas__node-title \{[^}]*font-size: 11px/)
    expect(src).toMatch(/const NODE_TITLE_FONT = 11/)
  })

  it('标题居中在点下（baseline = 点 y + 14），不越出画布边界', async () => {
    const w = await mountRows({ dates: days(14, '2026-10-26') })
    const vbWidth = canvasWidth(w)
    for (const id of ['1', '2', '3']) {
      const node = w.find(`[data-testid="lane-node-${id}"]`)
      const title = node.find('[data-role="node-title"]')
      const p = pointOf(w, id)
      expect(Number(title.attributes('y'))).toBeCloseTo(p.y + 14, 6)
      expect(Number(title.attributes('x'))).toBeCloseTo(p.x, 6)
      expect(title.attributes('text-anchor')).toBe('middle')
      const half = (title.text().length * 11) / 2
      expect(p.x - half).toBeGreaterThanOrEqual(0)
      expect(p.x + half).toBeLessThanOrEqual(vbWidth)
    }
  })
})

describe('LaneComposeCanvas — 同日堆叠与封顶（2.4）', () => {
  function sameDay(count: number): ComposeCandidate[] {
    return Array.from({ length: count }, (_, i) => section(
      `s${i}`,
      '2026-11-06',
      `同日${i}`,
      [1, 0],
    ))
  }

  it('同日 3 条各占一槽：18px 一档、各自带标题不重叠', async () => {
    const sections = sameDay(3)
    const w = await mountRows({ sections, chains: [], dates: days(14), selectedIds: new Set(['s0', 's1', 's2']) })
    const ys = ['s0', 's1', 's2'].map(id => pointOf(w, id).y)
    expect(ys[1]! - ys[0]!).toBe(24)
    expect(ys[2]! - ys[1]!).toBe(24)
    const titles = ['s0', 's1', 's2'].map(id => Number(w.find(`[data-testid="lane-node-${id}"] [data-role="node-title"]`).attributes('y')))
    expect(new Set(titles).size).toBe(3)
    // 行高随槽数增长：3 槽行（row 0）比地板行更高
    const laneHeight = Number(w.find('.lcc-canvas__row[data-row="0"] .lcc-canvas__dropzone').attributes('height'))
    // 行高 = 标签 32 + 3 槽 54 + 行间 26；落点框 = 行高 − 行间 − 1
    expect(laneHeight).toBe(32 + 3 * 24 - 1)
  })

  it('同列自下而上画：标题压在上一个点上，不被下一个点盖掉（18px 槽距的补偿）', async () => {
    const w = await mountRows({ sections: sameDay(3), chains: [], dates: days(14), selectedIds: new Set(['s0', 's1', 's2']) })
    // DOM 顺序 = 绘制顺序：y 降序（s2 在最下）
    const order = w.findAll('[data-testid^="lane-node-s"]').map(node => node.attributes('data-testid'))
    expect(order).toEqual(['lane-node-s2', 'lane-node-s1', 'lane-node-s0'])
  })

  it('同日 7 条封顶 6 槽并给可点的 +N；点开后该日全部展开', async () => {
    const sections = sameDay(7)
    const w = await mountRows({ sections, chains: [], dates: days(14), selectedIds: new Set(sameDay(7).map(s => s.id)) })
    const more = w.find('[data-role="day-more"]')
    expect(more.exists()).toBe(true)
    expect(more.text()).toBe('+1')
    // 封顶：只画出 6 个（第 7 条不落槽）
    expect(w.findAll('[data-testid^="lane-node-s"]').length).toBe(6)
    await more.trigger('click')
    await flushPromises()
    expect(w.findAll('[data-testid^="lane-node-s"]').length).toBe(7)
  })
})

describe('LaneComposeCanvas — 手势分流与拖拽选入（3.1–3.3）', () => {
  it('空白拖 = 平移（不改成员、不选中）', async () => {
    const w = await mountRows()
    const el = w.find('[data-testid="lane-compose-canvas"]').element as HTMLElement
    el.scrollLeft = 0
    el.scrollTop = 0
    await drag({ x: 500, y: 500 }, { x: 420, y: 500 })
    expect(el.scrollLeft).toBe(80)
    expect(w.emitted('toggle-member')).toBeUndefined()
    expect(w.emitted('focus')).toBeUndefined()
  })

  it('节点点按 = 聚焦（成员数不变，防回归到「点击选入」）', async () => {
    const w = await mountRows({ selectedIds: new Set<string>() })
    const node = w.find('[data-testid="lane-node-1"]')
    await node.trigger('click')
    await flushPromises()
    expect(w.emitted('focus')?.[0]).toEqual(['1'])
    expect(w.emitted('toggle-member')).toBeUndefined()
  })

  it('拖单节点到本次泳道行 = 选入；拖出本次泳道 = 移出', async () => {
    const loose = await mountRows({ selectedIds: new Set<string>() })
    const from = pointOf(loose, '1')
    const laneY = rowTop(loose, 0) + 40
    await drag(from, { x: from.x, y: laneY }, loose.find('[data-testid="lane-node-1"]').element)
    expect(loose.emitted('toggle-member')?.[0]).toEqual(['1', true])

    const picked = await mountRows({ selectedIds: new Set(['1', '2', '3']) })
    const p = pointOf(picked, '1')
    await drag(p, { x: p.x, y: rowTop(picked, 0) + 200 }, picked.find('[data-testid="lane-node-1"]').element)
    expect(picked.emitted('toggle-member')?.[0]).toEqual(['1', false])
  })

  it('拖拽期间不响应悬停卡；拖拽结束那一下 click 被吞掉', async () => {
    const w = await mountRows({ selectedIds: new Set<string>() })
    const nodeEl = w.find('[data-testid="lane-node-1"]').element
    const from = pointOf(w, '1')
    await drag(from, { x: from.x, y: rowTop(w, 0) + 40 }, nodeEl)
    expect(w.emitted('focus')).toBeUndefined()
  })

  it('Esc / pointercancel 取消拖拽：不产生任何成员变化', async () => {
    const w = await mountRows({ selectedIds: new Set<string>() })
    const from = pointOf(w, '1')
    const surface = w.find('[data-testid="lane-compose-canvas"]')
    const down = new Event('pointerdown', { bubbles: true }) as PointerEvent
    Object.defineProperties(down, { clientX: { value: from.x }, clientY: { value: from.y }, pointerId: { value: 1 } })
    w.find('[data-testid="lane-node-1"]').element.dispatchEvent(down)
    const move = new Event('pointermove', { bubbles: true }) as PointerEvent
    Object.defineProperties(move, { clientX: { value: from.x }, clientY: { value: rowTop(w, 0) + 40 }, pointerId: { value: 1 } })
    surface.element.dispatchEvent(move)
    await flushPromises()
    expect(w.find('[data-role="drag-ghost"]').exists()).toBe(true)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(w.find('[data-role="drag-ghost"]').exists()).toBe(false)
    const up = new Event('pointerup', { bubbles: true }) as PointerEvent
    Object.defineProperties(up, { clientX: { value: from.x }, clientY: { value: rowTop(w, 0) + 40 }, pointerId: { value: 1 } })
    surface.element.dispatchEvent(up)
    await flushPromises()
    expect(w.emitted('toggle-member')).toBeUndefined()
  })

  it('拖行标签整组采纳：该行全部未采纳成员一次进入本次泳道', async () => {
    const w = await mountRows({ selectedIds: new Set<string>() })
    const handle = w.find('[data-role="row-handle"][data-row="1"]')
    expect(handle.exists()).toBe(true)
    const from = {
      x: Number(handle.attributes('x')) + 20,
      y: Number(handle.attributes('y')) + 12,
    }
    await drag(from, { x: from.x, y: rowTop(w, 0) + 40 }, handle.element)
    expect(w.emitted('adopt-chain')?.[0]).toEqual(['1984:1'])
  })

  it('D7：拖拽只动 ghost，不重算 points', async () => {
    const w = await mountRows({ selectedIds: new Set<string>() })
    const vm = w.vm as unknown as { debugPointsEpoch: () => number }
    const before = vm.debugPointsEpoch()
    const from = pointOf(w, '1')
    const surface = w.find('[data-testid="lane-compose-canvas"]')
    const down = new Event('pointerdown', { bubbles: true }) as PointerEvent
    Object.defineProperties(down, { clientX: { value: from.x }, clientY: { value: from.y }, pointerId: { value: 1 } })
    w.find('[data-testid="lane-node-1"]').element.dispatchEvent(down)
    for (let i = 1; i <= 5; i++) {
      const move = new Event('pointermove', { bubbles: true }) as PointerEvent
      Object.defineProperties(move, { clientX: { value: from.x }, clientY: { value: from.y - i * 10 }, pointerId: { value: 1 } })
      surface.element.dispatchEvent(move)
    }
    await flushPromises()
    expect(w.find('[data-role="drag-ghost"]').exists()).toBe(true)
    expect(vm.debugPointsEpoch()).toBe(before)
    const up = new Event('pointerup', { bubbles: true }) as PointerEvent
    Object.defineProperties(up, { clientX: { value: from.x }, clientY: { value: rowTop(w, 0) + 40 }, pointerId: { value: 1 } })
    surface.element.dispatchEvent(up)
    await flushPromises()
    expect(vm.debugPointsEpoch()).toBe(before)
  })
})

describe('LaneComposeCanvas — 键盘通道与撤销（3.4 / 3.5）', () => {
  it('节点 tabindex=0；Enter/Space 走「钉住卡」，卡内动作改成员数', async () => {
    const w = await mountRows({ selectedIds: new Set(['1']) })
    const node = w.find('[data-testid="lane-node-4"]')
    expect(node.attributes('tabindex')).toBe('0')
    await node.trigger('keydown', { key: 'Enter' })
    expect(w.emitted('focus')?.[0]).toEqual(['4'])

    await w.setProps({ focusId: '4' })
    await flushPromises()
    const action = w.find('[data-testid="lane-node-card-action"]')
    expect(action.text()).toContain('选入本次泳道')
    await action.trigger('click')
    expect(w.emitted('toggle-member')?.[0]).toEqual(['4', true])
  })

  it('focus-visible 环建在圆外（源码锚：:focus-visible 规则存在）', () => {
    const src = readFileSync(`${import.meta.dirname}/LaneComposeCanvas.vue`, 'utf8')
    expect(src).toMatch(/\.lcc-canvas__node:focus-visible \.lcc-canvas__focus-ring/)
  })

  it('撤销入口：undoId 非空时出现，点击 emit undo-removal', async () => {
    const w = await mountRows({ undoId: '2' })
    const pill = w.find('[data-testid="lane-compose-undo"]')
    expect(pill.exists()).toBe(true)
    expect(pill.text()).toContain('已移出')
    await w.find('[data-testid="lane-compose-undo-btn"]').trigger('click')
    expect(w.emitted('undo-removal')).toHaveLength(1)
  })
})

describe('LaneComposeCanvas — 阈值门控与散料折叠（4.1 / 4.2 / 4.3）', () => {
  const far = section('7', '2026-10-27', '散料较远', [0.7, 0.714])

  it('切「仅高相关」：候选链成员 / 已选 / 只读参照逐个节点不变', async () => {
    const sections = [...SECTIONS, far, section('8', '2026-10-26', '只读参照', [1, 0], '2000', 'active')]
    const standard = await mountRows({ sections, selectedIds: new Set(['1', '2', '3']), showAssigned: true })
    const tight = await mountRows({ sections, selectedIds: new Set(['1', '2', '3']), showAssigned: true, thresholdTier: 'tight' })

    for (const id of ['1', '2', '3', '8']) {
      expect(tight.find(`[data-testid="lane-node-${id}"]`).exists()).toBe(standard.find(`[data-testid="lane-node-${id}"]`).exists())
    }
    // 散料按档位收紧：远处散料在紧档消失、标准档可见
    expect(standard.find('[data-testid="lane-node-7"]').exists()).toBe(true)
    expect(tight.find('[data-testid="lane-node-7"]').exists()).toBe(false)
  })

  it('散料折叠：只数散料、措辞为「低相关散料 N 条」，可展开/收起', async () => {
    const w = await mountRows({ sections: [...SECTIONS, far], selectedIds: new Set<string>() })
    // 无已选 → 无质心 → 散料全部收起（不伪造达标）
    const fold = w.find('[data-testid="lane-floor-fold"]')
    expect(fold.exists()).toBe(true)
    expect(fold.text()).toBe('低相关散料 2 条 · 展开')
    expect(fold.text()).not.toContain('不相关')
    expect(fold.text()).not.toContain('无关联')
    expect(fold.text()).not.toContain('置信度')

    await fold.trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-node-4"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-node-7"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-floor-fold"]').text()).toBe('收起')
    await w.find('[data-testid="lane-floor-fold"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="lane-node-4"]').exists()).toBe(false)
  })
})

describe('LaneComposeCanvas — 检索态（5.2 / 5.3）', () => {
  it('候选链行与地板降透明度、本次泳道行不降级；命中点四角括号 + 标题', async () => {
    const w = await mountRows({
      selectedIds: new Set(['1']),
      searchActive: true,
      searchHits: new Set(['4']),
    })
    // 本次泳道行不降级
    const laneRow = w.find('.lcc-canvas__row[data-row-kind="lane"]')
    expect(laneRow.classes()).not.toContain('is-search-bg')
    // 候选链行 / 地板降为背景
    for (const row of w.findAll('.lcc-canvas__row').filter(r => r.attributes('data-row-kind') !== 'lane')) {
      expect(row.classes()).toContain('is-search-bg')
    }
    // 命中：四角括号 + 常显截断标题（列宽够时例外显示；列宽不足整档关闭，见下一条）
    expect(w.find('[data-testid="lane-node-4"] [data-role="search-hit"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-node-4"] [data-role="node-title"]').exists()).toBe(true)
    // 未命中节点不可点
    expect(w.find('[data-testid="lane-node-2"]').classes()).toContain('is-search-muted')
    expect(w.find('[data-testid="lane-node-2"]').attributes('tabindex')).toBe('-1')
    // 命中点可点
    expect(w.find('[data-testid="lane-node-4"]').attributes('tabindex')).toBe('0')
  })

  it('退出检索回到进入前的阶：括号与降级一起撤除（标题仍在：所有节点常显）', async () => {
    const w = await mountRows({ selectedIds: new Set(['1']), searchActive: true, searchHits: new Set(['4']) })
    expect(w.findAll('[data-role="search-hit"]')).toHaveLength(1)
    await w.setProps({ searchActive: false, searchHits: new Set<string>() })
    await flushPromises()
    expect(w.findAll('[data-role="search-hit"]')).toHaveLength(0)
    expect(w.findAll('.is-search-bg')).toHaveLength(0)
    // 标题不因退出检索消失（散料常显），但不再有「命中」强调
    expect(w.find('[data-testid="lane-node-4"] [data-role="node-title"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-node-4"]').classes()).not.toContain('is-search-hit')
  })

  it('检索命中层不因阈值被藏起（命中点在真实位置浮出）', async () => {
    const far = section('7', '2026-10-27', '散料较远', [0.7, 0.714])
    const w = await mountRows({
      sections: [...SECTIONS, far],
      selectedIds: new Set(['1', '2', '3']),
      thresholdTier: 'tight',
      searchActive: true,
      searchHits: new Set(['7']),
    })
    expect(w.find('[data-testid="lane-node-7"]').exists()).toBe(true)
    const p = pointOf(w, '7')
    expect(p.y).toBeGreaterThan(rowTop(w, 0))
  })
})

describe('LaneComposeCanvas — 几何自查（双视口列宽 / 不越界不重叠）', () => {
  /** 行标签宽 / 右留白 / 时间列宽下限（与 laneComposeLogic 同口径）。 */
  const LABEL_W = 132
  const RIGHT_PAD = 24
  const MIN_COLUMN = 124

  for (const width of [820, 1300]) {
    it(`容器 ${width}px（1440/1920 三栏扣减）：节点/标题/行标签/日期轴都不越界、互不压字`, async () => {
      const w = await mountRows({ dates: days(14, '2026-10-26'), selectedIds: new Set(['1', '2', '3']) })
      const el = w.find('[data-testid="lane-compose-canvas"]').element as HTMLElement
      Object.defineProperty(el, 'clientWidth', { value: width, configurable: true })
      window.dispatchEvent(new Event('resize'))
      await flushPromises()

      // 新几何：列宽锁 124 下限，内容宽 = 132 + 天数 × 列宽 + 24，可超可视宽
      const columnWidth = Math.max(MIN_COLUMN, (width - LABEL_W - RIGHT_PAD) / 14)
      const expectedVbWidth = Math.max(width, LABEL_W + 14 * columnWidth + RIGHT_PAD)
      const vbWidth = canvasWidth(w)
      expect(vbWidth).toBeCloseTo(expectedVbWidth, 6)
      // 820 视口：14 天就超可视宽（横向滚动）；1300 视口：刚好装下、不滚动
      expect(vbWidth > width).toBe(width < LABEL_W + RIGHT_PAD + 14 * MIN_COLUMN)
      const vbHeight = Number(/^0 0 \d+ (\d+)/.exec(w.find('[data-testid="lane-compose-canvas-svg"]').attributes('viewBox')!)![1])

      // 节点在画布内
      for (const circle of w.findAll('.lcc-canvas__point')) {
        const cx = Number(circle.attributes('cx'))
        const cy = Number(circle.attributes('cy'))
        const r = Number(circle.attributes('r'))
        expect(cx - r).toBeGreaterThanOrEqual(0)
        expect(cx + r).toBeLessThanOrEqual(vbWidth)
        expect(cy).toBeGreaterThanOrEqual(0)
        expect(cy).toBeLessThanOrEqual(vbHeight)
      }

      // 日期轴刻度不越界（.lcc-canvas__ticks 才是日期文字；.lcc-canvas__axis 是列 guide）
      for (const label of w.findAll('.lcc-canvas__ticks text')) {
        const x = Number(label.attributes('x'))
        const half = (label.text().length * 9 * 0.55) / 2
        expect(x - half).toBeGreaterThanOrEqual(0)
        expect(x + half).toBeLessThanOrEqual(vbWidth)
      }

      // 行标签（名称 + 计数）留在左侧标签区，不与首列成员点相撞
      const labelRight = (() => {
        let right = 0
        for (const label of w.findAll('[data-role="row-label"]')) {
          right = Math.max(right, Number(label.attributes('x')) + label.text().length * 11)
        }
        return right
      })()
      const leftmostPoint = Math.min(...w.findAll('.lcc-canvas__point').map(circle => Number(circle.attributes('cx')) - Number(circle.attributes('r'))))
      expect(labelRight).toBeLessThan(leftmostPoint)

      // 节点标题不压相邻列、不越右边界（同列相邻标题纵向至少隔一个槽距 18px）
      const byColumn = new Map<number, number[]>()
      for (const title of w.findAll('[data-role="node-title"]')) {
        const x = Number(title.attributes('x'))
        const y = Number(title.attributes('y'))
        const half = (title.text().length * 11) / 2
        expect(x - half).toBeGreaterThanOrEqual(0)
        expect(x + half).toBeLessThanOrEqual(vbWidth)
        const list = byColumn.get(x) ?? []
        list.push(y)
        byColumn.set(x, list)
      }
      for (const ys of byColumn.values()) {
        const sorted = [...ys].sort((a, b) => a - b)
        for (let i = 1; i < sorted.length; i++) expect(sorted[i]! - sorted[i - 1]!).toBeGreaterThanOrEqual(24)
      }
    })
  }
})

describe('LaneComposeCanvas — 无成型候选空态（必修 1）', () => {
  it('窗口内没有候选链：地板全部散料常驻 + 顶部提示，不出现「低相关散料 N 条」折叠计数', async () => {
    const sections = [
      section('s1', '2026-11-06', '散料一', [1, 0]),
      section('s2', '2026-11-07', '散料二', [0.9, 0.1]),
      section('s3', '2026-11-08', '散料三', [0.5, 0.5]),
    ]
    // 无已选（无质心）+ 标准档：若无空链 override，全部散料会被判未达标而收起
    const w = await mountRows({
      sections,
      chains: [],
      selectedIds: new Set<string>(),
      dates: days(3, '2026-11-06'),
    })
    expect(w.find('[data-testid="lane-compose-no-chain-notice"]').text()).toBe('这个范围没有成型候选')
    // 散料全可见（逐个节点断言）
    for (const id of ['s1', 's2', 's3']) {
      expect(w.find(`[data-testid="lane-node-${id}"]`).exists()).toBe(true)
    }
    // 不再有折叠计数（地板没有「未展开」可数）
    expect(w.find('[data-testid="lane-floor-fold"]').exists()).toBe(false)
    // 仍只有「空的本次泳道行 + 地板」两行
    expect(w.findAll('.lcc-canvas__row')).toHaveLength(2)
    expect(w.find('.lcc-canvas__row[data-row-kind="lane"]').exists()).toBe(true)
    expect(w.find('.lcc-canvas__row[data-row-kind="floor"]').exists()).toBe(true)
  })

  it('「用户把链成员全拖入本次泳道」不算无成型候选：不弹提示、散料仍按档位收起', async () => {
    const w = await mountRows({ selectedIds: new Set(['1', '2', '3']) })
    expect(w.find('[data-testid="lane-compose-no-chain-notice"]').exists()).toBe(false)
    expect(w.findAll('.lcc-canvas__row[data-row-kind="chain"]')).toHaveLength(0)
    // 无已选外的散料（节点 4 靠近重心，可见）；紧档下远处散料仍应被收起
    const tight = await mountRows({ selectedIds: new Set(['1', '2', '3']), thresholdTier: 'tight' })
    expect(tight.find('[data-testid="lane-compose-no-chain-notice"]').exists()).toBe(false)
  })

  it('候选链取数失败（sections 也为空）：不弹提示、走既有空态文案', async () => {
    const w = await mountRows({ sections: [], chains: [], selectedIds: new Set<string>() })
    expect(w.find('[data-testid="lane-compose-no-chain-notice"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-compose-canvas-empty"]').exists()).toBe(true)
  })
})

describe('LaneComposeCanvas — 检索命中标题不降级（必修 2）', () => {
  it('30 天窗口：命中点照常带常显标题（列宽锁 124 → 10 字，不再是 2 字碎片/空档）', async () => {
    const w = await mountRows({
      dates: days(30, '2026-10-26'),
      selectedIds: new Set(['1']),
      searchActive: true,
      searchHits: new Set(['4']),
    })
    const hit = w.find('[data-testid="lane-node-4"]')
    expect(hit.find('[data-role="search-hit"]').exists()).toBe(true)
    const title = hit.find('[data-role="node-title"]')
    expect(title.exists()).toBe(true)
    expect(title.text().length).toBeGreaterThanOrEqual(3)
    expect(title.text().length).toBeLessThanOrEqual(5)
  })

  it('14 天窗口：命中点照常带截断标题（不因必修 2 误伤）', async () => {
    const w = await mountRows({
      dates: days(14, '2026-10-26'),
      selectedIds: new Set(['1']),
      searchActive: true,
      searchHits: new Set(['4']),
    })
    const title = w.find('[data-testid="lane-node-4"] [data-role="node-title"]')
    expect(title.exists()).toBe(true)
    expect(title.text().length).toBeLessThanOrEqual(5)
  })
})

// ── 吸顶（P-1：本次泳道行钉在画布顶部） ─────────────────────────────────────

/** 造 N 条候选链（每条 1 个成员）：内容高 ≈ 32 + 94 + N×76 + 76 > 可视 600px。 */
function denseSections(count = 9) {
  return Array.from({ length: count }, (_, i) => section(`c${i}`, '2026-10-26', `链成员${i}`, [1, 0], `T${i}`))
}

function denseChains(count = 9): ComposeCandidateChain[] {
  return Array.from({ length: count }, (_, i) => ({
    key: `T${i}:${i}`,
    topicId: `T${i}`,
    topicLabel: `候选链${i}`,
    topicStatus: 'candidate',
    sectionIds: [`c${i}`],
    firstDate: '2026-10-26',
    lastDate: '2026-10-26',
    spanDays: 0,
  }))
}

/** 设滚动位置并触发 scroll（happy-dom 不做真实滚动，直接打桩）。 */
async function setCanvasScrollTop(w: ReturnType<typeof mount>, top: number): Promise<void> {
  const el = w.find('[data-testid="lane-compose-canvas"]').element as HTMLElement
  Object.defineProperty(el, 'scrollTop', { value: top, configurable: true, writable: true })
  el.dispatchEvent(new Event('scroll'))
  await flushPromises()
}

/** 设横向滚动位置并触发 scroll。 */
async function setCanvasScrollLeft(w: ReturnType<typeof mount>, left: number): Promise<void> {
  const el = w.find('[data-testid="lane-compose-canvas"]').element as HTMLElement
  Object.defineProperty(el, 'scrollLeft', { value: left, configurable: true, writable: true })
  el.dispatchEvent(new Event('scroll'))
  await flushPromises()
}

/**
 * 行标签列的反向位移（逻辑坐标）：`translate(scrollLeft / zoom, 0)`。
 *
 * 找不到 label 组 / 不下发 transform 时返回 0（= 没横向吸顶）。
 */
function rowLabelOffset(w: ReturnType<typeof mount>, row: number): number {
  const inRow = w.find(`.lcc-canvas__row[data-row="${row}"] [data-role="row-label-group"]`)
  // 吸顶时本次泳道行的标签组画在置顶覆盖层（行循环里已跳过），位移从覆盖层读
  const group = inRow.exists()
    ? inRow
    : w.find('[data-role="lane-label-overlay"] [data-role="row-label-group"]')
  const match = /translate\((-?[\d.]+),\s*(-?[\d.]+)\)/.exec(group.attributes('transform') ?? '')
  return match ? Number(match[1]) : 0
}

/** 吸顶行的反向位移（逻辑坐标）。 */
function pinnedOffset(w: ReturnType<typeof mount>): number {
  const transform = w.find('.lcc-canvas__row[data-row-kind="lane"]').attributes('transform') ?? ''
  const match = /translate\(0,\s*(-?[\d.]+)\)/.exec(transform)
  return match ? Number(match[1]) : 0
}

describe('LaneComposeCanvas — 本次泳道行吸顶（P-1）', () => {
  it('内容溢出 + 滚动：第 0 行仍可见（屏幕顶边 == 画布顶 + 日期轴高度，不随滚动移动）', async () => {
    const w = await mountRows({
      sections: denseSections(),
      chains: denseChains(),
      selectedIds: new Set(['c0']),
    })
    const lane = w.find('.lcc-canvas__row[data-row-kind="lane"]')
    expect(lane.attributes('data-role')).toBe('sticky-row')
    expect(w.find('[data-role="sticky-bg"]').exists()).toBe(true)
    expect(w.find('[data-role="sticky-shadow"]').exists()).toBe(true)

    const bandTop = Number(w.find('.lcc-canvas__row[data-row-kind="lane"] [data-role="row-separator"]').attributes('y1'))
    const viewportTop = 0 // zoom = 1：容器顶 + bandTop*zoom
    const screenTop = (scrollTop: number): number => bandTop * 1 - scrollTop + pinnedOffset(w)
    const before = screenTop(0)
    expect(before).toBeCloseTo(bandTop, 6)
    expect(before).toBeGreaterThanOrEqual(viewportTop)

    await setCanvasScrollTop(w, 400)
    // 反向位移 == scrollTop / zoom（zoom = 1）
    expect(pinnedOffset(w)).toBeCloseTo(400, 6)
    // 屏幕顶边纹丝不动（这正是「bounding rect 顶边 == 可视区顶边」的等价断言）
    expect(screenTop(400)).toBeCloseTo(before, 6)

    await setCanvasScrollTop(w, 700)
    expect(pinnedOffset(w)).toBeCloseTo(700, 6)
    expect(screenTop(700)).toBeCloseTo(before, 6)
  })

  it('内容不溢出：不出现吸顶层（无吸顶底/阴影/位移），与既有渲染等价', async () => {
    const w = await mountRows({ sections: SECTIONS, chains: CHAINS, selectedIds: new Set(['1']) })
    const lane = w.find('.lcc-canvas__row[data-row-kind="lane"]')
    expect(lane.attributes('data-role')).toBeUndefined()
    expect(lane.attributes('transform')).toBeUndefined()
    expect(w.findAll('[data-role="sticky-bg"]')).toHaveLength(0)
    expect(w.findAll('[data-role="sticky-shadow"]')).toHaveLength(0)
    // 每个节点仍然只渲染一次（吸顶没有复制节点）
    for (const id of ['1', '2', '3']) {
      expect(w.findAll(`[data-testid="lane-node-${id}"]`)).toHaveLength(1)
    }
  })

  it('纵横同时滚动：行标签横向位移与吸顶行纵向位移各自生效（同一套逻辑位移）', async () => {
    const w = await mountRows({
      sections: denseSections(),
      chains: denseChains(),
      selectedIds: new Set(['c0']),
      dates: days(30, '2026-10-26'),
    })
    await setCanvasScrollTop(w, 400)
    await setCanvasScrollLeft(w, 200)
    expect(pinnedOffset(w)).toBeCloseTo(400, 6)
    expect(rowLabelOffset(w, 0)).toBeCloseTo(200, 6)
    // 吸顶时本次泳道行的标签组在置顶覆盖层：外层只承担纵向吸顶，内层只承担横向吸顶
    const overlay = w.find('[data-role="lane-label-overlay"]')
    expect(overlay.attributes('transform')).toBe('translate(0, 400)')
    const group = overlay.find('[data-role="row-label-group"]').attributes('transform') ?? ''
    expect(group).toMatch(/^translate\(200, 0\)$/)
  })

  it('滚到地板后把地板节点拖进吸顶行：落点按吸顶行实际位置判定（成员数 +1）', async () => {
    const floorNode = section('z', '2026-10-26', '地板散料', [1, 0])
    const w = await mountRows({
      sections: [...denseSections(), floorNode],
      chains: denseChains(),
      selectedIds: new Set(['c0']),
      thresholdTier: 'all',
    })
    await setCanvasScrollTop(w, 400)
    // 纵横同时滚动时落点判定仍成立（`contentPoint` 只对 y 做换算，x 不参与）
    await setCanvasScrollLeft(w, 120)

    const bandTop = Number(w.find('.lcc-canvas__row[data-row-kind="lane"] [data-role="row-separator"]').attributes('y1'))
    const from = pointOf(w, 'z')
    // 指针停在「视口里看到的吸顶行」上：内容坐标 = clientY + scrollTop 落在吸顶行区间
    const dropY = bandTop + 20
    await drag(from, { x: from.x, y: dropY }, w.find('[data-testid="lane-node-z"]').element)

    const contentY = dropY + 400
    expect(contentY).toBeGreaterThan(bandTop + 400)
    expect(contentY).toBeLessThan(bandTop + 94 + 400)
    expect(w.emitted('toggle-member')?.[0]).toEqual(['z', true])
  })

  it('同一套判定对「移出」成立：从吸顶行拖到滚动区（行外）→ 移出', async () => {
    const w = await mountRows({
      sections: denseSections(),
      chains: denseChains(),
      selectedIds: new Set(['c0']),
    })
    await setCanvasScrollTop(w, 400)
    const laneNode = w.find('[data-testid="lane-node-c0"]')
    const from = pointOf(w, 'c0')
    // 拖到视口下方深处（内容坐标 800 落在候选链行里，非本次泳道）
    await drag(from, { x: from.x, y: 400 }, laneNode.element)
    expect(w.emitted('toggle-member')?.[0]).toEqual(['c0', false])
  })
})

describe('LaneComposeCanvas — 吸顶层界自洽（日期轴不被遮、滚动内容不露出）', () => {
  it('底色从画布顶盖到本行下缘；日期轴刻度画在行之后（浮在底色上）', async () => {
    const w = await mountRows({
      sections: denseSections(),
      chains: denseChains(),
      selectedIds: new Set(['c0']),
    })
    const bg = w.find('[data-role="sticky-bg"]')
    expect(bg.attributes('y')).toBe('0')
    // 底色高度 == 下一条行的顶边（= 本次泳道行下缘），所以滚上去的行不会从日期轴区露出来
    const bandTop = Number(w.find('.lcc-canvas__row[data-row-kind="lane"] [data-role="row-separator"]').attributes('y1'))
    const nextRowTop = Math.min(...w.findAll('[data-role="row-separator"]')
      .map(separator => Number(separator.attributes('y1')))
      .filter(y => y > bandTop))
    expect(Number(bg.attributes('height'))).toBeCloseTo(nextRowTop, 6)

    const rowsGroup = w.find('.lcc-canvas__rows').element
    const ticksGroup = w.find('.lcc-canvas__ticks').element
    expect(rowsGroup.compareDocumentPosition(ticksGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    // 刻度仍只渲染一份，且日期文案在位
    expect(w.findAll('.lcc-canvas__ticks text').length).toBeGreaterThan(0)
  })
})

describe('LaneComposeCanvas — 吸顶态的节点卡（不跑偏）', () => {
  it('滚动后点本次泳道成员：节点卡跟着吸顶行落在可视区，不会被甩到视口上方', async () => {
    const w = await mountRows({
      sections: denseSections(),
      chains: denseChains(),
      selectedIds: new Set(['c0']),
      focusId: 'c0',
    })
    await setCanvasScrollTop(w, 400)
    const style = w.find('[data-testid="lane-node-card"]').attributes('style') ?? ''
    const top = Number(/top:\s*([\d.]+)px/.exec(style)?.[1] ?? '-1')
    const point = pointOf(w, 'c0')
    // 卡片顶部 ≈ (内容 y + scrollTop/zoom) − 卡高/2，夹在容器内
    expect(top).toBeGreaterThan(300)
    expect(top).toBeLessThanOrEqual(Math.max(4, 600 - 120 - 4))
    // 不补位移的话会落在 ≈16px（视口最上方），与「贴在点旁」不符
    expect(Math.abs(top - (point.y + 400 - 60))).toBeLessThan(40)
  })
})

// ── 返工 11.x：不画关系边 / 列宽下限与横向滚动 / 行标签横向吸顶 / 点尺寸 ─────

describe('LaneComposeCanvas — 不生成事件关系边（11.1 / 母 spec）', () => {
  it('画布上没有任何连线或箭头（identity 与 similarity 都不画）', async () => {
    const w = await mountRows({ sections: SECTIONS, relations: RELATIONS })
    expect(w.findAll('.lcc-canvas__edges')).toHaveLength(0)
    expect(w.findAll('.lcc-canvas__edge')).toHaveLength(0)
    expect(w.element.querySelectorAll('path.lcc-canvas__edge')).toHaveLength(0)
    // 二次曲线路径只可能是关系边（勾号/四角括号都是折线）→ 一条都不该有
    for (const path of w.findAll('path')) {
      expect(path.attributes('d') ?? '').not.toContain('Q')
    }
    // 源码锚：边渲染块与 edgePath 一起移除（不是靠 CSS 藏起来）
    const src = readFileSync(`${import.meta.dirname}/LaneComposeCanvas.vue`, 'utf8')
    expect(src).not.toContain('lcc-canvas__edge')
    expect(src).not.toContain('edgePath')
  })

  it('原关系仍以「节点卡条数」与「相关节点标题」表达（能力没丢）', async () => {
    const w = await mountRows({ focusId: '2' })
    const card = w.find('[data-testid="lane-node-card"]')
    expect(card.exists()).toBe(true)
    expect(card.text()).toContain('2 条原关系')
    expect(card.text()).not.toContain('连出')
    expect(w.findAll('.lcc-canvas__related-title').length).toBeGreaterThan(0)
  })
})

describe('LaneComposeCanvas — 列宽下限与横向滚动（11.2）', () => {
  it('30 天窄容器：列宽锁 124、内容宽 > 容器宽（靠横向滚动，不缩字）', async () => {
    const w = await mountRows({ dates: days(30, '2026-10-26') })
    // 内容宽 = 行标签 132 + 30 × 124 + 右留白 24
    expect(canvasWidth(w)).toBe(132 + 30 * 124 + 24)
    expect(canvasWidth(w)).toBeGreaterThan(900)
    // 相邻日期列的 x 间距 = 52（列宽没被压）
    const xs = ['1', '2', '3'].map(id => pointOf(w, id).x)
    expect(xs[1]! - xs[0]!).toBeCloseTo(124, 6)
    // SVG 尺寸按内容宽下发 → 容器真的可滚
    const svgWidth = Number(w.find('[data-testid="lane-compose-canvas-svg"]').attributes('width'))
    expect(svgWidth).toBe(132 + 30 * 124 + 24)
  })

  it('不溢出时行为与提档前一致：viewBox 宽 == 容器宽、无横向吸顶位移', async () => {
    const w = await mountRows({ dates: DATES })
    expect(canvasWidth(w)).toBe(900)
    expect(rowLabelOffset(w, 0)).toBe(0)
    expect(w.findAll('[data-role="row-label-bg"]')).toHaveLength(0)
    // 落点框仍是「可视宽 − 左右各 6」
    const dropzone = w.find('.lcc-canvas__row[data-row-kind="lane"] .lcc-canvas__dropzone')
    expect(Number(dropzone.attributes('x'))).toBe(6)
    expect(Number(dropzone.attributes('width'))).toBeCloseTo(900 - 12, 6)
  })
})

describe('LaneComposeCanvas — 行标签列横向吸顶（11.3）', () => {
  it('整组位移 == scrollLeft / zoom；底与 6 级渐隐阴影跟着走', async () => {
    const w = await mountRows({ dates: days(30, '2026-10-26') })
    await setCanvasScrollLeft(w, 200)
    expect(rowLabelOffset(w, 0)).toBeCloseTo(200, 6)
    expect(w.find('[data-role="row-label-bg"]').exists()).toBe(true)
    // 每个标签列区段各一套 6 级渐隐（band 1 带 data-role 作锚）
    expect(w.findAll('[data-role="row-label-shadow"]')).toHaveLength(w.findAll('[data-role="row-label-group"]').length)

    // 缩放后同一套换算：scrollLeft 220 / zoom 1.1 = 200
    await wheel(-100)
    await setCanvasScrollLeft(w, 220)
    expect(rowLabelOffset(w, 0)).toBeCloseTo(200, 6)
  })

  it('标签列画在节点之后（滑到标签下面的点/标题被不透明底盖住）', async () => {
    const w = await mountRows({ dates: days(30, '2026-10-26') })
    const row = w.find('.lcc-canvas__row[data-row-kind="lane"]').element
    const group = row.querySelector('[data-role="row-label-group"]')
    expect(group).not.toBeNull()
    const nodes = [...row.querySelectorAll('.lcc-canvas__node')]
    expect(nodes.length).toBeGreaterThan(0)
    for (const node of nodes) {
      expect(node.compareDocumentPosition(group!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    }
  })

  it('拖拽热区在标签组内同步平移（整组拖的落点不跑偏）', async () => {
    const w = await mountRows({ dates: days(30, '2026-10-26'), selectedIds: new Set<string>() })
    const handle = () => w.find('[data-role="row-handle"][data-row="1"]')
    // 热区在标签组里：几何由组的 transform 平移，自身 x 不重算（不会与标签错位）
    expect(handle().element.closest('[data-role="row-label-group"]')).not.toBeNull()
    expect(Number(handle().attributes('x'))).toBe(4)
    await setCanvasScrollLeft(w, 260)
    expect(rowLabelOffset(w, 1)).toBeCloseTo(260, 6)
    expect(Number(handle().attributes('x'))).toBe(4)
  })

  it('日期刻度只纵向吸顶、横向随内容移动（不给刻度加横向位移）', async () => {
    const w = await mountRows({
      sections: denseSections(),
      chains: denseChains(),
      selectedIds: new Set(['c0']),
      dates: days(30, '2026-10-26'),
    })
    await setCanvasScrollTop(w, 300)
    await setCanvasScrollLeft(w, 300)
    const axis = w.find('[data-role="date-axis"]')
    expect(axis.attributes('transform')).toBe('translate(0, 300)')
    // 刻度 x 仍是内容坐标（跟着日期列走）
    const xs = w.findAll('.lcc-canvas__ticks text').map(text => Number(text.attributes('x')))
    expect(Math.min(...xs)).toBeGreaterThanOrEqual(132)
  })
})

describe('LaneComposeCanvas — 点尺寸提档（11.5）', () => {
  it('散料 11px / 推荐 12.4–14px / 已选 15px，推荐 ≤ 已选', async () => {
    const far = section('9', '2026-10-27', '远处散料', [0, 1])
    const w = await mountRows({ sections: [...SECTIONS, far], selectedIds: new Set(['1', '2', '3']) })
    const radiusOf = (id: string): number => Number(w.find(`[data-testid="lane-node-${id}"] .lcc-canvas__point`).attributes('r'))
    // 已选 15px：半径 7.5
    expect(radiusOf('1')).toBeCloseTo(7.5, 6)
    // 推荐（近邻 4，相对归一后是批次里最近的那个）压在「已选 − 0.5」上限 → 14px；下限 12.4 由 11.5 下限公式保证
    expect(radiusOf('4')).toBeGreaterThanOrEqual(6.2)
    expect(radiusOf('4')).toBeLessThanOrEqual(7.5)
    expect(radiusOf('4')).toBeLessThanOrEqual(radiusOf('1'))

    // 远离质心的散料展开后可见：接近度 0 → 半径区间下界 5.9（11.8px）
    await w.find('[data-testid="lane-floor-fold"]').trigger('click')
    await flushPromises()
    expect(Number(w.find('[data-testid="lane-node-9"]').attributes('data-proximity'))).toBe(0)
    expect(radiusOf('9')).toBeCloseTo(5.9, 6)
  })
})

describe('LaneComposeCanvas — 无质心时散料仍是 5.5（相对归一只在有距离时接管）', () => {
  it('无已选 → 无质心 → 全部散料 5.5、无接近度', async () => {
    const w = await mountRows({
      sections: [section('a', '2026-10-27', '散料一', [1, 0]), section('b', '2026-10-27', '散料二', [0.8, 0.6])],
      chains: [],
      selectedIds: new Set<string>(),
      thresholdTier: 'all',
    })
    for (const id of ['a', 'b']) {
      expect(Number(w.find(`[data-testid="lane-node-${id}"] .lcc-canvas__point`).attributes('r'))).toBeCloseTo(5.5, 6)
      expect(w.find(`[data-testid="lane-node-${id}"]`).attributes('data-proximity')).toBe('')
    }
  })
})

describe('LaneComposeCanvas — 落点框覆盖可视宽（11.7）', () => {
  it('落点框宽度 = 可视宽 − 12（不是内容宽），且随横向滚动跟随视口', async () => {
    const w = await mountRows({ dates: days(30, '2026-10-26') })
    const dropzone = () => w.find('.lcc-canvas__row[data-row-kind="lane"] .lcc-canvas__dropzone')
    expect(Number(dropzone().attributes('width'))).toBeCloseTo(900 - 12, 6)
    await setCanvasScrollLeft(w, 300)
    expect(Number(dropzone().attributes('x'))).toBe(6 + 300)
    expect(Number(dropzone().attributes('width'))).toBeCloseTo(900 - 12, 6)
  })
})

describe('LaneComposeCanvas — 横向滚动后的拖拽坐标（11.7）', () => {
  it('contentPoint 含 scrollLeft：ghost 用内容坐标（= 指针屏幕位 + scrollLeft），松手落点不受横向偏移影响', async () => {
    const w = await mountRows({ dates: days(30, '2026-10-26'), selectedIds: new Set<string>() })
    await setCanvasScrollLeft(w, 200)
    const surface = w.find('[data-testid="lane-compose-canvas"]')
    const nodeEl = w.find('[data-testid="lane-node-1"]').element
    const down = new Event('pointerdown', { bubbles: true }) as PointerEvent
    Object.defineProperties(down, { clientX: { value: 400 }, clientY: { value: 200 }, pointerId: { value: 1 } })
    nodeEl.dispatchEvent(down)
    const move = new Event('pointermove', { bubbles: true }) as PointerEvent
    Object.defineProperties(move, { clientX: { value: 420 }, clientY: { value: 210 }, pointerId: { value: 1 } })
    surface.element.dispatchEvent(move)
    await flushPromises()

    const style = w.find('[data-role="drag-ghost"]').attributes('style') ?? ''
    // zoom = 1：ghost left = clientX（420）+ scrollLeft（200）= 内容坐标 620
    expect(Number(/left:\s*([\d.]+)px/.exec(style)?.[1])).toBeCloseTo(620, 6)

    const up = new Event('pointerup', { bubbles: true }) as PointerEvent
    Object.defineProperties(up, { clientX: { value: 420 }, clientY: { value: 210 }, pointerId: { value: 1 } })
    surface.element.dispatchEvent(up)
    await flushPromises()
  })
})

// ── 2026-11-21 用户实拍反馈：吸顶带与内容重叠 / 接近度映射被压没 ─────────────

describe('LaneComposeCanvas — 刻度不侵入吸顶标签列（缺陷 1-1）', () => {
  it('横向溢出：刻度组与列 guide 组都挂 clipPath，左界 = 标签列右缘 LABEL_W', async () => {
    const w = await mountRows({ dates: days(30, '2026-10-26') })
    const ticks = w.find('[data-role="date-axis"]')
    const guides = w.find('[data-role="column-guides"]')
    for (const group of [ticks, guides]) {
      expect(group.attributes('clip-path') ?? '').toMatch(/^url\(#.+\)$/)
    }
    // 同一个裁切框：x 从 132 起、宽度到内容右缘
    const clipRect = w.find('[data-role="axis-clip"] rect')
    expect(Number(clipRect.attributes('x'))).toBeCloseTo(132, 6)
    expect(Number(clipRect.attributes('width'))).toBeCloseTo(canvasWidth(w) - 132, 6)
    // 刻度字全在裁切区右侧（第一列中心 = 132 + 26）
    const xs = w.findAll('.lcc-canvas__ticks text').map(text => Number(text.attributes('x')))
    expect(Math.min(...xs)).toBeGreaterThan(132)
  })

  it('横向滚动后裁切框跟着滚：视口左界恒为 132（刻度不会滑到标签列上方）', async () => {
    const w = await mountRows({ dates: days(30, '2026-10-26') })
    const clipRect = () => w.find('[data-role="axis-clip"] rect')
    expect(Number(clipRect().attributes('x'))).toBeCloseTo(132, 6)
    await setCanvasScrollLeft(w, 300)
    // 视口 132 在内容坐标 = 132 + scrollLeft/zoom
    expect(Number(clipRect().attributes('x'))).toBeCloseTo(432, 6)
    expect(Number(clipRect().attributes('width'))).toBeCloseTo(canvasWidth(w) - 432, 6)
  })

  it('不横向溢出：不下发 clipPath（01–03 态渲染与旧版一致）', async () => {
    const w = await mountRows({ dates: DATES })
    expect(w.find('[data-role="date-axis"]').attributes('clip-path')).toBeUndefined()
    expect(w.find('[data-role="column-guides"]').attributes('clip-path')).toBeUndefined()
  })
})

describe('LaneComposeCanvas — 吸顶带分界与置顶标签层（缺陷 1-2 / 1-3）', () => {
  it('吸顶带底色提一档（不透明 elevated 叠在 bg-base 之上，不用半透明）', async () => {
    const w = await mountRows({
      sections: denseSections(),
      chains: denseChains(),
      selectedIds: new Set(['c0']),
      dates: days(30, '2026-10-26'),
    })
    const bg = w.find('[data-role="sticky-bg"]')
    const tint = w.find('[data-role="sticky-tint"]')
    expect(bg.exists()).toBe(true)
    expect(tint.exists()).toBe(true)
    // 两层同尺寸：底层不透明遮内容，上层只负责提色阶
    for (const rect of [bg, tint]) {
      expect(rect.attributes('y')).toBe('0')
      expect(rect.attributes('x')).toBe('0')
      expect(rect.attributes('height')).toBe(bg.attributes('height'))
      expect(rect.attributes('width')).toBe(bg.attributes('width'))
    }
    // 标签列底色同一档（两条吸顶轴看起来是一块固定框）
    expect(w.findAll('[data-role="row-label-tint"]').length).toBeGreaterThan(0)
    // 带下缘 1px 硬分隔 + 标签列/内容区竖向分隔
    expect(w.find('[data-role="sticky-edge"]').exists()).toBe(true)
    expect(w.find('[data-role="label-divider"]').exists()).toBe(true)

    // 源码锚：提档底色用 --color-bg-elevated，且规则里不出现 transparent / 半透明 rgba
    const css = readFileSync(`${import.meta.dirname}/LaneComposeCanvas.vue`, 'utf8')
    const rule = /\.lcc-canvas__sticky-tint\s*\{[^}]*\}/.exec(css)?.[0] ?? ''
    expect(rule).toContain('var(--color-bg-elevated)')
    expect(rule).not.toContain('transparent')
    expect(rule).not.toMatch(/rgba?\([^)]*,\s*0?\.\d+\s*\)/)
  })

  it('吸顶带高度整条覆盖：从画布顶盖到本次泳道行下缘（row.top + row.height）', async () => {
    const w = await mountRows({ sections: denseSections(), chains: denseChains(), selectedIds: new Set(['c0']) })
    const bandBottom = Number(w.find('.lcc-canvas__row[data-row-kind="lane"] [data-role="sticky-edge"]').attributes('y1'))
    const laneRow = w.find('.lcc-canvas__row[data-row-kind="lane"]')
    const rowTop = Number(laneRow.find('[data-role="row-separator"]').attributes('y1'))
    expect(Number(w.find('[data-role="sticky-bg"]').attributes('height'))).toBeCloseTo(bandBottom, 6)
    expect(bandBottom).toBeGreaterThan(rowTop)
  })

  it('本次泳道行标签组只出现一次：溢出时改画到置顶覆盖层（行循环里跳过）', async () => {
    const w = await mountRows({ sections: denseSections(), chains: denseChains(), selectedIds: new Set(['c0']) })
    const rows = w.findAll('.lcc-canvas__row').length
    // 每一行恰好一个标签名，总数与行数一致
    expect(w.findAll('[data-role="row-label"]')).toHaveLength(rows)
    const overlay = w.find('[data-role="lane-label-overlay"]')
    expect(overlay.exists()).toBe(true)
    // 行里不再画（避免重复绘制 = 两份 DOM）
    expect(w.find('.lcc-canvas__row[data-row-kind="lane"] [data-role="row-label"]').exists()).toBe(false)
    expect(overlay.find('[data-role="row-label"]').text()).toBe('本次泳道')
    // 热区仍挂在标签组里（整组拖的落点不因搬家失效）
    for (const handle of w.findAll('[data-role="row-handle"]')) {
      expect(handle.element.closest('[data-role="row-label-group"]')).not.toBeNull()
    }
  })

  it('z 序：覆盖层在所有行之后、日期刻度之前（刻度仍最上，但被裁到标签列右侧）', async () => {
    const w = await mountRows({
      sections: denseSections(),
      chains: denseChains(),
      selectedIds: new Set(['c0']),
      dates: days(30, '2026-10-26'),
    })
    const rowsGroup = w.find('.lcc-canvas__rows').element
    const overlay = w.find('[data-role="lane-label-overlay"]').element
    const divider = w.find('[data-role="label-divider"]').element
    const ticksGroup = w.find('.lcc-canvas__ticks').element
    expect(rowsGroup.compareDocumentPosition(overlay) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(overlay.compareDocumentPosition(ticksGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    // 竖向分隔画在标签层之上（不被标签底色的渐隐阴影糊掉）
    expect(overlay.compareDocumentPosition(divider) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(divider.compareDocumentPosition(ticksGroup) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('节点点仍留在行循环里（搬的只是标签组，不复制节点）', async () => {
    const w = await mountRows({ sections: denseSections(), chains: denseChains(), selectedIds: new Set(['c0']) })
    const laneRow = w.find('.lcc-canvas__row[data-row-kind="lane"]')
    expect(laneRow.find('[data-testid="lane-node-c0"]').exists()).toBe(true)
    expect(w.find('[data-role="lane-label-overlay"] [data-testid="lane-node-c0"]').exists()).toBe(false)
  })

  it('不溢出：不出现覆盖层/提档底/竖向分隔，标签组仍留在行里（旧渲染不变）', async () => {
    const w = await mountRows({ sections: SECTIONS, chains: CHAINS, selectedIds: new Set(['1']) })
    expect(w.findAll('[data-role="lane-label-overlay"]')).toHaveLength(0)
    expect(w.findAll('[data-role="sticky-tint"]')).toHaveLength(0)
    expect(w.findAll('[data-role="row-label-tint"]')).toHaveLength(0)
    expect(w.findAll('[data-role="label-divider"]')).toHaveLength(0)
    expect(w.findAll('[data-role="sticky-edge"]')).toHaveLength(0)
    expect(w.find('.lcc-canvas__row[data-row-kind="lane"] [data-role="row-label"]').exists()).toBe(true)
  })
})

describe('LaneComposeCanvas — 接近度映射可见化（缺陷 2）', () => {
  it('编排画布推荐半径随接近度铺开（5.9–7.4），恒 < 已选 7.5；阈值外的点在同一条梯度上', async () => {
    // 质心 [1,0]：近邻 cos≈0.9949（接近度 0.983）、中档 [0.8,0.6]（接近度 0.80）、远邻超出阈值
    const near = section('n', '2026-10-27', '近邻', [0.99, 0.1])
    const mid = section('f', '2026-10-27', '中档', [0.8, 0.6])
    const out = section('x', '2026-10-27', '远处散料', [0, 1])
    const w = await mountRows({
      sections: [section('m', '2026-10-26', '成员', [1, 0]), near, mid, out],
      relations: [],
      chains: [],
      selectedIds: new Set(['m']),
      thresholdTier: 'all',
    })
    const radiusOf = (id: string): number =>
      Number(w.find(`[data-testid="lane-node-${id}"] .lcc-canvas__point`).attributes('r'))
    const proximityOf = (id: string): number =>
      Number(w.find(`[data-testid="lane-node-${id}"]`).attributes('data-proximity'))
    const strengthOf = (id: string): number => {
      const style = w.find(`[data-testid="lane-node-${id}"]`).attributes('style') ?? ''
      return Number(/--proximity-fill:\s*([\d.]+)%/.exec(style)?.[1] ?? '-1')
    }

    // 已选 15px 直径（r 7.5），推荐按接近度线性铺在 5.9–7.4，且严格小于已选
    expect(radiusOf('m')).toBeCloseTo(7.5, 6)
    expect(radiusOf('n')).toBeCloseTo(5.9 + 1.5 * proximityOf('n'), 6)
    expect(radiusOf('f')).toBeCloseTo(5.9 + 1.5 * proximityOf('f'), 6)
    expect(radiusOf('n')).toBeGreaterThan(radiusOf('f'))
    expect(radiusOf('n') - radiusOf('f')).toBeGreaterThan(0.2)
    expect(radiusOf('n')).toBeLessThanOrEqual(7.4 + 1e-9)
    expect(radiusOf('n')).toBeLessThan(radiusOf('m'))
    // 阈值外的散料也在同一条相对归一梯度上：最远 → 5.9（11.8px）；无质心时才回到 5.5
    expect(radiusOf('x')).toBeCloseTo(5.9, 6)
    expect(radiusOf('x')).toBeLessThan(radiusOf('f'))

    // 深浅区间 18%–80%（旧档 12%–67%）
    expect(strengthOf('n')).toBeGreaterThan(strengthOf('f'))
    expect(strengthOf('f')).toBeGreaterThanOrEqual(18)
    expect(strengthOf('n')).toBeLessThanOrEqual(80)
    expect(strengthOf('x')).toBeCloseTo(18, 6)
  })
})

// ── 2026-11-24 用户实拍反馈：画布看不出远近（接近度要相对归一） ────────────────

describe('LaneComposeCanvas — 接近度相对归一（缺陷 A）', () => {
  /** 质心 [1,0] 下距离 0.59 / 0.63 / 0.67 的三个点（cos = 1 − d）。 */
  const normalized = () => [
    section('m', '2026-10-26', '成员', [1, 0]),
    section('a', '2026-10-27', '最近', [0.41, 0.91208]),
    section('b', '2026-10-27', '中距', [0.37, 0.92903]),
    section('c', '2026-10-27', '最远', [0.33, 0.94398]),
  ]

  it('批次内 min-max：最近 1 → 7.4/80%、中间 0.5 → 6.65/49%、最远 0 → 5.9/18%', async () => {
    const w = await mountRows({
      sections: normalized(),
      relations: [],
      chains: [],
      selectedIds: new Set(['m']),
      thresholdTier: 'all',
    })
    const proximityOf = (id: string): number => Number(w.find(`[data-testid="lane-node-${id}"]`).attributes('data-proximity'))
    const radiusOf = (id: string): number => Number(w.find(`[data-testid="lane-node-${id}"] .lcc-canvas__point`).attributes('r'))
    const strengthOf = (id: string): number => {
      const style = w.find(`[data-testid="lane-node-${id}"]`).attributes('style') ?? ''
      return Number(/--proximity-fill:\s*([\d.]+)%/.exec(style)?.[1] ?? '-1')
    }

    expect(proximityOf('a')).toBeCloseTo(1, 6)
    expect(proximityOf('b')).toBeCloseTo(0.5, 3)
    expect(proximityOf('c')).toBeCloseTo(0, 6)
    // 半径铺开 1.5px（旧绝对除法在这批数据上只差 0.03px）
    expect(radiusOf('a')).toBeCloseTo(7.4, 3)
    expect(radiusOf('b')).toBeCloseTo(6.65, 3)
    expect(radiusOf('c')).toBeCloseTo(5.9, 3)
    expect(radiusOf('a') - radiusOf('c')).toBeGreaterThan(1)
    // 深浅同样拉开 18%–80%
    expect(strengthOf('a')).toBeCloseTo(80, 3)
    expect(strengthOf('b')).toBeCloseTo(49, 3)
    expect(strengthOf('c')).toBeCloseTo(18, 3)
    expect(strengthOf('a') - strengthOf('c')).toBeGreaterThan(50)
    // 已选点不在这条梯度上：恒 7.5、无接近度
    expect(radiusOf('m')).toBeCloseTo(7.5, 6)
    expect(w.find('[data-testid="lane-node-m"]').attributes('data-proximity')).toBe('')
  })

  it('批次只有 1 个点 → 接近度 0.5（不把所有人推到 0 或 1）', async () => {
    const w = await mountRows({
      sections: [section('m', '2026-10-26', '成员', [1, 0]), section('a', '2026-10-27', '单点', [0.41, 0.91208])],
      relations: [],
      chains: [],
      selectedIds: new Set(['m']),
      thresholdTier: 'all',
    })
    expect(Number(w.find('[data-testid="lane-node-a"]').attributes('data-proximity'))).toBeCloseTo(0.5, 6)
    expect(Number(w.find('[data-testid="lane-node-a"] .lcc-canvas__point').attributes('r'))).toBeCloseTo(6.65, 6)
  })

  it('距离不可算（无 embedding）的点不进批次、保持 ordinary 外观', async () => {
    const noEmbedding = { ...section('z', '2026-10-27', '无向量', []), embedding: [] as number[] }
    const w = await mountRows({
      sections: [...normalized(), noEmbedding],
      relations: [],
      chains: [],
      selectedIds: new Set(['m']),
      thresholdTier: 'all',
    })
    expect(w.find('[data-testid="lane-node-z"]').attributes('data-proximity')).toBe('')
    expect(Number(w.find('[data-testid="lane-node-z"] .lcc-canvas__point').attributes('r'))).toBeCloseTo(5.5, 6)
    // 不参与批次：不影响别人的 min/max（a 仍是最远……不对，a 仍 = 1）
    expect(Number(w.find('[data-testid="lane-node-a"]').attributes('data-proximity'))).toBeCloseTo(1, 6)
  })
})

// ── 2026-11-24 用户实拍反馈：同一行有的点有标题、有的没有 ─────────────────────

describe('LaneComposeCanvas — 统一标题（缺陷 B）', () => {
  it('散料只要放得下就有标题；同一节点标题只出现一次', async () => {
    const w = await mountRows({
      sections: [
        section('m', '2026-10-26', '成员事件一', [1, 0]),
        section('s1', '2026-10-27', '散料事件甲', [0.9, 0.1]),
        section('s2', '2026-10-28', '散料事件乙', [0.8, 0.6]),
      ],
      relations: [],
      chains: [],
      selectedIds: new Set(['m']),
      thresholdTier: 'all',
    })
    for (const id of ['m', 's1', 's2']) {
      expect(w.findAll(`[data-testid="lane-node-${id}"] [data-role="node-title"]`)).toHaveLength(1)
    }
    expect(w.find('[data-testid="lane-node-s1"] [data-role="node-title"]').text()).toBe('散料事件甲')
    // 全画布唯一一套：标题数 == 非只读节点数（没有第二套相关标题）
    expect(w.findAll('[data-role="node-title"]')).toHaveLength(3)
  })

  it('标题两两不重叠（同日密排 + 相邻列也不叠字）', async () => {
    const sections = [
      ...['0', '1', '2', '3'].map(i => section(`d${i}`, '2026-10-26', `同日事件${i}号`, [1, 0])),
      section('e0', '2026-10-27', '次日事件甲', [0.9, 0.1]),
      section('e1', '2026-10-27', '次日事件乙', [0.8, 0.6]),
    ]
    const w = await mountRows({ sections, relations: [], chains: [], selectedIds: new Set(['d0']), thresholdTier: 'all' })
    const boxes = w.findAll('[data-role="node-title"]').map((title) => {
      const text = title.text()
      const w2 = estimateTextWidth(text, 11)
      const x = Number(title.attributes('x'))
      const y = Number(title.attributes('y'))
      return { x: x - w2 / 2, y: y - 14, w: w2, h: 14 }
    })
    expect(boxes.length).toBeGreaterThanOrEqual(5)
    for (let i = 0; i < boxes.length; i++) {
      for (let j = i + 1; j < boxes.length; j++) {
        const a = boxes[i]!
        const b = boxes[j]!
        const overlap = a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h
        expect(overlap).toBe(false)
      }
    }
  })

  it('只读参照梯子（密排）不画标题', async () => {
    const readonly = section('r1', '2026-10-27', '已有泳道事件', [0.9, 0.1], '1984', 'active')
    const w = await mountRows({
      sections: [...SECTIONS, readonly],
      selectedIds: new Set(['1']),
      showAssigned: true,
      thresholdTier: 'all',
    })
    expect(w.find('[data-testid="lane-node-r1"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-node-r1"] [data-role="node-title"]').exists()).toBe(false)
  })
})
