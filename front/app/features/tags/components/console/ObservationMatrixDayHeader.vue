<script setup lang="ts">
/**
 * 矩阵日期列头（rework-workshop-into-radar-and-desk 8C G2；8C.4S 结构规范包从
 * `ObservationMatrixGrid.vue` 按内聚行为抽出，只 props/emits + 自己那截列头样式）。
 *
 * 纯受控：`reportDates` 三态——省略 = 未接线（全不可点，旧调用零改动）；`null` = 已接线但
 * 索引未就绪/失败（不可点且不冒「无日报」）；`Set` = 真实报日（只有集合内日期是原生 button，
 * 点它 emit `selectDayHeader(date)`，与日格共用壳层同一 `selectedDate`）。
 *
 * DOM：**多根片段**（corner + 逐个列头），不包 wrapper——父网格 `grid-template-columns` 直接
 * 收这些元素为列，插入包装层会破坏列对齐/横滚。列头 title/aria 的文案口径与索引提示一致
 * （索引陈旧时不说「该日没有日报」）。
 */
import { computed, ref } from 'vue'
import type { ObservedDayNode } from '~/features/tags/composables/useOtherTopicObservation'
import DateAssociationMarker from './DateAssociationMarker.vue'

const props = defineProps<{
  /** 可见观察轴（升序 date-only；与行投影同一份）。 */
  axis: string[]
  observedDays?: ObservedDayNode[]
  today: string
  selectedDate: string | null
  /** 真实报日集合：省略 = 未接线；null = 已接线未就绪/失败；Set = 已知报日可选。 */
  reportDates?: ReadonlySet<string> | null
  /** 报日索引失败（陈旧）文案：只用于列头 title，不一刀禁掉已知报日。 */
  indexError?: string | null
}>()

const emit = defineEmits<{
  /** 点报日列头：与日格同一 `selectedDate` 的第三处入口（壳层统一写状态）。 */
  selectDayHeader: [date: string]
  selectObservedDay: [node: ObservedDayNode]
}>()

const hoveredDate = ref<string | null>(null)
const focusedDate = ref<string | null>(null)
const observedByDate = computed(() => new Map((props.observedDays ?? []).map(node => [node.date, node])))

const scrollable = computed(() => props.axis.length > 31)
const columnLabel = computed(() => props.axis.map(date => (date === props.today ? '今天' : date.slice(5))))

function monthOf(date: string): string { return date.slice(5, 7) }
function dayOf(date: string): string { return date.slice(8, 10) }

function isToday(date: string): boolean {
  return date === props.today
}

/** 列头三态：report = 可点报日；plain = 非报日（不可点）；unknown = 索引未就绪（不可点，不冒无日报）。 */
function headerState(date: string): 'report' | 'plain' | 'unknown' {
  const dates = props.reportDates
  if (dates === undefined) return 'plain'
  if (dates === null) return 'unknown'
  return dates.has(date) ? 'report' : 'plain'
}

function isReportDay(date: string): boolean {
  return headerState(date) === 'report' || observedByDate.value.has(date)
}

/** 列头 title/aria：报日给「选这一天」，非报日诚实说明不可选原因（索引陈旧时不冒「无日报」）。 */
function headerTitle(date: string): string {
  const today = isToday(date) ? ' 今天' : ''
  const node = observedByDate.value.get(date)
  if (node) return `${date}${today} · 同归属记录 · ${node.sectionTitles.join('；')} · 读取该日首个真实分区`
  if (isReportDay(date)) return `${date}${today} · 选这一天`
  if (props.reportDates === undefined) return date
  if (headerState(date) === 'unknown') return `${date} · 报日索引未就绪，暂不可选`
  if (props.indexError) return `${date} · 报日索引可能已陈旧，暂不可选`
  return `${date} · 该日没有日报`
}

function select(date: string): void {
  if (!isReportDay(date)) return
  const node = observedByDate.value.get(date)
  if (node) emit('selectObservedDay', node)
  else emit('selectDayHeader', date)
}
</script>

<template>
  <div class="omx-corner" />
  <component
    :is="isReportDay(axis[index]!) ? 'button' : 'div'"
    v-for="(label, index) in columnLabel"
    :key="axis[index]"
    :type="isReportDay(axis[index]!) ? 'button' : undefined"
    :aria-pressed="isReportDay(axis[index]!) ? axis[index] === selectedDate : undefined"
    class="omx-dayhead"
    :class="{
      'is-today': isToday(axis[index]!),
      'is-report': isReportDay(axis[index]!),
      'is-selected': axis[index] === selectedDate,
    }"
    :data-testid="`matrix-dayhead-${axis[index]}`"
    :data-date="axis[index]"
    :title="headerTitle(axis[index]!)"
    :aria-label="headerTitle(axis[index]!)"
    @click="select(axis[index]!)"
    @mouseenter="hoveredDate = axis[index]!"
    @mouseleave="hoveredDate = null"
    @focus="focusedDate = axis[index]!"
    @blur="focusedDate = null"
  >
    <DateAssociationMarker
      v-if="observedByDate.has(axis[index]!)"
      :node="observedByDate.get(axis[index]!)!"
      :visible="hoveredDate === axis[index] || focusedDate === axis[index]"
    />
    <template v-if="scrollable">{{ label }}</template>
    <template v-else>
      <span class="omx-day-month">{{ monthOf(axis[index]!) }}</span>
      <span class="omx-day-day">{{ dayOf(axis[index]!) }}</span>
    </template>
  </component>
</template>

<style scoped>
/* sticky 左列占位与列头视觉；网格列定义/横滚容器仍归父 ObservationMatrixGrid。 */
.omx-corner {
  position: sticky;
  left: 0;
  z-index: 3;
  height: 1.4rem;
  background: var(--color-bg-base);
}

.omx-dayhead {
  position: relative;
  min-width: 0;
  padding: 0;
  border: none;
  background: none;
  font-size: 0.66rem;
  color: var(--color-text-muted);
  text-align: center;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.omx-dayhead.is-report {
  color: var(--color-text-secondary);
  cursor: pointer;
  border-bottom: 1px solid var(--color-border-medium);
}

.omx-dayhead.is-report:hover,
.omx-dayhead.is-report:focus-visible {
  z-index: 4;
  color: var(--color-accent);
  border-bottom-color: var(--color-accent);
}

.omx-dayhead.is-report.is-selected {
  color: var(--color-accent);
  border-bottom-color: var(--color-accent);
  font-weight: 600;
}

.omx-day-month,
.omx-day-day {
  display: block;
  line-height: 1.1;
}

.omx-day-month {
  opacity: 0.72;
}

.omx-dayhead.is-today {
  color: var(--color-accent);
  font-weight: 600;
}
</style>
