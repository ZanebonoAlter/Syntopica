import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { NAV_LOADING_DELAY_MS, useNavLoading } from './useNavLoading'

/**
 * 路由切换加载反馈状态机（fix-spa-nav-loading-ux specs「路由切换加载反馈」）：
 * - 250ms 边界两端：249ms 内完成不显示 / 超时未完成显示（fake timers 显式边界）
 * - end 清理：导航完成（afterEach）与失败（onError）路径均卸载反馈
 * - 连续 begin 重置计时（新导航取代旧反馈）；visible 态再 begin 立即回 pending
 * - end 幂等：重复调用无副作用
 */

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('useNavLoading - 250ms 延迟触发', () => {
  it('begin 后 249ms 内不显示，251ms 超时显示', () => {
    const nav = useNavLoading()
    expect(nav.visible.value).toBe(false)

    nav.begin()
    vi.advanceTimersByTime(NAV_LOADING_DELAY_MS - 1)
    expect(nav.visible.value).toBe(false)

    vi.advanceTimersByTime(2)
    expect(nav.visible.value).toBe(true)
  })

  it('250ms 内 end 则全程不显示（快导航零打扰）', () => {
    const nav = useNavLoading()
    nav.begin()
    vi.advanceTimersByTime(100)
    nav.end()
    expect(nav.visible.value).toBe(false)

    vi.advanceTimersByTime(10_000)
    expect(nav.visible.value).toBe(false)
  })
})

describe('useNavLoading - 导航完成或失败卸载反馈', () => {
  it('可见中 end（afterEach 成功路径）立即卸载且计时器不再触发', () => {
    const nav = useNavLoading()
    nav.begin()
    vi.advanceTimersByTime(NAV_LOADING_DELAY_MS + 50)
    expect(nav.visible.value).toBe(true)

    nav.end()
    expect(nav.visible.value).toBe(false)
    vi.advanceTimersByTime(10_000)
    expect(nav.visible.value).toBe(false)
  })

  it('onError 路径：可见中直接 end 卸载反馈', () => {
    const nav = useNavLoading()
    nav.begin()
    vi.advanceTimersByTime(300)
    expect(nav.visible.value).toBe(true)

    // 导航失败由 plugins/nav-loading.ts 的 router.onError 调用同一个 end()
    nav.end()
    expect(nav.visible.value).toBe(false)
  })
})

describe('useNavLoading - 连续导航重置', () => {
  it('连续 begin 重置计时：旧计时器到点不再触发', () => {
    const nav = useNavLoading()
    nav.begin()
    vi.advanceTimersByTime(200)

    nav.begin()
    // 若旧计时器未清，此刻已过原 250ms 界限
    vi.advanceTimersByTime(NAV_LOADING_DELAY_MS - 1)
    expect(nav.visible.value).toBe(false)

    vi.advanceTimersByTime(2)
    expect(nav.visible.value).toBe(true)
  })

  it('visible 态下再 begin 立即回 pending 并重新计时（不残留旧反馈）', () => {
    const nav = useNavLoading()
    nav.begin()
    vi.advanceTimersByTime(300)
    expect(nav.visible.value).toBe(true)

    nav.begin()
    expect(nav.visible.value).toBe(false)
    vi.advanceTimersByTime(NAV_LOADING_DELAY_MS - 1)
    expect(nav.visible.value).toBe(false)

    vi.advanceTimersByTime(2)
    expect(nav.visible.value).toBe(true)
  })
})

describe('useNavLoading - 幂等', () => {
  it('未 begin 直接 end / 重复 end 不抛错且保持 idle', () => {
    const nav = useNavLoading()
    expect(() => {
      nav.end()
      nav.end()
    }).not.toThrow()
    expect(nav.visible.value).toBe(false)
  })

  it('visible 态下重复 end 仍为 idle', () => {
    const nav = useNavLoading()
    nav.begin()
    vi.advanceTimersByTime(300)
    nav.end()
    nav.end()
    expect(nav.visible.value).toBe(false)
  })
})
