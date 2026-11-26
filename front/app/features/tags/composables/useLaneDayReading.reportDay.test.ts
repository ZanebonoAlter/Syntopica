/**
 * useLaneDayReading「报告优先」读取子套件（8C G1，8C.4S 结构规范包从
 * useLaneDayReading.test.ts 拆出）。
 *
 *  - ensureReportDay 不依赖选中 lane / 当日桶：精确报日 ready、0 分区合法、无日报与索引/详情
 *    失败分层（errorSource）、失败不入成功缓存
 *  - RDC-2/3/4b 共享详情缓存与 lane/report 独立 epoch（互不取消；跨板块/跨日晚到丢弃；
 *    同日刷新失败 stale 保留；跨日首读先清显示）
 *  - RDC-5 报告优先路径继承 14/20 有界缓存与稳定显示（15 报日 FIFO、切板块全清）
 *  - R24 suspendDisplay 失效在途显示（报告优先级同口径）
 *
 * fixture/harness 见 `laneDayReadingTestHarness.ts`；索引请求/覆盖见
 * `useLaneDayReading.reportIndex.test.ts`。
 */
import { describe, expect, it, vi } from 'vitest'
import type { LaneDayReportDetail, LaneDayReportIndexItem } from '~/api/normalizers/laneDayReading'
import {
  deferred,
  harness,
  indexItems,
  makeDay,
  okList,
  reportDetail,
} from './laneDayReadingTestHarness'

describe('ensureReportDay — 报告优先读取（不依赖选中 lane / 当日桶）', () => {
  it('精确报日命中：ready + 该日报全部 sections（含泳道外分区）', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211'], ['2026-10-20', '1210']])))
    h.detail.mockResolvedValue({
      success: true,
      data: reportDetail('1211', '2026-10-22', [
        { sectionId: '501', threads: [{ threadId: 't1', title: '泳道内线索' }] },
        { sectionId: '777', threads: [{ threadId: 't2', title: '泳道外线索' }] },
      ]),
    })
    await h.store.ensureReportDay('2026-10-22')
    const day = h.store.reportDay.value!
    expect(day.date).toBe('2026-10-22')
    expect(day.status).toBe('ready')
    expect(day.reportId).toBe('1211')
    expect(day.error).toBeNull()
    expect(day.errorSource).toBeNull()
    expect(day.sections.map(section => section.sectionId)).toEqual(['501', '777'])
    expect(day.sections[1]!.threads[0]!.title).toBe('泳道外线索')
    // 不经过 lane 读取路径（current 仍为空）
    expect(h.store.current.value).toBeNull()
  })

  it('合法 0 分区：ready + 空 sections（不是失败、不是无日报）', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211']])))
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', []) })
    await h.store.ensureReportDay('2026-10-22')
    expect(h.store.reportDay.value?.status).toBe('ready')
    expect(h.store.reportDay.value?.sections).toEqual([])
  })

  it('所选日无日报：no-report（不给 0 行、不误报失败）', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-20', '1210']])))
    await h.store.ensureReportDay('2026-10-22')
    expect(h.store.reportDay.value?.status).toBe('no-report')
    expect(h.store.reportDay.value?.reportId).toBeNull()
    expect(h.store.reportDay.value?.sections).toEqual([])
    expect(h.store.reportDay.value?.error).toBeNull()
    expect(h.detail).not.toHaveBeenCalled()
  })

  it('索引失败：error + errorSource=index；重试成功后 ready', async () => {
    const h = harness()
    h.list.mockResolvedValueOnce({ success: false, error: '列表挂了' })
    await h.store.ensureReportDay('2026-10-22')
    expect(h.store.reportDay.value?.status).toBe('error')
    expect(h.store.reportDay.value?.errorSource).toBe('index')
    expect(h.store.reportDay.value?.error).toBe('列表挂了')
    expect(h.store.reportDay.value?.sections).toEqual([])
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211']])))
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '777' }]) })
    await h.store.ensureReportDay('2026-10-22', { force: true })
    expect(h.store.reportDay.value?.status).toBe('ready')
  })

  it('详情失败：error + errorSource=detail；失败不入成功缓存，force 重试成功', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211']])))
    h.detail.mockRejectedValueOnce(new Error('详情挂了'))
    await h.store.ensureReportDay('2026-10-22')
    expect(h.store.reportDay.value?.status).toBe('error')
    expect(h.store.reportDay.value?.errorSource).toBe('detail')
    expect(h.store.reportDay.value?.reportId).toBe('1211')
    expect(h.store.reportCacheCount()).toBe(0)
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '777', threads: [{ threadId: 't1', title: '重试后线索' }] }]) })
    await h.store.ensureReportDay('2026-10-22', { force: true })
    expect(h.detail).toHaveBeenCalledTimes(2)
    expect(h.store.reportCacheCount()).toBe(1)
    expect(h.store.reportDay.value?.status).toBe('ready')
    expect(h.store.reportDay.value?.sections[0]?.threads[0]?.title).toBe('重试后线索')
  })
})

describe('RDC-2/3/4b — 共享详情缓存与 lane/report 独立 epoch', () => {
  it('同报告详情共享：泳道编年与其他话题同日报日仅 1 次 GET', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211']])))
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501' }]) })
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    await h.store.ensureReportDay('2026-10-22')
    expect(h.detail).toHaveBeenCalledTimes(1)
    expect(h.store.reportCacheCount()).toBe(1)
    expect(h.store.reportDay.value?.status).toBe('ready')
    expect(h.store.current.value?.status).toBe('ready')
  })

  it('lane 与 report 各自 epoch：报告读取不取消在途 lane 读取，两路都落地', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211'], ['2026-10-20', '1210']])))
    const laneGate = deferred<{ success: boolean, data?: LaneDayReportDetail }>()
    h.detail.mockImplementationOnce(() => laneGate.promise)
    h.detail.mockImplementation((reportId: string) => Promise.resolve({
      success: true,
      data: reportDetail(reportId, 'x', [{ sectionId: '777', threads: [{ threadId: `t-${reportId}`, title: `线索${reportId}` }] }]),
    }))
    const laneRead = h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    await vi.waitFor(() => expect(h.detail).toHaveBeenCalledTimes(1))
    await h.store.ensureReportDay('2026-10-20')
    expect(h.store.reportDay.value?.date).toBe('2026-10-20')
    expect(h.store.reportDay.value?.status).toBe('ready')
    laneGate.resolve({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501', threads: [{ threadId: 't1', title: 'lane 线索' }] }]) })
    await laneRead
    expect(h.store.current.value?.date).toBe('2026-10-22')
    expect(h.store.current.value?.sections[0]?.threads[0]?.title).toBe('lane 线索')
  })

  it('切板块：lane 与 report 在途晚到都被丢弃，新板块不被污染', async () => {
    const h = harness()
    const listGate = deferred<{ success: boolean, data?: LaneDayReportIndexItem[] }>()
    h.list.mockReturnValueOnce(listGate.promise)
    const lanePending = h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    const reportPending = h.store.ensureReportDay('2026-10-22')
    h.store.resetForBoard()
    listGate.resolve(okList(indexItems([['2026-10-22', '1211']])))
    await Promise.all([lanePending, reportPending])
    expect(h.store.indexDays()).toBe(0)
    expect(h.store.reportDates.value.size).toBe(0)
    expect(h.store.current.value).toBeNull()
    expect(h.store.reportDay.value).toBeNull()
    expect(h.detail).not.toHaveBeenCalled()
  })

  it('跨日报日：旧报日晚到不覆盖新报日（reportDay.date gate）', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211'], ['2026-10-20', '1210']])))
    const slow = deferred<{ success: boolean, data?: LaneDayReportDetail }>()
    h.detail.mockImplementationOnce(() => slow.promise)
    const first = h.store.ensureReportDay('2026-10-22')
    await vi.waitFor(() => expect(h.detail).toHaveBeenCalledTimes(1))
    h.detail.mockImplementation((reportId: string) => Promise.resolve({
      success: true,
      data: reportDetail(reportId, '2026-10-20', [{ sectionId: '777', threads: [{ threadId: 't2', title: '新日报线索' }] }]),
    }))
    await h.store.ensureReportDay('2026-10-20')
    expect(h.store.reportDay.value?.date).toBe('2026-10-20')
    slow.resolve({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501', threads: [{ threadId: 't1', title: '旧日报线索' }] }]) })
    await first
    expect(h.store.reportDay.value?.date).toBe('2026-10-20')
    expect(h.store.reportDay.value?.sections[0]?.threads[0]?.title).toBe('新日报线索')
  })

  it('同日报日刷新失败：stale 保留旧内容可读；刷新成功后清 stale', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211']])))
    h.detail.mockResolvedValueOnce({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '777', threads: [{ threadId: 't1', title: '旧分区线索' }] }]) })
    await h.store.ensureReportDay('2026-10-22')
    h.detail.mockRejectedValueOnce(new Error('刷新失败'))
    await h.store.ensureReportDay('2026-10-22', { force: true })
    expect(h.store.reportStale.value).toBe(true)
    expect(h.store.reportDay.value?.status).toBe('ready')
    expect(h.store.reportDay.value?.sections[0]?.threads[0]?.title).toBe('旧分区线索')
    expect(h.store.reportDay.value?.error).toBe('刷新失败')
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '777', threads: [{ threadId: 't1', title: '新分区线索' }] }]) })
    await h.store.ensureReportDay('2026-10-22', { force: true })
    expect(h.store.reportStale.value).toBe(false)
    expect(h.store.reportDay.value?.error).toBeNull()
    expect(h.store.reportDay.value?.sections[0]?.threads[0]?.title).toBe('新分区线索')
  })

  it('跨日首读先清显示：新日期标题下不暂挂旧日 sections', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211'], ['2026-10-20', '1210']])))
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '777', threads: [{ threadId: 't1', title: '旧日线索' }] }]) })
    await h.store.ensureReportDay('2026-10-22')
    expect(h.store.reportDay.value?.sections).toHaveLength(1)
    const slow = deferred<{ success: boolean, data?: LaneDayReportDetail }>()
    h.detail.mockImplementationOnce(() => slow.promise)
    const pending = h.store.ensureReportDay('2026-10-20')
    expect(h.store.reportDay.value).toEqual({
      date: '2026-10-20', status: 'loading', error: null, errorSource: null, reportId: null, sections: [],
    })
    slow.resolve({ success: true, data: reportDetail('1210', '2026-10-20', []) })
    await pending
    expect(h.store.reportDay.value?.date).toBe('2026-10-20')
    expect(h.store.reportDay.value?.status).toBe('ready')
  })
})

describe('RDC-5 — 报告优先路径继承 14/20 有界缓存与稳定显示', () => {
  it('15 个报日：详情 FIFO 到 14 份、索引仍 1 次；当前 reportDay 内容不因逐出消失', async () => {
    const h = harness()
    const dates = Array.from({ length: 15 }, (_, i) => `2026-09-${String(i + 1).padStart(2, '0')}`)
    h.list.mockResolvedValue(okList(indexItems(dates.map((date, i) => [date, String(1000 + i)] as [string, string]))))
    h.detail.mockImplementation((reportId: string) => Promise.resolve({
      success: true,
      data: reportDetail(reportId, 'x', [{ sectionId: '777', threads: [{ threadId: `t-${reportId}`, title: `线索${reportId}` }] }]),
    }))
    for (const date of dates) await h.store.ensureReportDay(date)
    expect(h.list).toHaveBeenCalledTimes(1)
    expect(h.store.reportDates.value.size).toBe(15)
    expect(h.store.reportCacheCount()).toBe(14)
    const reading = h.store.reportDay.value!
    expect(reading.reportId).toBe('1014')
    // 再走一轮其他报日，旧显示对象仍完整
    await h.store.ensureReportDay(dates[0]!)
    expect(reading.sections[0]!.threads[0]!.title).toBe('线索1014')
  })

  it('切板块复位：reportDay/reportStale/indexError/reportDates 全清', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211']])))
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '777' }]) })
    await h.store.ensureReportDay('2026-10-22')
    expect(h.store.reportDay.value?.status).toBe('ready')
    h.store.resetForBoard()
    expect(h.store.reportDay.value).toBeNull()
    expect(h.store.reportStale.value).toBe(false)
    expect(h.store.indexError.value).toBeNull()
    expect(h.store.reportDates.value.size).toBe(0)
    expect(h.store.indexLoading.value).toBe(false)
  })
})

describe('R24 — suspendDisplay 失效在途显示（8C G1 报告优先级同口径）', () => {
  it('suspendDisplay 后旧在途响应晚到：不重新发布已离开的显示', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211']])))
    const slow = deferred<{ success: boolean, data?: LaneDayReportDetail }>()
    h.detail.mockImplementationOnce(() => slow.promise)
    const pending = h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    await vi.waitFor(() => expect(h.detail).toHaveBeenCalledTimes(1))
    h.store.suspendDisplay()
    slow.resolve({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501' }]) })
    await pending
    expect(h.store.current.value).toBeNull()
    // 成功响应仍入缓存：重入同上下文命中恢复，不重复 GET
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    expect(h.store.current.value?.status).toBe('ready')
    expect(h.detail).toHaveBeenCalledTimes(1)
  })
})

// ── 父复核 R1：索引强刷的新旧响应竞态（同板块请求序，2026-10-29） ─────────────
