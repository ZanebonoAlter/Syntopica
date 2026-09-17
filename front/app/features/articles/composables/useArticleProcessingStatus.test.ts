import { describe, expect, it } from 'vitest'
import type { Article } from '~/types'
import {
  getArticlePipelineState,
  getPipelineStateMeta,
} from './useArticleProcessingStatus'

/**
 * 行尾单图标四态派生（declutter-article-list-panel design D3）：
 * 优先级 失败 > 进行中 > 排队 > 完成；undefined 状态视为完成。
 */

function article(over: Partial<Article> = {}): Article {
  return {
    id: '1',
    feedId: 'f1',
    title: 't',
    description: '',
    content: '',
    link: '',
    pubDate: '2026-09-18T00:00:00Z',
    category: '',
    ...over,
  }
}

describe('getArticlePipelineState', () => {
  it('任一失败 → failed（最高优先级，即使同时 processing/pending）', () => {
    expect(getArticlePipelineState(article({ firecrawlStatus: 'failed', summaryStatus: 'pending' }))).toBe('failed')
    expect(getArticlePipelineState(article({ firecrawlStatus: 'processing', summaryStatus: 'failed' }))).toBe('failed')
  })

  it('任一进行中 → processing', () => {
    expect(getArticlePipelineState(article({ firecrawlStatus: 'processing', summaryStatus: 'incomplete' }))).toBe('processing')
    expect(getArticlePipelineState(article({ firecrawlStatus: 'pending', summaryStatus: 'pending' }))).toBe('processing')
  })

  it('任一排队（firecrawl pending / summary incomplete）→ queued', () => {
    expect(getArticlePipelineState(article({ firecrawlStatus: 'pending', summaryStatus: 'complete' }))).toBe('queued')
    expect(getArticlePipelineState(article({ firecrawlStatus: 'completed', summaryStatus: 'incomplete' }))).toBe('queued')
  })

  it('全部完成或无处理痕迹 → done', () => {
    expect(getArticlePipelineState(article({ firecrawlStatus: 'completed', summaryStatus: 'complete' }))).toBe('done')
    expect(getArticlePipelineState(article())).toBe('done')
  })
})

describe('getPipelineStateMeta', () => {
  it('四态映射图标与颜色 token', () => {
    expect(getPipelineStateMeta('queued').icon).toBe('mdi:clock-outline')
    expect(getPipelineStateMeta('queued').colorToken).toBe('warning')
    expect(getPipelineStateMeta('processing').icon).toBe('mdi:loading')
    expect(getPipelineStateMeta('processing').spinning).toBe(true)
    expect(getPipelineStateMeta('failed').icon).toBe('mdi:alert-circle')
    expect(getPipelineStateMeta('failed').colorToken).toBe('error')
    expect(getPipelineStateMeta('done').icon).toBe('mdi:check-circle')
    expect(getPipelineStateMeta('done').colorToken).toBe('muted')
  })
})
