/**
 * 常用类型定义
 */

/**
 * 排序选项
 */
export type SortOption = 'latest' | 'popular' | 'unread'

/**
 * 筛选选项
 */
export type FilterOption = 'all' | 'unread' | 'favorites'

/**
 * 筛选状态
 */
export interface FilterState {
  sort: SortOption
  filter: FilterOption
  category: string | null
  search: string
}

/**
 * 刷新状态类型
 */
export type RefreshStatus = 'idle' | 'refreshing' | 'success' | 'error'

/**
 * 视图模式类型
 */
export type ViewMode = 'preview' | 'iframe'

/**
 * 消息类型（用于 Toast 提示）
 */
export type MessageType = 'success' | 'error' | 'info'

/**
 * 时间范围选项
 */
export interface TimeRangeOption {
  label: string
  value: number
}

/**
 * 刷新定时器数据
 */
export interface RefreshTimer {
  feedId: string
  interval: number
  timer: ReturnType<typeof setInterval> | null
}

/**
 * 生成状态类型
 */
export type GenerationStatus = 'pending' | 'generating' | 'success' | 'failed' | 'timeout'

/**
 * 生成状态项
 */
export interface GenerationStatusItem {
  categoryId: string
  categoryName: string
  status: GenerationStatus
  error?: string
}
