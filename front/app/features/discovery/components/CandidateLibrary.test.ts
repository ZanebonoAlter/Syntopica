import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import CandidateLibrary from './CandidateLibrary.vue'
import { useDiscoveryStore } from '~/stores/discovery'
import type { DiscoveryCandidate } from '~/types/discovery'

/**
 * S15 落点（test-cases.md 故事 S15-1 组件层）+ S9-3 开关失败恢复：
 * - 四态区分：加载 / 空库（新增入口）/ 筛选无结果（清筛选）/ 请求失败（重试，不冒称空库）；
 * - 已订阅与参与推荐分列展示；来源类型徽标；
 * - 超长标题与 URL 换行类名锚点（u-break-title / u-break-url / min-width:0）；
 * - 推荐启停开关失败恢复原值、提交中防连点。
 * 走真 store + mock api（状态机与回滚逻辑在 store 内，组件测试穿透验证用户可见结果）。
 */

const getCandidatesMock = vi.fn()
const createCandidateMock = vi.fn()
const updateCandidateMock = vi.fn()
const syncCatalogMock = vi.fn()
const getCatalogStatusMock = vi.fn()

vi.mock('~/api/discovery', () => ({
  useDiscoveryApi: () => ({
    getRecommendations: vi.fn(),
    refreshRecommendations: vi.fn(),
    acceptRecommendation: vi.fn(),
    dismissRecommendation: vi.fn(),
    ask: vi.fn(),
    getCatalogStatus: getCatalogStatusMock,
    syncCatalog: syncCatalogMock,
    getCandidates: getCandidatesMock,
    createCandidate: createCandidateMock,
    updateCandidate: updateCandidateMock,
    getInterests: vi.fn(),
    getRun: vi.fn(),
  }),
}))

vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ error: vi.fn(), success: vi.fn(), warn: vi.fn() }),
}))

// 候选订阅弹窗（5.3）依赖真 apiStore（模块级 defineStore 依赖 Nuxt auto-import，测试环境无）
vi.mock('~/stores/api', () => ({
  useApiStore: () => ({ categories: [] }),
}))

// 订阅弹窗打开时拉 RSSHub 配置：mock 掉避免真 fetch（失败即兜底默认常量）
vi.mock('~/api/rsshub', () => ({
  useRsshubApi: () => ({ getStatus: () => Promise.resolve({ success: false }) }),
}))

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function candidate(over: Partial<DiscoveryCandidate> = {}): DiscoveryCandidate {
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
    ...over,
  }
}

function okList(items: DiscoveryCandidate[], pages = 1, total?: number) {
  return {
    success: true,
    data: items,
    pagination: { page: 1, pages, per_page: 30, total: total ?? items.length },
  }
}

function mountLibrary() {
  return mount(CandidateLibrary, {
    attachTo: document.body,
  })
}

beforeEach(() => {
  setActivePinia(createPinia())
  getCandidatesMock.mockReset()
  createCandidateMock.mockReset()
  updateCandidateMock.mockReset()
  syncCatalogMock.mockReset()
  getCatalogStatusMock.mockReset()
  getCatalogStatusMock.mockResolvedValue({ success: true, data: { total: 3097, ok: 1, broken: 0, unknown: 3096 } })
  document.body.innerHTML = ''
})

describe('CandidateLibrary — 四态区分（S15：空库与筛选无结果）', () => {
  it('首载加载态文案', async () => {
    getCandidatesMock.mockReturnValue(new Promise(() => {}))
    const wrapper = mountLibrary()
    await flushPromises()
    expect(wrapper.find('[data-testid="library-state-loading"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('正在加载候选源库')
    wrapper.unmount()
  })

  it('请求失败显示错误态 + 重试，不冒称空库', async () => {
    getCandidatesMock.mockResolvedValue({ success: false, error: '网络错误' })
    const wrapper = mountLibrary()
    await flushPromises()
    expect(wrapper.find('[data-testid="library-state-error"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('候选源库加载失败')
    expect(wrapper.text()).toContain('不是空库')
    expect(wrapper.find('[data-testid="library-retry-btn"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="library-state-empty"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('空库提示新增入口；筛选无结果提示清筛选（两种空不同）', async () => {
    getCandidatesMock.mockResolvedValue(okList([]))
    const wrapper = mountLibrary()
    await flushPromises()
    expect(wrapper.find('[data-testid="library-state-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="library-empty-create-btn"]').exists()).toBe(true)

    // 设置筛选后空 → 变为「筛选无结果」态
    const store = useDiscoveryStore()
    store.setCandidateFilters({ query: '设计' })
    await flushPromises()
    expect(wrapper.find('[data-testid="library-state-empty-filtered"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="library-clear-filters-btn"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('成功列表：来源徽标 + 已订阅与参与推荐分列展示', async () => {
    getCandidatesMock.mockResolvedValue(okList([
      candidate({ id: '1', subscribed: false, recommendationEnabled: true }),
      candidate({ id: '2', kind: 'rsshub', name: '设计周刊', subscribed: true, recommendationEnabled: false, address: 'rsshub://design/:sec' }),
    ]))
    const wrapper = mountLibrary()
    await flushPromises()
    expect(wrapper.findAll('.library__item')).toHaveLength(2)
    const first = wrapper.findAll('.library__item')[0]!
    expect(first.find('.library__badge').text()).toBe('原生 RSS')
    expect(first.find('[data-testid="library-subscribed"]').text()).toBe('未订阅')
    expect(first.find('.library__status-cell').text()).toContain('订阅状态')
    expect(first.findAll('.library__status-cell')).toHaveLength(2) // 订阅状态 + 参与推荐 两列
    const second = wrapper.findAll('.library__item')[1]!
    expect(second.find('.library__badge').text()).toBe('RSSHub 目录')
    expect(second.find('[data-testid="library-subscribed"]').text()).toBe('已订阅')
    wrapper.unmount()
  })
})

describe('CandidateLibrary — 长文本换行锚点（S15：长文本与双视口）', () => {
  it('超长标题与 URL 带换行类名（u-break-title / u-break-url）', async () => {
    const longTitle = '超'.repeat(120)
    const longUrl = `https://example.com/${'a'.repeat(300)}/feed.xml`
    getCandidatesMock.mockResolvedValue(okList([candidate({ name: longTitle, address: longUrl })]))
    const wrapper = mountLibrary()
    await flushPromises()
    expect(wrapper.find('.library__name.u-break-title').exists()).toBe(true)
    expect(wrapper.find('.library__url.u-break-url').exists()).toBe(true)
    // flex 收缩锚点：info 单元格与 name 都有 min-width:0 类承载（窄屏不横向溢出）
    expect(wrapper.find('.library__info').exists()).toBe(true)
    expect(wrapper.find('.library__name').classes()).toContain('u-break-title')
    wrapper.unmount()
  })
})

describe('CandidateLibrary — 推荐启停（S9：停用与订阅独立）', () => {
  it('开关失败恢复原值，开关提交中防连点', async () => {
    getCandidatesMock.mockResolvedValue(okList([candidate({ id: '1', recommendationEnabled: true })]))
    // 受控 promise：先观察到提交中态，再手动放行失败
    let resolveApi!: (v: unknown) => void
    updateCandidateMock.mockImplementation(() => new Promise(r => { resolveApi = r }))
    const wrapper = mountLibrary()
    await flushPromises()
    const store = useDiscoveryStore()

    // 触发关闭开关（AppToggle 的点击目标是 track）
    const toggle = wrapper.find('[data-testid="library-toggle"] .app-toggle__track')
    await toggle.trigger('click')

    // 提交中：id 进入 toggling 池（防连点），再次点击不发第二次请求
    expect(store.candidateTogglingIds).toContain('1')
    await toggle.trigger('click')
    expect(updateCandidateMock).toHaveBeenCalledTimes(1)

    resolveApi({ success: false, error: '服务不可用' })
    await flushPromises()
    // 失败：恢复原值（仍为参与推荐），提示重试
    expect(store.candidates[0]!.recommendationEnabled).toBe(true)
    expect(store.candidateTogglingIds).toHaveLength(0)
    expect(updateCandidateMock).toHaveBeenCalledWith('1', { recommendationEnabled: false })
    wrapper.unmount()
  })

  it('开关成功保持新值，订阅状态不受影响', async () => {
    getCandidatesMock.mockResolvedValue(okList([candidate({ id: '1', recommendationEnabled: true, subscribed: true })]))
    updateCandidateMock.mockResolvedValue({ success: true, data: candidate({ recommendationEnabled: false, subscribed: true }) })
    const wrapper = mountLibrary()
    await flushPromises()
    await wrapper.find('[data-testid="library-toggle"] .app-toggle__track').trigger('click')
    await flushPromises()
    const store = useDiscoveryStore()
    expect(store.candidates[0]!.recommendationEnabled).toBe(false)
    expect(store.candidates[0]!.subscribed).toBe(true)
    wrapper.unmount()
  })
})

describe('CandidateLibrary — 搜索与筛选', () => {
  it('搜索输入防抖后带 query 重拉；清筛选重置', async () => {
    vi.useFakeTimers()
    getCandidatesMock.mockResolvedValue(okList([]))
    const wrapper = mountLibrary()
    await flushPromises()
    getCandidatesMock.mockClear()

    const input = wrapper.find('[data-testid="library-search-input"]').find('input')
    await input.setValue('设计')
    vi.advanceTimersByTime(350)
    await flushPromises()
    expect(getCandidatesMock).toHaveBeenCalledWith(expect.objectContaining({ query: '设计' }))

    const store = useDiscoveryStore()
    store.clearCandidateFilters()
    await flushPromises()
    expect(store.candidateFilters.query).toBe('')
    vi.useRealTimers()
    wrapper.unmount()
  })

  it('工具栏筛选透传：来源类型与推荐参与状态写入请求参数', async () => {
    getCandidatesMock.mockResolvedValue(okList([]))
    const wrapper = mountLibrary()
    await flushPromises()
    getCandidatesMock.mockClear()

    // 来源类型下拉→store→api（api 层再映射为后端 wire 名 kind/recommendation_enabled）
    await wrapper.find('[data-testid="library-kind-select"]').setValue('rsshub')
    await flushPromises()
    expect(getCandidatesMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ kind: 'rsshub', participation: 'all' }),
    )

    await wrapper.find('[data-testid="library-participation-select"]').setValue('disabled')
    await flushPromises()
    expect(getCandidatesMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ query: '', kind: 'rsshub', participation: 'disabled' }),
    )

    const store = useDiscoveryStore()
    expect(store.candidateFilters).toEqual({ query: '', kind: 'rsshub', participation: 'disabled' })
    expect(store.candidateFiltersActive).toBe(true)
    wrapper.unmount()
  })
})

describe('CandidateLibrary — 同步目录常驻入口', () => {
  it('工具栏常驻「同步目录」按钮，点击触发目录同步并刷新目录状态', async () => {
    syncCatalogMock.mockResolvedValue({ success: true, data: { Total: 3097, Inserted: 0, Updated: 3097 } })
    getCandidatesMock.mockResolvedValue(okList([candidate()]))
    const wrapper = mountLibrary()
    await flushPromises()
    const btn = wrapper.find('[data-testid="library-sync-catalog-btn"]')
    expect(btn.exists()).toBe(true)
    await btn.trigger('click')
    await flushPromises()
    expect(syncCatalogMock).toHaveBeenCalledTimes(1)
    expect(getCatalogStatusMock).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('同步中按钮禁用防连点（syncingCatalog 期间不可再点）', async () => {
    syncCatalogMock.mockReturnValue(new Promise(() => {})) // 同步挂起
    getCandidatesMock.mockResolvedValue(okList([candidate()]))
    const wrapper = mountLibrary()
    await flushPromises()
    const btn = wrapper.find('[data-testid="library-sync-catalog-btn"]')
    await btn.trigger('click')
    await flushPromises()
    expect((btn.element as HTMLButtonElement).disabled).toBe(true)
    wrapper.unmount()
  })
})

describe('CandidateLibrary — 分页（3099 条候选看不全的修复）', () => {
  it('多页列表末尾显示分页条：页码/总数，首尾页对应按钮禁用', async () => {
    getCandidatesMock.mockResolvedValue(okList([candidate()], 4, 103))
    const wrapper = mountLibrary()
    await flushPromises()
    const pager = wrapper.find('[data-testid="library-pager"]')
    expect(pager.exists()).toBe(true)
    expect(wrapper.find('[data-testid="library-pager-info"]').text()).toContain('第 1 / 4 页 · 共 103 条')
    expect((wrapper.find('[data-testid="library-pager-prev"]').element as HTMLButtonElement).disabled).toBe(true)
    expect((wrapper.find('[data-testid="library-pager-next"]').element as HTMLButtonElement).disabled).toBe(false)
    wrapper.unmount()
  })

  it('单页不渲染分页条', async () => {
    getCandidatesMock.mockResolvedValue(okList([candidate()]))
    const wrapper = mountLibrary()
    await flushPromises()
    expect(wrapper.find('[data-testid="library-pager"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('点下一页带 page=2 重拉；筛选变化后页码重置回第 1 页', async () => {
    getCandidatesMock.mockResolvedValue(okList([candidate()], 4, 103))
    const wrapper = mountLibrary()
    await flushPromises()
    await wrapper.find('[data-testid="library-pager-next"]').trigger('click')
    await flushPromises()
    expect(getCandidatesMock).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }))

    // 筛选变化：页码重置回 1
    const store = useDiscoveryStore()
    store.setCandidateFilters({ query: '田野' })
    await flushPromises()
    expect(getCandidatesMock).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, query: '田野' }))
    wrapper.unmount()
  })
})

describe('CandidateLibrary — 上游已下架（C2：route.status=gone）', () => {
  it('gone 条目显示「上游已下架」徽标，非 gone（ok/缺省）不显示', async () => {
    getCandidatesMock.mockResolvedValue(okList([
      candidate({
        id: '1',
        kind: 'rsshub',
        route: { namespace: '/github', path: '/issue/:user/:repo', name: 'issue', description: '上游介绍', example: '', parameters: '{}', usableDirectly: true, requiresParameters: false, status: 'gone' },
      }),
      candidate({
        id: '2',
        kind: 'rsshub',
        route: { namespace: '/blog', path: '/:id', name: 'blog', description: '上游介绍', example: '', parameters: '{}', usableDirectly: true, requiresParameters: false, status: 'ok' },
      }),
      candidate({ id: '3' }),
    ]))
    const wrapper = mountLibrary()
    await flushPromises()

    const items = wrapper.findAll('.library__item')
    expect(items).toHaveLength(3)
    expect(items[0]!.find('[data-testid="library-gone-badge"]').exists()).toBe(true)
    expect(items[0]!.text()).toContain('上游已下架')
    expect(items[1]!.find('[data-testid="library-gone-badge"]').exists()).toBe(false)
    expect(items[2]!.find('[data-testid="library-gone-badge"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('gone 条目订阅按钮禁用并给出原因，非 gone 仍可订阅', async () => {
    getCandidatesMock.mockResolvedValue(okList([
      candidate({ id: '1', kind: 'rsshub', route: { namespace: '/a', path: '/b', name: 'a', description: '上游介绍', example: '', parameters: '{}', usableDirectly: true, requiresParameters: false, status: 'gone' } }),
      candidate({ id: '2', kind: 'rsshub', route: { namespace: '/c', path: '/d', name: 'c', description: '上游介绍', example: '', parameters: '{}', usableDirectly: true, requiresParameters: false, status: 'broken' } }),
    ]))
    const wrapper = mountLibrary()
    await flushPromises()

    const goneBtn = wrapper.find('[data-testid="library-subscribe-btn-1"]')
    expect(goneBtn.attributes('disabled')).toBeDefined()
    expect(goneBtn.attributes('title')).toContain('上游已下架')
    // broken 只是可用性提示，不拦订阅（不过度设计）
    const brokenBtn = wrapper.find('[data-testid="library-subscribe-btn-2"]')
    expect(brokenBtn.attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('已订阅的 gone 条目不再出现订阅入口（订阅不被取消）', async () => {
    getCandidatesMock.mockResolvedValue(okList([
      candidate({ id: '1', kind: 'rsshub', subscribed: true, route: { namespace: '/a', path: '/b', name: 'a', description: '上游介绍', example: '', parameters: '{}', usableDirectly: true, requiresParameters: false, status: 'gone' } }),
    ]))
    const wrapper = mountLibrary()
    await flushPromises()

    expect(wrapper.find('[data-testid="library-subscribe-btn-1"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="library-gone-badge"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="library-subscribed"]').text()).toBe('已订阅')
    wrapper.unmount()
  })
})
