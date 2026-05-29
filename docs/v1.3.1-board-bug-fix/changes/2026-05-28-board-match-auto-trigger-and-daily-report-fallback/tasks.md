## 1. 数据库迁移：topic_tag_board_labels 新增 downgraded 列

- [x] 1.1 `postgres_migrations.go`: 在 seed 列表中为 `topic_tag_board_labels` 表添加 `downgraded boolean NOT NULL DEFAULT false` 迁移
- [x] 1.2 `backend-go/internal/domain/models/semantic_label.go`: `TopicTagBoardLabel` struct 新增 `Downgraded bool` 字段
- [x] 1.3 验证：`go build ./...`

## 2. 匹配核心：降级标记

- [x] 2.1 `semantic_board_matching.go`: `SemanticBoardMatchResult` 新增 `Downgraded bool` 字段
- [x] 2.2 `semantic_board_matching.go`: `evaluateSemanticBoardMatches` 中 max_sim 规则判断时计算降级状态：在现有 `minHits := min(config.DirectMaxSimMinHits, len(tagAuxiliaries))` 后计算 `downgraded := minHits < config.DirectMaxSimMinHits`，当进入 max_sim 分支时设置 `Downgraded: downgraded`
- [x] 2.3 `semantic_board_matching.go`: `replaceTopicTagBoardLabels` 写入时包含 `Downgraded` 字段
- [x] 2.4 为降级标记补充单测：在 `semantic_board_matching_test.go` 中添加 `TestEvaluateSemanticBoardMatches_DowngradedMark` 测试 N=1 降级和 N≥2 不降级
- [x] 2.5 验证：`go build ./...` 和 `go vet ./...` 和 `go test ./internal/domain/tagging/...`

## 3. 匹配服务单例 + Embedding 完成后自动触发 board 匹配

- [x] 3.1 `semantic_board_matching.go` 或 `tagger.go`: 新增包级单例 `getSemanticBoardMatchingService()`（`sync.Once` + `NewSemanticBoardMatchingService(database.DB)`），与 `getEmbeddingQueueService()` 模式一致
- [x] 3.2 `embedding_queue.go`: `processNext` 在 event keyword embeddings 生成后（L325 之后）、mark completed（L327 之前），对 event tag 调用 `getSemanticBoardMatchingService().MatchTopicTag(ctx, tag.ID)`，失败只 log warning
- [x] 3.3 验证：`go build ./...`

## 4. 日报收集兜底补算

- [x] 4.1 `generator.go`: `collectBoardTags` 在现有查询后增加补算查询——查找指定日期范围内有文章关联、有辅助标签但无 `topic_tag_board_labels` 的 event tag，上限 50 个
- [x] 4.2 `generator.go`: 对补算查到的 tag 调用 `NewSemanticBoardMatchingService(database.DB).MatchTopicTag`，从匹配结果中过滤出匹配到当前 `boardID` 的 tag，合并到返回结果
- [x] 4.3 验证：`go build ./...`

## 5. 匹配详情 API：返回降级信息

- [x] 5.1 `semantic_board_handler.go`: `getTagMatchDetail` 响应新增 `downgraded` 和 `effective_min_hits` 字段
- [x] 5.2 `semantic_board_handler.go`: 从 `topic_tag_board_labels` 读取 `downgraded` 字段返回；计算 `effective_min_hits = min(config.DirectMaxSimMinHits, tagAuxiliaryCount)`
- [x] 5.3 验证：`go build ./...`

## 6. 后端：board articles API 返回 downgraded

- [x] 6.1 `semantic_board_handler.go`: board articles 查询结果中包含 `topic_tag_board_labels.downgraded` 字段
- [x] 6.2 验证：`go build ./...`

## 7. 前端 API 类型更新

- [x] 7.1 `semanticBoards.ts`: `MatchDetailResponse` 接口新增 `downgraded: boolean` 和 `effective_min_hits: number` 字段
- [x] 7.2 `semanticBoards.ts`: `BoardArticleTag` 接口新增 `downgraded: boolean` 字段（从 board articles API 返回）
- [x] 7.3 验证：`pnpm lint`

## 8. 前端：MatchDetailPanel 降级展示

- [x] 8.1 `MatchDetailPanel.vue`: 匹配流程步骤 ④ 中，当 downgraded=true 时显示降级说明文字（"⚠降级匹配（原阈值 X，因仅有 N 个辅助标签降为 M）"）
- [x] 8.2 验证：`pnpm lint`

## 9. 前端：tag chip 降级样式

- [x] 9.1 `TagsPage.vue`: tag chip 对 downgraded=true 的匹配降低视觉权重（更淡边框色、"↓" 后缀标记）
- [x] 9.2 验证：`pnpm lint`

## 10. 全量验证

- [x] 10.1 后端：`go build ./...` ✅ + `go vet ./...` ✅ + `go test ./internal/domain/tagging/...` ✅ + `go test ./internal/domain/daily_report/...` ✅ + `golangci-lint run ./...` ✅（无新增问题）
- [x] 10.2 前端：`pnpm lint` ✅ + `pnpm exec nuxi typecheck` ✅ + `pnpm build` ✅
