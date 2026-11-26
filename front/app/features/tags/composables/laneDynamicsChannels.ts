/**
 * 泳道动态读取通道（rework-workshop-into-radar-and-desk 矩阵阶段 G1，P4 有界缓存裁决）。
 *
 * 纯 store（无 Vue 响应式，状态引用归 useBoardConsole/useObservationWindow）：
 *  - 成功缓存：每板块最多 4 档（7/14/30/365；365 档与池共用同一键，不产生第五项），
 *    简单 Map FIFO 逐出，禁通用缓存框架/新依赖。
 *  - 在途去重：同 (boardId, days) 并发 load 共享同一请求（含 force 刷新）。
 *  - 失败不入成功缓存、不逐出成功项；force 刷新失败后旧成功项仍在。
 *  - epoch 隔离：invalidateBoard/dispose 递增代数，切换前发出的迟到响应不回填缓存；
 *    in-flight 表同步清理，切回旧板块重新发起请求而非搭旧班车。
 */
import type { LaneDynamicsWindowPayload } from '~/api/laneDynamicsWindow'

export interface LaneChannelLoadOptions {
  /** true = 绕过成功缓存强制刷新（在途请求仍去重合乘）。 */
  force?: boolean
}

export interface LaneChannelLoadResult {
  ok: boolean
  /** true = 命中成功缓存，未发请求。 */
  fromCache: boolean
  /** true = 迟到响应（板块已切换/已释放），调用方应整体丢弃。 */
  discarded?: boolean
  payload?: LaneDynamicsWindowPayload
  error?: string
}

/** 取数函数（由 API façade 注入；返回 ApiResponse 形状的成功/失败联合）。 */
export type LaneChannelFetcher = (
  boardId: number,
  days: number,
) => Promise<{ success: boolean, data?: LaneDynamicsWindowPayload, error?: string }>

export interface LaneDynamicsChannelStore {
  load(boardId: number, days: number, options?: LaneChannelLoadOptions): Promise<LaneChannelLoadResult>
  peek(boardId: number, days: number): LaneDynamicsWindowPayload | null
  /** 板块缓存与在途全部失效（切板块）；迟到响应不回填。 */
  invalidateBoard(boardId: number): void
  /** 全量释放（组件卸载）。 */
  dispose(): void
  /** 诊断/测试：某板块当前成功缓存档数。 */
  cachedTierCount(boardId: number): number
}

const DEFAULT_MAX_TIERS = 4

export function createLaneDynamicsChannelStore(
  fetcher: LaneChannelFetcher,
  options: { maxTiersPerBoard?: number } = {},
): LaneDynamicsChannelStore {
  const maxTiers = Math.max(1, options.maxTiersPerBoard ?? DEFAULT_MAX_TIERS)
  const tierCache = new Map<number, Map<number, LaneDynamicsWindowPayload>>()
  const inflight = new Map<string, Promise<LaneChannelLoadResult>>()
  let generation = 0

  function cacheSet(boardId: number, days: number, payload: LaneDynamicsWindowPayload): void {
    let boardCache = tierCache.get(boardId)
    if (!boardCache) {
      boardCache = new Map()
      tierCache.set(boardId, boardCache)
    }
    // 刷新成功：重置插入位（FIFO 逐出按最新插入序）
    if (boardCache.has(days)) boardCache.delete(days)
    boardCache.set(days, payload)
    while (boardCache.size > maxTiers) {
      const oldest = boardCache.keys().next().value
      if (oldest === undefined) break
      boardCache.delete(oldest)
    }
  }

  async function load(boardId: number, days: number, options: LaneChannelLoadOptions = {}): Promise<LaneChannelLoadResult> {
    if (!options.force) {
      const cached = tierCache.get(boardId)?.get(days)
      if (cached) return { ok: true, fromCache: true, payload: cached }
    }
    const key = `${boardId}:${days}`
    const pending = inflight.get(key)
    if (pending) return pending

    const capturedGeneration = generation
    const request = fetcher(boardId, days).then((res): LaneChannelLoadResult => {
      inflight.delete(key)
      // 迟到响应（invalidate/dispose 已发生）：不回填缓存，交调用方按 discarded 丢弃
      if (capturedGeneration !== generation) {
        return { ok: false, fromCache: false, discarded: true, error: '请求已过期' }
      }
      if (res.success && res.data) {
        cacheSet(boardId, days, res.data)
        return { ok: true, fromCache: false, payload: res.data }
      }
      return { ok: false, fromCache: false, error: res.error || '加载泳道动态失败' }
    })
    inflight.set(key, request)
    return request
  }

  function peek(boardId: number, days: number): LaneDynamicsWindowPayload | null {
    return tierCache.get(boardId)?.get(days) ?? null
  }

  function dropInflight(boardId: number): void {
    const prefix = `${boardId}:`
    for (const key of [...inflight.keys()]) {
      if (key.startsWith(prefix)) inflight.delete(key)
    }
  }

  function invalidateBoard(boardId: number): void {
    tierCache.delete(boardId)
    dropInflight(boardId)
    generation += 1
  }

  function dispose(): void {
    tierCache.clear()
    inflight.clear()
    generation += 1
  }

  return {
    load,
    peek,
    invalidateBoard,
    dispose,
    cachedTierCount: (boardId: number) => tierCache.get(boardId)?.size ?? 0,
  }
}
