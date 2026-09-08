# 前端测试（Vitest + Playwright）

<!--
doc-impact-applies: .test.ts, .spec.ts, front/tests/ | section=单元测试（Vitest）
-->

> **权威源**：本文件是前端测试约定的唯一权威。运行门禁见《开发执行规范》§5.1。

## 用例设计（测什么）

> 测什么 / 测到什么程度 / 验收措辞的权威源是 [`standard/shared/test-design.md`](../shared/test-design.md)（故事锚点四问句，跨前后端）。本节只列前端分层判据速查：

| 故事性质 | 层 | 载体 |
| --- | --- | --- |
| 纯逻辑 | 函数单测 | `*.test.ts`（纯函数） |
| 组件行为（单页内逻辑） | 组件测试 | Vitest + happy-dom |
| **完整交互故事（多步导航/表单流/前后端联动）** | **opencli 端到端** | 驱动真实 Chrome 走主链路（涉及前端交互流程的 change 主链路至少一个 opencli 落点或「人工」豁免）；视觉验证派 k3 子代理截图 |

## 套件总览

| 套件 | 框架 | 位置 | 环境 |
|------|------|------|------|
| 单元测试 | Vitest + Vue Test Utils | `front/app/**/*.test.ts` | happy-dom |
| E2E 测试 | Playwright | `front/tests/e2e/*.spec.ts` | Chromium（真实浏览器） |

## 单元测试（Vitest）

- 配置文件：`front/vitest.config.ts`（使用 `happy-dom`，**不需要真实浏览器**）
- 测试文件与源文件**同目录**，命名 `*.test.ts`（如 `app/utils/foo.test.ts`）
- `front/tests/e2e/` 通过 `vitest.config.ts` 排除在 Vitest 之外
- 使用 `describe`/`it` 块，描述行为而非实现：

```typescript
import { describe, expect, it } from 'vitest'
import { myFunction } from './myFunction'

describe('myFunction', () => {
  it('does the expected thing', () => {
    expect(myFunction('input')).toBe('expected')
  })
})
```

运行：

```bash
cd front
pnpm test:unit                                            # 全部
pnpm test:unit -- app/utils/articleContentSource.test.ts  # 单文件
pnpm test:unit -- app/utils/articleContentSource.test.ts -t "prefers firecrawl"  # 按名称
```

### 跨平台运行（WSL 必须切 Windows cmd）

> 与 `typecheck` / `build` 同源问题，本节为权威定义；`AGENTS.md` 只留红线速查。

**WSL bash 下 `pnpm test:unit` 跑不起来**：vitest 经 Vite → rollup，依赖原生 binding（`@rollup/rollup-linux-x64-gnu`），但本仓库 `node_modules` 是 Windows 侧 `pnpm install` 的，只装了 win32 平台包，Linux 平台的 optional 包被裁掉。报错形如：`Cannot find module '@rollup/rollup-linux-x64-gnu'`。

**正确做法**：vitest 一律走 Windows cmd（lint 可继续在 WSL 跑）：

```bash
# test:unit — 必须用 cmd（同 typecheck / build）
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm test:unit 2>&1"
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm test:unit topicAnchor 2>&1"  # 按名筛选
```

> 不要为了“在 WSL 跑通”去 `pnpm add -D @rollup/rollup-linux-x64-gnu` —— 会污染 `package.json` / `pnpm-lock.yaml`，引入与具体 change 无关的脏改动。

### 常见陷阱

**happy-dom 静默丢弃 `color-mix()` inline 值**：happy-dom 的 `CSSStyleDeclaration` 把 `color-mix(in srgb, ...)` 视为无效值并**静默丢弃**，赋值后 `getAttribute('style')` 返回 `null`。因此**组件测试不要断言 style 含 `color-mix(...)` 字符串**，否则结构性跑不过。

- 可正常断言：纯 `var(--token)`、`transparent`、固定色值 —— 这些 happy-dom 会保留。
- 走 `color-mix()` / 计算 alpha 的视觉：断言**形态维度**（如 `--solid` / `--hollow` class、`data-*` 属性）间接验证分档，颜色留给真浏览器/视觉验收。
- 既有范式：`SectionTierBadge` 颜色值只用 `var(--color-*)` + `transparent`，正是为规避此限制；`SectionAnchorBadge` 的 tier1/2（color-mix）也照此只断 class + `data-anchor-tier`。
- 组件分档逻辑本身用**纯函数单测**覆盖（如 `utils/topicAnchor.ts` 的边界值 0.05 / 0.15），把可测的逻辑与不可测的 CSS 渲染分离。

## E2E 测试（Playwright）

> UI 验收分流：组件单测验证逻辑，major UI change 的双层验收（opencli 交互断言 + 1440×900/1920×1080 双视口视觉检查）不沉淀为 Playwright 回归——见 [`layout.md`](layout.md)「major UI change 双视口验收」与《开发执行规范》§5.3。

- 配置文件：`front/playwright.config.ts`
- spec 文件放 `front/tests/e2e/*.spec.ts`
- 在 `http://localhost:3000` 自动启动 Nuxt 开发服务器后跑浏览器测试
- **串行执行**（`fullyParallel: false`、`workers: 1`）保证启动稳定

```typescript
import { test, expect } from '@playwright/test'

test('page loads', async ({ page }) => {
  await page.goto('/some-page')
  await expect(page.locator('body')).toBeVisible()
})
```

运行：

```bash
cd front
pnpm test:e2e           # 全部（自动启动 dev server）
pnpm test:e2e:ui        # Playwright UI
pnpm test:e2e:list      # 只列出
pnpm test:e2e:args -- --grep "topic-graph"  # 传参
```

### E2E 范围与交互验证策略

`tests/e2e/` **只放极稳定、全局的 smoke**（页面骨架、响应式、阅读器渲染等），**不堆业务交互回归脚本**——SPA 多步导航脆弱，换板块或大改 UI 就大面积失效，维护成本 > 收益。

**业务交互验证走 opencli 按需验证**：某次变更怀疑交互问题时，派子代理（执行器 deepseek-v4-flash）跑一段现写现跑的断言，验证完即弃，不沉淀 spec；视图验证派 kimi-coding/k3。规则与派发模板见：
- `.agents/skills/ui-verify/`（铁律：Vue 异步等 nextTick、只用 localhost、断言写代码自己判、子代理只报 JSON）
- [`architecture/ui-navigation.md`](../../architecture/ui-navigation.md)（多步导航 + 选择器 + 断言锡点）


E2E/Vitest 断言的是"元素出现/接口返回了"（契约），不是"用户看到的效果达预期"。对于依赖后端数据质量、LLM 实际行为、真实渲染效果的功能，测试通过只是起点——完成后应核对实际产出的视觉效果/数据内容是否符合设计预期，发现偏差及时和用户沟通，不要等用户自己发现。详细案例与硬约束见 [`standard/backend/testing.md` §绿灯 ≠ 功能有效](../backend/testing.md)（该原则跨端通用）。

## 覆盖率


未配置最低覆盖率阈值；Vitest `vitest.config.ts` 无覆盖率设置。

## 资料来源

收敛自原 `testing.md`（§前端单元 / §前端 E2E / §编写新测试）与《开发执行规范》§5.2。
