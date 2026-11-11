/**
 * 主舞台视图模型装配（8C G4b 从 `BoardConsoleShell` 抽出）：纯函数，无网络/无状态。
 *
 * 入参分两层：`useBoardConsole`/`useConsoleSelection` 的派生字段 + 右轨构成标签 + 归档初始日，
 * 以及 `useConsoleReadingWiring` 下发的阅读子集（报日索引/其他组/单一阅读槽/注入实例）。
 * 壳层因此在模板外只保留「取 .value → 装配」，不堆 40 行对象字面量。
 */
import type { ConsoleView } from '~/features/tags/composables/useBoardConsole'
import type { BoardConsoleStageProps, ConsoleReadingStageProps } from './consoleStageTypes'
import type { ConsoleLaneGroups } from './laneConsoleView'
import type { LaneTrendPoint } from './laneTrendSeries'
import type { DateBounds, ObservationRange } from './observationRange'
import type { LaneDayResolver, ObservationLaneRow } from './observationLanes'

export interface ConsoleStageBindingDeps {
  boardId: number
  boardLabel: string
  activeView: ConsoleView
  isNarrow: boolean
  emptyState: 'no-reports' | 'no-lanes' | null
  latestReportDate: string | null
  anchorDate: string | null
  loading: boolean
  error: string | null
  staleNotice: boolean
  today: string
  sourceDays: number
  observation: ObservationRange | null
  poolBounds: DateBounds | null
  poolSeries: LaneTrendPoint[] | null
  poolLoading: boolean
  poolError: string | null
  poolStaleNotice: boolean
  rangeNoticeText: string | null
  visibleAxis: string[]
  /** A 名单三分（rows = active + dormant 原序）。 */
  lanes: ConsoleLaneGroups
  selectedLaneKey: string | null
  selectedDate: string | null
  selectedLaneRow: ObservationLaneRow | null
  expandedDormant: boolean
  resolveLaneDay: LaneDayResolver
  compositionLabels: Array<{ id: number, label: string }>
  archiveInitialDate: string | null
  reading: ConsoleReadingStageProps
}

export function buildConsoleStageProps(deps: ConsoleStageBindingDeps): BoardConsoleStageProps {
  return {
    boardId: deps.boardId,
    boardLabel: deps.boardLabel,
    activeView: deps.activeView,
    isNarrow: deps.isNarrow,
    emptyState: deps.emptyState,
    latestReportDate: deps.latestReportDate,
    anchorDate: deps.anchorDate,
    loading: deps.loading,
    error: deps.error,
    staleNotice: deps.staleNotice,
    today: deps.today,
    sourceDays: deps.sourceDays,
    observation: deps.observation,
    poolBounds: deps.poolBounds,
    poolSeries: deps.poolSeries,
    poolLoading: deps.poolLoading,
    poolError: deps.poolError,
    poolStaleNotice: deps.poolStaleNotice,
    rangeNoticeText: deps.rangeNoticeText,
    visibleAxis: deps.visibleAxis,
    allLanes: deps.lanes.rows,
    activeLanes: deps.lanes.active,
    dormantLanes: deps.lanes.dormant,
    selectedLaneKey: deps.selectedLaneKey,
    selectedDate: deps.selectedDate,
    selectedLaneRow: deps.selectedLaneRow,
    expandedDormant: deps.expandedDormant,
    resolveLaneDay: deps.resolveLaneDay,
    compositionLabels: deps.compositionLabels,
    archiveInitialDate: deps.archiveInitialDate,
    ...deps.reading,
  }
}
