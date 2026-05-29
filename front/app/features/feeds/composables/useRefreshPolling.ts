import { REFRESH_POLLING_INTERVAL, MAX_POLLING_TIME } from '~/utils/constants'

export function useRefreshPolling() {
  const apiStore = useApiStore()
  const feedsStore = useFeedsStore()

  const selectedCategory = ref<string | null>(null)
  const selectedFeed = ref<string | null>(null)
  const refreshPollingInterval = ref<ReturnType<typeof setTimeout> | null>(null)
  const isPolling = ref(false)

  function startPolling() {
    const startTime = Date.now()

    const poll = async () => {
      if (Date.now() - startTime > MAX_POLLING_TIME) {
        stopPolling()
        return
      }

      if (selectedFeed.value) {
        await apiStore.fetchFeeds({ per_page: 10000 })
      } else if (selectedCategory.value === 'uncategorized') {
        await apiStore.fetchFeeds({ uncategorized: true, per_page: 10000 })
      } else if (
        selectedCategory.value
        && selectedCategory.value !== 'favorites'
      ) {
        await apiStore.fetchFeeds({
          category_id: parseInt(selectedCategory.value),
          per_page: 10000,
        })
      } else {
        await apiStore.fetchFeeds({ per_page: 10000 })
      }

      const monitoredFeeds = selectedFeed.value
        ? feedsStore.feeds.filter((f) => f.id === selectedFeed.value)
        : feedsStore.feeds

      const stillRefreshing = monitoredFeeds.some((f) => f.refreshStatus === 'refreshing')

      if (stillRefreshing) {
        refreshPollingInterval.value = setTimeout(poll, REFRESH_POLLING_INTERVAL)
      } else {
        stopPolling()
      }
    }

    isPolling.value = true
    poll()
  }

  function stopPolling() {
    if (refreshPollingInterval.value) {
      clearTimeout(refreshPollingInterval.value)
      refreshPollingInterval.value = null
    }
    isPolling.value = false
  }

  function updateSelection(category: string | null, feed: string | null) {
    selectedCategory.value = category
    selectedFeed.value = feed
  }

  return {
    isPolling,
    startPolling,
    stopPolling,
    updateSelection,
  }
}
