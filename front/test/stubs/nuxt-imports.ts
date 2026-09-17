/**
 * Nuxt `#imports` 的 Vitest 替身（stub）。
 *
 * 为什么需要：
 * `#imports` 在 Nuxt 里是**构建期虚拟模块**（由 nuxt 的 imports 插件生成，聚合 Vue /
 * Nuxt 内置 API 与项目自动导入），磁盘上没有对应文件；而 `vitest.config.ts` 不经过
 * Nuxt 构建管线，于是任何 `import { ... } from '#imports'` 的模块在单测里都会
 * `Failed to resolve import "#imports"`。`vitest.config.ts` 的 alias 把该说明符指到
 * 本文件，让这类模块（如 `app/plugins/chunk-error-fallback.ts`）可以被直接 import。
 *
 * 维护点：
 * - 这里只导出「被测试模块真正用到」的最小 API 集合。新增 API 时优先在此补最小实现；
 *   单个测试需要特殊行为时用 `vi.mock('#imports', ...)` 覆盖（alias 之后 vi.mock
 *   解析到的是同一个模块，仍然生效）。
 * - 与 Nuxt 真实实现的对应关系：`defineNuxtPlugin` 原样返回插件函数；`createError`
 *   返回带 statusCode / fatal 等字段的 Error（H3Error）;`showError` 在客户端跳转错误页。
 *   Nuxt 大版本升级或本文件新增导出时，回看这三条约定是否仍成立。
 * - 本文件只经 vitest alias 触达，业务代码不要直接 import。
 */

/** `#imports` 的 createError 入参（H3Error 的最小可用子集）。 */
interface NuxtErrorInput {
  statusCode?: number
  statusMessage?: string
  message?: string
  fatal?: boolean
}

/** 最小 createError：返回 Error 并平铺 Nuxt 错误字段（生产实现是 H3Error）。 */
export function createError(input: NuxtErrorInput | string): Error {
  const error = new Error(typeof input === 'string' ? input : input.message ?? '')
  if (typeof input !== 'string') Object.assign(error, input)
  return error
}

/** Nuxt 的 defineNuxtPlugin 只是原样返回插件函数（类型标记 + dev 期命名）。 */
export function defineNuxtPlugin<T>(plugin: T): T {
  return plugin
}

/** 真实 showError 会切到 Nuxt 错误页；单测里不需要副作用（要断言就 vi.mock）。 */
export function showError(_error: unknown): void {
  // no-op stub
}
