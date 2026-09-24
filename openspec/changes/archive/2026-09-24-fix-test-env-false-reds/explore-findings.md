
## vitest NODE_ENV 钉死与 useState registry 隔离的实现机制

两处 vitest 测试底座机制事实（2026-09-24 实测，fix-test-env-false-reds change）：

1. **vitest 3.2.4 的 `test.env` 不覆盖已存在的 NODE_ENV**：应用点是 `for (const name in envs) process.env[name] ??= envs[name]`（front/node_modules/vitest/dist/chunks/cli-api.*.js:10149），进程兜底也是 `process.env.NODE_ENV ??= "test"`（:10537）。宿主（pi 会话）自带 NODE_ENV=production 时 test.env 完全无效——Vue 生产构建裁剪事件追踪，`wrapper.emitted()` 断言假红。正确钉法：`front/vitest.config.ts` 顶层（defineConfig 之前）`process.env.NODE_ENV = 'test'`——主进程加载 config 时执行，早于 vite define 解析与 worker 启动；该文件仅 vitest 加载，不影响 pnpm build/dev。

2. **useState mock registry 的用例间隔离必须「重置值、保留引用」而非 clear**：`front/vitest.setup.ts` 的 registry 存 `{ r: Ref, init? }`，beforeEach 遍历 `entry.r.value = entry.init?.() ?? undefined`。不能 `registry.clear()`——模块顶层解构的 composable 引用（如 AppConfirmDialog.test.ts 的 `const { confirm } = useConfirm()`）跨用例持有 ref，clear 后组件内新建 ref 与之分支，弹窗类断言批量假红（7 条挂 6 条）。对齐 Nuxt 语义：值是请求级隔离、引用是 key 级稳定。

巡检脚本事实：`scripts/harness/test-patrol.sh` 后端 6 轮转分片已带 `-timeout 120s`（be-all 不入轮转、全量跑故不加）；smoke 契约 `test-patrol.smoke.sh` TC-C01 硬编码断言分片参数，改分片定义须同步。台账 DB 是 `.pi/harness/events.db`（非 test-patrol.db）。

**引用**：front/vitest.config.ts、front/vitest.setup.ts、scripts/harness/test-patrol.sh

<!-- pinned 2026-09-24T01:51:16Z -->
