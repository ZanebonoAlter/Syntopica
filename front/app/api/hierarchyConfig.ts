import { apiClient } from './client'
import type { ApiResponse } from '~/types'

export interface HierarchyTemplate {
  category: string
  sub_type: string
  max_level: number
  levels: HierarchyLevel[]
}

export interface HierarchyLevel {
  level: number
  name: string
  description: string
  is_leaf: boolean
  max_children: number
  forbidden_patterns: string[]
}

export interface HierarchyConfig {
  templates: HierarchyTemplate[]
  version: number
}

export interface HierarchyPendingChange {
  id: number
  tag_id: number
  tag_label: string
  change_type: string
  current_parent_id: number | null
  current_parent_label: string
  reason: string
  status: string
  created_at: string
  resolved_at: string | null
  tag: { id: number; label: string; category: string } | null
}

export interface PendingChangeApprovalResult {
  id: number
  tag_id: number
  change_type: string
  status: 'resolved' | 'failed' | string
  reason?: string
}

export interface PendingChangeApprovalResponse {
  approved: number
  failed: number
  results: PendingChangeApprovalResult[]
}

export interface ConfigImpact {
  total_tags: number
  depth_exceeded: number
  level_mismatch: number
  cross_category: number
  new_leaf_violations: number
  details: ConfigImpactDetail[]
}

export interface HierarchyClosureStatus {
  category: string
  active_sector_count: number
  unplaced_tag_count: number
  pending_change_count: number
  active_rebuild_job?: RebuildJob | null
  blocker_counts: Record<string, number>
  top_blocker?: string
}

export interface ConfigImpactDetail {
  tag_id: number
  tag_label: string
  category: string
  issue: string
  depth: number
  parent_id: number | null
}

export interface RebuildJob {
  id: number
  category: string
  trigger: string
  status: 'pending' | 'running' | 'paused' | 'completed' | 'failed'
  total_tags: number
  processed_tags: number
  failed_tags: number
  estimated_end: string | null
  started_at: string | null
  completed_at: string | null
  last_tag_id: number
  error_detail: string
  created_at: string
}

export function useHierarchyConfigApi() {
  return {
    async getConfig(): Promise<ApiResponse<HierarchyConfig>> {
      return apiClient.get<HierarchyConfig>('/hierarchy/config')
    },

    async updateConfig(templates: HierarchyTemplate[], changeLog?: string): Promise<ApiResponse<{ version: number; impact: ConfigImpact }>> {
      return apiClient.put<{ version: number; impact: ConfigImpact }>('/hierarchy/config', {
        templates,
        change_log: changeLog,
      })
    },

    async previewConfig(templates: HierarchyTemplate[], changeLog?: string): Promise<ApiResponse<{ version: number; impact: ConfigImpact; preview_only: boolean }>> {
      return apiClient.post<{ version: number; impact: ConfigImpact; preview_only: boolean }>('/hierarchy/config/preview', {
        templates,
        change_log: changeLog,
      })
    },

    async applyConfig(templates: HierarchyTemplate[], changeLog?: string): Promise<ApiResponse<{ version: number; impact: ConfigImpact; preview_only: boolean; changed_categories: string[]; rebuild_jobs: RebuildJob[] }>> {
      return apiClient.put<{ version: number; impact: ConfigImpact; preview_only: boolean; changed_categories: string[]; rebuild_jobs: RebuildJob[] }>('/hierarchy/config', {
        templates,
        change_log: changeLog,
        mode: 'apply',
        apply: true,
      })
    },

    async getClosureStatus(category: string): Promise<ApiResponse<HierarchyClosureStatus>> {
      const params = new URLSearchParams()
      params.set('category', category)
      return apiClient.get<HierarchyClosureStatus>(`/hierarchy/closure-status?${params.toString()}`)
    },

    async getPending(status?: string): Promise<ApiResponse<HierarchyPendingChange[]>> {
      const params = status ? `?status=${status}` : ''
      return apiClient.get<HierarchyPendingChange[]>(`/hierarchy/pending${params}`)
    },

    async triggerRebuild(category?: string, dryRun?: boolean): Promise<ApiResponse<{ total_pending: number; processed: number; dry_run?: boolean }>> {
      return apiClient.post<{ total_pending: number; processed: number; dry_run?: boolean }>('/hierarchy/rebuild', {
        category,
        dry_run: dryRun,
      })
    },

    async startRebuild(category: string, trigger?: string): Promise<ApiResponse<RebuildJob>> {
      return apiClient.post<RebuildJob>('/hierarchy/rebuild/start', { category, trigger })
    },

    async getRebuildStatus(id: number): Promise<ApiResponse<RebuildJob>> {
      return apiClient.get<RebuildJob>(`/hierarchy/rebuild/${id}`)
    },

    async approvePendingChanges(data: { ids?: number[]; approve_all?: boolean; category?: string }): Promise<ApiResponse<PendingChangeApprovalResponse>> {
      return apiClient.post<PendingChangeApprovalResponse>('/hierarchy/pending/approve', data)
    },
  }
}
