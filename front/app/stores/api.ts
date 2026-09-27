import type { Category, RssFeed, PaginatedData } from '~/types'
import { useCategoriesApi } from '~/api/categories'
import { useFeedsApi } from '~/api/feeds'
import { useOpmlApi } from '~/api/opml'

interface FeedPayload {
  id: number
  title: string
  description: string
  url: string
  category_id?: number
  icon?: string
  color?: string
  last_updated: string
  last_refresh_at?: string
  article_count: number
  unread_count: number
  max_articles: number
  refresh_interval: number
  refresh_status: string
  refresh_error?: string
  article_summary_enabled?: boolean
  completion_on_refresh?: boolean
  max_completion_retries?: number
  firecrawl_enabled?: boolean
  tagging_enabled?: boolean
}

export const useApiStore = defineStore('api', () => {
  const loading = ref(false)
  const error = ref<string | null>(null)

  // Categories
  const categories = ref<Category[]>([])

  async function fetchCategories() {
    loading.value = true
    error.value = null

    const categoriesApi = useCategoriesApi()
    const response = await categoriesApi.getCategories()

    if (response.success && response.data) {
      categories.value = response.data
    } else {
      error.value = response.error || 'Failed to fetch categories'
    }

    loading.value = false
    return response
  }

  function apiErrNotify(response: { success: boolean; error?: string }, fallback: string) {
    if (!response.success) {
      useNotify().error(response.error || fallback)
    }
  }

  async function createCategory(data: {
    name: string
    icon?: string
    color?: string
    description?: string
  }) {
    loading.value = true
    const categoriesApi = useCategoriesApi()
    const response = await categoriesApi.createCategory(data)
    loading.value = false

    if (response.success) {
      await fetchCategories()
    } else {
      apiErrNotify(response, "创建分类失败")
    }

    return response
  }

  async function updateCategory(
    id: string,
    data: {
      name?: string
      icon?: string
      color?: string
      description?: string
    }
  ) {
    loading.value = true
    const categoriesApi = useCategoriesApi()
    const response = await categoriesApi.updateCategory(Number(id), data)
    loading.value = false

    if (response.success) {
      await fetchCategories()
    } else {
      apiErrNotify(response, "更新分类失败")
    }

    return response
  }

  async function deleteCategory(id: string) {
    loading.value = true
    const categoriesApi = useCategoriesApi()
    const response = await categoriesApi.deleteCategory(Number(id))
    loading.value = false

    if (response.success) {
      await fetchCategories()
    } else {
      apiErrNotify(response, "删除分类失败")
    }

    return response
  }

  // Feeds
  const feeds = ref<RssFeed[]>([])

  async function fetchFeeds(params: { page?: number; per_page?: number; category_id?: number; uncategorized?: boolean } = {}) {
    loading.value = true
    error.value = null

    const feedsApi = useFeedsApi()
    const response = await feedsApi.getFeeds(params)

    if (response.success && response.data) {
      const data = response.data as unknown as PaginatedData<FeedPayload>
      const items = data.items || (response.data as unknown as FeedPayload[])

      const mappedFeeds = items.map((feed: FeedPayload) => ({
        id: String(feed.id),
        title: feed.title,
        description: feed.description || '',
        url: feed.url,
        category: feed.category_id ? String(feed.category_id) : '',
        icon: feed.icon || undefined, // Don't set default icon, let FeedIcon component handle fallback
        color: feed.color || '#6b7280',
        lastUpdated: feed.last_updated || new Date().toISOString(),
        articleCount: feed.article_count || 0,
        unreadCount: feed.unread_count || 0,
        maxArticles: feed.max_articles ?? 100,
        refreshInterval: feed.refresh_interval,
        refreshStatus: feed.refresh_status as RssFeed['refreshStatus'] || 'idle',
        refreshError: feed.refresh_error,
        lastRefreshAt: feed.last_refresh_at,
        articleSummaryEnabled: feed.article_summary_enabled ?? false, // 后端 gorm default:false
        completionOnRefresh: feed.completion_on_refresh ?? false, // unify-feed-summary-toggles：默认关，回退统一 false
        maxCompletionRetries: feed.max_completion_retries ?? 3,
        firecrawlEnabled: feed.firecrawl_enabled,
        taggingEnabled: feed.tagging_enabled,
      }))

      feeds.value = mappedFeeds
    } else {
      error.value = response.error || 'Failed to fetch feeds'
    }

    loading.value = false
    return response
  }

  async function createFeed(data: {
    url: string
    category_id?: number
    title?: string
    description?: string
    icon?: string
    color?: string
  }) {
    loading.value = true
    const feedsApi = useFeedsApi()
    const response = await feedsApi.createFeed(data)
    loading.value = false

    if (response.success) {
      await fetchFeeds({ per_page: 10000 })
    }

    return response
  }

  async function deleteFeed(id: string) {
    loading.value = true
    const feedsApi = useFeedsApi()
    const response = await feedsApi.deleteFeed(Number(id))
    loading.value = false

    if (response.success) {
      await fetchFeeds({ per_page: 10000 })
    }

    return response
  }

  async function updateFeed(
    id: string,
    data: {
      url?: string
      title?: string
      description?: string
      category_id?: number | null
      icon?: string
      color?: string
      max_articles?: number
      refresh_interval?: number
      article_summary_enabled?: boolean
      completion_on_refresh?: boolean
      max_completion_retries?: number
      firecrawl_enabled?: boolean
      tagging_enabled?: boolean
    }
  ) {
    loading.value = true
    const feedsApi = useFeedsApi()
    const response = await feedsApi.updateFeed(Number(id), data)
    loading.value = false

    if (response.success) {
      await fetchFeeds({ per_page: 10000 })
    }

    return response
  }

  async function refreshFeed(id: string) {
    loading.value = true
    const feedsApi = useFeedsApi()
    const response = await feedsApi.refreshFeed(Number(id))
    loading.value = false
    if (!response.success) apiErrNotify(response, "刷新订阅源失败")
    return response
  }

  async function refreshAllFeeds() {
    loading.value = true
    const feedsApi = useFeedsApi()
    const response = await feedsApi.refreshAllFeeds()
    loading.value = false
    if (!response.success) apiErrNotify(response, "刷新所有订阅源失败")
    return response
  }



  // OPML
  async function importOpml(file: File) {
    loading.value = true
    const opmlApi = useOpmlApi()
    const response = await opmlApi.importOpml(file)
    loading.value = false

    if (response.success) {
      await fetchFeeds({ per_page: 10000 })
      await fetchCategories()
    } else {
      apiErrNotify(response, "导入 OPML 失败")
    }

    return response
  }

  async function exportOpml() {
    const opmlApi = useOpmlApi()
    return opmlApi.exportOpml()
  }
  // Initialize — 只负责基础数据，articlesStore 由具体消费者自行初始化
  async function initialize() {
    await Promise.all([
      fetchCategories(),
      fetchFeeds({ per_page: 10000 }),
    ])
  }

  return {
    loading,
    error,
    categories,
    feeds,

    fetchCategories,
    createCategory,
    updateCategory,
    deleteCategory,
    fetchFeeds,
    createFeed,
    updateFeed,
    deleteFeed,
    refreshFeed,
    refreshAllFeeds,

    importOpml,
    exportOpml,
    initialize,
  }
})

