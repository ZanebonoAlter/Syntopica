import { apiClient } from './client'
import type { ApiResponse, PaginatedData } from '~/types'
import type {
  AuxiliaryRepairContextRaw,
  AuxiliaryRepairJobRaw,
} from './normalizers/auxiliaryRepair'

export type {
  AuxiliaryRepairContextRaw,
  AuxiliaryRepairJobRaw,
} from './normalizers/auxiliaryRepair'

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

  // ── 共享描述修复（board-health-triage-advice seam C1 / task 3.1）──────────
  // 四个资源与后端 auxrepair 路由一一对应（wire 仍是数字 ID；字符串 ID 视图在
  // api/normalizers/auxiliaryRepair.ts 产出）。本层只做 URL/方法/body 组装，
  // 不做状态判定、不 toast。

  /** 修复上下文（纯读）：原描述 + 不透明版本 + 直接引用版块 + 最新任务。 */
  async function getDescriptionRepairContext(id: number | string): Promise<ApiResponse<AuxiliaryRepairContextRaw>> {
    return apiClient.get(`/auxiliary-labels/${id}/description-repair-context`)
  }

  /** 创建修复任务（202）：同幂等键 + 同 body 重发返回同一任务。 */
  async function createDescriptionRepair(
    id: number | string,
    body: { description: string; expected_version: string; idempotency_key: string },
  ): Promise<ApiResponse<AuxiliaryRepairJobRaw>> {
    return apiClient.post(`/auxiliary-labels/${id}/description-repairs`, body)
  }

  /** 任务查询（纯读）：按任务 id 取持久阶段与计数。 */
  async function getDescriptionRepairJob(id: number | string, jobId: number | string): Promise<ApiResponse<AuxiliaryRepairJobRaw>> {
    return apiClient.get(`/auxiliary-labels/${id}/description-repairs/${jobId}`)
  }

  /** 明确重试（无参数）：只把同一任务重新排入，不新建、不改版本/描述/范围。 */
  async function retryDescriptionRepair(id: number | string, jobId: number | string): Promise<ApiResponse<AuxiliaryRepairJobRaw>> {
    return apiClient.post(`/auxiliary-labels/${id}/description-repairs/${jobId}/retry`, {})
  }

  return {
    getLabels,
    getClusters,
    disableLabel,
    mergeAlias,
    getDescriptionRepairContext,
    createDescriptionRepair,
    getDescriptionRepairJob,
    retryDescriptionRepair,
  }
}
