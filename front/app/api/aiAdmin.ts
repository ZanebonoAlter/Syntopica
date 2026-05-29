import { apiClient } from './client'
import type { ApiResponse, AIProvider, AIRoute, AIProviderUpsertRequest } from '~/types'

export function useAIAdminApi() {
  async function getSettings(): Promise<ApiResponse<Record<string, unknown>>> {
    return apiClient.get('/ai/settings')
  }

  async function saveSettings(data: {
    base_url?: string
    api_key: string
    model?: string
    narrative_board_embedding_threshold?: number
  }): Promise<ApiResponse<void>> {
    return apiClient.post('/ai/settings', data)
  }

  async function testConnection(data: {
    base_url: string
    api_key?: string
    model: string
    provider_type?: string
  }): Promise<ApiResponse<void>> {
    return apiClient.post('/ai/test', data)
  }

  async function listProviders(): Promise<ApiResponse<AIProvider[]>> {
    return apiClient.get('/ai/providers')
  }

  async function createProvider(data: AIProviderUpsertRequest): Promise<ApiResponse<{ id: number }>> {
    return apiClient.post('/ai/providers', data)
  }

  async function updateProvider(id: number, data: AIProviderUpsertRequest): Promise<ApiResponse<{ id: number }>> {
    return apiClient.put(`/ai/providers/${id}`, data)
  }

  async function deleteProvider(id: number): Promise<ApiResponse<void>> {
    return apiClient.delete(`/ai/providers/${id}`)
  }

  async function listRoutes(): Promise<ApiResponse<AIRoute[]>> {
    return apiClient.get('/ai/routes')
  }

  async function updateRoute(capability: string, data: {
    name?: string
    enabled?: boolean
    description?: string
    provider_ids: number[]
  }): Promise<ApiResponse<void>> {
    return apiClient.put(`/ai/routes/${capability}`, data)
  }

  return {
    getSettings,
    saveSettings,
    testConnection,
    listProviders,
    createProvider,
    updateProvider,
    deleteProvider,
    listRoutes,
    updateRoute,
  }
}
