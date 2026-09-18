<!-- complexity: simple -->
<!-- ui-impact: major -->
<!-- constraint-domains: reading, discovery -->

## Why

当前前端零媒体查询、377 个文件仅 7 处响应式断点，产品定位桌面优先（布局契约双视口验收仅覆盖 1440/1920 两档桌面视口）。手机 375px 宽打开主工作台时，常驻侧栏（min 200px）+ 文章列表 + 内容面板三栏同屏挤爆不可用。布局契约层（AppPageShell 四模式 / AppDialog 92vw 上限）已是弹性宽度，缺的是窄视口降级层。阶段一目标：**375px 不破版底线**——手机上能刷文章列表、看正文和日报；宽屏（≥1024px）行为零变化。

## What Changes

- **主工作台窄屏降级**（`FeedLayoutShell` 组件族）：视口 <768px 时常驻侧栏改为 overlay 抽屉（默认收起，汉堡入口）；文章列表 / 内容面板由双栏联动改为单栏切换（列表态 ↔ 阅读态，点选文章进入阅读态，返回回列表态）
- **视口高度修正**：`height: 100vh` → `100dvh`（带 fallback），修复手机浏览器地址栏收放导致的高度跳动
- **顶栏窄屏收纳**（`AppHeaderView`）：窄屏下溢出项收进菜单，保证单行
- **各页面 375px 破版走查修复**：reader/contained 模式页面（settings / tags / discovery / 日报）逐页走查，修复横向溢出与不可点元素
- **宽屏零变化**：≥1024px 视口下布局、交互与现状完全一致（回归保护）

**不在本阶段（阶段二另开 change）**：底部 tab 导航、触控目标全面加大（44px 规范化）、hover 交互的触屏替代、图谱等 workspace 复杂交互的移动端完整体验。

## Capabilities

### New Capabilities

- `mobile-viewport`: 窄视口（<768px）布局降级行为契约——侧栏抽屉化、列表/内容单栏切换、dvh 高度、各 shell 模式页面 375px 不破版、宽屏零变化回归

### Modified Capabilities

（无——`reading-list-panel` / `reading-article-pane` 的宽屏行为不变；窄屏降级统一收在新 capability，避免跨页面能力散落单面板 spec）

## Impact

- **代码**：`front/app/features/shell/components/`（FeedLayoutShell / AppSidebarView / AppHeaderView / ArticleListPanelView）、`front/app/components/FeedLayout.css`、`front/app/utils/constants.ts`（侧栏宽度常量）、各 `pages/*.vue` 窄屏走查修复
- **无后端/API/数据/依赖变更**：纯前端布局层，不触数据库与接口
- **文档**：`docs/reference/standard/frontend/layout.md` 补窄视口降级节（doc-impact 见 tasks 文档节）
