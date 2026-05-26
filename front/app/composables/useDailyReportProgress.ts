import { ref, onMounted, onUnmounted } from 'vue'
import { getApiOrigin } from '~/utils/api'

export interface BoardProgress {
  board_id: number
  board_name: string
  status: 'waiting' | 'generating' | 'completed' | 'failed'
  saved: number
  progress: string
}

export function useDailyReportProgress() {
  const ws = ref<WebSocket | null>(null)
  const progress = ref<Map<number, BoardProgress>>(new Map())
  const done = ref(false)
  const jobId = ref<string | null>(null)
  const totalSaved = ref(0)
  const totalBoards = ref(0)

  function connect() {
    if (ws.value?.readyState === WebSocket.OPEN) return
    if (ws.value?.readyState === WebSocket.CONNECTING) return

    const wsBase = getApiOrigin().replace(/^http/, 'ws')
    const url = `${wsBase}/ws`

    ws.value = new WebSocket(url)

    ws.value.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)

        if (msg.type === 'daily_report_progress') {
          if (!jobId.value) jobId.value = msg.job_id
          const bp: BoardProgress = {
            board_id: msg.board_id,
            board_name: msg.board_name,
            status: msg.status,
            saved: msg.saved ?? 0,
            progress: msg.progress ?? '',
          }
          progress.value.set(msg.board_id, bp)
          // Trigger reactivity
          progress.value = new Map(progress.value)
        }

        if (msg.type === 'daily_report_done') {
          done.value = true
          totalSaved.value = msg.total_saved ?? 0
          totalBoards.value = msg.total_boards ?? 0
        }
      } catch {
        // ignore non-JSON or unrelated messages
      }
    }

    ws.value.onclose = () => {
      ws.value = null
    }

    ws.value.onerror = () => {
      ws.value = null
    }
  }

  function disconnect() {
    if (ws.value) {
      ws.value.close(1000, 'Manual disconnect')
      ws.value = null
    }
  }

  function reset() {
    progress.value = new Map()
    done.value = false
    jobId.value = null
    totalSaved.value = 0
    totalBoards.value = 0
  }

  onMounted(() => connect())
  onUnmounted(() => disconnect())

  return {
    progress,
    done,
    jobId,
    totalSaved,
    totalBoards,
    reset,
  }
}
