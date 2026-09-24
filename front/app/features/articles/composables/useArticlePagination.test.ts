import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'

/**
 * useArticlePagination.refreshCurrentPage —— 自动刷新完成后的按需重取
 * （slim-article-list-payload C2/C4）。
 *
 * 契约：只重取「当前筛选 + 当前页」，不读写 loading、不重置 page/filters，
 * 响应回来时若视图已切换则丢弃，失败静默不写 state。
 */

// vitest.setup.ts 只把 ref/computed/onMounted/onUnmounted/watch 挂到 globalThis；
// 本 composable 还用 reactive，而静态 import 先于文件体执行 → stubGlobal + 动态 import。
vi.stubGlobal('reactive', reactive)

const mocks = vi.hoisted(() => ({ getArticles: vi.fn() }))

vi.mock('~/api/articles', () => ({
  useArticlesApi: () => ({ getArticles: mocks.getArticles }),
}))

const { useArticlePagination } = await import('./useArticlePagination')

function makePayload(id: number, title = `Article ${id}`) {
  return {
    id,
    feed_id: 1,
    title,
    link: `https://example.com/${id}`,
    pub_date: '2026-01-01T00:00:00Z',
    created_at: '2026-01-01T00:00:00Z',
    read: false,
    favorite: false,
  }
}

type Payload = ReturnType<typeof makePayload>

function pageResponse(items: Payload[], page: number, pages: number, total: number) {
  return {
    success: true,
    data: { items },
    pagination: { page, pages, per_page: 20, total },
  }
}

describe('useArticlePagination.refreshCurrentPage', () => {
  beforeEach(() => {
    mocks.getArticles.mockReset()
  })

  it('refreshCurrentPage 用当前页与当前筛选重取（per_page ≤ 100）', async () => {
    mocks.getArticles.mockResolvedValueOnce(pageResponse([makePayload(1)], 1, 1, 1))
    const p = useArticlePagination({ pageSize: 20 })
    await p.fetchFirstPage({ feed_id: 7, read: false })

    mocks.getArticles.mockClear()
    mocks.getArticles.mockResolvedValueOnce(pageResponse([makePayload(2)], 1, 1, 1))
    await p.refreshCurrentPage()

    expect(mocks.getArticles).toHaveBeenCalledTimes(1)
    const params = mocks.getArticles.mock.calls[0]![0] as Record<string, unknown>
    expect(params.page).toBe(1)
    expect(params.per_page).toBe(20)
    expect(params.per_page as number).toBeLessThanOrEqual(100)
    expect(params.feed_id).toBe(7)
    expect(params.read).toBe(false)
  })

  it('刷新前后 page 与选中行保持不变', async () => {
    mocks.getArticles.mockResolvedValueOnce(
      pageResponse([makePayload(1), makePayload(2)], 1, 1, 2),
    )
    const p = useArticlePagination({ pageSize: 20 })
    await p.fetchFirstPage()
    const selectedId = p.state.articles[1]!.id

    mocks.getArticles.mockResolvedValueOnce(
      pageResponse([makePayload(1, '更新一'), makePayload(2, '更新二')], 1, 1, 2),
    )
    await p.refreshCurrentPage()

    expect(p.state.page).toBe(1)
    expect(p.state.articles.map(a => a.id)).toContain(selectedId)
    expect(p.state.articles[1]!.title).toBe('更新二')
  })

  it('刷新期间用户切换视图：刷新结果被丢弃、不覆盖新视图', async () => {
    mocks.getArticles.mockResolvedValueOnce(pageResponse([makePayload(1)], 1, 1, 1))
    const p = useArticlePagination({ pageSize: 20 })
    await p.fetchFirstPage()

    let resolveRefresh!: (value: unknown) => void
    mocks.getArticles.mockImplementationOnce(
      () => new Promise((resolve) => { resolveRefresh = resolve }),
    )

    const refreshPromise = p.refreshCurrentPage()

    // 刷新请求仍挂起时，用户切到新筛选并完成加载（不被刷新阻塞）
    mocks.getArticles.mockResolvedValueOnce(
      pageResponse([makePayload(9, '新视图')], 1, 1, 1),
    )
    await p.fetchFirstPage({ feed_id: 42 })

    // 旧视图的刷新响应这时才回来 → 必须被 token 校验丢弃
    resolveRefresh(pageResponse([makePayload(1, '旧视图')], 1, 1, 1))
    await refreshPromise

    expect(p.state.articles.map(a => a.id)).toEqual(['9'])
    expect(p.filters.value.feed_id).toBe(42)
  })

  it('第 N 页刷新只替换当前页片段', async () => {
    const page1 = Array.from({ length: 20 }, (_, i) => makePayload(i + 1))
    const page2 = Array.from({ length: 20 }, (_, i) => makePayload(i + 21))
    mocks.getArticles.mockResolvedValueOnce(pageResponse(page1, 1, 2, 40))
    mocks.getArticles.mockResolvedValueOnce(pageResponse(page2, 2, 2, 40))
    const p = useArticlePagination({ pageSize: 20 })
    await p.fetchFirstPage()
    await p.loadMore()

    expect(p.state.page).toBe(2)
    const firstPageIds = p.state.articles.slice(0, 20).map(a => a.id)

    mocks.getArticles.mockResolvedValueOnce(
      pageResponse([makePayload(21, '替换行'), ...page2.slice(1)], 2, 2, 40),
    )
    await p.refreshCurrentPage()

    expect(p.state.page).toBe(2)
    expect(p.state.articles).toHaveLength(40)
    expect(p.state.articles.slice(0, 20).map(a => a.id)).toEqual(firstPageIds)
    expect(p.state.articles[20]!.title).toBe('替换行')
  })

  it('刷新失败不改动当前列表', async () => {
    mocks.getArticles.mockResolvedValueOnce(
      pageResponse([makePayload(1), makePayload(2)], 1, 1, 2),
    )
    const p = useArticlePagination({ pageSize: 20 })
    await p.fetchFirstPage()
    const before = JSON.parse(JSON.stringify(p.state.articles)) as Payload[]
    const pageBefore = p.state.page

    mocks.getArticles.mockResolvedValueOnce({ success: false, error: 'boom' })
    await p.refreshCurrentPage()

    mocks.getArticles.mockRejectedValueOnce(new Error('network'))
    await p.refreshCurrentPage()

    expect(p.state.articles).toEqual(before)
    expect(p.state.page).toBe(pageBefore)
    expect(p.state.error).toBeNull()
  })
})
