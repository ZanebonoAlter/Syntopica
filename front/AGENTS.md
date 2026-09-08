# Frontend Agent Guide

遵循根 `AGENTS.md` 的所有规则。以下为前端特有差异。openspec change 执行走 `docs/reference/开发执行规范.md` §0.6 标准编排流程。

> **权威源声明（方案 B，防孤立）**：前端代码规范、双主题、lint/测试配置的**唯一权威源**在 `docs/reference/standard/frontend/`。本文件只保留 agent 秒级要看到的**红线速查**，每节末尾给 `→ standard/xxx.md` 深链。规范有出入时以 standard/ 为准。

## Commands

```bash
pnpm install  &&  pnpm dev  &&  pnpm build
pnpm lint  &&  pnpm exec nuxi typecheck
pnpm test:unit  &&  pnpm test:e2e
```

> **⚠️ WSL 注意**：`pnpm lint` 可在 WSL 跑；`pnpm exec nuxi typecheck` 和 `pnpm build` 因缺少 Linux native binding 必须在 Windows cmd 中执行：
> ```bash
> cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"
> cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"
> ```
> 提交前检查全流程见 `docs/reference/standard/shared/commit-pr.md`。

## 红线速查（Anti-Patterns — 硬禁）

- ❌ 组件内直接调 API / 调 WebSocket / EventSource（全局事件走 `useEventStream()`）
- ❌ `any` 类型 / Options API / `@ts-ignore`
- ❌ 不调 API 直接改 Store 状态（写操作必须持久化 + 乐观更新 + 回滚）
- ❌ Store 间循环依赖（articlesStore 不 import apiStore mutation）
- ❌ 跨 feature 深 import（用 `features/*/public.ts` facade 或 `api/normalizers/`）
- ❌ 重复通知（同一失败多层 toast，底层 API 不弹 toast）

→ 完整 Anti-Patterns 与代码示例：`docs/reference/standard/frontend/code-style.md` §10

## 关键约定速查

- 组件 `<script setup lang="ts">`；组件名 PascalCase；composable 名 `use` 前缀 camelCase
- 导入顺序：Vue/Nuxt → 第三方 → 内部 → type-only；`import type` 导入纯类型；`~` alias
- 数字 ID 在 API 边界转 `string`；`snake_case → camelCase` 只在 store/API 层，模板里不出现
- HTTP 走 `ApiClient`（`app/api/client.ts`）；返回 `{ success, data, error, message }`
- 双主题：三层 Token（Primitive → Semantic → Component），`data-theme="editorial|dark"`，`useTheme()`；组件只引用 Layer 2；editorial/magazine 风格不回退 SaaS
- **布局契约**：新页面/重构必须用 `<AppPageShell mode="reader|contained|workspace|split">`（760/1120/全宽/主从栏），不自写 max-width；弹窗用 `<AppDialog size="sm|md|lg|xl">`（420/560/760/1040，92vw 上限），新代码禁自由 width；major UI 双视口验收（1440×900/1920×1080）
- **UI 影响声明**（syntopica-ui schema）：新 change proposal 头声明 `<!-- ui-impact: none|minor|major -->`，`ui-design.md` 为必经制品；major 原型未经用户批准（ui-approval: approved）不得进实现（ui-design-gate 拦截）。详见根 AGENTS.md「UI 分档速查」与《开发执行规范》§3.1
- 统一组件：`AppDialog` / `AppButton` / `AppToggle` / `AppInput` / `AppSectionHeader` / `AppPageShell`
- **UI 图标本地化**：`<Icon icon="mdi:*">` 运行时零联网——启动时 `app/plugins/iconify-local.ts` 注册本地子集 `app/assets/iconify-subset.json`；**新增图标后必须 `pnpm generate:icons` 并提交产物**（一致性单测 `iconify-subset.test.ts` 会拦漏）
- 大组件超 500 行 / ~15KB 拆分

→ 组件/API/Store/事件流/通知/Feature 共享：`docs/reference/standard/frontend/code-style.md`
→ 双主题与设计系统：`docs/reference/standard/frontend/theming.md`
→ 布局契约（page shell/dialog 尺寸档/双视口验收）：`docs/reference/standard/frontend/layout.md`

## 目录归属速查

| 代码类型 | 归属 |
|---------|------|
| 业务组件/composable | `features/*/components/`、`features/*/composables/`（私有） |
| 跨 feature 共享 | `features/*/public.ts` facade 或上移 `components/` |
| 全局 composable | `composables/`（`useEventStream`、`useNotify`） |
| HTTP 调用 | `api/`（**唯一边界**，`useXxxApi()`） |
| 数据 normalizer | `api/normalizers/` |
| 状态管理 | `stores/`（仅跨组件共享） |
| 领域类型 / 纯工具 | `types/` / `utils/` |
| 页面 | `pages/`（只挂载） |

→ 完整归属规则 + 新增功能 Checklist：`docs/reference/standard/frontend/code-style.md` §2、§8

## 测试与 Lint

- 单元：Vitest + happy-dom，`*.test.ts` 与源同目录
- E2E smoke：`front/tests/e2e/*.spec.ts`（Playwright，仅页面骨架/响应式，串行执行）
- 交互验证：opencli 按需验证（断言现写现跑，不堆回归脚本）；视图验证派 kimi-coding/k3 → `.agents/skills/ui-verify/` + `docs/reference/architecture/ui-navigation.md`
- 交互约定：状态标记左对齐、状态说明不伪装动作 → `docs/reference/standard/frontend/interaction-conventions.md`
- ESLint flat config：`front/eslint.config.js`

→ `docs/reference/standard/frontend/testing.md`、`docs/reference/standard/frontend/lint.md`
