/**
 * Vitest setup – mocks Nuxt/Vue auto-imports not available in test environment,
 * and restores browser globals that Node's experimental APIs shadow under happy-dom.
 */
import { ref, computed, onUnmounted, onMounted, watch, type Ref } from 'vue'
import { Storage } from 'happy-dom'
import { beforeEach } from 'vitest'

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

// Nuxt's useState returns a Ref<T> shared per key across the app（按 key 全局单例）。
// mock 用 per-key registry 对齐该语义：同一测试文件内，一处经 useState(key) 写入的
// 状态在别处（含被测组件内部）可见。此前每次调用都返回新 ref，会让「测试代码与被测
// 组件各拿各的 state」，跨实例共享的 composable（如 useConfirm）无法测。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const useStateRegistry = new Map<string, { r: Ref<any>; init?: () => any }>()
// eslint-disable-next-line @typescript-eslint/no-explicit-any
globalThis.useState = function useState<T = any>(key: string, init?: () => T) {
  let entry = useStateRegistry.get(key)
  if (!entry) {
    entry = { r: ref(init ? init() : undefined), init }
    useStateRegistry.set(key, entry)
  }
  return entry.r as { value: T }
}
// 用例间隔离：**重置值、保留引用**（不能 clear registry——模块顶层解构的 composable
// 如 AppConfirmDialog.test.ts 的 confirmFn 跨用例持有 ref，清空会让组件内新建 ref 与
// 之分支，跨实例共享断裂）。遍历把每个 ref 回到 init 语义初始态：前一用例的残留值
// 不再泄漏（2026-09-23 fe-composables 2 条「静默降级期望清零」假失败即此因，事实链
// 见 docs/research/test-env-pitfalls/），同时 per-key 单例语义完整保留。
beforeEach(() => {
  useStateRegistry.forEach((entry) => {
    entry.r.value = entry.init ? entry.init() : undefined
  })
})

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
