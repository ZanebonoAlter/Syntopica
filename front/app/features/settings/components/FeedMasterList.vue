<script setup lang="ts">
import { computed, ref } from 'vue'
import { Icon } from '@iconify/vue'
import FeedIcon from '~/components/feed/FeedIcon.vue'
import FeedSourceQualityPill from './FeedSourceQualityPill.vue'
import type { FeedBoardHitStats, RssFeed } from '~/types'
import {
  FEED_SORT_MODES,
  STATS_WINDOWS,
  compareFeedsBySortMode,
  isLowHitNoise,
  type FeedSortMode,
  type StatsWindowDays,
} from '../utils/sourceQuality'

const props = defineProps<{
  feedsByCategory: Record<string, RssFeed[]>
  collapsedCategories: Record<string, boolean>
  /** 按 feed_id（字符串键）索引的窗口内命中统计（add-source-board-hit-rate） */
  statsByFeed?: Record<string, FeedBoardHitStats>
  /** 统计聚合请求进行中（pill 显示「—」占位，meta 行沿用既有状态） */
  statsLoading?: boolean
  /** 当前统计窗口（与详情块共享同一状态，spec：任一处切换两处同步） */
  windowDays?: StatsWindowDays
}>()

const emit = defineEmits<{
  select: [feedId: string]
  toggleCollapse: [categoryName: string]
  setWindow: [days: StatsWindowDays]
}>()

const selectedFeedId = defineModel<string>('selectedFeedId')
const searchQuery = ref('')

// ---- 来源质量工具栏（add-source-board-hit-rate §4.1）----
const sortMode = ref<FeedSortMode>('default')
const lowHitOnly = ref(false)

function statsOf(feed: RssFeed): FeedBoardHitStats | undefined {
  return props.statsByFeed?.[String(feed.id)]
}

const filteredByCategory = computed(() => {
  const q = searchQuery.value.toLowerCase().trim()
  const result: Record<string, RssFeed[]> = {}
  for (const [cat, feeds] of Object.entries(props.feedsByCategory)) {
    let filtered = q
      ? feeds.filter(f => f.title.toLowerCase().includes(q) || f.url.toLowerCase().includes(q))
      : [...feeds]
    // 「只看低命中」：窗口内 ≥5 篇 且 率 <30% 且打标开启；筛选后空分类自动隐藏
    if (lowHitOnly.value) {
      filtered = filtered.filter(f => isLowHitNoise(props.statsByFeed?.[String(f.id)]))
    }
    // 排序在分类分组内生效，不打乱分类结构（spec：排序在既有分类分组内生效；
    // 默认选项 = 分类内标题序）
    filtered.sort((a, b) =>
      compareFeedsBySortMode(a, b, statsOf(a), statsOf(b), sortMode.value),
    )
    if (filtered.length > 0) result[cat] = filtered
  }
  return result
})

const totalFeeds = computed(() =>
  Object.values(props.feedsByCategory).reduce((sum, feeds) => sum + feeds.length, 0)
)

const totalFiltered = computed(() =>
  Object.values(filteredByCategory.value).reduce((sum, feeds) => sum + feeds.length, 0)
)

/** 筛选无结果（区别于搜索无结果）：提示 + 复位入口（spec Scenario：筛选无结果可复位） */
const filterEmpty = computed(() =>
  lowHitOnly.value && totalFeeds.value > 0 && totalFiltered.value === 0
)

function resetFilter() {
  lowHitOnly.value = false
}

const sortModeLabels: Record<FeedSortMode, string> = {
  default: '默认（分类内标题）',
  hit_rate_asc: '分类内 · 入板块率 ↑',
  articles_desc: '分类内 · 篇数 ↓',
  noise_desc: '分类内 · 杂音量 ↓',
}


function formatStatus(feed: RssFeed): string {
  if (feed.refreshStatus === 'refreshing') return '刷新中…'
  if (feed.refreshStatus === 'error') return '刷新失败'
  if (feed.lastRefreshAt) {
    const d = new Date(feed.lastRefreshAt)
    const now = new Date()
    const diffMin = Math.floor((now.getTime() - d.getTime()) / 60000)
    if (diffMin < 1) return '刚刚刷新'
    if (diffMin < 60) return `${diffMin} 分钟前`
    const diffHr = Math.floor(diffMin / 60)
    if (diffHr < 24) return `${diffHr} 小时前`
    return `${Math.floor(diffHr / 24)} 天前`
  }
  return '未刷新'
}

/**
 * 列表 meta 行（add-source-board-hit-rate §4.3）：
 * 有统计 → 「N 天内 X 篇 · 入板块 Y」（0 篇只显示「N 天内 0 篇」）；
 * 无统计（加载中/失败/未覆盖）→ 沿用既有刷新状态文案（State Matrix，不显示不可信数字）。
 */
function formatWindowMeta(feed: RssFeed): string | null {
  if (props.statsLoading) return null // 聚合请求中：沿用既有状态文案（State Matrix）
  const stats = statsOf(feed)
  if (!stats) return null
  if (stats.articles === 0) return `${props.windowDays ?? 7} 天内 0 篇`
  return `${props.windowDays ?? 7} 天内 ${stats.articles} 篇 · 入板块 ${stats.in_board}`
}
</script>

<template>
  <div class="feed-master">
    <!-- Search -->
    <div class="feed-master__search">
      <Icon icon="mdi:magnify" width="16" height="16" class="feed-master__search-icon" />
      <input
        v-model="searchQuery"
        type="text"
        placeholder="搜索订阅源…"
        class="feed-master__search-input"
      />
      <span v-if="totalFeeds > 0" class="feed-master__count">{{ totalFeeds }}</span>
    </div>

    <!-- 来源质量工具栏：窗口分段 / 排序 / 只看低命中（窄栏 flex-wrap 自动换行，不横向溢出） -->
    <div class="feed-master__toolbar" data-testid="feed-source-toolbar">
      <div class="feed-master__seg" role="group" aria-label="统计窗口">
        <button
          v-for="w in STATS_WINDOWS"
          :key="w"
          type="button"
          class="feed-master__seg-btn"
          :class="{ 'feed-master__seg-btn--on': (windowDays ?? 7) === w }"
          :data-window="w"
          @click="emit('setWindow', w)"
        >
          {{ w }} 天
        </button>
      </div>
      <select
        v-model="sortMode"
        class="feed-master__sort"
        data-testid="feed-sort-select"
        aria-label="列表排序"
      >
        <option v-for="mode in FEED_SORT_MODES" :key="mode" :value="mode">
          {{ sortModeLabels[mode] }}
        </option>
      </select>
      <button
        type="button"
        class="feed-master__filter-chip"
        :class="{ 'feed-master__filter-chip--on': lowHitOnly }"
        data-testid="feed-low-hit-filter"
        :aria-pressed="lowHitOnly"
        @click="lowHitOnly = !lowHitOnly"
      >
        只看低命中 <30%
      </button>
    </div>

    <!-- Feed list -->
    <div class="feed-master__list">
      <div v-if="totalFeeds === 0" class="feed-master__empty">
        <Icon icon="mdi:rss-off" width="32" height="32" style="color: var(--color-text-muted)" />
        <p style="color: var(--color-text-muted)">还没有订阅源</p>
      </div>

      <template v-for="(feeds, categoryName) in filteredByCategory" :key="categoryName">
        <!-- Category header -->
        <button
          class="feed-master__category"
          @click="emit('toggleCollapse', categoryName)"
        >
          <Icon
            :icon="collapsedCategories[categoryName] ? 'mdi:chevron-right' : 'mdi:chevron-down'"
            width="14"
            height="14"
          />
          <Icon icon="mdi:folder" width="14" height="14" />
          <span class="feed-master__category-name">{{ categoryName }}</span>
          <span class="feed-master__category-count">{{ feeds.length }}</span>
        </button>

        <!-- Feed items -->
        <div v-show="!collapsedCategories[categoryName]">
          <button
            v-for="feed in feeds"
            :key="feed.id"
            class="feed-master__item"
            :class="{ 'feed-master__item--active': selectedFeedId === feed.id }"
            @click="selectedFeedId = feed.id; emit('select', feed.id)"
          >
            <div
              class="feed-master__item-icon"
              :style="{ backgroundColor: (feed.color || '#999') + '18' }"
            >
              <FeedIcon :icon="feed.icon" :feed-id="feed.id" :color="feed.color" :size="16" />
            </div>
            <div class="feed-master__item-info">
              <span class="feed-master__item-title">{{ feed.title }}</span>
              <span class="feed-master__item-meta">
                {{ formatWindowMeta(feed) ?? formatStatus(feed) }}
                <template v-if="!formatWindowMeta(feed) && feed.articleCount"> · {{ feed.articleCount }} 篇</template>
              </span>
            </div>
            <FeedSourceQualityPill
              class="feed-master__item-pill"
              :articles="statsOf(feed)?.articles"
              :hit-rate="statsOf(feed)?.hit_rate"
              :tagging-enabled="statsOf(feed)?.tagging_enabled"
              :loading="statsLoading ?? false"
            />
          </button>
        </div>
      </template>

      <div v-if="filterEmpty" class="feed-master__empty feed-master__empty--filter" data-testid="feed-filter-empty">
        <Icon icon="mdi:filter-remove-outline" width="32" height="32" style="color: var(--color-text-muted)" />
        <p style="color: var(--color-text-muted)">没有符合筛选条件的订阅源</p>
        <button type="button" class="feed-master__reset-btn" data-testid="feed-filter-reset" @click="resetFilter">复位筛选</button>
      </div>
      <div v-else-if="Object.keys(filteredByCategory).length === 0 && totalFeeds > 0" class="feed-master__empty">
        <Icon icon="mdi:magnify-close" width="32" height="32" style="color: var(--color-text-muted)" />
        <p style="color: var(--color-text-muted)">未找到匹配的订阅源</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.feed-master {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
}

.feed-master__search {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.feed-master__search-icon {
  color: var(--color-text-muted);
  flex-shrink: 0;
}

.feed-master__search-input {
  flex: 1;
  min-width: 0;
  border: none;
  background: transparent;
  color: var(--color-text-primary);
  font-size: 13px;
  outline: none;
}

.feed-master__search-input::placeholder {
  color: var(--color-text-muted);
}

.feed-master__count {
  font-size: 11px;
  color: var(--color-text-muted);
  background: var(--color-bg-sunken);
  padding: 1px 6px;
  border-radius: 10px;
  flex-shrink: 0;
}

/* 来源质量工具栏（add-source-board-hit-rate §4.1）：窄栏自动换行不溢出 */
.feed-master__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 8px 12px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.feed-master__seg {
  display: inline-flex;
  border: 1px solid var(--color-border-medium);
  border-radius: 7px;
  overflow: hidden;
  flex: none;
}

.feed-master__seg-btn {
  background: none;
  border: none;
  font-size: 11px;
  padding: 4px 9px;
  color: var(--color-text-secondary);
  cursor: pointer;
  white-space: nowrap;
}

.feed-master__seg-btn + .feed-master__seg-btn {
  border-left: 1px solid var(--color-border-subtle);
}

.feed-master__seg-btn--on {
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  font-weight: 600;
}

.feed-master__sort {
  font-size: 11px;
  color: var(--color-text-secondary);
  background: var(--color-bg-base);
  border: 1px solid var(--color-border-medium);
  border-radius: 7px;
  padding: 4px 6px;
  max-width: 100%;
  cursor: pointer;
}

.feed-master__filter-chip {
  font-size: 11px;
  padding: 4px 10px;
  border-radius: 12px;
  cursor: pointer;
  border: 1px solid var(--color-border-medium);
  background: none;
  color: var(--color-text-secondary);
  white-space: nowrap;
  transition: color 0.15s, border-color 0.15s, background 0.15s;
}

.feed-master__filter-chip--on {
  border-color: var(--color-warning);
  background: var(--color-warning-subtle);
  color: var(--color-warning);
  font-weight: 600;
}

.feed-master__reset-btn {
  padding: 4px 12px;
  font-size: 12px;
  border: 1px solid var(--color-border-medium);
  border-radius: 8px;
  background: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  cursor: pointer;
}

.feed-master__reset-btn:hover {
  background: var(--color-bg-hover);
}

.feed-master__list {
  flex: 1;
  overflow-y: auto;
  padding: 4px 0;
}

.feed-master__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 32px 16px;
  font-size: 13px;
}

.feed-master__category {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 6px 12px;
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  text-align: left;
  transition: background 0.15s;
}

.feed-master__category:hover {
  background: var(--color-bg-hover);
}

.feed-master__category-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.feed-master__category-count {
  font-size: 11px;
  font-weight: 400;
  color: var(--color-text-muted);
}

.feed-master__item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 8px 12px 8px 28px;
  border: none;
  background: transparent;
  cursor: pointer;
  text-align: left;
  transition: background 0.15s;
  border-left: 2px solid transparent;
}

.feed-master__item:hover {
  background: var(--color-bg-hover);
}

.feed-master__item--active {
  background: var(--color-accent-subtle);
  border-left-color: var(--color-accent);
}

.feed-master__item-icon {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.feed-master__item-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.feed-master__item-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.feed-master__item-meta {
  font-size: 11px;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.feed-master__item-pill {
  margin-left: auto;
}
</style>
