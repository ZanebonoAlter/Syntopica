/**
 * 阅读行为相关类型定义
 */

import type { ApiResponse } from './api'

export type ReadingEventType = 'open' | 'close' | 'scroll' | 'favorite' | 'unfavorite'

/**
 * 阅读行为事件
 */
export interface ReadingBehaviorEvent {
  article_id: number
  feed_id: number
  category_id?: number
  session_id: string
  event_type: ReadingEventType
  scroll_depth?: number
  reading_time?: number
}

/**
 * 批量行为事件请求
 */
export interface BatchBehaviorRequest {
  events: ReadingBehaviorEvent[]
}

/**
 * 阅读统计数据
 */
export interface ReadingStats {
  total_articles: number
  total_reading_time: number
  avg_reading_time: number
  avg_scroll_depth: number
  most_active_feed_id: number
  most_active_category: number
}

/**
 * 用户偏好数据
 */
export interface UserPreference {
  id: number
  feed_id?: number
  category_id?: number
  preference_score: number
  avg_reading_time: number
  interaction_count: number
  scroll_depth_avg: number
  last_interaction_at?: string
  created_at: string
  updated_at: string
  feed_title?: string
  category_name?: string
}

/**
 * 用户偏好列表响应
 */
export type UserPreferencesResponse = ApiResponse<UserPreference[]>
