/**
 * R34 负向：浏览器 clamp 语义 + 延迟内容高度。
 * 普通可随意赋值的 happy-dom scrollTop 会掩盖 382→302 的真实问题，这里用
 * 会钳制的假宿主构造红例，验证有限重试/取消/耗尽。
 */
import { describe, expect, it, vi } from 'vitest'
import { restoreScrollWhenReady, type ScrollDriver } from './restoreScroll'

function clampedHost(initialMax: number, initialTop = 0): { driver: ScrollDriver, grow: (max: number) => void, top: () => number, attempts: () => number } {
  let max = initialMax
  let value = initialTop
  let attempts = 0
  return {
    driver: {
      getTop: () => value,
      getMax: () => max,
      setTop: (top: number) => {
        attempts += 1
        value = Math.max(0, Math.min(top, max))
      },
    },
    grow: (nextMax: number) => { max = nextMax },
    top: () => value,
    attempts: () => attempts,
  }
}

function manualSchedule() {
  const queue: Array<() => void> = []
  return {
    schedule: (callback: () => void) => { queue.push(callback); return queue.length - 1 },
    cancelSchedule: () => {},
    runNext: () => queue.shift()?.(),
  }
}

describe('restoreScrollWhenReady — R34 深滚动', () => {
  it('初期 max=302 被钳制，编年增高到 382 后仍恢复到 382', () => {
    const host = clampedHost(302)
    const clock = manualSchedule()
    const cancel = restoreScrollWhenReady({
      driver: host.driver,
      target: 382,
      maxAttempts: 10,
      schedule: clock.schedule,
      cancelSchedule: clock.cancelSchedule,
    })
    expect(host.top()).toBe(302)
    host.grow(382)
    clock.runNext()
    expect(host.top()).toBe(382)
    cancel()
  })

  it('用户取消后不再拉回（已应用的值保持，后续增高不抢）', () => {
    const host = clampedHost(302)
    const clock = manualSchedule()
    const cancel = restoreScrollWhenReady({
      driver: host.driver,
      target: 382,
      maxAttempts: 10,
      schedule: clock.schedule,
      cancelSchedule: clock.cancelSchedule,
    })
    expect(host.top()).toBe(302)
    cancel()
    host.grow(382)
    clock.runNext()
    expect(host.top()).toBe(302)
  })

  it('内容终态仍短：有界耗尽停止，不永久 pending', () => {
    const host = clampedHost(302)
    const clock = manualSchedule()
    restoreScrollWhenReady({
      driver: host.driver,
      target: 382,
      maxAttempts: 3,
      schedule: clock.schedule,
      cancelSchedule: clock.cancelSchedule,
    })
    expect(host.top()).toBe(302)
    clock.runNext()
    clock.runNext()
    expect(host.attempts()).toBe(3)
    expect(clock.runNext()).toBeUndefined()
  })

  it('自然完成/有界耗尽都只回调一次 onDone（调用方据此摘监听）', () => {
    const done = vi.fn()
    const host = clampedHost(382)
    restoreScrollWhenReady({
      driver: host.driver,
      target: 382,
      maxAttempts: 3,
      onDone: done,
    })
    expect(done).toHaveBeenCalledTimes(1)

    const shortDone = vi.fn()
    const short = clampedHost(302)
    const clock = manualSchedule()
    restoreScrollWhenReady({
      driver: short.driver,
      target: 382,
      maxAttempts: 3,
      schedule: clock.schedule,
      cancelSchedule: clock.cancelSchedule,
      onDone: shortDone,
    })
    clock.runNext()
    clock.runNext()
    expect(shortDone).toHaveBeenCalledTimes(1)
  })

  it('上下文已变化时取消，不套用旧位置', () => {
    const host = clampedHost(302)
    const clock = manualSchedule()
    let stale = false
    restoreScrollWhenReady({
      driver: host.driver,
      target: 382,
      maxAttempts: 10,
      isCancelled: () => stale,
      schedule: clock.schedule,
      cancelSchedule: clock.cancelSchedule,
    })
    expect(host.top()).toBe(302)
    stale = true
    host.grow(382)
    clock.runNext()
    expect(host.top()).toBe(302)
  })
})
