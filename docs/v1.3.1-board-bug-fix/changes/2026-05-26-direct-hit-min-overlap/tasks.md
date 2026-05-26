## 1. 匹配核心：direct_hit 最小交集数

- [x] 1.1 `semantic_board_matching.go`: `SemanticBoardMatchConfig` 新增 `DirectHitMinOverlap int` 字段，默认 2
- [x] 1.2 `semantic_board_matching.go`: `hasDirectSemanticBoardHit` 重命名为 `countDirectSemanticBoardHits`，返回交集数（int）而非 bool
- [x] 1.3 `semantic_board_matching.go`: `evaluateSemanticBoardMatches` 中 direct_hit 分支改为 `count >= config.DirectHitMinOverlap`，不足时 fallthrough 到相似度匹配
- [x] 1.4 `semantic_board_matching.go`: `loadConfig` 新增读取 `semantic_board_match_direct_hit_min_overlap` 配置项
- [x] 1.5 验证：`go build ./...` 和 `go vet ./...`

## 2. 匹配详情 API：direct_hit 场景展示完整 pairs

- [x] 2.1 `semantic_board_handler.go`: `getTagMatchDetail` 在 direct_hit 场景下也调用 `computeMatchDetail` 计算 pairs/hits/hitRate/maxSimilarity
- [x] 2.2 `semantic_board_handler.go`: `matchDetailConfigDTO` 新增 `DirectHitMinOverlap` 字段
- [x] 2.3 `semantic_board_handler.go`: `matchDetailConfigToDTO` 补充新字段映射
- [x] 2.4 验证：`go build ./...`

## 3. LLM Prompt：辅助标签相关性约束

- [x] 3.1 `extractor_enhanced.go`: `buildEventPersonPrompt` 的「辅助标签要求」段落增加三条相关性约束
- [x] 3.2 验证：`go build ./...`

## 4. 前端：MatchDetailPanel 适配

- [x] 4.1 `MatchDetailPanel.vue`: direct_hit 场景下，在精确匹配列表下方展示 pairs（相似度匹配对），复用已有的 pairs 展示逻辑
- [x] 4.2 验证：`pnpm lint` + `pnpm exec nuxi typecheck`

## 5. 后端全量验证

- [x] 5.1 `cd backend-go && golangci-lint run ./... && go vet ./... && go test ./... && go build ./...`

## 6. 前端全量验证

- [x] 6.1 `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build`

## 7. 数据库初始化

- [x] 7.1 `postgres_migrations.go`: seed 列表新增 `semantic_board_match_direct_hit_min_overlap`
- [x] 7.2 `db_test.go`: 期望列表新增对应条目
- [x] 7.3 验证：`go build ./...`
