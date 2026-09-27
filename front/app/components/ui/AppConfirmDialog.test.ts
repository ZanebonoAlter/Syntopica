import { afterEach, describe, expect, it } from 'vitest'
import type { VueWrapper } from '@vue/test-utils'
import { mount } from '@vue/test-utils'
import AppConfirmDialog from './AppConfirmDialog.vue'
import { useConfirm } from '~/composables/useConfirm'

/**
 * 全局确认弹窗渲染端（fix-provider-delete-route-deadlock 2b）：
 * 管道逻辑在 useConfirm.test.ts，此处锁定渲染与交互——
 * 文案/按钮映射、danger variant、确认/取消/Escape 三条关闭路径。
 *
 * 注意：useState mock 现按 key 全局共享（对齐 Nuxt 语义），组件与测试代码
 * 看到同一 state；故每个用例必须 unmount（存活组件会继续响应全局 state，
 * 不 unmount 会在下一用例触发对已清空 DOM 的更新）。
 */

let wrapper: VueWrapper | null = null

function mountDialog(): VueWrapper {
  wrapper = mount(AppConfirmDialog, { attachTo: document.body })
  return wrapper
}

const { confirm: confirmFn } = useConfirm()

function dialogEl(): HTMLElement | null {
  return document.body.querySelector('.app-dialog')
}

function footerButtons(): HTMLButtonElement[] {
  return [...document.body.querySelectorAll('.app-dialog__footer button')] as HTMLButtonElement[]
}

afterEach(() => {
  // 先结算未决 promise 再卸载，避免用例间状态泄漏与对已清空 DOM 的更新
  const { state, settle } = useConfirm()
  if (state.value?.open) settle(state.value.id, false)
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

describe('AppConfirmDialog', () => {
  it('无未决确认时不渲染弹窗', () => {
    mountDialog()
    expect(dialogEl()).toBeNull()
  })

  it('confirm 后渲染 title/message 与默认按钮文案', async () => {
    const w = mountDialog()
    void confirmFn({ title: '删除确认', message: '确定删除吗？' })
    await w.vm.$nextTick()

    expect(dialogEl()).not.toBeNull()
    expect(document.body.textContent).toContain('删除确认')
    expect(document.body.textContent).toContain('确定删除吗？')
    expect(footerButtons().map(b => b.textContent?.trim())).toEqual(['取消', '确认'])
  })

  it('danger=true 时确认按钮为 danger 样式，文案取 confirmText', async () => {
    const w = mountDialog()
    void confirmFn({ title: 't', message: 'm', confirmText: '删除', danger: true })
    await w.vm.$nextTick()

    const confirmBtn = footerButtons().find(b => b.textContent?.includes('删除'))!
    expect(confirmBtn.className).toContain('app-button--danger')
  })

  it('danger 缺省时确认按钮为 primary 样式', async () => {
    const w = mountDialog()
    void confirmFn({ title: 't', message: 'm' })
    await w.vm.$nextTick()

    const confirmBtn = footerButtons()[1]!
    expect(confirmBtn.className).toContain('app-button--primary')
  })

  it('点确认 → resolve(true) 且弹窗关闭', async () => {
    const w = mountDialog()
    const pending = confirmFn({ title: 't', message: 'm' })
    await w.vm.$nextTick()

    footerButtons().find(b => b.textContent?.trim() === '确认')!.click()
    await expect(pending).resolves.toBe(true)
    await w.vm.$nextTick()
    expect(dialogEl()).toBeNull()
  })

  it('点取消 → resolve(false)', async () => {
    const w = mountDialog()
    const pending = confirmFn({ title: 't', message: 'm' })
    await w.vm.$nextTick()

    footerButtons().find(b => b.textContent?.trim() === '取消')!.click()
    await expect(pending).resolves.toBe(false)
  })

  it('Escape 关闭（AppDialog closeOnEscape 通道）→ resolve(false)', async () => {
    const w = mountDialog()
    const pending = confirmFn({ title: 't', message: 'm' })
    await w.vm.$nextTick()

    // AppDialog 在 document 上监听 keydown
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await expect(pending).resolves.toBe(false)
  })
})
