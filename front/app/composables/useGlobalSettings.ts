import { computed, onMounted, ref } from 'vue'
import { useGlobalAutoRefresh } from '~/features/feeds/public'
import { useApiStore } from '~/stores/api'
import { useFeedsStore } from '~/stores/feeds'
import type { RssFeed } from '~/types'

export function useGlobalSettings() {
  const apiStore = useApiStore()
  const feedsStore = useFeedsStore()

  // ---- Dialog state ----
  const activeTab = ref<'feeds' | 'general' | 'queues' | 'firecrawl' | 'schedulers'>('feeds')
  const collapsedCategories = ref<Record<string, boolean>>({})
  const loading = ref(false)
  const error = ref<string | null>(null)
  const success = ref<string | null>(null)

  // ---- Computed ----
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

  // ---- Options ----
  const refreshOptions = [
    { label: '手动刷新', value: 0 },
    { label: '每 15 分钟', value: 15 },
    { label: '每 30 分钟', value: 30 },
    { label: '每小时', value: 60 },
    { label: '每 2 小时', value: 120 },
    { label: '每 6 小时', value: 360 },
    { label: '每天', value: 1440 },
  ]

  const maxArticlesOptions = [
    { label: '50 篇', value: 50 },
    { label: '100 篇', value: 100 },
    { label: '200 篇', value: 200 },
    { label: '500 篇', value: 500 },
    { label: '1000 篇', value: 1000 },
    { label: '无限制', value: 0 },
  ]

  // ---- Feed operations ----
  async function updateFeedSetting(
    feedId: string,
    setting: 'refresh_interval' | 'max_articles' | 'tagging_enabled' | 'firecrawl_enabled' | 'completion_on_refresh' | 'article_summary_enabled' | 'max_completion_retries' | 'category_id' | 'url',
    value: number | boolean | string | null
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

      if (setting === 'refresh_interval') {
        const autoRefresh = useGlobalAutoRefresh()
        autoRefresh.updateFeedRefresh(feedId, value as number)
      }

      setTimeout(() => { success.value = null }, 2000)
    } else {
      error.value = response.error || '更新失败'
    }
  }

  async function refreshFeed(feedId: string) {
    loading.value = true
    await apiStore.refreshFeed(feedId)
    await apiStore.fetchFeeds({ per_page: 10000 })
    loading.value = false
    success.value = '订阅源已刷新'
    setTimeout(() => { success.value = null }, 2000)
  }

  async function createCategoryAndAssign(feedId: string, name: string) {
    const trimmed = name.trim()
    if (!trimmed) return
    loading.value = true
    error.value = null
    success.value = null
    const response = await apiStore.createCategory({ name: trimmed })
    if (response.success && response.data) {
      await updateFeedSetting(feedId, 'category_id', Number(response.data.id))
      success.value = '分类已创建并应用'
      setTimeout(() => { success.value = null }, 2000)
    } else {
      error.value = response.error || '创建分类失败'
    }
    loading.value = false
  }

  async function deleteFeed(feedId: string, title: string, articleCount: number): Promise<boolean> {
    const ok = window.confirm(`确定删除「${title}」？将同时删除其下 ${articleCount} 篇文章，此操作不可撤销。`)
    if (!ok) return false
    loading.value = true
    error.value = null
    success.value = null
    const response = await apiStore.deleteFeed(feedId)
    loading.value = false
    if (response.success) {
      await apiStore.fetchFeeds({ per_page: 10000 })
      success.value = '订阅源已删除'
      setTimeout(() => { success.value = null }, 2000)
      return true
    }
    error.value = response.error || '删除失败'
    return false
  }

  // ---- AI summary / podcast settings (legacy — persisted but unused by panels) ----
  const aiBaseURL = ref('')
  const aiAPIKey = ref('')
  const aiModel = ref('')
  const showApiKey = ref(false)
  const autoSummaryEnabled = ref(false)
  const aiPodcastEnabled = ref(false)

  function loadAISettings() {
    aiBaseURL.value = ''
    aiAPIKey.value = ''
    aiModel.value = ''
    aiPodcastEnabled.value = false
    autoSummaryEnabled.value = false
  }

  onMounted(() => {
    loadAISettings()
  })

  return {
    // State
    activeTab, collapsedCategories, loading, error, success,

    // Computed
    feedsByCategory, categories: feedsStore.categories,

    // Constants
    refreshOptions, maxArticlesOptions,

    // Feed operations
    updateFeedSetting, refreshFeed, createCategoryAndAssign, deleteFeed,

    // Legacy AI settings (unused by panels, kept for back-compat)
    aiBaseURL, aiAPIKey, aiModel, showApiKey, autoSummaryEnabled,
    aiPodcastEnabled,

    // Stub
    testAIConnection: () => { error.value = '连接测试已迁移到 AI Router 面板，请在那里直接测试主模型。' },
  }
}

// Note: scheduler helpers are available directly from ~/utils/schedulerMeta
