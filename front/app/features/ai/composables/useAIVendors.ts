import { ref } from 'vue'
import { useAIAdminApi } from '~/api'
import type {
  AIVendor,
  AIVendorCreateResult,
  AIVendorUpsertRequest,
  VendorDeleteResult,
  VendorModelsRefreshResult,
} from '~/types'

/**
 * 供应商列表与 CRUD（ai-provider-model-overhaul D8）。
 * vendors 为与后端一致的数据缓存：fetch 函数拉取，动作成功后刷新缓存。
 * 动作失败（网络或业务 success=false，如测试连接失败）一律写 error、返回 null/false，
 * 不抛异常——组件层按需 toast。确认弹窗与表单属组件层（4.2 VendorDialog），
 * removeVendor 返回级联计数（deleted_models/detached_bindings）供其展示。
 */
export function useAIVendors() {
  const vendors = ref<AIVendor[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  function fail(err: unknown, fallback: string): null {
    error.value = err instanceof Error ? err.message : fallback
    return null
  }

  async function fetchVendors(): Promise<AIVendor[]> {
    loading.value = true
    error.value = null
    try {
      const res = await useAIAdminApi().listVendors()
      if (!res.success) throw new Error(res.error || '加载供应商失败')
      vendors.value = res.data || []
      return vendors.value
    } catch (err) {
      fail(err, '加载供应商失败')
      return []
    } finally {
      loading.value = false
    }
  }

  /** 新增供应商；后端保存后自动拉一次模型清单（models_refreshed=false 时带 refresh_error） */
  async function createVendor(data: AIVendorUpsertRequest): Promise<AIVendorCreateResult | null> {
    error.value = null
    try {
      const res = await useAIAdminApi().createVendor(data)
      if (!res.success || !res.data) throw new Error(res.error || '新增供应商失败')
      await fetchVendors()
      return res.data
    } catch (err) {
      return fail(err, '新增供应商失败')
    }
  }

  async function updateVendor(id: number, data: AIVendorUpsertRequest): Promise<boolean> {
    error.value = null
    try {
      const res = await useAIAdminApi().updateVendor(id, data)
      if (!res.success) throw new Error(res.error || '更新供应商失败')
      await fetchVendors()
      return true
    } catch (err) {
      fail(err, '更新供应商失败')
      return false
    }
  }

  /** 删除供应商（后端级联：解绑→删模型→删 vendor），返回级联计数供确认/结果展示 */
  async function removeVendor(id: number): Promise<VendorDeleteResult | null> {
    error.value = null
    try {
      const res = await useAIAdminApi().deleteVendor(id)
      if (!res.success || !res.data) throw new Error(res.error || '删除供应商失败')
      await fetchVendors()
      return res.data
    } catch (err) {
      return fail(err, '删除供应商失败')
    }
  }

  /** 拉取上游模型清单 diff（网络失败不落库，库内清单保持刷新前状态） */
  async function refreshModels(id: number): Promise<VendorModelsRefreshResult | null> {
    error.value = null
    try {
      const res = await useAIAdminApi().refreshVendorModels(id)
      if (!res.success || !res.data) throw new Error(res.error || '刷新模型列表失败')
      return res.data
    } catch (err) {
      return fail(err, '刷新模型列表失败')
    }
  }

  /** 按已存配置（含库内密钥）测试连接；成功返回后端 message（含可用模型数） */
  async function testConnection(id: number): Promise<string | null> {
    error.value = null
    try {
      const res = await useAIAdminApi().testVendor(id)
      if (!res.success) throw new Error(res.error || '连接测试失败')
      return res.message || '连接成功'
    } catch (err) {
      return fail(err, '连接测试失败')
    }
  }

  /** 未保存表单的内联测试；成功返回后端 message */
  async function testInline(data: {
    base_url: string
    api_key?: string
    provider_type?: string
    timeout_seconds?: number
  }): Promise<string | null> {
    error.value = null
    try {
      const res = await useAIAdminApi().testVendorInline(data)
      if (!res.success) throw new Error(res.error || '连接测试失败')
      return res.message || '连接成功'
    } catch (err) {
      return fail(err, '连接测试失败')
    }
  }

  return {
    vendors,
    loading,
    error,
    fetchVendors,
    createVendor,
    updateVendor,
    removeVendor,
    refreshModels,
    testConnection,
    testInline,
  }
}
