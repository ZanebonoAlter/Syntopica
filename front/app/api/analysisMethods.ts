import { apiClient } from './client'
import type { ApiResponse } from '~/types'

/**
 * 分析方法卡 API client —— board-level-deep-analysis 2.6。
 *
 * 对应后端 /analysis-methods CRUD（internal/dataenrichment/handler/analysis_method_handler.go）。
 * 方法卡是「过程/核查清单」而非作者人格画像：仅在未来调查按用户问题 + 父简报
 * 元数据适配选择 0-2 张后，才加载完整正文辅助假设生成；简报/事实阶段永不注入。
 */

/** 选择元数据：何时适用 / 何时禁用 / 需要哪些证据 / 已知失败模式。 */
export interface AnalysisMethodSelectionMeta {
  applicable_when: string[]
  avoid_when: string[]
  required_evidence: string[]
  failure_modes: string[]
}

/** 分析方法卡行（表 analysis_methods）。 */
export interface AnalysisMethod {
  id: number
  /** 唯一短名。 */
  name: string
  /** 展示标题。 */
  title: string
  /** 一句话摘要（选择阶段可见，正文只在选中后加载）。 */
  summary: string
  selection_meta: AnalysisMethodSelectionMeta
  /** 操作指引正文。 */
  content: string
  enabled: boolean
  /** legacy=true 表示由旧参考角色迁移而来，需人工整理边界后再启用。 */
  legacy: boolean
  created_at?: string
  updated_at?: string
}

export interface CreateAnalysisMethodBody {
  name: string
  title?: string
  summary?: string
  selection_meta?: AnalysisMethodSelectionMeta
  content: string
  enabled?: boolean
}

export type UpdateAnalysisMethodBody = Partial<CreateAnalysisMethodBody>

export function useAnalysisMethodsApi() {
  async function listMethods(): Promise<ApiResponse<AnalysisMethod[]>> {
    return apiClient.get('/analysis-methods')
  }

  async function createMethod(body: CreateAnalysisMethodBody): Promise<ApiResponse<AnalysisMethod>> {
    return apiClient.post('/analysis-methods', body)
  }

  async function getMethod(id: number): Promise<ApiResponse<AnalysisMethod>> {
    return apiClient.get(`/analysis-methods/${id}`)
  }

  async function updateMethod(id: number, body: UpdateAnalysisMethodBody): Promise<ApiResponse<AnalysisMethod>> {
    return apiClient.put(`/analysis-methods/${id}`, body)
  }

  async function setEnabled(id: number, enabled: boolean): Promise<ApiResponse<AnalysisMethod>> {
    return apiClient.put(`/analysis-methods/${id}/enable`, { enabled })
  }

  async function deleteMethod(id: number): Promise<ApiResponse<{ deleted: number }>> {
    return apiClient.delete(`/analysis-methods/${id}`)
  }

  return { listMethods, createMethod, getMethod, updateMethod, setEnabled, deleteMethod }
}
