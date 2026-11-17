/**
 * 「当日其他话题」纯模型（rework-workshop-into-radar-and-desk 矩阵阶段 8C G1）。
 *
 * 唯一职责：把**所选日报详情**归一成组视图模型（成员/行序/计数/状态/状态机文案）。
 *  - 成员只来自报告详情 sections：排除「正式/休眠泳道在所选日解析为 content 的 sectionId」
 *    与「persistentTopicId ∈ A 名单（active+dormant）」的并集；候选池/watch/辅助标签/当前话题名
 *    根本不进入本模型入参（不得拿它们凑行）。
 *  - 行序 = 报告 `cluster_index` 升序（当日原序，不按线索数/名称重排）。
 *  - 行计数 = 该 section 详情里的**非综合稿**线索数（0 合法）；综合稿单独记在
 *    `compositeCount`（不计入合计）。未 ready 的状态不给数字（不冒 0）。
 *  - 组计数 = 组内**非综合稿**线索数合计（= 各行 `threadCount` 之和，与矩阵日格
 *    同一口径）；分区数只在说明文案里出现（`rows.length`）。
 *  - 状态只读生成时快照 `topic_status_at_report`（active/candidate/archived/null/未知枚举），
 *    无 `persistent_topic_id` → 当次未归属；MUST NOT 读当前 topic.status。
 *  - 只标所选日由组件按本模型 rows 渲染所选日列实现；本模型不产跨日序列。
 *
 * 纯函数、零网络、零状态；组件/壳层不得再算第二套排除口径（单一实现点）。
 */
import type { LaneDayReportSection } from '~/api/normalizers/laneDayReading'
import type { LaneReportDay } from '~/features/tags/composables/useLaneDayReading'
import type { LaneDayResolver, ObservationLaneRow } from './observationLanes'

/** 组状态机：ready 允许 0 行；no-report / error / index-* 不得冒充 0 行。 */
export type OtherTopicsState = 'no-date' | 'index-loading' | 'index-error' | 'loading' | 'no-report' | 'error' | 'ready'

/** 行状态种类：与 spec 五种文案一一对应（unattributed = 当次未归属）。 */
export type OtherTopicStatusKind = 'active' | 'candidate' | 'archived' | 'unknown' | 'unattributed'

export const OTHER_TOPIC_STATUS_TEXT: Record<OtherTopicStatusKind, string> = {
  active: '当时已转正',
  candidate: '当时待观察',
  archived: '当时已归档',
  unknown: '当时状态未记录',
  unattributed: '当次未归属',
}

export const OTHER_TOPICS_NO_DATE_TEXT = '先选择一个有日报的日期'
export const OTHER_TOPICS_NO_WINDOW_REPORT_TEXT = '当前观察窗内没有日报'
export const OTHER_TOPICS_INDEX_LOADING_TEXT = '读取报日…'
export const OTHER_TOPICS_INDEX_ERROR_TEXT = '报日索引未取到，暂不能读取当日分区'
export const OTHER_TOPICS_LOADING_TEXT = '正在读取当日报告…'
export const OTHER_TOPICS_NO_REPORT_TEXT = '所选日没有日报（不是 0 条其他话题）'
export const OTHER_TOPICS_ERROR_TEXT = '当日报告读取失败'
/** ready + 0 行：真实结论（分部都已在泳道读到），不是失败也不是无日报。 */
export const OTHER_TOPICS_EMPTY_TEXT = '当日分区都已在泳道内可读'
/** 部分 A 泳道所选日 `unavailable`：组照常但注明来源。 */
export const OTHER_TOPICS_UNAVAILABLE_NOTE = '部分泳道当日内容未取到，其他话题按当日报告分区列出'

export interface OtherTopicRow {
  sectionId: string
  /** 当日报告分区原题（逐字；两行 clamp 由组件做）。 */
  label: string
  clusterIndex: number | null
  /** 该 section 详情的**非综合稿**线索数（0 合法；与矩阵日格同一口径）。 */
  threadCount: number
  /** 该 section 详情里的综合稿条数（不计入合计；展示层可标出）。 */
  compositeCount: number
  persistentTopicId: string | null
  statusKind: OtherTopicStatusKind
  statusText: string
}

export interface OtherTopicsInput {
  /** 壳层唯一选中日（null = 未选）。 */
  selectedDate: string | null
  /** store 发布的报告日模型（只有 `date === selectedDate` 才可用于渲染）。 */
  reportDay: LaneReportDay | null
  /** A 名单全部行（active + dormant；顺序不参与本模型）。 */
  allLanes: readonly ObservationLaneRow[]
  resolveLaneDay: LaneDayResolver
  indexLoading: boolean
  indexError: string | null
  /** 可见轴内报日数（0 → 「窗内没有日报」；null = 未接线，用中性文案）。 */
  windowReportDayCount?: number | null
}

export interface OtherTopicsView {
  state: OtherTopicsState
  rows: OtherTopicRow[]
  /** 组计数 = 组内**非综合稿**线索数合计（与矩阵日格同一口径）；仅 ready 给数字（0 合法），其余 null（不冒 0）。 */
  clueCount: number | null
  /** 组 summary 状态文案：ready 且有行时为 null（标题+计数由组件拼）。 */
  statusText: string | null
  /** 附加说明：读取失败的技术文案 / 部分泳道未取到。 */
  note: string | null
  retryable: boolean
}

function passive(
  state: OtherTopicsState,
  statusText: string,
  options: { retryable?: boolean, note?: string | null } = {},
): OtherTopicsView {
  return {
    state,
    rows: [],
    clueCount: null,
    statusText,
    note: options.note ?? null,
    retryable: options.retryable ?? false,
  }
}

function statusKindOf(section: LaneDayReportSection): OtherTopicStatusKind {
  const topicId = section.persistentTopicId ?? null
  if (topicId === null) return 'unattributed'
  switch (section.topicStatusAtReport ?? null) {
    case 'active':
      return 'active'
    case 'candidate':
      return 'candidate'
    case 'archived':
      return 'archived'
    default:
      // null / 未知枚举 / 大小写异常：不猜，诚实降级
      return 'unknown'
  }
}

function toRow(section: LaneDayReportSection): OtherTopicRow {
  const statusKind = statusKindOf(section)
  // 线索数口径：综合稿（thread 的 event 标签数 ≥ 2）不计入线索数——
  // 与矩阵日格同一口径（fix-lane-metric-provenance）。
  const compositeCount = section.threads.filter(thread => thread.scope === 'composite').length
  return {
    sectionId: section.sectionId,
    label: section.clusterLabel,
    clusterIndex: typeof section.clusterIndex === 'number' ? section.clusterIndex : null,
    threadCount: section.threads.length - compositeCount,
    compositeCount,
    persistentTopicId: section.persistentTopicId ?? null,
    statusKind,
    statusText: OTHER_TOPIC_STATUS_TEXT[statusKind],
  }
}

/** 身份排除：sectionId 已属当日可读泳道 或 persistentTopicId ∈ A 名单（两条证据取并集）。 */
function isExcluded(
  section: LaneDayReportSection,
  exclusions: { sectionIds: Set<string>, topicIds: Set<string> },
): boolean {
  if (exclusions.sectionIds.has(section.sectionId)) return true
  const topicId = section.persistentTopicId ?? null
  return topicId !== null && exclusions.topicIds.has(topicId)
}

/** 身份排除集合：所选日可读泳道 sections ∪ A 名单 topicId（两条件取并集）。 */
function collectExclusions(
  allLanes: readonly ObservationLaneRow[],
  selectedDate: string,
  resolveLaneDay: LaneDayResolver,
): { sectionIds: Set<string>, topicIds: Set<string>, unresolvedLaneCount: number } {
  const sectionIds = new Set<string>()
  const topicIds = new Set<string>()
  let unresolvedLaneCount = 0
  for (const lane of allLanes) {
    topicIds.add(lane.topicId)
    const resolution = resolveLaneDay(lane.topicId, selectedDate)
    if (resolution.kind === 'unavailable') {
      unresolvedLaneCount += 1
      continue
    }
    if (resolution.kind !== 'content') continue
    for (const section of resolution.day.sections) sectionIds.add(section.sectionId)
  }
  return { sectionIds, topicIds, unresolvedLaneCount }
}

/** 组视图模型唯一派生入口（壳层调用后经 props 下发；组件不得自建网络/排除逻辑）。 */
export function buildOtherTopics(input: OtherTopicsInput): OtherTopicsView {
  const selectedDate = input.selectedDate
  if (selectedDate === null) {
    if (input.indexLoading) return passive('index-loading', OTHER_TOPICS_INDEX_LOADING_TEXT)
    if (input.indexError) {
      return passive('index-error', OTHER_TOPICS_INDEX_ERROR_TEXT, { retryable: true, note: input.indexError })
    }
    const noWindowReport = input.windowReportDayCount === 0
    return passive('no-date', noWindowReport ? OTHER_TOPICS_NO_WINDOW_REPORT_TEXT : OTHER_TOPICS_NO_DATE_TEXT)
  }

  const day = input.reportDay !== null && input.reportDay.date === selectedDate ? input.reportDay : null
  if (day === null) {
    // 已选日但报告优先读取尚未开始/在途：按索引态与读取态分层（不冒「无日报」）
    if (input.indexLoading) return passive('index-loading', OTHER_TOPICS_INDEX_LOADING_TEXT)
    if (input.indexError) {
      return passive('index-error', OTHER_TOPICS_INDEX_ERROR_TEXT, { retryable: true, note: input.indexError })
    }
    return passive('loading', OTHER_TOPICS_LOADING_TEXT)
  }
  if (day.status === 'loading') {
    return passive('loading', OTHER_TOPICS_LOADING_TEXT)
  }
  if (day.status === 'no-report') {
    return passive('no-report', OTHER_TOPICS_NO_REPORT_TEXT)
  }
  if (day.status === 'error') {
    const fromIndex = day.errorSource === 'index'
    return passive(
      fromIndex ? 'index-error' : 'error',
      fromIndex ? OTHER_TOPICS_INDEX_ERROR_TEXT : OTHER_TOPICS_ERROR_TEXT,
      { retryable: true, note: day.error },
    )
  }

  const exclusions = collectExclusions(input.allLanes, selectedDate, input.resolveLaneDay)
  const rows = [...day.sections]
    .sort((a, b) => (a.clusterIndex ?? Number.MAX_SAFE_INTEGER) - (b.clusterIndex ?? Number.MAX_SAFE_INTEGER))
    .filter(section => !isExcluded(section, exclusions))
    .map(toRow)

  const note = exclusions.unresolvedLaneCount > 0 ? OTHER_TOPICS_UNAVAILABLE_NOTE : null
  return {
    state: 'ready',
    rows,
    // 组计数取**非综合稿**线索合计（与矩阵日格同一口径）：一个分区里 3 条非综合稿
    // 线索就显示 3，不是分区数 1，也不是含综合稿的 thread 总数。
    clueCount: rows.reduce((sum, row) => sum + row.threadCount, 0),
    statusText: rows.length === 0 ? OTHER_TOPICS_EMPTY_TEXT : null,
    note,
    retryable: false,
  }
}
