import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { LOADING_TIPS, useFakeProgress } from './useFakeProgress'

/**
 * 拟真进度组合式（loading-progress-tips specs）：
 * - ease-out 爬升、天花板 90%（不谎报完成）
 * - finish() 推进到 100%（含最小展示时长 400ms）
 * - 短句随机 + 4s 轮换（不与当前重复）
 * - dispose() 清理全部计时器（error 分支/组件卸载不泄漏）
 */

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('useFakeProgress - 拟真爬升', () => {
  it('启动后逐步爬升且始终低于天花板 90', () => {
    const fp = useFakeProgress()
    fp.start()
    expect(fp.progress.value).toBe(0)

    vi.advanceTimersByTime(1200)
    const after1s = fp.progress.value
    expect(after1s).toBeGreaterThan(0)
    expect(after1s).toBeLessThan(90)

    vi.advanceTimersByTime(60_000)
    // 长时间等待也只渐近 90，不越过（不谎报完成）
    expect(fp.progress.value).toBeLessThanOrEqual(90)
    expect(fp.progress.value).toBeGreaterThan(after1s)
    fp.dispose()
  })

  it('ease-out：等长时间窗内前快后慢', () => {
    const fp = useFakeProgress()
    fp.start()
    vi.advanceTimersByTime(600)
    const earlyDelta = fp.progress.value - 0
    vi.advanceTimersByTime(600)
    const lateDelta = fp.progress.value - earlyDelta
    // 同样 600ms，后一个窗口的增量必须更小（增速衰减）
    expect(lateDelta).toBeLessThan(earlyDelta)
    fp.dispose()
  })
})

describe('useFakeProgress - finish 收尾', () => {
  it('finish() 直接推进到 100 且标记完成', () => {
    const fp = useFakeProgress()
    fp.start()
    vi.advanceTimersByTime(2000)
    fp.finish()
    expect(fp.progress.value).toBe(100)
    expect(fp.finished.value).toBe(true)
  })

  it('finish() 在最小展示时长内时延迟到 400ms 才到 100（避免进度条闪跳）', () => {
    const fp = useFakeProgress()
    fp.start()
    fp.finish() // 立即 finish
    expect(fp.progress.value).not.toBe(100)
    vi.advanceTimersByTime(400)
    expect(fp.progress.value).toBe(100)
  })

  it('finish() 后爬升与短句轮换停止', () => {
    const fp = useFakeProgress()
    fp.start()
    vi.advanceTimersByTime(2000)
    fp.finish()
    const tipAtFinish = fp.tip.value
    vi.advanceTimersByTime(10_000)
    expect(fp.progress.value).toBe(100)
    expect(fp.tip.value).toBe(tipAtFinish)
  })
})

describe('useFakeProgress - 短句', () => {
  it('start() 即随机给出一条清单内短句', () => {
    const fp = useFakeProgress()
    expect(fp.tip.value).toBe('')
    fp.start()
    expect(LOADING_TIPS).toContain(fp.tip.value)
    fp.dispose()
  })

  it('加载超过 4s 后短句轮换且不与当前重复', () => {
    const fp = useFakeProgress()
    fp.start()
    const first = fp.tip.value
    vi.advanceTimersByTime(4100)
    expect(fp.tip.value).not.toBe(first)
    expect(LOADING_TIPS).toContain(fp.tip.value)
    fp.dispose()
  })
})

describe('useFakeProgress - 计时器清理', () => {
  it('dispose() 后进度与短句完全冻结', () => {
    const fp = useFakeProgress()
    fp.start()
    vi.advanceTimersByTime(1000)
    fp.dispose()
    const p = fp.progress.value
    const t = fp.tip.value
    vi.advanceTimersByTime(60_000)
    expect(fp.progress.value).toBe(p)
    expect(fp.tip.value).toBe(t)
  })

  it('未 start 直接 dispose / 重复 dispose 不抛错', () => {
    const fp = useFakeProgress()
    expect(() => fp.dispose()).not.toThrow()
    fp.start()
    expect(() => {
      fp.dispose()
      fp.dispose()
    }).not.toThrow()
  })
})
