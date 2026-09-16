<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

<!--
  minor 依据（proposal 同源）：新增加载/错误兜底视觉全部复用既有契约——
  主题令牌（editorial/dark）与 app.vue 已有全屏居中 loading/error 态模式；
  不新增页面布局结构、导航、dialog 或信息架构成员（error.vue 为 Nuxt
  fallback 兜底页，复用 app.vue 错误态视觉，非产品 IA 页面）。
-->

## 入口与入口变更

| 入口 | 变更 | 时机 |
| --- | --- | --- |
| 首屏入口（任何路由直接打开/刷新） | 新增 `spa-loading-template.html` 全屏加载动画，替代 Nuxt 默认白底模板 | HTML 解析即渲染 → JS bundle 挂载完成让位给 `app.vue` |
| `app.vue` 根组件 | 顶部新增 `NuxtLoadingIndicator` 路由进度条 | 路由切换（含懒加载 chunk 拉取）期间显示 |
| `app/error.vue`（新增） | Nuxt 全局错误兜底页 | chunk 加载失败 / 未捕获运行时错误触发 `showError` |

## 受影响状态

- **loading（受影响，两处）**：
  - 首屏 JS 下载期：`spa-loading-template.html` 转圈动画 + 「正在加载」文案；随 `data-theme`（localStorage）适配 editorial/dark，暗色不闪白
  - 路由切换期：顶部细进度条（NuxtLoadingIndicator 默认样式对接主题色）
- **error（受影响，一处）**：`error.vue` 全屏错误态——图标 + 错误文案 + 「重新加载」按钮（复用 app.vue 错误态结构：`h-screen` 居中单列、`mdi:alert-circle` 图标、`AppButton variant="primary"`）
- empty / success：不受影响

## 复用组件与布局模式

- 布局模式：全屏居中 fallback（`h-screen flex items-center justify-center`），与 app.vue 既有 loading/error 态同构；**不属于** page shell 四模式（reader/contained/workspace/split），无 dialog
- 主题令牌：`--color-text-secondary` / `--color-text-primary` / `--color-error` 等既有 CSS 变量；`spa-loading-template.html` 因在 CSS bundle 加载前渲染，内联复制各主题对应色值（同一取值来源，双主题两套）
- 图标：`mdi:loading`（spin 动画）/ `mdi:alert-circle`，均已在本地 iconify 子集（app.vue 现用）；模板阶段无 Icon 组件，用 CSS 绘制等价转圈动画
- 按钮：`AppButton variant="primary"`（error.vue 重试入口，与 app.vue 错误态一致）

## 验收映射

- 人工验证（主）：Chrome DevTools Network throttle（Slow 3G / Fast 3G）刷新首屏——JS 下载期间显示主题正确的加载动画（editorial/dark 各验一次，暗色无白闪）；阻断 chunk 请求验证 error.vue 兜底
- 组件测试：`pnpm test:unit` 覆盖 error.vue 渲染（错误文案 + 重试按钮存在性）
- 路由进度条：人工验证页面切换期间顶部进度条出现并随完成消失
