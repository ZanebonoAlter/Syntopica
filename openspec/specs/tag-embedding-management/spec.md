<!-- domain: semantic-board, db -->

# Tag Embedding Management

## Purpose

Manage lifecycle of tag embedding records, ensuring no duplicate or orphaned embeddings accumulate.

## Requirements

### Requirement: SaveEmbedding 清理同 tag 同 type 的旧记录

`SaveEmbedding` 在保存新 embedding 记录时，SHALL 删除同一 `topic_tag_id + embedding_type` 下 `text_hash` 不匹配的所有旧记录。

#### Scenario: 保存新 embedding 时清理旧记录
- **WHEN** tag 94712 已有 10 条 identity embedding（不同 text_hash），保存一条新的 identity embedding
- **THEN** 旧的 10 条记录被删除，只保留新的 1 条

#### Scenario: text_hash 匹配时更新而非清理
- **WHEN** 保存的 embedding 的 `topic_tag_id + embedding_type + text_hash` 与已有记录完全匹配
- **THEN** 更新已有记录（当前行为不变），不删除其他记录

### Requirement: 所有 tagmanagement 测试迁移到 Postgres

`internal/tagmanagement/` 下全部 14 个测试文件 SHALL 使用 `testutil.SetupTestDB(t)` 替代各自的 SQLite setup 函数。其中 13 个直接 import `glebarez/sqlite`，另 1 个（`semantic_board_backfill_test.go`）间接复用即将删除的 `setupSemanticBoardMatchingTestDB`。迁移文件列表：

- `repository/tagger_embedding_test.go`
- `repository/tag_job_queue_test.go`
- `service/core/embedding_test.go`
- `service/core/quality_score_test.go`
- `service/core/metadata_test.go`
- `service/core/hard_merge_test.go`
- `service/core/merge_tags_reembedding_test.go`
- `service/auxlabel/auxiliary_label_service_test.go`
- `service/board/semantic_board_matching_test.go`
- `service/board/semantic_board_upgrade_test.go`
- `service/board/semantic_board_backfill_test.go`
- `service/board/tag_clustering_test.go`
- `handler/semantic_board_handler_test.go`
- `handler/merge_reembedding_queue_test.go`

#### Scenario: 迁移后的测试文件不引用 glebarez/sqlite

- **WHEN** 检查上述文件的 import 声明
- **THEN** 不存在 `"github.com/glebarez/sqlite"` 导入

#### Scenario: 迁移后的测试在 Postgres 下全部通过

- **WHEN** Docker Postgres 运行且执行 `go test ./internal/tagmanagement/... -v`
- **THEN** 上述所有文件中的测试通过

### Requirement: embedding 测试使用真实 Postgres + pgvector

`embedding_test.go` 和 `tagger_embedding_test.go` 中的测试 SHALL 使用 `testutil.SetupTestDB(t)`，确保 embedding 写入和相似度查询走 pgvector 代码路径。

#### Scenario: embedding 写入测试验证 vector 列

- **WHEN** 测试通过 `testutil.SetupTestDB` 获取 Postgres 连接并创建一条 `TopicTagEmbedding` 记录
- **THEN** `embedding` (vector) 列被正确写入，查询 `SELECT embedding FROM topic_tag_embeddings` 返回非空结果

#### Scenario: 相似度查询使用 pgvector 距离操作符

- **WHEN** 测试调用 `FindSimilarTags` 或类似相似度搜索函数
- **THEN** 底层 SQL 使用 pgvector `<=>` 操作符，而非 Go 侧遍历比较

### Requirement: auxiliary_label_service 测试使用 Postgres

`auxiliary_label_service_test.go` SHALL 使用 `testutil.SetupTestDB(t)`，确保辅助标签匹配逻辑在 Postgres 环境下验证。

#### Scenario: 辅助标签匹配测试在 Postgres 下通过

- **WHEN** 运行 `TestAuxiliaryLabel*` 系列测试
- **THEN** 所有测试通过，包括 embedding 相似度匹配、别名合并、新建标签等场景

#### Scenario: merge embedding 比较使用与生产一致的路径

- **WHEN** `sqlMergeMatcher` 在测试中被调用
- **THEN** 相似度计算行为与生产一致（pgvector SQL 或保留的 Go 侧优化路径）

### Requirement: Board Embeddings 缓存存储已解析数据

Board embeddings 缓存 SHALL 存储 `map[uint][]float64`（已解析的 float64 切片），而非原始 pgvector 字符串。缓存加载时执行 `parsePgVector`，后续读取直接使用解析结果。

#### Scenario: parsePgVector called once per cache load

- **WHEN** board embeddings 缓存首次加载（或失效后重新加载），DB 中有 200 个 board embeddings
- **THEN** 系统 SHALL 调用 `parsePgVector` 恰好 200 次（每个 embedding 一次），结果存入缓存

#### Scenario: Subsequent reads skip parsePgVector

- **WHEN** 10 次 `MatchTopicTag` 调用发生在缓存有效期内
- **THEN** 系统 SHALL NOT 为 board embeddings 调用 `parsePgVector`，直接使用缓存的 `[]float64`

### Requirement: Merge Embeddings 缓存存储已解析数据

Merge embedding 缓存（`AuxiliaryLabelService` 内）SHALL 存储 `map[uint][]float64`（已解析的 float64 切片），而非原始 pgvector 字符串。缓存加载时执行 `ParsePgVector`，后续读取直接使用解析结果。

#### Scenario: ParsePgVector called once per merge embedding cache load

- **WHEN** merge embedding 缓存首次加载（或失效后重新加载），DB 中有 10K 个活跃辅助标签
- **THEN** 系统 SHALL 调用 `ParsePgVector` 恰好 10K 次（每个 merge_embedding 一次），结果存入缓存

#### Scenario: Subsequent sqlMergeMatcher calls skip ParsePgVector

- **WHEN** 10 次 `ResolveAuxiliaryLabel` L2 调用发生在缓存有效期内
- **THEN** 系统 SHALL NOT 为 merge embeddings 调用 `ParsePgVector`，直接使用缓存的 `[]float64`

### Requirement: Tag Embedding 不缓存

Tag identity embedding 和 tag auxiliary embeddings SHALL NOT 被缓存。每次 `MatchTopicTag` 调用 SHALL 从 DB 加载当前 tag 的 embedding 数据（按 topic_tag_id 索引查询，开销极低）。

#### Scenario: Tag embedding loaded per call

- **WHEN** `MatchTopicTag` 被连续调用 100 次处理不同 tags
- **THEN** 系统 SHALL 对每次调用执行 tag embedding 的 DB 查询，不缓存 tag embedding

### Requirement: tag 删除时 embedding 级联清理（DB FK 兜底）

系统 SHALL 保证 `topic_tag_embeddings` 中不存在指向已删除 `topic_tags` 的孤儿行：数据库层 MUST 建立外键约束（topic_tag_id → topic_tags.id）并声明 `ON DELETE CASCADE`，与 GORM 模型的 `OnDelete:CASCADE` 声明保持一致。

#### Scenario: 删除 tag 触发向量级联删除
- **WHEN** 通过任何路径删除一条 topic_tag 记录
- **THEN** 该 tag 关联的所有 topic_tag_embeddings 行被数据库自动删除，无孤儿残留

#### Scenario: 外键约束生效
- **WHEN** 向 topic_tag_embeddings 插入一条 topic_tag_id 指向不存在 tag 的记录
- **THEN** 数据库拒绝该写入（FK 违反）

### Requirement: 存量孤儿 embedding 一次性清理

提供一次性维护脚本，分批（单批 ≤5 万行）删除 `topic_tag_embeddings` 中 `topic_tag_id` 无对应 `topic_tags.id` 的行，执行前 MUST 备份相关表、执行后 MUST 复核孤儿计数为 0，然后才允许执行加 FK 迁移。

#### Scenario: 清理后加 FK
- **WHEN** 存量清理完成且孤儿计数复核为 0
- **THEN** FK 迁移执行成功，后续删除自动级联

### Requirement: embedding 模型名列宽不窄于模型注册表

存 embedding 模型名的列 SHALL NOT 窄于模型注册表 `ai_models.name` 的列宽（当前两者均为 100 字符）。`topic_tag_embeddings.model` SHALL 接受注册表里能存下的**任意**模型名——包括本地 llama.cpp / GGUF 模型以文件绝对路径注册的名称——SHALL NOT 因超限导致向量保存失败。

模型定义（gorm tag）与 DDL SHALL 保持一致：模型定义中的 `model` 字段长度 SHALL 等于该列实际列宽，SHALL NOT 出现「DDL 已加宽但 tag 仍是旧窄值」的漂移（否则下次 AutoMigrate 会把列改回窄列）。

#### Scenario: 长模型名写入成功

- **WHEN** 一条 `topic_tag_embeddings` 记录的 `model` 为 61 字符的本地模型路径（`/home/zanebono/.syntopica/models/Qwen3-Embedding-4B-Q6_K.gguf`）
- **THEN** 写入成功，`model` 列原样保存该字符串，不抛 `SQLSTATE 22001`（value too long for type character varying）

#### Scenario: 列宽不窄于模型注册表

- **WHEN** 比对 `topic_tag_embeddings.model` 与 `ai_models.name` 的列宽
- **THEN** `model` 列宽 ≥ `name` 列宽（当前均为 100）

#### Scenario: 模型定义与 DDL 一致

- **WHEN** GORM AutoMigrate 依据模型定义同步表结构
- **THEN** `topic_tag_embeddings.model` 列宽保持 100，不被改回 50

#### Scenario: 模型定义变更不得顺带丢掉存量列的 NOT NULL

- **WHEN** 模型定义的 `model` 字段长度变更（`size:50` → `size:100`）触发 GORM AutoMigrate 对**存量列**发 ALTER
- **THEN** `topic_tag_embeddings.model` 的 NOT NULL 约束 SHALL 保持（不因 ALTER 被顺带 DROP）；若已被丢弃，版本化迁移 SHALL 把它恢复（幂等）

#### Scenario: 既有向量不受影响

- **WHEN** 列宽变更在有存量行的表上执行（含既有 `model` 为短名的历史行）
- **THEN** 存量行的 `model` 值与 `embedding` 向量原样保留，无截断、无丢行

### Requirement: 标签相似查询使用 halfvec ANN 两段式

系统在按 embedding 相似度查找标签（L2 相似命中路径）时，SHALL 先用 halfvec HNSW 索引召回候选集（候选池 ≥ 50、`hnsw.ef_search ≥ 120`），再在候选集内用 float32 向量距离精确重排后返回结果。两段式的最终排序口径 SHALL 与全量 float32 精确排序一致（top10 召回重合度 = 10/10）。

匹配阈值（MatchThreshold=0.92）、cosine 校验、L1/L2/L3 三级去重语义均不变。

#### Scenario: 相似查询走索引且结果与精确排序一致
- **WHEN** semantic 类向量 ≥ 2 万条且存在 halfvec HNSW partial 索引，随机取 20 个真实标签向量做相似查询
- **THEN** 每个查询的 top10 与全量 float32 精确排序 top10 完全一致（overlap=10/10），且单次查询 p95 < 50ms（EXPLAIN 确认 Index Scan）

#### Scenario: 查询 cast 必须带 typmod
- **WHEN** 相似查询 SQL 将向量 cast 为 halfvec
- **THEN** cast 表达式为 `::halfvec(2560)`（与索引表达式精确匹配），不带 typmod 的 `::halfvec` 写法视为缺陷（不走索引）

#### Scenario: 索引覆盖三类 embedding
- **WHEN** 查看 topic_tag_embeddings 索引
- **THEN** semantic / identity / event_keyword 三类各有一个 partial halfvec HNSW 索引（按 embedding_type 过滤），坏索引 `tmp_hnsw_test` 不存在

### Requirement: embedding 队列 worker 连续处理且有限并发

embedding 队列 worker SHALL 在处理完当前任务后立即取下一任务（无固定 2 秒轮询间隔），并 SHALL 以 ≤4 的并发度并行处理任务。任务领取 SHALL 使用 `FOR UPDATE SKIP LOCKED` 防止重复处理。

#### Scenario: 连续处理无空转
- **WHEN** 队列有 100 个 pending 任务且 provider 正常
- **THEN** 任务间间隔中位数 < provider 单次往返耗时 + 1s（无 2s 固定空转）

#### Scenario: 并发上限
- **WHEN** 队列大量积压
- **THEN** 同时处于 processing 状态的任务数 ≤ 4

#### Scenario: worker 重启不残留
- **WHEN** 队列服务 Stop 后再次 Start
- **THEN** 旧 worker goroutine 退出（进程内 worker 计数回到期望值），不存在多 worker 抢队列

### Requirement: 队列任务批量调用 embedding

worker 处理任务时，SHALL 将同类型（identity/semantic/event_keyword）的待处理标签合并为批量请求（每批 ≤32 条）调用 provider，并 SHALL 对批内每条文本独立命中/写入 embedding 缓存。

#### Scenario: 批量调用
- **WHEN** 队列积压 10 个标签任务（各含 identity+semantic 两种类型）
- **THEN** provider 收到 identity 批量请求 ≤1 次（≤32 条/批）+ semantic 批量请求 ≤1 次，而非 20 次单条请求

#### Scenario: 批量下缓存按单条命中
- **WHEN** 批量请求中某 3 条文本已有缓存
- **THEN** 该 3 条命中缓存不重复调 provider，其余 miss 条目正常出向量并写入缓存

### Requirement: 失败自动重试与可观测

任务失败时，若 retry_count < 3，SHALL 在延迟（指数退避，基座 30s）后自动回到 pending；worker SHALL 使用真实 logger 输出处理日志。`GET /api/embedding/queue/status` 响应 SHALL 额外包含 `oldest_pending_age_seconds` 与 `throughput_per_min` 两个字段。

#### Scenario: 失败自动重试
- **WHEN** 任务因 provider 暂时错误失败且 retry_count=1
- **THEN** 任务延迟后回到 pending 并被再次处理，不进入终态 failed

#### Scenario: 重试上限
- **WHEN** 任务第 4 次失败（retry_count=3）
- **THEN** 任务进入终态 failed，等待人工 retry 接口

#### Scenario: status 新增字段
- **WHEN** 调用 GET /api/embedding/queue/status
- **THEN** 响应含 oldest_pending_age_seconds（最老 pending 的年龄秒数）与 throughput_per_min（最近完成速率）
