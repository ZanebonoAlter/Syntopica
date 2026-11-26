/**
 * rangeBrush 纯状态机测试（矩阵阶段 G2，test-cases-matrix-stage.md S9 步 11/14 + OX 边界）。
 *
 *  - new：正向/反向拖都归一为 [min,max] 合法区间。
 *  - pan：选区整体平移、宽度不变、池首/池尾钳制。
 *  - start/end：不越过对端、不出池。
 *  - 指针映射：越界钳制、零宽 rect 兜底。
 */
import { describe, expect, it } from 'vitest'
import {
  beginBrushGesture,
  brushModeForTarget,
  buildHitRegions,
  moveBrushGesture,
  pointerIndexFromClientX,
  resolveHitEndpoint,
} from './rangeBrush'

const N = 30

describe('rangeBrush — 命中判定', () => {
  it('手柄 → start/end；选区内 → pan；空白 → new', () => {
    expect(brushModeForTarget('start', 0, 10, 20)).toBe('start')
    expect(brushModeForTarget('end', 0, 10, 20)).toBe('end')
    expect(brushModeForTarget(null, 15, 10, 20)).toBe('pan')
    expect(brushModeForTarget(null, 5, 10, 20)).toBe('new')
    // 贴边：按下点恰在选区端点也算选区内平移
    expect(brushModeForTarget(null, 10, 10, 20)).toBe('pan')
    expect(brushModeForTarget(null, 20, 10, 20)).toBe('pan')
  })
})

describe('rangeBrush — new 刷选', () => {
  it('正向拖：origin=5 → 12 得 [5,12]', () => {
    let gesture = beginBrushGesture('new', 5, 10, 20, N)
    gesture = moveBrushGesture(gesture, 12, N)
    expect([gesture.start, gesture.end]).toEqual([5, 12])
  })

  it('反向拖仍合法：origin=12 → 5 得 [5,12]', () => {
    let gesture = beginBrushGesture('new', 12, 10, 20, N)
    gesture = moveBrushGesture(gesture, 5, N)
    expect([gesture.start, gesture.end]).toEqual([5, 12])
  })

  it('越界指针钳制在池内：拖出左/右边界', () => {
    let gesture = beginBrushGesture('new', 3, 10, 20, N)
    gesture = moveBrushGesture(gesture, -50, N)
    expect([gesture.start, gesture.end]).toEqual([0, 3])
    gesture = moveBrushGesture(gesture, 999, N)
    expect([gesture.start, gesture.end]).toEqual([3, N - 1])
  })
})

describe('rangeBrush — pan 平移', () => {
  it('选区整体位移、宽度不变', () => {
    let gesture = beginBrushGesture('pan', 15, 10, 20, N)
    gesture = moveBrushGesture(gesture, 18, N)
    expect([gesture.start, gesture.end]).toEqual([13, 23])
  })

  it('池首/池尾钳制：左移到顶/右移到底不再动', () => {
    let gesture = beginBrushGesture('pan', 5, 3, 9, N)
    gesture = moveBrushGesture(gesture, 0, N)
    expect([gesture.start, gesture.end]).toEqual([0, 6])
    gesture = moveBrushGesture(gesture, 29, N)
    expect([gesture.start, gesture.end]).toEqual([N - 7, N - 1])
  })
})

describe('rangeBrush — 双端手柄', () => {
  it('start 端不越过 end；end 端不越过 start', () => {
    let gesture = beginBrushGesture('start', 10, 10, 20, N)
    gesture = moveBrushGesture(gesture, 25, N)
    expect([gesture.start, gesture.end]).toEqual([20, 20])
    gesture = moveBrushGesture(gesture, 0, N)
    expect([gesture.start, gesture.end]).toEqual([0, 20])

    let endGesture = beginBrushGesture('end', 20, 10, 20, N)
    endGesture = moveBrushGesture(endGesture, 3, N)
    expect([endGesture.start, endGesture.end]).toEqual([10, 10])
    endGesture = moveBrushGesture(endGesture, 99, N)
    expect([endGesture.start, endGesture.end]).toEqual([10, N - 1])
  })

  it('begin 记录原选区（取消恢复用）', () => {
    const gesture = beginBrushGesture('new', 5, 10, 20, N)
    expect(gesture.originalStart).toBe(10)
    expect(gesture.originalEnd).toBe(20)
  })

  it('R40：抓取偏移保留，首动不会把端点对齐指针', () => {
    let start = beginBrushGesture('start', 13, 10, 20, N)
    expect(start.grabOffset).toBe(3)
    start = moveBrushGesture(start, 14, N)
    expect([start.start, start.end]).toEqual([11, 20])

    let end = beginBrushGesture('end', 17, 10, 20, N)
    expect(end.grabOffset).toBe(-3)
    end = moveBrushGesture(end, 16, N)
    expect([end.start, end.end]).toEqual([10, 19])
  })
})

describe('rangeBrush — 指针坐标映射', () => {
  it('真实绘图区 rect 线性映射 + 越界钳制', () => {
    const rect = { left: 100, width: 400 }
    expect(pointerIndexFromClientX(100, rect, N)).toBe(0)
    expect(pointerIndexFromClientX(300, rect, N)).toBe(15)
    expect(pointerIndexFromClientX(500, rect, N)).toBe(N - 1)
    expect(pointerIndexFromClientX(50, rect, N)).toBe(0)
    expect(pointerIndexFromClientX(0, rect, N)).toBe(0)
  })

  it('单点轴/零宽 rect 兜底返回 0', () => {
    expect(pointerIndexFromClientX(999, { left: 0, width: 100 }, 1)).toBe(0)
    expect(pointerIndexFromClientX(999, { left: 0, width: 0 }, N)).toBe(0)
  })
})


describe('R37/R40 — 24px 手柄命中区与最近可见端', () => {
  it('间隔≥24px：两端各自 24px 区，边界夹回绘图区', () => {
    const regions = buildHitRegions(0.5, 0.8, 400)
    expect(regions.map(region => region.kind)).toEqual(['start', 'end'])
    expect(regions[0]!.style.width).toBe('24px')
    expect(regions[1]!.style.width).toBe('24px')

    const left = buildHitRegions(0, 1, 400)
    expect(left[0]!.style.left).toBe('0px')
    expect(left[1]!.style.left).toBe('376px')
  })

  it('375 等效 7/14 日短窗：合并区仍 24px+间距，左右可见线中点解析到各自端点', () => {
    const width = 375
    const rect = { left: 0, top: 0, width, height: 80 }
    for (const days of [7, 14]) {
      const startFraction = 0.9
      const endFraction = startFraction + days / 365
      const regions = buildHitRegions(startFraction, endFraction, width)
      expect(regions).toHaveLength(1)
      expect(regions[0]!.kind).toBe('overlap')
      const px = Number.parseFloat(regions[0]!.style.width!)
      expect(px).toBeCloseTo(days / 365 * width + 24, 4)
      const startX = startFraction * width
      const endX = endFraction * width
      expect(resolveHitEndpoint('overlap', startX, 40, rect, startFraction, endFraction)).toBe('start')
      expect(resolveHitEndpoint('overlap', endX, 40, rect, startFraction, endFraction)).toBe('end')
    }
  })

  it('夹边导致 24px 区实际重叠：合并区按最近端（左夹偏向 end、右夹偏向 start）', () => {
    const width = 400
    const rect = { left: 0, top: 0, width, height: 80 }

    // 起点贴左边界：start [0,24]、end [16.6,40.6]，重叠 16.6–24 更近 end
    const leftClamped = buildHitRegions(0, 26 / 364, width)
    expect(leftClamped).toHaveLength(1)
    expect(leftClamped[0]!.kind).toBe('overlap')
    expect(resolveHitEndpoint('overlap', 18, 40, rect, 0, 26 / 364)).toBe('end')
    expect(resolveHitEndpoint('overlap', 5, 40, rect, 0, 26 / 364)).toBe('start')

    // 终点贴右边界：start [371.4,395.4]、end [376,400]，重叠 376–395.4 更近 start
    const rightClamped = buildHitRegions(338 / 364, 1, width)
    expect(rightClamped).toHaveLength(1)
    expect(rightClamped[0]!.kind).toBe('overlap')
    expect(resolveHitEndpoint('overlap', 380, 40, rect, 338 / 364, 1)).toBe('start')
    expect(resolveHitEndpoint('overlap', 398, 40, rect, 338 / 364, 1)).toBe('end')
  })

  it('单日同一 x：上/下半区分别起/止；热区外缘 1px 首动不跳一截', () => {
    const width = 375
    const rect = { left: 0, top: 0, width, height: 80 }
    const fraction = 0.5
    expect(resolveHitEndpoint('overlap', width / 2, 10, rect, fraction, fraction)).toBe('start')
    expect(resolveHitEndpoint('overlap', width / 2, 70, rect, fraction, fraction)).toBe('end')

    // 指针在可见线外 6px 按下（仍在 24px 区内），首动 1px 只变化有限下标
    const index = 180
    const pointerOffsetIndex = 6 / (width / 364) // 约 5.8 天
    const downIndex = Math.round(index + pointerOffsetIndex)
    let gesture = beginBrushGesture('start', downIndex, index, index + 10, 365)
    gesture = moveBrushGesture(gesture, downIndex + 1, 365)
    expect(gesture.start).toBe(index + 1)
  })
})
