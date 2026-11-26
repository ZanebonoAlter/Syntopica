/**
 * 当日其他话题阅读终态信号（rework-workshop-into-radar-and-desk 矩阵阶段 8C G3，M2）。
 *
 * 与 `useReadingSettledSignal` 同协议（laneKey/date/status），但本组件上下文多一层：
 *  - **DOM 就绪**：setup 阶段/挂载前不发；等 `onMounted` + 一次 `nextTick`（DOM 已按终态
 *    更新，含会改变高度的观察带）后才发。初始 cache 已 ready 也不能提前假装布局已稳。
 *  - **报告终态**：`ready|missing-section|no-report|error` 才算终态，加载中/上下文未 gate 不发。
 *  - **观察依赖真实终态**：`ready|error|missing-id` 且**不在强刷中**（已有数据但 `refreshing`
 *    仍是刷新，不能当本轮终态）；`idle|loading` 不发。尚未展开的相关文章不进入等待。
 *  - **旧排队作废**：切日期/分区/话题或卸载时，任何还没发的排队信号不得发出；同一 context
 *    刷新走完终态后可再发一次（供父级 pending 消费）。
 *
 * 共享 `useReadingSettledSignal.ts` 是 lane 专属 deps 类型且无挂载/post-DOM 门，本包不改它，
 * 按同一协议在本文件适配（G3 专属，不复制日报读取状态）。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { ObservationStatus } from '~/features/tags/composables/useOtherTopicObservation'

export interface OtherReadingSettledPayload {
  /** 稳定其他分区 context key：`other:<sectionId>`（与泳道 topicId 命名空间区分）。 */
  laneKey: string
  date: string
  status: string
  sectionId: string
  topicId: string | null
}

export interface OtherReadingSettledDeps {
  selectedDate: () => string | null
  selectedSectionId: () => string | null
  /** 报告终态（ready/missing-section/no-report/error）；null = 上下文未 gate 或仍在加载。 */
  reportStatus: () => string | null
  /** 观察依赖状态（idle/loading 不是终态）。 */
  observationStatus: () => ObservationStatus
  /** 已有成功数据时强刷在途（view 仍是 ready，但不能当本轮终态）。 */
  observationRefreshing: () => boolean
  topicId: () => string | null
}

export function useOtherReadingSettledSignal(
  deps: OtherReadingSettledDeps,
  onSettled: (payload: OtherReadingSettledPayload) => void,
): void {
  const mounted = ref(false)
  onMounted(() => { mounted.value = true })
  onBeforeUnmount(() => { mounted.value = false })

  const settledState = computed<OtherReadingSettledPayload | null>(() => {
    const date = deps.selectedDate()
    const sectionId = deps.selectedSectionId()
    if (date === null || sectionId === null) return null
    const status = deps.reportStatus()
    if (status === null) return null
    const observation = deps.observationStatus()
    if (observation === 'idle' || observation === 'loading') return null
    if (deps.observationRefreshing()) return null
    return {
      laneKey: `other:${sectionId}`,
      date,
      status,
      sectionId,
      topicId: deps.topicId(),
    }
  })

  const settledKey = computed(() => {
    const settled = settledState.value
    if (settled === null) return ''
    return `${settled.laneKey}|${settled.date}|${settled.status}|${settled.topicId ?? '-'}`
  })

  /** 已发出的终态 key；回到 pending 时清空，允许同一 context 刷新后再发一次。 */
  let emittedKey = ''
  /** 正在排队（已过 watcher、等下一次 DOM flush）的 key；上下文一变即作废。 */
  let queuedKey = ''

  async function flushQueued(): Promise<void> {
    const key = queuedKey
    // 等 Vue 把终态渲染进 DOM（观察带节点/状态区），再允许父级开始布局恢复。
    await nextTick()
    if (!mounted.value || queuedKey !== key) return
    const payload = settledState.value
    if (payload === null || settledKey.value !== key) return
    emittedKey = key
    queuedKey = ''
    onSettled(payload)
  }

  function sync(): void {
    const key = settledKey.value
    if (key === '') {
      // 离开终态（加载中/换上下文/依赖刷新中）：旧排队作废，下次终态可再次发。
      queuedKey = ''
      emittedKey = ''
      return
    }
    if (!mounted.value) return
    if (key === emittedKey) return
    queuedKey = key
    void flushQueued()
  }

  watch([settledKey, mounted], sync)
}
