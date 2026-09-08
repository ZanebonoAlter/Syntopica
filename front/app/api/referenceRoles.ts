import { apiClient } from './client'
import type { ApiResponse } from '~/types'

/**
 * 参考角色（方法论画像）API client —— board-level-deep-analysis 2.x。
 *
 * 对应后端 /reference-roles CRUD（internal/dataenrichment/handler/reference_role_handler.go）。
 * 画像内容在循环 B 三个 LLM 环节（interpret/analyze/agentLoop system prompt）注入，
 * 单条 rune 数 >4000 会在注入时整条丢弃（后端 referenceRoleAppendix 控制）。
 */

/** 参考角色行（表 reference_roles）。 */
export interface ReferenceRole {
  id: number
  /** 唯一短名（如 inside-america）。 */
  name: string
  /** 展示标题（如「内部看美国 · 分析基因画像」）。 */
  title: string
  /** 画像正文（注入 prompt 的方法论描述）。 */
  content: string
  /** 启用即注入；停用即时生效（后端现查 DB）。 */
  enabled: boolean
  created_at?: string
  updated_at?: string
}

export interface CreateReferenceRoleBody {
  name: string
  title?: string
  content: string
  enabled?: boolean
}

export type UpdateReferenceRoleBody = Partial<CreateReferenceRoleBody>

export function useReferenceRolesApi() {
  async function listRoles(): Promise<ApiResponse<ReferenceRole[]>> {
    return apiClient.get('/reference-roles')
  }

  async function createRole(body: CreateReferenceRoleBody): Promise<ApiResponse<ReferenceRole>> {
    return apiClient.post('/reference-roles', body)
  }

  async function getRole(id: number): Promise<ApiResponse<ReferenceRole>> {
    return apiClient.get(`/reference-roles/${id}`)
  }

  async function updateRole(id: number, body: UpdateReferenceRoleBody): Promise<ApiResponse<ReferenceRole>> {
    return apiClient.put(`/reference-roles/${id}`, body)
  }

  async function deleteRole(id: number): Promise<ApiResponse<{ deleted: boolean }>> {
    return apiClient.delete(`/reference-roles/${id}`)
  }

  return { listRoles, createRole, getRole, updateRole, deleteRole }
}
