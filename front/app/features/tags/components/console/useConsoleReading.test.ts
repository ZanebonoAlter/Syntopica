/**
 * useConsoleReading 测试（8C G4a：壳层单份读取组合 + 受控重试/生成后刷新编排）。
 *
 * 用真实 G1 store（受控 fetchers spy）验证：索引初取与编年挂载解耦、锚点晚到覆盖扩大、
 * 选中日报告优先读取、retryReport 的 force 索引 → force 详情 → 稳定 section 核对、
 * 生成后 observer 懒失效（无选择零 GET / 有选择重读）、异步间切板块/切日不拉回旧内容。
 * 不重复 G1 的缓存/epoch 细节（那些留在 useLaneDayReading.*.test.ts）。
 */
import { describe, expect, it, vi } from 'vitest'
import { effectScope, ref, type Ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import type { LaneDayReadingResponse } from '~/api/laneDayReading'
import type {
  LaneDayArticleMeta,
  LaneDayReportDetail,
  LaneDayReportIndexItem,
} from '~/api/normalizers/laneDayReading'
import {
  ANCHOR,
  BOARD,
  TODAY,
  indexItems,
  okList,
  reportDetail,
} from '~/features/tags/composables/laneDayReadingTestHarness'
import { useConsoleReading } from './useConsoleReading'

const INDEX = indexItems([['2026-10-23', '900'], ['2026-10-24', '901']])
const AXIS = ['2026-10-22', '2026-10-23', '2026-10-24']

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => { resolve = res })
  return { promise, resolve }
}

function detailFor(reportId: string) {
  const date = reportId === '901' ? '2026-10-24' : '2026-10-23'
  return reportDetail(reportId, date, [
    { sectionId: '501', threads: [{ threadId: 't1', title: '线索一' }] },
    { sectionId: '502', threads: [] },
  ])
}

function makeReading(options: { anchor?: string | null } = {}) {
  const boardId = ref(BOARD)
  const anchor = ref<string | null>(options.anchor === undefined ? ANCHOR : options.anchor)
  const axis = AXIS
  const calls: string[] = []
  const list = vi.fn(async (_boardId: number, _days: number): Promise<LaneDayReadingResponse<LaneDayReportIndexItem[]>> => {
    calls.push('list')
    return okList(INDEX)
  })
  const detail = vi.fn(async (reportId: string): Promise<LaneDayReadingResponse<LaneDayReportDetail>> => {
    calls.push(`detail:${reportId}`)
    return { success: true, data: detailFor(reportId) }
  })
  const article = vi.fn(async (_articleId: string): Promise<LaneDayReadingResponse<LaneDayArticleMeta>> => (
    { success: false, error: 'not used' }
  ))
  const timeline = vi.fn(async () => ({
    success: true as const,
    data: { records: [], coverage: 'all' as const },
  }))
  const scope = effectScope()
  const reading = scope.run(() => useConsoleReading({
    boardId,
    today: () => TODAY,
    anchorDate: () => anchor.value,
    axis: () => axis,
    readingFetchers: { list, detail, article },
    observationFetchers: { timeline },
  }))!
  return { boardId, anchor, axis, calls, list, detail, timeline, scope, reading }
}

async function bind(h: ReturnType<typeof makeReading>, selectedDate: Ref<string | null>) {
  h.reading.bindSelectedDate(selectedDate)
  await flushPromises()
}

describe('useConsoleReading — 单份索引与选中日读取', () => {
  it('创建即取报日索引一次（不依赖编年挂载），lookback 覆盖 365 池', async () => {
    const h = makeReading()
    await flushPromises()
    expect(h.list).toHaveBeenCalledTimes(1)
    expect(h.list.mock.calls[0]![0]).toBe(BOARD)
    expect(h.list.mock.calls[0]![1]).toBeGreaterThanOrEqual(365)
    expect([...h.reading.reportDates.value].sort()).toEqual(['2026-10-23', '2026-10-24'])
    h.scope.stop()
  })

  it('停更板块锚点晚到：覆盖扩大重取（不缩水、不重复同覆盖）', async () => {
    const h = makeReading({ anchor: null })
    await flushPromises()
    const firstDays = h.list.mock.calls[0]![1] as number
    expect(firstDays).toBeGreaterThanOrEqual(365)

    h.anchor.value = '2025-10-01'
    await flushPromises()
    expect(h.list).toHaveBeenCalledTimes(2)
    expect(h.list.mock.calls[1]![1] as number).toBeGreaterThan(firstDays)

    // 同锚点再次变化不触发（watch 只在值变化时跑）；覆盖未变时不重复请求
    h.anchor.value = '2025-10-01'
    await flushPromises()
    expect(h.list).toHaveBeenCalledTimes(2)
    h.scope.stop()
  })

  it('bindSelectedDate：所选日读当日详情；同日重复不重复取；无报日不给详情', async () => {
    const h = makeReading()
    await flushPromises()
    const selectedDate = ref<string | null>(null)
    await bind(h, selectedDate)
    expect(h.detail).not.toHaveBeenCalled()

    selectedDate.value = '2026-10-24'
    await flushPromises()
    expect(h.detail).toHaveBeenCalledTimes(1)
    expect(h.detail).toHaveBeenCalledWith('901')
    expect(h.reading.reportDay.value).toMatchObject({ date: '2026-10-24', status: 'ready', reportId: '901' })

    selectedDate.value = '2026-10-24'
    await flushPromises()
    expect(h.detail).toHaveBeenCalledTimes(1)

    const before = h.detail.mock.calls.length
    selectedDate.value = '2026-10-22'
    await flushPromises()
    expect(h.detail.mock.calls.length).toBe(before)
    expect(h.reading.reportDay.value).toMatchObject({ date: '2026-10-22', status: 'no-report' })
    h.scope.stop()
  })

  it('索引失败只标 indexError（不清旧索引/不挡后续读取），重试恢复', async () => {
    const h = makeReading()
    await flushPromises()
    h.list.mockResolvedValueOnce({ success: false, error: '列表挂了' })
    await h.reading.ensureReportIndex({ force: true })
    expect(h.reading.indexError.value).toBe('列表挂了')
    expect([...h.reading.reportDates.value].sort()).toEqual(['2026-10-23', '2026-10-24'])

    await h.reading.ensureReportIndex({ force: true })
    expect(h.reading.indexError.value).toBeNull()
    h.scope.stop()
  })
})

describe('useConsoleReading — retryReport（force 索引 → force 详情 → 稳定 section 核对）', () => {
  it('顺序固定且不沿旧 topic 发观察请求；稳定 section 命中', async () => {
    const h = makeReading()
    const selectedDate = ref<string | null>(null)
    await bind(h, selectedDate)
    selectedDate.value = '2026-10-24'
    await flushPromises()
    h.calls.length = 0
    h.timeline.mockClear()

    const result = await h.reading.retryReport({ date: '2026-10-24', sectionId: '502' })
    expect(h.calls).toEqual(['list', 'detail:901'])
    expect(h.timeline).not.toHaveBeenCalled()
    expect(result).toEqual({ applied: true, sectionFound: true })
    h.scope.stop()
  })

  it('报告已更新缺该分区：sectionFound=false，不清选择（由 G4b 显示报告已更新）', async () => {
    const h = makeReading()
    const selectedDate = ref<string | null>(null)
    await bind(h, selectedDate)
    selectedDate.value = '2026-10-24'
    await flushPromises()

    const result = await h.reading.retryReport({ date: '2026-10-24', sectionId: '599' })
    expect(result).toEqual({ applied: true, sectionFound: false })
    h.scope.stop()
  })

  it('详情失败且无旧内容：sectionFound=null（未知，不判缺失），不改选中日；同日报旧内容保留时仍按旧内容核对', async () => {
    const h = makeReading()
    const selectedDate = ref<string | null>(null)
    await bind(h, selectedDate)
    h.detail.mockResolvedValueOnce({ success: false as const, error: '详情挂了' })
    selectedDate.value = '2026-10-24'
    await flushPromises()
    expect(h.reading.reportDay.value?.status).toBe('error')

    h.detail.mockResolvedValueOnce({ success: false as const, error: '详情挂了' })
    const failed = await h.reading.retryReport({ date: '2026-10-24', sectionId: '502' })
    expect(failed).toEqual({ applied: true, sectionFound: null })
    expect(selectedDate.value).toBe('2026-10-24')

    // 已成功读过同日：强刷失败保留旧内容（stale），稳定 section 仍在可读旧内容里
    h.detail.mockResolvedValueOnce({ success: true as const, data: detailFor('901') })
    await h.reading.retryReport({ date: '2026-10-24', sectionId: '502' })
    h.detail.mockResolvedValueOnce({ success: false as const, error: '详情挂了' })
    const stale = await h.reading.retryReport({ date: '2026-10-24', sectionId: '502' })
    expect(stale).toEqual({ applied: true, sectionFound: true })
    expect(h.reading.reportStale.value).toBe(true)
    h.scope.stop()
  })

  it('异步间切板块：旧核对结果不适用（applied=false），旧板块详情不发布到新板块', async () => {
    const h = makeReading()
    const selectedDate = ref<string | null>(null)
    await bind(h, selectedDate)
    selectedDate.value = '2026-10-24'
    await flushPromises()

    const gate = deferred<{ success: true, data: ReturnType<typeof detailFor> }>()
    h.detail.mockReturnValueOnce(gate.promise)
    const running = h.reading.retryReport({ date: '2026-10-24', sectionId: '502' })
    await flushPromises()

    h.boardId.value = 1980
    gate.resolve({ success: true, data: detailFor('901') })
    await running
    expect(h.reading.reportDay.value).toBeNull()
    h.scope.stop()
  })
})

describe('useConsoleReading — 旧日重试不得抢写新选中日（G4b 入场回归）', () => {
  it('重试 A 的 force 索引在途时切到 B（B 已读就绪）：索引返回后不再读/发布 A，B 终态保持', async () => {
    const h = makeReading()
    const selectedDate = ref<string | null>(null)
    await bind(h, selectedDate)
    selectedDate.value = '2026-10-23'
    await flushPromises()
    expect(h.reading.reportDay.value).toMatchObject({ date: '2026-10-23', status: 'ready', reportId: '900' })

    const gate = deferred<{ success: true, data: LaneDayReportIndexItem[] }>()
    h.list.mockReturnValueOnce(gate.promise)
    const running = h.reading.retryReport({ date: '2026-10-23', sectionId: '501' })
    await Promise.resolve()

    // 用户切到 B：报告优先读取发布 B
    selectedDate.value = '2026-10-24'
    await flushPromises()
    expect(h.reading.reportDay.value).toMatchObject({ date: '2026-10-24', status: 'ready', reportId: '901' })
    h.calls.length = 0

    gate.resolve({ success: true, data: INDEX })
    const result = await running

    // 在触发旧 date 读取前就停止：不重读 A 详情，不把 reportDay 抢回 A，B 仍是当前终态
    expect(result).toEqual({ applied: false, sectionFound: null })
    expect(h.calls).toEqual([])
    expect(h.reading.reportDay.value).toMatchObject({ date: '2026-10-24', status: 'ready', reportId: '901' })
    h.scope.stop()
  })

  it('重试 A 索引在途时选中日被清空（切板块前的清场）：不在旧日读取/发布，applied=false', async () => {
    const h = makeReading()
    const selectedDate = ref<string | null>(null)
    await bind(h, selectedDate)
    selectedDate.value = '2026-10-23'
    await flushPromises()

    const gate = deferred<{ success: true, data: LaneDayReportIndexItem[] }>()
    h.list.mockReturnValueOnce(gate.promise)
    const running = h.reading.retryReport({ date: '2026-10-23', sectionId: '501' })
    await Promise.resolve()

    selectedDate.value = null
    await flushPromises()
    h.calls.length = 0

    gate.resolve({ success: true, data: INDEX })
    const result = await running
    expect(result).toEqual({ applied: false, sectionFound: null })
    expect(h.calls).toEqual([])
    h.scope.stop()
  })
})

describe('useConsoleReading — 生成完成/显式刷新', () => {
  it('无其他行选择：force 索引 + 当前阅读，观察只 invalidate（零观察 GET）', async () => {
    const h = makeReading()
    const selectedDate = ref<string | null>(null)
    await bind(h, selectedDate)
    selectedDate.value = '2026-10-24'
    await flushPromises()
    h.calls.length = 0
    h.timeline.mockClear()

    await h.reading.refreshAfterRebuild({ date: () => selectedDate.value, otherTopicId: () => null })
    expect(h.calls).toEqual(['list', 'detail:901'])
    expect(h.timeline).not.toHaveBeenCalled()
    expect(h.reading.observation.hasDataset()).toBe(false)
    h.scope.stop()
  })

  it('有其他行选择：invalidate 后按当前 topic 重读观察（不沿旧 topic 之外发请求）', async () => {
    const h = makeReading()
    const selectedDate = ref<string | null>(null)
    await bind(h, selectedDate)
    selectedDate.value = '2026-10-24'
    await flushPromises()
    h.timeline.mockClear()

    await h.reading.refreshAfterRebuild({ date: () => selectedDate.value, otherTopicId: () => '77' })
    expect(h.timeline).toHaveBeenCalledTimes(1)
    expect(h.reading.observation.view.value.status).toBe('ready')
    expect(h.reading.observation.view.value.topicId).toBe('77')
    h.scope.stop()
  })

  it('索引 force 在途时切日：按最新选中日重读（不把旧日报回填新标题）', async () => {
    const h = makeReading()
    const selectedDate = ref<string | null>(null)
    await bind(h, selectedDate)
    selectedDate.value = '2026-10-24'
    await flushPromises()

    const gate = deferred<{ success: true, data: LaneDayReportIndexItem[] }>()
    h.list.mockReturnValueOnce(gate.promise)
    h.detail.mockClear()
    const running = h.reading.refreshAfterRebuild({ date: () => selectedDate.value, otherTopicId: () => null })
    await Promise.resolve()

    selectedDate.value = '2026-10-23'
    gate.resolve({ success: true, data: INDEX })
    await running

    expect(h.detail).not.toHaveBeenCalledWith('901')
    expect(h.detail).toHaveBeenCalledWith('900')
    expect(h.reading.reportDay.value).toMatchObject({ date: '2026-10-23', status: 'ready' })
    h.scope.stop()
  })
})
