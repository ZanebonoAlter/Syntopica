import { apiClient } from './client'
import type { ApiResponse, AIProvider, AIRoute, AIProviderUpsertRequest, AIHealthSnapshot, AIHealthReprobeResult } from '~/types'

export function useAIAdminApi() {
  async function getSettings(): Promise<ApiResponse<Record<string, unknown>>> {
    return apiClient.get('/ai/settings')
  }

  async function saveSettings(data: {
    base_url?: string
    api_key?: string
    model?: string
    /** 日报生成时刻（HH:MM，restructure-settings-navigation D6：定时任务面板复用本端点） */
    daily_report_time?: string
  }): Promise<ApiResponse<void>> {
    return apiClient.post('/ai/settings', data)
  }

  async function testConnection(data: {
    base_url?: string
    api_key?: string
    model?: string
    provider_type?: string
    provider_id?: number
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

  async function getHealth(): Promise<ApiResponse<AIHealthSnapshot>> {
    return apiClient.get('/ai/health')
  }

  async function reprobeHealth(): Promise<ApiResponse<AIHealthReprobeResult>> {
    return apiClient.post('/ai/health/reprobe')
  }

  async function setAutoStartModels(enabled: boolean): Promise<ApiResponse<{ enabled: boolean }>> {
    return apiClient.put('/ai/health/auto-start-models', { enabled })
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
    getHealth,
    reprobeHealth,
    setAutoStartModels,
  }
}
