<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { formatMagazineDate, type QualityZone, type TopicGroup } from './dailyReportMagazine'
import type { DailyReportListItem } from '~/api/dailyReports'

/** 版块切换所需的最小版块信息（结构兼容 SemanticBoard）。 */
interface BoardOption {
  id: number
  label: string
}

defineProps<{
  zones: QualityZone[]
  activeTopics: TopicGroup[]
  reports: DailyReportListItem[]
  currentIndex: number
  boards?: BoardOption[]
  boardId?: number
  /** 今天已经有日报的版块（阅读台左队列，spec 6.1）。 */
  todayBoards?: { id: number, label: string, date: string }[]
  /** 阅读台：历史列表只载入近 N 天，允许继续向前取（B-5）。 */
  canLoadMore?: boolean
}>()

const emit = defineEmits<{
  scrollTo: [target: string]
  selectReport: [index: number]
  loadMore: []
  selectBoard: [boardId: number]
  selectToday: [payload: { boardId: number, date: string }]
}>()
</script>

<template>
  <aside class="drm-sidebar" aria-label="日报导航">
    <section class="drm-sidebar__section">
      <h2>本期目录</h2>
      <button type="button" @click="emit('scrollTo', 'report-lead')">头条</button>
      <button v-if="zones.length" type="button" @click="emit('scrollTo', `report-zone-${zones[0]?.key}`)">话题正文</button>
    </section>

    <section v-if="todayBoards?.length" class="drm-sidebar__section">
      <h2>今天有日报</h2>
      <button
        v-for="board in todayBoards"
        :key="`today-${board.id}`"
        type="button"
        :class="{ 'is-active': board.id === boardId }"
        @click="emit('selectToday', { boardId: board.id, date: board.date })"
      >
        <span>{{ board.label }}</span>
        <Icon icon="mdi:newspaper-variant-outline" width="13" />
      </button>
    </section>

    <section v-if="activeTopics.length" class="drm-sidebar__section">
      <h2>持续追踪</h2>
      <button
        v-for="topic in activeTopics"
        :key="topic.key"
        type="button"
        class="drm-sidebar__topic"
        @click="emit('scrollTo', `report-topic-${topic.topicId}`)"
      >
        <span class="drm-sidebar__swatch" :style="{ backgroundColor: topic.color || 'var(--color-accent)' }" />
        <span>{{ topic.canonicalLabel || topic.label }}</span>
      </button>
    </section>

    <section class="drm-sidebar__section drm-sidebar__archive">
      <h2>历史日报</h2>
      <button
        v-for="(report, index) in reports"
        :key="report.id"
        type="button"
        :class="{ 'is-active': index === currentIndex }"
        @click="emit('selectReport', index)"
      >
        <span>{{ formatMagazineDate(report.period_date) }}</span>
        <Icon v-if="index === currentIndex" icon="mdi:circle-small" width="18" />
      </button>
      <button
        v-if="canLoadMore"
        type="button"
        class="drm-sidebar__more"
        data-testid="desk-load-more"
        @click="emit('loadMore')"
      >
        加载更早
      </button>
    </section>

    <section v-if="boards && boards.length > 1" class="drm-sidebar__section drm-sidebar__boards">
      <h2>版块切换</h2>
      <button
        v-for="board in boards"
        :key="board.id"
        type="button"
        :class="{ 'is-active': board.id === boardId }"
        @click="emit('selectBoard', board.id)"
      >
        <span>{{ board.label }}</span>
      </button>
    </section>
  </aside>
</template>

<style scoped>
.drm-sidebar {
  position: sticky;
  top: 5.5rem;
  display: flex;
  flex-direction: column;
  gap: 1.75rem;
  align-self: start;
  max-height: calc(100vh - 7rem);
  overflow-y: auto;
  padding-right: 1rem;
  color: var(--color-text-secondary);
  scrollbar-width: thin;
}

.drm-sidebar__section {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  padding-top: 0.8rem;
  border-top: 1px solid var(--color-border-strong);
}

.drm-sidebar h2 {
  margin: 0 0 0.5rem;
  color: var(--color-accent);
  font-family: "Noto Serif SC", serif;
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.14em;
}

.drm-sidebar button {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  width: 100%;
  padding: 0.45rem 0;
  border: 0;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 0.75rem;
  line-height: 1.45;
  text-align: left;
  cursor: pointer;
}

.drm-sidebar button:hover,
.drm-sidebar button:focus-visible,
.drm-sidebar button.is-active {
  color: var(--color-text-primary);
}

.drm-sidebar button:focus-visible {
  outline: 2px solid var(--color-input-focus);
  outline-offset: 2px;
}

.drm-sidebar__topic span:last-child {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.drm-sidebar__swatch {
  flex: 0 0 auto;
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 2px;
}

.drm-sidebar__archive button {
  justify-content: space-between;
  margin-bottom: 0.2rem;
  padding: 0.55rem 0.75rem;
  border: 0;
  border-left: 2px solid transparent;
  background: var(--color-bg-active);
  font-family: "Noto Serif SC", serif;
  font-size: 0.875rem;
  font-weight: 500;
  transition: border-color 0.2s ease, background 0.2s ease, color 0.2s ease;
}

.drm-sidebar__archive button:hover,
.drm-sidebar__archive button:focus-visible {
  border-left-color: var(--color-border-strong);
  background: var(--color-bg-active);
  color: var(--color-text-primary);
}

.drm-sidebar__archive button.is-active {
  border-left-color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 10%, transparent);
  color: var(--color-accent);
  font-weight: 700;
}

.drm-sidebar__boards button {
  margin-bottom: 0.2rem;
  padding: 0.5rem 0.75rem;
  border: 0;
  border-left: 2px solid transparent;
  background: var(--color-bg-active);
  font-family: "Noto Serif SC", serif;
  font-size: 0.8rem;
  font-weight: 500;
  transition: border-color 0.2s ease, background 0.2s ease, color 0.2s ease;
}

.drm-sidebar__boards button:hover,
.drm-sidebar__boards button:focus-visible {
  border-left-color: var(--color-border-strong);
  background: var(--color-bg-active);
  color: var(--color-text-primary);
}

.drm-sidebar__boards button.is-active {
  border-left-color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 10%, transparent);
  color: var(--color-accent);
  font-weight: 700;
}

@media (max-width: 1100px) {
  .drm-sidebar {
    position: static;
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    max-height: none;
    padding: 0 0 1rem;
    overflow: visible;
  }
}

@media (max-width: 720px) {
  .drm-sidebar {
    grid-template-columns: 1fr;
    gap: 0.75rem;
  }

  .drm-sidebar__section:nth-child(2) {
    display: none;
  }
}
</style>
