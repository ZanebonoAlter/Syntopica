import type { RssFeed, Article, FeedResponse } from '~/types'
import { cleanHtml, extractFirstImage, getCategoryColor } from '~/utils/text'

export function useRssParser() {
  const loading = ref(false)
  const error = ref<string | null>(null)

  /**
   * Parse RSS feed from URL
   * Uses a CORS proxy to fetch RSS feeds
   */
  async function fetchFeed(url: string): Promise<FeedResponse | null> {
    loading.value = true
    error.value = null

    try {
      // Using RSS2JSON API as a CORS proxy
      const apiUrl = `https://api.rss2json.com/v1/api.json?rss_url=${encodeURIComponent(url)}`

      const response = await $fetch<FeedResponse>(apiUrl)

      if (response.status !== 'ok') {
        throw new Error('Failed to parse RSS feed')
      }

      return response
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch feed'
      console.error('Error fetching RSS feed:', e)
      return null
    } finally {
      loading.value = false
    }
  }

  /**
   * Convert RSS2JSON response to our Article format
   */
  function convertToArticles(feedId: string, feedData: FeedResponse): Article[] {
    return feedData.items.map((item, index) => ({
      id: `${feedId}-${index}`,
      feedId,
      title: item.title,
      description: cleanHtml(item.description || ''),
      content: item.content || item.description || '',
      link: item.link,
      pubDate: item.pubDate,
      author: item.author,
      category: feedId, // Will be updated by the caller
      read: false,
      favorite: false,
      imageUrl: item.thumbnail || item.enclosure?.link || extractFirstImage(item.content || ''),
    }))
  }

  /**
   * Create a new RssFeed object from RSS2JSON response
   */
  function createFeedFromResponse(
    url: string,
    categoryId: string,
    feedData: FeedResponse
  ): Omit<RssFeed, 'id' | 'articleCount'> {
    return {
      title: feedData.feed.title,
      description: feedData.feed.description || '',
      url,
      category: categoryId,
      icon: 'mdi:rss',
      color: getCategoryColor(categoryId),
      lastUpdated: new Date().toISOString(),
    }
  }

  return {
    loading: readonly(loading),
    error: readonly(error),
    fetchFeed,
    convertToArticles,
    createFeedFromResponse,
  }
}

