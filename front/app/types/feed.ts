/**
 * Feed-related type definitions.
 */

/**
 * Response shape returned by rss2json.com used by server/api/fetch-feed.post.ts.
 */
export interface FeedResponse {
  status: 'ok' | 'error'
  feed?: {
    url?: string
    title?: string
    link?: string
    author?: string
    description?: string
    image?: string
  }
  items?: Array<{
    title?: string
    pubDate?: string
    link?: string
    guid?: string
    author?: string
    thumbnail?: string
    description?: string
    content?: string
    enclosure?: unknown
    categories?: string[]
  }>
  message?: string
}

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
  icon_source?: 'auto' | 'custom' | 'fallback'
  color?: string
  lastUpdated: string
  articleCount: number
  unreadCount?: number
  maxArticles?: number
  refreshInterval?: number
  refreshStatus?: 'idle' | 'refreshing' | 'success' | 'error'
  refreshError?: string
  lastRefreshAt?: string
  articleSummaryEnabled?: boolean
  completionOnRefresh?: boolean
  maxCompletionRetries?: number
  firecrawlEnabled?: boolean
  taggingEnabled?: boolean
}

/**
 * 单源窗口内命中板块分布项（add-source-board-hit-rate）。
 * 同一文章命中多个板块时各计一次，故 Σarticles 可能大于该源 in_board。
 */
export interface FeedBoardHitBoard {
  board_id: number
  label: string
  articles: number
}

/**
 * 单订阅源在统计窗口内的入板块命中统计（add-source-board-hit-rate）。
 * 字段名与后端 sourcestats.FeedStat 契约严格一致（snake_case）。
 * 口径（唯一权威：openspec/specs/source-board-hit-rate/spec.md）：
 * 窗口 coalesce(pub_date, created_at) >= now - N 天、含已归档文章、按文章去重。
 */
export interface FeedBoardHitStats {
  feed_id: number
  title: string
  tagging_enabled: boolean
  /** 窗口内文章总数（含已归档） */
  articles: number
  /** 命中文章数（去重） */
  in_board: number
  /** 有标签但未命中板块 */
  tagged_no_board: number
  /** 无标签且打标任务排队中 */
  untagged_pending: number
  /** 无标签且无未完成任务（含从未入队） */
  untagged_settled: number
  /** in_board / articles，articles=0 时为 0 */
  hit_rate: number
  /** 命中板块分布，空时为 [] 非 null */
  boards: FeedBoardHitBoard[]
}

/**
 * 板块来源构成响应（add-source-board-hit-rate）。
 * GET /api/semantic-boards/:id/source-breakdown 的 data 段；后端默认按篇数降序。
 */
export interface BoardSourceBreakdownSource {
  feed_id: number
  title: string
  /** 该源在本板块的命中篇数（去重） */
  articles: number
  /** articles / total_articles，total_articles=0 时为 0 */
  share: number
  /** 该源窗口内文章总量（上下文列：识别只偶尔命中的源） */
  feed_articles: number
  /** 该源整体入板块率（上下文列） */
  feed_hit_rate: number
}

export interface BoardSourceBreakdown {
  /** 该板块窗口内命中文章总数（按文章去重） */
  total_articles: number
  source_count: number
  sources: BoardSourceBreakdownSource[]
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
  category_id?: number | null
  title?: string
  description?: string
  icon?: string
  color?: string
  max_articles?: number
  refresh_interval?: number
  refresh_status?: string
  refresh_error?: string
  last_refresh_at?: string
  article_summary_enabled?: boolean
  completion_on_refresh?: boolean
  max_completion_retries?: number
  firecrawl_enabled?: boolean
  tagging_enabled?: boolean
}
