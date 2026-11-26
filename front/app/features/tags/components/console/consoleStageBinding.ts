/**
 * 主舞台视图模型装配（8C G4b 从 `BoardConsoleShell` 抽出）：纯函数，无网络/无状态。
 *
 * 入参分两层：`useBoardConsole`/`useConsoleSelection` 的派生字段 + 右轨构成标签 + 归档初始日，
 * 以及 `useConsoleReadingWiring` 下发的阅读子集（报日索引/其他组/单一阅读槽/注入实例）。
 * 壳层因此在模板外只保留「取 .value → 装配」，不堆 40 行对象字面量。
 */
import type { ConsoleView } from '~/features/tags/composables/useBoardConsole'
import type { WindowOtherTopicDay } from '~/api/normalizers/laneDynamicsWindow'
import type { BoardConsoleStageProps, ConsoleReadingStageProps } from './consoleStageTypes'
import type { CoverageLaneRow } from './coverageLanes'
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
  /** 「其他话题（未入泳道）」逐日读数（后端 `other_topics` 归一结果）。 */
  otherTopicDays: Record<string, WindowOtherTopicDay>
  /** 后端是否下发 `other_topics`（false = 旧后端 → 整行不渲染）。 */
  otherTopicDaysAvailable: boolean
  /** 「其他话题」逐日读数的档窗起点（YYYY-MM-DD；null = 未知）。 */
  otherTopicsSince: string | null
  /** 覆盖组行（窗口内未进日报的活跃泳道）。 */
  coverageLanes: CoverageLaneRow[]
  /** 覆盖清单读取中。 */
  coverageLoading: boolean
  /** 覆盖清单读取失败。 */
  coverageError: string | null
  /** 行头就地重命名（复用既有 PATCH）。 */
  renameLane: (topicId: string, label: string) => Promise<boolean>
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
    otherTopicDays: deps.otherTopicDays,
    otherTopicDaysAvailable: deps.otherTopicDaysAvailable,
    otherTopicsSince: deps.otherTopicsSince,
    coverageLanes: deps.coverageLanes,
    coverageLoading: deps.coverageLoading,
    coverageError: deps.coverageError,
    renameLane: deps.renameLane,
    ...deps.reading,
  }
}
