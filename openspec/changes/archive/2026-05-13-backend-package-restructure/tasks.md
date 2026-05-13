## 1. 创建 tagging/ 目录骨架

- [x] 1.1 创建 `internal/domain/tagging/` 及子目录 `extraction/`、`analysis/`、`embedding/`、`hierarchy/`、`merge/`、`watched/`
- [x] 1.2 创建各域 model 文件：`feed/model.go`、`article/model.go`、`category/model.go`、`content/model.go`、`preferences/model.go`、`aiadmin/model.go`、`narrative/model.go`（跳过 — 循环依赖，models 保留原位）

## 2. 搬移 topictypes 文件

- [x] 2.1 `git mv` topictypes/*.go → tagging/ 根层
- [x] 2.2 改 package 声明为 `package tagging`
- [x] 2.3 更新全项目 import `topictypes` → `tagging`

## 3. 搬移 topicextraction 文件

- [x] 3.1 `git mv` extractor.go、extractor_enhanced.go、quality_score.go → tagging/extraction/
- [x] 3.2 `git mv` tagger.go、article_tagger.go、tag_cache.go、tag_queue.go、tag_queue_handler.go、description_backfill.go → tagging/ 根层
- [x] 3.3 改 package 声明：extraction/ 子文件 → `package extraction`，根层文件 → `package tagging`
- [x] 3.4 更新全项目 import `topicextraction` → `tagging` 或 `tagging/extraction`

## 4. 拆分 topicanalysis → tagging 子包

- [x] 4.1 搬移 hierarchy/ 文件（abstract_tag_*.go、hierarchy_*.go、queue_batch_processor.go、tree_bridge.go、person_metadata_backfill.go、tag_prompt_context.go）→ tagging/ 根层（循环依赖，无法独立子包）
- [x] 4.2 搬移 embedding/ 文件（embedding.go、embedding_queue.go、embedding_queue_handler.go、merge_reembedding_queue.go、merge_reembedding_queue_handler.go、embedding_config_handler.go、config_service.go）→ tagging/ 根层（循环依赖）
- [x] 4.3 搬移 analysis/ 文件（ai_analysis.go、analysis_handler.go、analysis_queue.go、analysis_service.go）→ tagging/analysis/
- [x] 4.4 搬移 merge/ 文件（tag_cleanup.go、tag_clustering.go、tag_management_handler.go、tag_merge_preview.go、tag_merge_preview_handler.go）→ tagging/ 根层（循环依赖）
- [x] 4.5 搬移 watched/ 文件（watched_tags_handler.go、watched_tags_service.go）→ tagging/watched/
- [x] 4.6 搬移根层共享文件（batch_tag_judgment.go、cotag_expansion.go）→ tagging/ 根层
- [x] 4.7 改所有搬移文件的 package 声明为对应包名
- [x] 4.8 更新全项目 import `topicanalysis` → 对应包路径

## 5. 分散 models/

- [x] 5.1 各域 model 文件写入对应 struct（跳过 — Feed/Article 交叉引用导致循环依赖）
- [x] 5.2 更新全项目所有 `models.X` 引用的 import 路径（不需要 — models 保持原位）
- [x] 5.3 删除 models/ 中已搬走的文件，仅保留 scheduler_task.go（跳过）
- [x] 5.4 可选：删除 models/utils.go（跳过 — FormatDatetimeCST 仍被 models 内部使用）

## 6. 重命名包

- [x] 6.1 `git mv feeds/ → feed/`、`articles/ → article/`、`categories/ → category/`、`contentprocessing/ → content/`
- [x] 6.2 改各包的 package 声明
- [x] 6.3 更新全项目 import 路径（feeds→feed、articles→article、categories→category、contentprocessing→content）

## 7. 更新 app/ 和 jobs/ 入口

- [x] 7.1 更新 `router.go` 所有 import 路径
- [x] 7.2 更新 `runtime.go`：import `tagging`，调用 `tagging.StartAllWorkers()` / `tagging.StopAllWorkers()`
- [x] 7.3 创建 `tagging/workers.go`（StartAllWorkers / StopAllWorkers 代理函数）
- [x] 7.4 更新 `jobs/*.go` 所有 import 路径
- [x] 7.5 更新 `platform/database/migrator.go` import 路径

## 8. 编译修复 + 测试

- [x] 8.1 `go build ./...` 修复所有编译错误（跨包 unexported → exported、缺失 import 等）
- [x] 8.2 `go vet ./...` 无报错
- [x] 8.3 `golangci-lint run ./...` 确认无 lint 报错

## 9. 更新文档

- [ ] 9.1 更新 `docs/reference/architecture/backend.md` 目录结构和包名
- [ ] 9.2 更新 `docs/reference/architecture/overview.md` 目录结构
- [ ] 9.3 更新 `backend-go/AGENTS.md` 命令和路径引用
- [ ] 9.4 更新根 `AGENTS.md` 中 backend 相关路径
