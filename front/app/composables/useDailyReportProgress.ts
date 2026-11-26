/**
 * 日报进度管理器 — 基于 useEventStream
 *
 * 现有消费者（DailyReportGenerateDialog.vue）的 API 完全兼容：
 *   progress (Map), done, jobId, totalSaved, totalBoards, reset()
 */

import { useEventStream } from '~/composables/useEventStream'
import { EVENT_TYPES } from '~/utils/eventTypes'

export interface BoardProgress {
  board_id: number
  board_name: string
  status: 'waiting' | 'generating' | 'completed' | 'failed'
  saved: number
  progress: string
}

export function useDailyReportProgress() {
  const stream = useEventStream()
  const progress = ref<Map<number, BoardProgress>>(new Map())
  const done = ref(false)
  const jobId = ref<string | null>(null)
  const totalSaved = ref(0)
  const totalBoards = ref(0)

  // 订阅 daily_report_progress
  const unsubProgress = stream.on<Record<string, unknown>>(EVENT_TYPES.DAILY_REPORT_PROGRESS, (msg) => {
    // 缺口补填的逐格进度复用同一事件类型（run_scope=backfill）——不属于本弹窗
    // 的这次生成：串台会让补填的「1/1 完成」提前把 done 拉起来（补填终态走
    // 独立事件 daily_report_backfill_done，不走本通道）。
    if (msg.run_scope === 'backfill') return
    if (!jobId.value) jobId.value = (msg.job_id as string) ?? null
    const rawStatus = (msg.status as string) === 'processing' ? 'generating' : (msg.status as string)
    const status: BoardProgress['status'] = ['waiting', 'generating', 'completed', 'failed'].includes(rawStatus)
      ? rawStatus as BoardProgress['status']
      : 'generating'
    const boardId = Number(msg.board_id ?? 0)
    const progressText = (msg.progress as string) ?? ''
    const bp: BoardProgress = {
      board_id: boardId,
      board_name: (msg.board_name as string) ?? `#${boardId}`,
      status,
      saved: (msg.saved as number) ?? 0,
      progress: progressText,
    }
    progress.value.set(boardId, bp)
    if (status === 'completed' && progressText === '1/1') {
      done.value = true
      totalSaved.value = bp.saved
      totalBoards.value = 1
    }
    // Trigger reactivity
    progress.value = new Map(progress.value)
  })

  // 订阅 daily_report_done
  const unsubDone = stream.on<Record<string, unknown>>(EVENT_TYPES.DAILY_REPORT_DONE, (msg) => {
    done.value = true
    totalSaved.value = (msg.total_saved as number) ?? 0
    totalBoards.value = (msg.total_boards as number) ?? 0
  })

  // 组件卸载时取消订阅
  onUnmounted(() => {
    unsubProgress()
    unsubDone()
  })

  function reset() {
    progress.value = new Map()
    done.value = false
    jobId.value = null
    totalSaved.value = 0
    totalBoards.value = 0
  }

  return {
    progress,
    done,
    jobId,
    totalSaved,
    totalBoards,
    reset,
  }
}
