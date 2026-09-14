import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import InterestRecords from './InterestRecords.vue'
import { useDiscoveryStore } from '~/stores/discovery'
import type { DiscoveryInterest } from '~/types/discovery'

/**
 * R3 Independent Interest Records / ui-design IA「兴趣记录」页签（test-cases S1-3 组件面）：
 * - 顶部说明「近期记录用于补充推荐，影响会逐渐降低」；
 * - 逐条展示查询原句 / 版块名或「未匹配版块」/ 时间 / 状态徽标三态文案（active/faded/legacy）；
 * - 全页不出现百分比（不把向量相似度伪装成兴趣百分比）；
 * - 四态：加载 / 错误（重试，不冒称空）/ 空（去找订阅源 → emit go-ask）/ 列表。
 * 走真 store + mock api（沿用 5.1 CandidateLibrary.test.ts 风格）。
 */

const getInterestsMock = vi.fn()

vi.mock('~/api/discovery', () => ({
  useDiscoveryApi: () => ({
    getInterests: getInterestsMock,
  }),
}))

vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ error: vi.fn(), success: vi.fn(), warn: vi.fn() }),
}))

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function interest(over: Partial<DiscoveryInterest> = {}): DiscoveryInterest {
  return {
    id: '1',
    queryText: '日本本地新闻',
    boardLabel: '日本新闻',
    createdAt: '2026-09-10T08:00:00Z',
    status: 'active',
    ...over,
  }
}

function mountRecords() {
  return mount(InterestRecords, {
    attachTo: document.body,
  })
}

beforeEach(() => {
  setActivePinia(createPinia())
  getInterestsMock.mockReset()
  document.body.innerHTML = ''
})

describe('InterestRecords — 四态', () => {
  it('首载加载态', async () => {
    getInterestsMock.mockReturnValue(new Promise(() => {}))
    const wrapper = mountRecords()
    await flushPromises()
    expect(wrapper.find('[data-testid="interest-state-loading"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('正在加载兴趣记录')
    wrapper.unmount()
  })

  it('加载失败显示错误态 + 重试，不冒称空', async () => {
    getInterestsMock.mockResolvedValue({ success: false, error: '网络错误' })
    const wrapper = mountRecords()
    await flushPromises()
    expect(wrapper.find('[data-testid="interest-state-error"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('兴趣记录加载失败')
    expect(wrapper.find('[data-testid="interest-state-empty"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="interest-retry-btn"]').exists()).toBe(true)

    // 重试成功 → 转列表/空态
    getInterestsMock.mockResolvedValue({ success: true, data: [interest()] })
    await wrapper.find('[data-testid="interest-retry-btn"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="interest-state-error"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="interest-list"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('空态提示去找订阅源并 emit go-ask（切回为你推荐聚焦查询框）', async () => {
    getInterestsMock.mockResolvedValue({ success: true, data: [] })
    const wrapper = mountRecords()
    await flushPromises()
    expect(wrapper.find('[data-testid="interest-state-empty"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('还没有问答兴趣')

    await wrapper.find('[data-testid="interest-go-ask-btn"]').trigger('click')
    expect(wrapper.emitted('go-ask')).toHaveLength(1) // 冒泡给 workspace 切页签聚焦查询框
    wrapper.unmount()
  })
})

describe('InterestRecords — 列表条目契约', () => {
  it('逐条展示原句/版块名/时间；未匹配显示「未匹配版块」', async () => {
    getInterestsMock.mockResolvedValue({
      success: true,
      data: [
        interest({ id: '1', queryText: '日本本地新闻', boardLabel: '日本新闻', createdAt: '2026-09-10T08:00:00Z' }),
        interest({ id: '2', queryText: '软件工程随笔', boardLabel: null, createdAt: '2026-09-11T09:30:00Z' }),
      ],
    })
    const wrapper = mountRecords()
    await flushPromises()
    const items = wrapper.findAll('.interests__item')
    expect(items).toHaveLength(2)
    expect(items[0]!.text()).toContain('日本本地新闻')
    expect(items[0]!.text()).toContain('日本新闻')
    expect(items[0]!.text()).toContain('2026-09-10T08:00:00Z')
    expect(items[1]!.text()).toContain('软件工程随笔')
    expect(items[1]!.text()).toContain('未匹配版块') // board_id=NULL 独立保留，不挂标签
    wrapper.unmount()
  })

  it('状态徽标三态文案：参与中/已淡出/历史迁移', async () => {
    getInterestsMock.mockResolvedValue({
      success: true,
      data: [
        interest({ id: '1', status: 'active' }),
        interest({ id: '2', status: 'faded' }),
        interest({ id: '3', status: 'legacy' }),
      ],
    })
    const wrapper = mountRecords()
    await flushPromises()
    const badges = wrapper.findAll('.interests__badge')
    expect(badges.map(b => b.text())).toEqual(['参与中', '已淡出', '历史迁移'])
    // 三态各有独立样式分档（形态维度锚，颜色留给视觉验收）
    expect(badges[0]!.classes()).toContain('is-active')
    expect(badges[1]!.classes()).toContain('is-faded')
    expect(badges[2]!.classes()).toContain('is-legacy')
    wrapper.unmount()
  })

  it('全页不出现百分比（不把相似度伪装成兴趣强度）', async () => {
    getInterestsMock.mockResolvedValue({
      success: true,
      data: [interest({ id: '1' }), interest({ id: '2', boardLabel: null })],
    })
    const wrapper = mountRecords()
    await flushPromises()
    expect(wrapper.text()).not.toContain('%')
    expect(wrapper.text()).not.toContain('％')
    wrapper.unmount()
  })

  it('顶部说明「近期记录用于补充推荐，影响会逐渐降低」始终可见', async () => {
    getInterestsMock.mockResolvedValue({ success: true, data: [interest()] })
    const wrapper = mountRecords()
    await flushPromises()
    expect(wrapper.find('.interests__note').text()).toContain('近期记录用于补充推荐')
    expect(wrapper.find('.interests__note').text()).toContain('影响会逐渐降低')
    wrapper.unmount()
  })

  it('超长查询原句带换行锚点（u-break-title）', async () => {
    getInterestsMock.mockResolvedValue({
      success: true,
      data: [interest({ id: '1', queryText: '非'.repeat(200) })],
    })
    const wrapper = mountRecords()
    await flushPromises()
    expect(wrapper.find('.interests__query.u-break-title').exists()).toBe(true)
    wrapper.unmount()
  })
})

describe('InterestRecords — 加载时机', () => {
  it('已加载过则不重复请求（ask 成功后 store 已静默刷新）', async () => {
    getInterestsMock.mockResolvedValue({ success: true, data: [interest()] })
    const store = useDiscoveryStore()
    await store.loadInterests()
    getInterestsMock.mockClear()

    const wrapper = mountRecords()
    await flushPromises()
    expect(getInterestsMock).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="interest-list"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
