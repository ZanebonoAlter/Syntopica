<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: reading -->

## Why

日报/版块页点开「文章预览」弹窗（`ArticlePreviewModal`）后，切到「内嵌网页」（iframe 模式），网页只显示约 150px 高的一小条，无法阅读。根因：弹窗宿主的高度链断裂——`AppDialog` 的 body 不是 flex 容器且弹窗高度由内容撑开，`ArticlePreviewModal` 的 `.preview-body` 写的 `flex: 1` 无效（高度 auto），导致 `ArticleContentView` 的 `h-full` → `.iframe-mode`(flex:1) → `iframe`(h-full) 整链塌陷到 iframe 浏览器默认高度。主界面（FeedLayoutShell）从根上有 `100vh` 确定高度不受影响，仅弹窗宿主中招。

## What Changes

- `ArticlePreviewModal` 的内容区 `.preview-body` 改为**确定高度**（`calc(85vh - 110px)`：85vh 为 AppDialog 上限，减去 header ≈61px 与 body 上下 padding 40px，留 9px 余量），使内部 `h-full`/`flex:1` 链条可解析——iframe 模式撑满、文本预览模式改为内容区内部滚动（工具栏常驻，体验顺带变好）。
- 新增组件测试固化「弹窗内容区必须带确定高度」的结构契约（jsdom 无法测真实布局，用内联 height 样式作为可断言的契约锚点）。
- 不改 `AppDialog`（通用外壳，其余弹窗依赖内容撑高，动它会波及全部弹窗）；不改 `ArticleContentView` / `ArticleContent.css`。

## Capabilities

### New Capabilities
- `article-preview-dialog`: 文章预览弹窗（tags 域日报/版块共用的 `ArticlePreviewModal`）的能力契约——AppDialog 宿主 + ArticleContentView，内容区确定高度，预览/内嵌网页两种模式都撑满弹窗可用高度。

### Modified Capabilities

（无——`unified-dialog` 只约束外壳组件行为，外壳未改；`reading-article-pane` 只约束阅读版式，版式未改。）

## Impact

- 代码：`front/app/features/tags/components/ArticlePreviewModal.vue`（scoped 样式 + 模板 height 绑定）；新增 `front/app/features/tags/components/ArticlePreviewModal.test.ts`。
- 行为变化（部署后可见）：弹窗文章预览切「内嵌网页」后网页占满弹窗内容区；文本预览模式滚动条从弹窗 body 移到内容区内部（工具栏/进度条不再随内容滚走）。
- 无数据迁移、无接口变更、无后端改动。
