import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { DegradationEvent } from '~/types'

/** useAIAdminApi mock：getDegradations 返回可控 report */
const apiMocks = vi.hoisted(() => ({
  getDegradations: vi.fn(),
}))
vi.mock('~/api/aiAdmin', () => ({
  useAIAdminApi: () => apiMocks,
}))

import { useRouteHealth } from './useRouteHealth'

function ev(partial: Partial<DegradationEvent> & { capability: string }): DegradationEvent {
  return {
    occurred_at: '2026-09-29T02:14:33Z',
    from: 0,
    to: 1,
    model: 'deepseek-flash',
    type: 'fallback',
    code: 'timeout',
    ...partial,
  }
}

function report(events: DegradationEvent[]) {
  return { success: true, data: { window: '24h', stats: { calls: 10, degradations: events.length, failures: 0 }, events } }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useRouteHealth（tasks 8.3：泳道降级标注聚合，FB-5）', () => {
  it('fetchHealth 只保留 fallback 事件（structured 不进泳道标注）', async () => {
    apiMocks.getDegradations.mockResolvedValue(report([
      ev({ capability: 'summary' }),
      ev({ capability: 'summary', type: 'structured', from: null, to: null, code: 'json_schema→json_object' }),
    ]))
    const ctx = useRouteHealth()
    await ctx.fetchHealth()
    expect(apiMocks.getDegradations).toHaveBeenCalledWith('24h')
    expect(ctx.loaded.value).toBe(true)
    // 卡级聚合只算 fallback：structured 事件不计 fail/takeover（迁移事件命中 deepseek-flash 在 to 位）
    const h = ctx.cardHealth('summary', 0, ['qwythos', 'deepseek-flash'])
    expect(h.fail).toBe(1)
  })

  it('cardHealth：迁移事件需 to 位卡名与命中模型一致才计入两侧（防换绑错标）；全链失败需 from 位卡名吻合', async () => {
    apiMocks.getDegradations.mockResolvedValue(report([
      ev({ capability: 'summary', from: 0, to: 1, model: 'qwythos', code: 'timeout' }),
      ev({ capability: 'summary', from: 0, to: 1, model: 'qwythos', code: 'empty_response' }),
      // 换绑后旧事件：命中模型不在 to 位（当前 to 位是 another-model）→ 整体不标
      ev({ capability: 'summary', from: 1, to: 2, model: 'qwythos', code: 'stale-event' }),
    ]))
    const ctx = useRouteHealth()
    await ctx.fetchHealth()

    const names = ['deepseek-flash', 'qwythos', 'another-model']
    // 顺位 1（失败源）：前两事件结构吻合 → 2 次失败；第三事件 from=1 但 to 位不吻合不计
    expect(ctx.cardHealth('summary', 0, names)).toEqual({ fail: 2, lastErr: 'empty_response', takeover: 0 })
    // 顺位 2（接管方）：模型名匹配 → 接管 2
    expect(ctx.cardHealth('summary', 1, names)).toEqual({ fail: 0, lastErr: null, takeover: 2 })
    // 顺位 3：旧事件 to 位不吻合 → 零值
    expect(ctx.cardHealth('summary', 2, names)).toEqual({ fail: 0, lastErr: null, takeover: 0 })

    // 全链失败（to=null）：model=最后尝试的模型，from 位吻合才标
    apiMocks.getDegradations.mockResolvedValue(report([
      ev({ capability: 'topic_tagging', from: 0, to: null, model: 'qwythos', code: 'timeout' }),
    ]))
    const ctx2 = useRouteHealth()
    await ctx2.fetchHealth()
    expect(ctx2.cardHealth('topic_tagging', 0, ['qwythos'])).toEqual({ fail: 1, lastErr: 'timeout', takeover: 0 })
    expect(ctx2.cardHealth('topic_tagging', 0, ['renamed'])).toEqual({ fail: 0, lastErr: null, takeover: 0 })
  })

  it('rowHealth 三态：无事件=ok；有失败且有接管（有兜底）=warn；失败无兜底/全链失败=err', async () => {
    apiMocks.getDegradations.mockResolvedValue(report([
      ev({ capability: 'summary', from: 0, to: 1, model: 'qwythos' }),
      ev({ capability: 'topic_tagging', from: 0, to: null, model: 'qwythos' }),
      ev({ capability: 'digest_polish', from: 0, to: null, model: 'last-try' }),
    ]))
    const ctx = useRouteHealth()
    await ctx.fetchHealth()

    // 无事件能力 → ok
    expect(ctx.rowHealth('embedding', false)).toEqual({ level: 'ok', text: '正常', degradedCount: 0 })
    // 迁移事件（to≠null）且有备份 → warn「已兜底」
    const warn = ctx.rowHealth('summary', true)
    expect(warn.level).toBe('warn')
    expect(warn.text).toContain('降级 1 次')
    expect(warn.text).toContain('已兜底')
    // 全链失败（to=null）且无兜底 → err
    const err = ctx.rowHealth('topic_tagging', false)
    expect(err.level).toBe('err')
    expect(err.text).toContain('无兜底')
    // 失败但有备份（to=null 全链失败场景带 hasBackup=true）→ err 不带「无兜底」
    const err2 = ctx.rowHealth('digest_polish', true)
    expect(err2.level).toBe('err')
    expect(err2.text).not.toContain('无兜底')
  })

  it('fetchHealth 失败：error 置位且不抛出（泳道不因观测数据缺失挂掉）', async () => {
    apiMocks.getDegradations.mockResolvedValue({ success: false, error: 'boom' })
    const ctx = useRouteHealth()
    await ctx.fetchHealth()
    expect(ctx.error.value).toBe('boom')
    expect(ctx.loaded.value).toBe(true)
  })
})
