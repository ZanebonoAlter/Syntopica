import { apiClient } from './client'
import type { ApiResponse, CreateCategoryData, UpdateCategoryData, Category } from '~/types'

interface CategoryPayload {
  id: number
  name: string
  slug: string
  icon: string
  color: string
  description: string
  feed_count: number
}

export function normalizeCategory(cat: CategoryPayload): Category {
  return {
    id: String(cat.id),
    name: cat.name || '',
    slug: cat.slug || (cat.name || '').toLowerCase().replace(/\s+/g, '-'),
    icon: cat.icon || 'mdi:folder',
    color: cat.color || '#6b7280',
    description: cat.description || '',
    feedCount: cat.feed_count || 0,
  }
}

export function useCategoriesApi() {
  async function getCategories(): Promise<ApiResponse<Category[]>> {
    const response = await apiClient.get<CategoryPayload[]>('/categories')
    if (response.success && response.data) {
      return {
        ...response,
        data: response.data.map(normalizeCategory),
      }
    }
    return response as unknown as ApiResponse<Category[]>
  }

  async function createCategory(data: CreateCategoryData): Promise<ApiResponse<Category>> {
    const response = await apiClient.post<CategoryPayload>('/categories', data)
    if (response.success && response.data) {
      return { ...response, data: normalizeCategory(response.data) }
    }
    return response as unknown as ApiResponse<Category>
  }

  async function updateCategory(id: number, data: UpdateCategoryData): Promise<ApiResponse<Category>> {
    const response = await apiClient.put<CategoryPayload>(`/categories/${id}`, data)
    if (response.success && response.data) {
      return { ...response, data: normalizeCategory(response.data) }
    }
    return response as unknown as ApiResponse<Category>
  }

  async function deleteCategory(id: number): Promise<ApiResponse<void>> {
    return apiClient.delete<void>(`/categories/${id}`)
  }

  return {
    getCategories,
    createCategory,
    updateCategory,
    deleteCategory,
  }
}
