# Tasks: add-ghost-mount-unmount

## 1. 墓碑模型与匹配防复活

- [ ] 1.1 `tag_models.go` 新增 `TopicTagBoardLabelTombstone`（UNIQUE(topic_tag_id, semantic_board_id)），wire.go 注入注册 AutoMigrate。验证：`cd backend-go && go build ./...` 通过；启动后表存在（`\d topic_tag_board_label_tombstones`）
- [ ] 1.2 `semantic_board_matching.go`：`LoadTagTombstones` + `filterTombstonedMatches`（内存过滤），MatchTopicTag 在 evaluate 后、MaxBoards 截断前调用。验证：`go test ./internal/tagmanagement/service/board -run TestTombstone -v` 通过（含墓碑不占名额/全部命中/composite_hit 同滤三用例）

## 2. 卸载与墓碑 API

- [ ] 2.1 `POST /api/semantic-boards/:id/unmount-tags`（批量、单事务删挂载+upsert 墓碑、幂等、404 校验），路由注册。验证：`go test ./internal/tagmanagement/handler -run TestUnmount -v` 通过（同事务/幂等重放/404 三用例）
- [ ] 2.2 `GET /api/semantic-boards/:id/mount-tombstones`（列表带 tag label）+ `DELETE /api/semantic-boards/:id/mount-tombstones/:topicTagId`（删墓碑+重算+摘要）。验证：`go test ./internal/tagmanagement/handler -run TestTombstone -v` 通过（列表/撤销重算/404 三用例）

## 3. 体检候选

- [ ] 3.1 boardhealth `UnmountCandidates(ctx, db, boardID)`（双挂口径 SQL，mounted cosine 升序 LIMIT N）+ 快照模型加 `UnmountCandidates` 列 + config 键 `board_health_unmount_candidates_limit`（默认 20）。验证：`go test ./internal/tagmanagement/service/boardhealth -run TestUnmountCandidates -v` 通过（双挂入列/单挂不入/LIMIT 三用例）

## 4. 前端

- [ ] 4.1 `BoardHealthPanel.vue` 卸载建议区块（清单+复选+批量卸载）+ 已卸载区块（列表+恢复），api client 三方法。验证：`cd front && pnpm exec nuxi typecheck` 通过 + `pnpm lint` 通过

## 5. 测试资产

- [ ] 5.1 test-cases.md 已建；按主链路表补 agent-browser 端到端落点（勾选→批量卸载→恢复，静态 :5100）。验证：人工：浏览器操作留痕（截图或步骤记录）——后端全绿后执行

## 6. 真库效果核对

- [ ] 6.1 部署后下一轮体检快照含 unmount_candidates；科技巨头预期 ~167、美国 ~31、影视 ~7。验证：SQL 计数对比留痕本节
- [ ] 6.2 通过 API 批量卸载科技巨头候选 → 触发 rematch-all → ghost ≤10 且墓碑 0 复活（挂载数不回升）。验证：卸载前后 `SELECT count(*) FROM topic_tag_board_labels WHERE semantic_board_id=15220 AND direction_mismatch` 留痕 + rematch-all 后复核
- [ ] 6.3 次日体检科技巨头评分回升（ghost 占比降驱动）；日报无异常。验证：快照 SQL 对比 + 人工日报抽查

## 7. 文档

<!-- doc-impact: semantic-board, api -->

- [ ] 7.1 `docs/reference/flow/semantic-board.md` 红线 4 末尾补卸载动线一句（mismatch 可经墓碑动线显式摘除，红线 20 体检附属候选口径）；`docs/reference/api/` 补三端点。验证：`grep -c "墓碑" docs/reference/flow/semantic-board.md` ≥ 1 且 api 文档存在新增条目

## 8. 验证

- [ ] 8.1 `cd backend-go && golangci-lint run ./internal/tagmanagement/...`，期望 0 issues
- [ ] 8.2 `cd backend-go && go vet ./internal/tagmanagement/...`，期望无输出
- [ ] 8.3 `cd backend-go && go test ./internal/tagmanagement/service/board ./internal/tagmanagement/service/boardhealth ./internal/tagmanagement/handler`，期望 ok（影响包）
- [ ] 8.4 `cd front && pnpm exec nuxi typecheck && pnpm lint`，期望通过
- [ ] 8.5 `bash scripts/harness/scenario-trace.sh openspec/changes/add-ghost-mount-unmount`，期望退出码 0
- [ ] 8.6 `bash scripts/harness/archive-readiness.sh add-ghost-mount-unmount`，期望全绿

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 卸载后 rematch 不复活 | backend-go/internal/tagmanagement/service/board/board_match_repo_pg_test.go |
| 墓碑行不占 MaxBoards 名额 | backend-go/internal/tagmanagement/service/board/semantic_board_matching_unit_test.go |
| 撤销墓碑恢复匹配 | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| 批量卸载同事务 | backend-go/internal/tagmanagement/service/board/board_match_repo_pg_test.go |
| 幂等重放 | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| 版块不存在 | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| 列表带标签名 | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| 撤销触发重算 | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| 撤销不存在的墓碑 | backend-go/internal/tagmanagement/handler/board_crud_handler_test.go |
| 双挂 mismatch 进入候选 | backend-go/internal/tagmanagement/service/boardhealth/repo_pg_test.go |
| 单挂 mismatch 不进候选 | backend-go/internal/tagmanagement/service/boardhealth/repo_pg_test.go |
| 候选数受配置上限 | backend-go/internal/tagmanagement/service/boardhealth/repo_pg_test.go |
| 体检轮次零副作用 | backend-go/internal/tagmanagement/service/boardhealth/run_test.go |

## 9. Review

- [ ] 9.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账
