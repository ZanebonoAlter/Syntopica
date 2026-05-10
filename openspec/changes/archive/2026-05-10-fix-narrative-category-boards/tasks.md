## 1. 后端：GetScopes 改为从 boards 查询

- [x] 1.1 重写 `service.go` 的 `GetScopes` 方法，将聚合查询从 `narrative_summaries` 改为 `narrative_boards`，字段名改为 `board_count`
- [x] 1.2 `GetScopes` 增加 `days` 参数，支持多日范围查询（默认 7 天）
- [x] 1.3 `handler.go` 的 `getNarrativeScopes` 解析 `days` 查询参数并传入 service
- [x] 1.4 验证：`go test ./internal/domain/narrative/... -run TestGetScopes -v`

## 2. 后端：生成结束后清理空 board

- [x] 2.1 在 `service.go` 新增 `cleanEmptyBoards(date, categoryID)` 函数，按日期范围删除无 narrative 关联的 board
- [x] 2.2 在 `GenerateAndSaveForCategory` 末尾调用 `cleanEmptyBoards`
- [x] 2.3 在 `GenerateAndSave` 末尾调用 `cleanEmptyBoards`（清理全局空 board）
- [x] 2.4 验证：`go test ./internal/domain/narrative/... -run TestCleanEmpty -v`

## 3. 前端：分类版块交互修复

- [x] 3.1 `topicGraph.ts` 中 `NarrativeScopeCategory` 接口字段 `narrative_count` 改为 `board_count`
- [x] 3.2 `NarrativePanel.vue` 的 `switchScope` 函数中，当 mode 为 `'category'` 时调用 `loadScopes()`
- [x] 3.3 `loadScopes` 调用传入 `days` 参数与 timeline 保持一致
- [x] 3.4 模板中 `cat.narrative_count` 改为 `cat.board_count`
- [x] 3.5 验证：`pnpm exec nuxi typecheck` 和 `pnpm build`
