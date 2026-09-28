import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import DiscoveryCard from './DiscoveryCard.vue'
import type { DiscoveryRecommendation } from '~/types/discovery'

/**
 * 「为你推荐」卡片（R6 展示边界 + fix-discovery-subscribed-hint）：
 * - 已订阅徽标：后端 subscribed 下发即展示，title 附已有订阅地址供对照；
 * - 提示 ≠ 禁用：一键订阅 / 填写参数并订阅入口保持原样（RSSHub 路由级命中仍可订其它参数）；
 * - 未订阅不显示徽标（不误报）。
 */

const acceptMock = vi.fn()
const dismissMock = vi.fn()

vi.mock('~/stores/discovery', async () => {
  const { reactive } = await import('vue')
  return {
    useDiscoveryStore: () => reactive({
      actingIds: [] as string[],
      accept: acceptMock,
      dismiss: dismissMock,
    }),
  }
})

vi.mock('~/stores/api', async () => {
  const { reactive } = await import('vue')
  return {
    useApiStore: () => reactive({
      categories: [],
      fetchFeeds: vi.fn(),
    }),
  }
})

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function card(over: Partial<DiscoveryRecommendation> = {}): DiscoveryRecommendation {
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
    usableDirectly: true,
    requiresParameters: false,
    parameters: '{}',
    paramOptions: {},
    subscribed: false,
    subscribedUrls: [],
    createdAt: '2026-07-25T00:00:00Z',
    ...over,
  }
}

function mountCard(c: DiscoveryRecommendation) {
  return mount(DiscoveryCard, { props: { card: c } })
}

describe('DiscoveryCard — 已订阅提示（路由级命中只提示、不隐藏入口）', () => {
  it('已订阅：显示「已订阅」徽标，title 列出已有订阅地址，一键订阅入口保留', () => {
    const wrapper = mountCard(card({
      subscribed: true,
      subscribedUrls: ['https://rsshub.example/bilibili/user/video/2267573'],
    }))
    const badge = wrapper.find('[data-testid="card-subscribed-badge"]')
    expect(badge.exists()).toBe(true)
    expect(badge.text()).toBe('已订阅')
    expect(badge.attributes('title')).toContain('https://rsshub.example/bilibili/user/video/2267573')
    // 订阅入口保留：是否重复由订阅弹窗按最终地址判定（本 change 的用户决策）
    expect(wrapper.findAll('button').map(b => b.text())).toContain('订阅')
    wrapper.unmount()
  })

  it('已订阅 + 需填参数：徽标与「填写参数并订阅」入口并存', () => {
    const wrapper = mountCard(card({
      subscribed: true,
      subscribedUrls: [],
      usableDirectly: false,
      requiresParameters: true,
    }))
    expect(wrapper.find('[data-testid="card-subscribed-badge"]').exists()).toBe(true)
    expect(wrapper.findAll('button').map(b => b.text())).toContain('填写参数并订阅')
    wrapper.unmount()
  })

  it('未订阅：不显示已订阅徽标（不误报）', () => {
    const wrapper = mountCard(card({ subscribed: false }))
    expect(wrapper.find('[data-testid="card-subscribed-badge"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('已订阅但无地址：title 兜底说明，不渲染空地址', () => {
    const wrapper = mountCard(card({ subscribed: true, subscribedUrls: [] }))
    const badge = wrapper.find('[data-testid="card-subscribed-badge"]')
    expect(badge.attributes('title')).toBe('该来源已有订阅')
    wrapper.unmount()
  })
})
