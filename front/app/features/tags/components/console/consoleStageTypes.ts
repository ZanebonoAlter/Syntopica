/**
 * 主舞台受控视图模型（矩阵阶段 G5 + 8C G4b）：壳层构造、BoardConsoleStage 消费。
 *
 * 纯数据契约，不含网络/状态机；唯一例外是 `reading`/`observation` 两个**注入的共享实例**
 * （G4a 单例，下游只读消费，不由视图再建第二份索引/缓存）。报日索引/其他组/阅读槽的
 * 状态全部来自壳层同一份 store（见 `useConsoleReadingWiring`）。
 */
import type { ConsoleView } from '~/features/tags/composables/useBoardConsole'
import type { LaneDayReadingStore, LaneReportDay } from '~/features/tags/composables/useLaneDayReading'
import type { OtherTopicObservationStore } from '~/features/tags/composables/useOtherTopicObservation'
import type { OtherTopicsView } from './dayOtherTopics'
import type { LaneTrendPoint } from './laneTrendSeries'
import type { DateBounds, ObservationRange } from './observationRange'
import type { LaneDayResolver, ObservationLaneRow } from './observationLanes'

export interface BoardConsoleStageProps {
  boardId: number
  boardLabel: string
  activeView: ConsoleView
  isNarrow: boolean
  emptyState: 'no-reports' | 'no-lanes' | null
  latestReportDate: string | null
  anchorDate: string | null
  /** 事件身份是否已回填（lane-dynamics stories_backfilled；null = 旧响应未知，不提示降级）。 */
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
  allLanes: ObservationLaneRow[]
  activeLanes: ObservationLaneRow[]
  dormantLanes: ObservationLaneRow[]
  selectedLaneKey: string | null
  selectedDate: string | null
  selectedLaneRow: ObservationLaneRow | null
  expandedDormant: boolean
  resolveLaneDay: LaneDayResolver
  compositionLabels: Array<{ id: number, label: string }>
  archiveInitialDate: string | null
  // ── 8C G4b：报日索引 / 当日其他话题组 / 单一阅读槽（壳层同一份 store 状态） ──
  /** 真实报日集合：null = 索引未就绪或失败（列头不可点，不冒「无日报」）。 */
  reportDates: ReadonlySet<string> | null
  indexLoading: boolean
  indexError: string | null
  otherTopics: OtherTopicsView | null
  othersOpen: boolean
  selectedOtherSectionId: string | null
  selectedOtherTopicId: string | null
  reportDay: LaneReportDay | null
  reportStale: boolean
  /** 壳层共享读取实例（报日索引/详情/文章缓存 + lane 编年同一份；下游不得自建第二份）。 */
  reading: LaneDayReadingStore
  /** 壳层共享其他话题观察依赖（G3 section-timeline 单份缓存）；与观察范围 `observation` 区分。 */
  otherObservation: OtherTopicObservationStore
}

/**
 * 壳层接线（G4b `useConsoleReadingWiring`）负责的「报日索引 / 当日其他话题 / 单一阅读槽」字段子集。
 * 舞台其余字段仍来自 `useBoardConsole`/`useConsoleSelection`/右轨，不在此重复声明。
 */
export type ConsoleReadingStageProps = Pick<BoardConsoleStageProps,
  | 'reportDates'
  | 'indexLoading'
  | 'indexError'
  | 'otherTopics'
  | 'othersOpen'
  | 'selectedOtherSectionId'
  | 'selectedOtherTopicId'
  | 'reportDay'
  | 'reportStale'
  | 'reading'
  | 'otherObservation'>
