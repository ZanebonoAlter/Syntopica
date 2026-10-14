import { describe, expect, it, vi } from 'vitest'
import type { LocationQuery } from 'vue-router'
import type { ApiResponse } from '~/types'
import { applyArticleDeepLink } from './useArticleDeepLink'

/**
 * /?article=<id> 深链编排四分支（add-article-search D1）：
 * 合法 id（详情直取 + normalize + onArticle）/ 非法 id / 缺参 / 详情失败静默。
 */

const payload = {
  id: 101,
  feed_id: 7,
  title: '特斯拉Model Y地下车库自燃',
  content: '<p>事故原因初步指向电池管线老化。</p>',
  link: 'https://example.com/a',
  pub_date: '2026-10-01T00:00:00Z',
  created_at: '2026-10-01T00:00:00Z',
  read: true,
  favorite: false,
}

function ok(data: unknown): ApiResponse<unknown> {
  return { success: true, data }
}

describe('applyArticleDeepLink — /?article=<id> 深链', () => {
  it('合法 id：按数字 id 直取详情，normalize（id 字符串化/camelCase）后交给 onArticle', async () => {
    const getArticle = vi.fn(async () => ok(payload))
    const onArticle = vi.fn()
    await applyArticleDeepLink({ article: '101' }, { getArticle, onArticle })

    expect(getArticle).toHaveBeenCalledTimes(1)
    expect(getArticle).toHaveBeenCalledWith(101)
    expect(onArticle).toHaveBeenCalledTimes(1)
    const article = onArticle.mock.calls[0]![0] as unknown as Record<string, unknown>
    expect(article.id).toBe('101')
    expect(article.feedId).toBe('7')
    expect(article.title).toBe('特斯拉Model Y地下车库自燃')
    expect(article.read).toBe(true)
  })

  it('非法 id（非数字/0/空串/数组）：不取详情不回调', async () => {
    for (const query of [{ article: 'abc' }, { article: '0' }, { article: '' }, { article: ['101'] }, { article: 101 as unknown as string }]) {
      const getArticle = vi.fn()
      const onArticle = vi.fn()
      await applyArticleDeepLink(query, { getArticle, onArticle })
      expect(getArticle).not.toHaveBeenCalled()
      expect(onArticle).not.toHaveBeenCalled()
    }
  })

  it('缺参：query 无 article 键或 query 为 undefined（无 router 上下文的单测直挂）：不取详情不回调', async () => {
    const cases: (LocationQuery | undefined)[] = [undefined, {}, { feed: '1' }]
    for (const query of cases) {
      const getArticle = vi.fn()
      const onArticle = vi.fn()
      await applyArticleDeepLink(query, { getArticle, onArticle })
      expect(getArticle).not.toHaveBeenCalled()
      expect(onArticle).not.toHaveBeenCalled()
    }
  })

  it('详情失败（success=false 或网络异常）：静默忽略不抛错、不回调', async () => {
    const onArticle = vi.fn()
    await applyArticleDeepLink(
      { article: '101' },
      { getArticle: vi.fn(async () => ({ success: false, error: 'not found' })), onArticle },
    )
    await applyArticleDeepLink(
      { article: '101' },
      { getArticle: vi.fn(async () => { throw new Error('network') }), onArticle },
    )
    expect(onArticle).not.toHaveBeenCalled()
  })
})
