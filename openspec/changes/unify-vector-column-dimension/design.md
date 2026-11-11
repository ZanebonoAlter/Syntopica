## Context

- **三个维度数字并存**：现网向量列 `vector(2560)`（主线程只读查证 `format_type`）、迁移 `20260403_0003` 硬编码 `vector(4096)`、`embedding_config.embedding_dimension=1024` 已被迁移 `20260614_0001` 删为死配置（真库查无此行）。
- **model 与实际 schema 长期不一致**：库内 6 个向量列是 `vector(2560)` 限定、8 个是无限定 `vector`；14 处 model 声明中 13 处是无限定 `type:vector`，唯一例外是 `margin_notes_repository.go:82` 的 `type:vector(2560)`。GORM AutoMigrate 明确 "Never drops or alters"（`internal/platform/database/db.go` Phase 1 注释），两边永不自动收敛。
- **失败残留**：`tmp_hnsw_test` 是 `topic_tag_embeddings` 上的 **invalid 索引**（`indisvalid=false`、`indisready=false`）——float32 HNSW 建在 2560 维列上，超过 pgvector 的 2000 维硬上限故创建失败，残骸从未清理。`pg_dump` 跳过 invalid 索引，故不影响备份档。
- **现存向量索引**：仅 `idx_topic_tag_embeddings_identity_halfvec_hnsw`（`USING hnsw (((embedding)::halfvec(2560)) halfvec_cosine_ops) WHERE embedding_type='identity'`，valid）。**无 float32 HNSW 索引**——2560 维超过其 2000 维硬上限，本就建不了。
- **运行时真相源已在位**：`embedding.go:99` 把模型输出维度写进每行 `Dimension` 字段；`ensureVectorDimension`（`embedding.go:473`）按该维度自动纠正列类型——纠正动作本身落回 DDL，是分叉的最后一环。
- **关键机制澄清**：`vector` → `vector(4096)` 是**加** typmod（不 binary coercible，需 cast，pgvector 无跨维度 cast → 报错，这正是灾备恢复失败的机制）；而 `vector(2560)` → `vector` 是**去** typmod（binary coercible，Postgres 对同 base type 的 typmod 变化通常只改 `pg_attribute.typmod` 而不重写数据）。**该假设由 `fix-disaster-recovery-channel` 的任务 2.2 在含非空向量的表上实测确认，未确认前不动迁移。**
- **数据量**：`topic_tag_embeddings` 51473 行、`semantic_labels` 204820 行、`daily_report_sections` 6026 行、`board_persistent_topics` 1804 行、`term_notes` 17 行。

## Goals / Non-Goals

**Goals:**

- 让**全部向量列的 schema 不再承载维度**，存量统一到无维度限定形态。
- 让运行时模型输出成为维度的唯一真相源，并在应用层补上写入前的维度一致性校验（列级约束消失后，这是唯一防线）。
- 清理失败残留的无效索引。
- 让 model 声明与实际 schema 一致（全部无维度限定）。

**Non-Goals:**

- 不引入 embedding 空间的显式版本号（属素材包协议 change；本 change 只保证「维度来自运行时」这一基础事实成立）。
- 不更换 embedding 模型、不重算存量向量。
- 不改动向量检索的业务语义（半精度 ANN 索引策略保持）。
- 不做跨模型的向量空间迁移工具。

## Decisions

### D1. 目标形态是无维度限定的 `vector`

- **结论**：所有向量列统一为无维度限定的 `vector`。
- **落点**：`backend-go/internal/tagmanagement/models/tag_models.go:24`（model 声明已是 `type:vector`，本 change 只需把唯一的例外 `margin_notes_repository.go:82` 归一）
- **取舍**：不改为「统一限定为某个维度」——那只是把硬编码从迁移挪到 DDL，换模型时仍要 ALTER 全表，分叉原地复发。无限定形态与既有 13 处 model 声明一致，是「让 model 与库对齐」而非「改 model 迁就库」。

### D2. 存量统一由一条版本迁移完成，幂等且带锁守卫

- **结论**：新增一条版本迁移，把带维度约束的向量列改为无维度限定，同时 `DROP INDEX` 掉无效索引残留；已是无限定的列直接跳过。
- **落点**：`backend-go/internal/platform/database/postgres_migrations.go`
- **取舍**：不写成独立的运维脚本 + 迁移只做校验。理由：无限定列是终态、且 ALTER 若不重写数据则耗时极短，放进启动路径可自动收敛，不依赖人记得跑。迁移内用 `withLockTimeout` 守卫（沿用 `db-migration-execution` 的既有约束），并对每张表分步记录日志，便于观察进度。

### D3. `ensureVectorDimension` 在无限定列上不再 ALTER

- **结论**：列已是无限定 `vector` 时，`ensureVectorDimension` SHALL NOT 执行 `ALTER COLUMN ... TYPE`；只负责按当前运行时维度校验/重建向量索引。
- **落点**：`backend-go/internal/tagmanagement/service/core/embedding.go:473`
- **取舍**：保留原有的「列有限定且维度不符则 ALTER」分支（向后兼容，直到存量转换完成）。不彻底删掉 ALTER 能力——若将来有人重新引入带维度约束的列，系统仍能自愈。

### D4. 索引维度由运行时决定

- **结论**：halfvec ANN 索引的表达式维度 SHALL 取当前运行时维度；检测到与既有索引表达式内嵌维度不一致时重建该索引。
- **落点**：`backend-go/internal/platform/database/postgres_migrations.go:4275`（`TopicTagIdentityHalfvecIndexDDL(dim)` 已是参数化的单一 DDL 源）
- **取舍**：不把索引维度写死进迁移。索引重建逻辑已存在于 `embedding.go:520-546`（含 `maintenance_work_mem` / `max_parallel_maintenance_workers` 的设置与复位），本 change 只让它在「列已无限定」的新常态下仍被正确触发。

### D5. model 归一

- **结论**：`margin_notes_repository.go:82` 的 `type:vector(2560)` 改为 `type:vector`。
- **落点**：`backend-go/internal/topicgraph/repository/margin_notes_repository.go:82`
- **取舍**：不动其余 13 处声明（它们已经是无限定）。改这一处的唯一目的，是消除「model 里还残留维度硬编码」这个会被后来者模仿的反例。

### D6. 无效索引残留随迁移清理

- **结论**：新迁移中 `DROP INDEX IF EXISTS tmp_hnsw_test`。
- **落点**：任务 2.2
- **取舍**：只删这一个已确认的残留，不写通用的「清理所有 invalid 索引」逻辑（那需要遍历系统目录并可能误删正在构建中的索引）。`IF EXISTS` 保证幂等。

### D7. 写入前的维度一致性校验

- **结论**：向量写入路径 SHALL 校验传入向量的实际维度与随值留存的 `Dimension` 字段一致，不一致则拒绝写入并记错误日志。
- **落点**：任务 3.1
- **取舍**：列级约束消失后，pgvector 本身仍会在**比较查询**时报维度不匹配（清晰信号），但**写入**不再被列拒绝。校验放在写入侧（而非只在读侧）是因为「写进了一条错维度的向量」比「查询时报错」更难排查。校验点复用既有的 `Dimension` 字段，不新增字段。

### D8. 显式空间版本号不在本 change

- **结论**：不在此引入 embedding 空间的版本标识（`v1`/`v2`）。
- **落点**：无代码落点：素材包协议 change（`add-material-pack-protocol`），因为空间版本的价值在于跨端协商——只有当服务端要向客户端下发向量时，「这份数据是哪个空间的」才成为必须携带的契约
- **取舍**：接受在素材包协议落地前，跨端场景仍需靠「模型标识 + 维度」两个字段隐式判定空间。本 change 不制造半成品的版本机制。

## Risks / Trade-offs

- **⚠️ `ALTER vector(2560)` → `vector` 是否触发全表重写**：机制上「去 typmod」应走 binary coercible 路径（不改数据），但**必须以 `fix-disaster-recovery-channel` 任务 2.2 的实测为准**。若实测显示会重写（`topic_tag_embeddings` 512MB 量级，Pi 上 SD 卡约 1.7MB/s），本 change 的执行窗口与迁移实现方式需重估——届时改为「运维脚本执行 + 迁移只做存在性校验并提示」。
- **halfvec 索引在列类型变更后的有效性**：索引表达式 `embedding::halfvec(2560)` 内嵌维度，在无限定列上应仍合法；但 Postgres ALTER TYPE 会重建依赖索引，需实测重建后 `indisvalid` 仍为 true、查询计划未退化（任务 2.3）。
- **失去列级维度校验**：由 D7 兜底。风险是**写入路径覆盖不全**——需排查所有向量写入点是否都经过校验点（任务 3.2 给出完整写入路径清单）。
- **迁移在启动路径执行**：若耗时超预期会拖慢应用启动（桌面客户端首次启动尤其敏感）。缓解：迁移内分表打点日志；桌面端后续的启动预算（0-3 遗留清单里的「ready ≤3s」目标）需按本 change 实测的启动耗时重新评估。
- **model 归一后 AutoMigrate 行为**：`type:vector` 无限定 → AutoMigrate 不再尝试对齐维度（本就不 ALTER），无副作用。
- **与前一个 change 的文件冲突**：两者都会触碰 `postgres_migrations.go`。已按用户决策**串行执行**，本 change 在 `fix-disaster-recovery-channel` 归档后开始。
