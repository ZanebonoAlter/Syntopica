/**
 * Feed-related type definitions.
 */

/**
 * RSS feed data model.
 */
export interface RssFeed {
  id: string
  title: string
  description: string
  url: string
  category: string
  icon?: string
  color?: string
  lastUpdated: string
  articleCount: number
  unreadCount?: number
  maxArticles?: number
  refreshInterval?: number
  refreshStatus?: 'idle' | 'refreshing' | 'success' | 'error'
  refreshError?: string
  lastRefreshAt?: string
  aiSummaryEnabled?: boolean
  articleSummaryEnabled?: boolean
  completionOnRefresh?: boolean
  maxCompletionRetries?: number
  firecrawlEnabled?: boolean
  taggingEnabled?: boolean
}

/**
 * Payload for creating a feed.
 */
export interface CreateFeedData {
  url: string
  category_id?: number
  title?: string
  description?: string
  icon?: string
  color?: string
}

/**
 * Payload for updating a feed.
 */
export interface UpdateFeedData {
  url?: string
  category_id?: number
  title?: string
  description?: string
  icon?: string
  color?: string
  max_articles?: number
  refresh_interval?: number
  refresh_status?: string
  refresh_error?: string
  last_refresh_at?: string
  ai_summary_enabled?: boolean
  article_summary_enabled?: boolean
  completion_on_refresh?: boolean
  max_completion_retries?: number
  firecrawl_enabled?: boolean
  tagging_enabled?: boolean
}

/**
 * RSS fetch response data.
 */
export interface FeedResponse {
  status: string
  feed: {
    title: string
    description: string
    image?: string
  }
  items: FeedItem[]
}

/**
 * RSS item entry.
 */
export interface FeedItem {
  title: string
  link: string
  pubDate: string
  description?: string
  content?: string
  author?: string
  thumbnail?: string
  enclosure?: {
    link: string
  }
}
