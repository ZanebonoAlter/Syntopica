/**
 * Article type definitions.
 */

/**
 * 文章级 AI 生成疑似度判定状态（add-ai-generated-article-detection design D14）。
 * - unknown：未判定（未进判定范围 / 判定未完成）——界面不展示任何标记
 * - suspect：疑似 AI 生成——界面展示「疑似 AI 生成」标记
 * - not_ai：已确认非 AI——界面不展示任何标记
 */
export type AiDetectionStatus = 'unknown' | 'suspect' | 'not_ai'

/** 判定来源：llm=自动判定 / manual=人工标记（人工结论优先，不被自动判定覆盖）。 */
export type AiDetectionSource = 'llm' | 'manual'

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
  /** 判定状态：unknown=未判定 / suspect=疑似 AI 生成 / not_ai=已确认非 AI（仅 suspect 展示标记） */
  aiDetectionStatus?: AiDetectionStatus
  /** 判定置信分 0-1（未判定为 0） */
  aiDetectionScore?: number
  /** 判定来源：llm=自动判定 / manual=人工标记 */
  aiDetectionSource?: AiDetectionSource
  /** 判定时间 */
  aiDetectionAt?: string
}

/**
 * 文章判定字段的 camelCase 视图（列表行 / 版块文章行 / 日报线索共用同一组字段）。
 */
export type ArticleDetection = Pick<
  Article,
  'aiDetectionStatus' | 'aiDetectionScore' | 'aiDetectionSource' | 'aiDetectionAt'
>

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
  /** 标签描述（topic_tags.description）：归档标签卡 popover 的「{类别}描述」区用。 */
  description?: string
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
  /** 人工标记/取消疑似 AI：suspect=标记疑似、not_ai=取消标记（须配合 ai_detection_source=manual） */
  ai_detection_status?: AiDetectionStatus
  /** 判定来源：manual=人工标记（人工结论不被后续自动判定覆盖） */
  ai_detection_source?: AiDetectionSource
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
