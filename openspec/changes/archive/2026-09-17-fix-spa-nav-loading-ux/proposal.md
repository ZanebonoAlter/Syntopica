<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->

## Why

用户实测反馈两个可感知问题（2026-09-17 诊断，数据见 `docs/research/spa-loading-white-screen/explore-findings.md`）：

1. **加载动画仍有白屏**：深色主题下刷新，HTML ~25ms 即达但 first-paint ~776ms（树莓派高负载），期间浏览器默认白底——`spa-loading-template.html` 只给 `.spa-loading` 容器设了背景，没给 `<html>` 元素预设主题背景色，FCP 前的空窗是白色。
2. **点「叙事工坊」卡且无加载动画**：vue-router/Suspense 行为导致路由切换时旧页面内容区**即刻卸载**、目标页面 chunk 加载+挂载前内容区完全空白（生产构建冷切换 581ms 空白，dev 冷切换数秒）；期间唯一指示是顶部 2px `NuxtLoadingIndicator`（实测 opacity 常为 0，几乎不可见）。tags 页静态导入 5 个 tab 面板 + ~20 组件一棵树，是全站最重路由，加剧冷切换成本。

（诊断同时确认 dev server 存在 ECONNRESET 崩溃重启循环，是「每次都卡」的环境放大器；用户已决定本次不做 prod 日常模式，留待后续。）

## What Changes

1. **路由切换可见加载反馈（全局）**：导航超过 ~250ms 未完成时显示轻量加载反馈（居中/内容区 spinner，复用既有加载态视觉语言：`--color-accent` 转圈 + 主题令牌），导航完成即消失；顶部 2px 进度条保留不动。快导航（暖切换 ~100ms）不出现反馈，避免闪烁。
2. **pre-FCP 白屏修复**：`spa-loading-template.html` 内联样式为 `<html>` 预设双主题背景色（editorial `#faf7f2` / dark `#080c12`），FCP 前空窗不再闪白。
3. **叙事工坊页瘦身**：`TagsPage.vue` 五个 tab 面板（话题总览/日报/文章/数据增强四个非默认 tab）改 `defineAsyncComponent` 懒加载，默认「板块内容」tab 保持 eager；懒加载面板首次切入时的轻量加载态复用既有视觉语言。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `spa-loading-ux`：
  - 「路由切换加载反馈」Requirement 升级——从「顶部进度条」升级为「顶部进度条 + 导航超时可见加载态（>250ms 触发）」；
  - 新增「首屏 pre-FCP 背景色」Requirement——HTML 文档背景在 first-paint 前即匹配主题色，深色主题刷新全程无白闪。

（叙事工坊 tab 懒加载为纯实现层性能优化，无 Requirement 级行为变化，不进 spec delta。）

## Impact

- `front/app/spa-loading-template.html`：内联样式新增 html 背景色（与 main.css 主题令牌同源维护约定不变）。
- `front/app/app.vue` 或新增 `components/common/` 全局导航加载组件 + `composables/` 路由导航状态插件（router beforeEach/afterEach + 延迟显示）。
- `front/app/features/tags/components/TagsPage.vue`：tab 面板懒加载改造。
- 测试：导航反馈 composable 单测（延迟触发/清理/失败路径）、TagsPage 挂载单测、spa 模板断言（html 背景存在性）。
- 文档：`docs/reference/standard/frontend/loading-experience.md`（若存在）与 flow/semantic-board 代码入口节如有涉及需回检。
