import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { Icon } from '@iconify/vue'

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

// AI 未就绪警示态（ai-health-to-notifications）：useSchedulerStatus 的
// analysisPaused/aiHealthy 是 useState 共享态，用可控 ref 替换（AppHeaderView.test.ts
// 同款 mock 先例；ref 保证状态翻转可响应）。默认健康 → 面板置顶条不渲染。
const schedulerState = {
  analysisPaused: ref(false),
  aiHealthy: ref(true),
}
const reprobeMocks = {
  reprobing: ref(false),
  reprobeHealth: vi.fn(async () => null),
  loadSchedulersStatus: vi.fn(async () => {}),
}

vi.mock('~/composables/useSchedulerStatus', () => ({
  useSchedulerStatus: () => ({
    analysisPaused: schedulerState.analysisPaused,
    aiHealthy: schedulerState.aiHealthy,
    loadSchedulersStatus: reprobeMocks.loadSchedulersStatus,
  }),
}))

vi.mock('~/composables/useHealthReprobe', () => ({
  useHealthReprobe: () => ({
    reprobing: reprobeMocks.reprobing,
    reprobeHealth: reprobeMocks.reprobeHealth,
  }),
}))

import NotificationBell from './NotificationBell.vue'

import type { VueWrapper } from '@vue/test-utils'

const mounted: VueWrapper[] = []

function mountBell(): VueWrapper {
  const wrapper = mount(NotificationBell, {
    attachTo: document.body,
    // NuxtLink 自动导入组件（警示态下真实面板的置顶条会用到）测试环境手动提供
    global: { components: { NuxtLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } },
  })
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
  // 警示态状态复位（默认健康）
  schedulerState.analysisPaused.value = false
  schedulerState.aiHealthy.value = true
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

describe('NotificationBell — AI 未就绪警示态（与未读角标正交）', () => {
  it('健康/暂停时普通态：bell-outline 图标 + 默认配色 + title「通知」', () => {
    const wrapper = mountBell()
    const icon = wrapper.findComponent(Icon)
    expect(icon.props('icon')).toBe('mdi:bell-outline')
    expect(icon.classes()).toContain('text-gray-600')
    expect(wrapper.find('[data-testid="notification-bell"]').attributes('title')).toBe('通知')
  })

  it('意图运行但不健康时警示态：bell-alert 图标 + warning 配色 + title 提示', async () => {
    const wrapper = mountBell()
    schedulerState.aiHealthy.value = false
    await nextTick()
    const icon = wrapper.findComponent(Icon)
    expect(icon.props('icon')).toBe('mdi:bell-alert')
    expect(icon.classes()).toContain('notif-bell--warning')
    expect(wrapper.find('[data-testid="notification-bell"]').attributes('title')).toContain('AI 模型未就绪')
  })

  it('用户主动暂停时不警示（已知暂停，无需再提示健康）', async () => {
    const wrapper = mountBell()
    schedulerState.analysisPaused.value = true
    schedulerState.aiHealthy.value = false
    await nextTick()
    expect(wrapper.findComponent(Icon).props('icon')).toBe('mdi:bell-outline')
  })

  it('健康恢复后回归普通态（状态驱动）', async () => {
    const wrapper = mountBell()
    schedulerState.aiHealthy.value = false
    await nextTick()
    expect(wrapper.findComponent(Icon).props('icon')).toBe('mdi:bell-alert')
    schedulerState.aiHealthy.value = true
    await nextTick()
    expect(wrapper.findComponent(Icon).props('icon')).toBe('mdi:bell-outline')
  })

  it('正交叠加：警示态下未读角标仍按未读数显示/隐藏，数值不受影响', async () => {
    const wrapper = mountBell()
    schedulerState.aiHealthy.value = false
    await nextTick()
    // 未读 2（beforeEach 默认）+ 警示态 → 双信号并存
    expect(wrapper.findComponent(Icon).props('icon')).toBe('mdi:bell-alert')
    expect(wrapper.find('[data-testid="notification-badge"]').text()).toBe('2')

    unreadCount.value = 0
    await nextTick()
    // 角标归零不吞警示：警示图标仍在、角标消失
    expect(wrapper.findComponent(Icon).props('icon')).toBe('mdi:bell-alert')
    expect(wrapper.find('[data-testid="notification-badge"]').exists()).toBe(false)
  })
})
