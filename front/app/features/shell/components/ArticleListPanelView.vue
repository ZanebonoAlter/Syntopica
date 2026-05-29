<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { useVirtualList } from '@vueuse/core'
import ArticleCard from '~/features/articles/components/ArticleCardView.vue'
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

const apiStore = useApiStore()
const feedsStore = useFeedsStore()

const showDateFilter = ref(false)
const localStartDate = ref(props.startDate)
const localEndDate = ref(props.endDate)
const selectedQuickDate = ref<number | null>(null)
const feedStatusExpanded = ref(true)
const listContainerRef = ref<HTMLElement | null>(null)

const { list, containerProps, wrapperProps } = useVirtualList(
  toRef(() => props.articles),
  { itemHeight: 120, overscan: 5 }
)

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
}, { immediate: true })

onUnmounted(() => {
  if (containerProps.ref.value) {
    containerProps.ref.value.removeEventListener('scroll', onContainerScroll)
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

const feedStatusItems = computed(() => {
  if (!currentFeed.value) return []

  return [
    {
      label: '刷新',
      value: currentFeed.value.refreshStatus === 'refreshing'
        ? '进行中'
        : currentFeed.value.refreshStatus === 'success'
          ? '正常'
          : currentFeed.value.refreshStatus === 'error'
            ? '失败'
            : '空闲',
      tone: currentFeed.value.refreshStatus === 'error'
        ? 'rose'
        : currentFeed.value.refreshStatus === 'success'
          ? 'emerald'
          : currentFeed.value.refreshStatus === 'refreshing'
            ? 'sky'
            : 'stone',
      icon: currentFeed.value.refreshStatus === 'refreshing' ? 'mdi:loading' : 'mdi:refresh',
      spinning: currentFeed.value.refreshStatus === 'refreshing',
    },
    {
      label: '总结',
      value: currentFeed.value.articleSummaryEnabled ? '开启' : '关闭',
      tone: currentFeed.value.articleSummaryEnabled ? 'emerald' : 'stone',
      icon: 'mdi:brain',
      spinning: false,
    },
    {
      label: '抓取',
      value: currentFeed.value.firecrawlEnabled ? '开启' : '关闭',
      tone: currentFeed.value.firecrawlEnabled ? 'sky' : 'stone',
      icon: 'mdi:spider-web',
      spinning: false,
    },
  ]
})

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
  emit('dateFilterChange', localStartDate.value, localEndDate.value)
}

function clearDateFilter() {
  localStartDate.value = ''
  localEndDate.value = ''
  showDateFilter.value = false
  selectedQuickDate.value = null
  emit('dateFilterClear')
}

function handleArticleClick(article: Article) {
  emit('articleClick', article)
}

function handleFavorite(id: string) {
  emit('articleFavorite', id)
}

function statusToneClasses(tone: string) {
  if (tone === 'rose') return 'border-rose-200 bg-rose-50 text-rose-700'
  if (tone === 'emerald') return 'border-emerald-200 bg-emerald-50 text-emerald-700'
  if (tone === 'sky') return 'border-sky-200 bg-sky-50 text-sky-700'
  return 'border-stone-200 bg-stone-100 text-stone-700'
}

import '~/components/layout/ArticleListPanel.css'
</script>

<template>
  <div class="article-list-panel">
    <div class="panel-header">
      <div class="header-content">
        <h2 class="header-title">{{ panelTitle }}</h2>
        <span class="article-count">{{ props.total }}</span>
      </div>
    </div>

    <div v-if="currentFeed" class="mx-4 mt-4">
      <button
        v-if="!feedStatusExpanded"
        class="flex items-center gap-2 rounded-full border border-ink-200 bg-white/75 px-3 py-1.5 shadow-subtle transition-colors hover:bg-white"
        @click="feedStatusExpanded = true"
      >
        <Icon icon="mdi:information-slab-circle" width="18" height="18" class="text-ink-400" />
      </button>
      <div v-else class="rounded-2xl border border-ink-200 bg-white/75 p-3 shadow-subtle">
        <div class="flex flex-wrap items-center gap-2">
          <button @click="feedStatusExpanded = false">
            <Icon icon="mdi:chevron-up" width="16" height="16" class="text-ink-400" />
          </button>
          <span class="mr-1 text-sm font-semibold text-ink-black">订阅源状态</span>
          <div v-for="item in feedStatusItems" :key="item.label" class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-semibold" :class="statusToneClasses(item.tone)">
            <Icon :icon="item.icon" width="14" height="14" :class="{ 'animate-spin': item.spinning }" />
            <span>{{ item.label }}</span>
            <span>{{ item.value }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="filter-bar">
      <div class="filter-left">
        <button class="filter-toggle-btn" :class="{ active: showDateFilter || localStartDate || localEndDate || selectedQuickDate }" @click="showDateFilter = !showDateFilter">
          <Icon :icon="showDateFilter ? 'mdi:chevron-up' : 'mdi:chevron-down'" width="14" height="14" class="toggle-icon" />
          <Icon icon="mdi:calendar-filter" width="14" height="14" />
          <span>日期筛选</span>
        </button>
      </div>
    </div>

    <div v-if="showDateFilter" class="date-filter-panel">
      <div class="quick-options-row">
        <span class="row-label">快速选择：</span>
        <div class="quick-options">
          <button v-for="option in quickDateOptions" :key="option.days" class="quick-option-btn" :class="{ active: selectedQuickDate === option.days }" @click="applyQuickDateFilter(option.days)">
            {{ option.label }}
          </button>
        </div>
      </div>

      <div class="custom-date-row-vertical">
        <div class="date-input-row">
          <span class="row-label">开始日期</span>
          <input v-model="localStartDate" type="date" class="date-input" @change="selectedQuickDate = null" />
        </div>
        <div class="date-input-row">
          <span class="row-label">结束日期</span>
          <input v-model="localEndDate" type="date" class="date-input" @change="selectedQuickDate = null" />
        </div>
      </div>

      <div class="panel-actions">
        <button class="btn-secondary-sm" @click="clearDateFilter">清除筛选</button>
        <button class="btn-primary-sm" @click="applyCustomDateFilter">应用筛选</button>
      </div>
    </div>

    <div ref="listContainerRef" class="panel-content">
      <div v-if="props.loading && props.articles.length === 0" class="loading-state">
        <div class="text-center">
          <Icon icon="mdi:loading" width="32" height="32" class="animate-spin text-blue-600 mx-auto mb-2" />
          <p class="text-sm text-gray-500">加载中...</p>
        </div>
      </div>

      <div v-else-if="props.articles.length > 0" class="articles-list">
        <div v-bind="containerProps" class="virtual-list">
          <div v-bind="wrapperProps">
            <div v-for="{ data: article, index } in list" :key="article.id" class="virtual-item">
              <ArticleCard
                :article="article"
                :selected="props.selectedArticle?.id === article.id"
                compact
                @click="handleArticleClick"
                @favorite="handleFavorite"
              />
            </div>
          </div>
        </div>

        <div v-if="props.loading" class="loading-more">
          <Icon icon="mdi:loading" width="20" height="20" class="animate-spin text-blue-600" />
          <span class="text-sm text-gray-500">加载更多...</span>
        </div>

        <div v-else-if="!props.hasMore && props.articles.length > 0" class="no-more">
          <span class="text-xs text-gray-400">已加载全部文章</span>
        </div>
      </div>

      <div v-else class="empty-state">
        <div class="text-center">
          <Icon icon="mdi:file-document-outline" width="48" height="48" class="text-gray-300 mx-auto mb-2" />
          <h3 class="mb-1 text-base font-semibold text-gray-700">暂无文章</h3>
          <p class="text-sm text-gray-500">
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