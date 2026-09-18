import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AppSidebarDrawer from './AppSidebarDrawer.vue'

/**
 * 窄屏抽屉容器契约（mobile-viewport-stage1，ui-design.md Layout Contract）：
 * 锁 Teleport 挂载/开合态 class/遮罩与 Esc 关闭/min(80vw, 320px) 宽度约束/语义 role；
 * 260ms 滑入动效与双主题配色属视觉行为，由 major UI 验收截图分流。
 *
 * 关闭回调经 onClose 监听器 prop 断言（而非 wrapper.emitted()）：
 * 本环境 VTU 2.4.6 的 emitted 捕获依赖 devtools hook，在 pnpm 多 Vue runtime 实例
 * 下断链（既有 AppDialog.test.ts 交互用例同样受影响），监听器 prop 是稳定等价断言。
 */

function mountDrawer(props: Record<string, unknown> = {}, slot = '') {
  const onClose = vi.fn()
  const wrapper = mount(AppSidebarDrawer, {
    props: { open: true, onClose, ...props },
    slots: { default: slot },
    attachTo: document.body,
  })
  return { wrapper, onClose }
}

function drawerEl(): HTMLElement {
  return document.body.querySelector('.app-sidebar-drawer') as HTMLElement
}

function scrimEl(): HTMLElement {
  return document.body.querySelector('.app-sidebar-drawer__scrim') as HTMLElement
}

function panelEl(): HTMLElement {
  return document.body.querySelector('.app-sidebar-drawer__panel') as HTMLElement
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('AppSidebarDrawer 开合态', () => {
  it('open=false：容器常驻但无 is-open（关闭态 visibility:hidden + 面板 translateX(-100%) 由样式表定义）', () => {
    mountDrawer({ open: false })
    expect(drawerEl()).toBeTruthy()
    expect(drawerEl().classList.contains('is-open')).toBe(false)
  })

  it('open=true：is-open 态且默认插槽内容渲染进面板', async () => {
    const { wrapper } = mountDrawer({ open: false }, '<nav>订阅源列表</nav>')
    expect(panelEl().textContent).toContain('订阅源列表')
    await wrapper.setProps({ open: true })
    expect(drawerEl().classList.contains('is-open')).toBe(true)
  })
})

describe('AppSidebarDrawer 关闭交互', () => {
  it('点击遮罩（scrim）→ emit close', async () => {
    const { onClose } = mountDrawer()
    await scrimEl().click()
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('点击面板内部不触发 close（面板是遮罩的兄弟层，点击不冒泡到遮罩）', async () => {
    const { onClose } = mountDrawer({}, '<button>筛选</button>')
    await panelEl().click()
    expect(onClose).not.toHaveBeenCalled()
  })

  it('Esc 键 → emit close（open 态监听 window keydown）', () => {
    const { onClose } = mountDrawer()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('关闭态不监听 keydown；open→false 后监听解除', async () => {
    const closed = mountDrawer({ open: false })
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(closed.onClose).not.toHaveBeenCalled()

    const { wrapper, onClose } = mountDrawer()
    await wrapper.setProps({ open: false })
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(onClose).not.toHaveBeenCalled()
  })
})

describe('AppSidebarDrawer 布局契约', () => {
  it('面板宽度约束 --drawer-w = min(80vw, 320px)（ui-design.md Layout Contract「抽屉」行）', () => {
    mountDrawer()
    expect(panelEl().style.getPropertyValue('--drawer-w')).toBe('min(80vw, 320px)')
  })

  it('a11y：面板 role=dialog + aria-modal=true，遮罩 aria-hidden 与面板为兄弟层，抽屉根 Teleport 挂到 body', () => {
    mountDrawer()
    expect(panelEl().getAttribute('role')).toBe('dialog')
    expect(panelEl().getAttribute('aria-modal')).toBe('true')
    expect(scrimEl().getAttribute('aria-hidden')).toBe('true')
    // aria-hidden 祖先不得包裹 role=dialog 面板：scrim 与 panel 是兄弟
    expect(scrimEl().parentElement).toBe(drawerEl())
    expect(panelEl().parentElement).toBe(drawerEl())
    expect(drawerEl().parentElement).toBe(document.body)
  })
})
