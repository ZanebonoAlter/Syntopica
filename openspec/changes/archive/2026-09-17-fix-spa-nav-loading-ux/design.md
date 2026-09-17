# fix-spa-nav-loading-ux 设计

## Context

诊断数据与三层根因见 `docs/research/spa-loading-white-screen/explore-findings.md` 与 proposal.md Why 节。三个修复点互相独立，可单独落地验证：

1. 路由切换无可见反馈（Suspense 无 fallback + 2px 顶条不可见；prod 冷切换 581ms 空白）
2. pre-FCP 白窗（HTML 24ms / FCP 776ms，`<html>` 无预设背景）
3. tags 路由过重（5 tab 面板 + ~20 组件全静态导入一棵树）

dev server ECONNRESET 崩溃循环为环境问题，本 change 不处理（已留档 research，后续单独立项）。

## Goals / Non-Goals

**Goals**
- 所有路由切换超 250ms 有可见加载反馈，快导航零打扰
- 深色/浅色主题刷新全程无白闪（含 pre-FCP 空窗）
- 叙事工坊路由进入成本下降（非默认 tab 面板 + 侦探墙按需加载）

**Non-Goals**
- 不做骨架屏体系（沿用既有面板 loading 态）
- 不改顶部 2px `NuxtLoadingIndicator`（保留）
- 不处理 dev server 稳定性 / prod 日常模式
- 不做 tags 页数据加载策略变更（onMounted 并行拉取维持现状）

## Decisions

### D1 导航加载态：router 插件 + useState + 全局轻量浮层

**状态机**（`composables/useNavLoading.ts`，单测锚点）：

```
idle --(beforeEach)--> pending --(250ms 未完成)--> visible
pending/visible --(afterEach 成功|onError|新 beforeEach)--> idle（清计时器）
```

- 触发接线放 `plugins/nav-loading.ts`（与既有 `chunk-error-fallback` 插件同层）：`router.beforeEach` 启动 250ms 计时器；`router.afterEach`（含 NavigationFailure）与 `router.onError` 清理。连续导航（旧导航未完成又发起新的）由 beforeEach 重置计时覆盖，天然满足「新导航取代旧反馈」。
- 状态载体：`useState('nav-loading', () => ref(false))`（Nuxt 惯例，ssr:false 下等价全局单例，便于插件/组件共享与单测注入）。
- 展示组件 `components/common/NavLoadingOverlay.vue`：`v-if` 渲染 fixed 居中 spinner（`Icon: mdi:loading` + `animate-spin` + `var(--color-accent)`，视觉与 app.vue 初始化加载态同构）；`pointer-events: none`（不拦截点击，用户可反悔导航）；`z-index: 30`（低于 dialog 系与 bottombar 40，高于内容）；`role="status"` + `aria-live="polite"`；`motion-reduce:animate-none`。挂载于 `app.vue`，`<NuxtPage>` 同层（NuxtLoadingIndicator 旁）。
- **为什么不用 Suspense fallback / pageTransition**：NuxtPage 内部 Suspense 不暴露 fallback 插槽；pageTransition 只做过渡不提供反馈内容。插件方案不侵入路由渲染链，覆盖含懒 chunk 在内的全部等待期。

### D2 pre-FCP 背景：模板内 `<style>` 加 html 背景 + 主题脚本前置

`spa-loading-template.html` 调整：

1. 主题判定脚本移到 `<style>` **之前**（当前顺序 style→script；浏览器可能在解析中途首绘，属性选择器规则需 data-theme 已设置才命中 dark 分支）。
2. `<style>` 追加：`html { background: #faf7f2; }` 与 `html[data-theme="dark"] { background: #080c12; }`（色值与模板内 `--spa-bg` 同源，继续遵守「改主题令牌两处同步」既有维护约定，现在为三处：main.css / 模板容器 / 模板 html 背景）。

### D3 tags 页瘦身：非默认 tab 面板 defineAsyncComponent

`TagsPage.vue`：

- 保持 eager：`BoardCompositionPanel`（默认 tab）、`BoardListSidebar`、dialog 群（体积小或交互即需）。
- 懒加载（`defineAsyncComponent`）：`BoardThreadBrowser`（话题总览）、`BoardDailyReportTimeline`（日报）、`BoardTimelinePanel`（文章）、`BoardEnrichmentPanel`（数据增强）、`TopicDetectiveWall.client`（侦探墙，v-if 默认 false 但模块仍会进首包）。
- loadingComponent：新增极简 `PanelAsyncPlaceholder.vue`（居中 spinner，视觉同 D1 构件），`delay: 200`（暖切 <200ms 不闪占位）；`timeout` 不设（慢设备容忍，由导航加载态兜底整体反馈）。
- 单测影响：`TagsPage.test.ts` 现有 mock 是 Proxy 兜 ref，懒加载组件在 tab 未激活时不渲染，既有断言不受影响；新增用例断言「默认 tab 不加载面板模块 / 切 tab 后渲染对应占位→面板」。

### D4 验证方式

- 单测：`useNavLoading.test.ts`（fake timers：250ms 前不显示/后显示、afterEach 清理、onError 清理、连续导航重置）；`NavLoadingOverlay.test.ts`（visible 渲染、aria、reduced-motion class）；`TagsPage.test.ts` 增补；`PanelAsyncPlaceholder.test.ts`。
- 模板断言：新增 `spa-loading-template.test.ts` 读文件断言含 `html[data-theme="dark"]` 背景规则与脚本顺序（主题脚本先于 style）。
- 手动/opencli（归档证据）：agent-browser 或浏览器复测 research 中的测量法——深色主题硬刷新无白窗；冷切叙事工坊 >250ms 出现反馈。

## Risks / Trade-offs

- **overlay 遮挡**：`pointer-events:none` + 适中 z-index，不挡侧边栏与浮层交互；视觉为内容区居中小 spinner，不做整页遮罩（避免打断感）。
- **懒加载首切多一跳**：面板模块单独请求（prod 为独立小 chunk；dev 为按需 transform），有 delay=200 占位 + 导航加载态双反馈，体验不回退。
- **模板三处色值同步**：接受（既有约定的延伸），模板测试只断言规则存在不断言色值，避免脆断言。

## Open Questions

（无——范围已与用户确认：反馈 + 白屏快修 + 瘦身三项，不含 prod 模式。）
