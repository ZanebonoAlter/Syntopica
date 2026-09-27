import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, ref } from 'vue'

/**
 * AppHeaderView 窄屏适配测试（mobile-viewport-stage1 任务 2.2）：
 * - 汉堡按钮（.drawer-menu-btn）点击 → emit openDrawer（onOpenDrawer spy 断言）
 * - 「⋯」溢出菜单：开合/点外部关闭/菜单项触发原行为并收起
 * - 宽屏零变化锚：原 toggleSidebar 按钮/7 枚 header-btn 结构保持 + 源码级媒体查询锚
 *
 * 环境注意（预存在问题，非本 change 引入）：本机 happy-dom@20.8.4 + VTU@2.4.6 下
 * wrapper.emitted() 记录失效（devtools hook 捕获链断，见 NotificationPanel.test.ts 头注释），
 * 故全部用行为断言（props spy / DOM 断言）+ 源码级机械锚（layout.md「样式规则锚」）：
 * 窄屏/宽屏显隐是纯 CSS 媒体查询（happy-dom 不应用媒体查询样式），视口行为由 major UI
 * 验收截图分流，此处锁「元素存在 + 宽屏默认 display:none 的样式规则存在」。
 */

// matchMedia 防御性兜底（useOnboarding 读取 prefers-reduced-motion；断言不依赖匹配值）
beforeEach(() => {
  if (typeof window.matchMedia !== 'function') {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      configurable: true,
      value: (query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: () => {},
        removeListener: () => {},
        addEventListener: () => {},
        removeEventListener: () => {},
        dispatchEvent: () => false,
      }),
    })
  }
})

const navigateToSpy = vi.fn(() => Promise.resolve())
vi.stubGlobal('navigateTo', navigateToSpy)

// #imports stub（test/stubs/nuxt-imports.ts）未覆盖 useHead：useTheme / useAnalysisPauseFavicon 需要
vi.mock('#imports', async (importOriginal) => {
  const actual = await importOriginal<Record<string, unknown>>()
  return { ...actual, useHead: () => {} }
})

const schedulerMocks = {
  loadSchedulersStatus: vi.fn(),
  setAnalysisPaused: vi.fn(async () => ({ ok: true, message: '已恢复' })),
}
vi.mock('~/composables/useSchedulerStatus', () => ({
  useSchedulerStatus: () => ({
    analysisPaused: ref(false),
    aiHealthy: ref(true),
    loadSchedulersStatus: schedulerMocks.loadSchedulersStatus,
    setAnalysisPaused: schedulerMocks.setAnalysisPaused,
  }),
}))

vi.mock('~/composables/useNotifications', () => ({
  useNotifications: () => ({
    unreadCount: ref(0),
    openPanel: vi.fn(),
    closePanel: vi.fn(),
  }),
}))

vi.mock('~/composables/useTagQueueProgress', () => ({
  useTagQueueProgress: () => ({
    status: ref({ pending: 0, processing: 0, completedToday: 0, failed: 0 }),
    activeCount: ref(0),
    visible: ref(false),
    isFailedState: ref(false),
    roundTotal: ref(0),
    progressPercent: ref(0),
    ensureStarted: vi.fn(),
    stop: vi.fn(),
    reconcile: vi.fn(async () => {}),
  }),
}))

import AppHeaderView from './AppHeaderView.vue'
import viewSourceRaw from './AppHeaderView.vue?raw'

const VIEW_SOURCE = viewSourceRaw as string

function mountView() {
  const spies = {
    onOpenDrawer: vi.fn(),
    onToggleSidebar: vi.fn(),
    onRefresh: vi.fn(),
    onMarkAllRead: vi.fn(),
    onSettings: vi.fn(),
  }
  const wrapper = mount(AppHeaderView, { props: spies, attachTo: document.body })
  return { wrapper, ...spies }
}

function btnByTitle(title: string): HTMLButtonElement {
  return document.body.querySelector(`button[title="${title}"]`) as HTMLButtonElement
}

function overflowItems(): HTMLButtonElement[] {
  return [...document.body.querySelectorAll('.overflow-item')] as HTMLButtonElement[]
}

afterEach(() => {
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

describe('窄屏汉堡按钮（openDrawer 契约）', () => {
  it('汉堡按钮存在，点击 → emit openDrawer，不触发宽屏 toggleSidebar', async () => {
    const { onOpenDrawer, onToggleSidebar } = mountView()
    const burger = document.body.querySelector('.drawer-menu-btn') as HTMLButtonElement
    expect(burger).toBeTruthy()
    burger.click()
    await nextTick()
    expect(onOpenDrawer).toHaveBeenCalledTimes(1)
    expect(onToggleSidebar).not.toHaveBeenCalled()
  })

  it('宽屏 toggleSidebar 按钮语义隔离：点击只走 toggleSidebar', async () => {
    const { onOpenDrawer, onToggleSidebar } = mountView()
    const wideMenuBtn = document.body.querySelector(
      '.logo-container .menu-btn',
    ) as HTMLButtonElement
    expect(wideMenuBtn).toBeTruthy()
    wideMenuBtn.click()
    await nextTick()
    expect(onToggleSidebar).toHaveBeenCalledTimes(1)
    expect(onOpenDrawer).not.toHaveBeenCalled()
  })
})

describe('「⋯」溢出菜单（窄屏收纳）', () => {
  it('初始关闭；点「⋯」展开、再点收起（toggle）', async () => {
    mountView()
    expect(document.body.querySelector('.overflow-menu')).toBeNull()

    const moreBtn = btnByTitle('更多操作')
    expect(moreBtn).toBeTruthy()
    moreBtn.click()
    await nextTick()
    expect(document.body.querySelector('.overflow-menu')).toBeTruthy()

    moreBtn.click()
    await nextTick()
    expect(document.body.querySelector('.overflow-menu')).toBeNull()
  })

  it('菜单展开时点击外部（document click）→ 关闭', async () => {
    mountView()
    btnByTitle('更多操作').click()
    await nextTick()
    expect(document.body.querySelector('.overflow-menu')).toBeTruthy()

    document.body.click()
    await nextTick()
    expect(document.body.querySelector('.overflow-menu')).toBeNull()
  })

  it('溢出项「刷新」→ emit refresh 且菜单收起（点选即关）', async () => {
    const { onRefresh } = mountView()
    btnByTitle('更多操作').click()
    await nextTick()

    const refreshItem = overflowItems().find((b) => b.textContent?.includes('刷新'))
    expect(refreshItem).toBeTruthy()
    refreshItem!.click()
    await nextTick()
    expect(onRefresh).toHaveBeenCalledTimes(1)
    expect(document.body.querySelector('.overflow-menu')).toBeNull()
  })

  it('溢出项复用宽屏同一行为入口：「暂停分析」走 setAnalysisPaused', async () => {
    mountView()
    btnByTitle('更多操作').click()
    await nextTick()

    const pauseItem = overflowItems().find((b) => b.textContent?.includes('暂停分析'))
    pauseItem!.click()
    await nextTick()
    expect(schedulerMocks.setAnalysisPaused).toHaveBeenCalledWith(true)
    expect(document.body.querySelector('.overflow-menu')).toBeNull()
  })

  it('unmount 后移除 document click 监听（清理不抛错）', () => {
    const { wrapper } = mountView()
    wrapper.unmount()
    expect(() =>
      document.dispatchEvent(new MouseEvent('click', { bubbles: true })),
    ).not.toThrow()
  })
})

describe('宽屏零变化锚（结构 + 源码机械锚）', () => {
  it('宽屏结构完整：7 枚次要操作 header-btn 仍渲染在 wide-only-group 内', () => {
    mountView()
    expect(document.body.querySelectorAll('.wide-only-group .header-btn')).toHaveLength(7)
    expect(document.body.querySelector('.logo-container .menu-btn')).toBeTruthy()
    expect(document.body.querySelector('.header-divider')).toBeTruthy()
  })

  it('样式规则锚：媒体查询断点 767.98px；新增元素宽屏默认 display:none；包裹层宽屏 display:contents', () => {
    expect(VIEW_SOURCE).toContain('@media (max-width: 767.98px)')
    // 汉堡与「⋯」容器宽屏隐藏（红线：新增元素只在窄屏媒体块内可见）
    expect(VIEW_SOURCE).toMatch(/\.drawer-menu-btn\s*\{\s*display: none/)
    expect(VIEW_SOURCE).toMatch(/\.mobile-overflow\s*\{[^}]*display: none/)
    // 宽屏包裹层不产生盒子（header-right flex 布局与重构前一致）
    expect(VIEW_SOURCE).toMatch(/\.wide-only-group\s*\{\s*display: contents/)
  })
})
