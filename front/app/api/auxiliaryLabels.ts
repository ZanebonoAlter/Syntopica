import { apiClient } from './client'
import type { ApiResponse, PaginatedData } from '~/types'

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

export interface AuxiliaryLabelClusterItem {
  id: number
  label: string
  slug: string
  ref_count: number
}

export interface AuxiliaryLabelCluster {
  labels: AuxiliaryLabelClusterItem[]
  size: number
  label: string
}

export interface AuxiliaryLabelClustersResponse {
  clusters: AuxiliaryLabelCluster[]
  unclustered_count: number
}

export function useAuxiliaryLabelsApi() {
  async function getLabels(params?: { search?: string; status?: string; page?: number; per_page?: number }): Promise<ApiResponse<PaginatedData<AuxiliaryLabel>>> {
    const query = apiClient.buildQueryParams(params)
    return apiClient.get(`/auxiliary-labels${query ? `?${query}` : ''}`)
  }

  async function getClusters(): Promise<ApiResponse<AuxiliaryLabelClustersResponse>> {
    return apiClient.get('/auxiliary-labels/clusters')
  }

  async function disableLabel(id: number): Promise<ApiResponse<{ id: number }>> {
    return apiClient.post(`/auxiliary-labels/${id}/disable`)
  }

  async function mergeAlias(sourceId: number, targetId: number): Promise<ApiResponse<{ source_id: number; target_id: number }>> {
    return apiClient.post('/auxiliary-labels/merge-alias', { source_id: sourceId, target_id: targetId })
  }

  return {
    getLabels,
    getClusters,
    disableLabel,
    mergeAlias,
  }
}
