<script setup lang="ts">
/**
 * 窄屏泳道行列表（G3 内聚拆分，纯视图）。
 *
 * 正式行与休眠行同一套行结构（名称/追踪/休眠徽标 + 14 日分区数迷你条 + 线索/分区数 + 选中后
 * 的「下方编年」入口）；迷你条分档由父级按可见行统一算好（`cells`），本组件不重算阈值/计数，
 * 不补 0。只 emit 选中/编年意图，不改日期、不滚动页面。
 */
import type { ObservationLaneRow } from './observationLanes'

defineProps<{
  /** 行 + 已算好的迷你条格（date/bucket class/title）。 */
  rows: { row: ObservationLaneRow, cells: { date: string, bucket: string, title: string }[] }[]
  /** 迷你条列数（= 可见区间最后 14 日）。 */
  cols: number
  selectedLaneId: string | null
  /** 列表 testid（正式 `narrow-active-lanes` / 休眠 `narrow-dormant-lanes`）。 */
  testid: string
  dormant?: boolean
}>()

const emit = defineEmits<{
  selectLane: [topicId: string]
  gotoChronicle: []
}>()
</script>

<template>
  <ol v-if="rows.length > 0" class="bns-list" :data-testid="testid">
    <li
      v-for="{ row, cells } in rows"
      :key="row.topicId"
      class="bns-row"
      :class="{ 'is-selected': row.topicId === selectedLaneId, 'is-dormant': dormant === true }"
      :data-testid="`narrow-lane-row-${row.topicId}`"
    >
      <button
        type="button"
        class="bns-row-main"
        :data-testid="`narrow-lane-${row.topicId}`"
        :aria-pressed="row.topicId === selectedLaneId"
        @click="emit('selectLane', row.topicId)"
      >
        <span class="bns-name" :title="row.label">
          {{ row.label }}
          <span v-if="row.watchLinked" class="bns-watch">追踪</span>
          <span v-if="dormant" class="bns-dormant">休眠</span>
        </span>
        <span class="bns-mini" aria-hidden="true" :style="{ '--bns-cols': cols }">
          <i
            v-for="cell in cells"
            :key="cell.date"
            class="bns-cell"
            :class="cell.bucket"
            :title="cell.title"
          />
        </span>
        <span class="bns-meta">{{ row.clueTotal }} 线索 · {{ row.sectionTotal }} 分区</span>
      </button>
      <button
        v-if="row.topicId === selectedLaneId"
        type="button"
        class="bns-chronicle"
        data-testid="narrow-goto-chronicle"
        @click="emit('gotoChronicle')"
      >下方编年 / 当天线索 ↓</button>
    </li>
  </ol>
</template>

<style scoped>
.bns-list {
  display: flex;
  flex-direction: column;
  gap: 0;
  margin: 0;
  padding: 0;
  list-style: none;
}

.bns-row {
  border-bottom: 1px solid var(--color-border-subtle, var(--color-border-medium));
}

.bns-row.is-selected {
  background: color-mix(in srgb, var(--color-accent) 6%, transparent);
}

.bns-row-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 0.4rem;
  width: 100%;
  padding: 0.65rem 0.1rem;
  border: 0;
  background: none;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.bns-name {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  min-width: 0;
  overflow: hidden;
  color: var(--color-text-primary);
  font-size: 0.8rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bns-watch,
.bns-dormant {
  flex: 0 0 auto;
  padding: 0 0.2rem;
  border: 1px solid color-mix(in srgb, var(--color-accent) 30%, transparent);
  border-radius: 2px;
  color: var(--color-accent);
  font-size: 0.5rem;
}

.bns-dormant {
  border-color: var(--color-border-medium);
  color: var(--color-text-muted);
}

.bns-mini {
  display: grid;
  grid-template-columns: repeat(var(--bns-cols, 14), minmax(0, 1fr));
  gap: 0.15rem;
  width: 100%;
}

.bns-cell {
  height: 0.45rem;
  border-radius: 1px;
  background: var(--color-bg-hover);
}

.bns-cell.is-unknown {
  background: transparent;
  border: 1px dashed var(--color-border-medium);
}

.bns-cell.is-b1 { background: color-mix(in srgb, var(--color-accent) 16%, transparent); }
.bns-cell.is-b2 { background: color-mix(in srgb, var(--color-accent) 30%, transparent); }
.bns-cell.is-b3 { background: color-mix(in srgb, var(--color-accent) 48%, transparent); }
.bns-cell.is-b4 { background: color-mix(in srgb, var(--color-accent) 72%, transparent); }

.bns-meta {
  color: var(--color-text-muted);
  font-family: var(--font-mono, monospace);
  font-size: 0.58rem;
}

.bns-chronicle {
  margin: 0 0 0.55rem;
  padding: 0.25rem 0.5rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 5px;
  background: none;
  color: var(--color-accent);
  font-size: 0.7rem;
  cursor: pointer;
}
</style>
