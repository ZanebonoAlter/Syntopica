/**
 * 文章数据 normalizer
 * 将后端 snake_case ArticlePayload 转换为前端 camelCase Article 类型
 * 所有 feature 和 store 统一通过此函数转换 API 响应
 */

import type { AiDetectionSource, AiDetectionStatus, Article, ArticleDetection, ArticleTag } from '~/types'

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
  /** 标签描述（topic_tags.description）：归档标签卡 popover 的「{类别}描述」区用。 */
  description?: string
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
  /** 判定状态：unknown / suspect / not_ai */
  ai_detection_status?: string
  /** 判定置信分 0-1 */
  ai_detection_score?: number
  /** 判定来源：llm / manual */
  ai_detection_source?: string
  /** 判定时间 */
  ai_detection_at?: string
}

/**
 * 文章判定字段的原始载荷形态（列表行 / 详情 / 版块文章行共用）。
 * 取值用 unknown：版块文章行带索引签名，实际字段类型不可知，由 normalizer 收敛。
 */
export interface ArticleDetectionPayload {
  ai_detection_status?: unknown
  ai_detection_score?: unknown
  ai_detection_source?: unknown
  ai_detection_at?: unknown
}

const AI_DETECTION_STATUSES: readonly AiDetectionStatus[] = ['unknown', 'suspect', 'not_ai']
const AI_DETECTION_SOURCES: readonly AiDetectionSource[] = ['llm', 'manual']

/** 判定状态取值收敛：非枚举值一律视为「未判定」（不展示任何标记）。 */
export function normalizeAiDetectionStatus(value: unknown): AiDetectionStatus | undefined {
  return AI_DETECTION_STATUSES.includes(value as AiDetectionStatus) ? (value as AiDetectionStatus) : undefined
}

/** 判定来源取值收敛：非枚举值一律视为无来源。 */
export function normalizeAiDetectionSource(value: unknown): AiDetectionSource | undefined {
  return AI_DETECTION_SOURCES.includes(value as AiDetectionSource) ? (value as AiDetectionSource) : undefined
}

/**
 * 文章判定字段的 snake_case → camelCase 映射（add-ai-generated-article-detection）。
 * 映射只发生在 API/normalizer 层，组件层只见 camelCase 字段。
 */
export function normalizeArticleDetection(
  payload: ArticleDetectionPayload | null | undefined,
): ArticleDetection {
  return {
    aiDetectionStatus: normalizeAiDetectionStatus(payload?.ai_detection_status),
    aiDetectionScore: typeof payload?.ai_detection_score === 'number' ? payload.ai_detection_score : undefined,
    aiDetectionSource: normalizeAiDetectionSource(payload?.ai_detection_source),
    aiDetectionAt: typeof payload?.ai_detection_at === 'string' ? payload.ai_detection_at : undefined,
  }
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
      description: typeof tag.description === 'string' ? tag.description : undefined,
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
    ...normalizeArticleDetection(article),
  }
}
