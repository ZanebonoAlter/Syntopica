import type { Ref } from 'vue'
import { isHotScheduler } from '~/utils/schedulerMeta'
import { usePollApi, type PollBundleResponse } from '~/api/poll'
import type { SchedulerAIHealthRoute, SchedulerStatus } from '~/types/scheduler'

/**
 * 常驻对账单例（client-poll-budget 契约，openspec: harden-go-same-origin-serving）：
 *
 * - 三类常驻状态（调度器状态 / 标签队列计数 / 未读数）合并为单一 /api/poll
 *   请求；结果分发到三个既有消费点的共享 state（useState 键名不变，组件层
 *   props/DOM 零改动）。
 * - 自适应间隔硬下限：热态（调度器执行中）≥ 5s、近期触发反馈 ≥ 15s、空闲
 *   ≥ 30s；热态结束回到空闲间隔。
 * - 频次硬顶：任意 60 秒窗口内 ≤ 4 次请求（含热态与恢复可见的立即对账）。
 * - 页面不可见（document.hidden）暂停；恢复可见立即对账一次。
 * - 对账失败保留全部旧值（不清零、不转错误态）并指数退避（下限=空闲间隔，
 *   封顶 5 分钟）。
 *
 * 实现注意：**状态与函数全部模块级单例**。若函数随每次 usePollBundle() 调用
 * 重建，visibilitychange 监听的 remove 会因引用不同而永远失效（每次调用泄漏
 * 一个监听 → 恢复可见时并发 fire 数 = 泄漏数），fire 链也会互相踩并发。
 */

export const POLL_IDLE_INTERVAL_MS = 30_000
export const POLL_RECENT_INTERVAL_MS = 15_000
export const POLL_HOT_INTERVAL_MS = 5_000
/** 近期反馈窗口：触发/用户动作后该时长内按「近期反馈」节奏对账（与原 useSchedulerStatus 的 20s 一致） */
export const POLL_RECENT_FEEDBACK_WINDOW_MS = 20_000
export const POLL_BUDGET_WINDOW_MS = 60_000
export const POLL_BUDGET_MAX_REQUESTS = 4
export const POLL_BACKOFF_MAX_MS = 300_000

interface PollStates {
  schedulers: Ref<SchedulerStatus[]>
  analysisPaused: Ref<boolean>
  analysisPausedAt: Ref<string>
  aiHealthy: Ref<boolean>
  aiHealthRoutes: Ref<SchedulerAIHealthRoute[]>
  tagQueueStatus: Ref<{ pending: number; processing: number; completedToday: number; failed: number }>
  tagQueueLastReconciledAt: Ref<number>
  unreadCount: Ref<number>
  browsingSessionActive: Ref<boolean>
  lastFeedbackAt: Ref<number>
}

// useState 须在首次 usePollBundle()（setup 上下文）调用时绑定一次，之后
// 所有上下文（事件回调、定时器、其他组件）复用同一组引用。
let states: PollStates | null = null

let started = false
let visibilityHooked = false
let pollTimer: ReturnType<typeof setTimeout> | null = null
/** 频次预算窗口内的请求时间戳（滑动窗口） */
let fireTimestamps: number[] = []
let consecutiveFailures = 0

function bindStates(): PollStates {
  if (!states) {
    states = {
      // ── 分发目标：三个既有消费点的全局 state（键名与各 composable 一致）──
      schedulers: useState<SchedulerStatus[]>('poll:schedulers', () => []),
      analysisPaused: useState<boolean>('scheduler:analysis-paused', () => false),
      analysisPausedAt: useState<string>('scheduler:analysis-paused-at', () => ''),
      aiHealthy: useState<boolean>('scheduler:ai-healthy', () => true),
      aiHealthRoutes: useState<SchedulerAIHealthRoute[]>('scheduler:ai-health-routes', () => []),
      tagQueueStatus: useState('tag-queue-progress:status', () => ({
        pending: 0,
        processing: 0,
        completedToday: 0,
        failed: 0,
      })),
      tagQueueLastReconciledAt: useState<number>('tag-queue-progress:last', () => 0),
      unreadCount: useState<number>('notifications:unread-count', () => 0),
      // 浏览会话标记（useNotifications 定义）：面板开着时对账不回写未读数，
      // 避免 markAllRead 落库失败（fail-open）时角标在浏览会话内闪烁回跳。
      browsingSessionActive: useState<boolean>('notifications:browsing', () => false),
      // 近期反馈时间戳：由 useSchedulerStatus 的触发/更新动作写入。
      lastFeedbackAt: useState<number>('poll:last-feedback', () => 0),
    }
  }
  return states
}

function stopTimer() {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
}

/** 热态：任一热调度器正在执行（原 useSchedulerStatus 的 8s 档判定） */
function isHot(): boolean {
  return (states?.schedulers.value ?? []).some(
    s => isHotScheduler(s.name) && s.is_executing === true,
  )
}

function hasRecentFeedback(): boolean {
  const last = states?.lastFeedbackAt.value ?? 0
  return last > 0 && Date.now() - last < POLL_RECENT_FEEDBACK_WINDOW_MS
}

/** 当前节奏的基线间隔（热态 ≥5s / 近期反馈 ≥15s / 空闲 ≥30s） */
function baseInterval(): number {
  if (isHot()) return POLL_HOT_INTERVAL_MS
  if (hasRecentFeedback()) return POLL_RECENT_INTERVAL_MS
  return POLL_IDLE_INTERVAL_MS
}

/** 失败退避：指数翻倍，下限=空闲间隔（spec 边界值），封顶 5 分钟 */
function backoffDelay(): number {
  const factor = Math.min(2 ** consecutiveFailures, 16)
  return Math.min(POLL_IDLE_INTERVAL_MS * factor, POLL_BACKOFF_MAX_MS)
}

/** 频次预算：60s 窗口内已发满则推迟到最早一次滑出窗口（SHALL ≤ 4 次/分钟） */
function budgetedDelay(base: number): number {
  const now = Date.now()
  fireTimestamps = fireTimestamps.filter(t => now - t < POLL_BUDGET_WINDOW_MS)
  if (fireTimestamps.length >= POLL_BUDGET_MAX_REQUESTS) {
    const oldest = fireTimestamps[0] ?? now
    return Math.max(base, POLL_BUDGET_WINDOW_MS - (now - oldest) + 50)
  }
  return base
}

function scheduleNext() {
  stopTimer()
  const base = consecutiveFailures > 0 ? Math.max(backoffDelay(), baseInterval()) : baseInterval()
  pollTimer = setTimeout(() => {
    void fire()
  }, budgetedDelay(base))
}

async function reconcile(): Promise<boolean> {
  const s = bindStates()
  try {
    const { getPollBundle } = usePollApi()
    const response: PollBundleResponse = await getPollBundle()
    if (!response.success || !response.data) return false

    // 调度器状态：数组缺失/非数组保留旧值；顶层语义仅在调度器分支更新时同步
    if (Array.isArray(response.data.schedulers)) {
      s.schedulers.value = response.data.schedulers
      s.analysisPaused.value = response.analysis_paused === true
      s.analysisPausedAt.value = response.analysis_paused_at ?? ''
      s.aiHealthy.value = response.ai_healthy !== false
      s.aiHealthRoutes.value = response.ai_health_routes ?? []
    }
    // 标签队列计数
    if (response.data.tag_queue) {
      s.tagQueueStatus.value = {
        pending: response.data.tag_queue.pending ?? 0,
        processing: response.data.tag_queue.processing ?? 0,
        completedToday: response.data.tag_queue.completed_today ?? 0,
        failed: response.data.tag_queue.failed ?? 0,
      }
      s.tagQueueLastReconciledAt.value = Date.now()
    }
    // 未读数：浏览会话内跳过回写（见 browsingSessionActive 注释）
    if (response.data.notifications && !s.browsingSessionActive.value) {
      s.unreadCount.value = response.data.notifications.unread
    }
    return true
  } catch {
    // 静默降级：失败保留旧值 + 退避（由 fire 的调度逻辑处理），不转错误态
    return false
  }
}

async function fire(): Promise<void> {
  fireTimestamps.push(Date.now())
  const ok = await reconcile()
  if (ok) {
    consecutiveFailures = 0
  } else {
    consecutiveFailures++
  }
  scheduleNext()
}

/** 立即对账一次并重新排程（恢复可见 / 用户动作后的即时刷新走这里） */
async function reconcileNow(): Promise<void> {
  stopTimer()
  await fire()
}

function onVisibilityChange() {
  if (typeof document === 'undefined') return
  if (document.hidden) {
    // 后台标签页零轮询
    stopTimer()
  } else {
    // 恢复可见立即对账一次（不等待下一个间隔）
    void reconcileNow()
  }
}

/**
 * 启动常驻对账（幂等，多次调用只生效一次）；不与组件生命周期绑定——
 * 由三个消费 composable 的 ensureStarted 转调（app.vue 根层 / 芯片组件挂载）。
 */
function ensureStarted() {
  bindStates()
  if (started) return
  // 定时器/监听均 client-only（与 useNotifications 的 window 探测一致，SSR 安全）
  if (typeof window === 'undefined') return
  started = true
  if (!visibilityHooked && typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', onVisibilityChange)
    visibilityHooked = true
  }
  void fire()
}

export function usePollBundle() {
  const s = bindStates()
  return {
    /** 调度器状态列表（分发目标，供热态判定与消费点读取） */
    schedulers: s.schedulers,
    reconcile,
    reconcileNow,
    ensureStarted,
    isHot,
    hasRecentFeedback,
    baseInterval,
    /** 测试隔离钩子：重置单例（不负责恢复真实 timers，由测试的 useRealTimers 处理） */
    __resetPollBundleForTest() {
      stopTimer()
      started = false
      if (visibilityHooked && typeof document !== 'undefined') {
        document.removeEventListener('visibilitychange', onVisibilityChange)
        visibilityHooked = false
      }
      fireTimestamps = []
      consecutiveFailures = 0
    },
  }
}
