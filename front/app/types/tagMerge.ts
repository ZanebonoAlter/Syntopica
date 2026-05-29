export interface ArticleTitlePreview {
  articleId: number
  title: string
  link: string
}

export interface TagMergeCandidate {
  sourceTagId: number
  sourceLabel: string
  sourceSlug: string
  targetTagId: number
  targetLabel: string
  targetSlug: string
  category: string
  similarity: number
  sourceArticles: number
  targetArticles: number
  sourceArticleTitles?: ArticleTitlePreview[]
  targetArticleTitles?: ArticleTitlePreview[]
}

export interface MergePreviewResponse {
  candidates: TagMergeCandidate[]
  total: number
}

export interface MergeWithCustomNameRequest {
  sourceTagId: number
  targetTagId: number
  newName: string
}

export interface MergeWithCustomNameResult {
  sourceId: number
  targetId: number
  newLabel: string
  mergedAt?: string
}

export interface MergeSummary {
  mergedCount: number
  skippedCount: number
  failedCount: number
  mergedDetails: Array<{
    sourceId: number
    sourceLabel: string
    targetId: number
    newLabel: string
  }>
}
