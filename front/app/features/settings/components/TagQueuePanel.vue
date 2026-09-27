<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { useTagQueueApi, type TagQueueStatus, type TagQueueTask } from '~/api'
import { useEventStream } from '~/composables/useEventStream'
import { EVENT_TYPES } from '~/utils/eventTypes'

const loading = ref(false)
const error = ref<string | null>(null)
const status = ref<TagQueueStatus>({
  pending: 0,
  processing: 0,
  completed: 0,
  failed: 0,
  total: 0,
})
const tasks = ref<TagQueueTask[]>([])
const totalTasks = ref(0)
const statusFilter = ref('')
const currentPage = ref(1)
const pageSize = 20
const retrying = ref(false)
const retaggingToday = ref(false)

let refreshTimer: ReturnType<typeof setInterval> | null = null
let refreshDebounce: ReturnType<typeof setTimeout> | null = null

const api = useTagQueueApi()
const stream = useEventStream()

// 通过统一事件流订阅标签事件，触发队列刷新
const refreshOnEvent = () => {
  if (refreshDebounce) clearTimeout(refreshDebounce)
  refreshDebounce = setTimeout(() => {
    loadStatus()
    loadTasks()
  }, 500)
}

const unsubCompleted = stream.on(EVENT_TYPES.TAG_COMPLETED, refreshOnEvent)
const unsubFailed = stream.on(EVENT_TYPES.TAG_FAILED, refreshOnEvent)

async function loadStatus() {
  try {
    const response = await api.getStatus()
    if (response.success && response.data) {
      status.value = response.data
    }
  } catch (err) {
    console.error('Failed to load tag queue status:', err)
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

async function retagToday() {
  retaggingToday.value = true
  try {
    const response = await api.retagToday()
    if (response.success) {
      await Promise.all([loadStatus(), loadTasks()])
    }
  } catch (err) {
    console.error('Failed to retag today:', err)
  } finally {
    retaggingToday.value = false
  }
}

function getStatusStyle(s: string) {
  switch (s) {
    case 'pending': return { background: 'var(--color-warning-bg, rgba(196, 136, 60, 0.1))', color: 'var(--color-warning)' }
    case 'leased': return { background: 'var(--color-link-subtle)', color: 'var(--color-link)' }
    case 'completed': return { background: 'var(--color-success-bg, rgba(61, 138, 74, 0.1))', color: 'var(--color-success)' }
    case 'failed': return { background: 'var(--color-error-bg, rgba(196, 47, 60, 0.1))', color: 'var(--color-error)' }
    default: return { background: 'var(--color-bg-sunken)', color: 'var(--color-text-muted)' }
  }
}

function getStatusLabel(s: string) {
  switch (s) {
    case 'pending': return '待处理'
    case 'leased': return '打标中'
    case 'completed': return '已完成'
    case 'failed': return '失败'
    default: return s
  }
}

function formatDate(dateStr: string | null) {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString('zh-CN')
}

/** 面板计数语义（notification-center/tag-queue-progress-chip）：活跃量=pending+leased 主展示；今日完成取 completed_today（旧后端未合入时回退显示累计 completed 并在标签注明累计） */
const activeCount = computed(() => status.value.pending + status.value.processing)
const hasCompletedToday = computed(() => status.value.completed_today != null)
const completedDisplay = computed(() => hasCompletedToday.value ? status.value.completed_today : status.value.completed)

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
  refreshTimer = setInterval(loadStatus, 30000)
})

onUnmounted(() => {
  unsubCompleted()
  unsubFailed()
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
  if (refreshDebounce) {
    clearTimeout(refreshDebounce)
  }
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between gap-4">
      <AppSectionHeader title="标签打标队列" description="追踪文章自动打标进度" icon-name="mdi:tag-multiple" />
      <div class="flex items-center gap-2">
        <button
          class="px-4 py-2 text-sm font-medium text-white rounded-lg transition-colors disabled:opacity-50"
          style="background: var(--color-accent)"
          :disabled="retaggingToday"
          @click="retagToday"
        >
          {{ retaggingToday ? '提交中...' : '重打今日标签' }}
        </button>
        <button
          class="px-3 py-1.5 text-sm transition-colors"
          style="color: var(--color-text-secondary)"
          @click="refreshAll"
        >
          <Icon icon="mdi:refresh" width="16" height="16" />
        </button>
        <button
          v-if="status.failed > 0"
          class="px-4 py-2 text-sm font-medium text-white rounded-lg transition-colors disabled:opacity-50"
          style="background: var(--color-accent)"
          :disabled="retrying"
          @click="retryFailed"
        >
          {{ retrying ? '重试中...' : `重试失败 (${status.failed})` }}
        </button>
      </div>
    </div>

    <!-- Status Cards（计数语义：活跃量主展示 + 今日完成；累计 total 口径停用） -->
    <div class="grid grid-cols-3 gap-3">
      <div class="rounded-lg p-3" style="background: var(--color-bg-sunken); border: 1px solid var(--color-border-subtle)" data-testid="queue-active-count">
        <div class="text-2xl font-bold" style="color: var(--color-link)">{{ activeCount }}</div>
        <div class="text-xs" style="color: var(--color-text-muted)">活跃任务（待处理+打标中）</div>
      </div>
      <div class="rounded-lg p-3" style="background: var(--color-bg-sunken); border: 1px solid var(--color-border-subtle)" data-testid="queue-completed-today">
        <div class="text-2xl font-bold" style="color: var(--color-success)">{{ completedDisplay }}</div>
        <div class="text-xs" style="color: var(--color-text-muted)">{{ hasCompletedToday ? '今日完成' : '已完成（累计）' }}</div>
      </div>
      <div class="rounded-lg p-3" style="background: var(--color-bg-sunken); border: 1px solid var(--color-border-subtle)" data-testid="queue-failed-count">
        <div class="text-2xl font-bold" style="color: var(--color-error)">{{ status.failed }}</div>
        <div class="text-xs" style="color: var(--color-text-muted)">失败</div>
      </div>
    </div>

    <!-- 累计进度条停用（total 累计口径不再展示；进度语义由 header 芯片承接） -->

    <!-- Filter -->
    <div class="flex items-center gap-2">
      <span class="text-sm" style="color: var(--color-text-muted)">筛选:</span>
      <button
        v-for="s in ['', 'pending', 'leased', 'completed', 'failed']"
        :key="s"
        class="px-3 py-1 text-xs rounded-full transition-colors"
        :style="statusFilter === s ? 'background: var(--color-accent); color: var(--color-text-inverted)' : 'background: var(--color-bg-sunken); color: var(--color-text-secondary)'"
        @click="changeFilter(s)"
      >
        {{ s === '' ? '全部' : getStatusLabel(s) }}
      </button>
    </div>

    <!-- Tasks Table -->
    <div v-if="loading" class="py-8 flex justify-center">
      <Icon icon="mdi:loading" width="28" height="28" class="animate-spin" style="color: var(--color-accent)" />
    </div>

    <div v-else-if="error" class="rounded-lg px-4 py-3 text-sm" style="background: var(--color-error-bg, rgba(196, 47, 60, 0.1)); border: 1px solid var(--color-error-border, rgba(196, 47, 60, 0.25)); color: var(--color-error)">
      {{ error }}
    </div>

    <div v-else-if="tasks.length === 0" class="py-8 text-center" style="color: var(--color-text-muted)">
      暂无任务
    </div>

    <div v-else class="overflow-x-auto">
      <table class="w-full text-sm">
        <thead>
          <tr style="border-bottom: 1px solid var(--color-border-subtle)">
            <th class="text-left py-2 px-3 font-medium" style="color: var(--color-text-secondary)">文章标题</th>
            <th class="text-left py-2 px-3 font-medium" style="color: var(--color-text-secondary)">来源</th>
            <th class="text-left py-2 px-3 font-medium" style="color: var(--color-text-secondary)">分类</th>
            <th class="text-left py-2 px-3 font-medium" style="color: var(--color-text-secondary)">状态</th>
            <th class="text-left py-2 px-3 font-medium" style="color: var(--color-text-secondary)">重试</th>
            <th class="text-left py-2 px-3 font-medium" style="color: var(--color-text-secondary)">创建时间</th>
            <th class="text-left py-2 px-3 font-medium" style="color: var(--color-text-secondary)">错误信息</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="task in tasks" :key="task.id" class="hover:bg-[var(--color-bg-hover)]" style="border-bottom: 1px solid var(--color-border-subtle)">
            <td class="py-2 px-3 max-w-xs truncate">
              {{ task.article_title || `Article #${task.article_id}` }}
            </td>
            <td class="py-2 px-3 max-w-[120px] truncate" style="color: var(--color-text-muted)">
              {{ task.feed_name_snapshot || '-' }}
            </td>
            <td class="py-2 px-3 max-w-[100px] truncate" style="color: var(--color-text-muted)">
              {{ task.category_name_snapshot || '-' }}
            </td>
            <td class="py-2 px-3">
              <span
                class="px-2 py-0.5 text-xs rounded-full"
                :style="getStatusStyle(task.status)"
              >
                {{ getStatusLabel(task.status) }}
              </span>
            </td>
            <td class="py-2 px-3" style="color: var(--color-text-secondary)">
              {{ task.attempt_count }}/{{ task.max_attempts }}
            </td>
            <td class="py-2 px-3 whitespace-nowrap" style="color: var(--color-text-secondary)">
              {{ formatDate(task.created_at) }}
            </td>
            <td class="py-2 px-3 text-xs max-w-[200px] truncate" style="color: var(--color-error)" :title="task.last_error">
              {{ task.last_error || '-' }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Pagination -->
    <div v-if="totalPages > 1" class="flex items-center justify-between">
      <div class="text-sm" style="color: var(--color-text-muted)">
        共 {{ totalTasks }} 条任务
      </div>
      <div class="flex items-center gap-1">
        <button
          class="px-3 py-1 text-sm rounded hover:bg-[var(--color-bg-hover)] disabled:opacity-50"
          :disabled="currentPage <= 1"
          @click="changePage(currentPage - 1)"
        >
          上一页
        </button>
        <span class="px-3 py-1 text-sm">
          {{ currentPage }} / {{ totalPages }}
        </span>
        <button
          class="px-3 py-1 text-sm rounded hover:bg-[var(--color-bg-hover)] disabled:opacity-50"
          :disabled="currentPage >= totalPages"
          @click="changePage(currentPage + 1)"
        >
          下一页
        </button>
      </div>
    </div>
  </div>
</template>
