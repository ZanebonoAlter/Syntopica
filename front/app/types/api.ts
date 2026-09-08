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
  /** HTTP 状态码（仅失败时由 client 填充；成功路径恒 2xx 不需要）。409 冲突等场景前端需按状态码 + data 恢复。 */
  status?: number
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
