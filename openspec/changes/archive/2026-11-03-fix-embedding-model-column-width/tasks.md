# Tasks: fix-embedding-model-column-width

## 1. 实现（迁移 + 模型定义）

- [x] 1.1 新增版本化迁移 `20261103_0001`：五张表的 `model` 列 `ALTER COLUMN ... TYPE VARCHAR(100)`；**条件执行**——表不存在跳过、列不存在跳过、列无长度上限跳过、列宽已 ≥100 跳过（真 no-op：不 ALTER、不取锁、不重建索引），只有列宽 <100 才 ALTER（`withLockTimeout` 5s 守卫）。可重复执行。
- [x] 1.2 新增版本化迁移 `20261103_0002`：恢复 `topic_tag_embeddings.model` 的 NOT NULL（用既有 `ensureNotNullDefault(db, "topic_tag_embeddings", "model", "''")`）。起因：本 change 的 gorm tag 变更触发 GORM AutoMigrate 对存量列发 ALTER，而 gorm postgres driver 的 `AlterColumn` 在 `field.NotNull == false`（本仓库约定 not null 不写进 tag）时执行 `DROP NOT NULL`，把迁移 `20260723_0001` 固化的约束静默丢弃。
- [x] 1.3 同步五处 gorm 模型定义的 `model` 字段长度（`size:50` → `size:100`）：`backend-go/internal/tagmanagement/models/tag_models.go`（`TopicTagEmbedding`）、`backend-go/internal/discovery/models/catalog.go`（`RouteEmbedding`）、`backend-go/internal/discovery/models/discovery_run.go`（`CandidateEmbedding`、`DiscoveryInterestEntry` 两处）、`backend-go/internal/discovery/models/preference_vector.go`（`PreferenceVector`）。**不加 `not null` tag**（仓库约定：not null 由显式迁移固化）。
- [x] 1.4 确认 gorm tag 与 DDL 一致：`grep -n 'size:100' backend-go/internal/tagmanagement/models/tag_models.go backend-go/internal/discovery/models/*.go` 五处 `model` 字段均已改；`grep -rn 'Model.*size:50' backend-go/internal/` 无残留。

## 2. 部署与存量数据修复

- [x] 2.1 重启后端使迁移生效：`bash scripts/dev/start-dev.sh --restart`；`schema_migrations` 记录 `20261103_0001` / `20261103_0002`。
- [x] 2.2 校验列宽：`information_schema` 中五张表 `model` 列宽 = 100（实际 100）；全库 `column_name='model'` 且有长度上限的列**无一 < 100**。
- [x] 2.3 校验存量数据未丢：迁移前后 `topic_tag_embeddings` 行数只增不减（增量来自 worker 正常写入）；既有 `model` 值无截断（均为 ≤15 字符短名）；`NULL`/`''` 行数均为 0。
- [x] 2.4 重排失败任务：调用既有 `POST /api/embedding/queue/retry` → 「已重试 672 个失败任务」（689 条 failed 中 17 条为标签已删的孤儿，由 `RetryFailed` 顺带丢弃）；随后 `GET /api/embedding/queue/status` 的 failed 归零。
- [x] 2.5 观察重排结果：重排后新增的 `character varying` 失败行数 = **0**；队列 failed 计数持续为 0。
- [x] 2.6 复核 NOT NULL 约束：`pg_attribute.attnotnull` 显示 `topic_tag_embeddings.model` = **true**（恢复）；其余四张表 `model` 列仍为可空（本来就是可空，未动）。

## 3. 测试

- [x] 3.1 迁移测试（testcontainer PG）：`backend-go/internal/platform/database/embedding_model_column_width_test.go` —— 造窄列 + 复现 22001 + 插入存量行（短名 / **恰好 50 字符边界值** / 一行**真实向量**）→ 跑迁移 → 断言列宽变 100、存量行 `model` 逐字节保留、**向量 `embedding::text` 逐字节保留**、行数不变、61 字符长名写得进、重复执行幂等无错。
- [x] 3.2 条件执行测试：列宽已达 100 时再跑迁移 → 不取锁、不重建索引（索引 oid 不变）、列宽仍 100。
- [x] 3.3 NOT NULL 恢复测试：先 `DROP NOT NULL` 复现事故现场 → 跑 `20261103_0002` → 断言恢复为 NOT NULL、幂等。
- [x] 3.4 列宽不变式测试（**数据驱动**）：查全库 `column_name='model'` 且有长度上限的列，逐张断言 ≥ `ai_models.name` 宽度（将来新增窄列即红）；迁移表清单经 `export_test.go` 导出比对（消除手工副本漂移）；gorm tag `size` 与 DDL 实际列宽一致；`topic_tag_embeddings.model` 仍是 NOT NULL。
- [x] 3.5 影响包测试通过：`cd backend-go && go test ./internal/platform/database/... ./internal/tagmanagement/... ./internal/discovery/...`。

## 4. 文档

<!-- doc-impact: database, flow -->

- [x] 4.1 `docs/reference/database/tables/_conventions.md`：新增「模型名列宽规则总述」节——全库 `model` 列宽一览 + 「MUST NOT 窄于 `ai_models.name`」不变式 + 违反后果（22001 实测）+ 两条维护纪律（tag 与 DDL 同批改 / 测试守住）。
- [x] 4.2 `docs/reference/database/tables/topic-tags.md`：§4.2 `topic_tag_embeddings.model` 列宽改 100 + 不变式说明（含 22001 事故链与缓存命中放大器）。
- [x] 4.3 `docs/reference/database/tables/preference-discovery.md`：discovery 四张向量表 `model` 列宽改 100 + 同一不变式说明。
- [x] 4.4 归档后按 §12.2 补 flow 变更溯源行：`docs/reference/flow/semantic-board.md`、`docs/reference/flow/topic-graph.md`、`docs/reference/flow/discovery.md` 各追加一行，指向 `openspec/changes/archive/2026-11-03-fix-embedding-model-column-width`。

## 5. 验证

- [x] 5.1 `cd backend-go && golangci-lint run ./internal/platform/database/...` → 0 issues。
- [x] 5.2 `cd backend-go && go vet ./internal/platform/database/...` → 零错误。
- [x] 5.3 `cd backend-go && go build ./...` → 编译通过。
- [x] 5.4 `cd backend-go && go test ./internal/platform/database/... -run 'EmbeddingModelColumnWidth' -v` → 4 个用例全 PASS。
- [x] 5.5 `cd backend-go && go test ./internal/platform/database/... ./internal/tagmanagement/... ./internal/discovery/...` → 全部 PASS。
- [x] 5.6 `bash scripts/harness/check-tasks-tail.sh openspec/changes/fix-embedding-model-column-width` → 退出码 0。
- [x] 5.7 `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-embedding-model-column-width` → 退出码 0。
- [x] 5.8 `bash scripts/harness/check-standards.sh --change fix-embedding-model-column-width` → 通过 257 / 失败 0。
- [x] 5.9 `bash scripts/harness/scenario-trace.sh openspec/changes/fix-embedding-model-column-width` → 退出码 0。
- [x] 5.10 `bash scripts/harness/review-trace.sh openspec/changes/fix-embedding-model-column-width` → 退出码 0（review.md 建账 9 条）。
- [x] 5.11 本 change 零前端改动（`ui-impact: none`）：本 change 未编辑 `front/` 任何文件（工作树里的 `front/` 改动均归属其它 active change，非本 change）。
- [x] 5.12 `openspec validate fix-embedding-model-column-width` → valid。

| Scenario | 测试文件 |
| --- | --- |
| 长模型名写入成功 | `backend-go/internal/platform/database/embedding_model_column_width_test.go` |
| 列宽不窄于模型注册表 | `backend-go/internal/platform/database/embedding_model_column_width_test.go` |
| 模型定义与 DDL 一致 | `backend-go/internal/platform/database/embedding_model_column_width_test.go` |
| 模型定义变更不得顺带丢掉存量列的 NOT NULL | `backend-go/internal/platform/database/embedding_model_column_width_test.go` |
| 既有向量不受影响 | `backend-go/internal/platform/database/embedding_model_column_width_test.go` |
| 长模型名写入候选向量成功 | `backend-go/internal/platform/database/embedding_model_column_width_test.go` |
| 长模型名写入偏好向量成功 | `backend-go/internal/platform/database/embedding_model_column_width_test.go` |
| 四张表列宽一致且不窄于模型注册表 | `backend-go/internal/platform/database/embedding_model_column_width_test.go` |
