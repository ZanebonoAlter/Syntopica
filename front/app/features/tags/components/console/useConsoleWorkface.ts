/**
 * 单级宽工作面开关与焦点归还（矩阵阶段 G5，R33/R38 小接口）。
 *
 * 只管理 dialog 可见性/页签及触发点焦点；不拥有业务数据，关闭时 preventScroll
 * 归还触发点，避免把用户滚动位置拉走。
 */
import { nextTick, ref } from 'vue'
import type { WorkfaceTab } from './railContext'

export function useConsoleWorkface() {
  const open = ref(false)
  const tab = ref<WorkfaceTab>('context')
  let triggerFocus: HTMLElement | null = null

  function captureFocus(): HTMLElement | null {
    return typeof document !== 'undefined' ? document.activeElement as HTMLElement | null : null
  }

  function restoreFocus(target: HTMLElement | null): void {
    void nextTick(() => target?.focus?.({ preventScroll: true }))
  }

  function openWorkface(nextTab: WorkfaceTab): void {
    if (!open.value) triggerFocus = captureFocus()
    tab.value = nextTab
    open.value = true
  }

  function closeWorkface(): void {
    open.value = false
    restoreFocus(triggerFocus)
    triggerFocus = null
  }

  function setVisible(visible: boolean): void {
    if (visible) open.value = true
    else closeWorkface()
  }

  return { open, tab, openWorkface, closeWorkface, setVisible }
}
