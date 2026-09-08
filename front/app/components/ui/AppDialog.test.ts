import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AppDialog from './AppDialog.vue'
import { effectiveDialogWidth } from './layout-contract'

/**
 * 布局契约单元锚点（standard/frontend/layout.md「弹窗拒绝随手宽度」）：
 * 结构与行为（Teleport 渲染/Escape/overlay/close/size 档）在此锁定；
 * 92vw 视觉行为由 effectiveDialogWidth 数值边界断言，截图构图留给验收分流。
 */

function mountDialog(props: Record<string, unknown> = {}) {
  return mount(AppDialog, {
    props: { modelValue: true, ...props },
    attachTo: document.body,
  })
}

function dialogEl(): HTMLElement {
  return document.body.querySelector('.app-dialog') as HTMLElement
}

function overlayEl(): HTMLElement {
  return document.body.querySelector('.app-dialog-overlay') as HTMLElement
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('AppDialog 尺寸档', () => {
  it.each([
    ['sm', '420px'],
    ['md', '560px'],
    ['lg', '760px'],
    ['xl', '1040px'],
  ] as const)('size=%s → is-sized + --dialog-w=%s（宽度 min(var, 92vw) 由样式表统一约束）', (size, px) => {
    mountDialog({ size })
    const el = dialogEl()
    expect(el.classList.contains('is-sized')).toBe(true)
    expect(el.style.getPropertyValue('--dialog-w')).toBe(px)
    // 不走旧自由宽度通道
    expect(el.style.maxWidth).toBe('')
  })

  it('旧 width prop 渲染兼容：仍按 maxWidth 生效且不进尺寸档样式', () => {
    mountDialog({ width: '672px' })
    const el = dialogEl()
    expect(el.style.maxWidth).toBe('672px')
    expect(el.classList.contains('is-sized')).toBe(false)
  })

  it('size 优先于旧 width（同时传时走尺寸档）', () => {
    mountDialog({ size: 'md', width: '672px' })
    const el = dialogEl()
    expect(el.classList.contains('is-sized')).toBe(true)
    expect(el.style.getPropertyValue('--dialog-w')).toBe('560px')
    expect(el.style.maxWidth).toBe('')
  })
})

describe('AppDialog 关闭行为', () => {
  it('Escape 触发 update:modelValue(false)（初始即打开也能生效）', async () => {
    const wrapper = mountDialog()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await Promise.resolve()
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([false])
  })

  it('closeOnEscape=false 时 Escape 不关闭', () => {
    const wrapper = mountDialog({ closeOnEscape: false })
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('update:modelValue')).toBeFalsy()
  })

  it('点击 overlay 关闭；closeOnOverlay=false 时不关闭', async () => {
    const a = mountDialog()
    await overlayEl().click()
    expect(a.emitted('update:modelValue')?.[0]).toEqual([false])

    document.body.innerHTML = ''
    const b = mountDialog({ closeOnOverlay: false })
    await overlayEl().click()
    expect(b.emitted('update:modelValue')).toBeFalsy()
  })

  it('close 按钮触发关闭；showClose=false 不渲染按钮', async () => {
    const a = mountDialog({ title: 'T' })
    const btn = document.body.querySelector('.app-dialog__close') as HTMLButtonElement
    expect(btn).toBeTruthy()
    await btn.click()
    expect(a.emitted('update:modelValue')?.[0]).toEqual([false])

    document.body.innerHTML = ''
    mountDialog({ showClose: false })
    expect(document.body.querySelector('.app-dialog__close')).toBeNull()
  })
})

describe('弹窗 92vw 边界值（effectiveDialogWidth）', () => {
  it('窄于档位：受 92vw 限制（无横向溢出）', () => {
    expect(effectiveDialogWidth(400, 'md')).toBe(Math.floor(0.92 * 400)) // 368 < 560
    expect(effectiveDialogWidth(456, 'sm')).toBe(Math.floor(0.92 * 456)) // 419 < 420
  })

  it('恰达档位边界：92vw 首次覆盖档位值', () => {
    // sm=420：92vw ≥ 420 的最小整数视口是 ceil(420/0.92)=457
    expect(effectiveDialogWidth(457, 'sm')).toBe(420)
  })

  it('宽于档位：取档位宽度（1440/1920 下四档均不超上限）', () => {
    for (const size of ['sm', 'md', 'lg', 'xl'] as const) {
      expect(effectiveDialogWidth(1440, size)).toBeLessThanOrEqual(Math.floor(0.92 * 1440))
      expect(effectiveDialogWidth(1920, size)).toBeLessThanOrEqual(Math.floor(0.92 * 1920))
    }
    expect(effectiveDialogWidth(1920, 'xl')).toBe(1040)
  })
})
