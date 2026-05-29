export type ArticleContentSource = 'firecrawl' | 'original'

interface ArticleContentInput {
  firecrawlContent?: string
  content?: string
}

export interface ArticleContentSources {
  available: ArticleContentSource[]
  defaultSource: ArticleContentSource | null
  firecrawlContent: string
  originalContent: string
}

function cleanContent(content?: string): string {
  return content?.trim() || ''
}

export function getArticleContentSources(input: ArticleContentInput): ArticleContentSources {
  const firecrawlContent = cleanContent(input.firecrawlContent)
  const originalContent = cleanContent(input.content)

  const available: ArticleContentSource[] = []

  if (firecrawlContent) {
    available.push('firecrawl')
  }

  if (originalContent) {
    available.push('original')
  }

  return {
    available,
    defaultSource: available[0] ?? null,
    firecrawlContent,
    originalContent,
  }
}

export function resolveArticleContentBySource(
  sources: ArticleContentSources,
  selectedSource?: ArticleContentSource,
): string {
  const source = selectedSource && sources.available.includes(selectedSource)
    ? selectedSource
    : sources.defaultSource

  if (source === 'original') {
    return sources.originalContent || sources.firecrawlContent
  }

  if (source === 'firecrawl') {
    return sources.firecrawlContent || sources.originalContent
  }

  return ''
}
