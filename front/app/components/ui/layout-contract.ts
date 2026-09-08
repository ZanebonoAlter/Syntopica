/**
 * 布局契约常量与纯函数。
 *
 * 权威源：docs/reference/standard/frontend/layout.md（change: make-ui-design-first-class D4）。
 * 组件（AppPageShell / AppDialog）只做 CSS var 应用；边界值数值断言直接测这里。
 */

/** 页面 shell 四模式 */
export type ShellMode = 'reader' | 'contained' | 'workspace' | 'split'

/** 弹窗尺寸档 */
export type DialogSize = 'sm' | 'md' | 'lg' | 'xl'

/** 各模式内容列最大宽度（px）；null = 无上限（workspace/split 用满可用宽度） */
export const SHELL_MAX_WIDTH: Record<ShellMode, number | null> = {
  reader: 760,
  contained: 1120,
  workspace: null,
  split: null,
}

/** 弹窗四档目标宽度（px） */
export const DIALOG_SIZES: Record<DialogSize, number> = {
  sm: 420,
  md: 560,
  lg: 760,
  xl: 1040,
}

/** 弹窗可视区宽度约束比例（92vw） */
export const DIALOG_VIEWPORT_RATIO = 0.92

/**
 * 给定视口下 shell 内容列的有效宽度（px）：
 * min(视口宽 − 2×gutter, 模式上限)；无上限模式 = 视口宽 − 2×gutter（小屏即 100%−gutter）。
 */
export function effectiveContentWidth(
  viewportWidth: number,
  mode: ShellMode,
  gutter = 24,
): number {
  const avail = Math.max(0, viewportWidth - 2 * gutter)
  const max = SHELL_MAX_WIDTH[mode]
  return max === null ? avail : Math.min(avail, max)
}

/** 给定视口下弹窗有效宽度（px）：min(档位宽度, floor(92vw))——任何档位都不超过 92vw */
export function effectiveDialogWidth(viewportWidth: number, size: DialogSize): number {
  return Math.min(DIALOG_SIZES[size], Math.floor(DIALOG_VIEWPORT_RATIO * viewportWidth))
}
