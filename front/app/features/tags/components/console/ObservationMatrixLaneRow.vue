<script setup lang="ts">
/**
 * 矩阵单条泳道行（rework-workshop-into-radar-and-desk 8C G2；8C.4S 结构规范包从
 * `ObservationMatrixGrid.vue` 按内聚行为抽出，只 props/emits + 与其他组共用的行/格样式）。
 *
 * DOM：**多根片段**（标签按钮 + 逐个日格），不包 wrapper——父网格按列直接收这些元素；
 * 插包装层会破坏 sticky 标签列/列对齐。正式行与休眠行共用本组件（`dormant` 只切弱化视觉与
 * 「休眠」标），格值/可点性/title/aria 全由父投影好的 `cells` 携带，本组件不解析日内容。
 *
 * 交互契约不变：点标签 emit `selectLane(topicId)`；点格 emit `selectCell({ row, cell, date })`
 * 由父统一判定可开性（禁用格的真实点击不可达，合成事件兜底提示在父层，本组件不弹浮层）。
 *
 * 色深（2026-11-11 用户看版后修订，tasks 3.5）：格底色 = 格视图自带的 `depth`（当日线索合计在
 * 本行**可见窗**内的归一深浅，行内峰值最深、0/未知不着色）；颜色只是冗余强化通道，格值数字与
 * title/aria 仍是唯一信息承载。本组件不自己算分母（归一分母由父/纯模型注入）。
 */
import { clueMix, type MatrixCellView, type ObservationLaneRow } from './observationLanes'

const props = defineProps<{
  row: ObservationLaneRow
  /** 与父 `axis` 等长的逐日投影（`matrixCellView` 产物）。 */
  cells: MatrixCellView[]
  /** 可见观察轴（升序 date-only；仅用于取每格的日期键/选中交点）。 */
  axis: string[]
  today: string
  selectedLaneId: string | null
  selectedDate: string | null
  /** 休眠行：弱化 + 「休眠」标（分组/折叠态由父持有）。 */
  dormant: boolean
}>()

const emit = defineEmits<{
  selectLane: [topicId: string]
  /** 点格：可开性判定与提示在父层（禁用格不 emit selectDate）。 */
  selectCell: [payload: { row: ObservationLaneRow, cell: MatrixCellView, date: string }]
}>()

/** 行头用 source 全窗指标；不随可见日期裁切重算排序或统计。 */
function rowTitle(): string {
  return `${props.row.label}：${props.row.clueTotal} 条线索 · ${props.row.sectionTotal} 个主题桶`
}

function isToday(date: string): boolean {
  return date === props.today
}

/** 单格色深混入比例；depth 0（零线索/未知/断档）返回空串 → 不注入样式，保持底色与未知态。 */
function cellMix(cell: MatrixCellView): string {
  return clueMix(cell.depth)
}

function cellClass(cell: MatrixCellView, date: string): unknown[] {
  return [
    {
      'is-today': isToday(date),
      'is-gap': cell.kind === 'gap',
      'is-unknown': cell.kind === 'unavailable',
      'is-selected': props.row.topicId === props.selectedLaneId && date === props.selectedDate,
      'is-clue-depth': cellMix(cell) !== '',
    },
  ]
}

/** 色深经 CSS 变量交给主题色阶（`--color-accent` mix `--color-bg-base`），不新增第二份色板。 */
function cellStyle(cell: MatrixCellView): Record<string, string> {
  const mix = cellMix(cell)
  return mix === '' ? {} : { '--omx-clue-mix': mix }
}
</script>

<template>
  <button
    type="button"
    class="omx-lane"
    :class="{ 'is-selected': row.topicId === selectedLaneId, 'is-dormant': dormant }"
    :title="rowTitle()"
    :aria-label="rowTitle()"
    :aria-pressed="row.topicId === selectedLaneId"
    :data-testid="`matrix-lane-${row.topicId}`"
    :data-lane-id="row.topicId"
    @click="emit('selectLane', row.topicId)"
  >
    <span class="omx-lane-name">{{ row.label }}</span>
    <span v-if="dormant" class="omx-lane-dormant-tag">休眠</span>
    <span v-else-if="row.watchLinked" class="omx-lane-watch">追踪</span>
  </button>
  <button
    v-for="(cell, cellIndex) in cells"
    :key="`${row.topicId}-${axis[cellIndex]}`"
    type="button"
    class="omx-cell"
    :class="[cellClass(cell, axis[cellIndex]!), { 'is-dormant': dormant }]"
    :style="cellStyle(cell)"
    :disabled="!cell.openable"
    :data-testid="`matrix-cell-${row.topicId}-${axis[cellIndex]}`"
    :data-lane-id="row.topicId"
    :data-date="axis[cellIndex]"
    :aria-label="cell.aria"
    :title="cell.title"
    @click="emit('selectCell', { row, cell, date: axis[cellIndex]! })"
  >{{ cell.text }}</button>
</template>

<style scoped src="./observationMatrixRows.css" />
