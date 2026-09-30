import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AIRoute } from '~/types'

const apiMocks = vi.hoisted(() => ({
  listRoutes: vi.fn(),
  saveRoute: vi.fn(),
}))

vi.mock('~/api', () => ({
  useAIAdminApi: () => apiMocks,
}))

import { capabilityOrder, routeDescriptions, routeLabels, useAIRoutes } from './useAIRoutes'

function makeRoute(partial: Partial<AIRoute> & { capability: string }): AIRoute {
  return {
    id: 10,
    name: 'default',
    enabled: true,
    description: '',
    max_concurrency: 0,
    route_models: [],
    ...partial,
  }
}

describe('useAIRoutes', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listRoutes.mockResolvedValue({ success: true, data: [] })
  })

  it('routeByCapability 按 capability 建索引，每能力一条绑定链', async () => {
    const ctx = useAIRoutes()
    apiMocks.listRoutes.mockResolvedValueOnce({
      success: true,
      data: [
        makeRoute({ id: 10, capability: 'summary' }),
        makeRoute({ id: 11, capability: 'embedding', max_concurrency: 5 }),
      ],
    })
    await ctx.fetchRoutes()

    expect(ctx.routeByCapability.value.summary?.id).toBe(10)
    expect(ctx.routeByCapability.value.embedding?.max_concurrency).toBe(5)
    expect(ctx.routeByCapability.value.digest_polish).toBeUndefined()
  })

  it('saveRoute 成功后刷新路由缓存；saving 锁贯穿请求并复位', async () => {
    let releaseSave: ((v: unknown) => void) | undefined
    apiMocks.saveRoute.mockImplementation(() => new Promise(resolve => { releaseSave = resolve }))
    const ctx = useAIRoutes()

    const pending = ctx.saveRoute('summary', { bindings: [{ model_id: 1 }] })
    expect(ctx.saving.value).toBe(true)
    releaseSave?.({ success: true })
    await pending
    expect(ctx.saving.value).toBe(false)
    expect(apiMocks.saveRoute).toHaveBeenCalledWith('summary', { bindings: [{ model_id: 1 }] })
    // 保存成功后 refetch（顺位/绑定以后端真值为准）
    expect(apiMocks.listRoutes).toHaveBeenCalledTimes(1)
    expect(ctx.error.value).toBeNull()
  })

  it('saveRoute 失败写 error（后端校验文案透传）返回 false，saving 复位', async () => {
    const ctx = useAIRoutes()
    apiMocks.saveRoute.mockResolvedValue({ success: false, error: '模型「x」当前不可绑定（stale 或未启用）' })

    const ok = await ctx.saveRoute('summary', { bindings: [{ model_id: 99 }] })

    expect(ok).toBe(false)
    expect(ctx.error.value).toBe('模型「x」当前不可绑定（stale 或未启用）')
    expect(ctx.saving.value).toBe(false)
  })

  it('capabilityOrder 是渲染白名单：含数据增强两条与 open_notebook（历史遗漏已补）', () => {
    // 迁移自 useAIRouterSettings 白名单用例：这仨 capability 的 route 早已存在于
    // 后端，前端白名单历史性遗漏导致设置页看不到、无法单独配链。
    expect(capabilityOrder).toContain('data_enrichment_news')
    expect(capabilityOrder).toContain('data_enrichment_analysis')
    expect(capabilityOrder).toContain('open_notebook')
    expect(routeLabels.data_enrichment_news).toBe('新闻总结')
    expect(routeLabels.data_enrichment_analysis).toBe('数据分析')
    expect(routeLabels.open_notebook).toBe('页边注问答')
  })

  it('capabilityOrder 顺序稳定且五核心能力在前（渲染顺序契约）', () => {
    expect(capabilityOrder).toEqual([
      'summary', 'topic_tagging', 'digest_polish', 'embedding', 'open_notebook',
      'feed_discovery', 'data_enrichment_news', 'data_enrichment_analysis',
    ])
  })

  it('routeDescriptions 覆盖白名单全部能力且非空（行头供血说明）', () => {
    for (const cap of capabilityOrder) {
      expect(routeDescriptions[cap], `capability ${cap} 缺供血说明`).toBeTruthy()
      expect(routeDescriptions[cap]!.trim()).not.toBe('')
    }
  })

  it('关键分工文案与真实供血一致（spec：新闻总结=月/年归档非日报，日报润色含日报+近14天）', () => {
    // data_enrichment_news 供血 lifeline 周期归档（summarize_context，泳道趋势月/年档），
    // 与日报无关——文案绝不能暗示日报，否则回到「蒙蔽」原点。
    expect(routeDescriptions.data_enrichment_news).toContain('月')
    expect(routeDescriptions.data_enrichment_news).toContain('年')
    expect(routeDescriptions.data_enrichment_news).toContain('归档')
    expect(routeDescriptions.data_enrichment_news).not.toContain('日报')
    // digest_polish 两个消费方：日报生成 + 泳道趋势近14天档（lane_snapshot）
    expect(routeDescriptions.digest_polish).toContain('日报')
    expect(routeDescriptions.digest_polish).toContain('14')
  })
})
