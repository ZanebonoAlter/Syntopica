import { getApiOrigin } from '~/utils/api'
import type { ArticleTag } from '~/types/article'

interface TagCompletedItem {
  slug: string
  label: string
  category: string
  score: number
  icon: string
}

interface TagCompletedMessage {
  type: 'tag_completed'
  article_id: number
  job_id: number
  tags: TagCompletedItem[]
}

interface TagFailedMessage {
  type: 'tag_failed'
  article_id: number
  job_id: number
  error: string
}

type TagWSMessage = TagCompletedMessage | TagFailedMessage

type TagResultHandler = (articleId: number, tags: ArticleTag[], jobId: number) => void
type TagErrorHandler = (articleId: number, error: string, jobId: number) => void

export function useTagWebSocket() {
  const ws = ref<WebSocket | null>(null)
  const connected = ref(false)
  const pendingArticleId = ref<number | null>(null)

  const resultHandlers = ref<TagResultHandler[]>([])
  const errorHandlers = ref<TagErrorHandler[]>([])

  function connect() {
    if (ws.value?.readyState === WebSocket.OPEN) return
    if (ws.value?.readyState === WebSocket.CONNECTING) return

    const wsBase = getApiOrigin().replace(/^http/, 'ws')
    const url = `${wsBase}/ws`

    ws.value = new WebSocket(url)

    ws.value.onopen = () => {
      connected.value = true
    }

    ws.value.onmessage = (event) => {
      try {
        const msg: TagWSMessage = JSON.parse(event.data)
        if (msg.type === 'tag_completed') {
          const tags: ArticleTag[] = msg.tags.map(t => ({
            slug: t.slug,
            label: t.label,
            category: t.category,
            score: t.score,
            icon: t.icon,
          }))
          for (const handler of resultHandlers.value) {
            handler(msg.article_id, tags, msg.job_id)
          }
          if (pendingArticleId.value === msg.article_id) {
            pendingArticleId.value = null
          }
        } else if (msg.type === 'tag_failed') {
          for (const handler of errorHandlers.value) {
            handler(msg.article_id, msg.error, msg.job_id)
          }
          if (pendingArticleId.value === msg.article_id) {
            pendingArticleId.value = null
          }
        }
      } catch {
        // ignore non-JSON or unrelated messages
      }
    }

    ws.value.onclose = () => {
      connected.value = false
      ws.value = null
    }

    ws.value.onerror = () => {
      connected.value = false
      ws.value = null
    }
  }

  function disconnect() {
    if (ws.value) {
      ws.value.close(1000, 'Manual disconnect')
      ws.value = null
      connected.value = false
    }
  }

  function onResult(handler: TagResultHandler) {
    resultHandlers.value.push(handler)
  }

  function onError(handler: TagErrorHandler) {
    errorHandlers.value.push(handler)
  }

  function watchArticle(articleId: number) {
    pendingArticleId.value = articleId
  }

  function clearWatch() {
    pendingArticleId.value = null
  }

  onMounted(() => connect())
  onUnmounted(() => disconnect())

  return {
    connected,
    pendingArticleId,
    onResult,
    onError,
    watchArticle,
    clearWatch,
    connect,
    disconnect,
  }
}
