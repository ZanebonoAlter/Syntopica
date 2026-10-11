import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface SearxngConfig {
  endpoint: string
  enabled: boolean
}

/** 连通性探测结果四态（add-searxng-health-indicator）：ok/degraded=服务可达
 *  （后者引擎全挂无结果）、unreachable=endpoint 拨不通、disabled=未配置/未启用。 */
export interface SearxngHealth {
  status: 'ok' | 'degraded' | 'unreachable' | 'disabled'
  latency_ms: number
  detail: string
}

export function useSearxngApi() {
  async function getConfig(): Promise<ApiResponse<SearxngConfig>> {
    return apiClient.get('/settings/searxng')
  }

  async function saveSettings(config: SearxngConfig): Promise<ApiResponse<null>> {
    return apiClient.post('/settings/searxng', config)
  }

  /** 连通性主动探测：真实探针搜索（~秒级），按需触发。 */
  async function checkHealth(): Promise<ApiResponse<SearxngHealth>> {
    return apiClient.get('/settings/searxng/health')
  }

  return {
    getConfig,
    saveSettings,
    checkHealth,
  }
}
