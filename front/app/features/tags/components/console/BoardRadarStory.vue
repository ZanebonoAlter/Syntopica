<script setup lang="ts">
/**
 * 雷达总览故事行（rework-workshop-into-radar-and-desk 8A.1，已批准报刊式总览；
 * retire-story-adopt-event-metrics 3.2 换当天线索口径）。
 *
 * 一行完整横比：序号 / 板块名 + 最新日报（标题·日期·文章事件元信息）/ 逐泳道
 * 14 日**当日线索合计**小矩阵 / 关注强度分档（Σ 泳道当天热度）/ 近 3 日线索环比 / 进入。
 * 纯展示组件：输入为 API 边界归一后的 camelCase/字符串 ID 读模型（RadarBoard），
 * 分档、环比、延迟与小矩阵口径来自 boardRadarHelpers，跳转由父级承接
 * （进版块 / 进阅读台 / 进 ?lane= 控制台）。本行网格列宽与总览表头 `.rd-labels`
 * 保持一致（父组件列定义仅限定其自有 .rd-labels，不命中本组件根，避免覆盖移动端
 * 列定义，R-20），改断点须两处同步。
 *
 * 分档徽标滞后弱化（2026-11-11 看版后修订，tasks 3.6）：版本锚点日 ≠ 总览全局锚点日时
 * 不挂「热点 / 关注」分档徽标（阈值按今日分布标定，旧数据不可比），改为中性小标注
 * 「数据至 MM/DD」；关注度数值照旧如实显示、不被隐藏也不被打折。
 *
 * 小矩阵（D5）：格值 = 该泳道该日线索合计（`clue_counts`）；**颜色 = 该数在本泳道 14 天窗口内的
 * 归一深浅**（2026-11-11 用户看版后修订，tasks 3.5：行内归一只，0/未知不着色，行内峰值最深）。
 * 颜色只是冗余强化通道，数值 + title/aria 仍是唯一信息承载；旧后端缺 `clue_counts` 的格子标
 * 「线索数未知」，SHALL NOT 造 0，也不拿分区数/事件数冒充线索数。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import { RADAR_HOT_ATTENTION, isDelayed, isLaggingAnchor, miniAxis, shortDate, tierLabel, trendOf } from './boardRadarHelpers'
import { clueDepth, clueMix, clueRowMax } from './observationLanes'
import type { RadarBoard, RadarLane } from '~/api/semanticBoards'

const props = defineProps<{
  row: RadarBoard
  rank: number
  /** 总览全局日期；板块锚日落后于它时本行必须标注「数据延迟」。 */
  globalDate?: string
}>()

const emit = defineEmits<{
  (e: 'openBoard', boardId: string): void
  (e: 'openReport', row: RadarBoard): void
  (e: 'openLane', payload: { boardId: string, topicId: string }): void
}>()

const tier = computed(() => tierLabel(props.row.attention))
const trend = computed(() => trendOf(props.row))
const delayed = computed(() => isDelayed(props.row, props.globalDate))
/** 滞后 = 本行锚点日 ≠ 总览全局锚点日（分档徽标不挂，改「数据至」）。 */
const lagging = computed(() => isLaggingAnchor(props.row.anchorDate, props.globalDate))
const todayText = computed(() => (props.row.todayClues === null ? '当日线索未知' : `当日线索 ${props.row.todayClues}`))
const anchorText = computed(() => {
  const asOf = delayed.value ? shortDate(props.row.anchorDate) : '今日'
  return `${asOf} · ${todayText.value}`
})
const reportFallback = computed(() => (props.row.reportDate ? '查看已有日报' : '今天暂无日报'))
/** 小矩阵格子 title 的日期轴（无全局日期时退回板块自身锚日）。 */
const miniDates = computed(() => miniAxis(props.globalDate || props.row.anchorDate || '', 14))

/** 单格线索数；null = 该日未知（旧后端缺 clue_counts），展示为「线索数未知」。 */
interface LaneCell { count: number | null }

/** 每行（泳道）的 14 天格子 + 行内归一分母（色深只看本行，跨行不归一）。 */
const miniRows = computed(() => props.row.lanes.map((lane) => {
  const cells: LaneCell[] = lane.clueCounts
    ? lane.clueCounts.map(count => ({ count: typeof count === 'number' && Number.isFinite(count) ? count : null }))
    : Array.from({ length: 14 }, () => ({ count: null }))
  return { lane, cells, rowMax: clueRowMax(cells.map(cell => cell.count)) }
}))

/** 格子可读文本：已知写「N 条线索」，未知写「线索数未知」——不造 0、不拿分区数冒充。 */
function cellTitle(cell: LaneCell, dayIndex: number): string {
  const date = miniDates.value[dayIndex] || '日期未知'
  return `${date} · ${cell.count === null ? '线索数未知' : `${cell.count} 条线索`}`
}

/** 已知线索格有值时按行内归一比例着色（0/未知 = 底色，不构成信息）；颜色只是冗余强化通道。 */
function cellClass(cell: LaneCell, rowMax: number): string {
  return clueDepth(cell.count, rowMax) > 0 ? 'is-clue-depth' : ''
}

/** 色深经 CSS 变量交给主题色阶（`--color-accent` mix `--color-bg-hover`），不新增第二份色板。 */
function cellStyle(cell: LaneCell, rowMax: number): Record<string, string> {
  const mix = clueMix(clueDepth(cell.count, rowMax))
  return mix === '' ? {} : { '--rd-clue-mix': mix }
}

function laneTitle(lane: RadarLane): string {
  return `${lane.label} · 全窗 ${lane.threadCount14d} 条线索 · 进入泳道`
}

function openLane(lane: RadarLane): void {
  emit('openLane', { boardId: props.row.boardId, topicId: lane.topicId })
}
</script>

<template>
  <article
    class="rd-story rd-grid"
    :class="{ 'is-hot': row.attention >= RADAR_HOT_ATTENTION, 'is-warm': row.attention < RADAR_HOT_ATTENTION }"
    :data-testid="`radar-row-${row.boardId}`"
    @click="emit('openBoard', row.boardId)"
  >
    <span class="rd-rank">{{ String(rank).padStart(2, '0') }}</span>

    <div class="rd-story-main">
      <button type="button" class="rd-story-title" :title="row.label" @click.stop="emit('openBoard', row.boardId)">
        {{ row.label }}
      </button>
      <button
        type="button"
        class="rd-story-report"
        :title="row.reportTitle || reportFallback"
        @click.stop="emit('openReport', row)"
      >
        {{ row.reportTitle || reportFallback }}
      </button>
      <p class="rd-story-meta">
        <span>{{ row.reportDate ? shortDate(row.reportDate) : '无日报日期' }}</span>
        <span>{{ row.reportArticleCount }} 篇文章 · {{ row.reportEventCount }} 个事件</span>
        <span v-if="delayed" class="rd-delay">数据延迟 · 截至 {{ shortDate(row.anchorDate) }}</span>
      </p>
    </div>

    <div class="rd-mini rd-grid-mini">
      <div v-for="entry in miniRows" :key="entry.lane.topicId" class="rd-mini-lane">
        <button
          type="button"
          class="rd-mini-name"
          :title="laneTitle(entry.lane)"
          @click.stop="openLane(entry.lane)"
        >
          {{ entry.lane.label }}
        </button>
        <div class="rd-mini-cells">
          <button
            v-for="(cell, dayIndex) in entry.cells"
            :key="dayIndex"
            type="button"
            class="rd-mini-cell"
            :class="cellClass(cell, entry.rowMax)"
            :style="cellStyle(cell, entry.rowMax)"
            :data-known="cell.count === null ? 'false' : 'true'"
            :title="cellTitle(cell, dayIndex)"
            :aria-label="`${entry.lane.label} · ${cellTitle(cell, dayIndex)}，进入泳道`"
            @click.stop="openLane(entry.lane)"
          />
        </div>
      </div>
      <small v-if="!row.lanes.length" class="rd-mini-empty">无活跃泳道 · 已有日报仍可读</small>
    </div>

    <div class="rd-attention-wrap">
      <span class="rd-attention">{{ row.attention.toFixed(1) }}</span>
      <!-- 滞后行不挂分档徽标（阈值按今日分布标定，旧数据不可比），改中性「数据至」。 -->
      <small v-if="lagging" class="rd-tier rd-tier-stale" data-testid="radar-lagging-tier">
        数据至 {{ shortDate(row.anchorDate) }}
      </small>
      <small v-else class="rd-tier">关注强度 · {{ tier }}</small>
    </div>

    <div class="rd-trend">
      <b class="rd-trend-value" :class="`is-${trend.dir}`">{{ trend.text }}</b>
      <small class="rd-trend-base">
        近 3 日 {{ row.last3Clues ?? '—' }} / 前 3 日 {{ row.prev3Clues ?? '—' }}<br>
        {{ anchorText }}
      </small>
    </div>

    <button type="button" class="rd-enter" @click.stop="emit('openBoard', row.boardId)">
      矩阵 <Icon icon="mdi:arrow-top-right" width="13" />
    </button>
  </article>
</template>

<style scoped>
/* 与总览表头 `.rd-labels` 列宽同步（改断点两处一起改；父组件不定义 .rd-grid，
 * 避免其 scoped 样式命中本组件根覆盖移动端列，R-20）。 */
.rd-grid {
  display: grid;
  grid-template-columns: 2.2rem minmax(13rem, 1.25fr) minmax(14rem, 1.1fr) 3.8rem 6.6rem minmax(7rem, max-content);
  gap: clamp(0.9rem, 1.1vw, 1.5rem);
  align-items: center;
}

.rd-story {
  width: 100%;
  padding: clamp(0.9rem, 2vh, 1.5rem) 0.2rem;
  border-bottom: 1px solid var(--color-border-subtle, var(--color-border-medium));
  border-left: 2px solid transparent;
  cursor: pointer;
}

.rd-story:hover,
.rd-story:focus-within {
  background: var(--color-bg-hover);
}

.rd-story.is-hot {
  border-left-color: var(--color-accent);
}

.rd-story.is-warm {
  border-left-color: color-mix(in srgb, var(--color-accent) 35%, transparent);
}

.rd-rank {
  color: var(--color-text-muted);
  font-family: var(--font-mono, monospace);
  font-size: 0.72rem;
  text-align: right;
}

.rd-story.is-hot .rd-rank {
  color: var(--color-accent);
}

.rd-story-main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.rd-story-title {
  min-width: 0;
  border: 0;
  padding: 0;
  background: none;
  color: var(--color-text-primary);
  font-family: var(--font-serif, Georgia, serif);
  font-size: clamp(1.05rem, 1.15vw, 1.5rem);
  font-weight: 650;
  line-height: 1.5;
  text-align: left;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
}

.rd-story-title:hover,
.rd-story-title:focus-visible {
  color: var(--color-accent);
  outline: none;
}

.rd-story-report {
  border: 0;
  padding: 0;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.76rem;
  line-height: 1.6;
  text-align: left;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
}

.rd-story-report:hover,
.rd-story-report:focus-visible {
  color: var(--color-accent);
  text-decoration: underline;
  outline: none;
}

.rd-story-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.3rem 0.75rem;
  margin: 0;
  color: var(--color-text-muted);
  font-size: 0.66rem;
}

.rd-delay {
  color: var(--color-accent);
}

.rd-mini {
  display: flex;
  flex-direction: column;
  gap: 0.22rem;
  min-width: 0;
}

.rd-mini-lane {
  display: grid;
  grid-template-columns: minmax(4.2rem, 0.55fr) minmax(7rem, 1fr);
  gap: 0.5rem;
  align-items: center;
}

.rd-mini-name {
  border: 0;
  padding: 0;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.62rem;
  text-align: left;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
}

.rd-mini-name:hover,
.rd-mini-name:focus-visible {
  color: var(--color-accent);
  outline: none;
}

.rd-mini-cells {
  display: grid;
  grid-template-columns: repeat(14, minmax(0, 1fr));
  gap: 3px;
  height: 0.6rem;
}

/* 色深（2026-11-11 用户看版后修订，tasks 3.5）：行内归一的连续深浅（0/未知 = 底色，
 * 本行峰值 = 66% accent），跨行不归一；颜色只是冗余强化通道，数字仍在 title/aria。 */
.rd-mini-cell {
  min-width: 0;
  padding: 0;
  border: 0;
  border-radius: 1px;
  background: var(--color-bg-hover);
  cursor: pointer;
}

.rd-mini-cell.is-clue-depth {
  background: color-mix(in srgb, var(--color-accent) var(--rd-clue-mix, 0%), var(--color-bg-hover));
}

.rd-mini-empty {
  color: var(--color-text-muted);
  font-size: 0.64rem;
}

.rd-attention-wrap {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  text-align: center;
}

.rd-attention {
  color: var(--color-accent);
  font-family: var(--font-mono, monospace);
  font-size: clamp(1.3rem, 1.8vw, 1.9rem);
  font-weight: 500;
  line-height: 1.2;
}

.rd-tier {
  color: var(--color-text-muted);
  font-size: 0.58rem;
  white-space: nowrap;
}

.rd-story.is-hot .rd-tier {
  color: var(--color-accent);
}

/* 滞后行分档徽标退场后的中性标注（不分档、不抢跟；数值仍在左侧如实可见）。 */
.rd-story .rd-tier-stale,
.rd-story.is-hot .rd-tier-stale {
  color: var(--color-text-muted);
  white-space: nowrap;
}

.rd-trend {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  min-width: 0;
}

.rd-trend-value {
  font-family: var(--font-mono, monospace);
  font-size: 0.9rem;
  font-weight: 500;
}

.rd-trend-value.is-up { color: var(--color-accent); }
.rd-trend-value.is-down { color: var(--color-success); }
.rd-trend-value.is-flat { color: var(--color-text-muted); }

.rd-trend-base {
  color: var(--color-text-muted);
  font-size: 0.6rem;
  line-height: 1.6;
  white-space: nowrap;
}

.rd-enter {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.2rem;
  min-width: 7rem;
  padding: 0.35rem 0.6rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 0.7rem;
  white-space: nowrap;
  cursor: pointer;
}

.rd-story:hover .rd-enter,
.rd-enter:focus-visible {
  border-color: var(--color-accent);
  color: var(--color-accent);
  outline: none;
}

@media (min-width: 768px) and (max-width: 1439.98px) {
  .rd-grid {
    grid-template-columns: 1.4rem minmax(0, 1fr) 3.4rem 5.4rem minmax(7rem, max-content);
    gap: 0.7rem;
  }

  .rd-grid-mini {
    display: none;
  }
}

@media (max-width: 767.98px) {
  .rd-grid {
    grid-template-columns: 1.6rem minmax(0, 1fr) 3.4rem;
    gap: 0.55rem;
  }

  .rd-grid-mini,
  .rd-enter {
    display: none;
  }

  .rd-story {
    padding: 0.85rem 0.1rem;
  }

  .rd-story-title {
    font-size: 1.05rem;
  }

  .rd-attention {
    font-size: 1.3rem;
  }

  .rd-trend {
    grid-column: 2;
    grid-row: 2;
    flex-direction: row;
    gap: 0.4rem;
  }

  .rd-trend-base {
    display: inline;
    white-space: normal;
  }
}
</style>
