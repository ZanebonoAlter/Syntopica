/**
 * laneDayReadingCache 测试（矩阵阶段 G4，P4 有界缓存裁决）。
 * 覆盖：FIFO 逐出与插入位刷新、在途合乘、失败不入成功缓存/不逐出成功项、force、clear。
 * R2（父复核 2026-10-29）：clear 之后的晚到请求不得回填缓存、不得删新在途槽、不得发布。
 */
import { describe, expect, it, vi } from 'vitest'
import { createBoundedRequestCache } from './laneDayReadingCache'

function ok<T>(value: T): Promise<T> {
  return Promise.resolve(value)
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

describe('createBoundedRequestCache', () => {
  it('FIFO：超上限逐出最旧；命中缓存不再发请求', async () => {
    const loader = vi.fn((key: string) => ok(`${key}-data`))
    const cache = createBoundedRequestCache<string, string>(2, loader)
    await cache.load('a')
    await cache.load('b')
    await cache.load('c')
    expect(cache.size()).toBe(2)
    expect(cache.has('a')).toBe(false)
    expect(cache.has('b')).toBe(true)
    expect(cache.has('c')).toBe(true)
    // b 命中缓存（fromCache），a 被逐出后重读允许再请求
    const hit = await cache.load('b')
    expect(hit.fromCache).toBe(true)
    expect(loader).toHaveBeenCalledTimes(3)
    await cache.load('a')
    expect(loader).toHaveBeenCalledTimes(4)
  })

  it('重写同键刷新插入位（FIFO 按最新插入序逐出）', async () => {
    const loader = vi.fn((key: string) => ok(`${key}-v`))
    const cache = createBoundedRequestCache<string, string>(2, loader)
    await cache.load('a')
    await cache.load('b')
    cache.set('a', 'a-v2')
    await cache.load('c')
    expect(cache.has('a')).toBe(true)
    expect(cache.has('b')).toBe(false)
  })

  it('在途合乘：同键并发 load 只发一次请求', async () => {
    let release!: (value: string) => void
    const loader = vi.fn(() => new Promise<string>(resolve => { release = resolve }))
    const cache = createBoundedRequestCache<string, string>(3, loader)
    const first = cache.load('x')
    const second = cache.load('x')
    release('v')
    expect(await first).toEqual({ ok: true, fromCache: false, data: 'v' })
    expect(await second).toEqual({ ok: true, fromCache: false, data: 'v' })
    expect(loader).toHaveBeenCalledTimes(1)
  })

  it('失败不入成功缓存、不逐出成功项；重试确实再发', async () => {
    let fail = true
    const loader = vi.fn((key: string) => (fail && key === 'bad' ? Promise.reject(new Error('boom')) : ok(`${key}-data`)))
    const cache = createBoundedRequestCache<string, string>(1, loader)
    await cache.load('good')
    const bad = await cache.load('bad')
    expect(bad.ok).toBe(false)
    expect(bad.error).toBe('boom')
    expect(cache.size()).toBe(1)
    expect(cache.has('good')).toBe(true)
    fail = false
    const retried = await cache.load('bad')
    expect(retried.ok).toBe(true)
    expect(loader).toHaveBeenCalledTimes(3)
  })

  it('loader 返回 undefined 视为失败（不入缓存）', async () => {
    const loader = vi.fn(() => ok(undefined))
    const cache = createBoundedRequestCache<string, string>(2, loader)
    const res = await cache.load('x')
    expect(res.ok).toBe(false)
    expect(cache.size()).toBe(0)
  })

  it('force 绕过成功缓存重读（在途仍合乘）', async () => {
    const loader = vi.fn((key: string) => ok(`${key}-${loader.mock.calls.length}`))
    const cache = createBoundedRequestCache<string, string>(3, loader)
    await cache.load('x')
    const forced = await cache.load('x', { force: true })
    expect(forced.fromCache).toBe(false)
    expect(loader).toHaveBeenCalledTimes(2)
  })

  it('clear 释放成功项与在途表', () => {
    const cache = createBoundedRequestCache<string, string>(3, async key => `${key}-v`)
    cache.set('a', 'a-v')
    cache.clear()
    expect(cache.size()).toBe(0)
    expect(cache.peek('a')).toBeNull()
  })
})

// ── 父复核 R2：clear 后旧请求晚到（缓存代失效，2026-10-29） ───────────────────────

describe('createBoundedRequestCache — clear 后旧请求晚到', () => {
  it('clear 后旧成功晚到：不回填成功缓存，结果标 stale', async () => {
    const gate = deferred<string>()
    const loader = vi.fn(() => gate.promise)
    const cache = createBoundedRequestCache<string, string>(3, loader)
    const pending = cache.load('k')
    cache.clear()
    gate.resolve('old')
    const res = await pending
    expect(res.ok).toBe(false)
    expect(res.stale).toBe(true)
    expect(cache.size()).toBe(0)
    expect(cache.has('k')).toBe(false)
  })

  it('clear→同键新请求在途→旧成功晚到：不删新在途槽、不发布旧 payload，第三个 load 合乘新请求', async () => {
    const oldGate = deferred<string>()
    const newGate = deferred<string>()
    const loader = vi.fn()
      .mockReturnValueOnce(oldGate.promise)
      .mockReturnValueOnce(newGate.promise)
    const cache = createBoundedRequestCache<string, string>(3, loader)
    const oldRead = cache.load('k')
    cache.clear()
    const newRead = cache.load('k')
    oldGate.resolve('old')
    await oldRead
    // 旧 payload 不得回填
    expect(cache.has('k')).toBe(false)
    // 新请求的在途槽仍在：第三次 load 合乘，不另发请求
    const third = cache.load('k')
    expect(loader).toHaveBeenCalledTimes(2)
    newGate.resolve('new')
    expect(await newRead).toEqual({ ok: true, fromCache: false, data: 'new' })
    expect(await third).toEqual({ ok: true, fromCache: false, data: 'new' })
    expect(cache.peek('k')).toBe('new')
    expect(loader).toHaveBeenCalledTimes(2)
  })

  it('clear 后新请求先成功、旧成功晚到：新内容/缓存/在途槽不变', async () => {
    const oldGate = deferred<string>()
    const newGate = deferred<string>()
    const loader = vi.fn()
      .mockReturnValueOnce(oldGate.promise)
      .mockReturnValueOnce(newGate.promise)
    const cache = createBoundedRequestCache<string, string>(3, loader)
    const oldRead = cache.load('k')
    cache.clear()
    const newRead = cache.load('k')
    newGate.resolve('new')
    await newRead
    expect(cache.peek('k')).toBe('new')
    oldGate.resolve('old')
    const stale = await oldRead
    expect(stale.stale).toBe(true)
    expect(cache.peek('k')).toBe('new')
    expect(cache.size()).toBe(1)
    // 第三次直接命中新成功缓存，不再发请求
    expect(await cache.load('k')).toEqual({ ok: true, fromCache: true, data: 'new' })
    expect(loader).toHaveBeenCalledTimes(2)
  })

  it('旧请求 reject 晚到：不删新在途槽、不污染缓存', async () => {
    const oldGate = deferred<string>()
    const newGate = deferred<string>()
    const loader = vi.fn()
      .mockReturnValueOnce(oldGate.promise)
      .mockReturnValueOnce(newGate.promise)
    const cache = createBoundedRequestCache<string, string>(3, loader)
    const oldRead = cache.load('k')
    cache.clear()
    const newRead = cache.load('k')
    oldGate.reject(new Error('old fail'))
    const stale = await oldRead
    expect(stale.ok).toBe(false)
    expect(stale.stale).toBe(true)
    // 新请求槽未被旧结算删掉：第三个 load 仍合乘
    const third = cache.load('k')
    expect(loader).toHaveBeenCalledTimes(2)
    newGate.resolve('new')
    expect(await newRead).toEqual({ ok: true, fromCache: false, data: 'new' })
    expect(await third).toEqual({ ok: true, fromCache: false, data: 'new' })
    expect(cache.peek('k')).toBe('new')
  })
})
