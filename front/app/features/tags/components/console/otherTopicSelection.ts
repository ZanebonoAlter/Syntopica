/**
 * 「当日其他话题」选择/展开态控制器（8C G4a，非 Vue；G2/G3 组件仍纯受控）。
 *
 * 唯一职责（实现合同 §4.2/§4.3 与 G4a brief B）：
 *  - 持有 `selectedOther`（当日局部身份：date+sectionId，reportId/topicId 从真实报告/行取得；
 *    **不存标题**）与 `othersOpen`（首份**成功 A 响应**按 active+dormant 总行数 == 0 初始化一次，
 *    0 正式展开、成熟收起；之后只由用户 toggle 改变，切档/裁范围不强改，切板块复位）。
 *  - `otherTopics` 直接调用 G1 `buildOtherTopics`（排除/行序/计数/状态词/状态机唯一实现），
 *    入参全部来自既有单 store 与 useConsoleSelection 派生，不复制身份/计数/排除算法，不拿
 *    B 池/candidates/watch 凑行。
 *  - 选日/选行/点观察节点只写状态与选择：**不弹层、不强滚、不抢焦点、不写 URL**（无 date=）。
 *  - 清场规则：`selectedDate` 变化且与选择日期不一致才清（比较运行时的当前值，避免
 *    「日期 watcher 排队后把刚原子写入的新 selectedOther 清掉」）；切泳道/切板块/范围排出
 *    （经 selectedDate 校正）都排出旧日选择。详情缺原 section **不**清选择（阅读槽显示
 *    「报告已更新」可重试，不退回“先选一条”）。
 *  - **已验证记录日**：经黄点（观察节点）选中的那天是「已验证记录日」（观察带里同 ID 的真实
 *    分区记录，比报日索引更强）；报日索引缺该日（陈旧/不全）时选中日校正不得把它当普通无报日
 *    搬走（否则黄点点击被静默撤销，8E review R-41）。只保护当前这一个目标，换选/清场/切泳道/
 *    切板块立即失效 → 普通日期的回落语义不变。
 *
 * 不拥有：selectedDate 本身（useConsoleSelection 唯一持有）、日报读取（useConsoleReading）、
 * 返回载体/滚动（useConsoleReturnContext）、任何 Vue 模板。
 */
import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import type { LaneDynamicsWindowData } from '~/api/normalizers/laneDynamicsWindow'
import type { LaneReportDay } from '~/features/tags/composables/useLaneDayReading'
import { otherReadingKey } from './consoleReturnContext'
import { buildOtherTopics, type OtherTopicsView } from './dayOtherTopics'
import { reportDaysInAxis } from './laneConsoleView'
import type { LaneDayResolver, ObservationLaneRow } from './observationLanes'

/** 其他分区选择（当日局部身份；date 必须与唯一 selectedDate 一致）。 */
export interface OtherSelectionTarget {
  sectionId: string
  date: string
  reportId: string | null
  topicId: string | null
}

/** G2 `DayOtherTopicsGroup`/`ObservationMatrixGrid` 的 selectOther payload。 */
export interface SelectOtherPayload {
  sectionId: string
  date: string
  topicId?: string | null
}

/** G3 `OtherTopicReading` 观察节点 payload（真实报告/分区/话题身份）。 */
export interface SelectObservedDayPayload {
  date: string
  sectionId: string
  reportId: string
  topicId: string
}

export interface OtherTopicSelectionDeps {
  boardId: Ref<number>
  /** 壳层唯一选中日（本控制器只读 + 校正选择，不写日期）。 */
  selectedDate: Ref<string | null>
  /** 选中泳道实体 key（切泳道清其他选择；不参与其他身份）。 */
  selectedLaneKey: ComputedRef<string | null>
  /** 可见观察轴（可选日/节点合法性 + 窗内报日数）。 */
  visibleAxis: ComputedRef<string[]>
  /** G1 真实报日集合（列头可选性；空集 = 索引未就绪，不据此断言无日报）。 */
  reportDates: ComputedRef<ReadonlySet<string>>
  indexLoading: Ref<boolean>
  indexError: Ref<string | null>
  /** G1 报告优先读取（display gate 由 G1/G3 各自再核 date）。 */
  reportDay: Ref<LaneReportDay | null>
  allLanes: ComputedRef<ObservationLaneRow[]>
  resolveLaneDay: LaneDayResolver
  /** A 通道成功响应（null = 未到达；只用于首份成功响应判定，不参与组成员）。 */
  sourceWindow: Ref<LaneDynamicsWindowData | null>
}

const ID_PATTERN = /^\d+$/

function isId(value: unknown): value is string {
  return typeof value === 'string' && ID_PATTERN.test(value)
}

export function useOtherTopicSelection(deps: OtherTopicSelectionDeps) {
  const selectedOther = ref<OtherSelectionTarget | null>(null)
  const othersOpen = ref(false)
  /**
   * **已验证记录日**（当前观察目标日）：只认本控制器经 `selectObservedDay` 原子写入的那一个日子
   * ——它来自观察带里同 ID 的真实分区记录（自带 reportId/sectionId），比报日索引更强。报日索引
   * 缺该日（陈旧/不全）时，选中日校正不得把它当普通无报日搬走（否则黄点点击被静默撤销，
   * 8E review R-41）。换选/清场/切泳道/切板块立即失效 → 普通日期的回落语义不变。
   */
  const observedTargetDay = ref<string | null>(null)
  /** 「首份成功 A 响应」只消费一次：切档/裁范围/刷新都不重算用户主权。 */
  let othersInitialized = false

  const selectedOtherSectionId = computed<string | null>(() => selectedOther.value?.sectionId ?? null)
  const selectedOtherTopicId = computed<string | null>(() => selectedOther.value?.topicId ?? null)
  /** 阅读 key：与 G3 阅读终态 `laneKey` 同口径（`other:<sectionId>`）；无选择为 null。 */
  const selectedOtherReadingKey = computed<string | null>(() => {
    const selection = selectedOther.value
    return selection === null ? null : otherReadingKey(selection.sectionId)
  })

  const otherTopics = computed<OtherTopicsView>(() => buildOtherTopics({
    selectedDate: deps.selectedDate.value,
    reportDay: deps.reportDay.value,
    allLanes: deps.allLanes.value,
    resolveLaneDay: deps.resolveLaneDay,
    indexLoading: deps.indexLoading.value,
    indexError: deps.indexError.value,
    windowReportDayCount: reportDaysInAxis(deps.visibleAxis.value, deps.reportDates.value).length,
  }))

  function inAxis(date: string): boolean {
    return deps.visibleAxis.value.includes(date)
  }

  function clearOther(): void {
    selectedOther.value = null
    observedTargetDay.value = null
  }

  function resetForBoard(): void {
    selectedOther.value = null
    observedTargetDay.value = null
    othersOpen.value = false
    othersInitialized = false
  }

  /** 首份成功 A 响应初始化默认展开（0 正式/休眠行 → 展开；成熟 → 收起），只一次/板块。 */
  function maybeInitializeOthersOpen(): void {
    if (othersInitialized || deps.sourceWindow.value === null) return
    othersInitialized = true
    othersOpen.value = deps.allLanes.value.length === 0
  }

  function toggleOthers(): void {
    // 非 ready 没有可读行（组件已 disabled；这里再守一道，防编程调用造成假展开态）。
    if (otherTopics.value.state !== 'ready') return
    othersOpen.value = !othersOpen.value
  }

  /** 其他组选行/选格：只接受与当前选中日一致、且真实属于 G1 其他组视图成员的分区。 */
  function selectOther(payload: SelectOtherPayload): void {
    const date = deps.selectedDate.value
    if (date === null || payload.date !== date || !inAxis(date)) return
    if (!isId(payload.sectionId)) return
    // 成员合法性：正式/休眠泳道分区也在当日详情里，不能只看“详情存在”——直接复用 G1
    // `buildOtherTopics` 的行（排除/行序/状态唯一实现），不在这里复制排除算法。
    const view = otherTopics.value
    if (view.state !== 'ready') return
    if (!view.rows.some(row => row.sectionId === payload.sectionId)) return
    const day = deps.reportDay.value
    if (day === null || day.date !== date || day.status !== 'ready') return
    // 普通行选择不是观察目标：释放「已验证记录日」保护（普通日期回落语义不变）
    observedTargetDay.value = null
    selectedOther.value = {
      sectionId: payload.sectionId,
      date,
      reportId: day.reportId,
      topicId: isId(payload.topicId) ? payload.topicId : null,
    }
  }

  /** 列头选日：只改同一 selectedDate（不碰选择；旧日选择由下面的日期 watcher 排出）。 */
  function selectDayHeader(date: string): void {
    if (!inAxis(date) || !deps.reportDates.value.has(date)) return
    deps.selectedDate.value = date
  }

  /**
   * 观察节点：**原子**更新同一 selectedDate + 真实 report/section/topic 目标。
   * 先写日期再写选择，避免中间态被组件当作跨日旧选择；日期 watcher 比较的是运行时当前值，
   * 不会把刚写入的新选择清掉。
   */
  function selectObservedDay(payload: SelectObservedDayPayload): void {
    if (!inAxis(payload.date)) return
    if (!isId(payload.sectionId) || !isId(payload.reportId) || !isId(payload.topicId)) return
    // 先钉住「已验证记录日」再写日期：日期写出会触发选中日校正，校正必须看得到这个保护
    observedTargetDay.value = payload.date
    deps.selectedDate.value = payload.date
    selectedOther.value = {
      sectionId: payload.sectionId,
      date: payload.date,
      reportId: payload.reportId,
      topicId: payload.topicId,
    }
  }

  // 日期变化：只有选择日期与**当前**新日期不一致才清（用户主权/范围排出/切日清场）。
  watch(deps.selectedDate, (next) => {
    const current = selectedOther.value
    if (current !== null && current.date !== next) {
      selectedOther.value = null
      observedTargetDay.value = null
    }
  })

  // 切泳道：其他分区是当日局部身份，换泳道即清（避免阅读槽冒充新泳道上下文）。
  // 用 sync：返回恢复会先 selectLane 再写 selectedOther，排队的 watcher 会把刚恢复的选择清掉。
  watch(deps.selectedLaneKey, () => {
    if (selectedOther.value !== null) selectedOther.value = null
    observedTargetDay.value = null
  }, { flush: 'sync' })

  // 切板块：索引/详情缓存由 store 复位；选择与默认展开态一并复位（下次首份 A 再初始化）。
  watch(deps.boardId, () => resetForBoard())

  watch(
    [deps.sourceWindow, deps.allLanes],
    () => maybeInitializeOthersOpen(),
    { immediate: true },
  )

  return {
    selectedOther,
    selectedOtherSectionId,
    selectedOtherTopicId,
    selectedOtherReadingKey,
    /** 已验证记录日（当前观察目标日）：壳层喂给选中日校正，普通回落不得搬走它。 */
    observedTargetDay,
    othersOpen,
    otherTopics,
    toggleOthers,
    selectOther,
    selectDayHeader,
    selectObservedDay,
    clearOther,
    resetForBoard,
  }
}

export type OtherTopicSelection = ReturnType<typeof useOtherTopicSelection>

/** 便于 G4b 组合两个 store 时保持 ref 身份（selectedOther 是 `ref`，返回类型不丢）。 */
export type OtherSelectionRef = Ref<OtherSelectionTarget | null>
