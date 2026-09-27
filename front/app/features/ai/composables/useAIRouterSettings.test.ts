import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AIProvider } from '~/types'

const apiMocks = vi.hoisted(() => ({
  getSettings: vi.fn(),
  listProviders: vi.fn(),
  listRoutes: vi.fn(),
  createProvider: vi.fn(),
  updateProvider: vi.fn(),
  deleteProvider: vi.fn(),
  updateRoute: vi.fn(),
  testConnection: vi.fn(),
}))

vi.mock('~/api', () => ({
  useAIAdminApi: () => apiMocks,
}))

vi.mock('~/composables/useAI', () => ({
  useAI: () => ({ loadSettings: vi.fn() }),
}))

// fix-provider-delete-route-deadlock 2b：确认弹窗改走全局 useConfirm（promise 式），
// 原生 confirm 的 stubGlobal 方式废弃；用例内用 confirmMock.mockResolvedValue 控制确认/取消。
const confirmMock = vi.hoisted(() => vi.fn())

vi.mock('~/composables/useConfirm', () => ({
  useConfirm: () => ({ confirm: confirmMock }),
}))

import { useAIRouterSettings } from './useAIRouterSettings'

function makeProvider(partial: Partial<AIProvider> & { id: number; name: string }): AIProvider {
  return {
    provider_type: 'openai_compatible',
    base_url: 'http://localhost:8081/v1',
    model: 'm',
    model_kind: 'llm',
    start_command_configured: false,
    enabled: true,
    timeout_seconds: 120,
    enable_thinking: false,
    api_key_configured: false,
    ...partial,
  }
}

describe('useAIRouterSettings（model_kind）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.getSettings.mockResolvedValue({ success: true, data: {} })
    apiMocks.listProviders.mockResolvedValue({ success: true, data: [] })
    apiMocks.listRoutes.mockResolvedValue({ success: true, data: [] })
  })

  it('按 capability 过滤候选：embedding 路由只列 embedding provider，其余只列 llm', () => {
    const ctx = useAIRouterSettings()
    ctx.providers.value = [
      makeProvider({ id: 1, name: 'primary-llm' }),
      makeProvider({ id: 2, name: 'backup-llm' }),
      makeProvider({ id: 3, name: 'backup-emb', model_kind: 'embedding' }),
    ]
    ctx.primaryProviderId.value = 1

    expect(ctx.candidatesForCapability('embedding').map(p => p.name)).toEqual(['backup-emb'])
    expect(ctx.candidatesForCapability('summary').map(p => p.name)).toEqual(['backup-llm'])
    expect(ctx.primaryAllowedForCapability('summary')).toBe(true)
    expect(ctx.primaryAllowedForCapability('embedding')).toBe(false)
  })

  it('主 provider 为 embedding 时，只允许挂到 embedding 路由', () => {
    const ctx = useAIRouterSettings()
    ctx.providers.value = [makeProvider({ id: 9, name: 'primary-emb', model_kind: 'embedding' })]
    ctx.primaryProviderId.value = 9

    expect(ctx.primaryAllowedForCapability('embedding')).toBe(true)
    expect(ctx.primaryAllowedForCapability('digest_polish')).toBe(false)
  })

  it('编辑 provider 提交时带 model_kind/start_command/clear_start_command', async () => {
    // 表单是 reactive 对象且保存成功后会重置，需在 mock 里快照入参再断言
    let captured: Record<string, unknown> | undefined
    apiMocks.updateProvider.mockImplementation((_id: number, data: Record<string, unknown>) => {
      captured = { ...data }
      return Promise.resolve({ success: true, data: { id: 2 } })
    })
    const ctx = useAIRouterSettings()
    ctx.startEditingProvider(makeProvider({ id: 2, name: 'backup-emb', model_kind: 'embedding', start_command_configured: true }))
    ctx.editProviderForm.start_command = 'llama-server -m qwen.gguf --port 8081'
    ctx.editProviderForm.clear_start_command = true

    await ctx.saveEditedProvider()

    expect(apiMocks.updateProvider).toHaveBeenCalledWith(2, expect.anything())
    expect(captured).toMatchObject({
      model_kind: 'embedding',
      start_command: 'llama-server -m qwen.gguf --port 8081',
      clear_start_command: true,
    })
  })

  it('新建 provider 默认 model_kind=llm，提交后表单重置回默认', async () => {
    let captured: Record<string, unknown> | undefined
    apiMocks.createProvider.mockImplementation((data: Record<string, unknown>) => {
      captured = { ...data }
      return Promise.resolve({ success: true, data: { id: 5 } })
    })
    const ctx = useAIRouterSettings()
    expect(ctx.newProviderForm.model_kind).toBe('llm')

    ctx.newProviderForm.name = 'local-emb'
    ctx.newProviderForm.base_url = 'http://localhost:8082/v1'
    ctx.newProviderForm.model = 'bge-m3'
    ctx.newProviderForm.model_kind = 'embedding'
    ctx.newProviderForm.start_command = 'llama-server --port 8082'

    await ctx.saveNewProvider()

    expect(captured).toMatchObject({
      model_kind: 'embedding',
      start_command: 'llama-server --port 8082',
    })
    expect(ctx.newProviderForm.model_kind).toBe('llm')
    expect(ctx.newProviderForm.start_command).toBe('')
  })

  it('能力路由显式包含数据增强两条 capability（新闻总结 / 数据分析）', () => {
    // 这俩 capability 的 route 早已存在于后端，但前端 capabilityOrder 白名单
    // 历史性遗漏，导致设置页能力路由 UI 不显示、用户无法为「数据分析」单独配强模型。
    const ctx = useAIRouterSettings()
    expect(ctx.capabilityOrder).toContain('data_enrichment_news')
    expect(ctx.capabilityOrder).toContain('data_enrichment_analysis')
    expect(ctx.routeLabels.data_enrichment_news).toBe('新闻总结')
    expect(ctx.routeLabels.data_enrichment_analysis).toBe('数据分析')
  })

  it('能力路由显式包含 open_notebook（页边注问答，daily-report-margin-notes）', () => {
    // 后端 UpsertRoute 已支持 open_notebook 且无门槛；前端白名单此前漏列，
    // 导致页边注问答无法在设置页配 provider 链。
    const ctx = useAIRouterSettings()
    expect(ctx.capabilityOrder).toContain('open_notebook')
    expect(ctx.routeLabels.open_notebook).toBe('页边注问答')
  })

  it('保存 LLM 主模型：同步挂载到所有 LLM 能力路由首位，且绝不碰 embedding 路由', async () => {
    // 主模型的语义是「同类型能力的默认首选」。同步循环历史上无脑挂全部路由，
    // 旧后端把 LLM 挂上过 embedding 路由（脏数据），带类型校验的后端则直接报
    // 「同步 向量嵌入 主路由失败」导致主模型存不进去。
    apiMocks.updateProvider.mockResolvedValue({ success: true, data: { id: 1 } })
    apiMocks.updateRoute.mockResolvedValue({ success: true })
    apiMocks.listProviders.mockResolvedValue({
      success: true,
      data: [
        makeProvider({ id: 1, name: 'qwen' }),
        makeProvider({ id: 2, name: 'backup-llm' }),
        makeProvider({ id: 3, name: 'qwen3-embedding', model_kind: 'embedding' }),
      ],
    })
    apiMocks.listRoutes.mockResolvedValue({
      success: true,
      data: [
        { id: 10, name: 'default', capability: 'summary', enabled: true, route_providers: [{ provider_id: 2, priority: 1 }] },
        { id: 11, name: 'default', capability: 'embedding', enabled: true, route_providers: [{ provider_id: 3, priority: 1 }] },
      ],
    })

    const ctx = useAIRouterSettings()
    await ctx.loadData()
    ctx.primaryProviderForm.name = 'qwen'
    ctx.primaryProviderForm.base_url = 'http://localhost:8081/v1'
    ctx.primaryProviderForm.model = 'qwen3'
    ctx.primaryProviderForm.model_kind = 'llm'

    await ctx.savePrimaryProvider()

    const syncedCaps = apiMocks.updateRoute.mock.calls.map(call => call[0])
    expect(syncedCaps).not.toContain('embedding')
    expect(syncedCaps).toEqual(expect.arrayContaining(['summary', 'topic_tagging', 'digest_polish', 'feed_discovery', 'data_enrichment_news', 'data_enrichment_analysis']))
    // summary 同步后首位是主模型 id=1，原备挂 id=2 保持在后；embedding 路由完全不被触碰
    const summaryCall = apiMocks.updateRoute.mock.calls.find(call => call[0] === 'summary')!
    expect(summaryCall[1].provider_ids).toEqual([1, 2])
  })

  it('删除备用模型：后端级联解绑，成功后 loadData 重建且不再本地改线路选择', async () => {
    // fix-provider-delete-route-deadlock：旧实现先调删除接口（挂线路时被后端 409
    // 拦下抛错），后面的本地解绑循环永远执行不到；现在后端删除时自动解绑，
    // 前端只依赖 loadData() 重建，不得再手动改 routeSelections。确认弹窗走 useConfirm。
    confirmMock.mockResolvedValue(true)
    apiMocks.deleteProvider.mockResolvedValue({ success: true, message: 'provider deleted (detached from 2 route(s))' })

    const ctx = useAIRouterSettings()
    await ctx.loadData()
    ctx.routeSelections.value = { summary: [7] }
    const removeSpy = vi.spyOn(ctx, 'removeProviderFromRoute')

    await ctx.deleteBackupProvider(makeProvider({ id: 7, name: ' doomed ' }))

    const confirmOptions = confirmMock.mock.calls[0]?.[0] ?? {}
    expect(confirmOptions.message).toContain('解绑')
    expect(confirmOptions.danger).toBe(true)
    expect(apiMocks.deleteProvider).toHaveBeenCalledWith(7)
    expect(removeSpy).not.toHaveBeenCalled()
    expect(ctx.error.value).toBeNull()
    // loadData 被重新拉取（线路选择用后端真值重建，恢复为空默认值）
    expect(ctx.routeSelections.value.summary).toEqual([])
  })

  it('删除备用模型：用户取消确认则不调后端', async () => {
    confirmMock.mockResolvedValue(false)
    const ctx = useAIRouterSettings()
    await ctx.loadData()

    await ctx.deleteBackupProvider(makeProvider({ id: 7, name: 'x' }))

    expect(apiMocks.deleteProvider).not.toHaveBeenCalled()
  })
})
