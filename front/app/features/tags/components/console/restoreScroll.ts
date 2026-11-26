/**
 * 返回上下文滚动恢复（矩阵阶段 G5，R34）。
 *
 * 真实浏览器会把 scrollTop/scrollY 钳制到当前内容高度；编年异步增高后单次
 * nextTick 恢复会被截断（1440 382→302、1280 693→414）。本 helper 立即应用一次，
 * 再在有限帧内重试，直到达到目标、内容终态仍不足、用户取消或达到上限；
 * 不是固定 sleep/无限 observer，也不造假占位高度。
 */
export interface ScrollDriver {
  getTop: () => number
  /** 当前宿主允许的最大滚动值（scrollHeight-clientHeight / docHeight-innerHeight）。 */
  getMax: () => number
  setTop: (top: number) => void
}

export interface RestoreScrollOptions {
  driver: ScrollDriver
  target: number
  isCancelled?: () => boolean
  /** 有限重试上限；默认约 3 秒（60fps）。 */
  maxAttempts?: number
  /** 测试注入；默认 requestAnimationFrame，无则 16ms 定时器。 */
  schedule?: (callback: () => void) => unknown
  cancelSchedule?: (handle: unknown) => void
  /** 自然完成（达标或有界耗尽）后回调：调用方据此摘除用户监听，避免泄漏。 */
  onDone?: () => void
}

function defaultSchedule(callback: () => void): unknown {
  if (typeof requestAnimationFrame === 'function') return requestAnimationFrame(() => callback())
  return setTimeout(callback, 16)
}

function defaultCancel(handle: unknown): void {
  if (typeof cancelAnimationFrame === 'function' && typeof handle === 'number') {
    cancelAnimationFrame(handle)
    return
  }
  clearTimeout(handle as ReturnType<typeof setTimeout>)
}

/** 开始恢复；返回 cancel（用户滚动/切上下文/卸载时调用）。 */
export function restoreScrollWhenReady(options: RestoreScrollOptions): () => void {
  const schedule = options.schedule ?? defaultSchedule
  const cancelSchedule = options.cancelSchedule ?? defaultCancel
  const maxAttempts = options.maxAttempts ?? 180
  let attempts = 0
  let cancelled = false
  let finished = false
  let pending: unknown = null

  const stop = (): void => {
    if (pending !== null) {
      cancelSchedule(pending)
      pending = null
    }
  }

  const finish = (): void => {
    if (finished) return
    finished = true
    stop()
    options.onDone?.()
  }

  const run = (): void => {
    if (finished) return
    if (cancelled || options.isCancelled?.()) {
      finish()
      return
    }
    const max = Math.max(0, options.driver.getMax())
    const desired = Math.min(options.target, max)
    options.driver.setTop(desired)
    attempts += 1
    if (options.driver.getTop() >= options.target - 1 || attempts >= maxAttempts) {
      finish()
      return
    }
    pending = schedule(run)
  }

  run()

  return () => {
    cancelled = true
    finish()
  }
}
