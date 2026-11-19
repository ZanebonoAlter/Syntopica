<script setup lang="ts">
/**
 * 全历史导航条（archive-time-chart-range，归档时间导航块的第三段）。
 *
 * 受控纯 UI：全历史每日计数 + 当前视窗 → 画一条密度折线 + 一个可拖的视窗窗口；
 * 组件内部只做「像素 ↔ 日期」线性换算，不请求 API、不持有业务名单。
 *
 * 语义：
 *  - 窗内按下 = 平移视窗；窗外按下 = 先把点击日居中再继续拖（两段合成一次手势）；
 *  - 拖动中 emit `viewport-preview`（父写草稿，不发请求）；松手 emit `viewport-commit`
 *    （父提交区间 + 视窗，重拉列表）；Esc / pointercancel emit `viewport-cancel`
 *    （父回滚到拖动前，不提交）；
 *  - 键盘：←/→ 平移 1 天、Shift+←/→ 平移 7 天、Home/End 到最早/最新，**每次按键
 *    只 emit 一次 commit**（与主图「一次手势恰一次 commit」同纪律）。
 *
 * 视觉：与主图共用同一套语义色 token（不引入新色值）；viewBox 1000×28 +
 * preserveAspectRatio="none"（全历史可能上千天，按天画 tick 不可行，只做密度示意）。
 */
import { computed, onBeforeUnmount, ref } from 'vue'
import { buildStripSeries, viewportFor } from './archiveTimeSeries'
import { daysBetweenInclusive, shiftDate, type ObservationRange } from './observationRange'
import { usePlotWidth } from './usePlotWidth'

const props = withDefaults(defineProps<{
  /** 最早有命中文章之日（YYYY-MM-DD）；null = 本版块还没有命中文章（不渲染曲线）。 */
  earliest: string | null
  /** 全历史右端（= 视窗右端上限，通常是今天）。 */
  latest: string
  /** 每日计数（稀疏 → 查表；缺日按 0）。 */
  byDate: Map<string, number>
  /** 当前视窗（父持有，组件不自行推进）。 */
  viewport: ObservationRange
  /** 禁用态（计数加载中）：不响应手势但保留轮廓。 */
  disabled?: boolean
}>(), { disabled: false })

const emit = defineEmits<{
  'viewport-preview': [range: ObservationRange]
  'viewport-commit': [range: ObservationRange]
  'viewport-cancel': []
}>()

const STRIP_VIEW_WIDTH = 1000
const STRIP_VIEW_HEIGHT = 28

const stripRef = ref<SVGSVGElement | null>(null)
const stripWidth = usePlotWidth(stripRef)

/** 全历史天数（含两端）。 */
const historyDays = computed(() => (
  props.earliest ? Math.max(1, daysBetweenInclusive(props.earliest, props.latest)) : 0
))

/**
 * 导航条密度序列：桶数按容器真实宽度取（超长历史降采样）。
 * 桶内求和——导航条只表达密度，精确值看主图（桶和不是当天篇数）。
 */
const density = computed(() => {
  if (!props.earliest) return []
  const buckets = Math.max(30, Math.round(stripWidth.value) || 240)
  return buildStripSeries(props.earliest, props.latest, props.byDate, buckets)
})

const densityMax = computed(() => density.value.reduce((max, point) => Math.max(max, point.value), 0))

/** 日期 → 归一化位置（0..1）；早于 earliest 记 0，晚于 latest 记 1。 */
function fractionOf(date: string): number {
  if (!props.earliest || historyDays.value <= 1) return 0
  const index = daysBetweenInclusive(props.earliest, date) - 1
  return Math.max(0, Math.min(1, index / (historyDays.value - 1)))
}

function yOf(value: number): number {
  if (densityMax.value <= 0) return STRIP_VIEW_HEIGHT
  return STRIP_VIEW_HEIGHT - (value / densityMax.value) * (STRIP_VIEW_HEIGHT - 2)
}

const densityPoints = computed(() => (
  density.value.map(point => `${(fractionOf(point.date) * STRIP_VIEW_WIDTH).toFixed(1)},${yOf(point.value).toFixed(1)}`).join(' ')
))

const densityArea = computed(() => {
  if (density.value.length === 0) return ''
  const first = (fractionOf(density.value[0]!.date) * STRIP_VIEW_WIDTH).toFixed(1)
  const last = (fractionOf(density.value[density.value.length - 1]!.date) * STRIP_VIEW_WIDTH).toFixed(1)
  return `M${densityPoints.value.replace(/ /g, ' L')} L${last},${STRIP_VIEW_HEIGHT} L${first},${STRIP_VIEW_HEIGHT} Z`
})

/** 视窗窗口矩形（视窗可能早于 earliest，位置钳制在轨道内）。 */
const windowRect = computed(() => {
  const start = fractionOf(props.viewport.start) * STRIP_VIEW_WIDTH
  const end = fractionOf(props.viewport.end) * STRIP_VIEW_WIDTH
  return { x: start, width: Math.max(2, end - start) }
})

/** 视窗起点在历史轴上的下标（aria-valuenow 用）。 */
const windowStartIndex = computed(() => {
  if (!props.earliest || historyDays.value <= 0) return 0
  return Math.max(0, Math.min(
    historyDays.value - 1,
    daysBetweenInclusive(props.earliest, props.viewport.start) - 1,
  ))
})

const windowLabel = computed(() => (
  `${props.viewport.start.slice(5)} ~ ${props.viewport.end.slice(5)}`
))

// ── 手势：指针 → 锚点右端 → 视窗 ────────────────────────────────────────────

/** 拖动中的锚点偏移（锚点右端 − 光标日）。 */
const dragOffset = ref<number | null>(null)
/** 最近一次 preview 的视窗（松手时提交它）。 */
const lastPreview = ref<ObservationRange | null>(null)

/** 像素 → 日期（线性换算；全历史可能上千天，禁止按天渲染 tick）。 */
function dateAtClientX(clientX: number): string {
  if (!props.earliest) return props.viewport.end
  const rect = stripRef.value?.getBoundingClientRect()
  const ratio = rect && rect.width > 0 ? (clientX - rect.left) / rect.width : 0
  const index = Math.round(Math.max(0, Math.min(1, ratio)) * (historyDays.value - 1))
  return shiftDate(props.earliest, index)
}

/** 把视窗右端移到 anchorEnd（宽度保持；历史不足一窗时整段可见）。 */
function viewportAt(anchorEnd: string): ObservationRange {
  const width = daysBetweenInclusive(props.viewport.start, props.viewport.end)
  return viewportFor(anchorEnd, props.earliest, props.latest, width)
}

function onPointerDown(event: PointerEvent): void {
  if (props.disabled || !props.earliest || event.button !== 0) return
  const cursorDate = dateAtClientX(event.clientX)
  const width = daysBetweenInclusive(props.viewport.start, props.viewport.end)
  const inside = cursorDate >= props.viewport.start && cursorDate <= props.viewport.end
  // 窗内按下 = 抓住窗口原位拖；窗外按下 = 先让点击日居中再继续拖。
  dragOffset.value = inside
    ? daysBetweenInclusive(cursorDate, props.viewport.end) - 1
    : Math.floor((width - 1) / 2)
  try {
    stripRef.value?.setPointerCapture?.(event.pointerId)
  }
  catch { /* 捕获失败不阻断：指针离开轨道时手势中断，可用键盘补偿 */ }
  stripRef.value?.addEventListener('pointermove', onPointerMove)
  stripRef.value?.addEventListener('pointerup', onPointerUp)
  stripRef.value?.addEventListener('pointercancel', onPointerCancel)
  document.addEventListener('keydown', onDocumentKeydown)
  event.preventDefault()
  emitPreview(shiftDate(cursorDate, dragOffset.value))
}

function emitPreview(anchorEnd: string): void {
  const next = viewportAt(anchorEnd)
  lastPreview.value = next
  emit('viewport-preview', next)
}

function onPointerMove(event: PointerEvent): void {
  if (dragOffset.value === null) return
  emitPreview(shiftDate(dateAtClientX(event.clientX), dragOffset.value))
}

function onPointerUp(): void {
  const range = lastPreview.value
  cleanupGesture()
  if (range) emit('viewport-commit', range)
}

function onPointerCancel(): void {
  cleanupGesture()
  emit('viewport-cancel')
}

function onDocumentKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape' && dragOffset.value !== null) onPointerCancel()
}

function cleanupGesture(): void {
  stripRef.value?.removeEventListener('pointermove', onPointerMove)
  stripRef.value?.removeEventListener('pointerup', onPointerUp)
  stripRef.value?.removeEventListener('pointercancel', onPointerCancel)
  document.removeEventListener('keydown', onDocumentKeydown)
  dragOffset.value = null
  lastPreview.value = null
}

onBeforeUnmount(cleanupGesture)

/** 键盘步进：每次按键 = 一次完成动作（只 emit commit 一次）。 */
function onKeydown(event: KeyboardEvent): void {
  if (props.disabled || !props.earliest) return
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const width = daysBetweenInclusive(props.viewport.start, props.viewport.end)
  let anchor: string
  if (event.key === 'Home') {
    anchor = shiftDate(props.earliest, width - 1)
  }
  else if (event.key === 'End') {
    anchor = props.latest
  }
  else {
    const delta = (event.key === 'ArrowLeft' ? -1 : 1) * (event.shiftKey ? 7 : 1)
    anchor = shiftDate(props.viewport.end, delta)
  }
  emit('viewport-commit', viewportAt(anchor))
}
</script>

<template>
  <div class="ahs" data-testid="archive-history-strip">
    <svg
      ref="stripRef"
      class="ahs-track"
      viewBox="0 0 1000 28"
      preserveAspectRatio="none"
      role="group"
      aria-label="全历史导航条：拖动视窗窗口平移时间范围"
      :class="{ 'is-disabled': disabled }"
      @pointerdown="onPointerDown"
    >
      <path v-if="densityArea" class="ahs-area" :d="densityArea" />
      <polyline v-if="densityPoints" class="ahs-line" :points="densityPoints" />
      <rect
        v-if="earliest"
        class="ahs-window"
        data-testid="archive-history-window"
        :x="windowRect.x"
        y="0"
        :width="windowRect.width"
        :height="STRIP_VIEW_HEIGHT"
        :tabindex="disabled ? -1 : 0"
        role="slider"
        aria-label="时间范围视窗"
        :aria-valuemin="0"
        :aria-valuemax="historyDays > 0 ? historyDays - 1 : 0"
        :aria-valuenow="windowStartIndex"
        :aria-valuetext="windowLabel"
        :aria-disabled="disabled ? 'true' : undefined"
        @keydown="onKeydown"
      />
    </svg>
    <div class="ahs-ends">
      <span class="ahs-end">{{ earliest ? earliest.slice(5) : '--' }}</span>
      <span class="ahs-end">{{ latest.slice(5) }}</span>
    </div>
  </div>
</template>

<style scoped>
.ahs {
  margin-top: 12px;
  min-width: 0;
}

.ahs-track {
  display: block;
  width: 100%;
  height: 28px;
  overflow: visible;
  touch-action: none;
  cursor: grab;
}

.ahs-track.is-disabled {
  cursor: default;
  opacity: 0.6;
}

.ahs-area {
  fill: var(--color-accent);
  fill-opacity: 0.12;
  pointer-events: none;
}

.ahs-line {
  fill: none;
  stroke: var(--color-accent);
  stroke-width: 1.5;
  vector-effect: non-scaling-stroke;
  pointer-events: none;
}

.ahs-window {
  fill: var(--color-tertiary);
  fill-opacity: 0.13;
  stroke: var(--color-tertiary);
  stroke-width: 2;
  cursor: grab;
}

.ahs-window:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: 3px;
}

.ahs-ends {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: var(--color-text-muted);
}
</style>
