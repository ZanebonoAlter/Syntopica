import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getMock } = vi.hoisted(() => ({ getMock: vi.fn() }))

vi.mock('./client', () => ({
  apiClient: { get: getMock },
}))

import { useWebSearchApi } from './webSearch'

/**
 * 统一网页搜索 API（switch-margin-note-search-to-deepseek）：
 * 只有两个 GET（关联状态 / 连通性探测），URL 路径与后端路由一一对应，
 * 无保存端点（密钥在「AI 模型」分区的供应商里管理）。
 */
describe('useWebSearchApi', () => {
  beforeEach(() => {
    getMock.mockReset()
  })

  it('getSettings 走 /settings/web-search 并归一关联状态', async () => {
    getMock.mockResolvedValue({
      success: true,
      data: { vendor_configured: true, vendor_name: 'DeepSeek' },
    })

    const response = await useWebSearchApi().getSettings()

    expect(getMock).toHaveBeenCalledWith('/settings/web-search')
    expect(response.data).toEqual({ vendor_configured: true, vendor_name: 'DeepSeek' })
  })

  it('getSettings 缺字段时归一为未关联（旧部署不谎报已配置）', async () => {
    getMock.mockResolvedValue({ success: true, data: {} })

    const response = await useWebSearchApi().getSettings()

    expect(response.data).toEqual({ vendor_configured: false, vendor_name: '' })
  })

  it('getHealth 走 /settings/web-search/health 并透传四态与耗时', async () => {
    getMock.mockResolvedValue({
      success: true,
      data: { status: 'ok', latency_ms: 460, detail: '搜索正常，返回 10 条结果' },
    })

    const response = await useWebSearchApi().getHealth()

    expect(getMock).toHaveBeenCalledWith('/settings/web-search/health')
    expect(response.data).toEqual({ status: 'ok', latency_ms: 460, detail: '搜索正常，返回 10 条结果', error_kind: undefined })
  })

  it('getHealth 保留 unreachable 的 error_kind，过滤集外值', async () => {
    getMock.mockResolvedValueOnce({
      success: true,
      data: { status: 'unreachable', latency_ms: 8000, detail: '余额不足', error_kind: 'quota' },
    })
    const quota = await useWebSearchApi().getHealth()
    expect(quota.data?.error_kind).toBe('quota')

    getMock.mockResolvedValueOnce({
      success: true,
      data: { status: 'unreachable', latency_ms: 1, detail: '', error_kind: 'teapot' },
    })
    const unknown = await useWebSearchApi().getHealth()
    expect(unknown.data?.error_kind).toBeUndefined()
  })

  it('getHealth 集外 status 归一为 unreachable、缺 latency 落 0', async () => {
    getMock.mockResolvedValue({ success: true, data: { status: 'melted', detail: null } })

    const response = await useWebSearchApi().getHealth()

    expect(response.data).toEqual({ status: 'unreachable', latency_ms: 0, detail: '', error_kind: undefined })
  })

  it('失败响应原样透传（不伪造 data）', async () => {
    getMock.mockResolvedValue({ success: false, error: '后端炸了' })

    const response = await useWebSearchApi().getSettings()

    expect(response.success).toBe(false)
    expect(response.error).toBe('后端炸了')
    expect(response.data).toBeUndefined()
  })
})
