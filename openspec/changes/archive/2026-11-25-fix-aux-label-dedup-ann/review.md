# Review — fix-aux-label-dedup-ann

> 格式契约：`openspec/specs/review-trace-gate`。审查方式：子线程聚焦 review（`change-review` 档 + `zai-coding-cn/glm-5.4`），审查制品 + 本轮 diff，按 §0.6「review 执行要点」执行（design 落点复核 → 高风险文件精读 → grep 不变量 → High/Medium/Low 分类）。

## 审查范围

- 制品：`proposal.md` / `design.md`（D1–D8）/ `specs/auxiliary-label/spec.md` / `tasks.md` / `explore-findings.md` / `human-feedback.md`
- 代码：`backend-go/internal/tagmanagement/service/auxlabel/`（`auxiliary_label_service.go` / `dedup_parity.go` / 测试）、`backend-go/internal/platform/database/postgres_migrations.go` + 新增测试、`backend-go/cmd/aux-label-dedup-parity/`
- 文档：`docs/reference/database/tables/semantic-labels.md`、`tables/_conventions.md`、`docs/reference/flow/semantic-board.md`
- 不在范围：`migrator.go` 的 2 行改动（属 `add-ai-generated-article-detection`）

## 审查结论

1 条 High、5 条 Medium。design D1–D8 落点全部可解析，无「可删项」。

---

### R-1 2026-11-25【High】`applyCreate` 原地写 L1 map，重现 2026-10-07 的 lock-free 读者 fatal

- **状态**：已修复
- **位置**：`auxiliary_label_service.go` `applyCreate`（`c.active.add(row)`）、`auxLabelIndex.add`/`match`、`planAuxiliaryLabels` 锁外调用 `active.match`
- **问题**：`loadActiveAuxiliaryLabels` 返回共享 `*auxLabelIndex`，`planAuxiliaryLabels` 在 `RUnlock` 之后无锁读三张 map；`applyCreate` 在写锁内对同一指针原地 `add` 写 map。写锁只与写锁互斥，拦不住无锁读者 → `fatal error: concurrent map read and map write`（不可 recover，进程退出）。旧实现只 append slice 且注释明确「map 一律不碰」，改造把隐患迁到了 L1 map 上但保留了原地写。触发面：`core/tag_queue.go` 以 `concurrency=10` 并行跑 `AttachAuxiliaryLabels`。
- **证据**：`applyCreate` 内 `c.active.labels = append(...)` + `c.active.add(row)`；`planAuxiliaryLabels` 内 `active.match(...)` 无锁；被删旧注释原文「readers index it without holding the lock — an in-place map write races them into fatal error ... (observed 2026-10-07)」。
- **建议**：L1 查找持读锁（`auxLabelCache.matchL1` 在 `RLock` 内查 `c.active`），或 `applyCreate` 改为重建索引后换指针（与 `applyDisable/applyMerge` 对齐）；删掉「append beyond any reader's snapshot length」过期注释；补 `-race` 回归（一 goroutine 循环 `match`、一 goroutine `applyCreate`）。
- **处置**：采纳建议①。新增 `auxLabelCache.matchL1`（`RLock` 内查 `c.active`），`planAuxiliaryLabels` 的 L1 改走它，锁外读 map 的路径与过期注释一并删除；`applyDisable/applyMerge` 本来就是换指针、未动。回归闸 `TestAuxLabelCacheConcurrentCreateAndMatchL1`（写侧 5000 次 `applyCreate` × 读侧循环 `matchL1`）已加：修复前形状 3/3 次挂 `fatal error: concurrent map read and map write`，修复后 3/3 次通过。本机 `-race` 不可用（Go 1.27.1 race runtime 在 arm64 内核报 `unsupported VMA range`，hello-world 同报），故用 Go 运行时自带 map 并发检测作为闸门。

### R-2 2026-11-25【Medium】换模型维度切换窗口：召回 SQL 对旧维度行报错、整批 attach 失败

- **状态**：已修复
- **位置**：`mergeRecallSQL`、`EnsureSemanticLabelMergeHalfvecIndex`、spec Scenario「维度变化后索引重建」、`docs/reference/database/tables/semantic-labels.md`
- **问题**：`semantic_labels` 两个向量列是裸 `vector`，换模型后新旧维度行可并存。旧实现对维度不符的行 `err != nil → continue`（跳过）；新实现 `merge_embedding::halfvec(<newDim>)` 遇旧维度行 → SQLSTATE 报错 → 整批 attach 失败（含本应 L1 命中的标签）。守卫 `CREATE INDEX ... halfvec(newDim)` 在旧维度行存在时必然失败，故 spec「维度变化 SHALL 重建」在真实库上不成立。design Risks / 文档「退化为顺序扫描，结果仍正确」在此窗口不成立——不是慢，是错。
- **建议**：守卫重建前比对存量维度与运行时维度，不一致打 ERROR 并给出清理 SQL（清旧维度向量 → 走既有 backfill）；把维度类 SQLSTATE 包成明确错误；spec/design/文档改口为「旧维度行清理后重建；切换窗口内 L2 报错而非降级」，Migration Plan 补「换模型先清旧向量」。
- **处置**：代码 + 制品 + 文档同步改口。新增 `SemanticLabelMergeDimensionCleanupSQL` 常量与 `warnSemanticLabelMergeDimensionMismatch`（重建路径比对存量维度，不一致/mixed 打 **ERROR** 并附清理 SQL + backfill 端点）；`wrapMergeRecallError` 把 pgvector 维度类错误（`22P02`/`42883`）包成带指引的可诊断错误；`EnsureVectorDimensionOnce` 删掉「退化为顺序扫描、结果仍正确」的假承诺；`semantic-labels.md` 新增「降级口径（诚实版）」；design D2 新增「换模型窗口的诚实口径」+ Risks + Migration Plan 补「先清旧向量」；spec Scenario 补「旧维度行清理后重建」「不一致时显式报错」。

### R-3 2026-11-25【Medium】两阶段探针漏报 `idle in transaction (aborted)`，且丢了「阶段二开过事务」断言

- **状态**：已修复
- **位置**：`attach_two_phase_test.go`、`planAuxiliaryLabels` 头注释
- **问题**：探针改查 `pg_stat_activity` 的方向正确（新召回只读事务确会误报旧单调探针），但 ① `state = 'idle in transaction'` 精确匹配漏掉 `idle in transaction (aborted)`；② 旧断言「阶段二确实开过事务」被删，该行为不再被钉住。另头注释仍写「L2 … all OUTSIDE any transaction」，与 D6 后事实相反。
- **建议**：条件改 `state LIKE 'idle in transaction%'`；补回「阶段二开过事务」断言；修正注释为「L1/merge 批嵌入在事务外，L2 召回在只读事务内，storage 批嵌入在事务外」。
- **处置**：探针条件改 `state LIKE 'idle in transaction%'`；新增 `txBeginCounter`（包 `ConnPool` 拦 `BeginTx`）+ `TestAttachTwoPhaseOpensRecallAndWriteTransactions` 断言 `begins=2 / readOnly=1 / readWrite=1`，把「阶段一召回只读事务 + 阶段二读写事务且原子」钉回测试；`planAuxiliaryLabels` 头注释改为「L1 与 merge 批嵌入在事务外、L2 召回在每批一个只读事务内、storage 批嵌入在事务外」。

### R-4 2026-11-25【Medium】制品写 `20261124_0001`，实现/文档/测试是 `20261124_0002`

- **状态**：已修复
- **位置**：`design.md` D2/D7/Context、`tasks.md` 1.2
- **问题**：`20261124_0001` 已被同树 `topicLaneConstraintMigration` 占用，实现正确改用 `_0002`，但制品未同步；归档后按制品检索会找错版本号，tasks 1.2 验证命令也与实际测试名不一致。
- **建议**：design D2/D7、tasks 1.2 统一改 `20261124_0002`，Context 那句注明被同树 change 占用后改号。
- **处置**：design D2/D7/Context 与 tasks 1.2 统一改为 `20261124_0002`，Context 注明被同树 change 占用后改号。

### R-5 2026-11-25【Medium】spec Requirement 正文 711 字符，`openspec validate --strict` 退出码 1

- **状态**：已修复
- **位置**：`specs/auxiliary-label/spec.md`「辅助标签语义去重走数据库 ANN 索引」
- **问题**：Requirement 正文含 6 条 bullet 共 >500 字符，`--strict` 报 WARNING 且 exit 1；其中多条与 Scenario 已覆盖的行为重复。
- **建议**：正文压到一句 SHALL/SHALL NOT，细节下沉到 Scenario 或拆成 2 条 Requirement。
- **处置**：拆为两条 Requirement（「辅助标签语义去重走数据库 ANN 召回」+「L2 阈值裁决使用原列精确距离」），细节下沉到 Scenario；另新增第三条「L1 索引增量写入对并发读者安全」（R-1 的行为契约）。`openspec validate --strict` 现 exit 0。

### R-6 2026-11-25【Medium】一次性对账程序连同旧算法复刻常驻 service 包，归宿未定义

- **状态**：已修复
- **位置**：`auxlabel/dedup_parity.go`（392 行，编入 server 二进制）、`cmd/aux-label-dedup-parity/main.go`、`docs/reference/flow/semantic-board.md` 第 38 条
- **问题**：design/tasks 定义它是一次性对账，但把被消灭的旧算法完整复刻进 service 包，无退役计划；service 包内零调用方，仅 cmd 用。
- **建议**：二选一——(a) 8.5 通过后删除；(b) 转常驻运维工具并把实现挪出 service 包。
- **处置**：`dedup_parity.go` 从 `internal/tagmanagement/service/auxlabel/` 删除，实现整体移入 `backend-go/cmd/aux-label-dedup-parity/`（`parity.go` + `matcher.go`），service 包侧导出全部删除、零残留；cmd 侧用导出的 `database.SemanticLabelMergeHalfvecIndexPredicate` 保证谓词逐字一致。代价：cmd 与服务侧两套召回实现，已在两侧写互指注释 + `semantic-labels.md` 登记「改服务侧 MUST 同步工具」，tasks 增 8.8 人工核对项。

---

## 覆盖清单（子线程实际核过的不变量）

| # | 不变量 / 检查 | 结果 |
| --- | --- | --- |
| 0 | design 落点复核 D1–D8 | 全部可解析，无「可删项」 |
| 1 | 旧向量缓存符号零残留 | 0 命中 |
| 1b | 过期注释删除（tasks 1.4） | 0 命中 |
| 2 | 归并路径不写向量列 | 仅 L3 新建、disable 清空、backfill 写向量；alias/merge 只 `Select("aliases").Updates` |
| 3 | 召回 WHERE 与部分索引谓词逐字一致 | 共用 `SemanticLabelMergeHalfvecIndexPredicate` 常量 |
| 4 | 精排用原列 `<=> ?::vector`，裁决用精确距离 | 召回 halfvec、精排原列、裁决用精排距离；halfvec 距离未进阈值判断 |
| 5 | L2 事务内零 embedder 调用 | 闭包只调 `mergeMatcher` |
| 6 | `SET LOCAL` 只在事务内；只读事务无写 | 成立 |
| 7 | raw SQL 参数化 | 绑定值全走 `?`；拼接只含 int 与包内常量 |
| 8 | 对照 `findSimilarByVector` / `ensureTopicTagHalfvecIndex` 范式 | 口径一致 |
| 9 | 迁移在事务内执行 | 默认 in-tx 路径，`_0002` 排在 `_0001` 之后 |
| 10 | Branch 3 先删新索引 | 成立 |
| 11 | 守卫挂载顺序 | 两个列守卫之后、`vectorDimEnsurers` 之前 |
| 12 | L1 map 先到先得与三口径 | 成立（跨类优先顺序差异属 D5 明示取舍） |
| 13 | 缓存增量一致性 | `applyDisable/applyMerge` 换指针安全；`applyCreate` 见 R-1 |
| 14 | `openspec validate --strict` | exit 1（R-5） |
| 15 | 制品迁移号一致 | 不一致（R-4） |
| 16 | 文档落位 | 索引段落与第 38 条红线格式合规 |
| 17 | 测试与任务映射 | tasks 2.2 五条、2.4 兜底、1.1–1.3 四路径、3.2 两条均有对应用例 |
