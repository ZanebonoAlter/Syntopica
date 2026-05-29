/**
 * API 请求和响应类型定义
 */

/**
 * API 通用响应格式
 */
export interface ApiResponse<T = unknown> {
  success: boolean
  data?: T
  pagination?: PaginationMeta
  message?: string
  error?: string
}

/**
 * 分页参数
 */
export interface PaginationParams {
  page?: number
  per_page?: number
}

/**
 * 分页元数据
 */
export interface PaginationMeta {
  page: number
  pages: number
  per_page: number
  total: number
}

/**
 * 分页响应数据（Go 后端格式）
 */
export interface PaginatedData<T> {
  items: T[]
  pagination: PaginationMeta
}

/**
 * 分页 API 响应
 */
export interface PaginatedApiResponse<T = unknown> {
  success: boolean
  data: T[]
  pagination: PaginationMeta
  message?: string
  error?: string
}
