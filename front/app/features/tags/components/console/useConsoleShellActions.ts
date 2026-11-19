/**
 * 壳层动作编排（8C G4b 从 `BoardConsoleShell` 按内聚行为抽出）：视图切换/归档往返、报告与
 * 研究工作台入口、版块切换、失败重试与右轨定位。
 *
 * 只做「用户意图 → 既有入口」的转发与归档往返滚动存取；不拥有矩阵/阅读状态，不改归档页面，
 * 不新写 URL 键（四参与 `?focus=report` 语义仍由 `useConsoleReturnContext` 承担）。
 */
import type { ComputedRef, Ref } from 'vue'
import type { Router } from 'vue-router'
import type { ConsoleView } from '~/features/tags/composables/useBoardConsole'
import type { RetryReportPayload, RetryReportResult } from './useConsoleReading'
import type { useConsoleReturnContext } from './useConsoleReturnContext'

export interface ConsoleShellActionsDeps {
  boardId: number
  activeView: ComputedRef<ConsoleView>
  latestReportDate: ComputedRef<string | null>
  archiveInitialDate: Ref<string | null>
  setView: (view: ConsoleView) => void
  router: Router
  /** 返回载体/滚动（同一实例，归档往返复用其 pending 机制）。 */
  returnContext: ReturnType<typeof useConsoleReturnContext>
  /** A 档重取（矩阵/窄屏 A 失败重试）。 */
  reloadSource: () => Promise<void>
  /** 报日索引受控强刷。 */
  retryReportIndex: () => Promise<void>
  /** 索引 → 当日报告 → 观察失效 → lane 编年重建（同一防重窗口）。 */
  refreshAfterRebuild: () => Promise<void>
  /** G3 阅读槽受控重试。 */
  retryReport: (payload: RetryReportPayload) => Promise<RetryReportResult>
  /** 选泳道（显式清其他选择）。 */
  selectLaneFromStage: (topicId: string) => void
  closeWorkface: () => void
}

export function useConsoleShellActions(deps: ConsoleShellActionsDeps) {
  /** 归档往返的滚动位置（同实例返回矩阵时恢复；不写路由/不消耗返回载体）。 */
  let archiveReturnScroll: number | null = null

  function handleSetView(view: ConsoleView): void {
    const leavingArchive = deps.activeView.value === 'archive' && view !== 'archive'
    deps.setView(view)
    if (!leavingArchive) return
    const target = archiveReturnScroll
    archiveReturnScroll = null
    if (target !== null) deps.returnContext.restoreStageScroll(target)
  }

  function handleViewArchive(date?: string): void {
    // 默认日期口径（archive-time-chart-range）：带日期进归档 = 从历史格子「看归档」，该日期优先；
    // 不带日期 = 点顶栏「文章归档」普通切换，MUST 回到日历今天（旧实现保留上次带入日期 = 粘滞，
    // 会让「默认今天」在第二次进入时失效）。
    deps.archiveInitialDate.value = date ?? null
    archiveReturnScroll = deps.returnContext.captureStageScroll()
    deps.setView('archive')
  }

  function openLatestReport(): void {
    deps.returnContext.openReport(deps.latestReportDate.value)
  }

  function openResearch(): void {
    deps.returnContext.openResearch()
  }

  function selectBoard(id: number): void {
    if (id === deps.boardId) return
    void deps.router.push(`/tags/${id}`)
  }

  /** A 档失败重试：重取 A **同时**强刷报日索引（不能只刷旧 load）。 */
  function handleRetryStage(): void {
    void deps.reloadSource()
    void deps.retryReportIndex()
  }

  /** 报日链重试（索引/当日报告）：force 索引 → force 当日报告，不只刷 A。 */
  function handleRetryReportChain(): void {
    void deps.refreshAfterRebuild()
  }

  /** G3 阅读槽重试：结果只驱动同一份 reportDay（不留旧核对结论）。 */
  async function handleRetryOtherReport(payload: RetryReportPayload): Promise<void> {
    await deps.retryReport(payload)
  }

  /** 右轨「定位泳道」：关工作面、选同一泳道（清其他选择）并就地滚到格位。 */
  function locateLaneFromRail(topicId: string): void {
    deps.closeWorkface()
    deps.selectLaneFromStage(topicId)
    void Promise.resolve().then(() => {
      document.querySelector<HTMLElement>(`[data-testid="matrix-lane-${topicId}"]`)?.scrollIntoView({ block: 'nearest' })
    })
  }

  return {
    handleSetView,
    handleViewArchive,
    openLatestReport,
    openResearch,
    selectBoard,
    handleRetryStage,
    handleRetryReportChain,
    handleRetryOtherReport,
    locateLaneFromRail,
  }
}
