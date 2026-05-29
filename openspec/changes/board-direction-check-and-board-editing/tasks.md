## 1. DB Migration: direction_mismatch 列

- [ ] 1.1 `postgres_migrations.go`: 新增迁移 `topic_tag_board_labels` 添加 `direction_mismatch BOOLEAN NOT NULL DEFAULT false`
- [ ] 1.2 `backend-go/internal/domain/models/semantic_label.go`: `TopicTagBoardLabel` 新增 `DirectionMismatch bool` 字段
- [ ] 1.3 验证：`go build ./...`

## 2. Bug Fix: LLM升级建议创建板块时生成 embedding

- [ ] 2.1 `semantic_board_upgrade.go`: 将 `semanticBoardLabelEmbedder` 注入 `SemanticBoardUpgradeService`（新增字段 + 构造函数参数）
- [ ] 2.2 `semantic_board_upgrade.go`: Confirm 中 create_new 分支调用 embedder 生成 embedding（输入 `label + ". " + description`），写入 `SemanticLabel.Embedding`
- [ ] 2.3 更新 `NewSemanticBoardUpgradeService` 所有调用点（检查 router.go 或 runtime.go 中的初始化）
- [ ] 2.4 验证：`go build ./...`

## 3. Board embedding backfill + description 变更刷新

- [ ] 3.1 `semantic_board_handler.go`: updateBoard handler 中 description 变更时也调用 `semanticBoardLabelEmbedder(label + ". " + description)` 生成 embedding（统一 embedding 输入为 `label + ". " + description`）
- [ ] 3.2 `semantic_board_handler.go`: 新增 `POST /api/semantic-boards/backfill-embeddings` handler，查询所有 `embedding IS NULL AND label_type='board'` 的板块，逐一生成 embedding
- [ ] 3.3 `router.go`: 注册新路由
- [ ] 3.4 验证：`go build ./...`

- [ ] 3.5 `semantic_board_handler.go`: 新增 `POST /api/semantic-boards/rematch-all` handler，查询所有 `topic_tag_board_labels` 中有记录的 tag，逐个调用 `MatchTopicTag` 重新匹配（用于 backfill 后刷新 direction_mismatch 标记）
- [ ] 3.6 `router.go`: 注册 rematch-all 路由
- [ ] 3.7 验证：`go build ./...`

## 3b. 数据验证: direction_sim 阈值校准

- [ ] 3b.1 手动调用 backfill-embeddings + rematch-all 后，查询所有 max_sim 匹配的 direction_sim 分布（min, P10, median, P90, max），确认阈值 0.5 是否合理
- [ ] 3b.2 如阈值需调整，通过 matching-config API 更新 `direction_sim_threshold`

## 4. Matching Core: 方向性校验

- [ ] 4.1 `semantic_board_matching.go`: `SemanticBoardMatchConfig` 新增 `DirectionSimThreshold float64`
- [ ] 4.2 `semantic_board_matching.go`: `SemanticBoardMatchResult` 新增 `DirectionMismatch bool`
- [ ] 4.3 `semantic_board_matching.go`: `evaluateSemanticBoardMatches` 签名新增 `tagEmbedding []float64` 和 `boardEmbeddings map[uint][]float64` 参数
- [ ] 4.4 `evaluateSemanticBoardMatches` max_sim 成功后计算 direction check：`cosine(tagEmbedding, boardEmbeddings[boardID]) < config.DirectionSimThreshold → directionMismatch=true`
- [ ] 4.5 `replaceTopicTagBoardLabels` 写入 `DirectionMismatch` 字段
- [ ] 4.6 `loadConfig` 新增 `semantic_board_match_direction_sim_threshold` 读取
- [ ] 4.7 为方向校验补充单测：`TestEvaluateSemanticBoardMatches_DirectionCheck`
- [ ] 4.8 验证：`go build ./...` + `go test ./internal/domain/tagging/...`

## 5. MatchTopicTag 加载方向数据

- [ ] 5.1 `semantic_board_matching.go`: MatchTopicTag 新增加载 tag identity embedding（`topic_tag_embeddings WHERE topic_tag_id=X AND embedding_type='identity'`）
- [ ] 5.2 `semantic_board_matching.go`: MatchTopicTag 新增加载所有活跃 board embedding（`semantic_labels WHERE label_type='board' AND status='active' AND embedding IS NOT NULL`）
- [ ] 5.3 将 tagEmbedding 和 boardEmbeddings 传入 evaluateSemanticBoardMatches
- [ ] 5.4 验证：`go build ./...`

## 6. 后端 API: direction_sim + filtering

- [ ] 6.1 `semantic_board_handler.go`: `matchDetailResponse` 新增 `DirectionSim *float64` 字段
- [ ] 6.2 `semantic_board_handler.go`: `getTagMatchDetail` 加载 tag identity embedding 和 board embedding，计算 direction_sim 写入响应
- [ ] 6.3 `semantic_board_handler.go`: `boardArticleTagDTO` 新增 `DirectionMismatch bool`
- [ ] 6.4 `semantic_board_handler.go`: `filteredTagRow` 新增 `DirectionMismatch bool`
- [ ] 6.5 `semantic_board_handler.go`: `getBoardArticles` 的 filtered_tags 查询新增 `tbl.direction_mismatch` 字段，默认排除 `direction_mismatch=true`，支持 `?show_direction_mismatch=true` 参数
- [ ] 6.6 `semantic_board_handler.go`: matching config handler 新增 `direction_sim_threshold` 参数支持
- [ ] 6.7 验证：`go build ./...`

## 7. 日报排除 direction_mismatch

- [ ] 7.1 `generator.go`: collectBoardTags 主查询添加 `AND NOT COALESCE(topic_tag_board_labels.direction_mismatch, false)` 条件
- [ ] 7.2 `generator.go`: fallback 补算结果中排除 `DirectionMismatch=true` 的匹配
- [ ] 7.3 验证：`go build ./...`

## 8. 前端 API 类型 + 板块编辑 UI

- [ ] 8.1 `semanticBoards.ts`: `BoardArticleTag` 新增 `direction_mismatch: boolean`
- [ ] 8.2 `semanticBoards.ts`: `MatchDetailResponse` 新增 `direction_sim: number | null`
- [ ] 8.3 `TagsPage.vue`: 板块列表每项新增编辑按钮
- [ ] 8.4 `TagsPage.vue`: 新增编辑对话框组件（inline 或单独文件），支持修改 label 和 description
- [ ] 8.5 `TagsPage.vue`: 保存调用 `updateBoard` API，成功后刷新板块列表
- [ ] 8.6 验证：`pnpm lint`

## 9. 前端: direction_mismatch 展示控制

- [ ] 9.1 `TagsPage.vue`: 新增 `showDirectionMismatch` ref (默认 false)
- [ ] 9.2 `TagsPage.vue`: computed 过滤 filtered_tags，默认排除 `direction_mismatch=true`；toggle 开启后显示
- [ ] 9.3 `TagsPage.vue`: direction_mismatch 标签样式——虚线边框 + "⊘" 后缀
- [ ] 9.4 `TagsPage.vue`: 新增"显示方向不符"toggle UI 元素
- [ ] 9.5 验证：`pnpm lint`

## 10. 前端: MatchDetailPanel 方向校验展示

- [ ] 10.1 `MatchDetailPanel.vue`: 步骤 ④ 展示方向校验结果（direction_sim 值 + 是否通过阈值）
- [ ] 10.2 验证：`pnpm lint`

## 11. 全量验证

- [ ] 11.1 后端：`go build ./...` + `go vet ./...` + `go test ./internal/domain/tagging/... ./internal/domain/daily_report/...` + `golangci-lint run ./...`
- [ ] 11.2 前端：`pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build`
- [ ] 11.3 端到端验证：调用 backfill-embeddings → rematch-all → 确认日报排除 direction_mismatch 标签 → 前端 toggle 显示/隐藏方向不符标签正常工作
