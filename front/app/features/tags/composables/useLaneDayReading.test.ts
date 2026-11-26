/**
 * useLaneDayReading 测试（矩阵阶段 G4，test-cases-matrix-stage.md RD/CA 组）——lane 读取主路。
 *
 *  - RD-1/2/3 列表 1 份·详情/文章缓存命中不重复请求
 *  - RD-4 epoch：切板块/切日在途晚到丢弃（不渲染旧板块/旧日内容；跨板块迟到不回填缓存）
 *  - RD-5 失败分层与隔离（列表/详情/文章各失败互不污染成功缓存）
 *  - 跨上下文防错配：新日期标题下不暂挂旧日摘要；同上下文刷新失败 stale 保留可读
 *  - R24 离开可读上下文失效显示 / sectionKey 上下文隔离
 *
 * 8C.4S 结构规范包按内聚子套件拆分：报日索引见 `useLaneDayReading.reportIndex.test.ts`、
 * 报告优先读取见 `useLaneDayReading.reportDay.test.ts`、有界缓存见
 * `useLaneDayReading.cache.test.ts`、相关文章晚到见 `useLaneDayReading.relatedArticles.test.ts`；
 * fixture/harness 在 `laneDayReadingTestHarness.ts`（断言与用例名不变）。
 */
import { describe, expect, it, vi } from 'vitest'
import type { LaneDayReportDetail, LaneDayReportIndexItem } from '~/api/normalizers/laneDayReading'
import {
  articleMeta,
  deferred,
  harness,
  indexItems,
  makeDay,
  okList,
  reportDetail,
} from './laneDayReadingTestHarness'

describe('RD-1/2/3 — 列表 1 份与详情/文章缓存', () => {
  it('同板块两个不同日：列表仅 1 次、每报告详情仅 1 次', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211'], ['2026-10-20', '1210']])))
    h.detail.mockImplementation((reportId: string) =>
      Promise.resolve({ success: true, data: reportDetail(reportId, reportId === '1211' ? '2026-10-22' : '2026-10-20', [{ sectionId: '501', threads: [{ threadId: 't1', title: '线索A' }] }]) }))
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-20' }, makeDay('2026-10-20'))
    // 再切回第一日：全部命中缓存
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    expect(h.list).toHaveBeenCalledTimes(1)
    expect(h.detail).toHaveBeenCalledTimes(2)
    expect(h.store.current.value?.status).toBe('ready')
  })

  it('同文章跨 thread 展开复用：getArticle 仅 1 次', async () => {
    const h = harness()
    h.article.mockImplementation((articleId: string) => Promise.resolve({ success: true, data: articleMeta(articleId) }))
    await h.store.ensureRelatedArticles(['5', '6'])
    await h.store.ensureRelatedArticles(['5', '7'])
    expect(h.article).toHaveBeenCalledTimes(3)
    expect(h.store.readRelated('5').status).toBe('success')
    expect(h.store.articleCacheCount()).toBe(3)
  })
})

describe('RD-4 — epoch：切板块/切日迟到丢弃', () => {
  it('切板块后旧列表响应晚到：不回填索引、不渲染旧板块内容', async () => {
    const h = harness()
    const pending = deferred<{ success: boolean, data?: LaneDayReportIndexItem[] }>()
    h.list.mockReturnValueOnce(pending.promise)
    const first = h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    // 切板块（resetForBoard：epoch +1、全部清空）
    h.store.resetForBoard()
    pending.resolve(okList(indexItems([['2026-10-22', '1211']])))
    await first
    expect(h.store.indexDays()).toBe(0)
    expect(h.store.current.value).toBeNull()
  })

  it('切日后旧详情晚到：显示保持新日；同板块旧详情允许只入缓存', async () => {
    const h = harness()
    const listGate = deferred<{ success: boolean, data?: LaneDayReportIndexItem[] }>()
    const slowDetail = deferred<{ success: boolean, data?: LaneDayReportDetail }>()
    h.list.mockReturnValueOnce(listGate.promise) // day1 的列表受控放行
    h.detail.mockImplementationOnce(() => slowDetail.promise) // day1 的详情悬着
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211'], ['2026-10-20', '1210']])))
    h.detail.mockImplementation((reportId: string) => Promise.resolve({ success: true, data: reportDetail(reportId, '2026-10-20', [{ sectionId: '501', threads: [{ threadId: 't2', title: '新日线索' }] }]) }))
    const day1 = h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    listGate.resolve(okList(indexItems([['2026-10-22', '1211'], ['2026-10-20', '1210']])))
    // 等 day1 走到详情在途（seq=1）
    await vi.waitFor(() => expect(h.detail).toHaveBeenCalledTimes(1))
    const day2 = h.store.ensureDayReading({ laneKey: '10', date: '2026-10-20' }, makeDay('2026-10-20'))
    await day2
    expect(h.store.current.value?.date).toBe('2026-10-20')
    slowDetail.resolve({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501', threads: [{ threadId: 't1', title: '旧日线索' }] }]) })
    await day1
    // 旧日详情不覆盖新日显示（同板块迟到只入缓存）
    expect(h.store.current.value?.date).toBe('2026-10-20')
    expect(h.store.current.value?.sections[0]?.threads[0]?.title).toBe('新日线索')
  })
})

describe('RD-5 — 失败分层与隔离', () => {
  it('列表失败：error 态 + 重试成功；索引不缓存失败', async () => {
    const h = harness()
    h.list.mockResolvedValueOnce({ success: false, error: '日报列表挂了' })
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    expect(h.store.current.value?.status).toBe('error')
    expect(h.store.current.value?.error).toBe('日报列表挂了')
    expect(h.store.indexDays()).toBe(0)
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211']])))
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501' }]) })
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    expect(h.store.current.value?.status).toBe('ready')
  })

  it('详情失败：error 态；成功缓存不受失败污染', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211'], ['2026-10-20', '1210']])))
    h.detail.mockImplementation((reportId: string) =>
      reportId === '1210'
        ? Promise.resolve({ success: true, data: reportDetail('1210', '2026-10-20', [{ sectionId: '501' }]) })
        : Promise.reject(new Error('detail boom')))
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-20' }, makeDay('2026-10-20'))
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    expect(h.store.current.value?.status).toBe('error')
    expect(h.store.reportCacheCount()).toBe(1) // 只含成功的 1210
  })

  it('文章失败：条目 error + 单项重试成功（总数语义在组件层）', async () => {
    const h = harness()
    h.article.mockImplementation((articleId: string) =>
      articleId === '9' ? Promise.resolve({ success: false, error: '404' }) : Promise.resolve({ success: true, data: articleMeta(articleId) }))
    await h.store.ensureRelatedArticles(['9', '8'])
    expect(h.store.readRelated('9')).toEqual({ status: 'error', error: '404' })
    expect(h.store.readRelated('8').status).toBe('success')
    expect(h.store.articleCacheCount()).toBe(1)
    h.article.mockResolvedValue({ success: true, data: articleMeta('9') })
    await h.store.retryRelatedArticle('9')
    expect(h.store.readRelated('9').status).toBe('success')
  })
})

describe('join 分层 — degraded / no-report / 跨上下文 / stale', () => {
  it('报告存在但 section 缺失：degraded + base 折叠数；force 重读后恢复', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211']])))
    h.detail.mockResolvedValueOnce({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '888', threads: [] }]) })
    const day = makeDay('2026-10-22', [{ sectionId: '501', foldedCount: 4, eventTitles: ['旧标题'] }])
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, day)
    const degraded = h.store.current.value!
    expect(degraded.status).toBe('degraded')
    expect(degraded.missingSectionCount).toBe(1)
    expect(degraded.foldedTotal).toBe(4)
    expect(degraded.sections[0]!.fallbackTitles).toEqual(['旧标题'])
    // 强制重读（报告已重建，501 回来了）
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501', threads: [{ threadId: 't1', title: '重建后的线索' }] }]) })
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, day, { force: true })
    expect(h.store.current.value?.status).toBe('ready')
    expect(h.store.current.value?.sections[0]?.threads[0]?.title).toBe('重建后的线索')
  })

  it('当天无报告：no-report（不误报为失败）', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-21', '1210']])))
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    expect(h.store.current.value?.status).toBe('no-report')
    expect(h.store.current.value?.reportId).toBeNull()
  })

  it('跨上下文：切日先清显示（新日期标题下无旧日摘要）', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211'], ['2026-10-20', '1210']])))
    h.detail.mockImplementation((reportId: string) => Promise.resolve({ success: true, data: reportDetail(reportId, 'x', [{ sectionId: '501', threads: [{ threadId: 't', title: `线索${reportId}` }] }]) }))
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    expect(h.store.current.value?.sections[0]?.threads).toHaveLength(1)
    const slow = deferred<{ success: boolean, data?: LaneDayReportDetail }>()
    h.detail.mockImplementationOnce(() => slow.promise as never)
    const pending = h.store.ensureDayReading({ laneKey: '10', date: '2026-10-20' }, makeDay('2026-10-20'))
    // 在途期间：current 已清（null），不暂挂 10-22 的内容
    expect(h.store.current.value).toBeNull()
    slow.resolve({ success: true, data: reportDetail('1210', '2026-10-20', [{ sectionId: '501', threads: [{ threadId: 't', title: '新日' }] }]) })
    await pending
    expect(h.store.current.value?.sections[0]?.threads[0]?.title).toBe('新日')
  })

  it('同上下文刷新失败：stale 保留旧数据可读；成功后 stale 清除', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211']])))
    h.detail.mockResolvedValueOnce({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501', threads: [{ threadId: 't1', title: '旧内容' }] }]) })
    const day = makeDay('2026-10-22')
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, day)
    h.detail.mockRejectedValueOnce(new Error('刷新失败'))
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, day, { force: true })
    expect(h.store.staleNotice.value).toBe(true)
    expect(h.store.current.value?.sections[0]?.threads[0]?.title).toBe('旧内容')
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501', threads: [{ threadId: 't1', title: '新内容' }] }]) })
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, day, { force: true })
    expect(h.store.staleNotice.value).toBe(false)
    expect(h.store.current.value?.sections[0]?.threads[0]?.title).toBe('新内容')
  })
})

describe('R24 — 离开可读上下文失效显示 / sectionKey 上下文隔离', () => {
  it('suspendDisplay：失效在途显示、不动成功缓存；重入同上下文缓存命中恢复', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211']])))
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501', threads: [{ threadId: 't1', title: '线索' }] }]) })
    const day = makeDay('2026-10-22')
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, day)
    expect(h.store.current.value?.status).toBe('ready')
    h.store.suspendDisplay()
    expect(h.store.current.value).toBeNull()
    expect(h.store.staleNotice.value).toBe(false)
    expect(h.store.error.value).toBeNull()
    // 成功缓存不被清（列表/详情仍在）
    expect(h.store.indexDays()).toBeGreaterThan(0)
    expect(h.store.reportCacheCount()).toBe(1)
    // 重入同上下文：缓存命中恢复，不重复请求
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, day)
    expect(h.store.current.value?.status).toBe('ready')
    expect(h.detail).toHaveBeenCalledTimes(1)
    expect(h.list).toHaveBeenCalledTimes(1)
  })

  it('sectionKey 变化即跨上下文：同日换分区集后刷新失败不 stale 保留旧分区内容', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211']])))
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501', threads: [{ threadId: 't1', title: '旧分区线索' }] }]) })
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22', [{ sectionId: '501' }]))
    expect(h.store.current.value?.status).toBe('ready')
    // 同日同泳道换分区集（777 不在详情）+ force 详情失败：上下文已变 → error 态，不 stale 保留旧分区
    h.detail.mockRejectedValueOnce(new Error('刷新失败'))
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22', [{ sectionId: '777' }]), { force: true })
    expect(h.store.current.value?.status).toBe('error')
    expect(h.store.staleNotice.value).toBe(false)
    expect(h.store.current.value?.sections[0]?.sectionId).toBe('777')
  })
})
