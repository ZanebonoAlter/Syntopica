<!-- complexity: simple -->
<!-- ui-impact: major -->
<!-- constraint-domains: reading -->

## Why

阅读页中间栏（文章列表面板）信息密度失衡且观感拼贴：feed 级订阅状态卡（刷新/总结/抓取三 pill）常驻列表顶部把文章往下顶；每张文章卡固定挂 3 个处理流程 chip（抓取/总结/打标签），feed 开启抓取+总结时全量显示、AI 未连通时满屏 amber「待抓取/待总结」警告色；头部三段 chrome 用 85% 白（`--color-bg-hover`）而列表区是奶油底（`#f5f0e6`）、卡片与栏底同色全靠描边阴影区分——「白条 + 咖啡底 + 一摞框」两种 surface 语言打架。阅读场景的常态（处理成功/排队）无需逐卡展示，列表应回归「标题 + 来源 + 时间」的扫描优先形态。

## What Changes

- 文章列表「卡片摞」改**行式列表**：去卡片框与投影，细分隔线分层，整栏统一 surface 底色（仅中间栏样式作用域内重排 token 引用，不动 `main.css` 全局色板），选中行高亮
- 处理状态 chip 收敛为「常态零 chip」：成功/排队（pending）**不显示**；进行中 → 行内小转圈图标；失败 → 行内红色 ⚠ 小标，点击就地展开浮层显示错误详情（与标签展开同一套交互）
- 标签状态（待打标签/已标记 N）默认隐藏：行角落淡色 🏷 图标，点击就地展开/收起浮层显示标签状态
- 移除订阅源状态大卡：缩为标题栏 ⓘ 图标 + 点击 popover（只读展示 刷新/总结/抓取 三项，改配置仍去设置页）
- 日期筛选从独占行并入标题栏 📅 图标按钮（下拉面板复用现有快速选项 + 日期范围），激活时标题栏显示条件 chip、可一键清除
- 单 feed 视图隐藏行内重复 feed 名；全部文章/收藏夹/分类视图保留来源显示
- 修复虚拟列表 `itemHeight: 120` 与实际行高不符的滚动错位风险（行式布局后行高收敛为固定基准值）

纯前端展示层改动，不涉及后端 API/数据变更，无 BREAKING。

## Capabilities

### New Capabilities
- `reading-list-panel`: 阅读页文章列表面板展示契约——行式列表布局与行状态（默认/hover/选中/已读）、处理状态可见性规则（成功/排队隐藏、进行中转圈、失败红标+点开详情浮层）、标签状态默认隐藏可点开、订阅源状态 popover（只读）、日期筛选头部化与激活条件 chip、单 feed 视图元数据去重

### Modified Capabilities

（无——现有 spec 无阅读列表面板能力，本 change 新建）

## Impact

- **代码**（纯前端，4 个既有文件 + 可能新增 1 个浮层小组件）：
  - `front/app/features/shell/components/ArticleListPanelView.vue`（头部三合一、订阅状态 popover、日期面板挂载点、虚拟列表行高修正）
  - `front/app/features/articles/components/ArticleCardView.vue`（行式重构、chip 收敛、标签/错误展开浮层）
  - `front/app/components/layout/ArticleListPanel.css`、`front/app/components/article/ArticleCard.css`（surface 统一、行式样式）
- **复用面**：`ArticleCardView` 仅被 `ArticleListPanelView` 引用（经 `features/articles/public.ts` 导出，已核实），其他页面不受影响
- **不改动**：右侧阅读区、左侧边栏、顶栏、`main.css` 全局 token、后端；新增 mdi 图标名须重新生成本地图标子集（`pnpm generate:icons`，flow/reading.md 约束 5）
