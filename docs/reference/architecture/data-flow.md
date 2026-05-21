# 数据流

> **互补阅读**：[数据生命周期](../database/DATA_LIFECYCLE.md) 从数据状态字段变迁角度描述同样的核心链路——哪些表被写入、状态字段怎么流转、数据产出依赖。本文档侧重代码执行流。

## 主链路

```text
RSS 源
  -> backend-go 拉取和解析
  -> PostgreSQL 持久化
  -> 可选全文抓取 / 内容补全 / AI 总结 / Digest 聚合
  -> 可选主题标签 embedding 向量化 / 自动合并 / 叙事摘要
  -> 前端通过 app/api 拉取
  -> apiStore 映射为前端模型
  -> 派生 store 和 feature 组件消费
  -> UI 渲染
```

## 前端数据流

```text
page
  -> feature shell / feature view
  -> app/api/*
  -> backend API
  -> useApiStore
  -> useFeedsStore / useArticlesStore / usePreferencesStore
  -> 组件渲染
```

## 前端状态职责

### `useApiStore`

主数据源。

- 拉分类
- 拉 feed
- 拉文章
- 执行分类、feed、文章相关 CRUD
- 处理 OPML 导入导出
- 处理 AI 总结接口
- 初始化应用启动数据

### `useFeedsStore`

派生订阅视图。

- feed 分组
- 分类视图
- feed 未读数

### `useArticlesStore`

派生文章视图。

- 当前筛选条件
- 当前文章
- 已读 / 收藏统计
- 文章列表排序与过滤

### `usePreferencesStore`

阅读偏好相关状态。

- 读取偏好分数
- 读取阅读统计
- 手动触发偏好更新

## 字段映射规则

- 后端响应保留 `snake_case`
- 前端内部统一 `camelCase`
- 前端的 `id` 统一转成 `string`
- 转换集中在 API 模块和 `useApiStore`
- 组件层不应散落字段映射逻辑

## 主阅读页交互流

### 应用启动

```text
app.vue mounted
  -> apiStore.initialize()
  -> Promise.all(fetchCategories, fetchFeeds, fetchArticles)
  -> FeedLayoutShell 渲染
```

### 切分类

```text
AppSidebar
  -> FeedLayoutShell.handleCategoryClick()
  -> apiStore.fetchFeeds(...)
  -> apiStore.fetchArticles(...)
  -> 列表栏和正文区响应更新
```

### 切 feed

```text
AppSidebar
  -> FeedLayoutShell.handleFeedClick()
  -> apiStore.fetchArticles(feed_id)
  -> apiStore.refreshFeed(feed_id)
  -> 轮询 refresh_status
  -> 刷新完成后再拉文章
```

### 打开文章

```text
ArticleListPanel
  -> ArticleContentView
  -> apiStore.markAsRead()
  -> useReadingTracker 记录 open / scroll / close / favorite
  -> reading_behavior 接口批量上报
```

## 文章内容增强流

### Firecrawl / 内容补全状态

```text
ArticleContentView
  -> useContentCompletion.getCompletionStatus(articleId)
  -> /content-completion/articles/:id/status
  -> UI 展示抓取状态、整理状态、错误信息
```

### 手动抓取全文

```text
ArticleContentView
  -> useFirecrawlApi.crawlArticle(articleId)
  -> 后端执行抓取
  -> 再次查询 completion status
  -> 更新 article.firecrawlContent / firecrawlStatus / summaryStatus
```

### 手动生成整理稿

```text
ArticleContentView
  -> completeArticle(articleId, { force: true })
  -> 后端生成 ai_content_summary
  -> 更新 summary_status / summary_generated_at
  -> 再次查询 completion status
  -> UI 渲染整理稿
```

## AI 总结流

```text
AISummariesListView
  -> apiStore.submitQueueSummary()
  -> backend 创建批次任务
  -> useSummaryWebSocket.connect()
  -> /ws 推送进度
  -> 批次完成后 fetchSummaries()
  -> 右栏显示 AISummaryDetailView
```

## Digest 流

```text
DigestListView
  -> getStatus()
  -> getPreview(daily|weekly, date)
  -> 左栏分类 + 中栏 summary 列表 + 右栏详情
  -> runNow() 可立即生成新版本
  -> DigestDetail 按 article_ids 拉关联文章
  -> 关联文章在弹窗中复用 ArticleContentView
```

## 定时任务链路

- feed 自动刷新
- Firecrawl / 内容补全处理
- AI 总结批量生成
- Digest 日报 / 周报生成
- 阅读偏好聚合任务
- 阻塞文章恢复
- 标签自动合并（源 DELETE，不再用 status='merged'）
- 标签质量分数重算
- 叙事摘要生成（双轨制：热点板 + Sector 匹配）
- 叙事后处理（Board 连接派生、标签反馈、空 Board 清理）
- 层级清理（7 Phase: 僵尸 Tag → 低质量 → 空 Node → 同 Level 去重 → Template 校验 → Sector 健康 → 聚类信号）
- Sector 生成（auto/LLM/manual 三模式）
- 重建任务（模板变更触发 rebuild_jobs）
- 关注标签叙事维度总结

### scheduler 状态回传

```text
GlobalSettingsDialog.schedulers tab
  -> useSchedulerApi.getSchedulersStatus()
  -> /api/schedulers/status
  -> backend 返回 database_state + last_run_summary + is_executing
  -> UI 渲染 auto_refresh / auto_summary / ai_summary / firecrawl 状态卡
```

### 手动 trigger 链路

```text
GlobalSettingsDialog.schedulers tab
  -> useSchedulerApi.triggerScheduler(name)
  -> POST /api/schedulers/:name/trigger
  -> backend 判断 accepted / started / reason / message
  -> 前端显示真实反馈，不再只看 HTTP 200
  -> 短周期轮询刷新最新状态
```

### `auto_refresh` 状态流

```text
auto_refresh scheduler
  -> 扫描 refresh_interval > 0 的 feed
  -> 判断是否到点
  -> 标记 feed.refresh_status=refreshing
  -> 异步调用 feedService.RefreshFeed()
  -> 把扫描数 / 到点数 / 触发数 / 已在刷新数写回 scheduler_tasks.last_execution_result
```

### `auto_summary` 状态流

```text
auto_summary scheduler
  -> 读取 AI 配置
  -> 扫描 ai_summary_enabled=true 的 feed
  -> 聚合近 time_range 内文章
  -> 调 AI 生成 summary
  -> 把 feed 数 / 生成数 / 跳过数 / 失败数写回 scheduler_tasks.last_execution_result
  -> 手动 trigger 时也走同一套执行链路
```

## 叙事数据流

### 每日叙事生成

```text
NarrativeSummaryScheduler 触发
  → GenerateAndSave(date)
    → GenerateAndSaveForAllCategories
      → 逐分类双轨生成:
        Pass 1: CollectAbstractTreeInputs
          → 大树(≥6) → 热点板 (is_system=true)
          → 小树 → MatchTagToConcept → 概念板或未归类
        Pass 2: CollectUnclassifiedEventTags
          → MatchTagToConcept → 概念板或未归类
    → GenerateAndSaveGlobal
      → CollectTagInputs → MatchTagToConcept → 概念板
    → runFallbackAssociations (关联前日叙事)
    → DeriveBoardConnections (派生 Board 连接)
    → runFeedbackFromTodayNarratives (反馈标签)
    → cleanEmptyBoards (清理空 Board)
```

### Board Concept 管理

```text
BoardConceptManager → SectorGenerationService
  → auto 模式: unplaced Tag > 阈值 → LLM 提议 → 0.85 去重 → 创建 Sector + embedding
  → LLM 模式:  用户触发 → LLM 增量建议 (keep/add/merge/split) → diff 预览 → 确认执行
  → manual 模式: 用户输入 label → LLM 补全 description → 创建 Sector + protected
  → 日常: MatchTagToConcept 使用 embedding cosine similarity 匹配
    → event 标签: 加权平均 (title×2.0 + keyword×1.0)
    → 其他标签: 单 embedding 余弦相似度
  → Sector 健康检查: auto 空→DELETE, LLM 衰退→declining, manual 不动
```

### 概念 Bootstrap 流程

```text
BootstrapConcepts(category)
  → 加载分类内 active 标签 + semantic embedding
  → 总标签 < 10 → 跳过
  → buildNeighborGraph → findConnectedComponents (pgvector distance < 0.65)
  → 过滤: 簇 < 5 标签 → 归入默认概念
  → 有效簇: LLM 命名 → 创建 pending 概念 → 用户审阅激活
  → 默认概念: 按分类命名 (event="事件", keyword="关键词", person="人物") → active + 生成 embedding
```

### 层级清理流程

```text
TagHierarchyCleanup 调度器 (3600s, time budget 限制)
  Phase 1: 僵尸 Tag — DELETE 无文章/无关系/age>7d
  Phase 2: 低质量 Tag — DELETE quality<0.15 且 article_count=1
  Phase 3: 空 Node — DELETE 无子节点 Node (source='abstract')
  Phase 4: 同 Level 去重 — 同 Sector 同 Level Node 相似>0.90 → HardMergeTags
  Phase 5: Template 校验 — depth/leaf 位置/children 超限 → hierarchy_pending_changes
  Phase 6: Sector 健康检查 — auto 空→DELETE, LLM 衰退→declining, manual 不动
  Phase 7: 聚类信号 — GenerateAnchorSignals 持久化 hierarchy_anchor_signals，供 PlaceTagInHierarchy 消费
```

### 标签层级闭环状态机

```text
/tags 页面
  → GET /api/hierarchy/closure-status?category=event
  → 展示 active_sector_count / unplaced_tag_count / pending_change_count / active_rebuild_job / blocker_counts
  → no_active_sector 且 unplaced 超阈值: orchestration bootstrap 触发 AutoGenerateSectors
  → 有 Sector: PlaceTagInHierarchy 尝试链接现有 Node 或创建合格 Node
  → 无法放置: 返回 blocker reason，closure status 聚合展示
  → PendingChange 审批: 按 change_type 执行明确 relation 操作，缺 payload 标记 failed
```

已知限制：`HierarchyPendingChange` 当前没有独立 payload 字段，因此 `move` / `reparent` / `create` / `delete` 类型无法安全执行，只能返回 failed + reason。后续若要支持这些类型，需要先扩展可审计 payload，再接入明确执行器。

### 重建任务流程

```text
模板变更触发:
  → preview: POST /api/hierarchy/config/preview 只返回 impact，不保存、不删除、不创建 rebuild_job
  → apply: PUT /api/hierarchy/config with apply=true 保存新 template 到 hierarchy_config
  → DELETE 旧 Node (source='abstract') + relations + embeddings
  → 叶 Tag (source='llm'/'heuristic') 保留, concept_id 不变
  → 创建 rebuild_job (status='pending')

RebuildService 执行:
  → SELECT Tags WHERE id > last_tag_id LIMIT batch_size (默认 20)
  → PlaceTagInHierarchy 逐个处理
  → 更新 processed_tags / last_tag_id / estimated_end
  → batch 间 sleep (默认 1s) 限流
  → WebSocket 推送 hierarchy_rebuild (status=processing/completed/failed)
  → 断点续传: 启动时检测 status='running' → 设为 paused
```

### 叙事面板数据流

```text
NarrativePanel
  → loadBoardTimeline(date) → GET /api/narratives/boards/timeline
  → loadScopes(date) → GET /api/narratives/scopes
  → loadNarratives(date) → GET /api/narratives?date=...
  → switchScope('category') → loadScopes → 展示 board_count
  → triggerGeneration() → POST /api/narratives/regenerate
```

## 约束

- 不再维护本地镜像数组同步链
- 不再使用 `syncToLocalStores()`
- 组件层优先消费已映射好的前端模型
- 与后端交互的细节只应停留在 `app/api` 和 store
