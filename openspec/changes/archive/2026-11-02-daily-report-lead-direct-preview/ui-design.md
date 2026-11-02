<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

- 入口：日报阅读台 `/tags/:boardId/:date`（desk 模式），线索正文区可原位展开「相关文章」标题列表；点击标题即触发 `openArticle(articleId)`。
- 变更：点击后的响应从「右侧文章抽屉（`DeskArticleDrawer`，60vw）→ 抽屉内点『站内阅读』→ 预览弹窗」两级链路，改为**直达站内阅读预览弹窗**（`ArticlePreviewModal`）。本页页边注（`MarginNotesRail` 打开引文）已是直达预览弹窗的既有先例，本次把线索文章对齐到同一交互模式；不新增任何入口，`openArticle(articleId)` 事件契约与侦探墙出口不变。

## 受影响状态

| 状态 | 现状（抽屉） | 改后（直达弹窗） |
| --- | --- | --- |
| loading | 抽屉打开后内部显示「文章加载中…」 | 弹窗先开、内容区显示加载态（`loading-preview-article` prop 既有通道，同 search 页模式） |
| error | 抽屉内嵌错误行 + 重试按钮 | 弹窗打开前请求失败 → `useNotify` 可见错误提示、不弹空弹窗；弹窗内错误态沿用 `ArticleContentView` 既有行为，不加新实现 |
| empty | 抽屉显示「暂无正文摘要」占位 | 沿用 `ArticleContentView` 既有空态/摘要回退链（description guard 等），无新增 |
| success | 抽屉摘要 + 两按钮（站内阅读/打开原文） | 弹窗全功能：正文视图、工具栏收藏（乐观更新）、状态菜单、「在新窗口打开原文」、内嵌网页/全屏 |

## 复用组件与布局模式

- 复用 `ArticlePreviewModal`（`AppDialog` 90vw×85vh，desk 页面 shell 已挂载该实例，本次仅改触发接线）；复用 `useNotify`、`normalizeArticle`、`useArticlesApi.getArticle`。
- 页面 layout mode 不变（`AppPageShell mode="workspace"` 的 desk 全高布局）；弹窗尺寸档走 `ArticlePreviewModal` 既有契约（90vw，非四档 sm/md/lg/xl，属既有豁免形态，本次不另立宽度）。
- 删除 `DeskArticleDrawer.vue`（含 `AppDrawer` 用法；`AppDrawer` 组件本身仍被 FeedLayoutShell/侧栏等其他宿主使用，不动）。

## 验收映射

- 组件测试：`front/app/features/tags/components/daily-report/DailyReportDeskShell.test.ts`（改写：open-article → 直接渲染预览弹窗 stub 并带上正确文章数据；删除 drawer stub 断言）；`front/app/pages/tags/[boardId]/[date].test.ts`（核对引用面不回归）。
- 端到端：主链路「点线索文章 → 预览弹窗出现且无抽屉」走 agent-browser，在 `ui-verification/report.md` 建账（静态 :5100）。
- 既有行为回归：收藏乐观更新/回滚、状态回流场景由 `DailyReportDeskShell.test.ts` 现有断言继承（详见 test-cases.md 继承与调整表）。
