import { defineComponent, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import { useWebSocketRebuild } from './useWebSocketRebuild'

vi.mock('~/utils/api', () => ({
  getApiOrigin: () => 'http://localhost:5000',
}))

class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static last: MockWebSocket | null = null

  readyState = MockWebSocket.OPEN
  onmessage: ((event: MessageEvent) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null

  constructor(public url: string) {
    MockWebSocket.last = this
  }

  close() {
    this.readyState = 3
    this.onclose?.()
  }

  emitMessage(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent)
  }
}

describe('useWebSocketRebuild', () => {
  beforeEach(() => {
    MockWebSocket.last = null
    vi.stubGlobal('WebSocket', MockWebSocket)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('updates state from processing hierarchy_rebuild messages', async () => {
    const exposed = mountProbe()
    await nextTick()

    MockWebSocket.last?.emitMessage({
      type: 'hierarchy_rebuild',
      status: 'processing',
      job_id: 42,
      processed: 1,
      total: 3,
      category: 'event',
      failed_count: 1,
      estimated_remaining_seconds: 12.5,
      current_tag: 'Tag A',
    })

    expect(exposed.status.value).toBe('processing')
    expect(exposed.jobId.value).toBe(42)
    expect(exposed.category.value).toBe('event')
    expect(exposed.processed.value).toBe(1)
    expect(exposed.total.value).toBe(3)
    expect(exposed.failedCount.value).toBe(1)
    expect(exposed.estimatedRemainingSeconds.value).toBe(12.5)
    expect(exposed.currentTag.value).toBe('Tag A')
  })

  it('updates state from completed hierarchy_rebuild messages and clears stale error', async () => {
    const exposed = mountProbe()
    await nextTick()

    MockWebSocket.last?.emitMessage({
      type: 'hierarchy_rebuild',
      status: 'failed',
      processed: 2,
      total: 3,
      category: 'event',
      error: 'boom',
    })
    MockWebSocket.last?.emitMessage({
      type: 'hierarchy_rebuild',
      status: 'completed',
      job_id: 42,
      processed: 3,
      total: 3,
      category: 'event',
      failed_count: 0,
    })

    expect(exposed.status.value).toBe('completed')
    expect(exposed.processed.value).toBe(3)
    expect(exposed.total.value).toBe(3)
    expect(exposed.failedCount.value).toBe(0)
    expect(exposed.errorMessage.value).toBe('')
  })

  it('updates state from failed hierarchy_rebuild messages', async () => {
    const exposed = mountProbe()
    await nextTick()

    MockWebSocket.last?.emitMessage({
      type: 'hierarchy_rebuild',
      status: 'failed',
      job_id: 42,
      processed: 1,
      total: 3,
      category: 'event',
      failed_count: 1,
      error: 'query tags failed',
    })

    expect(exposed.status.value).toBe('failed')
    expect(exposed.jobId.value).toBe(42)
    expect(exposed.failedCount.value).toBe(1)
    expect(exposed.errorMessage.value).toBe('query tags failed')
  })
})

function mountProbe(): ReturnType<typeof useWebSocketRebuild> {
  let exposed: ReturnType<typeof useWebSocketRebuild> | undefined
  const Probe = defineComponent({
    setup() {
      exposed = useWebSocketRebuild()
      return () => null
    },
  })

  mount(Probe)
  if (!exposed) {
    throw new Error('probe did not expose composable state')
  }
  return exposed
}
