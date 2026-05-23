## 1. 前端：Board Concepts 路径和模型对齐

- [x] 1.1 `api/boardConcepts.ts`：所有 API 路径从 `/narratives/board-concepts` 改为 `/hierarchy/concepts`，`BoardConcept` 接口移除 `is_active: boolean` 新增 `status: string` 和 `category: string`
- [x] 1.2 `BoardConceptManager.vue`：同步 `BoardConcept` 接口变更，确认模板无 `is_active` 引用
- [x] 1.3 `NarrativePanel.vue`：移除未使用的 `useBoardConceptsApi` import 和 `boardConceptsApi` 变量声明
- [x] 1.4 前端质量门禁：`pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build`

## 2. 后端：Concept Suggest 端点

- [x] 2.1 `concept/suggest.go`（新文件）：实现 `SuggestConcepts(ctx, category)` — 加载该类别下未匹配 concept 的 active tags（≤50 条 label+description），调用 LLM 返回 3-5 个 `{name, description}` 建议（JSON mode），LLM 失败时返回空列表
- [x] 2.2 `concept/handler.go`：新增 `suggestConceptsHandler`，注册 `POST /hierarchy/concepts/suggest` 路由
- [x] 2.3 `concept/suggest_test.go`：测试有效类别返回建议、空类别返回空列表、LLM 失败优雅处理
- [x] 2.4 后端质量门禁：`golangci-lint run ./... && go vet ./... && go test ./... && go build ./...`

## 3. 后端：Cleanup Cycle 简化

- [x] 3.1 `jobs/tag_hierarchy_cleanup.go`：移除 Phase 3d（`CleanupTemplateViolations`）、Phase 4（`ProcessPendingAdoptNarrowerTasks`）、Phase 5（`ProcessPendingAbstractTagUpdateTasks`）、Phase 6（模板树审查 block），将 Phase 3 关系清理 block 移到 Phase 2 之前
- [x] 3.2 `jobs/tag_hierarchy_cleanup.go`：更新 `TagHierarchyCleanupRunSummary` 移除 `TemplateDepthViolations`、`TemplateCrossCategory`、`AdoptNarrowerProcessed`、`AbstractUpdateProcessed`、`TreesReviewed`、`MovesApplied`、`GroupsCreated`、`GroupsReused` 字段，更新 `Reason` 摘要字符串
- [x] 3.3 `jobs/tag_hierarchy_cleanup.go`：更新 `registerOrSyncTask` 中的 description 字符串，移除 "adopt narrower"、"abstract update"、"tree review"
- [x] 3.4 验证：确认 `scheduler_tasks` 表中 `tag_hierarchy_cleanup` 的 description 已自动更新为简化版
- [x] 3.5 后端质量门禁
- [x] 4.1 `tagging/types.go`：`ExtractionInput` 新增 `PubDate string` 字段
- [x] 4.2 `tagging/extractor_enhanced.go`：`buildExtractionUserPrompt` 新增 `发布日期: %s` 行（`PubDate` 非空时）
- [x] 4.3 `tagging/article_tagger.go`：`articleContext` 构建时 prepend `[日期: YYYY-MM-DD]`（`article.PubDate` 非空时）
- [x] 4.4 `tagging/tag_clustering.go`：`formatTagPromptContext` 或聚类候选构建逻辑中，为每个 tag 附加其关联文章的日期范围（查询 `articles.pub_date` via `article_topic_tags`）
- [x] 4.5 编写测试：`ExtractionInput` 含 PubDate 时 prompt 包含日期行、空 PubDate 时不包含；articleContext 含日期前缀；聚类候选含日期范围
- [x] 4.6 后端质量门禁

## 5. 验证

- [ ] 5.1 启动后端，`curl -X POST http://localhost:5000/api/hierarchy/concepts/suggest -H 'Content-Type: application/json' -d '{"category":"event"}'` 验证返回建议列表
- [ ] 5.2 启动前端，确认 BoardConceptManager 的 "LLM 建议" 按钮不再 404
- [ ] 5.3 手动触发一次 cleanup cycle，观察日志确认 Phase 顺序正确且无 Phase 3d/4/5/6 执行
- [ ] 5.4 查询数据库确认 event tag 孤儿数量在下一次 cleanup 后下降
- [x] 5.5 全栈质量门禁：前后端各自通过门禁
