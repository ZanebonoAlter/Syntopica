## 1. 索引与维度守卫

- [x] 1.1 在 `postgres_migrations.go` 新增 `semantic_labels` merge halfvec 表达式 HNSW 部分索引的 DDL 生成函数（模式对齐 `TopicTagIdentityHalfvecIndexDDL`，表达式内嵌运行时维度，WHERE 为 `label_type='auxiliary' AND status='active' AND merge_embedding IS NOT NULL`），验证：`cd backend-go && go test ./internal/platform/database -run TestSemanticLabelMergeHalfvecIndexDDL -count=1` 覆盖 dim=2560 的 DDL 文本与 `WHERE` 子句。
- [x] 1.2 新增 `20261124_0001` migration：维度按「列无限定 → 存量数据采样（表空则跳过）」取得（对齐 `ensureTopicTagIdentityHalfvecIndexForColumn`），事务内 `SET LOCAL maintenance_work_mem='256MB'` / `max_parallel_maintenance_workers=0` 后 `CREATE INDEX IF NOT EXISTS`。验证：`cd backend-go && go build ./...` 通过，且 `cd backend-go && go test ./internal/platform/database -run 'TestSemanticLabelMergeHalfvecIndexMigration' -count=1` 在测试库覆盖「空表跳过 / 有数据建成 / 重复执行幂等」。
- [x] 1.3 新增启动期守卫 `ensureSemanticLabelMergeHalfvecIndex(db, dim)`：读取索引表达式内嵌维度（复用 `halfvecExpressionDimension`），不存在 / 不符 / invalid 则重建；构建在固定连接上 `SET maintenance_work_mem='256MB'` + `SET max_parallel_maintenance_workers=0`，`RESET` 兜底；失败只 WARN 不阻断启动；挂载在 `EnsureVectorDimensionOnce` 的列维度守卫之后。验证：`cd backend-go && go test ./internal/platform/database -run TestEnsureSemanticLabelMergeHalfvecIndex -count=1` 覆盖「已存在且维度一致→不动」「维度不一致→重建」「不存在→创建」「invalid→重建」四条路径。
- [x] 1.4 `ensureSemanticLabelVectorColumnDim` Branch 3 在 `ALTER COLUMN … TYPE` 前先 `DROP INDEX IF EXISTS idx_semantic_labels_merge_halfvec_hnsw`（对齐 `core/embedding.go:ensureVectorDimension`「Drop indexes first」），并删除「semantic_labels has no vector index」过期注释。验证：`cd backend-go && go build ./...` 通过，且 `grep -n 'has no vector index' backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go` 零命中。

## 2. matcher 重写

- [x] 2.1 `mergeMatcherFunc` 签名只移除 `labels`（保留 `mergePgVector` 作为 SQL 绑定参数），同步更新 `planAuxiliaryLabels` 调用点。验证：`cd backend-go && go build ./...` 通过。
- [x] 2.2 `sqlMergeMatcher` 改两段式（design D3）：召回 `WHERE label_type='auxiliary' AND status='active' AND merge_embedding IS NOT NULL ORDER BY merge_embedding::halfvec(<dim>) <=> $1::halfvec(<dim>) LIMIT 32` → 候选用 `merge_embedding <=> $1::vector` 精排 → Go 侧按 `1-dist >= threshold`（`loadAuxLabelDedupeSim`）过滤、`RefCount` 降序 / `ID` 升序选 best。验证：`cd backend-go && go test ./internal/tagmanagement/service/auxlabel -run TestSQLMergeMatcher -count=1` 覆盖「阈值内多条→选 RefCount 大」「RefCount 相同→选 ID 小」「无候选→nil」「阈值外→nil」「halfvec 距离在阈值外但精确距离在阈值内→命中」。
- [x] 2.3 L2 查询包在每批一个只读事务里，`SET LOCAL hnsw.ef_search = 100` 后逐标签查询；embedder 调用保持在事务外。验证：`cd backend-go && go test ./internal/tagmanagement/service/auxlabel -run 'TestAttachTwoPhaseNoEmbedderInsideTransaction|TestSQLMergeMatcher' -count=1` 全绿。
- [x] 2.4 实现 design D6 的截断兜底：精排后 32 条全部 ≥ 阈值时，同事务内 `SET LOCAL hnsw.ef_search = 200` 后以 `LIMIT 128` 重查一次再裁决。验证：单测打桩查询层断言第二次查询发生且带上更大的 LIMIT。

## 3. 删除向量缓存与 L1 改造

- [x] 3.1 从 `auxLabelCache` 移除 `mergeEmbeddings` / `mergeEmbeddingsAt`，删除 `auxMergeEmbeddingsTTL` 与 `packageAuxLabelCacheGetOrLoad`，`InvalidateAuxLabelCache` 只保留 `activeLabels` 清理，`applyCreate` / `applyDisable` / `applyMerge` / `invalidateOnAliasChange` / `lightweightLabelCopy` 的 merge map 注释同步删除；**同步删改 `aux_label_cache_test.go`**（`TestAuxLabelCacheAliasChangePreservesMergeEmbeddings` 删除，其余 5 个去掉 merge 断言）。验证：`cd backend-go && grep -rn 'mergeEmbeddings\|auxMergeEmbeddingsTTL\|packageAuxLabelCacheGetOrLoad' internal/` **零命中（含测试）**，且 `go test ./internal/tagmanagement/service/auxlabel -run TestAuxLabelCache -count=1` 全绿。
- [x] 3.2 `loadActiveAuxiliaryLabels` 附带构建 `bySlug`（含每个别名的 `Slugify(alias)`）/ `byAliasFold` / `byNormKey` 三个 map，先到先得；`planAuxiliaryLabels` 的 L1 改为按 slug → aliasFold → normKey 顺序查找。验证：`cd backend-go && go test ./internal/tagmanagement/service/auxlabel -run TestAuxiliaryLabelServiceL1 -count=1` 全绿，并新增「别名 slug 口径命中」「同 key 多行取列表首个」两条用例。

## 4. 对账与回归

- [x] 4.1 写一次性对账程序（`backend-go/cmd/aux-label-dedup-parity/`）：从真实库**只读**加载全部 active 辅助标签的 `merge_embedding`，对同一批样本（库中现有标签的向量，默认 200 条）分别执行旧实现（Go 全量余弦）与新实现（SQL 召回 + 精排），打印当次召回执行计划（索引 / 顺序扫描）、差异清单（label / 旧目标 / 新目标 / 相似度 / 归因）与「阈值内有 ≥2 个邻居」的样本数。验证：`cd backend-go && go run ./cmd/aux-label-dedup-parity --sample 200` 输出 `mismatch=0`，或差异条目均有归因记录。
- [x] 4.2 用对账结果确认语义等价；若出现差异，按 design「Risks」处理（逐条归因，召回漏召记为缺陷样本）并在 tasks 追加修复项。验证：对账报告 `mismatch=0`，或差异条目均有归因记录。
- [x] 4.3 运行受影响包回归：`cd backend-go && go test ./internal/tagmanagement/service/auxlabel ./internal/platform/database -count=1`，期望全部 PASS。

## 5. 文档
<!-- doc-impact: database, flow -->
<!-- doc-impact-excuse: api=edit.map 归属轨里的 backend-go/internal/topicgraph/handler/ 与 front/ 路径是前序会话遗留记录，本 change 未改任何 handler / router / 前端文件（新增的是 cmd 一次性对账工具，无 API 契约变化） -->

- [x] 5.1 `docs/reference/database/tables/semantic-labels.md` 记录 `semantic_labels` 的新索引（索引名、表达式、WHERE 条件、迁移内维度来源与启动期守卫重建规则、建索引的 GUC 约束），写法对齐 `tables/topic-tags.md` 的「语义搜索向量索引」段；`tables/_conventions.md` 若有向量索引口径段落则补一行。按 `docs/reference/standard/shared/doc-authoring.md` 的头部注释与注册点要求落位。验证：`grep -rn 'idx_semantic_labels_merge_halfvec_hnsw' docs/reference/database/` 命中。
- [x] 5.2 `docs/reference/flow/semantic-board.md`「业务约束与不变量」补一条顶层红线句（按 doc-authoring 红线句格式）：辅助标签 L2 去重 MUST 走数据库 ANN 索引召回并以原列精确距离裁决，SHALL NOT 在进程内全量加载向量。验证：`grep -n 'ANN' docs/reference/flow/semantic-board.md` 命中且位于「业务约束与不变量」节内。
- [x] 5.3 归档前跑 `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-aux-label-dedup-ann`，期望「通过（声明: database, flow）」。

## 6. 测试

- `cd backend-go && go test ./internal/tagmanagement/service/auxlabel -count=1`（打标 / 缓存 / L1 / L2 / 两阶段 attach）
- `cd backend-go && go test ./internal/platform/database -count=1`（migration / 索引守卫 / DDL 文本）
- `cd backend-go && go vet ./internal/tagmanagement/service/auxlabel ./internal/platform/database ./cmd/aux-label-dedup-parity`

## 7. Review

- [x] 7.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账（主线程执行；实现子线程不勾选）。
- [x] 7.2 处置 review 意见并回写 review.md 状态：R-1（High，`applyCreate` 原地写 L1 map 与无锁读者冲突）→ `matchL1` 持读锁 + 并发回归闸；R-2（换模型维度窗口的诚实口径 + 守卫 ERROR）；R-3（探针 `LIKE 'idle in transaction%'` + 补回阶段二事务断言）；R-4（制品迁移号 → `20261124_0002`）；R-5（spec 拆两条 Requirement + 新增 L1 并发安全要求）；R-6（对账程序移出 service 包）。验证：`bash scripts/harness/review-trace.sh openspec/changes/fix-aux-label-dedup-ann` 退出码 0 且无 High 停留「待处理」。
- [x] 7.3 核对 cmd/aux-label-dedup-parity 与服务侧 `sqlMergeMatcher` 的召回/精排/裁决口径一致（R-6 的代价：两套实现）——比对 WHERE 谓词来源、`merge_embedding::halfvec(dim) <=> ?::halfvec(dim)` 形态、原列 `<=> ?::vector` 精排、`1-dist >= threshold` 与 RefCount/ID 裁决顺序。验证：`diff <(sed -n '/func mergeRecallSQL/,/^}/p' backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go) <(sed -n '/func mergeRecallSQL/,/^}/p' backend-go/cmd/aux-label-dedup-parity/matcher.go)` 差异只在允许项（或以注释写明差异理由），且两侧都引用 `database.SemanticLabelMergeHalfvecIndexPredicate`。

## 8. 验证

- [x] 8.1 静态检查：`cd backend-go && golangci-lint run ./... && go vet ./...`，期望 0 issues。
- [x] 8.2 构建：`cd backend-go && go build ./...`，期望退出码 0。
- [x] 8.3 索引存在与维度（部署后，主线程执行）：`docker exec syntopica-postgres psql -U postgres -d syntopica -A -t -c "select indexdef from pg_indexes where indexname='idx_semantic_labels_merge_halfvec_hnsw';"`，期望输出含 `halfvec(2560)` 与 `WHERE` 条件。
- [x] 8.4 查询走索引（部署后）：`docker exec syntopica-postgres psql -U postgres -d syntopica -c "EXPLAIN SELECT id FROM semantic_labels WHERE label_type='auxiliary' AND status='active' AND merge_embedding IS NOT NULL ORDER BY merge_embedding::halfvec(2560) <=> (SELECT merge_embedding::halfvec(2560) FROM semantic_labels WHERE merge_embedding IS NOT NULL LIMIT 1) LIMIT 32;"`，期望出现 `Index Scan using idx_semantic_labels_merge_halfvec_hnsw`。
- [x] 8.5 召回层对账（部署后）：重跑 `cd backend-go && go run ./cmd/aux-label-dedup-parity --sample 200`，期望执行计划为索引扫描且 `mismatch=0`（或差异逐条归因）。
- [x] 8.6 运行期内存：部署后等 15 分钟（跨过旧 TTL 周期），用 start-dev.sh pidfile 定位进程，`grep -E 'VmHWM|VmRSS' /proc/<pid>/status`，期望 VmRSS 稳定在 300MB 量级、VmHWM 不再出现 2.6GB 级峰值，且 `grep -c 'dedup gate slow' backend-go/logs/app.log` 不再增长。**前置**：必须先用最终代码重启后端（当前 PID 19691 是 00:40:45 编译的中间态，落后源码 1 小时+，其 VmHWM 1.8GB 不代表最终实现）。
  - **实测 2026-11-25**：VmRSS 96–157MB（目标 300MB 达成）、VmHWM 1.6GB（启动期处理积压的峰值，非 2.6GB 级）。`dedup gate slow` 仍有新增，但归因为 embedder 批量推理 3.2–4.5s/32 条（L2 召回仅 5–17ms，见 8.9），不属本 change 范围。
- [x] 8.7 归并后向量不变：`docker exec syntopica-postgres psql -U postgres -d syntopica -A -t -c "select count(*) from semantic_labels where label_type='auxiliary' and status='active' and (embedding is null or merge_embedding is null);"`，期望 0（沿用既有不变量）。

- [x] 8.8 cmd 对账工具与服务侧召回口径一致性核对（R-6 双实现代价）：`diff <(sed -n '/func mergeRecallSQL/,/^}/p' backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go) <(sed -n '/func mergeRecallSQL/,/^}/p' backend-go/cmd/aux-label-dedup-parity/matcher.go)` → 输出为空（完全一致），两侧均引用 `database.SemanticLabelMergeHalfvecIndexPredicate`。
- [x] 8.9 L2 召回延迟实测（shared_buffers 512MB + pg_prewarm 后）：同向量热路径 17.4ms → 9.0ms → **5.1ms**；不同向量冷路径 278ms → **18–58ms**；精排 32 行 TOAST **8.5ms**。命令见 `review.md` 覆盖清单与本文档 8.4。
