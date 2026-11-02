<script setup lang="ts">
/**
 * 雷达总览阅读右栏（rework-workshop-into-radar-and-desk 8A/R-18 拆分）。
 * 纯展示：「今日该读」（真实日报标题/日期/文章事件元信息）与
 * 「今天可能升温」（N.N× 倍数 + 理由句 + 阈值脚注；无升温给明确文案）。
 * 升温判定与排序在 useBoardRadarOverview，本组件不改口径。
 */
import { shortDate, tierLabel } from './boardRadarHelpers'
import type { RadarBoard } from '~/api/semanticBoards'
import type { RadarWarmingItem } from '~/features/tags/composables/useBoardRadarOverview'

defineProps<{
  todayRead: RadarBoard[]
  warming: RadarWarmingItem[]
}>()

const emit = defineEmits<{
  (e: 'openReport', row: RadarBoard): void
  (e: 'openBoard', boardId: string): void
}>()
</script>

<template>
  <aside class="rd-rail" aria-label="今日阅读与升温">
    <section class="rd-rail-block">
      <div class="rd-rule-title">
        <h2>今日该读</h2>
        <span class="rd-rule-tag">READING LIST</span>
      </div>
      <button
        v-for="row in todayRead"
        :key="`read-${row.boardId}`"
        type="button"
        class="rd-read"
        :data-testid="`radar-read-${row.boardId}`"
        @click="emit('openReport', row)"
      >
        <span class="rd-read-board">{{ row.label }}</span>
        <span class="rd-read-title">{{ row.reportTitle }}</span>
        <small class="rd-read-meta">
          {{ shortDate(row.reportDate) }} · {{ row.reportArticleCount }} 篇文章 · {{ row.reportEventCount }} 个事件
        </small>
      </button>
      <p v-if="!todayRead.length" class="rd-rail-empty">今天还没有日报。</p>
    </section>

    <section class="rd-rail-block rd-warm-block">
      <h2>今天可能升温</h2>
      <template v-for="item in warming" :key="`warm-${item.row.boardId}`">
        <button type="button" class="rd-warm" @click="emit('openBoard', item.row.boardId)">
          <span>{{ item.row.label }}</span>
          <b>{{ item.ratio.toFixed(1) }}×</b>
        </button>
        <p class="rd-warm-why">
          {{ item.row.todaySections }} 个当日分区 / 近 3 日均值 {{ (item.row.last3Sections / 3).toFixed(1) }}。仍属{{ tierLabel(item.row.attention) }}档，不是热点判定。
        </p>
      </template>
      <p v-if="!warming.length" class="rd-rail-empty" data-testid="radar-no-warm">
        暂未触发升温条件。
      </p>
      <p class="rd-warm-note">当日分区 ≥3，且达到近 3 日均值的 2 倍。</p>
    </section>
  </aside>
</template>

<style scoped>
.rd-rail {
  min-height: 0;
  overflow-y: auto;
  padding-left: 1.5rem;
  border-left: 1px solid var(--color-border-medium);
  scrollbar-width: thin;
}

.rd-rail-block {
  padding: 0.2rem 0 1rem;
}

.rd-rule-title {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.6rem 0 0.55rem;
  border-top: 2px solid var(--color-text-primary);
}

.rd-rail h2 {
  margin: 0;
  color: var(--color-text-primary);
  font-family: var(--font-serif, Georgia, serif);
  font-size: 1rem;
  font-weight: 650;
  letter-spacing: 0.02em;
}

.rd-rule-tag {
  color: var(--color-text-muted);
  font-size: 0.58rem;
  letter-spacing: 0.16em;
}

.rd-read {
  display: flex;
  flex-direction: column;
  gap: 0.18rem;
  width: 100%;
  padding: 0.6rem 0;
  border: 0;
  border-bottom: 1px solid var(--color-border-subtle);
  background: none;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.rd-read:hover .rd-read-title {
  color: var(--color-accent);
}

.rd-read-board {
  color: var(--color-accent);
  font-size: 0.62rem;
  letter-spacing: 0.04em;
}

.rd-read-title {
  color: var(--color-text-primary);
  font-family: var(--font-serif, Georgia, serif);
  font-size: 0.86rem;
  line-height: 1.5;
}

.rd-read-meta {
  color: var(--color-text-muted);
  font-size: 0.62rem;
}

.rd-rail-empty {
  margin: 0.35rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.72rem;
}

.rd-warm-block {
  margin-top: 0.8rem;
  padding: 0.9rem;
  border: 1px solid color-mix(in srgb, var(--color-accent) 20%, transparent);
  border-top: 2px solid var(--color-accent);
  border-radius: 6px;
  background: color-mix(in srgb, var(--color-accent) 6%, transparent);
}

.rd-warm-block h2 {
  margin: 0 0 0.6rem;
  color: var(--color-accent);
  font-size: 0.95rem;
}

.rd-warm {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.6rem;
  width: 100%;
  padding: 0.28rem 0;
  border: 0;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.75rem;
  text-align: left;
  cursor: pointer;
}

.rd-warm:hover span {
  color: var(--color-accent);
}

.rd-warm b {
  color: var(--color-accent);
  font-family: var(--font-mono, monospace);
  font-size: 0.95rem;
  font-weight: 500;
  white-space: nowrap;
}

.rd-warm-why {
  margin: 0 0 0.45rem;
  color: var(--color-text-muted);
  font-size: 0.64rem;
  line-height: 1.6;
}

.rd-warm-note {
  margin: 0.5rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.62rem;
}

@media (min-width: 768px) and (max-width: 1439.98px) {
  .rd-rail {
    overflow: visible;
    margin-top: 1.4rem;
    padding-left: 0;
    border-left: 0;
    border-top: 1px solid var(--color-border-medium);
  }
}

@media (max-width: 767.98px) {
  .rd-rail {
    overflow: visible;
    padding: 0.8rem 0 0;
    border-left: 0;
    border-top: 1px solid var(--color-border-medium);
  }

  .rd-rail-block {
    padding-bottom: 0.6rem;
  }
}
</style>
