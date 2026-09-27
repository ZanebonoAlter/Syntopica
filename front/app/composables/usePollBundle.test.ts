import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * usePollBundle 单测（client-poll-budget 契约）：
 * - 单一合并入口：一次 /api/poll 同时分发三类常驻状态
 * - 间隔硬下限：空闲 ≥30s / 近期反馈 ≥15s / 热态 ≥5s
 * - 频次硬顶：任意 60 秒窗口 ≤4 次
 * - 页面隐藏暂停、恢复可见立即对账
 * - 失败保留旧值 + 退避（下限=空闲间隔）
 * - 字段缺失：缺失类保留旧值，其余正常更新
 */

const { getPollBundleMock } = vi.hoisted(() => ({
  getPollBundleMock: vi.fn(),
}))

vi.mock('~/api/poll', () => ({
  usePollApi: () => ({ getPollBundle: getPollBundleMock }),
}))

const { usePollBundle, POLL_BUDGET_MAX_REQUESTS } = await import('./usePollBundle')

function bundleResponse(overrides: {
  executing?: boolean
  withoutTagQueue?: boolean
  unread?: number
} = {}) {
  const data: Record<string, unknown> = {
    schedulers: [{ name: 'auto_refresh', is_executing: overrides.executing ?? false }],
    tag_queue: {
      pending: 1, processing: 2, completed: 10, failed: 0, total: 13, completed_today: 5,
    },
    notifications: { unread: overrides.unread ?? 7 },
  }
  if (overrides.withoutTagQueue) delete data.tag_queue
  return {
    success: true,
    data,
    analysis_paused: false,
    analysis_paused_at: '',
    ai_healthy: true,
    ai_health_routes: [],
  }
}

function setHidden(hidden: boolean) {
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden })
  document.dispatchEvent(new Event('visibilitychange'))
}

beforeEach(() => {
  vi.useFakeTimers()
  getPollBundleMock.mockReset().mockResolvedValue(bundleResponse())
})

afterEach(async () => {
  usePollBundle().__resetPollBundleForTest()
  // 冲净 in-flight 的 fire 异步链：stopTimer 只清已排程 timer，已挂起的
  // reconcile promise 恢复后会重新 scheduleNext —— 先 flush 链、再跑完
  // 新排的 timer，避免泄漏到下一个用例（残留链会污染请求计数断言）。
  await vi.advanceTimersByTimeAsync(0)
  await vi.advanceTimersByTimeAsync(10 * 60_000)
  // 恢复 document.hidden（happy-dom 原型链上的 getter）
  delete (document as { hidden?: unknown }).hidden
  vi.useRealTimers()
})

describe('usePollBundle — 单一合并入口', () => {
  it('一次 /api/poll 请求同时分发调度器状态、队列计数与未读数', async () => {
    const poll = usePollBundle()
    poll.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)

    expect(getPollBundleMock).toHaveBeenCalledTimes(1)
    expect(poll.schedulers.value).toHaveLength(1)
    expect(useState<{ pending: number }>('tag-queue-progress:status').value).toEqual({
      pending: 1,
      processing: 2,
      completedToday: 5,
      failed: 0,
    })
    expect(useState<number>('notifications:unread-count').value).toBe(7)
    expect(useState<boolean>('scheduler:analysis-paused').value).toBe(false)
    expect(useState<boolean>('scheduler:ai-healthy').value).toBe(true)
  })

  it('字段缺失：缺失类保留旧值，其余两类正常更新', async () => {
    const poll = usePollBundle()
    poll.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)

    getPollBundleMock.mockResolvedValue(bundleResponse({ withoutTagQueue: true, unread: 9 }))
    await poll.reconcileNow()
    await vi.advanceTimersByTimeAsync(0)

    // 队列计数保留旧值；未读数正常更新
    expect(useState<{ pending: number }>('tag-queue-progress:status').value).toEqual({
      pending: 1,
      processing: 2,
      completedToday: 5,
      failed: 0,
    })
    expect(useState<number>('notifications:unread-count').value).toBe(9)
  })
})

describe('usePollBundle — 间隔下限与频次预算', () => {
  it('空闲态相邻两次对账间隔 ≥ 30 秒', async () => {
    const poll = usePollBundle()
    poll.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)
    expect(getPollBundleMock).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(29_999)
    expect(getPollBundleMock).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(getPollBundleMock).toHaveBeenCalledTimes(2)
  })

  it('热态（调度器执行中）间隔 ≥ 5s；执行结束后回到 ≥ 30s', async () => {
    getPollBundleMock.mockResolvedValue(bundleResponse({ executing: true }))
    const poll = usePollBundle()
    poll.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)

    await vi.advanceTimersByTimeAsync(4_999)
    expect(getPollBundleMock).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(getPollBundleMock).toHaveBeenCalledTimes(2)

    // 执行结束 → 回到空闲档（30s 后才下一次）
    getPollBundleMock.mockResolvedValue(bundleResponse({ executing: false }))
    await vi.advanceTimersByTimeAsync(30_000)
    expect(getPollBundleMock).toHaveBeenCalledTimes(3)
  })

  it('近期触发反馈进入 ≥ 15s 档（20s 反馈窗口内）', async () => {
    const poll = usePollBundle()
    poll.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)

    // 模拟触发动作写反馈时间戳后立即对账（同 triggerScheduler → reconcileNow）
    useState<number>('poll:last-feedback').value = Date.now()
    await poll.reconcileNow()

    await vi.advanceTimersByTimeAsync(14_999)
    expect(getPollBundleMock).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(getPollBundleMock).toHaveBeenCalledTimes(3)
  })

  it(`单页频次上限：热态驱动下 60 秒窗口内 ≤ ${POLL_BUDGET_MAX_REQUESTS} 次`, async () => {
    getPollBundleMock.mockResolvedValue(bundleResponse({ executing: true }))
    const poll = usePollBundle()
    poll.ensureStarted()
    // 热态 5s 基线会被预算硬顶：60 秒窗口内最多 4 次
    await vi.advanceTimersByTimeAsync(60_000)
    expect(getPollBundleMock).toHaveBeenCalledTimes(4)
    // 窗口滑出后恢复放行
    await vi.advanceTimersByTimeAsync(200)
    expect(getPollBundleMock).toHaveBeenCalledTimes(5)
  })
})

describe('usePollBundle — 页面可见性', () => {
  it('后台标签页 10 分钟 0 请求；恢复可见立即对账 1 次', async () => {
    const poll = usePollBundle()
    poll.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)
    expect(getPollBundleMock).toHaveBeenCalledTimes(1)
    getPollBundleMock.mockClear()

    setHidden(true)
    await vi.advanceTimersByTimeAsync(10 * 60_000)
    expect(getPollBundleMock).not.toHaveBeenCalled()

    setHidden(false)
    await vi.advanceTimersByTimeAsync(0)
    expect(getPollBundleMock).toHaveBeenCalledTimes(1)
  })
})

describe('usePollBundle — 失败退避与旧值保留', () => {
  it('对账失败保留旧值；下次对账间隔不小于空闲下限（连续失败指数退避）', async () => {
    const poll = usePollBundle()
    poll.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)
    expect(poll.schedulers.value).toHaveLength(1)

    // 第 2 次对账失败：旧值保留、不转错误态
    getPollBundleMock.mockRejectedValue(new Error('network down'))
    await vi.advanceTimersByTimeAsync(30_000)
    await vi.advanceTimersByTimeAsync(0)
    expect(getPollBundleMock).toHaveBeenCalledTimes(2)
    expect(poll.schedulers.value).toHaveLength(1)
    expect(useState<number>('notifications:unread-count').value).toBe(7)

    // 退避：连续 1 次失败 → 下一次间隔 = 2×30s = 60s（≥ 空闲下限）
    await vi.advanceTimersByTimeAsync(30_000)
    expect(getPollBundleMock).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(getPollBundleMock).toHaveBeenCalledTimes(3)
  })

  it('失败后恢复成功：退避计数清零、回到空闲间隔', async () => {
    const poll = usePollBundle()
    poll.ensureStarted()
    await vi.advanceTimersByTimeAsync(0)

    getPollBundleMock.mockRejectedValueOnce(new Error('down'))
    await vi.advanceTimersByTimeAsync(30_000)
    // 第 3 次恢复成功（60s 退避后）
    await vi.advanceTimersByTimeAsync(60_000)
    expect(getPollBundleMock).toHaveBeenCalledTimes(3)
    expect(poll.schedulers.value).toHaveLength(1)

    // 恢复后回到空闲 30s 档
    await vi.advanceTimersByTimeAsync(30_000)
    expect(getPollBundleMock).toHaveBeenCalledTimes(4)
  })
})
