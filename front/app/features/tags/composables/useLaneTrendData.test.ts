import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { nextTick, ref } from 'vue'
import { useLaneTrendData, contextCacheKey } from './useLaneTrendData'
import type { LaneDynamicsResponse } from '~/api/laneDynamics'
import type { ContextRow } from '~/api/boardEnrichment'

// API 层 mock（组合式单测不触网）。
const api = vi.hoisted(() => ({
  getLaneDynamics: vi.fn(),
  listContexts: vi.fn(),
}))

vi.mock('~/api/laneDynamics', () => ({
  useLaneDynamicsApi: () => ({ getLaneDynamics: api.getLaneDynamics }),
}))

vi.mock('~/api/boardEnrichment', () => ({
  useBoardEnrichmentApi: () => ({ listContexts: api.listContexts }),
}))

function lanePayload(topicId = 5): LaneDynamicsResponse {
  return {
    window_days: 14,
    has_reports: true,
    lanes: [{
      topic_id: topicId,
      label: '测试泳道',
      watch_linked: false,
      section_count_14d: 1,
      snapshot: { summary: '短版', detail: '长版全文', as_of: '2026-06-21' },
      timeline: [],
    }],
    candidates: [],
  }
}

function contextRow(over: Partial<ContextRow> = {}): ContextRow {
  return {
    id: 1,
    granularity: 'month',
    period: '2026-06',
    content: '六月归档。',
    as_of_date: '2026-06-30',
    source: 'llm_assisted',
    ...over,
  }
}

beforeEach(() => {
  api.getLaneDynamics.mockResolvedValue({ success: true, data: lanePayload() })
  api.listContexts.mockResolvedValue({ success: true, data: [contextRow()] })
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('useLaneTrendData — 板块级 lane-dynamics', () => {
  it('HD-1: 首个泳道展开触发一次板块级请求；第二个泳道展开不重复请求', async () => {
    const boardId = ref(1974)
    const trend = useLaneTrendData(boardId)

    // 第二个泳道展开 = 再次 emit ensureLaneDynamics → 命中缓存不再发
    await Promise.all([trend.ensureLaneDynamics(), trend.ensureLaneDynamics()])
    await trend.ensureLaneDynamics()

    expect(api.getLaneDynamics).toHaveBeenCalledTimes(1)
    expect(api.getLaneDynamics).toHaveBeenCalledWith(1974, 14)
    expect(trend.getLaneDynamicsEntry().status).toBe('success')
  })

  it('HD-3: 请求失败保持 error 态且不自动重发；重试（force）成功后恢复', async () => {
    api.getLaneDynamics.mockRejectedValueOnce(new Error('网络错误'))
    const trend = useLaneTrendData(ref(1974))

    await trend.ensureLaneDynamics()
    expect(trend.getLaneDynamicsEntry().status).toBe('error')
    expect(trend.getLaneDynamicsEntry().error).toContain('网络错误')

    // 错误态再次展开不自动重发（重试入口是唯一 force 通道）
    await trend.ensureLaneDynamics()
    expect(api.getLaneDynamics).toHaveBeenCalledTimes(1)

    await trend.ensureLaneDynamics(true)
    expect(api.getLaneDynamics).toHaveBeenCalledTimes(2)
    expect(trend.getLaneDynamicsEntry().status).toBe('success')
  })

  it('换版块清空趋势缓存：新板块首展开重新请求', async () => {
    const boardId = ref(1974)
    const trend = useLaneTrendData(boardId)
    await trend.ensureLaneDynamics()
    expect(api.getLaneDynamics).toHaveBeenCalledTimes(1)

    boardId.value = 3001
    await nextTick()
    await trend.ensureLaneDynamics()
    expect(api.getLaneDynamics).toHaveBeenCalledTimes(2)
    expect(api.getLaneDynamics).toHaveBeenLastCalledWith(3001, 14)
  })
})

describe('useLaneTrendData — contexts 月/年缓存', () => {
  it('HD-4: 同 topic 同粒度仅请求一次；跨粒度、跨泳道互不污染', async () => {
    const trend = useLaneTrendData(ref(1974))

    await Promise.all([trend.ensureContext(5, 'month'), trend.ensureContext(5, 'month')])
    await trend.ensureContext(5, 'month')
    expect(api.listContexts).toHaveBeenCalledTimes(1)
    expect(api.listContexts).toHaveBeenCalledWith(5, 'month')

    // 不同粒度 / 不同话题各自独立
    await trend.ensureContext(5, 'year')
    await trend.ensureContext(7, 'month')
    expect(api.listContexts).toHaveBeenCalledTimes(3)
    expect(api.listContexts).toHaveBeenNthCalledWith(2, 5, 'year')
    expect(api.listContexts).toHaveBeenNthCalledWith(3, 7, 'month')

    // 缓存条目按 contextCacheKey 分桶
    expect(trend.contextEntries.value.get(contextCacheKey(5, 'month'))?.status).toBe('success')
  })

  it('contexts 失败保持 error 态；retry=true 才 force 重拉', async () => {
    api.listContexts.mockRejectedValueOnce(new Error('归档超时'))
    const trend = useLaneTrendData(ref(1974))

    await trend.ensureContext(5, 'month')
    const key = contextCacheKey(5, 'month')
    expect(trend.contextEntries.value.get(key)?.status).toBe('error')

    await trend.ensureContext(5, 'month')
    expect(api.listContexts).toHaveBeenCalledTimes(1)

    await trend.ensureContext(5, 'month', true)
    expect(api.listContexts).toHaveBeenCalledTimes(2)
    expect(trend.contextEntries.value.get(key)?.status).toBe('success')
  })

  it('API 返回 success=false 同样按失败处理（error 态、不写入脏数据）', async () => {
    api.listContexts.mockResolvedValue({ success: false, error: 'bad request' })
    const trend = useLaneTrendData(ref(1974))

    await trend.ensureContext(5, 'month')
    expect(trend.contextEntries.value.get(contextCacheKey(5, 'month'))?.status).toBe('error')
    expect(trend.contextEntries.value.get(contextCacheKey(5, 'month'))?.data).toBeUndefined()
  })
})
