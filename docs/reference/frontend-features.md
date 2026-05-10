# 前端功能说明

## 页面入口

- 主阅读页：`/`
- Digest 总览：`/digest`
- Digest 单视图：`/digest/daily`、`/digest/weekly`
- Digest 详情：`/digest/[id]`
- Topic Graph：`/topics`

## 主阅读页

主阅读页是三栏结构。

- 左栏：导航、分类、feed、快捷入口
- 中栏：文章列表或 AI 总结列表
- 右栏：文章正文或 AI 总结详情

### 顶部工具栏

支持这些动作：

- 刷新当前 feed 或全部 feed
- 全部标记为已读
- 新增订阅源
- 新增分类
- 导入 OPML
- 打开全局设置

### 左侧导航

支持这些入口：

- 全部文章
- 收藏
- AI 总结
- Digest
- 分类树
- feed 列表

分类和 feed 的点击会驱动主壳重新拉取对应文章。

## Feed 与分类管理

### 分类

- 新增分类
- 编辑分类
- 删除分类
- 分类可配置名称、图标、颜色、描述

删除分类时，主界面文案明确说明"不删除该分类下订阅源"。

### Feed

- 新增 feed
- 编辑 feed
- 删除 feed
- 单 feed 手动刷新
- 全量刷新
- OPML 导入
- OPML 导出

feed 还带这些能力开关：

- `ai_summary_enabled`
- `article_summary_enabled`
- `completion_on_refresh`
- `max_completion_retries`
- `firecrawl_enabled`

## 文章阅读

正文区由 `ArticleContentView.vue` 驱动。

### 基础阅读能力

- 展示文章标题、时间、作者、封面
- 收藏 / 取消收藏
- 打开原文
- 上一篇 / 下一篇
- 全屏阅读
- 预览模式和 iframe 模式切换

### 已读与阅读行为

- 打开文章时自动标记已读
- 跟踪打开、关闭、滚动、收藏、取消收藏
- 30 秒批量上传一次阅读行为
- 累积 10 条事件也会触发上传

### 内容增强

如果后端已配置内容增强能力，正文区会展示状态面板。

- Firecrawl 抓取状态
- AI 整理状态
- 抓取时间、总结时间、尝试次数
- 手动抓取全文
- 手动生成整理稿

### 内容源切换

当原始内容和 Firecrawl 全文都存在时，正文区支持切换：

- 原始内容
- Firecrawl 全文

如果已经有 `aiContentSummary`，会优先展示 AI 整理稿。

### 文章标签

文章会展示由 AI 提取的 topic tags，支持标签合并、抽象层级和标签质量评分。

## AI 总结

AI 总结列表在主阅读页中栏展示，详情在右栏展示。

### 列表能力

- 按分类过滤
- 按 feed 过滤
- 按日期过滤
- 快捷日期范围筛选
- 分页
- 删除总结

### 生成能力

- 可选择时间窗口
- 发起多分类批量总结任务
- 通过 WebSocket 实时显示队列进度
- 支持失败任务错误展开

### AI Provider 管理

通过 Web UI 管理 AI Provider 和路由：

- 配置多个 AI Provider（不同 base URL、API key、model）
- 为不同能力（摘要、内容补全、话题分析等）配置路由
- 支持测试连接

### 依赖

AI 总结依赖全局 AI 设置中的：

- `baseURL`
- `apiKey`
- `model`

如果没有配置 API Key，列表顶部会给出提示。

## Digest

Digest 是独立页面，不嵌在主阅读壳里。

### Digest 总览

- 支持日报 / 周报切换
- 支持按日期切换锚点
- 支持刷新当前版面
- 支持立即执行
- 支持查看任务状态

### Digest 详情

- 左栏看分类与运行状态
- 中栏看 feed 级 AI 总结
- 右栏看总结正文

### 关联文章

每条 digest summary 都可以拉取关联文章。

- 点击关联文章后弹窗阅读
- 弹窗里复用 `ArticleContentView`
- 仍保留收藏、抓取、整理等动作

### 设置项

Digest 设置支持：

- 日报开关和时间
- 周报开关、星期和时间
- 飞书推送开关与 webhook（支持摘要/详情两种推送模式）
- Obsidian 导出开关与 vault 路径
- Open Notebook 发送配置
- 测试飞书
- 测试 Obsidian

## Topic Graph

Topic Graph 是独立页面，不走主阅读页三栏壳。

### 页面能力

- 支持 `daily / weekly` 切换
- 支持按锚点日期刷新图谱
- 支持 3D topic/feed 图谱浏览
- 支持热点标签分组浏览（事件 / 人物 / 关键词）
- 支持 topic 详情、相关标签、相关文章
- 支持按标签反查相关 digest
- 支持底部 AI analysis 面板与历史面板
- 支持文章预览，且复用 `ArticleContentView`
- 支持标签层级视图和抽象标签浏览
- 支持标签质量评分和低质量标签过滤
- 支持叙事面板，查看每日叙事摘要和 Board 时间线
- 支持叙事分类版块切换，按分类查看 Board 列表
- 支持板块概念管理（创建/编辑/停用/LLM 建议）
- 支持 Board 可视化画布（NarrativeBoardCanvas），区分概念板和热点板

### 数据链路特点

- 图谱主体、热点分类、topic detail 是分开拉取的
- 点击热点标签后，时间线会优先切到反查 digest 结果
- 底部 analysis 面板会按 topic 类型自动加载对应 analysis
- 叙事面板通过 `/api/narratives/boards/timeline` 和 `/api/narratives/scopes` 拉取 Board 时间线和分类列表
- Board 概念管理通过 `/api/narratives/board-concepts` CRUD API 操作
- 分类版块切换时调用 `loadScopes()` 加载分类列表，展示 `board_count`

更完整的组件分层和数据流说明见 `docs/guides/topic-graph.md`。

## 视觉与交互约束

前端当前不是标准 SaaS 模板风格，文档和实现要保持一致。

- 主色为 Ink Blue，不用紫色
- 强调色为 Print Red
- 背景带纸张质感和渐变
- 交互动效以短促、克制为主
- Digest 页面允许更强烈的版式设计

## 当前已落地但容易忽略的点

- `apiStore` 是主数据源，不要再写副本同步逻辑
- `feedsStore`、`articlesStore` 是派生视图
- WebSocket 只用于 AI 总结队列进度
- Digest 详情里的文章弹窗直接复用主阅读组件
- 文章内容源切换和 Firecrawl 状态已经进主阅读链路
- 文章标签展示已进入主阅读链路和 Topic Graph 文章预览
- AI Provider 和路由管理通过 `/api/ai/providers` 和 `/api/ai/routes` 端点管理
- 叙事面板支持 global/category 双 scope 切换，分类模式下展示 board 数量
- `BoardConceptManager.vue` 支持手动创建和 LLM 建议两种概念创建方式
- `NarrativeBoardCanvas.client.vue` 通过 `board_concept_id` 区分概念板和热点板渲染样式
- 叙事重新生成改为 `POST /api/narratives/regenerate`（JSON body），替代旧的 DELETE + trigger 模式
- 板块概念 API 客户端在 `front/app/api/boardConcepts.ts`
