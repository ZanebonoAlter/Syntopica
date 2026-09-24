import { useFeedsApi } from '~/api'

/** 两次刷新启动时刻的最小间隔：机械保证「同一分钟至多 1 个刷新」 */
const MIN_GAP_MS = 60_000

interface RefreshTask {
  feedId: string
  intervalMinutes: number
  nextDueAt: number
}

export interface AutoRefreshOptions {
  /** 每次 feed 刷新成功后回调（按当前筛选 + 当前页重取列表，见 slim-article-list-payload） */
  onFeedRefreshed?: () => void | Promise<void>
}

export const useAutoRefresh = (options: AutoRefreshOptions = {}) => {
  const tasks = ref<Map<string, RefreshTask>>(new Map())
  const apiStore = useApiStore()
  const isRefreshing = ref(false)

  /** 单调度器句柄：同一时刻至多一个 pending setTimeout（串行推进） */
  let timer: ReturnType<typeof setTimeout> | null = null
  /** 上一次刷新启动时刻，用于 MIN_GAP_MS 串行错峰 */
  let lastRunStartedAt = 0
  let onFeedRefreshed = options.onFeedRefreshed

  function clearTimer() {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
  }

  function setOnFeedRefreshed(callback?: AutoRefreshOptions['onFeedRefreshed']) {
    onFeedRefreshed = callback
  }

  /** 只排「最早到期项」；实际启动时刻 = max(nextDueAt, lastRunStartedAt + MIN_GAP_MS) */
  function scheduleNext() {
    clearTimer()
    if (tasks.value.size === 0) return

    let earliest: RefreshTask | null = null
    for (const task of tasks.value.values()) {
      if (!earliest || task.nextDueAt < earliest.nextDueAt) {
        earliest = task
      }
    }
    if (!earliest) return

    const dueAt = Math.max(earliest.nextDueAt, lastRunStartedAt + MIN_GAP_MS)
    const feedId = earliest.feedId
    timer = setTimeout(() => {
      void runTask(feedId)
    }, Math.max(0, dueAt - Date.now()))
  }

  async function runTask(feedId: string) {
    const task = tasks.value.get(feedId)
    if (!task) {
      scheduleNext()
      return
    }

    // 被 MIN_GAP_MS 推迟：重排（不丢任务），到点再跑
    if (Date.now() < Math.max(task.nextDueAt, lastRunStartedAt + MIN_GAP_MS)) {
      scheduleNext()
      return
    }

    lastRunStartedAt = Date.now()
    isRefreshing.value = true
    try {
      const api = useFeedsApi()
      await api.refreshFeed(Number(feedId))
      await apiStore.fetchFeeds({ per_page: 10000 })
      await onFeedRefreshed?.()
    } catch (error) {
      console.error(`Auto-refresh failed for feed ${feedId}:`, error)
    } finally {
      isRefreshing.value = false
    }

    const current = tasks.value.get(feedId)
    if (current) {
      current.nextDueAt = Date.now() + current.intervalMinutes * 60 * 1000
    }
    scheduleNext()
  }

  function setupAutoRefresh(feedId: string, intervalMinutes: number) {
    if (intervalMinutes <= 0) {
      tasks.value.delete(feedId)
    } else {
      tasks.value.set(feedId, {
        feedId,
        intervalMinutes,
        nextDueAt: Date.now() + intervalMinutes * 60 * 1000,
      })
    }
    scheduleNext()
  }

  function initialize() {
    clearTimer()
    tasks.value.clear()

    // 同周期分组错峰：第 i 个 nextDueAt = now + i * spacing，spacing = max(1s, intervalMs / groupCount)
    const now = Date.now()
    const groups = new Map<number, string[]>()
    apiStore.feeds.forEach((feed) => {
      const interval = feed.refreshInterval || 0
      if (interval <= 0) return
      const group = groups.get(interval)
      if (group) {
        group.push(feed.id)
      } else {
        groups.set(interval, [feed.id])
      }
    })

    groups.forEach((feedIds, intervalMinutes) => {
      const intervalMs = intervalMinutes * 60 * 1000
      const spacing = Math.max(1000, Math.floor(intervalMs / feedIds.length))
      feedIds.forEach((feedId, index) => {
        tasks.value.set(feedId, {
          feedId,
          intervalMinutes,
          nextDueAt: now + index * spacing,
        })
      })
    })

    scheduleNext()
  }

  function updateFeedRefresh(feedId: string, intervalMinutes: number) {
    setupAutoRefresh(feedId, intervalMinutes)
  }

  function removeFeed(feedId: string) {
    tasks.value.delete(feedId)
    scheduleNext()
  }

  function cleanup() {
    clearTimer()
    tasks.value.clear()
  }

  const activeCount = computed(() => tasks.value.size)

  return {
    isRefreshing,
    activeCount,
    setOnFeedRefreshed,
    setupAutoRefresh,
    initialize,
    updateFeedRefresh,
    removeFeed,
    cleanup,
  }
}

let globalAutoRefresh: ReturnType<typeof useAutoRefresh> | null = null

export function useGlobalAutoRefresh(options: AutoRefreshOptions = {}) {
  if (!globalAutoRefresh) {
    globalAutoRefresh = useAutoRefresh()
  }
  // 只在显式传入时覆写：`useGlobalSettings.updateFeedSetting` 等调用点以无 options 形态
  // 取单例（仅用 updateFeedRefresh），无条件覆写会把已注册的 onFeedRefreshed 清成 undefined
  if (options.onFeedRefreshed) {
    globalAutoRefresh.setOnFeedRefreshed(options.onFeedRefreshed)
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
