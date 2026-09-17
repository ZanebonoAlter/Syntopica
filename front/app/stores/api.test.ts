import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, defineStore, setActivePinia } from 'pinia'
import { ref, computed } from 'vue'
import { acceptHMRUpdate } from 'pinia'

import type { Article, RssFeed } from '~/types'

const updateArticleMock = vi.fn()
const bulkUpdateArticlesMock = vi.fn()
const getFeedsMock = vi.fn()

vi.mock('~/api/categories', () => ({
  useCategoriesApi: () => ({}),
}))

vi.mock('~/api/feeds', () => ({
  useFeedsApi: () => ({
    getFeeds: getFeedsMock,
  }),
}))

vi.mock('~/api/articles', () => ({
  useArticlesApi: () => ({
    updateArticle: updateArticleMock,
    bulkUpdateArticles: bulkUpdateArticlesMock,
  }),
}))

vi.mock('~/api/opml', () => ({
  useOpmlApi: () => ({}),
}))

const testGlobals = globalThis as typeof globalThis & {
  defineStore: typeof defineStore
  ref: typeof ref
  computed: typeof computed
  acceptHMRUpdate: typeof acceptHMRUpdate
}

testGlobals.defineStore = defineStore
testGlobals.ref = ref
testGlobals.computed = computed
testGlobals.acceptHMRUpdate = acceptHMRUpdate

async function createStores() {
  const { useApiStore } = await import('./api')
  const { useArticlesStore } = await import('./articles')
  const apiStore = useApiStore()
  const articlesStore = useArticlesStore()
  return { apiStore, articlesStore }
}

function createFeed(overrides: Partial<RssFeed> = {}): RssFeed {
  return {
    id: '1',
    title: 'Feed',
    description: '',
    url: 'https://example.com/feed.xml',
    category: 'cat-1',
    lastUpdated: '2026-04-11T00:00:00Z',
    articleCount: 3,
    unreadCount: 2,
    ...overrides,
  }
}

function createArticle(overrides: Partial<Article> = {}): Article {
  return {
    id: '1',
    feedId: '1',
    title: 'Article',
    description: '',
    content: '',
    link: 'https://example.com/article',
    pubDate: '2026-04-11T00:00:00Z',
    category: '',
    read: false,
    favorite: false,
    ...overrides,
  }
}

describe('useApiStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    updateArticleMock.mockReset()
    bulkUpdateArticlesMock.mockReset()
    getFeedsMock.mockReset()
  })

  it('fetchFeeds maps article_summary_enabled to the AI summary toggle', async () => {
    const { apiStore } = await createStores()
    getFeedsMock.mockResolvedValue({
      success: true,
      data: [{ id: 7, title: 'F', url: 'https://example.com/f', article_summary_enabled: false }],
    })

    await apiStore.fetchFeeds()

    expect(apiStore.feeds).toHaveLength(1)
    expect(apiStore.feeds[0]!.articleSummaryEnabled).toBe(false)
    // 死字段 aiSummaryEnabled 不得再出现在映射产物里（后端从不返回 ai_summary_enabled）
    expect('aiSummaryEnabled' in apiStore.feeds[0]!).toBe(false)
  })

  it('fetchFeeds defaults AI summary toggle to false when field missing', async () => {
    const { apiStore } = await createStores()
    getFeedsMock.mockResolvedValue({
      success: true,
      data: [{ id: 8, title: 'F2', url: 'https://example.com/f2' }],
    })

    await apiStore.fetchFeeds()

    expect(apiStore.feeds[0]!.articleSummaryEnabled).toBe(false)
  })

  it('markAllAsRead via articlesStore clears apiStore feed unread counts', async () => {
    const { apiStore, articlesStore } = await createStores()
    const testFeeds = [
      createFeed({ id: '1', category: 'cat-1', unreadCount: 3 }),
      createFeed({ id: '2', category: '', unreadCount: 4 }),
    ]

    apiStore.feeds = testFeeds

    bulkUpdateArticlesMock.mockResolvedValue({ success: true })

    await articlesStore.markAllAsRead()

    expect(bulkUpdateArticlesMock).toHaveBeenCalledWith({ read: true, all: true })
    expect(apiStore.feeds.map(feed => feed.unreadCount)).toEqual([0, 0])
  })

  it('toggleFavorite calls updateArticle with favorite toggle', async () => {
    const { articlesStore } = await createStores()
    articlesStore.articles = [createArticle({ id: '1', favorite: false })]

    updateArticleMock.mockResolvedValue({ success: true })

    const result = await articlesStore.toggleFavorite('1')
    expect(result.success).toBe(true)
    expect(updateArticleMock).toHaveBeenCalledWith(1, { favorite: true })
  })
})
