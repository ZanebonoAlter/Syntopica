import { apiClient } from './client'
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

  async function getArticle(id: number): Promise<ApiResponse<Article>> {
    return apiClient.get<Article>(`/articles/${id}`)
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
    retagArticle,
    bulkUpdateArticles,
    getArticlesStats,
  }
}
