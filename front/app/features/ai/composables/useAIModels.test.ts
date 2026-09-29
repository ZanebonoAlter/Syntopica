import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AIModel } from '~/types'

const apiMocks = vi.hoisted(() => ({
  listModels: vi.fn(),
  updateModel: vi.fn(),
}))

vi.mock('~/api', () => ({
  useAIAdminApi: () => apiMocks,
}))

import { useAIModels } from './useAIModels'

let nextId = 1
function makeModel(partial: Partial<AIModel> & { name: string }): AIModel {
  const id = partial.id ?? nextId++
  return {
    id,
    vendor_id: 1,
    model_kind: 'llm',
    thinking_level: null,
    supports_xhigh_max: false,
    enabled: true,
    status: 'ok',
    last_seen_at: null,
    vendor: { id: 1, name: 'qwythos', provider_type: 'openai_compatible', base_url: 'http://x/v1' },
    usable: true,
    ...partial,
  }
}

describe('useAIModels', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listModels.mockResolvedValue({ success: true, data: [] })
  })

  it('usableModels 只保留 usable 行（stale/停用/供应商停用均不可绑）', async () => {
    // 迁移自 useAIRouterSettings 候选过滤语义：绑定候选必须排除 stale 与停用模型
    // （W9-7：stale/disabled 模型不出现在绑定候选）。
    const ctx = useAIModels()
    apiMocks.listModels.mockResolvedValueOnce({
      success: true,
      data: [
        makeModel({ name: 'ok-llm' }),
        makeModel({ name: 'stale-llm', status: 'stale', usable: false }),
        makeModel({ name: 'disabled-llm', enabled: false, usable: false }),
      ],
    })
    await ctx.fetchModels()

    expect(ctx.usableModels.value.map(m => m.name)).toEqual(['ok-llm'])
  })

  it('usableModelsForCapability：embedding 路由只列 embedding 模型，其余能力只列 llm', async () => {
    const ctx = useAIModels()
    apiMocks.listModels.mockResolvedValueOnce({
      success: true,
      data: [
        makeModel({ name: 'chat-llm' }),
        makeModel({ name: 'bge-m3', model_kind: 'embedding', vendor_id: 2, vendor: { id: 2, name: 'local-emb', provider_type: 'ollama', base_url: 'http://y/v1' } }),
        makeModel({ name: 'stale-emb', model_kind: 'embedding', status: 'stale', usable: false }),
      ],
    })
    await ctx.fetchModels()

    expect(ctx.usableModelsForCapability('embedding').map(m => m.name)).toEqual(['bge-m3'])
    expect(ctx.usableModelsForCapability('summary').map(m => m.name)).toEqual(['chat-llm'])
    expect(ctx.usableModelsForCapability('data_enrichment_analysis').map(m => m.name)).toEqual(['chat-llm'])
  })

  it('byVendor 按 vendor 分组（chips 数据源），缺 vendor 摘要的行跳过', async () => {
    const ctx = useAIModels()
    apiMocks.listModels.mockResolvedValueOnce({
      success: true,
      data: [
        makeModel({ name: 'a', vendor_id: 2, vendor: { id: 2, name: 'beta', provider_type: 'ollama', base_url: '' } }),
        makeModel({ name: 'b' }),
        makeModel({ name: 'c', vendor_id: 1 }),
        makeModel({ name: 'orphan', vendor: undefined, usable: false }),
      ],
    })
    await ctx.fetchModels()

    const groups = ctx.byVendor.value
    expect(groups.map(g => g.vendor.name)).toEqual(['qwythos', 'beta'])
    expect(groups[0]?.models.map(m => m.name)).toEqual(['b', 'c'])
    expect(groups[1]?.models.map(m => m.name)).toEqual(['a'])
  })

  it('updateModel 透传配置（thinking_level 串清除语义）并刷新缓存', async () => {
    let captured: Record<string, unknown> | undefined
    apiMocks.updateModel.mockImplementation((_id: number, data: Record<string, unknown>) => {
      captured = { ...data }
      return Promise.resolve({ success: true, data: {} })
    })
    const ctx = useAIModels()
    await ctx.fetchModels()

    const ok = await ctx.updateModel(5, { enabled: false, thinking_level: '' })

    expect(ok).toBe(true)
    expect(captured).toEqual({ enabled: false, thinking_level: '' })
    // 成功后 refetch（保持 vendor 摘要与 usable 布尔为后端真值）
    expect(apiMocks.listModels).toHaveBeenCalledTimes(2)
  })

  it('updateModel 失败写 error 返回 false，不抛异常', async () => {
    apiMocks.updateModel.mockResolvedValue({ success: false, error: 'embedding 路由不能挂 llm 模型' })
    const ctx = useAIModels()

    const ok = await ctx.updateModel(5, { model_kind: 'llm' })

    expect(ok).toBe(false)
    expect(ctx.error.value).toBe('embedding 路由不能挂 llm 模型')
  })
})
