import { onMounted, onUnmounted, ref } from 'vue'
import type { Ref } from 'vue'

/**
 * useMediaQuery — 响应式媒体查询（mobile-viewport-stage1 任务 1.1）。
 *
 * client 端：mounted 后经 `window.matchMedia` 读取真实匹配值并监听 change（组件卸载时移除监听）；
 * server 端：不触 window，返回默认 false 的 ref——真实值在 client 挂载后才写入，
 * SSR 渲染输出恒为 false，不会产生水合错配。当前应用 `ssr: false`（SPA），
 * 守卫为防御性预留，与 useOnboarding 的客户端守卫同款。
 *
 * 详见 openspec/changes/mobile-viewport-stage1/design.md（D1 断点 / D2 CSS+JS 双层方案）。
 */

/**
 * Client-environment guard. Defaults to the Nuxt `import.meta.client` macro so
 * production stays SSR-safe; exposed as a mutable token purely so unit tests can
 * toggle client/SSR behavior — the macro itself is not runtime-overridable under
 * Vitest (it resolves to `undefined` there). See useMediaQuery.test.ts.
 */
export const __mediaQueryClient = { value: import.meta.client }

/** 防御性客户端守卫：非客户端环境一律短路。 */
function isClient(): boolean {
  return __mediaQueryClient.value
}

/** 窄视口断点查询串：视口 <768px 为窄屏（与 Tailwind md 对齐，design.md D1）。 */
export const NARROW_VIEWPORT_QUERY = '(max-width: 767.98px)'

/**
 * 订阅一条媒体查询的匹配状态。必须在组件 setup 内调用（注册 onMounted/onUnmounted）。
 *
 * 返回的 ref：server / 挂载前为 false；client 挂载后为真实匹配值，并随 change 事件更新。
 */
export function useMediaQuery(query: string): Ref<boolean> {
  const matches = ref(false)

  if (!isClient()) return matches

  let mql: MediaQueryList | null = null
  const handleChange = (event: MediaQueryListEvent): void => {
    matches.value = event.matches
  }

  onMounted(() => {
    mql = window.matchMedia(query)
    matches.value = mql.matches
    mql.addEventListener('change', handleChange)
  })

  onUnmounted(() => {
    mql?.removeEventListener('change', handleChange)
    mql = null
  })

  return matches
}

/** 是否窄视口（<768px）。主工作台窄屏降级分支（viewMode 切换 / 侧栏抽屉）统一读取本值。 */
export function useIsNarrowViewport(): Ref<boolean> {
  return useMediaQuery(NARROW_VIEWPORT_QUERY)
}
