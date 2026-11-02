import { readFileSync } from 'node:fs'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AppDrawer from './AppDrawer.vue'
import { hasOpenOverlay, resetOverlayStack } from './overlayStack'

/**
 * 通用抽屉契约（layout.md 抽屉档 + 浮层展示合理性锚）：
 * Teleport 到 body / overlay 样式规则存在 / 遮罩与 Esc 关闭 / 32rem 宽度与 92vw 上限 /
 * body 滚动锁定的开合恢复。
 *
 * 关闭回调用 `onClose` 监听器断言（与 AppSidebarDrawer 同因：VTU emitted 在
 * pnpm 多 Vue runtime 下不可靠）。
 */

function mountDrawer(props: Record<string, unknown> = {}, slots: Record<string, string> = {}) {
  const onClose = vi.fn()
  return {
    wrapper: mount(AppDrawer, {
      props: { open: true, label: '测试抽屉', onClose, ...props },
      slots,
      attachTo: document.body,
    }),
    onClose,
  }
}

const drawerEl = () => document.body.querySelector('.ad') as HTMLElement | null
const scrimEl = () => document.body.querySelector('.ad-scrim') as HTMLElement
const panelEl = () => document.body.querySelector('.ad-panel') as HTMLElement

afterEach(() => {
  document.body.innerHTML = ''
  document.body.style.overflow = ''
  resetOverlayStack()
})

describe('AppDrawer 渲染与语义', () => {
  it('open=true 时 Teleport 到 body，且 panel 带 dialog 语义与 aria-label', () => {
    mountDrawer()
    expect(drawerEl()).not.toBeNull()
    expect(document.body.querySelector(':scope > .ad')).not.toBeNull()
    expect(panelEl()).toBeTruthy()
    expect(document.body.querySelector('[role="dialog"]')).toBe(panelEl())
    expect(document.body.querySelector('[role="dialog"]')?.getAttribute('aria-modal')).toBe('true')
    expect(document.body.querySelector('[role="dialog"]')?.getAttribute('aria-label')).toBe('测试抽屉')
  })

  it('open=false 时不渲染任何浮层', () => {
    mountDrawer({ open: false })
    expect(drawerEl()).toBeNull()
  })

  it('head / default / footer 三个插槽分别落位；无 footer 插槽不渲染动作条', () => {
    mountDrawer({}, {
      head: '<span>头部</span>',
      default: '<p>正文</p>',
      footer: '<button>动作</button>',
    })
    expect(document.body.querySelector('.ad-head')!.textContent).toContain('头部')
    expect(document.body.querySelector('.ad-body')!.textContent).toContain('正文')
    expect(document.body.querySelector('.ad-foot')!.textContent).toContain('动作')
  })

  it('width 属性落到 panel，并受 92vw 上限约束', () => {
    mountDrawer({ width: '40rem' })
    expect((panelEl() as HTMLElement).style.width).toBe('40rem')
    const source = readFileSync(`${import.meta.dirname}/AppDrawer.vue`, 'utf8')
    expect(source).toMatch(/max-width:\s*92vw/)
  })
})

describe('AppDrawer 关闭交互与滚动锁', () => {
  it('遮罩点击 / Esc / 关闭按钮都触发 close', () => {
    const { onClose } = mountDrawer()
    scrimEl().dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onClose).toHaveBeenCalledTimes(1)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(onClose).toHaveBeenCalledTimes(2)

    document.body.querySelector('.ad-close')!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onClose).toHaveBeenCalledTimes(3)
  })

  it('打开时锁定 body 滚动，关闭后恢复原值', async () => {
    document.body.style.overflow = 'auto'
    const { wrapper } = mountDrawer()
    expect(document.body.style.overflow).toBe('hidden')

    await wrapper.setProps({ open: false })
    expect(document.body.style.overflow).toBe('auto')
  })

  it('打开时登记浮层栈，关闭/卸载后出栈（阅读台 Esc 让位依赖）', async () => {
    const { wrapper } = mountDrawer()
    expect(hasOpenOverlay()).toBe(true)
    await wrapper.setProps({ open: false })
    expect(hasOpenOverlay()).toBe(false)
    await wrapper.setProps({ open: true })
    expect(hasOpenOverlay()).toBe(true)
    wrapper.unmount()
    expect(hasOpenOverlay()).toBe(false)
  })

  it('关闭后 Esc 不再触发 close', async () => {
    const { wrapper, onClose } = mountDrawer()
    await wrapper.setProps({ open: false })
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(onClose).not.toHaveBeenCalled()
  })
})

describe('AppDrawer 浮层展示合理性锚（layout.md）', () => {
  it('overlay 规则：position fixed + inset 0 + z-index；panel 绝对定位贴右', () => {
    const source = readFileSync(`${import.meta.dirname}/AppDrawer.vue`, 'utf8')
    const style = source.slice(source.indexOf('<style'))
    expect(style).toMatch(/\.ad\s*\{[^}]*position:\s*fixed/s)
    expect(style).toMatch(/\.ad\s*\{[^}]*inset:\s*0/s)
    expect(style).toMatch(/\.ad\s*\{[^}]*z-index:/s)
    expect(style).toMatch(/\.ad-panel\s*\{[^}]*position:\s*absolute/s)
    expect(style).toMatch(/\.ad-panel\s*\{[^}]*right:\s*0/s)
  })
})
