import { describe, expect, it } from 'vitest'

import { getArticleContentSources, resolveArticleContentBySource } from './articleContentSource'

describe('articleContentSource', () => {
  it('prefers firecrawl when both firecrawl and original content exist', () => {
    const sources = getArticleContentSources({
      firecrawlContent: '# Firecrawl body',
      content: '<p>Original fallback</p>',
    })

    expect(sources.available).toEqual(['firecrawl', 'original'])
    expect(sources.defaultSource).toBe('firecrawl')
    expect(resolveArticleContentBySource(sources, 'firecrawl')).toBe('# Firecrawl body')
    expect(resolveArticleContentBySource(sources, 'original')).toBe('<p>Original fallback</p>')
  })

  it('only exposes original when firecrawl content is missing', () => {
    const sources = getArticleContentSources({
      content: '<p>Original fallback</p>',
    })

    expect(sources.available).toEqual(['original'])
    expect(sources.defaultSource).toBe('original')
    expect(resolveArticleContentBySource(sources, 'original')).toBe('<p>Original fallback</p>')
  })

  it('only exposes firecrawl when original content is missing', () => {
    const sources = getArticleContentSources({
      firecrawlContent: '# Firecrawl body',
    })

    expect(sources.available).toEqual(['firecrawl'])
    expect(sources.defaultSource).toBe('firecrawl')
    expect(resolveArticleContentBySource(sources, 'original')).toBe('# Firecrawl body')
  })
})
