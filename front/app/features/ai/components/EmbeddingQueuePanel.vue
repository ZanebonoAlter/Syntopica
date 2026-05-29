<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { useEmbeddingQueueApi, type EmbeddingQueueStatus, type EmbeddingQueueTask } from '~/api'

const loading = ref(false)
const error = ref<string | null>(null)
const status = ref<EmbeddingQueueStatus>({
  pending: 0,
  processing: 0,
  completed: 0,
  failed: 0,
  total: 0,
})
const tasks = ref<EmbeddingQueueTask[]>([])
const totalTasks = ref(0)
const statusFilter = ref('')
const currentPage = ref(1)
const pageSize = 20
const retrying = ref(false)

let refreshTimer: ReturnType<typeof setInterval> | null = null

const api = useEmbeddingQueueApi()

async function loadStatus() {
  try {
    const response = await api.getStatus()
    if (response.success && response.data) {
      status.value = response.data
    }
  } catch (err) {
    console.error('Failed to load queue status:', err)
  }
}

async function loadTasks() {
  loading.value = true
  error.value = null
  try {
    const response = await api.getTasks({
      status: statusFilter.value || undefined,
      limit: pageSize,
      offset: (currentPage.value - 1) * pageSize,
    })
    if (response.success && response.data) {
      tasks.value = response.data.tasks
      totalTasks.value = response.data.total
    } else {
      throw new Error(response.error || '加载任务列表失败')
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function retryFailed() {
  retrying.value = true
  try {
    const response = await api.retryFailed()
    if (response.success) {
      await Promise.all([loadStatus(), loadTasks()])
    }
  } catch (err) {
    console.error('Failed to retry:', err)
  } finally {
    retrying.value = false
  }
}

function getStatusColor(s: string) {
  switch (s) {
    case 'pending': return 'bg-yellow-100 text-yellow-800'
    case 'processing': return 'bg-blue-100 text-blue-800'
    case 'completed': return 'bg-green-100 text-green-800'
    case 'failed': return 'bg-red-100 text-red-800'
    default: return 'bg-gray-100 text-gray-800'
  }
}

function getStatusLabel(s: string) {
  switch (s) {
    case 'pending': return '待处理'
    case 'processing': return '处理中'
    case 'completed': return '已完成'
    case 'failed': return '失败'
    default: return s
  }
}

function formatDate(dateStr: string | null) {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString('zh-CN')
}

const progressPercent = computed(() => {
  if (status.value.total === 0) return 0
  return Math.round((status.value.completed / status.value.total) * 100)
})

const totalPages = computed(() => Math.ceil(totalTasks.value / pageSize))

function changePage(page: number) {
  currentPage.value = page
  loadTasks()
}

function changeFilter(value: string) {
  statusFilter.value = value
  currentPage.value = 1
  loadTasks()
}

async function refreshAll() {
  await Promise.all([loadStatus(), loadTasks()])
}

onMounted(async () => {
  await refreshAll()
  refreshTimer = setInterval(loadStatus, 5000)
})

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between gap-4">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 rounded-lg bg-gradient-to-br from-purple-500 to-purple-700 flex items-center justify-center">
          <Icon icon="mdi:playlist-check" width="20" height="20" class="text-white" />
        </div>
        <div>
          <h3 class="font-semibold text-gray-900">Embedding 队列</h3>
          <p class="text-xs text-gray-500">实时追踪embedding生成进度</p>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <button
          class="px-3 py-1.5 text-sm text-gray-600 hover:text-gray-900 transition-colors"
          @click="refreshAll"
        >
          <Icon icon="mdi:refresh" width="16" height="16" />
        </button>
        <button
          v-if="status.failed > 0"
          class="px-4 py-2 text-sm font-medium text-white bg-orange-600 rounded-lg hover:bg-orange-700 transition-colors disabled:opacity-50"
          :disabled="retrying"
          @click="retryFailed"
        >
          {{ retrying ? '重试中...' : `重试失败 (${status.failed})` }}
        </button>
      </div>
    </div>

    <!-- Status Cards -->
    <div class="grid grid-cols-4 gap-3">
      <div class="rounded-lg border border-gray-200 p-3 bg-yellow-50">
        <div class="text-2xl font-bold text-yellow-700">{{ status.pending }}</div>
        <div class="text-xs text-yellow-600">待处理</div>
      </div>
      <div class="rounded-lg border border-gray-200 p-3 bg-blue-50">
        <div class="text-2xl font-bold text-blue-700">{{ status.processing }}</div>
        <div class="text-xs text-blue-600">处理中</div>
      </div>
      <div class="rounded-lg border border-gray-200 p-3 bg-green-50">
        <div class="text-2xl font-bold text-green-700">{{ status.completed }}</div>
        <div class="text-xs text-green-600">已完成</div>
      </div>
      <div class="rounded-lg border border-gray-200 p-3 bg-red-50">
        <div class="text-2xl font-bold text-red-700">{{ status.failed }}</div>
        <div class="text-xs text-red-600">失败</div>
      </div>
    </div>

    <!-- Progress Bar -->
    <div v-if="status.total > 0" class="space-y-1">
      <div class="flex justify-between text-xs text-gray-500">
        <span>总体进度</span>
        <span>{{ progressPercent }}% ({{ status.completed }}/{{ status.total }})</span>
      </div>
      <div class="h-2 bg-gray-200 rounded-full overflow-hidden">
        <div
          class="h-full bg-gradient-to-r from-purple-500 to-purple-600 transition-all duration-300"
          :style="{ width: `${progressPercent}%` }"
        />
      </div>
    </div>

    <!-- Filter -->
    <div class="flex items-center gap-2">
      <span class="text-sm text-gray-500">筛选:</span>
      <button
        v-for="s in ['', 'pending', 'processing', 'completed', 'failed']"
        :key="s"
        class="px-3 py-1 text-xs rounded-full transition-colors"
        :class="statusFilter === s ? 'bg-purple-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'"
        @click="changeFilter(s)"
      >
        {{ s === '' ? '全部' : getStatusLabel(s) }}
      </button>
    </div>

    <!-- Tasks Table -->
    <div v-if="loading" class="py-8 flex justify-center">
      <Icon icon="mdi:loading" width="28" height="28" class="animate-spin text-purple-600" />
    </div>

    <div v-else-if="error" class="rounded-lg bg-red-50 border border-red-200 px-4 py-3 text-sm text-red-700">
      {{ error }}
    </div>

    <div v-else-if="tasks.length === 0" class="py-8 text-center text-gray-500">
      暂无任务
    </div>

    <div v-else class="overflow-x-auto">
      <table class="w-full text-sm">
        <thead>
          <tr class="border-b border-gray-200">
            <th class="text-left py-2 px-3 font-medium text-gray-600">标签</th>
            <th class="text-left py-2 px-3 font-medium text-gray-600">状态</th>
            <th class="text-left py-2 px-3 font-medium text-gray-600">创建时间</th>
            <th class="text-left py-2 px-3 font-medium text-gray-600">完成时间</th>
            <th class="text-left py-2 px-3 font-medium text-gray-600">错误信息</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="task in tasks" :key="task.id" class="border-b border-gray-100 hover:bg-gray-50">
            <td class="py-2 px-3">
              <span v-if="task.tag">{{ task.tag.label }}</span>
              <span v-else class="text-gray-400">Tag #{{ task.tag_id }}</span>
            </td>
            <td class="py-2 px-3">
              <span
                class="px-2 py-0.5 text-xs rounded-full"
                :class="getStatusColor(task.status)"
              >
                {{ getStatusLabel(task.status) }}
              </span>
            </td>
            <td class="py-2 px-3 text-gray-500">{{ formatDate(task.created_at) }}</td>
            <td class="py-2 px-3 text-gray-500">{{ formatDate(task.completed_at) }}</td>
            <td class="py-2 px-3 text-red-600 text-xs max-w-xs truncate">
              {{ task.error_message || '-' }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Pagination -->
    <div v-if="totalPages > 1" class="flex items-center justify-between">
      <div class="text-sm text-gray-500">
        共 {{ totalTasks }} 条任务
      </div>
      <div class="flex items-center gap-1">
        <button
          class="px-3 py-1 text-sm rounded hover:bg-gray-100 disabled:opacity-50"
          :disabled="currentPage <= 1"
          @click="changePage(currentPage - 1)"
        >
          上一页
        </button>
        <span class="px-3 py-1 text-sm">
          {{ currentPage }} / {{ totalPages }}
        </span>
        <button
          class="px-3 py-1 text-sm rounded hover:bg-gray-100 disabled:opacity-50"
          :disabled="currentPage >= totalPages"
          @click="changePage(currentPage + 1)"
        >
          下一页
        </button>
      </div>
    </div>
  </div>
</template>
