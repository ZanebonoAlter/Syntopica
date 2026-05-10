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

export interface ConfigImpact {
  total_tags: number
  depth_exceeded: number
  level_mismatch: number
  cross_category: number
  new_leaf_violations: number
  details: ConfigImpactDetail[]
}

export interface ConfigImpactDetail {
  tag_id: number
  tag_label: string
  category: string
  issue: string
  depth: number
  parent_id: number | null
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
  }
}
