import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useMarginNoteSelection } from './useMarginNoteSelection'

/**
 * selection guard / mark 分层 / 气泡判定单元（specs 红线三 Scenario + WB-1）：
 * RG-2 划选不误展开（guard 吞 click）/ RG-1 纯点击正常通过 / RG-4 点 mark 跳卡不展开 /
 * WB-1 <2 字符 guard 不吞、气泡不弹。选区经 vi.spyOn(window, 'getSelection') 打桩，
 * 与真实浏览器行为（1.2 已在 Chromium 152 真机核定）解耦，锁的是监听器分层语义。
 */

function fakeSelection(options: { collapsed: boolean; text?: string }): Selection {
  return {
    isCollapsed: options.collapsed,
    toString: () => options.text ?? '',
    rangeCount: options.collapsed ? 0 : 1,
  } as unknown as Selection
}

function mountHarness() {
  document.body.innerHTML = `
    <div id="root">
      <button id="toggle" type="button">thread header</button>
      <mark class="mn-highlight" data-jump="77">高亮文本</mark>
    </div>`
  const root = document.getElementById('root') as HTMLElement
  const onMarkClick = vi.fn()
  const active = ref(true)
  const selection = useMarginNoteSelection({
    active,
    reportRoot: ref(root),
    onMarkClick,
  })
  // active watch 挂载后立即生效需要组件上下文；直接手动 attach 不行（composable 内部），
  // 这里用 watch 立即触发的既有行为：composable 的 watch(active) 非 immediate，
  // 因此测试里翻转一次 active 触发 attach。
  active.value = false
  active.value = true
  return { root, onMarkClick, selection, active }
}

describe('selection guard（划选冲突消解）', () => {
  it('RG-2：有效选区（≥2 字符）存续期间 click 被 capture 吞掉，toggle 不触发', () => {
    const { root } = mountHarness()
    vi.spyOn(window, 'getSelection').mockReturnValue(fakeSelection({ collapsed: false, text: '被划选的文本' }))
    const toggle = document.getElementById('toggle')!
    const toggleSpy = vi.fn()
    toggle.addEventListener('click', toggleSpy)
    const event = new MouseEvent('click', { bubbles: true, cancelable: true })
    toggle.dispatchEvent(event)
    expect(toggleSpy).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(true)
    root.innerHTML = ''
  })

  it('RG-1：无选区（collapsed）时纯点击正常穿透到 toggle', () => {
    mountHarness()
    vi.spyOn(window, 'getSelection').mockReturnValue(fakeSelection({ collapsed: true }))
    const toggle = document.getElementById('toggle')!
    const toggleSpy = vi.fn()
    toggle.addEventListener('click', toggleSpy)
    toggle.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
    expect(toggleSpy).toHaveBeenCalledTimes(1)
  })

  it('WB-1：选区 <2 字符 guard 不吞（空串/单字符穿透）', () => {
    mountHarness()
    vi.spyOn(window, 'getSelection').mockReturnValue(fakeSelection({ collapsed: false, text: '字' }))
    const toggle = document.getElementById('toggle')!
    const toggleSpy = vi.fn()
    toggle.addEventListener('click', toggleSpy)
    toggle.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
    expect(toggleSpy).toHaveBeenCalledTimes(1)
  })

  it('RG-4：点 mark → onMarkClick 收到 data-jump id，且事件被阻断不冒泡到行头', () => {
    const { onMarkClick } = mountHarness()
    vi.spyOn(window, 'getSelection').mockReturnValue(fakeSelection({ collapsed: true }))
    const mark = document.querySelector('mark.mn-highlight')!
    // mark 并非 toggle 子节点：再套一层行头结构验证 stopPropagation
    const header = document.createElement('button')
    header.addEventListener('click', () => {
      throw new Error('mark click 不应冒泡触发 toggle')
    })
    mark.parentElement?.replaceChild(header, mark)
    header.appendChild(mark)
    mark.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
    expect(onMarkClick).toHaveBeenCalledWith(77)
  })

  it('RG-5：有效选区存续时点击「问一问」气泡 → guard 豁免，气泡 click 正常触发', () => {
    const { root } = mountHarness()
    const bubble = document.createElement('button')
    bubble.className = 'mn-ask-bubble'
    bubble.type = 'button'
    root.appendChild(bubble)
    vi.spyOn(window, 'getSelection').mockReturnValue(fakeSelection({ collapsed: false, text: '逆回购操作' }))
    const confirm = vi.fn()
    bubble.addEventListener('click', confirm)
    const event = new MouseEvent('click', { bubbles: true, cancelable: true })
    bubble.dispatchEvent(event)
    // 气泡出现的前提即选区存续：guard 必须豁免气泡，否则落锚链路整条坏死（2026-09-24 用户报障）
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(event.defaultPrevented).toBe(false)
    root.innerHTML = ''
  })
})
