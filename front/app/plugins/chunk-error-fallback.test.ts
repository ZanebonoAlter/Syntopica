import { describe, expect, it } from 'vitest'
import { CHUNK_FALLBACK_WINDOW_MS, shouldFallbackToErrorPage } from './chunk-error-fallback'

/**
 * chunk 持续失败兜底决策锚点（spa-loading-ux design.md D4）：
 * - 首次失败 → 让 Nuxt 内建自愈 reload 先行（false）
 * - 窗口内第二次失败 → 兜底页接管（true）
 * - 窗口外再失败 → 视为新一轮首次失败（false，重新自愈）
 */

describe('shouldFallbackToErrorPage', () => {
  it('首次失败（无历史）不自愈兜底，交给 Nuxt 内建 reload', () => {
    expect(shouldFallbackToErrorPage(undefined, 1_000)).toBe(false)
  })

  it('窗口内第二次失败 → 走兜底页', () => {
    const first = 1_000
    expect(shouldFallbackToErrorPage(first, first + CHUNK_FALLBACK_WINDOW_MS - 1)).toBe(true)
  })

  it('超出窗口的失败视为新一轮首次，重新自愈', () => {
    const first = 1_000
    expect(shouldFallbackToErrorPage(first, first + CHUNK_FALLBACK_WINDOW_MS)).toBe(false)
  })
})
