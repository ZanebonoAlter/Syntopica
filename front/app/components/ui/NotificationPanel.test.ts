import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import { mount } from '@vue/test-utils'

// M4：通知点击跳转（daily-report → /tags）；TagQueueProgressChip.test.ts 同款 stub 先例
const navigateToSpy = vi.fn(() => Promise.resolve())
vi.stubGlobal('navigateTo', navigateToSpy)

/**
 * NotificationPanel 组件测试（notification-center，S1 故事 + 白盒 D 组）
 *
 * 环境注意（预存在问题，非本 change 引入）：本机 happy-dom@20.8.4 + VTU@2.4.6 下
 * wrapper.emitted() 记录失效（devtools hook 捕获链断），既有 AppDialog.test.ts /
 * TopicWatchCreateDialog.test.ts 的 emitted 断言同样失败。故本文件全部用
 * 行为断言（mock 调用 / DOM 断言）+ 源码级机械锚（layout.md「样式规则锚」）替代 emitted()。
 *
 * 机械锚（standard/frontend/layout.md「浮层组件展示合理性锚」）：
 * - 样式规则锚：SFC 源码 <style> 含 position:fixed + z-index + 380px 宽约束（下方源码断言）
 * - Teleport 挂载锚：面板渲染在 document.body 而非组件树内
 */

const view = ref({ list: [] as never[], total: 0, loading: false, error: null as string | null })
const unreadCount = ref(0)
const browsingSessionActive = ref(true)

const notifMocks = {
  markRead: vi.fn(async () => {}),
  markAllRead: vi.fn(async () => true),
  clearAll: vi.fn(async () => true),
  fetchList: vi.fn(async () => {}),
}

vi.mock('~/composables/useNotifications', () => ({
  useNotifications: () => ({
    view,
    unreadCount,
    browsingSessionActive,
    markRead: notifMocks.markRead,
    markAllRead: notifMocks.markAllRead,
    clearAll: notifMocks.clearAll,
    hasMore: vi.fn(() => false),
    pageSize: 20,
    fetchList: notifMocks.fetchList,
  }),
}))

import AppButton from './AppButton.vue'
import NotificationPanel from './NotificationPanel.vue'
import panelSourceRaw from './NotificationPanel.vue?raw'
import itemSourceRaw from './NotificationItem.vue?raw'

const PANEL_SOURCE = panelSourceRaw as string
const ITEM_SOURCE = itemSourceRaw as string

const items = [
  { id: '1', type: 'success', title: '日报已生成 · 9-17', summary: '共 6 个版面，保存 42 条', link_type: null, link_id: null, is_read: false, created_at: '2026-09-17T04:00:00Z' },
  { id: '2', type: 'error', title: '日报生成有失败 · 9-16', summary: '5 成功 1 失败', link_type: 'daily-report', link_id: '9', is_read: false, created_at: '2026-09-16T21:00:00Z' },
] as const

function mountPanel() {
  return mount(NotificationPanel, {
    attachTo: document.body,
    // AppButton 走 Nuxt 自动导入（测试环境需手动提供）；AppDialog 是面板显式导入（真实渲染）
    global: { components: { AppButton } },
  })
}

function panelEl(): HTMLElement | null {
  return document.body.querySelector('[data-testid="notification-panel"]')
}

afterEach(() => {
  document.body.innerHTML = ''
  vi.clearAllMocks()
  view.value = { list: [], total: 0, loading: false, error: null }
})

describe('NotificationPanel — 机械锚（Teleport + 浮层样式规则锚）', () => {
  it('Teleport 挂载锚：面板渲染在 document.body 而非组件树内', () => {
    mountPanel()
    expect(panelEl()).toBeTruthy()
  })

  it('浮层样式规则锚：源码 <style> 含 position:fixed + z-index + 380px 宽 + 480px 高（防样式重构丢定位）', () => {
    const styleBlock = PANEL_SOURCE.slice(PANEL_SOURCE.indexOf('<style scoped>'))
    expect(styleBlock).toMatch(/\.notif-panel\s*\{[\s\S]*?position:\s*fixed/)
    expect(styleBlock).toMatch(/\.notif-panel\s*\{[\s\S]*?width:\s*min\(380px,\s*92vw\)/)
    expect(styleBlock).toMatch(/\.notif-panel\s*\{[\s\S]*?max-height:\s*480px/)
    expect(styleBlock).toMatch(/\.notif-panel\s*\{[\s\S]*?z-index:/)
  })
})

describe('NotificationPanel — 状态矩阵（ui-design State Matrix）', () => {
  it('loading 态：首次拉取显示骨架行', () => {
    view.value = { list: [], total: 0, loading: true, error: null }
    mountPanel()
    expect(document.body.querySelectorAll('[data-testid="panel-skeleton"]').length).toBeGreaterThan(0)
  })

  it('empty 态（V11）：无任何通知显示「暂无通知」空态', () => {
    mountPanel()
    const empty = document.body.querySelector('[data-testid="panel-empty"]')
    expect(empty?.textContent).toContain('暂无通知')
  })

  it('error 态（V12）：面板内错误 + 重试按钮；重试触发 fetchList', async () => {
    view.value = { list: [], total: 0, loading: false, error: '加载失败' }
    mountPanel()
    const err = document.body.querySelector('[data-testid="panel-error"]')
    expect(err?.textContent).toContain('加载失败')
    const retry = err?.querySelector('button')
    await retry?.click()
    expect(notifMocks.fetchList).toHaveBeenCalledWith(0)
  })

  it('success 态：渲染条目且未读条目有标已读按钮', () => {
    view.value = { list: [...items] as never[], total: 2, loading: false, error: null }
    mountPanel()
    expect(document.body.querySelectorAll('[data-testid="notification-item"]').length).toBe(2)
    expect(document.body.querySelectorAll('[data-testid="notification-item-mark-read"]').length).toBe(2)
  })

  it('V15：超长摘要 line-clamp 截断（样式锚：NotificationItem 源码含 -webkit-line-clamp: 2）', () => {
    expect(ITEM_SOURCE).toMatch(/-webkit-line-clamp:\s*2/)
    expect(ITEM_SOURCE).toMatch(/overflow:\s*hidden/)
  })
})

describe('NotificationPanel — 浏览与已读语义（白盒 D1/D2）', () => {
  it('打开即算浏览：浏览提示可见、条目未读强调保留（highlight-unread）', () => {
    view.value = { list: [...items] as never[], total: 2, loading: false, error: null }
    mountPanel()
    expect(document.body.querySelector('[data-testid="panel-browsing-hint"]')).toBeTruthy()
    const unreadItems = document.body.querySelectorAll('.notif-item--unread')
    expect(unreadItems.length).toBe(2)
  })

  it('D2：重复打开幂等——两次渲染各自独立、无报错', () => {
    view.value = { list: [...items] as never[], total: 2, loading: false, error: null }
    mountPanel()
    mount(NotificationPanel, {
      attachTo: document.body,
      global: { components: { AppButton } },
    })
    expect(document.body.querySelectorAll('[data-testid="notification-panel"]').length).toBe(2)
  })

  it('全部标已读触发 composable markAllRead', async () => {
    view.value = { list: [...items] as never[], total: 2, loading: false, error: null }
    mountPanel()
    const btn = document.body.querySelector('[data-testid="mark-all-read"]') as HTMLButtonElement
    await btn.click()
    expect(notifMocks.markAllRead).toHaveBeenCalled()
  })

  it('V14：清空先弹 confirm（真实 AppDialog sm 档），未确认不删除；确认后清除', async () => {
    view.value = { list: [...items] as never[], total: 2, loading: false, error: null }
    mountPanel()
    const clearBtn = document.body.querySelector('[data-testid="panel-clear"]') as HTMLButtonElement
    await clearBtn.click()
    await nextTick()
    // confirm 弹窗已出现，clearAll 未被调用
    expect(notifMocks.clearAll).not.toHaveBeenCalled()
    const confirmBtn = document.body.querySelector('[data-testid="panel-clear-confirm"]') as HTMLButtonElement
    expect(confirmBtn).toBeTruthy()
    await confirmBtn.click()
    expect(notifMocks.clearAll).toHaveBeenCalled()
  })

  it('单条点击：标已读 + 通知父层关闭（onClose spy 断言 emit 契约）', async () => {
    view.value = { list: [...items] as never[], total: 2, loading: false, error: null }
    const wrapper = mountPanel()
    const item = document.body.querySelectorAll('[data-testid="notification-item"]')[0] as HTMLElement
    // 环境限制（happy-dom@20.8.4 下 VTU emitted() 记录失效，见文件头注释）：
    // 以 onClose props spy 验证 emit('close') 契约
    const props = (wrapper.vm.$ as { vnode: { props: Record<string, unknown> } }).vnode.props
    const onClose = vi.fn()
    props.onClose = onClose as never
    item.click()
    await nextTick()
    expect(notifMocks.markRead).toHaveBeenCalledWith('1')
    expect(onClose).toHaveBeenCalled()
  })

  it('M4：daily-report 通知点击跳 /tags；无 link_type 的通知不跳转', async () => {
    view.value = { list: [...items] as never[], total: 2, loading: false, error: null }
    mountPanel()
    const panelItems = document.body.querySelectorAll('[data-testid="notification-item"]')
    // item[1] 是日报失败通知（link_type=daily-report）→ 跳转
    ;(panelItems[1] as HTMLElement).click()
    await nextTick()
    expect(navigateToSpy).toHaveBeenCalledWith('/tags')

    // 无 link_type（item[0]）→ 不触发跳转
    navigateToSpy.mockClear()
    ;(panelItems[0] as HTMLElement).click()
    await nextTick()
    expect(navigateToSpy).not.toHaveBeenCalled()
  })
})
