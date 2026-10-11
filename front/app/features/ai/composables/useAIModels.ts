import { computed, ref } from 'vue'
import { useAIAdminApi } from '~/api'
import type { AIModel, AIModelUpdateRequest } from '~/types'

/**
 * 模型列表与配置（ai-provider-model-overhaul D8）。
 * models 为与后端一致的数据缓存（GET /ai/models 附 vendor 摘要与 usable 布尔）。
 * usableModels / usableModelsForCapability 是能力路由绑定候选的数据源
 * （embedding 路由只接 embedding 模型，后端 UpsertRouteModels 同样强校验）；
 * byVendor 按 vendor 分组，是「+ chips 按 vendor 分组添加」的数据源。
 */
export function useAIModels() {
  const models = ref<AIModel[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  function fail(err: unknown, fallback: string): null {
    error.value = err instanceof Error ? err.message : fallback
    return null
  }

  async function fetchModels(): Promise<AIModel[]> {
    loading.value = true
    error.value = null
    try {
      const res = await useAIAdminApi().listModels()
      if (!res.success) throw new Error(res.error || '加载模型失败')
      models.value = res.data || []
      return models.value
    } catch (err) {
      fail(err, '加载模型失败')
      return []
    } finally {
      loading.value = false
    }
  }

  /** 绑定候选：usable（enabled && status=ok && vendor.enabled），stale/停用模型不可绑 */
  const usableModels = computed(() => models.value.filter(m => m.usable === true))

  /** 能力路由候选：embedding 路由只列 embedding 模型，其余能力只列 llm 模型 */
  function usableModelsForCapability(capability: string): AIModel[] {
    const wantKind = capability === 'embedding' ? 'embedding' : 'llm'
    return usableModels.value.filter(m => m.model_kind === wantKind)
  }

  /** 按 vendor 分组（跳过缺 vendor 摘要的行），chips 数据源 */
  const byVendor = computed(() => {
    const groups = new Map<number, { vendor: NonNullable<AIModel['vendor']>; models: AIModel[] }>()
    for (const m of models.value) {
      if (!m.vendor) continue
      let group = groups.get(m.vendor.id)
      if (!group) {
        group = { vendor: m.vendor, models: [] }
        groups.set(m.vendor.id, group)
      }
      group.models.push(m)
    }
    return Array.from(groups.values()).sort((a, b) => a.vendor.id - b.vendor.id)
  })

  /** 模型配置（enabled/kind/thinking_level/supports_xhigh_max）；成功后刷新缓存保持 vendor 摘要与 usable 最新 */
  async function updateModel(id: number, data: AIModelUpdateRequest): Promise<boolean> {
    error.value = null
    try {
      const res = await useAIAdminApi().updateModel(id, data)
      if (!res.success) throw new Error(res.error || '更新模型失败')
      await fetchModels()
      return true
    } catch (err) {
      fail(err, '更新模型失败')
      return false
    }
  }

  return {
    models,
    loading,
    error,
    fetchModels,
    usableModels,
    usableModelsForCapability,
    byVendor,
    updateModel,
  }
}
