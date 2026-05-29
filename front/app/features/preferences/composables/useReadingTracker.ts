import { ref, watch, onUnmounted, type Ref } from 'vue'
import { useIntervalFn, useScroll } from '@vueuse/core'
import type { Article } from '~/types'
import type { ReadingBehaviorEvent, ReadingEventType } from '~/types/reading_behavior'
import { useReadingBehaviorApi } from '~/api/reading_behavior'

interface ReadingTrackerOptions {
  article: Ref<Article | null>
  onTrack?: (event: ReadingBehaviorEvent) => void
}

export function useReadingTracker({ article, onTrack }: ReadingTrackerOptions) {
  const sessionId = ref(generateSessionId())
  const events = ref<ReadingBehaviorEvent[]>([])
  const readingStartTime = ref<Date | null>(null)
  const readingTime = ref(0)
  const api = useReadingBehaviorApi()

  let timer: ReturnType<typeof setInterval> | null = null
  const uploadInterval = 30000

  function generateSessionId(): string {
    return `session_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
  }

  async function trackEvent(
    eventType: ReadingEventType,
    scrollDepth = 0,
    currentReadingTime = 0
  ) {
    if (!article.value) return

    const event: ReadingBehaviorEvent = {
      article_id: Number(article.value.id),
      feed_id: Number(article.value.feedId),
      category_id: article.value.category ? Number(article.value.category) : undefined,
      session_id: sessionId.value,
      event_type: eventType,
      scroll_depth: scrollDepth,
      reading_time: currentReadingTime,
    }

    events.value.push(event)
    onTrack?.(event)

    if (eventType === 'close' || events.value.length >= 10) {
      await uploadEvents()
    }
  }

  async function uploadEvents() {
    if (events.value.length === 0) return

    const eventsToUpload = [...events.value]
    events.value = []

    try {
      await api.trackBehaviorBatch(eventsToUpload)
    } catch (error) {
      console.error('Failed to upload reading events:', error)
      events.value.unshift(...eventsToUpload)
    }
  }

  function startReadingTimer() {
    readingStartTime.value = new Date()
    readingTime.value = 0

    timer = setInterval(() => {
      if (readingStartTime.value) {
        readingTime.value = Math.floor((Date.now() - readingStartTime.value.getTime()) / 1000)
      }
    }, 1000)
  }

  function stopReadingTimer() {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
    readingStartTime.value = null
  }

  watch(
    article,
    (newArticle, oldArticle) => {
      if (newArticle && newArticle.id !== oldArticle?.id) {
        startReadingTimer()
        trackEvent('open', 0, 0)
      } else if (oldArticle && !newArticle) {
        stopReadingTimer()
        if (readingTime.value > 0) {
          trackEvent('close', 0, readingTime.value)
        }
        uploadEvents()
      }
    },
    { immediate: true }
  )

  const { pause: pauseInterval } = useIntervalFn(() => {
    uploadEvents()
  }, uploadInterval)

  onUnmounted(() => {
    stopReadingTimer()
    pauseInterval()
    uploadEvents()
  })

  return {
    sessionId,
    readingTime,
    events,
    trackEvent,
    uploadEvents,
    startReadingTimer,
    stopReadingTimer,
  }
}

export function useScrollDepthTracker(
  container: Ref<HTMLElement | undefined>,
  onScrollDepthChange: (depth: number) => void
) {
  const scrollDepth = ref(0)

  if (container.value) {
    const { y, arrivedState } = useScroll(container)

    watch(
      [y, arrivedState],
      () => {
        if (!container.value) return

        const scrollTop = y.value
        const scrollHeight = container.value.scrollHeight
        const clientHeight = container.value.clientHeight
        const maxScroll = scrollHeight - clientHeight
        const depth = maxScroll > 0 ? Math.round((scrollTop / maxScroll) * 100) : 0

        if (depth !== scrollDepth.value) {
          scrollDepth.value = depth
          onScrollDepthChange(depth)
        }
      },
      { immediate: true }
    )
  }

  return { scrollDepth }
}
