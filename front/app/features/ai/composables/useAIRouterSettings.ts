import { ref, computed, reactive, onMounted } from 'vue'
import { useAIAdminApi } from '~/api'
import type { AIProvider, AIRoute, AIProviderUpsertRequest } from '~/types'
import { useAI } from '~/composables/useAI'
import { useConfirm } from '~/composables/useConfirm'

const routeLabels: Record<string, string> = {
  summary: '文章总结',
  topic_tagging: '主题提取',
  digest_polish: '日报润色',
  embedding: '向量嵌入',
  open_notebook: '页边注问答',
  feed_discovery: '订阅源发现',
  data_enrichment_news: '新闻总结',
  data_enrichment_analysis: '数据分析',
}

// capabilityOrder 同时是「能力路由」UI 的渲染白名单与「主模型同步」的遍历表。// data_enrichment_news/analysis 历史性遗漏于此，导致设置页看不到这两条 route、// 也无法为「数据分析」单独配强模型——补上。// open_notebook（daily-report-margin-notes）：后端 UpsertRoute 已支持，
// 前端白名单此前漏列致页边注问答无法配 provider 链。
const capabilityOrder = ['summary', 'topic_tagging', 'digest_polish', 'embedding', 'open_notebook', 'feed_discovery', 'data_enrichment_news', 'data_enrichment_analysis']

export function useAIRouterSettings() {
  const loading = ref(false)
  const saving = ref(false)
  const testing = ref(false)
  const error = ref<string | null>(null)
  const success = ref<string | null>(null)

  const providers = ref<AIProvider[]>([])
  const routes = ref<AIRoute[]>([])
  const routeSelections = ref<Record<string, number[]>>({})

  const primaryProviderId = ref<number | null>(null)
  const primaryProviderForm = reactive<AIProviderUpsertRequest & { time_range: number }>({
    name: 'default-primary',
    provider_type: 'openai_compatible',
    base_url: '',
    api_key: '',
    model: '',
    model_kind: 'llm',
    start_command: '',
    clear_start_command: false,
    enabled: true,
    timeout_seconds: 120,
    enable_thinking: false,
    time_range: 180,
  })

  const newProviderForm = reactive<AIProviderUpsertRequest>({
    name: '',
    provider_type: 'openai_compatible',
    base_url: '',
    api_key: '',
    model: '',
    model_kind: 'llm',
    start_command: '',
    enabled: true,
    timeout_seconds: 120,
    enable_thinking: false,
  })

  const showNewProviderForm = ref(false)
  const showPrimaryApiKey = ref(false)
  const showNewProviderApiKey = ref(false)
  const { loadSettings: reloadAISettings } = useAI()
  const editingProviderId = ref<number | null>(null)
  const draggingProviderId = ref<number | null>(null)
  const draggingCapability = ref<string | null>(null)
  const editProviderForm = reactive<AIProviderUpsertRequest>({
    name: '',
    provider_type: 'openai_compatible',
    base_url: '',
    api_key: '',
    model: '',
    model_kind: 'llm',
    start_command: '',
    enabled: true,
    timeout_seconds: 120,
    enable_thinking: false,
    clear_api_key: false,
    clear_start_command: false,
  })
  const showEditProviderApiKey = ref(false)

  // 当前正在测连通的备用 provider id（卡片按钮 loading/防重入）
  const testingProviderId = ref<number | null>(null)

  const backupProviders = computed(() => providers.value.filter(p => p && typeof p.id === 'number' && p.id !== primaryProviderId.value))

  // ---- Helpers ----
  function routeSummary(capability: string): number[] {
    return routeSelections.value[capability] || []
  }

  function providerName(providerId: number): string {
    return providers.value.find(p => p.id === providerId)?.name || `#${providerId}`
  }

  function isProviderLinked(providerId: number): boolean {
    return routes.value.some(r => r.route_providers.some(l => l.provider_id === providerId))
  }

  // ---- Hydration ----
  function hydratePrimaryProvider() {
    const preferred = providers.value.find(p => p.id === primaryProviderId.value)
      || providers.value.find(p => p.name === 'default-primary')
      || providers.value[0]
    primaryProviderId.value = preferred?.id || null
    applyPrimaryProvider(preferred || null)
  }

  function applyPrimaryProvider(provider: AIProvider | null | undefined) {
    primaryProviderId.value = provider?.id || null
    primaryProviderForm.name = provider?.name || 'default-primary'
    primaryProviderForm.provider_type = provider?.provider_type || 'openai_compatible'
    primaryProviderForm.base_url = provider?.base_url || ''
    primaryProviderForm.api_key = ''
    primaryProviderForm.model = provider?.model || ''
    primaryProviderForm.model_kind = provider?.model_kind || 'llm'
    primaryProviderForm.start_command = ''
    primaryProviderForm.clear_start_command = false
    primaryProviderForm.enabled = provider?.enabled ?? true
    primaryProviderForm.timeout_seconds = provider?.timeout_seconds || 120
    primaryProviderForm.enable_thinking = provider?.enable_thinking ?? false
  }

  function hydrateRouteSelections() {
    const next: Record<string, number[]> = {}
    for (const cap of capabilityOrder) {
      const route = routes.value.find(r => r.capability === cap)
      if (!route) { next[cap] = []; continue }
      next[cap] = route.route_providers
        .slice().sort((a, b) => a.priority - b.priority).map(l => l.provider_id)
    }
    routeSelections.value = next
  }

  // ---- Data loading ----
  async function loadData() {
    loading.value = true
    error.value = null
    try {
      const aiAdminApi = useAIAdminApi()
      const [settingsRes, providersRes, routesRes] = await Promise.all([
        aiAdminApi.getSettings(),
        aiAdminApi.listProviders(),
        aiAdminApi.listRoutes(),
      ])
      if (!providersRes.success || !routesRes.success) {
        throw new Error(providersRes.error || routesRes.error || '加载 AI 配置失败')
      }
      providers.value = providersRes.data || []
      routes.value = routesRes.data || []
      primaryProviderId.value = settingsRes.success && settingsRes.data?.provider_id
        ? Number(settingsRes.data.provider_id) : null
      primaryProviderForm.time_range = settingsRes.success && typeof settingsRes.data?.time_range === 'number'
        ? settingsRes.data.time_range : 180
      hydratePrimaryProvider()
      hydrateRouteSelections()
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载 AI 配置失败'
    } finally {
      loading.value = false
    }
  }

  // ---- Notification ----
  function pushMessage(kind: 'success' | 'error', message: string) {
    if (kind === 'success') {
      success.value = message
      error.value = null
      setTimeout(() => { success.value = null }, 2500)
    } else {
      error.value = message
      success.value = null
    }
  }

  // ---- Route candidates（按 capability 过滤 model_kind：embedding 路由只接 embedding provider）----
  function providerModelKind(providerId: number): 'llm' | 'embedding' {
    return providers.value.find(p => p.id === providerId)?.model_kind || 'llm'
  }

  function candidatesForCapability(capability: string): AIProvider[] {
    const wantKind = capability === 'embedding' ? 'embedding' : 'llm'
    return backupProviders.value.filter(p => (p.model_kind || 'llm') === wantKind)
  }

  function primaryAllowedForCapability(capability: string): boolean {
    if (primaryProviderId.value === null) return false
    const wantKind = capability === 'embedding' ? 'embedding' : 'llm'
    return providerModelKind(primaryProviderId.value) === wantKind
  }

  // ---- Route management ----
  function addProviderToRoute(capability: string, providerId: number) {
    const current = routeSummary(capability)
    if (current.includes(providerId)) return
    routeSelections.value[capability] = [...current, providerId]
  }

  function removeProviderFromRoute(capability: string, providerId: number) {
    routeSelections.value[capability] = routeSummary(capability).filter(id => id !== providerId)
  }

  function isPrimaryInRoute(capability: string): boolean {
    return routeSummary(capability).includes(primaryProviderId.value ?? -1)
  }

  function removePrimaryFromRoute(capability: string) {
    if (primaryProviderId.value) removeProviderFromRoute(capability, primaryProviderId.value)
  }

  function addPrimaryToRoute(capability: string) {
    if (primaryProviderId.value) addProviderToRoute(capability, primaryProviderId.value)
  }

  function moveProvider(capability: string, providerId: number, direction: -1 | 1) {
    const current = [...routeSummary(capability)]
    const index = current.indexOf(providerId)
    const nextIndex = index + direction
    if (index < 0 || nextIndex < 0 || nextIndex >= current.length) return
    const cur = current[index]; const nxt = current[nextIndex]
    if (cur === undefined || nxt === undefined) return
    current[index] = nxt; current[nextIndex] = cur
    routeSelections.value[capability] = current
  }

  function handleDragStart(capability: string, providerId: number) {
    draggingCapability.value = capability
    draggingProviderId.value = providerId
  }

  function handleDragEnd() {
    draggingCapability.value = null
    draggingProviderId.value = null
  }

  function handleDropOnProvider(capability: string, targetProviderId: number) {
    if (draggingCapability.value !== capability || draggingProviderId.value === null) {
      handleDragEnd(); return
    }
    const current = [...routeSummary(capability)]
    const fromIdx = current.indexOf(draggingProviderId.value)
    const toIdx = current.indexOf(targetProviderId)
    if (fromIdx < 0 || toIdx < 0 || fromIdx === toIdx) { handleDragEnd(); return }
    const [moved] = current.splice(fromIdx, 1)
    if (moved === undefined) { handleDragEnd(); return }
    current.splice(toIdx, 0, moved)
    routeSelections.value[capability] = current
    handleDragEnd()
  }

  // ---- Primary provider CRUD ----
  async function savePrimaryProvider() {
    if (!primaryProviderForm.base_url || !primaryProviderForm.model) {
      pushMessage('error', '主模型至少要有 Base URL 和 Model'); return
    }
    saving.value = true; error.value = null
    try {
      const aiAdminApi = useAIAdminApi()
      let providerId = primaryProviderId.value
      if (providerId) {
        const res = await aiAdminApi.updateProvider(providerId, primaryProviderForm)
        if (!res.success) throw new Error(res.error || '更新主模型失败')
      } else {
        const res = await aiAdminApi.createProvider(primaryProviderForm)
        if (!res.success || !res.data) throw new Error(res.error || '创建主模型失败')
        providerId = res.data.id
        primaryProviderId.value = providerId
      }
      const providerSnapshot: AIProvider = {
        id: providerId || 0, name: primaryProviderForm.name,
        provider_type: primaryProviderForm.provider_type || 'openai_compatible',
        base_url: primaryProviderForm.base_url, model: primaryProviderForm.model,
        model_kind: primaryProviderForm.model_kind || 'llm',
        start_command_configured: false,
        enabled: primaryProviderForm.enabled ?? true,
        timeout_seconds: primaryProviderForm.timeout_seconds || 120,
        max_tokens: primaryProviderForm.max_tokens ?? null,
        temperature: primaryProviderForm.temperature ?? null,
        enable_thinking: primaryProviderForm.enable_thinking ?? false,
        metadata: primaryProviderForm.metadata, api_key_configured: true,
      }
      if (providerId) {
        // 主模型 = 同类型能力的默认首选：LLM 主模型只同步到 LLM 能力路由，
        // embedding 路由保持独立（不把 LLM 挂上 embedding 路由——后端类型
        // 校验会拒绝，旧行为还曾把 LLM 写进 embedding 路由形成脏数据）。
        // 用表单的 model_kind 判断（刚保存的值），不查可能过期的 providers 列表。
        const primaryKind = primaryProviderForm.model_kind || 'llm'
        for (const capability of capabilityOrder) {
          const wantKind = capability === 'embedding' ? 'embedding' : 'llm'
          if (primaryKind !== wantKind) continue
          const existingRoute = routes.value.find(r => r.capability === capability)
          let ids = routeSummary(capability)
          if (!ids.includes(providerId)) ids = [providerId, ...ids]
          const res = await aiAdminApi.updateRoute(capability, {
            name: existingRoute?.name || 'default', enabled: existingRoute?.enabled ?? true,
            description: existingRoute?.description, provider_ids: ids,
          })
          if (!res.success) throw new Error(res.error || `同步 ${routeLabels[capability]} 主路由失败`)
        }
      }
      await loadData()
      applyPrimaryProvider(providerSnapshot)
      await reloadAISettings(true)
      pushMessage('success', '主模型配置已保存')
    } catch (err) {
      pushMessage('error', err instanceof Error ? err.message : '保存失败')
    } finally {
      saving.value = false
    }
  }

  // ---- Backup provider CRUD ----
  async function saveNewProvider() {
    if (!newProviderForm.name || !newProviderForm.base_url || !newProviderForm.model) {
      pushMessage('error', '备用模型表单还没填完整'); return
    }
    saving.value = true
    try {
      const aiAdminApi = useAIAdminApi()
      const res = await aiAdminApi.createProvider(newProviderForm)
      if (!res.success) throw new Error(res.error || '创建备用模型失败')
      newProviderForm.name = ''
      newProviderForm.base_url = ''
      newProviderForm.api_key = ''
      newProviderForm.model = ''
      newProviderForm.model_kind = 'llm'
      newProviderForm.start_command = ''
      newProviderForm.enabled = true
      newProviderForm.timeout_seconds = 120
      newProviderForm.enable_thinking = false
      showNewProviderForm.value = false
      await loadData()
      pushMessage('success', '备用模型已添加')
    } catch (err) {
      pushMessage('error', err instanceof Error ? err.message : '创建失败')
    } finally {
      saving.value = false
    }
  }

  function startEditingProvider(provider: AIProvider) {
    editingProviderId.value = provider.id
    editProviderForm.name = provider.name
    editProviderForm.provider_type = provider.provider_type
    editProviderForm.base_url = provider.base_url
    editProviderForm.api_key = ''
    editProviderForm.model = provider.model
    editProviderForm.model_kind = provider.model_kind || 'llm'
    editProviderForm.start_command = ''
    editProviderForm.clear_start_command = false
    editProviderForm.enabled = provider.enabled
    editProviderForm.timeout_seconds = provider.timeout_seconds
    editProviderForm.enable_thinking = provider.enable_thinking ?? false
    editProviderForm.clear_api_key = false
  }

  function cancelEditingProvider() {
    editingProviderId.value = null
    editProviderForm.name = ''
    editProviderForm.provider_type = 'openai_compatible'
    editProviderForm.base_url = ''
    editProviderForm.api_key = ''
    editProviderForm.model = ''
    editProviderForm.model_kind = 'llm'
    editProviderForm.start_command = ''
    editProviderForm.clear_start_command = false
    editProviderForm.enabled = true
    editProviderForm.timeout_seconds = 120
    editProviderForm.enable_thinking = false
    editProviderForm.clear_api_key = false
  }

  async function saveEditedProvider() {
    if (!editingProviderId.value) return
    if (!editProviderForm.name || !editProviderForm.base_url || !editProviderForm.model) {
      pushMessage('error', '编辑备用模型时，名称、Base URL 和 Model 不能为空'); return
    }
    saving.value = true
    try {
      const aiAdminApi = useAIAdminApi()
      const res = await aiAdminApi.updateProvider(editingProviderId.value, editProviderForm)
      if (!res.success) throw new Error(res.error || '更新备用模型失败')
      cancelEditingProvider()
      await loadData()
      pushMessage('success', '备用模型已更新')
    } catch (err) {
      pushMessage('error', err instanceof Error ? err.message : '更新失败')
    } finally {
      saving.value = false
    }
  }

  async function deleteBackupProvider(provider: AIProvider) {
    // 后端删除时级联解绑（fix-provider-delete-route-deadlock）：该 provider 会从
    // 所有线路摘除，线路可能因此被摘空；删除成功后 loadData() 会用后端真值
    // 重建线路选择（hydrateRouteSelections），本地无需也不应再手动改 state。
    // 确认弹窗走全局 useConfirm（替代原生 confirm）。
    const { confirm } = useConfirm()
    const ok = await confirm({
      title: '删除备用模型',
      message: `确定删除备用模型 ${provider.name} 吗？它将从所有线路解绑。`,
      confirmText: '删除',
      danger: true,
    })
    if (!ok) return
    saving.value = true
    try {
      const aiAdminApi = useAIAdminApi()
      const res = await aiAdminApi.deleteProvider(provider.id)
      if (!res.success) throw new Error(res.error || '删除备用模型失败')
      if (editingProviderId.value === provider.id) cancelEditingProvider()
      await loadData()
      pushMessage('success', '备用模型已删除')
    } catch (err) {
      pushMessage('error', err instanceof Error ? err.message : '删除失败')
    } finally {
      saving.value = false
    }
  }

  // ---- Route save ----
  async function saveRoutes() {
    const hasProviders = capabilityOrder.some(cap => routeSummary(cap).length > 0)
    if (!hasProviders) { pushMessage('error', '至少为一条能力路由配置一个 provider'); return }
    saving.value = true
    try {
      const aiAdminApi = useAIAdminApi()
      for (const capability of capabilityOrder) {
        const ids = routeSummary(capability)
        if (ids.length === 0) continue
        const res = await aiAdminApi.updateRoute(capability, {
          name: 'default', enabled: true, provider_ids: ids,
        })
        if (!res.success) throw new Error(res.error || `保存 ${routeLabels[capability]} 路由失败`)
      }
      await loadData()
      await reloadAISettings(true)
      pushMessage('success', '多路由顺序已保存')
    } catch (err) {
      pushMessage('error', err instanceof Error ? err.message : '保存路由失败')
    } finally {
      saving.value = false
    }
  }

  // ---- Test connection ----
  async function testPrimaryProvider() {
    if (!primaryProviderForm.base_url || !primaryProviderForm.model) {
      pushMessage('error', '测试连接前请填入 Base URL 和 Model'); return
    }
    testing.value = true
    try {
      const aiAdminApi = useAIAdminApi()
      const res = await aiAdminApi.testConnection({
        base_url: primaryProviderForm.base_url,
        api_key: primaryProviderForm.api_key || undefined,
        model: primaryProviderForm.model,
        provider_type: primaryProviderForm.provider_type,
      })
      if (!res.success) throw new Error(res.error || '连接测试失败')
      pushMessage('success', res.message || '连接测试成功')
    } catch (err) {
      pushMessage('error', err instanceof Error ? err.message : '连接测试失败')
    } finally {
      testing.value = false
    }
  }

  // 按已保存配置（含库里密钥）测试备用 provider 连通性，provider_id 由后端取数
  async function testBackupProvider(provider: AIProvider) {
    if (testingProviderId.value !== null) return
    testingProviderId.value = provider.id
    try {
      const aiAdminApi = useAIAdminApi()
      const res = await aiAdminApi.testConnection({ provider_id: provider.id })
      if (!res.success) throw new Error(res.error || '连接测试失败')
      pushMessage('success', `${provider.name}：${res.message || '连接测试成功'}`)
    } catch (err) {
      pushMessage('error', `${provider.name}：${err instanceof Error ? err.message : '连接测试失败'}`)
    } finally {
      testingProviderId.value = null
    }
  }

  onMounted(() => { void loadData() })

  return {
    // State
    loading, saving, testing, error, success,
    providers, routes, routeSelections,
    primaryProviderId, primaryProviderForm,
    newProviderForm, showNewProviderForm,
    showPrimaryApiKey, showNewProviderApiKey,
    editingProviderId, editProviderForm, showEditProviderApiKey,
    draggingProviderId, draggingCapability,
    backupProviders,
    testingProviderId,

    // Constants
    routeLabels, capabilityOrder,

    // Route helpers
    routeSummary, providerName, isProviderLinked,
    providerModelKind, candidatesForCapability, primaryAllowedForCapability,
    addProviderToRoute, removeProviderFromRoute,
    isPrimaryInRoute, removePrimaryFromRoute, addPrimaryToRoute,
    moveProvider, handleDragStart, handleDragEnd, handleDropOnProvider,

    // CRUD
    loadData, savePrimaryProvider,
    saveNewProvider, startEditingProvider, cancelEditingProvider,
    saveEditedProvider, deleteBackupProvider,
    saveRoutes,
    testPrimaryProvider,
    testBackupProvider,
  }
}
