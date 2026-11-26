/**
 * 返回上下文滚动恢复控制器（矩阵阶段 G5 R34；8C G4b 从 `useConsoleReturnContext` 内聚拆出）。
 *
 * 只负责「回到矩阵后把阅读位置放回去」这一件事：
 *  - 实际滚动宿主**每次动态判定**：stage 有可滚高度时用 stage，否则整页（<1440/手机
 *    overflow visible）；不在 body 上滚、不加固定 sleep。
 *  - `pending` 建立即挂用户意图（wheel/touch/pointer/keydown）取消监听；等待阅读终态期间
 *    **不消耗任何布局帧**，终态命中当前 reading key + date 后才开始 `restoreScrollWhenReady`
 *    的有限帧重试（内容异步增高时逐帧追赶，达标/上限/取消即停并摘监听）。
 *  - 上下文身位比对：lane 实体 + 阅读 key（lane 原 key 或 `other:<sectionId>`）+ date + 档
 *    + 范围，任一变化即放弃（旧异步不串到新上下文）。
 *  - 同一 scope 内只保留一份 pending；`stop()` 幂等。
 *
 * 不拥有：返回载体（`consoleReturnContext`）、阅读终态的发布（组件）、选中状态。
 */
import { getCurrentScope, onScopeDispose, watch, type ComputedRef, type Ref } from 'vue'
import type { ObservationRange } from './observationRange'
import { restoreScrollWhenReady, type ScrollDriver } from './restoreScroll'

/** 阅读上下文终态通知（仅同一阅读 key/date 才可用于滚动兑现）。 */
export interface ReadingSettledState {
  laneKey: string
  date: string
  status: string
}

export interface ReturnScrollDeps {
  stageRef: () => HTMLElement | null
  /** 泳道实体（身位比对用；其他分区不写这里）。 */
  selectedLaneKey: ComputedRef<string | null>
  /** 阅读 key（lane 原 key 或 `other:<sectionId>`），决定性滚动兑现门。 */
  currentReadingKey: ComputedRef<string | null>
  selectedDate: Ref<string | null>
  sourceDays: Ref<number>
  observation: Ref<ObservationRange | null>
  /** 当前编年是否已就绪/终态（pending 时不耗布局帧预算）。 */
  readingSettled: Ref<ReadingSettledState | null>
}

export interface ReturnScrollController {
  /** 当前实际滚动位置（stage.scrollTop 或 window.scrollY）。 */
  readScrollTop: () => number
  /** 建立 pending 并尝试开始恢复（等待阅读终态命中）。 */
  restoreScroll: (target: number) => void
  /** 放弃当前 pending/进行中的恢复（幂等）。 */
  stopRestore: () => void
}

export function createReturnScrollController(deps: ReturnScrollDeps): ReturnScrollController {
  /** 实际滚动宿主：stage 有可滚高度时用 stage；否则整页（<1440/手机overflow visible）。 */
  function scrollDriver(): ScrollDriver | null {
    if (typeof window === 'undefined' && !deps.stageRef()) return null
    // 每次重试动态判定唯一宿主：返回时 body 可能尚未 mount；桌面 stage 也可能在内容增高后才可滚。
    const currentStage = (): HTMLElement | null => {
      const stage = deps.stageRef()
      return stage && stage.scrollHeight > stage.clientHeight + 1 ? stage : null
    }
    return {
      getTop: () => {
        const stage = currentStage()
        return stage ? stage.scrollTop : (typeof window !== 'undefined' ? window.scrollY : 0)
      },
      getMax: () => {
        const stage = currentStage()
        if (stage) return stage.scrollHeight - stage.clientHeight
        if (typeof window === 'undefined') return 0
        const doc = document.documentElement
        const body = document.body
        return Math.max(doc.scrollHeight, body?.scrollHeight ?? 0) - window.innerHeight
      },
      setTop: (top) => {
        const stage = currentStage()
        if (stage) {
          stage.scrollTop = top
          return
        }
        if (typeof window !== 'undefined') window.scrollTo(0, top)
      },
    }
  }

  function hasScrollableStage(): boolean {
    const stage = deps.stageRef()
    return !!stage && stage.scrollHeight > stage.clientHeight + 1
  }

  function readScrollTop(): number {
    const stage = hasScrollableStage() ? deps.stageRef() : null
    if (stage) return stage.scrollTop
    return typeof window !== 'undefined' ? window.scrollY : 0
  }

  interface PendingScroll {
    target: number
    /** 泳道实体（用于上下文身位比对；其他分区不写这里）。 */
    lane: string | null
    /** 阅读 key（lane 原 key 或 `other:<sectionId>`），决定性滚动兑现门。 */
    readingKey: string | null
    date: string | null
    days: number
    range: string
  }

  let stopScrollRestore: (() => void) | null = null
  let removeUserScrollListeners: (() => void) | null = null
  let pendingScroll: PendingScroll | null = null

  function stopRestore(): void {
    pendingScroll = null
    stopScrollRestore?.()
    stopScrollRestore = null
    removeUserScrollListeners?.()
    removeUserScrollListeners = null
  }

  /** pending 建立即挂用户意图监听；布局阶段复用，不重复注册。 */
  function attachUserIntentListeners(): void {
    if (removeUserScrollListeners || typeof window === 'undefined') return
    const onUserIntent = (): void => stopRestore()
    window.addEventListener('wheel', onUserIntent, { passive: true })
    window.addEventListener('touchstart', onUserIntent, { passive: true })
    window.addEventListener('pointerdown', onUserIntent)
    window.addEventListener('keydown', onUserIntent)
    removeUserScrollListeners = () => {
      window.removeEventListener('wheel', onUserIntent)
      window.removeEventListener('touchstart', onUserIntent)
      window.removeEventListener('pointerdown', onUserIntent)
      window.removeEventListener('keydown', onUserIntent)
    }
  }

  function rangeKey(range: ObservationRange | null): string {
    return range ? `${range.start}~${range.end}` : ''
  }

  function currentScrollContext(): Omit<PendingScroll, 'target'> {
    return {
      lane: deps.selectedLaneKey.value,
      readingKey: deps.currentReadingKey.value,
      date: deps.selectedDate.value,
      days: deps.sourceDays.value,
      range: rangeKey(deps.observation.value),
    }
  }

  function matchesContext(pending: PendingScroll): boolean {
    const current = currentScrollContext()
    return current.lane === pending.lane
      && current.readingKey === pending.readingKey
      && current.date === pending.date
      && current.days === pending.days
      && current.range === pending.range
  }

  /** 终态信号必须命中当前 return 目标（阅读 key + 日期）才允许恢复；pending 期间不排帧。 */
  function readingReady(pending: PendingScroll): boolean {
    if (pending.readingKey === null || pending.date === null) return true
    const settled = deps.readingSettled.value
    return settled?.laneKey === pending.readingKey && settled.date === pending.date
  }

  function maybeStartPendingScroll(): void {
    const pending = pendingScroll
    if (!pending) return
    if (!matchesContext(pending)) {
      stopRestore()
      return
    }
    if (!readingReady(pending)) return
    pendingScroll = null
    startLayoutScrollRestore(pending)
  }

  function startLayoutScrollRestore(pending: PendingScroll): void {
    const driver = scrollDriver()
    if (!driver) {
      stopRestore()
      return
    }
    const targetContext = {
      lane: pending.lane,
      readingKey: pending.readingKey,
      date: pending.date,
      days: pending.days,
      range: pending.range,
    }
    let finishedSync = false
    const cleanup = (): void => {
      removeUserScrollListeners?.()
      removeUserScrollListeners = null
      if (stopScrollRestore) stopScrollRestore = null
    }
    const cancel = restoreScrollWhenReady({
      driver,
      target: pending.target,
      maxAttempts: 180,
      isCancelled: () => deps.selectedLaneKey.value !== targetContext.lane
        || deps.currentReadingKey.value !== targetContext.readingKey
        || deps.selectedDate.value !== targetContext.date
        || deps.sourceDays.value !== targetContext.days
        || rangeKey(deps.observation.value) !== targetContext.range,
      onDone: () => {
        finishedSync = true
        cleanup()
      },
    })
    if (!finishedSync) {
      stopScrollRestore = () => {
        cancel()
        cleanup()
      }
    }
  }

  function restoreScroll(target: number): void {
    stopRestore()
    const context = currentScrollContext()
    pendingScroll = { target, ...context }
    attachUserIntentListeners()
    maybeStartPendingScroll()
  }

  watch(deps.readingSettled, maybeStartPendingScroll)
  watch(
    [deps.selectedLaneKey, deps.currentReadingKey, deps.selectedDate, deps.sourceDays, deps.observation],
    () => { if (pendingScroll) maybeStartPendingScroll() },
  )

  if (getCurrentScope()) onScopeDispose(stopRestore)

  return { readScrollTop, restoreScroll, stopRestore }
}
