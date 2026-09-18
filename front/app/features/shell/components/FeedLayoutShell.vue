<script setup lang="ts">
import { nextTick, provide, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useRouter } from 'vue-router'
import AppSidebarDrawer from '~/components/ui/AppSidebarDrawer.vue'
import { useIsNarrowViewport } from '~/composables/useMediaQuery'
import AppSidebarView from '~/features/shell/components/AppSidebarView.vue'
import { FEED_LIST_SCROLL_BRIDGE, type FeedListScrollBridge } from './feedListScrollBridge'
import LayoutAppHeader from '~/features/shell/components/AppHeaderShell.vue'
import LayoutAppSidebar from '~/features/shell/components/AppSidebarShell.vue'
import LayoutArticleListPanel from '~/features/shell/components/ArticleListPanelShell.vue'
import { useArticlesApi } from '~/api/articles'
import { ArticleContentView as ArticleContent } from '~/features/articles/public'
import { normalizeArticle, type ArticlePayload } from '~/api/normalizers/article'
import { useGlobalAutoRefresh } from '~/features/feeds/public'
import { useArticlePagination } from '~/features/articles/public'
import { useOnboarding } from '~/composables/useOnboarding'
import FeedEmptyGuide from '~/features/feeds/components/FeedEmptyGuide.vue'
import { SIDEBAR_DEFAULT_WIDTH, MAX_POLLING_TIME, REFRESH_POLLING_INTERVAL } from '~/utils/constants'
import type { WatchedTag } from '~/api/watchedTags'
import type { Article, ArticleFilters } from '~/types/article'
import type { ApiResponse } from '~/types'

const apiStore = useApiStore()
const feedsStore = useFeedsStore()
const articlesApi = useArticlesApi()
const articlesStore = useArticlesStore()

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

const { isFirstRun, startTour } = useOnboarding()

// Feed 空状态引导：无订阅源且无分类时显示
const hasAnyFeedsOrCategories = computed(
  () => feedsStore.feeds.length > 0 || feedsStore.categories.length > 0,
)

const sidebarCollapsed = ref(false)
const sidebarWidth = ref(SIDEBAR_DEFAULT_WIDTH)
const selectedCategory = ref<string | null>(null)
const selectedFeed = ref<string | null>(null)
const selectedArticle = ref<Article | null>(null)

const showAddFeedDialog = ref(false)

// ── 窄屏降级状态（mobile-viewport-stage1 任务 2.3/3.1/3.2，design.md D2/D4）──
const isNarrowViewport = useIsNarrowViewport()
/** 列表态/阅读态互斥切换：仅窄屏分支读取，宽屏渲染路径不经过（宽屏零变化红线） */
const viewMode = ref<'list' | 'reading'>('list')
const drawerOpen = ref(false)

/** 列表滚动记忆桥：容器元素由 ArticleListPanelView 注册（useVirtualList 的 containerProps.ref 归其所有） */
const listScrollBridge: FeedListScrollBridge = { el: null }
provide(FEED_LIST_SCROLL_BRIDGE, listScrollBridge)
/** 进阅读态前的列表滚动位置（D4：返回列表后恢复） */
let savedScrollTop = 0

// 任务 3.2：跨断点状态保持——重置单栏态、保留 selectedArticle（宽屏恢复双栏联动）。
// 宽→窄回列表态（简单初始态）；窄→宽顺带关闭抽屉（抽屉不进宽屏渲染路径）。
watch(isNarrowViewport, (narrow) => {
  viewMode.value = 'list'
  if (!narrow) {
    drawerOpen.value = false
  }
})

/** 进阅读态：记录列表 scrollTop（已在阅读态内跳转文章不重复记录；正文区滚动不记忆） */
function enterReadingMode() {
  if (viewMode.value === 'list') {
    savedScrollTop = listScrollBridge.el?.scrollTop ?? 0
  }
  viewMode.value = 'reading'
}

/** 返回列表态：nextTick 恢复滚动；恢复点超出已加载范围先补一页（上限一次），仍不足落顶（D4） */
async function restoreListScroll() {
  await nextTick()
  const el = listScrollBridge.el
  if (!el || savedScrollTop <= 0) return
  if (el.scrollHeight - el.clientHeight < savedScrollTop && hasMore.value) {
    await loadMore()
    await nextTick()
  }
  if (el.scrollHeight - el.clientHeight < savedScrollTop) {
    el.scrollTop = 0
    return
  }
  el.scrollTop = savedScrollTop
}

function exitReadingMode() {
  viewMode.value = 'list'
  void restoreListScroll()
}

const router = useRouter()

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
  const response = await articlesStore.fetchArticlesStats()
  if (response.success && response.data) {
    globalUnreadCount.value = response.data.unread || 0
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
  if (selectedCategory.value === CAT_UNCATEGORIZED) {
    await apiStore.fetchFeeds({ uncategorized: true, per_page: 10000 })
  } else if (selectedCategory.value && selectedCategory.value !== CAT_FAVORITES) {
    await apiStore.fetchFeeds({ category_id: parseInt(selectedCategory.value), per_page: 10000 })
  } else {
    await apiStore.fetchFeeds({ per_page: 10000 })
  }
}

function buildArticleFilters() {
	const filters: ArticleFilters = {}
  if (selectedCategory.value === CAT_WATCHED_TAGS) {
    if (selectedWatchedTagId.value) {
      filters.watched_tag_ids = selectedWatchedTagId.value
      filters.sort_by = 'date'
    } else {
      filters.watched_tags = true
      filters.sort_by = 'relevance'
    }
  } else if (selectedFeed.value) {
    filters.feed_id = parseInt(selectedFeed.value)
  } else if (selectedCategory.value === CAT_UNCATEGORIZED) {
    filters.uncategorized = true
  } else if (selectedCategory.value === CAT_FAVORITES) {
    filters.favorite = true
  } else if (selectedCategory.value && selectedCategory.value !== CAT_FAVORITES) {
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
  // 并行加载所有初始数据 — feeds 已由 app.vue 通过 apiStore.initialize() 提前加载
  await Promise.allSettled([
    loadArticles(),
    loadWatchedTags(),
    fetchGlobalUnreadCount(),
  ])

  // 首次访问自动启动新手引导（侧边栏等锚点此时已在 DOM 中）
  if (isFirstRun.value) {
    void startTour()
  }
})

onUnmounted(() => {
  stopPollingRefreshStatus()
})

async function hydrateSelectedArticle(article: Article) {
  selectedArticle.value = article

  try {
    const response = await articlesApi.getArticle(Number(article.id))
    if (response.success && response.data && selectedArticle.value?.id === article.id) {
      const hydrated = normalizeArticle(response.data as unknown as ArticlePayload)
      // 保留乐观更新（hydrate 期间可能已调用 markAsRead）
      selectedArticle.value = {
        ...hydrated,
        read: selectedArticle.value.read || hydrated.read,
      }
    }
  } catch (error) {
    console.error('Failed to load article detail:', error)
  }
}

async function handleArticleClick(article: Article) {
  // 窄屏单栏切换（任务 3.1）：点列表项进阅读态；宽屏路径不经过 viewMode（零变化红线）
  if (isNarrowViewport.value) {
    enterReadingMode()
  }
  void hydrateSelectedArticle(article)
  if (!article.read) {
    updateArticle(article.id, { read: true })
    if (selectedArticle.value) {
      selectedArticle.value = { ...selectedArticle.value, read: true }
    }
    // 直接调 API 标记已读（store.markAsRead 搜的是空数组）
    await articlesApi.updateArticle(Number(article.id), { read: true })
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

function handleArticleUpdate(articleId: string, updates: Partial<Article>) {
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
  selectedCategory.value = CAT_FAVORITES
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

async function handleWatchedTagsClick() {
  selectedCategory.value = CAT_WATCHED_TAGS
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

    await apiStore.fetchFeeds({ per_page: 10000 })

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
  let response: ApiResponse<unknown> | undefined
  if (selectedFeed.value) {
    response = await articlesStore.markAllAsRead({ feedId: selectedFeed.value })
  } else if (selectedCategory.value) {
    if (selectedCategory.value === CAT_UNCATEGORIZED) {
      response = await articlesStore.markAllAsRead({ uncategorized: true })
    } else {
      const categoryId = parseInt(selectedCategory.value)
      if (categoryId > 0) {
        response = await articlesStore.markAllAsRead({ categoryId })
      }
    }
  } else {
    response = await articlesStore.markAllAsRead()
  }
  if (response && !response.success) {
    refreshMessage.value = response.error || '标记已读失败'
    refreshMessageType.value = 'error'
    showRefreshMessage.value = true
    return
  }
  await fetchGlobalUnreadCount()
}

// ── 窄屏抽屉（任务 2.3）：复用宽屏筛选逻辑；选定导航项关抽屉 + 回列表态顶部 ──
function closeDrawerAndResetToListTop() {
  drawerOpen.value = false
  if (!isNarrowViewport.value) return
  savedScrollTop = 0
  viewMode.value = 'list'
  void nextTick(() => {
    if (listScrollBridge.el) listScrollBridge.el.scrollTop = 0
  })
}

async function handleDrawerCategoryClick(categoryId: string) {
  closeDrawerAndResetToListTop()
  await handleCategoryClick(categoryId)
}

async function handleDrawerFeedClick(feedId: string) {
  closeDrawerAndResetToListTop()
  await handleFeedClick(feedId)
}

async function handleDrawerFavoritesClick() {
  closeDrawerAndResetToListTop()
  await handleFavoritesClick()
}

async function handleDrawerAllArticlesClick() {
  closeDrawerAndResetToListTop()
  await handleAllArticlesClick()
}

async function handleDrawerWatchedTagsClick() {
  closeDrawerAndResetToListTop()
  await handleWatchedTagsClick()
}

async function handleDrawerWatchedTagClick(tagId: string) {
  closeDrawerAndResetToListTop()
  await handleWatchedTagClick(tagId)
}

function handleDrawerEditCategory(categoryId: string) {
  closeDrawerAndResetToListTop()
  handleEditCategory(categoryId)
}

function handleDrawerEditFeed(feedId: string) {
  closeDrawerAndResetToListTop()
  handleEditFeed(feedId)
}

async function handleDrawerDeleteCategory(categoryId: string, categoryName: string) {
  closeDrawerAndResetToListTop()
  await handleDeleteCategory(categoryId, categoryName)
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
  // 文案必须与后端语义一致：删除分类会连带删除其下全部订阅源及其文章
  // （真库靠存量 FK 级联，代码显式删除，两处行为等价；见 heal-dangling-article-refs）。
  if (confirm(`确定要删除分类 "${categoryName}" 吗？该分类下的订阅源及其文章也会一并删除，且不可撤销。`)) {
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
      @open-drawer="drawerOpen = true"
      @refresh="handleRefresh"
      @mark-all-read="handleMarkAllRead"
      @settings="router.push('/settings')"
      @close-refresh-message="showRefreshMessage = false"
    />

    <!-- 主内容区：窄屏单栏化类（is-narrow/is-reading 仅窄屏分支绑定，宽屏 DOM 无此类） -->
    <div
      class="main-content"
      :class="{
        'is-narrow': isNarrowViewport,
        'is-reading': isNarrowViewport && viewMode === 'reading',
        'is-content-empty': !hasAnyFeedsOrCategories,
      }"
    >
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
        <!-- 窄屏阅读态返回条（任务 3.1）：仅窄屏阅读态渲染，宽屏 DOM 不存在 -->
        <button
          v-if="isNarrowViewport && viewMode === 'reading'"
          type="button"
          class="reading-back-bar"
          data-testid="reading-back-bar"
          @click="exitReadingMode"
        >
          <Icon icon="mdi:arrow-left" width="18" height="18" />
          <span>返回列表</span>
        </button>
        <FeedEmptyGuide
          v-if="!hasAnyFeedsOrCategories"
          @add="showAddFeedDialog = true"
        />
        <ArticleContent
          v-else
          :article="selectedArticle"
          :articles="articles"
          @favorite="handleArticleFavorite"
          @navigate="handleArticleClick"
          @article-update="handleArticleUpdate"
        />
      </div>
    </div>

    <!-- 添加订阅源对话框 -->
    <AddFeedDialog
      v-if="showAddFeedDialog"
      @close="showAddFeedDialog = false"
      @added="() => {}"
    />

    <!-- 编辑分类对话框 -->
    <EditCategoryDialog
      v-if="editCategoryId && editingCategory"
      :category="editingCategory"
      @close="editCategoryId = null"
      @updated="() => {}"
    />

    <!-- 编辑订阅源对话框 -->
    <EditFeedDialog
      v-if="editFeedId && editingFeed"
      :feed="editingFeed"
      @close="editFeedId = null"
      @updated="() => {}"
      @deleted="() => {}"
    />

    <!-- 窄屏导航抽屉（任务 2.3）：同一份 AppSidebarView 信息结构（design.md D3），
         选中态 props 单向传递；v-if 窄屏——宽屏渲染路径完全不挂载（宽屏零变化红线） -->
    <AppSidebarDrawer
      v-if="isNarrowViewport"
      :open="drawerOpen"
      @close="drawerOpen = false"
    >
      <AppSidebarView
        :selected-category="selectedCategory"
        :selected-feed="selectedFeed"
        :global-unread-count="globalUnreadCount"
        :watched-tags="watchedTags"
        :selected-watched-tag-id="selectedWatchedTagId"
        @category-click="handleDrawerCategoryClick"
        @feed-click="handleDrawerFeedClick"
        @favorites-click="handleDrawerFavoritesClick"
        @all-articles-click="handleDrawerAllArticlesClick"
        @edit-category="handleDrawerEditCategory"
        @edit-feed="handleDrawerEditFeed"
        @delete-category="handleDrawerDeleteCategory"
        @watched-tags-click="handleDrawerWatchedTagsClick"
        @watched-tag-click="handleDrawerWatchedTagClick"
      />
    </AppSidebarDrawer>

  </div>
</template>




