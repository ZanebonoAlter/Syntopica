## 1. Bug 修复：alias 写路径列级更新

- [x] 1.1 `auxiliary_label_service.go` `addAlias`：`Save(label)` 改列级更新，只动 aliases 列；验证：新增回归用例「L2 归并后 embedding/merge_embedding 原样保留」（见 §3）通过
  - 实现注记：tasks 原文的 map 形式 `Update("aliases", label.Aliases)` 会踩 GORM 坑——map 路径不走字段 serializer（callbacks/update.go map 分支 raw value 直通 driver），[]string 直写 jsonb 列报类型错；改用等价的 `Select("aliases")` + struct `Updates`（同为只动 aliases 一列，struct 路径经 field.ValueOf 序列化 jsonb，updated_at 照常自动维护）
- [x] 1.2 `MergeAuxiliaryLabelAlias`：`tx.Save(&target)` 改列级更新（同上 struct+Select 形式；source 侧既有的 disabled+NULL Updates 保持不变——那是合法的向量清理路径）；验证：新增回归用例「手动 alias 合并后目标向量不变」通过
- [x] 1.3 审计 `semantic_labels` 表全部写点（grep 全量 + 逐点人工确认，2026-09-27）：
  - **生成路径（向量列合法出现）**：L3 新建 `Create`（带双向量）；组合标签新建；disable 置 NULL（DisableAuxiliaryLabel / gcCleanup(disable) / deleteSemanticBoard / MergeAuxiliaryLabelAlias source 侧 / migration.go:187，均为红线 11 合法清理）；回填写入（本 change backfillRow，map Updates 只含缺失列）；board embedding 生成（updateSemanticBoard label/desc 变更重生成、board_upgrade_handler.go:327）
  - **alias 写路径（本 change 已列级化）**：addAlias / MergeAuxiliaryLabelAlias target 侧
  - **其余写点（不碰向量列或巧合式安全，留痕不扩散 scope）**：AttachAuxiliaryLabels / migration.go:184 / merge ref_count 三处 `UpdateColumn/Update("ref_count")` 只动计数列 ✓；migration.go:135 RunAuxLabelDupMerge `tx.Save(&target)`（target 为 tx.First 全列加载，巧合式安全，一次性迁移）；handler updateSemanticBoard L309 `Save(&board)`（First 全列加载且 board 向量语义即重生成）；composite addCompositeAlias L513 `Save(best)`（best 的 Select 含 embedding 不含 merge_embedding，composite 的 merge_embedding 设计恒 NULL（红线 15），当前无害；同型陷阱留痕，建议后续统一列级加固）

## 2. 死向量回填端点

- [x] 2.1 `AuxiliaryLabelService` 新增 `BackfillMissingEmbeddings(ctx)` + `CountMissingEmbeddings(ctx)`：扫描 `status='active' AND label_type='auxiliary' AND (embedding IS NULL OR merge_embedding IS NULL)`，游标分页（id > lastID）每批 50 行，逐行按 L3 同款生成（merge←label、storage←"label: description"或label）**只补缺失的那列**，行级失败记 warn 跳过；验证：fake embedder 单测覆盖「只补 NULL 列 / 已有列不动 / 单行失败跳过 / 重触发幂等」四断言
- [x] 2.2 handler 新增 `POST /api/auxiliary-labels/backfill-embeddings`（挂 `auxiliary` 路由组，`/gc` 邻居）：同步查 pending 数、detached goroutine 跑回填、立即返回 202 `{pending: N}`；验证：httptest 断言 202 + pending 计数与库中死向量行数一致
- [x] 2.3 无死向量时端点为空操作（pending=0、零 LLM 调用、不起 goroutine）；验证：fake embedder 计数为 0 的用例通过

## 3. 测试

<!-- test-cases: change 目录 test-cases.md（主链路表 + 变体走查 + 效果核对），spec Scenario 全量落点 -->

- [x] 3.1 按 `test-cases.md` 主链路表补齐全部断言（6 个 Scenario：L2 归并向量保持 / 手动合并向量保持 / active⇒非NULL 不变量 / 回填补齐 / 回填不动已有列 / 回填失败跳过 + 空操作）：TestAuxiliaryLabelServiceAddAliasPreservesVectors（含 3 变体串行参数化）/ MergeAuxiliaryAliasPreservesTargetVectors / BackfillMissingEmbeddingsFillsNulls / BackfillSkipsExistingColumns / BackfillRowFailureSkips / BackfillNoopHealthy + handler TestAuxLabelBackfillEndpointNoopHealthy / PendingCountAndAsyncFill；层选：service 层 testcontainer PG（`testutil.SetupTestDB`），禁 SQLite；断言用 requireSameVector（按元素 float32 比对——pgvector 存 float4，回读文本是最短表示，字符串逐字节比较会被格式归一化误伤）
- [x] 3.2 变体走查（test-cases.md 变体节全部有答案或划除留痕）：重复触发回填→TestBackfillRowFailureSkips 二次断言（pending 只含残余 1）；并发归并同标签→留痕（丢 alias 竞态与旧 Save 行为一致，非本 change 引入；向量列已安全）；超长 label→留痕（L3 无截断，与生成路径一致）；其余输入/时间窗口/UI 变体划除
- [x] 3.3 效果核对（真库量化，见 test-cases.md 效果节）：部署后触发回填前后 `SELECT count(*) FROM semantic_labels WHERE label_type='auxiliary' AND status='active' AND embedding IS NULL` 从 ~1505 降至 0；抽一个版块构成标签（如高市早苗 id=113731）确认向量非 NULL
  - 实测（2026-09-27 23:10~23:57）：新代码经 start-dev.sh --restart 部署，端点首轮 202+`{pending:1505}` 与库内死向量数一致；期间自托管 embedding 服务（10.11.12.111:8081）一度不可达，首轮 fixed=922/skipped=583（行级跳过语义真库验证），服务恢复后重触发第二轮 583→0；终态 active aux 17515 行、死向量 0，锚点 id=113731（高市早苗，ref 47）与 id=3238（日本央行，ref 23）双向量非 NULL，日本新闻版块构成 30 个 aux 全部恢复。可选 rematch-all 留给用户决定

## 4. 文档

<!-- doc-impact: flow, api -->
<!-- doc-impact-excuse: database=唯一改动是 testutil.go 测试基建修复（blank import discovery/models 让 init 注册模型，修复 4421c068 解耦重构漏改 testutil 致全部 PG testcontainer 测试红的存量回归）；无 schema/迁移变更，database 文档无需更新 -->

- [x] 4.1 `docs/reference/flow/semantic-board.md`：红线 2 追加实现事实（addAlias/alias-merge 已列级更新，fix-alias-save-vector-wipe）；「队列与回填运维」表新增 `auxiliary-labels/backfill-embeddings` 行；验证：`bash scripts/harness/doc-impact.sh verify openspec/changes/fix-alias-save-vector-wipe` → 通过（声明: flow, api）
- [x] 4.2 `docs/reference/api/semantic-boards.md`：路由表 L27 + 详节 `### POST /auxiliary-labels/backfill-embeddings`（无请求体、202+pending、幂等/只补缺列/行级容错语义）；验证：grep 到新端点行且路径 `/api/auxiliary-labels/backfill-embeddings` 与路由注册（board_crud_handler.go auxiliary 组）一致

## 5. 验证

- [x] 5.1 `cd backend-go && golangci-lint run ./internal/tagmanagement/... ./internal/platform/testutil/...` → 退出码 0（无新告警）
- [x] 5.2 `cd backend-go && go vet ./internal/tagmanagement/... ./internal/platform/testutil/...` → 退出码 0
- [x] 5.3 `cd backend-go && go test ./internal/tagmanagement/service/auxlabel/... ./internal/tagmanagement/handler/...` → 全部 PASS（只跑影响包，禁全量）
- [x] 5.4 `cd backend-go && go build ./...` → 编译通过
- [x] 5.5 `bash scripts/harness/check-standards.sh` → 通过 213/213；`bash scripts/harness/doc-impact.sh verify openspec/changes/fix-alias-save-vector-wipe` → 通过（database 域启发式误报已加 `doc-impact-excuse` 留痕：唯一改动是 testutil 测试基建修复）
- [x] 5.6 归档门禁复核（§11）：spec delta / test-cases / 验证节「命令+期望」齐全 → `openspec validate fix-alias-save-vector-wipe` 通过

> 附带修复与登记（非本 change 范围但阻断验证）：① testutil.go blank import discovery/models——修复 4421c068 解耦重构漏改 testutil 致全部 PG testcontainer 测试红的存量回归；② platform/database 包两个同类漏网（TestBoardLevelAnalysisScopeMigration 缺 reference_roles / TestModelTagConstraints_MaterializedInDB 缺 scheduler_tasks 表）已登记 test-patrol 台账待后续统一修复。

## 部署后影响与操作（必读）

- **行为变化**：新别名归并不再清向量（止血）；回填端点上线后存量 1505 个死向量标签可恢复。
- **需要手动操作**：部署后 `curl -X POST http://localhost:5100/api/auxiliary-labels/backfill-embeddings`，重触发至返回 `pending:0`；可选跑 `POST /api/semantic-boards/rematch-all` 让版块挂载基于恢复后的向量重新竞争。
- **旧数据降级**：无破坏性变更；回填写入的是 L3 同款合规向量，回滚无需清理。
