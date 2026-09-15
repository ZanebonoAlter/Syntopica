/**
 * Vitest setup – mocks Nuxt/Vue auto-imports not available in test environment.
 */
import { ref, computed, onUnmounted, onMounted, watch } from 'vue'

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
