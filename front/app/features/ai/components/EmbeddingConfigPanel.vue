<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { useEmbeddingConfigApi, type EmbeddingConfigItem } from '~/api'

const loading = ref(false)
const saving = ref(false)
const error = ref<string | null>(null)
const success = ref<string | null>(null)

const configs = ref<EmbeddingConfigItem[]>([])
const editValues = ref<Record<string, string>>({})
const dirtyKeys = ref<Set<string>>(new Set())

const configLabels: Record<string, { label: string; hint: string; unit?: string }> = {
  high_similarity_threshold: { label: '高相似度阈值', hint: '标签自动复用的最低相似度，高于此值直接复用', unit: '0.0-1.0' },
  low_similarity_threshold: { label: '低相似度阈值', hint: '低于此值创建新标签，中间地带也创建新标签', unit: '0.0-1.0' },
  embedding_dimension: { label: 'Embedding 维度', hint: '实际维度由模型自动决定，此值仅供参考（如 nomic-embed=768, 3-small=1536）', unit: '' },
}

function getLabel(key: string): string {
  return configLabels[key]?.label || key
}

function getHint(key: string): string {
  return configLabels[key]?.hint || ''
}

function getUnit(key: string): string {
  return configLabels[key]?.unit || ''
}

function markDirty(key: string) {
  dirtyKeys.value.add(key)
}

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

async function loadConfig() {
  loading.value = true
  error.value = null
  try {
    const api = useEmbeddingConfigApi()
    const response = await api.getConfig()
    if (!response.success || !response.data) {
      throw new Error(response.error || '加载 embedding 配置失败')
    }
    configs.value = response.data.filter(c => configLabels[c.key])
    const values: Record<string, string> = {}
    for (const item of configs.value) {
      values[item.key] = item.value
    }
    editValues.value = values
    dirtyKeys.value.clear()
  } catch (err) {
    pushMessage('error', err instanceof Error ? err.message : '加载失败')
  } finally {
    loading.value = false
  }
}

async function saveConfig() {
  if (dirtyKeys.value.size === 0) {
    pushMessage('success', '没有修改')
    return
  }

  for (const key of dirtyKeys.value) {
    if (key.endsWith('_threshold')) {
      const val = parseFloat(editValues.value[key] || '')
      if (isNaN(val) || val < 0 || val > 1) {
        pushMessage('error', `${getLabel(key)} 必须是 0.0-1.0 之间的数字`)
        return
      }
    }
  }

  saving.value = true
  error.value = null
  try {
    const api = useEmbeddingConfigApi()
    for (const key of dirtyKeys.value) {
      const response = await api.updateConfig(key, editValues.value[key] || '')
      if (!response.success) {
        throw new Error(response.error || `保存 ${getLabel(key)} 失败`)
      }
    }
    dirtyKeys.value.clear()
    await loadConfig()
    pushMessage('success', 'Embedding 配置已保存')
  } catch (err) {
    pushMessage('error', err instanceof Error ? err.message : '保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadConfig()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between gap-4">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 rounded-lg bg-gradient-to-br from-teal-500 to-teal-700 flex items-center justify-center">
          <Icon icon="mdi:vector-square" width="20" height="20" class="text-white" />
        </div>
        <div>
          <h3 class="font-semibold text-gray-900">Embedding 配置</h3>
          <p class="text-xs text-gray-500">向量搜索阈值与维度，模型在 AI 路由中配置</p>
        </div>
      </div>
      <button
        class="px-4 py-2 text-sm font-medium text-white bg-teal-600 rounded-lg hover:bg-teal-700 transition-colors disabled:opacity-50"
        :disabled="saving || dirtyKeys.size === 0"
        @click="saveConfig"
      >
        {{ saving ? '保存中...' : dirtyKeys.size > 0 ? `保存 (${dirtyKeys.size})` : '已保存' }}
      </button>
    </div>

    <div v-if="loading" class="py-8 flex justify-center">
      <Icon icon="mdi:loading" width="28" height="28" class="animate-spin text-teal-600" />
    </div>

    <div v-else class="space-y-3">
      <div v-for="config in configs" :key="config.key" class="rounded-lg border border-gray-200 p-4 bg-gray-50/50">
        <div class="flex items-center justify-between gap-4 mb-1.5">
          <label class="block text-sm font-medium text-gray-700">{{ getLabel(config.key) }}</label>
          <span v-if="getUnit(config.key)" class="text-xs text-gray-400">{{ getUnit(config.key) }}</span>
        </div>
        <input
          v-model="editValues[config.key]"
          type="text"
          class="input w-full"
          :class="dirtyKeys.has(config.key) ? 'border-teal-400 ring-1 ring-teal-200' : ''"
          @input="markDirty(config.key)"
        >
        <p class="mt-1 text-xs text-gray-500">{{ getHint(config.key) }}</p>
      </div>
    </div>

    <div v-if="success" class="rounded-lg bg-emerald-50 border border-emerald-200 px-4 py-3 text-sm text-emerald-700">
      {{ success }}
    </div>
    <div v-if="error" class="rounded-lg bg-red-50 border border-red-200 px-4 py-3 text-sm text-red-700">
      {{ error }}
    </div>
  </div>
</template>
