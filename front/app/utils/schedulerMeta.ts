import type { SchedulerArticleRef, SchedulerStatus } from '~/types/scheduler'

const contentCompletionAliases = new Set(['content_completion', 'ai_summary'])

type ContentCompletionPanelStatus = Pick<SchedulerStatus, 'name' | 'overview' | 'is_executing' | 'current_article'>
type SchedulerStatusLike = Pick<SchedulerStatus, 'name' | 'status' | 'is_executing' | 'database_state'>
type ContentCompletionArticleStatus = Pick<SchedulerStatus, 'name' | 'is_executing' | 'current_article' | 'stale_processing_article' | 'last_run_summary'>

export function isContentCompletionScheduler(name: string): boolean {
  return contentCompletionAliases.has(name)
}

export function isHotScheduler(name: string): boolean {
  return name === 'auto_refresh' || isContentCompletionScheduler(name) || name === 'firecrawl'
}

export function getSchedulerDisplayName(name: string): string {
  if (isContentCompletionScheduler(name)) {
    return '文章总结'
  }

  const names: Record<string, string> = {
    'auto_refresh': '后台刷新',
    'firecrawl': '全文爬取',
    'tag_hierarchy_cleanup': '标签清理',
  }

  return names[name] || name
}

export function getSchedulerIcon(name: string): string {
  if (isContentCompletionScheduler(name)) {
    return 'mdi:text-box-search-outline'
  }

  const icons: Record<string, string> = {
    'auto_refresh': 'mdi:refresh',
    'firecrawl': 'mdi:spider-web',
    'tag_hierarchy_cleanup': 'mdi:tag-remove-outline',
  }

  return icons[name] || 'mdi:cog'
}

export function getSchedulerColor(name: string): string {
  if (isContentCompletionScheduler(name)) {
    return 'from-amber-500 to-orange-500'
  }

  const colors: Record<string, string> = {
    'auto_refresh': 'from-blue-500 to-cyan-500',
    'firecrawl': 'from-rose-500 to-orange-500',
    'tag_hierarchy_cleanup': 'from-violet-500 to-purple-600',
  }

  return colors[name] || 'from-gray-500 to-gray-600'
}

export function shouldShowContentCompletionPanel(scheduler: ContentCompletionPanelStatus): boolean {
  return isContentCompletionScheduler(scheduler.name)
    && Boolean(scheduler.overview || scheduler.is_executing || scheduler.current_article)
}

export function getSchedulerStatusLabel(scheduler: SchedulerStatusLike): string | undefined {
  if (isContentCompletionScheduler(scheduler.name) && scheduler.is_executing !== true && scheduler.status) {
    return scheduler.status
  }

  return scheduler.database_state?.status || scheduler.status
}

export function getCurrentContentCompletionArticle(scheduler: ContentCompletionArticleStatus): SchedulerArticleRef | null | undefined {
  if (scheduler.current_article) {
    return scheduler.current_article
  }

  if (isContentCompletionScheduler(scheduler.name) && scheduler.is_executing !== true) {
    return scheduler.stale_processing_article || scheduler.last_run_summary?.stale_processing_article || null
  }

  return null
}
