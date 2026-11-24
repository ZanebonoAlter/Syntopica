import { apiClient } from './client'
import type { ApiResponse } from '~/types'

/**
 * 统一网页搜索 API（switch-margin-note-search-to-deepseek design D6）：
 * 页边注问答的联网后端由本地自建搜索实例换成 DeepSeek Search API，凭据只认 AI
 * 供应商注册表里的 DeepSeek vendor——所以这里**没有保存端点**，面板只读关联
 * 状态 + 按需探测（同一把 key 不允许两处可改）。
 */

/** 关联状态：vendor_configured=false 时联网参考关闭（问答本身不受影响）。 */
export interface WebSearchSettings {
  vendor_configured: boolean
  vendor_name: string
}

/** 探测四态（settings-workspace spec：disabled / unreachable / degraded / ok）。 */
export type WebSearchHealthStatus = 'disabled' | 'unreachable' | 'degraded' | 'ok'

/** 探测失败类别（后端 websearch.ErrorKind；仅 unreachable 时非空）。 */
export type WebSearchErrorKind =
  | 'not_configured'
  | 'invalid_query'
  | 'auth'
  | 'quota'
  | 'rate_limited'
  | 'timeout'
  | 'unavailable'

/** 探测结果：四态 + 耗时 + 人话说明，unreachable 时带 error_kind 供面板分流文案。 */
export interface WebSearchHealth {
  status: WebSearchHealthStatus
  latency_ms: number
  detail: string
  error_kind?: WebSearchErrorKind
}

interface WebSearchSettingsPayload {
  vendor_configured?: unknown
  vendor_name?: unknown
}

interface WebSearchHealthPayload {
  status?: unknown
  latency_ms?: unknown
  detail?: unknown
  error_kind?: unknown
}

const HEALTH_STATUSES: WebSearchHealthStatus[] = ['disabled', 'unreachable', 'degraded', 'ok']

const ERROR_KINDS: WebSearchErrorKind[] = [
  'not_configured',
  'invalid_query',
  'auth',
  'quota',
  'rate_limited',
  'timeout',
  'unavailable',
]

/** 缺字段按「未关联」处理（旧部署无 vendor 时前端走空态指引，不谎报已配置）。 */
function normalizeSettings(payload: WebSearchSettingsPayload): WebSearchSettings {
  return {
    vendor_configured: payload.vendor_configured === true,
    vendor_name: typeof payload.vendor_name === 'string' ? payload.vendor_name : '',
  }
}

/** 集外 status 按 unreachable 归一：探测结果不认识时不能声称「健康」，
 *  面板据此走通用不可用文案（error_kind 缺省即「搜索服务暂不可用」）。 */
function normalizeHealth(payload: WebSearchHealthPayload): WebSearchHealth {
  const status = HEALTH_STATUSES.includes(payload.status as WebSearchHealthStatus)
    ? (payload.status as WebSearchHealthStatus)
    : 'unreachable'
  const errorKind = ERROR_KINDS.includes(payload.error_kind as WebSearchErrorKind)
    ? (payload.error_kind as WebSearchErrorKind)
    : undefined
  return {
    status,
    latency_ms: typeof payload.latency_ms === 'number' && payload.latency_ms >= 0 ? payload.latency_ms : 0,
    detail: typeof payload.detail === 'string' ? payload.detail : '',
    error_kind: errorKind,
  }
}

export function useWebSearchApi() {
  /** 关联状态读取（无保存端点：密钥在「AI 模型」分区的 DeepSeek 供应商里管理）。 */
  async function getSettings(): Promise<ApiResponse<WebSearchSettings>> {
    const response = await apiClient.get<WebSearchSettingsPayload>('/settings/web-search')
    if (!response.success || !response.data) return response as ApiResponse<WebSearchSettings>
    return { ...response, data: normalizeSettings(response.data) }
  }

  /** 连通性主动探测：真实探针搜索（~秒级），按需触发、不轮询。 */
  async function getHealth(): Promise<ApiResponse<WebSearchHealth>> {
    const response = await apiClient.get<WebSearchHealthPayload>('/settings/web-search/health')
    if (!response.success || !response.data) return response as ApiResponse<WebSearchHealth>
    return { ...response, data: normalizeHealth(response.data) }
  }

  return {
    getSettings,
    getHealth,
  }
}
