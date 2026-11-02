<script setup lang="ts">
/**
 * 雷达总览常温区（rework-workshop-into-radar-and-desk 8A/R-18 拆分）。
 * 常温（关注度 < 15）默认紧凑索引（label + 截止日期 + 日报入口），可展开为
 * 与关注行相同的完整横比（BoardRadarStory）。展开状态本组件自持；分档判定
 * 在 useBoardRadarOverview，这里不改口径。
 */
import { ref } from 'vue'
import { Icon } from '@iconify/vue'
import BoardRadarStory from './BoardRadarStory.vue'
import { isDelayed, shortDate } from './boardRadarHelpers'
import type { RadarBoard } from '~/api/semanticBoards'

const props = defineProps<{
  coldRows: RadarBoard[]
  /** 展开行的起始序号（延续关注行编号）。 */
  rankOffset: number
  globalDate?: string
}>()

const emit = defineEmits<{
  (e: 'openBoard', boardId: string): void
  (e: 'openReport', row: RadarBoard): void
  (e: 'openLane', payload: { boardId: string, topicId: string }): void
}>()

const coldOpen = ref(false)

function rowDelayed(row: RadarBoard): boolean {
  return isDelayed(row, props.globalDate)
}
</script>

<template>
  <div class="rd-cold">
    <button type="button" class="rd-cold-toggle" :aria-expanded="coldOpen" @click="coldOpen = !coldOpen">
      <Icon :icon="coldOpen ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="14" />
      <span>常温 · {{ coldRows.length }} 个版块</span>
      <small>{{ coldOpen ? '收起横比' : '展开完整横比' }}</small>
    </button>
    <div v-if="coldOpen" class="rd-cold-list">
      <BoardRadarStory
        v-for="(row, index) in coldRows"
        :key="row.boardId"
        :row="row"
        :rank="rankOffset + index + 1"
        :global-date="globalDate"
        @open-board="boardId => emit('openBoard', boardId)"
        @open-report="row => emit('openReport', row)"
        @open-lane="payload => emit('openLane', payload)"
      />
    </div>
    <div v-else class="rd-cold-digest">
      <div
        v-for="row in coldRows"
        :key="row.boardId"
        class="rd-cold-row"
        @click="emit('openBoard', row.boardId)"
      >
        <button type="button" class="rd-cold-name" @click.stop="emit('openBoard', row.boardId)">
          {{ row.label }}
        </button>
        <small>
          <span v-if="rowDelayed(row)">{{ shortDate(row.anchorDate || globalDate) }} 截止 · </span>
          <button
            v-if="row.reportDate"
            type="button"
            class="rd-cold-report"
            @click.stop="emit('openReport', row)"
          >
            {{ shortDate(row.reportDate) }} 日报 ↗
          </button>
          <span v-else>近 14 天暂无日报</span>
        </small>
      </div>
    </div>
  </div>
</template>

<style scoped>
.rd-cold {
  margin-top: 0.4rem;
}

.rd-cold-toggle {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  width: 100%;
  min-width: 0;
  margin-top: 0.9rem;
  padding: 0.75rem 0.2rem;
  border: 0;
  border-top: 1px solid var(--color-border-medium);
  border-bottom: 1px solid var(--color-border-medium);
  background: none;
  color: var(--color-text-primary);
  font-family: var(--font-serif, Georgia, serif);
  font-size: 0.95rem;
  text-align: left;
  cursor: pointer;
}

.rd-cold-toggle span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rd-cold-toggle small {
  margin-left: auto;
  flex: 0 0 auto;
  color: var(--color-text-muted);
  font-size: 0.64rem;
}

.rd-cold-toggle:hover {
  color: var(--color-accent);
}

.rd-cold-digest {
  display: grid;
  grid-template-columns: 1fr 1fr;
  column-gap: 2rem;
}

.rd-cold-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  min-width: 0;
  padding: 0.6rem 0.1rem;
  border-bottom: 1px solid var(--color-border-subtle);
  cursor: pointer;
}

.rd-cold-row:hover {
  background: var(--color-bg-hover);
}

.rd-cold-name {
  min-width: 0;
  border: 0;
  padding: 0;
  background: none;
  color: var(--color-text-primary);
  font-size: 0.8rem;
  text-align: left;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
}

.rd-cold-row small {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  flex: 0 0 auto;
  color: var(--color-text-muted);
  font-size: 0.64rem;
  white-space: nowrap;
}

.rd-cold-report {
  border: 0;
  padding: 0;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.64rem;
  cursor: pointer;
}

.rd-cold-report:hover,
.rd-cold-report:focus-visible {
  color: var(--color-accent);
  text-decoration: underline;
  outline: none;
}

@media (max-width: 767.98px) {
  .rd-cold-digest {
    display: block;
  }
}
</style>
