<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useHierarchyConfigApi } from '~/api/hierarchyConfig'
import type { HierarchyPendingChange } from '~/api/hierarchyConfig'

const api = useHierarchyConfigApi()
const pending = ref<HierarchyPendingChange[]>([])
const statusFilter = ref('pending')
const loading = ref(false)
const rebuilding = ref(false)
const error = ref('')

onMounted(load)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await api.getPending(statusFilter.value)
    if (result.success && result.data) {
      pending.value = result.data
    } else {
      error.value = result.error || '加载失败'
    }
  } catch (e: any) {
    error.value = e.message || '网络错误'
  } finally {
    loading.value = false
  }
}

async function triggerRebuild(category?: string) {
  rebuilding.value = true
  try {
    const result = await api.triggerRebuild(category)
    if (result.success) {
      await load()
    } else {
      error.value = result.error || '触发重建失败'
    }
  } catch (e: any) {
    error.value = e.message || '网络错误'
  } finally {
    rebuilding.value = false
  }
}
</script>

<template>
  <div class="p-4">
    <div class="flex items-center gap-3 mb-4">
      <h2 class="text-lg font-bold">待处理标签变更</h2>
      <select
        v-model="statusFilter"
        @change="load"
        class="border rounded px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-800"
      >
        <option value="pending">待处理</option>
        <option value="resolved">已处理</option>
      </select>
    </div>
    <div v-if="loading" class="text-sm text-gray-500">加载中...</div>
    <div v-else-if="pending.length === 0" class="text-sm text-gray-500">暂无数据</div>
    <div v-else class="space-y-2">
      <div
        v-for="item in pending"
        :key="item.id"
        class="border rounded-lg p-3 dark:border-gray-700"
      >
        <div class="flex items-center gap-2">
          <span class="font-bold text-sm">{{ item.tag_label }}</span>
          <span class="text-xs text-gray-500 bg-gray-100 dark:bg-gray-800 px-2 py-0.5 rounded">
            {{ item.change_type }}
          </span>
        </div>
        <p class="text-sm text-gray-600 dark:text-gray-400 mt-1">{{ item.reason }}</p>
        <p v-if="item.current_parent_label" class="text-xs text-gray-400 mt-1">
          当前父节点: {{ item.current_parent_label }}
        </p>
      </div>
    </div>
    <button
      @click="triggerRebuild()"
      :disabled="rebuilding"
      class="mt-4 px-4 py-2 bg-blue-600 text-white rounded text-sm hover:bg-blue-700 disabled:opacity-50"
    >
      {{ rebuilding ? '重建中...' : '重新整理层级' }}
    </button>
    <p v-if="error" class="mt-2 text-sm text-red-500">{{ error }}</p>
  </div>
</template>
