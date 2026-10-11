import { computed, ref } from 'vue'
import { useAIAdminApi } from '~/api'
import type { AIRoute, RouteSaveRequest } from '~/types'

// routeLabels/capabilityOrder 同时是「能力路由」UI 的渲染白名单与文案表。
// data_enrichment_news/analysis 历史性遗漏于白名单导致设置页看不到这两条 route、
// 也无法为「数据分析」单独配强模型；open_notebook（页边注问答）同理——均已补上。
export const routeLabels: Record<string, string> = {
  summary: '文章总结',
  topic_tagging: '主题提取',
  digest_polish: '日报润色',
  embedding: '向量嵌入',
  open_notebook: '页边注问答',
  feed_discovery: '订阅源发现',
  data_enrichment_news: '新闻总结',
  data_enrichment_analysis: '数据分析',
}

export const capabilityOrder = ['summary', 'topic_tagging', 'digest_polish', 'embedding', 'open_notebook', 'feed_discovery', 'data_enrichment_news', 'data_enrichment_analysis']

// routeDescriptions：泳道行「供血对象」说明小字（与 routeLabels 同源维护，
// 缺省不渲染说明行）。口径必须与后端真实调用方一致，勿凭名字猜：
// - digest_polish 供血日报生成 + 泳道动态/泳道趋势「近 14 天」档
//   （topicgraph daily_report.* / lane_snapshot LLM 调用）；
// - data_enrichment_news 供血话题生命线周期归档摘要
//   （data_enrichment.summarize_context，月/年定时任务生成）=
//   泳道趋势「月」「年」档数据源，与日报无关。
export const routeDescriptions: Record<string, string> = {
  summary: '阅读流文章自动总结',
  topic_tagging: '标签提取与语义操作',
  digest_polish: '日报生成（聚类/要闻/叙事）与泳道趋势「近 14 天」档',
  embedding: '文章向量嵌入（语义检索）',
  open_notebook: '日报页边注问答（含搜索词生成）',
  feed_discovery: '订阅源发现推荐精排',
  data_enrichment_news: '话题生命线周期归档摘要（泳道趋势「月」「年」档）',
  data_enrichment_analysis: '数据增强三角色编排（解读/分析/评审）',
}

/**
 * 能力路由列表与保存（ai-provider-model-overhaul D8）。
 * saveRoute 带 saving 锁防重复提交；保存体为全删全建绑定列表
 * （[{model_id, thinking_level?}]，类型约束由后端 UpsertRouteModels 校验并透传错误文案）。
 */
export function useAIRoutes() {
  const routes = ref<AIRoute[]>([])
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<string | null>(null)

  /** 按 capability 索引（每能力一条绑定链） */
  const routeByCapability = computed<Record<string, AIRoute>>(() => {
    const index: Record<string, AIRoute> = {}
    for (const route of routes.value) index[route.capability] = route
    return index
  })

  async function fetchRoutes(): Promise<AIRoute[]> {
    loading.value = true
    error.value = null
    try {
      const res = await useAIAdminApi().listRoutes()
      if (!res.success) throw new Error(res.error || '加载能力路由失败')
      routes.value = res.data || []
      return routes.value
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载能力路由失败'
      return []
    } finally {
      loading.value = false
    }
  }

  /** 保存单条能力的绑定链；成功后刷新缓存（顺位/绑定以后端真值为准） */
  async function saveRoute(capability: string, data: RouteSaveRequest): Promise<boolean> {
    if (saving.value) return false
    saving.value = true
    error.value = null
    try {
      const res = await useAIAdminApi().saveRoute(capability, data)
      if (!res.success) throw new Error(res.error || `保存 ${routeLabels[capability] || capability} 路由失败`)
      await fetchRoutes()
      return true
    } catch (err) {
      error.value = err instanceof Error ? err.message : '保存路由失败'
      return false
    } finally {
      saving.value = false
    }
  }

  return {
    routes,
    loading,
    saving,
    error,
    fetchRoutes,
    routeByCapability,
    saveRoute,
  }
}
