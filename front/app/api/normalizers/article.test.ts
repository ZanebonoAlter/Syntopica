import { describe, expect, it } from 'vitest'
import { normalizeArticle, type ArticlePayload } from './article'

/**
 * slim-article-list-payload task 3.1：列表窄投影后 normalizer 的容错契约。
 * - 正文类字段（description/content/firecrawl_content）列表层缺省时不抛错，description/content 兜底空串；
 * - excerpt（列表窄投影导语）透传，缺失兜底空串；
 * - 详情接口返回的完整字段仍原样映射。
 */

function payload(over: Partial<ArticlePayload> = {}): ArticlePayload {
  return {
    id: 1,
    feed_id: 2,
    title: '标题',
    link: 'https://example.com/a',
    pub_date: '2026-09-18T00:00:00Z',
    created_at: '2026-09-18T00:00:00Z',
    read: false,
    favorite: false,
    ...over,
  }
}

describe('normalizeArticle', () => {
  it('列表窄投影：正文类字段缺省时兜底空串，excerpt 透传', () => {
    const article = normalizeArticle(payload({ excerpt: '列表层导语纯文本。' }))

    expect(article.description).toBe('')
    expect(article.content).toBe('')
    expect(article.excerpt).toBe('列表层导语纯文本。')
    expect(article.firecrawlContent).toBeUndefined()
  })

  it('详情契约：完整字段原样映射，excerpt 缺失兜底空串', () => {
    const article = normalizeArticle(payload({
      description: '<p>导语</p>',
      content: '<p>正文</p>',
      firecrawl_content: '<p>抓取正文</p>',
    }))

    expect(article.description).toBe('<p>导语</p>')
    expect(article.content).toBe('<p>正文</p>')
    expect(article.firecrawlContent).toBe('<p>抓取正文</p>')
    expect(article.excerpt).toBe('')
  })

  it('判定字段映射：snake_case → camelCase，非枚举值/缺省收敛为未判定', () => {
    const article = normalizeArticle(payload({
      ai_detection_status: 'suspect',
      ai_detection_score: 0.82,
      ai_detection_source: 'manual',
      ai_detection_at: '2026-11-20T08:00:00Z',
    }))

    expect(article.aiDetectionStatus).toBe('suspect')
    expect(article.aiDetectionScore).toBe(0.82)
    expect(article.aiDetectionSource).toBe('manual')
    expect(article.aiDetectionAt).toBe('2026-11-20T08:00:00Z')

    // 非法枚举值不得当判定态用（界面一律不显示标记）
    expect(normalizeArticle(payload({ ai_detection_status: 'weird' })).aiDetectionStatus).toBeUndefined()
    expect(normalizeArticle(payload({ ai_detection_source: 'robot' })).aiDetectionSource).toBeUndefined()

    const missing = normalizeArticle(payload())
    expect(missing.aiDetectionStatus).toBeUndefined()
    expect(missing.aiDetectionScore).toBeUndefined()
    expect(missing.aiDetectionSource).toBeUndefined()
    expect(missing.aiDetectionAt).toBeUndefined()
  })
})
