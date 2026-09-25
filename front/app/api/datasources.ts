import { apiClient } from './client'
import type { ApiResponse } from '~/types'

/** 研究数据源目录行（GET /api/datasources 投影） */
export interface DataSourceEntry {
  code: string
  name: string
  provider: string
  homepage_url: string
  coverage: string
  topics: string[]
  frequency: string
  typical_lag: string
  unit_policy: string
  requires_key: boolean
  config_key_name?: string
  status: 'enabled' | 'disabled'
  status_reason?: string
}

/** Comtrade 设置（脱敏回显，同博查语义） */
export interface ComtradeStatus {
  enabled: boolean
  api_key_configured: boolean
  api_key_hint?: string
}

export interface ComtradeConfig {
  enabled: boolean
  /** 空串 = 不修改已有 key，非空才覆盖 */
  api_key: string
}

/** probe 结果（成功形态；失败走 error_code） */
export interface ProbeResult {
  code: string
  elapsed_ms: number
  summary: Record<string, unknown>
  retrieved_at: string
}

export function useDatasourcesApi() {
  async function listCatalog(): Promise<ApiResponse<{ data_sources: DataSourceEntry[] }>> {
    return apiClient.get('/datasources')
  }

  async function probe(code: string): Promise<ApiResponse<ProbeResult>> {
    return apiClient.post(`/datasources/${code}/probe`)
  }

  async function getComtradeStatus(): Promise<ApiResponse<ComtradeStatus>> {
    return apiClient.get('/settings/comtrade')
  }

  async function saveComtradeSettings(config: ComtradeConfig): Promise<ApiResponse<ComtradeStatus>> {
    return apiClient.post('/settings/comtrade', config)
  }

  return {
    listCatalog,
    probe,
    getComtradeStatus,
    saveComtradeSettings,
  }
}
