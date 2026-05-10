/**
 * Article type definitions.
 */

export interface Article {
  id: string
  feedId: string
  title: string
  description: string
  content: string
  link: string
  pubDate: string
  author?: string
  category: string
  read?: boolean
  favorite?: boolean
  summaryStatus?: 'complete' | 'incomplete' | 'pending' | 'failed'
  summaryGeneratedAt?: string
  completionAttempts?: number
  completionError?: string
  aiContentSummary?: string
  firecrawlStatus?: 'pending' | 'processing' | 'completed' | 'failed'
  firecrawlError?: string
  firecrawlContent?: string
  firecrawlCrawledAt?: string
  imageUrl?: string
  tagCount?: number
  tags?: ArticleTag[]
}

export interface ArticleTag {
  id?: number
  slug: string
  label: string
  category: string
  kind?: string
  icon?: string
  score?: number
  articleCount?: number
  isWatched?: boolean
}

export interface ArticleFilters {
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
  watched_tag_ids?: string
  watched_tags?: boolean
  sort_by?: 'relevance' | 'date'
}

export interface UpdateArticleData {
  read?: boolean
  favorite?: boolean
}

export interface BulkUpdateArticlesData {
  ids?: number[]
  feed_id?: number
  category_id?: number
  uncategorized?: boolean
  read?: boolean
  favorite?: boolean
}
