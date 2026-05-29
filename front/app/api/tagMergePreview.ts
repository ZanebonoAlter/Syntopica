import { apiClient } from './client'
import type { ApiResponse } from '~/types'
import type { TagMergeCandidate, MergePreviewResponse, MergeWithCustomNameRequest, MergeWithCustomNameResult, ArticleTitlePreview } from '~/types/tagMerge'

interface RawArticleTitle {
  article_id: number
  title: string
  link: string
}

interface RawMergeCandidate {
  source_tag_id: number
  source_label: string
  source_slug: string
  target_tag_id: number
  target_label: string
  target_slug: string
  category: string
  similarity: number
  source_articles: number
  target_articles: number
  source_article_list?: RawArticleTitle[]
  target_article_list?: RawArticleTitle[]
}

interface RawMergeResult {
  source_id: number
  target_id: number
  new_label: string
  merged_at?: string
}

function mapArticle(a: RawArticleTitle): ArticleTitlePreview {
  return { articleId: a.article_id, title: a.title, link: a.link }
}

function mapCandidate(c: RawMergeCandidate): TagMergeCandidate {
  return {
    sourceTagId: c.source_tag_id,
    sourceLabel: c.source_label,
    sourceSlug: c.source_slug,
    targetTagId: c.target_tag_id,
    targetLabel: c.target_label,
    targetSlug: c.target_slug,
    category: c.category,
    similarity: c.similarity,
    sourceArticles: c.source_articles,
    targetArticles: c.target_articles,
    sourceArticleTitles: c.source_article_list?.map(mapArticle),
    targetArticleTitles: c.target_article_list?.map(mapArticle),
  }
}

function mapMergeResult(r: RawMergeResult): MergeWithCustomNameResult {
  return {
    sourceId: r.source_id,
    targetId: r.target_id,
    newLabel: r.new_label,
    mergedAt: r.merged_at,
  }
}

export function useTagMergePreviewApi() {
  return {
    async scanMergePreview(params?: { limit?: number; includeArticles?: boolean; categoryId?: string; feedId?: string }) {
      const queryParams = apiClient.buildQueryParams({
        limit: params?.limit ? String(params.limit) : undefined,
        include_articles: params?.includeArticles ? 'true' : undefined,
        category_id: params?.categoryId ?? undefined,
        feed_id: params?.feedId ?? undefined,
      })
      const endpoint = queryParams ? `/topic-tags/merge-preview?${queryParams}` : '/topic-tags/merge-preview'
      const response = await apiClient.get<{ candidates: RawMergeCandidate[]; total: number }>(endpoint)
      if (response.success && response.data) {
        return {
          ...response,
          data: {
            candidates: response.data.candidates.map(mapCandidate),
            total: response.data.total,
          } satisfies MergePreviewResponse,
        } as ApiResponse<MergePreviewResponse>
      }
      return response as unknown as ApiResponse<MergePreviewResponse>
    },

    async mergeTagsWithCustomName(request: MergeWithCustomNameRequest) {
      const response = await apiClient.post<RawMergeResult>('/topic-tags/merge-with-name', {
        source_tag_id: request.sourceTagId,
        target_tag_id: request.targetTagId,
        new_name: request.newName,
      })
      if (response.success && response.data) {
        return { ...response, data: mapMergeResult(response.data) } as unknown as ApiResponse<MergeWithCustomNameResult>
      }
      return response as unknown as ApiResponse<MergeWithCustomNameResult>
    },
  }
}
