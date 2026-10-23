# Review: optimize-search-semantic-index

审查方式：`change-review-glm` 子线程只读审查（制品 + scoped diff + build/vet/lint/三包测试复跑），2026-10-23。
总评：迁移/ANN/精确复算/维度守卫与 design 一致，无 High；1 条 Medium（文档残留）+ 4 条 Low，均已处置或接受。修复 R-1~R-4 后可归档。

### R-1 2026-10-23【Medium】`_conventions.md` 残留旧断言，与新增迁移 20261023_0001 直接矛盾
- **状态**：已修复
- **证据**：`docs/reference/database/tables/_conventions.md:57` 原文「`topic_tag_embeddings.embedding` 无独立 HNSW 迁移语句；运行时 HNSW 仅作用于 daily_report_sections/board_persistent_topics（dim ≤2000）」。
- **处置**：改为「float32 HNSW `idx_topic_tag_embeddings_embedding`（运行时 ≤2000）+ 迁移 `20261023_0001` 的 halfvec identity 部分 HNSW `idx_topic_tag_embeddings_identity_halfvec_hnsw`（≤4000，语义搜索用）」，并保留其他表 ≤2000 的口径。

### R-2 2026-10-23【Low】`hnsw.ef_search` 未 clamp 到 pgvector 上限 1000：TopN>250 时 ANN 必失败、语义路回退全表扫描
- **状态**：已修复
- **证据**：`semantic.go` 的 `candidateLimit = max(128, cfg.TopN*4)` 直接进 `set_config('hnsw.ef_search', ...)`；pgvector 0.8.3 合法域 1..1000，TopN≥251 时越界报错 → 回退旧全表扫描 → 可能超时降级。
- **处置**：新增 `semanticMaxCandidates = 1000`，`candidateLimit` 与 `ef_search` 均 clamp 到 1000；`search_semantic_top_n` 极端大时最多召回 1000 候选（默认 8 远不触顶），不会因 GUC 越界把语义路打回超时。

### R-3 2026-10-23【Low】`rankMatchedTags` 对 NaN cosine 的取舍与旧 SQL HAVING 语义相反
- **状态**：已修复
- **证据**：原过滤 `if row.Cosine < cfg.MinCosine { continue }`，NaN 比较为 false → NaN 穿透阈值；旧 SQL `HAVING MAX(...) >= ?` 会因 NULL 过滤。
- **处置**：改为 `if !(row.Cosine >= cfg.MinCosine) { continue }`，NaN 一并排除，恢复旧语义。

### R-4 2026-10-23【Low】`ensureVectorDimension` 的 Connection 回调存在 GUC 泄漏早退路径
- **状态**：已修复
- **证据**：原 `defer RESET` 注册在第二个 SET 之后；若第二个 SET 失败，pinned 连接带着 256MB maintenance_work_mem 回池。
- **处置**：把 `defer RESET` 提到第一个 SET 之前注册（RESET 未设置过的 GUC 无害）。

### R-5 2026-10-23【Low】测试覆盖缺口标注（INVALID 索引重建分支无自动化用例）
- **状态**：已驳回（接受缺口）
- **证据**：迁移的同名 INVALID 索引先 drop 再建分支无测试；ANN 用例未断言 EXPLAIN 计划。
- **处置**：该分支为防御性代码（真库 `tmp_hnsw_test` 先例），静态审查无问题；计划命中证据由 tasks 6.5 真库 EXPLAIN（Index Scan 冷 171ms/热 7.5ms）覆盖。后续如补测，可在 PG 用例中把 `pg_index.indisvalid` 置 false 后重跑迁移断言 drop/rebuild。

### R-6 2026-10-23【Low】工作树多 change 交织，提交拆分需注意
- **状态**：已采纳
- **证据**：`postgres_migrations.go` append 链同时带上 `signalDispositionMigration()`（20261015_0001）/`questionResearchBriefMigration()`（20261019_0001）两条外来迁移；二者真库 `schema_migrations` 已应用、幂等。
- **处置**：同文件交织无法拆分，随本 change 提交时在 commit message 注明「交织收账」；迁移按版本排序执行，append 顺序不影响执行序。

审毕无 High 发现（审查范围：迁移/ANN 查询/精确复算/维度守卫/三包测试/三份文档；真库性能数据取 tasks 6.5 在档声明）。
