/**
 * 壳层单份读取组合（8C G4a，非 Vue）。
 *
 * 在 Shell setup 创建**一次**，供日期轴/编年/其他话题组/阅读槽共用：
 *  - `useLaneDayReading`（既有单 store：报日索引 1 份、详情 ≤14、文章 ≤20、双 epoch）；
 *  - `useOtherTopicObservation`（G3：板块级 section-timeline 单份缓存 + 懒失效）。
 * 不新建第二读取器/缓存，也不在视图层再触发首读。
 *
 * 职责边界（实现合同 §4.4 / G4a brief A）：
 *  - **初取报日索引与编年挂载解耦**：本组合在壳层 setup 即按板块/锚点取一次；锚点晚到或
 *    停更差额扩大由 G1 `reportIndexDaysFor` 覆盖，不由 lane 组件是否挂载决定。
 *  - **所选日读取**：`bindSelectedDate(selectedDate)`（useConsoleSelection 之后调用）把唯一
 *    选中日接到报告优先读取；同日不重复、跨日先清显示（G1 display gate），组折叠与否都读。
 *  - **受控重试** `retryReport`：`ensureReportIndex({force:true})` → `ensureReportDay(date,
 *    {force:true})` → 稳定 sectionId 核对。只读，不改选择、不沿旧 topic 发观察请求。
 *  - **生成完成/显式刷新** `refreshAfterRebuild`：强刷索引 → 当前阅读 → 观察失效（无其他行
 *    选择时零 GET；有选择才 `ensureObservation(topicId)`）。`useConsoleRefresh.after` 在同一
 *    防重窗口内调用本方法（元信息 → A → 本方法）。
 *  - 异步之间切板块/切日：旧结果不写回（G1 epoch/seq + 本文件 board/date 复核），G4b 据
 *    `applied=false` 忽略过期结果。
 *
 * 不拥有：selectedDate/选中泳道/范围/其他选择与展开态/滚动与返回载体（G4b 壳层与
 * `otherTopicSelection`/`useConsoleReturnContext` 持有）。
 */
import { shallowRef, watch, type Ref } from 'vue'
import { useLaneDayReading, type LaneDayReadingFetchers } from '~/features/tags/composables/useLaneDayReading'
import {
  useOtherTopicObservation,
  type OtherTopicObservationFetchers,
} from '~/features/tags/composables/useOtherTopicObservation'

export interface ConsoleReadingDeps {
  boardId: Ref<number>
  /** 今天（YYYY-MM-DD，Asia/Shanghai；报日索引 lookback 用）。 */
  today: () => string
  /** 板块数据锚点（日报元信息优先；晚到/停更由 G1 覆盖扩大，不在这里算公式）。 */
  anchorDate: () => string | null
  /** 可见观察轴（G3 观察带投影；变化不触发请求）。 */
  axis: () => readonly string[]
  readingFetchers?: LaneDayReadingFetchers
  observationFetchers?: OtherTopicObservationFetchers
}

export interface RetryReportPayload {
  date: string
  sectionId: string
}

export interface RetryReportResult {
  /** 板块/选中日在异步间仍与 payload 一致 → 结果可用于当前阅读槽；false = G4b 忽略。 */
  applied: boolean
  /** 稳定 sectionId 核对：true=仍在当日详情；false=报告已更新缺该分区；null=详情未到/失败。 */
  sectionFound: boolean | null
}

/** 生成完成/显式刷新的阅读目标（getter：异步间以最新上下文为准，不拉回旧选择）。 */
export interface ReadingRebuildTarget {
  /** 当前选中日报日；null = 无阅读槽（只强刷索引）。 */
  date: () => string | null
  /** 当前其他分区话题身份；null = 无其他行选择 → 观察只失效、零 GET。 */
  otherTopicId: () => string | null
}

export function useConsoleReading(deps: ConsoleReadingDeps) {
  const reading = useLaneDayReading({
    boardId: deps.boardId,
    today: deps.today,
    anchorDate: deps.anchorDate,
    ...(deps.readingFetchers ? { fetchers: deps.readingFetchers } : {}),
  })
  const observation = useOtherTopicObservation({
    boardId: deps.boardId,
    axis: deps.axis,
    ...(deps.observationFetchers ? { fetchers: deps.observationFetchers } : {}),
  })

  // 初取报日索引（1 次/板块；随后日期轴/编年/其他组/报告优先读取全部复用同一份）。
  // 锚点晚到或停更差额扩大时，G1 只扩大覆盖、不缩水、不重复同覆盖请求。
  watch(
    [deps.boardId, () => deps.anchorDate()],
    () => { void reading.ensureReportIndex().catch(() => {}) },
    { immediate: true },
  )

  // 选中日绑定：用 shallowRef 装**持有对象**是让 watch 能感知“绑定对象变化”，并在绑定后重新
  // 追踪新 ref 的 .value（直接闭包捕获 ref 会在换绑后丢依赖）。持有对象再包一层也避开
  // `shallowRef<Ref<T>>` 的类型分发（Vue 3.5 会把内层 Ref 的 value 类型并入外层 value），
  // 运行时仍是同一个绑定 ref，读写路径与包前一致。
  const boundSelectedDate = shallowRef<{ current: Ref<string | null> } | null>(null)
  watch(
    [boundSelectedDate, () => boundSelectedDate.value?.current.value ?? null],
    () => {
      const date = boundSelectedDate.value?.current.value ?? null
      if (date !== null) void reading.ensureReportDay(date).catch(() => {})
    },
    { immediate: true },
  )

  /** 构造顺序第 4 步：useConsoleSelection 产生唯一 selectedDate 后绑定（只读跟随，不持第二份）。 */
  function bindSelectedDate(selectedDate: Ref<string | null>): void {
    boundSelectedDate.value = { current: selectedDate }
  }

  function currentSelectedDate(): string | null {
    return boundSelectedDate.value?.current.value ?? null
  }

  /**
   * 选中日是否已离开 `date`（含被清空）。未绑定（无唯一选中日可核）时不拦，保持旧调用可用；
   * 绑定后必须一致——否则不允许触发旧日读取（旧日 `ensureReportDay` 会抢写/取消新选中日）。
   */
  function selectionMovedAway(date: string): boolean {
    const bound = boundSelectedDate.value
    return bound !== null && bound.current.value !== date
  }

  /**
   * 受控报告重试（G3 M1 意图的编排端）：先强刷报日索引（重建会换 reportId），再强刷当日详情，
   * 最后按稳定 sectionId 核对。只改 reportDay 读取结果，不清选择、不发观察请求。
   */
  async function retryReport(payload: RetryReportPayload): Promise<RetryReportResult> {
    const board = deps.boardId.value
    const date = payload.date
    const sectionId = payload.sectionId
    if (sectionId === '') return { applied: false, sectionFound: null }

    await reading.ensureReportIndex({ force: true })
    if (deps.boardId.value !== board) return { applied: false, sectionFound: null }
    // 索引 await 之后、任何旧 date 读取之前复核上下文：用户已切日/清场就不读旧日——
    // 旧日 ensureReportDay 会清掉并覆盖新选中日的 reportDay（末尾 applied=false 补救不了已写状态）。
    if (selectionMovedAway(date)) return { applied: false, sectionFound: null }

    await reading.ensureReportDay(date, { force: true })
    if (deps.boardId.value !== board) return { applied: false, sectionFound: null }

    // 选中日在异步间被切走：旧日核对结果不塞回新标题（G4b 看到 applied=false 即忽略）。
    const selectedDate = currentSelectedDate()
    if (selectedDate !== null && selectedDate !== date) return { applied: false, sectionFound: null }

    const day = reading.reportDay.value
    if (day === null || day.date !== date || day.status !== 'ready') {
      return { applied: true, sectionFound: null }
    }
    return { applied: true, sectionFound: day.sections.some(section => section.sectionId === sectionId) }
  }

  /**
   * 生成完成/显式刷新（在 `useConsoleRefresh.after` 内调用）：强刷索引 → 当前阅读 →
   * 观察懒失效（无其他行选择时零观察 GET；有选择才重读当前 topic）。异步间切板块则停止。
   */
  async function refreshAfterRebuild(target: ReadingRebuildTarget): Promise<void> {
    const board = deps.boardId.value
    await reading.ensureReportIndex({ force: true })
    if (deps.boardId.value !== board) return

    const date = target.date()
    const selectedDate = currentSelectedDate()
    if (date !== null && (selectedDate === null || selectedDate === date)) {
      await reading.ensureReportDay(date, { force: true })
      if (deps.boardId.value !== board) return
    }

    // 懒失效不发请求；没有其他行选择时到此为止（不沿旧 topic 发无谓 GET）。
    observation.invalidate()
    const topicId = target.otherTopicId()
    if (topicId !== null) await observation.ensureObservation(topicId)
  }

  return {
    reading,
    observation,
    bindSelectedDate,
    retryReport,
    refreshAfterRebuild,
    // 便捷只读（G4b 直接下发日期轴/其他组/阅读槽 props；仍是同一份 store 状态）。
    reportDates: reading.reportDates,
    reportDay: reading.reportDay,
    reportStale: reading.reportStale,
    indexLoading: reading.indexLoading,
    indexError: reading.indexError,
    ensureReportIndex: reading.ensureReportIndex,
    ensureReportDay: reading.ensureReportDay,
  }
}

export type ConsoleReading = ReturnType<typeof useConsoleReading>
