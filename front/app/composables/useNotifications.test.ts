import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'

/**
 * useNotifications 单测（notification-center，S1 故事 + 白盒 D 组）：
 * - WS notification 事件驱动角标（S1 步 3）
 * - 启动 API 对账（未读数）
 * - 打开面板即算浏览（D1/D2）、标已读乐观更新、全部标已读
 */

const { listMock, unreadCountMock, markReadMock, markAllReadMock, clearAllMock } = vi.hoisted(() => ({
  listMock: vi.fn(),
  unreadCountMock: vi.fn(),
  markReadMock: vi.fn(),
  markAllReadMock: vi.fn(),
  clearAllMock: vi.fn(),
}))

vi.mock('~/api/notifications', () => ({
  useNotificationsApi: () => ({
    list: listMock,
    unreadCount: unreadCountMock,
    markRead: markReadMock,
    markAllRead: markAllReadMock,
    clearAll: clearAllMock,
  }),
}))

let eventHandler: ((data: unknown) => void) | null = null
const onCalls: string[] = []

vi.mock('~/composables/useEventStream', () => ({
  useEventStream: () => ({
    on: (type: string, handler: (data: unknown) => void) => {
      onCalls.push(type)
      eventHandler = handler
      return () => { eventHandler = null }
    },
    off: vi.fn(),
    connected: true,
  }),
}))

const { useNotifications, __resetNotificationsForTest } = await import('./useNotifications')

const items = [
  { id: '1', type: 'success', title: '日报已生成 · 9-17', summary: '共 6 个版面', link_type: null, link_id: null, is_read: false, created_at: '2026-09-17T04:00:00Z' },
  { id: '2', type: 'error', title: '日报生成有失败 · 9-16', summary: '1 个版面失败', link_type: 'daily-report', link_id: '9', is_read: false, created_at: '2026-09-16T21:00:00Z' },
]

beforeEach(() => {
  __resetNotificationsForTest()
  listMock.mockReset().mockResolvedValue({ success: true, data: { notifications: items, total: items.length } })
  unreadCountMock.mockReset().mockResolvedValue({ success: true, data: { unread: 0 } })
  markReadMock.mockReset().mockResolvedValue({ success: true, data: {} })
  markAllReadMock.mockReset().mockResolvedValue({ success: true, data: {} })
  clearAllMock.mockReset().mockResolvedValue({ success: true, data: {} })
  eventHandler = null
  onCalls.length = 0
})

afterEach(() => {
  __resetNotificationsForTest()
})

describe('useNotifications — 启动对账与 WS 事件驱动', () => {
  it('S1 步 3：ensureStarted 拉一次未读数对账，WS notification 事件使角标 +1', async () => {
    unreadCountMock.mockResolvedValue({ success: true, data: { unread: 0 } })
    const notif = useNotifications()
    notif.ensureStarted()
    await Promise.resolve()
    expect(notif.unreadCount.value).toBe(0)
    expect(unreadCountMock).toHaveBeenCalledTimes(1)

    eventHandler?.({})
    await Promise.resolve()
    expect(notif.unreadCount.value).toBe(1)
  })

  it('未读数 API 失败静默降级：不抛错、角标不误报', async () => {
    unreadCountMock.mockRejectedValue(new Error('network down'))
    const notif = useNotifications()
    notif.ensureStarted()
    await Promise.resolve()
    expect(notif.unreadCount.value).toBe(0)
  })

  it('订阅常驻（ensureStarted 幂等，不重复注册 handler）', () => {
    const notif = useNotifications()
    notif.ensureStarted()
    notif.ensureStarted()
    expect(onCalls.filter(t => t === 'notification')).toHaveLength(1)
    expect(eventHandler).not.toBeNull()
  })

  it('面板开着时收到事件：角标 +1 且列表首拉刷新', async () => {
    unreadCountMock.mockResolvedValue({ success: true, data: { unread: 0 } })
    const notif = useNotifications()
    notif.ensureStarted()
    await notif.openPanel()
    expect(notif.view.value.list).toHaveLength(2)
    listMock.mockClear()
    eventHandler?.({})
    await Promise.resolve()
    expect(notif.unreadCount.value).toBe(1)
    expect(listMock).toHaveBeenCalledWith({ limit: 20, offset: 0 })
  })
})

describe('useNotifications — 面板列表与已读语义（白盒 D）', () => {
  it('openPanel 拉列表并清零角标（D1：打开即算浏览），落库已浏览且视觉强调保留', async () => {
    // design Decision 4：已读状态落库语义 = 已浏览过 → openPanel 调 raw api.markAllRead；
    // 本地 list 的 is_read 不翻转（浏览会话内保留未读视觉强调，与落库正交）
    const notif = useNotifications()
    notif.ensureStarted()
    await notif.openPanel()
    expect(notif.view.value.list).toHaveLength(2)
    expect(notif.unreadCount.value).toBe(0)
    expect(notif.browsingSessionActive.value).toBe(true)
    expect(markAllReadMock).toHaveBeenCalledTimes(1)
    expect(notif.view.value.list.every(n => !n.is_read)).toBe(true)
  })

  it('M2：浏览会话内 reconcile 跳过回写（60s 定时器不覆盖本地清零）', async () => {
    vi.useFakeTimers()
    try {
      const notif = useNotifications()
      notif.ensureStarted()
      await notif.openPanel()
      unreadCountMock.mockResolvedValue({ success: true, data: { unread: 7 } })
      unreadCountMock.mockClear()
      // 快进 60s：浏览会话内 reconcile 不应回写
      vi.advanceTimersByTime(60_000)
      await Promise.resolve()
      expect(unreadCountMock).not.toHaveBeenCalled()
      expect(notif.unreadCount.value).toBe(0)
      // 关闭面板后恢复回写
      notif.closePanel()
      await Promise.resolve()
      expect(notif.unreadCount.value).toBe(7)
      // 再快进一轮：非浏览态下 reconcile 恢复
      unreadCountMock.mockClear()
      vi.advanceTimersByTime(60_000)
      await Promise.resolve()
      expect(unreadCountMock).toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('closePanel 结束浏览态并重拉未读数对账', async () => {
    unreadCountMock.mockResolvedValue({ success: true, data: { unread: 5 } })
    const notif = useNotifications()
    notif.ensureStarted()
    await notif.openPanel()
    notif.closePanel()
    expect(notif.browsingSessionActive.value).toBe(false)
    await Promise.resolve()
    expect(notif.unreadCount.value).toBe(5)
  })

  it('markRead 乐观置已读；打开面板清零后标已读不再使角标变负；API 失败回滚', async () => {
    unreadCountMock.mockResolvedValue({ success: true, data: { unread: 2 } })
    const notif = useNotifications()
    notif.ensureStarted()
    await notif.openPanel()
    expect(notif.unreadCount.value).toBe(0)
    await notif.markRead('1')
    expect(markReadMock).toHaveBeenCalledWith('1')
    expect(notif.view.value.list.find(n => n.id === '1')?.is_read).toBe(true)
    expect(notif.unreadCount.value).toBe(0)

    // 失败回滚（对未读条 '2'）
    markReadMock.mockResolvedValueOnce({ success: false, error: 'boom' })
    await notif.markRead('2')
    expect(notif.view.value.list.find(n => n.id === '2')?.is_read).toBe(false)
  })

  it('未浏览（面板未开）场景下角标 >0 时单条标已读使角标 -1（D3 完整语义）', async () => {
    unreadCountMock.mockResolvedValue({ success: true, data: { unread: 1 } })
    const notif = useNotifications()
    notif.ensureStarted()
    await Promise.resolve()
    expect(notif.unreadCount.value).toBe(1)
    // 直接标记列表内条目（绕过面板浏览流程，模拟在面板关闭前发出的一次标记）
    notif.view.value.list = [{ id: '1', type: 'success', title: 't', summary: 's', link_type: null, link_id: null, is_read: false, created_at: '2026-09-17T04:00:00Z' }]
    await notif.markRead('1')
    expect(notif.unreadCount.value).toBe(0)
  })

  it('重复标已读同一条：is_read 已 true 时 no-op 不再发请求（D4 前端侧）', async () => {
    unreadCountMock.mockResolvedValue({ success: true, data: { unread: 2 } })
    const notif = useNotifications()
    notif.ensureStarted()
    await notif.openPanel()
    await notif.markRead('1')
    const callsAfterFirst = markReadMock.mock.calls.length
    await notif.markRead('1')
    expect(markReadMock.mock.calls.length).toBe(callsAfterFirst)
  })

  it('markAllRead 全置已读且角标归零', async () => {
    unreadCountMock.mockResolvedValue({ success: true, data: { unread: 2 } })
    const notif = useNotifications()
    notif.ensureStarted()
    await notif.openPanel()
    await notif.markAllRead()
    expect(markAllReadMock).toHaveBeenCalled()
    expect(notif.unreadCount.value).toBe(0)
    expect(notif.view.value.list.every(n => n.is_read)).toBe(true)
  })

  it('clearAll 清列表清角标', async () => {
    const notif = useNotifications()
    notif.ensureStarted()
    await notif.openPanel()
    await notif.clearAll()
    expect(clearAllMock).toHaveBeenCalled()
    expect(notif.view.value.list).toHaveLength(0)
    expect(notif.unreadCount.value).toBe(0)
  })
})
