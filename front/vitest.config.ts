import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// 钉死 NODE_ENV=test：pi 会话等宿主自带 NODE_ENV=production 时，vitest 内部两处
// 都不会覆盖它——进程兑底是 `process.env.NODE_ENV ??= "test"`（仅填缺失变量），
// test.env 应用点同样是 `??=`，对已存在的 NODE_ENV 无效（实测复现 Vue 生产构建
// 裁剪事件追踪导致 emitted() 假红）。这里在 config 顶层（主进程、早于 vite define
// 解析与 worker 启动）强制赋值；本文件仅被 vitest 加载，不影响 pnpm build（仍
// production）/ pnpm dev（仍 development）。事实链见
// docs/research/test-env-pitfalls/explore-findings.md（2026-09-23 假红 116 条）。
process.env.NODE_ENV = 'test'

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
