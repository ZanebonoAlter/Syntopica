/**
 * useLaneDayReading 相关文章晚到子套件（父复核 R2，2026-10-29；8C.4S 结构规范包从
 * useLaneDayReading.test.ts 拆出）。
 *
 * 相关文章条目与文章缓存在切板块后：旧成功晚到不复活、缓存不回填；旧失败晚到不发布成
 * 新板块错误；同 ID 新板块先成功时旧板块晚到不覆盖。
 *
 * fixture/harness 见 `laneDayReadingTestHarness.ts`；卡片/展开渲染仍在组件测试。
 */
import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import type { LaneDayArticleMeta } from '~/api/normalizers/laneDayReading'
import { articleMeta, deferred, harness } from './laneDayReadingTestHarness'

describe('R2 — 相关文章跨板块晚到不发布', () => {
  it('resetForBoard 后旧成功晚到：relatedEntries 不复活、文章缓存不回填', async () => {
    const h = harness()
    const gate = deferred<{ success: boolean, data?: LaneDayArticleMeta }>()
    h.article.mockReturnValueOnce(gate.promise)
    const pending = h.store.ensureRelatedArticles(['5'])
    expect(h.store.readRelated('5').status).toBe('loading')
    h.store.resetForBoard()
    expect(h.store.readRelated('5').status).toBe('idle')
    gate.resolve({ success: true, data: articleMeta('5') })
    await pending
    expect(h.store.readRelated('5').status).toBe('idle')
    expect(h.store.articleCacheCount()).toBe(0)
    expect(h.store.getArticlePayload('5')).toBeNull()
  })

  it('切板块后旧失败晚到：不把「缓存已失效」发布成新板块错误', async () => {
    const h = harness()
    const gate = deferred<{ success: boolean, data?: LaneDayArticleMeta }>()
    h.article.mockReturnValueOnce(gate.promise)
    const pending = h.store.ensureRelatedArticles(['6'])
    h.boardId.value = 1975
    await nextTick()
    gate.reject(new Error('旧板块 404'))
    await pending
    expect(h.store.readRelated('6').status).toBe('idle')
    expect(h.store.articleCacheCount()).toBe(0)
  })

  it('同 ID 新板块先成功、旧板块晚到：不覆盖新板块条目与缓存', async () => {
    const h = harness()
    const oldGate = deferred<{ success: boolean, data?: LaneDayArticleMeta }>()
    h.article.mockReturnValueOnce(oldGate.promise)
    const oldRead = h.store.ensureRelatedArticles(['7'])
    h.boardId.value = 1975
    await nextTick()
    h.article.mockResolvedValue({ success: true, data: articleMeta('7', '新板块文章') })
    await h.store.ensureRelatedArticles(['7'])
    expect(h.store.readRelated('7').status).toBe('success')
    oldGate.resolve({ success: true, data: articleMeta('7', '旧板块文章') })
    await oldRead
    const after = h.store.readRelated('7')
    expect(after.status).toBe('success')
    expect(after.status === 'success' ? after.meta.title : '').toBe('新板块文章')
    expect(h.store.getArticlePayload('7')?.title).toBe('新板块文章')
    expect(h.store.articleCacheCount()).toBe(1)
    expect(h.article).toHaveBeenCalledTimes(2)
  })
})

// ── 父复核 R3：生成后 anchor 推进使索引需求覆盖收缩（369→365，2026-10-29） ──────
