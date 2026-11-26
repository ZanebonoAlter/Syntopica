import { apiClient } from './client'
import { normalizeArticleDetection, type ArticlePayload } from './normalizers/article'
import type {
  ApiResponse,
  PaginatedData,
  Article,
  ArticleFilters,
  UpdateArticleData,
  BulkUpdateArticlesData,
} from '~/types'

export function useArticlesApi() {
  async function getArticles(filters: ArticleFilters = {}): Promise<ApiResponse<PaginatedData<Article>>> {
    const query = apiClient.buildQueryParams(filters)
    return apiClient.get<PaginatedData<Article>>(`/articles${query ? `?${query}` : ''}`)
  }

  /**
   * 文章详情（读路径）。
   *
   * 判定四字段的 snake_case → camelCase 映射在本函数内收口（reading.md 约束 1：
   * camelCase 映射只发生在 API/normalizer 层）。调用方直接消费 `aiDetectionStatus`
   * 等 camelCase 键，**不得**再对 `ai_detection_*` 做一次映射（review R-3）。
   *
   * 其余 snake_case 字段仍由调用方的 `normalizeArticle` 统一归一，本次不改那层。
   */
  async function getArticle(id: number): Promise<ApiResponse<Article>> {
    const response = await apiClient.get<ArticlePayload>(`/articles/${id}`)
    if (!response.data) return response as unknown as ApiResponse<Article>
    return {
      ...response,
      data: {
        ...(response.data as unknown as Article),
        ...normalizeArticleDetection(response.data),
      },
    }
  }

  /** 显式重新判定（design D10/D14）：入队一条 manual_recheck 作业，绕过「人工结论优先」的跳过。 */
  async function recheckAIDetection(id: number): Promise<ApiResponse<{ job_id: number; article_id: number; status: string }>> {
    return apiClient.post<{ job_id: number; article_id: number; status: string }>(`/articles/${id}/ai-detection/recheck`)
  }

  async function updateArticle(id: number, data: UpdateArticleData): Promise<ApiResponse<Article>> {
    return apiClient.put<Article>(`/articles/${id}`, data)
  }

  async function retagArticle(id: number): Promise<ApiResponse<{ job_id: number; article_id: number; status: string }>> {
    return apiClient.post<{ job_id: number; article_id: number; status: string }>(`/articles/${id}/tags`)
  }

  async function bulkUpdateArticles(data: BulkUpdateArticlesData): Promise<ApiResponse<void>> {
    return apiClient.put<void>('/articles/bulk-update', data)
  }

  async function getArticlesStats(): Promise<ApiResponse<{ total: number; unread: number; favorite: number }>> {
    return apiClient.get<{ total: number; unread: number; favorite: number }>('/articles/stats')
  }

  return {
    getArticles,
    getArticle,
    updateArticle,
    recheckAIDetection,
    retagArticle,
    bulkUpdateArticles,
    getArticlesStats,
  }
}
