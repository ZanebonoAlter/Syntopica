import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface TagQueueStatus {
  pending: number
  processing: number
  completed: number
  failed: number
  total: number
}

export interface TagQueueTask {
  id: number
  article_id: number
  article_title: string
  feed_name_snapshot: string
  category_name_snapshot: string
  priority: number
  status: 'pending' | 'leased' | 'completed' | 'failed'
  attempt_count: number
  max_attempts: number
  force_retag: boolean
  reason: string
  last_error: string
  created_at: string
  leased_at: string | null
}

export interface TagQueueTasksResponse {
  tasks: TagQueueTask[]
  total: number
}

export function useTagQueueApi() {
  return {
    getStatus(): Promise<ApiResponse<TagQueueStatus>> {
      return apiClient.get<TagQueueStatus>('/tag-queue/status')
    },

    getTasks(params?: { status?: string; limit?: number; offset?: number }): Promise<ApiResponse<TagQueueTasksResponse>> {
      const query = new URLSearchParams()
      if (params?.status) query.append('status', params.status)
      if (params?.limit) query.append('limit', String(params.limit))
      if (params?.offset) query.append('offset', String(params.offset))

      const qs = query.toString()
      const endpoint = qs ? `/tag-queue/tasks?${qs}` : '/tag-queue/tasks'
      return apiClient.get<TagQueueTasksResponse>(endpoint)
    },

    retryFailed(): Promise<ApiResponse<{ message: string }>> {
      return apiClient.post<{ message: string }>('/tag-queue/retry')
    },

    retagToday(): Promise<ApiResponse<{ message: string; data: { total: number; enqueued: number } }>> {
      return apiClient.post<{ message: string; data: { total: number; enqueued: number } }>('/tag-queue/retag-today')
    },
  }
}
