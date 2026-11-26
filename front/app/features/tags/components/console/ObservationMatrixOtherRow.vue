<script setup lang="ts">
/**
 * 矩阵「其他话题（未入泳道）」整行（change `lane-matrix-coverage-and-others-row` 4.3）。
 *
 * 这一行**不是泳道**：它是「当日未入任何泳道的分区里有多少条非综合稿线索」的逐日读数，
 * 让用户一眼看出哪几天的其他话题多，而不必一天一天点开「当日其他话题」组。
 *
 * DOM：**多根片段**（行头容器 + 逐个日格），与 `ObservationMatrixLaneRow` 同款——不包整行
 * wrapper，父网格按列直接收这些元素（插包装层会破坏 sticky 标签列/列对齐）。
 *
 * 口径（与泳道日格、当日其他话题组**同一量纲**）：
 *  - 格值 = 当日未入泳道分区的**非综合稿线索数**（`clues`）；0 是合法值、可点；
 *  - 无该日报日 → 「—」禁用（不把「没有日报」当成「0 条」）；
 *  - 色深 = 该值在**本行**可见窗内的归一深浅（`clueDepth`/`clueMix`，与泳道格同一实现，
 *    行内归一、跨行不归一）。
 *
 * 交互边界：点格只做两件事——切选中日 + 展开「当日其他话题」组（emit `selectOthersDay`）；
 * 不弹窗、不发写请求、不提供泳道生命周期操作（行头**无**「⋯」菜单）。
 */
import { computed } from 'vue'
import type { WindowOtherTopicDay } from '~/api/normalizers/laneDynamicsWindow'
import { clueDepth, clueMix } from './observationLanes'

const props = defineProps<{
  /** 可见观察轴（升序 date-only；与泳道行同一份，只用于对齐列）。 */
  axis: string[]
  /** 壳层唯一选中日（同一份 `selectedDate`，不新增第二份选中态）。 */
  selectedDate: string | null
  today: string
  /** date → 当日未入泳道分区数与非综合稿线索数（后端 `other_topics` 归一结果）。 */
  days: Record<string, WindowOtherTopicDay>
  /**
   * 逐日读数的**档窗起点**（YYYY-MM-DD）：后端只聚合 `[anchor-(days-1), anchor]`。
   * 该日之前没有读数是「本次聚合没算」而不是「没有日报」——文案要分得开。
   */
  since?: string | null
}>()

const emit = defineEmits<{
  /** 点有日报的格：切选中日 + 展开「当日其他话题」组（壳层统一写状态）。 */
  selectOthersDay: [date: string]
}>()

/** 行内归一分母：本行可见窗内最大线索数（0/无数据 → 整行不着色）。 */
const rowMax = computed(() => {
  let max = 0
  for (const date of props.axis) {
    const clues = props.days[date]?.clues
    if (typeof clues === 'number' && Number.isFinite(clues) && clues > max) max = clues
  }
  return max
})

function entryOf(date: string): WindowOtherTopicDay | undefined {
  return props.days[date]
}

function cellClass(date: string): Record<string, boolean> {
  const entry = entryOf(date)
  return {
    'is-today': date === props.today,
    // 选中列淡底只铺在**有数据**的那天（与泳道格同一纪律）。
    'is-col-selected': date === props.selectedDate && entry !== undefined,
    'is-clue-depth': entry !== undefined && clueMix(clueDepth(entry.clues, rowMax.value)) !== '',
  }
}

function cellStyle(date: string): Record<string, string> {
  const entry = entryOf(date)
  if (!entry) return {}
  const mix = clueMix(clueDepth(entry.clues, rowMax.value))
  return mix === '' ? {} : { '--omx-clue-mix': mix }
}

function cellText(date: string): string {
  const entry = entryOf(date)
  return entry ? String(entry.clues) : '—'
}

/** 有日报日：日期 + 分区数 + 线索数（数值与分区数都进 title/aria，颜色不是唯一信息载体）。 */
function cellLabel(date: string): string {
  const entry = entryOf(date)
  if (entry) return `${date} · ${entry.sections} 个分区 · ${entry.clues} 条线索`
  // 档窗外（观察范围拖进 365 池区段）：后端本次没聚合这一天 —— 不能说成「无日报」。
  if (props.since && date < props.since) return `${date}：档窗外，其他话题读数未取到`
  return `${date}：无日报`
}

/** 只接受有日报的格（无日报的格是 disabled，这里只是兜底）。 */
function onCellClick(date: string): void {
  if (entryOf(date) === undefined) return
  emit('selectOthersDay', date)
}
</script>

<template>
  <div class="omx-lane omx-other-row-head" data-testid="matrix-other-row">
    <span class="omx-lane-name omx-other-name" title="未进入任何泳道的分区线索数">其他话题</span>
    <span class="omx-lane-dormant-tag">未入泳道</span>
  </div>
  <button
    v-for="date in axis"
    :key="date"
    type="button"
    class="omx-cell omx-other-cell"
    :class="cellClass(date)"
    :style="cellStyle(date)"
    :disabled="entryOf(date) === undefined"
    :data-testid="`matrix-other-cell-${date}`"
    :data-date="date"
    :aria-label="cellLabel(date)"
    :title="cellLabel(date)"
    @click="onCellClick(date)"
  >{{ cellText(date) }}</button>
</template>

<style scoped src="./observationMatrixRows.css" />

<style scoped>
/* 行头：与泳道行同一列宽/行高，但不可选中（不是泳道），故无「⋯」菜单、无 hover 指针。 */
.omx-other-row-head {
  cursor: default;
}

.omx-other-row-head .omx-other-name {
  color: var(--color-text-secondary);
}

/* 格边框用虚线区分「不是泳道」；色阶仍是泳道格同一套（不新增第二份色板/语义色）。 */
.omx-other-cell {
  border-style: dashed;
  border: 1px dashed var(--color-border-subtle);
}

.omx-other-cell:disabled {
  background: transparent;
  color: var(--color-text-muted);
}
</style>
