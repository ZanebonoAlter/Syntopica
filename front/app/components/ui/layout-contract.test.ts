import { describe, expect, it } from 'vitest'
import { Z_INDEX, nestedOverlayZ } from './layout-contract'

/**
 * 浮层层级契约单元锚点（standard/frontend/layout.md「浮层层级契约」）：
 * 档位表是唯一事实源，数值边界直接在这里断言——
 * 组件若改回手写层级数字或改坏档位顺序，本文件先红。
 */
describe('浮层层级档位表（Z_INDEX）', () => {
  it('档位顺序单调：dialog < dialogNested < readingDialog < readingFullscreen < toast', () => {
    expect(Z_INDEX.dialog).toBeLessThan(Z_INDEX.dialogNested)
    expect(Z_INDEX.dialogNested).toBeLessThan(Z_INDEX.readingDialog)
    expect(Z_INDEX.readingDialog).toBeLessThan(Z_INDEX.readingFullscreen)
    expect(Z_INDEX.readingFullscreen).toBeLessThan(Z_INDEX.toast)
  })

  it('档位取值与契约表一致（数值锚，防手写漂移）', () => {
    expect(Z_INDEX.dialog).toBe(1000)
    expect(Z_INDEX.dialogNested).toBe(1100)
    expect(Z_INDEX.readingTooltip).toBe(9099)
    expect(Z_INDEX.readingDialog).toBe(9100)
    expect(Z_INDEX.readingFullscreen).toBe(9200)
    expect(Z_INDEX.toast).toBe(9999)
  })

  it('阅读台行内浮层低于阅读弹窗：点开的阅读预览必须盖住浮层（review R-12）', () => {
    expect(Z_INDEX.readingTooltip).toBeLessThan(Z_INDEX.readingDialog)
    expect(Z_INDEX.readingTooltip).toBeGreaterThan(Z_INDEX.dialogNested)
  })
})

describe('嵌套浮层相对抬升（nestedOverlayZ）', () => {
  it('严格大于宿主层级', () => {
    expect(nestedOverlayZ(Z_INDEX.dialog)).toBeGreaterThan(Z_INDEX.dialog)
  })

  it('基础档抬升结果 = 嵌套浮层档（1000 → 1100）', () => {
    expect(nestedOverlayZ(Z_INDEX.dialog)).toBe(Z_INDEX.dialogNested)
  })

  it('宿主在更高档（阅读层 9100）时抬升后仍严格高于宿主', () => {
    expect(nestedOverlayZ(Z_INDEX.readingDialog)).toBeGreaterThan(Z_INDEX.readingDialog)
  })

  /**
   * 撞档锚（review R-1）：裸抬升 hostZ+100 会正好落在「弹窗内全屏层」9200——
   * 与之同层只能靠 DOM 顺序决胜，正是档位表要消灭的 tie-break。
   */
  it('宿主在阅读层时：抬升结果不撞「弹窗内全屏层」档（9200），落到档间空隙', () => {
    const raised = nestedOverlayZ(Z_INDEX.readingDialog)
    expect(raised).toBeGreaterThan(Z_INDEX.readingDialog)
    expect(raised).not.toBe(Z_INDEX.readingFullscreen)
    expect(raised).toBeLessThan(Z_INDEX.toast)
  })

  it('抬升结果不落在任何已占档位上（嵌套浮层自己的 dialogNested 档除外）', () => {
    const occupied = Object.values(Z_INDEX) as number[]
    const hosts = [Z_INDEX.dialog, Z_INDEX.dialogNested, Z_INDEX.readingDialog, Z_INDEX.readingFullscreen]
    for (const host of hosts) {
      const raised = nestedOverlayZ(host)
      expect(raised).toBeGreaterThan(host)
      if (raised !== Z_INDEX.dialogNested) expect(occupied).not.toContain(raised)
    }
  })

  it('上限护栏：不得越过全局提示档 toast（9999）', () => {
    expect(nestedOverlayZ(Z_INDEX.toast)).toBeLessThanOrEqual(Z_INDEX.toast)
    expect(nestedOverlayZ(Z_INDEX.toast + 5000)).toBeLessThanOrEqual(Z_INDEX.toast)
  })
})
