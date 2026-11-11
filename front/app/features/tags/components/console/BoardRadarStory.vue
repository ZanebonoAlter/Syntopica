<script setup lang="ts">
/**
 * 雷达总览故事行（rework-workshop-into-radar-and-desk 8A.1，已批准报刊式总览）。
 *
 * 一行完整横比：序号 / 板块名 + 最新日报（标题·日期·文章事件元信息）/ 逐泳道
 * 14 日事件线数小矩阵 / 关注强度分档（lane_heat 量纲）/ 近 3 日热度环比 / 进入。纯展示组件：输入为 API
 * 边界归一后的 camelCase/字符串 ID 读模型（RadarBoard），分档、环比、延迟与
 * 小矩阵口径来自 boardRadarHelpers，跳转由父级承接（进版块 / 进阅读台 /
 * 进 ?lane= 控制台）。本行网格列宽与总览表头 `.rd-labels` 保持一致（父组件列定义
 * 仅限定其自有 .rd-labels，不命中本组件根，避免覆盖移动端列定义，R-20），
 * 改断点须两处同步。
 *
 * 小矩阵**色阶语义**（修复「矩阵格子颜色没修复」）：已确证事件线格走 `.b1..b4`
 * （基准 = 行内事件线数最大值），降级格（事件线身份未回填、按分区数兜底）走
 * 独立的 `.d1..d4`（基准 = 行内分区数最大值）+ `.is-degraded` 斜纹形态——两种量纲
 * 不共用一条色阶，代理值不冒充已确证值、更不占最深实心档。颜色只影响展示：
 * MUST NOT 进 lane_heat / 行序 / 关注分档。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import { RADAR_HOT_ATTENTION, hasHeatSignal, isDelayed, laneDegradedLevel, laneLevel, miniAxis, shortDate, tierLabel, trendOf } from './boardRadarHelpers'
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
/** 有 heat 口径（add-event-story-heat 3b）：无 heat（旧响应）回退 sections 旧口径展示。 */
const heat = computed(() => hasHeatSignal(props.row))
/** 当日事件线数（无 heat 回退当日分区数；同一数字不再叫两个名字）。 */
const todayCount = computed(() => (heat.value ? props.row.todayStories : props.row.todaySections))
const anchorText = computed(() => {
  const asOf = delayed.value ? shortDate(props.row.anchorDate) : '今日'
  return `${asOf} · 当日事件线数 ${todayCount.value}`
})
const reportFallback = computed(() => (props.row.reportDate ? '查看已有日报' : '今天暂无日报'))
/** 小矩阵格子 title 的日期轴（无全局日期时退回板块自身锚日）。 */
const miniDates = computed(() => miniAxis(props.globalDate || props.row.anchorDate || '', 14))

/** 小矩阵逐格降级（matrix-clue-fallback-and-rail-merge D1）：事件线数 > 0 用事件线数，
 *  否则按该日**分区数**兜底——雷达行实际可得的口径就是分区数（`counts[i] =
 *  len(day.Sections)`，雷达行不带逐日线索合计；控制台矩阵可得线索合计，那边用线索）。
 *  降级只影响展示：MUST NOT 进 lane_heat / 行序 / 关注分档。
 *  旧后端未下发 story_counts（`storyCountsKnown=false`）时回退值本身就是分区数，走
 *  `sections` 口子按「N 个分区」措辞，不得静默当成事件线数（matrix-clue R-2）。 */
type LaneCellMode = 'story' | 'degraded' | 'sections'

function laneCells(lane: RadarLane): { count: number, mode: LaneCellMode }[] {
  const known = lane.storyCountsKnown !== false
  const stories = lane.storyCounts ?? lane.counts
  const sections = lane.counts ?? []
  return stories.map((story, i) => {
    const sectionCount = sections[i] ?? 0
    if (!known) return { count: story, mode: 'sections' as const }
    const degraded = story === 0 && sectionCount > 0
    return { count: degraded ? sectionCount : story, mode: degraded ? 'degraded' as const : 'story' as const }
  })
}

/**
 * 两条**互不共用**的分档基准（色阶语义，修复「矩阵格子颜色没修复」）：
 * - `confirmedMax`：行内**已确证事件线数**最大值 —— 已确证格（`.b*`）专用；
 *   旧后端 sections 口径下 `storyCounts = counts`，此时该基准 = 行内分区数
 *   最大值（沿用现状口径，整行同量纲、档位与改动前一致）。
 * - `proxyMax`：行内**代理值（分区数）**最大值 —— 降级格（`.d*`）专用。
 *
 * 为什么要分开：事件线数与分区数是**两种量纲**（一个事件线可能落多个分区，
 * 一个分区也可能没有事件线），共用一条色阶会让「1 个分区」和「1 条事件线」
 * 同色同档（代理值与确证值不可区分），还会让代理值稀释已确证格的对比度、
 * 在行内事件线都很少时独占最深档（零事件线的一天被画成最热的一天）。
 */
function confirmedMax(lane: RadarLane): number {
  return Math.max(1, ...laneCells(lane).filter(cell => cell.mode !== 'degraded').map(cell => cell.count))
}

function proxyMax(lane: RadarLane): number {
  return Math.max(1, ...laneCells(lane).filter(cell => cell.mode === 'degraded').map(cell => cell.count))
}

/** 格子色阶层名：已确证事件线格走 `.b1..b4`（基准=行内事件线最大值）；降级格走
 *  `.d1..d4`（基准=行内代理值最大值）并带 `.is-degraded` 形态标记（斜纹）；
 *  旧后端分区口径沿用 `.b*`（整行同量纲）。 */
function cellClass(lane: RadarLane, cell: { count: number, mode: LaneCellMode }): string {
  if (cell.mode === 'degraded') return `is-degraded d${laneDegradedLevel(cell.count, proxyMax(lane))}`
  return `b${laneLevel(cell.count, confirmedMax(lane))}`
}

/** 格子可读文本：降级格明示「无事件线（按分区数 N 显示）」，旧后端分区口径写「N 个分区」，
 *  不得静默冒充事件线数。 */
function cellTitle(cell: { count: number, mode: LaneCellMode }, dayIndex: number): string {
  const date = miniDates.value[dayIndex] || '日期未知'
  const label = cell.mode === 'degraded'
    ? `无事件线（按分区数 ${cell.count} 显示）`
    : cell.mode === 'sections'
      ? `${cell.count} 个分区`
      : `${cell.count} 条事件线`
  return `${date} · ${label}`
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
      <div v-for="lane in row.lanes" :key="lane.topicId" class="rd-mini-lane">
        <button
          type="button"
          class="rd-mini-name"
          :title="lane.label"
          @click.stop="openLane(lane)"
        >
          {{ lane.label }}
        </button>
        <div class="rd-mini-cells">
          <button
            v-for="(cell, dayIndex) in laneCells(lane)"
            :key="dayIndex"
            type="button"
            class="rd-mini-cell"
            :class="cellClass(lane, cell)"
            :title="cellTitle(cell, dayIndex)"
            :aria-label="`${lane.label} · ${cellTitle(cell, dayIndex)}，进入泳道`"
            @click.stop="openLane(lane)"
          />
        </div>
      </div>
      <small v-if="!row.lanes.length" class="rd-mini-empty">无活跃泳道 · 已有日报仍可读</small>
    </div>

    <div class="rd-attention-wrap">
      <span class="rd-attention">{{ row.attention.toFixed(1) }}</span>
      <small class="rd-tier">关注强度 · {{ tier }}</small>
    </div>

    <div class="rd-trend">
      <b class="rd-trend-value" :class="`is-${trend.dir}`">{{ trend.text }}</b>
      <small class="rd-trend-base">
        近 3 日 {{ heat ? row.last3Heat : row.last3Sections }} / 前 3 日 {{ heat ? row.prev3Heat : row.prev3Sections }}<br>
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

.rd-mini-cell {
  min-width: 0;
  padding: 0;
  border: 0;
  border-radius: 1px;
  background: var(--color-bg-hover);
  cursor: pointer;
}

.rd-mini-cell.b1 { background: color-mix(in srgb, var(--color-accent) 26%, var(--color-bg-base)); }
.rd-mini-cell.b2 { background: color-mix(in srgb, var(--color-accent) 47%, var(--color-bg-base)); }
.rd-mini-cell.b3 { background: color-mix(in srgb, var(--color-accent) 72%, var(--color-bg-base)); }
.rd-mini-cell.b4 { background: var(--color-accent); }

/* 降级格（事件线身份未回填、按分区数兜底）：与已确证事件线格**量纲不同**，
   故走独立色阶 `.d1..d4`（基准 = 行内分区数最大值）——色阶随兜底值变化，
   且不与任何 `.b*` 档同色；色强封顶在 `.b3` 之下（最深实心档只留给已确证的
   事件线峰值：身份缺失不得画成最热的一天）。再叠斜纹形态，与实心的已确证格
   一眼可辨（形状差异 = 第二信息通道，颜色不是唯一信息载体）。 */
.rd-mini-cell.d1 { background: color-mix(in srgb, var(--color-accent) 6%, var(--color-bg-base)); }
.rd-mini-cell.d2 { background: color-mix(in srgb, var(--color-accent) 18%, var(--color-bg-base)); }
.rd-mini-cell.d3 { background: color-mix(in srgb, var(--color-accent) 36%, var(--color-bg-base)); }
.rd-mini-cell.d4 { background: color-mix(in srgb, var(--color-accent) 58%, var(--color-bg-base)); }

.rd-mini-cell.is-degraded {
  background-image: repeating-linear-gradient(
    45deg,
    transparent 0 2px,
    color-mix(in srgb, var(--color-text-primary) 22%, transparent) 2px 3px
  );
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
