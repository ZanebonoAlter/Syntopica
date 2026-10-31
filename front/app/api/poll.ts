import type { ApiResponse } from '~/types'
import type { SchedulerAIHealthRoute, SchedulerStatus } from '~/types/scheduler'
import { apiClient } from './client'
import type { EmbeddingQueueStatus } from './embeddingQueue'
import type { TagQueueStatus } from './tagQueue'

/**
 * GET /api/poll —— 常驻状态类数据的单一批量对账端点（client-poll-budget 契约）。
 * 一次请求同时返回：调度器状态（含 analysis_paused / ai_healthy 顶层语义）、
 * 标签队列计数、embedding 队列计数、通知未读数。分项端点（/schedulers/status
 * 等）保留不删，供已打开的旧标签页继续工作。
 *
 * 两个队列的量纲不同：tag_queue 单位是**文章**，embedding_queue 单位是**标签**
 * （embedding_queues 行按 tag_id 排）——展示时必须分别标注。
 */
export interface PollBundleData {
  schedulers: SchedulerStatus[]
  tag_queue: TagQueueStatus
  embedding_queue?: EmbeddingQueueStatus
  notifications: { unread: number }
}

export type PollBundleResponse = ApiResponse<PollBundleData> & {
  analysis_paused?: boolean
  analysis_paused_at?: string
  ai_healthy?: boolean
  ai_health_routes?: SchedulerAIHealthRoute[]
  server_time?: string
}

export function usePollApi() {
  return {
    async getPollBundle(): Promise<PollBundleResponse> {
      return apiClient.get<PollBundleData>('/poll') as Promise<PollBundleResponse>
    },
  }
}
