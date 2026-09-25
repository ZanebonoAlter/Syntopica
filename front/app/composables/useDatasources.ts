import { ref } from 'vue'
import { useDatasourcesApi, type DataSourceEntry, type ProbeResult } from '~/api/datasources'

export function useDatasources() {
  const sources = ref<DataSourceEntry[]>([])
  const catalogLoading = ref(false)
  const catalogError = ref<string | null>(null)

  // Comtrade key 管理（同博查语义：脱敏回显、空串不改）
  const comtradeEnabled = ref(true)
  const comtradeApiKey = ref('')
  const comtradeApiKeyConfigured = ref(false)
  const comtradeApiKeyHint = ref('')
  const comtradeApiKeyVisible = ref(false)
  const comtradeLoading = ref(false)
  const comtradeError = ref<string | null>(null)
  const comtradeSuccess = ref<string | null>(null)

  // probe 逐源状态
  const probing = ref<Record<string, boolean>>({})
  const probeResults = ref<Record<string, { ok: boolean; text: string }>>({})

  async function loadCatalog() {
    catalogLoading.value = true
    catalogError.value = null
    try {
      const { listCatalog } = useDatasourcesApi()
      const res = await listCatalog()
      if (res.success && res.data?.data_sources) {
        sources.value = res.data.data_sources
      } else {
        throw new Error(res.error || '加载失败')
      }
    } catch {
      catalogError.value = '加载研究数据源目录失败'
    } finally {
      catalogLoading.value = false
    }
  }

  async function loadComtrade() {
    try {
      const { getComtradeStatus } = useDatasourcesApi()
      const res = await getComtradeStatus()
      if (res.success && res.data) {
        comtradeEnabled.value = res.data.enabled
        comtradeApiKeyConfigured.value = res.data.api_key_configured === true
        comtradeApiKeyHint.value = res.data.api_key_hint || ''
      }
    } catch {
      comtradeError.value = '加载 Comtrade 配置失败'
    }
  }

  async function saveComtrade() {
    comtradeLoading.value = true
    comtradeError.value = null
    comtradeSuccess.value = null
    try {
      const { saveComtradeSettings } = useDatasourcesApi()
      const res = await saveComtradeSettings({
        enabled: comtradeEnabled.value,
        api_key: comtradeApiKey.value,
      })
      if (!res.success) throw new Error(res.error || '保存失败')
      if (comtradeApiKey.value) {
        comtradeApiKeyConfigured.value = true
        comtradeApiKey.value = ''
      }
      comtradeSuccess.value = '已保存，即时生效'
      await loadCatalog() // status 可能随 key 配置翻转
    } catch (e) {
      comtradeError.value = e instanceof Error ? e.message : '保存失败'
    } finally {
      comtradeLoading.value = false
    }
  }

  async function runProbe(code: string) {
    probing.value = { ...probing.value, [code]: true }
    try {
      const { probe } = useDatasourcesApi()
      const res = await probe(code)
      if (res.success && res.data) {
        const p = res.data as ProbeResult
        const rows = (p.summary as { rows?: number }).rows
        probeResults.value = {
          ...probeResults.value,
          [code]: { ok: true, text: `连通正常 · ${rows ?? '?'} 行 · ${p.elapsed_ms}ms` },
        }
      } else {
        const detail = (res as unknown as { error_code?: string; message?: string; detail?: string })
        probeResults.value = {
          ...probeResults.value,
          [code]: { ok: false, text: detail.detail || detail.message || '取数失败' },
        }
      }
    } catch {
      probeResults.value = { ...probeResults.value, [code]: { ok: false, text: '请求失败' } }
    } finally {
      probing.value = { ...probing.value, [code]: false }
    }
  }

  return {
    sources,
    catalogLoading,
    catalogError,
    loadCatalog,
    comtradeEnabled,
    comtradeApiKey,
    comtradeApiKeyConfigured,
    comtradeApiKeyHint,
    comtradeApiKeyVisible,
    comtradeLoading,
    comtradeError,
    comtradeSuccess,
    loadComtrade,
    saveComtrade,
    probing,
    probeResults,
    runProbe,
  }
}
