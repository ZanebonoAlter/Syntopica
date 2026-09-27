/**
 * 全局确认弹窗管道（fix-provider-delete-route-deadlock 2b）
 *
 * 替代原生 window.confirm() 的统一确认交互：
 * - `confirm(options): Promise<boolean>`，promise 式调用，语义与原生 confirm 对齐
 * - 弹窗渲染由 `AppConfirmDialog.vue` 承担（挂载 app.vue，与 NotifyContainer/useNotify 同模式）
 * - state 用 useState（SSR 兼容惯例）；resolve 回调存模块级 Map 以 id 关联，
 *   避免把函数塞进 useState state
 *
 * 注意：confirm 是排队语义上的一次一个——重复调用时后发者覆盖前发者，
 * 未决的前一个 promise 以 false（取消）结算，不会悬挂。
 */

export interface ConfirmOptions {
  title: string
  message: string
  /** 确认按钮文案，默认「确认」 */
  confirmText?: string
  /** 取消按钮文案，默认「取消」 */
  cancelText?: string
  /** true 时确认按钮为 danger 样式（删除类动作） */
  danger?: boolean
}

interface ConfirmState {
  id: number
  open: boolean
  title: string
  message: string
  confirmText: string
  cancelText: string
  danger: boolean
}

type ResolveFn = (confirmed: boolean) => void

const resolvers = new Map<number, ResolveFn>()
let idCounter = 0

export function useConfirm() {
  const state = useState<ConfirmState | null>('confirm:state', () => null)

  /** 弹出确认弹窗；resolve(true)=确认，resolve(false)=取消/Escape/被新弹窗顶替 */
  function confirm(options: ConfirmOptions): Promise<boolean> {
    // 若已有未决弹窗，先以取消结算，保持「同时最多一个」
    if (state.value?.open) {
      settle(state.value.id, false)
    }
    const id = ++idCounter
    state.value = {
      id,
      open: true,
      title: options.title,
      message: options.message,
      confirmText: options.confirmText ?? '确认',
      cancelText: options.cancelText ?? '取消',
      danger: options.danger ?? false,
    }
    return new Promise<boolean>((resolve) => {
      resolvers.set(id, resolve)
    })
  }

  /** 供 AppConfirmDialog 回传用户选择；幂等（重复 settle 忽略） */
  function settle(id: number, confirmed: boolean) {
    const resolve = resolvers.get(id)
    if (!resolve) return
    resolvers.delete(id)
    resolve(confirmed)
    if (state.value?.id === id) {
      state.value = { ...state.value, open: false }
    }
  }

  return { state, confirm, settle }
}
