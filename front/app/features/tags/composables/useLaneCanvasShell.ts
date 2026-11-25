import { computed, nextTick, onBeforeUnmount, onMounted, ref, toValue, type ComputedRef, type MaybeRefOrGetter, type Ref } from 'vue'
import {
  clampZoom,
  LANE_ZOOM_DEFAULT,
  zoomAnchorScroll,
  zoomStep,
} from './laneComposeLogic'

/**
 * 泳道画布**外壳层**（unify-lane-canvas-shell D1）：容器测量 / 缩放 / 滚动同步 / `Esc` 捕获。
 *
 * 两支画布（编排态 `compose/LaneComposeCanvas.vue`、修正态 `console/LaneCorrectionCanvas.vue`）
 * 原本各写一份这六件事，且已开始漂移。这里是**唯一一份**实现，宿主只保留各自的业务状态。
 *
 * 三条纪律：
 * - **不含任何画布业务类型**（节点 / 候选链 / 关系都不进本文件）——外壳只认像素与比例；
 * - **不改任何常量与公式**（D2）：缩放区间 / 步进 / 钳制 / 锚点换算全部沿用
 *   `laneComposeLogic` 的既有导出，本文件只做状态编排；
 * - **不变量**：拖拽期间屏蔽滚轮缩放（D3-①）、`Esc` 同时取消拖拽与平移（D3-②）。
 *
 * 宿主自己的溢出判据要用它自己的内容尺寸，所以「内容宽 / 内容高」由调用方以 getter 传入：
 * 计算属性惰性求值，宿主在这之后声明的 `vbWidth` / `canvasHeight` 也不会踩 TDZ
 * （`watch` 的 source 会立即求值——那条 watch 仍须留在 `zoom` 之后，见 design 风险段第 2 条）。
 */

export interface LaneCanvasShellOptions {
  /** 画布内容高（逻辑坐标，**未乘 zoom**）——纵向溢出判据的输入。 */
  contentHeight: MaybeRefOrGetter<number>
  /** 画布内容宽（逻辑坐标，**未乘 zoom**）——横向溢出判据的输入。 */
  contentWidth: MaybeRefOrGetter<number>
  /**
   * 滚轮缩放是否应被屏蔽（缺省不屏蔽）。
   *
   * 两支画布都传「正在拖节点」，即**拖拽期间不响应滚轮缩放**：拖拽中触发缩放会让拖拽
   * 坐标基准（`canvasContentPoint` 里的除 `zoom`）中途变化，属缺陷而非特性。
   */
  isZoomBlocked?: () => boolean
  /**
   * 当前是否有进行中的手势（拖拽或平移）。`Esc` 只在此时接管，避免干扰宿主的
   * 「退出编排态」快捷键。
   */
  hasActiveGesture?: () => boolean
  /** `Esc` 命中手势时的取消回调：宿主复位自己的手势态（并给 click 抑制打时间戳）。 */
  onCancelGesture?: () => void
}

export interface LaneCanvasShell {
  /** 滚动容器（`overflow: auto` 的那一层，画布 1:1 的测量基准）。 */
  containerRef: Ref<HTMLElement | null>
  containerWidth: Ref<number>
  containerHeight: Ref<number>
  /** 测量容器并同步滚动位置（`ResizeObserver` / `window.resize` / 宿主的 `defineExpose` 共用）。 */
  measure: () => void
  zoom: Ref<number>
  /** 缩放读数（`100%` 形态，直接进控件）。 */
  zoomPercent: ComputedRef<string>
  /** 滚轮缩放（非 passive 监听，`preventDefault` 挡掉整页滚动）。 */
  onWheel: (event: WheelEvent) => void
  /** 直接应用一个缩放值（按光标锚点换算滚动位置）。 */
  applyZoom: (next: number, clientX?: number, clientY?: number) => void
  /** 相对步进（`delta > 1` 放大）：修正画布的 `＋ / －` 按钮用。 */
  changeZoom: (delta: number) => void
  /** 回到 100%：比例与滚动位置一起归零。 */
  resetZoom: () => void
  scrollTop: Ref<number>
  scrollLeft: Ref<number>
  /** 逻辑滚动位移（= CSS px / zoom）：吸顶位移在内容坐标里要用它。 */
  scrollTopLogical: ComputedRef<number>
  scrollLeftLogical: ComputedRef<number>
  /** 内容是否真的超出可视高（决定要不要纵向吸顶；不溢出时宿主不下发 transform）。 */
  overflowing: ComputedRef<boolean>
  /** 内容是否真的超出可视宽（决定要不要横向吸顶）。 */
  overflowingX: ComputedRef<boolean>
  /** 从容器读回滚动位置（`scroll` 事件 / 测量后 / 缩放后）。 */
  syncScroll: () => void
}

export function useLaneCanvasShell(options: LaneCanvasShellOptions): LaneCanvasShell {
  const containerRef = ref<HTMLElement | null>(null)
  const containerWidth = ref(0)
  const containerHeight = ref(0)
  const zoom = ref(LANE_ZOOM_DEFAULT)
  const scrollTop = ref(0)
  const scrollLeft = ref(0)

  const zoomPercent = computed(() => `${Math.round(zoom.value * 100)}%`)
  const scrollTopLogical = computed(() => (zoom.value > 0 ? scrollTop.value / zoom.value : scrollTop.value))
  const scrollLeftLogical = computed(() => (zoom.value > 0 ? scrollLeft.value / zoom.value : scrollLeft.value))

  /**
   * 纵向溢出：内容高 × zoom 超过容器实测高（+1 容差吃掉亚像素）。
   *
   * 容器高度未测量（0）时判「不溢出」——首帧不该有吸顶痕迹。
   */
  const overflowing = computed(() => {
    const viewport = containerHeight.value
    if (!viewport) return false
    return toValue(options.contentHeight) * zoom.value > viewport + 1
  })

  /** 横向溢出：内容宽 × zoom 超过容器实测宽（判据同纵向）。 */
  const overflowingX = computed(() => {
    const viewport = containerWidth.value
    if (!viewport) return false
    return toValue(options.contentWidth) * zoom.value > viewport + 1
  })

  function syncScroll(): void {
    const el = containerRef.value
    scrollTop.value = el ? el.scrollTop || 0 : 0
    scrollLeft.value = el ? el.scrollLeft || 0 : 0
  }

  function measure(): void {
    const el = containerRef.value
    if (!el) return
    containerWidth.value = el.clientWidth || 0
    containerHeight.value = el.clientHeight || 0
    // 容器尺寸变化后滚动位置可能被浏览器夹回，吸顶位移要跟着重算
    syncScroll()
  }

  function onWindowResize(): void {
    measure()
  }

  function onWindowKeydown(event: KeyboardEvent): void {
    if (event.key !== 'Escape') return
    if (!options.hasActiveGesture?.()) return
    event.preventDefault()
    // 捕获阶段拦下：拖拽中的 Esc 只取消手势，不冒泡到宿主的「退出编排态」
    event.stopPropagation()
    options.onCancelGesture?.()
  }

  function onWheel(event: WheelEvent): void {
    // 拖拽期间不响应滚轮缩放（D3-①）
    if (options.isZoomBlocked?.()) return
    event.preventDefault()
    const clientX = Number.isFinite(event.clientX) ? event.clientX : undefined
    const clientY = Number.isFinite(event.clientY) ? event.clientY : undefined
    applyZoom(zoomStep(zoom.value, event.deltaY), clientX, clientY)
  }

  /** 应用缩放：改比例 + 按光标锚点换算滚动位置（等 SVG 尺寸更新后再落滚动，避免被旧范围夹掉）。 */
  function applyZoom(next: number, clientX?: number, clientY?: number): void {
    const to = clampZoom(next)
    const from = zoom.value
    if (to === from) return
    const surface = containerRef.value
    if (!surface) {
      zoom.value = to
      return
    }
    const rect = surface.getBoundingClientRect()
    const anchorX = (clientX ?? rect.left + surface.clientWidth / 2) - rect.left
    const anchorY = (clientY ?? rect.top + surface.clientHeight / 2) - rect.top
    const nextScrollLeft = zoomAnchorScroll(anchorX, surface.scrollLeft, from, to)
    const nextScrollTop = zoomAnchorScroll(anchorY, surface.scrollTop, from, to)
    zoom.value = to
    void nextTick().then(() => {
      surface.scrollLeft = nextScrollLeft
      surface.scrollTop = nextScrollTop
    })
  }

  function changeZoom(delta: number): void {
    applyZoom(zoom.value * delta)
  }

  /** 重置到 100%：比例与滚动位置一起回到初始态。 */
  function resetZoom(): void {
    zoom.value = LANE_ZOOM_DEFAULT
    const surface = containerRef.value
    if (!surface) return
    void nextTick().then(() => {
      surface.scrollLeft = 0
      surface.scrollTop = 0
    })
  }

  let resizeObserver: ResizeObserver | null = null

  onMounted(() => {
    measure()
    window.addEventListener('resize', onWindowResize)
    window.addEventListener('keydown', onWindowKeydown, true)
    if (typeof ResizeObserver !== 'undefined' && containerRef.value) {
      resizeObserver = new ResizeObserver(() => measure())
      resizeObserver.observe(containerRef.value)
    }
    // 滚轮缩放：必须非 passive，否则 preventDefault 不生效（画布上滚轮会把整页滚走）
    containerRef.value?.addEventListener('wheel', onWheel, { passive: false })
  })

  onBeforeUnmount(() => {
    window.removeEventListener('resize', onWindowResize)
    window.removeEventListener('keydown', onWindowKeydown, true)
    resizeObserver?.disconnect()
    resizeObserver = null
    containerRef.value?.removeEventListener('wheel', onWheel)
  })

  return {
    containerRef,
    containerWidth,
    containerHeight,
    measure,
    zoom,
    zoomPercent,
    onWheel,
    applyZoom,
    changeZoom,
    resetZoom,
    scrollTop,
    scrollLeft,
    scrollTopLogical,
    scrollLeftLogical,
    overflowing,
    overflowingX,
    syncScroll,
  }
}
