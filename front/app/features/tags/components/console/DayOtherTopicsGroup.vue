<script setup lang="ts">
/**
 * 当日其他话题折叠组（rework-workshop-into-radar-and-desk 矩阵阶段 8C G2）。
 *
 * 纯受控：行/计数/状态文案全部来自壳层用 G1 `buildOtherTopics` 派生后经 props 下发的
 * `OtherTopicsView`。本组件**不自建网络/日期/身份排除/行序/计数**，也不持有第二份展开态
 * （真实 `othersOpen` 由 G4 壳持有，首份 A 响应后初始化一次；本组件只渲染 + emit）。
 *
 * DOM：summary（全宽、原生 button + `aria-expanded`）在前，自身行紧随其后；行只在
 * `expanded && state === 'ready'` 时进入 DOM——折叠时行离 DOM，summary 仍显示真实计数/状态。
 * 每行 = 标题（与正式行相同单行省略 + 完整 `title`/`aria-label`，含当时状态词）+ **仅所选日列**的线索数格
 * （0 合法可点）；其余日期空白 `aria-hidden` 不可聚焦（不伪造跨日泳道）。
 * 选行与选格发同一 `selectOther` 事件 `{ sectionId, date, topicId }`（不弹层/不强滚/不抢焦点）。
 *
 * 视觉复用 `observationMatrixRows.css`，列对齐沿用父网格，本组件不另立样式语言。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import {
  OTHER_TOPICS_EMPTY_TEXT,
  OTHER_TOPICS_NO_DATE_TEXT,
  type OtherTopicRow,
  type OtherTopicsView,
} from './dayOtherTopics'

const props = defineProps<{
  /** 可见观察轴（升序 date-only；与正式行同一份，只用于对齐列）。 */
  axis: string[]
  /** 壳层唯一选中日（组只在该列标数）。 */
  selectedDate: string | null
  /** G1 纯模型视图（null = 壳层尚未接线，按「未选日」诚实降级）。 */
  view: OtherTopicsView | null
  /** 展开态（壳层持有；省略 = 收起）。 */
  expanded?: boolean
  /** 当前阅读槽选中的其他分区（高亮用）。 */
  selectedSectionId?: string | null
}>()

const emit = defineEmits<{
  toggleOthers: []
  /** 选行与选格同一事件：`sectionId` 是当日局部身份，`date` 固定为所选日。 */
  selectOther: [payload: { sectionId: string, date: string, topicId?: string | null }]
}>()

const open = computed(() => props.expanded === true)
const ready = computed(() => props.view?.state === 'ready')
const rows = computed<OtherTopicRow[]>(() => (ready.value ? props.view?.rows ?? [] : []))
/** 计数只在 ready 出现（0 合法）；取**组内线索合计**（与矩阵主行格值同口径），不是分区数。 */
const countText = computed(() => (
  typeof props.view?.clueCount === 'number' ? `${props.view.clueCount} 条线索` : null
))
/** 只有 ready 可展开：非 ready 没有可读行，展开也无内容。 */
const toggleable = computed(() => ready.value)

const noteText = computed(() => {
  const view = props.view
  if (view === null) return OTHER_TOPICS_NO_DATE_TEXT
  if (view.state !== 'ready') return view.statusText
  if (view.rows.length === 0) return OTHER_TOPICS_EMPTY_TEXT
  return open.value ? '只标所选日' : `${view.rows.length} 个分区未入泳道`
})

/** 仅视觉分档：与正式格同一阈值（observationLanes.matrixCellView 的桶峰值分档）。 */
function bucketOf(count: number): string {
  if (count <= 0) return ''
  return count >= 4 ? 'is-b4' : `is-b${count}`
}

/** 行 title/aria：完整标题 + 当时状态词 + 点下方读全文（不截断）。 */
function rowTitle(row: OtherTopicRow): string {
  return `${row.label} · ${row.statusText} · 点下方读全文`
}

function selectRow(row: OtherTopicRow): void {
  const date = props.selectedDate
  if (date === null) return
  emit('selectOther', { sectionId: row.sectionId, date, topicId: row.persistentTopicId })
}
</script>

<template>
  <div class="otg" data-testid="other-topics-group">
    <div class="omx-group-summary" data-testid="other-topics-toggle">
      <button
        type="button"
        class="omx-group-btn"
        :aria-expanded="open"
        :disabled="!toggleable"
        @click="emit('toggleOthers')"
      >
        <Icon :icon="open ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="14" />
        <span class="omx-group-title">
          当日其他话题<span v-if="countText !== null" class="omx-group-count" data-testid="other-topics-count"> · {{ countText }}</span>
        </span>
        <span class="omx-group-note" data-testid="other-topics-note">{{ noteText }}</span>
      </button>
      <span v-if="view && view.note" class="omx-group-extra" data-testid="other-topics-extra">{{ view.note }}</span>
    </div>

    <template v-if="open && rows.length > 0">
      <template v-for="row in rows" :key="row.sectionId">
        <button
          type="button"
          class="omx-lane omx-other-lane"
          :class="{ 'is-selected': row.sectionId === selectedSectionId }"
          :title="rowTitle(row)"
          :aria-label="rowTitle(row)"
          :aria-pressed="row.sectionId === selectedSectionId"
          :data-testid="`other-topic-row-${row.sectionId}`"
          :data-section-id="row.sectionId"
          @click="selectRow(row)"
        >
          <span class="omx-lane-name omx-other-name">{{ row.label }}</span>
        </button>
        <template v-for="date in axis" :key="`${row.sectionId}-${date}`">
          <template v-if="date === selectedDate">
            <button
              type="button"
              class="omx-cell omx-other-cell"
              :class="[bucketOf(row.threadCount), { 'is-selected': row.sectionId === selectedSectionId }]"
              :data-testid="`other-topic-cell-${row.sectionId}-${date}`"
              :data-section-id="row.sectionId"
              :data-date="date"
              :aria-label="`${row.label} · ${date} · ${row.threadCount} 条线索 · 就地下读`"
              :title="`${date} · ${row.threadCount} 条线索 · 就地下读`"
              @click="selectRow(row)"
            >{{ row.threadCount }}</button>
          </template>
          <div v-else class="omx-other-blank" aria-hidden="true" />
        </template>
      </template>
    </template>

    <div v-else-if="open" class="omx-group-noterow" data-testid="other-topics-note-row">
      <span class="omx-group-notetext">{{ noteText }}</span>
    </div>
  </div>
</template>

<style scoped src="./observationMatrixRows.css" />

<style scoped>
/* 参与父网格：summary 与自身行直接成为 ObservationMatrixGrid 的 grid item。 */
.otg {
  display: contents;
}

.omx-other-blank {
  height: 2.9rem;
  min-width: 0;
}
</style>
