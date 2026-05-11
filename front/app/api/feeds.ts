import { apiClient } from './client'
import type {
  ApiResponse,
  CreateFeedData,
  PaginationParams,
  RssFeed,
  UpdateFeedData,
} from '~/types'

export function useFeedsApi() {
  async function getFeeds(params: PaginationParams = {}): Promise<ApiResponse<RssFeed[]>> {
    const query = apiClient.buildQueryParams(params)
    return apiClient.get<RssFeed[]>(`/feeds${query ? `?${query}` : ''}`)
  }

  async function fetchFeed(url: string): Promise<ApiResponse<Record<string, unknown>>> {
    return apiClient.post('/feeds/fetch', { url })
  }

  async function createFeed(data: CreateFeedData): Promise<ApiResponse<RssFeed>> {
    return apiClient.post<RssFeed>('/feeds', data)
  }

  async function updateFeed(id: number, data: UpdateFeedData): Promise<ApiResponse<RssFeed>> {
    return apiClient.put<RssFeed>(`/feeds/${id}`, data)
  }

  async function deleteFeed(id: number): Promise<ApiResponse<void>> {
    return apiClient.delete<void>(`/feeds/${id}`)
  }

  async function refreshFeed(id: number): Promise<ApiResponse<{ message?: string }>> {
    return apiClient.post<{ message?: string }>(`/feeds/${id}/refresh`)
  }

  async function refreshAllFeeds(): Promise<ApiResponse<{ message?: string }>> {
    return apiClient.post<{ message?: string }>('/feeds/refresh-all')
  }

  return {
    getFeeds,
    fetchFeed,
    createFeed,
    updateFeed,
    deleteFeed,
    refreshFeed,
    refreshAllFeeds,
  }
}
