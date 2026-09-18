<script setup lang="ts">
import { inject } from 'vue'
import { Icon } from '@iconify/vue'
import { useVirtualList } from '@vueuse/core'
import { ArticleCardView as ArticleCard } from '~/features/articles/public'
import { FEED_LIST_SCROLL_BRIDGE } from './feedListScrollBridge'
import type { Article } from '~/types'

interface Props {
  articles: Article[]
  selectedCategory?: string | null
  selectedFeed?: string | null
  selectedArticle?: Article | null
  loading?: boolean
  hasMore?: boolean
  total?: number
  startDate?: string
  endDate?: string
}

const props = withDefaults(defineProps<Props>(), {
  selectedCategory: null,
  selectedFeed: null,
  selectedArticle: null,
  loading: false,
  hasMore: false,
  total: 0,
  startDate: '',
  endDate: '',
})

const emit = defineEmits<{
  articleClick: [article: Article]
  articleFavorite: [id: string]
  loadMore: []
  dateFilterChange: [startDate: string, endDate: string]
  dateFilterClear: []
}>()

const feedsStore = useFeedsStore()

/** 行式布局固定行高（design D1）：标题 2 行 + meta，实测校准值 */
const ROW_HEIGHT = 100

const listContainerRef = ref<HTMLElement | null>(null)
const panelHeaderRef = ref<HTMLElement | null>(null)

const { list, containerProps, wrapperProps } = useVirtualList(
  toRef(() => props.articles),
  { itemHeight: ROW_HEIGHT, overscan: 5 }
)

// 窄屏滚动记忆桥（任务 3.1，design.md D4）：把虚拟列表滚动容器注册给 FeedLayoutShell，
// 供列表/阅读切换时记忆与恢复 scrollTop；宽屏无 provide 方，inject 落空即不注册
const scrollBridge = inject(FEED_LIST_SCROLL_BRIDGE, null)

function onContainerScroll(event: Event) {
  const target = event.target as HTMLElement
  if (!target) return

  const scrollBottom = target.scrollHeight - target.scrollTop - target.clientHeight
  if (scrollBottom < 100 && props.hasMore && !props.loading) {
    emit('loadMore')
  }
}

watch(containerProps.ref, (el) => {
  if (el) {
    el.addEventListener('scroll', onContainerScroll)
  }
  if (scrollBridge) {
    scrollBridge.el = el
  }
}, { immediate: true })

onUnmounted(() => {
  if (containerProps.ref.value) {
    containerProps.ref.value.removeEventListener('scroll', onContainerScroll)
  }
  if (scrollBridge) {
    scrollBridge.el = null
  }
})

interface QuickDateOption {
  label: string
  days: number
}

const quickDateOptions: QuickDateOption[] = [
  { label: '1天内', days: 1 },
  { label: '3天内', days: 3 },
  { label: '7天内', days: 7 },
  { label: '30天内', days: 30 },
]

const currentFeed = computed(() => props.selectedFeed ? feedsStore.feeds.find(feed => feed.id === props.selectedFeed) ?? null : null)

watch(() => props.startDate, (val) => {
  localStartDate.value = val
})

watch(() => props.endDate, (val) => {
  localEndDate.value = val
})

const panelTitle = computed(() => {
  if (currentFeed.value) return currentFeed.value.title
  if (props.selectedCategory === 'favorites') return '收藏夹'
  if (props.selectedCategory === 'uncategorized') return '未分类文章'
  if (props.selectedCategory) return '分类文章'
  return '全部文章'
})

// ---------- 头部浮层（日期面板 / 订阅状态 popover） ----------
const showDateFilter = ref(false)
const localStartDate = ref(props.startDate)
const localEndDate = ref(props.endDate)
const selectedQuickDate = ref<number | null>(null)
const feedPopOpen = ref(false)

const dateFilterActive = computed(() => Boolean(localStartDate.value || localEndDate.value || selectedQuickDate.value))

const dateFilterLabel = computed(() => {
  if (selectedQuickDate.value) return `${selectedQuickDate.value}天内`
  if (localStartDate.value && localEndDate.value) return `${localStartDate.value} ~ ${localEndDate.value}`
  return localStartDate.value || localEndDate.value || ''
})

function toggleFeedPop() {
  feedPopOpen.value = !feedPopOpen.value
  if (feedPopOpen.value) showDateFilter.value = false
}

function toggleDatePanel() {
  showDateFilter.value = !showDateFilter.value
  if (showDateFilter.value) feedPopOpen.value = false
}

function onDocMouseDown(event: MouseEvent) {
  const target = event.target as HTMLElement
  // 点击面板内部或头部按钮不关（按钮自身 toggle；面板内交互保持）
  if (panelHeaderRef.value?.contains(target)) return
  showDateFilter.value = false
  feedPopOpen.value = false
}

watch([showDateFilter, feedPopOpen], ([a, b]) => {
  if (a || b) {
    document.addEventListener('mousedown', onDocMouseDown)
  } else {
    document.removeEventListener('mousedown', onDocMouseDown)
  }
})

onUnmounted(() => {
  document.removeEventListener('mousedown', onDocMouseDown)
})

const feedStatusItems = computed(() => {
  if (!currentFeed.value) return []

  const refresh = currentFeed.value.refreshStatus
  return [
    {
      label: '刷新',
      value: refresh === 'refreshing'
        ? '进行中'
        : refresh === 'success'
          ? '正常'
          : refresh === 'error'
            ? '失败'
            : '空闲',
      tone: refresh === 'error' ? 'danger' : refresh === 'success' ? 'success' : refresh === 'refreshing' ? 'info' : 'neutral',
      spinning: refresh === 'refreshing',
    },
    {
      label: '总结',
      value: currentFeed.value.articleSummaryEnabled ? '开启' : '关闭',
      tone: currentFeed.value.articleSummaryEnabled ? 'success' : 'neutral',
      spinning: false,
    },
    {
      label: '抓取',
      value: currentFeed.value.firecrawlEnabled ? '开启' : '关闭',
      tone: currentFeed.value.firecrawlEnabled ? 'info' : 'neutral',
      spinning: false,
    },
  ]
})

// ---------- 日期筛选 ----------
function applyQuickDateFilter(days: number) {
  if (selectedQuickDate.value === days) {
    selectedQuickDate.value = null
    clearDateFilter()
    return
  }

  selectedQuickDate.value = days
  const end = new Date()
  const start = new Date()
  start.setDate(start.getDate() - days)

  localStartDate.value = start.toISOString().split('T')[0] || ''
  localEndDate.value = end.toISOString().split('T')[0] || ''
  emit('dateFilterChange', localStartDate.value, localEndDate.value)
}

function applyCustomDateFilter() {
  showDateFilter.value = false
  emit('dateFilterChange', localStartDate.value, localEndDate.value)
}

function clearDateFilter() {
  localStartDate.value = ''
  localEndDate.value = ''
  showDateFilter.value = false
  selectedQuickDate.value = null
  emit('dateFilterClear')
}

// ---------- 行交互 ----------
function handleArticleClick(article: Article) {
  emit('articleClick', article)
}

function handleFavorite(id: string) {
  emit('articleFavorite', id)
}

import '~/components/layout/ArticleListPanel.css'
</script>

<template>
  <div class="article-list-panel">
    <div ref="panelHeaderRef" class="panel-header">
      <h2 class="header-title">{{ panelTitle }}</h2>
      <span class="article-count">{{ props.total }}</span>

      <span v-if="dateFilterActive" class="filter-chip">
        <span class="filter-chip-text">{{ dateFilterLabel }}</span>
        <button class="filter-chip-clear" aria-label="清除日期筛选" @click="clearDateFilter">
          <Icon icon="mdi:close" width="11" height="11" />
        </button>
      </span>

      <span class="header-spacer"></span>

      <button
        v-if="currentFeed"
        class="header-icon-btn"
        :class="{ on: feedPopOpen }"
        aria-label="订阅源状态"
        title="订阅源状态"
        @click="toggleFeedPop"
      >
        <Icon icon="mdi:information-slab-circle" width="16" height="16" />
      </button>
      <button
        class="header-icon-btn"
        :class="{ on: showDateFilter }"
        aria-label="日期筛选"
        title="日期筛选"
        @click="toggleDatePanel"
      >
        <Icon icon="mdi:calendar-filter" width="16" height="16" />
      </button>

      <!-- 日期筛选下拉面板（absolute 于 header，覆盖下方行） -->
      <div v-if="showDateFilter" class="head-pop date-pop">
        <div class="hp-section-label">快速选择</div>
        <div class="quick-options">
          <button
            v-for="option in quickDateOptions"
            :key="option.days"
            class="quick-option-btn"
            :class="{ active: selectedQuickDate === option.days }"
            @click="applyQuickDateFilter(option.days)"
          >
            {{ option.label }}
          </button>
        </div>

        <div class="date-input-row">
          <span class="row-label">开始日期</span>
          <input v-model="localStartDate" type="date" class="date-input" @change="selectedQuickDate = null" />
        </div>
        <div class="date-input-row">
          <span class="row-label">结束日期</span>
          <input v-model="localEndDate" type="date" class="date-input" @change="selectedQuickDate = null" />
        </div>

        <div class="panel-actions">
          <AppButton variant="secondary" size="sm" @click="clearDateFilter">清除筛选</AppButton>
          <AppButton variant="primary" size="sm" @click="applyCustomDateFilter">应用筛选</AppButton>
        </div>
      </div>

      <!-- 订阅源状态 popover（只读） -->
      <div v-if="feedPopOpen && currentFeed" class="head-pop feed-pop">
        <div class="hp-section-label">订阅源状态（只读）</div>
        <div v-for="item in feedStatusItems" :key="item.label" class="feed-line">
          <span
            v-if="item.spinning"
            class="feed-dot feed-dot-spin"
            role="status"
          ></span>
          <span v-else class="feed-dot" :class="`feed-dot-${item.tone}`"></span>
          <span class="feed-key">{{ item.label }}</span>
          <span class="feed-value" :class="`feed-value-${item.tone}`">{{ item.value }}</span>
        </div>
        <div class="feed-hint">改开关请前往 设置 → 订阅源管理</div>
      </div>
    </div>

    <div ref="listContainerRef" class="panel-content">
      <div v-if="props.loading && props.articles.length === 0" class="loading-state">
        <div class="text-center">
          <Icon icon="mdi:loading" width="32" height="32" class="animate-spin mx-auto mb-2" style="color: var(--color-accent)" />
          <p class="text-sm" style="color: var(--color-text-secondary)">加载中...</p>
        </div>
      </div>

      <div v-else-if="props.articles.length > 0" class="articles-list">
        <div v-bind="containerProps" class="virtual-list">
          <div v-bind="wrapperProps">
            <div v-for="{ data: article, index } in list" :key="article.id" class="virtual-item">
              <ArticleCard
                :article="article"
                :selected="props.selectedArticle?.id === article.id"
                :show-feed-title="!currentFeed"
                compact
                @click="handleArticleClick"
                @favorite="handleFavorite"
              />
            </div>
          </div>
        </div>

        <div v-if="props.loading" class="loading-more">
          <Icon icon="mdi:loading" width="20" height="20" class="animate-spin" style="color: var(--color-accent)" />
          <span class="text-sm" style="color: var(--color-text-secondary)">加载更多...</span>
        </div>

        <div v-else-if="!props.hasMore && props.articles.length > 0" class="no-more">
          <span class="text-xs" style="color: var(--color-text-muted)">已加载全部文章</span>
        </div>
      </div>

      <div v-else class="empty-state">
        <div class="text-center">
          <Icon icon="mdi:file-document-outline" width="48" height="48" class="mx-auto mb-2" style="color: var(--color-text-muted)" />
          <h3 class="mb-1 text-base font-semibold" style="color: var(--color-text-secondary)">暂无文章</h3>
          <p class="text-sm" style="color: var(--color-text-muted)">
            {{ localStartDate || localEndDate ? '当前筛选条件下没有文章，请调整日期范围。' : '添加一些 RSS 订阅源开始阅读吧。' }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.virtual-list {
  flex: 1;
  overflow-y: auto;
}

.virtual-list::-webkit-scrollbar {
  width: 0.375rem;
}

.virtual-list::-webkit-scrollbar-track {
  background: transparent;
  border-radius: 4px;
}

.virtual-list::-webkit-scrollbar-thumb {
  background: rgba(26, 26, 26, 0.1);
  border-radius: 4px;
}

.virtual-list::-webkit-scrollbar-thumb:hover {
  background: rgba(26, 26, 26, 0.18);
}

.virtual-item {
  padding: 0 0.5rem;
  position: relative;
}

.loading-more {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 1rem;
  flex-shrink: 0;
}

.no-more {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  flex-shrink: 0;
}
</style>
