import type { Category, RssFeed, Article, BulkUpdateArticlesData } from '~/types'
import { useCategoriesApi } from '~/api/categories'
import { useFeedsApi } from '~/api/feeds'
import { useArticlesApi } from '~/api/articles'
import { useOpmlApi } from '~/api/opml'
import { normalizeArticle } from '~/features/articles/utils/normalizeArticle'

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
    }

    return response
  }

  // Feeds
  const feeds = ref<RssFeed[]>([])
  const allFeeds = ref<RssFeed[]>([]) // Cache all feeds for sidebar display

  async function fetchFeeds(params: { page?: number; per_page?: number; category_id?: number; uncategorized?: boolean } = {}) {
    loading.value = true
    error.value = null

    const feedsApi = useFeedsApi()
    const response = await feedsApi.getFeeds(params)

    if (response.success && response.data) {
      const data = response.data as any
      const items = data.items || data

      const mappedFeeds = items.map((feed: any) => ({
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
        refreshInterval: feed.refresh_interval || 60,
        refreshStatus: feed.refresh_status || 'idle',
        refreshError: feed.refresh_error,
        lastRefreshAt: feed.last_refresh_at,
        aiSummaryEnabled: feed.ai_summary_enabled !== undefined ? feed.ai_summary_enabled : true, // Default to true if not set
        articleSummaryEnabled: feed.article_summary_enabled,
        completionOnRefresh: feed.completion_on_refresh,
        maxCompletionRetries: feed.max_completion_retries,
        firecrawlEnabled: feed.firecrawl_enabled,
        taggingEnabled: feed.tagging_enabled,
      }))

      feeds.value = mappedFeeds

      // If fetching without filters (except per_page), cache all feeds
      const hasFilters = params.category_id !== undefined || params.uncategorized === true
      if (!hasFilters) {
        allFeeds.value = mappedFeeds
      }
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
      await fetchArticles({ per_page: 10000 })
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
      await fetchArticles({ per_page: 10000 })
    }

    return response
  }

  async function updateFeed(
    id: string,
    data: {
      url?: string
      title?: string
      description?: string
      category_id?: number
      icon?: string
      color?: string
      max_articles?: number
      refresh_interval?: number
      ai_summary_enabled?: boolean
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
      await fetchArticles({ per_page: 10000 })
    }

    return response
  }

  async function refreshFeed(id: string) {
    loading.value = true
    const feedsApi = useFeedsApi()
    const response = await feedsApi.refreshFeed(Number(id))
    loading.value = false
    return response
  }

  async function refreshAllFeeds() {
    loading.value = true
    const feedsApi = useFeedsApi()
    const response = await feedsApi.refreshAllFeeds()
    loading.value = false
    return response
  }

  // Articles
  const articles = ref<Article[]>([])
  const totalArticles = ref(0)

  function syncFeedUnreadCount(feedId: string, updateCount: (current: number) => number) {
    const seen = new Set<RssFeed>()

    for (const collection of [feeds.value, allFeeds.value]) {
      const feed = collection.find(item => item.id === feedId)
      if (!feed || seen.has(feed)) {
        continue
      }

      feed.unreadCount = updateCount(feed.unreadCount ?? 0)
      seen.add(feed)
    }
  }

  function clearFeedUnreadCounts(matchFeed: (feed: RssFeed) => boolean) {
    const seen = new Set<RssFeed>()

    for (const collection of [feeds.value, allFeeds.value]) {
      for (const feed of collection) {
        if (!matchFeed(feed) || seen.has(feed)) {
          continue
        }

        feed.unreadCount = 0
        seen.add(feed)
      }
    }
  }

  async function fetchArticles(filters: {
    page?: number
    per_page?: number
    feed_id?: number
    category_id?: number
    uncategorized?: boolean
    read?: boolean
    favorite?: boolean
    search?: string
    start_date?: string
    end_date?: string
  } = {}) {
    loading.value = true
    error.value = null

    // Set a high default per_page to get all articles
    const params = { per_page: 10000, ...filters }

    const articlesApi = useArticlesApi()
    const response = await articlesApi.getArticles(params)

    if (response.success && response.data) {
      const data = response.data as any
      const items = data.items || data

      articles.value = items.map((article: any) => normalizeArticle(article))

      totalArticles.value = data.total || items.length
    } else {
      error.value = response.error || 'Failed to fetch articles'
    }

    loading.value = false
    return response
  }

  async function updateArticle(
    id: string,
    data: { read?: boolean; favorite?: boolean }
  ) {
    const articlesApi = useArticlesApi()
    const article = articles.value.find((a) => a.id === id)
    const previousRead = article?.read
    const wasFavorite = article?.favorite

    const response = await articlesApi.updateArticle(Number(id), data)

    if (response.success) {
      if (article) {
        Object.assign(article, data)
      }

      if (article && data.read !== undefined && previousRead !== undefined && previousRead !== data.read) {
        syncFeedUnreadCount(article.feedId, current => (data.read ? Math.max(0, current - 1) : current + 1))
      }

      if (data.favorite !== undefined && wasFavorite !== undefined && article) {
        article.favorite = data.favorite
      }
    }

    return response
  }

  async function toggleFavorite(id: string) {
    const article = articles.value.find((a) => a.id === id)
    if (article) {
      return updateArticle(id, { favorite: !article.favorite })
    }
    return { success: false, error: 'Article not found' }
  }

  async function markAsRead(id: string) {
    return updateArticle(id, { read: true })
  }

  async function markAllAsRead(options?: { feedId?: string; categoryId?: number; uncategorized?: boolean }) {
    const data: BulkUpdateArticlesData = { read: true }
    if (options?.feedId) {
      data.feed_id = Number(options.feedId)
    } else if (options?.categoryId) {
      data.category_id = options.categoryId
    } else if (options?.uncategorized) {
      data.uncategorized = true
    }

    const articlesApi = useArticlesApi()
    const response = await articlesApi.bulkUpdateArticles(data)

    if (response.success) {
      articles.value.forEach((a) => {
        if (!options) {
          a.read = true
        } else if (options.feedId && a.feedId === options.feedId) {
          a.read = true
        } else if (options.categoryId) {
          const feed = feeds.value.find(f => f.id === a.feedId)
          if (feed && Number(feed.category) === options.categoryId) {
            a.read = true
          }
        } else if (options.uncategorized) {
          const feed = feeds.value.find(f => f.id === a.feedId)
          if (feed && !feed.category) {
            a.read = true
          }
        }
      })

      if (!options) {
        clearFeedUnreadCounts(() => true)
      } else if (options.feedId) {
        clearFeedUnreadCounts(feed => feed.id === options.feedId)
      } else if (options.categoryId) {
        clearFeedUnreadCounts(feed => Number(feed.category) === options.categoryId)
      } else if (options.uncategorized) {
        clearFeedUnreadCounts(feed => !feed.category)
      }
    }

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
    }

    return response
  }

  async function exportOpml() {
    const opmlApi = useOpmlApi()
    return opmlApi.exportOpml()
  }

  async function fetchArticlesStats() {
    const articlesApi = useArticlesApi()
    const response = await articlesApi.getArticlesStats()
    return response
  }

  // Initialize
  async function initialize() {
    await Promise.all([
      fetchCategories(),
      fetchFeeds({ per_page: 10000 }),
      fetchArticles({ per_page: 10000 }),
    ])
  }

  return {
    loading,
    error,
    categories,
    feeds,
    allFeeds,
    articles,
    totalArticles,
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
    fetchArticles,
    updateArticle,
    toggleFavorite,
    markAsRead,
    markAllAsRead,
    importOpml,
    exportOpml,
    fetchArticlesStats,
    initialize,
  }
})


