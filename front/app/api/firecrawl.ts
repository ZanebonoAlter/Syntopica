import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface FirecrawlStatus {
  enabled: boolean
  api_url: string
  mode: string
  timeout: number
  max_content_length: number
  api_key_configured: boolean
}

export interface FirecrawlConfig {
  enabled: boolean
  api_url: string
  api_key: string
  mode: string
  timeout: number
  max_content_length: number
}

export function useFirecrawlApi() {
  async function crawlArticle(id: number): Promise<ApiResponse<{
    firecrawl_content: string
    firecrawl_status: string
  }>> {
    return apiClient.post(`/firecrawl/article/${id}`)
  }

  async function enableFeedFirecrawl(id: number, enabled: boolean): Promise<ApiResponse<{
    firecrawl_enabled: boolean
  }>> {
    return apiClient.post(`/firecrawl/feed/${id}/enable`, { enabled })
  }

  async function getStatus(): Promise<ApiResponse<FirecrawlStatus>> {
    return apiClient.get('/firecrawl/status')
  }

  async function saveSettings(config: FirecrawlConfig): Promise<ApiResponse<FirecrawlStatus>> {
    return apiClient.post('/firecrawl/settings', config)
  }

  return {
    crawlArticle,
    enableFeedFirecrawl,
    getStatus,
    saveSettings,
  }
}
