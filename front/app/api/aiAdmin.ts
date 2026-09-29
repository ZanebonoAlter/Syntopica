import { apiClient } from './client'
import type {
  ApiResponse,
  AIModel,
  AIModelUpdateRequest,
  AIRoute,
  AIVendor,
  AIVendorCreateResult,
  AIVendorUpsertRequest,
  AIHealthSnapshot,
  AIHealthReprobeResult,
  DegradationReport,
  RouteSaveRequest,
  VendorDeleteResult,
  VendorModelsRefreshResult,
} from '~/types'

/**
 * AI 管理端点（ai-provider-model-overhaul D7：vendor/model/route 三层 + 观测，
 * 旧 /ai/providers* 端点原子删除，无兼容窗口。字段对齐
 * backend-go/internal/admin/handler/ai_handler.go）
 */

/** 测试连接响应 data（成功时 message 带「共 N 个可用模型」） */
interface VendorTestResult {
  models: string[]
}

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

  // ---- Vendor ----

  async function listVendors(): Promise<ApiResponse<AIVendor[]>> {
    return apiClient.get('/ai/vendors')
  }

  async function createVendor(data: AIVendorUpsertRequest): Promise<ApiResponse<AIVendorCreateResult>> {
    return apiClient.post('/ai/vendors', data)
  }

  async function updateVendor(id: number, data: AIVendorUpsertRequest): Promise<ApiResponse<{ id: number }>> {
    return apiClient.put(`/ai/vendors/${id}`, data)
  }

  async function deleteVendor(id: number): Promise<ApiResponse<VendorDeleteResult>> {
    return apiClient.delete(`/ai/vendors/${id}`)
  }

  async function refreshVendorModels(id: number): Promise<ApiResponse<VendorModelsRefreshResult>> {
    return apiClient.post(`/ai/vendors/${id}/models/refresh`)
  }

  /** 按已保存配置（含库内密钥）测试连通性；失败为业务结果（success=false + error） */
  async function testVendor(id: number): Promise<ApiResponse<VendorTestResult>> {
    return apiClient.post(`/ai/vendors/${id}/test`)
  }

  /** 未保存表单的内联测试：不走库、不落库，base_url 必填 */
  async function testVendorInline(data: {
    base_url: string
    api_key?: string
    provider_type?: string
    timeout_seconds?: number
  }): Promise<ApiResponse<VendorTestResult>> {
    return apiClient.post('/ai/vendors/test', data)
  }

  // ---- Model ----

  async function listModels(): Promise<ApiResponse<AIModel[]>> {
    return apiClient.get('/ai/models')
  }

  async function updateModel(id: number, data: AIModelUpdateRequest): Promise<ApiResponse<AIModel>> {
    return apiClient.put(`/ai/models/${id}`, data)
  }

  // ---- Route ----

  async function listRoutes(): Promise<ApiResponse<AIRoute[]>> {
    return apiClient.get('/ai/routes')
  }

  async function saveRoute(capability: string, data: RouteSaveRequest): Promise<ApiResponse<void>> {
    return apiClient.put(`/ai/routes/${encodeURIComponent(capability)}`, data)
  }

  // ---- Observability ----

  async function getDegradations(window: '24h' | '7d', capability?: string): Promise<ApiResponse<DegradationReport>> {
    const query = apiClient.buildQueryParams({ window, capability: capability || undefined })
    return apiClient.get(`/ai/observability/degradations${query ? `?${query}` : ''}`)
  }

  // ---- Health ----

  async function getHealth(): Promise<ApiResponse<AIHealthSnapshot>> {
    return apiClient.get('/ai/health')
  }

  async function reprobeHealth(): Promise<ApiResponse<AIHealthReprobeResult>> {
    return apiClient.post('/ai/health/reprobe')
  }

  async function setAutoStartModels(enabled: boolean): Promise<ApiResponse<{ enabled: boolean }>> {
    return apiClient.put('/ai/health/auto-start-models', { enabled })
  }

  return {
    getSettings,
    saveSettings,
    listVendors,
    createVendor,
    updateVendor,
    deleteVendor,
    refreshVendorModels,
    testVendor,
    testVendorInline,
    listModels,
    updateModel,
    listRoutes,
    saveRoute,
    getDegradations,
    getHealth,
    reprobeHealth,
    setAutoStartModels,
  }
}
