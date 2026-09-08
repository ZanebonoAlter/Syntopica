import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface CompositeComponent {
  label_id: number
  label: string
  position: number
}

export interface CompositeLabel {
  id: number
  label: string
  slug: string
  description: string
  source: string
  status: string
  ref_count: number
  aliases: string[]
  created_at: string
  components: CompositeComponent[]
}

export interface CompositeLabelListResponse {
  items: CompositeLabel[]
  total: number
}

export interface MountedBoardRef {
  id: number
  label: string
}

/** 手动创建对话框的组件候选（design D7：推荐排序 + 版块上下文 + 共现联动）。 */
export interface ComponentOption {
  id: number
  label: string
  ref_count: number
  board_count: number
  /** board_id 上下文：该组件已被当前版块挂载 */
  in_board: boolean
  /** related_to 联动：与已选组件的同 tag 共现次数 */
  cooccurrence: number
  mounted_boards: MountedBoardRef[]
}

export interface CompositeLabelCreateResponse {
  id: number
  label: string
  slug: string
  aliases: string[]
  status: string
  source: string
  ref_count: number
  /** created / reused_l1 / alias_l2 */
  outcome: string
  reused_label?: string
  message: string
}

export function useCompositeLabelsApi() {
  async function getLabels(params?: { status?: string }): Promise<ApiResponse<CompositeLabelListResponse>> {
    const query = apiClient.buildQueryParams(params)
    return apiClient.get(`/composite-labels${query ? `?${query}` : ''}`)
  }

  async function getComponentOptions(params?: { limit?: number; board_id?: number; related_to?: number }): Promise<ApiResponse<{ items: ComponentOption[] }>> {
    const query = apiClient.buildQueryParams(params)
    return apiClient.get(`/composite-labels/component-options${query ? `?${query}` : ''}`)
  }

  async function createLabel(data: { label: string; description?: string; component_label_ids: number[] }): Promise<ApiResponse<CompositeLabelCreateResponse>> {
    return apiClient.post('/composite-labels', data)
  }

  async function disableLabel(id: number): Promise<ApiResponse<{ id: number; status: string }>> {
    return apiClient.post(`/composite-labels/${id}/disable`)
  }

  async function enableLabel(id: number): Promise<ApiResponse<{ id: number; status: string }>> {
    return apiClient.post(`/composite-labels/${id}/enable`)
  }

  return {
    getLabels,
    getComponentOptions,
    createLabel,
    disableLabel,
    enableLabel,
  }
}
