/**
 * 文章数据 normalizer
 * 将后端 snake_case ArticlePayload 转换为前端 camelCase Article 类型
 * 所有 feature 和 store 统一通过此函数转换 API 响应
 */

import type { Article, ArticleTag } from '~/types'

export interface ArticleTagPayload {
  id?: number
  slug: string
  label: string
  category: string
  kind?: string
  icon?: string
  score?: number
  article_count?: number
  articleCount?: number
  is_watched?: boolean
}

export interface ArticlePayload {
  id: number
  feed_id: number
  title: string
  /** 列表窄投影下缺省；详情接口返回完整值 */
  description?: string
  /** 列表窄投影下缺省；详情接口返回完整值 */
  content?: string
  /** 列表窄投影导语（后端去标签纯文本，≤200 字符） */
  excerpt?: string
  link: string
  pub_date: string
  created_at: string
  author?: string
  category_id?: number
  read: boolean
  favorite: boolean
  summary_status?: string
  summary_generated_at?: string
  completion_attempts?: number
  completion_error?: string
  ai_content_summary?: string
  firecrawl_status?: string
  firecrawl_error?: string
  /** 列表窄投影下缺省；详情接口返回完整值 */
  firecrawl_content?: string
  firecrawl_crawled_at?: string
  image_url?: string
  tag_count?: number
  tags?: ArticleTagPayload[]
}

function normalizeArticleTags(tags: ArticleTagPayload[] | undefined): ArticleTag[] {
  if (!Array.isArray(tags)) return []

  return tags
    .filter(tag => tag && typeof tag.slug === 'string' && typeof tag.label === 'string')
    .map(tag => ({
      id: typeof tag.id === 'number' ? tag.id : undefined,
      slug: tag.slug,
      label: tag.label,
      category: tag.category || 'keyword',
      kind: tag.kind,
      icon: tag.icon,
      score: typeof tag.score === 'number' ? tag.score : undefined,
      articleCount: typeof tag.article_count === 'number'
        ? tag.article_count
        : typeof tag.articleCount === 'number'
          ? tag.articleCount
          : undefined,
      isWatched: typeof tag.is_watched === 'boolean' ? tag.is_watched : undefined,
    }))
}

export function normalizeArticle(article: ArticlePayload): Article {
  return {
    id: String(article.id),
    feedId: String(article.feed_id),
    title: article.title,
    description: article.description || '',
    content: article.content || '',
    excerpt: article.excerpt || '',
    link: article.link,
    pubDate: article.pub_date || article.created_at || '',
    author: article.author,
    category: article.category_id ? String(article.category_id) : '',
    read: article.read || false,
    favorite: article.favorite || false,
    summaryStatus: article.summary_status as Article['summaryStatus'],
    summaryGeneratedAt: article.summary_generated_at,
    completionAttempts: article.completion_attempts,
    completionError: article.completion_error,
    aiContentSummary: article.ai_content_summary,
    firecrawlStatus: article.firecrawl_status as Article['firecrawlStatus'],
    firecrawlError: article.firecrawl_error,
    firecrawlContent: article.firecrawl_content,
    firecrawlCrawledAt: article.firecrawl_crawled_at,
    imageUrl: article.image_url,
    tagCount: typeof article.tag_count === 'number' ? article.tag_count : undefined,
    tags: normalizeArticleTags(article.tags),
  }
}
