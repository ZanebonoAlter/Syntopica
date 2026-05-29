# API 文档索引

> 通用约定（响应格式、分页、WebSocket）见 [_conventions.md](_conventions.md)

| 文件 | 领域 | 路由前缀 |
|------|------|----------|
| [system.md](system.md) | 系统信息、健康检查、全局任务 | `/`, `/health`, `/api/tasks/status` |
| [categories.md](categories.md) | 分类 CRUD | `/api/categories` |
| [feeds.md](feeds.md) | 订阅 CRUD、刷新 | `/api/feeds` |
| [articles.md](articles.md) | 文章列表、详情、状态 | `/api/articles` |
| [summaries.md](summaries.md) | AI 摘要、队列任务、自动总结配置 | `/api/summaries`, `/api/auto-summary` |
| [ai-admin.md](ai-admin.md) | AI 设置、Provider、Route | `/api/ai` |
| [opml.md](opml.md) | OPML 导入导出 | `/api/import-opml`, `/api/export-opml` |
| [schedulers.md](schedulers.md) | 定时任务管理 | `/api/schedulers` |
| [content-completion.md](content-completion.md) | 文章内容补全 | `/api/content-completion` |
| [firecrawl.md](firecrawl.md) | Firecrawl 全文抓取 | `/api/firecrawl` |
| [reading.md](reading.md) | 阅读行为、用户偏好 | `/api/reading-behavior`, `/api/user-preferences` |
| [topic-graph.md](topic-graph.md) | 主题图谱、主题分析、标签管理、Embedding | `/api/topic-graph`, `/api/topic-tags`, `/api/embedding` |
| [semantic-boards.md](semantic-boards.md) | SemanticBoard、辅助标签、升级建议、匹配回填 | `/api/semantic-boards`, `/api/auxiliary-labels`, `/api/tags/:id/semantic-boards` |
| [daily-reports.md](daily-reports.md) | 板块日报生成、列表、详情、WebSocket 进度 | `/api/daily-reports`, `/api/semantic-boards/:id/daily-reports` |
| [traces.md](traces.md) | 链路追踪 | `/api/traces` |
