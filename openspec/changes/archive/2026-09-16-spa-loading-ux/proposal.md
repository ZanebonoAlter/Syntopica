<!-- complexity: simple -->
<!-- ui-impact: minor -->

## Why

树莓派部署 + 远程访问场景下网络不稳定，前端首屏资源（JS bundle、Google Fonts 外链）加载慢：`ssr: false` 空壳 HTML 在 JS 挂载前无任何视觉反馈（默认 Nuxt loading 模板为白底，暗色主题下闪白），fonts.googleapis.com stylesheet 渲染阻塞加剧白屏；页面 chunk 懒加载与加载失败也无反馈/兜底，用户体验差。

## What Changes

- 新增自定义 `app/spa-loading-template.html`：JS 挂载前的首屏加载动画（纯内联 HTML+CSS，无 JS 依赖），读 `localStorage` 主题令牌适配 editorial/dark 双主题，替代 Nuxt 默认白底模板
- Google Fonts 外链本地化（或等效非阻塞加载）：消除 `fonts.googleapis.com` stylesheet 对首屏渲染的阻塞，弱网/离线不再等待外链超时
- 全局路由切换反馈：`app.vue` 增加 `NuxtLoadingIndicator`，页面 chunk 懒加载期间显示顶部进度条
- 新增 `app/error.vue` 全局错误兜底页：chunk/运行时加载失败时展示错误信息 + 重试入口（复用 app.vue 既有错误态视觉模式），替代白屏
- 不改变任何业务功能与数据流；纯前端加载体验层改动

## Capabilities

### New Capabilities
- `spa-loading-ux`: SPA 首屏/路由切换/加载失败三个阶段的加载体验契约——首屏加载模板（含主题适配）、外链字体非阻塞、路由加载指示、全局错误兜底页

### Modified Capabilities

（无——现有 specs 无加载体验相关能力；theme-system 不改需求，仅消费其主题令牌）

## Impact

- **代码**：`front/nuxt.config.ts`（字体外链处理）、`front/app/spa-loading-template.html`（新增）、`front/app/error.vue`（新增）、`front/app/app.vue`（加 NuxtLoadingIndicator）；可能新增 `front/app/assets/fonts/` 本地字体产物
- **构建/依赖**：本地化字体引入 `@fontsource` 类依赖或等价自托管方案；无后端改动
- **部署**：自托管字体后静态产物体积增加（Noto Serif SC 中文子集，需权衡 subset 方案）；部署后弱网首屏从白屏变为加载动画
- **文档**：`docs/reference/standard/frontend/` 可能需补加载体验相关约定（doc-impact 于 tasks 文档节声明）
