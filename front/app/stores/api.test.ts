import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, defineStore, setActivePinia } from 'pinia'
import { ref } from 'vue'

import type { Article, RssFeed } from '~/types'

const updateArticleMock = vi.fn()
const bulkUpdateArticlesMock = vi.fn()

vi.mock('~/api/categories', () => ({
  useCategoriesApi: () => ({}),
}))

vi.mock('~/api/feeds', () => ({
  useFeedsApi: () => ({}),
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
}

testGlobals.defineStore = defineStore
testGlobals.ref = ref

async function createStore() {
  const { useApiStore } = await import('./api')
  return useApiStore()
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
  })

  it('updateArticle refreshes unread count when marking an article as read', async () => {
    const store = await createStore()
    const sidebarFeed = createFeed({ unreadCount: 2 })
    const filteredFeed = createFeed({ unreadCount: 2 })

    store.allFeeds = [sidebarFeed]
    store.feeds = [filteredFeed]
    store.articles = [createArticle({ read: false })]

    updateArticleMock.mockResolvedValue({ success: true })

    await store.updateArticle('1', { read: true })

    expect(updateArticleMock).toHaveBeenCalledWith(1, { read: true })
    expect(store.articles[0]?.read).toBe(true)
    expect(store.allFeeds[0]?.unreadCount).toBe(1)
    expect(store.feeds[0]?.unreadCount).toBe(1)
  })

  it('markAllAsRead refreshes all feed unread counts including uncategorized', async () => {
    const store = await createStore()
    const sidebarFeeds = [
      createFeed({ id: '1', category: 'cat-1', unreadCount: 3 }),
      createFeed({ id: '2', category: '', unreadCount: 4 }),
    ]
    const filteredFeeds = [
      createFeed({ id: '1', category: 'cat-1', unreadCount: 3 }),
      createFeed({ id: '2', category: '', unreadCount: 4 }),
    ]

    store.allFeeds = sidebarFeeds
    store.feeds = filteredFeeds
    store.articles = [
      createArticle({ id: '1', feedId: '1', read: false }),
      createArticle({ id: '2', feedId: '2', read: false }),
    ]

    bulkUpdateArticlesMock.mockResolvedValue({ success: true })

    await store.markAllAsRead()

    expect(bulkUpdateArticlesMock).toHaveBeenCalledWith({ read: true })
    expect(store.articles.every(article => article.read)).toBe(true)
    expect(store.allFeeds.map(feed => feed.unreadCount)).toEqual([0, 0])
    expect(store.feeds.map(feed => feed.unreadCount)).toEqual([0, 0])
  })
})
