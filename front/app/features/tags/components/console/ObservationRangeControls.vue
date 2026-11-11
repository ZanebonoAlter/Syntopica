<script setup lang="ts">
/**
 * 时间视窗控件（rework-workshop-into-radar-and-desk 矩阵阶段 G2，design D11 观察窗 A 路线）。
 *
 * 受控纯 UI：bounds/range/sourceDays/series 由上层给定，不请求 API、不写 router、
 * 不持有第二份业务名单（拖拽草稿仅存在于手势期间）。事件语义：
 *  - `preview`（拖动中）→ 父调 applyObservationRange(...,{writeQuery:false})；
 *    `commit`（松手/表单/键盘/按钮完成）→ 父默认提交（R-22 同值松手恰写一次）。
 *  - 拖/输/缩放至恰 7/14/30/365 日不切档（OX-1）；只有独立档按钮 emit `source-change`。
 *  - 刷选：拖空白（反向合法）/拖选区平移/双端手柄（rangeBrush 纯状态机）；键盘逐日/
 *    Shift 逐周/Home/End；pointercancel/Esc 恢复起始选区不提交，一次手势恰一次 commit。
 *  - 坐标映射只消费绘图区真实 rect；日期/事件线数轴外置（TrendAxes）；
 *    series null=未取到（不补 0 不桥接），已知 0 画基线；无数据不造今天。
 */
import { computed, onBeforeUnmount, ref } from 'vue'
import TrendAxes from './TrendAxes.vue'
import RangeDateForm from './RangeDateForm.vue'
import RangeHandleHits from './RangeHandleHits.vue'
import { usePlotWidth } from './usePlotWidth'
import { buildBrushCurve, BRUSH_GEOMETRY } from './brushCurve'
import { xFraction, type LaneTrendPoint } from './laneTrendSeries'
import {
  SOURCE_TIER_DAYS,
  daysBetweenInclusive,
  edgeToBound,
  stepRangeEdge,
  type DateBounds,
  type ObservationRange,
  type SourceTierDays,
} from './observationRange'
import {
  beginBrushGesture,
  brushModeForTarget,
  moveBrushGesture,
  pointerIndexFromClientX,
  resolveHitEndpoint,
  type BrushGesture,
  type HitRegionKind,
} from './rangeBrush'

const props = withDefaults(defineProps<{
  /** 365 日真实池日期界（含两端）；null = 数据未就绪（整组禁用）。 */
  bounds: DateBounds | null
  /** 当前观察范围（已提交值；预览经父写回 observation 后由本 prop 回流）。 */
  range: ObservationRange | null
  /** 当前档（A 通道 days），仅用于档按钮激活态。 */
  sourceDays: number
  /** 365 池「已返回泳道日展示降级口径」曲线（事件线数 > 0 用事件线数，否则线索合计兜底；
   *  与矩阵格同口径）；null = 未取到（刷选仍可用）。 */
  series: LaneTrendPoint[] | null
  /** 池曲线加载中。 */
  loading?: boolean
  /** 上层提示文案（G1 rangeNotice）。 */
  notice?: string | null
}>(), {
  loading: false,
  notice: null,
})

const emit = defineEmits<{
  preview: [range: ObservationRange]
  commit: [range: ObservationRange]
  'source-change': [days: SourceTierDays]
}>()

const disabled = computed(() => props.bounds === null || props.range === null)
const plotRef = ref<SVGSVGElement | null>(null)

const curve = computed(() => (props.bounds ? buildBrushCurve(props.bounds, props.series) : null))
const axis = computed(() => curve.value?.axis ?? [])
const axisCount = computed(() => axis.value.length)

const rangeStartIndex = computed(() => {
  if (!props.bounds || !props.range) return 0
  return daysBetweenInclusive(props.bounds.start, props.range.start) - 1
})
const rangeEndIndex = computed(() => {
  if (!props.bounds || !props.range) return 0
  return daysBetweenInclusive(props.bounds.start, props.range.end) - 1
})

// ---- 刷选手势：pointer 事件 → rangeBrush 纯状态机 → preview/commit ----
const gesture = ref<BrushGesture | null>(null)

function gestureRange(g: BrushGesture): ObservationRange {
  return { start: axis.value[g.start] ?? '', end: axis.value[g.end] ?? '' }
}

function plotIndexAt(clientX: number): number {
  const svg = plotRef.value
  return svg ? pointerIndexFromClientX(clientX, svg.getBoundingClientRect(), axisCount.value) : 0
}

function onPlotPointerDown(event: PointerEvent, hitBound: 'start' | 'end' | null): void {
  if (disabled.value || event.button !== 0 || !plotRef.value) return
  const index = plotIndexAt(event.clientX)
  const mode = brushModeForTarget(hitBound, index, rangeStartIndex.value, rangeEndIndex.value)
  const next = beginBrushGesture(mode, index, rangeStartIndex.value, rangeEndIndex.value, axisCount.value)
  gesture.value = next
  try {
    // 捕获后全部 pointer 事件重定向到绘图区：move/up 监听挂 svg 自身即可跟完手势
    plotRef.value.setPointerCapture?.(event.pointerId)
  }
  catch { /* 捕获失败不阻断：指针离开绘图区时手势中断，可用表单/按钮补偿 */ }
  plotRef.value.addEventListener('pointermove', onPlotPointerMove)
  plotRef.value.addEventListener('pointerup', onPlotPointerUp)
  plotRef.value.addEventListener('pointercancel', onPlotPointerCancel)
  document.addEventListener('keydown', onDocumentKeydown)
  if (!hitBound) event.preventDefault() // 空白/选区拖动不抢正文焦点
  emit('preview', gestureRange(next))
}

function onPlotPointerMove(event: PointerEvent): void {
  if (!gesture.value) return
  const next = moveBrushGesture(gesture.value, plotIndexAt(event.clientX), axisCount.value)
  gesture.value = next
  emit('preview', gestureRange(next))
}

function finishGesture(commit: boolean): void {
  removeDocumentListeners()
  const current = gesture.value
  gesture.value = null
  if (!current) return
  if (commit) {
    emit('commit', gestureRange(current))
  }
  else {
    // pointercancel/Esc：不提交错误范围，恢复起始选区显示（仅预览回写）
    emit('preview', { start: axis.value[current.originalStart] ?? '', end: axis.value[current.originalEnd] ?? '' })
  }
}

function onPlotPointerUp(): void {
  finishGesture(true)
}

function onPlotPointerCancel(): void {
  finishGesture(false)
}

function onDocumentKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape' && gesture.value) finishGesture(false)
}

function removeDocumentListeners(): void {
  plotRef.value?.removeEventListener('pointermove', onPlotPointerMove)
  plotRef.value?.removeEventListener('pointerup', onPlotPointerUp)
  plotRef.value?.removeEventListener('pointercancel', onPlotPointerCancel)
  document.removeEventListener('keydown', onDocumentKeydown)
}

onBeforeUnmount(removeDocumentListeners)

/** 键盘端点调整（手柄 slider）：逐日/Shift 逐周/Home/End，每次按键即一次完成动作。 */
function onHandleKeydown(event: KeyboardEvent, edge: 'start' | 'end'): void {
  if (disabled.value || !props.bounds || !props.range) return
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  let next: ObservationRange
  if (event.key === 'Home' || event.key === 'End') {
    next = edgeToBound(props.range, edge, event.key === 'Home' ? 'start' : 'end', props.bounds)
  }
  else {
    const delta = (event.key === 'ArrowLeft' ? -1 : 1) * (event.shiftKey ? 7 : 1)
    next = stepRangeEdge(props.range, edge, delta, props.bounds)
  }
  emit('commit', next)
}

/** 手势草稿（渲染 selection/handle 位置）；无手势时用 props.range。 */
const viewStart = computed(() => gesture.value?.start ?? rangeStartIndex.value)
const viewEnd = computed(() => gesture.value?.end ?? rangeEndIndex.value)
const viewStartX = computed(() => xFraction(viewStart.value, axisCount.value) * BRUSH_GEOMETRY.viewWidth)
const viewEndX = computed(() => xFraction(viewEnd.value, axisCount.value) * BRUSH_GEOMETRY.viewWidth)

const plotWidth = usePlotWidth(plotRef)

function onHitPointerDown(kind: HitRegionKind, event: PointerEvent): void {
  const rect = plotRef.value?.getBoundingClientRect()
  if (!rect) return
  const endpoint = resolveHitEndpoint(
    kind,
    event.clientX,
    event.clientY,
    { left: rect.left, top: rect.top, width: rect.width, height: rect.height },
    viewStartX.value / BRUSH_GEOMETRY.viewWidth,
    viewEndX.value / BRUSH_GEOMETRY.viewWidth,
  )
  onPlotPointerDown(event, endpoint)
}

/** 表单校验问题（来自 RangeDateForm；拖动文案优先于它）。 */
const formIssue = ref<string | null>(null)

const feedback = computed(() => {
  if (gesture.value) return `${axis.value[viewStart.value]} — ${axis.value[viewEnd.value]} · 松开应用`
  if (formIssue.value) return formIssue.value
  return props.notice
})
</script>

<template>
  <section class="orc" aria-label="时间视窗" data-testid="observation-range-controls">
    <div class="orc-head">
      <h3 class="orc-title">时间视窗</h3>
      <div class="orc-tiers" role="group" aria-label="独立数据档（决定名单与排序）">
        <button
          v-for="days in SOURCE_TIER_DAYS"
          :key="days"
          type="button"
          class="orc-chip orc-tier"
          :class="{ 'is-active': days === sourceDays }"
          :aria-pressed="days === sourceDays"
          :disabled="disabled"
          @click="emit('source-change', days)"
        >{{ days === 365 ? '近 1 年' : `${days} 天` }}</button>
      </div>
      <span class="orc-sub">Asia/Shanghai · 含起止日</span>
    </div>

    <div v-if="disabled" class="orc-empty" data-testid="observation-range-empty">
      数据尚未就绪，暂不能设置观察范围。
    </div>

    <template v-else>
      <RangeDateForm
        :bounds="bounds!"
        :range="range!"
        @commit="emit('commit', $event)"
        @issue="formIssue = $event"
      />

      <div class="orc-caption">
        <span>365 日池 · 已返回泳道的日事件线数</span>
        <span>无事件线日按线索合计兜底 · 非文章数</span>
      </div>

      <TrendAxes v-if="curve" :y-ticks="curve.yTicks" :x-ticks="curve.xTicks">
        <div class="orc-plot-wrap">
        <svg
          ref="plotRef"
          class="orc-plot"
          viewBox="0 0 1000 80"
          preserveAspectRatio="none"
          role="group"
          aria-label="时间趋势：拖空白刷选，拖选区平移，拖两端改变起止"
          data-testid="observation-range-plot"
          @pointerdown="onPlotPointerDown($event, null)"
        >
          <template v-for="(segment, segmentIndex) in curve.segments" :key="`seg-${segmentIndex}`">
            <path v-if="segment.length > 1" class="orc-area" :d="curve.segmentAreaPath(segment)" />
            <polyline v-if="segment.length > 1" class="orc-line" :points="curve.segmentPoints(segment)" />
          </template>
          <line class="orc-baseline" x1="0" x2="1000" :y1="BRUSH_GEOMETRY.baselineY" :y2="BRUSH_GEOMETRY.baselineY" />
          <rect
            class="orc-selection"
            data-testid="brush-selection"
            :x="viewStartX"
            y="0"
            :width="Math.max(2, viewEndX - viewStartX)"
            height="74"
          />
          <rect
            class="orc-handle"
            data-testid="brush-handle-start"
            :x="viewStartX - 5"
            y="0"
            width="10"
            height="74"
            tabindex="0"
            role="slider"
            aria-label="视窗起始端"
            aria-valuemin="0"
            :aria-valuemax="axisCount - 1"
            :aria-valuenow="viewStart"
            :aria-valuetext="axis[viewStart]"
            @pointerdown.stop="onPlotPointerDown($event, 'start')"
            @keydown="onHandleKeydown($event, 'start')"
          />
          <rect
            class="orc-handle"
            data-testid="brush-handle-end"
            :x="viewEndX - 5"
            y="0"
            width="10"
            height="74"
            tabindex="0"
            role="slider"
            aria-label="视窗截止端"
            aria-valuemin="0"
            :aria-valuemax="axisCount - 1"
            :aria-valuenow="viewEnd"
            :aria-valuetext="axis[viewEnd]"
            @pointerdown.stop="onPlotPointerDown($event, 'end')"
            @keydown="onHandleKeydown($event, 'end')"
          />
        </svg>
        <RangeHandleHits
          :plot-width="plotWidth"
          :start-fraction="viewStartX / BRUSH_GEOMETRY.viewWidth"
          :end-fraction="viewEndX / BRUSH_GEOMETRY.viewWidth"
          @grab="({ kind, event }) => onHitPointerDown(kind, event)"
        />
        </div>
      </TrendAxes>

      <p v-if="loading" class="orc-series-state" data-testid="observation-range-loading">池曲线加载中…</p>
      <p v-else-if="!series || series.length === 0" class="orc-series-state" data-testid="observation-range-no-series">
        365 日池趋势未取到（不影响日期选择，不补 0）。
      </p>
      <p class="orc-instruction">日期轴 · 拖动选区 / 两端 · 方向键逐日，Shift 逐周</p>
      <p class="orc-feedback" role="status" data-testid="observation-range-feedback">{{ feedback }}</p>
    </template>
  </section>
</template>

<style scoped>
.orc {
  border-bottom: 1px solid var(--color-border-medium);
  padding-bottom: 8px;
  min-width: 0;
}

.orc-head {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.orc-title {
  font-size: 1.08em;
  font-weight: 600;
  margin: 0;
}

.orc-tiers {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.orc-chip {
  border: 1px solid var(--color-border-medium);
  background: transparent;
  color: var(--color-text-secondary);
  padding: 3px 10px;
  font-size: 0.85em;
  cursor: pointer;
  white-space: nowrap;
}

.orc-tier.is-active {
  border-color: var(--color-accent);
  color: var(--color-accent);
  font-weight: 600;
}

.orc-tier:disabled {
  opacity: 0.5;
  cursor: default;
}

.orc-sub {
  font-size: 11px;
  color: var(--color-text-muted);
  margin-left: auto;
}

.orc-empty {
  margin-top: 10px;
  color: var(--color-text-muted);
  font-size: 0.9em;
}

.orc-caption {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 11px;
  color: var(--color-text-muted);
  margin-top: 14px;
}

.orc-plot-wrap {
  position: relative;
  min-width: 0;
}


.orc-plot {
  display: block;
  width: 100%;
  height: 88px;
  overflow: visible;
  touch-action: none;
  cursor: crosshair;
  margin: 8px 0 0;
}

.orc-area {
  fill: var(--color-accent);
  fill-opacity: 0.12;
  pointer-events: none;
}

.orc-line {
  fill: none;
  stroke: var(--color-accent);
  stroke-width: 1.5;
  vector-effect: non-scaling-stroke;
  pointer-events: none;
}

.orc-baseline {
  stroke: var(--color-border-medium);
  pointer-events: none;
}

.orc-selection {
  fill: var(--color-tertiary);
  fill-opacity: 0.13;
  stroke: var(--color-tertiary);
  stroke-width: 1;
  cursor: grab;
}

.orc-handle {
  fill: var(--color-tertiary);
  stroke: var(--color-bg-base);
  stroke-width: 2;
  cursor: ew-resize;
}

.orc-handle:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: 3px;
}

.orc-series-state,
.orc-instruction {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--color-text-muted);
}

.orc-feedback {
  margin-top: 10px;
  line-height: 1.6;
  font-size: 12px;
  color: var(--color-accent);
  min-height: 1.6em;
}
</style>
