/**
 * 右轨/单级宽工作面数据（rework-workshop-into-radar-and-desk 矩阵阶段 G5）。
 *
 * 健康、构成、watch 视图只在壳层取一次；右轨与工作面消费同一份 RailContext。
 * 「当日日报」同理：读的是壳层**已有的**当日日报详情（报告优先读取）与 lane-dynamics
 * 泳道清单（矩阵日格同源），**零新增请求**；模型由 `nowBrief.ts` 纯函数组装。
 * 收编/移除沿用既有真实处理函数（确认/错误/回执不因换 UI 丢失）。
 */
import { computed, type ComputedRef, type Ref } from 'vue'
import type { LaneDynamicsWindowData, WindowLane } from '~/api/normalizers/laneDynamicsWindow'
import type { BoardHealthCandidate } from '~/api/semanticBoards'
import type { TopicWatch } from '~/api/topicWatches'
import { normalizeRailHealth } from '~/api/normalizers/consoleRail'
import { useNotify } from '~/composables/useNotify'
import { useBoardHealth } from '~/features/tags/composables/useBoardHealth'
import { useBoardComposition } from '~/features/tags/composables/useBoardComposition'
import type { LaneReportDay } from '~/features/tags/composables/useLaneDayReading'
import type { ObservationLaneRow } from './observationLanes'
import type { OtherTopicsView } from './dayOtherTopics'
import { buildNowBriefModel } from './nowBrief'
import { toRailBinding, type RailContext } from './railContext'

export interface ConsoleRailDeps {
  boardId: Ref<number>
  sourceWindow: Ref<LaneDynamicsWindowData | null>
  allLanes: ComputedRef<ObservationLaneRow[]>
  watches: Ref<TopicWatch[]>
  watchesLoading: Ref<boolean>
  watchesError: Ref<string | null>
  /** 当日日报详情（报告优先读取的当日模型；null = 未到）。 */
  reportDay: Ref<LaneReportDay | null>
  /** 唯一选中日（矩阵选中日；右轨不另设日期源）。 */
  selectedDate: Ref<string | null>
  /** lane-dynamics 泳道清单（A 档 + 365 池合并；线索数与矩阵日格同源同值）。 */
  laneDynamicsLanes: Ref<WindowLane[]>
  /**
   * 「当日其他话题」组视图模型（壳层既有派生结果：唯一实现点 `dayOtherTopics.ts`；
   * 右轨只消费，不重算排除口径/计数，也不再发请求）。
   */
  otherTopics: Ref<OtherTopicsView>
}

export function useConsoleRail(deps: ConsoleRailDeps) {
  const { error: notifyError } = useNotify()
  const composition = useBoardComposition(deps.boardId)
  const health = useBoardHealth(deps.boardId, {
    onCaptured: () => { void composition.load() },
  })

  const healthRailSnapshot = computed(() => normalizeRailHealth(health.snapshot.value))

  /**
   * 「当日日报」视图模型：详情日期必须与选中日一致（display gate）——
   * 跨日的旧详情不进右轨（不把前一天的日报挂到今天的标题下）。
   */
  const nowModel = computed(() => {
    const selectedDate = deps.selectedDate.value
    const reportDay = deps.reportDay.value
    const report = reportDay && reportDay.date === selectedDate ? reportDay : null
    return buildNowBriefModel(report, selectedDate, deps.laneDynamicsLanes.value)
  })

  const railContext = computed<RailContext>(() => ({
    boardId: deps.boardId.value,
    laneIds: deps.allLanes.value.map(row => row.topicId),
    topicCandidateCount: deps.sourceWindow.value?.candidates.length ?? 0,
    health: {
      snapshot: healthRailSnapshot.value,
      loading: health.loading.value,
      error: health.error.value,
      stale: health.staleNotice.value,
      score: health.score.value,
      tier: health.tier.value,
      checkedAtText: health.checkedAtText.value,
      trendPolyline: health.trendPolyline.value,
      trendSummary: health.trendSummary.value,
    },
    watches: {
      items: deps.watches.value,
      loading: deps.watchesLoading.value,
      error: deps.watchesError.value,
    },
    now: nowModel.value,
    otherTopics: deps.otherTopics.value,
  }))

  const railBinding = computed(() => toRailBinding(railContext.value))

  /**
   * 摘除构成（既有 DELETE 链：removeFromComposition → 本地重取 → 体检重取）。
   * 返回**是否成功**：宿主据此回填「已摘除」反馈（失败不谎称已摘除）。
   */
  async function removeComposition(auxiliaryLabelId: number): Promise<boolean> {
    const ok = await composition.removeLabel(auxiliaryLabelId)
    await health.load()
    return ok
  }

  async function capture(candidate: BoardHealthCandidate): Promise<void> {
    const result = await health.capture(candidate)
    if (!result.ok) notifyError(`收编「${candidate.label}」失败：${result.error ?? '请重试'}`)
  }

  return {
    health,
    composition,
    healthRailSnapshot,
    railContext,
    railBinding,
    removeComposition,
    capture,
  }
}
