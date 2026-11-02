<script setup lang="ts">
/**
 * 分区索引（日期下拉 + 分区锦点 + scroll-spy）。
 * ⚠️ 2026-10-27 用户复审后**未挂载**：日报阅读页已恢复旧版三栏（大刊头 / 左目录 /
 * 宽屏正文 / 右页边注），本组件与 scroll-spy 一并撤下；保留文件以备后续重议。
 */
import { Icon } from '@iconify/vue'
import { formatMagazineDate } from './dailyReportMagazine'
import type { DailyReportListItem } from '~/api/dailyReports'
import type { DeskZoneGroup } from './deskTypes'

withDefaults(defineProps<{
  groups: DeskZoneGroup[]
  activeAnchor?: string
  /** 1000–1200px 收起左索引栏时，用正文顶部的横向锚点条。 */
  compact?: boolean
  /** 阅读台：历史日期从最左栏并进索引顶部。 */
  reports?: DailyReportListItem[]
  currentIndex?: number
  canLoadMore?: boolean
  boards?: Array<{ id: number, label: string }>
  boardId?: number
  /** 有 watch 命中的日报日期，用于跨期追踪提示。 */
  watchDates?: string[]
}>(), {
  activeAnchor: '',
  compact: false,
  reports: () => [],
  currentIndex: -1,
  canLoadMore: false,
  boards: () => [],
  boardId: undefined,
  watchDates: () => [],
})

const emit = defineEmits<{
  scroll: [target: string]
  openLane: [topicId: number]
  selectReport: [index: number]
  loadMore: []
  selectBoard: [boardId: number]
}>()
</script>

<template>
  <nav
    class="ddi"
    :class="{ 'is-compact': compact }"
    aria-label="日报分区索引"
    data-testid="desk-index"
  >
    <div v-if="reports.length" class="ddi-date">
      <label>
        <span class="sr-only">选择日报日期</span>
        <select
          :value="currentIndex"
          data-testid="desk-date-select"
          @change="emit('selectReport', Number(($event.target as HTMLSelectElement).value))"
        >
          <option v-for="(report, index) in reports" :key="report.id" :value="index">
            {{ formatMagazineDate(report.period_date) }}{{ watchDates.includes(report.period_date.slice(0, 10)) ? ' · 追踪命中' : '' }}
          </option>
        </select>
      </label>
      <button v-if="canLoadMore" type="button" class="ddi-more" data-testid="desk-load-more" @click="emit('loadMore')">
        加载更早
      </button>
    </div>
    <div v-if="boards && boards.length > 1" class="ddi-board">
      <label>
        <span class="sr-only">切换版块</span>
        <select
          :value="boardId"
          data-testid="desk-board-select"
          @change="emit('selectBoard', Number(($event.target as HTMLSelectElement).value))"
        >
          <option v-for="board in boards" :key="board.id" :value="board.id">{{ board.label }}</option>
        </select>
      </label>
    </div>
    <p class="ddi-kicker">本期分区</p>
    <div class="ddi-items">
      <button type="button" class="ddi-item" :class="{ 'is-on': activeAnchor === 'report-lead' }" @click="emit('scroll', 'report-lead')">
        头条
      </button>
      <template v-for="group in groups" :key="group.key">
        <button
          type="button"
          class="ddi-item is-zone"
          :class="{ 'is-on': activeAnchor === `report-zone-${group.key}` }"
          @click="emit('scroll', `report-zone-${group.key}`)"
        >
          {{ group.label }}
        </button>
        <div
          v-for="topic in group.topics"
          :key="topic.key"
          class="ddi-topic-row"
        >
          <button
            type="button"
            class="ddi-item is-topic"
            :class="{ 'is-on': activeAnchor === `report-topic-${topic.topicId}` }"
            @click="emit('scroll', `report-topic-${topic.topicId}`)"
          >
            <span class="ddi-dot" :style="{ backgroundColor: topic.color || 'var(--color-accent)' }" />
            <span class="ddi-item-label" :title="topic.canonicalLabel || topic.label">{{ topic.canonicalLabel || topic.label }}</span>
          </button>
          <button
            v-if="topic.topicId != null"
            type="button"
            class="ddi-matrix"
            :aria-label="`在矩阵中查看 ${topic.canonicalLabel || topic.label}`"
            title="在矩阵中查看这条线"
            @click="emit('openLane', topic.topicId)"
          >
            <Icon icon="mdi:vector-square" width="12" />
          </button>
        </div>
      </template>
    </div>
  </nav>
</template>

<style scoped>
.ddi {
  min-width: 0;
  position: sticky;
  top: 4.25rem;
  align-self: start;
  max-height: calc(100vh - 5rem);
  overflow-y: auto;
  padding: 0.25rem 0.15rem;
  scrollbar-width: thin;
}

.ddi-date {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  margin-bottom: 0.7rem;
}

.ddi-date select {
  width: 100%;
  padding: 0.32rem 0.4rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  font: inherit;
  font-size: 0.72rem;
  color-scheme: light dark;
}

.ddi-more {
  align-self: flex-start;
  border: 0;
  background: none;
  color: var(--color-accent);
  font: inherit;
  font-size: 0.68rem;
  text-decoration: underline;
  cursor: pointer;
}

.ddi.is-compact .ddi-date {
  flex-direction: row;
  align-items: center;
  margin-bottom: 0.4rem;
}

.ddi.is-compact .ddi-date label {
  flex: 1;
  min-width: 0;
}

.ddi-board {
  margin-bottom: 0.5rem;
}

.ddi-board select {
  width: 100%;
  padding: 0.28rem 0.35rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-secondary);
  font: inherit;
  font-size: 0.7rem;
}

.ddi-kicker {
  margin: 0 0 0.5rem;
  color: var(--color-text-muted);
  font-size: 0.62rem;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.ddi-items {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 0.1rem;
  width: 100%;
}

.ddi-item {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  width: 100%;
  padding: 0.35rem 0.4rem;
  border: 0;
  border-left: 2px solid transparent;
  border-radius: 0 5px 5px 0;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  line-height: 1.35;
  text-align: left;
  cursor: pointer;
}

.ddi-item:hover,
.ddi-item:focus-visible {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
  outline: none;
}

.ddi-item.is-on {
  border-left-color: var(--color-accent);
  background: var(--color-bg-elevated);
  color: var(--color-text-primary);
}

.ddi-item.is-zone {
  margin-top: 0.35rem;
  color: var(--color-text-muted);
  font-family: var(--font-serif);
  font-size: 0.7rem;
  letter-spacing: 0.04em;
}

.ddi-topic-row {
  display: flex;
  width: 100%;
  min-width: 0;
  align-items: center;
  gap: 0.1rem;
  padding-left: 0.4rem;
}

.ddi-topic-row .ddi-item {
  flex: 1;
  min-width: 0;
}

.ddi-item.is-topic {
  overflow: hidden;
  font-size: 0.7rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ddi-dot {
  flex: none;
  width: 0.36rem;
  height: 0.36rem;
  border-radius: 50%;
}

.ddi-matrix {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 1.35rem;
  height: 1.35rem;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
}

.ddi-matrix:hover,
.ddi-matrix:focus-visible {
  background: var(--color-bg-active);
  color: var(--color-accent);
  outline: none;
}

.ddi.is-compact {
  position: static;
  max-height: none;
  overflow: visible;
  padding: 0.4rem 0;
  border-bottom: 1px solid var(--color-border-medium);
}

.ddi.is-compact .ddi-date {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  margin-bottom: 0.7rem;
}

.ddi-date select {
  width: 100%;
  padding: 0.32rem 0.4rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  font: inherit;
  font-size: 0.72rem;
  color-scheme: light dark;
}

.ddi-more {
  align-self: flex-start;
  border: 0;
  background: none;
  color: var(--color-accent);
  font: inherit;
  font-size: 0.68rem;
  text-decoration: underline;
  cursor: pointer;
}

.ddi.is-compact .ddi-date {
  flex-direction: row;
  align-items: center;
  margin-bottom: 0.4rem;
}

.ddi.is-compact .ddi-date label {
  flex: 1;
  min-width: 0;
}

.ddi-board {
  margin-bottom: 0.5rem;
}

.ddi-board select {
  width: 100%;
  padding: 0.28rem 0.35rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-secondary);
  font: inherit;
  font-size: 0.7rem;
}

.ddi-kicker {
  display: none;
}

.ddi.is-compact .ddi-items {
  display: flex;
  align-items: center;
  gap: 0.2rem;
  overflow-x: auto;
}

.ddi.is-compact .ddi-item {
  width: auto;
  flex: none;
  border-left: 0;
  border-bottom: 2px solid transparent;
  border-radius: 4px 4px 0 0;
  white-space: nowrap;
}

.ddi.is-compact .ddi-item.is-on {
  border-bottom-color: var(--color-accent);
}

.ddi.is-compact .ddi-topic-row {
  display: contents;
  padding-left: 0;
}

.ddi.is-compact .ddi-matrix {
  display: none;
}

/* 手机没有右栏出口，紧凑索引保留矩阵入口 */
@media (max-width: 767.98px) {
  .ddi.is-compact .ddi-matrix {
    display: inline-flex;
  }
}
</style>
