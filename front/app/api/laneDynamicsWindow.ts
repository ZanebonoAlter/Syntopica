/**
 * 泳道动态观察窗 API façade（rework-workshop-into-radar-and-desk 矩阵阶段 G1）。
 *
 * 只读消费既有 `GET /semantic-boards/:id/lane-dynamics?days=N`（后端零改动、零新参数）。
 * 对新数据面（useBoardConsole 的 A/B 双通道）在 API 边界给出 camelCase/字符串 ID 读模型；
 * 同时携带 `legacy` 原始响应，供未迁移的旧矩阵/演进/抽屉消费者沿用（G5 收口后移除）。
 * 失败路径 error/status/message 逐字段透传，不双重断言、不改 shared ApiResponse。
 */
import { useLaneDynamicsApi, type LaneDynamicsResponse } from './laneDynamics'
import { normalizeLaneDynamicsWindow, type LaneDynamicsWindowData } from './normalizers/laneDynamicsWindow'
import type { ApiResponse } from '~/types'

/** 新数据面载荷：归一读模型 + 兼容用原始响应。 */
export interface LaneDynamicsWindowPayload {
  window: LaneDynamicsWindowData
  /** 旧消费者兼容（F10：lane-dynamics DTO 沿用 snake_case 透出的既定约定）。 */
  legacy: LaneDynamicsResponse
}

export type LaneDynamicsWindowResponse
  = { success: true, data: LaneDynamicsWindowPayload, message?: string }
  | { success: false, message?: string, error?: string, status?: number }

/** raw 响应 → 观察窗载荷；失败（含 success 无 data）原样透传错误体。 */
export function mapLaneDynamicsWindowResponse(res: ApiResponse<LaneDynamicsResponse>): LaneDynamicsWindowResponse {
  if (res.success && res.data) {
    return {
      success: true,
      data: { window: normalizeLaneDynamicsWindow(res.data), legacy: res.data },
      message: res.message,
    }
  }
  return { success: false, error: res.error, message: res.message, status: res.status }
}

export function useLaneDynamicsWindowApi() {
  const api = useLaneDynamicsApi()

  /** 观察窗通道取数（只读）：days 仅 7/14/30/365 四档（由调用方约束）。 */
  async function loadLaneDynamics(boardId: number, days: number): Promise<LaneDynamicsWindowResponse> {
    return mapLaneDynamicsWindowResponse(await api.getLaneDynamics(boardId, days))
  }

  return { loadLaneDynamics }
}
