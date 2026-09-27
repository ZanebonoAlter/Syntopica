import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { RssFeed } from '~/types'

/**
 * useAutoRefresh 单调度器串行错峰（slim-article-list-payload C1/C4）。
 *
 * 背景：原实现给每个 feed 建独立 `setInterval`，页面挂载时毫秒级批量创建，
 * 生产实测同分钟 20–41 个 `GET /api/articles?per_page=10000` 并发。
 * 本测试锁死三条不变量：
 * 1. 同一分钟至多 1 个刷新（MIN_GAP_MS + 同周期错峰）；
 * 2. 刷新流程不再请求文章列表（负向护栏，mock `~/api/articles`）；
 * 3. 单个 feed 失败不终止整体调度。
 *
 * 时钟：fake timers 同时接管 `Date.now()`，`advanceTimersByTimeAsync` 会在
 * 每个定时器触发后让出微任务队列，串行 await 链可继续推进。
 */

const mocks = vi.hoisted(() => ({
  refreshFeed: vi.fn(),
  fetchFeeds: vi.fn(),
  getArticles: vi.fn(),
  feeds: [] as RssFeed[],
}))

vi.mock('~/api', () => ({
  useFeedsApi: () => ({ refreshFeed: mocks.refreshFeed }),
}))

// 负向护栏：刷新流程不得再触碰文章列表接口（原 `fetchArticles({ per_page: 10000 })` 已删除）
vi.mock('~/api/articles', () => ({
  useArticlesApi: () => ({ getArticles: mocks.getArticles }),
}))

import { useAutoRefresh, useGlobalAutoRefresh } from './useAutoRefresh'

function makeFeed(id: string, refreshInterval: number): RssFeed {
  return {
    id,
    title: `Feed ${id}`,
    description: '',
    url: `https://example.com/${id}.xml`,
    category: '',
    lastUpdated: '',
    articleCount: 0,
    unreadCount: 0,
    refreshInterval,
  }
}

describe('useAutoRefresh 单调度器', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00Z'))
    mocks.feeds.length = 0
    mocks.refreshFeed.mockReset()
    mocks.refreshFeed.mockResolvedValue({ success: true })
    mocks.fetchFeeds.mockReset()
    mocks.fetchFeeds.mockResolvedValue({ success: true })
    mocks.getArticles.mockReset()
    vi.stubGlobal('useApiStore', () => ({ feeds: mocks.feeds, fetchFeeds: mocks.fetchFeeds }))
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('单 feed 刷新完成：按 due 触发一次并回调当前页重取', async () => {
    const onFeedRefreshed = vi.fn()
    const auto = useAutoRefresh({ onFeedRefreshed })
    auto.setupAutoRefresh('1', 60)

    await vi.advanceTimersByTimeAsync(60 * 60 * 1000)

    expect(mocks.refreshFeed).toHaveBeenCalledTimes(1)
    expect(mocks.refreshFeed).toHaveBeenCalledWith(1)
    expect(onFeedRefreshed).toHaveBeenCalledTimes(1)
    expect(mocks.fetchFeeds).toHaveBeenCalledTimes(1)
  })

  it('不发起全量重拉：刷新流程不请求文章列表', async () => {
    const auto = useAutoRefresh()
    auto.setupAutoRefresh('1', 1)

    // 推进 3 个周期，确保调度器确实跑了多次刷新
    await vi.advanceTimersByTimeAsync(3 * 60 * 1000)

    expect(mocks.refreshFeed.mock.calls.length).toBeGreaterThanOrEqual(2)
    expect(mocks.getArticles).not.toHaveBeenCalled()
  })

  it('多 feed 同周期：同一分钟至多 1 个刷新', async () => {
    mocks.feeds.push(
      ...Array.from({ length: 23 }, (_, i) => makeFeed(String(i + 1), 60)),
    )

    const starts: number[] = []
    mocks.refreshFeed.mockImplementation(async () => {
      starts.push(Date.now())
      return { success: true }
    })

    const auto = useAutoRefresh()
    auto.initialize()

    await vi.advanceTimersByTimeAsync(60 * 60 * 1000)

    const perMinute = new Map<number, number>()
    for (const startedAt of starts) {
      const minute = Math.floor(startedAt / 60_000)
      perMinute.set(minute, (perMinute.get(minute) ?? 0) + 1)
    }

    // 确实都在跑（不是没调度），而不是靠“少触发”伪造不变量
    expect(starts.length).toBeGreaterThanOrEqual(20)
    for (const count of perMinute.values()) {
      expect(count).toBeLessThanOrEqual(1)
    }
  })

  it('刷新失败不终止调度', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    mocks.refreshFeed.mockRejectedValueOnce(new Error('boom'))

    const auto = useAutoRefresh()
    auto.setupAutoRefresh('1', 1)
    auto.setupAutoRefresh('2', 1)
    auto.setupAutoRefresh('3', 1)

    await vi.advanceTimersByTimeAsync(10 * 60 * 1000)

    const calledFeedIds = new Set(mocks.refreshFeed.mock.calls.map(([id]) => id))
    expect(calledFeedIds.size).toBeGreaterThanOrEqual(2)
    expect(calledFeedIds.has(2)).toBe(true)
    expect(calledFeedIds.has(3)).toBe(true)
    expect(consoleError).toHaveBeenCalled()
  })

  it('无 options 的 useGlobalAutoRefresh 调用不清掉已注册回调（useGlobalSettings 调用点）', async () => {
    const { mount } = await import('@vue/test-utils')
    const { defineComponent, h } = await import('vue')
    const onFeedRefreshed = vi.fn()
    mocks.feeds.push(makeFeed('9', 1))

    const Probe = defineComponent({
      setup() {
        useGlobalAutoRefresh({ onFeedRefreshed })
        // useGlobalSettings.updateFeedSetting 的调用形态：只取单例（用 updateFeedRefresh）、不传 options
        const auto = useGlobalAutoRefresh()
        auto.cleanup()
        auto.initialize()
        return () => h('div')
      },
    })
    const wrapper = mount(Probe)
    await vi.advanceTimersByTimeAsync(2 * 60 * 1000)

    expect(onFeedRefreshed).toHaveBeenCalled()
    wrapper.unmount()
  })
})
