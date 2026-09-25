<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { onMounted, ref } from 'vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import { useSearxngApi } from '~/api'

// SearXNG 本地搜索配置（daily-report-margin-notes design D7）：
// 页边注问答联网后端，每次提问现读配置——界面改即时生效，无需重启。
// 无 API key（本地实例），失败静默降级为纯文章+模型知识。

const enabled = ref(false)
const endpoint = ref('')
const loading = ref(false)
const error = ref<string | null>(null)
const success = ref<string | null>(null)

onMounted(async () => {
  loading.value = true
  error.value = null
  try {
    const { getConfig } = useSearxngApi()
    const response = await getConfig()
    if (response.success && response.data) {
      enabled.value = response.data.enabled
      endpoint.value = response.data.endpoint
    }
  } catch {
    error.value = '加载 SearXNG 设置失败'
  } finally {
    loading.value = false
  }
})

async function save() {
  loading.value = true
  error.value = null
  success.value = null
  try {
    const { saveSettings } = useSearxngApi()
    const response = await saveSettings({ endpoint: endpoint.value.trim(), enabled: enabled.value })
    if (!response.success) {
      throw new Error(response.error || '保存失败')
    }
    success.value = 'SearXNG 设置已保存，下次提问即时生效'
    setTimeout(() => { success.value = null }, 2000)
  } catch {
    error.value = '保存 SearXNG 设置失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="space-y-6">
    <div v-if="loading && !endpoint && !enabled" class="flex items-center justify-center py-12">
      <Icon icon="mdi:loading" width="48" height="48" class="animate-spin" style="color: var(--color-link)" />
    </div>
    <template v-else>
      <div v-if="success" class="p-3 rounded-lg text-sm" style="background: var(--color-success-bg, rgba(61, 138, 74, 0.1)); border: 1px solid var(--color-success-border, rgba(61, 138, 74, 0.25)); color: var(--color-success)">
        {{ success }}
      </div>
      <div v-if="error" class="p-3 rounded-lg text-sm" style="background: var(--color-error-bg, rgba(196, 47, 60, 0.1)); border: 1px solid var(--color-error-border, rgba(196, 47, 60, 0.25)); color: var(--color-error)">
        {{ error }}
      </div>

      <div class="flex items-center justify-between">
        <div>
          <h3 class="font-semibold" style="color: var(--color-text-primary)">SearXNG 本地搜索</h3>
          <p class="text-sm mt-0.5" style="color: var(--color-text-secondary)">
            日报页边注问答的联网补强后端；搜索失败或未配置时静默降级，不影响问答
          </p>
        </div>
        <AppToggle v-model="enabled" />
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label class="block text-sm font-medium mb-1" style="color: var(--color-text-secondary)">实例地址</label>
          <input
            :value="endpoint"
            type="text"
            class="w-full px-3 py-2 rounded-lg text-sm input"
            placeholder="http://localhost:8889"
            @input="endpoint = ($event.target as HTMLInputElement).value"
          />
          <p class="text-xs mt-1" style="color: var(--color-text-muted)">
            本地 SearXNG 实例的地址（JSON API 需启用），无需 API Key
          </p>
        </div>
      </div>

      <div class="flex justify-end">
        <button
          type="button"
          class="px-4 py-2 text-white rounded-lg text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          style="background: var(--color-accent)"
          :disabled="loading"
          @click="save"
        >
          <span v-if="loading" class="flex items-center gap-2">
            <Icon icon="mdi:loading" width="16" class="animate-spin" />
            保存中...
          </span>
          <span v-else>保存设置</span>
        </button>
      </div>
    </template>
  </div>
</template>
