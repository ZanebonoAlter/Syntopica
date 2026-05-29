import type { ApiResponse } from '~/types'
import type { SchedulerStatus, SchedulerTriggerResult } from '~/types/scheduler'
import { apiClient } from './client'

async function triggerSchedulerRequest(name: string, params?: Record<string, string>): Promise<ApiResponse<SchedulerTriggerResult>> {
  const query = params ? '?' + new URLSearchParams(params).toString() : ''
  return apiClient.post<SchedulerTriggerResult>(`/schedulers/${name}/trigger${query}`, {})
}

export function useSchedulerApi() {
  return {
    async getSchedulersStatus() {
      return apiClient.get<SchedulerStatus[]>('/schedulers/status')
    },

    async getSchedulerStatus(name: string) {
      return apiClient.get<SchedulerStatus>(`/schedulers/${name}/status`)
    },

    async triggerScheduler(name: string, params?: Record<string, string>) {
      return triggerSchedulerRequest(name, params)
    },

    async resetSchedulerStats(name: string) {
      return apiClient.post<{ message: string }>(`/schedulers/${name}/reset-stats`)
    },
  }
}
