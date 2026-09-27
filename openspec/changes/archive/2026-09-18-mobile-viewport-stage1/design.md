## Context

主工作台为桌面式 flex 三栏（顶栏 + 常驻侧栏 200~500px 可拖宽 + 列表/内容双栏联动），样式集中在 `FeedLayout.css` 语义类 + 组件 scoped style，全应用零媒体查询。布局契约层（`AppPageShell` / `AppDialog` / `layout-contract.ts`）已弹性宽度。动机见 proposal.md；行为要求见 specs/mobile-viewport/spec.md。

**实现面关键事实**（探索结论）：
- `FeedLayoutShell.vue`（537 行）组装 `AppHeaderShell` / `AppSidebarShell` / `ArticleListPanelShell`，列表用 `useVirtualList` 虚拟滚动
- `index.vue` 仅 7 行薄壳；`pages/discovery.vue` 是唯一用 `AppPageShell` 的页面
- 无 layouts/ 目录；`SIDEBAR_DEFAULT/MIN/MAX_WIDTH` 常量在 `utils/constants.ts`

## Goals / Non-Goals

**Goals:**
- 375px 视口核心阅读循环可用（列表 → 阅读 → 返回，含抽屉导航）
- 宽屏（≥768px）代码路径与现状零差异，回归可证
- 窄屏降级逻辑集中可测（断点 composable + 少量组件状态）

**Non-Goals:**
- 不重构宽屏布局/不改 `layout-contract.ts` 常量（四模式、dialog 档不动）
- 不做触控目标全面规范化、底部导航、图谱移动端交互（阶段二）
- 不引入响应式 CSS 范式迁移（语义类 + `@media`，不批量改 Tailwind 断点类）

## Decisions

**D1 断点值取 768px（= Tailwind md）**
主工作台最小可用三栏 ≈ 侧栏 200 + 列表 400 + 内容 400，768px 以下双栏联动已不可用。备选 640px（三栏仍挤爆）、1024px（平板竖屏被误伤）。与 Tailwind md 对齐便于未来混用。

**D2 CSS 降级 + JS 状态机双层方案**
样式收起（侧栏隐藏、单栏化、chips 横滚）用 `@media (max-width: 767.98px)` 纯 CSS；「列表态 ↔ 阅读态」互斥切换是交互语义，用 `useMediaQuery` composable + `FeedLayoutShell` 内 `viewMode: 'list' | 'reading'` 状态实现。纯 CSS 方案无法记忆滚动位置与承载点选语义，被否。`viewMode` 仅窄屏分支读取——宽屏路径不经过该状态，保证零变化。

**D3 抽屉新建 `AppSidebarDrawer.vue`，不复用 AppDialog**
抽屉是锚定侧边的滑入面板，语义与居中 dialog 不同；改造 `AppDialog` 加 anchor 模式会侵入统一弹窗契约（unified-dialog spec）。新组件复用主题 token、Teleport + scrim + `transform` 过渡模式，内容仍由 `AppSidebarView` 供给（同一份信息结构渲染两处：宽屏常驻 / 窄屏抽屉）。

**D4 滚动位置记忆**
列表用虚拟滚动，切换阅读态前记录容器 `scrollTop`，返回列表态后 `nextTick` 恢复；若恢复点超出已加载数据范围，先按页加载补齐再恢复（上限一次补齐，仍不足则落顶）。阅读态自身滚动不记忆（每次进入从头）。

**D5 100dvh 渐进增强**
`FeedLayout.css` 的 `height: 100vh` 改为 `@supports (height: 100dvh)` 内 `100dvh`、外层保留 `100vh` fallback。不引入 PostCSS 插件依赖。

**D6 测试策略（complexity: simple）**
- 组件测试：`AppSidebarDrawer`（开/关/遮罩点击/选中回调）、`useMediaQuery`（断点跨越重置逻辑）、`viewMode` 切换 + 滚动恢复（jsdom 模拟窄屏）
- 契约纯函数不动、无新纯函数；无后端测试
- 视觉验收按 ui-design.md Acceptance：agent-browser/opencli 375×667、390×844、1440×900、1920×1080 四档截图

## Risks / Trade-offs

- [resize 跨断点丢状态] → `useMediaQuery` 变化回调中重置 `viewMode` 并保留 `selectedArticle`，宽屏恢复双栏联动
- [虚拟列表恢复点与分页交互复杂] → D4 的补齐策略 + 组件测试覆盖「深滚动后进阅读再返回」用例；仍不稳则降级为记忆到已加载顶部（记入 tasks 验证节，需与 spec「恢复滚动位置」核对）
- [侧栏两处渲染（常驻+抽屉）状态漂移] → 抽屉内容由同一 `AppSidebarShell` 数据源驱动，仅容器不同；选中态经 props 单向传递
- [iOS 旧版 Safari 无 dvh] → fallback vh（高度略跳，可接受；不阻断）
- [树莓派上视觉验收占资源] → 四档截图串行跑，不与 `pnpm build` 并行（遵守 AGENTS.md 负载纪律）

## Migration Plan

纯前端变更，无数据/API 迁移：构建部署即生效。回滚 = 回退前端构建产物（静态托管，见 deployment.md）。用户可见行为变化：手机访问从「破版不可用」变为「单栏可用」；桌面无任何变化。

## Open Questions

（无——抽屉动效时长、chips 具体样式在实现/原型评审时微调，不影响 spec 与任务拆分。）
