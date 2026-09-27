## Context

见 proposal.md - Why。现状约束：`ssr: false` 纯 SPA；主题通过 head 内联脚本同步设置 `data-theme`（editorial/dark，localStorage `syntopica-theme`）；iconify 已本地化（`plugins/iconify-local.ts`）；`nuxt.config.ts` head 中挂 fonts.googleapis.com 外链 stylesheet。视觉契约见 ui-design.md（minor：全部复用既有主题令牌与 app.vue 全屏居中 fallback 模式）。

## Goals / Non-Goals

**Goals:**
- JS 挂载前、路由切换期、加载失败三个阶段均有主题正确的视觉反馈，全程无白屏
- 首屏渲染路径零外部域名请求（字体自托管后与 iconify 本地化对齐，彻底离线可用）

**Non-Goals:**
- 不做骨架屏（skeleton，后续可另行立项）
- 不做 SSR/SSG、PWA Service Worker 离线缓存
- 不改任何业务组件的数据加载态（FeedLayoutShell 等维持现状）
- 不做 bundle 体积优化/手动 chunk 分割（若需另行立项）

## Decisions

### D1: 字体本地化用 `@fontsource` 自托管，不做手动 subset

- **选择**：`pnpm add @fontsource/noto-serif-sc`，入口 import 对应字重 CSS；删除 head 中 fonts.googleapis.com 外链。
- **理由**：@fontsource 的 CJK 包按 unicode-range 切片成百余个小 woff2，浏览器只下载页面实际用到的分片（单分片 ~10-100KB），弱网首屏增量极小；资源同源部署在 `_nuxt/assets/`，与 iconify 本地化策略一致，彻底消除墙外请求与渲染阻塞。
- **备选**：① 保留外链 + `media="print" onload` 非阻塞技巧——请求仍发往墙外，弱网照样慢，否决；② `cn-font-split`/fonttools 手动 subset——体积更优但引入 Python 工具链与维护成本，对当前痛点是过度设计。

### D2: 加载模板 = 纯静态 HTML + 内联 CSS/主题判定，不用任何框架机制

- **选择**：`app/spa-loading-template.html` 内联 `<style>` 与一段几行的内联 script（读 localStorage 设 `data-theme`，逻辑与 head 主题脚本一致）；双主题用 `[data-theme="dark"]` 选择器覆盖内联色值；转圈用纯 CSS 动画（不依赖 iconify）。
- **理由**：模板渲染时 CSS/JS bundle 均未加载，必须自包含；主题判定逻辑与 `app.vue` head 脚本同源同判（`editorial`/`dark`，异常回退 editorial），保证模板色与应用色一致、暗色不闪白。
- **备选**：纯 CSS 无 script——无法读 localStorage，暗色必闪白，否决。

### D3: 路由进度条显式传主题色

- **选择**：`app.vue` 顶层 `<NuxtLoadingIndicator :color="'var(--color-accent)'" />`（具体令牌实现期按主题系统实际 accent 变量名取）。
- **理由**：组件默认色是 Nuxt 品牌绿，与主题不搭；CSS 变量取值自动跟随 `data-theme` 切换。

### D4: chunk 加载失败依赖 Nuxt 内建错误链路，实测不触发再补 router 捕获

- **选择**：新增 `app/error.vue` 接管 `useError` 渲染兜底页。动态 import 失败（chunk 加载错误）预期走 vue-router errorHandler → Nuxt `showError` → error.vue；实现期以弱网/断 chunk 实测验证，若 Nuxt 未自动捕获则在插件中对 router 错误补调 `showError`。
- **理由**：优先用框架内建链路，避免过早包一层自定义错误处理；验证任务已列入 tasks。
- **error.vue 结构**：复用 app.vue 错误态同构布局（`h-screen` 居中、`mdi:alert-circle`、`AppButton variant="primary"` 触发 `router.go(0)`/`clearError({ redirect: '/' })` 按错误类型选择恢复方式）。

## Risks / Trade-offs

- [字体包磁盘占用增加]（CJK 全量分片总量较大，构建产物体积上升）→ 首屏只拉实际分片、网络增量小；树莓派本地磁盘代价可接受；后续可用 subset 收紧
- [模板与应用挂载瞬间视觉跳变]（模板转圈 → app.vue 初始化转圈，两套实现）→ 两者视觉刻意同构（同居中/同动画形态/同文案），跳变感知最小化；验收含连续性人工检查
- [chunk 失败是否自动进 error.vue 存在框架行为不确定性] → D4 验证任务兜底，实测不触发则补捕获
- [error.vue 过度捕获正常业务错误]（把可局部恢复的错误也全屏化）→ 兜底页仅接管路由级/未捕获错误；现有 AiHealthBanner/NotifyContainer 局部错误展示不变

## Migration Plan

纯前端静态变更，无数据/接口迁移。部署即生效：弱网首屏从白屏变为加载动画，字体来源切自托管。回滚 = git revert 前端仓库。
