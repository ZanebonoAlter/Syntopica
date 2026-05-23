## 1. Phase 1: Span 属性注入 + AICallLog 关联

- [x] 1.1 `Router.Chat` span 注入业务属性：从 `ChatRequest.Metadata["operation"]` 和 `ChatRequest.Capability` 读取并写入 span attribute（`internal/platform/airouter/router.go`）
- [x] 1.2 `Router.Embed` span 注入业务属性：从 `EmbeddingRequest` 读取 capability 写入 span attribute（`internal/platform/airouter/router.go`）
- [x] 1.3 `AICallLog` 模型新增 `TraceID` 字段（`internal/domain/models/ai_models.go`）
- [x] 1.4 `LogCall` 方法在写入时自动从 span context 提取并填入 `TraceID`（`internal/platform/airouter/store.go`）
- [ ] 1.5 验证：启动后端，触发标签清理和文章摄入，确认 `otel_spans` 表中对应的 span records 包含 `ai.operation`、`ai.capability` 属性

## 2. Phase 2: 关键链路 ctx 传递

- [x] 2.1 `runCleanupCycle` 补 `ctx context.Context` 参数，调度器 tick 传入（`internal/jobs/tag_hierarchy_cleanup.go`）
- [x] 2.2 `ExecuteFlatMerge` 补 `ctx` 参数，透传到 LLM 调用（`internal/domain/topicanalysis/tag_cleanup.go`）
- [x] 2.3 `processJob` 补 `ctx` 参数，worker loop 传入（`internal/domain/topicextraction/tag_queue.go`）
- [x] 2.4 `TagArticle` / `tagArticle` 补 `ctx` 参数（`internal/domain/topicextraction/article_tagger.go`）
- [x] 2.5 `findOrCreateTag` 补 `ctx` 参数（`internal/domain/topicextraction/tagger.go`）
- [x] 2.6 所有调用方同步更新：`go build ./...` 验证无编译错误
- [x] 2.7 验证：运行 `go test ./...` 确认现有测试通过

## 3. Phase 3: 父 span + baggage

- [x] 3.1 `runCleanupCycle` 入口创建 `workflow.hierarchy_cleanup.cycle` span + 设置 baggage（`workflow.name`, `workflow.domain`, `workflow.trigger`）
- [x] 3.2 TagJob worker `processJob` 入口创建 `workflow.article_tagging` span + 设置 baggage
- [x] 3.3 `Router.Chat` 从 context baggage 读取并写入 span attribute（`baggage.*` 前缀）
- [x] 3.4 清理函数：在 `scheduler.go` 的 `TraceSchedulerTick` 中保留 ctx（移除 `_ = ctx`），确保子 span 关联到调度器 span
- [ ] 3.5 验证：触发标签清理调度器和文章摄入，在 `/api/traces/timeline` 查看父子 span 拓扑正确

## 4. Go 测试验证

- [x] 4.1 运行 `go test ./internal/platform/airouter/... -v` 确认 Router 测试通过
- [x] 4.2 运行 `go test ./internal/domain/topicextraction/... -v` 确认 tag 相关测试通过
- [x] 4.3 运行 `go test ./internal/domain/topicanalysis/... -v` 确认 topic analysis 测试通过
- [x] 4.4 运行 `go test ./... -v` 全量测试确认无回归
