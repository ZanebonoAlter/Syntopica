import { getApiOrigin } from '~/utils/api'

interface OrganizeGroupInfo {
  new_label: string
  candidate_count: number
  action: string
  similarity?: number
}

interface OrganizeProgressMessage {
  type: 'organize_progress'
  status: 'processing' | 'completed'
  total_unclassified: number
  processed: number
  current_group?: OrganizeGroupInfo
  groups?: OrganizeGroupInfo[]
  category?: string
}

export function useOrganizeWebSocket() {
  const ws = ref<WebSocket | null>(null)
  const status = ref<'idle' | 'processing' | 'completed'>('idle')
  const totalUnclassified = ref(0)
  const processed = ref(0)
  const currentGroup = ref<OrganizeGroupInfo | null>(null)
  const category = ref<string>('')

  function connect() {
    if (ws.value?.readyState === WebSocket.OPEN) return
    if (ws.value?.readyState === WebSocket.CONNECTING) return

    const wsBase = getApiOrigin().replace(/^http/, 'ws')
    const url = `${wsBase}/ws`

    ws.value = new WebSocket(url)

    ws.value.onopen = () => {
      console.log('[OrganizeWS] Connected')
    }

    ws.value.onmessage = (event) => {
      try {
        const msg: OrganizeProgressMessage = JSON.parse(event.data)
        if (msg.type !== 'organize_progress') return

        status.value = msg.status as 'processing' | 'completed'
        totalUnclassified.value = msg.total_unclassified
        processed.value = msg.processed
        currentGroup.value = msg.current_group ?? null
        category.value = msg.category ?? ''
      } catch {
        // ignore non-JSON messages
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
    totalUnclassified.value = 0
    processed.value = 0
    currentGroup.value = null
    category.value = ''
  }

  onMounted(() => connect())
  onUnmounted(() => disconnect())

  return {
    status,
    totalUnclassified,
    processed,
    currentGroup,
    category,
    connect,
    disconnect,
    reset,
  }
}
