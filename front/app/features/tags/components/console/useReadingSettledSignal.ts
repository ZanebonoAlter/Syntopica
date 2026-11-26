/**
 * 当前阅读上下文终态信号（矩阵阶段 G4/G5，R34）。
 *
 * LaneChronicle 只负责把「同一 lane/date 的 display 已就绪且非 loading」或
 * gap/unavailable 终态通知壳层；壳层据此再开始有限布局滚动恢复，
 * pending 期间不消耗任何帧预算。
 */
import { computed, watch } from 'vue'
import type { LaneDayResolution } from '~/features/tags/composables/useObservationWindow'
import type { ObservationLaneRow } from './observationLanes'

export interface ReadingSettledPayload {
  laneKey: string
  date: string
  status: string
}

export interface ReadingSettledDeps {
  lane: () => ObservationLaneRow | null
  selectedDate: () => string | null
  resolution: () => LaneDayResolution | null
  display: () => { status: string } | null
  loading: () => boolean
}

export function useReadingSettledSignal(deps: ReadingSettledDeps, onSettled: (payload: ReadingSettledPayload) => void): void {
  const settledState = computed<ReadingSettledPayload | null>(() => {
    const lane = deps.lane()
    const date = deps.selectedDate()
    if (!lane || !date) return null
    const kind = deps.resolution()?.kind
    if (kind === 'gap' || kind === 'unavailable') {
      return { laneKey: lane.topicId, date, status: kind }
    }
    const display = deps.display()
    if (!display || deps.loading()) return null
    return { laneKey: lane.topicId, date, status: display.status }
  })

  const settledKey = computed(() => settledState.value
    ? `${settledState.value.laneKey}|${settledState.value.date}|${settledState.value.status}`
    : '')

  watch(settledKey, () => {
    if (settledState.value) onSettled(settledState.value)
  }, { immediate: true })
}
