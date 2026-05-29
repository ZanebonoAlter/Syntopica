<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { useMergeReembeddingQueueApi, type MergeReembeddingQueueStatus, type MergeReembeddingQueueTask } from '~/api'

const loading = ref(false)
const error = ref<string | null>(null)
const status = ref<MergeReembeddingQueueStatus>({
  pending: 0,
  processing: 0,
  completed: 0,
  failed: 0,
  total: 0,
})
const tasks = ref<MergeReembeddingQueueTask[]>([])
const totalTasks = ref(0)
const statusFilter = ref('')
const currentPage = ref(1)
const pageSize = 20
const retrying = ref(false)

let refreshTimer: ReturnType<typeof setInterval> | null = null

const api = useMergeReembeddingQueueApi()

async function loadStatus() {
  try {
    const response = await api.getStatus()
    if (response.success && response.data) {
      status.value = response.data
    }
  } catch (err) {
    console.error('Failed to load merge re-embedding queue status:', err)
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
    console.error('Failed to retry merge re-embedding tasks:', err)
  } finally {
    retrying.value = false
  }
}

function getStatusColor(value: string) {
  switch (value) {
    case 'pending': return 'bg-yellow-100 text-yellow-800'
    case 'processing': return 'bg-blue-100 text-blue-800'
    case 'completed': return 'bg-green-100 text-green-800'
    case 'failed': return 'bg-red-100 text-red-800'
    default: return 'bg-gray-100 text-gray-800'
  }
}

function getStatusLabel(value: string) {
  switch (value) {
    case 'pending': return '待处理'
    case 'processing': return '处理中'
    case 'completed': return '已完成'
    case 'failed': return '失败'
    default: return value
  }
}

function formatDate(dateStr: string | null) {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString('zh-CN')
}

function formatTagLabel(tag: MergeReembeddingQueueTask['source_tag'] | MergeReembeddingQueueTask['target_tag'], fallbackId: number) {
  if (!tag) {
    return `Tag #${fallbackId}`
  }
  return `${tag.label} · ${tag.category}`
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
  <div class="space-y-4 rounded-2xl border border-emerald-100 bg-gradient-to-br from-emerald-50 via-white to-cyan-50 p-5">
    <div class="flex items-center justify-between gap-4">
      <div class="flex items-center gap-3">
        <div class="flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br from-emerald-500 to-cyan-700">
          <Icon icon="mdi:source-merge" width="20" height="20" class="text-white" />
        </div>
        <div>
          <h3 class="font-semibold text-gray-900">标签合并重算队列</h3>
          <p class="text-xs text-gray-500">合并完成后异步重算目标标签 embedding</p>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <button
          class="px-3 py-1.5 text-sm text-gray-600 transition-colors hover:text-gray-900"
          @click="refreshAll"
        >
          <Icon icon="mdi:refresh" width="16" height="16" />
        </button>
        <button
          v-if="status.failed > 0"
          class="rounded-lg bg-orange-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-orange-700 disabled:opacity-50"
          :disabled="retrying"
          @click="retryFailed"
        >
          {{ retrying ? '重试中...' : `重试失败 (${status.failed})` }}
        </button>
      </div>
    </div>

    <div class="grid grid-cols-4 gap-3">
      <div class="rounded-lg border border-yellow-200 bg-yellow-50 p-3">
        <div class="text-2xl font-bold text-yellow-700">{{ status.pending }}</div>
        <div class="text-xs text-yellow-600">待处理</div>
      </div>
      <div class="rounded-lg border border-blue-200 bg-blue-50 p-3">
        <div class="text-2xl font-bold text-blue-700">{{ status.processing }}</div>
        <div class="text-xs text-blue-600">处理中</div>
      </div>
      <div class="rounded-lg border border-green-200 bg-green-50 p-3">
        <div class="text-2xl font-bold text-green-700">{{ status.completed }}</div>
        <div class="text-xs text-green-600">已完成</div>
      </div>
      <div class="rounded-lg border border-red-200 bg-red-50 p-3">
        <div class="text-2xl font-bold text-red-700">{{ status.failed }}</div>
        <div class="text-xs text-red-600">失败</div>
      </div>
    </div>

    <div v-if="status.total > 0" class="space-y-1">
      <div class="flex justify-between text-xs text-gray-500">
        <span>总体进度</span>
        <span>{{ progressPercent }}% ({{ status.completed }}/{{ status.total }})</span>
      </div>
      <div class="h-2 overflow-hidden rounded-full bg-gray-200">
        <div
          class="h-full bg-gradient-to-r from-emerald-500 to-cyan-600 transition-all duration-300"
          :style="{ width: `${progressPercent}%` }"
        />
      </div>
    </div>

    <div class="flex items-center gap-2">
      <span class="text-sm text-gray-500">筛选:</span>
      <button
        v-for="value in ['', 'pending', 'processing', 'completed', 'failed']"
        :key="value"
        class="rounded-full px-3 py-1 text-xs transition-colors"
        :class="statusFilter === value ? 'bg-emerald-600 text-white' : 'bg-white text-gray-600 hover:bg-gray-100'"
        @click="changeFilter(value)"
      >
        {{ value === '' ? '全部' : getStatusLabel(value) }}
      </button>
    </div>

    <div v-if="loading" class="flex justify-center py-8">
      <Icon icon="mdi:loading" width="28" height="28" class="animate-spin text-emerald-600" />
    </div>

    <div v-else-if="error" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
      {{ error }}
    </div>

    <div v-else-if="tasks.length === 0" class="py-8 text-center text-gray-500">
      暂无重算任务
    </div>

    <div v-else class="overflow-x-auto rounded-xl border border-white/70 bg-white/80">
      <table class="w-full text-sm">
        <thead>
          <tr class="border-b border-gray-200 bg-white/80">
            <th class="px-3 py-2 text-left font-medium text-gray-600">来源标签</th>
            <th class="px-3 py-2 text-left font-medium text-gray-600">目标标签</th>
            <th class="px-3 py-2 text-left font-medium text-gray-600">状态</th>
            <th class="px-3 py-2 text-left font-medium text-gray-600">创建时间</th>
            <th class="px-3 py-2 text-left font-medium text-gray-600">完成时间</th>
            <th class="px-3 py-2 text-left font-medium text-gray-600">错误信息</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="task in tasks" :key="task.id" class="border-b border-gray-100 hover:bg-gray-50/80">
            <td class="px-3 py-2 text-gray-700">
              {{ formatTagLabel(task.source_tag, task.source_tag_id) }}
            </td>
            <td class="px-3 py-2 font-medium text-gray-900">
              {{ formatTagLabel(task.target_tag, task.target_tag_id) }}
            </td>
            <td class="px-3 py-2">
              <span class="rounded-full px-2 py-0.5 text-xs" :class="getStatusColor(task.status)">
                {{ getStatusLabel(task.status) }}
              </span>
            </td>
            <td class="px-3 py-2 text-gray-500">{{ formatDate(task.created_at) }}</td>
            <td class="px-3 py-2 text-gray-500">{{ formatDate(task.completed_at) }}</td>
            <td class="max-w-xs px-3 py-2 text-xs text-red-600">
              <span class="line-clamp-2">{{ task.error_message || '-' }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="totalPages > 1" class="flex items-center justify-between">
      <div class="text-sm text-gray-500">
        共 {{ totalTasks }} 条任务
      </div>
      <div class="flex items-center gap-1">
        <button
          class="rounded px-3 py-1 text-sm hover:bg-gray-100 disabled:opacity-50"
          :disabled="currentPage <= 1"
          @click="changePage(currentPage - 1)"
        >
          上一页
        </button>
        <span class="px-3 py-1 text-sm">
          {{ currentPage }} / {{ totalPages }}
        </span>
        <button
          class="rounded px-3 py-1 text-sm hover:bg-gray-100 disabled:opacity-50"
          :disabled="currentPage >= totalPages"
          @click="changePage(currentPage + 1)"
        >
          下一页
        </button>
      </div>
    </div>
  </div>
</template>
