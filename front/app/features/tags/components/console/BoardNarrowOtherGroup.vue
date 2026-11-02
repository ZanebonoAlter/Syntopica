<script setup lang="ts">
/**
 * 窄屏「当日其他话题」折叠组（G3 内聚拆分）。
 *
 * 计数/状态词/行集全部来自 G1 `OtherTopicsView`（唯一模型），本组件只做同源简单渲染：
 * summary 在前、行标题两行 clamp + 完整 title/aria（含当时状态词与「点下方读全文」）、
 * 只标所选日线索数（0 合法可点）。展开态由壳层持有（与休眠组互不串联），点击只 emit。
 */
import { computed } from 'vue'
import {
  OTHER_TOPICS_EMPTY_TEXT,
  OTHER_TOPICS_NO_DATE_TEXT,
  type OtherTopicRow,
  type OtherTopicsView,
} from './dayOtherTopics'
import BoardNarrowGroupToggle from './BoardNarrowGroupToggle.vue'

const props = defineProps<{
  /** G1 buildOtherTopics 视图（null = 未接线的中性文案）。 */
  view: OtherTopicsView | null
  /** 其他组展开态（壳层持有）。 */
  open: boolean
  selectedOtherSectionId?: string | null
}>()

const emit = defineEmits<{
  toggle: []
  select: [row: OtherTopicRow]
}>()

const rows = computed<OtherTopicRow[]>(() => (props.view?.state === 'ready' ? props.view.rows : []))
const count = computed<string | null>(() => (typeof props.view?.count === 'number' ? String(props.view.count) : null))

const note = computed<string>(() => {
  const view = props.view
  if (view === null) return OTHER_TOPICS_NO_DATE_TEXT
  if (view.state !== 'ready') return view.statusText ?? ''
  if (view.rows.length === 0) return OTHER_TOPICS_EMPTY_TEXT
  return props.open
    ? `${view.rows.length} 个分区 · 只标所选日`
    : `${view.rows.length} 个分区未入泳道`
})

function toggle(): void {
  if (props.view?.state !== 'ready') return
  emit('toggle')
}

function rowTitle(row: OtherTopicRow): string {
  return `${row.label} · 所选日 ${row.threadCount} 条线索 · ${row.statusText} · 点下方读全文`
}
</script>

<template>
  <section class="bns-group" data-testid="narrow-other-group">
    <BoardNarrowGroupToggle
      title="当日其他话题"
      :count="count"
      count-testid="narrow-other-count"
      :note="note"
      note-testid="narrow-other-note"
      :expanded="open"
      :disabled="view?.state !== 'ready'"
      testid="narrow-other-toggle"
      @toggle="toggle"
    />
    <p v-if="view?.note" class="bns-group-extra" data-testid="narrow-other-extra">{{ view.note }}</p>
    <ol v-if="open && rows.length > 0" class="bns-other-list" data-testid="narrow-other-list">
      <li v-for="row in rows" :key="row.sectionId">
        <button
          type="button"
          class="bns-other-row"
          :class="{ 'is-selected': row.sectionId === selectedOtherSectionId }"
          :data-testid="`narrow-other-row-${row.sectionId}`"
          :aria-pressed="row.sectionId === selectedOtherSectionId"
          :title="rowTitle(row)"
          :aria-label="rowTitle(row)"
          @click="emit('select', row)"
        >
          <span class="bns-other-name">{{ row.label }}</span>
          <span class="bns-other-count">{{ row.threadCount }}</span>
        </button>
      </li>
    </ol>
  </section>
</template>

<style scoped>
.bns-group {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  border-top: 1px solid var(--color-border-subtle, var(--color-border-medium));
  padding-top: 0.35rem;
  min-width: 0;
}

.bns-group-extra {
  margin: 0;
  padding: 0 0.1rem;
  color: var(--color-text-muted);
  font-size: 0.64rem;
  line-height: 1.5;
}

.bns-other-list {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
  margin: 0.1rem 0 0.3rem;
  padding: 0;
  list-style: none;
}

.bns-other-row {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  width: 100%;
  padding: 0.35rem 0.1rem;
  border: 0;
  border-bottom: 1px solid var(--color-border-subtle, var(--color-border-medium));
  background: none;
  color: var(--color-text-secondary);
  text-align: left;
  cursor: pointer;
}

.bns-other-row.is-selected {
  color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 6%, transparent);
}

.bns-other-name {
  flex: 1;
  min-width: 0;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  overflow: hidden;
  font-size: 0.76rem;
  line-height: 1.5;
  overflow-wrap: break-word;
}

.bns-other-count {
  flex: 0 0 auto;
  color: var(--color-text-muted);
  font-size: 0.68rem;
  font-variant-numeric: tabular-nums;
}
</style>
