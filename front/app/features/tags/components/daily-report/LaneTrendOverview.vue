<script setup lang="ts">
import { computed, ref } from 'vue'
import { Icon } from '@iconify/vue'
import type { LaneDynamicsLane, LaneDynamicsResponse } from '~/api/laneDynamics'
import type { ContextRow } from '~/api/boardEnrichment'
import type { RequestCacheEntry } from './dailyReportMagazine'

/**
 * 泳道趋势区（lane-trend-overview design §D5/D6）。
 *
 * 泳道展开体顶部的跨周期趋势概览：近 14 天长版态势 / 月 / 年三档切换，
 * 概要全文展示不截断（pre-line 纯文本，不做 markdown 解析）。月/年档按需
 * 取话题周期归档（宿主 useLaneTrendData 管缓存，组件仅在 entry idle 时发
 * 取数信号）；14 天档附逐日事件就地展开（默认收起，单日 5 条折叠 + 后端
 * folded_count 如实标注，交互模式复制 LaneDynamicsCard、不共享以守住卡片不改）。
 *
 * 降级矩阵（D6）：长版缺失 → 短版 + 「长版随下次日报结算生成」提示；快照缺失
 * → 「态势待结算」占位（三档切换仍可用）；月/年空归档 → 「该周期暂无归档摘要」；
 * 请求失败 → 内联错误条 + 重试；加载中 → 加载提示（不以旧档内容冒充）。
 * 档位与展开态全部组件实例级（ref）：泳道收起即卸载重置，无跨展开残留。
 */
const props = defineProps<{
  topicId: number
  /** 泳道左缘线颜色（与泳道卡同源）。 */
  topicColor?: string
  /** 本泳道聚合数据（宿主按 topic_id 从 lanes 过滤），null = 不在聚合结果中。 */
  lane: LaneDynamicsLane | null
  /** 板块级 lane-dynamics 请求态（14 天档数据源，宿主 useLaneTrendData）。 */
  laneEntry: RequestCacheEntry<LaneDynamicsResponse>
  /** 月度归档缓存条目（键 topicId:month）。 */
  monthEntry: RequestCacheEntry<ContextRow[]>
  /** 年度归档缓存条目（键 topicId:year）。 */
  yearEntry: RequestCacheEntry<ContextRow[]>
}>()

const emit = defineEmits<{
  /** 14 天档错误重试（板块级 lane-dynamics force 重拉）。 */
  retryLane: []
  /** 切到月/年档且该粒度尚未请求（idle）时发一次；错误重试带 retry=true。 */
  ensureContext: [granularity: 'month' | 'year', retry?: boolean]
}>()

type TrendTab = '14d' | 'month' | 'year'

const TABS: Array<{ key: TrendTab, label: string }> = [
  { key: '14d', label: '近 14 天' },
  { key: 'month', label: '月' },
  { key: 'year', label: '年' },
]

/** 当前档：默认 14 天，每泳道独立（组件实例级）。 */
const activeTab = ref<TrendTab>('14d')
/** 逐日事件展开开关（默认收起）。 */
const eventsExpanded = ref(false)

const trendStyle = computed(() => (props.topicColor ? { '--topic-color': props.topicColor } : undefined))

// ── 14 天档：快照长/短版降级 ────────────────────────────────────────────────

const lanePending = computed(() => props.laneEntry.status === 'idle' || props.laneEntry.status === 'loading')
const hasSnapshot = computed(() => !!props.lane?.snapshot)
/** 长版缺失但短版在（存量快照/生成失败降级）。 */
const detailMissing = computed(() => {
  const snapshot = props.lane?.snapshot
  return !!snapshot && !snapshot.detail
})
const summaryText = computed(() => props.lane?.snapshot?.detail || props.lane?.snapshot?.summary || '')
const snapshotAsOf = computed(() => props.lane?.snapshot?.as_of ?? '')

// ── 月/年档：最新归档（period 字典序最大一条） ──────────────────────────────

/** 月/年档共享一份面板渲染：取当前档的缓存条目。 */
const contextEntry = computed<RequestCacheEntry<ContextRow[]>>(() =>
  activeTab.value === 'month' ? props.monthEntry : props.yearEntry,
)
const contextGranularityLabel = computed(() => (activeTab.value === 'month' ? '月度' : '年度'))
const contextPending = computed(() => contextEntry.value.status === 'idle' || contextEntry.value.status === 'loading')

/** 前端自取 period 字典序最大一条（不依赖响应排序）。 */
const latestRow = computed<ContextRow | null>(() => {
  const rows = contextEntry.value.data ?? []
  if (!rows.length) return null
  return rows.reduce((latest, row) => (row.period > latest.period ? row : latest))
})
const hasArchive = computed(() => !!latestRow.value)
const contextContent = computed(() => latestRow.value?.content ?? '')
const contextAsOf = computed(() => latestRow.value?.as_of_date ?? '')

function selectTab(tab: TrendTab) {
  activeTab.value = tab
  if (tab === 'month' || tab === 'year') {
    const entry = tab === 'month' ? props.monthEntry : props.yearEntry
    // 仅在尚未请求过（idle）时发取数信号：成功命中缓存不发（FD-9），错误态交给重试按钮。
    if (entry.status === 'idle') emit('ensureContext', tab)
  }
}

function retryContext() {
  if (activeTab.value === 'month' || activeTab.value === 'year') emit('ensureContext', activeTab.value, true)
}

// ── 逐日事件（14 天档内，默认收起） ─────────────────────────────────────────

/** 单日事件折叠阈值（与 LaneDynamicsCard 同口径：单日超 5 条折叠）。 */
const DAY_EVENT_LIMIT = 5

/** 单日视图模型：当日事件 + 后端截断未载入数。 */
interface TrendDay {
  date: string
  events: string[]
  foldedFromBackend: number
}

const eventDays = computed<TrendDay[]>(() => {
  const days = (props.lane?.timeline ?? []).map(day => ({
    date: day.date,
    events: (day.sections ?? []).flatMap(section => section.events ?? []),
    foldedFromBackend: (day.sections ?? []).reduce((sum, section) => sum + (section.folded_count ?? 0), 0),
  }))
  // 后端契约即日期倒序；防御性再排一次，保证「日期倒序」渲染语义。
  return days.sort((a, b) => b.date.localeCompare(a.date))
})

/** 已展开「还有 N 条」的日期（date 在单泳道内唯一）。 */
const expandedDays = ref(new Set<string>())

function isCollapsed(day: TrendDay): boolean {
  return day.events.length > DAY_EVENT_LIMIT && !expandedDays.value.has(day.date)
}

function visibleEntries(day: TrendDay): string[] {
  return isCollapsed(day) ? day.events.slice(0, DAY_EVENT_LIMIT) : day.events
}

/** 「还有 N 条」计数 = 前端折叠数 + 后端截断数（如实提示被折叠总数）。 */
function hiddenCount(day: TrendDay): number {
  const clientFolded = isCollapsed(day) ? day.events.length - DAY_EVENT_LIMIT : 0
  return clientFolded + day.foldedFromBackend
}

function toggleDay(date: string) {
  const next = new Set(expandedDays.value)
  if (next.has(date)) next.delete(date)
  else next.add(date)
  expandedDays.value = next
}
</script>

<template>
  <section class="lto" data-testid="lane-trend-overview" :style="trendStyle" aria-label="泳道趋势">
    <header class="lto__head">
      <span class="lto__title">泳道趋势</span>
      <div class="lto__tabs" aria-label="趋势周期切换">
        <button
          v-for="tab in TABS"
          :key="tab.key"
          type="button"
          class="lto__tab"
          :class="{ active: activeTab === tab.key }"
          :data-testid="`trend-tab-${tab.key}`"
          @click="selectTab(tab.key)"
        >
          {{ tab.label }}
        </button>
      </div>
    </header>

    <!-- 近 14 天档：长版 / 短版回退 / 待结算占位 + 逐日事件 -->
    <div v-if="activeTab === '14d'" class="lto__panel" data-testid="trend-panel-14d">
      <div v-if="lanePending" class="lto__status" data-testid="trend-loading">
        <Icon icon="mdi:loading" width="13" class="lto__spin" aria-hidden="true" />
        正在加载态势…
      </div>
      <div v-else-if="laneEntry.status === 'error'" class="lto__error" data-testid="trend-error" role="alert">
        <span>{{ laneEntry.error || '态势加载失败' }}</span>
        <button type="button" data-testid="trend-retry" @click="emit('retryLane')">重试</button>
      </div>
      <template v-else>
        <template v-if="hasSnapshot">
          <p v-if="!detailMissing" class="lto__text" data-testid="trend-detail">{{ summaryText }}</p>
          <template v-else>
            <p class="lto__text" data-testid="trend-summary">{{ summaryText }}</p>
            <p class="lto__hint" data-testid="trend-detail-hint">长版随下次日报结算生成</p>
          </template>
          <p class="lto__asof" data-testid="trend-asof">汇总截止 {{ snapshotAsOf }}</p>
        </template>
        <div v-else class="lto__status" data-testid="trend-pending">态势待结算 · 可切换月 / 年档查看归档</div>

        <div class="lto__events">
          <button
            type="button"
            class="lto__events-toggle"
            data-testid="trend-events-toggle"
            @click="eventsExpanded = !eventsExpanded"
          >
            <Icon :icon="eventsExpanded ? 'mdi:chevron-up' : 'mdi:chevron-down'" width="14" aria-hidden="true" />
            {{ eventsExpanded ? '收起逐日事件' : '展开逐日事件' }}
          </button>
          <div v-if="eventsExpanded" data-testid="trend-events">
            <p v-if="!eventDays.length" class="lto__status" data-testid="trend-events-empty">近 14 天暂无事件脉络。</p>
            <div v-for="day in eventDays" :key="day.date" class="lto__day" :data-testid="`trend-day-${day.date}`">
              <span class="lto__day-date">{{ day.date }}</span>
              <p v-for="(eventText, index) in visibleEntries(day)" :key="index" class="lto__event">{{ eventText }}</p>
              <button
                v-if="isCollapsed(day)"
                type="button"
                class="lto__more"
                :data-testid="`trend-day-more-${day.date}`"
                @click="toggleDay(day.date)"
              >
                还有 {{ hiddenCount(day) }} 条
              </button>
              <span v-else-if="day.foldedFromBackend > 0" class="lto__folded" :data-testid="`trend-day-folded-${day.date}`">
                另有 {{ day.foldedFromBackend }} 条未载入
              </span>
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- 月 / 年档：最新归档全文（pre-line 纯文本） -->
    <div v-else class="lto__panel" :data-testid="`trend-panel-${activeTab}`">
      <div v-if="contextPending" class="lto__status" data-testid="trend-loading">
        <Icon icon="mdi:loading" width="13" class="lto__spin" aria-hidden="true" />
        正在加载{{ contextGranularityLabel }}归档…
      </div>
      <div v-else-if="contextEntry.status === 'error'" class="lto__error" data-testid="trend-error" role="alert">
        <span>{{ contextEntry.error || `${contextGranularityLabel}归档加载失败` }}</span>
        <button type="button" data-testid="trend-retry" @click="retryContext">重试</button>
      </div>
      <div v-else-if="!hasArchive" class="lto__status" data-testid="trend-empty">该周期暂无归档摘要</div>
      <template v-else>
        <p class="lto__text" data-testid="trend-context-content">{{ contextContent }}</p>
        <p class="lto__asof" data-testid="trend-asof">汇总截止 {{ contextAsOf }}</p>
      </template>
    </div>
  </section>
</template>

<style scoped>
.lto {
  padding: 0.75rem 0.875rem 0.85rem;
  border: 1px solid var(--color-border-subtle);
  border-left: 3px solid var(--topic-color, var(--color-accent));
  border-radius: 8px;
  background: var(--color-bg-elevated);
}

.lto__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.lto__title {
  color: var(--color-text-secondary);
  font-family: "Noto Serif SC", serif;
  font-size: 0.78rem;
  font-weight: 700;
}

/* 分段切换（复用 BoardThreadBrowser .btb-days-btn 同族视觉，原生 button + active 态） */
.lto__tabs {
  display: flex;
  gap: 0.25rem;
}

.lto__tab {
  padding: 0.2rem 0.55rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 0.65rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.lto__tab:hover {
  border-color: var(--color-border-strong);
  color: var(--color-text-secondary);
}

.lto__tab.active {
  border-color: var(--color-border-strong);
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.lto__tab:focus-visible {
  outline: 2px solid var(--color-input-focus);
  outline-offset: 2px;
}

/* 概要/归档全文：完整展示不截断（无省略号/行数钳制），pre-line 保留段落换行 */
.lto__text {
  margin: 0.55rem 0 0;
  color: var(--color-text-primary);
  font-size: 0.8rem;
  line-height: 1.75;
  white-space: pre-line;
}

.lto__hint {
  margin: 0.3rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.68rem;
}

.lto__asof {
  margin: 0.3rem 0 0;
  color: var(--color-text-muted);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.66rem;
}

/* 占位条（--bg-sunken，与 LaneDynamicsCard .ldc-pending 同族） */
.lto__status {
  margin-top: 0.55rem;
  padding: 0.45rem 0.6rem;
  border-radius: 6px;
  background: var(--color-bg-sunken);
  color: var(--color-text-muted);
  font-size: 0.72rem;
  text-align: center;
}

.lto__status .lto__spin {
  vertical-align: -2px;
  margin-right: 0.2rem;
}

/* 内联错误条 + 重试 */
.lto__error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  margin-top: 0.55rem;
  padding: 0.45rem 0.6rem;
  border-radius: 6px;
  background: var(--color-bg-sunken);
  color: var(--color-error);
  font-size: 0.72rem;
}

.lto__error button {
  flex-shrink: 0;
  padding: 0.15rem 0.55rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  font-size: 0.68rem;
  cursor: pointer;
}

.lto__error button:hover {
  border-color: var(--color-border-strong);
  color: var(--color-text-primary);
}

/* 逐日事件（默认收起） */
.lto__events {
  margin-top: 0.65rem;
  padding-top: 0.5rem;
  border-top: 1px dashed var(--color-border-subtle);
}

.lto__events-toggle {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  padding: 0;
  border: 0;
  background: none;
  color: var(--color-info);
  font-size: 0.7rem;
  cursor: pointer;
}

.lto__events-toggle:hover {
  text-decoration: underline;
}

.lto__day {
  margin-top: 0.55rem;
}

.lto__day-date {
  color: var(--color-text-muted);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.66rem;
}

.lto__event {
  position: relative;
  margin: 0.1rem 0 0;
  padding-left: 0.6rem;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  line-height: 1.6;
}

.lto__event::before {
  content: '·';
  position: absolute;
  left: 0;
  color: var(--color-text-muted);
}

/* 「还有 N 条」就地展开（与 .ldc-more 同族） */
.lto__more {
  margin-top: 0.15rem;
  padding: 0;
  border: none;
  background: none;
  color: var(--color-info);
  font-size: 0.68rem;
  cursor: pointer;
}

.lto__more:hover {
  text-decoration: underline;
}

/* 后端截断的如实标注（不可展开） */
.lto__folded {
  display: inline-block;
  margin-top: 0.15rem;
  color: var(--color-text-muted);
  font-size: 0.66rem;
}

.lto__spin {
  animation: ltoSpin 0.9s linear infinite;
}

@keyframes ltoSpin {
  to { transform: rotate(1turn); }
}

@media (max-width: 720px) {
  .lto__head {
    flex-direction: column;
    align-items: start;
    gap: 0.35rem;
  }
}

@media (prefers-reduced-motion: reduce) {
  .lto__spin {
    animation: none;
  }
}
</style>
