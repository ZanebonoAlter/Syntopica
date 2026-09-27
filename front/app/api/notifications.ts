import { apiClient } from './client'
import type { ApiResponse } from '~/types'

/**
 * 通知中心 API（notification-center，单用户无鉴权）
 * 契约对齐 change spec：未读数 / 分页列表（unread 过滤）/ 单条标已读 / 全部标已读 / 清空
 */

export interface AppNotification {
  id: string
  type: 'success' | 'error'
  title: string
  summary: string
  link_type: string | null
  link_id: string | null
  is_read: boolean
  created_at: string
}

export interface NotificationListResponse {
  notifications: AppNotification[]
  total: number
}

/** 后端原始行（id/link_id 为数字）→ 前端字符串 ID（红线：数字 ID 在 API 边界转 string） */
function normalizeNotification(raw: Record<string, unknown>): AppNotification {
  return {
    id: String(raw.id ?? ''),
    type: raw.type === 'error' ? 'error' : 'success',
    title: String(raw.title ?? ''),
    summary: String(raw.summary ?? ''),
    link_type: raw.link_type == null ? null : String(raw.link_type),
    link_id: raw.link_id == null ? null : String(raw.link_id),
    is_read: Boolean(raw.is_read),
    created_at: String(raw.created_at ?? ''),
  }
}

export function useNotificationsApi() {
  return {
    list(params?: { limit?: number; offset?: number; unreadOnly?: boolean }): Promise<ApiResponse<{ notifications: AppNotification[]; total: number }>> {
      const qs = new URLSearchParams()
      if (params?.limit != null) qs.set('limit', String(params.limit))
      if (params?.offset != null) qs.set('offset', String(params.offset))
      if (params?.unreadOnly) qs.set('unread', 'true')
      const suffix = qs.toString() ? `?${qs.toString()}` : ''
      // 红线：数字 ID 在 API 边界转 string（normalizeNotification 统一做边界转换）
      return apiClient.get<{ notifications: Record<string, unknown>[]; total: number }>(`/notifications${suffix}`).then((response) => {
        if (!response.success || !response.data) {
          return { success: false, error: response.error } as ApiResponse<{ notifications: AppNotification[]; total: number }>
        }
        return {
          success: true,
          data: {
            notifications: (response.data.notifications ?? []).map(normalizeNotification),
            total: response.data.total,
          },
        } as ApiResponse<{ notifications: AppNotification[]; total: number }>
      })
    },

    unreadCount(): Promise<ApiResponse<{ unread: number }>> {
      return apiClient.get('/notifications/unread-count')
    },

    markRead(id: string): Promise<ApiResponse<{ message?: string }>> {
      return apiClient.post(`/notifications/${encodeURIComponent(id)}/read`)
    },

    markAllRead(): Promise<ApiResponse<{ message?: string }>> {
      return apiClient.post('/notifications/read-all')
    },

    clearAll(): Promise<ApiResponse<{ message?: string }>> {
      return apiClient.delete('/notifications')
    },
  }
}

export type { AppNotification as NotificationItem }
