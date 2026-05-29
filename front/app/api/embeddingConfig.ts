import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface EmbeddingConfigItem {
  id: number
  key: string
  value: string
  description: string
  created_at: string
  updated_at: string
}

export function useEmbeddingConfigApi() {
  return {
    async getConfig(): Promise<ApiResponse<EmbeddingConfigItem[]>> {
      return apiClient.get<EmbeddingConfigItem[]>('/embedding/config')
    },

    async updateConfig(key: string, value: string): Promise<ApiResponse<void>> {
      return apiClient.put<void>(`/embedding/config/${key}`, { value })
    },
  }
}
