import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * ApiClient.request 的错误信封契约（board-level-deep-analysis 5.5 A）：
 *  - !ok 时不得丢后端 data（409 冲突体携带当前任务身份，前端据此恢复轮询）
 *  - 错误信封带 status（HTTP 状态码）
 *  - 成功路径透传 data / message / 顶层额外字段（不回归）
 */

const fetchMock = vi.fn()

vi.mock('~/utils/api', () => ({
  getApiBaseUrl: () => 'http://localhost:5000',
}))

const { apiClient } = await import('./client')

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ApiClient.request — 错误信封保留 status 与后端 data', () => {
  it('409 冲突体：success=false + status=409 + data 原样保留', async () => {
    const conflict = {
      success: false,
      error: 'board analysis already running',
      data: {
        job_id: 'abc123def456',
        job_kind: 'board_brief',
        scope: 'board',
        target_id: 31,
        running: true,
        started_at: '2026-09-01T00:00:00Z',
        finished: false,
      },
    }
    fetchMock.mockResolvedValueOnce(jsonResponse(conflict, 409))

    const res = await apiClient.post<{ job_id: string; job_kind: string }>('/semantic-boards/31/enrichment/analysis/trigger')

    expect(res.success).toBe(false)
    expect(res.status).toBe(409)
    expect(res.error).toBe('board analysis already running')
    expect(res.data).toEqual(conflict.data)
    expect(res.data?.job_id).toBe('abc123def456')
    expect(res.data?.job_kind).toBe('board_brief')
  })

  it('普通错误（无 data）：success=false + status，data 为 undefined 不崩', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ success: false, error: 'enrichment not enabled for this board' }, 400))

    const res = await apiClient.get('/semantic-boards/31/enrichment/analysis/results')

    expect(res.success).toBe(false)
    expect(res.status).toBe(400)
    expect(res.error).toBe('enrichment not enabled for this board')
    expect(res.data).toBeUndefined()
  })

  it('成功：透传 data / message 与顶层额外字段（extras 不回归）', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({
      success: true,
      data: { items: [1, 2] },
      message: 'ok',
      analysis_paused: true,
    }))

    const res = await apiClient.get<{ items: number[] }>('/example')

    expect(res.success).toBe(true)
    expect(res.data).toEqual({ items: [1, 2] })
    expect(res.message).toBe('ok')
    expect((res as unknown as Record<string, unknown>).analysis_paused).toBe(true)
  })

  it('网络异常（fetch reject）：success=false，无 status 不崩', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'))

    const res = await apiClient.get('/example')

    expect(res.success).toBe(false)
    expect(res.error).toBe('Failed to fetch')
    expect(res.status).toBeUndefined()
  })
})
