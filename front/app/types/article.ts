/**
 * Article type definitions.
 */

export interface Article {
  id: string
  feedId: string
  title: string
  /** 列表窄投影下为空串（列表不携带正文类字段）；完整值仅在详情接口返回 */
  description: string
  /** 列表窄投影下为空串（列表不携带正文类字段）；完整值仅在详情接口返回 */
  content: string
  /** 列表窄投影导语（后端去标签纯文本，≤200 字符）；详情返回后由 description 优先 */
  excerpt?: string
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
  /** 列表窄投影下缺省；完整值仅在详情接口返回 */
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
  concept_id?: number
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
  /** 显式全站 scope（fix-bulk-markall-all-scope）：与 ids/feed_id/category_id/uncategorized 互斥 */
  all?: boolean
  read?: boolean
  favorite?: boolean
}
