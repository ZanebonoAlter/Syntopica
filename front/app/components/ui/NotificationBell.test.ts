import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'

/**
 * NotificationBell 组件测试（notification-center）
 *
 * 回归背景（review H2）：外点关闭守卫曾用不存在的 `.notif-bell` 选择器，
 * 点击铃铛打开即被 document 级 click 立即关闭、面板内点击也立即关闭。
 * 本文件渲染真实双组件（Bell + Panel，面板 Teleport 到 body），验证事件行为：
 * - 点铃铛打开且保持打开
 * - 面板外 document click 关闭
 * - 面板内点击不关闭
 * - Esc 关闭
 */

const openPanel = vi.fn(async () => {})
const closePanel = vi.fn()
const unreadCount = ref(2)
const browsingSessionActive = ref(false)
const view = ref({
  list: [] as never[],
  total: 0,
  loading: false,
  error: null as string | null,
})
const panelMocks = {
  markRead: vi.fn(async () => {}),
  markAllRead: vi.fn(async () => true),
  clearAll: vi.fn(async () => true),
  fetchList: vi.fn(async () => {}),
  hasMore: vi.fn(() => false),
}

vi.mock('~/composables/useNotifications', () => ({
  useNotifications: () => ({
    unreadCount,
    openPanel,
    closePanel,
    // NotificationPanel 需要的完整面（Bell 渲染真实 Panel，缺字段会在 render 报 undefined）
    view,
    browsingSessionActive,
    ...panelMocks,
    pageSize: 20,
  }),
}))

import NotificationBell from './NotificationBell.vue'

import type { VueWrapper } from '@vue/test-utils'

const mounted: VueWrapper[] = []

function mountBell(): VueWrapper {
  const wrapper = mount(NotificationBell, { attachTo: document.body })
  mounted.push(wrapper)
  return wrapper
}

async function clickBell(wrapper: ReturnType<typeof mountBell>) {
  await wrapper.find('[data-testid="notification-bell"]').trigger('click')
  await nextTick()
}

function panelEl(): HTMLElement | null {
  return document.querySelector('[data-testid="notification-panel"]')
}

beforeEach(() => {
  document.body.innerHTML = ''
  openPanel.mockClear()
  closePanel.mockClear()
  unreadCount.value = 2
})

afterEach(() => {
  // attachTo 挂载必须显式 unmount：否则 document 级 click/keydown 监听跨用例残留
  while (mounted.length) {
    mounted.pop()?.unmount()
  }
  document.body.innerHTML = ''
})

describe('NotificationBell — 开关与外点关闭（H2 回归锚）', () => {
  it('点铃铛打开面板，且不被 document 级 click 立即关闭（H2：wrapper 类在守卫内）', async () => {
    const wrapper = mountBell()
    await clickBell(wrapper)
    expect(panelEl()).toBeTruthy()
    expect(openPanel).toHaveBeenCalledTimes(1)

    // click 事件目标落在铃铛 wrapper 内（badge 区域，不触发按钮 toggle）→ 不误关
    const wrapEl = document.querySelector('.notif-bell-wrap') as HTMLElement
    wrapEl.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(panelEl()).toBeTruthy()
    expect(closePanel).not.toHaveBeenCalled()
  })

  it('面板内点击不关闭（面板根类 .notif-panel 在守卫内）', async () => {
    const wrapper = mountBell()
    await clickBell(wrapper)
    const panel = panelEl() as HTMLElement
    expect(panel).toBeTruthy()
    // 面板标题区域点击（Teleport 后的 body 内节点）→ 派发到元素上，冒泡到 document
    const head = panel.querySelector('.notif-panel__head') as HTMLElement
    head.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(panelEl()).toBeTruthy()
    expect(closePanel).not.toHaveBeenCalled()
  })

  it('面板外 document click 关闭', async () => {
    const outside = document.createElement('div')
    outside.setAttribute('data-testid', 'outside-zone')
    document.body.appendChild(outside)

    const wrapper = mountBell()
    await clickBell(wrapper)
    expect(panelEl()).toBeTruthy()

    outside.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(panelEl()).toBeNull()
    expect(closePanel).toHaveBeenCalledTimes(1)
  })

  it('再次点击铃铛关闭（toggle）', async () => {
    const wrapper = mountBell()
    await clickBell(wrapper)
    expect(panelEl()).toBeTruthy()
    await clickBell(wrapper)
    await nextTick()
    expect(panelEl()).toBeNull()
    expect(closePanel).toHaveBeenCalledTimes(1)
  })

  it('Esc 关闭', async () => {
    const wrapper = mountBell()
    await clickBell(wrapper)
    expect(panelEl()).toBeTruthy()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(panelEl()).toBeNull()
  })

  it('未读角标：未读数 >0 显示，0 隐藏；>99 显示 99+', async () => {
    const wrapper = mountBell()
    await clickBell(wrapper)
    await nextTick()
    expect(wrapper.find('[data-testid="notification-badge"]').text()).toBe('2')

    unreadCount.value = 120
    await nextTick()
    expect(wrapper.find('[data-testid="notification-badge"]').text()).toBe('99+')

    unreadCount.value = 0
    await nextTick()
    expect(wrapper.find('[data-testid="notification-badge"]').exists()).toBe(false)
  })
})
