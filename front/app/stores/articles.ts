import { defineStore } from 'pinia'
import type { Article, FilterState } from '~/types'

export const useArticlesStore = defineStore('articles', () => {
  const apiStore = useApiStore()

  const articles = computed<Article[]>(() => apiStore.articles)
  const filters = ref<FilterState>({
    sort: 'latest',
    filter: 'all',
    category: null,
    search: ''
  })
  const loading = computed(() => apiStore.loading)
  const currentArticle = ref<Article | null>(null)

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

  function markAsRead(id: string) {
    const article = apiStore.articles.find(a => a.id === id)
    if (article) {
      article.read = true
    }
  }

  function markAllAsRead(options?: { feedId?: string; categoryId?: number; uncategorized?: boolean }) {
    apiStore.articles.forEach((article) => {
      if (!options) {
        article.read = true
      } else if (options.feedId && article.feedId === options.feedId) {
        article.read = true
      } else if (options.categoryId) {
        const feed = apiStore.feeds.find(f => f.id === article.feedId)
        if (feed && Number(feed.category) === options.categoryId) {
          article.read = true
        }
      } else if (options.uncategorized) {
        const feed = apiStore.feeds.find(f => f.id === article.feedId)
        if (feed && !feed.category) {
          article.read = true
        }
      }
    })
  }

  function toggleFavorite(id: string) {
    const article = apiStore.articles.find(a => a.id === id)
    if (article) {
      article.favorite = !article.favorite
    }
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

  function setCurrentArticle(article: Article | null) {
    currentArticle.value = article
    if (article && !article.read) {
      markAsRead(article.id)
    }
  }

  return {
    articles,
    filters,
    loading,
    currentArticle,
    filteredArticles,
    unreadCount,
    favoriteCount,
    articlesByFeed,
    unreadCountByFeed,
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

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useArticlesStore, import.meta.hot))
}
