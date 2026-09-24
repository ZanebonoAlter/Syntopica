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
})
