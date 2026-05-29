import type { Article, ArticleFilters, PaginatedData } from '~/types'
import { useArticlesApi } from '~/api/articles'
import { normalizeArticle, type ArticlePayload } from '../utils/normalizeArticle'

export interface PaginationState {
  articles: Article[]
  page: number
  pageSize: number
  total: number
  hasMore: boolean
  loading: boolean
  error: string | null
}

export interface UseArticlePaginationOptions {
  pageSize?: number
}

export function useArticlePagination(options: UseArticlePaginationOptions = {}) {
  const articlesApi = useArticlesApi()
  const pageSize = options.pageSize ?? 20

  const state = reactive<PaginationState>({
    articles: [],
    page: 1,
    pageSize,
    total: 0,
    hasMore: false,
    loading: false,
    error: null,
  })

  const filters = ref<ArticleFilters>({})

  async function fetchFirstPage(newFilters: ArticleFilters = {}): Promise<void> {
    filters.value = { ...newFilters }
    state.articles = []
    state.page = 1
    state.hasMore = false
    state.error = null
    await loadPage()
  }

  async function loadMore(): Promise<void> {
    if (state.loading || !state.hasMore) return
    state.page++
    await loadPage()
  }

  async function loadPage(): Promise<void> {
    if (state.loading) return

    state.loading = true
    state.error = null

    const params: ArticleFilters = {
      ...filters.value,
      page: state.page,
      per_page: state.pageSize,
    }

    const response = await articlesApi.getArticles(params)

    if (response.success && response.data) {
      const rawData = response.data as unknown as PaginatedData<ArticlePayload>
      const rawArticles = (rawData.items || (response.data as unknown as ArticlePayload[])) as ArticlePayload[]
      const newArticles = rawArticles.map(normalizeArticle)

      if (state.page === 1) {
        state.articles = newArticles
      } else {
        state.articles.push(...newArticles)
      }

      state.total = response.pagination?.total ?? state.articles.length
      const pages = response.pagination?.pages ?? 1
      state.hasMore = state.page < pages
    } else {
      state.error = response.error ?? 'Failed to load articles'
    }

    state.loading = false
  }

  function reset(): void {
    state.articles = []
    state.page = 1
    state.total = 0
    state.hasMore = false
    state.loading = false
    state.error = null
    filters.value = {}
  }

  function updateArticle(id: string, updates: Partial<Article>): void {
    const index = state.articles.findIndex(a => a.id === id)
    if (index !== -1 && state.articles[index]) {
      Object.assign(state.articles[index], updates)
    }
  }

  function removeArticle(id: string): void {
    const index = state.articles.findIndex(a => a.id === id)
    if (index !== -1) {
      state.articles.splice(index, 1)
      state.total = Math.max(0, state.total - 1)
    }
  }

  return {
    state,
    filters,
    fetchFirstPage,
    loadMore,
    reset,
    updateArticle,
    removeArticle,
  }
}