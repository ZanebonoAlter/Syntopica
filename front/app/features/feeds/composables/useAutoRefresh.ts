import { useFeedsApi } from '~/api'

interface RefreshTimer {
  feedId: string
  interval: number
  timer: ReturnType<typeof setInterval> | null
}

export const useAutoRefresh = () => {
  const timers = ref<Map<string, RefreshTimer>>(new Map())
  const apiStore = useApiStore()
  const isRefreshing = ref(false)

  function clearTimer(feedId: string) {
    const timerData = timers.value.get(feedId)
    if (timerData?.timer) {
      clearInterval(timerData.timer)
      timers.value.delete(feedId)
    }
  }

  function setupAutoRefresh(feedId: string, intervalMinutes: number) {
    clearTimer(feedId)

    if (intervalMinutes <= 0) {
      return
    }

    const intervalMs = intervalMinutes * 60 * 1000
    const api = useFeedsApi()

    const timer = setInterval(async () => {
      try {
        await api.refreshFeed(Number(feedId))
        await apiStore.fetchFeeds({ per_page: 10000 })
        await apiStore.fetchArticles({ per_page: 10000 })
      } catch (error) {
        console.error(`Auto-refresh failed for feed ${feedId}:`, error)
      }
    }, intervalMs)

    timers.value.set(feedId, {
      feedId,
      interval: intervalMinutes,
      timer,
    })
  }

  function initialize() {
    timers.value.forEach((timerData) => {
      if (timerData.timer) {
        clearInterval(timerData.timer)
      }
    })
    timers.value.clear()

    apiStore.feeds.forEach((feed) => {
      const interval = feed.refreshInterval || 0
      if (interval > 0) {
        setupAutoRefresh(feed.id, interval)
      }
    })
  }

  function updateFeedRefresh(feedId: string, intervalMinutes: number) {
    setupAutoRefresh(feedId, intervalMinutes)
  }

  function removeFeed(feedId: string) {
    clearTimer(feedId)
  }

  function cleanup() {
    timers.value.forEach((timerData) => {
      if (timerData.timer) {
        clearInterval(timerData.timer)
      }
    })
    timers.value.clear()
  }

  const activeCount = computed(() => timers.value.size)

  return {
    timers,
    isRefreshing,
    activeCount,
    setupAutoRefresh,
    initialize,
    updateFeedRefresh,
    removeFeed,
    cleanup,
  }
}

let globalAutoRefresh: ReturnType<typeof useAutoRefresh> | null = null

export function useGlobalAutoRefresh() {
  if (!globalAutoRefresh) {
    globalAutoRefresh = useAutoRefresh()
  }

  onMounted(() => {
    const apiStore = useApiStore()
    watch(
      () => apiStore.feeds.length,
      (length) => {
        if (length > 0) {
          globalAutoRefresh?.initialize()
        }
      },
      { immediate: true }
    )
  })

  onUnmounted(() => {
    globalAutoRefresh?.cleanup()
  })

  return globalAutoRefresh
}
