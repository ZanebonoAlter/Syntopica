/**
 * useConsoleRefresh 测试（8C G4a：刷新后回调）。
 *
 * 契约：元信息 → A → `after`；同一防重窗口内不并发重复；`after` 失败不改主刷新结果，
 * `refreshing` 一定回落（不让按钮永久 busy）。
 */
import { describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { ref } from 'vue'
import { useConsoleRefresh } from './useConsoleRefresh'

function deferred<T = void>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

describe('useConsoleRefresh — 刷新链', () => {
  it('顺序固定：元信息 → A → after（after 在防重窗口内）', async () => {
    const calls: string[] = []
    const meta = deferred()
    const source = deferred()
    const store = useConsoleRefresh({
      loadReportMeta: async () => { calls.push('meta'); await meta.promise },
      loadSource: async () => { calls.push('source'); await source.promise },
      reportMetaLoading: ref(false),
      after: async () => { calls.push('after') },
    })

    const running = store.refresh()
    expect(calls).toEqual(['meta'])
    expect(store.refreshing.value).toBe(true)

    // after 不会抢跑：A 未完成前不执行
    meta.resolve()
    await flushPromises()
    expect(calls).toEqual(['meta', 'source'])
    expect(calls).not.toContain('after')

    source.resolve()
    await running
    expect(calls).toEqual(['meta', 'source', 'after'])
    expect(store.refreshing.value).toBe(false)
  })

  it('防重：刷新中再次触发直接返回，after 只跑一次', async () => {
    const gate = deferred()
    const after = vi.fn(async () => {})
    const store = useConsoleRefresh({
      loadReportMeta: async () => { await gate.promise },
      loadSource: async () => {},
      reportMetaLoading: ref(false),
      after,
    })

    const first = store.refresh()
    const second = store.refresh()
    gate.resolve()
    await Promise.all([first, second])
    expect(after).toHaveBeenCalledTimes(1)
    expect(store.refreshing.value).toBe(false)
  })

  it('after 失败不改主刷新结果，refreshing 回落', async () => {
    const loadReportMeta = vi.fn(async () => {})
    const loadSource = vi.fn(async () => {})
    const store = useConsoleRefresh({
      loadReportMeta,
      loadSource,
      reportMetaLoading: ref(false),
      after: async () => { throw new Error('索引刷新失败') },
    })

    await expect(store.refresh()).resolves.toBeUndefined()
    expect(loadReportMeta).toHaveBeenCalledTimes(1)
    expect(loadSource).toHaveBeenCalledTimes(1)
    expect(store.refreshing.value).toBe(false)
  })

  it('未传 after 保持旧行为（仅元信息 + A）', async () => {
    const loadReportMeta = vi.fn(async () => {})
    const loadSource = vi.fn(async () => {})
    const reportMetaLoading = ref(false)
    const store = useConsoleRefresh({ loadReportMeta, loadSource, reportMetaLoading })

    expect(store.busy.value).toBe(false)
    reportMetaLoading.value = true
    expect(store.busy.value).toBe(true)
    reportMetaLoading.value = false

    await store.refresh()
    expect(loadReportMeta).toHaveBeenCalledTimes(1)
    expect(loadSource).toHaveBeenCalledTimes(1)
    expect(store.busy.value).toBe(false)
  })
})
