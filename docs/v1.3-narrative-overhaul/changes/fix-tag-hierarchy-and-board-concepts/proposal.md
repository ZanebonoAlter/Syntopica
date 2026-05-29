## Why

探索发现 3 个互联问题：(1) 板块概念前端调用 `/narratives/board-concepts/suggest` 返回 404，路径与模型均与后端不匹配，bootstrap 从未被触发；(2) `tag_hierarchy_cleanup` 的 Phase 2.5/3/6 相互抵消——每轮 15 个 empty abstract + 6 个 single-child 被清理，393/425 个 event topic 仍是孤儿；(3) LLM 在标签提取、描述生成、聚类判断时从未获得文章日期，导致产出 "2024年特朗普访华" 等错误时序引用。

## What Changes

- 前端 board concept API 路径和模型对齐到后端 `/hierarchy/concepts` 和实际 JSON 字段
- 后端新增 `POST /hierarchy/concepts/suggest` 端点：基于现有 tag+description 让 LLM 建议新板块概念，只返回建议不创建
- 后端简化 `tag_hierarchy_cleanup` scheduler：保留清理（Phase 1-1.7/3/3.5-3.6）和 event 聚类（Phase 2.5），移除模板强制阶段（Phase 3d/4/5/6）；将 Phase 3 关系清理移到 Phase 2 之前，避免清理刚创建的抽象标签
- 后端在 `ExtractionInput`、`articleContext`、聚类 prompt 三处注入 `pub_date`，让 LLM 在标签命名和聚合时获得事件时间上下文
- 清理前端 `NarrativePanel.vue` 中未使用的 `boardConceptsApi` 导入

## Capabilities

### New Capabilities

- `concept-suggest-endpoint`: LLM 驱动的板块概念建议端点，分析未分配概念的 tag 并返回建议列表
- `tag-date-context`: 在标签提取、描述生成、事件聚类三处 LLM prompt 中注入文章 publi_date

### Modified Capabilities

- `board-concept-management`: API 路径从 `/narratives/board-concepts` 改为 `/hierarchy/concepts`，模型字段 `is_active` 改为 `status`
- `tag-hierarchy-quality`: cleanup cycle 移除 Phase 3d/4/5/6（模板合规检查、adopt-narrower、abstract-update、树审查），Phase 3 关系清理移至 Phase 2 之前

## Impact

- **后端**: `concept/handler.go`（+suggest 端点）、`concept/service.go`（+SuggestConcepts）、`jobs/tag_hierarchy_cleanup.go`（-Phase 3d/4/5/6、Phase 3 上移）、`tagging/types.go`（ExtractionInput +PubDate）、`tagging/extractor_enhanced.go`（prompt +日期）、`tagging/tagger.go`（description +日期）、`tagging/tag_clustering.go`（cluster +日期上下文）
- **前端**: `api/boardConcepts.ts`（路径+模型）、`features/topic-graph/components/BoardConceptManager.vue`（模型对齐）、`features/topic-graph/components/NarrativePanel.vue`（清理死 import）
- **LLM 调用**: suggest 端点新增 LLM 调用（tag+description 分析，轻量）；标签提取/描述/聚类各增加日期字段，不增加调用次数
- **数据**: 无 schema 变更，`pub_date` 已存在于 `articles` 表无需新增列

## Engineering Standards

本变更必须遵循 `docs/reference/开发执行规范.md` 的全部规则，包括：
- TDD 铁律：先写失败测试，再写最小实现
- 后端质量门禁：`golangci-lint run ./... && go vet ./... && go test ./... && go build ./...`
- 前端质量门禁：`pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build`
- GitNexus 强制：每个子任务完成后执行 impact + detect_changes
