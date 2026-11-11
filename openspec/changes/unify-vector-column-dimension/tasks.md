## 1. 前置：确认 ALTER 语义与耗时（阻断项）

- [ ] 1.1 复核 `fix-disaster-recovery-channel` 任务 2.2 的实测结论：`vector(2560)` → 无维度限定 `vector` 的 ALTER 是否成功、是否重写数据、耗时多少。**若实测显示会全表重写**，停下并回报（design Risks 第一条），本 change 的迁移实现方式需重估后再继续
- [ ] 1.2 实测 halfvec 索引在列类型变更后的有效性：在临时库中造一张含向量的表 + 建 halfvec HNSW 索引 → 执行列类型变更 → 确认索引重建后 `indisvalid=true` 且 `pg_get_indexdef` 表达式仍为 `((embedding)::halfvec(N)) halfvec_cosine_ops`，验证：实测命令与输出记入本 change 的 `review.md` 与 `docs/reference/database/` 对应文档

## 2. 迁移：存量列统一 + 无效索引清理

- [ ] 2.1 `backend-go/internal/platform/database/postgres_migrations.go`：新增一条版本迁移，把带维度约束的向量列改为**无维度限定**的 `vector`——覆盖 `topic_tag_embeddings.embedding`、`daily_report_sections.embedding`、`semantic_labels.embedding`、`semantic_labels.merge_embedding`、`term_notes.embedding`、`board_persistent_topics.embedding`；已是无限定的列直接跳过（幂等）；每张表用 `withLockTimeout(db, "5s", ...)` 守卫并逐表打点日志，验证：`grep -n "ALTER COLUMN embedding TYPE vector" backend-go/internal/platform/database/postgres_migrations.go` 命中新迁移且目标类型无维度限定
- [ ] 2.2 同一条迁移内：`DROP INDEX IF EXISTS tmp_hnsw_test`（已确认的 invalid 索引残留），验证：迁移在无该索引的库上执行不报错
- [ ] 2.3 迁移幂等与保值单测（扩展 `fix-disaster-recovery-channel` 建立的 `backend-go/internal/platform/database/vector_dimension_migration_test.go`）：① 迁移执行后各列类型为无维度限定；② 行数不变；③ 重复执行成功且列类型不变；④ 非空向量的 `vector_dims` 不变，验证：`cd backend-go && go test ./internal/platform/database/... -run VectorDimension` 退出码 0

## 3. 运行时：纠正器改造与写入校验

- [ ] 3.1 `backend-go/internal/tagmanagement/service/core/embedding.go`：`ensureVectorDimension` 增加分支——列已是无维度限定 `vector` 时**不执行** `ALTER COLUMN ... TYPE`，改为按当前运行时维度校验向量索引（表达式内嵌维度不符则重建，复用既有 `TopicTagIdentityHalfvecIndexDDL(dim)` 与 `maintenance_work_mem` 设置/复位逻辑），验证：`grep -n "ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE" backend-go/internal/tagmanagement/service/core/embedding.go` 仅出现在「列有限定」分支内
- [ ] 3.2 排查并列出**全部向量写入路径**（不止 `SaveEmbedding`），确认每条路径都经过维度一致性校验点；补齐遗漏路径的校验，验证：产出写入路径清单（文件:行）并记入 `review.md`
- [ ] 3.3 维度一致性校验单测：维度一致 → 写入成功；维度不一致 → 拒绝写入且记错误日志（含期望维度与实际维度），验证：`cd backend-go && go test ./internal/tagmanagement/service/core/...` 退出码 0

## 4. model 归一

- [ ] 4.1 `backend-go/internal/topicgraph/repository/margin_notes_repository.go:82`：`type:vector(2560)` → `type:vector`，与其它 13 处声明一致，验证：`grep -rn "type:vector(" backend-go/internal --include=*.go | grep -v "_test"` 仅剩测试文件命中
- [ ] 4.2 确认 model 归一不引起 AutoMigrate 副作用（AutoMigrate 不 ALTER 类型），验证：`cd backend-go && go test ./internal/topicgraph/...` 退出码 0

## 5. 测试

- [ ] 5.1 影响包测试（只跑受影响包），验证：`cd backend-go && go test ./internal/platform/database/... ./internal/tagmanagement/service/core/... ./internal/topicgraph/...` 退出码 0
- [ ] 5.2 后端质量门禁，验证：`cd backend-go && golangci-lint run ./...` 退出码 0、`go vet ./...` 退出码 0、`go build ./...` 退出码 0

## 6. 文档

<!-- doc-impact: database, flow -->

- [ ] 6.1 `docs/reference/database/` 增补向量维度契约说明：维度来自运行时模型输出、schema 不承载维度、写入前校验、索引维度跟随运行时
- [ ] 6.2 `docs/reference/flow/daily-report.md` 同步：`daily_report_sections.embedding` 列类型由 `vector(2560)` 改为无维度限定，维度契约改为「取内容化文本 + 运行时维度」；核对「section embedding 必须取内容化文本而非标题，话题匹配锚点必须用 centroid」这条红线未被本 change 触及
- [ ] 6.3 `docs/reference/flow/semantic-board.md` / `docs/reference/flow/topic-graph.md`：同步向量列类型与维度契约的变化（若对应节有 embedding 维度描述）
- [ ] 6.4 变更溯源：archive 后按 §12.2 补 flow 变更溯源链接

## 7. 验证

- [ ] 7.1 `cd backend-go && go test ./internal/platform/database/... ./internal/tagmanagement/service/core/... ./internal/topicgraph/...` → 退出码 0
- [ ] 7.2 `cd backend-go && golangci-lint run ./...` → 退出码 0
- [ ] 7.3 `cd backend-go && go vet ./...` → 退出码 0
- [ ] 7.4 `cd backend-go && go build ./...` → 退出码 0
- [ ] 7.5 `openspec validate unify-vector-column-dimension --strict` → 退出码 0
- [ ] 7.6 `bash scripts/harness/scenario-trace.sh openspec/changes/unify-vector-column-dimension` → 退出码 0
- [ ] 7.7 `bash scripts/harness/doc-impact.sh verify openspec/changes/unify-vector-column-dimension` → 退出码 0
- [ ] 7.8 `bash scripts/harness/check-standards.sh --change unify-vector-column-dimension` → 退出码 0
- [ ] 7.9 **存量库实跑核对**（在一次性恢复库上，不碰生产库）：恢复到最新备份档 → 应用启动（跑本 change 的迁移）→ 查 `pg_attribute` 确认全部向量列无维度限定 → 查各表行数与向量 `vector_dims` 与恢复前一致 → halfvec 索引 `indisvalid=true`，验证：核对输出记入 `review.md`
- [ ] 7.10 `bash scripts/harness/review-trace.sh openspec/changes/unify-vector-column-dimension` → 退出码 0

### Scenario → 测试文件

| Scenario | 测试文件 |
| --- | --- |
| 维度取自模型实际输出 | `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go` |
| 配置表不持有维度键 | `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go` |
| 存量向量列统一为无维度限定 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |
| 迁移对已统一的列幂等 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |
| 列类型变更不丢弃向量值 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |
| 维度一致时写入成功 | `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go` |
| 维度不一致时拒绝写入 | `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go` |
| 维度变化后索引重建 | `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go` |
| 超过索引维度上限时跳过并记日志 | `backend-go/internal/tagmanagement/service/core/embedding_dimension_test.go` |
| 无效索引残留被清理 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |

- [ ] 7.11 Review：派 `change-review` 子线程只读审查全量制品与 diff（重点审：ALTER 是否在有数据表上安全、写入路径校验覆盖是否完整、索引重建逻辑在无限定列下是否仍被触发），意见按 High/Medium/Low 处置后在 `review.md` 建账，验证：`bash scripts/harness/review-trace.sh openspec/changes/unify-vector-column-dimension` 退出码 0
