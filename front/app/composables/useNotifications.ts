/**
 * 通知中心全局 composable（notification-center）
 *
 * 状态管理（SSR 安全）走 useState 全局共享；WS 订阅守卫用模块级变量
 * （参照 useEventStream 的模块级单例先例——WS 本身是 client-only 逻辑）。
 *
 * 生命周期：ensureStarted() 由 app.vue 根层调用一次，订阅常驻不随组件卸载断开。
 * 未读数策略：启动拉一次 API 对账 → WS notification 事件 +1 → 60s 定时对账兜底
 * （useEventStream 未暴露重连回调，定时对账同时覆盖「断线重连后重拉」——见任务回报偏差说明）。
 */

import { useEventStream } from '~/composables/useEventStream'
import { EVENT_TYPES } from '~/utils/eventTypes'
import { useNotificationsApi, type AppNotification } from '~/api/notifications'

export interface NotificationViewState {
  list: AppNotification[]
  total: number
  loading: boolean
  error: string | null
}

const RECONCILE_INTERVAL_MS = 60_000
const PAGE_SIZE = 20

let started = false
let unsubNotification: (() => void) | null = null
let reconcileTimer: ReturnType<typeof setInterval> | null = null

export function useNotifications() {
  const api = useNotificationsApi()
  const stream = useEventStream()

  const unreadCount = useState<number>('notifications:unread-count', () => 0)
  const view = useState<NotificationViewState>('notifications:view', () => ({
    list: [],
    total: 0,
    loading: false,
    error: null,
  }))
  /** 打开即算浏览：本次浏览会话内保留未读强调（面板开着标记，关闭后强调消失） */
  const browsingSessionActive = useState<boolean>('notifications:browsing', () => false)
  /** 淘汰上限（与后端契约一致，前端仅用于滚动加载判断） */
  const hardCap = 500

  async function refreshUnreadCount() {
    try {
      const response = await api.unreadCount()
      if (response.success && response.data) {
        unreadCount.value = response.data.unread
      }
    } catch {
      // 静默降级：角标不显示不误报，定时对账/重连后自动恢复
    }
  }

  async function fetchList(offset = 0) {
    view.value = { ...view.value, loading: true, error: null }
    try {
      const response = await api.list({ limit: PAGE_SIZE, offset })
      if (response.success && response.data) {
        const normalized = response.data.notifications ?? []
        view.value = {
          list: offset === 0 ? normalized : [...view.value.list, ...normalized],
          total: response.data.total ?? normalized.length,
          loading: false,
          error: null,
        }
      } else {
        throw new Error(response.error || '加载通知失败')
      }
    } catch (err) {
      view.value = { ...view.value, loading: false, error: err instanceof Error ? err.message : '加载失败' }
    }
  }

  function onNotificationEvent() {
    // 实时事件：角标 +1（计数路径）；面板开着时同时刷新列表（首次页）
    unreadCount.value++
    if (browsingSessionActive.value) {
      void fetchList(0)
    }
  }

  /**
   * 启动常驻订阅（幂等，多次调用只生效一次）
   * 由 app.vue 根层调用；不与组件生命周期绑定
   */
  function ensureStarted() {
    if (started) return
    // WS 订阅仅客户端（SSR 侧不建 WebSocket；用 window 探测，Nuxt SSR 与 Vitest 环境一致）
    if (typeof window === 'undefined') return
    started = true

    unsubNotification = stream.on(EVENT_TYPES.NOTIFICATION, onNotificationEvent)
    void refreshUnreadCount()
    reconcileTimer = setInterval(() => {
      // 浏览会话内跳过回写：DB 端尚未收到 markAllRead 落库时（fail-open 失败态），
      // 旧未读数回写会让角标在浏览会话内闪烁回跳
      if (!browsingSessionActive.value) void refreshUnreadCount()
    }, RECONCILE_INTERVAL_MS)
  }

  /**
   * 打开面板：拉最新列表 + 打开即算浏览（design Decision 4：已读状态落库语义=已浏览过）
   * markAllRead 落库防止 60s reconcile 用 DB 真实未读数回写；未读条目的视觉强调
   * 保留到本次浏览会话（browsingSessionActive 标记，与落库已读正交）
   */
  async function openPanel() {
    browsingSessionActive.value = true
    await fetchList(0)
    unreadCount.value = 0
    // 落库：打开即全部已读（后端幂等；失败不阻断面板打开，reconcile 会在关闭后重拉）
    try {
      await api.markAllRead()
    } catch {
      // fail-open：角标已本地清零，关闭面板后 reconcile 以 DB 为准恢复
    }
  }

  /** 关闭面板：浏览会话结束（未读强调消失），重拉未读数对账 */
  function closePanel() {
    browsingSessionActive.value = false
    void refreshUnreadCount()
  }

  /** 单条标已读（乐观更新 + 回滚） */
  async function markRead(id: string) {
    const target = view.value.list.find(n => n.id === id)
    if (!target || target.is_read) return
    target.is_read = true
    try {
      const response = await api.markRead(id)
      if (!response.success) {
        if (target) target.is_read = false
        return
      }
      if (unreadCount.value > 0) unreadCount.value--
    } catch {
      if (target) target.is_read = false
    }
  }

  /** 全部标已读（幂等兜底由后端保证） */
  async function markAllRead() {
    const response = await api.markAllRead()
    if (response.success) {
      view.value = { ...view.value, list: view.value.list.map(n => ({ ...n, is_read: true })) }
      unreadCount.value = 0
    }
    return response.success
  }

  /** 清空全部（组件层已做 confirm 弹窗防护） */
  async function clearAll() {
    const response = await api.clearAll()
    if (response.success) {
      view.value = { list: [], total: 0, loading: false, error: null }
      unreadCount.value = 0
    }
    return response.success
  }

  function hasMore(): boolean {
    return view.value.list.length < view.value.total
  }

  return {
    unreadCount,
    view,
    browsingSessionActive,
    hardCap,
    pageSize: PAGE_SIZE,
    ensureStarted,
    openPanel,
    closePanel,
    fetchList,
    markRead,
    markAllRead,
    clearAll,
    hasMore,
    refreshUnreadCount,
  }
}

/** 测试隔离钩子：清掉模块级守卫与订阅（Vitest 各用例独立启动） */
export function __resetNotificationsForTest() {
  started = false
  if (unsubNotification) {
    unsubNotification()
    unsubNotification = null
  }
  if (reconcileTimer) {
    clearInterval(reconcileTimer)
    reconcileTimer = null
  }
}
