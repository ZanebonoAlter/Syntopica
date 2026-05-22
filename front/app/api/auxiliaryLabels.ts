import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface AuxiliaryLabel {
  id: number
  label: string
  slug: string
  aliases: string[]
  ref_count: number
  description: string
  display_order: number
  source: string
  status: string
  protected: boolean
}

export function useAuxiliaryLabelsApi() {
  async function getLabels(params?: { search?: string; status?: string }): Promise<ApiResponse<{ items: AuxiliaryLabel[]; total: number }>> {
    const query = apiClient.buildQueryParams(params)
    return apiClient.get(`/auxiliary-labels${query ? `?${query}` : ''}`)
  }

  async function disableLabel(id: number): Promise<ApiResponse<{ id: number }>> {
    return apiClient.post(`/auxiliary-labels/${id}/disable`)
  }

  async function mergeAlias(sourceId: number, targetId: number): Promise<ApiResponse<{ source_id: number; target_id: number }>> {
    return apiClient.post('/auxiliary-labels/merge-alias', { source_id: sourceId, target_id: targetId })
  }

  return {
    getLabels,
    disableLabel,
    mergeAlias,
  }
}
