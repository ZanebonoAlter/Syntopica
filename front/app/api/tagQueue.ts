import { apiClient } from './client'
import type { ApiResponse } from '~/types'
import { createQueueApi, type QueueApi } from './createQueueApi'

export interface TagQueueStatus {
  pending: number
  processing: number
  completed: number
  failed: number
  total: number
  /** 今日 completed 计数（notification-center 新增；旧后端未合入时可选） */
  completed_today?: number
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

const queue: QueueApi<TagQueueTask> = createQueueApi<TagQueueTask>('tag-queue')

export function useTagQueueApi() {
  return {
    getStatus(): Promise<ApiResponse<TagQueueStatus>> {
      return queue.getStatus() as Promise<ApiResponse<TagQueueStatus>>
    },

    getTasks(params?: { status?: string; limit?: number; offset?: number }): Promise<ApiResponse<TagQueueTasksResponse>> {
      return queue.getTasks(params) as Promise<ApiResponse<TagQueueTasksResponse>>
    },

    retryFailed(): Promise<ApiResponse<{ message: string }>> {
      return queue.retryFailed()
    },

    retagToday(): Promise<ApiResponse<{ message: string; data: { total: number; enqueued: number } }>> {
      return apiClient.post<{ message: string; data: { total: number; enqueued: number } }>('/tag-queue/retag-today')
    },
  }
}
