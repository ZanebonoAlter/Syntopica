/**
 * Timeline type definitions for topic graph feature.
 */

import type { TopicCategory } from '~/api/topicGraph'

export type DateRangeFilter = 'today' | 'week' | 'month' | 'custom' | null

export interface TimelineFilters {
  dateRange: DateRangeFilter
  startDate?: string
  endDate?: string
  sources: string[]
}

export interface TimelineArticleTag {
  slug: string
  label: string
  category: TopicCategory
}

export interface TimelineDigestSourceArticle {
  id: number
  title: string
  link: string
  feedName?: string
  feedIcon?: string
  feedColor?: string
}

export interface TimelineDigest {
  id: string
  title: string
  summary: string
  createdAt: string
  feedName: string
  feedIcon?: string
  feedColor?: string
  categoryName: string
  articleCount: number
  tags: TimelineArticleTag[]
  articles: TimelineDigestSourceArticle[]
}

export interface TimelineDigestSelection extends TimelineDigest {
  matchedArticleIds: number[]
  matchedArticlesTags?: TimelineArticleTag[]
}

export interface PendingArticle {
  id: number
  title: string
  link: string
  pubDate?: string
  feedName: string
  feedIcon?: string
  feedColor?: string
}

export interface PendingArticlesResponse {
  articles: PendingArticle[]
  total: number
}

export type TimelineAggregationMode = 'day' | 'hour'

export interface TimelineAggregationGroup {
  key: string
  label: string
  startDate: Date
  endDate: Date
  articles: TimelineAggregationArticle[]
}

export interface TimelineAggregationArticle {
  id: string
  title: string
  link: string
  pubDate: string
  feedName: string
  feedIcon?: string
  feedColor?: string
  tags: Array<{
    slug: string
    label: string
    category: string
  }>
}
