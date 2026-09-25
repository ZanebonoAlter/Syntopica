import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'

/**
 * useTagQueueProgress 单测（tag-queue-progress-chip，S2 故事 + 白盒 E 组）：
 * - 数据源已合并进 /api/poll（usePollBundle 分发，harden-go-same-origin-serving）：
 *   对账断言走 poll bundle mock；completed_today → completedToday 映射不变
 * - WS 事件驱动刷新（V24：不本地累加，以 API 对账为准）
 * - 可见性唯一判据（E1/E2/E4）
 * - 对账失败静默降级（V23）
 */

const { getPollBundleMock, onMock } = vi.hoisted(() => ({
  getPollBundleMock: vi.fn(),
  onMock: vi.fn(() => () => {}),
}))

vi.mock('~/api/poll', () => ({
  usePollApi: () => ({ getPollBundle: getPollBundleMock }),
}))

let completedHandler: ((data: unknown) => void) | null = null
let failedHandler: ((data: unknown) => void) | null = null

vi.mock('~/composables/useEventStream', () => ({
  useEventStream: () => ({
    on: (type: string, handler: (data: unknown) => void) => {
      if (type === 'tag_completed') completedHandler = handler
      if (type === 'tag_failed') failedHandler = handler
      return () => {}
    },
    off: vi.fn(),
    connected: true,
  }),
}))

const { useTagQueueProgress, __resetTagQueueProgressForTest } = await import('./useTagQueueProgress')
const { usePollBundle } = await import('./usePollBundle')

/** /api/poll 合并响应（本用例只关心 schedulers/tag_queue 两个分项） */
function pollResponse(overrides: Partial<Record<string, number>> = {}) {
  return {
    success: true,
    data: {
      schedulers: [],
      tag_queue: {
        pending: 3,
        processing: 2,
        completed: 35,
        failed: 0,
        total: 40,
        completed_today: 5,
        ...overrides,
      },
      notifications: { unread: 0 },
    },
  }
}

beforeEach(() => {
  __resetTagQueueProgressForTest()
  usePollBundle().__resetPollBundleForTest()
  getPollBundleMock.mockReset().mockResolvedValue(pollResponse())
  onMock.mockClear()
  completedHandler = null
  failedHandler = null
  vi.useFakeTimers()
})

afterEach(() => {
  __resetTagQueueProgressForTest()
  usePollBundle().__resetPollBundleForTest()
  vi.useRealTimers()
})

describe('useTagQueueProgress — 对账与事件驱动', () => {
  it('ensureStarted 经合并入口对账一次（completed_today 映射 completedToday）', async () => {
    const p = useTagQueueProgress()
    p.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)
    expect(getPollBundleMock).toHaveBeenCalled()
    expect(p.status.value.pending).toBe(3)
    expect(p.status.value.processing).toBe(2)
    expect(p.status.value.completedToday).toBe(5)
    expect(p.status.value.failed).toBe(0)
  })

  it('S2 步 2 / V24：tag_completed 事件触发重新对账（计数以 API 为准，不本地累加）', async () => {
    const p = useTagQueueProgress()
    p.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)

    // 事件到达时 API 尚未更新 completed_today（仍是 5）→ 事件只触发刷新，计数不凭空 +1
    getPollBundleMock.mockResolvedValue(pollResponse({ completed_today: 6 }))
    completedHandler?.({})
    await vi.advanceTimersByTimeAsync(400)
    expect(p.status.value.completedToday).toBe(6)
  })

  it('V23：对账失败静默降级，保留旧值不报错', async () => {
    getPollBundleMock.mockRejectedValue(new Error('down'))
    const p = useTagQueueProgress()
    p.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)
    expect(p.status.value).toEqual({ pending: 0, processing: 0, completedToday: 0, failed: 0 })
    expect(p.visible.value).toBe(false)
  })

  it('tag_failed 事件同样触发对账', async () => {
    const p = useTagQueueProgress()
    p.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)
    getPollBundleMock.mockResolvedValue(pollResponse({ failed: 2 }))
    failedHandler?.({})
    await vi.advanceTimersByTimeAsync(400)
    expect(p.status.value.failed).toBe(2)
  })
})

describe('useTagQueueProgress — 可见性判定（白盒 E 唯一判据）', () => {
  it('E1：pending+leased==0 且 failed==0 → 不可见', async () => {
    getPollBundleMock.mockResolvedValue(pollResponse({ pending: 0, processing: 0, failed: 0 }))
    const p = useTagQueueProgress()
    p.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)
    expect(p.visible.value).toBe(false)
  })

  it('E2：pending+leased>0 → 可见', async () => {
    getPollBundleMock.mockResolvedValue(pollResponse({ pending: 3, processing: 2 }))
    const p = useTagQueueProgress()
    p.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)
    expect(p.activeCount.value).toBe(5)
    expect(p.visible.value).toBe(true)
  })

  it('E4：failed>0 且活跃==0 → 保持可见（失败态）', async () => {
    getPollBundleMock.mockResolvedValue(pollResponse({ pending: 0, processing: 0, failed: 2 }))
    const p = useTagQueueProgress()
    p.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)
    expect(p.visible.value).toBe(true)
    expect(p.isFailedState.value).toBe(true)
  })

  it('进度百分比 = completedToday / (active + completedToday)', async () => {
    getPollBundleMock.mockResolvedValue(pollResponse({ pending: 3, processing: 2, completed_today: 5 }))
    const p = useTagQueueProgress()
    p.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)
    expect(p.roundTotal.value).toBe(10)
    expect(p.progressPercent.value).toBe(50)
  })
})
