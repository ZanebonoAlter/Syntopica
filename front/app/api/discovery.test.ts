import { beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * 候选源库 API normalizer（improve-discovery-recommendations 5.1 补充 + spec
 * feed-candidate-catalog C2「上游已下架」）：
 * - 列表响应形状兼容后端 { items, total } 包装（pagination.total 透出）；
 * - route 必须保留（含 status = gone，前端据此标「上游已下架」并禁订阅）；
 * - address 推导：后端 CandidateView 无 address 字段，取 feed_url / route 拼装。
 */

const { getMock, postMock, patchMock, buildQueryParamsMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  postMock: vi.fn(),
  patchMock: vi.fn(),
  buildQueryParamsMock: vi.fn<(params?: Record<string, unknown>) => string>(() => ''),
}))

vi.mock('./client', () => ({
  apiClient: {
    get: getMock,
    post: postMock,
    patch: patchMock,
    buildQueryParams: buildQueryParamsMock,
  },
}))

import { useDiscoveryApi } from './discovery'
import { buildQueryString } from '~/utils/api-helpers'

function candidateItem(over: Record<string, unknown> = {}) {
  return {
    id: 11,
    kind: 'rsshub',
    name: 'design-weekly',
    description: '',
    language: '',
    region: '',
    recommendation_enabled: true,
    subscribed: false,
    availability: 'unknown',
    last_checked_at: null,
    ...over,
  }
}

describe('useDiscoveryApi — 候选列表 normalizer', () => {
  beforeEach(() => {
    getMock.mockReset()
    postMock.mockReset()
    patchMock.mockReset()
    buildQueryParamsMock.mockReset().mockReturnValue('')
  })

  it('展开 { items, total } 包装，保留 route.status=gone 并透出分页 total', async () => {
    getMock.mockResolvedValue({
      success: true,
      data: {
        items: [
          candidateItem({
            feed_url: null,
            route: {
              namespace: '/github',
              path: '/issue/:user/:repo',
              name: 'issue',
              example: '/github/issue/foo/bar',
              parameters: '[]',
              usable_directly: true,
              requires_parameters: false,
              status: 'gone',
            },
          }),
          candidateItem({ id: 12, kind: 'rss', feed_url: 'https://example.com/feed.xml' }),
        ],
        total: 42,
      },
    })

    const api = useDiscoveryApi()
    const res = await api.getCandidates({ page: 1, perPage: 30 })

    expect(res.success).toBe(true)
    expect(res.data).toHaveLength(2)
    const gone = res.data![0]!
    expect(gone.route?.status).toBe('gone')
    // address 推导：rsshub 条目取 namespace+path（后端无 address 字段）
    expect(gone.address).toBe('/github/issue/:user/:repo')
    const rss = res.data![1]!
    expect(rss.route).toBeNull()
    expect(rss.address).toBe('https://example.com/feed.xml')
    expect(res.pagination?.total).toBe(42)
  })

  it('兼容扁平数组响应，且 route.status=ok 不进 gone 判定路径', async () => {
    getMock.mockResolvedValue({
      success: true,
      data: [
        candidateItem({
          id: 13,
          address: '显式地址优先',
          route: {
            namespace: '/blog',
            path: '/:id',
            name: 'blog',
            example: '',
            parameters: '{}',
            usable_directly: true,
            requires_parameters: false,
            status: 'ok',
          },
        }),
      ],
    })

    const api = useDiscoveryApi()
    const res = await api.getCandidates()

    expect(res.data).toHaveLength(1)
    expect(res.data![0]!.address).toBe('显式地址优先')
    expect(res.data![0]!.route?.status).toBe('ok')
  })

  it('详情 normalizer 同样保留 route.status（订阅弹窗判断依据）', async () => {
    getMock.mockResolvedValue({
      success: true,
      data: {
        id: 21,
        kind: 'rsshub',
        name: 'old-route',
        feed_url: null,
        subscribed: false,
        route: {
          namespace: '/old',
          path: '/:id',
          name: 'old',
          example: '',
          parameters: '{}',
          usable_directly: false,
          requires_parameters: true,
          status: 'gone',
        },
      },
    })

    const api = useDiscoveryApi()
    const res = await api.getCandidateDetail('21')

    expect(res.data?.route?.status).toBe('gone')
    expect(res.data?.route?.requiresParameters).toBe(true)
  })
})

/**
 * 参数名映射（后端 handler.ListCandidates 只认 q/page/page_size/kind/recommendation_enabled）：
 * 前端类型层保持语义命名，映射在 api 层完成；发旧名会被后端静默忽略，
 * 用户可见症状是「搜索与筛选点了没反应」。此组用例锁住 wire 名与 URL。
 */
describe('useDiscoveryApi — 候选列表查询参数映射', () => {
  beforeEach(() => {
    getMock.mockReset()
    buildQueryParamsMock.mockReset()
    // 用真实 buildQueryString，断言 URL 而不是复述自己写的 mock 返回值
    buildQueryParamsMock.mockImplementation(p => buildQueryString(p ?? {}).replace(/^\?/, ''))
  })

  function queryOf(url: string): URLSearchParams {
    return new URLSearchParams(url.slice(url.indexOf('?') + 1))
  }

  it('query/participation/perPage 映射为 q/recommendation_enabled/page_size 并随 URL 发出', async () => {
    getMock.mockResolvedValue({ success: true, data: { items: [candidateItem()], total: 1 } })
    const api = useDiscoveryApi()

    await api.getCandidates({
      query: ' 设计 ',
      kind: 'rsshub',
      participation: 'enabled',
      page: 2,
      perPage: 50,
    })

    expect(buildQueryParamsMock).toHaveBeenCalledWith({
      page: 2,
      page_size: 50,
      q: '设计',
      kind: 'rsshub',
      recommendation_enabled: true,
    })

    const url = getMock.mock.calls[0]![0] as string
    expect(url.startsWith('/discovery/candidates?')).toBe(true)
    const qs = queryOf(url)
    expect(qs.get('q')).toBe('设计') // trim 后下发
    expect(qs.get('page')).toBe('2')
    expect(qs.get('page_size')).toBe('50')
    expect(qs.get('kind')).toBe('rsshub')
    expect(qs.get('recommendation_enabled')).toBe('true')
    // 旧参数名不得再出现（后端会忽略）
    expect(qs.has('query')).toBe(false)
    expect(qs.has('per_page')).toBe(false)
    expect(qs.has('participation')).toBe(false)
  })

  it('participation=disabled → recommendation_enabled=false；all 与空关键词不下发', async () => {
    getMock.mockResolvedValue({ success: true, data: { items: [], total: 0 } })
    const api = useDiscoveryApi()

    await api.getCandidates({ query: '   ', kind: 'all', participation: 'disabled' })
    let qs = queryOf(getMock.mock.calls[0]![0] as string)
    expect(qs.get('recommendation_enabled')).toBe('false')
    expect(qs.has('q')).toBe(false) // 纯空白 trim 后不下发
    expect(qs.has('kind')).toBe(false) // all = 不筛选

    await api.getCandidates({ participation: 'all' })
    qs = queryOf(getMock.mock.calls[1]![0] as string)
    expect(qs.has('recommendation_enabled')).toBe(false)
    expect(qs.has('q')).toBe(false)
  })
})

/**
 * 写路径 wire 名对账（权威源 = 后端 DTO json tag，见报告表）：
 * - POST /discovery/candidates → service.CandidateCreateInput 的地址字段是 `feed_url`；
 *   发 `url` 会被 ShouldBindJSON 静默丢弃，后端报 "feed url must not be empty"；
 * - PATCH /discovery/candidates/:id → service.CandidateUpdateInput 同理（仅 kind=rss 接受）；
 * - accept / import-confirm 的 body 名一并锁住，防止后续再漂移。
 * 这些用例断言真实请求体（而非复述 mock 返回值），发错名会直接红。
 */
describe('useDiscoveryApi — 写路径 wire 名对账', () => {
  beforeEach(() => {
    getMock.mockReset()
    postMock.mockReset()
    patchMock.mockReset()
    buildQueryParamsMock.mockReset().mockReturnValue('')
  })

  function bodyOf(call: number): Record<string, unknown> {
    return postMock.mock.calls[call]?.[1] as Record<string, unknown>
  }

  it('createCandidate 发 feed_url（不发 url）+ recommendation_enabled，并透传人工字段', async () => {
    postMock.mockResolvedValue({ success: true, data: candidateItem({ id: 31, kind: 'rss', feed_url: 'https://example.com/feed.xml' }) })
    const api = useDiscoveryApi()

    await api.createCandidate({
      name: '新源',
      url: 'https://example.com/feed.xml',
      description: '说明',
      language: 'zh',
      region: 'CN',
      recommendationEnabled: true,
    })

    expect(postMock.mock.calls[0]![0]).toBe('/discovery/candidates')
    const body = bodyOf(0)
    expect(body).toEqual({
      name: '新源',
      feed_url: 'https://example.com/feed.xml',
      description: '说明',
      language: 'zh',
      region: 'CN',
      recommendation_enabled: true,
    })
    // 旧 wire 名不得再出现（后端 CandidateCreateInput 无 url tag）
    expect(body).not.toHaveProperty('url')

    // 关闭推荐的新建：开关必须随请求体下发（后端 nil=true / false=不参与推荐）
    postMock.mockResolvedValue({ success: true, data: candidateItem({ id: 32, kind: 'rss', feed_url: 'https://example.com/paused.xml', recommendation_enabled: false }) })
    await api.createCandidate({
      name: '暂停源',
      url: 'https://example.com/paused.xml',
      description: '',
      language: '',
      region: '',
      recommendationEnabled: false,
    })
    expect(bodyOf(1)).toEqual({
      name: '暂停源',
      feed_url: 'https://example.com/paused.xml',
      description: '',
      language: '',
      region: '',
      recommendation_enabled: false,
    })
  })

  it('updateCandidate 改地址发 feed_url；rsshub 地址不下发（urlEditable=false）', async () => {
    patchMock.mockResolvedValue({ success: true, data: candidateItem({ id: 31, kind: 'rss', feed_url: 'https://example.com/new.xml' }) })
    const api = useDiscoveryApi()

    await api.updateCandidate('31', { url: 'https://example.com/new.xml' })

    expect(patchMock.mock.calls[0]![0]).toBe('/discovery/candidates/31')
    const body = patchMock.mock.calls[0]![1] as Record<string, unknown>
    expect(body).toEqual({ feed_url: 'https://example.com/new.xml' })
    expect(body).not.toHaveProperty('url')

    // 仅改开关：只发 recommendation_enabled（对应后端同名 tag）
    patchMock.mockResolvedValue({ success: true, data: candidateItem({ id: 31, recommendation_enabled: false }) })
    await api.updateCandidate('31', { recommendationEnabled: false })
    expect(patchMock.mock.calls[1]![1]).toEqual({ recommendation_enabled: false })
  })

  it('accept 发 category_id/parameters；import/confirm 发 fingerprint/local_revision/import', async () => {
    postMock.mockResolvedValue({ success: true, data: { id: 1, title: 't', url: 'u' } })
    const api = useDiscoveryApi()
    await api.acceptRecommendation('7', { categoryId: '3', parameters: { user: 'foo' } })
    expect(postMock.mock.calls[0]![0]).toBe('/discovery/recommendations/7/accept')
    expect(bodyOf(0)).toEqual({ category_id: 3, parameters: { user: 'foo' } })

    postMock.mockReset()
    postMock.mockResolvedValue({ success: true, data: { applied: [], skipped_duplicate: [], skipped_conflict: [], failed: [], invalid: [] } })
    const file = { format: 'syntopica-candidate-catalog', version: 1, entries: [] }
    await api.confirmCatalogImport({ fingerprint: 'fp', localRevision: 9, file })
    expect(postMock.mock.calls[0]![0]).toBe('/discovery/candidates/import/confirm')
    expect(bodyOf(0)).toEqual({ fingerprint: 'fp', local_revision: 9, import: file })
  })
})
