import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface EmbeddingQueueStatus {
  pending: number
  processing: number
  completed: number
  failed: number
  total: number
}

export interface EmbeddingQueueTask {
  id: number
  tag_id: number
  status: 'pending' | 'processing' | 'completed' | 'failed'
  error_message: string | null
  retry_count: number
  created_at: string
  started_at: string | null
  completed_at: string | null
  tag?: {
    id: number
    label: string
    category: string
    slug: string
  }
}

export interface EmbeddingQueueTasksResponse {
  tasks: EmbeddingQueueTask[]
  total: number
}

export function useEmbeddingQueueApi() {
  return {
    getStatus(): Promise<ApiResponse<EmbeddingQueueStatus>> {
      return apiClient.get<EmbeddingQueueStatus>('/embedding/queue/status')
    },

    getTasks(params?: { status?: string; limit?: number; offset?: number }): Promise<ApiResponse<EmbeddingQueueTasksResponse>> {
      const query = new URLSearchParams()
      if (params?.status) query.append('status', params.status)
      if (params?.limit) query.append('limit', String(params.limit))
      if (params?.offset) query.append('offset', String(params.offset))

      const qs = query.toString()
      const endpoint = qs ? `/embedding/queue/tasks?${qs}` : '/embedding/queue/tasks'
      return apiClient.get<EmbeddingQueueTasksResponse>(endpoint)
    },

    retryFailed(): Promise<ApiResponse<{ message: string }>> {
      return apiClient.post<{ message: string }>('/embedding/queue/retry')
    },
  }
}
