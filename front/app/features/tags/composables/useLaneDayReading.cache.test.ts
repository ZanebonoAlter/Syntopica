/**
 * useLaneDayReading 有界缓存子套件（矩阵阶段 G4 CA 组，8C.4S 结构规范包从
 * useLaneDayReading.test.ts 拆出）。
 *
 *  - CA-3 报告 0→14 份无提前逐出、命中不请求（失败不占位）
 *  - CA-4 第 15 份 FIFO 逐出最旧；被逐出项重读允许再请求
 *  - CA-5 文章 20→21 FIFO 逐出；命中复用
 *  - CA-7 切板块释放（索引/详情/文章/相关条目/当前显示）；卸载释放
 *  - CA-8 当前阅读对象独立持有——FIFO 逐出不消失
 *
 * fixture/harness 见 `laneDayReadingTestHarness.ts`。
 */
import { describe, expect, it } from 'vitest'
import { effectScope, ref } from 'vue'
import {
  ANCHOR,
  BOARD,
  TODAY,
  type Harness,
  articleMeta,
  harness,
  indexItems,
  makeDay,
  okList,
  reportDetail,
} from './laneDayReadingTestHarness'
import { useLaneDayReading } from './useLaneDayReading'

describe('CA-3/4/5/6/7/8 — 有界缓存', () => {
  function boardWithManyReports(h: Harness, dates: string[]): void {
    h.list.mockResolvedValue(okList(indexItems(dates.map((date, i) => [date, String(1000 + i)] as [string, string]))))
    h.detail.mockImplementation((reportId: string) =>
      Promise.resolve({ success: true, data: reportDetail(reportId, 'x', [{ sectionId: '501', threads: [{ threadId: `t-${reportId}`, title: `线索${reportId}` }] }]) }))
  }

  it('CA-3：报告 0→14 份无提前逐出、命中不请求', async () => {
    const h = harness()
    const dates = Array.from({ length: 14 }, (_, i) => `2026-09-${String(i + 1).padStart(2, '0')}`)
    boardWithManyReports(h, dates)
    for (const date of dates) await h.store.ensureDayReading({ laneKey: '10', date }, makeDay(date))
    expect(h.store.reportCacheCount()).toBe(14)
    await h.store.ensureDayReading({ laneKey: '10', date: dates[0]! }, makeDay(dates[0]!))
    expect(h.detail).toHaveBeenCalledTimes(14)
  })

  it('CA-4：第 15 份 FIFO 逐出最旧；被逐出项重读允许再请求', async () => {
    const h = harness()
    const dates = Array.from({ length: 15 }, (_, i) => `2026-09-${String(i + 1).padStart(2, '0')}`)
    boardWithManyReports(h, dates)
    for (const date of dates) await h.store.ensureDayReading({ laneKey: '10', date }, makeDay(date))
    expect(h.store.reportCacheCount()).toBe(14)
    await h.store.ensureDayReading({ laneKey: '10', date: dates[0]! }, makeDay(dates[0]!))
    expect(h.detail).toHaveBeenCalledTimes(16)
  })

  it('CA-5：文章 20→21 FIFO 逐出；命中复用', async () => {
    const h = harness()
    h.article.mockImplementation((articleId: string) => Promise.resolve({ success: true, data: articleMeta(articleId) }))
    const first20 = Array.from({ length: 20 }, (_, i) => String(i + 1))
    await h.store.ensureRelatedArticles(first20)
    expect(h.store.articleCacheCount()).toBe(20)
    await h.store.ensureRelatedArticles(['21'])
    expect(h.store.articleCacheCount()).toBe(20)
    // 第 1 篇被逐出后重读：允许再请求（共 20+1+1 次）
    await h.store.ensureRelatedArticles(['1'])
    expect(h.article).toHaveBeenCalledTimes(22)
  })

  it('CA-7：切板块释放（索引/详情/文章/相关条目/当前显示）；卸载释放', async () => {
    const h = harness()
    boardWithManyReports(h, ['2026-10-22'])
    h.article.mockResolvedValue({ success: true, data: articleMeta('5') })
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    await h.store.ensureRelatedArticles(['5'])
    expect(h.store.reportCacheCount()).toBe(1)
    h.store.resetForBoard()
    expect(h.store.reportCacheCount()).toBe(0)
    expect(h.store.articleCacheCount()).toBe(0)
    expect(h.store.indexDays()).toBe(0)
    expect(h.store.current.value).toBeNull()
    expect(h.store.readRelated('5').status).toBe('idle')

    // 卸载释放（effectScope stop = onScopeDispose）
    const scope = effectScope()
    const boardId2 = ref(BOARD)
    const h2 = scope.run(() => useLaneDayReading({
      boardId: boardId2,
      today: () => TODAY,
      anchorDate: () => ANCHOR,
      fetchers: { list: h.list, detail: h.detail, article: h.article },
    }))!
    await h2.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    expect(h2.reportCacheCount()).toBe(1)
    scope.stop()
    expect(h2.reportCacheCount()).toBe(0)
    expect(h2.current.value).toBeNull()
  })

  it('CA-8：当前阅读对象独立持有——FIFO 逐出不消失', async () => {
    const h = harness()
    const dates = ['2026-10-22', ...Array.from({ length: 15 }, (_, i) => `2026-09-${String(i + 1).padStart(2, '0')}`)]
    boardWithManyReports(h, dates)
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    // 组件持有的显示对象（current 模型内嵌 threads，独立于缓存 Map）
    const readingCopy = h.store.current.value!
    for (const date of dates.slice(1)) await h.store.ensureDayReading({ laneKey: '10', date }, makeDay(date))
    // 缓存已逐出 1000（第一份），但旧显示对象内容完整不消失/不回退
    expect(h.store.reportCacheCount()).toBe(14)
    expect(readingCopy.reportId).toBe('1000')
    expect(readingCopy.sections[0]!.threads[0]!.title).toBe('线索1000')
    // 重读该日：被逐出允许重新 GET，内容等价
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    expect(h.detail).toHaveBeenCalledTimes(17)
    expect(h.store.current.value?.sections[0]?.threads[0]?.title).toBe('线索1000')

    // 文章同口径：成功条目展示不因逐出回退
    h.article.mockImplementation((articleId: string) => Promise.resolve({ success: true, data: articleMeta(articleId) }))
    await h.store.ensureRelatedArticles(['77'])
    expect(h.store.readRelated('77').status).toBe('success')
    await h.store.ensureRelatedArticles(Array.from({ length: 21 }, (_, i) => String(200 + i)))
    expect(h.store.readRelated('77').status).toBe('success')
  })
})

// ── 8C G1：报日索引 / 报告优先读取 / 双 epoch（test-cases-matrix-day-others RDY·RDC） ──────
