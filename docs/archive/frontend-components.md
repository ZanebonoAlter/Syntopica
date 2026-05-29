# 前端组件分工

## 路由层

- `front/app/pages/index.vue` 只挂载 `FeedLayoutShell`
- `front/app/pages/digest/index.vue` 只挂载 `DigestListView`
- `front/app/pages/digest/[id].vue` 只负责把 `daily` / `weekly` 路由参数锁定给 digest 视图

路由层不承载业务状态。

## Shell 层

### `front/app/features/shell/components/FeedLayoutShell.vue`

主阅读页编排层。

- 串起顶部栏、侧栏、文章列表栏、正文区
- 管理当前选中的分类、feed、文章、AI 总结
- 管理添加 / 编辑 / 导入 / 设置类弹窗
- 负责手动刷新、全部已读、OPML 导出
- 在 feed 刷新后轮询订阅源状态并回填文章列表
- 负责从主壳跳转到 `/digest`

### `front/app/features/shell/components/AppHeaderShell.vue`

- 顶部操作按钮
- 刷新消息提示
- “全部已读”、新增 feed、分类、导入、设置入口

### `front/app/features/shell/components/AppSidebarShell.vue`

- 分类树
- feed 列表
- 全部文章、收藏、AI 总结、Digest 快捷入口
- 分类和 feed 的编辑入口
- 侧栏折叠 / 展开

### `front/app/features/shell/components/ArticleListPanelShell.vue`

- 中栏列表壳
- 文章点击、收藏、筛选等交互桥接到 view 组件

## Articles

### `front/app/features/articles/components/ArticleCardView.vue`

- 单篇文章卡片
- 已读、收藏、摘要信息和基础元数据展示

### `front/app/features/articles/components/ArticleContentView.vue`

文章阅读主组件，也是当前最复杂的前端组件之一。

- 展示正文、元信息、封面图
- 切换预览模式与 iframe 模式
- 支持上下篇切换
- 支持全屏阅读
- 支持收藏、打开原文
- 集成阅读行为埋点
- 展示 Firecrawl 抓取状态与 AI 整理状态
- 支持手动抓取全文、手动生成整理稿
- 在“原始内容”和“Firecrawl 全文”之间切换
- 如果已有 `aiContentSummary`，优先展示整理稿

### `front/app/features/articles/components/ContentCompletionView.vue`

- 更偏状态展示
- 用于内容补全过程反馈

### `front/app/features/articles/composables/useContentCompletion.ts`

- 查询单篇文章内容补全状态
- 触发单篇文章补全
- 触发整条 feed 的批量补全

### `front/app/features/articles/composables/useArticleProcessingStatus.ts`

- 将 Firecrawl / AI 状态转成适合 UI 展示的标签、图标和语气色

## Summaries

### `front/app/features/summaries/components/AISummariesListView.vue`

- AI 总结列表页
- 支持按分类、feed、日期过滤
- 支持分页和每页数量切换
- 支持删除总结
- 支持发起新的总结任务
- 通过 WebSocket 监听总结队列进度

### `front/app/features/summaries/components/AISummaryDetailView.vue`

- 展示单条 AI 总结详情
- 配合主壳右栏显示

### `front/app/features/summaries/composables/useSummaryWebSocket.ts`

- 连接 `/ws`
- 接收队列进度
- 把 WS 消息转成前端批次对象
- 处理断线重连

## Digest

### `front/app/features/digest/components/DigestListView.vue`

- Digest 总览壳
- 在日报和周报之间切换
- 通过日期选择器切换锚点日
- 拉取预览、执行状态和当前版面
- 支持立即执行
- 支持打开设置抽屉
- 左栏展示分类和状态，中栏展示 feed 级 AI 总结，右栏挂 `DigestDetail`

### `front/app/features/digest/components/DigestDetail.vue`

- 展示单条 digest summary 的正文
- 渲染 markdown
- 拉取关联文章
- 支持在弹窗里直接阅读关联文章
- 能把总结里的链接映射回已知文章

### `front/app/features/digest/components/DigestSettings.vue`

- 配置日报 / 周报
- 配置飞书推送
- 配置 Obsidian 导出
- 测试飞书和 Obsidian 连接

## Feeds 与 Preferences

### `front/app/features/feeds/composables/useAutoRefresh.ts`

- 启动全局自动刷新逻辑

### `front/app/features/feeds/composables/useRefreshPolling.ts`

- 提供刷新轮询能力
- 配合主壳刷新 feed 状态

### `front/app/features/preferences/composables/useReadingTracker.ts`

- 跟踪文章打开、关闭、滚动、收藏、取消收藏
- 30 秒批量上报一次
- 达到事件数量阈值也会主动上报

## 通用组件层

`front/app/components/*` 现在主要保留可复用组件，而不是业务入口。

主要包括：

- `components/dialog/*`：新增 / 编辑 / 导入 / 全局设置弹窗
- `components/feed/*`：feed 图标和刷新状态
- `components/category/*`：分类卡片
- `components/common/*`：通用 UI 小组件
- `components/layout/*`：布局相关组件
- `components/ai/AISummary.vue`：临时 AI 分析卡片
- `components/article/*`：文章相关通用组件

## 约定

- 业务壳在 `features/*/components/*`
- 路由只做挂载
- API 不写进组件
- 跨层数据流优先走 store 和 composable
- 旧式 `components/*` 业务壳不再继续扩展
