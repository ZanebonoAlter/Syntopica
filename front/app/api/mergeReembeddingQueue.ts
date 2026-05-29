import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface MergeReembeddingQueueStatus {
	pending: number
	processing: number
	completed: number
	failed: number
	total: number
}

export interface MergeReembeddingQueueTask {
	id: number
	source_tag_id: number
	target_tag_id: number
	status: 'pending' | 'processing' | 'completed' | 'failed'
	error_message: string | null
	retry_count: number
	created_at: string
	started_at: string | null
	completed_at: string | null
	source_tag?: {
		id: number
		label: string
		category: string
		slug: string
	}
	target_tag?: {
		id: number
		label: string
		category: string
		slug: string
	}
}

export interface MergeReembeddingQueueTasksResponse {
	tasks: MergeReembeddingQueueTask[]
	total: number
}

export function useMergeReembeddingQueueApi() {
	return {
		getStatus(): Promise<ApiResponse<MergeReembeddingQueueStatus>> {
			return apiClient.get<MergeReembeddingQueueStatus>('/embedding/merge-reembedding/status')
		},

		getTasks(params?: { status?: string; limit?: number; offset?: number }): Promise<ApiResponse<MergeReembeddingQueueTasksResponse>> {
			const query = new URLSearchParams()
			if (params?.status) query.append('status', params.status)
			if (params?.limit) query.append('limit', String(params.limit))
			if (params?.offset) query.append('offset', String(params.offset))

			const qs = query.toString()
			const endpoint = qs ? `/embedding/merge-reembedding/tasks?${qs}` : '/embedding/merge-reembedding/tasks'
			return apiClient.get<MergeReembeddingQueueTasksResponse>(endpoint)
		},

		retryFailed(): Promise<ApiResponse<{ message: string }>> {
			return apiClient.post<{ message: string }>('/embedding/merge-reembedding/retry')
		},
	}
}
