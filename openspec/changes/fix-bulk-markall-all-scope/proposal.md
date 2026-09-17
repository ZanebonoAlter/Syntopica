<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: reading -->

## Why

「全部文章」视图点 header 的「全部标为已读」必现 400：前端 `handleMarkAllRead` 在无选中 feed/category 时走无 scope 分支，而后端 `PUT /api/articles/bulk-update`（2026-06 起硬化）要求 `ids/feed_id/category_id/uncategorized` 至少一个。存量 bug（非近期回归），用户实测复现（2026-09-18）。

## What Changes

- **后端**：bulk-update 增加 `all: true` 显式 scope——全站标记仅接受显式 `all`（不放松误操作保护：无 scope 且无 all 仍 400）；`all` 只与 `read/favorite` 字段更新组合生效。
- **前端**：`markAllAsRead()` 无 options 分支传 `all: true`；`BulkUpdateArticlesData` 类型加 `all?` 字段。
- **Bug 修复纪律**：先写 handler 复现测试（无 scope → 400 现状断言）再修，修后转绿（§2 不可豁免）。

## Capabilities

### New Capabilities
- `article-bulk-update`: 文章批量操作（bulk-update）的 scope 契约——五种 scope（all/ids/feed_id/category_id/uncategorized）语义、校验顺序、至少一字段（read/favorite）约束。

### Modified Capabilities

## Impact

- 后端 `internal/reader/handler/article_handler.go`（bulk 校验分支 + 新 all 分支）；前端 `front/app/stores/articles.ts`（无 options 分支传 all）、`front/app/types`（BulkUpdateArticlesData）。
- 无数据模型/迁移变更；无 UI 结构变更（仅让既有按钮按既有语义工作）。
