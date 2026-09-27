/**
 * 标签队列进度 composable（tag-queue-progress-chip）
 *
 * 数据源：GET /api/poll 对账分发（usePollBundle，client-poll-budget）
 * + WS tag_completed/tag_failed 事件驱动即时刷新
 * （事件只触发重新对账，不做本地增量——白盒 V24：WS 重放不导致重复累加）。
 *
 * 可见性判据（白盒 E，唯一判据）：pending+leased>0 || failed>0
 * —— WS 断线/事件不改变可见性判定来源（以最近一次对账为准）。
 */

import { useEventStream } from '~/composables/useEventStream'
import { EVENT_TYPES } from '~/utils/eventTypes'
import { usePollBundle } from '~/composables/usePollBundle'

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
let eventDebounce: ReturnType<typeof setTimeout> | null = null

export function useTagQueueProgress() {
  const poll = usePollBundle()
  const status = useState<TagQueueProgressState>('tag-queue-progress:status', () => ({
    pending: 0,
    processing: 0,
    completedToday: 0,
    failed: 0,
  }))
  /** 最近一次 status 对账成功时间戳（0 = 从未成功）；由 usePollBundle 对账分发 */
  const lastReconciledAt = useState<number>('tag-queue-progress:last', () => 0)

  /**
   * 对账已合并进 /api/poll（usePollBundle 分发，键 tag-queue-progress:status），
   * 保留函数名兼容 WS 事件路径与消费点：转调单例立即对账一次。
   */
  async function reconcile() {
    await poll.reconcileNow()
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
    // 常驻对账由 usePollBundle 单例承载（定时 + 可见性暂停）；本 composable
    // 只保留 WS 事件驱动的即时刷新。启动 bundle 幂等，重复调用无副作用。
    poll.ensureStarted()
    unsubCompleted = useEventStream().on(EVENT_TYPES.TAG_COMPLETED, onTagEvent)
    unsubFailed = useEventStream().on(EVENT_TYPES.TAG_FAILED, onTagEvent)
  }

  function stop() {
    started = false
    unsubCompleted?.()
    unsubCompleted = null
    unsubFailed?.()
    unsubFailed = null
    if (eventDebounce) {
      clearTimeout(eventDebounce)
      eventDebounce = null
    }
    // 注意：不停止 usePollBundle —— 它是全局常驻单例，不随芯片组件卸载断开
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
  if (eventDebounce) {
    clearTimeout(eventDebounce)
    eventDebounce = null
  }
}
