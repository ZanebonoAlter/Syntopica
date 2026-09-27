/**
 * chunk 持续失败兜底（spa-loading-ux，design.md D4 补捕获分支）。
 *
 * Nuxt 内建行为：页面 chunk 拉取失败 → `app:chunkError` → `nuxt:chunk-reload`
 * 插件整页自愈 reload（10s TTL 防循环）。弱网持续故障时自愈也无法恢复，
 * 但框架不会把错误交给 error.vue——用户停在原地/无限转圈。
 * 本插件在「同一窗口内第二次 chunk 失败」（即自愈 reload 后仍失败）时
 * 主动 showError 进全局兜底页，给出网络提示与手动重试入口；同时推后
 * `nuxt:reload` TTL 阻止 chunk-reload 插件再次整页刷新冲掉兜底页。
 */
import { createError, defineNuxtPlugin, showError } from '#imports'

/** 同一路径两次失败视为「自愈无效」的时间窗口 */
export const CHUNK_FALLBACK_WINDOW_MS = 30_000

/** 决策逻辑（纯函数，单测锚点）：窗口内第二次失败 → 走兜底页 */
export function shouldFallbackToErrorPage(prevFailTs: number | undefined, now: number): boolean {
  return prevFailTs !== undefined && now - prevFailTs < CHUNK_FALLBACK_WINDOW_MS
}

export default defineNuxtPlugin((nuxtApp) => {
  const failures = new Map<string, number>()

  nuxtApp.hook('app:chunkError', () => {
    const path = window.location.pathname
    const now = Date.now()
    const prev = failures.get(path)

    if (!shouldFallbackToErrorPage(prev, now)) {
      // 首次失败：记录时间戳，让 Nuxt 内建自愈 reload 先行
      failures.set(path, now)
      return
    }

    // 第二次失败（自愈 reload 未恢复）：接管为兜底页
    // 1) 推后 chunk-reload 的 TTL，阻止其再次整页刷新冲掉兜底页
    try {
      sessionStorage.setItem(
        'nuxt:reload',
        JSON.stringify({ path: path + window.location.search, expires: now + CHUNK_FALLBACK_WINDOW_MS }),
      )
    } catch { /* sessionStorage 不可用时跳过，兜底页仍生效（存在与 reload 的竞争窗口） */ }
    // 2) showError 渲染 error.vue（message 命中其 chunk 错误正则 → 网络提示文案）
    showError(createError({
      statusCode: 0,
      fatal: true,
      message: 'Failed to fetch dynamically imported module',
    }))
  })
})
