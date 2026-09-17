import type { Article, RssFeed } from '~/types'

export type StatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger'

export interface PipelineLine {
  label: string
  value: string
  tone: StatusTone
}

/** 行尾单图标四态：失败 > 进行中 > 排队 > 完成（见 change declutter-article-list-panel design D3） */
export type PipelineState = 'queued' | 'processing' | 'failed' | 'done'

export function getArticlePipelineState(article: Article): PipelineState {
  const statuses = [article.firecrawlStatus, article.summaryStatus]
  if (statuses.includes('failed')) return 'failed'
  if (article.firecrawlStatus === 'processing' || article.summaryStatus === 'pending') return 'processing'
  if (article.firecrawlStatus === 'pending' || article.summaryStatus === 'incomplete') return 'queued'
  return 'done'
}

export interface PipelineStateMeta {
  icon: string
  spinning: boolean
  /** 状态图标颜色（语义 token 名） */
  colorToken: 'warning' | 'info' | 'error' | 'muted'
  title: string
}

export function getPipelineStateMeta(state: PipelineState): PipelineStateMeta {
  switch (state) {
    case 'failed':
      return { icon: 'mdi:alert-circle', spinning: false, colorToken: 'error', title: '处理状态：失败' }
    case 'processing':
      return { icon: 'mdi:loading', spinning: true, colorToken: 'info', title: '处理状态：进行中' }
    case 'queued':
      return { icon: 'mdi:clock-outline', spinning: false, colorToken: 'warning', title: '处理状态：排队中' }
    case 'done':
    default:
      return { icon: 'mdi:check-circle', spinning: false, colorToken: 'muted', title: '处理状态：完成' }
  }
}

export interface ProcessingStatusMeta {
  label: string
  icon: string
  tone: StatusTone
  hint?: string
}

function toPlainText(input?: string): string {
  if (!input) return ''

  return input
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/`([^`]*)`/g, '$1')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, ' ')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/[#>*_~-]+/g, ' ')
    .replace(/<[^>]+>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

export function getFirecrawlStatusMeta(article: Article): ProcessingStatusMeta {
  switch (article.firecrawlStatus) {
    case 'processing':
      return { label: '抓取中', icon: 'mdi:loading', tone: 'info' }
    case 'completed':
      return {
        label: '抓取成功',
        icon: 'mdi:check-decagram',
        tone: 'success',
        hint: article.firecrawlCrawledAt || undefined,
      }
    case 'failed':
      return {
        label: '抓取失败',
        icon: 'mdi:alert-circle',
        tone: 'danger',
        hint: article.firecrawlError || undefined,
      }
    case 'pending':
    default:
      return { label: '待抓取', icon: 'mdi:clock-outline', tone: 'warning' }
  }
}

export function getSummaryStatusMeta(article: Article): ProcessingStatusMeta {
  switch (article.summaryStatus) {
    case 'pending':
      return { label: '总结中', icon: 'mdi:loading', tone: 'info' }
    case 'complete':
      return {
        label: '已总结',
        icon: 'mdi:check-circle',
        tone: 'success',
        hint: article.summaryGeneratedAt || undefined,
      }
    case 'failed':
      return {
        label: '总结失败',
        icon: 'mdi:alert',
        tone: 'danger',
        hint: article.completionError || undefined,
      }
    case 'incomplete':
    default:
      return { label: '待总结', icon: 'mdi:text-box-search-outline', tone: 'warning' }
  }
}

export function getSummaryPreview(article: Article): string {
  const summary = toPlainText(article.aiContentSummary)
  if (summary) return summary

  return toPlainText(article.description || article.content)
}

export function shouldShowFirecrawlStatus(article: Article, feed?: RssFeed | null): boolean {
  if (feed?.firecrawlEnabled) return true

  return Boolean(
    article.firecrawlCrawledAt
      || article.firecrawlError
      || article.firecrawlContent?.trim()
      || (article.firecrawlStatus && article.firecrawlStatus !== 'pending')
  )
}

export function shouldShowSummaryStatus(article: Article, feed?: RssFeed | null): boolean {
  if (feed?.articleSummaryEnabled) return true

  return Boolean(
    article.summaryGeneratedAt
      || article.completionError
      || article.aiContentSummary?.trim()
      || article.summaryStatus === 'pending'
      || article.summaryStatus === 'failed'
  )
}

export function getStatusToneClasses(tone: StatusTone): string {
  switch (tone) {
    case 'info':
      return 'bg-sky-50 text-sky-700 border-sky-200'
    case 'success':
      return 'bg-emerald-50 text-emerald-700 border-emerald-200'
    case 'warning':
      return 'bg-amber-50 text-amber-700 border-amber-200'
    case 'danger':
      return 'bg-rose-50 text-rose-700 border-rose-200'
    case 'neutral':
    default:
      return 'bg-stone-100 text-stone-700 border-stone-200'
  }
}
