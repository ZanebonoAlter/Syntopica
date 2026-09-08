# API 文档索引

> 通用约定（响应格式、分页、WebSocket）见 [_conventions.md](_conventions.md)

| 文件 | 领域 | 路由前缀 |
| ------ | ------ | ---------- |
| [system.md](system.md) | 系统信息、健康检查、全局任务 | `/`, `/health`, `/api/tasks/status` |
| [categories.md](categories.md) | 分类 CRUD | `/api/categories` |
| [feeds.md](feeds.md) | 订阅 CRUD、刷新 | `/api/feeds` |
| [articles.md](articles.md) | 文章列表、详情、状态 | `/api/articles` |
| [ai-admin.md](ai-admin.md) | AI 设置、Provider、Route | `/api/ai` |
| [ai-call-logs.md](ai-call-logs.md) | AI 调用日志 | `/api/ai/call-logs` |
| [ai-sessions.md](ai-sessions.md) | 编排 session 聚合（业务日志 + 链路时间线） | `/api/ai/sessions` |
| [opml.md](opml.md) | OPML 导入导出 | `/api/import-opml`, `/api/export-opml` |
| [schedulers.md](schedulers.md) | 定时任务管理 | `/api/schedulers` |
| [content-completion.md](content-completion.md) | 文章内容补全 | `/api/content-completion` |
| [firecrawl.md](firecrawl.md) | Firecrawl 全文抓取 | `/api/firecrawl` |
| [reading.md](reading.md) | 阅读行为上报与统计 | `/api/reading-behavior` |
| [preference-profile.md](preference-profile.md) | 偏好向量画像（按版块聚合的兴趣画像） | `/api/preference-profile` |
| [discovery.md](discovery.md) | RSSHub 路由目录同步、订阅源推荐卡片、问答发现、RSSHub 实例配置 | `/api/discovery`, `/api/settings/rsshub` |
| [semantic-boards.md](semantic-boards.md) | SemanticBoard、辅助标签、升级建议、匹配回填 | `/api/semantic-boards`, `/api/auxiliary-labels`, `/api/tags/:id/semantic-boards` |
| [daily-reports.md](daily-reports.md) | 板块日报生成、列表、详情、WebSocket 进度 | `/api/daily-reports`, `/api/semantic-boards/:id/daily-reports` |
| [dataenrichment.md](dataenrichment.md) | 话题数据增强（生命周期上下文/增强结果/评审/辩论/数据源绑定 + 板块简报/调查/QA + 跨版块关系发现/裁决 + 分析方法卡） | `/api/persistent-topics/:topicId/enrichment`, `/api/semantic-boards/:id/{data-sources,enrichment/analysis}, /api/semantic-boards/:id/enrichment/analysis/relations`, `/api/enrichment/analysis-status`, `/api/analysis-methods`, `/api/reference-roles`（只读） |
| [tag-ops.md](tag-ops.md) | 标签队列与标签运维（打标队列、嵌入队列、合并再嵌入、标签搜索/合并/关注） | `/api/embedding/queue`, `/api/embedding/merge-reembedding`, `/api/tag-queue`, `/api/topic-tags` |
| [traces.md](traces.md) | 链路追踪 | `/api/traces` |
