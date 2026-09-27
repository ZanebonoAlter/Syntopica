import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface SearxngConfig {
  endpoint: string
  enabled: boolean
}

export function useSearxngApi() {
  async function getConfig(): Promise<ApiResponse<SearxngConfig>> {
    return apiClient.get('/settings/searxng')
  }

  async function saveSettings(config: SearxngConfig): Promise<ApiResponse<null>> {
    return apiClient.post('/settings/searxng', config)
  }

  return {
    getConfig,
    saveSettings,
  }
}
