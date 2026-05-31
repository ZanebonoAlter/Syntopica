import { apiClient } from './client'
import { getApiOrigin } from '~/utils/api'
import type { ApiResponse } from '~/types'
import type { MergeWithCustomNameRequest, MergeWithCustomNameResult, ScanProgress, EvaluateProgress, MergeGroupResponse } from '~/types/tagMerge'

interface RawMergeResult {
  source_id: number
  target_id: number
  new_label: string
  merged_at?: string
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
    async loadMergeGroups(params?: { limit?: number; categoryId?: string; feedId?: string }) {
      const queryParams = apiClient.buildQueryParams({
        limit: params?.limit ? String(params.limit) : undefined,
        category_id: params?.categoryId ?? undefined,
        feed_id: params?.feedId ?? undefined,
      })
      const endpoint = queryParams ? `/topic-tags/merge-preview?${queryParams}` : '/topic-tags/merge-preview'
      const response = await apiClient.get<MergeGroupResponse>(endpoint)
      return response as ApiResponse<MergeGroupResponse>
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

    async triggerFullScan() {
      return apiClient.post<{ message: string }>('/topic-tags/merge-preview/scan', {})
    },

    createScanEventSource(onProgress: (progress: ScanProgress) => void): EventSource {
      const origin = getApiOrigin()
      const es = new EventSource(`${origin}/api/topic-tags/merge-preview/scan/stream`)
      es.onmessage = (e) => {
        onProgress(JSON.parse(e.data))
      }
      return es
    },

    async dismissSuggestion(newTagId: number, existingTagId: number) {
      return apiClient.post('/topic-tags/merge-preview/dismiss', {
        new_tag_id: newTagId,
        existing_tag_id: existingTagId,
      })
    },

    // --- LLM Evaluate ---

    async triggerEvaluate() {
      return apiClient.post<{ message: string }>('/topic-tags/merge-preview/evaluate', {})
    },

    createEvaluateEventSource(onProgress: (progress: EvaluateProgress) => void): EventSource {
      const origin = getApiOrigin()
      const es = new EventSource(`${origin}/api/topic-tags/merge-preview/evaluate/stream`)
      es.onmessage = (e) => {
        onProgress(JSON.parse(e.data))
      }
      return es
    },

    async addToGroup(targetTagId: number, newTagId: number) {
      return apiClient.post('/topic-tags/merge-preview/add-to-group', {
        target_tag_id: targetTagId,
        new_tag_id: newTagId,
      })
    },

    async searchTags(query: string) {
      return apiClient.get<Array<{ id: number; label: string; slug: string; category: string; feed_count: number }>>(
        `/topic-tags/search?q=${encodeURIComponent(query)}&limit=20`,
      )
    },
  }
}
