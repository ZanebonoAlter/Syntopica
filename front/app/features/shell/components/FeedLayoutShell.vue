<script setup lang="ts">
import { Icon } from '@iconify/vue'
import LayoutAppHeader from '~/features/shell/components/AppHeaderShell.vue'
import LayoutAppSidebar from '~/features/shell/components/AppSidebarShell.vue'
import LayoutArticleListPanel from '~/features/shell/components/ArticleListPanelShell.vue'
import { useArticlesApi } from '~/api/articles'
import ArticleContent from '~/features/articles/components/ArticleContentView.vue'
import { normalizeArticle, type ArticlePayload } from '../../articles/utils/normalizeArticle'
import { useGlobalAutoRefresh } from '~/features/feeds/composables/useAutoRefresh'
import { useArticlePagination } from '~/features/articles/composables/useArticlePagination'
import { SIDEBAR_DEFAULT_WIDTH, MAX_POLLING_TIME, REFRESH_POLLING_INTERVAL } from '~/utils/constants'
import type { WatchedTag } from '~/api/watchedTags'
import type { Article } from '~/types/article'
import type { ArticleFilters } from '~/types/article'

const apiStore = useApiStore()
const feedsStore = useFeedsStore()
const articlesApi = useArticlesApi()

const {
  state: paginationState,
  fetchFirstPage,
  loadMore,
  updateArticle,
} = useArticlePagination({ pageSize: 20 })

const startDate = ref<string>('')
const endDate = ref<string>('')

const articles = computed(() => paginationState.articles)
const hasMore = computed(() => paginationState.hasMore)
const total = computed(() => paginationState.total)
const loading = computed(() => paginationState.loading)

useGlobalAutoRefresh()

const sidebarCollapsed = ref(false)
const sidebarWidth = ref(SIDEBAR_DEFAULT_WIDTH)
const selectedCategory = ref<string | null>(null)
const selectedFeed = ref<string | null>(null)
const selectedArticle = ref<Article | null>(null)

const showAddFeedDialog = ref(false)
const showAddCategoryDialog = ref(false)
const showImportDialog = ref(false)
const showGlobalSettings = ref(false)

const editCategoryId = ref<string | null>(null)
const editFeedId = ref<string | null>(null)

const refreshMessage = ref('')
const refreshMessageType = ref<'success' | 'error' | 'info'>('info')
const showRefreshMessage = ref(false)

const globalUnreadCount = ref(0)

const watchedTags = ref<WatchedTag[]>([])
const selectedWatchedTagId = ref<string | null>(null)

const editingCategory = computed(() =>
  editCategoryId.value ? feedsStore.categories.find(c => c.id === editCategoryId.value) : null
)
const editingFeed = computed(() =>
  editFeedId.value ? feedsStore.feeds.find(f => f.id === editFeedId.value) : null
)

async function fetchGlobalUnreadCount() {
  const response = await apiStore.fetchArticlesStats()
  if (response.success && response.data) {
    globalUnreadCount.value = (response.data as any).unread || 0
  }
}

async function loadWatchedTags() {
  const { useWatchedTagsApi } = await import('~/api/watchedTags')
  const api = useWatchedTagsApi()
  const res = await api.listWatchedTags()
  if (res.success && res.data) {
    watchedTags.value = res.data as WatchedTag[]
  }
}

async function fetchFeeds() {
  if (selectedCategory.value === 'uncategorized') {
    await apiStore.fetchFeeds({ uncategorized: true, per_page: 10000 })
  } else if (selectedCategory.value && selectedCategory.value !== 'favorites') {
    await apiStore.fetchFeeds({ category_id: parseInt(selectedCategory.value), per_page: 10000 })
  } else {
    await apiStore.fetchFeeds({ per_page: 10000 })
  }
}

function buildArticleFilters() {
	const filters: ArticleFilters = {}
  if (selectedCategory.value === 'watched-tags') {
    if (selectedWatchedTagId.value) {
      filters.watched_tag_ids = selectedWatchedTagId.value
      filters.sort_by = 'date'
    } else {
      filters.watched_tags = true
      filters.sort_by = 'relevance'
    }
  } else if (selectedFeed.value) {
    filters.feed_id = parseInt(selectedFeed.value)
  } else if (selectedCategory.value === 'uncategorized') {
    filters.uncategorized = true
  } else if (selectedCategory.value === 'favorites') {
    filters.favorite = true
  } else if (selectedCategory.value && selectedCategory.value !== 'favorites') {
    filters.category_id = parseInt(selectedCategory.value)
  }
  if (startDate.value) {
    filters.start_date = startDate.value
  }
  if (endDate.value) {
    filters.end_date = endDate.value
  }
  return filters
}

async function loadArticles() {
  await fetchFirstPage(buildArticleFilters())
}

onMounted(async () => {
  // Wave 1: no dependencies between these two
  await Promise.allSettled([
    fetchFeeds(),
    loadWatchedTags(),
  ])
  // Wave 2: may depend on feeds list
  await Promise.allSettled([
    loadArticles(),
    fetchGlobalUnreadCount(),
  ])
})

onUnmounted(() => {
  stopPollingRefreshStatus()
})

async function hydrateSelectedArticle(article: any) {
  selectedArticle.value = article

  try {
    const response = await articlesApi.getArticle(Number(article.id))
    if (response.success && response.data && selectedArticle.value?.id === article.id) {
      selectedArticle.value = normalizeArticle(response.data as unknown as ArticlePayload)
    }
  } catch (error) {
    console.error('Failed to load article detail:', error)
  }
}

function handleArticleClick(article: any) {
  void hydrateSelectedArticle(article)
  if (!article.read) {
    updateArticle(article.id, { read: true })
    if (selectedArticle.value) {
      selectedArticle.value = { ...selectedArticle.value, read: true }
    }
    apiStore.markAsRead(article.id)
  }
}

async function handleArticleFavorite(articleId: string) {
  const article = articles.value.find(a => a.id === articleId)
  const newFavorite = !article?.favorite
  try {
    const response = await articlesApi.updateArticle(Number(articleId), { favorite: newFavorite })
    if (response.success) {
      if (article) {
        updateArticle(articleId, { favorite: newFavorite })
      }
      if (selectedArticle.value?.id === articleId) {
        selectedArticle.value = { ...selectedArticle.value, favorite: newFavorite }
      }
    } else {
      refreshMessage.value = response.error || '操作失败'
      refreshMessageType.value = 'error'
      showRefreshMessage.value = true
    }
  } catch {
    refreshMessage.value = '操作失败'
    refreshMessageType.value = 'error'
    showRefreshMessage.value = true
  }
}

function handleArticleUpdate(articleId: string, updates: Partial<any>) {
  updateArticle(articleId, updates)
}

function handleLoadMore() {
  void loadMore()
}

function handleDateFilterChange(newStartDate: string, newEndDate: string) {
  startDate.value = newStartDate
  endDate.value = newEndDate
  void loadArticles()
}

function handleDateFilterClear() {
  startDate.value = ''
  endDate.value = ''
  void loadArticles()
}

const handleCategoryClick = async (categoryId: string) => {
  selectedCategory.value = categoryId
  selectedFeed.value = null
  selectedWatchedTagId.value = null
  startDate.value = ''
  endDate.value = ''

  await fetchFeeds()
  await loadArticles()
  await fetchGlobalUnreadCount()
}

async function handleFeedClick(feedId: string) {
  selectedFeed.value = feedId
  selectedWatchedTagId.value = null
  startDate.value = ''
  endDate.value = ''

  await loadArticles()

  const response = await apiStore.refreshFeed(feedId)

  if (response.success) {
    refreshMessage.value = '正在刷新订阅源...'
    refreshMessageType.value = 'info'
    showRefreshMessage.value = true

    const feed = feedsStore.feeds.find(f => f.id === feedId)
    if (feed) {
      feed.refreshStatus = 'refreshing'
    }

    pollRefreshStatus()
  } else {
    refreshMessage.value = response.error || '刷新失败'
    refreshMessageType.value = 'error'
    showRefreshMessage.value = true
  }
}

async function handleFavoritesClick() {
  selectedCategory.value = 'favorites'
  selectedFeed.value = null
  selectedWatchedTagId.value = null
  startDate.value = ''
  endDate.value = ''

  await loadArticles()
}

async function handleAllArticlesClick() {
  selectedCategory.value = null
  selectedFeed.value = null
  selectedWatchedTagId.value = null
  startDate.value = ''
  endDate.value = ''

  await fetchFeeds()
  await loadArticles()
}

function handleTopicGraphClick() {
  selectedCategory.value = 'topic-graph'
  selectedFeed.value = null
  selectedWatchedTagId.value = null
  navigateTo('/topics')
}

async function handleWatchedTagsClick() {
  selectedCategory.value = 'watched-tags'
  selectedFeed.value = null
  selectedWatchedTagId.value = null
  startDate.value = ''
  endDate.value = ''
  await loadWatchedTags()
  await loadArticles()
}

async function handleWatchedTagClick(tagId: string) {
  selectedCategory.value = 'watched-tags'
  selectedFeed.value = null
  selectedWatchedTagId.value = tagId
  startDate.value = ''
  endDate.value = ''
  await loadWatchedTags()
  await loadArticles()
}

const refreshPollingInterval = ref<ReturnType<typeof setTimeout> | null>(null)

async function pollRefreshStatus() {
  const startTime = Date.now()

  const poll = async () => {
    if (Date.now() - startTime > MAX_POLLING_TIME) {
      stopPollingRefreshStatus()
      return
    }

    const response = await apiStore.fetchFeeds({ per_page: 10000 })

    if (response.success && response.data) {
      const data = response.data as any
      const items = data.items || data
      apiStore.allFeeds = items.map((feed: any) => ({
        id: String(feed.id),
        title: feed.title,
        description: feed.description || '',
        url: feed.url,
        category: feed.category_id ? String(feed.category_id) : '',
        icon: feed.icon || undefined,
        color: feed.color || '#6b7280',
        lastUpdated: feed.last_updated || new Date().toISOString(),
        articleCount: feed.article_count || 0,
        unreadCount: feed.unread_count || 0,
        maxArticles: feed.max_articles || 100,
        refreshInterval: feed.refresh_interval || 60,
        refreshStatus: feed.refresh_status || 'idle',
        refreshError: feed.refresh_error,
        lastRefreshAt: feed.last_refresh_at,
        aiSummaryEnabled: feed.ai_summary_enabled !== undefined ? feed.ai_summary_enabled : true,
        articleSummaryEnabled: feed.article_summary_enabled,
        completionOnRefresh: feed.completion_on_refresh,
        maxCompletionRetries: feed.max_completion_retries,
        firecrawlEnabled: feed.firecrawl_enabled,
      }))
    }

    const monitoredFeeds = selectedFeed.value
      ? feedsStore.feeds.filter(f => f.id === selectedFeed.value)
      : feedsStore.feeds

    const stillRefreshing = monitoredFeeds.some(f => f.refreshStatus === 'refreshing')

    if (stillRefreshing) {
      refreshPollingInterval.value = setTimeout(poll, REFRESH_POLLING_INTERVAL)
    } else {
      stopPollingRefreshStatus()
      await loadArticles()
      await fetchGlobalUnreadCount()

      refreshMessage.value = '刷新完成'
      refreshMessageType.value = 'success'
      showRefreshMessage.value = true
      setTimeout(() => {
        showRefreshMessage.value = false
      }, 3000)
    }
  }

  poll()
}

function stopPollingRefreshStatus() {
  if (refreshPollingInterval.value) {
    clearTimeout(refreshPollingInterval.value)
    refreshPollingInterval.value = null
  }
}

async function handleRefresh() {
  stopPollingRefreshStatus()

  if (selectedFeed.value) {
    const response = await apiStore.refreshFeed(selectedFeed.value)
    if (response.success) {
      refreshMessage.value = response.message || '已开始后台刷新当前订阅源'
      refreshMessageType.value = 'info'
      await fetchFeeds()
      pollRefreshStatus()
    } else {
      refreshMessage.value = response.error || '刷新失败'
      refreshMessageType.value = 'error'
    }
  } else {
    const response = await apiStore.refreshAllFeeds()
    if (response.success) {
      refreshMessage.value = response.message || '已开始后台刷新全部订阅源'
      refreshMessageType.value = 'info'
      await fetchFeeds()
      pollRefreshStatus()
    } else {
      refreshMessage.value = response.error || '刷新失败'
      refreshMessageType.value = 'error'
    }
  }

  showRefreshMessage.value = true
}

async function handleMarkAllRead() {
  let response: any
  if (selectedFeed.value) {
    response = await apiStore.markAllAsRead({ feedId: selectedFeed.value })
  } else if (selectedCategory.value) {
    if (selectedCategory.value === 'uncategorized') {
      response = await apiStore.markAllAsRead({ uncategorized: true })
    } else {
      const categoryId = parseInt(selectedCategory.value)
      if (categoryId > 0) {
        response = await apiStore.markAllAsRead({ categoryId })
      }
    }
  } else {
    response = await apiStore.markAllAsRead()
  }
  if (response && !response.success) {
    refreshMessage.value = response.error || '标记已读失败'
    refreshMessageType.value = 'error'
    showRefreshMessage.value = true
    return
  }
  await fetchGlobalUnreadCount()
}

async function handleExportOpml() {
  try {
    const blob = await apiStore.exportOpml()
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `feeds-export-${new Date().toISOString().split('T')[0]}.opml`
    a.click()
    window.URL.revokeObjectURL(url)
  } catch (error) {
    refreshMessage.value = '导出失败'
    refreshMessageType.value = 'error'
    showRefreshMessage.value = true
  }
}

function toggleSidebar() {
  sidebarCollapsed.value = !sidebarCollapsed.value
}

function handleEditCategory(categoryId: string) {
  editCategoryId.value = categoryId
}

function handleEditFeed(feedId: string) {
  editFeedId.value = feedId
}

async function handleDeleteCategory(categoryId: string, categoryName: string) {
  if (confirm(`确定要删除分类 "${categoryName}" 吗？这个操作不会删除分类下的订阅源。`)) {
    const response = await apiStore.deleteCategory(categoryId)
    if (response.success) {
    } else {
      alert(response.error || '删除失败')
    }
  }
}

import '~/components/FeedLayout.css'
</script>

<template>
  <div class="feed-layout">
    <!-- 顶部工具栏 -->
    <LayoutAppHeader
      :show-refresh-message="showRefreshMessage"
      :refresh-message="refreshMessage"
      :refresh-message-type="refreshMessageType"
      @toggle-sidebar="toggleSidebar"
      @refresh="handleRefresh"
      @mark-all-read="handleMarkAllRead"
      @add-feed="showAddFeedDialog = true"
      @add-category="showAddCategoryDialog = true"
      @import-opml="showImportDialog = true"
      @export-opml="handleExportOpml"
      @settings="showGlobalSettings = true"
      @close-refresh-message="showRefreshMessage = false"
    />

      <!-- 主内容区 -->
    <div class="main-content">
      <!-- 侧边栏 -->
      <LayoutAppSidebar
        :sidebar-collapsed="sidebarCollapsed"
        :sidebar-width="sidebarWidth"
        :selected-category="selectedCategory"
        :selected-feed="selectedFeed"
        :global-unread-count="globalUnreadCount"
        :watched-tags="watchedTags"
        :selected-watched-tag-id="selectedWatchedTagId"
        @toggle-sidebar="toggleSidebar"
        @category-click="handleCategoryClick"
        @feed-click="handleFeedClick"
        @favorites-click="handleFavoritesClick"
        @topic-graph-click="handleTopicGraphClick"
        @all-articles-click="handleAllArticlesClick"
        @edit-category="handleEditCategory"
        @edit-feed="handleEditFeed"
        @delete-category="handleDeleteCategory"
        @watched-tags-click="handleWatchedTagsClick"
        @watched-tag-click="handleWatchedTagClick"
      />

<!-- 文章列表 -->
      <LayoutArticleListPanel
        :articles="articles"
        :selected-category="selectedCategory"
        :selected-feed="selectedFeed"
        :selected-article="selectedArticle"
        :loading="loading"
        :has-more="hasMore"
        :total="total"
        :start-date="startDate"
        :end-date="endDate"
        @article-click="handleArticleClick"
        @article-favorite="handleArticleFavorite"
        @load-more="handleLoadMore"
        @date-filter-change="handleDateFilterChange"
        @date-filter-clear="handleDateFilterClear"
      />

<!-- 文章内容 -->
      <div class="content-panel">
        <ArticleContent
          :article="selectedArticle"
          :articles="articles"
          @favorite="handleArticleFavorite"
          @navigate="handleArticleClick"
          @article-update="handleArticleUpdate"
        />
      </div>
    </div>

    <!-- 添加订阅源对话框 -->
    <DialogAddFeedDialog
      v-if="showAddFeedDialog"
      @close="showAddFeedDialog = false"
      @added="() => {}"
    />

    <!-- 添加分类对话框 -->
    <DialogAddCategoryDialog
      v-if="showAddCategoryDialog"
      @close="showAddCategoryDialog = false"
      @added="() => {}"
    />

    <!-- 编辑分类对话框 -->
    <DialogEditCategoryDialog
      v-if="editCategoryId && editingCategory"
      :category="editingCategory"
      @close="editCategoryId = null"
      @updated="() => {}"
    />

    <!-- 编辑订阅源对话框 -->
    <DialogEditFeedDialog
      v-if="editFeedId && editingFeed"
      :feed="editingFeed"
      @close="editFeedId = null"
      @updated="() => {}"
      @deleted="() => {}"
    />

    <!-- 导入 OPML 对话框 -->
    <DialogImportOpmlDialog
      v-if="showImportDialog"
      @close="showImportDialog = false"
      @imported="() => {}"
    />

    <!-- 全局设置对话框 -->
    <DialogGlobalSettingsDialog :show="showGlobalSettings" @update:show="showGlobalSettings = $event" />
  </div>
</template>




