/**
 * 标签队列进度 composable（tag-queue-progress-chip）
 *
 * 数据源：GET /api/tag-queue/status 对账 + WS tag_completed/tag_failed 事件驱动刷新
 * （事件只触发重新对账，不做本地增量——白盒 V24：WS 重放不导致重复累加）。
 *
 * 可见性判据（白盒 E，唯一判据）：pending+leased>0 || failed>0
 * —— WS 断线/事件不改变可见性判定来源（以最近一次 status 对账为准）。
 */

import { useEventStream } from '~/composables/useEventStream'
import { EVENT_TYPES } from '~/utils/eventTypes'
import { useTagQueueApi } from '~/api'

const RECONCILE_INTERVAL_MS = 60_000
const EVENT_DEBOUNCE_MS = 300

export interface TagQueueProgressState {
  pending: number
  processing: number
  completedToday: number
  failed: number
}

let started = false
let unsubCompleted: (() => void) | null = null
let unsubFailed: (() => void) | null = null
let reconcileTimer: ReturnType<typeof setInterval> | null = null
let eventDebounce: ReturnType<typeof setTimeout> | null = null

export function useTagQueueProgress() {
  const api = useTagQueueApi()
  const status = useState<TagQueueProgressState>('tag-queue-progress:status', () => ({
    pending: 0,
    processing: 0,
    completedToday: 0,
    failed: 0,
  }))
  /** 最近一次 status 对账成功时间戳（0 = 从未成功） */
  const lastReconciledAt = useState<number>('tag-queue-progress:last', () => 0)

  async function reconcile() {
    try {
      const response = await api.getStatus()
      if (response.success && response.data) {
        status.value = {
          pending: response.data.pending ?? 0,
          processing: response.data.processing ?? 0,
          completedToday: response.data.completed_today ?? 0,
          failed: response.data.failed ?? 0,
        }
        lastReconciledAt.value = Date.now()
      }
    } catch {
      // 静默降级（V23）：status 失败不报错，保持旧值；恢复由下一轮对账/事件驱动
    }
  }

  function onTagEvent() {
    // 事件驱动刷新（V24 幂等：以 API 对账修正，不本地累加）
    if (eventDebounce) clearTimeout(eventDebounce)
    eventDebounce = setTimeout(() => {
      void reconcile()
    }, EVENT_DEBOUNCE_MS)
  }

  /** 启动常驻订阅（幂等）；由消费组件挂载时调用，stop() 由其卸载时调用 */
  function ensureStarted() {
    if (started) return
    started = true
    void reconcile()
    unsubCompleted = useEventStream().on(EVENT_TYPES.TAG_COMPLETED, onTagEvent)
    unsubFailed = useEventStream().on(EVENT_TYPES.TAG_FAILED, onTagEvent)
    reconcileTimer = setInterval(() => {
      void reconcile()
    }, RECONCILE_INTERVAL_MS)
  }

  function stop() {
    started = false
    unsubCompleted?.()
    unsubCompleted = null
    unsubFailed?.()
    unsubFailed = null
    if (reconcileTimer) {
      clearInterval(reconcileTimer)
      reconcileTimer = null
    }
    if (eventDebounce) {
      clearTimeout(eventDebounce)
      eventDebounce = null
    }
  }

  const activeCount = computed(() => status.value.pending + status.value.processing)
  /** 可见性唯一判据（白盒 E）：有活跃或仅剩失败都可见 */
  const visible = computed(() => activeCount.value > 0 || status.value.failed > 0)
  const isFailedState = computed(() => status.value.failed > 0)
  /** 本轮总量 = 今日已处理 + 活跃量；进度 = 已处理 / 总量 */
  const roundTotal = computed(() => activeCount.value + status.value.completedToday)
  const progressPercent = computed(() => {
    if (roundTotal.value === 0) return 0
    return Math.round((status.value.completedToday / roundTotal.value) * 100)
  })

  return {
    status,
    lastReconciledAt,
    activeCount,
    visible,
    isFailedState,
    roundTotal,
    progressPercent,
    ensureStarted,
    stop,
    reconcile,
  }
}

/** 测试隔离钩子 */
export function __resetTagQueueProgressForTest() {
  started = false
  unsubCompleted?.()
  unsubCompleted = null
  unsubFailed?.()
  unsubFailed = null
  if (reconcileTimer) {
    clearInterval(reconcileTimer)
    reconcileTimer = null
  }
  if (eventDebounce) {
    clearTimeout(eventDebounce)
    eventDebounce = null
  }
}
