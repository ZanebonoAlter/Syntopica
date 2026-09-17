import { defineStore } from 'pinia'
import { useArticlesApi } from '~/api/articles'
import type { Article, ArticleFilters, FilterState, UpdateArticleData } from '~/types'
import { useNotify } from '~/composables/useNotify'
import { useFeedsStore } from '~/stores/feeds'
import { normalizeArticle, type ArticlePayload } from '~/api/normalizers/article'
interface ArticlesResponse {
  items?: ArticlePayload[]
  total?: number
  pagination?: { total?: number }
}

export const useArticlesStore = defineStore('articles', () => {
  // ---- 自有状态 ----
  const articles = ref<Article[]>([])
  const totalArticles = ref(0)
  const loading = ref(false)

  const filters = ref<FilterState>({
    sort: 'latest',
    filter: 'all',
    category: null,
    search: ''
  })
  const currentArticle = ref<Article | null>(null)

  // ---- API 方法 ----
  const articlesApi = useArticlesApi()

  async function fetchArticles(filters?: ArticleFilters) {
    loading.value = true
    const response = await articlesApi.getArticles(filters ?? {})
    if (response.success && response.data) {
      const data = response.data as unknown as ArticlePayload[] | ArticlesResponse
      const items = Array.isArray(data) ? data : (data as ArticlesResponse).items ?? []
      articles.value = items.map((item: ArticlePayload) => normalizeArticle(item))
      totalArticles.value = (data as ArticlesResponse).total ?? items.length
    }
    loading.value = false
    return response
  }

  async function updateArticle(id: string, data: UpdateArticleData) {
    const response = await articlesApi.updateArticle(Number(id), data)
    if (response.success) {
      const existing = articles.value.find(a => a.id === id)
      if (existing) {
        Object.assign(existing, data)
      }
    }
    return response
  }

  async function fetchArticlesStats() {
    return articlesApi.getArticlesStats()
  }

  // ---- 派生状态 ----
  const filteredArticles = computed(() => {
    let result = [...articles.value]

    if (filters.value.category) {
      result = result.filter(a => a.category === filters.value.category)
    }

    if (filters.value.filter === 'unread') {
      result = result.filter(a => !a.read)
    } else if (filters.value.filter === 'favorites') {
      result = result.filter(a => a.favorite)
    }

    if (filters.value.search) {
      const searchLower = filters.value.search.toLowerCase()
      result = result.filter(a =>
        a.title.toLowerCase().includes(searchLower) ||
        a.description.toLowerCase().includes(searchLower)
      )
    }

    if (filters.value.sort === 'latest') {
      result.sort((a, b) => new Date(b.pubDate).getTime() - new Date(a.pubDate).getTime())
    } else if (filters.value.sort === 'popular') {
      result.sort((a, b) => b.title.localeCompare(a.title))
    } else if (filters.value.sort === 'unread') {
      result.sort((a, b) => Number(a.read) - Number(b.read))
    }

    return result
  })

  const unreadCount = computed(() => articles.value.filter(a => !a.read).length)
  const favoriteCount = computed(() => articles.value.filter(a => a.favorite).length)

  const articlesByFeed = computed(() => {
    const grouped: Record<string, Article[]> = {}
    articles.value.forEach((article) => {
      if (!grouped[article.feedId]) {
        grouped[article.feedId] = []
      }
      grouped[article.feedId]!.push(article)
    })
    return grouped
  })

  const unreadCountByFeed = computed(() => {
    const grouped: Record<string, number> = {}
    articles.value.forEach((article) => {
      if (!article.read) {
        if (!grouped[article.feedId]) {
          grouped[article.feedId] = 0
        }
        grouped[article.feedId]!++
      }
    })
    return grouped
  })

  /**
   * 标记已读 — 经 API 持久化 + 乐观更新回滚
   */
  async function markAsRead(id: string) {
    const article = articles.value.find(a => a.id === id)
    if (!article || article.read) return { success: true }

    article.read = true // 乐观更新

    const response = await articlesApi.updateArticle(Number(id), { read: true })
    if (!response.success) {
      article.read = false // 回滚
      useNotify().error('标记已读失败')
    }

    return response
  }

  /**
   * 批量标记已读 — 经 API 持久化 + 乐观更新回滚
   */
  async function markAllAsRead(options?: { feedId?: string; categoryId?: number; uncategorized?: boolean }) {
    const snapshots = new Map<string, boolean>()
    articles.value.forEach((article) => {
      let shouldMark = false
      if (!options) {
        shouldMark = true
      } else if (options.feedId && article.feedId === options.feedId) {
        shouldMark = true
      } else if (options.categoryId) {
        if (Number(article.category) === options.categoryId) {
          shouldMark = true
        }
      } else if (options.uncategorized) {
        if (!article.category) {
          shouldMark = true
        }
      }
      if (shouldMark) {
        snapshots.set(article.id, article.read ?? false)
        article.read = true
      }
    })

    const data: import('~/types').BulkUpdateArticlesData = { read: true }
    if (!options) {
      // 全站标记必须显式 all（后端拒绝无 scope 请求）
      data.all = true
    }
    if (options?.feedId) {
      data.feed_id = Number(options.feedId)
    } else if (options?.categoryId) {
      data.category_id = options.categoryId
    } else if (options?.uncategorized) {
      data.uncategorized = true
    }

    const response = await articlesApi.bulkUpdateArticles(data)
    if (!response.success) {
      for (const [id, wasRead] of snapshots) {
        const a = articles.value.find(art => art.id === id)
        if (a) a.read = wasRead
      }
      useNotify().error('批量标记已读失败')
      return response
    }

    // 成功后通过 feedsStore 的小 Interface 同步 feed unread count
    const feedsStore = useFeedsStore()
    if (!options) {
      feedsStore.clearUnreadCounts(() => true)
    } else if (options.feedId) {
      feedsStore.clearUnreadCounts(feed => feed.id === options.feedId)
    } else if (options.categoryId) {
      feedsStore.clearUnreadCounts(feed => Number(feed.category) === options.categoryId)
    } else if (options.uncategorized) {
      feedsStore.clearUnreadCounts(feed => !feed.category)
    }

    return response
  }

  /**
   * 切换收藏 — 经 API 持久化 + 乐观更新回滚
   */
  async function toggleFavorite(id: string) {
    const article = articles.value.find(a => a.id === id)
    if (!article) return { success: false, error: 'Article not found' }

    const wasFavorite = article.favorite
    article.favorite = !wasFavorite

    const response = await articlesApi.updateArticle(Number(id), { favorite: article.favorite })
    if (!response.success) {
      article.favorite = wasFavorite
      useNotify().error('收藏操作失败')
    }

    return response
  }

  function updateFilters(newFilters: Partial<FilterState>) {
    filters.value = { ...filters.value, ...newFilters }
  }

  function resetFilters() {
    filters.value = {
      sort: 'latest',
      filter: 'all',
      category: null,
      search: ''
    }
  }

  function getArticleById(id: string) {
    return articles.value.find(a => a.id === id)
  }

  function getArticlesByFeed(feedId: string) {
    return articles.value.filter(a => a.feedId === feedId)
  }

  async function setCurrentArticle(article: Article | null) {
    currentArticle.value = article
    if (article && !article.read) {
      await markAsRead(article.id)
    }
  }

  return {
    articles,
    totalArticles,
    loading,
    filters,
    currentArticle,
    filteredArticles,
    unreadCount,
    favoriteCount,
    articlesByFeed,
    unreadCountByFeed,
    fetchArticles,
    updateArticle,
    fetchArticlesStats,
    markAsRead,
    markAllAsRead,
    toggleFavorite,
    updateFilters,
    resetFilters,
    getArticleById,
    getArticlesByFeed,
    setCurrentArticle,
  }
})

if (import.meta.hot && typeof acceptHMRUpdate !== 'undefined') {
  import.meta.hot.accept(acceptHMRUpdate(useArticlesStore, import.meta.hot))
}
