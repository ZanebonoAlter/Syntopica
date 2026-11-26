import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useSearch } from './useSearch'
import type { SearchResponseData } from '~/types'

/**
 * useSearch 状态机与幂等防护（add-article-search test-cases 故事 S3 变体走查落点）：
 * initial→loading→success/empty/error 迁移、offset 追加分页不重置、
 * 输入不变不重查、进行中重复提交防护、过滤器变更立即重查、语义降级标记随响应更新。
 */

const api = vi.hoisted(() => ({ search: vi.fn() }))

vi.mock('~/api/search', () => ({
  useSearchApi: () => ({ search: api.search }),
}))

function hit(id: string, over: Record<string, unknown> = {}) {
  return {
    article: {
      id, feedId: 'f1', title: `文章${id}`, description: '', content: '',
      link: '', pubDate: '2026-10-01T00:00:00Z', category: '',
    },
    archived: false,
    hitReasons: [{ type: 'keyword' as const, text: '测试' }],
    ...over,
  }
}

function response(over: Partial<SearchResponseData> = {}): SearchResponseData {
  return {
    hits: [hit('a1'), hit('a2')],
    total: 2,
    page: 1,
    perPage: 20,
    sortBy: 'date',
    tookMs: 312,
    semanticSkipped: false,
    semanticTags: [{ label: '新能源车', score: 0.87 }],
    expandWords: [],
    ...over,
  }
}

/** 挂起中的响应句柄：resolve/reject 由用例手动放行 */
interface PendingCall {
  params: unknown
  resolve: (value: unknown) => void
}

function pendingSearch(): PendingCall {
  const pending: PendingCall = { params: null, resolve: () => {} }
  api.search.mockImplementationOnce((params: unknown) => {
    pending.params = params
    return new Promise((resolve) => { pending.resolve = resolve })
  })
  return pending
}

beforeEach(() => {
  api.search.mockReset()
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('useSearch — 状态机主链路', () => {
  it('initial：未搜索不发起请求', () => {
    const s = useSearch()
    expect(s.status.value).toBe('initial')
    expect(api.search).not.toHaveBeenCalled()
  })

  it('提交→loading→success：按默认参数请求（include + expand off + page 1），结果与摘要字段落位', async () => {
    api.search.mockResolvedValue({ success: true, data: response() })
    const s = useSearch()
    s.query.value = '特斯拉'
    s.submit()
    expect(s.status.value).toBe('loading')
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    expect(api.search).toHaveBeenCalledTimes(1)
    expect(api.search).toHaveBeenCalledWith({
      q: '特斯拉', archived: 'include', expand: 'off', sortBy: 'date', page: 1, perPage: 20,
    })
    expect(s.hits.value.length).toBe(2)
    expect(s.total.value).toBe(2)
    expect(s.tookMs.value).toBe(312)
    expect(s.semanticTags.value[0]?.label).toBe('新能源车')
    expect(s.hasMore.value).toBe(false)
  })

  it('零命中→empty：状态空态、结果清空', async () => {
    api.search.mockResolvedValue({ success: true, data: response({ hits: [], total: 0, semanticTags: [] }) })
    const s = useSearch()
    s.query.value = '不存在词'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('empty'))
    expect(s.hits.value.length).toBe(0)
  })

  it('失败→error：错误消息落位；retry 绕过幂等重查成功（保留输入）', async () => {
    api.search.mockResolvedValueOnce({ success: false, error: '网络错误' })
    api.search.mockResolvedValueOnce({ success: true, data: response() })
    const s = useSearch()
    s.query.value = '特斯拉'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('error'))
    expect(s.errorMessage.value).toBe('网络错误')
    s.retry()
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    expect(api.search).toHaveBeenCalledTimes(2)
    // 输入未被错误态清空
    expect(s.query.value).toBe('特斯拉')
  })

  it('空/纯空白查询：submit 直接忽略，不发起请求', () => {
    const s = useSearch()
    s.query.value = '   '
    s.submit()
    expect(api.search).not.toHaveBeenCalled()
    expect(s.status.value).toBe('initial')
  })
})

describe('useSearch — 幂等防护（变体走查）', () => {
  it('输入不变不重查：同 q 同过滤器重复 submit 不发第二次请求', async () => {
    api.search.mockResolvedValue({ success: true, data: response() })
    const s = useSearch()
    s.query.value = '特斯拉'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    s.submit()
    s.submit()
    expect(api.search).toHaveBeenCalledTimes(1)
  })

  it('进行中重复提交防护：loading 中再次 submit 被忽略', async () => {
    const first = pendingSearch()
    const s = useSearch()
    s.query.value = '特斯拉'
    s.submit()
    s.query.value = '特斯拉电池'
    s.submit() // 进行中 → 忽略
    expect(api.search).toHaveBeenCalledTimes(1)

    first.resolve({ success: true, data: response() })
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    // 放行后可提交新查询
    api.search.mockResolvedValueOnce({ success: true, data: response() })
    s.submit()
    await vi.waitFor(() => expect(api.search).toHaveBeenCalledTimes(2))
  })

  it('loadMore 追加不重置：已有结果保留、page=2 请求、hasMore 随 total 收敛', async () => {
    api.search.mockResolvedValueOnce({
      success: true,
      data: response({ hits: [hit('a1'), hit('a2')], total: 3, page: 1 }),
    })
    const s = useSearch()
    s.query.value = '特斯拉'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    expect(s.hasMore.value).toBe(true)

    api.search.mockResolvedValueOnce({
      success: true,
      data: response({ hits: [hit('a3')], total: 3, page: 2 }),
    })
    s.loadMore()
    expect(s.loadingMore.value).toBe(true)
    await vi.waitFor(() => expect(s.loadingMore.value).toBe(false))
    expect(api.search).toHaveBeenLastCalledWith({
      q: '特斯拉', archived: 'include', expand: 'off', sortBy: 'date', page: 2, perPage: 20,
    })
    expect(s.hits.value.map(h => h.article.id)).toEqual(['a1', 'a2', 'a3'])
    expect(s.hasMore.value).toBe(false)
    // 已无更多：loadMore 不再发请求
    await s.loadMore()
    expect(api.search).toHaveBeenCalledTimes(2)
  })

  it('新查询使进行中的 loadMore 响应过期：不污染新结果', async () => {
    api.search.mockResolvedValueOnce({ success: true, data: response({ hits: [hit('a1')], total: 10, page: 1 }) })
    const s = useSearch()
    s.query.value = '特斯拉'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('success'))

    const stale = pendingSearch()
    s.loadMore()
    // loadMore 进行中用户改查询提交（seq 递增）
    api.search.mockResolvedValueOnce({ success: true, data: response({ hits: [hit('b1')], total: 1, page: 1 }) })
    s.query.value = '比亚迪'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    // 过期的 loadMore 响应到达：不得追加进新结果
    stale.resolve({ success: true, data: response({ hits: [hit('a2')], total: 10, page: 2 }) })
    await Promise.resolve()
    await Promise.resolve()
    expect(s.hits.value.map(h => h.article.id)).toEqual(['b1'])
    expect(s.total.value).toBe(1)
  })
})

describe('useSearch — 过滤器与降级标记', () => {
  it('含归档切换：已执行查询存在时立即重查（active-only 参数）；无查询时只改状态不发请求', async () => {
    api.search.mockResolvedValue({ success: true, data: response() })
    const s = useSearch()
    s.setIncludeArchived(false)
    expect(api.search).not.toHaveBeenCalled()
    expect(s.includeArchived.value).toBe(false)

    s.query.value = '特斯拉'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    expect(api.search).toHaveBeenLastCalledWith(expect.objectContaining({ archived: 'active-only' }))

    s.setIncludeArchived(true)
    await vi.waitFor(() => expect(api.search).toHaveBeenCalledTimes(2))
    expect(api.search).toHaveBeenLastCalledWith(expect.objectContaining({ archived: 'include' }))
  })

  it('LLM 扩展开关：开启即带 expand=on 重查（空态引导「开启扩展再试」同路径）；关闭亦重查', async () => {
    api.search.mockResolvedValue({ success: true, data: response({ hits: [], total: 0 }) })
    const s = useSearch()
    s.query.value = '特斯拉'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('empty'))
    expect(s.expandEnabled.value).toBe(false)

    api.search.mockResolvedValueOnce({ success: true, data: response({ hits: [hit('x1')], expandWords: ['起火', '热失控'] }) })
    s.setExpandEnabled(true)
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    expect(api.search).toHaveBeenLastCalledWith(expect.objectContaining({ expand: 'on' }))
    expect(s.expandEnabled.value).toBe(true)
    expect(s.expandWords.value).toEqual(['起火', '热失控'])
  })

  it('语义降级标记随响应更新：skipped=true 落位，下次查询自动重试（重新置 false）', async () => {
    api.search.mockResolvedValueOnce({
      success: true,
      data: response({ semanticSkipped: true, semanticTags: [] }),
    })
    const s = useSearch()
    s.query.value = '特斯拉'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    expect(s.semanticSkipped.value).toBe(true)
    expect(s.semanticTags.value.length).toBe(0)

    api.search.mockResolvedValueOnce({ success: true, data: response() })
    s.query.value = '比亚迪'
    s.submit()
    await vi.waitFor(() => expect(s.semanticSkipped.value).toBe(false))
    expect(s.semanticTags.value.length).toBe(1)
  })
})

describe('useSearch — 排序模式', () => {
  it('默认 sortBy=date：首次请求携带 sortBy=date', async () => {
    api.search.mockResolvedValue({ success: true, data: response() })
    const s = useSearch()
    expect(s.sortBy.value).toBe('date')

    s.query.value = '特斯拉'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    expect(api.search).toHaveBeenLastCalledWith(expect.objectContaining({ q: '特斯拉', sortBy: 'date' }))
  })

  it('切换排序：无已执行查询时只改本地状态；有结果时以同查询立即重查并携带新排序', async () => {
    api.search.mockResolvedValue({ success: true, data: response() })
    const s = useSearch()

    s.setSortBy('relevance')
    expect(api.search).not.toHaveBeenCalled()
    expect(s.sortBy.value).toBe('relevance')

    s.query.value = '特斯拉'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    expect(api.search).toHaveBeenLastCalledWith(expect.objectContaining({ q: '特斯拉', sortBy: 'relevance' }))

    s.setSortBy('date')
    await vi.waitFor(() => expect(api.search).toHaveBeenCalledTimes(2))
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    expect(api.search).toHaveBeenLastCalledWith(expect.objectContaining({ q: '特斯拉', sortBy: 'date' }))

    // 同值再设置：不重查
    s.setSortBy('date')
    expect(api.search).toHaveBeenCalledTimes(2)
  })

  it('loadMore 沿用当前排序：切换后在翻页请求中携带新值', async () => {
    api.search.mockResolvedValueOnce({ success: true, data: response({ hits: [hit('a1')], total: 2, page: 1 }) })
    const s = useSearch()
    s.query.value = '特斯拉'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('success'))

    api.search.mockResolvedValueOnce({ success: true, data: response({ hits: [hit('a2')], total: 2, page: 1 }) })
    s.setSortBy('relevance')
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    expect(api.search).toHaveBeenCalledTimes(2)
    expect(s.hasMore.value).toBe(true)

    // 再给一页数据，验证翻页参数
    api.search.mockResolvedValueOnce({ success: true, data: response({ hits: [hit('a3')], total: 3, page: 2 }) })
    s.loadMore()
    await vi.waitFor(() => expect(s.loadingMore.value).toBe(false))
    expect(api.search).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2, sortBy: 'relevance' }))
  })

  it('查询指纹包含排序：切换排序后再 submit 同 q 不重复请求', async () => {
    api.search.mockResolvedValue({ success: true, data: response() })
    const s = useSearch()
    s.query.value = '特斯拉'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('success'))

    s.setSortBy('relevance')
    await vi.waitFor(() => expect(api.search).toHaveBeenCalledTimes(2))
    await vi.waitFor(() => expect(s.status.value).toBe('success'))

    s.submit()
    expect(api.search).toHaveBeenCalledTimes(2)
  })

  it('响应 sortBy 不反向覆盖本地状态（本地是用户意图）', async () => {
    api.search.mockResolvedValue({ success: true, data: response({ sortBy: 'relevance' }) })
    const s = useSearch()
    s.query.value = '特斯拉'
    s.submit()
    await vi.waitFor(() => expect(s.status.value).toBe('success'))
    expect(s.sortBy.value).toBe('date')
  })
})
