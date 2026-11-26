import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

/**
 * AppSidebarView 导航区结构（add-article-search 任务 3.5 落点）：
 * 「文章搜索」入口位于「发现订阅源」之后、分隔线之前，点击 navigateTo('/search')。
 */

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const feedsPublic = vi.hoisted(() => ({ updateSelection: vi.fn() }))

vi.mock('~/features/feeds/public', () => ({
  useRefreshPolling: () => ({ updateSelection: feedsPublic.updateSelection }),
}))

const router = vi.hoisted(() => ({ push: vi.fn() }))

// Nuxt auto-imports：组件内裸引用的 store 与 useNuxtApp（$router）挂 globalThis
const globals = globalThis as unknown as Record<string, unknown>
globals.useApiStore = () => ({})
globals.useFeedsStore = () => ({ feeds: [], categories: [], getFeedsByCategory: () => [] })
globals.useArticlesStore = () => ({ favoriteCount: 0 })
globals.useNuxtApp = () => ({ $router: router })

import AppSidebarView from './AppSidebarView.vue'

function mountSidebar() {
  return mount(AppSidebarView, {
    props: { watchedTags: [] },
    global: {
      components: {
        AppTooltip: { name: 'AppTooltip', template: '<span><slot /></span>' },
        FeedActionMenu: { name: 'FeedActionMenu', template: '<span />' },
      },
      stubs: { FeedIcon: true },
    },
  })
}

beforeEach(() => {
  router.push.mockReset()
  feedsPublic.updateSelection.mockReset()
})

afterEach(() => {
  document.body.innerHTML = ''
})

describe('AppSidebarView — 文章搜索入口（add-article-search）', () => {
  it('入口顺序：全部文章 → 收藏夹 → 叙事工坊 → 发现订阅源 → 文章搜索（分隔线之前）', () => {
    const w = mountSidebar()
    // 展开态：主导航按钮 + 关注标签空态内无 sidebar-item 主导航
    const labels = w.findAll('.sidebar-item').map(b => b.text().replace(/\d+$/, '').trim())
    expect(labels).toEqual(['全部文章', '收藏夹', '叙事工坊', '发现订阅源', '文章搜索'])

    // 文章搜索按钮位于第一个 divider 之前（DOM 顺序锚）
    const searchBtn = w.findAll('.sidebar-item').find(b => b.text().includes('文章搜索'))
    const firstDivider = w.find('.divider')
    expect(searchBtn).toBeDefined()
    expect((searchBtn!.element as HTMLElement).compareDocumentPosition(firstDivider.element as HTMLElement)
      & Node.DOCUMENT_POSITION_FOLLOWING).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
  })

  it('点击「文章搜索」→ navigateTo /search（与发现订阅源同款 router 跳转）', async () => {
    const w = mountSidebar()
    const searchBtn = w.findAll('.sidebar-item').find(b => b.text().includes('文章搜索'))
    await searchBtn!.trigger('click')
    expect(router.push).toHaveBeenCalledWith('/search')
    expect(router.push).toHaveBeenCalledTimes(1)
  })

  it('点击「发现订阅源」仍跳 /discovery（既有行为回归）', async () => {
    const w = mountSidebar()
    const discoveryBtn = w.findAll('.sidebar-item').find(b => b.text().includes('发现订阅源'))
    await discoveryBtn!.trigger('click')
    expect(router.push).toHaveBeenCalledWith('/discovery')
  })
})
