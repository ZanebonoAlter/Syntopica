<script setup lang="ts">
/**
 * 雷达总览阅读右栏（rework-workshop-into-radar-and-desk 8A/R-18 拆分 +
 * surface-story-events-in-radar 事件线化 + matrix-clue-fallback-and-rail-merge 单清单化）。
 *
 * 一块清单「今日该读」（≤ 8 条，后端截断）：版块名 + 事件线标题 + 来源数 + 热度 +
 * 锚点日，分档信息用**行内徽标**表达——`新起`（当日有证据 ∧ 近 3 日无基线）/
 * `升温 N×`（有基线 ∧ 倍率 ≥ 2 ∧ 当日有证据），无徽标 = 普通当日事件线。
 * 独立的「今天可能升温」「今日新起」卡片、剩余计数摘要行与三态空文案已随
 * 单清单合并删除（2026-11-09 用户拍板）；只剩一行「跨天延续建设中」脚注，
 * 且只在服务端判据 cross_day_story_count == 0 时出现。
 * 旧后端响应（无事件线清单段）回退版块级日报标题列表与版块级升温清单。
 * 判定、排序与截断全在后端完成（workshop-radar 契约），本组件不改口径。
 */
import { shortDate, tierLabel } from './boardRadarHelpers'
import type { RadarBoard } from '~/api/semanticBoards'
import type { RadarStoryItem } from '~/api/semanticBoards'
import type { RadarWarmingItem } from '~/features/tags/composables/useBoardRadarOverview'

defineProps<{
  /** 事件线清单可用（新版后端带 stories 段）；false = 旧响应回退版块级列表。 */
  storyMode: boolean
  topStories: RadarStoryItem[]
  /** 跨天延续脚注（服务端 cross_day_story_count == 0；旧响应缺字段 = false 不出脚注）。 */
  crossDayBuilding: boolean
  /** 事件身份未回填的版块数（该版块无条目，未纳入排序）。 */
  unbackfilledCount: number
  /** 部分版块降级 → 本次事件线排序不完整。 */
  partial: boolean
  /** 降级版块数（超时或事件线计算失败——提示里带上数量）。 */
  degradedCount: number
  /** 滞后版块数（各条目按自己的锚点日计——已知口径偏差如实呈现）。 */
  laggingCount: number
  /** 旧响应回退用：版块级日报标题列表与版块级升温清单。 */
  todayRead: RadarBoard[]
  warming: RadarWarmingItem[]
}>()

const emit = defineEmits<{
  (e: 'openStory', item: RadarStoryItem): void
  (e: 'openReport', row: RadarBoard): void
  (e: 'openBoard', boardId: string): void
}>()

/** 「今日该读」事件数口径（workshop-radar §28）：回填态 = 当日事件线数；
 * 未回填 = 非综合稿事件数上界（todayStories 降级值）并标注降级；
 * 老 payload 缺字段（null）回落日报事件数且不加标注。 */
function readEventText(row: RadarBoard): string {
  if (row.storiesBackfilled === false) return `${row.todayStories} 个事件（未回填上界）`
  if (row.storiesBackfilled === true) return `${row.todayStories} 个事件`
  return `${row.reportEventCount} 个事件`
}

/** 事件线标题：空标题回退「事件线 #ID」（身份靠 id，标题只是展示）。 */
function storyTitle(item: RadarStoryItem): string {
  const title = item.storyTitle?.trim()
  return title ? title : `事件线 #${item.storyId}`
}

/** 来源数：字段缺失就省略数字，不伪造 0（真实 0 显示「0 个来源」）。 */
function participantsText(item: RadarStoryItem): string {
  if (item.participants === null) return ''
  return `${item.participants} 个来源`
}

/** 条目可读全名（title/aria）：版块名 · 标题 · N 个来源 · 热度 X.X。 */
function storyAria(item: RadarStoryItem): string {
  return [item.boardLabel, storyTitle(item), participantsText(item), `热度 ${item.heat.toFixed(1)}`]
    .filter(Boolean)
    .join(' · ')
}

/** 「今日新起」/「今日升温」徽标的判定（D3，与被删卡片逐字相同，只换位置）：
 *  - `新起`：当日有证据 ∧ 近 3 日无基线（不显示倍率，不以 0/∞ 硬凑，红线 29）；
 *  - `升温`：有基线 ∧ 倍率 ≥ 2 ∧ 当日有证据（倍率挂事件线、不挂版块名）；
 *  - 无徽标 = 普通当日事件线（两判据互斥，hasBaseline 二值）。 */
function isNewStory(item: RadarStoryItem): boolean {
  return !item.hasBaseline && item.hasTodayEvidence
}

function isWarmingStory(item: RadarStoryItem): boolean {
  return item.hasBaseline && item.hasTodayEvidence && item.ratio >= 2
}

/** 有正文落点才可点（无落点 = 静态行，不跳假位置）。 */
function canOpen(item: RadarStoryItem): boolean {
  return Boolean(item.reportDate && item.boardId && item.storyId)
}

/**
 * 残热旧线（`hasTodayEvidence = false`）：锚点日只剩 48h 残热、没有当日报道证据。
 *
 * 这类条目既不出「新起」也不出「升温」（两判据都要求当日有证据），与
 * 「普通当日事件线」在视觉上不可区分（review R-7 展示层缺口）。处理：
 * meta 行补一句「今日无报道证据」标记——**不新增徽标档位、不改判据**
 * （红线 29 的徽标判据逐字不变），只把「今天没有报道证据」如实说出来。
 */
function isStaleHeat(item: RadarStoryItem): boolean {
  return !item.hasTodayEvidence
}
</script>

<template>
  <aside class="rd-rail" aria-label="今日阅读与升温">
    <section class="rd-rail-block">
      <div class="rd-rule-title">
        <h2>今日该读</h2>
        <span class="rd-rule-tag">READING LIST</span>
      </div>

      <!-- 未回填版块：明示未纳入排序（不得呈现为「今天没有事件」）。 -->
      <p v-if="storyMode && unbackfilledCount > 0" class="rd-rail-note" data-testid="radar-unbackfilled">
        {{ unbackfilledCount }} 个版块事件身份未回填，未纳入事件线排序。
      </p>
      <!-- 部分版块降级（超时或事件线计算失败）：本次排序不完整。 -->
      <p v-if="storyMode && partial" class="rd-rail-note" data-testid="radar-partial">
        部分版块数据延迟或降级（{{ degradedCount }} 个版块），本次事件线排序不完整。
      </p>
      <!-- 滞后版块：各条目按自己的锚点日计（跨版块混排的已知口径偏差，如实呈现）。 -->
      <p v-if="storyMode && laggingCount > 0" class="rd-rail-note" data-testid="radar-lagging">
        {{ laggingCount }} 个版块数据滞后（各条目按自己的锚点日计）。
      </p>

      <template v-if="storyMode">
        <component
          :is="canOpen(item) ? 'button' : 'div'"
          v-for="item in topStories"
          :key="`story-${item.storyId}`"
          :type="canOpen(item) ? 'button' : undefined"
          class="rd-read"
          :class="{ 'is-static': !canOpen(item) }"
          :data-testid="`radar-story-${item.storyId}`"
          :title="storyAria(item)"
          :aria-label="storyAria(item)"
          @click="canOpen(item) && emit('openStory', item)"
        >
          <span class="rd-read-board">{{ item.boardLabel }}</span>
          <span class="rd-read-line">
            <span class="rd-read-title">{{ storyTitle(item) }}</span>
            <!-- 行内徽标（D3）：只在存在时出现，无徽标 = 普通当日事件线 -->
            <span
              v-if="isNewStory(item)"
              class="rd-badge"
              :data-testid="`radar-badge-new-${item.storyId}`"
              :title="`${item.boardLabel} · 近 3 日无基线的新起事件线`"
            >新起</span>
            <span
              v-else-if="isWarmingStory(item)"
              class="rd-badge rd-badge-warm"
              :data-testid="`radar-badge-warm-${item.storyId}`"
              :title="`${item.boardLabel} · 当日热度 ${item.heat.toFixed(1)} / 近 3 日均值 ${item.baselineHeat.toFixed(1)}`"
            >升温 {{ item.ratio.toFixed(1) }}×</span>
          </span>
          <small class="rd-read-meta">
            <template v-if="participantsText(item)">{{ participantsText(item) }} · </template>
            热度 {{ item.heat.toFixed(1) }} · 截至 {{ shortDate(item.anchorDate) }}
            <span
              v-if="isStaleHeat(item)"
              class="rd-no-evidence"
              :data-testid="`radar-no-evidence-${item.storyId}`"
            >· 今日无报道证据</span>
          </small>
          <small v-if="!canOpen(item)" class="rd-read-meta">该事件线暂无正文落点</small>
        </component>
        <p v-if="!topStories.length" class="rd-rail-empty" data-testid="radar-no-story">
          今天没有可识别的事件线。
        </p>
        <!-- 徽标图例（一行，不占卡片）：分档信息由徽标表达，这里只说明判据。 -->
        <p class="rd-warm-note" data-testid="radar-badge-legend">
          徽标：新起 = 近 3 日无基线；升温 = 当日热度达自身近 3 日均值 2 倍（均要求当日有报道证据）。
        </p>
        <!-- 跨天延续建设中：只在服务端判据为 0 时一行脚注，不占卡片；有跨天线但没升温
             不出任何空态（徽标已表达），旧响应缺字段同样不出（无法判定不断言）。 -->
        <p v-if="crossDayBuilding" class="rd-rail-note" data-testid="radar-crossday-note">
          跨天延续事件线建设中。
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
            {{ shortDate(row.reportDate) }} · {{ row.reportArticleCount }} 篇文章 · {{ readEventText(row) }}
          </small>
        </button>
        <p v-if="!todayRead.length" class="rd-rail-empty">今天还没有日报。</p>
      </template>
    </section>

    <!-- 旧响应回退（无事件线清单段）：版块级升温清单原样保留——这是兼容路径，
         新口径（storyMode）下的升温已收敛为清单行内徽标，不再有独立卡片。 -->
    <section v-if="!storyMode" class="rd-rail-block rd-warm-block">
      <h2>今天可能升温</h2>
      <template v-for="item in warming" :key="`warm-${item.row.boardId}`">
        <button type="button" class="rd-warm" @click="emit('openBoard', item.row.boardId)">
          <span>{{ item.row.label }}</span>
          <b>{{ item.ratio.toFixed(1) }}×</b>
        </button>
        <p class="rd-warm-why">
          {{ item.todayCount }} 个当日事件线 / 近 3 日均值 {{ item.baselineAvg.toFixed(1) }}。热度达均值 {{ item.ratio.toFixed(1) }}×，仍属{{ tierLabel(item.row.attention) }}档，不是热点判定。
        </p>
      </template>
      <p v-if="!warming.length" class="rd-rail-empty" data-testid="radar-no-warm">
        暂未触发升温条件。
      </p>
      <p class="rd-warm-note">当日事件线 ≥3，且事件线热度达到近 3 日均值的 2 倍。</p>
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

.rd-read.is-static {
  cursor: default;
}

.rd-read:hover .rd-read-title {
  color: var(--color-accent);
}

.rd-read.is-static:hover .rd-read-title {
  color: var(--color-text-primary);
}

.rd-read-board {
  color: var(--color-accent);
  font-size: 0.62rem;
  letter-spacing: 0.04em;
}

/* 标题行：标题 + 行内徽标（徽标只在存在时出现，不占位）。 */
.rd-read-line {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 0.3rem;
  min-width: 0;
}

/* 行内徽标（D3）：复用既有 accent 系 token 与右栏排版语言，新增最小样式。
   字号/字距取右栏既有小字档，不新增色值；「升温」加淡底提一档存在感。 */
.rd-badge {
  flex: 0 0 auto;
  padding: 0 0.3rem;
  border: 1px solid color-mix(in srgb, var(--color-accent) 45%, transparent);
  border-radius: 3px;
  color: var(--color-accent);
  font-size: 0.58rem;
  letter-spacing: 0.04em;
  line-height: 1.6;
  white-space: nowrap;
}

.rd-badge-warm {
  background: color-mix(in srgb, var(--color-accent) 10%, transparent);
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

/* 残热旧线标记（review R-7）：锚点日无报道证据的旧线，两徽标都不出——
   在 meta 行如实标一句，与「普通当日事件线」区分开（不新增徽标档位）。 */
.rd-no-evidence {
  color: var(--color-warning);
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

/* 旧响应回退块（storyMode=false）：版块级升温清单，样式沿用合并前的口径。 */
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
