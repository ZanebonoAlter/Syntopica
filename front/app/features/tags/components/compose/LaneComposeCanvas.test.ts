import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import LaneComposeCanvas from './LaneComposeCanvas.vue'
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
    for (const bar of w.findAll('.lcc-canvas__band-bar')) {
      const x = Number(bar.attributes('x'))
      const width = Number(bar.attributes('width'))
      expect(x).toBeGreaterThanOrEqual(0)
      expect(x + width).toBeLessThanOrEqual(vbWidth)
    }

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
