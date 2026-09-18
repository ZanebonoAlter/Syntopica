import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import { __mediaQueryClient } from '~/composables/useMediaQuery'
import type { Article } from '~/types'
import FeedLayoutShell from './FeedLayoutShell.vue'

/**
 * FeedLayoutShell 窄屏集成测试（mobile-viewport-stage1 任务 2.3/3.1/3.2）
 *
 * 环境限制（与 NotificationPanel.test.ts 头注释同款先例）：
 * 本机 happy-dom@20.8.4 + VTU@2.4.6 下 wrapper.emitted() 记录失效——
 * 交互断言一律用「handler spy（store/api stub 调用）+ DOM 断言」替代。
 *
 * mock 策略：三个 Shell 子组件（Header/Sidebar/ArticleListPanel）以轻桩替换——
 * 列表桩渲染可点击文章按钮 + 虚拟列表滚动容器 div 并经滚动桥注册（真实
 * ArticleListPanelView 的注册逻辑同构）；AppSidebarView 桩渲染抽屉内点选按钮。
 * 断点用可控 matchMedia 桩驱动（useMediaQuery.test.ts 同款做法）。
 *
 * 宽屏零变化红线用例：宽屏路径不渲染抽屉/返回条、不进阅读态。
 */

const articlesPublic = vi.hoisted(() => ({
  mocks: null as null | {
    state: { articles: Article[]; hasMore: boolean; total: number; loading: boolean }
    fetchFirstPage: ReturnType<typeof vi.fn>
    loadMore: ReturnType<typeof vi.fn>
    updateArticle: ReturnType<typeof vi.fn>
  },
}))

vi.mock('~/features/articles/public', async () => {
  const { defineComponent, h, reactive } = await import('vue')
  const state = reactive({
    articles: [] as Article[],
    hasMore: false,
    total: 0,
    loading: false,
  })
  const mocks = {
    state,
    fetchFirstPage: vi.fn(async () => {}),
    loadMore: vi.fn(async () => {}),
    updateArticle: vi.fn(),
  }
  articlesPublic.mocks = mocks
  const stub = defineComponent({
    name: 'Stub',
    props: { article: { type: null, default: null } },
    setup(props: { article?: { title?: string } | null }) {
      return () => h('div', { 'data-testid': 'article-content' }, props.article?.title ?? 'empty')
    },
  })
  return {
    useArticlePagination: () => mocks,
    ArticleContentView: stub,
    ArticleCardView: stub,
    RowStatusPopover: stub,
    getArticlePipelineState: () => null,
    getPipelineStateMeta: () => ({ label: '', tone: 'neutral' }),
  }
})

vi.mock('~/features/shell/components/AppHeaderShell.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      name: 'AppHeaderShellStub',
      emits: ['openDrawer', 'toggleSidebar', 'refresh', 'markAllRead', 'settings', 'closeRefreshMessage'],
      setup(_props, { emit }) {
        return () =>
          h('button', { 'data-testid': 'header-open-drawer', onClick: () => emit('openDrawer') }, 'menu')
      },
    }),
  }
})

vi.mock('~/features/shell/components/AppSidebarView.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      name: 'AppSidebarViewStub',
      emits: [
        'categoryClick',
        'feedClick',
        'favoritesClick',
        'allArticlesClick',
        'editCategory',
        'editFeed',
        'deleteCategory',
        'watchedTagsClick',
        'watchedTagClick',
      ],
      setup(_props, { emit }) {
        return () => [
          h(
            'button',
            { 'data-testid': 'drawer-feed-item', onClick: () => emit('feedClick', 'feed-1') },
            '订阅源一'
          ),
          h(
            'button',
            { 'data-testid': 'drawer-category-item', onClick: () => emit('categoryClick', 'cat-1') },
            '分类一'
          ),
        ]
      },
    }),
  }
})

vi.mock('~/features/shell/components/ArticleListPanelShell.vue', async () => {
  const { defineComponent, h, inject, onMounted } = await import('vue')
  const { FEED_LIST_SCROLL_BRIDGE } = await import('./feedListScrollBridge')
  return {
    default: defineComponent({
      name: 'ArticleListPanelShellStub',
      props: [
        'articles',
        'selectedCategory',
        'selectedFeed',
        'selectedArticle',
        'loading',
        'hasMore',
        'total',
        'startDate',
        'endDate',
      ],
      emits: ['articleClick', 'articleFavorite', 'loadMore', 'dateFilterChange', 'dateFilterClear'],
      setup(props: { articles?: Article[] }, { emit }) {
        const bridge = inject(FEED_LIST_SCROLL_BRIDGE, null)
        onMounted(() => {
          // 与真实 ArticleListPanelView 注册 useVirtualList containerProps.ref 同构
          const el = document.querySelector('[data-testid="virtual-list-container"]') as HTMLElement | null
          if (bridge && el) bridge.el = el
        })
        return () =>
          h('div', { class: 'article-list-panel' }, [
            h('div', { 'data-testid': 'virtual-list-container' }),
            ...(props.articles ?? []).map((a, i) =>
              h(
                'button',
                { key: a.id, 'data-testid': `article-item-${i}`, onClick: () => emit('articleClick', a) },
                a.title
              )
            ),
          ])
      },
    }),
  }
})

vi.mock('~/features/feeds/public', () => ({
  useGlobalAutoRefresh: vi.fn(),
  useRefreshPolling: vi.fn(() => ({ updateSelection: vi.fn() })),
}))

vi.mock('~/api/articles', () => ({
  useArticlesApi: () => ({
    getArticle: vi.fn(async () => ({ success: true, data: null })),
    updateArticle: vi.fn(async () => ({ success: true })),
    getArticles: vi.fn(async () => ({ success: true, data: { items: [] } })),
  }),
}))

vi.mock('~/api/watchedTags', () => ({
  useWatchedTagsApi: () => ({
    listWatchedTags: vi.fn(async () => ({ success: true, data: [] })),
  }),
}))

vi.mock('~/composables/useOnboarding', async () => {
  const { ref } = await import('vue')
  return {
    useOnboarding: () => ({ isFirstRun: ref(false), startTour: vi.fn() }),
  }
})

// ── Nuxt 自动导入的全局桩（FeedLayoutShell 以裸标识符消费） ──
const apiStoreStub = {
  fetchFeeds: vi.fn(async () => ({ success: true })),
  refreshFeed: vi.fn(async () => ({ success: true })),
  refreshAllFeeds: vi.fn(async () => ({ success: true })),
  deleteCategory: vi.fn(async () => ({ success: true })),
}
const feedsStoreStub = {
  feeds: [] as unknown[],
  categories: [{ id: 'cat-1', name: '分类一', icon: 'mdi:folder', feedCount: 0 }],
  getFeedsByCategory: () => [] as unknown[],
  unreadCountsByFeed: {} as Record<string, number>,
}
const articlesStoreStub = {
  fetchArticlesStats: vi.fn(async () => ({ success: true, data: { unread: 0 } })),
  markAllAsRead: vi.fn(async () => ({ success: true })),
  favoriteCount: 0,
}

function makeArticle(id: number, title: string): Article {
  return {
    id: String(id),
    feedId: '1',
    title,
    description: '',
    content: '',
    link: '',
    pubDate: '2026-09-18T00:00:00Z',
    category: '',
    read: false,
    favorite: false,
  }
}

/**
 * matchMedia 可控桩（useMediaQuery.test.ts 同款）：按视口宽对 (max-width: Npx)
 * 求值并播发 change——断点跨转用 resizeTo 驱动。
 */
type ChangeListener = (event: MediaQueryListEvent) => void

function createMatchMediaStub() {
  let viewportWidth = 1280
  const created: {
    media: string
    readonly matches: boolean
    addEventListener: (type: string, listener: ChangeListener) => void
    removeEventListener: (type: string, listener: ChangeListener) => void
    listeners: Set<ChangeListener>
  }[] = []

  const matchMedia = vi.fn((query: string) => {
    const listeners = new Set<ChangeListener>()
    const evaluate = (): boolean => {
      const hit = /^\(max-width:\s*([\d.]+)px\)$/.exec(query)
      return hit !== null && viewportWidth <= Number.parseFloat(hit[1] ?? '')
    }
    const list = {
      media: query,
      get matches() {
        return evaluate()
      },
      addEventListener(type: string, listener: ChangeListener) {
        if (type === 'change') listeners.add(listener)
      },
      removeEventListener(type: string, listener: ChangeListener) {
        if (type === 'change') listeners.delete(listener)
      },
      listeners,
    }
    created.push(list)
    return list
  })

  function resizeTo(width: number): void {
    viewportWidth = width
    for (const list of created) {
      for (const listener of [...list.listeners]) {
        listener(new MediaQueryListEvent('change', { matches: list.matches, media: list.media }))
      }
    }
  }

  return { matchMedia, resizeTo }
}

type MatchMediaStub = ReturnType<typeof createMatchMediaStub>

/** happy-dom 无布局引擎：直接覆盖几何属性供滚动恢复逻辑消费 */
function setScrollGeometry(el: HTMLElement, scrollHeight: number, clientHeight: number): void {
  Object.defineProperty(el, 'scrollHeight', { value: scrollHeight, configurable: true })
  Object.defineProperty(el, 'clientHeight', { value: clientHeight, configurable: true })
}

function listContainerEl(): HTMLElement {
  const el = document.querySelector('[data-testid="virtual-list-container"]')
  expect(el).toBeTruthy()
  return el as HTMLElement
}

function mainContentEl(): HTMLElement {
  const el = document.querySelector('.main-content')
  expect(el).toBeTruthy()
  return el as HTMLElement
}

async function clickTestId(testId: string, scope = ''): Promise<void> {
  const el = document.querySelector(`${scope}[data-testid="${testId}"]`)
  expect(el, `element ${testId} not found`).toBeTruthy()
  ;(el as HTMLElement).click()
  await nextTick()
}

async function mountShell(): Promise<ReturnType<typeof mount<typeof FeedLayoutShell>>> {
  const wrapper = mount(FeedLayoutShell, { attachTo: document.body })
  await flushPromises()
  return wrapper
}

describe('FeedLayoutShell 窄屏集成（mobile-viewport-stage1）', () => {
  let stub: MatchMediaStub
  const originalMatchMedia = window.matchMedia

  beforeEach(() => {
    __mediaQueryClient.value = true
    stub = createMatchMediaStub()
    window.matchMedia = stub.matchMedia as unknown as typeof window.matchMedia
    vi.stubGlobal('useApiStore', () => apiStoreStub)
    vi.stubGlobal('useFeedsStore', () => feedsStoreStub)
    vi.stubGlobal('useArticlesStore', () => articlesStoreStub)
    vi.stubGlobal('CAT_FAVORITES', 'favorites')
    vi.stubGlobal('CAT_UNCATEGORIZED', 'uncategorized')
    vi.stubGlobal('CAT_WATCHED_TAGS', 'watched-tags')

    const mocks = articlesPublic.mocks!
    Object.assign(mocks.state, {
      articles: [makeArticle(1, '标题一'), makeArticle(2, '标题二')],
      hasMore: false,
      total: 2,
      loading: false,
    })
    mocks.loadMore.mockReset()
    mocks.loadMore.mockResolvedValue(undefined)
    mocks.fetchFirstPage.mockReset()
    mocks.fetchFirstPage.mockResolvedValue(undefined)
  })

  afterEach(() => {
    window.matchMedia = originalMatchMedia
    __mediaQueryClient.value = false
    vi.unstubAllGlobals()
    vi.clearAllMocks()
    document.body.innerHTML = ''
  })

  describe('宽屏零变化（≥768px 渲染路径不经过窄屏态）', () => {
    it('宽屏挂载：不渲染抽屉/返回条，主内容无窄屏类', async () => {
      stub.resizeTo(1280)
      await mountShell()
      expect(document.querySelector('.app-sidebar-drawer')).toBeNull()
      expect(document.querySelector('.reading-back-bar')).toBeNull()
      expect(mainContentEl().classList.contains('is-narrow')).toBe(false)
      expect(mainContentEl().classList.contains('is-reading')).toBe(false)
      expect(mainContentEl().classList.contains('is-content-empty')).toBe(false)
    })

    it('宽屏点列表项：双栏联动（selectedArticle 更新）且不进阅读态', async () => {
      stub.resizeTo(1280)
      await mountShell()
      await clickTestId('article-item-0')
      await flushPromises()

      expect(mainContentEl().classList.contains('is-reading')).toBe(false)
      expect(document.querySelector('.reading-back-bar')).toBeNull()
      expect(document.body.textContent).toContain('标题一')
    })
  })

  describe('单栏切换与滚动记忆（任务 3.1）', () => {
    it('窄屏点列表项进阅读态：is-reading + 返回条出现 + 内容显示所选文章', async () => {
      stub.resizeTo(375)
      await mountShell()
      expect(mainContentEl().classList.contains('is-narrow')).toBe(true)
      expect(mainContentEl().classList.contains('is-reading')).toBe(false)

      await clickTestId('article-item-0')
      await flushPromises()

      expect(mainContentEl().classList.contains('is-reading')).toBe(true)
      expect(document.querySelector('.reading-back-bar')).toBeTruthy()
      expect(document.body.textContent).toContain('标题一')
    })

    it('返回列表态：恢复离开时 scrollTop', async () => {
      stub.resizeTo(375)
      await mountShell()
      const el = listContainerEl()
      setScrollGeometry(el, 5000, 400)
      el.scrollTop = 1200

      await clickTestId('article-item-0')
      await clickTestId('reading-back-bar')
      await flushPromises()

      expect(mainContentEl().classList.contains('is-reading')).toBe(false)
      expect(el.scrollTop).toBe(1200)
    })

    it('恢复点超出已加载范围：先补一页再恢复（上限一次）', async () => {
      stub.resizeTo(375)
      await mountShell()
      const el = listContainerEl()
      setScrollGeometry(el, 1000, 400) // 可滚动范围 600 < 1200
      el.scrollTop = 1200
      articlesPublic.mocks!.state.hasMore = true
      articlesPublic.mocks!.loadMore.mockImplementation(async () => {
        // 补页后内容高度抬升
        setScrollGeometry(el, 6000, 400)
      })

      await clickTestId('article-item-0')
      await clickTestId('reading-back-bar')
      await flushPromises()

      expect(articlesPublic.mocks!.loadMore).toHaveBeenCalledTimes(1)
      expect(el.scrollTop).toBe(1200)
    })

    it('补页后仍不足：落顶且只补一页', async () => {
      stub.resizeTo(375)
      await mountShell()
      const el = listContainerEl()
      setScrollGeometry(el, 1000, 400)
      el.scrollTop = 1200
      articlesPublic.mocks!.state.hasMore = true
      // 补页后几何不变（模拟数据不足以覆盖恢复点）

      await clickTestId('article-item-0')
      await clickTestId('reading-back-bar')
      await flushPromises()

      expect(articlesPublic.mocks!.loadMore).toHaveBeenCalledTimes(1)
      expect(el.scrollTop).toBe(0)
    })

    it('无更多数据（hasMore=false）：恢复点超范围直接落顶，不触发补页', async () => {
      stub.resizeTo(375)
      await mountShell()
      const el = listContainerEl()
      setScrollGeometry(el, 1000, 400)
      el.scrollTop = 1200

      await clickTestId('article-item-0')
      await clickTestId('reading-back-bar')
      await flushPromises()

      expect(articlesPublic.mocks!.loadMore).not.toHaveBeenCalled()
      expect(el.scrollTop).toBe(0)
    })
  })

  describe('跨断点状态保持（任务 3.2）', () => {
    it('窄屏阅读态放宽 → 恢复宽屏形态，selectedArticle 保留', async () => {
      stub.resizeTo(375)
      await mountShell()
      await clickTestId('article-item-0')
      await flushPromises()
      expect(mainContentEl().classList.contains('is-reading')).toBe(true)

      stub.resizeTo(1280)
      await nextTick()

      expect(mainContentEl().classList.contains('is-narrow')).toBe(false)
      expect(mainContentEl().classList.contains('is-reading')).toBe(false)
      expect(document.querySelector('.reading-back-bar')).toBeNull()
      // 宽屏双栏联动：选中文章仍在内容面板
      expect(document.body.textContent).toContain('标题一')
    })

    it('宽屏缩窄 → 置回列表态（is-narrow 出现且无 is-reading）', async () => {
      stub.resizeTo(1280)
      await mountShell()
      await clickTestId('article-item-0')

      stub.resizeTo(375)
      await nextTick()

      expect(mainContentEl().classList.contains('is-narrow')).toBe(true)
      expect(mainContentEl().classList.contains('is-reading')).toBe(false)
    })
  })

  describe('抽屉接入（任务 2.3）', () => {
    it('窄屏点汉堡 → 抽屉打开且渲染 AppSidebarView 桩内容', async () => {
      stub.resizeTo(375)
      await mountShell()
      await clickTestId('header-open-drawer')

      const drawer = document.body.querySelector('.app-sidebar-drawer')
      expect(drawer).toBeTruthy()
      expect(drawer!.classList.contains('is-open')).toBe(true)
      const panel = document.body.querySelector('.app-sidebar-drawer__panel')
      expect(panel!.querySelector('[data-testid="drawer-feed-item"]')).toBeTruthy()
    })

    it('阅读态下点汉堡仍可开抽屉；抽屉内选订阅源 → 应用筛选 + 关抽屉 + 回列表态顶部', async () => {
      stub.resizeTo(375)
      await mountShell()
      const el = listContainerEl()
      setScrollGeometry(el, 5000, 400)
      el.scrollTop = 1200

      await clickTestId('article-item-0')
      expect(mainContentEl().classList.contains('is-reading')).toBe(true)

      await clickTestId('header-open-drawer')
      expect(document.body.querySelector('.app-sidebar-drawer')!.classList.contains('is-open')).toBe(true)

      // 常驻侧栏（窄屏仅 CSS 隐藏）与抽屉是同一桩组件两处实例：必须限定抽屉面板内点选
      await clickTestId('drawer-feed-item', '.app-sidebar-drawer__panel ')
      await flushPromises()

      // 应用筛选（复用宽屏 handleFeedClick 逻辑）
      expect(apiStoreStub.refreshFeed).toHaveBeenCalledWith('feed-1')
      // 关抽屉 + 回列表态
      expect(document.body.querySelector('.app-sidebar-drawer')!.classList.contains('is-open')).toBe(false)
      expect(mainContentEl().classList.contains('is-reading')).toBe(false)
      // 重置到列表态顶部（ui-design Interaction Contract）
      expect(el.scrollTop).toBe(0)
    })

    it('点遮罩 → 抽屉收起', async () => {
      stub.resizeTo(375)
      await mountShell()
      await clickTestId('header-open-drawer')
      expect(document.body.querySelector('.app-sidebar-drawer')!.classList.contains('is-open')).toBe(true)

      const scrim = document.body.querySelector('.app-sidebar-drawer__scrim') as HTMLElement
      scrim.click()
      await nextTick()

      expect(document.body.querySelector('.app-sidebar-drawer')!.classList.contains('is-open')).toBe(false)
    })

    it('宽屏点汉堡：抽屉不进入渲染路径（零变化红线）', async () => {
      stub.resizeTo(1280)
      await mountShell()
      await clickTestId('header-open-drawer')

      expect(document.querySelector('.app-sidebar-drawer')).toBeNull()
    })
  })
})
