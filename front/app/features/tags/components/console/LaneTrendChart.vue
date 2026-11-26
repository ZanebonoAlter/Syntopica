<script setup lang="ts">
/**
 * 泳道日线索数趋势图（rework-workshop-into-radar-and-desk 矩阵阶段 G2，EV-6 长窗形态）。
 *
 * 纯 series 绘图：时间 X 轴 + 线索数（条）Y 轴/真实比例刻度/0 基线，数据由上层提供
 * （G3 BoardLaneEvolution 在可见日 >31 时挂载），无 topic/board API 读取、不做 raw 字段
 * 映射或主排序。系列契约 `{date, value: number|null}`：
 *  - null = 未取到/未知：不画点、不补 0、不跨未知段桥接成假实线；
 *  - 已知 0 = 合法结论：画点落在 0 基线，可点选。
 * 受控选择：selectedDate 高亮由父给定；点已知值日 emit `select-date`（G3 复用），
 * 未知日不可点。节点 title 读日期/实际数；装饰元素不抢 Tab（点不设 tabindex，
 * 键盘选日由 G3 的既有导航承接）。轴外置复用 TrendAxes（与观察窗刷选同构）。
 */
import { computed } from 'vue'
import TrendAxes from './TrendAxes.vue'
import {
  dateTickIndexes,
  indexToX,
  knownMaxValue,
  knownSegments,
  tickTopPercent,
  trendTickValues,
  valueToY,
  xFraction,
  type LaneTrendPoint,
} from './laneTrendSeries'

const props = withDefaults(defineProps<{
  /** 逐日序列（date 升序由上层保证；窗口裁切/统计在上层完成）。 */
  series: LaneTrendPoint[]
  /** 受控选中日（YYYY-MM-DD）；不在序列内则不高亮。 */
  selectedDate?: string | null
  loading?: boolean
  /** 图表上方说明文案（宿主给口径说明，如「所选泳道 · 所示日期内日合计（非矩阵桶峰值）」）。 */
  label?: string
  /** 空/全未知时的说明文案。 */
  emptyText?: string
}>(), {
  selectedDate: null,
  loading: false,
  label: '',
  emptyText: '该区间暂无逐日数据。',
})

const emit = defineEmits<{
  /** 点选已知值日（0 是已知值；null 未知日不 emit）。 */
  'select-date': [date: string]
}>()

/** 批准原型 lane-trend 绘图区：viewBox 480×74，基线 y=64、满值 y=8（显示高 88px 与 TrendAxes 对齐）。 */
const GEOMETRY = { viewWidth: 480, viewHeight: 74, baselineY: 64, topY: 8 }

const count = computed(() => props.series.length)
const max = computed(() => knownMaxValue(props.series))
const segments = computed(() => knownSegments(props.series))
const hasKnown = computed(() => max.value > 0 || segments.value.length > 0)

const yTicks = computed(() => trendTickValues(max.value).map(value => ({
  value,
  topPercent: tickTopPercent(value, max.value, GEOMETRY),
})))
const xTicks = computed(() => dateTickIndexes(count.value).map((index) => {
  const align = index === 0 ? 'start' as const : index === count.value - 1 ? 'end' as const : 'center' as const
  return { label: props.series[index]?.date ?? '', leftPercent: xFraction(index, count.value) * 100, align }
}))

const indexOfDate = computed(() => new Map(props.series.map((point, index) => [point.date, index])))

function xAt(index: number): number {
  return indexToX(index, count.value, GEOMETRY)
}

function yAt(value: number): number {
  return valueToY(value, max.value, GEOMETRY)
}

function segmentPoints(segment: LaneTrendPoint[]): string {
  return segment
    .map(point => `${xAt(indexOfDate.value.get(point.date) ?? 0)},${yAt(point.value ?? 0)}`)
    .join(' ')
}

function onSelectPoint(point: LaneTrendPoint): void {
  if (point.value === null) return
  emit('select-date', point.date)
}

const selectedIndex = computed(() =>
  props.selectedDate ? indexOfDate.value.get(props.selectedDate) ?? null : null,
)

/** aria 总述：非视觉读法（日期/量级/未知数概览）。 */
const ariaSummary = computed(() => {
  const known = props.series.filter(point => point.value !== null)
  const unknown = count.value - known.length
  const span = count.value > 0
    ? `${props.series[0]?.date ?? ''} 至 ${props.series[count.value - 1]?.date ?? ''}`
    : ''
  const peak = known.length > 0
    ? `峰值 ${Math.max(...known.map(point => point.value ?? 0))} 条`
    : '暂无已知值'
  return `日线索数趋势，${span}，${peak}${unknown > 0 ? `，另有 ${unknown} 日未知不补 0` : ''}`
})
</script>

<template>
  <div class="ltc" data-testid="lane-trend-chart">
    <span v-if="label" class="ltc-label">{{ label }}</span>

    <div v-if="loading" class="ltc-state" data-testid="lane-trend-loading">逐日数据加载中…</div>
    <div v-else-if="count === 0" class="ltc-state" data-testid="lane-trend-empty">{{ emptyText }}</div>
    <div v-else-if="!hasKnown" class="ltc-state" data-testid="lane-trend-unknown">
      本区间线索数未知（未取到，不补 0）。
    </div>

    <TrendAxes v-else unit-label="线索数（条）" :y-ticks="yTicks" :x-ticks="xTicks">
      <svg
        class="ltc-plot"
        viewBox="0 0 480 74"
        preserveAspectRatio="none"
        role="img"
        :aria-label="ariaSummary"
        data-testid="lane-trend-plot"
      >
        <line class="ltc-baseline" x1="0" x2="480" :y1="GEOMETRY.baselineY" :y2="GEOMETRY.baselineY" />
        <polyline
          v-for="(segment, segmentIndex) in segments.filter(segment => segment.length > 1)"
          :key="`seg-${segmentIndex}`"
          class="ltc-line"
          :points="segmentPoints(segment)"
        />
        <template v-for="(segment, segmentIndex) in segments" :key="`dots-${segmentIndex}`">
          <circle
            v-for="point in segment"
            :key="point.date"
            class="ltc-dot"
            :class="{ 'is-selected': point.date === selectedDate }"
            :cx="xAt(indexOfDate.get(point.date) ?? 0)"
            :cy="yAt(point.value ?? 0)"
            r="4"
            :data-date="point.date"
            :data-value="point.value"
            role="button"
            :aria-label="`${point.date} 当日线索数 ${point.value} 条`"
            @click="onSelectPoint(point)"
          >
            <title>{{ point.date }} · 当日线索数 {{ point.value }} 条</title>
          </circle>
        </template>
        <line
          v-if="selectedIndex !== null"
          class="ltc-cursor"
          :x1="xAt(selectedIndex)"
          :x2="xAt(selectedIndex)"
          y1="0"
          :y2="GEOMETRY.baselineY"
        />
      </svg>
    </TrendAxes>
  </div>
</template>

<style scoped>
.ltc {
  min-width: 0;
}

.ltc-label {
  display: block;
  font-size: 12px;
  color: var(--color-text-muted);
  margin-bottom: 4px;
}

.ltc-state {
  font-size: 12px;
  color: var(--color-text-muted);
  padding: 10px 0;
}

.ltc-plot {
  display: block;
  width: 100%;
  height: 88px;
  overflow: visible;
  margin: 0;
}

.ltc-baseline {
  stroke: var(--color-border-medium);
  pointer-events: none;
}

.ltc-line {
  fill: none;
  stroke: var(--color-accent);
  stroke-width: 1.5;
  vector-effect: non-scaling-stroke;
  pointer-events: none;
}

.ltc-dot {
  fill: var(--color-accent);
  cursor: pointer;
}

.ltc-dot.is-selected {
  stroke: var(--color-accent);
  stroke-width: 2;
  fill: var(--color-bg-base);
}

.ltc-cursor {
  stroke: var(--color-accent);
  stroke-dasharray: 3 3;
  pointer-events: none;
}
</style>
