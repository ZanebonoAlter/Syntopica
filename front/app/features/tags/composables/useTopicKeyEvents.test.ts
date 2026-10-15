import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import type { TopicKeyEventMarker } from '~/api/dailyReports'

// useTopicKeyEvents 持有模块级共享缓存（跨入口同步是它的产品语义），测试用
// resetModules + 动态 import 逐用例隔离单例状态。
type KeApiMock = { success?: boolean; status?: number; error?: string; data?: unknown }

const api = vi.hoisted(() => ({
  getReportKeyEvents: vi.fn<(id: number, force?: boolean) => Promise<KeApiMock>>(),
  getTopicKeyEvents: vi.fn<(id: number, days: number) => Promise<KeApiMock>>(),
  putThreadKeyEvent: vi.fn<(topicId: number, threadId: number, action: string, rev?: number) => Promise<KeApiMock>>(),
  patchKeyEventMarker: vi.fn<(topicId: number, markerId: number, action: string, rev: number) => Promise<KeApiMock>>(),
}))

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({
    getReportKeyEvents: api.getReportKeyEvents,
    getTopicKeyEvents: api.getTopicKeyEvents,
    putThreadKeyEvent: api.putThreadKeyEvent,
    patchKeyEventMarker: api.patchKeyEventMarker,
  }),
}))

async function freshComposable() {
  vi.resetModules()
  const mod = await import('./useTopicKeyEvents')
  return mod.useTopicKeyEvents()
}

function makeMarker(over: Partial<TopicKeyEventMarker> = {}): TopicKeyEventMarker {
  return {
    id: 11,
    semantic_board_id: 1,
    persistent_topic_id: 5,
    source_report_id: 77,
    source_section_id: 100,
    source_thread_id: 1001,
    source_period_date: '2026-10-09T12:00:00Z',
    topic_label_snapshot: '泳道',
    title_snapshot: '关键事件',
    summary_snapshot: '摘要',
    tag_ids_snapshot: [],
    article_ids_snapshot: [9],
    state: 'confirmed',
    revision: 1,
    ...over,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  api.getReportKeyEvents.mockResolvedValue({ success: true, data: { status: 'done', markers: [] } })
  api.getTopicKeyEvents.mockResolvedValue({ success: true, data: { topic_id: 5, days: 14, markers: [] } })
})

describe('useTopicKeyEvents — 每条请求锁 / revision / 幂等', () => {
  it('locks per thread: concurrent duplicate actions collapse into ONE request', async () => {
    let release!: (v: KeApiMock) => void
    api.putThreadKeyEvent.mockImplementation(() => new Promise<KeApiMock>(resolve => { release = resolve }))
    const ke = await freshComposable()
    await ke.ensureReportKeyEvents(77)

    const p1 = ke.actThread(5, 1001, 'mark')
    const p2 = ke.actThread(5, 1001, 'mark')
    await flushPromises()
    expect(api.putThreadKeyEvent).toHaveBeenCalledTimes(1)
    release({ success: true, data: makeMarker() })
    await Promise.all([p1, p2])
    expect(api.putThreadKeyEvent).toHaveBeenCalledTimes(1)
  })

  it('sends expected_revision only when the known marker changes state', async () => {
    api.putThreadKeyEvent.mockResolvedValue({ success: true, data: makeMarker() })
    const ke = await freshComposable()
    // 无已知注记 → 首建不带 revision。
    await ke.actThread(5, 1001, 'mark')
    expect(api.putThreadKeyEvent).toHaveBeenLastCalledWith(5, 1001, 'mark', undefined)

    // 已知 confirmed → 再 mark 是同目标幂等，直接本地判定，不发请求。
    api.putThreadKeyEvent.mockClear()
    await ke.actThread(5, 1001, 'mark')
    expect(api.putThreadKeyEvent).not.toHaveBeenCalled()

    // confirmed → dismiss 变更状态 → 携带最新 revision。
    api.putThreadKeyEvent.mockResolvedValue({ success: true, data: makeMarker({ state: 'dismissed', revision: 2 }) })
    await ke.actThread(5, 1001, 'dismiss')
    expect(api.putThreadKeyEvent).toHaveBeenLastCalledWith(5, 1001, 'dismiss', 1)
  })

  it('adopts the server latest marker on 409 and surfaces a retry hint', async () => {
    api.putThreadKeyEvent.mockResolvedValueOnce({
      success: false, status: 409, error: 'revision 冲突',
      data: makeMarker({ state: 'dismissed', revision: 7 }),
    })
    const ke = await freshComposable()
    await ke.ensureReportKeyEvents(77)
    await ke.actThread(5, 1001, 'mark')

    const view = ke.threadView(1001)
    expect(view.marker?.revision).toBe(7)
    expect(view.marker?.state).toBe('dismissed')
    expect(view.error).toContain('更新')
  })
})

describe('useTopicKeyEvents — 乐观待保存 / 回滚 / 未知结果重读', () => {
  it('rolls back to the last known state on definite failure (4xx)', async () => {
    api.putThreadKeyEvent.mockResolvedValue({ success: false, status: 400, error: '来源已失效' })
    const ke = await freshComposable()
    const known = makeMarker()
    api.getReportKeyEvents.mockResolvedValue({ success: true, data: { status: 'done', markers: [known] } })
    await ke.ensureReportKeyEvents(77)

    await ke.actThread(5, 1001, 'dismiss')
    const view = ke.threadView(1001)
    expect(view.marker?.state).toBe('confirmed') // 已知状态保持（回滚）
    expect(view.pending).toBeNull()
    expect(view.error).toContain('来源已失效')
  })

  it('re-reads the report after an UNKNOWN outcome (network error without status)', async () => {
    api.putThreadKeyEvent.mockResolvedValueOnce({ success: false, error: '网络错误' } as never)
    api.getReportKeyEvents.mockClear()
    api.getReportKeyEvents.mockResolvedValue({ success: true, data: { status: 'done', markers: [makeMarker({ state: 'dismissed', revision: 3 })] } })
    const ke = await freshComposable()
    await ke.ensureReportKeyEvents(77)

    await ke.actThread(5, 1001, 'mark')
    // retry 只是缓存旁路参数，API 层始终只按 reportId 重拉（计数 2 = 初拉 + 重读）。
    expect(api.getReportKeyEvents).toHaveBeenCalledTimes(2)
    expect(api.getReportKeyEvents).toHaveBeenLastCalledWith(77)
    const view = ke.threadView(1001)
    expect(view.marker?.state).toBe('dismissed') // 重读后采纳服务端真状态
    expect(view.error).toContain('未知')
  })

  it('marks an in-flight action optimistically (pending view) and clears it after', async () => {
    let release!: (v: KeApiMock) => void
    api.putThreadKeyEvent.mockImplementation(() => new Promise<KeApiMock>(resolve => { release = resolve }))
    const ke = await freshComposable()
    await ke.ensureReportKeyEvents(77)

    const p = ke.actThread(5, 1001, 'mark')
    await flushPromises()
    expect(ke.threadView(1001).pending).toBe('mark')
    release({ success: true, data: makeMarker() })
    await p
    expect(ke.threadView(1001).pending).toBeNull()
  })
})

describe('useTopicKeyEvents — 跨入口缓存同步 / 迟到响应', () => {
  it('shares one cache across two composable consumers (report read syncs to topic projection)', async () => {
    api.getTopicKeyEvents.mockResolvedValue({ success: true, data: { topic_id: 5, days: 14, markers: [] } })
    const keA = await freshComposable()
    const keB = (await import('./useTopicKeyEvents')).useTopicKeyEvents()
    // 句柄各自新建，但底层模块级缓存共享（跨入口同步的载体）。

    // topic 读端拉到 confirmed；随后日报读端 unmark → topic 投影即时退出。
    api.getTopicKeyEvents.mockResolvedValue({ success: true, data: { topic_id: 5, days: 14, markers: [makeMarker()] } })
    await keA.ensureTopicKeyEvents(5)
    expect(keB.topicEntry(5).markers).toHaveLength(1)

    api.patchKeyEventMarker.mockResolvedValue({ success: true, data: makeMarker({ state: 'dismissed', revision: 2 }) })
    await keB.actMarker(5, 11, 'unmark')
    expect(keA.topicEntry(5).markers).toHaveLength(0) // 写动作跨入口同步
  })

  it('a late report read never clobbers another owner (keyed caches)', async () => {
    let releaseA!: () => void
    api.getReportKeyEvents.mockImplementationOnce(() => new Promise(resolve => {
      releaseA = () => resolve({ success: true, data: { status: 'done', markers: [] } })
    }))
    const ke = await freshComposable()
    const pA = ke.ensureReportKeyEvents(77)

    // 另一份报告的读取先完成。
    api.getReportKeyEvents.mockResolvedValueOnce({ success: true, data: { status: 'done', markers: [makeMarker({ id: 99, source_report_id: 78, source_thread_id: 1002 })] } })
    await ke.ensureReportKeyEvents(78)
    expect(ke.reportEntry(78).markers).toHaveLength(1)

    // 迟到的 77 读取随后落盘：只写 77 的键。
    releaseA()
    await pA
    expect(ke.reportEntry(77).markers).toHaveLength(0)
    expect(ke.reportEntry(78).markers).toHaveLength(1)
  })
})
