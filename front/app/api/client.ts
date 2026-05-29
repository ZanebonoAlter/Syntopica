import { getApiBaseUrl } from '~/utils/api'
import type { ApiResponse } from '~/types'

let currentTraceId: string | null = null

function generateSpanId(): string {
  const bytes = new Uint8Array(8)
  crypto.getRandomValues(bytes)
  return Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join('')
}

function traceHeaders(): Record<string, string> {
  if (!currentTraceId) return {}
  return { traceparent: `00-${currentTraceId}-${generateSpanId()}-01` }
}

function captureTraceId(response: Response) {
  const tp = response.headers.get('traceparent')
  if (tp) {
    const parts = tp.split('-')
    if (parts.length >= 2) {
      currentTraceId = parts[1] ?? null
      console.debug('[trace]', currentTraceId)
    }
  }
}

class ApiClient {
  private baseURL?: string

  constructor(baseURL?: string) {
    this.baseURL = baseURL
  }

  private resolveBaseURL(): string {
    return this.baseURL || getApiBaseUrl()
  }

  private async request<T>(endpoint: string, options: RequestInit = {}): Promise<ApiResponse<T>> {
    try {
      const url = `${this.resolveBaseURL()}${endpoint}`
      const response = await fetch(url, {
        ...options,
        headers: {
          'Content-Type': 'application/json',
          ...traceHeaders(),
          ...options.headers,
        },
      })

      captureTraceId(response)
      const data = await response.json()

      if (!response.ok) {
        return {
          success: false,
          error: data.error || data.message || '请求失败',
          message: data.message,
        }
      }

      return {
        success: true,
        data: data.data,
        pagination: data.pagination,
        message: data.message,
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '网络错误',
      }
    }
  }

  async get<T>(endpoint: string, options?: RequestInit): Promise<ApiResponse<T>> {
    return this.request<T>(endpoint, { ...options, method: 'GET' })
  }

  async post<T>(endpoint: string, data?: unknown, options?: RequestInit): Promise<ApiResponse<T>> {
    return this.request<T>(endpoint, {
      ...options,
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  async put<T>(endpoint: string, data?: unknown, options?: RequestInit): Promise<ApiResponse<T>> {
    return this.request<T>(endpoint, {
      ...options,
      method: 'PUT',
      body: JSON.stringify(data),
    })
  }

  async delete<T>(endpoint: string, options?: RequestInit): Promise<ApiResponse<T>> {
    return this.request<T>(endpoint, { ...options, method: 'DELETE' })
  }

  async upload<T>(endpoint: string, formData: FormData): Promise<ApiResponse<T>> {
    try {
      const url = `${this.resolveBaseURL()}${endpoint}`
      const response = await fetch(url, {
        method: 'POST',
        body: formData,
        headers: {
          ...traceHeaders(),
        },
      })

      captureTraceId(response)
      const data = await response.json()

      if (!response.ok) {
        return {
          success: false,
          error: data.error || data.message || '上传失败',
        }
      }

      return {
        success: true,
        data: data.data || data,
        message: data.message,
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : '网络错误',
      }
    }
  }

  async download(endpoint: string): Promise<Blob> {
    const url = `${this.resolveBaseURL()}${endpoint}`
    const response = await fetch(url, {
      headers: {
        ...traceHeaders(),
      },
    })
    captureTraceId(response)
    if (!response.ok) {
      throw new Error('下载失败')
    }
    return response.blob()
  }

  buildQueryParams(params: unknown): string {
    const searchParams = new URLSearchParams()
    if (params && typeof params === 'object') {
      Object.entries(params as Record<string, unknown>).forEach(([key, value]) => {
      if (value !== undefined && value !== null) {
        searchParams.append(key, String(value))
      }
    })
    }
    return searchParams.toString()
  }
}

export const apiClient = new ApiClient()
