<script setup lang="ts">
import { ref } from 'vue'
import { useHierarchyConfigApi } from '~/api/hierarchyConfig'
import { useWebSocketRebuild } from '~/composables/useWebSocketRebuild'

const api = useHierarchyConfigApi()
const { status, total, processed, category: wsCategory, currentTag, errorMessage, reset } = useWebSocketRebuild()

const categories = [
  { value: '', label: '全部' },
  { value: 'event', label: '事件' },
  { value: 'keyword', label: '关键词' },
  { value: 'person', label: '人物' },
] as const

const selectedCategory = ref('')
const dryRun = ref(false)
const triggering = ref(false)
const error = ref('')
const result = ref<{ total_pending: number; processed: number; dry_run?: boolean } | null>(null)

async function trigger() {
  triggering.value = true
  error.value = ''
  result.value = null
  reset()

  try {
    const category = selectedCategory.value || undefined
    const res = await api.triggerRebuild(category, dryRun.value)
    if (res.success && res.data) {
      result.value = res.data
    } else {
      error.value = res.error || '触发重建失败'
    }
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : '网络错误'
  } finally {
    triggering.value = false
  }
}

const progressPercent = computed(() => {
  if (total.value === 0) return 0
  return Math.round((processed.value / total.value) * 100)
})
</script>

<template>
  <div class="p-4 border rounded-lg dark:border-gray-700 space-y-4">
    <h2 class="text-lg font-bold">层级重建</h2>

    <div class="flex items-center gap-4 flex-wrap">
      <label class="text-sm font-medium">分类</label>
      <select
        v-model="selectedCategory"
        :disabled="triggering"
        class="border rounded px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-800"
      >
        <option v-for="cat in categories" :key="cat.value" :value="cat.value">
          {{ cat.label }}
        </option>
      </select>

      <label class="text-sm flex items-center gap-2">
        <input type="checkbox" v-model="dryRun" :disabled="triggering" />
        Dry Run
      </label>

      <button
        @click="trigger"
        :disabled="triggering"
        class="px-4 py-2 bg-blue-600 text-white rounded text-sm hover:bg-blue-700 disabled:opacity-50 transition-colors"
      >
        {{ triggering ? '执行中...' : '触发重建' }}
      </button>
    </div>

    <div v-if="error" class="text-sm text-red-500">{{ error }}</div>

    <div v-if="errorMessage" class="text-sm text-red-500">{{ errorMessage }}</div>

    <div v-if="status === 'processing'" class="space-y-1">
      <div class="text-sm text-gray-600 dark:text-gray-400">
        重建中... {{ processed }} / {{ total }} ({{ progressPercent }}%)
      </div>
      <div class="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
        <div
          class="bg-blue-600 h-2 rounded-full transition-all"
          :style="{ width: `${progressPercent}%` }"
        />
      </div>
      <div v-if="currentTag" class="text-xs text-gray-500 truncate">
        当前: {{ currentTag }}
      </div>
    </div>

    <div v-if="status === 'completed'" class="text-sm text-green-600">
      重建完成，共处理 {{ processed }} 项{{ wsCategory ? ` (${wsCategory})` : '' }}
    </div>

    <div v-if="status === 'failed'" class="text-sm text-red-500">
      重建失败: {{ errorMessage }}
    </div>

    <div v-if="result && status === 'idle'" class="text-sm text-gray-600 dark:text-gray-400">
      <span v-if="result.dry_run" class="text-amber-600">[Dry Run] </span>
      待处理 {{ result.total_pending }} 项，已处理 {{ result.processed }} 项
    </div>
  </div>
</template>
