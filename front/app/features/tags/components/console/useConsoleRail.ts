/**
 * 右轨/单级宽工作面数据（rework-workshop-into-radar-and-desk 矩阵阶段 G5）。
 *
 * 健康、构成、watch 视图只在壳层取一次；右轨与工作面消费同一份 RailContext。
 * 收编/移除沿用既有真实处理函数（确认/错误/回执不因换 UI 丢失）。
 */
import { computed, type ComputedRef, type Ref } from 'vue'
import type { LaneDynamicsWindowData } from '~/api/normalizers/laneDynamicsWindow'
import type { BoardHealthCandidate } from '~/api/semanticBoards'
import type { TopicWatch } from '~/api/topicWatches'
import { normalizeRailHealth } from '~/api/normalizers/consoleRail'
import { useNotify } from '~/composables/useNotify'
import { useBoardHealth } from '~/features/tags/composables/useBoardHealth'
import { useBoardComposition } from '~/features/tags/composables/useBoardComposition'
import type { ObservationLaneRow } from './observationLanes'
import { toRailBinding, type RailContext } from './railContext'

export interface ConsoleRailDeps {
  boardId: Ref<number>
  sourceWindow: Ref<LaneDynamicsWindowData | null>
  allLanes: ComputedRef<ObservationLaneRow[]>
  watches: Ref<TopicWatch[]>
  watchesLoading: Ref<boolean>
  watchesError: Ref<string | null>
}

export function useConsoleRail(deps: ConsoleRailDeps) {
  const { error: notifyError } = useNotify()
  const composition = useBoardComposition(deps.boardId)
  const health = useBoardHealth(deps.boardId, {
    onCaptured: () => { void composition.load() },
  })

  const healthRailSnapshot = computed(() => normalizeRailHealth(health.snapshot.value))

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
