## 1. 删除路径补齐引用清理（feed-delete-referential-integrity）

- [x] 1.1 `ReaderRepository` 新增引用表容错清理原语：`reading_behaviors`（有 Go 模型）与 `user_preferences`（模型已删除，走参数化 `Table(...)`/`Exec`）的按条件清理入口；表存在性探测沿 `existingArticleDependents` 模式（探测为假或探测失败 → 跳过该表不报错；列缺失 → DELETE 自身报错并回滚事务）
- [x] 1.2 `DeleteFeedCascade` 补 `user_preferences`（按 `feed_id`）清理，位置与既有 `reading_behaviors` 清理并列、在 `deleteArticlesWithRefs` 之前
- [x] 1.3 `DeleteCategoryCascade` 补 `reading_behaviors`（按分类下 `feed_id` 集合）与 `user_preferences`（`feed_id ∈ 分类下订阅源` 或 `category_id = 分类`）清理，位置在 `deleteArticlesOfFeedsWithRefs` 之前
- [x] 1.4 同步修订 repository.go 既有注释：`deleteArticleDependents` 的「reading_behaviors is untouched, because its NO ACTION foreign key already fails such a delete today」与 `DeleteCategoryCascade` 的「reading_behaviors / user_preferences are deliberately untouched … already fail this deletion today」随行为改变改写（现在由删除路径同事务清理）
- [x] 1.5 集成测试（PG，`backend-go/internal/reader/repository/feed_delete_refs_test.go`）：表存在 + 有引用行 → 删源成功且引用行消失；表存在 + 分类场景（含纯分类级偏好行）→ 删分类成功；引用表缺失 → 跳过不报错；无引用行 → 照常；分类外数据不受影响；其他订阅源不受影响

## 2. 序列漂移启动自愈（sequence-drift-self-heal）

- [x] 2.1 纯函数判定（`backend-go/internal/platform/database/sequence_heal.go`）：`(lastValue int64, isCalled bool, maxID int64) → (needsHeal bool, target int64)`，语义按 design D5（下一次分配值 ≤ `max(id)` 即漂移；目标 `max(id)+1`；领先时不修）
- [x] 2.2 平台层实现：枚举候选表（有 `id` 列且存在 `<table>_id_seq`，schema 限定 `public`）→ 逐表读 `last_value`/`is_called` 与 `MAX(id)` → 漂移表执行 `setval(seq, max+1, false)` → WARN 日志（表名 / 序列名 / 漂移前下一次分配值 / `max(id)` / 对齐目标 / 漂移根因提示）
- [x] 2.3 失败语义：单表检测或 `setval` 失败只记日志继续；候选表枚举（目录查询）失败同样只记日志；均不阻塞启动
- [x] 2.4 `StartRuntime` 接线：位置在 `resetStaleStates()` 之后、调度器与 HTTP 启动之前（runtime.go）
- [x] 2.5 单测（`sequence_heal_unit_test.go`，纯逻辑无外部依赖，按 testing.md 分层命名；含一条内存 SQLite 的错误分支）：六类边界组合（已调用且落后 / 已调用且领先 / 未调用且落后 / 未调用且领先 / 空表 max=0 / 恰好等于 max）
- [x] 2.6 集成测试（`sequence_heal_pg_test.go`，testcontainer PG）：人为 `setval` 至落后值 → 触发自愈 → 插入新行拿到 `max(id)+1` 不冲突；已对齐时二次执行零写入（幂等，序列不回退）；日志含修复记录

## 3. 文档

<!-- doc-impact: flow, deployment, database -->

- [x] 3.1 `docs/reference/flow/reading.md` 约束 #6：把「有 `reading_behaviors` / `user_preferences` 子行时该删除仍会因 NO ACTION 外键失败（既有行为）」改写为「引用行由删除路径在同一事务内显式清理，删源/删分类不再被 NO ACTION 外键阻断」；保留「删分类的破坏性语义必须对用户可见（前端文案一致）」与「topic_tags 孤儿不归删除路径清理」两条不变
- [x] 3.2 `docs/reference/deployment.md`：在恢复/破坏性迁移相邻节补「sequence 漂移」小节——成因（`TRUNCATE ... RESTART IDENTITY` + `pg_restore -t` 漏 SEQUENCE SET）、启动自愈行为、手工核对/兜底 SQL（只读比对 + `setval` 形式）
- [x] 3.3 `deployment.md` 同一节内补：带 `-t` 表过滤的恢复会漏序列状态（`setval` 不受事务回滚保护，无法用 `BEGIN/ROLLBACK` 预演）
- [x] 3.3b `docs/reference/database/tables/_conventions.md`：DB 级外键表「NO ACTION」行（现写「被引用行有这些子行时删除会 FK 报错」）更新为「删除路径必须在同事务内先清引用行」并注明 `DeleteFeedCascade` / `DeleteCategoryCascade` 的清理范围；B-2 表 `user_preferences` 两行标注「表已废弃（无 model）、可能残留、清理按存在性探测容错」
- [x] 3.3c `docs/reference/database/DATA_LIFECYCLE.md`：§进程启动状态重置补 `sequence` 漂移自愈项；`preference_update` 调度器行（已随 preference-vector-feed-discovery 从代码删除）与 §明确无自动清理表的 `user_preferences` 条目补废弃标注
- [x] 3.3d `docs/reference/database/tables/content.md`：ER 图的 `user_preferences` 关系标注「（已废弃）」（与既有 `ai_summaries` 标注风格一致）
- [x] 3.4 `docs/reference/` 内迁移「破坏性迁移被跳过却记 applied」权衡的记录（`db-migration-safety` spec 明文要求记 applied 以避免重复告警 → 废弃表在未开启开关的环境永久残留，由删除路径容错而非依赖其消失），标注后续独立 change 入口
- [x] 3.5 `backend-go/DATABASE.md` / `DATABASE_SUMMARY.md` 的 `user_preferences` 条目补「已废弃」标注（最小改动）。**顺带发现**：两文件整体过时（称「支持 7 张表」、用 `sqlite_master` 判断、指向 `pkg/database/db.go` 旧路径；实际约 60 张表 / PostgreSQL / `internal/platform/database/`）——属独立文档漂移，建议另开清理项（已在完工汇报点出）
- [ ] 3.6 **归档后**（§12.2 强制）：在 `flow/reading.md`「变更溯源」表补本 change 行（`<date>-fix-feed-delete-and-sequence-drift` + 一句话说明）

## 4. 测试

测试分层与落点（为什么这么分）：

- **删除路径**（`feed-delete-referential-integrity`）：内存 SQLite **无 FK 语义**，根本测不出「删除被外键挡住」这个真问题，因此全部走 testcontainer PG 集成测试（`testutil.SetupTestDB`）。其中「引用表存在」场景需在测试内 `CREATE TABLE user_preferences` 并补 NO ACTION FK（golden schema 由 `runTestMigrations` 构建，测试环境 `MIGRATIONS_ALLOW_DESTRUCTIVE=1` 会真的 DROP 该表）；`reading_behaviors` 的两个历史 FK 也要在测试内还原（`DisableForeignKeyConstraintWhenMigrating` 使新建库不带 FK）——否则用例会在「本该报错的库」上通过，那正是本次故障的形态。
- **序列漂移判定**（`sequence-drift-self-heal`）：判定与对齐目标是纯函数，SQLite 环境即可覆盖全部边界；真实 `setval`/`nextval` 语义只能在 testcontainer PG 验证（design D6），两层都覆盖。
- **既有回归**：`article_refs_test.go`（daily-report jsonb 引用维护、文章从属行清理、不越权清理孤儿标签）必须全绿——本次只新增引用行清理步骤，不改既有语义。

## 5. 验证

| Scenario | 测试文件 |
| --- | --- |
| 有 user_preferences 引用的订阅源可删除 | backend-go/internal/reader/repository/feed_delete_refs_test.go |
| 有 reading_behaviors 引用的订阅源可删除 | backend-go/internal/reader/repository/feed_delete_refs_test.go |
| 引用表缺失时跳过而不报错 | backend-go/internal/reader/repository/feed_delete_refs_test.go |
| 无引用行的订阅源照常删除 | backend-go/internal/reader/repository/feed_delete_refs_test.go |
| 清理失败整体回滚 | backend-go/internal/reader/repository/feed_delete_refs_test.go |
| 分类下有阅读行为与偏好引用时可删除 | backend-go/internal/reader/repository/feed_delete_refs_test.go |
| 分类级偏好行（无 feed 归属）一并清理 | backend-go/internal/reader/repository/feed_delete_refs_test.go |
| 分类外数据不受影响 | backend-go/internal/reader/repository/feed_delete_refs_test.go |
| 其他订阅源的文章与引用不受影响 | backend-go/internal/reader/repository/feed_delete_refs_test.go |
| daily-report 引用同步维护 | backend-go/internal/reader/repository/article_refs_test.go |
| 不越权清理孤儿标签 | backend-go/internal/reader/repository/article_refs_test.go |
| 落后的序列被对齐到 max(id)+1 | backend-go/internal/platform/database/sequence_heal_pg_test.go |
| 未漂移的序列不被改动 | backend-go/internal/platform/database/sequence_heal_unit_test.go |
| 空表与从未调用过的序列不被误修 | backend-go/internal/platform/database/sequence_heal_unit_test.go |
| 重复执行不使序列倒退 | backend-go/internal/platform/database/sequence_heal_unit_test.go backend-go/internal/platform/database/sequence_heal_pg_test.go |
| 五张已知漂移表全部被覆盖 | backend-go/internal/platform/database/sequence_heal_pg_test.go |
| 修复留痕 | backend-go/internal/platform/database/sequence_heal_pg_test.go |
| 单表失败不影响启动 | backend-go/internal/platform/database/sequence_heal_pg_test.go |
| 目录查询失败不影响启动 | 人工：代码审查探测失败分支 + 单测覆盖错误返回分支（验证命令 3） |
| 判定边界被单测覆盖 | backend-go/internal/platform/database/sequence_heal_unit_test.go |
| 真实序列行为由集成测试覆盖 | backend-go/internal/platform/database/sequence_heal_pg_test.go |

> 映射说明（review Low-3 处置）：「五张已知漂移表全部被覆盖」由**机制泛化**覆盖——实现按 catalog 全量枚举 + 同一 `SequenceNeedsHeal` 判定，不按表名分支；PG 用例实测钉住两侧（漂移表被修 = `feeds`、未漂移表不被碰 = `categories`），五张表的字面状态由真机复验（下文 ① 与真机复验段）与纯函数边界矩阵共同佐证。

1. `cd backend-go && golangci-lint run ./internal/reader/... ./internal/platform/database/... ./internal/app/...` → 0 error
2. `cd backend-go && go vet ./...` → 无输出（exit 0）
3. `cd backend-go && go test -short ./internal/reader/repository/ ./internal/platform/database/` → 全绿（`-short` 跳过 testcontainer 用例，纯函数与 SQLite 用例通过）
4. `cd backend-go && go test ./internal/reader/repository/ ./internal/platform/database/`（Docker 可用时）→ 全绿，含「删源/删分类不再被 FK 阻断」与「序列漂移被对齐」集成用例
5. `bash scripts/harness/change-scope.sh` → 判定影响包（预期 reader/repository + platform/database + app），仅跑受影响范围
6. 真库复验（人工，先 `count(*)` 留底）：① 删除一个此前删不掉的订阅源（`user_preferences` 有引用的 18 个之一）→ 删除成功、引用行消失、文章同步删除；② 人为把某表序列 `setval` 到落后值后重启后端 → WARN 日志记录修复、`SELECT last_value FROM <seq>` 等于 `max(id)+1`；③ 重启两次 → 第二次无修复日志（幂等）
7. `openspec validate fix-feed-delete-and-sequence-drift --strict` → 通过
8. 归档前：`bash scripts/harness/doc-impact.sh verify` + `bash scripts/harness/check-standards.sh` → 通过
9. **域外红已记账**（§11.4）：`bash scripts/harness/test-patrol.sh --report` 可见两条 be-skeleton 欠账——`backend-go/internal/platform/database::TestBoardLevelAnalysisScopeMigration`（期望 `reference_roles`，模型已随 restructure-settings-navigation(dddc3408) 删除）与 `::TestModelTagConstraints_MaterializedInDB`（本包测试二进制未链接 `internal/platform/scheduler`），两者在 `git archive HEAD` 干净树上同样 FAIL，均非本 change 引起，已按 AGENTS.md 欠账纪律登记（context=fix-feed-delete-and-sequence-drift）

> 实测证据（2026-09-28）：`go test -count=1 -skip 'TestBoardLevelAnalysisScopeMigration|TestModelTagConstraints_MaterializedInDB' ./internal/platform/database/` 全绿（含本 change 的 4 个 PG 用例）；`go test -short -count=1 ./internal/reader/... ./internal/platform/database/ ./internal/app/` 全绿。
>
> **同轮撞见的第三条域外红（已记台账）**：hotfix 触及 `internal/tagmanagement/service/sourcestats` 后，增量门禁跑了整棵 `./internal/tagmanagement/...`，撞上 `merge` 包的已知偶发红（`TestStartFullScan_RejectsConcurrent` 起的后台 goroutine 里 `repository.Repo` 为 nil → `DB()` nil 解引用 panic 打崩进程；3 跑 1 红）。2026-09-20 已在 test_debt 有同源 open 条目，本轮又按 `--register ... --context fix-feed-delete-and-sequence-drift` 登记一次（domain=be-tagmanagement）。
>
> **本轮 hotfix（独立提交 `126ba347`，属 2026-09-28 全站 pending 事故的紧急修复，按纪律 24h 内需独立 change 归档）**：`internal/platform/database/db_test.go`（无 short 守卫、真库 DSN + `InitDB` → 真库 AutoMigrate）已删除；`sourcestats_pg_test.go` 的真库只读抽检改为纯 `gorm.Open`（不再调 `InitDB`）。该事故与本 change 无关（本 change 未动 `semantic_labels` 与测试连接路径），但与本 change 同一轮排查中发现并处置。

> **真机复验（2026-09-28，本机 :5100，`bash scripts/dev/start-dev.sh --restart back` 加载新代码）**：
> ① **序列自愈**：先把 `categories_id_seq` `setval` 到 1（`is_called=false`，表内 `max(id)=5`）→ 重启后端 → 日志 `level=WARN msg="Sequence drift repaired: table=categories sequence=categories_id_seq next_would_be=1 max_id=5 new_next=6; cause is usually a TRUNCATE ... RESTART IDENTITY followed by a table-filtered pg_restore that skipped SEQUENCE SET"` + `level=INFO msg="Sequence drift self-heal repaired 1 sequence(s) on startup"`；复查 `last_value=6, is_called=f` ✅
> ② **删源**（造测试数据：`feeds.id=38` + 1 文章 + 1 `reading_behaviors` + 1 `user_preferences`）→ `DELETE /api/feeds/38` → `HTTP 200 {"success":true}`；复查 `feeds` / `articles` / `reading_behaviors` / `user_preferences` 对 38 计数全为 0 ✅（修复前同一场景必报 500 + `fk_user_preferences_feed` 全量回滚）
> ③ **删分类**（造测试分类 6 + 其下 feed 39 + 引用行）→ `DELETE /api/categories/6` → `HTTP 200 {"success":true}`；复查 category/feed/articles/behaviors/prefs 对 6/39 计数全为 0、`categories` 总数回到 5、无孤儿偏好行 ✅
> ④ **用户原始故障闭环**：用户已自行成功新增 oschina 订阅源（`feeds.id=36`，`http://47.110.71.194:1200/oschina/event`）——新增路径真机可用。
> 测试数据均经 API 正常删除，无残留（`feeds` 总数 = 原有 24 + 用户新增 1 = 25，测试源 0 条）。
