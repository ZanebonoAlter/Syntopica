/**
 * 泳道动态通道 store 测试（矩阵阶段 G1，test-cases-matrix-stage.md CA 组 store 层）。
 *
 *  - CA-1 空缓存按需读取 / CA-2 四档与 365 去重（在途合乘一次请求）
 *  - CA-6 失败不入成功缓存、不逐出成功项、重试确实再发
 *  - CA-7 切板块释放 + 迟到响应不回填；dispose 全量释放
 *  - FIFO 上限 4 档；force 刷新成功更新缓存、失败保留旧成功项
 */
import { describe, expect, it, vi } from 'vitest'
import type { LaneDynamicsWindowPayload } from '~/api/laneDynamicsWindow'
import { createLaneDynamicsChannelStore, type LaneChannelFetcher } from './laneDynamicsChannels'

function makePayload(): LaneDynamicsWindowPayload {
  return {
    window: {
      windowDays: 14,
      hasReports: true,
      lanes: [],
      candidates: [],
      anchorDate: '2026-10-23',
      otherTopics: {},
      otherTopicsAvailable: false,
    },
    legacy: { window_days: 14, has_reports: true, lanes: [], candidates: [] },
  }
}

/** 立即成功的 fetcher（vi.fn，可断言调用次数）。 */
function okFetch() {
  return vi.fn(async () => ({ success: true, data: makePayload() }))
}

/** 受控 gate fetcher：调用后挂起，release() 才成功返回。 */
function gatedFetch() {
  let releaseRequest: () => void = () => {}
  const spy = vi.fn(() => {
    const pending = new Promise<{ success: boolean, data?: LaneDynamicsWindowPayload }>((resolve) => {
      releaseRequest = () => resolve({ success: true, data: makePayload() })
    })
    return pending
  })
  return {
    spy,
    fetcher: spy as unknown as LaneChannelFetcher,
    release: () => releaseRequest(),
  }
}

describe('createLaneDynamicsChannelStore — 缓存与去重', () => {
  it('CA-1 空缓存按需读取：load 发请求并缓存；fromCache 命中不再请求', async () => {
    const fetcher = okFetch()
    const store = createLaneDynamicsChannelStore(fetcher as unknown as LaneChannelFetcher)
    const first = await store.load(1974, 14)
    expect(first.ok).toBe(true)
    expect(first.fromCache).toBe(false)
    const second = await store.load(1974, 14)
    expect(second.fromCache).toBe(true)
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(store.cachedTierCount(1974)).toBe(1)
  })

  it('CA-2 在途去重：并发 load 同 (board, days) 只发一次请求（含 force 合乘）', async () => {
    const { fetcher, release, spy } = gatedFetch()
    const store = createLaneDynamicsChannelStore(fetcher)
    const a = store.load(1974, 365)
    const b = store.load(1974, 365, { force: true })
    const c = store.load(1974, 365)
    release()
    const [ra, rb, rc] = await Promise.all([a, b, c])
    expect(spy).toHaveBeenCalledTimes(1)
    expect(ra.ok && rb.ok && rc.ok).toBe(true)
    expect(store.cachedTierCount(1974)).toBe(1)
  })

  it('CA-2 四档上限：7/14/30/365 各一成功项，365 档/池共用不产生第五项', async () => {
    const fetcher = okFetch()
    const store = createLaneDynamicsChannelStore(fetcher as unknown as LaneChannelFetcher)
    for (const days of [7, 14, 30, 365]) {
      await store.load(1974, days)
    }
    expect(store.cachedTierCount(1974)).toBe(4)
    // 池通道再次读 365（非 force）→ 命中同一成功项
    const poolHit = await store.load(1974, 365)
    expect(poolHit.fromCache).toBe(true)
    expect(fetcher).toHaveBeenCalledTimes(4)
    expect(store.cachedTierCount(1974)).toBe(4)
  })

  it('FIFO 超上限逐出最先入项；被逐出项再读允许重新 GET', async () => {
    const fetcher = okFetch()
    const store = createLaneDynamicsChannelStore(fetcher as unknown as LaneChannelFetcher)
    for (const days of [1, 2, 3, 4, 5]) {
      await store.load(1974, days)
    }
    expect(store.cachedTierCount(1974)).toBe(4)
    expect(store.peek(1974, 1)).toBeNull()
    expect(store.peek(1974, 5)).not.toBeNull()
    const again = await store.load(1974, 1)
    expect(again.fromCache).toBe(false)
  })

  it('板块隔离：A/B 板块同 days 各自独立缓存', async () => {
    const fetcher = okFetch()
    const store = createLaneDynamicsChannelStore(fetcher as unknown as LaneChannelFetcher)
    await store.load(1974, 14)
    await store.load(1975, 14)
    expect(store.cachedTierCount(1974)).toBe(1)
    expect(store.cachedTierCount(1975)).toBe(1)
  })
})

describe('createLaneDynamicsChannelStore — 失败与生命周期', () => {
  it('CA-6 失败不入成功缓存：peek null；不逐出既有成功项；重试确实再发', async () => {
    let fail = false
    const fetcher = vi.fn(async () =>
      fail
        ? { success: false, error: '网络错误' }
        : { success: true, data: makePayload() },
    )
    const store = createLaneDynamicsChannelStore(fetcher as unknown as LaneChannelFetcher)
    await store.load(1974, 14)
    await store.load(1974, 30)
    fail = true
    const failed = await store.load(1974, 7)
    expect(failed.ok).toBe(false)
    expect(failed.error).toBe('网络错误')
    expect(store.peek(1974, 7)).toBeNull()
    expect(store.peek(1974, 14)).not.toBeNull()
    // force 刷新失败：旧成功项保留（失败不覆盖成功缓存）
    const staleRefresh = await store.load(1974, 14, { force: true })
    expect(staleRefresh.ok).toBe(false)
    expect(store.peek(1974, 14)).not.toBeNull()
    // 重试确实再发
    fail = false
    const retried = await store.load(1974, 7)
    expect(retried.ok).toBe(true)
    expect(retried.fromCache).toBe(false)
  })

  it('CA-7 切板块释放：旧板块缓存/in-flight 清空；迟到响应不回填、标 discarded', async () => {
    const { fetcher, release, spy } = gatedFetch()
    const store = createLaneDynamicsChannelStore(fetcher)
    const inFlight = store.load(1974, 365)
    store.invalidateBoard(1974)
    release()
    const late = await inFlight
    expect(late.ok).toBe(false)
    expect(late.discarded).toBe(true)
    expect(store.peek(1974, 365)).toBeNull()
    // 切回旧板块：in-flight 已清理，重新发请求而非合乘旧班车
    const again = store.load(1974, 365)
    expect(spy).toHaveBeenCalledTimes(2)
    release()
    const result = await again
    expect(result.ok).toBe(true)
  })

  it('dispose 全量释放；dispose 后迟到响应 discarded', async () => {
    const { fetcher, release } = gatedFetch()
    const store = createLaneDynamicsChannelStore(fetcher)
    const inFlight = store.load(1975, 14)
    store.dispose()
    release()
    const late = await inFlight
    expect(late.discarded).toBe(true)
    expect(store.cachedTierCount(1975)).toBe(0)
  })

  it('force 刷新成功更新缓存值', async () => {
    const fetcher = okFetch()
    const store = createLaneDynamicsChannelStore(fetcher as unknown as LaneChannelFetcher)
    await store.load(1974, 14)
    const refreshed = await store.load(1974, 14, { force: true })
    expect(refreshed.fromCache).toBe(false)
    expect(store.peek(1974, 14)).toBe(refreshed.payload)
  })
})
