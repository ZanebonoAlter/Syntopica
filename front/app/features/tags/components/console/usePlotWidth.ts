/**
 * 绘图区真实像素宽度（矩阵阶段 G2，R37/R40）。
 * 命中区按 24 CSS px 计算需要真实宽度；happy-dom 无布局时由测试打桩。
 */
import { nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'

export function usePlotWidth(plotRef: Ref<SVGSVGElement | null>) {
  const plotWidth = ref(0)
  let observer: ResizeObserver | null = null

  function measure(): void {
    const width = plotRef.value?.getBoundingClientRect().width ?? 0
    plotWidth.value = Number.isFinite(width) && width > 0 ? width : 0
  }

  function disconnectObserver(): void {
    observer?.disconnect()
    observer = null
  }

  function observe(svg: SVGSVGElement | null): void {
    disconnectObserver()
    if (!svg || typeof ResizeObserver === 'undefined') return
    observer = new ResizeObserver(measure)
    observer.observe(svg)
  }

  // Controls 首挂 bounds/range=null 时 SVG 不存在；数据晚到后 plotRef 才出现。
  // flush:'post' 确保 DOM 已替换再测宽；消失/替换会断开旧 RO 并观察新 SVG。
  watch(plotRef, (svg) => {
    measure()
    observe(svg)
  }, { flush: 'post' })

  onMounted(() => {
    measure()
    void nextTick(measure)
    observe(plotRef.value)
  })
  onBeforeUnmount(() => {
    disconnectObserver()
  })

  return plotWidth
}
