/**
 * otherTopicSelection 测试共用 fixture 与装配（8C G4a；按 8C.4S 结构规范抽共享 harness）。
 *
 * 只放测试数据与 effectScope 装配：可见轴/日报日/报告详情/家庭窗口 fixture、真实
 * `buildObservationRows` 派生 allLanes。断言与故事留在：
 *  - `otherTopicSelection.test.ts`：首份成功 A 默认展开/组视图（G1 buildOtherTopics 派生）
 *  - `otherTopicSelection.select.test.ts`：选行/列头/观察节点、清场与维护性 guard
 *
 * 不得以 `*.test.ts` 命名（会被 Vitest 当测试收集）；不 mock 模块、不跨文件共享实例。
 */
import { computed, effectScope, ref } from 'vue'
import type { LaneDynamicsWindowData } from '~/api/normalizers/laneDynamicsWindow'
import type { LaneDayReportSection } from '~/api/normalizers/laneDayReading'
import type { LaneReportDay } from '~/features/tags/composables/useLaneDayReading'
import type { LaneDayResolution } from '~/features/tags/composables/useObservationWindow'
import { buildObservationRows, type LaneDayResolver } from './observationLanes'
import { useOtherTopicSelection } from './otherTopicSelection'

export const AXIS = ['2026-10-22', '2026-10-23', '2026-10-24']
export const DATE = '2026-10-24'

export function windowData(lanes: { topicId: string, days?: { date: string, sectionIds: string[] }[] }[]): LaneDynamicsWindowData {
  return {
    windowDays: 14,
    hasReports: true,
    anchorDate: AXIS[AXIS.length - 1]!,
    candidates: [],
    lanes: lanes.map(lane => ({
      topicId: lane.topicId,
      label: `泳道${lane.topicId}`,
      watchLinked: false,
      sectionCountWindow: 0,
      threadCountWindow: null,
      snapshot: null,
      timeline: (lane.days ?? []).map(day => ({
        date: day.date,
        sections: day.sectionIds.map(sectionId => ({
          sectionId,
          label: `桶${sectionId}`,
          threadCount: null,
          fallbackClueCount: 1,
          foldedCount: 0,
          eventTitles: ['事件'],
        })),
      })),
    })),
  }
}

function resolverFor(source: LaneDynamicsWindowData | null): LaneDayResolver {
  const index = new Map<string, Map<string, NonNullable<LaneDynamicsWindowData['lanes'][number]['timeline'][number]>>>()
  for (const lane of source?.lanes ?? []) index.set(lane.topicId, new Map(lane.timeline.map(day => [day.date, day])))
  return (topicId, date): LaneDayResolution => {
    const day = index.get(topicId)?.get(date)
    return day ? { kind: 'content', source: 'tier', day } : { kind: 'gap' }
  }
}

export function section(
  sectionId: string,
  clusterIndex: number,
  persistentTopicId: string | null,
  threadCount = 1,
): LaneDayReportSection {
  return {
    sectionId,
    clusterIndex,
    clusterLabel: `分区${sectionId}`,
    bucketTitle: null,
    topicStatusAtReport: 'active',
    persistentTopicId,
    threads: Array.from({ length: threadCount }, (_, i) => ({
      threadId: `${sectionId}-${i}`,
      sectionId,
      title: `线索${i}`,
      summary: '',
      confidence: 0.8,
      relatedArticleIds: [],
      fitDistance: null,
      createdAt: '',
    })),
  }
}

export function readyDay(date: string, sections: LaneDayReportSection[], reportId = '900'): LaneReportDay {
  return { date, status: 'ready', error: null, errorSource: null, reportId, sections }
}

export const EMPTY_WINDOW = windowData([])
export const MATURE_WINDOW = windowData([
  { topicId: '10', days: [{ date: DATE, sectionIds: ['601'] }] },
  { topicId: '11', days: [{ date: '2026-10-23', sectionIds: ['602'] }] },
])

export function makeSelection(options: {
  source?: LaneDynamicsWindowData | null
  selectedDate?: string | null
  selectedLaneKey?: string | null
  reportDay?: LaneReportDay | null
  reportDates?: string[]
  boardId?: number
} = {}) {
  const boardId = ref(options.boardId ?? 1974)
  const selectedDate = ref<string | null>(options.selectedDate === undefined ? null : options.selectedDate)
  const selectedLaneKeyValue = ref<string | null>(options.selectedLaneKey === undefined ? null : options.selectedLaneKey)
  const sourceWindow = ref<LaneDynamicsWindowData | null>(options.source === undefined ? null : options.source)
  const reportDates = ref<ReadonlySet<string>>(new Set(options.reportDates ?? []))
  const reportDay = ref<LaneReportDay | null>(options.reportDay === undefined ? null : options.reportDay)
  const indexLoading = ref(false)
  const indexError = ref<string | null>(null)
  const visibleAxis = computed(() => [...AXIS])
  const allLanes = computed(() => buildObservationRows(sourceWindow.value, AXIS))
  const scope = effectScope()
  const selection = scope.run(() => useOtherTopicSelection({
    boardId,
    selectedDate,
    selectedLaneKey: computed(() => selectedLaneKeyValue.value),
    visibleAxis,
    reportDates: computed(() => reportDates.value),
    indexLoading,
    indexError,
    reportDay,
    allLanes,
    resolveLaneDay: (topicId, date) => resolverFor(sourceWindow.value)(topicId, date),
    sourceWindow,
  }))!
  return {
    boardId,
    selectedDate,
    selectedLaneKey: selectedLaneKeyValue,
    sourceWindow,
    reportDates,
    reportDay,
    indexLoading,
    indexError,
    selection,
    scope,
    setSource: (next: LaneDynamicsWindowData | null) => { sourceWindow.value = next },
  }
}
