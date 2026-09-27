import { ref, watch, type Ref } from 'vue'
import { useLaneDynamicsApi, type LaneDynamicsResponse } from '~/api/laneDynamics'
import { useBoardEnrichmentApi, type ContextRow } from '~/api/boardEnrichment'
import { createRequestCache, type RequestCacheEntry } from '~/features/tags/components/daily-report/dailyReportMagazine'

/** 泳道趋势区月/年档粒度（14 天档走板块级 lane-dynamics，不经 contexts 缓存）。 */
export type TrendGranularity = 'month' | 'year'

/** contexts 缓存键：同 topic 同粒度仅请求一次，跨泳道不污染（HD-4）。 */
export function contextCacheKey(topicId: number, granularity: TrendGranularity): string {
  return `${topicId}:${granularity}`
}

function parseContextKey(key: string): [number, TrendGranularity] {
  const index = key.lastIndexOf(':')
  return [Number(key.slice(0, index)), key.slice(index + 1) as TrendGranularity]
}

/**
 * 泳道趋势区宿主取数（lane-trend-overview design §D4）。
 *
 * - lane-dynamics：板块级一次（days=14），首个泳道展开触发，页面级缓存 +
 *   pending 去重；数据锚定板块最新报告期 MAX(period_date)，与正在阅读的
 *   报告日期解耦——翻期（shiftReportPeel/selectReportPeel）不触碰本缓存
 *   「趋势=现在」；换版块清缓存。
 * - contexts：月/年档按需拉取（切档才发，组件仅在 entry idle 时发信号），
 *   createRequestCache 保证成功命中不再发、错误态只由重试按钮 force 重拉。
 */
export function useLaneTrendData(boardId: Ref<number>) {
  const { getLaneDynamics } = useLaneDynamicsApi()
  const { listContexts } = useBoardEnrichmentApi()

  const laneEntries = ref(new Map<number, RequestCacheEntry<LaneDynamicsResponse>>())
  const laneCache = createRequestCache<number, LaneDynamicsResponse>(async (id) => {
    const response = await getLaneDynamics(id, 14)
    if (!response.success || !response.data) throw new Error(response.error || '泳道动态加载失败')
    return response.data
  }, entries => { laneEntries.value = entries })

  const contextEntries = ref(new Map<string, RequestCacheEntry<ContextRow[]>>())
  const contextCache = createRequestCache<string, ContextRow[]>(async (key) => {
    const [topicId, granularity] = parseContextKey(key)
    const response = await listContexts(topicId, granularity)
    if (!response.success || !response.data) throw new Error(response.error || '周期归档加载失败')
    return response.data
  }, entries => { contextEntries.value = entries })

  /** 首个泳道展开触发一次板块级请求；后续展开命中缓存/pending 去重，不重复请求（HD-1）。 */
  function ensureLaneDynamics(retry = false) {
    return laneCache.load(boardId.value, retry)
  }

  /** 月/年档取数；错误态重试按钮带 retry=true force 重拉。 */
  function ensureContext(topicId: number, granularity: TrendGranularity, retry = false) {
    return contextCache.load(contextCacheKey(topicId, granularity), retry)
  }

  function getLaneDynamicsEntry(): RequestCacheEntry<LaneDynamicsResponse> {
    return laneEntries.value.get(boardId.value) ?? { status: 'idle' }
  }

  // 换版块：趋势缓存按板块/话题维度私有，跨版块不共享。
  watch(boardId, () => {
    laneCache.clear()
    contextCache.clear()
  })

  return {
    laneEntries,
    contextEntries,
    ensureLaneDynamics,
    ensureContext,
    getLaneDynamicsEntry,
  }
}
