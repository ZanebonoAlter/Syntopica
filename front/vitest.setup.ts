/**
 * Vitest setup – mocks Nuxt/Vue auto-imports not available in test environment,
 * and restores browser globals that Node's experimental APIs shadow under happy-dom.
 */
import { ref, computed, onUnmounted, onMounted, watch } from 'vue'
import { Storage } from 'happy-dom'

// Node ≥ 22 自带实验性 Web Storage 全局：未传 `--localstorage-file` 时
// `globalThis.localStorage` 是 undefined，但**键已经存在**。Vitest 的 happy-dom 环境
// （populateGlobal）只在不冲突时才把 window 上的属性复制到 globalThis，于是浏览器语义的
// `localStorage` 被 Node 的 undefined 遮蔽（`sessionStorage` 不受影响，Node 有内存实现）。
// happy-dom 的 BrowserWindow 内部就是 `new Storage()`，这里按同样方式补一个等价实现；
// 将来 Node/Vitest 不再遮蔽（键不存在或已有可用实现）时，下面的 typeof 检查会自动跳过。
for (const key of ['localStorage', 'sessionStorage'] as const) {
  const storageGlobal = globalThis as Record<string, unknown>
  if (typeof storageGlobal[key] === 'undefined') {
    Object.defineProperty(globalThis, key, {
      value: new Storage(),
      writable: true,
      configurable: true,
    })
  }
}

// Nuxt's useState returns a Ref<T>; we mock it with Vue's ref
// eslint-disable-next-line @typescript-eslint/no-explicit-any
globalThis.useState = function useState<T = any>(key: string, init?: () => T) {
  return init ? ref(init()) : ref()
}

// Nuxt's useRuntimeConfig returns runtime config
// apiBase 默认与 nuxt.config.ts 一致（后端 5100 绝对直连）；需要相对 base 的用例在各自文件里覆盖。
globalThis.useRuntimeConfig = function useRuntimeConfig() {
  return {
    public: {
      apiBase: 'http://localhost:5100/api',
    },
    app: { baseURL: '/' },
  }
}

// Vue composable APIs that may not be auto-imported in test env
globalThis.ref = ref
globalThis.computed = computed
globalThis.onUnmounted = onUnmounted
globalThis.onMounted = onMounted
globalThis.watch = watch
