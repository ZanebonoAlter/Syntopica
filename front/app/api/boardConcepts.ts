import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface BoardConcept {
  id: number
  name: string
  description: string
  category: string
  scope_type: string
  scope_category_id: number | null
  is_system: boolean
  status: string
  display_order: number
  created_at: string
  updated_at: string
}

export interface ConceptSuggestion {
  name: string
  description: string
}

export interface SectorItem {
  id: number
  name: string
  description: string
  category: string
  source: 'auto' | 'llm' | 'manual'
  protected: boolean
  declining: boolean
  peak_tag_count: number
  tag_count: number
  status: string
  display_order: number
  created_at: string
  updated_at: string
}

export interface SectorDiff {
  keep: Array<{ id: number; name: string }> | null
  add: Array<{ name: string; description: string }> | null
  merge: Array<{ source_ids: number[]; target_id: number; name: string }> | null
  split: Array<{ source_id: number; new_items: Array<{ name: string; description: string }> }> | null
  affected_tag_count: number
}

export interface SectorDiffExecutionItemResult {
  operation: 'add' | 'merge' | 'split' | string
  status: 'success' | 'failed' | string
  name?: string
  source_id?: number
  source_ids?: number[]
  target_id?: number
  affected_tag_count: number
  moved_tag_count: number
  created_ids?: number[]
  error?: string
}

export interface SectorDiffExecutionResult {
  results: SectorDiffExecutionItemResult[]
  success_count: number
  failed_count: number
  affected_tag_count: number
  moved_tag_count: number
  created_ids?: number[]
}

export function useBoardConceptsApi() {
  async function getBoardConcepts(): Promise<ApiResponse<BoardConcept[]>> {
    return apiClient.get<BoardConcept[]>('/hierarchy/concepts')
  }

  async function createBoardConcept(data: {
    name: string
    description: string
    scope_type?: string
    scope_category_id?: number
  }): Promise<ApiResponse<BoardConcept>> {
    return apiClient.post<BoardConcept>('/hierarchy/concepts', data)
  }

  async function updateBoardConcept(
    id: number,
    data: { name: string; description: string },
  ): Promise<ApiResponse<BoardConcept>> {
    return apiClient.put<BoardConcept>(`/hierarchy/concepts/${id}`, data)
  }

  async function deleteBoardConcept(id: number): Promise<ApiResponse<void>> {
    return apiClient.delete<void>(`/hierarchy/concepts/${id}`)
  }

  async function suggestConcepts(category: string): Promise<ApiResponse<ConceptSuggestion[]>> {
    return apiClient.post<ConceptSuggestion[]>('/hierarchy/concepts/suggest', { category })
  }

  async function getSectors(category: string, all?: boolean): Promise<ApiResponse<SectorItem[]>> {
    const params = new URLSearchParams()
    params.set('category', category)
    if (all) params.set('all', 'true')
    return apiClient.get<SectorItem[]>(`/narratives/board-concepts?${params.toString()}`)
  }

  async function createSector(data: {
    name: string
    description?: string
    category: string
    source?: string
  }): Promise<ApiResponse<SectorItem>> {
    return apiClient.post<SectorItem>('/narratives/board-concepts', data)
  }

  async function deleteSector(id: number, confirm?: boolean): Promise<ApiResponse<void>> {
    const query = confirm ? '?confirm=true' : ''
    return apiClient.delete<void>(`/narratives/board-concepts/${id}${query}`)
  }

  async function regenerateSectors(category: string): Promise<ApiResponse<SectorDiff>> {
    return apiClient.post<SectorDiff>('/narratives/board-concepts/regenerate', { category })
  }

  async function confirmRegenerateSectors(category: string, diff: SectorDiff): Promise<ApiResponse<SectorDiffExecutionResult>> {
    return apiClient.post<SectorDiffExecutionResult>('/narratives/board-concepts/regenerate/confirm', {
      category,
      diff,
    })
  }

  return {
    getBoardConcepts,
    createBoardConcept,
    updateBoardConcept,
    deleteBoardConcept,
    suggestConcepts,
    getSectors,
    createSector,
    deleteSector,
    regenerateSectors,
    confirmRegenerateSectors,
  }
}
