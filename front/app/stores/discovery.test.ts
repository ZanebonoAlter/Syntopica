import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import type { DiscoveryRecommendation } from '~/types/discovery'

const getRecommendationsMock = vi.fn()
const refreshRecommendationsMock = vi.fn()
const acceptRecommendationMock = vi.fn()
const dismissRecommendationMock = vi.fn()
const askDiscoveryMock = vi.fn()
const getRunMock = vi.fn()
const getInterestsMock = vi.fn()
const getRecommendationHistoryMock = vi.fn()
const restoreRecommendationMock = vi.fn()
const getCatalogStatusMock = vi.fn()
const syncCatalogMock = vi.fn()
const getCandidatesMock = vi.fn()
const createCandidateMock = vi.fn()
const updateCandidateMock = vi.fn()

const notifyErrorMock = vi.fn()
const notifySuccessMock = vi.fn()
const notifyWarnMock = vi.fn()

vi.mock('~/api/discovery', () => ({
  useDiscoveryApi: () => ({
    getRecommendations: getRecommendationsMock,
    refreshRecommendations: refreshRecommendationsMock,
    acceptRecommendation: acceptRecommendationMock,
    dismissRecommendation: dismissRecommendationMock,
    askDiscovery: askDiscoveryMock,
    getRun: getRunMock,
    getInterests: getInterestsMock,
    getRecommendationHistory: getRecommendationHistoryMock,
    restoreRecommendation: restoreRecommendationMock,
    getCatalogStatus: getCatalogStatusMock,
    syncCatalog: syncCatalogMock,
    getCandidates: getCandidatesMock,
    createCandidate: createCandidateMock,
    updateCandidate: updateCandidateMock,
  }),
}))

vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({
    error: notifyErrorMock,
    success: notifySuccessMock,
    warn: notifyWarnMock,
  }),
}))

import { useDiscoveryStore } from './discovery'
import type { DiscoveryCandidate } from '~/types/discovery'

function createCard(overrides: Partial<DiscoveryRecommendation> = {}): DiscoveryRecommendation {
  return {
    id: '1',
    routeId: '10',
    boardId: '3',
    boardLabel: 'AI 前沿',
    source: 'manual_refresh',
    score: 0.8,
    llmReason: '理由',
    status: 'pending',
    routeNamespace: 'bilibili',
    routePath: '/user/video/:uid',
    routeName: 'UP 主投稿',
    routeExample: '/bilibili/user/video/2267573',
    usableDirectly: false,
    requiresParameters: true,
    parameters: '{"uid":"用户 id"}',
    paramOptions: {},
    createdAt: '2026-07-25T00:00:00Z',
    ...overrides,
  }
}

describe('useDiscoveryStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('loads pending recommendations into cards', async () => {
    getRecommendationsMock.mockResolvedValue({ success: true, data: [createCard()] })
    const store = useDiscoveryStore()
    await store.loadRecommendations()
    expect(store.cards).toHaveLength(1)
    expect(store.cards[0]!.routeName).toBe('UP 主投稿')
    expect(notifyErrorMock).not.toHaveBeenCalled()
  })

  it('notifies on load failure', async () => {
    getRecommendationsMock.mockResolvedValue({ success: false, error: '网络错误' })
    const store = useDiscoveryStore()
    await store.loadRecommendations()
    expect(notifyErrorMock).toHaveBeenCalledWith('网络错误')
  })

  it('refresh reloads cards and toasts inserted count', async () => {
    refreshRecommendationsMock.mockResolvedValue({
      success: true,
      data: { candidates: 5, inserted: 2, skipped: 3, cooldownBlocked: 0 },
    })
    getRecommendationsMock.mockResolvedValue({ success: true, data: [createCard()] })
    const store = useDiscoveryStore()
    await store.refresh()
    expect(notifySuccessMock).toHaveBeenCalledWith('换了一批新推荐（新增 2 条）')
    expect(getRecommendationsMock).toHaveBeenCalled()
  })

  it('refresh warns when no candidates', async () => {
    refreshRecommendationsMock.mockResolvedValue({
      success: true,
      data: { candidates: 0, inserted: 0, skipped: 0, cooldownBlocked: 0 },
    })
    getRecommendationsMock.mockResolvedValue({ success: true, data: [] })
    const store = useDiscoveryStore()
    await store.refresh()
    expect(notifyWarnMock).toHaveBeenCalled()
  })

  it('submitQuery trims question and skips blank input (S1 输入错误)', async () => {
    const store = useDiscoveryStore()
    const ok = await store.submitQuery('   ')
    expect(ok).toBe(false)
    expect(askDiscoveryMock).not.toHaveBeenCalled()
    expect(store.askStatus).toBe('idle') // 未进入执行态，未打开结果视图
    expect(store.askViewOpen).toBe(false)
  })

  it('submitQuery posts ask, polls run to succeeded and stores result (S1 主链路)', async () => {
    askDiscoveryMock.mockResolvedValue({ success: true, data: { runId: '7' } })
    getRunMock.mockResolvedValue({
      success: true,
      data: { id: '7', kind: 'qa', query: '量子计算', status: 'succeeded', startedAt: '2026-09-12T00:00:00Z', finishedAt: '2026-09-12T00:00:05Z', items: [] },
    })
    const store = useDiscoveryStore()
    const ok = await store.submitQuery(' 量子计算 ')
    expect(ok).toBe(true)
    expect(askDiscoveryMock).toHaveBeenCalledWith('量子计算') // 去空白后提交
    expect(getRunMock).toHaveBeenCalledWith('7')
    expect(store.askStatus).toBe('succeeded')
    expect(store.askViewOpen).toBe(true)
    expect(store.lastQuery).toBe('量子计算')
    expect(store.askRun?.id).toBe('7')
    expect(store.askRun?.items).toEqual([]) // 成功零条也是成功，不是失败
  })

  it('submitQuery rejects duplicate submission while running (S1 重复提交)', async () => {
    askDiscoveryMock.mockResolvedValue({ success: true, data: { runId: '7' } })
    getRunMock.mockImplementation(() => new Promise(() => {})) // 一直 running
    const store = useDiscoveryStore()
    const first = store.submitQuery('长查询')
    const second = await store.submitQuery('长查询')
    expect(second).toBe(false)
    expect(askDiscoveryMock).toHaveBeenCalledTimes(1) // 执行中不重复发起
    void first
  })

  it('submitQuery start failure keeps old cards and marks failed (S1 查询失败后恢复)', async () => {
    askDiscoveryMock.mockResolvedValue({ success: false, error: '服务不可用' })
    const store = useDiscoveryStore()
    const ok = await store.submitQuery('量子计算')
    expect(ok).toBe(false)
    expect(store.askStatus).toBe('failed')
    expect(store.askError).toBe('服务不可用')
    expect(store.askViewOpen).toBe(true) // 失败仍打开独立视图展示「未更新+重试」
    expect(getRunMock).not.toHaveBeenCalled()
  })

  it('submitQuery run failed on first poll → failed with message', async () => {
    askDiscoveryMock.mockResolvedValue({ success: true, data: { runId: '8' } })
    getRunMock.mockResolvedValue({
      success: true,
      data: { id: '8', kind: 'qa', query: 'x', status: 'failed', startedAt: '', finishedAt: '', items: [] },
    })
    const store = useDiscoveryStore()
    const ok = await store.submitQuery('x')
    expect(ok).toBe(false)
    expect(store.askStatus).toBe('failed')
    expect(getRunMock).toHaveBeenCalledTimes(1)
  })

  it('submitQuery poll timeout (2s×60) → failed with manual refresh hint (design D2)', async () => {
    vi.useFakeTimers()
    askDiscoveryMock.mockResolvedValue({ success: true, data: { runId: '9' } })
    getRunMock.mockResolvedValue({
      success: true,
      data: { id: '9', kind: 'qa', query: 'x', status: 'running', startedAt: '', finishedAt: null, items: [] },
    })
    const store = useDiscoveryStore()
    const pending = store.submitQuery('x')
    await vi.advanceTimersByTimeAsync(2000 * 60 + 2000) // 推满全部轮询窗口
    const ok = await pending
    expect(ok).toBe(false)
    expect(store.askStatus).toBe('failed')
    expect(store.askError).toContain('手动刷新') // 超时转失败并提示手动刷新
    expect(getRunMock).toHaveBeenCalledTimes(60) // 上限 60 次，不无限轮询
    vi.useRealTimers()
  })

  it('closeRunView only hides view, keeps query state for return (S1 返回为你推荐)', async () => {
    askDiscoveryMock.mockResolvedValue({ success: true, data: { runId: '7' } })
    getRunMock.mockResolvedValue({
      success: true,
      data: { id: '7', kind: 'qa', query: 'q', status: 'succeeded', startedAt: '', finishedAt: '', items: [] },
    })
    const store = useDiscoveryStore()
    await store.submitQuery('q')
    store.closeRunView()
    expect(store.askViewOpen).toBe(false)
    expect(store.askStatus).toBe('succeeded') // 状态与结果保留，不清空
    expect(store.askRun?.id).toBe('7')
  })

  it('submitQuery success silently reloads interests when tab already loaded', async () => {
    getInterestsMock.mockResolvedValue({ success: true, data: [] })
    const store = useDiscoveryStore()
    await store.loadInterests() // 兴趣页签已加载过
    expect(getInterestsMock).toHaveBeenCalledTimes(1)
    askDiscoveryMock.mockResolvedValue({ success: true, data: { runId: '7' } })
    getRunMock.mockResolvedValue({
      success: true,
      data: { id: '7', kind: 'qa', query: 'q', status: 'succeeded', startedAt: '', finishedAt: '', items: [] },
    })
    await store.submitQuery('q')
    expect(getInterestsMock).toHaveBeenCalledTimes(2) // 成功查询静默刷新兴趣记录
  })

  it('accept removes card and toasts feed title', async () => {
    getRecommendationsMock.mockResolvedValue({ success: true, data: [createCard()] })
    acceptRecommendationMock.mockResolvedValue({
      success: true,
      data: { id: '9', title: '新源', url: 'http://x' },
    })
    const store = useDiscoveryStore()
    await store.loadRecommendations()
    const ok = await store.accept('1', { parameters: { uid: '123' } })
    expect(ok).toBe(true)
    expect(acceptRecommendationMock).toHaveBeenCalledWith('1', { parameters: { uid: '123' } })
    expect(store.cards).toHaveLength(0)
    expect(notifySuccessMock).toHaveBeenCalledWith('已订阅「新源」')
  })

  it('accept keeps card and notifies on validation failure', async () => {
    getRecommendationsMock.mockResolvedValue({ success: true, data: [createCard()] })
    acceptRecommendationMock.mockResolvedValue({ success: false, error: 'feed fetch 验证失败' })
    const store = useDiscoveryStore()
    await store.loadRecommendations()
    const ok = await store.accept('1')
    expect(ok).toBe(false)
    expect(store.cards).toHaveLength(1)
    expect(notifyErrorMock).toHaveBeenCalledWith('feed fetch 验证失败')
  })

  it('dismiss removes card on success', async () => {
    getRecommendationsMock.mockResolvedValue({ success: true, data: [createCard()] })
    dismissRecommendationMock.mockResolvedValue({ success: true })
    const store = useDiscoveryStore()
    await store.loadRecommendations()
    const ok = await store.dismiss('1')
    expect(ok).toBe(true)
    expect(store.cards).toHaveLength(0)
  })

  it('syncCatalog reloads catalog status', async () => {
    syncCatalogMock.mockResolvedValue({
      success: true,
      data: { Inserted: 100, Updated: 0, Gone: 0, Total: 3245, NewToEmbed: 100 },
    })
    getCatalogStatusMock.mockResolvedValue({
      success: true,
      data: { total: 3245, ok: 0, broken: 0, unknown: 3245, gone: 0, embedded: 0 },
    })
    const store = useDiscoveryStore()
    const ok = await store.syncCatalog()
    expect(ok).toBe(true)
    expect(store.catalogStatus?.total).toBe(3245)
  })
})

function createCandidate(overrides: Partial<DiscoveryCandidate> = {}): DiscoveryCandidate {
  return {
    id: '1',
    kind: 'rss',
    name: '田野笔记',
    description: '城市观察',
    language: '中文',
    region: '中国',
    address: 'https://fieldnotes.example/feed.xml',
    recommendationEnabled: true,
    subscribed: false,
    availability: 'unknown',
    lastCheckedAt: null,
    ...overrides,
  }
}

describe('useDiscoveryStore — 候选源库（S9/S15）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('loadCandidates 成功写入列表与总数', async () => {
    getCandidatesMock.mockResolvedValue({
      success: true,
      data: [createCandidate()],
      pagination: { page: 1, pages: 1, per_page: 30, total: 1 },
    })
    const store = useDiscoveryStore()
    const ok = await store.loadCandidates()
    expect(ok).toBe(true)
    expect(store.candidates).toHaveLength(1)
    expect(store.candidatesTotal).toBe(1)
    expect(store.candidatesLoaded).toBe(true)
    expect(store.candidatesError).toBeNull()
    // 请求带默认筛选
    expect(getCandidatesMock).toHaveBeenCalledWith({ query: '', kind: 'all', participation: 'all' })
  })

  it('loadCandidates 失败置错误态：请求失败 ≠ 空库，不置 loaded', async () => {
    getCandidatesMock.mockResolvedValue({ success: false, error: '网络错误' })
    const store = useDiscoveryStore()
    const ok = await store.loadCandidates()
    expect(ok).toBe(false)
    expect(store.candidatesError).toBe('网络错误')
    expect(store.candidatesLoaded).toBe(false) // 错误态不是空库态
    expect(store.candidates).toHaveLength(0)
  })

  it('setCandidateFilters 透传筛选（含参与状态）并重拉', async () => {
    getCandidatesMock.mockResolvedValue({ success: true, data: [], pagination: { page: 1, pages: 1, per_page: 30, total: 0 } })
    const store = useDiscoveryStore()
    store.setCandidateFilters({ query: '设计', kind: 'rsshub', participation: 'disabled' })
    await flushPromises()
    // 保持语义命名交给 api 层映射（后端 wire 名见 api/discovery.test.ts）
    expect(getCandidatesMock).toHaveBeenLastCalledWith({ query: '设计', kind: 'rsshub', participation: 'disabled' })
    expect(store.candidateFiltersActive).toBe(true)

    store.clearCandidateFilters()
    await flushPromises()
    expect(store.candidateFiltersActive).toBe(false)
  })

  it('saveCandidate 新增成功：toast 明示尚未订阅并重拉列表', async () => {
    createCandidateMock.mockResolvedValue({ success: true, data: createCandidate({ id: '9' }) })
    getCandidatesMock.mockResolvedValue({ success: true, data: [createCandidate({ id: '9' })], pagination: { page: 1, pages: 1, per_page: 30, total: 1 } })
    const store = useDiscoveryStore()
    const res = await store.saveCandidate({
      name: '田野笔记', url: 'https://fieldnotes.example/feed.xml', description: '', language: '', region: '', recommendationEnabled: true,
    })
    expect(res.status).toBe('saved')
    // 保存语义红线：入库不订阅，文案必须含「尚未订阅」
    expect(notifySuccessMock).toHaveBeenCalledWith(expect.stringContaining('尚未订阅'))
    expect(getCandidatesMock).toHaveBeenCalled()
  })

  it('saveCandidate 409 返回 duplicate + 已有条目，不静默新建', async () => {
    createCandidateMock.mockResolvedValue({
      success: false,
      status: 409,
      data: createCandidate({ id: '5', name: '已有来源' }),
      error: '候选库已有这个地址',
    })
    const store = useDiscoveryStore()
    const res = await store.saveCandidate({
      name: '新名字', url: 'https://dup.example/feed', description: '', language: '', region: '', recommendationEnabled: true,
    })
    expect(res.status).toBe('duplicate')
    if (res.status === 'duplicate') expect(res.existing?.id).toBe('5')
    expect(getCandidatesMock).not.toHaveBeenCalled() // 未入库不重拉
  })

  it('saveCandidate 保存中防连点：第二次调用直接拒绝', async () => {
    createCandidateMock.mockReturnValue(new Promise(() => {}))
    const store = useDiscoveryStore()
    const first = store.saveCandidate({
      name: 'a', url: 'https://a.example/feed', description: '', language: '', region: '', recommendationEnabled: true,
    })
    const second = await store.saveCandidate({
      name: 'b', url: 'https://b.example/feed', description: '', language: '', region: '', recommendationEnabled: true,
    })
    expect(second.status).toBe('error')
    expect(createCandidateMock).toHaveBeenCalledTimes(1)
    void first
  })

  it('setCandidateEnabled 失败恢复原值并提示重试（S9 停用与订阅独立）', async () => {
    getCandidatesMock.mockResolvedValue({
      success: true,
      data: [createCandidate({ id: '1', recommendationEnabled: true, subscribed: true })],
      pagination: { page: 1, pages: 1, per_page: 30, total: 1 },
    })
    updateCandidateMock.mockResolvedValue({ success: false, error: '服务不可用' })
    const store = useDiscoveryStore()
    await store.loadCandidates()
    const ok = await store.setCandidateEnabled('1', false)
    expect(ok).toBe(false)
    expect(store.candidates[0]!.recommendationEnabled).toBe(true) // 恢复原值
    expect(store.candidates[0]!.subscribed).toBe(true) // 订阅不受影响
    expect(notifyErrorMock).toHaveBeenCalledWith(expect.stringContaining('已恢复'))
  })

  it('setCandidateEnabled 提交中防连点：第二次调用不重复发请求', async () => {
    getCandidatesMock.mockResolvedValue({
      success: true,
      data: [createCandidate({ id: '1' })],
      pagination: { page: 1, pages: 1, per_page: 30, total: 1 },
    })
    updateCandidateMock.mockReturnValue(new Promise(() => {}))
    const store = useDiscoveryStore()
    await store.loadCandidates()
    void store.setCandidateEnabled('1', false)
    const ok = await store.setCandidateEnabled('1', true)
    expect(ok).toBe(false)
    expect(updateCandidateMock).toHaveBeenCalledTimes(1)
  })

  it('setCandidateEnabled 成功保持新值（仅改推荐资格）', async () => {
    getCandidatesMock.mockResolvedValue({
      success: true,
      data: [createCandidate({ id: '1', recommendationEnabled: true })],
      pagination: { page: 1, pages: 1, per_page: 30, total: 1 },
    })
    updateCandidateMock.mockResolvedValue({
      success: true,
      data: createCandidate({ id: '1', recommendationEnabled: false }),
    })
    const store = useDiscoveryStore()
    await store.loadCandidates()
    const ok = await store.setCandidateEnabled('1', false)
    expect(ok).toBe(true)
    expect(store.candidates[0]!.recommendationEnabled).toBe(false)
    expect(notifySuccessMock).toHaveBeenCalledWith(expect.stringContaining('已有订阅不受影响'))
  })
})

function historyItem(overrides: Partial<import('~/types/discovery').DiscoveryHistoryItem> = {}): import('~/types/discovery').DiscoveryHistoryItem {
  return {
    id: '1',
    name: '某推荐源',
    status: 'excluded',
    snoozedUntil: null,
    lastSelectedAt: '2026-09-01T00:00:00Z',
    reason: '当时的推荐理由',
    ...overrides,
  }
}

describe('useDiscoveryStore — 兴趣记录与历史（S1/S3/S5）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('loadInterests 成功写入列表并置 loaded', async () => {
    getInterestsMock.mockResolvedValue({
      success: true,
      data: [
        { id: '1', queryText: '体育新闻', boardLabel: '日本新闻', createdAt: '2026-09-10T00:00:00Z', status: 'active' },
        { id: '2', queryText: '软件工程', boardLabel: null, createdAt: '2026-09-11T00:00:00Z', status: 'faded' },
      ],
    })
    const store = useDiscoveryStore()
    const ok = await store.loadInterests()
    expect(ok).toBe(true)
    expect(store.interests).toHaveLength(2)
    expect(store.interestsLoaded).toBe(true)
    expect(store.interestsError).toBeNull()
    expect(store.interests[1]!.boardLabel).toBeNull() // 未匹配独立保留
  })

  it('loadInterests 失败置错误态，旧数据保留', async () => {
    getInterestsMock.mockResolvedValueOnce({ success: true, data: [] })
    const store = useDiscoveryStore()
    await store.loadInterests()
    getInterestsMock.mockResolvedValue({ success: false, error: '网络错误' })
    const ok = await store.loadInterests()
    expect(ok).toBe(false)
    expect(store.interestsError).toBe('网络错误')
    expect(store.interestsLoaded).toBe(true) // 首次成功过
  })

  it('loadHistory 成功/失败分开置态', async () => {
    getRecommendationHistoryMock.mockResolvedValue({ success: false, error: '网关超时' })
    const store = useDiscoveryStore()
    expect(await store.loadHistory()).toBe(false)
    expect(store.historyError).toBe('网关超时')
    expect(store.historyLoaded).toBe(false)

    getRecommendationHistoryMock.mockResolvedValue({ success: true, data: [historyItem()] })
    expect(await store.loadHistory()).toBe(true)
    expect(store.history).toHaveLength(1)
    expect(store.historyError).toBeNull()
  })

  it('restoreRecommendation 成功仅恢复资格，不承诺出卡不自动订阅（R5）', async () => {
    getRecommendationHistoryMock.mockResolvedValue({ success: true, data: [historyItem({ id: '5', status: 'excluded' })] })
    restoreRecommendationMock.mockResolvedValue({ success: true })
    const store = useDiscoveryStore()
    await store.loadHistory()
    const ok = await store.restoreRecommendation('5')
    expect(ok).toBe(true)
    expect(restoreRecommendationMock).toHaveBeenCalledWith('5')
    expect(store.history[0]!.status).toBe('restored') // 本地展示态
    // 文案不承诺立即出卡、不自动订阅
    expect(notifySuccessMock).toHaveBeenCalledWith(expect.stringContaining('不会自动订阅'))
    expect(getRecommendationsMock).not.toHaveBeenCalled() // 不重拉推荐列表
  })

  it('restoreRecommendation 失败提示重试；提交中防连点', async () => {
    getRecommendationHistoryMock.mockResolvedValue({ success: true, data: [historyItem({ id: '5' })] })
    const store = useDiscoveryStore()
    await store.loadHistory()
    restoreRecommendationMock.mockReturnValue(new Promise(() => {}))
    void store.restoreRecommendation('5')
    expect(store.restoringIds).toContain('5')
    const again = await store.restoreRecommendation('5')
    expect(again).toBe(false)
    expect(restoreRecommendationMock).toHaveBeenCalledTimes(1) // 防连点
  })

  it('restoreRecommendation 未知 id 直接拒绝', async () => {
    const store = useDiscoveryStore()
    const ok = await store.restoreRecommendation('nope')
    expect(ok).toBe(false)
    expect(restoreRecommendationMock).not.toHaveBeenCalled()
  })
})
