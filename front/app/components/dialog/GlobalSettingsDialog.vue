<script setup lang="ts">
import { Icon } from "@iconify/vue";
import AIRouterSettingsPanel from '~/features/ai/components/AIRouterSettingsPanel.vue'
import EmbeddingConfigPanel from '~/features/ai/components/EmbeddingConfigPanel.vue'
import EmbeddingQueuePanel from '~/features/ai/components/EmbeddingQueuePanel.vue'
import TagQueuePanel from '~/features/topic-graph/components/TagQueuePanel.vue'
import type { RssFeed } from '~/types'
import type { ReadingStats, UserPreference } from '~/types/reading_behavior'
import type { SchedulerStatus, SchedulerTriggerResult } from '~/types/scheduler'
import { useFirecrawlApi, useSchedulerApi } from '~/api'
import { useGlobalAutoRefresh } from '~/features/feeds/composables/useAutoRefresh'
import {
  getCurrentContentCompletionArticle,
  getSchedulerColor,
  getSchedulerDisplayName,
  getSchedulerIcon,
  getSchedulerStatusLabel,
  isContentCompletionScheduler,
  isHotScheduler,
  shouldShowContentCompletionPanel,
} from '~/utils/schedulerMeta'

interface Props {
  show: boolean
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:show': [value: boolean]
}>()

const apiStore = useApiStore()
const feedsStore = useFeedsStore()
const preferencesStore = usePreferencesStore()

const activeTab = ref<'feeds' | 'categories' | 'general' | 'queues' | 'preferences' | 'firecrawl' | 'schedulers'>('feeds')
const collapsedCategories = ref<Record<string, boolean>>({})
const loading = ref(false)
const error = ref<string | null>(null)
const success = ref<string | null>(null)

const schedulerStatuses = ref<SchedulerStatus[]>([])
const schedulerTriggerFeedback = ref<Record<string, SchedulerTriggerResult | undefined>>({})
const lastSchedulerTriggerAt = ref<number | null>(null)
const schedulerLoading = ref(false)
let schedulerPollTimer: ReturnType<typeof setTimeout> | null = null

// AI Summary Settings
const aiSummaryEnabled = ref(false)
const aiBaseURL = ref('')
const aiAPIKey = ref('')
const aiModel = ref('')
const showApiKey = ref(false)
const autoSummaryEnabled = ref(false)

// AI Podcast Settings
const aiPodcastEnabled = ref(false)

// Firecrawl Settings
const firecrawlEnabled = ref(false)
const firecrawlApiUrl = ref('')
const firecrawlApiKey = ref('')
const firecrawlMode = ref('scrape')
const firecrawlTimeout = ref(60)
const firecrawlMaxContentLength = ref(50000)
const firecrawlApiKeyVisible = ref(false)
const firecrawlLoading = ref(false)

// Reading preferences
const preferenceType = ref<'feed' | 'category'>('feed')
const readingStats = ref<ReadingStats | null>(null)
const userPreferences = ref<UserPreference[]>([])
const preferencesLoading = ref(false)
const preferencesUpdating = ref(false)

// Get feeds grouped by category
const feedsByCategory = computed(() => {
  const grouped: Record<string, RssFeed[]> = {}
  apiStore.feeds.forEach((feed: RssFeed) => {
    const categoryName = feedsStore.categories.find(c => c.id === feed.category)?.name || '未分类'
    if (!grouped[categoryName]) {
      grouped[categoryName] = []
    }
    grouped[categoryName].push(feed)
  })
  return grouped
})

// Refresh interval options
const refreshOptions = [
  { label: '手动刷新', value: 0 },
  { label: '每 15 分钟', value: 15 },
  { label: '每 30 分钟', value: 30 },
  { label: '每小时', value: 60 },
  { label: '每 2 小时', value: 120 },
  { label: '每 6 小时', value: 360 },
  { label: '每天', value: 1440 },
]

// Max articles options
const maxArticlesOptions = [
  { label: '50 篇', value: 50 },
  { label: '100 篇', value: 100 },
  { label: '200 篇', value: 200 },
  { label: '500 篇', value: 500 },
  { label: '1000 篇', value: 1000 },
  { label: '无限制', value: 0 },
]

async function updateFeedSetting(
  feedId: string,
  setting: 'refresh_interval' | 'max_articles' | 'ai_summary_enabled' | 'tagging_enabled' | 'firecrawl_enabled' | 'completion_on_refresh',
  value: number | boolean
) {
  loading.value = true
  error.value = null
  success.value = null

  const response = await apiStore.updateFeed(feedId, {
    [setting]: value,
  })

  loading.value = false

  if (response.success) {
    await apiStore.fetchFeeds({ per_page: 10000 })
    success.value = '设置已更新'

    // Update auto-refresh if refresh interval changed
    if (setting === 'refresh_interval') {
      const autoRefresh = useGlobalAutoRefresh()
      autoRefresh.updateFeedRefresh(feedId, value as number)
    }

    setTimeout(() => {
      success.value = null
    }, 2000)
  } else {
    error.value = response.error || '更新失败'
  }
}

function formatInterval(minutes: number): string {
  const option = refreshOptions.find(opt => opt.value === minutes)
  return option?.label || `${minutes} 分钟`
}

function formatMaxArticles(count: number): string {
  if (count <= 0 || count >= 9999) return '无限制'
  const option = maxArticlesOptions.find(opt => opt.value === count)
  return option?.label || `${count} 篇`
}

function getIntervalColor(minutes: number): string {
  if (minutes === 0) return 'text-gray-500'
  if (minutes <= 30) return 'text-green-600'
  if (minutes <= 120) return 'text-blue-600'
  return 'text-ink-600'
}

async function refreshFeed(feedId: string) {
  loading.value = true
  await apiStore.refreshFeed(feedId)
  await apiStore.fetchFeeds({ per_page: 10000 })
  loading.value = false
  success.value = '订阅源已刷新'
  setTimeout(() => {
    success.value = null
  }, 2000)
}

function close() {
  emit('update:show', false)
}

function loadAISettings() {
  aiSummaryEnabled.value = false
  aiBaseURL.value = ''
  aiAPIKey.value = ''
  aiModel.value = ''
  aiPodcastEnabled.value = false
  autoSummaryEnabled.value = false
}

// Load firecrawl settings
async function loadFirecrawlSettings() {
  firecrawlLoading.value = true
  try {
    const { getStatus } = useFirecrawlApi()
    const response = await getStatus()
    if (response.success && response.data) {
      firecrawlEnabled.value = response.data.enabled
      firecrawlApiUrl.value = response.data.api_url
      firecrawlMode.value = response.data.mode || 'scrape'
      firecrawlTimeout.value = response.data.timeout || 60
      firecrawlMaxContentLength.value = response.data.max_content_length || 50000
    }
  } catch (e) {
    console.error('Failed to load firecrawl settings:', e)
  } finally {
    firecrawlLoading.value = false
  }
}

// Save firecrawl settings
async function saveFirecrawlSettings() {
  firecrawlLoading.value = true
  error.value = null
  try {
    const { saveSettings } = useFirecrawlApi()
    const response = await saveSettings({
      enabled: firecrawlEnabled.value,
      api_url: firecrawlApiUrl.value,
      api_key: firecrawlApiKey.value,
      mode: firecrawlMode.value,
      timeout: firecrawlTimeout.value,
      max_content_length: firecrawlMaxContentLength.value,
    })

    if (!response.success) {
      throw new Error(response.error || '保存失败')
    }
    
    success.value = 'Firecrawl 设置已保存'
    setTimeout(() => {
      success.value = null
    }, 2000)
  } catch (e) {
    error.value = '保存失败'
  } finally {
    firecrawlLoading.value = false
  }
}

// Load settings from localStorage
onMounted(() => {
  loadAISettings()
})

// Watch active tab changes
watch(activeTab, async (newTab) => {
  if (newTab === 'preferences') {
    await loadPreferencesData()
  } else if (newTab === 'firecrawl') {
    await loadFirecrawlSettings()
  } else if (newTab === 'schedulers') {
    await loadSchedulersStatus()
  } else {
    stopSchedulerPolling()
  }
})

watch(() => props.show, (visible) => {
  if (!visible) {
    stopSchedulerPolling()
  }
})

onBeforeUnmount(() => {
  stopSchedulerPolling()
})

// Load reading preferences data
async function loadPreferencesData() {
  preferencesLoading.value = true
  try {
    await Promise.all([
      preferencesStore.fetchStats(),
      preferencesStore.fetchPreferences(preferenceType.value)
    ])
    readingStats.value = preferencesStore.stats
    userPreferences.value = preferencesStore.preferences.filter(
      preference => Boolean(preference.feed_title || preference.category_name)
    )
  } catch (e) {
    console.error('Failed to load preferences:', e)
  } finally {
    preferencesLoading.value = false
  }
}

// Trigger preference update
async function triggerPreferenceUpdate() {
  preferencesUpdating.value = true
  try {
    await preferencesStore.triggerUpdate()
    await loadPreferencesData()
  } finally {
    preferencesUpdating.value = false
  }
}

// Get score color
function getScoreColor(score: number): string {
  if (score >= 0.7) return 'bg-green-500'
  if (score >= 0.4) return 'bg-yellow-500'
  return 'bg-gray-400'
}

// Test AI connection
async function testAIConnection() {
  error.value = '连接测试已迁移到 AI Router 面板，请在那里直接测试主模型。'
}

async function loadSchedulersStatus() {
  schedulerLoading.value = true
  try {
    const { getSchedulersStatus } = useSchedulerApi()
    const response = await getSchedulersStatus()
    if (response.success && response.data) {
      schedulerStatuses.value = response.data
    }
  } catch (e) {
    console.error('Failed to load scheduler status:', e)
  } finally {
    schedulerLoading.value = false
    scheduleSchedulerPolling()
  }
}

function stopSchedulerPolling() {
  if (schedulerPollTimer) {
    clearTimeout(schedulerPollTimer)
    schedulerPollTimer = null
  }
}

function scheduleSchedulerPolling() {
  stopSchedulerPolling()
  if (!props.show || activeTab.value !== 'schedulers') return

  const hasHotScheduler = schedulerStatuses.value.some(item => {
    if (isHotScheduler(item.name)) {
      return item.is_executing === true
    }
    return false
  })
  const hasRecentFeedback = lastSchedulerTriggerAt.value !== null && Date.now() - lastSchedulerTriggerAt.value < 20000
  const interval = hasHotScheduler ? 8000 : hasRecentFeedback ? 15000 : 30000
  schedulerPollTimer = setTimeout(() => {
    loadSchedulersStatus()
  }, interval)
}

async function triggerScheduler(name: string) {
  loading.value = true
  error.value = null
  try {
    const { triggerScheduler: trigger } = useSchedulerApi()
    const response = await trigger(name)
    if (response.success) {
      schedulerTriggerFeedback.value[name] = response.data
      lastSchedulerTriggerAt.value = Date.now()
      success.value = response.data?.message || response.message || '任务请求已处理'
      setTimeout(() => {
        success.value = null
      }, 2000)
      await loadSchedulersStatus()
    } else {
      schedulerTriggerFeedback.value[name] = response.data ?? {
        name,
        accepted: false,
        started: false,
        reason: 'request_rejected',
        message: response.error || response.message || '触发失败',
      }
      lastSchedulerTriggerAt.value = Date.now()
      error.value = response.error || '触发失败'
    }
  } catch (e) {
    schedulerTriggerFeedback.value[name] = {
      name,
      accepted: false,
      started: false,
      reason: 'request_failed',
      message: '触发失败',
    }
    lastSchedulerTriggerAt.value = Date.now()
    error.value = '触发失败'
  } finally {
    loading.value = false
  }
}

function isSchedulerBusy(scheduler: SchedulerStatus): boolean {
  return scheduler.is_executing === true || scheduler.database_state?.status === 'running'
}

function getSchedulerFeedback(name: string): SchedulerTriggerResult | undefined {
  return schedulerTriggerFeedback.value[name]
}

function getAutoRefreshRunSummary(scheduler: SchedulerStatus) {
  return scheduler.last_run_summary?.scanned_feeds !== undefined ? scheduler.last_run_summary : null
}

function getAutoRefreshReasonLabel(reason: string | undefined): string {
  switch (reason) {
    case 'feeds_triggered':
      return '有 feed 到点，已经放出去刷了。'
    case 'no_feeds_due':
      return '这轮扫描没发现到点的 feed。'
    case 'all_due_feeds_already_refreshing':
      return '到点的 feed 已经在刷新中。'
    case 'no_feeds_enabled':
      return '现在没有开启自动刷新的 feed。'
    case 'query_failed':
      return '查 feed 时就出错了。'
    default:
      return '这轮扫描已经结束。'
  }
}

function getStatusColor(status: string | undefined): string {
  if (!status) return 'bg-gray-400'
  if (status === 'running') return 'bg-green-500'
  if (status === 'error') return 'bg-red-500'
  if (status === 'stopped') return 'bg-stone-400'
  if (status === 'triggered') return 'bg-amber-500'
  return 'bg-blue-500'
}

function formatSchedulerInterval(seconds: number | undefined): string {
  if (!seconds) return '-'
  if (seconds < 60) return `${seconds} 秒`
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`
  if (seconds % 60 === 0) return `${seconds / 60} 分钟`
  return `${seconds} 秒`
}

function formatArticleLabel(article: SchedulerStatus['current_article'] | SchedulerStatus['last_processed']): string {
  if (!article) return '-'
  return article.title
}

function formatErrorCategory(category: string | undefined): string {
  switch (category) {
    case 'network':
      return '网络波动'
    case 'config':
      return '配置问题'
    case 'content':
      return '正文异常'
    case 'retries':
      return '重试耗尽'
    default:
      return '其他错误'
  }
}

function getOverviewValue(
  scheduler: SchedulerStatus,
  key: 'pending_count' | 'processing_count' | 'completed_count' | 'failed_count' | 'blocked_count' | 'total_count'
): number {
  return scheduler.overview?.[key] ?? 0
}

function getBlockedReasonValue(
  scheduler: SchedulerStatus,
  key: keyof NonNullable<NonNullable<SchedulerStatus['overview']>['blocked_reasons']>
): number {
  return scheduler.overview?.blocked_reasons?.[key] ?? 0
}

function getLastRunValue(
  scheduler: SchedulerStatus,
  key: 'completed_count' | 'failed_count' | 'blocked_count' | 'stale_processing_count'
): number {
  return scheduler.last_run_summary?.[key] ?? 0
}

function formatDuration(seconds: number | null | undefined): string {
  if (!seconds) return '-'
  if (seconds < 60) return `${seconds.toFixed(1)}s`
  return `${(seconds / 60).toFixed(1)}min`
}

function formatNextRun(nextRun: string | null | undefined): string {
  if (!nextRun) return '-'
  try {
    const date = new Date(nextRun)
    const now = new Date()
    const diff = date.getTime() - now.getTime()
    if (diff < 0) return '即将执行'
    if (diff < 60000) return `${Math.floor(diff / 1000)}秒后`
    if (diff < 3600000) return `${Math.floor(diff / 60000)}分钟后`
    return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  } catch {
    return '-'
  }
}
</script>

<template>
  <div
    v-if="props.show"
    class="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
    @click.self="close"
  >
    <div
      class="bg-white rounded-xl shadow-xl w-full max-w-4xl mx-4 overflow-hidden max-h-[90vh] flex flex-col"
      style="max-height: calc(90vh - 2rem);"
      @click.stop
    >
      <!-- Header -->
      <div class="px-6 py-4 border-b border-gray-200 flex items-center justify-between flex-shrink-0">
        <h2 class="text-xl font-bold text-gray-900">RSS 阅读器设置</h2>
        <button
          class="p-2 hover:bg-gray-100 rounded-lg transition-colors"
          @click="close"
        >
          <Icon icon="mdi:close" width="20" height="20" />
        </button>
      </div>

      <!-- Tabs -->
      <div class="flex border-b border-gray-200 flex-shrink-0">
        <button
          class="px-6 py-3 text-sm font-medium transition-colors"
          :class="activeTab === 'feeds' ? 'text-blue-600 border-b-2 border-blue-600' : 'text-gray-500 hover:text-gray-700'"
          @click="activeTab = 'feeds'"
        >
          订阅源配置
        </button>
        <button
          class="px-6 py-3 text-sm font-medium transition-colors"
          :class="activeTab === 'general' ? 'text-blue-600 border-b-2 border-blue-600' : 'text-gray-500 hover:text-gray-700'"
          @click="activeTab = 'general'"
        >
          通用设置
        </button>
        <button
          class="px-6 py-3 text-sm font-medium transition-colors"
          :class="activeTab === 'queues' ? 'text-blue-600 border-b-2 border-blue-600' : 'text-gray-500 hover:text-gray-700'"
          @click="activeTab = 'queues'"
        >
          标签 & 队列
        </button>
        <button
          class="px-6 py-3 text-sm font-medium transition-colors"
          :class="activeTab === 'preferences' ? 'text-blue-600 border-b-2 border-blue-600' : 'text-gray-500 hover:text-gray-700'"
          @click="activeTab = 'preferences'"
        >
          阅读偏好
        </button>
        <button
          class="px-6 py-3 text-sm font-medium transition-colors"
          :class="activeTab === 'firecrawl' ? 'text-blue-600 border-b-2 border-blue-600' : 'text-gray-500 hover:text-gray-700'"
          @click="activeTab = 'firecrawl'"
        >
          Firecrawl
        </button>
        <button
          class="px-6 py-3 text-sm font-medium transition-colors"
          :class="activeTab === 'schedulers' ? 'text-blue-600 border-b-2 border-blue-600' : 'text-gray-500 hover:text-gray-700'"
          @click="activeTab = 'schedulers'"
        >
          定时任务
        </button>
      </div>

      <!-- Success/Error messages -->
      <div
        v-if="success"
        class="mx-6 mt-4 p-3 bg-green-50 border border-green-200 rounded-lg text-sm text-green-600 flex-shrink-0"
      >
        {{ success }}
      </div>
      <div
        v-if="error"
        class="mx-6 mt-4 p-3 bg-red-50 border border-red-200 rounded-lg text-sm text-red-600 flex-shrink-0"
      >
        {{ error }}
      </div>

      <!-- Content -->
      <div class="flex-1 overflow-y-auto p-6 min-h-0">
        <!-- Feeds Configuration Tab -->
        <div v-if="activeTab === 'feeds'" class="space-y-6">
          <div v-if="Object.keys(feedsByCategory).length === 0" class="text-center text-gray-500 py-8">
            <Icon icon="mdi:rss-off" width="48" height="48" class="mx-auto mb-2 opacity-50" />
            <p>还没有订阅源</p>
          </div>

          <div
            v-for="(feeds, categoryName) in feedsByCategory"
            :key="categoryName"
            class="space-y-3"
          >
            <h3
              class="text-sm font-semibold text-gray-700 flex items-center gap-2 cursor-pointer hover:text-gray-900 select-none"
              @click="collapsedCategories[categoryName] = !collapsedCategories[categoryName]"
            >
              <Icon :icon="collapsedCategories[categoryName] ? 'mdi:chevron-right' : 'mdi:chevron-down'" width="16" height="16" />
              <Icon icon="mdi:folder" width="16" height="16" />
              {{ categoryName }}
              <span class="text-xs font-normal text-gray-400">({{ feeds.length }})</span>
            </h3>

            <div v-show="!collapsedCategories[categoryName]" class="space-y-2">
              <div
                v-for="feed in feeds"
                :key="feed.id"
                class="border border-gray-200 rounded-lg p-4 hover:border-gray-300 transition-colors"
              >
                <div class="flex items-start gap-3">
                  <div
                    class="w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0"
                    :style="{ backgroundColor: feed.color + '15' }"
                  >
                    <FeedIcon
                      :icon="feed.icon"
                      :feed-id="feed.id"
                      :color="feed.color"
                      :size="20"
                    />
                  </div>

                  <div class="flex-1 min-w-0">
                    <div class="flex items-start justify-between gap-2 mb-3">
                      <div>
                        <h4 class="font-medium text-gray-900 truncate">{{ feed.title }}</h4>
                        <p class="text-xs text-gray-500 truncate">{{ feed.url }}</p>
                      </div>
                      <button
                        class="p-1.5 hover:bg-gray-100 rounded-lg transition-colors"
                        :title="'立即刷新'"
                        :disabled="loading"
                        @click="refreshFeed(feed.id)"
                      >
                        <Icon
                          :icon="loading ? 'mdi:loading' : 'mdi:refresh'"
                          :class="{ 'animate-spin': loading }"
                          width="16"
                          height="16"
                          class="text-gray-500"
                        />
                      </button>
                    </div>

                    <div class="grid grid-cols-2 gap-3">
                      <!-- Refresh Interval -->
                      <div>
                        <label class="block text-xs font-medium text-gray-600 mb-1">
                          自动刷新
                        </label>
                        <select
                          :value="feed.refreshInterval"
                          class="w-full text-xs px-2 py-1.5 border border-gray-200 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                          @change="updateFeedSetting(feed.id, 'refresh_interval', Number(($event.target as HTMLSelectElement).value))"
                        >
                          <option
                            v-for="option in refreshOptions"
                            :key="option.value"
                            :value="option.value"
                          >
                            {{ option.label }}
                          </option>
                        </select>
                      </div>

                      <!-- Max Articles -->
                      <div>
                        <label class="block text-xs font-medium text-gray-600 mb-1">
                          最大文章数
                        </label>
                        <select
                          :value="feed.maxArticles"
                          class="w-full text-xs px-2 py-1.5 border border-gray-200 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                          @change="updateFeedSetting(feed.id, 'max_articles', Number(($event.target as HTMLSelectElement).value))"
                        >
                          <option
                            v-for="option in maxArticlesOptions"
                            :key="option.value"
                            :value="option.value"
                          >
                            {{ option.label }}
                          </option>
                        </select>
                      </div>
                    </div>

                    <div class="space-y-2 mt-3 pt-3 border-t border-gray-100">
                      <div class="flex items-center justify-between">
                        <div class="flex items-center gap-2">
                          <Icon icon="mdi:web" width="14" height="14" class="text-blue-500" />
                          <span class="text-xs font-medium text-gray-700">Firecrawl</span>
                        </div>
                        <button
                          class="relative inline-flex h-5 w-9 items-center rounded-full transition-colors"
                          :class="feed.firecrawlEnabled ? 'bg-blue-500' : 'bg-gray-300'"
                          @click="updateFeedSetting(feed.id, 'firecrawl_enabled', !feed.firecrawlEnabled)"
                        >
                          <span
                            class="inline-block h-3 w-3 transform rounded-full bg-white transition-transform"
                            :class="feed.firecrawlEnabled ? 'translate-x-5' : 'translate-x-1'"
                          />
                        </button>
                      </div>

                      <div class="flex items-center justify-between">
                        <div class="flex items-center gap-2">
                          <Icon icon="mdi:tag-multiple" width="14" height="14" class="text-amber-500" />
                          <span class="text-xs font-medium text-gray-700">打标签</span>
                        </div>
                        <button
                          class="relative inline-flex h-5 w-9 items-center rounded-full transition-colors"
                          :class="feed.taggingEnabled !== false ? 'bg-amber-500' : 'bg-gray-300'"
                          @click="updateFeedSetting(feed.id, 'tagging_enabled', !(feed.taggingEnabled !== false))"
                        >
                          <span
                            class="inline-block h-3 w-3 transform rounded-full bg-white transition-transform"
                            :class="feed.taggingEnabled !== false ? 'translate-x-5' : 'translate-x-1'"
                          />
                        </button>
                      </div>

                      <div class="flex items-center justify-between">
                        <div class="flex items-center gap-2">
                          <Icon icon="mdi:text-box-plus" width="14" height="14" class="text-green-500" />
                          <span class="text-xs font-medium text-gray-700">内容补全</span>
                        </div>
                        <button
                          class="relative inline-flex h-5 w-9 items-center rounded-full transition-colors"
                          :class="feed.completionOnRefresh !== false ? 'bg-green-500' : 'bg-gray-300'"
                          @click="updateFeedSetting(feed.id, 'completion_on_refresh', !(feed.completionOnRefresh !== false))"
                        >
                          <span
                            class="inline-block h-3 w-3 transform rounded-full bg-white transition-transform"
                            :class="feed.completionOnRefresh !== false ? 'translate-x-5' : 'translate-x-1'"
                          />
                        </button>
                      </div>
                    </div>

                    <!-- Current Settings Summary -->
                    <div class="flex items-center gap-3 mt-2 text-xs">
                      <span :class="getIntervalColor(feed.refreshInterval || 0)">
                        <Icon icon="mdi:clock" width="12" height="12" class="inline-block mr-1" />
                        {{ formatInterval(feed.refreshInterval || 0) }}
                      </span>
                      <span class="text-gray-500">
                        <Icon icon="mdi:file-document-multiple" width="12" height="12" class="inline-block mr-1" />
                        {{ formatMaxArticles(feed.maxArticles ?? 100) }}
                      </span>
                      <span class="text-gray-500">
                        <Icon icon="mdi:article" width="12" height="12" class="inline-block mr-1" />
                        {{ feed.articleCount }} 篇
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Reading Preferences Tab -->
        <div v-if="activeTab === 'preferences'" class="space-y-6">
          <!-- Loading State -->
          <div v-if="preferencesLoading" class="flex items-center justify-center py-12">
            <div class="text-center">
              <Icon icon="mdi:loading" width="48" height="48" class="animate-spin text-blue-500 mx-auto mb-3" />
              <p class="text-sm text-gray-500">加载偏好数据...</p>
            </div>
          </div>

          <!-- Preferences Content -->
          <div v-else class="space-y-6">
            <!-- Quick Stats -->
            <div class="grid grid-cols-4 gap-4">
              <div class="bg-gradient-to-br from-blue-50 to-blue-100 rounded-xl p-4 border border-blue-200">
                <div class="flex items-center gap-2 mb-2">
                  <Icon icon="mdi:file-document" width="18" height="18" class="text-blue-600" />
                  <span class="text-xs font-medium text-blue-900">总文章数</span>
                </div>
                <div class="text-2xl font-bold text-blue-700">
                  {{ readingStats?.total_articles || 0 }}
                </div>
              </div>

              <div class="bg-gradient-to-br from-green-50 to-green-100 rounded-xl p-4 border border-green-200">
                <div class="flex items-center gap-2 mb-2">
                  <Icon icon="mdi:clock" width="18" height="18" class="text-green-600" />
                  <span class="text-xs font-medium text-green-900">总阅读时长</span>
                </div>
                <div class="text-2xl font-bold text-green-700">
                  {{ readingStats?.total_reading_time || 0 }}s
                </div>
              </div>

              <div class="bg-gradient-to-br from-ink-50 to-paper-cream rounded-xl p-4 border border-ink-200">
                <div class="flex items-center gap-2 mb-2">
                  <Icon icon="mdi:timer" width="18" height="18" class="text-ink-600" />
                  <span class="text-xs font-medium text-ink-900">平均时长</span>
                </div>
                <div class="text-2xl font-bold text-ink-700">
                  {{ Math.round(readingStats?.avg_reading_time || 0) }}s
                </div>
              </div>

              <div class="bg-gradient-to-br from-orange-50 to-orange-100 rounded-xl p-4 border border-orange-200">
                <div class="flex items-center gap-2 mb-2">
                  <Icon icon="mdi:arrow-down-bold" width="18" height="18" class="text-orange-600" />
                  <span class="text-xs font-medium text-orange-900">平均深度</span>
                </div>
                <div class="text-2xl font-bold text-orange-700">
                  {{ Math.round(readingStats?.avg_scroll_depth || 0) }}%
                </div>
              </div>
            </div>

            <!-- Controls -->
            <div class="flex items-center justify-between bg-gray-50 rounded-lg p-4">
              <div class="flex items-center gap-3">
                <div class="text-sm">
                  <div class="font-medium text-gray-900">查看偏好</div>
                  <div class="text-xs text-gray-500">按订阅源或分类查看</div>
                </div>
                <div class="flex gap-2">
                  <button
                    class="px-3 py-1.5 text-xs font-medium rounded-lg transition-colors"
                    :class="preferenceType === 'feed' ? 'bg-blue-600 text-white' : 'bg-white text-gray-700 border border-gray-200'"
                    @click="preferenceType = 'feed'; loadPreferencesData()"
                  >
                    订阅源
                  </button>
                  <button
                    class="px-3 py-1.5 text-xs font-medium rounded-lg transition-colors"
                    :class="preferenceType === 'category' ? 'bg-blue-600 text-white' : 'bg-white text-gray-700 border border-gray-200'"
                    @click="preferenceType = 'category'; loadPreferencesData()"
                  >
                    分类
                  </button>
                </div>
              </div>
              <button
                class="px-4 py-2 text-sm font-medium text-white bg-orange-600 rounded-lg hover:bg-orange-700 transition-colors flex items-center gap-2"
                :disabled="preferencesUpdating"
                @click="triggerPreferenceUpdate"
              >
                <Icon
                  :icon="preferencesUpdating ? 'mdi:loading' : 'mdi:refresh'"
                  :class="{ 'animate-spin': preferencesUpdating }"
                  width="16"
                  height="16"
                />
                更新偏好
              </button>
            </div>

            <!-- Preferences List -->
            <div class="bg-white rounded-xl border border-gray-200 overflow-hidden">
              <div v-if="userPreferences.length === 0" class="text-center py-12">
                <Icon icon="mdi:chart-line" width="48" height="48" class="text-gray-300 mx-auto mb-3" />
                <p class="text-sm text-gray-500">暂无偏好数据</p>
                <p class="text-xs text-gray-400 mt-1">阅读文章后将自动生成偏好分析</p>
              </div>

              <div v-else class="divide-y divide-gray-100">
                <div
                  v-for="pref in userPreferences"
                  :key="pref.id"
                  class="p-4 hover:bg-gray-50 transition-colors"
                >
                  <div class="flex items-center justify-between">
                    <div class="flex-1">
                      <div class="flex items-center gap-2 mb-1">
                        <h4 class="text-sm font-medium text-gray-900">
                          {{ pref.feed_title || pref.category_name }}
                        </h4>
                        <span class="px-2 py-0.5 text-xs font-medium rounded-full text-white"
                              :class="getScoreColor(pref.preference_score)">
                          {{ (pref.preference_score * 100).toFixed(0) }}%
                        </span>
                      </div>
                      <div class="flex items-center gap-4 text-xs text-gray-500">
                        <span class="flex items-center gap-1">
                          <Icon icon="mdi:cursor-default-click" width="12" height="12" />
                          {{ pref.interaction_count }} 次互动
                        </span>
                        <span class="flex items-center gap-1">
                          <Icon icon="mdi:clock" width="12" height="12" />
                          平均 {{ pref.avg_reading_time }} 秒
                        </span>
                        <span class="flex items-center gap-1">
                          <Icon icon="mdi:arrow-down-bold" width="12" height="12" />
                          {{ Math.round(pref.scroll_depth_avg) }}% 深度
                        </span>
                      </div>
                    </div>

                    <!-- Preference Score Bar -->
                    <div class="ml-4 w-24">
                      <div class="h-2 bg-gray-200 rounded-full overflow-hidden">
                        <div
                          class="h-full rounded-full transition-all"
                          :class="getScoreColor(pref.preference_score)"
                          :style="{ width: (pref.preference_score * 100) + '%' }"
                        />
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Info Box -->
            <div class="bg-blue-50 rounded-lg p-4 flex items-start gap-3">
              <Icon icon="mdi:information" width="20" height="20" class="text-blue-600 flex-shrink-0 mt-0.5" />
              <div class="text-sm text-blue-900">
                <div class="font-medium mb-1">关于阅读偏好</div>
                <p class="text-blue-700 text-xs">
                  系统会根据您的阅读行为（滚动深度、阅读时长、互动频率）自动计算偏好分数。
                  分数范围 0-100%，越高表示您对该订阅源或分类越感兴趣。
                  偏好数据每 30 分钟自动更新，也可手动触发更新。
                </p>
              </div>
            </div>
          </div>
        </div>

        <!-- General Settings Tab -->
        <div v-if="activeTab === 'general'" class="space-y-6">
          <AIRouterSettingsPanel />
          <EmbeddingConfigPanel />

          <!-- AI Podcast Settings -->
          <!-- <div class="bg-gradient-to-br from-green-50 to-teal-50 rounded-xl p-6 border border-green-100">
            <div class="flex items-start justify-between mb-4">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-lg bg-gradient-to-br from-green-500 to-teal-600 flex items-center justify-center">
                  <Icon icon="mdi:podium" width="20" height="20" class="text-white" />
                </div>
                <div>
                  <h3 class="font-semibold text-gray-900">AI 播客</h3>
                  <p class="text-xs text-gray-500">将文章转换为播客音频（即将推出）</p>
                </div>
              </div>
              <button
                class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors cursor-not-allowed opacity-50"
                :class="aiPodcastEnabled ? 'bg-green-600' : 'bg-gray-300'"
                @click="aiPodcastEnabled = !aiPodcastEnabled"
              >
                <span
                  class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform"
                  :class="aiPodcastEnabled ? 'translate-x-6' : 'translate-x-1'"
                />
              </button>
            </div>

            <div class="flex items-center gap-2 text-sm text-gray-500 bg-white/50 rounded-lg p-3">
              <Icon icon="mdi:information" width="16" height="16" class="text-green-600" />
              <span>AI 播客功能正在开发中，敬请期待...</span>
            </div>
          </div> -->
        </div>

        <div v-if="activeTab === 'queues'" class="space-y-6">
          <EmbeddingQueuePanel />
          <TagQueuePanel />
        </div>

        <!-- Firecrawl Settings Tab -->
        <div v-if="activeTab === 'firecrawl'" class="space-y-6">
          <!-- Loading State -->
          <div v-if="firecrawlLoading" class="flex items-center justify-center py-12">
            <Icon icon="mdi:loading" width="48" height="48" class="animate-spin text-blue-500" />
          </div>

          <!-- Firecrawl Configuration -->
          <div v-else class="bg-gradient-to-br from-purple-50 to-indigo-50 rounded-xl p-6 border border-purple-100">
            <div class="flex items-start justify-between mb-4">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-lg bg-gradient-to-br from-purple-500 to-indigo-600 flex items-center justify-center">
                  <Icon icon="mdi:spider-web" width="20" height="20" class="text-white" />
                </div>
                <div>
                  <h3 class="font-semibold text-gray-900">Firecrawl 全文抓取</h3>
                  <p class="text-xs text-gray-500">抓取文章完整内容，支持复杂网页</p>
                </div>
              </div>
              <button
                class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors"
                :class="firecrawlEnabled ? 'bg-purple-600' : 'bg-gray-300'"
                @click="firecrawlEnabled = !firecrawlEnabled"
              >
                <span
                  class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform"
                  :class="firecrawlEnabled ? 'translate-x-6' : 'translate-x-1'"
                />
              </button>
            </div>

            <div v-if="firecrawlEnabled" class="space-y-4 mt-4">
              <!-- API URL -->
              <div>
                <label class="block text-sm font-medium text-gray-700 mb-1.5">
                  API URL
                </label>
                <input
                  v-model="firecrawlApiUrl"
                  type="text"
                  placeholder="https://api.firecrawl.dev/v1"
                  class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:ring-2 focus:ring-purple-500 focus:border-transparent"
                />
              </div>

              <!-- API Key -->
              <div>
                <label class="block text-sm font-medium text-gray-700 mb-1.5">
                  API Key
                </label>
                <div class="relative">
                  <input
                    v-model="firecrawlApiKey"
                    :type="firecrawlApiKeyVisible ? 'text' : 'password'"
                    placeholder="fc-..."
                    class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:ring-2 focus:ring-purple-500 focus:border-transparent pr-20"
                  />
                  <div class="absolute right-2 top-1/2 -translate-y-1/2">
                    <button
                      class="p-1 hover:bg-gray-100 rounded text-gray-400 hover:text-gray-600"
                      @click="firecrawlApiKeyVisible = !firecrawlApiKeyVisible"
                    >
                      <Icon :icon="firecrawlApiKeyVisible ? 'mdi:eye-off' : 'mdi:eye'" width="16" height="16" />
                    </button>
                  </div>
                </div>
              </div>

              <!-- Mode -->
              <div>
                <label class="block text-sm font-medium text-gray-700 mb-1.5">
                  抓取模式
                </label>
                <select
                  v-model="firecrawlMode"
                  class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:ring-2 focus:ring-purple-500 focus:border-transparent"
                >
                  <option value="scrape">Scrape（单页抓取）</option>
                  <option value="crawl">Crawl（整站爬取）</option>
                </select>
              </div>

              <!-- Timeout & Max Content -->
              <div class="grid grid-cols-2 gap-4">
                <div>
                  <label class="block text-sm font-medium text-gray-700 mb-1.5">
                    超时时间（秒）
                  </label>
                  <input
                    v-model.number="firecrawlTimeout"
                    type="number"
                    min="10"
                    max="300"
                    class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:ring-2 focus:ring-purple-500 focus:border-transparent"
                  />
                </div>
                <div>
                  <label class="block text-sm font-medium text-gray-700 mb-1.5">
                    最大内容长度
                  </label>
                  <input
                    v-model.number="firecrawlMaxContentLength"
                    type="number"
                    min="1000"
                    max="100000"
                    class="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg focus:ring-2 focus:ring-purple-500 focus:border-transparent"
                  />
                </div>
              </div>

              <!-- Save Button -->
              <div class="flex gap-2 pt-2">
                <button
                  class="px-4 py-2 text-sm font-medium text-white bg-purple-600 rounded-lg hover:bg-purple-700 transition-colors"
                  :disabled="firecrawlLoading"
                  @click="saveFirecrawlSettings"
                >
                  <Icon v-if="firecrawlLoading" icon="mdi:loading" width="14" height="14" class="animate-spin inline-block mr-1" />
                  保存配置
                </button>
              </div>
            </div>
          </div>

          <!-- Info Box -->
          <div class="bg-purple-50 rounded-lg p-4 flex items-start gap-3">
            <Icon icon="mdi:information" width="20" height="20" class="text-purple-600 flex-shrink-0 mt-0.5" />
            <div class="text-sm text-purple-900">
              <div class="font-medium mb-1">关于 Firecrawl</div>
              <p class="text-purple-700 text-xs">
                Firecrawl 是一个强大的网页抓取服务，可以提取网页的完整 Markdown 内容。
                在订阅源设置中启用后，系统会自动抓取文章全文。
              </p>
            </div>
          </div>
        </div>

        <!-- Schedulers Status Tab -->
        <div v-if="activeTab === 'schedulers'" class="space-y-6">
          <div v-if="schedulerLoading" class="flex items-center justify-center py-12">
            <Icon icon="mdi:loading" width="48" height="48" class="animate-spin text-blue-500" />
          </div>

          <div v-else class="space-y-4">
            <div
              v-for="scheduler in schedulerStatuses"
              :key="scheduler.name"
              class="border border-gray-200 rounded-xl overflow-hidden"
            >
              <div class="p-4 flex items-start justify-between">
                <div class="flex items-start gap-3">
                  <div
                    class="w-10 h-10 rounded-lg flex items-center justify-center bg-gradient-to-br"
                    :class="getSchedulerColor(scheduler.name)"
                  >
                    <Icon :icon="getSchedulerIcon(scheduler.name)" width="20" height="20" class="text-white" />
                  </div>
                  <div>
                    <div class="flex items-center gap-2">
                      <h3 class="font-semibold text-gray-900">{{ getSchedulerDisplayName(scheduler.name) }}</h3>
                      <span
                        class="px-2 py-0.5 text-xs font-medium rounded-full text-white"
                        :class="getStatusColor(getSchedulerStatusLabel(scheduler))"
                      >
                        {{ getSchedulerStatusLabel(scheduler) || 'idle' }}
                      </span>
                    </div>
                    <p class="text-xs text-gray-500 mt-0.5">
                      检查间隔: {{ formatSchedulerInterval(scheduler.check_interval) }}
                    </p>
                  </div>
                </div>
                <button
                  class="px-3 py-1.5 text-xs font-medium text-blue-600 bg-blue-50 rounded-lg hover:bg-blue-100 transition-colors"
                  :disabled="loading || isSchedulerBusy(scheduler)"
                  @click="triggerScheduler(scheduler.name)"
                >
                  <Icon v-if="loading" icon="mdi:loading" width="14" height="14" class="animate-spin inline-block mr-1" />
                  {{ isSchedulerBusy(scheduler) ? '执行中' : '手动执行' }}
                </button>
              </div>

              <div
                v-if="getSchedulerFeedback(scheduler.name)"
                class="mx-4 mb-4 rounded-lg border px-3 py-3 text-xs"
                :class="getSchedulerFeedback(scheduler.name)?.accepted ? 'border-emerald-200 bg-emerald-50 text-emerald-800' : 'border-rose-200 bg-rose-50 text-rose-700'"
              >
                {{ getSchedulerFeedback(scheduler.name)?.message }}
              </div>

              <div v-if="scheduler.database_state" class="border-t border-gray-100 p-4 bg-gray-50/50">
                <div class="grid grid-cols-4 gap-4 text-center">
                  <div>
                    <div class="text-lg font-bold text-gray-900">
                      {{ scheduler.database_state.total_executions }}
                    </div>
                    <div class="text-xs text-gray-500">总执行</div>
                  </div>
                  <div>
                    <div class="text-lg font-bold text-green-600">
                      {{ scheduler.database_state.successful_executions }}
                    </div>
                    <div class="text-xs text-gray-500">成功</div>
                  </div>
                  <div>
                    <div class="text-lg font-bold text-red-600">
                      {{ scheduler.database_state.failed_executions }}
                    </div>
                    <div class="text-xs text-gray-500">失败</div>
                  </div>
                  <div>
                    <div class="text-lg font-bold text-blue-600">
                      {{ (scheduler.database_state.success_rate || 0).toFixed(0) }}%
                    </div>
                    <div class="text-xs text-gray-500">成功率</div>
                  </div>
                </div>

                <div class="mt-4 pt-4 border-t border-gray-200/50 grid grid-cols-3 gap-4 text-xs">
                  <div class="flex items-center gap-2">
                    <Icon icon="mdi:clock-outline" width="14" height="14" class="text-gray-400" />
                    <span class="text-gray-600">上次执行:</span>
                    <span class="text-gray-900">
                      {{ scheduler.database_state.last_execution_time ? new Date(scheduler.database_state.last_execution_time).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '-' }}
                    </span>
                  </div>
                  <div class="flex items-center gap-2">
                    <Icon icon="mdi:timer-sand" width="14" height="14" class="text-gray-400" />
                    <span class="text-gray-600">执行耗时:</span>
                    <span class="text-gray-900">{{ formatDuration(scheduler.database_state.last_execution_duration) }}</span>
                  </div>
                  <div class="flex items-center gap-2">
                    <Icon icon="mdi:calendar-clock" width="14" height="14" class="text-gray-400" />
                    <span class="text-gray-600">下次执行:</span>
                    <span class="text-gray-900">{{ formatNextRun(scheduler.next_run) }}</span>
                  </div>
                </div>

                <div
                  v-if="scheduler.name === 'auto_refresh' && getAutoRefreshRunSummary(scheduler)"
                  class="mt-4 rounded-2xl border border-sky-200 bg-gradient-to-br from-sky-50 via-cyan-50 to-white p-4"
                >
                  <div class="flex items-start justify-between gap-4">
                    <div>
                      <div class="text-sm font-semibold text-gray-900">后台刷新状态</div>
                      <p class="mt-1 text-xs text-gray-600">
                        {{ getAutoRefreshReasonLabel(getAutoRefreshRunSummary(scheduler)?.reason) }}
                      </p>
                    </div>
                    <div class="rounded-full border border-sky-200 bg-white px-3 py-1 text-xs font-medium text-sky-700">
                      下次 {{ formatNextRun(scheduler.next_run || scheduler.database_state.next_execution_time) }}
                    </div>
                  </div>

                  <div class="mt-4 grid grid-cols-2 gap-3 md:grid-cols-4">
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-sky-100">
                      <div class="text-xs text-gray-500">扫描 feed</div>
                      <div class="mt-1 text-2xl font-bold text-gray-900">{{ getAutoRefreshRunSummary(scheduler)?.scanned_feeds ?? 0 }}</div>
                    </div>
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-sky-100">
                      <div class="text-xs text-gray-500">到点 feed</div>
                      <div class="mt-1 text-2xl font-bold text-sky-700">{{ getAutoRefreshRunSummary(scheduler)?.due_feeds ?? 0 }}</div>
                    </div>
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-sky-100">
                      <div class="text-xs text-gray-500">已触发</div>
                      <div class="mt-1 text-2xl font-bold text-emerald-600">{{ getAutoRefreshRunSummary(scheduler)?.triggered_feeds ?? 0 }}</div>
                    </div>
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-sky-100">
                      <div class="text-xs text-gray-500">已在刷新</div>
                      <div class="mt-1 text-2xl font-bold text-amber-600">{{ getAutoRefreshRunSummary(scheduler)?.already_refreshing_feeds ?? 0 }}</div>
                    </div>
                  </div>
                </div>

                <div
                  v-if="scheduler.name === 'tag_hierarchy_cleanup' && scheduler.last_run_summary"
                  class="mt-4 rounded-2xl border border-violet-200 bg-gradient-to-br from-violet-50 via-purple-50 to-white p-4"
                >
                  <div class="flex items-start justify-between gap-4">
                    <div>
                      <div class="text-sm font-semibold text-gray-900">标签清理状态</div>
                      <p class="mt-1 text-xs text-gray-600">
                        僵尸清理 · 平级合并 · 树审查 · LLM 预算
                      </p>
                    </div>
                    <div class="rounded-full border border-violet-200 bg-white px-3 py-1 text-xs font-medium text-violet-700">
                      {{ formatDuration(scheduler.database_state.last_execution_duration) }}
                    </div>
                  </div>

                  <div class="mt-4 grid grid-cols-2 gap-3 md:grid-cols-4">
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-violet-100">
                      <div class="text-xs text-gray-500">僵尸清理</div>
                      <div class="mt-1 text-2xl font-bold text-gray-900">{{ scheduler.last_run_summary.zombie_deactivated ?? 0 }}</div>
                    </div>
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-violet-100">
                      <div class="text-xs text-gray-500">平级合并</div>
                      <div class="mt-1 text-2xl font-bold text-sky-700">{{ scheduler.last_run_summary.flat_merges_applied ?? 0 }}</div>
                    </div>
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-violet-100">
                      <div class="text-xs text-gray-500">树审查</div>
                      <div class="mt-1 text-2xl font-bold text-emerald-600">{{ scheduler.last_run_summary.trees_reviewed ?? 0 }}</div>
                    </div>
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-violet-100">
                      <div class="text-xs text-gray-500">LLM 调用</div>
                      <div class="mt-1 text-2xl font-bold" :class="scheduler.last_run_summary.timed_out ? 'text-rose-600' : 'text-violet-600'">
                        {{ scheduler.last_run_summary.llm_calls_total ?? '-' }}
                        <span class="text-xs text-gray-400">/ {{ scheduler.last_run_summary.llm_budget_total ?? '-' }}</span>
                      </div>
                    </div>
                  </div>
                  <div v-if="scheduler.last_run_summary.timed_out" class="mt-2 text-xs text-rose-600">
                    ⚠ 执行超时，部分阶段未完成
                  </div>
                </div>

                <div
                  v-if="shouldShowContentCompletionPanel(scheduler)"
                  class="mt-4 rounded-2xl border border-amber-200 bg-gradient-to-br from-amber-50 via-orange-50 to-white p-4"
                >
                  <div class="flex items-start justify-between gap-4">
                    <div>
                      <div class="text-sm font-semibold text-gray-900">文章总结进度</div>
                      <p class="mt-1 text-xs text-gray-600">
                        别再猜了，这里直接看队列。
                      </p>
                    </div>
                    <div class="rounded-full border border-amber-200 bg-white px-3 py-1 text-xs font-medium text-amber-700">
                      下次 {{ formatNextRun(scheduler.next_run || scheduler.database_state.next_execution_time) }}
                    </div>
                  </div>

                  <div class="mt-4 grid grid-cols-2 gap-3 md:grid-cols-5">
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-amber-100">
                      <div class="text-xs text-gray-500">待总结</div>
                      <div class="mt-1 text-2xl font-bold text-gray-900">{{ getOverviewValue(scheduler, 'pending_count') }}</div>
                    </div>
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-amber-100">
                      <div class="text-xs text-gray-500">总结中</div>
                      <div class="mt-1 text-2xl font-bold text-amber-600">{{ getOverviewValue(scheduler, 'processing_count') }}</div>
                    </div>
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-amber-100">
                      <div class="text-xs text-gray-500">已完成</div>
                      <div class="mt-1 text-2xl font-bold text-emerald-600">{{ getOverviewValue(scheduler, 'completed_count') }}</div>
                    </div>
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-amber-100">
                      <div class="text-xs text-gray-500">失败</div>
                      <div class="mt-1 text-2xl font-bold text-rose-600">{{ getOverviewValue(scheduler, 'failed_count') }}</div>
                    </div>
                    <div class="rounded-xl bg-white p-3 shadow-sm ring-1 ring-amber-100">
                      <div class="text-xs text-gray-500">卡住</div>
                      <div class="mt-1 text-2xl font-bold text-sky-600">{{ getOverviewValue(scheduler, 'blocked_count') }}</div>
                    </div>
                  </div>

                  <div class="mt-4 rounded-xl border border-orange-200 bg-orange-50/80 p-4">
                    <div class="flex items-center gap-2 text-sm font-semibold text-gray-900">
                      <Icon icon="mdi:history" width="16" height="16" class="text-orange-600" />
                      <span>上一轮执行</span>
                    </div>

                    <div class="mt-3 grid grid-cols-2 gap-3 md:grid-cols-4">
                      <div class="rounded-lg bg-white px-3 py-2 ring-1 ring-orange-100">
                        <div class="text-[11px] text-gray-500">完成</div>
                        <div class="mt-1 text-lg font-semibold text-emerald-600">{{ getLastRunValue(scheduler, 'completed_count') }}</div>
                      </div>
                      <div class="rounded-lg bg-white px-3 py-2 ring-1 ring-orange-100">
                        <div class="text-[11px] text-gray-500">失败</div>
                        <div class="mt-1 text-lg font-semibold text-rose-600">{{ getLastRunValue(scheduler, 'failed_count') }}</div>
                      </div>
                      <div class="rounded-lg bg-white px-3 py-2 ring-1 ring-orange-100">
                        <div class="text-[11px] text-gray-500">卡住</div>
                        <div class="mt-1 text-lg font-semibold text-sky-600">{{ getLastRunValue(scheduler, 'blocked_count') }}</div>
                      </div>
                      <div class="rounded-lg bg-white px-3 py-2 ring-1 ring-orange-100">
                        <div class="text-[11px] text-gray-500">遗留 pending</div>
                        <div class="mt-1 text-lg font-semibold text-stone-700">{{ getLastRunValue(scheduler, 'stale_processing_count') }}</div>
                      </div>
                    </div>

                    <div class="mt-3 grid gap-3 text-xs md:grid-cols-2">
                      <div class="rounded-lg bg-white px-3 py-2 ring-1 ring-orange-100">
                        <div class="text-gray-500">上一轮最后处理</div>
                        <div class="mt-1 font-medium text-gray-900">{{ formatArticleLabel(scheduler.last_run_summary?.last_processed || null) }}</div>
                      </div>
                      <div class="rounded-lg bg-white px-3 py-2 ring-1 ring-orange-100">
                        <div class="text-gray-500">遗留 pending 指向</div>
                        <div class="mt-1 font-medium text-gray-900">{{ formatArticleLabel(scheduler.last_run_summary?.stale_processing_article || scheduler.stale_processing_article || null) }}</div>
                      </div>
                    </div>

                    <div v-if="scheduler.last_run_summary?.error_samples?.length" class="mt-3 rounded-lg bg-white px-3 py-3 ring-1 ring-orange-100">
                      <div class="text-xs text-gray-500">上一轮失败样本</div>
                      <div class="mt-2 space-y-2 text-xs">
                        <div v-for="sample in scheduler.last_run_summary.error_samples" :key="`${sample.article_id}-${sample.message}`" class="rounded-lg bg-stone-50 px-3 py-2">
                          <div class="font-medium text-gray-900">#{{ sample.article_id }} · {{ formatErrorCategory(sample.category) }}</div>
                          <div class="mt-1 text-gray-600">{{ sample.message }}</div>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div class="mt-4 grid gap-3 text-xs md:grid-cols-2">
                    <div class="rounded-xl bg-white/90 p-3 ring-1 ring-amber-100">
                      <div class="flex items-center gap-2 text-gray-500">
                        <Icon icon="mdi:loading" width="14" height="14" :class="scheduler.is_executing ? 'animate-spin text-amber-600' : 'text-gray-400'" />
                        <span>当前处理</span>
                      </div>
                      <div class="mt-2 font-medium text-gray-900">{{ formatArticleLabel(getCurrentContentCompletionArticle(scheduler)) }}</div>
                      <div v-if="!scheduler.current_article && getCurrentContentCompletionArticle(scheduler)" class="mt-1 text-[11px] text-stone-500">
                        当前没有活跃执行，这里展示的是遗留 pending 指向。
                      </div>
                      <div v-else-if="!scheduler.current_article && (scheduler.stale_processing_count || scheduler.overview?.stale_processing_count)" class="mt-1 text-[11px] text-stone-500">
                        当前进程没在跑，这更像遗留 pending。
                      </div>
                    </div>
                    <div class="rounded-xl bg-white/90 p-3 ring-1 ring-amber-100">
                      <div class="flex items-center gap-2 text-gray-500">
                        <Icon icon="mdi:check-decagram-outline" width="14" height="14" class="text-emerald-500" />
                        <span>最近处理</span>
                      </div>
                      <div class="mt-2 font-medium text-gray-900">{{ formatArticleLabel(scheduler.last_processed) }}</div>
                    </div>
                  </div>

                  <div class="mt-4 rounded-xl bg-white/90 p-3 ring-1 ring-amber-100">
                    <div class="flex items-center gap-2 text-xs text-gray-500">
                      <Icon icon="mdi:traffic-cone" width="14" height="14" class="text-amber-600" />
                      <span>卡住原因</span>
                    </div>

                    <div class="mt-3 grid grid-cols-2 gap-3 md:grid-cols-4">
                      <div class="rounded-lg bg-stone-50 px-3 py-2">
                        <div class="text-[11px] text-gray-500">等全文</div>
                        <div class="mt-1 text-lg font-semibold text-gray-900">{{ getBlockedReasonValue(scheduler, 'waiting_for_firecrawl_count') }}</div>
                      </div>
                      <div class="rounded-lg bg-stone-50 px-3 py-2">
                        <div class="text-[11px] text-gray-500">Feed 没开</div>
                        <div class="mt-1 text-lg font-semibold text-gray-900">{{ getBlockedReasonValue(scheduler, 'feed_disabled_count') }}</div>
                      </div>
                      <div class="rounded-lg bg-stone-50 px-3 py-2">
                        <div class="text-[11px] text-gray-500">AI 没配</div>
                        <div class="mt-1 text-lg font-semibold text-gray-900">{{ getBlockedReasonValue(scheduler, 'ai_unconfigured_count') }}</div>
                      </div>
                      <div class="rounded-lg bg-stone-50 px-3 py-2">
                        <div class="text-[11px] text-gray-500">正文是空的</div>
                        <div class="mt-1 text-lg font-semibold text-gray-900">{{ getBlockedReasonValue(scheduler, 'ready_but_missing_content_count') }}</div>
                      </div>
                    </div>

                    <div class="mt-3 text-[11px] text-gray-500">
                      <span v-if="scheduler.overview?.ai_configured">AI 配置在线。</span>
                      <span v-else>AI 配置还没就位，待总结那批会继续排队。</span>
                    </div>
                  </div>
                </div>

                <div v-if="scheduler.database_state.last_error" class="mt-3 p-2 bg-red-50 rounded-lg text-xs text-red-700 flex items-start gap-2">
                  <Icon icon="mdi:alert-circle" width="14" height="14" class="flex-shrink-0 mt-0.5" />
                  <span>{{ scheduler.database_state.last_error }}</span>
                </div>
              </div>

              <div v-else-if="scheduler.is_executing" class="border-t border-gray-100 p-3 bg-green-50/50 text-xs text-green-700 flex items-center gap-2">
                <Icon icon="mdi:loading" width="14" height="14" class="animate-spin" />
                正在执行中...
              </div>
            </div>

            <div class="bg-amber-50 rounded-lg p-4 flex items-start gap-3">
              <Icon icon="mdi:information" width="20" height="20" class="text-amber-600 flex-shrink-0 mt-0.5" />
              <div class="text-sm text-amber-900">
                <div class="font-medium mb-1">定时任务说明</div>
                <ul class="text-amber-700 text-xs space-y-1">
                  <li>• <b>后台刷新</b>: 自动检查并刷新有更新间隔设置的订阅源</li>
                  <li>• <b>自动总结</b>: 为启用 AI 总结的订阅源自动生成内容汇总</li>
                  <li>• <b>文章总结</b>: 用 Firecrawl 全文生成单篇 AI 总结</li>
                  <li>• <b>全文爬取</b>: 使用 Firecrawl 抓取文章完整内容</li>
                </ul>
              </div>
            </div>
          </div>
        </div>

      </div>

      <!-- Footer -->
      <div class="px-6 py-4 bg-gray-50 border-t border-gray-200 flex justify-end flex-shrink-0">
        <button class="btn btn-primary" @click="close">
          完成
        </button>
      </div>
    </div>
  </div>
</template>

