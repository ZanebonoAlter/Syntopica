<!-- domain: dev -->

# api-docs

## Purpose

TBD: This spec governs the accuracy and maintenance of API documentation files (`docs/reference/api/*.md`) relative to the actual backend codebase.

## Requirements

### Requirement: schedulers.md SHALL list all registered schedulers

The `api/schedulers.md` document SHALL list exactly the schedulers registered in `backend-go/internal/app/runtime.go`, each with name, alias (if any), and description.

#### Scenario: Scheduler list matches runtime.go
- **WHEN** a developer reads the "支持的调度器" section
- **THEN** the table SHALL include exactly these 9 schedulers (matching the `registry.Register` calls in `runtime.go`):
  - `auto_refresh` - 自动刷新 RSS
  - `preference_update` - 更新阅读偏好
  - `content_completion`（别名 `ai_summary`）- 文章内容补全
  - `firecrawl` - Firecrawl 全文抓取
  - `blocked_article_recovery` - 恢复阻塞文章
  - `daily_report` - 生成语义板日报
  - `tag_quality_score` - 重算标签质量分数
  - `log_cleanup` - 清理过期日志与追踪数据
  - `aux_label_cleanup` - 清理无活跃引用的辅助标签

#### Scenario: BlockedArticleRecovery is visible
- **WHEN** a developer queries `GET /api/schedulers/status`
- **THEN** the response SHALL include `blocked_article_recovery`
- **AND** the scheduler SHALL be triggerable via `POST /api/schedulers/blocked_article_recovery/trigger`

### Requirement: schedulers.md SHALL NOT list phantom schedulers

The `api/schedulers.md` document SHALL NOT list schedulers that are not registered in `runtime.go`.

#### Scenario: Phantom schedulers are removed
- **WHEN** a developer reads the "支持的调度器" section
- **THEN** the table SHALL NOT include `digest`, `narrative_summary`, or `tag_hierarchy_cleanup` (none are registered in `runtime.go`)
