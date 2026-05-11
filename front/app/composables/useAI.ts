import { useAIAdminApi } from '~/api'

interface ApiResponse<T> {
  success: boolean
  data?: T
  message?: string
  error?: string
}

interface AISettingsState {
  baseURL: string
  model: string
  providerId: number | null
  providerName: string
  routeName: string
  summaryEnabled: boolean
  apiKeyConfigured: boolean
  timeRange: number
}

function createDefaultSettings(): AISettingsState {
  return {
    baseURL: '',
    model: '',
    providerId: null,
    providerName: '',
    routeName: '',
    summaryEnabled: false,
    apiKeyConfigured: false,
    timeRange: 180,
  }
}

function normalizeSettings(payload: Record<string, unknown> | null | undefined): AISettingsState {
  return {
    baseURL: (payload?.base_url as string) || '',
    model: (payload?.model as string) || '',
    providerId: typeof payload?.provider_id === 'number' ? (payload.provider_id as number) : null,
    providerName: (payload?.provider_name as string) || '',
    routeName: (payload?.route_name as string) || '',
    summaryEnabled: Boolean(payload?.provider_id || payload?.base_url),
    apiKeyConfigured: Boolean(payload?.api_key_configured),
    timeRange: typeof payload?.time_range === 'number' ? (payload.time_range as number) : 180,
  }
}

export const useAI = () => {
  const settingsState = useState<AISettingsState>('ai-settings', createDefaultSettings)
  const settingsLoaded = useState<boolean>('ai-settings-loaded', () => false)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const aiSettings = computed(() => settingsState.value)
  const isAIEnabled = computed(() => settingsState.value.summaryEnabled && settingsState.value.apiKeyConfigured)

  async function loadSettings(force = false) {
    if (settingsLoaded.value && !force) {
      return settingsState.value
    }

    const aiAdminApi = useAIAdminApi()
    const response = await aiAdminApi.getSettings()
    if (response.success) {
      settingsState.value = normalizeSettings(response.data)
      settingsLoaded.value = true
    } else {
      settingsState.value = normalizeSettings(null)
      settingsLoaded.value = false
    }

    return settingsState.value
  }

  if (import.meta.client && !settingsLoaded.value) {
    loadSettings().catch(() => {
      settingsLoaded.value = false
    })
  }

  const testConnection = async (config?: {
    baseURL?: string
    apiKey?: string
    model?: string
  }): Promise<ApiResponse<void>> => {
    loading.value = true
    error.value = null

    try {
      const loadedSettings = await loadSettings()
      const settings = {
        baseURL: config?.baseURL || loadedSettings.baseURL,
        apiKey: config?.apiKey || '',
        model: config?.model || loadedSettings.model,
      }

      if (!settings.baseURL || !settings.apiKey || !settings.model) {
        throw new Error('AI 配置不完整')
      }

      const aiAdminApi = useAIAdminApi()
      const response = await aiAdminApi.testConnection({
        base_url: settings.baseURL,
        api_key: settings.apiKey,
        model: settings.model,
      })

      if (!response.success) {
        return {
          success: false,
          error: response.error || '连接测试失败',
        }
      }

      return {
        success: true,
        message: response.message,
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : '未知错误'
      error.value = errorMessage
      return {
        success: false,
        error: errorMessage,
      }
    } finally {
      loading.value = false
    }
  }

  return {
    loading,
    error,
    aiSettings,
    isAIEnabled,
    loadSettings,
    testConnection,
  }
}
