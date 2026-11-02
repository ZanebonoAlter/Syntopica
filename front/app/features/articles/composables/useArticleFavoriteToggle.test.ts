import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { Article } from '~/types'
import { useArticleFavoriteToggle } from './useArticleFavoriteToggle'

/**
 * useArticleFavoriteToggle — 收藏乐观更新 + 失败回滚。
 *
 * 覆盖（锚 daily-report-system §阅读台文章抽屉与站内阅读预览的状态同步）：
 *  - 乐观翻转：点击后界面立即取反（不等接口返回）
 *  - 成功：不回滚、不提示
 *  - 失败（success:false / 抛异常）：回滚到点击前状态 + 错误提示
 *  - 连点不丢更新：迟到的失败响应不回滚后续点击的新状态
 *  - 事件与当前文章对不上 / 无当前文章：忽略，不发请求不改状态
 */
const apiMocks = vi.hoisted(() => ({ updateArticle: vi.fn() }))
const notifyMocks = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warn: vi.fn() }))

vi.mock('~/api/articles', () => ({ useArticlesApi: () => apiMocks }))
vi.mock('~/composables/useNotify', () => ({ useNotify: () => notifyMocks }))

function makeArticle(id: string, favorite = false): Article {
  return {
    id,
    feedId: '1',
    title: `文章 ${id}`,
    description: '',
    content: '',
    excerpt: '',
    link: 'https://example.com/a',
    pubDate: '2026-10-28',
    author: undefined,
    category: '',
    read: false,
    favorite,
    summaryStatus: undefined,
    summaryGeneratedAt: undefined,
    completionAttempts: undefined,
    completionError: undefined,
    aiContentSummary: undefined,
    firecrawlStatus: undefined,
    firecrawlError: undefined,
    firecrawlContent: undefined,
    firecrawlCrawledAt: undefined,
    imageUrl: undefined,
    tagCount: undefined,
    tags: [],
  }
}

/** 宿主：持有当前文章，apply 即写回 */
function mountHost(initial: Article | null) {
  const article = ref<Article | null>(initial)
  let toggle: ((articleId: string) => Promise<void>) | null = null

  const Host = defineComponent({
    setup() {
      const { toggleFavorite } = useArticleFavoriteToggle()
      toggle = (articleId: string) => toggleFavorite(articleId, {
        get: () => article.value,
        apply: (next: Article) => { article.value = next },
      })
      return () => h('div')
    },
  })

  const wrapper = mount(Host)
  return {
    wrapper,
    article,
    toggle: (articleId: string) => toggle!(articleId),
  }
}

/** 可手动兑现的 Promise（用于模拟「请求未返回」） */
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => { resolve = r })
  return { promise, resolve }
}

describe('useArticleFavoriteToggle — 收藏乐观更新', () => {
  beforeEach(() => {
    apiMocks.updateArticle.mockReset()
    notifyMocks.error.mockReset()
  })

  it('点击即翻转：接口未返回时界面已是新状态', async () => {
    const call = deferred<{ success: boolean, data?: unknown }>()
    apiMocks.updateArticle.mockReturnValue(call.promise)
    const host = mountHost(makeArticle('158497', false))

    const pending = host.toggle('158497')
    expect(host.article.value?.favorite).toBe(true)
    expect(apiMocks.updateArticle).toHaveBeenCalledWith(158497, { favorite: true })

    call.resolve({ success: true, data: {} })
    await pending
    expect(host.article.value?.favorite).toBe(true)
    expect(notifyMocks.error).not.toHaveBeenCalled()
  })

  it('成功：不回滚、不提示', async () => {
    apiMocks.updateArticle.mockResolvedValue({ success: true, data: {} })
    const host = mountHost(makeArticle('158497', false))

    await host.toggle('158497')
    expect(host.article.value?.favorite).toBe(true)
    expect(notifyMocks.error).not.toHaveBeenCalled()
  })

  it('失败（success:false）：回滚到点击前状态并提示', async () => {
    apiMocks.updateArticle.mockResolvedValue({ success: false, error: '写入失败' })
    const host = mountHost(makeArticle('158497', false))

    await host.toggle('158497')
    expect(host.article.value?.favorite).toBe(false)
    expect(notifyMocks.error).toHaveBeenCalledWith('写入失败')
  })

  it('失败（抛异常）：回滚到点击前状态并提示', async () => {
    apiMocks.updateArticle.mockRejectedValue(new Error('网络错误'))
    const host = mountHost(makeArticle('158497', true))

    await host.toggle('158497')
    expect(host.article.value?.favorite).toBe(true)
    expect(notifyMocks.error).toHaveBeenCalledWith('网络错误')
  })

  it('连点不丢更新：迟到的失败响应不回滚后续点击的新状态', async () => {
    const first = deferred<{ success: boolean, error?: string }>()
    const second = deferred<{ success: boolean, error?: string }>()
    apiMocks.updateArticle
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)
    const host = mountHost(makeArticle('158497', false))

    const firstToggle = host.toggle('158497')   // 乐观 → true
    const secondToggle = host.toggle('158497')  // 乐观 → false（基于第一次的乐观值）
    expect(host.article.value?.favorite).toBe(false)

    first.resolve({ success: false, error: '写入失败' })  // 先到的失败
    await firstToggle
    expect(host.article.value?.favorite).toBe(false)

    second.resolve({ success: true })
    await secondToggle
    expect(host.article.value?.favorite).toBe(false)
  })

  it('事件与当前文章对不上：忽略，不发请求不改状态', async () => {
    const host = mountHost(makeArticle('158497', false))

    await host.toggle('999999')
    expect(apiMocks.updateArticle).not.toHaveBeenCalled()
    expect(host.article.value?.favorite).toBe(false)
  })

  it('无当前文章：忽略，不发请求', async () => {
    const host = mountHost(null)

    await host.toggle('158497')
    expect(apiMocks.updateArticle).not.toHaveBeenCalled()
    await flushPromises()
  })
})
