import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '~': fileURLToPath(new URL('./app', import.meta.url)),
      // Nuxt 的 `#imports` 是构建期虚拟模块，vitest 不走 Nuxt 管线（否则 import 直接
      // 解析失败）。指向测试专用 stub，理由与维护点见 test/stubs/nuxt-imports.ts 头注释。
      '#imports': fileURLToPath(new URL('./test/stubs/nuxt-imports.ts', import.meta.url)),
    },
  },
  test: {
    // Exclude Playwright e2e tests from Vitest
    exclude: ['**/node_modules/**', '**/tests/e2e/**'],
    include: ['app/**/*.test.ts', 'app/**/*.test.tsx'],
    environment: 'happy-dom',
    globals: true,
    setupFiles: ['./vitest.setup.ts'],
  },
})
