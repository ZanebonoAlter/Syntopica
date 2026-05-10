import type { Article, ArticleTag } from '~/types'

function normalizeArticleTags(tags: any[] | undefined): ArticleTag[] {
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

export function normalizeArticle(article: any): Article {
  return {
    id: String(article.id),
    feedId: String(article.feed_id),
    title: article.title,
    description: article.description || '',
    content: article.content || '',
    link: article.link,
    pubDate: article.pub_date || article.created_at || '',
    author: article.author,
    category: article.category_id ? String(article.category_id) : '',
    read: article.read || false,
    favorite: article.favorite || false,
    summaryStatus: article.summary_status,
    summaryGeneratedAt: article.summary_generated_at,
    completionAttempts: article.completion_attempts,
    completionError: article.completion_error,
    aiContentSummary: article.ai_content_summary,
    firecrawlStatus: article.firecrawl_status,
    firecrawlError: article.firecrawl_error,
    firecrawlContent: article.firecrawl_content,
    firecrawlCrawledAt: article.firecrawl_crawled_at,
    imageUrl: article.image_url,
    tagCount: typeof article.tag_count === 'number' ? article.tag_count : undefined,
    tags: normalizeArticleTags(article.tags),
  }
}
