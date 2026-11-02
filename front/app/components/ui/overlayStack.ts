/**
 * 轻量浮层栈计数（无渲染）。
 *
 * 背景：阅读台和右侧抽屉都在 window 上监听 Escape；阅读台先注册，抽屉后注册，
 * 同一击键会先被阅读台处理并退出路由，抽屉来不及关（B-2）。AppDrawer 打开时
 * 计数 +1，阅读台在 Esc 分支先让位给任何已打开的抽屉。
 */
let openCount = 0

export function pushOverlay(): void {
  openCount += 1
}

export function popOverlay(): void {
  openCount = Math.max(0, openCount - 1)
}

export function hasOpenOverlay(): boolean {
  return openCount > 0
}

/** 测试用：重置计数，避免跨用例污染。 */
export function resetOverlayStack(): void {
  openCount = 0
}
