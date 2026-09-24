<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

**无新增入口。** 受影响入口均为既有入口，交互路径不变：

| 入口 | 现状 | 本 change 后 |
|---|---|---|
| 中间栏文章列表（`FeedLayoutShell` → `useArticlePagination`） | 列表请求返回含正文的宽响应 | 列表请求返回窄投影（含 `excerpt`）；行渲染字段不变 |
| 阅读页导语（`ArticleContentPreviewPanel` 的 `article.description`） | 依赖 store 里列表带来的 `description` | 首帧用列表 `excerpt`，详情返回后换成详情 `description`（既有的「先放列表行做首帧、再取详情覆盖」流程不变：`FeedLayoutShell.hydrateSelectedArticle`） |
| 自动刷新（每 feed 定时器 → `fetchArticles({per_page:10000})`） | 刷新后全量重拉 100 条并整表替换 | 刷新后重取当前视图当前页；列表内容与选中态不变 |

## 受影响状态

| 状态 | 变化 |
|---|---|
| loading | 不变（列表骨架/加载态逻辑不触） |
| empty | 不变 |
| error | 列表接口失败路径不变；新增「`excerpt` 为空」→ 导语区不渲染（沿用现有 `showDescription` guard，无占位块） |
| success | 行渲染字段一致；刷新后列表更新由「整表替换」变为「当前页重取」，滚动位置与选中行 MUST 保持不变 |
| success（导语去重，2026-09-24 补充） | 导语与正文重复的文章（实测 `description == content` 占非归档 92%；另有导语来自正文兜底的 198 条）**不返回 excerpt** → 首帧与终态一致（均不渲染导语），不出现「首帧闪现→详情返回后消失」的布局跳动；导语与正文不同的文章首帧渲染 excerpt、详情返回后渲染详情 `description` |
| 部署切换窗口（旧标签页 + 新后端） | normalizer 容错：大字段缺失时 `content`/`description` 取空串、导语回退 `excerpt`，界面不报错 |

## 复用组件与布局模式

- 布局模式：**reader（760）**，无变化 —— 中间栏与阅读页的既有 layout mode、宽度与溢出策略均不修改。
- 复用组件：`ArticleCardView`（行卡片，仅消费 title/link/author/read/favorite，无需改）、`ArticleContentPreviewPanel`（阅读页面板，数据来源调整，DOM 结构不变）、`RowStatusPopover`/`ArticleStatusMenu`（处理状态，不进本 change 范围）。
- 不新增组件、不新增布局契约、不涉及 dialog 尺寸档。

## 验收映射

| 契约点 | 验收方式 |
|---|---|
| 列表行渲染字段不变（title/author/时间/已读/收藏/状态图标） | 组件测试 `ArticleCardView.test.ts` + 人工比对（1440×900 单视口） |
| 列表响应体积与字段集（无 content/firecrawl_content/HTML description，含 excerpt） | 后端 handler 测试断言字段集与体积上限；`curl` 验证节 |
| 阅读页导语仍正常显示（改由详情提供） | `useArticleContentView` 相关单测 + 人工验证打开一篇文章 |
| 刷新后选中行与滚动位置不变 | 人工验证（刷新触发后列表不跳顶、选中不丢） |
