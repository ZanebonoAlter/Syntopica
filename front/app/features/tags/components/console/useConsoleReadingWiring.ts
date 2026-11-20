/**
 * 控制台「报日索引 × 当日其他话题 × 单一阅读槽」接线（8C G4b，非 Vue）。
 *
 * 在 Shell setup 创建**一次**，把 G4a 的三个控制器（`useConsoleReading` 单份读取、
 * `useConsoleSelection` 选中日/名单投影、`useOtherTopicSelection` 其他分区选择）按合同顺序
 * 串起来，并向舞台下发**一组纯视图字段 + 注入的共享实例**：
 *
 *  - `useConsoleReading`（第 2 步，索引首取与编年挂载解耦）→ `useConsoleSelection`（第 3 步，
 *    注入同一份报日集合做三级校正）→ `bindSelectedDate`（第 4 步，唯一选中日跟随报告优先读取）
 *    → `useOtherTopicSelection`（第 5 步，其他组行/默认展开/阅读选择）。
 *  - 索引未就绪/失败时向选中日校正与 G2 列头都下发 `null`（三态：不是「已知 0 报日」）。
 *  - **点已选泳道/普通格/演进日都显式清其他选择**：不能只靠 `selectedLaneKey` watcher（同泳道
 *    再点不产生 key 变化），否则会残留旧阅读槽并出现双重选中。
 *  - **已验证记录日接线（8E review R-41）**：其他分区选择控制器持有「当前观察目标日」（黄点
 *    点击的真实同 ID 记录日）；本层把它同步给选中日校正（`useConsoleSelection`），使报日索引
 *    缺该日时黄点点击仍原子生效（不被普通回落静默撤销）。用同步 watcher：选择一变，保护立即
 *    生效/失效（清场、切泳道、切板块都回到普通回落语义）。
 *  - 生成完成/显式刷新/报日链重试都走同一编排：force 索引 → 当前报日 force → 观察懒失效 →
 *    lane 编年**基于刷新后缓存**非 force 重建（不重复第二遍 force 列表）。
 *  - `retryReport` 结果只用于驱动同一份 store 的 `reportDay`；本层不留「上次核对结论」，
 *    异步间切条时旧 `sectionFound` 不会显示到新选中条上（G4b 入场契约）。
 *
 * 不拥有：返回载体/滚动（`useConsoleReturnContext`）、范围/档/泳道（`useBoardConsole`）、
 * 任何模板。
 */
import { computed, shallowRef, watch, type ComputedRef, type Ref } from 'vue'
import type { LaneDynamicsWindowData } from '~/api/normalizers/laneDynamicsWindow'
import type { ConsoleReadingStageProps } from './consoleStageTypes'
import type { DateBounds } from './observationRange'
import type { LaneDayResolver } from './observationLanes'
import { useConsoleReading, type RetryReportPayload, type RetryReportResult } from './useConsoleReading'
import { useConsoleSelection } from './useConsoleSelection'
import { useOtherTopicSelection } from './otherTopicSelection'

export interface ConsoleReadingWiringDeps {
  boardId: Ref<number>
  /** 今天（YYYY-MM-DD；报日索引 lookback 用）。 */
  today: ComputedRef<string>
  /** 板块数据锚点（日报元信息优先）。 */
  anchorDate: ComputedRef<string | null>
  /** A 档自身全轴（不是可见裁切轴；名单/休眠基准）。 */
  axis: ComputedRef<string[]>
  /** 可见观察轴（报日列头/观察带可见性）。 */
  visibleAxis: ComputedRef<string[]>
  sourceWindow: Ref<LaneDynamicsWindowData | null>
  selectedLaneId: Ref<number | null>
  resolveLaneDay: LaneDayResolver
  poolBounds: ComputedRef<DateBounds | null>
  /** 原生选泳道（写 query 语义不变；壳层只在需要时包一层清其他选择）。 */
  selectLane: (topicId: string, options?: { writeQuery?: boolean }) => void
}

/** 舞台下发字段（纯视图 + 两个注入实例；字段全部来自壳层同一份 store 状态）。 */
export type { ConsoleReadingStageProps }

export function useConsoleReadingWiring(deps: ConsoleReadingWiringDeps) {
  // ── 2) 单份读取（索引首取在此触发，不依赖编年挂载） ──────────────────────
  const readingBridge = useConsoleReading({
    boardId: deps.boardId,
    today: () => deps.today.value,
    anchorDate: () => deps.anchorDate.value,
    axis: () => deps.visibleAxis.value,
  })

  /**
   * 报日集合三态（给选中日校正与 G2 列头）：null = 索引未就绪/失败（不是「已知无报日」）；
   * 成功过（含真实 0 报日）才下发 Set，已知报日在陈旧索引下仍可选（陈旧由 indexError 自述）。
   */
  const reportDates = computed<ReadonlySet<string> | null>(() => {
    const dates = readingBridge.reportDates.value
    if (dates.size > 0) return dates
    if (readingBridge.indexLoading.value || readingBridge.indexError.value) return null
    return dates
  })

  // ── 3) 选中日/名单投影（注入同一份报日集合 → 三级校正） ──────────────────
  /**
   * 已验证记录日（当前观察目标日）：由第 5 步的选择控制器回写（它才知道哪一天是经观察带
   * 验证过的真实记录日）。放在壳层是因为两个控制器的构造顺序（3 在 5 之前）——校正器只认
   * 这个壳层持有的值，不反向依赖选择控制器。
   */
  const observedTargetDay = shallowRef<string | null>(null)

  const selection = useConsoleSelection({
    sourceWindow: deps.sourceWindow,
    axis: deps.axis,
    visibleAxis: deps.visibleAxis,
    selectedLaneId: deps.selectedLaneId,
    resolveLaneDay: deps.resolveLaneDay,
    poolBounds: deps.poolBounds,
    reportDates,
    verifiedDay: observedTargetDay,
  })

  // ── 4) 唯一选中日 → 报告优先读取 ────────────────────────────────────────
  readingBridge.bindSelectedDate(selection.selectedDate)

  // ── 5) 其他分区选择/默认展开/组视图 ─────────────────────────────────────
  const other = useOtherTopicSelection({
    boardId: deps.boardId,
    selectedDate: selection.selectedDate,
    selectedLaneKey: selection.selectedLaneKey,
    visibleAxis: deps.visibleAxis,
    // 本控制器的依赖合同是 G1 真实报日集合（「空集 = 索引未就绪」），故直传 store 原始集合；
    // 三态 null 只留给需要区分「未知 vs 已知 0 报日」的选中日校正/列头。null 与空集在这里
    // 消费等价（`reportDaysInAxis(null)` → []、`.has()` 不成立），不砍三态也不放宽数据结构。
    reportDates: readingBridge.reportDates,
    indexLoading: readingBridge.indexLoading,
    indexError: readingBridge.indexError,
    reportDay: readingBridge.reportDay,
    allLanes: selection.allLanes,
    resolveLaneDay: deps.resolveLaneDay,
    sourceWindow: deps.sourceWindow,
  })

  const selectedOther = other.selectedOther
  const selectedOtherSectionId = other.selectedOtherSectionId

  // 已验证记录日 → 选中日校正：同步生效（选择一变就换保护/失效），不靠排队 flush 留窗口期。
  watch(
    other.observedTargetDay,
    (day) => { observedTargetDay.value = day },
    { immediate: true, flush: 'sync' },
  )

  // ── 显式用户动作：点泳道/普通格/演进日都清其他选择（同泳道再点也要清） ──
  function selectLaneFromStage(topicId: string): void {
    other.clearOther()
    deps.selectLane(topicId)
  }

  function selectCellFromStage(payload: { topicId: string, date: string }): void {
    other.clearOther()
    deps.selectLane(payload.topicId)
    selection.selectedDate.value = payload.date
  }

  function selectDateFromStage(date: string): void {
    other.clearOther()
    selection.selectedDate.value = date
  }

  // ── 受控重试/刷新编排 ────────────────────────────────────────────────────
  /** G3 阅读槽「重试/刷新」：force 索引 → force 当日详情 → 稳定 section 核对（store 自守上下文）。 */
  async function retryReport(payload: RetryReportPayload): Promise<RetryReportResult> {
    return readingBridge.retryReport(payload)
  }

  /**
   * lane 编年按当前共享缓存重建（非 force）：刷新链已在同一窗口强刷索引/详情，这里只重新 join，
   * 避免第二遍 force 列表；离开可读上下文（gap/unavailable/无选日）不读。
   */
  async function rebuildLaneReading(): Promise<void> {
    const row = selection.selectedLaneRow.value
    const date = selection.selectedDate.value
    if (!row || !date) return
    const resolution = deps.resolveLaneDay(row.topicId, date)
    if (resolution.kind !== 'content') return
    await readingBridge.reading.ensureDayReading({ laneKey: row.topicId, date }, resolution.day)
  }

  /** 生成完成/显式刷新/报日链重试：索引 → 当前报日 → 观察懒失效 → lane 编年重建。 */
  async function refreshAfterRebuild(): Promise<void> {
    await readingBridge.refreshAfterRebuild({
      date: () => selection.selectedDate.value,
      otherTopicId: () => other.selectedOther.value?.topicId ?? null,
    })
    await rebuildLaneReading()
  }

  /** 报日索引受控重试（G2 列头提示无独立按钮 → 舞台最小入口）。 */
  async function retryReportIndex(): Promise<void> {
    await readingBridge.ensureReportIndex({ force: true })
  }

  /**
   * 当日日报详情受控重试（右轨「当日日报」块局部降级用）：只强刷该日详情，
   * 不重建报日索引、不改选中日、不发观察请求。
   */
  async function retryReportDay(date: string): Promise<void> {
    if (!date) return
    await readingBridge.ensureReportDay(date, { force: true })
  }

  const stageProps = computed<ConsoleReadingStageProps>(() => ({
    reportDates: reportDates.value,
    indexLoading: readingBridge.indexLoading.value,
    indexError: readingBridge.indexError.value,
    otherTopics: other.otherTopics.value,
    othersOpen: other.othersOpen.value,
    selectedOtherSectionId: other.selectedOtherSectionId.value,
    selectedOtherTopicId: other.selectedOtherTopicId.value,
    reportDay: readingBridge.reportDay.value,
    reportStale: readingBridge.reportStale.value,
    reading: readingBridge.reading,
    otherObservation: readingBridge.observation,
  }))

  return {
    readingBridge,
    reading: readingBridge.reading,
    otherObservation: readingBridge.observation,
    reportDays: reportDates,
    selection,
    allLanes: selection.allLanes,
    selectedDate: selection.selectedDate,
    selectedLaneRow: selection.selectedLaneRow,
    selectedLaneKey: selection.selectedLaneKey,
    selectedOther,
    selectedOtherSectionId,
    selectedOtherTopicId: other.selectedOtherTopicId,
    selectedOtherReadingKey: other.selectedOtherReadingKey,
    othersOpen: other.othersOpen,
    otherTopics: other.otherTopics,
    toggleOthers: other.toggleOthers,
    selectDayHeader: other.selectDayHeader,
    selectOther: other.selectOther,
    selectObservedDay: other.selectObservedDay,
    clearOther: other.clearOther,
    selectLaneFromStage,
    selectCellFromStage,
    selectDateFromStage,
    retryReport,
    retryReportIndex,
    retryReportDay,
    refreshAfterRebuild,
    rebuildLaneReading,
    stageProps,
  }
}

export type ConsoleReadingWiring = ReturnType<typeof useConsoleReadingWiring>
