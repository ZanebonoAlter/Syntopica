<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

- **全局导航加载反馈**：新增一个全局瞬时元素（不进入任何页面布局），挂在 `app.vue`（与 `NuxtLoadingIndicator` 同层）。触发入口 = 任意路由切换（侧边栏「叙事工坊」「发现订阅源」、页内返回首页链接、话题跳转等全部路由导航）。
- **首屏 pre-FCP 背景**：`spa-loading-template.html` 内联 `<style>` 追加 `html` 背景色规则（无新入口，纯渲染时机修复）。
- **叙事工坊 tab 面板**：入口不变（tab 点击切换），仅非默认四个 tab 面板改为懒加载。

## 受影响状态

- **loading（新增：路由切换）**：导航开始后 >250ms 未完成 → 内容区上方显示轻量加载反馈；导航完成/失败/取消 → 立即消失。≤250ms 的快导航全程不出现（防闪烁）。视觉 = 既有加载语言：`mdi:loading` 图标 `animate-spin` + `--color-accent` 描边色，居中于内容区视口，半透明不遮挡整页（不盖侧边栏，用户可反悔点别处）。
- **loading（懒加载 tab 首切）**：异步组件就绪前该面板区域显示既有面板级加载样式（居中 spinner，与 `BoardCompositionPanel` loading 态同构）。
- **error（路由切换）**：导航失败沿用既有 `error.vue` 兜底与本反馈的卸载逻辑，不新增错误 UI。
- **success**：反馈消失即原页面挂载流程，无额外确认态。
- **empty**：不受影响。

## 复用组件与布局模式

- 导航加载反馈：复用 `app.vue` 初始化加载态的视觉构件（`Icon(mdi:loading)` + `animate-spin` + 主题令牌 `--color-accent`/`--color-bg-elevated`），不新造视觉语言；不选 layout mode（非页面级布局，fixed 瞬时浮层，双主题令牌适配）。
- 懒加载 tab 加载态：复用各面板既有 loading 插槽/样式，不新增骨架屏体系。
- `spa-loading-template.html` 背景色值与 `app/assets/css/main.css` 主题令牌同源（既有维护约定：改令牌须同步模板内联副本）。

## 验收映射

- 组件测试：导航反馈 composable/组件单测（延迟触发、完成卸载、失败清理、reduced-motion）——`front/app/composables/` 同目录 `.test.ts`。
- opencli/人工验证：dev 环境冷点「叙事工坊」，>250ms 出现加载反馈、切换后消失；深色主题硬刷新无白闪（对照 `docs/research/spa-loading-white-screen/explore-findings.md` 的测量方法）。
