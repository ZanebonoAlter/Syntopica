import { describe, expect, it } from 'vitest'
import type { SchedulerTask } from '~/types/scheduler'

import {
  getCurrentContentCompletionArticle,
  getSchedulerColor,
  getSchedulerDisplayName,
  getSchedulerIcon,
  getSchedulerStatusLabel,
  isContentCompletionScheduler,
  isHotScheduler,
  shouldShowContentCompletionPanel,
} from './schedulerMeta'

describe('schedulerMeta', () => {
  it('treats content_completion as the canonical article completion scheduler', () => {
    expect(isContentCompletionScheduler('content_completion')).toBe(true)
    expect(getSchedulerDisplayName('content_completion')).toBe('文章总结')
    expect(getSchedulerIcon('content_completion')).toBe('mdi:text-box-search-outline')
    expect(getSchedulerColor('content_completion')).toBe('from-amber-500 to-orange-500')
  })

  it('keeps ai_summary as a backward-compatible alias', () => {
    expect(isContentCompletionScheduler('ai_summary')).toBe(true)
    expect(getSchedulerDisplayName('ai_summary')).toBe('文章总结')
  })

  it('marks content completion as a hot scheduler for polling', () => {
    expect(isHotScheduler('content_completion')).toBe(true)
    expect(isHotScheduler('firecrawl')).toBe(true)
  })

  it('shows content completion panel while a current article is executing even without overview', () => {
    expect(shouldShowContentCompletionPanel({
      name: 'content_completion',
      is_executing: true,
      current_article: {
        id: 1,
        feed_id: 2,
        title: '正在补全的文章',
      },
    })).toBe(true)
  })

  it('prefers runtime idle over stale database running status for content completion', () => {
    expect(getSchedulerStatusLabel({
      name: 'content_completion',
      status: 'idle',
      is_executing: false,
      database_state: {
        status: 'running',
      } as SchedulerTask,
    })).toBe('idle')
  })

  it('falls back to stale processing article when no live current article exists', () => {
    expect(getCurrentContentCompletionArticle({
      name: 'content_completion',
      is_executing: false,
      current_article: null,
      stale_processing_article: {
        id: 44471,
        feed_id: 25,
        title: '遗留 pending 文章',
      },
    })?.title).toBe('遗留 pending 文章')
  })
})
