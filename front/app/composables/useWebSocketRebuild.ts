import { ref, onMounted, onUnmounted } from 'vue'
import { getApiOrigin } from '~/utils/api'

interface RebuildProgressMessage {
  type: 'hierarchy_rebuild'
  status: 'processing' | 'completed' | 'failed'
  job_id?: number
  total: number
  processed: number
  category: string
  failed_count?: number
  estimated_remaining_seconds?: number
  current_tag?: string
  error?: string
}

export function useWebSocketRebuild() {
  const ws = ref<WebSocket | null>(null)
  const status = ref<'idle' | 'processing' | 'completed' | 'failed'>('idle')
  const total = ref(0)
  const processed = ref(0)
  const category = ref('')
  const jobId = ref<number | null>(null)
  const failedCount = ref(0)
  const estimatedRemainingSeconds = ref(0)
  const currentTag = ref('')
  const errorMessage = ref('')

  function connect() {
    if (ws.value?.readyState === WebSocket.OPEN) return
    if (ws.value?.readyState === WebSocket.CONNECTING) return

    const wsBase = getApiOrigin().replace(/^http/, 'ws')
    const url = `${wsBase}/ws`

    ws.value = new WebSocket(url)

    ws.value.onmessage = (event) => {
      try {
        const msg: RebuildProgressMessage = JSON.parse(event.data)
        if (msg.type !== 'hierarchy_rebuild') return

        status.value = msg.status
        jobId.value = msg.job_id ?? null
        total.value = msg.total
        processed.value = msg.processed
        category.value = msg.category ?? ''
        failedCount.value = msg.failed_count ?? 0
        estimatedRemainingSeconds.value = msg.estimated_remaining_seconds ?? 0
        currentTag.value = msg.current_tag ?? ''
        errorMessage.value = msg.error ?? ''
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
    status.value = 'idle'
    total.value = 0
    processed.value = 0
    category.value = ''
    jobId.value = null
    failedCount.value = 0
    estimatedRemainingSeconds.value = 0
    currentTag.value = ''
    errorMessage.value = ''
  }

  onMounted(() => connect())
  onUnmounted(() => disconnect())

  return {
    status,
    total,
    processed,
    category,
    jobId,
    failedCount,
    estimatedRemainingSeconds,
    currentTag,
    errorMessage,
    reset,
  }
}
