/**
 * 刷选交互纯状态机（rework-workshop-into-radar-and-desk 矩阵阶段 G2）。
 *
 * ObservationRangeControls 的拖拽语义在此以纯函数实现（批准原型 range-controls.js 的
 * new/pan/start/end 四模式），组件层只做「pointer 事件 → 轴下标」的 DOM 映射：
 *  - new：拖空白处刷选新区间，**反向拖仍合法区间**（origin 与当前点取 min/max 归一）。
 *  - pan：拖既有选区整体平移（钳制在池内，宽度不变）。
 *  - start/end：拖双端手柄，不越过对端、不出池。
 * 全部结果都钳制在 [0, axisLength-1] 且 start ≤ end；本模块不做任何日期/档位业务判断
 * （档位唯一入口是独立档按钮，OX-1），日期换算由组件用 G1 observationRange 纯函数完成。
 */
export type BrushMode = 'new' | 'pan' | 'start' | 'end'

export interface BrushGesture {
  mode: BrushMode
  /** 按下点下标（pan 的位移基准 / new 的锚点）。 */
  origin: number
  /** 手势开始时的选区（取消时恢复用）。 */
  originalStart: number
  originalEnd: number
  /** 当前草稿选区。 */
  start: number
  end: number
  /** R40：抓取偏移（指针下标 − 实际端点下标），首次移动不把端点对齐指针。 */
  grabOffset: number
}

function clampIndex(index: number, axisLength: number): number {
  return Math.max(0, Math.min(Math.round(index), axisLength - 1))
}

/** 开始手势：mode 由命中目标决定（手柄 → start/end；选区内 → pan；空白 → new）。 */
export function beginBrushGesture(
  mode: BrushMode,
  pointerIndex: number,
  currentStart: number,
  currentEnd: number,
  axisLength: number,
): BrushGesture {
  const index = clampIndex(pointerIndex, axisLength)
  return {
    mode,
    origin: index,
    originalStart: currentStart,
    originalEnd: currentEnd,
    start: mode === 'new' ? index : currentStart,
    end: mode === 'new' ? index : currentEnd,
    grabOffset: mode === 'start'
      ? index - currentStart
      : mode === 'end'
        ? index - currentEnd
        : 0,
  }
}

/** 手势进行中：按模式推进草稿选区（输入已钳制，输出保证 start ≤ end 且在池内）。 */
export function moveBrushGesture(
  gesture: BrushGesture,
  pointerIndex: number,
  axisLength: number,
): BrushGesture {
  const index = clampIndex(pointerIndex, axisLength)
  switch (gesture.mode) {
    case 'start':
      return {
        ...gesture,
        start: Math.min(clampIndex(index - gesture.grabOffset, axisLength), gesture.end),
      }
    case 'end':
      return {
        ...gesture,
        end: Math.max(clampIndex(index - gesture.grabOffset, axisLength), gesture.start),
      }
    case 'new': {
      // 反向拖合法：锚点与当前点归一为 [min, max]
      return { ...gesture, start: Math.min(gesture.origin, index), end: Math.max(gesture.origin, index) }
    }
    case 'pan': {
      const delta = Math.max(
        -gesture.originalStart,
        Math.min(axisLength - 1 - gesture.originalEnd, index - gesture.origin),
      )
      return { ...gesture, start: gesture.originalStart + delta, end: gesture.originalEnd + delta }
    }
  }
}

/** 指针坐标 → 轴下标（真实绘图区 rect 映射；供组件层调用，越界钳制）。 */
export function pointerIndexFromClientX(
  clientX: number,
  rect: { left: number, width: number },
  axisLength: number,
): number {
  if (axisLength <= 1 || rect.width <= 0) return 0
  const fraction = (clientX - rect.left) / rect.width
  return clampIndex(fraction * (axisLength - 1), axisLength)
}

/** 按下目标命中判定：手柄 → start/end；选区内 → pan；空白 → new。 */
export function brushModeForTarget(
  hitBound: 'start' | 'end' | null,
  pointerIndex: number,
  currentStart: number,
  currentEnd: number,
): BrushMode {
  if (hitBound === 'start' || hitBound === 'end') return hitBound
  return pointerIndex >= currentStart && pointerIndex <= currentEnd ? 'pan' : 'new'
}


/**
 * R37/R40：24 CSS px 手柄命中区。
 *
 * 两端像素间距 ≥24 时各自一区；<24 时合并为一区（几何重叠允许），
 * 由真实指针 x 解析最近可见端；单日同一 x 时按上下半区解析起/止。
 */
export type HitRegionKind = 'start' | 'end' | 'overlap'

export interface HitRegion {
  kind: HitRegionKind
  style: Record<string, string>
}

function clampPx(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(value, max))
}

export function buildHitRegions(startFraction: number, endFraction: number, plotWidth: number): HitRegion[] {
  if (!Number.isFinite(plotWidth) || plotWidth <= 0) return []
  const startX = Math.max(0, Math.min(1, startFraction)) * plotWidth
  const endX = Math.max(0, Math.min(1, endFraction)) * plotWidth
  const hitWidth = Math.min(24, plotWidth)
  const leftFor = (x: number): number => clampPx(x - hitWidth / 2, 0, Math.max(0, plotWidth - hitWidth))

  const startLeft = leftFor(startX)
  const startRight = startLeft + hitWidth
  const endLeft = leftFor(endX)
  const endRight = endLeft + hitWidth

  // 不只按原始 gap 判断：端点被夹到绘图区边缘时，两个 24px 区可能实际重叠。
  if (startLeft < endRight && endLeft < startRight) {
    const left = Math.min(startLeft, endLeft)
    const right = Math.max(startRight, endRight)
    return [{
      kind: 'overlap',
      style: { left: `${left}px`, width: `${Math.max(1, right - left)}px` },
    }]
  }

  return [
    { kind: 'start', style: { left: `${startLeft}px`, width: `${hitWidth}px` } },
    { kind: 'end', style: { left: `${endLeft}px`, width: `${hitWidth}px` } },
  ]
}

/** 命中区 → 实际端点：非重叠直接对应；重叠按最近可见端，单日按上下半区。 */
export function resolveHitEndpoint(
  kind: HitRegionKind,
  clientX: number,
  clientY: number,
  rect: { left: number, top: number, width: number, height: number },
  startFraction: number,
  endFraction: number,
): 'start' | 'end' {
  if (kind === 'start' || kind === 'end') return kind
  const startX = rect.left + Math.max(0, Math.min(1, startFraction)) * rect.width
  const endX = rect.left + Math.max(0, Math.min(1, endFraction)) * rect.width
  if (Math.abs(startX - endX) < 0.5) {
    return clientY < rect.top + rect.height / 2 ? 'start' : 'end'
  }
  return Math.abs(clientX - startX) <= Math.abs(clientX - endX) ? 'start' : 'end'
}
