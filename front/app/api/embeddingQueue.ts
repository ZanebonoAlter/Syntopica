import type { ApiResponse } from '~/types'
import { createQueueApi, type QueueApi } from './createQueueApi'

export interface EmbeddingQueueStatus {
  pending: number
  processing: number
  completed: number
  failed: number
  total: number
  /** 今日入队且已完成的标签数（旧后端未合入时可选）。 */
  completed_today?: number
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

const queue: QueueApi<EmbeddingQueueTask> = createQueueApi<EmbeddingQueueTask>('embedding/queue')

export function useEmbeddingQueueApi() {
  return {
    getStatus(): Promise<ApiResponse<EmbeddingQueueStatus>> {
      return queue.getStatus() as Promise<ApiResponse<EmbeddingQueueStatus>>
    },

    getTasks(params?: { status?: string; limit?: number; offset?: number }): Promise<ApiResponse<EmbeddingQueueTasksResponse>> {
      return queue.getTasks(params) as Promise<ApiResponse<EmbeddingQueueTasksResponse>>
    },

    retryFailed(): Promise<ApiResponse<{ message: string }>> {
      return queue.retryFailed()
    },
  }
}
