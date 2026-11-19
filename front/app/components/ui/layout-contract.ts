/**
 * 布局契约常量与纯函数。
 *
 * 权威源：docs/reference/standard/frontend/layout.md（change: make-ui-design-first-class D4）。
 * 组件（AppPageShell / AppDialog）只做 CSS var 应用；边界值数值断言直接测这里。
 *
 * 浮层层级：权威源同为 docs/reference/standard/frontend/layout.md「浮层层级契约」节。
 * 组件**不得**手写层级数字——一律取 Z_INDEX 档位表或 nestedOverlayZ()。
 * （存量组件里的历史手写数字列入该文档「存量迁移」登记表：不迁移，主动重构时收敛。）
 */

/**
 * 浮层层级档位表（唯一事实源）。
 *
 * 档位递增即绘制顺序：基础弹窗 < 弹窗内嵌套浮层 < 阅读层弹窗 < 弹窗内全屏层 < 全局提示。
 */
export const Z_INDEX = {
  dialog: 1000,            // 基础弹窗遮罩（AppDialog 默认）
  dialogNested: 1100,      // 弹窗内再开的嵌套浮层
  readingDialog: 9100,     // 阅读层弹窗
  readingFullscreen: 9200, // 弹窗内全屏层
  // 阅读台内的行内预览浮层（如线索 chip 的相关度浮层）：高于阅读台自身遮罩层
  // （DailyReportReader 的 .drm-overlay 9000），低于阅读弹窗 9100——点开的阅读预览
  // 弹窗必须盖住浮层，浮层不得穿到弹窗之上。
  readingTooltip: 9099,
  toast: 9999,             // 全局提示 / 菜单
} as const

/** 嵌套浮层相对宿主的抬升步长（与档位表的百位档间距对齐） */
const OVERLAY_STEP = 100

/** 已被占用的档位值清单——档位表 Z_INDEX 是唯一占用清单 */
const OCCUPIED_TIERS: readonly number[] = Object.values(Z_INDEX)

/**
 * 嵌套浮层相对宿主的层级：严格大于宿主，且不落在任何已占档位上。
 *
 * 口径（对应 standard/frontend/layout.md「浮层层级契约」规则 2）：
 *  - 档位表是唯一占用清单：抬升结果若落在已占档位（例如宿主在阅读层 9100 时裸抬升
 *    会撞上「弹窗内全屏层」9200），SHALL 跳过该档继续抬升——同层只能靠 DOM 顺序决胜，
 *    那正是档位表要消灭的 tie-break。
 *  - 例外：`dialogNested` 是嵌套浮层自己的落位档，不算「被他人占用」——
 *    宿主在基础档 dialog 时结果恰为 Z_INDEX.dialogNested。
 *  - 上限护栏：全局提示层 toast 是顶档，嵌套浮层不得越过它（越过会盖住全局提示）；
 *    宿主本身已 ≥ toast 属契约边界（不该在提示层宿主里再开浮层），此时封顶返回 toast 档。
 */
export function nestedOverlayZ(hostZ: number): number {
  let candidate = hostZ + OVERLAY_STEP
  while (
    candidate < Z_INDEX.toast
    && candidate !== Z_INDEX.dialogNested
    && OCCUPIED_TIERS.includes(candidate)
  ) {
    candidate += OVERLAY_STEP
  }
  return Math.min(candidate, Z_INDEX.toast)
}

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
