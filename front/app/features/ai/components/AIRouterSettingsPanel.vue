<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { useAIAdminApi } from '~/api'
import type { AIProvider, AIRoute, AIProviderUpsertRequest } from '~/types'

const routeLabels: Record<string, string> = {
  summary: '文章总结',
  article_completion: '正文补全',
  topic_tagging: '主题提取',
  digest_polish: '日报润色',
  embedding: '向量嵌入',
}

const capabilityOrder = ['summary', 'article_completion', 'topic_tagging', 'digest_polish', 'embedding']

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
  enabled: true,
  timeout_seconds: 120,
  enable_thinking: false,
})

const showNewProviderForm = ref(false)
const showPrimaryApiKey = ref(false)
const showNewProviderApiKey = ref(false)
const embeddingThreshold = ref(0.7)
const savingThreshold = ref(false)
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
  enabled: true,
  timeout_seconds: 120,
  enable_thinking: false,
})
const showEditProviderApiKey = ref(false)

const backupProviders = computed(() => providers.value.filter(provider => provider.id !== primaryProviderId.value))

function routeSummary(capability: string) {
  return routeSelections.value[capability] || []
}

function providerName(providerId: number) {
  return providers.value.find(provider => provider.id === providerId)?.name || `#${providerId}`
}

function isProviderLinked(providerId: number) {
  return routes.value.some(route => route.route_providers.some(link => link.provider_id === providerId))
}

function hydratePrimaryProvider() {
  const preferredProvider = providers.value.find(provider => provider.id === primaryProviderId.value)
    || providers.value.find(provider => provider.name === 'default-primary')
    || providers.value[0]

  primaryProviderId.value = preferredProvider?.id || null
  primaryProviderForm.name = preferredProvider?.name || 'default-primary'
  primaryProviderForm.provider_type = preferredProvider?.provider_type || 'openai_compatible'
  primaryProviderForm.base_url = preferredProvider?.base_url || ''
  primaryProviderForm.api_key = ''
  primaryProviderForm.model = preferredProvider?.model || ''
  primaryProviderForm.enabled = preferredProvider?.enabled ?? true
  primaryProviderForm.timeout_seconds = preferredProvider?.timeout_seconds || 120
  primaryProviderForm.enable_thinking = preferredProvider?.enable_thinking ?? false
}

function applyPrimaryProvider(provider: AIProvider | null | undefined) {
  primaryProviderId.value = provider?.id || null
  primaryProviderForm.name = provider?.name || 'default-primary'
  primaryProviderForm.provider_type = provider?.provider_type || 'openai_compatible'
  primaryProviderForm.base_url = provider?.base_url || ''
  primaryProviderForm.api_key = ''
  primaryProviderForm.model = provider?.model || ''
  primaryProviderForm.enabled = provider?.enabled ?? true
  primaryProviderForm.timeout_seconds = provider?.timeout_seconds || 120
  primaryProviderForm.enable_thinking = provider?.enable_thinking ?? false
}

function hydrateRouteSelections() {
  const nextSelections: Record<string, number[]> = {}
  for (const capability of capabilityOrder) {
    const route = routes.value.find(item => item.capability === capability)
    if (!route) {
      nextSelections[capability] = []
      continue
    }

    nextSelections[capability] = route.route_providers
      .slice()
      .sort((a, b) => a.priority - b.priority)
      .map(link => link.provider_id)
  }
  routeSelections.value = nextSelections
}

async function loadData() {
  loading.value = true
  error.value = null

  try {
    const aiAdminApi = useAIAdminApi()
    const [settingsResponse, providersResponse, routesResponse] = await Promise.all([
      aiAdminApi.getSettings(),
      aiAdminApi.listProviders(),
      aiAdminApi.listRoutes(),
    ])

    if (!providersResponse.success || !routesResponse.success) {
      throw new Error(providersResponse.error || routesResponse.error || '加载 AI 配置失败')
    }

    providers.value = providersResponse.data || []
    routes.value = routesResponse.data || []
    primaryProviderId.value = settingsResponse.success && settingsResponse.data?.provider_id
      ? Number(settingsResponse.data.provider_id)
      : null
    primaryProviderForm.time_range = settingsResponse.success && typeof settingsResponse.data?.time_range === 'number'
      ? settingsResponse.data.time_range
      : 180

    embeddingThreshold.value = settingsResponse.success && settingsResponse.data?.narrative_board_embedding_threshold
      ? parseFloat(String(settingsResponse.data.narrative_board_embedding_threshold))
      : 0.7

    hydratePrimaryProvider()
    hydrateRouteSelections()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载 AI 配置失败'
  } finally {
    loading.value = false
  }
}

function pushMessage(kind: 'success' | 'error', message: string) {
  if (kind === 'success') {
    success.value = message
    error.value = null
    setTimeout(() => {
      success.value = null
    }, 2500)
    return
  }

  error.value = message
  success.value = null
}

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
  if (primaryProviderId.value) {
    removeProviderFromRoute(capability, primaryProviderId.value)
  }
}

function addPrimaryToRoute(capability: string) {
  if (primaryProviderId.value) {
    addProviderToRoute(capability, primaryProviderId.value)
  }
}

function moveProvider(capability: string, providerId: number, direction: -1 | 1) {
  const current = [...routeSummary(capability)]
  const index = current.indexOf(providerId)
  const nextIndex = index + direction
  if (index < 0 || nextIndex < 0 || nextIndex >= current.length) return
  const currentValue = current[index]
  const nextValue = current[nextIndex]
  if (currentValue === undefined || nextValue === undefined) return
  current[index] = nextValue
  current[nextIndex] = currentValue
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
    handleDragEnd()
    return
  }

  const current = [...routeSummary(capability)]
  const fromIndex = current.indexOf(draggingProviderId.value)
  const toIndex = current.indexOf(targetProviderId)
  if (fromIndex < 0 || toIndex < 0 || fromIndex === toIndex) {
    handleDragEnd()
    return
  }

  const [moved] = current.splice(fromIndex, 1)
  if (moved === undefined) {
    handleDragEnd()
    return
  }
  current.splice(toIndex, 0, moved)
  routeSelections.value[capability] = current
  handleDragEnd()
}

async function savePrimaryProvider() {
  if (!primaryProviderForm.base_url || !primaryProviderForm.model) {
    pushMessage('error', '主模型至少要有 Base URL 和 Model')
    return
  }

  const isOllama = primaryProviderForm.provider_type === 'ollama'
  if (!isOllama && !primaryProviderId.value && !primaryProviderForm.api_key) {
    pushMessage('error', '首次创建主模型需要填写 API Key')
    return
  }

  saving.value = true
  error.value = null

  try {
    const aiAdminApi = useAIAdminApi()
    let providerId = primaryProviderId.value

    if (providerId) {
      const response = await aiAdminApi.updateProvider(providerId, primaryProviderForm)
      if (!response.success) {
        throw new Error(response.error || '更新主模型失败')
      }
    } else {
      const response = await aiAdminApi.createProvider(primaryProviderForm)
      if (!response.success || !response.data) {
        throw new Error(response.error || '创建主模型失败')
      }
      providerId = response.data.id
      primaryProviderId.value = providerId
    }

    const providerSnapshot: AIProvider = {
      id: providerId || 0,
      name: primaryProviderForm.name,
      provider_type: primaryProviderForm.provider_type || 'openai_compatible',
      base_url: primaryProviderForm.base_url,
      model: primaryProviderForm.model,
      enabled: primaryProviderForm.enabled ?? true,
      timeout_seconds: primaryProviderForm.timeout_seconds || 120,
      max_tokens: primaryProviderForm.max_tokens ?? null,
      temperature: primaryProviderForm.temperature ?? null,
      enable_thinking: primaryProviderForm.enable_thinking ?? false,
      metadata: primaryProviderForm.metadata,
      api_key_configured: true,
    }

    if (providerId) {
      for (const capability of capabilityOrder) {
        const existingRoute = routes.value.find(route => route.capability === capability)
        let providerIds = routeSummary(capability)
        if (!providerIds.includes(providerId)) {
          providerIds = [providerId, ...providerIds]
        }
        const response = await aiAdminApi.updateRoute(capability, {
          name: existingRoute?.name || 'default',
          enabled: existingRoute?.enabled ?? true,
          description: existingRoute?.description,
          provider_ids: providerIds,
        })
        if (!response.success) {
          throw new Error(response.error || `同步 ${routeLabels[capability]} 主路由失败`)
        }
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

async function saveNewProvider() {
  if (!newProviderForm.name || !newProviderForm.base_url || !newProviderForm.model) {
    pushMessage('error', '备用模型表单还没填完整')
    return
  }

  const isOllama = newProviderForm.provider_type === 'ollama'
  if (!isOllama && !newProviderForm.api_key) {
    pushMessage('error', '非 Ollama 类型的备用模型需要填写 API Key')
    return
  }

  saving.value = true
  try {
    const aiAdminApi = useAIAdminApi()
    const response = await aiAdminApi.createProvider(newProviderForm)
    if (!response.success) {
      throw new Error(response.error || '创建备用模型失败')
    }

    newProviderForm.name = ''
    newProviderForm.base_url = ''
    newProviderForm.api_key = ''
    newProviderForm.model = ''
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
  editProviderForm.enabled = provider.enabled
  editProviderForm.timeout_seconds = provider.timeout_seconds
  editProviderForm.enable_thinking = provider.enable_thinking ?? false
}

function cancelEditingProvider() {
  editingProviderId.value = null
  editProviderForm.name = ''
  editProviderForm.provider_type = 'openai_compatible'
  editProviderForm.base_url = ''
  editProviderForm.api_key = ''
  editProviderForm.model = ''
  editProviderForm.enabled = true
  editProviderForm.timeout_seconds = 120
  editProviderForm.enable_thinking = false
}

async function saveEditedProvider() {
  if (!editingProviderId.value) return
  if (!editProviderForm.name || !editProviderForm.base_url || !editProviderForm.model) {
    pushMessage('error', '编辑备用模型时，名称、Base URL 和 Model 不能为空')
    return
  }

  saving.value = true
  try {
    const aiAdminApi = useAIAdminApi()
    const response = await aiAdminApi.updateProvider(editingProviderId.value, editProviderForm)
    if (!response.success) {
      throw new Error(response.error || '更新备用模型失败')
    }
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
  if (!confirm(`确定删除备用模型 ${provider.name} 吗？`)) {
    return
  }

  saving.value = true
  try {
    const aiAdminApi = useAIAdminApi()
    const response = await aiAdminApi.deleteProvider(provider.id)
    if (!response.success) {
      throw new Error(response.error || '删除备用模型失败')
    }

    for (const capability of capabilityOrder) {
      removeProviderFromRoute(capability, provider.id)
    }

    if (editingProviderId.value === provider.id) {
      cancelEditingProvider()
    }

    await loadData()
    pushMessage('success', '备用模型已删除')
  } catch (err) {
    pushMessage('error', err instanceof Error ? err.message : '删除失败')
  } finally {
    saving.value = false
  }
}

async function saveRoutes() {
  const hasProviders = capabilityOrder.some(cap => routeSummary(cap).length > 0)
  if (!hasProviders) {
    pushMessage('error', '至少为一条能力路由配置一个 provider')
    return
  }

  saving.value = true
  try {
    const aiAdminApi = useAIAdminApi()
    for (const capability of capabilityOrder) {
      const providerIds = routeSummary(capability)
      if (providerIds.length === 0) continue
      const response = await aiAdminApi.updateRoute(capability, {
        name: 'default',
        enabled: true,
        provider_ids: providerIds,
      })
      if (!response.success) {
        throw new Error(response.error || `保存 ${routeLabels[capability]} 路由失败`)
      }
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

async function saveThreshold() {
  savingThreshold.value = true
  try {
    const aiAdminApi = useAIAdminApi()
    const response = await aiAdminApi.saveSettings({
      narrative_board_embedding_threshold: embeddingThreshold.value,
    } as any)
    if (!response.success) {
      throw new Error(response.error || '保存阈值失败')
    }
    pushMessage('success', '匹配阈值已保存')
  } catch (err) {
    pushMessage('error', err instanceof Error ? err.message : '保存阈值失败')
  } finally {
    savingThreshold.value = false
  }
}

async function testPrimaryProvider() {
  if (!primaryProviderForm.base_url || !primaryProviderForm.model) {
    pushMessage('error', '测试连接前请填入 Base URL 和 Model')
    return
  }

  const isOllama = primaryProviderForm.provider_type === 'ollama'
  if (!isOllama && !primaryProviderForm.api_key) {
    pushMessage('error', '非 Ollama 类型测试连接前请填入 API Key')
    return
  }

  testing.value = true
  try {
    const aiAdminApi = useAIAdminApi()
    const response = await aiAdminApi.testConnection({
      base_url: primaryProviderForm.base_url,
      api_key: primaryProviderForm.api_key || undefined,
      model: primaryProviderForm.model,
      provider_type: primaryProviderForm.provider_type,
    })
    if (!response.success) {
      throw new Error(response.error || '连接测试失败')
    }
    pushMessage('success', response.message || '连接测试成功')
  } catch (err) {
    pushMessage('error', err instanceof Error ? err.message : '连接测试失败')
  } finally {
    testing.value = false
  }
}

onMounted(() => {
  loadData()
})
</script>

<template>
  <div class="space-y-5">
    <div v-if="loading" class="py-12 flex justify-center">
      <Icon icon="mdi:loading" width="32" height="32" class="animate-spin text-ink-500" />
    </div>

    <template v-else>
      <!-- Section 1: Primary Provider -->
      <div class="rounded-xl border border-ink-100 bg-gradient-to-br from-ink-50/80 via-white to-paper-cream/60 overflow-hidden">
        <div class="px-5 py-3.5 border-b border-ink-100/60 flex items-center justify-between bg-ink-50/40">
          <div class="flex items-center gap-2.5">
            <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-ink-600 to-ink-800 flex items-center justify-center shadow-sm">
              <Icon icon="mdi:star-four-points" width="16" height="16" class="text-white" />
            </div>
            <div>
              <h3 class="text-sm font-semibold text-gray-900">主模型</h3>
              <p class="text-[11px] text-gray-500">默认 AI 提供者，保存后自动挂载到所有能力路由</p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button
              class="px-3 py-1.5 text-xs font-medium text-blue-700 bg-blue-50 border border-blue-200 rounded-lg hover:bg-blue-100 transition-colors disabled:opacity-50"
              :disabled="testing"
              @click="testPrimaryProvider"
            >
              <Icon v-if="testing" icon="mdi:loading" width="12" height="12" class="animate-spin inline-block mr-1" />
              测试连接
            </button>
            <button
              class="px-3 py-1.5 text-xs font-medium text-white bg-ink-700 rounded-lg hover:bg-ink-800 transition-colors disabled:opacity-50"
              :disabled="saving"
              @click="savePrimaryProvider"
            >
              保存
            </button>
          </div>
        </div>

        <div class="p-5 space-y-4">
          <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div>
              <label class="block text-[11px] font-medium text-gray-500 uppercase tracking-wider mb-1">名称</label>
              <input v-model="primaryProviderForm.name" type="text" class="input w-full text-sm" placeholder="default-primary">
            </div>
            <div>
              <label class="block text-[11px] font-medium text-gray-500 uppercase tracking-wider mb-1">类型</label>
              <select v-model="primaryProviderForm.provider_type" class="input w-full text-sm">
                <option value="openai_compatible">OpenAI Compatible</option>
                <option value="ollama">Ollama (本地)</option>
              </select>
            </div>
            <div>
              <label class="block text-[11px] font-medium text-gray-500 uppercase tracking-wider mb-1">模型</label>
              <input v-model="primaryProviderForm.model" type="text" class="input w-full text-sm" placeholder="gpt-4o-mini">
            </div>
          </div>

          <div>
            <label class="block text-[11px] font-medium text-gray-500 uppercase tracking-wider mb-1">Base URL</label>
            <input
              v-model="primaryProviderForm.base_url"
              type="text"
              class="input w-full text-sm"
              :placeholder="primaryProviderForm.provider_type === 'ollama' ? 'http://localhost:11434/v1' : 'https://api.openai.com/v1'"
            >
          </div>

          <div v-if="primaryProviderForm.provider_type !== 'ollama'">
            <label class="block text-[11px] font-medium text-gray-500 uppercase tracking-wider mb-1">API Key</label>
            <div class="relative">
              <input
                v-model="primaryProviderForm.api_key"
                :type="showPrimaryApiKey ? 'text' : 'password'"
                class="input w-full text-sm pr-10"
                placeholder="留空表示沿用已保存密钥"
              >
              <button class="absolute right-2.5 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600" @click="showPrimaryApiKey = !showPrimaryApiKey">
                <Icon :icon="showPrimaryApiKey ? 'mdi:eye-off' : 'mdi:eye'" width="15" height="15" />
              </button>
            </div>
          </div>
          <div v-else class="rounded-lg bg-amber-50 border border-amber-200/80 px-3 py-2 text-xs text-amber-700 flex items-center gap-2">
            <Icon icon="mdi:information-outline" width="14" height="14" class="shrink-0" />
            <span>Ollama 无需 API Key，确保服务已启动</span>
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="block text-[11px] font-medium text-gray-500 uppercase tracking-wider mb-1">超时 (秒)</label>
              <input v-model.number="primaryProviderForm.timeout_seconds" type="number" min="30" class="input w-full text-sm">
            </div>
            <div>
              <label class="block text-[11px] font-medium text-gray-500 uppercase tracking-wider mb-1">总结时间范围 (分钟)</label>
              <input v-model.number="primaryProviderForm.time_range" type="number" min="60" step="60" class="input w-full text-sm">
            </div>
          </div>

          <label class="flex items-center gap-2.5 cursor-pointer select-none">
            <input v-model="primaryProviderForm.enable_thinking" type="checkbox" class="rounded">
            <span class="text-sm text-gray-700">启用 Thinking（推理模型的思考过程，会消耗额外 token）</span>
          </label>
        </div>
      </div>

      <!-- Section 2: Backup Providers -->
      <div class="rounded-xl border border-gray-200 bg-white overflow-hidden">
        <div class="px-5 py-3.5 border-b border-gray-100 flex items-center justify-between">
          <div class="flex items-center gap-2.5">
            <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-teal-500 to-teal-700 flex items-center justify-center shadow-sm">
              <Icon icon="mdi:server-network" width="16" height="16" class="text-white" />
            </div>
            <div>
              <h3 class="text-sm font-semibold text-gray-900">备用模型池</h3>
              <p class="text-[11px] text-gray-500">挂到能力路由做 failover，主模型挂了自动切</p>
            </div>
          </div>
          <button
            class="px-3 py-1.5 text-xs font-medium rounded-lg border border-gray-200 text-gray-700 hover:bg-gray-50 transition-colors flex items-center gap-1"
            @click="showNewProviderForm = !showNewProviderForm"
          >
            <Icon :icon="showNewProviderForm ? 'mdi:chevron-up' : 'mdi:plus'" width="14" height="14" />
            {{ showNewProviderForm ? '收起' : '新增' }}
          </button>
        </div>

        <div class="p-5 space-y-4">
          <div v-if="showNewProviderForm" class="rounded-lg border border-dashed border-gray-300 p-4 bg-gray-50/60 space-y-3">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
              <input v-model="newProviderForm.name" type="text" class="input w-full text-sm" placeholder="名称">
              <input v-model="newProviderForm.model" type="text" class="input w-full text-sm" placeholder="模型名">
              <select v-model="newProviderForm.provider_type" class="input w-full text-sm">
                <option value="openai_compatible">OpenAI Compatible</option>
                <option value="ollama">Ollama (本地)</option>
              </select>
              <input
                v-model="newProviderForm.base_url"
                type="text"
                class="input w-full text-sm"
                :placeholder="newProviderForm.provider_type === 'ollama' ? 'http://localhost:11434/v1' : 'https://api.example.com/v1'"
              >
              <div v-if="newProviderForm.provider_type !== 'ollama'" class="relative md:col-span-2">
                <input v-model="newProviderForm.api_key" :type="showNewProviderApiKey ? 'text' : 'password'" class="input w-full text-sm pr-10" placeholder="API Key">
                <button class="absolute right-2.5 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600" @click="showNewProviderApiKey = !showNewProviderApiKey">
                  <Icon :icon="showNewProviderApiKey ? 'mdi:eye-off' : 'mdi:eye'" width="15" height="15" />
                </button>
              </div>
              <div v-else class="md:col-span-2 rounded-lg bg-amber-50 border border-amber-200/80 px-3 py-2 text-xs text-amber-700">
                Ollama 模式无需 API Key
              </div>
              <label class="flex items-center gap-2 text-sm text-gray-700 self-center">
                <input v-model="newProviderForm.enable_thinking" type="checkbox" class="rounded">
                Thinking
              </label>
            </div>
            <div class="flex justify-end">
              <button class="px-3 py-1.5 text-xs font-medium text-white bg-teal-600 rounded-lg hover:bg-teal-700 transition-colors disabled:opacity-50" :disabled="saving" @click="saveNewProvider">
                添加
              </button>
            </div>
          </div>

          <div v-if="backupProviders.length === 0" class="text-center py-6 text-xs text-gray-400">
            还没有备用模型，先加一个
          </div>

          <div v-else class="space-y-2">
            <div
              v-for="provider in backupProviders"
              :key="provider.id"
              class="rounded-lg border border-gray-150 bg-gray-50/40 px-4 py-3"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="flex items-center gap-3 min-w-0">
                  <div class="w-7 h-7 rounded-md bg-gradient-to-br from-gray-200 to-gray-300 flex items-center justify-center shrink-0">
                    <Icon icon="mdi:cube-outline" width="14" height="14" class="text-gray-600" />
                  </div>
                  <div class="min-w-0">
                    <div class="text-sm font-medium text-gray-900 truncate">{{ provider.name }}</div>
                    <div class="text-[11px] text-gray-500 truncate">{{ provider.model }} · {{ provider.base_url }}</div>
                  </div>
                </div>
                <div class="flex items-center gap-1.5 shrink-0">
                  <span class="text-[10px] px-1.5 py-0.5 rounded-full font-medium" :class="provider.enabled ? 'bg-emerald-50 text-emerald-600' : 'bg-gray-100 text-gray-400'">
                    {{ provider.enabled ? '启用' : '停用' }}
                  </span>
                  <button class="p-1 rounded hover:bg-gray-200 text-gray-400 hover:text-gray-600 transition-colors" @click="startEditingProvider(provider)">
                    <Icon icon="mdi:pencil-outline" width="14" height="14" />
                  </button>
                  <button
                    class="p-1 rounded hover:bg-red-100 text-gray-400 hover:text-red-600 transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                    :disabled="isProviderLinked(provider.id)"
                    @click="deleteBackupProvider(provider)"
                  >
                    <Icon icon="mdi:trash-can-outline" width="14" height="14" />
                  </button>
                </div>
              </div>
              <p v-if="isProviderLinked(provider.id)" class="mt-2 text-[11px] text-amber-600 pl-10">
                还挂在某条路由上，先移除再删
              </p>

              <div v-if="editingProviderId === provider.id" class="mt-3 pt-3 border-t border-gray-200 space-y-3">
                <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                  <input v-model="editProviderForm.name" type="text" class="input w-full text-sm" placeholder="名称">
                  <input v-model="editProviderForm.model" type="text" class="input w-full text-sm" placeholder="模型名">
                  <select v-model="editProviderForm.provider_type" class="input w-full text-sm">
                    <option value="openai_compatible">OpenAI Compatible</option>
                    <option value="ollama">Ollama (本地)</option>
                  </select>
                  <input
                    v-model="editProviderForm.base_url"
                    type="text"
                    class="input w-full text-sm"
                    :placeholder="editProviderForm.provider_type === 'ollama' ? 'http://localhost:11434/v1' : 'https://api.example.com/v1'"
                  >
                  <div v-if="editProviderForm.provider_type !== 'ollama'" class="relative md:col-span-2">
                    <input v-model="editProviderForm.api_key" :type="showEditProviderApiKey ? 'text' : 'password'" class="input w-full text-sm pr-10" placeholder="留空表示沿用已保存密钥">
                    <button class="absolute right-2.5 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600" @click="showEditProviderApiKey = !showEditProviderApiKey">
                      <Icon :icon="showEditProviderApiKey ? 'mdi:eye-off' : 'mdi:eye'" width="15" height="15" />
                    </button>
                  </div>
                  <div v-else class="md:col-span-2 rounded-lg bg-amber-50 border border-amber-200/80 px-3 py-2 text-xs text-amber-700">
                    Ollama 模式无需 API Key
                  </div>
                  <input v-model.number="editProviderForm.timeout_seconds" type="number" min="30" class="input w-full text-sm" placeholder="Timeout (秒)">
                  <label class="flex items-center gap-2 text-sm text-gray-700 self-center">
                    <input v-model="editProviderForm.enabled" type="checkbox" class="rounded">
                    启用
                  </label>
                  <label class="flex items-center gap-2 text-sm text-gray-700 self-center">
                    <input v-model="editProviderForm.enable_thinking" type="checkbox" class="rounded">
                    Thinking
                  </label>
                </div>
                <div class="flex justify-end gap-2">
                  <button class="px-3 py-1.5 text-xs rounded-lg border border-gray-200 text-gray-700 hover:bg-gray-50" @click="cancelEditingProvider">取消</button>
                  <button class="px-3 py-1.5 text-xs rounded-lg bg-ink-700 text-white hover:bg-ink-800 disabled:opacity-50" :disabled="saving" @click="saveEditedProvider">保存</button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Section 3: Capability Routes -->
      <div class="rounded-xl border border-gray-200 bg-white overflow-hidden">
        <div class="px-5 py-3.5 border-b border-gray-100 flex items-center justify-between">
          <div class="flex items-center gap-2.5">
            <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-slate-600 to-slate-800 flex items-center justify-center shadow-sm">
              <Icon icon="mdi:transit-connection-variant" width="16" height="16" class="text-white" />
            </div>
            <div>
              <h3 class="text-sm font-semibold text-gray-900">能力路由</h3>
              <p class="text-[11px] text-gray-500">按顺序依次尝试，失败自动降级到下一个</p>
            </div>
          </div>
          <button
            class="px-3 py-1.5 text-xs font-medium text-white bg-slate-700 rounded-lg hover:bg-slate-800 transition-colors disabled:opacity-50"
            :disabled="saving"
            @click="saveRoutes"
          >
            <Icon v-if="saving" icon="mdi:loading" width="12" height="12" class="animate-spin inline-block mr-1" />
            保存路由
          </button>
        </div>

        <div class="divide-y divide-gray-100">
          <div v-for="capability in capabilityOrder" :key="capability" class="px-5 py-4">
            <div class="flex items-center gap-2 mb-3">
              <div
                class="w-6 h-6 rounded flex items-center justify-center text-[10px] font-bold shrink-0"
                :class="routeSummary(capability).length > 0 ? 'bg-slate-700 text-white' : 'bg-gray-200 text-gray-500'"
              >
                {{ routeLabels[capability]?.charAt(0) }}
              </div>
              <span class="text-sm font-medium text-gray-800">{{ routeLabels[capability] }}</span>
              <span class="text-[11px] text-gray-400">{{ routeSummary(capability).length }} provider</span>
            </div>

            <div v-if="routeSummary(capability).length === 0" class="text-center py-3 text-[11px] text-gray-400 rounded-lg border border-dashed border-gray-200 mb-3">
              点击下方按钮添加 provider
            </div>

            <div v-else class="space-y-1.5 mb-3">
              <div
                v-for="(providerId, index) in routeSummary(capability)"
                :key="providerId"
                draggable="true"
                class="flex items-center gap-2 px-3 py-2 rounded-lg border transition-all cursor-move select-none"
                :class="[
                  providerId === primaryProviderId
                    ? 'border-ink-200/80 bg-ink-50/50'
                    : 'border-gray-200 bg-gray-50/50 hover:bg-gray-100/60',
                  draggingCapability === capability && draggingProviderId === providerId ? 'opacity-40 ring-2 ring-blue-300' : ''
                ]"
                @dragstart="handleDragStart(capability, providerId)"
                @dragend="handleDragEnd"
                @dragover.prevent
                @drop.prevent="handleDropOnProvider(capability, providerId)"
              >
                <span
                  class="w-5 h-5 rounded-full flex items-center justify-center text-[10px] font-bold shrink-0"
                  :class="index === 0 ? 'bg-ink-700 text-white' : 'bg-gray-300 text-gray-600'"
                >
                  {{ index + 1 }}
                </span>
                <Icon icon="mdi:drag" width="12" height="12" class="text-gray-300 shrink-0" />
                <div class="flex-1 min-w-0">
                  <span class="text-sm truncate" :class="providerId === primaryProviderId ? 'font-medium text-ink-900' : 'text-gray-700'">
                    {{ providerName(providerId) }}
                  </span>
                </div>
                <span
                  v-if="providerId === primaryProviderId"
                  class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-ink-100 text-ink-600 shrink-0"
                >
                  主
                </span>
                <span v-else class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-teal-50 text-teal-600 shrink-0">备</span>
                <div class="flex items-center gap-0.5 shrink-0">
                  <button
                    class="p-0.5 rounded hover:bg-gray-200 text-gray-400 hover:text-gray-600 transition-colors disabled:opacity-30"
                    :disabled="index === 0"
                    @click="moveProvider(capability, providerId, -1)"
                  >
                    <Icon icon="mdi:chevron-up" width="14" height="14" />
                  </button>
                  <button
                    class="p-0.5 rounded hover:bg-gray-200 text-gray-400 hover:text-gray-600 transition-colors disabled:opacity-30"
                    :disabled="index === routeSummary(capability).length - 1"
                    @click="moveProvider(capability, providerId, 1)"
                  >
                    <Icon icon="mdi:chevron-down" width="14" height="14" />
                  </button>
                  <button
                    class="p-0.5 rounded hover:bg-red-100 text-gray-400 hover:text-red-500 transition-colors"
                    @click="removeProviderFromRoute(capability, providerId)"
                  >
                    <Icon icon="mdi:close" width="13" height="13" />
                  </button>
                </div>
              </div>
            </div>

            <div class="flex flex-wrap gap-1.5">
              <button
                v-if="primaryProviderId && !routeSummary(capability).includes(primaryProviderId)"
                class="px-2 py-0.5 text-[11px] font-medium rounded border border-ink-200 bg-ink-50 text-ink-700 hover:bg-ink-100 transition-colors"
                @click="addPrimaryToRoute(capability)"
              >
                + {{ primaryProviderForm.name || '主模型' }}
              </button>
              <button
                v-for="provider in backupProviders"
                :key="provider.id"
                class="px-2 py-0.5 text-[11px] font-medium rounded border transition-colors"
                :class="routeSummary(capability).includes(provider.id) ? 'border-teal-200 bg-teal-50 text-teal-700' : 'border-gray-200 bg-white text-gray-500 hover:bg-gray-50'"
                @click="routeSummary(capability).includes(provider.id) ? removeProviderFromRoute(capability, provider.id) : addProviderToRoute(capability, provider.id)"
              >
                {{ routeSummary(capability).includes(provider.id) ? '✓' : '+' }} {{ provider.name }}
              </button>
              <span v-if="!primaryProviderId && backupProviders.length === 0" class="text-[11px] text-gray-400 self-center">
                先在上方创建 provider
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- Section 4: Embedding Threshold -->
      <div class="rounded-xl border border-gray-200 bg-white overflow-hidden">
        <div class="px-5 py-3.5 border-b border-gray-100 flex items-center justify-between">
          <div class="flex items-center gap-2.5">
            <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-violet-500 to-violet-700 flex items-center justify-center shadow-sm">
              <Icon icon="mdi:tune-variant" width="16" height="16" class="text-white" />
            </div>
            <div>
              <h3 class="text-sm font-semibold text-gray-900">板块匹配阈值</h3>
              <p class="text-[11px] text-gray-500">Embedding 相似度阈值，越低标签越容易匹配到板块</p>
            </div>
          </div>
          <button
            class="px-3 py-1.5 text-xs font-medium text-white bg-violet-600 rounded-lg hover:bg-violet-700 transition-colors disabled:opacity-50"
            :disabled="savingThreshold"
            @click="saveThreshold"
          >
            <Icon v-if="savingThreshold" icon="mdi:loading" width="12" height="12" class="animate-spin inline-block mr-1" />
            保存
          </button>
        </div>
        <div class="p-5">
          <div class="flex items-center gap-4">
            <input
              v-model.number="embeddingThreshold"
              type="range"
              min="0.1"
              max="0.95"
              step="0.05"
              class="flex-1 h-2 rounded-lg appearance-none cursor-pointer"
              :style="{ background: `linear-gradient(to right, #8b5cf6 0%, #8b5cf6 ${(embeddingThreshold - 0.1) / 0.85 * 100}%, #e5e7eb ${(embeddingThreshold - 0.1) / 0.85 * 100}%, #e5e7eb 100%)` }"
            >
            <span class="text-base font-bold text-violet-700 w-12 text-right tabular-nums">{{ embeddingThreshold.toFixed(2) }}</span>
          </div>
          <div class="flex justify-between mt-1">
            <span class="text-[10px] text-gray-400">0.10 宽松</span>
            <span class="text-[10px] text-gray-400">0.95 严格</span>
          </div>
        </div>
      </div>
    </template>

    <div v-if="success" class="rounded-lg bg-emerald-50 border border-emerald-200 px-4 py-2.5 text-xs text-emerald-700 flex items-center gap-2">
      <Icon icon="mdi:check-circle" width="14" height="14" />
      {{ success }}
    </div>
    <div v-if="error" class="rounded-lg bg-red-50 border border-red-200 px-4 py-2.5 text-xs text-red-700 flex items-center gap-2">
      <Icon icon="mdi:alert-circle" width="14" height="14" />
      {{ error }}
    </div>
  </div>
</template>
