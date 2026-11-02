<script setup lang="ts">
/**
 * 窄屏同源报日条（G3 内聚拆分，纯视图 + 自身几何）。
 *
 * 轴 = 壳层同一个连续 `visibleAxis`：只有 `reportDates` 内日期渲染为原生 `BUTTON`
 * （`aria-pressed`），非报日保留为 `SPAN aria-hidden`（不压轴、不可聚焦）。索引加载/失败只给
 * 说明，不冒「无日报」。选中日定位**只写日期条自身 `scrollLeft`**（挂载/选日/轴变化后 nextTick
 * 测量 + 双向 clamp），不调用 `scrollIntoView`/`window.scrollTo`，不改变用户纵向滚动与焦点。
 */
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import type { ObservedDayNode } from '~/features/tags/composables/useOtherTopicObservation'
import DateAssociationMarker from './DateAssociationMarker.vue'

const props = defineProps<{
  /** 可见观察轴（升序连续）。 */
  axis: string[]
  observedDays?: ObservedDayNode[]
  /** G1 真实报日集合；null = 已接线但索引未就绪/失败。 */
  reportDates?: ReadonlySet<string> | null
  /** 壳层唯一选中日。 */
  selectedDate?: string | null
  indexLoading?: boolean
  indexError?: string | null
}>()

const emit = defineEmits<{
  /** 点一个有日报的日期（写同一 selectedDate）。 */
  selectDay: [date: string]
  selectObservedDay: [node: ObservedDayNode]
}>()

const stripHint = computed<string | null>(() => {
  if (props.indexLoading) return '读取报日…'
  if (props.indexError) return '报日索引未取到，日期条暂不可选'
  if (props.reportDates === null) return '报日索引未就绪，日期条暂不可选'
  return null
})

const hoveredDate = ref<string | null>(null)
const focusedDate = ref<string | null>(null)
const observedByDate = computed(() => new Map((props.observedDays ?? []).map(node => [node.date, node])))

function dayTitle(date: string): string {
  const node = observedByDate.value.get(date)
  return node ? `${date} · 同归属记录 · ${node.sectionTitles.join('；')} · 读取该日首个真实分区` : `${date} · 选这一天`
}

function hasReport(date: string): boolean {
  return (props.reportDates?.has(date) ?? false) || observedByDate.value.has(date)
}

function shortDate(date: string): string {
  return date.slice(5).replace('-', '/')
}

function selectReportDay(date: string): void {
  const node = observedByDate.value.get(date)
  if (node) emit('selectObservedDay', node)
  else emit('selectDay', date)
}

const stripTrack = ref<HTMLElement | null>(null)

function centerSelectedDate(): void {
  const track = stripTrack.value
  const selected = props.selectedDate ?? null
  if (track === null || selected === null) return
  const target = track.querySelector<HTMLElement>(`[data-strip-date="${selected}"]`)
  if (target === null) return
  const max = Math.max(0, track.scrollWidth - track.clientWidth)
  const centered = target.offsetLeft - Math.max(0, (track.clientWidth - target.offsetWidth) / 2)
  track.scrollLeft = Math.min(max, Math.max(0, centered))
}

onMounted(centerSelectedDate)
watch(
  () => [props.selectedDate ?? null, props.axis.join('|'), props.reportDates] as const,
  () => { void nextTick(centerSelectedDate) },
)
</script>

<template>
  <div class="bns-strip" data-testid="narrow-day-strip">
    <div class="bns-strip-head">
      <span class="bns-strip-kicker">日期轴 · 与矩阵列头同源</span>
      <span v-if="stripHint" class="bns-strip-hint" data-testid="narrow-day-strip-hint">{{ stripHint }}</span>
    </div>
    <div ref="stripTrack" class="bns-strip-track" data-testid="narrow-day-strip-track" aria-label="报日日期条">
      <template v-for="date in axis" :key="date">
        <button
          v-if="hasReport(date)"
          type="button"
          class="bns-strip-day is-report"
          :class="{ 'is-selected': date === selectedDate }"
          :data-strip-date="date"
          :data-testid="`narrow-dayhead-${date}`"
          :aria-pressed="date === selectedDate"
          :title="dayTitle(date)"
          :aria-label="dayTitle(date)"
          @click="selectReportDay(date)"
          @mouseenter="hoveredDate = date"
          @mouseleave="hoveredDate = null"
          @focus="focusedDate = date"
          @blur="focusedDate = null"
        >
          <DateAssociationMarker
            v-if="observedByDate.has(date)"
            :node="observedByDate.get(date)!"
            :visible="hoveredDate === date || focusedDate === date"
          />
          {{ shortDate(date) }}
        </button>
        <span
          v-else
          class="bns-strip-day is-off"
          :class="{ 'is-selected': date === selectedDate }"
          :data-strip-date="date"
          :data-testid="`narrow-day-${date}`"
          aria-hidden="true"
        >{{ shortDate(date) }}</span>
      </template>
    </div>
    <p v-if="stripHint" class="bns-strip-foot">轴保持连续；没有日报的日期不可选。</p>
  </div>
</template>

<style scoped>
.bns-strip {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  min-width: 0;
}

.bns-strip-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.6rem;
}

.bns-strip-kicker {
  color: var(--color-text-muted);
  font-size: 0.58rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.bns-strip-hint {
  color: var(--color-text-muted);
  font-size: 0.62rem;
}

.bns-strip-track {
  display: flex;
  gap: 0.25rem;
  overflow-x: auto;
  padding: 0.15rem 0 0.35rem;
  scrollbar-width: thin;
  -webkit-overflow-scrolling: touch;
  min-width: 0;
}

.bns-strip-day {
  position: relative;
  flex: 0 0 auto;
  padding: 0.22rem 0.4rem;
  border: 1px solid transparent;
  border-radius: 5px;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.64rem;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.bns-strip-day.is-report {
  border-color: var(--color-border-medium);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.bns-strip-day.is-report:hover,
.bns-strip-day.is-report:focus-visible {
  z-index: 4;
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.bns-strip-day.is-selected {
  border-color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 10%, transparent);
  color: var(--color-accent);
}

.bns-strip-day.is-off {
  opacity: 0.55;
}

.bns-strip-foot {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.6rem;
}
</style>
