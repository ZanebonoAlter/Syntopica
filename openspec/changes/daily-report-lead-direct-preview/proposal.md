<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report -->

## Why

日报阅读台点击线索引用文章，当前先弹右侧文章抽屉（`DeskArticleDrawer`，60vw），用户要再点一次「站内阅读」才能进入真正的阅读预览——两次点击、两次打断。预览弹窗（`ArticlePreviewModal`）本身就是站内阅读入口且已在本页页边注（`MarginNotesRail`）直达使用，线索点击没必要在中间垫一层抽屉。

## What Changes

- 日报阅读台（`/tags/:boardId/:date`）点击线索引用文章 SHALL **直接打开站内阅读预览弹窗**（复用既有 `ArticlePreviewModal`），不再先弹 `DeskArticleDrawer`。
- 直达路径补齐弹窗打开前的加载态与失败反馈：详情接口拉取期间弹窗显示 loading，失败给出可见错误提示（沿用 `useNotify`），不再像旧抽屉那样内嵌重试按钮。
- 删除 `DeskArticleDrawer.vue` 组件（唯一使用方就是阅读台壳；其「来源/日期/摘要」信息与「打开原文」动作在预览弹窗的工具栏/正文视图中均有等价承载，无功能丢失）。
- 不变项：`openArticle(articleId)` 事件契约（`DailyReportTopicSection` / `TopicDetectiveWall` → `DailyReportReader` → 壳）、预览弹窗收藏乐观更新与状态回流契约（daily-report flow 约束 32）、弹窗全屏层级契约（reading 约束 14）、线索文章标题列表的缓存复用与重试——全部保持现状。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `daily-report-system`：「阅读台文章抽屉与站内阅读预览的状态同步」Requirement 调整为「阅读台线索文章直达站内阅读预览」——点开文章直达预览弹窗（含加载/失败反馈），抽屉层级移除，乐观收藏/回滚/连点/状态回流四个场景原样保留；「日报时间线组件 BoardDailyReportTimeline（报纸布局）」Requirement 中 thread 文章点击行为同步改为直达预览弹窗。

## Impact

- **代码**（全部前端，`front/app/features/tags/`）：
  - `components/daily-report/DailyReportDeskShell.vue`：`@open-article` 由「置 articleId 开抽屉」改为「拉详情 → normalizeArticle → 开预览弹窗」（参照 `pages/search.vue` openPreview 同款模式）；新增 `loadingPreviewArticle` 状态与失败 notify。
  - `components/daily-report/DeskArticleDrawer.vue`：删除。
  - `components/daily-report/DailyReportDeskShell.test.ts`：drawer stub 断言改为「open-article → 直接渲染预览弹窗」；`pages/tags/[boardId]/[date].test.ts` 相应核对。
- **归档顺序依赖**：本 change 的 delta 修改的需求名来自 `rework-workshop-into-radar-and-desk`（99/106，未归档）的 delta——其归档先于本 change 时主 spec 才有被修改的需求本体。**本 change SHALL 在 rework-workshop-into-radar-and-desk 归档之后归档**，避免 delta 同步互相覆盖。
- **文档**：`docs/reference/flow/daily-report.md` L172 链路图、L278 代码入口（含 DeskArticleDrawer 条目）、约束 32 措辞（「文章抽屉」）随归档 §12 溯源更新。
