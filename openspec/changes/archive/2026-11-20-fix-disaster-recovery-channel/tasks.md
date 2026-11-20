## 1. 备份语义修复（L1：迁移历史进备份档）

- [x] 1.1 `scripts/db/backup-key-tables.sh`：排除清单移除 `schema_migrations`；同步更新脚本头部注释，说明它必须在档内的理由（它是恢复保真的必要输入，缺失会导致迁移链在有数据的库上重放），并修正「恢复时 AutoMigrate 自动重跑」这一错误表述，验证：`grep -c "schema_migrations" scripts/db/backup-key-tables.sh` 的 EXCLUDED 段内 0 命中
- [x] 1.2 同脚本：确认排除清单其余 14 张表的语义与顺序不变（`ai_call_logs`/`otel_spans`/`ai_embedding_cache`/`firecrawl_jobs`/`tag_jobs`/`embedding_queues`/`merge_reembedding_queues`/`notifications`/`discovery_runs`/`discovery_run_items`/`cross_board_relation_runs`/`topic_watch_hits`/`scheduler_tasks`/`reading_behaviors`），验证：`bash scripts/db/backup-key-tables.smoke.sh` 断言排除清单恰为这 14 张
- [x] 1.3 同脚本：dump 成功后于同目录写一份同时间戳的行数基线清单（`<档名>.counts.tsv`，每行 `表名\t行数`，覆盖全部被备份的表），供恢复侧逐表比对，验证：跑一次备份后 `ls backups/` 出现 `.dump` 与 `.counts.tsv` 配对文件
- [x] 1.4 同脚本：轮转逻辑同步处理基线文件（保留最近 7 份 dump，基线文件随对应 dump 一并删除，不留孤儿文件），验证：`bash scripts/db/backup-key-tables.smoke.sh` 断言轮转后 `backups/` 下 `.counts.tsv` 数与 `.dump` 数一致
- [x] 1.5 备份档体积影响核对：跑一次真实备份，记录档大小并与历史档对比（`schema_migrations` 仅数十行，预期增量 <100KB），验证：`ls -l backups/` 中新旧档大小差 < 200KB

## 2. 迁移幂等与维度无关（L2）

- [x] 2.1 `backend-go/internal/platform/database/postgres_migrations.go`：迁移 `20260403_0003` 的 `ADD COLUMN` 与 `ALTER COLUMN TYPE` 目标类型由 `vector(4096)` 改为**无维度限定的 `vector`**；`ALTER` 语句保留在 `withLockTimeout(db, "5s", ...)` 守卫内；`Description` 同步更新为「确保向量列存在且维度无关（目标类型无维度限定）」，验证：`grep -n "vector(4096)" backend-go/internal/platform/database/postgres_migrations.go` 0 命中
- [x] 2.2 **专项实测 `ALTER vector(2560)` → `vector` 的可行性**：在一张含非空向量的表上执行该 ALTER，确认成功且存量数据维度不变（这是 design D2 的关键假设，未实测前不得进入实现），验证：实测命令与输出记入 `docs/reference/database/RESTORE.md` 附录；**若 pgvector 不支持该转换**，改为「列已是 `vector` 则跳过；否则跳过并记警告」并在 design Risks 中标注退路生效
- [x] 2.3 迁移幂等单测：在含非空向量、维度非目标值的表上重放该迁移 → 成功且数据维度不变；列已是无限定 `vector` 时重放 → 成功且列类型不变，验证：`cd backend-go && go test ./internal/platform/database/... -run VectorDimension` 退出码 0

## 3. 恢复脚本（L3）

- [x] 3.1 新建 `scripts/db/restore-key-tables.sh`：命令行接口（`--dump <档>`、`--target-db <库>`、`--allow-prod`、`--keep`、`--report <输出>`）、前置校验（档存在、`pg_restore -l` 可列目录）、分步日志与退出码约定，验证：`bash scripts/db/restore-key-tables.smoke.sh` 覆盖参数校验分支
- [x] 3.2 落库闸门：默认新建一次性独立恢复库；目标库名等于生产库名（`syntopica`）时拒绝执行并非零退出，除非 `--allow-prod` + 交互二次确认，验证：smoke 脚本断言「指定生产库名 → 非零退出且生产库行数不变」
- [x] 3.3 扩展预装：restore 前于目标库创建 `vector` 与 `pg_trgm`；创建失败则在 restore 之前非零退出（不留半恢复库），验证：smoke 脚本断言「扩展缺失 → 明确报错且未进入 restore」
- [x] 3.4 `pg_restore` 执行：TOC 过滤掉 `SCHEMA public` 条目；restore 失败非零退出，验证：smoke 脚本断言「过滤后无 SCHEMA public 冲突」
- [x] 3.5 count 完整性校验：读 dump 同目录的 `.counts.tsv` 基线，逐表比对恢复库行数；不一致时报告基线值与实际值并非零退出，验证：smoke 脚本断言「基线一致 → 零退出」「基线不一致 → 列出差异表并非零退出」
- [x] 3.6 迁移状态对账：解析 `backend-go/internal/platform/database/postgres_migrations.go` 中各迁移闭包触及的表名，与恢复库实际表集合比对，列出「迁移已应用但表不在档内」的缺口；缺口只进报告不影响退出码，验证：smoke 脚本断言「检出缺口并列出」「无缺口时列表为空」
- [x] 3.7 报告输出：脚本产出人读报告（档来源、目标库、各表 count 比对结果、迁移对账缺口、耗时），验证：smoke 脚本断言报告含上述小节

## 4. 测试

- [x] 4.1 备份脚本冒烟（新增 `scripts/db/backup-key-tables.smoke.sh`）：排除清单内容、迁移历史在档内、基线文件产出、轮转配对，验证：`bash scripts/db/backup-key-tables.smoke.sh` 退出码 0
- [x] 4.2 恢复脚本冒烟（新增 `scripts/db/restore-key-tables.smoke.sh`）：参数校验、落库闸门、扩展预装、TOC 过滤、count 校验、迁移对账、报告输出，验证：`bash scripts/db/restore-key-tables.smoke.sh` 退出码 0
- [x] 4.3 迁移单测（新增 `backend-go/internal/platform/database/vector_dimension_migration_test.go`）：维度迁移幂等与维度无关，验证：`cd backend-go && go test ./internal/platform/database/... -run VectorDimension` 退出码 0
- [x] 4.4 影响包测试（只跑受影响包），验证：`cd backend-go && go test ./internal/platform/database/...` 退出码 0
- [x] 4.5 后端质量门禁，验证：`cd backend-go && golangci-lint run ./...` 退出码 0、`go vet ./...` 退出码 0、`go build ./...` 退出码 0

## 5. 文档

<!-- doc-impact: database, deployment -->

- [x] 5.1 新增 `docs/reference/database/RESTORE.md`：恢复流程（前置条件 → 分步命令 → 每步期望结果 → 验证清单 → 失败时的回滚/换档路径）+ 已知失败模式与对策（`gin_trgm_ops does not exist`、迁移维度冲突、`schema_migrations` 缺失）+ 实测耗时量级（restore 约 35min / dump 约 10min，SD 卡 1.7MB/s）+ 2.2 的 ALTER 可行性实测记录
- [x] 5.2 `docs/reference/database/_index.md`：登记 `RESTORE.md` 条目
- [x] 5.3 `docs/reference/deployment.md`：补「备份与恢复」小节（指向 RESTORE.md，说明每日备份档位置与恢复入口）
- 无 flow 影响：本 change 为纯数据层/运维层（备份脚本 + 迁移闭包 + 恢复流程），不触及任何业务 flow 的生成/编排流程（`backend-go/internal/` 业务域与 `front/` 零改动），按《开发执行规范》§12.2 豁免 flow 变更溯源

## 6. 验证

- [x] 6.1 `bash scripts/db/backup-key-tables.smoke.sh` → 退出码 0
- [x] 6.2 `bash scripts/db/restore-key-tables.smoke.sh` → 退出码 0
- [x] 6.3 `cd backend-go && go test ./internal/platform/database/...` → 退出码 0，输出各包 `ok`
- [x] 6.4 `cd backend-go && golangci-lint run ./...` → 退出码 0
- [x] 6.5 `cd backend-go && go vet ./...` → 退出码 0
- [x] 6.6 `cd backend-go && go build ./...` → 退出码 0
- [x] 6.7 `openspec validate fix-disaster-recovery-channel --strict` → 退出码 0
- [x] 6.8 `bash scripts/harness/scenario-trace.sh openspec/changes/fix-disaster-recovery-channel` → 退出码 0
- [x] 6.9 `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-disaster-recovery-channel` → 退出码 0
- [x] 6.10 `bash scripts/harness/check-standards.sh --change fix-disaster-recovery-channel` → 退出码 0
- [x] 6.11 **真档恢复演练**：用 `backups/` 里最新一档恢复到一次性独立库 → count 校验零差异 → 迁移对账缺口列表为空或已人工判读 → 在该恢复库上启动后端 → `/health` 返回 200（证明「恢复出来的库能起来」这条主张成立），验证：演练记录写入 `docs/reference/database/RESTORE_DRILL.md`
- [ ] 6.12 **历史档恢复演练**（**关键点已验证，/health 未取到 → 移交**）：2026-11-05 演练② 用 `pg-key-20261104-040001.dump`（L1 前产出、无 `schema_migrations`）恢复 → 迁移链从零重放**无维度冲突**（`vector(2560)` → `vector` 的 ALTER 131s 成功），本条主张成立；但迁移链在 `20260417_0002` search_vector 全表回填处耗时小时级，后端 `/health` 200 未取到。演练记录见 `docs/reference/database/RESTORE_DRILL.md` 演练②与「遗留」节；剩余项（search_vector 增量回填 + 补测 /health）移交后续 change，见 `review.md` R-3
- [x] 6.13 **生产库未被触碰核对**：演练前后记录生产库关键表行数，两者完全一致，验证：演练记录含前后行数对比
- [x] 6.14 `bash scripts/harness/review-trace.sh openspec/changes/fix-disaster-recovery-channel` → 退出码 0（归档前实测：建账 4 条通过）

### Scenario → 测试文件

| Scenario | 测试文件 |
| --- | --- |
| 默认恢复到新建独立库 | `scripts/db/restore-key-tables.smoke.sh` |
| 目标库为生产库时拒绝执行 | `scripts/db/restore-key-tables.smoke.sh` |
| 预装后备份档可完整恢复 | `scripts/db/restore-key-tables.smoke.sh` |
| 扩展缺失时明确报错而非中途失败 | `scripts/db/restore-key-tables.smoke.sh` |
| 行数一致时校验通过 | `scripts/db/restore-key-tables.smoke.sh` |
| 行数不一致时报告并以非零退出码结束 | `scripts/db/restore-key-tables.smoke.sh` |
| 迁移历史与表集合一致时无缺口 | `scripts/db/restore-key-tables.smoke.sh` |
| 检出被排除表导致的迁移缺口 | `scripts/db/restore-key-tables.smoke.sh` |
| 文档给出的步骤可直接执行 | 人工：`docs/reference/database/RESTORE.md`（按文档逐步实跑 6.11） |
| 演练留痕可查 | 人工：`docs/reference/database/RESTORE_DRILL.md` |
| 演练失败被记录 | 人工：`docs/reference/database/RESTORE_DRILL.md` |
| 排除清单内的表不进备份档 | `scripts/db/backup-key-tables.smoke.sh` |
| 迁移历史进入备份档 | `scripts/db/backup-key-tables.smoke.sh` |
| 新建业务表自动被备份 | `scripts/db/backup-key-tables.smoke.sh` |
| SET NOT NULL 迁移重复执行不报错 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |
| 重复执行 InitDB 不产生迁移错误 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |
| constrain helper 尊重 notNull=false 声明 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |
| 向量列维度迁移不因存量维度不同而失败 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |
| 向量列类型迁移重复执行不改变列类型 | `backend-go/internal/platform/database/vector_dimension_migration_test.go` |

- [x] 6.15 Review：只读审查全量制品与 diff（重点审：备份语义变更是否动了不该动的排除项、恢复脚本是否存在误伤生产库的路径、迁移改写是否真的幂等），意见按 High/Medium/Low 处置后在 `review.md` 建账，验证：`bash scripts/harness/review-trace.sh openspec/changes/fix-disaster-recovery-channel` 退出码 0
