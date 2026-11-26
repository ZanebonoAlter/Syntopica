<script setup lang="ts">
/**
 * 雷达总览阅读右栏（rework-workshop-into-radar-and-desk 8A/R-18 拆分；
 * retire-story-adopt-event-metrics 3.2 换「当天线索最多的泳道」清单）。
 *
 * 一块清单（≤ 8 条，服务端归并/排序/截断）：版块名 + 泳道名 + 当天线索数 + 该版块
 * 自己的锚点日。事件线清单、`新起`/`升温 N×` 行内徽标、跨天延续脚注与
 * 「今天可能升温」独立卡片整体退场（判据全部依赖事件线热度环比）。
 * 诚实标记保留：`partial`（部分版块延迟/降级 → 本次排序不完整，带降级版块数）与
 * 滞后版块数（各条目按自己的锚点日计）。
 *
 * 滞后弱化（2026-11-11 看版后修订，tasks 3.6）：锚点日 ≠ 全局锚点日的条目灰显并标
 * 「数据至 MM/DD」——旧数据不该与今天的数据平起平坐；沉底由服务端排序保证，本组件
 * 只负责视觉弱化与诚实日期，不改顺序、不重算线索数。
 * 旧后端响应（无 `top_lanes` 段）回退版块级日报标题列表；本组件不改口径。
 */
import { isLaggingAnchor, shortDate } from './boardRadarHelpers'
import type { RadarBoard, RadarTopLaneItem } from '~/api/semanticBoards'

const props = defineProps<{
  /** 泳道清单可用（新版后端带 top_lanes 段）；false = 旧响应回退版块级列表。 */
  laneMode: boolean
  laneItems: RadarTopLaneItem[]
  /** 全局锚点日（`top_lanes.anchor_date`）：条目锚点日与它不同即滞后 → 灰显。 */
  globalAnchorDate: string
  /** 部分版块降级 → 本次泳道排序不完整。 */
  partial: boolean
  /** 降级版块数（超时或泳道数据计算失败——提示里带上数量）。 */
  degradedCount: number
  /** 滞后版块数（各条目按自己的锚点日计——已知口径偏差如实呈现）。 */
  laggingCount: number
  /** 旧响应回退用：版块级日报标题列表。 */
  todayRead: RadarBoard[]
}>()

const emit = defineEmits<{
  (e: 'openLane', payload: { boardId: string, topicId: string }): void
  (e: 'openReport', row: RadarBoard): void
}>()

/** 条目是否滞后：锚点日 ≠ 全局锚点日（旧数据沉底 + 灰显 + 标「数据至」）。 */
function isLagging(item: RadarTopLaneItem): boolean {
  return isLaggingAnchor(item.anchorDate, props.globalAnchorDate)
}

/** 条目日期读法：滞后写「数据至」、今日写「截至」（分量不伪装成今天）。 */
function asOfText(item: RadarTopLaneItem): string {
  return `${isLagging(item) ? '数据至' : '截至'} ${shortDate(item.anchorDate)}`
}

/** 条目可读全名（title/aria）：版块名 · 泳道名 · N 条线索 · 日期。 */
function laneAria(item: RadarTopLaneItem): string {
  return [item.boardLabel, item.topicLabel, `${item.clueCount} 条线索`, asOfText(item)]
    .filter(Boolean)
    .join(' · ')
}

function openLane(item: RadarTopLaneItem): void {
  emit('openLane', { boardId: item.boardId, topicId: item.topicId })
}

/** 「今日该读」事件数口径（旧响应回退块用）：回落日报事件数；未知显示「—」。 */
function readEventText(row: RadarBoard): string {
  return `${row.reportEventCount ?? '—'} 个事件`
}
</script>

<template>
  <aside class="rd-rail scroll-thin" aria-label="今日阅读与线索">
    <section class="rd-rail-block">
      <div class="rd-rule-title">
        <h2>{{ laneMode ? '当天线索最多的泳道' : '今日该读' }}</h2>
        <span class="rd-rule-tag">{{ laneMode ? 'TOP LANES' : 'READING LIST' }}</span>
      </div>

      <!-- 部分版块降级（超时或泳道数据计算失败）：本次排序不完整。 -->
      <p v-if="laneMode && partial" class="rd-rail-note" data-testid="radar-partial">
        部分版块数据延迟或降级（{{ degradedCount }} 个版块），本次泳道排序不完整。
      </p>
      <!-- 滞后版块：各条目按自己的锚点日计（跨版块混排的已知口径偏差，如实呈现）。 -->
      <p v-if="laneMode && laggingCount > 0" class="rd-rail-note" data-testid="radar-lagging">
        {{ laggingCount }} 个版块数据滞后（各条目按自己的锚点日计）。
      </p>

      <template v-if="laneMode">
        <button
          v-for="item in laneItems"
          :key="`lane-${item.boardId}-${item.topicId}`"
          type="button"
          class="rd-read"
          :class="{ 'is-lagging': isLagging(item) }"
          :data-testid="`radar-lane-${item.boardId}-${item.topicId}`"
          :title="laneAria(item)"
          :aria-label="laneAria(item)"
          @click="openLane(item)"
        >
          <span class="rd-read-board">{{ item.boardLabel }}</span>
          <span class="rd-read-title">{{ item.topicLabel }}</span>
          <small class="rd-read-meta">
            {{ item.clueCount }} 条线索 · {{ asOfText(item) }}
          </small>
        </button>
        <p v-if="!laneItems.length" class="rd-rail-empty" data-testid="radar-no-lane">
          今天没有可排的泳道。
        </p>
        <p class="rd-warm-note" data-testid="radar-lane-caption">
          清单 = 跨版块按泳道当天线索数降序，最多 8 条；当天与全局锚点日一致的版块在前，滞后版块沉底并标注自己的日期。
        </p>
      </template>

      <template v-else>
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
            {{ shortDate(row.reportDate) }} · {{ row.reportArticleCount ?? '—' }} 篇文章 · {{ readEventText(row) }}
          </small>
        </button>
        <p v-if="!todayRead.length" class="rd-rail-empty">今天还没有日报。</p>
      </template>
    </section>
  </aside>
</template>

<style scoped>
.rd-rail {
  min-height: 0;
  overflow-y: auto;
  padding-left: 1.5rem;
  border-left: 1px solid var(--color-border-medium);
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

/* 滞后条目灰显（2026-11-11 看版后修订，tasks 3.6）：旧数据不与今日数据平起平坐，
 * 但仍如实可读（线索数与日期照常展示，悬浮恢复对比度以便点开）。 */
.rd-read.is-lagging {
  opacity: 0.58;
}

.rd-read.is-lagging:hover,
.rd-read.is-lagging:focus-visible {
  opacity: 1;
}

.rd-read.is-lagging .rd-read-board {
  color: var(--color-text-muted);
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
  overflow-wrap: anywhere;
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

.rd-rail-note {
  margin: 0.3rem 0 0.5rem;
  color: var(--color-text-muted);
  font-size: 0.66rem;
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
