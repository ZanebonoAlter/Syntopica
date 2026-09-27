import { apiClient } from './client'
import type {
  ApiResponse,
  CreateFeedData,
  FeedBoardHitStats,
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

  /**
   * 全量订阅源窗口内入板块命中统计（add-source-board-hit-rate，只读）。
   * window 仅允许 7/30/90；响应 data 外层为 { items } 包裹。
   */
  async function getBoardHitStats(windowDays: number): Promise<ApiResponse<{ items: FeedBoardHitStats[] }>> {
    return apiClient.get<{ items: FeedBoardHitStats[] }>(`/feeds/board-hit-stats?window=${windowDays}`)
  }

  return {
    getFeeds,
    getBoardHitStats,
    fetchFeed,
    createFeed,
    updateFeed,
    deleteFeed,
    refreshFeed,
    refreshAllFeeds,
  }
}
