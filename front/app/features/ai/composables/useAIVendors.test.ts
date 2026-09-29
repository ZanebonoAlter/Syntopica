import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AIVendor } from '~/types'

const apiMocks = vi.hoisted(() => ({
  listVendors: vi.fn(),
  createVendor: vi.fn(),
  updateVendor: vi.fn(),
  deleteVendor: vi.fn(),
  refreshVendorModels: vi.fn(),
  testVendor: vi.fn(),
  testVendorInline: vi.fn(),
}))

vi.mock('~/api', () => ({
  useAIAdminApi: () => apiMocks,
}))

import { useAIVendors } from './useAIVendors'

function makeVendor(partial: Partial<AIVendor> & { id: number; name: string }): AIVendor {
  return {
    provider_type: 'openai_compatible',
    base_url: 'http://localhost:8081/v1',
    timeout_seconds: 120,
    thinking_dialect: 'none',
    structured_output: 'json_schema',
    start_command_configured: false,
    api_key_configured: false,
    enabled: true,
    ...partial,
  }
}

describe('useAIVendors', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listVendors.mockResolvedValue({ success: true, data: [] })
  })

  it('fetchVendors 成功填充列表；刷新失败写 error 且保留旧清单（State Matrix：刷新失败 toast 保留旧数据）', async () => {
    const ctx = useAIVendors()
    apiMocks.listVendors.mockResolvedValueOnce({
      success: true,
      data: [makeVendor({ id: 1, name: 'qwythos', models_count: 3, capabilities_count: 5 })],
    })
    await ctx.fetchVendors()
    expect(ctx.vendors.value.map(v => v.name)).toEqual(['qwythos'])
    expect(ctx.error.value).toBeNull()

    apiMocks.listVendors.mockResolvedValueOnce({ success: false, error: 'boom' })
    await ctx.fetchVendors()
    expect(ctx.error.value).toBe('boom')
    expect(ctx.vendors.value.map(v => v.name)).toEqual(['qwythos'])
  })

  it('createVendor 返回后端刷新结果（models_refreshed/added）并刷新列表', async () => {
    const ctx = useAIVendors()
    apiMocks.createVendor.mockResolvedValue({ success: true, data: { id: 9, models_refreshed: true, added: 3 } })

    const result = await ctx.createVendor({ name: 'local', base_url: 'http://localhost:8081/v1' })

    expect(result).toEqual({ id: 9, models_refreshed: true, added: 3 })
    expect(apiMocks.createVendor).toHaveBeenCalledTimes(1)
    // 动作成功后 vendors 缓存刷新（fetchVendors 由 createVendor 内部触发）
    expect(apiMocks.listVendors).toHaveBeenCalledTimes(1)
  })

  it('createVendor 刷新失败不回滚：models_refreshed=false 带 refresh_error 原样返回', async () => {
    const ctx = useAIVendors()
    apiMocks.createVendor.mockResolvedValue({
      success: true,
      data: { id: 9, models_refreshed: false, refresh_error: 'network unreachable' },
    })

    const result = await ctx.createVendor({ name: 'local', base_url: 'http://localhost:8081/v1' })

    expect(result?.models_refreshed).toBe(false)
    expect(result?.refresh_error).toBe('network unreachable')
    expect(ctx.error.value).toBeNull()
  })

  it('updateVendor 请求体原样透传（含 start_command/clear_start_command/clear_api_key）', async () => {
    // 迁移自 useAIRouterSettings「编辑 provider 提交时带 model_kind/start_command/clear_start_command」：
    // 启动命令原文不回显，清除必须显式标记——透传丢了字段就会误清除/误沿用。
    let captured: Record<string, unknown> | undefined
    apiMocks.updateVendor.mockImplementation((_id: number, data: Record<string, unknown>) => {
      captured = { ...data }
      return Promise.resolve({ success: true, data: { id: 2 } })
    })
    const ctx = useAIVendors()

    const ok = await ctx.updateVendor(2, {
      name: 'local-emb',
      base_url: 'http://localhost:8081/v1',
      start_command: 'llama-server -m qwen.gguf --port 8081',
      clear_start_command: true,
      clear_api_key: true,
    })

    expect(ok).toBe(true)
    expect(apiMocks.updateVendor).toHaveBeenCalledWith(2, expect.anything())
    expect(captured).toMatchObject({
      name: 'local-emb',
      start_command: 'llama-server -m qwen.gguf --port 8081',
      clear_start_command: true,
      clear_api_key: true,
    })
  })

  it('removeVendor 返回级联计数（deleted_models/detached_bindings）并刷新列表', async () => {
    // 迁移自 useAIRouterSettings「删除备用模型：后端级联解绑」：删除语义下沉到
    // composable 返回计数，确认弹窗/结果提示由组件层（4.2）用计数展示。
    apiMocks.deleteVendor.mockResolvedValue({ success: true, data: { deleted_models: 3, detached_bindings: 5 } })
    const ctx = useAIVendors()
    await ctx.fetchVendors()

    const result = await ctx.removeVendor(7)

    expect(apiMocks.deleteVendor).toHaveBeenCalledWith(7)
    expect(result).toEqual({ deleted_models: 3, detached_bindings: 5 })
    expect(ctx.error.value).toBeNull()
    expect(apiMocks.listVendors).toHaveBeenCalledTimes(2)
  })

  it('removeVendor：用户取消确认发生在组件层，composable 只在被调用时发请求', async () => {
    // 迁移自 useAIRouterSettings「用户取消确认则不调后端」：确认门禁在组件层，
    // composable 层只需保证「未调用不发请求」。
    const ctx = useAIVendors()
    expect(apiMocks.deleteVendor).not.toHaveBeenCalled()
    void ctx
  })

  it('refreshModels 返回清单 diff（added/stale/visible_count）；失败 error 且不抛', async () => {
    const ctx = useAIVendors()
    apiMocks.refreshVendorModels.mockResolvedValue({
      success: true,
      data: { added: ['qwen3-new'], stale: ['old-model'], visible_count: 4 },
    })
    expect(await ctx.refreshModels(1)).toEqual({ added: ['qwen3-new'], stale: ['old-model'], visible_count: 4 })

    apiMocks.refreshVendorModels.mockResolvedValue({ success: false, error: 'upstream timeout' })
    expect(await ctx.refreshModels(1)).toBeNull()
    expect(ctx.error.value).toBe('upstream timeout')
  })

  it('testConnection 返回后端 message；测试失败是业务结果（success=false）→ null + error', async () => {
    const ctx = useAIVendors()
    apiMocks.testVendor.mockResolvedValue({ success: true, message: '连接成功，共 3 个可用模型', data: { models: [] } })
    expect(await ctx.testConnection(1)).toBe('连接成功，共 3 个可用模型')

    apiMocks.testVendor.mockResolvedValue({ success: false, error: 'connection refused' })
    expect(await ctx.testConnection(1)).toBeNull()
    expect(ctx.error.value).toBe('connection refused')
  })

  it('testInline 透传未保存表单参数（base_url 必填语义在请求体）', async () => {
    const ctx = useAIVendors()
    apiMocks.testVendorInline.mockResolvedValue({ success: true, message: '连接成功', data: { models: [] } })

    const msg = await ctx.testInline({ base_url: 'http://localhost:9999/v1', api_key: 'sk-x' })

    expect(apiMocks.testVendorInline).toHaveBeenCalledWith({ base_url: 'http://localhost:9999/v1', api_key: 'sk-x' })
    expect(msg).toBe('连接成功')
  })
})
