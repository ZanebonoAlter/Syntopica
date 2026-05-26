import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface DailyReportHighlight {
  title: string
  reason: string
  tag_ids: number[]
}

export interface DailyReportThread {
  title: string
  summary: string
  status: string
  related_tag_ids: number[]
  related_article_ids: number[]
  parent_thread_id: string
}

export interface DailyReportSection {
  id: number
  cluster_index: number
  cluster_label: string
  cluster_tag_ids: number[]
  threads: DailyReportThread[]
  article_count: number
}

export interface DailyReport {
  id: number
  semantic_board_id: number
  period_date: string
  title: string
  summary: string
  status: string
  cluster_count: number
  article_count: number
  event_tag_count: number
  highlights: DailyReportHighlight[]
  dynamics: string
  sections: DailyReportSection[]
  created_at: string
}

export interface DailyReportListItem {
  id: number
  semantic_board_id: number
  period_date: string
  title: string
  summary: string
  status: string
  cluster_count: number
  article_count: number
  event_tag_count: number
  created_at: string
}

export function useDailyReportsApi() {
  async function generateDailyReport(params: { date: string; board_id?: number }) {
    return apiClient.post<{ job_id: string; status: string }>('/daily-reports/generate', params)
  }

  async function getBoardDailyReports(boardId: number, params?: { days?: number }): Promise<ApiResponse<{ reports: DailyReportListItem[] }>> {
    const query = params ? apiClient.buildQueryParams(params) : ''
    return apiClient.get(`/semantic-boards/${boardId}/daily-reports${query ? `?${query}` : ''}`)
  }

  async function getDailyReportDetail(id: number): Promise<ApiResponse<{ report: DailyReport }>> {
    return apiClient.get(`/daily-reports/${id}`)
  }

  return {
    generateDailyReport,
    getBoardDailyReports,
    getDailyReportDetail,
  }
}
