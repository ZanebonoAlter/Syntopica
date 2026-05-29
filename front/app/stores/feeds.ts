import { defineStore } from 'pinia'
import type { RssFeed, Category } from '~/types'

export const useFeedsStore = defineStore('feeds', () => {
  const apiStore = useApiStore()

  const feeds = computed<RssFeed[]>(() => (
    apiStore.allFeeds.length > 0 ? apiStore.allFeeds : apiStore.feeds
  ))

  const categories = computed<Category[]>(() => apiStore.categories)

  const feedCount = computed(() => feeds.value.length)

  const categorizedFeeds = computed(() => {
    const grouped: Record<string, RssFeed[]> = {}
    feeds.value.forEach((feed) => {
      if (!grouped[feed.category]) {
        grouped[feed.category] = []
      }
      grouped[feed.category]?.push(feed)
    })
    return grouped
  })

  const getFeedUnreadCount = (feedId: string) => {
    const feed = feeds.value.find(f => f.id === feedId)
    return feed?.unreadCount || 0
  }

  const unreadCountsByFeed = computed(() => {
    const counts: Record<string, number> = {}
    feeds.value.forEach((feed) => {
      counts[feed.id] = feed.unreadCount || 0
    })
    return counts
  })

  function getFeedsByCategory(categoryId: string) {
    return feeds.value.filter(f => f.category === categoryId)
  }

  function getCategoryBySlug(slug: string) {
    return categories.value.find(c => c.slug === slug)
  }

  return {
    feeds,
    categories,
    feedCount,
    categorizedFeeds,
    getFeedUnreadCount,
    unreadCountsByFeed,
    getFeedsByCategory,
    getCategoryBySlug,
  }
})

if (import.meta.hot) {
  import.meta.hot.accept(acceptHMRUpdate(useFeedsStore, import.meta.hot))
}
