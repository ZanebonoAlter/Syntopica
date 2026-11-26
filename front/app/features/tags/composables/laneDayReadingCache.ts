/**
 * 有界读取缓存（rework-workshop-into-radar-and-desk 矩阵阶段 G4，P4 裁决）。
 *
 * 简单 Map FIFO + 在途去重（禁通用缓存框架/新依赖）：
 *  - 成功项先入先出、超上限逐出最旧；重写同键时移到最新插入位。
 *  - `load(key, loader)`：同键在途请求合乘一个 Promise；只有 loader 正常返回才入
 *    成功缓存（抛错/undefined 不入、不逐出既有成功项）——失败响应不入成功缓存。
 *  - `force`：绕过成功缓存重读（仍在途时合乘同一请求）。
 *  - 失效代（`clear()` 递增）：clear 之前发出的请求即使晚到成功也不得回填缓存、不得删掉
 *    clear 之后同键新请求的在途槽；这类结果带 `stale=true`，调用方不得发布（跨板块/卸载晚到）。
 *  - 渲染稳定性（CA-8）由调用方「当前读对象独立持有」保证：本缓存只管 Map 有无，
 *    逐出后调用方持有的对象引用不受影响。
 */
export interface BoundedCacheLoadResult<T> {
  ok: boolean
  /** true = 命中成功缓存（未发请求）。 */
  fromCache: boolean
  /** true = 请求在 clear() 之后才回来：缓存已拒收，调用方不得发布成功或失败。 */
  stale?: boolean
  data?: T
  error?: string
}

export interface BoundedRequestCache<K, T> {
  load(key: K, options?: { force?: boolean }): Promise<BoundedCacheLoadResult<T>>
  peek(key: K): T | null
  has(key: K): boolean
  /** 强制写入成功项（外部已有成功数据时；重置插入位）。 */
  set(key: K, value: T): void
  size(): number
  keys(): K[]
  clear(): void
}

export function createBoundedRequestCache<K, T>(
  capacity: number,
  loader: (key: K) => Promise<T | undefined>,
): BoundedRequestCache<K, T> {
  const max = Math.max(1, Math.floor(capacity))
  const cache = new Map<K, T>()
  const inflight = new Map<K, { generation: number, promise: Promise<BoundedCacheLoadResult<T>> }>()
  /** 失效代：clear() 递增（区分 clear 前后发出的同键请求）。 */
  let generation = 0

  function set(key: K, value: T): void {
    if (cache.has(key)) cache.delete(key)
    cache.set(key, value)
    while (cache.size > max) {
      const oldest = cache.keys().next().value
      if (oldest === undefined) break
      cache.delete(oldest)
    }
  }

  async function load(key: K, options: { force?: boolean } = {}): Promise<BoundedCacheLoadResult<T>> {
    if (!options.force) {
      const cached = cache.get(key)
      if (cached !== undefined) return { ok: true, fromCache: true, data: cached }
    }
    const pending = inflight.get(key)
    if (pending !== undefined && pending.generation === generation) return pending.promise

    const requestGeneration = generation
    const entry: { generation: number, promise: Promise<BoundedCacheLoadResult<T>> } = {
      generation: requestGeneration,
      promise: Promise.resolve<BoundedCacheLoadResult<T>>({ ok: false, fromCache: false, stale: true }),
    }
    entry.promise = loader(key).then((data): BoundedCacheLoadResult<T> => {
      // 只删自己占的槽（clear 后的新请求槽不能被旧结算删掉）
      if (inflight.get(key) === entry) inflight.delete(key)
      if (requestGeneration !== generation) return { ok: false, fromCache: false, stale: true }
      if (data === undefined) return { ok: false, fromCache: false, error: '加载失败' }
      set(key, data)
      return { ok: true, fromCache: false, data }
    }).catch((error: unknown) => {
      if (inflight.get(key) === entry) inflight.delete(key)
      if (requestGeneration !== generation) return { ok: false, fromCache: false, stale: true }
      const message = error instanceof Error ? error.message : String(error)
      return { ok: false, fromCache: false, error: message }
    })
    inflight.set(key, entry)
    return entry.promise
  }

  return {
    load,
    peek: key => cache.get(key) ?? null,
    has: key => cache.has(key),
    set,
    size: () => cache.size,
    keys: () => [...cache.keys()],
    clear: () => {
      generation += 1
      cache.clear()
      inflight.clear()
    },
  }
}
