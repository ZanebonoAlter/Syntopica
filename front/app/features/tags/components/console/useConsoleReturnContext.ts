/**
 * 控制台自有返回上下文（矩阵阶段 G5；8C G4a 扩其他分区阅读槽/阅读 key；G4b 内聚拆滚动）。
 *
 * 覆盖阅读台（`?focus=report&lane=`）/研究工作台/外部返回：恢复泳道、选中日、档、范围、滚动与
 * 稳定其他分区选择；URL 已显式表达的项优先，不重放 report/annotation 一次性深链。一次性读取。
 *
 * G4b：滚动宿主/帧预算/用户意图取消拆到 `consoleReturnScroll`（本文件只做上下文恢复编排），
 * 并补「内部归档 → 矩阵同实例返回」的滚动存取入口（`captureStageScroll`/`restoreStageScroll`），
 * 不改归档页面、不写路由。
 */
import { computed, nextTick, watch, type ComputedRef, type Ref } from 'vue'
import type { LocationQuery, LocationQueryRaw, Router } from 'vue-router'
import type { LaneDynamicsWindowData } from '~/api/normalizers/laneDynamicsWindow'
import type { ObservationRange } from './observationRange'
import type { ObservationLaneRow } from './observationLanes'
import { createReturnScrollController, type ReadingSettledState } from './consoleReturnScroll'
import {
  otherReadingKey,
  saveReturnContext,
  takeReturnContext,
  type ConsoleOtherSelection,
  type ConsoleReturnContext,
} from './consoleReturnContext'

export type { ReadingSettledState } from './consoleReturnScroll'

export interface ConsoleReturnRouteLike {
  query?: LocationQuery
  path?: string
}

export interface ConsoleReturnDeps {
  boardId: Ref<number>
  sourceWindow: Ref<LaneDynamicsWindowData | null>
  sourceDays: Ref<number>
  allLanes: ComputedRef<ObservationLaneRow[]>
  selectedLaneKey: ComputedRef<string | null>
  selectedDate: Ref<string | null>
  observation: Ref<ObservationRange | null>
  stageRef: () => HTMLElement | null
  route: () => ConsoleReturnRouteLike | undefined
  router: () => Router | undefined
  selectLane: (topicId: string, options?: { writeQuery?: boolean }) => void
  setSourceDays: (days: number) => void
  applyObservationRange: (start: string, end: string, options?: { writeQuery?: boolean }) => boolean
  /** 当前编年是否已就绪/终态（pending 时不耗布局帧预算）。 */
  readingSettled: Ref<ReadingSettledState | null>
  /**
   * 8C G4a：稳定「其他分区」阅读槽（可选；省略 = 未接线，退回旧 lane 口径）。
   * 选中泳道实体与阅读 key 分开：`selectedOther` 不写进 `laneId`/URL `lane`，只决定下方阅读槽。
   */
  selectedOther?: Ref<ConsoleOtherSelection | null>
  /** 8C G4a：其他组展开态（仅在保存了 other 时一并恢复；可选）。 */
  othersOpen?: Ref<boolean>
}

export function useConsoleReturnContext(deps: ConsoleReturnDeps) {
  let pendingReturn: ConsoleReturnContext | null = safeTake(deps.boardId.value)
  let restoring = false

  /** 当前阅读 key：泳道 topicId 原 key，或 `other:<sectionId>`（与 G3 阅读终态 laneKey 同口径）。 */
  const currentReadingKey = computed<string | null>(() => {
    const other = deps.selectedOther?.value ?? null
    return other !== null ? otherReadingKey(other.sectionId) : deps.selectedLaneKey.value
  })

  const scroll = createReturnScrollController({
    stageRef: deps.stageRef,
    selectedLaneKey: deps.selectedLaneKey,
    currentReadingKey,
    selectedDate: deps.selectedDate,
    sourceDays: deps.sourceDays,
    observation: deps.observation,
    readingSettled: deps.readingSettled,
  })

  function safeTake(boardId: number): ConsoleReturnContext | null {
    try {
      return takeReturnContext(boardId)
    } catch {
      return null
    }
  }

  function readQueryValue(key: string): string | undefined {
    const raw = deps.route?.()?.query?.[key]
    const value = Array.isArray(raw) ? raw[0] : raw
    return typeof value === 'string' ? value : undefined
  }

  function hasExplicitRange(): boolean {
    return readQueryValue('start') !== undefined || readQueryValue('end') !== undefined
  }

  /** 保存矩阵自有上下文（泳道/日期/档/范围/滚动 + 稳定其他分区阅读槽），不含一次性 report/annotation。 */
  function saveContext(): void {
    const range = deps.observation.value
    const scrollTop = scroll.readScrollTop()
    const other = deps.selectedOther?.value ?? null
    // 旧六字段形态保持不变：只有真实选中其他分区时才追加 other/othersOpen（恢复阅读槽必需）。
    const otherFields = other === null
      ? {}
      : {
          other: {
            sectionId: other.sectionId,
            date: other.date,
            reportId: other.reportId,
            topicId: other.topicId,
          },
          othersOpen: deps.othersOpen?.value ?? true,
        }
    saveReturnContext(deps.boardId.value, {
      laneId: deps.selectedLaneKey.value,
      date: deps.selectedDate.value,
      days: deps.sourceDays.value,
      start: range?.start ?? null,
      end: range?.end ?? null,
      scrollTop,
      ...otherFields,
    })
  }

  function openReport(date: string | null): void {
    if (!date) return
    saveContext()
    const lane = deps.selectedLaneKey.value
    void deps.router?.()?.push({
      path: `/tags/${deps.boardId.value}/${date}`,
      query: lane ? { lane } : {},
    })
  }

  function openResearch(): void {
    saveContext()
    void deps.router?.()?.push(`/tags/${deps.boardId.value}/research`)
  }

  /**
   * 内部视图（归档）离开矩阵：记下实际滚动位置，供同实例返回时恢复。
   * 不写路由/不消费返回载体（归档不是真实路由往返）。
   */
  function captureStageScroll(): number {
    return scroll.readScrollTop()
  }

  /**
   * 内部视图（归档）返回矩阵：按同一 pending 机制恢复（等阅读终态 + 用户意图可取消）；
   * 目标 0 只清理旧 pending，不排帧。
   */
  function restoreStageScroll(target: number): void {
    if (target > 0) scroll.restoreScroll(target)
    else scroll.stopRestore()
  }

  function focusSelectedLane(): void {
    const lane = deps.selectedLaneKey.value
    if (lane && typeof document !== 'undefined') {
      document.querySelector<HTMLElement>(`[data-testid="matrix-lane-${lane}"]`)?.focus({ preventScroll: true })
    }
  }

  /** 阅读台返回焦点（`focus=report`）：聚焦选中泳道并消费一次性 query 键。 */
  async function consumeReportFocus(): Promise<void> {
    const route = deps.route?.()
    if (route?.query?.focus !== 'report') return
    await nextTick()
    focusSelectedLane()
    if (!route) return
    const query: LocationQueryRaw = { ...(route.query ?? {}) }
    delete query.focus
    await deps.router?.()?.replace({ path: route.path ?? '', query })
  }

  // 返回上下文恢复：逐项恢复；URL 显式值优先，未显式项才回填保存值。一次性。
  // R30：范围/泳道/档合并为一次完整 query 写入，避免真 router 异步导航互相取消。
  watch([deps.sourceWindow, deps.sourceDays, deps.allLanes], async () => {
    const context = pendingReturn
    if (!context || !deps.sourceWindow.value || restoring) return
    restoring = true
    try {
      if (readQueryValue('days') === undefined && context.days !== null && context.days !== deps.sourceDays.value) {
        deps.setSourceDays(context.days)
        return
      }

      const rangeApplied = !hasExplicitRange() && context.start !== null && context.end !== null
        ? deps.applyObservationRange(context.start, context.end, { writeQuery: false })
        : false
      const explicitLane = readQueryValue('lane')
      const laneValid = context.laneId !== null
        && deps.allLanes.value.some(row => row.topicId === context.laneId)
      const laneRestored = explicitLane === undefined && laneValid
      if (laneRestored && context.laneId) {
        deps.selectLane(context.laneId, { writeQuery: false })
      }
      if (context.date) deps.selectedDate.value = context.date

      // 其他分区阅读槽（8C G4a）：与泳道实体分开恢复——绝不写 `laneId`/URL `lane`。
      // 显式 URL lane 不同 → 显式导航优先（不恢复缓存阅读槽）；同一 lane 是「从其他话题
      // 打开日报再回来」的真实往返，必须恢复原阅读槽（S11-20）。
      const savedOther = context.other
      const laneConflict = explicitLane !== undefined && explicitLane !== context.laneId
      if (savedOther && !laneConflict && savedOther.date === context.date && deps.selectedOther) {
        deps.selectedOther.value = { ...savedOther }
        if (deps.othersOpen) deps.othersOpen.value = context.othersOpen !== false
      }

      const route = deps.route?.()
      if (route) {
        const query: LocationQueryRaw = { ...(route.query ?? {}) }
        let changed = false
        const hadFocus = query.focus === 'report'
        if (hadFocus) {
          delete query.focus
          changed = true
        }
        if (readQueryValue('days') === undefined && context.days !== null) {
          query.days = String(context.days)
          changed = true
        }
        if (rangeApplied && context.start && context.end) {
          query.start = context.start
          query.end = context.end
          changed = true
        }
        if (laneRestored && context.laneId) {
          query.lane = context.laneId
          changed = true
        }
        if (changed) {
          await deps.router?.()?.replace({ path: route.path ?? '', query })
        }
        if (hadFocus) focusSelectedLane()
      }

      await nextTick()
      if (context.scrollTop > 0) scroll.restoreScroll(context.scrollTop)
      else scroll.stopRestore()
      pendingReturn = null
    } finally {
      restoring = false
    }
  }, { immediate: true })

  watch([deps.allLanes, deps.selectedLaneKey], async () => {
    if (pendingReturn) return
    if (readQueryValue('focus') !== 'report' || deps.allLanes.value.length === 0) return
    await consumeReportFocus()
  }, { immediate: true })

  watch(deps.boardId, () => {
    scroll.stopRestore()
    pendingReturn = safeTake(deps.boardId.value)
  })

  return {
    saveContext,
    openReport,
    openResearch,
    captureStageScroll,
    restoreStageScroll,
    stopScrollRestore: scroll.stopRestore,
  }
}
