import { apiClient } from './client'
import type { ApiResponse } from '~/types'
import type { TagHierarchyNode, TagHierarchyResponse } from '~/types/topicTag'

interface RawTagHierarchyNode {
  id: number
  label: string
  slug: string
  category: string
  icon: string
  feed_count: number
  article_count: number
  similarity_score?: number
  is_active: boolean
  quality_score?: number
  is_low_quality?: boolean
  children: RawTagHierarchyNode[]
}

interface RawHierarchyResponse {
  nodes: RawTagHierarchyNode[]
  total: number
}

function mapNode(raw: RawTagHierarchyNode): TagHierarchyNode {
  return {
    id: raw.id,
    label: raw.label,
    slug: raw.slug,
    category: raw.category,
    icon: raw.icon,
    feedCount: raw.feed_count,
    articleCount: raw.article_count || 0,
    similarityScore: raw.similarity_score,
    isActive: raw.is_active,
    qualityScore: raw.quality_score,
    isLowQuality: raw.is_low_quality,
    children: raw.children ? raw.children.map(mapNode) : [],
  }
}

export function useAbstractTagApi() {
  return {
    async fetchHierarchy(category?: string, feedId?: string, categoryId?: string, unclassified?: boolean, timeRange?: string, conceptId?: number): Promise<ApiResponse<TagHierarchyResponse>> {
      const params = new URLSearchParams()
      if (category) params.set('category', category)
      if (feedId) params.set('feed_id', feedId)
      if (categoryId) params.set('category_id', categoryId)
      if (unclassified) params.set('unclassified', 'true')
      if (timeRange) params.set('time_range', timeRange)
      if (conceptId) params.set('concept_id', String(conceptId))
      const query = params.toString() ? `?${params.toString()}` : ''
      const response = await apiClient.get<RawHierarchyResponse>(`/topic-tags/hierarchy${query}`)
      if (response.success && response.data) {
        return {
          ...response,
          data: {
            nodes: (response.data.nodes ?? []).map(mapNode),
            total: response.data.total,
          },
        } as ApiResponse<TagHierarchyResponse>
      }
      return response as unknown as ApiResponse<TagHierarchyResponse>
    },

    async updateAbstractName(tagId: number, newName: string): Promise<ApiResponse<{ id: number; newName: string }>> {
      return apiClient.put('/topic-tags/' + tagId + '/abstract-name', { new_name: newName })
    },

    async detachChild(parentId: number, childId: number): Promise<ApiResponse<{ message: string }>> {
      return apiClient.post('/topic-tags/' + parentId + '/detach', { child_id: childId })
    },

    async reassignTag(tagId: number, newParentId: number): Promise<ApiResponse<{ message: string }>> {
      return apiClient.post('/topic-tags/' + tagId + '/reassign', { parent_id: newParentId })
    },

    async organizeTags(category?: string): Promise<ApiResponse<{ message: string }>> {
      const params = new URLSearchParams()
      if (category) params.set('category', category)
      const query = params.toString() ? `?${params.toString()}` : ''
      return apiClient.post('/topic-tags/organize' + query, {})
    },
  }
}
