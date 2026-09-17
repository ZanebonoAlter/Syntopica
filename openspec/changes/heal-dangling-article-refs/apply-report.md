# apply-report — heal-dangling-article-refs

范围：tasks.md §1-§6（代码 + 测试）+ §4.3 验证 SQL + §8.1/8.2 门禁。**未做** §7 文档（主线程负责）、未 commit、未触碰真库数据、未改前端。

## 1. 逐节点交付（文件:符号）

### §1 引用维护器（新包 `backend-go/internal/platform/articlerefs/`）
- `articlerefs.go`：包文档（为什么「只剪不映射」+ 删除路径必须在删行前同事务调用）；`refsArraySQL`（`jsonb_typeof` CASE 守卫，防 22023）；`parseRefIDs`（非 JSON 数组 → dropped=true，脏元素剔除）；`rewriteRefIDs`（保序 + 去重，首个为准）；`marshalRefIDs`/`updateRefs`（`?::jsonb` 绑定）；`loadRefRowsContaining`/`loadRefRowsAfter`/`rewriteRows`；`idStrings`/`uniqueIDs`；`DefaultBatchSize=500`、`articleIDChunk=1000`。
- `rewire.go`：`RewireArticleRefs(db, oldID, keeperID) (int64, error)`、`PruneArticleRefs(db, ids) (int64, error)`。
- `repair.go`：`NormalizeThreadRefs`、`PruneDanglingRefs(db, batch) (rows, refs int64, err error)`、`CountDanglingArticleRefs`、`ExistingArticleIDs`（新增导出，见 §4 偏差 1）。
- 存否判断统一 `a.id::text = e.elem` 文本比对（无 `::bigint` 强转），命中过滤用 `jsonb_array_elements_text(CASE WHEN jsonb_typeof(...)='array' THEN ... ELSE '[]'::jsonb END)` + `e.elem IN (SELECT jsonb_array_elements_text(?::jsonb))`（单参数、无 jsonb `?` 操作符与 GORM 占位符冲突）。

### §2 删除路径接线
- `internal/platform/database/postgres_migrations.go:mergeDuplicateArticleGroup`：`DELETE FROM articles` **之前**调 `articlerefs.RewireArticleRefs(db, r.ID, keeper)`，错误上抛（事务回滚）。keeper 选择/边重指/计数重算语义未动。
- `internal/reader/repository/repository.go`：新增私有核心 `deleteArticlesWithRefs(tx, feedID)`（pluck 文章 id → `PruneArticleRefs` → 删行）；`DeleteArticlesByFeed` 走事务；`DeleteCascadeByFeed` 复用同一核心；新增 `DeleteFeedCascade(feedID)`（事务：删 `reading_behaviors` → prune + 删文章 → 删 feed 行）。
- `internal/reader/handler/feed_handler.go:DeleteFeed`：去掉内联裸 DB 调用（原为「删 behaviors + `Delete(&feed)` 靠 FK 级联删文章」），改调 `repository.Repo.DeleteFeedCascade`。净数据结果等价（文章仍被删），多出来的只是引用维护。

### §3 日报写路径
- `internal/topicgraph/service/daily_report_article_filter.go`（新）：`applyExistingArticleRefs`（纯函数）+ `filterVanishedArticleRefs(db, threadsByCluster map[int][]repository.Thread)`（整份日报**一次**存在性探测，失败 `logging.Warnf` 后按原候选写入）。
- `daily_report_orchestrator.go`：组装 thread 批次**前**调用 `filterVanishedArticleRefs(repository.Repo.DB(), threadsByCluster)`；`th.TagIDs` / `th.RelatedArticleIDs` 改走 `marshalJSONArray`。
- `daily_report_merge.go`：`mergedTagIDs` 改走 `marshalJSONArray`（消灭 JSON `null` 写入口）。

### §4 存量修复迁移 + 验证 SQL
- `postgres_migrations.go`：`healDanglingArticleRefsMigration()`（Version `20260917_0002`，注册在 `postgresMigrations()` 链尾，`migrationsSorted()` 按版本排序），`Up` = 表缺失守卫 → `NormalizeThreadRefs` → `PruneDanglingRefs(DefaultBatchSize)` → `logging.Infof` 记录 `normalized/rows_repaired/refs_removed`；无 Down（不可逆），Description 注明。
- `openspec/changes/heal-dangling-article-refs/verification.sql`：部署后核对 5 条查询（悬空引用=0、非数组行=0、9 条线索引用现状、巡检口径一致、快照计数仅记录）。

### §5 巡检
- `internal/admin/scheduler/job_daily_report.go`：`DailyReportJob` 收尾（backfill 之后、返回前）调用 `articlerefs.CountDanglingArticleRefs(repository.Repo.DB())`：>0 → `Warnf("daily-report: dangling article refs=%d ...")`，=0 → `Infof`，查询失败 → `Warnf` 且不影响 job 成功。

## 2. 测试（Scenario → 落点）

| Scenario / 契约点 | 落点（文件::用例） |
| --- | --- |
| 归并副本时引用改指保留条 | `platform/database/dedupe_rss_articles_migration_test.go::TestDedupeRSSArticlesMigrationRewiresDailyReportRefs`；`platform/articlerefs/rewire_test.go::TestRewireArticleRefsPointsReferencesAtKeeper` |
| 保留条已在数组内时只去重 | 同上两处 |
| 删除订阅源时引用被剔除 | `reader/repository/article_refs_test.go::TestDeleteFeedCascadePrunesArticleReferences` |
| 无引用命中时不产生写操作 | `platform/articlerefs/rewire_test.go::TestRewireArticleRefsIsIdempotentAndNoOp`；`reader/repository/article_refs_test.go::TestDeleteFeedCascadeLeavesUnrelatedRowsAlone` |
| 单引用的空数组规范化（`[]` 非 null） | `rewire_test.go::TestPruneArticleRefsDropsDeletedIDs`；`article_refs_test.go::TestDeleteArticlesByFeedPrunesReferences` |
| 悬空 id 被剔除 / 保序 | `platform/database/heal_dangling_article_refs_migration_test.go::TestHealDanglingArticleRefsMigrationRepairsReferences`；`articlerefs/repair_test.go::TestPruneDanglingRefsRemovesOrphanedReferences` |
| JSON null / SQL NULL → `[]` | `heal_dangling_article_refs_migration_test.go::...RepairsReferences`；`articlerefs/repair_test.go::TestNormalizeThreadRefsRewritesNonArrays` |
| 重复执行不改变已修复数据 | `...::TestHealDanglingArticleRefsMigrationIsIdempotent`（xmin 未变） |
| 无悬空数据时零写入 | `...::TestHealDanglingArticleRefsMigrationWritesNothingWhenClean`（xmin 未变） |
| 迁移在无表二进制下跳过（CLI 工具） | `...::TestHealDanglingArticleRefsMigrationSkipsMissingTable` |
| 候选文章在写库前被删除 | `topicgraph/service/daily_report_article_filter_test.go::TestFilterVanishedArticleRefsDropsDeletedCandidates` |
| 全部候选都已消失 → `[]` | 同文件 `::TestApplyExistingArticleRefs`（断言 `marshalJSONArray` 产出 `[]`） |
| 存在性校验查询失败时降级 | 同文件 `::TestFilterVanishedArticleRefsKeepsCandidatesWhenCheckFails`（不可达 DSN 注入） |
| 无候选时不发查询 | 同文件 `::TestFilterVanishedArticleRefsNoCandidatesSkipsProbe` |
| 悬空计数 0 / >0 / 查询失败 | `articlerefs/repair_test.go::TestCountDanglingArticleRefs`（含不可达 DSN、nil db） |
| 白盒分支 B1-B7（零命中/替换/多次替换/去重/空数组/脏元素/空 ids） | `articlerefs/rewire_test.go`（7 个用例逐条覆盖） |
| 分批边界（batch<=0 → 500、batch=1 走完全表） | `articlerefs/repair_test.go::TestPruneDanglingRefsBatchBoundaries` |

新增测试文件 6 个 / 用例 26 个（除下述改名外与 tasks 落点一致）。

## 3. 命令与结果

| 命令 | 结果 |
| --- | --- |
| `cd backend-go && golangci-lint run ./...` | 0 issues |
| `cd backend-go && go vet ./...` | 无输出（clean） |
| `cd backend-go && go build ./...` | OK |
| `go test -count=1 ./internal/platform/articlerefs/... ./internal/platform/database/... ./internal/reader/... ./internal/topicgraph/... ./internal/admin/scheduler/...` | 全部 ok（11.4s / 50.4s / handler 0.5s + repository 13.8s + service 24.2s / handler 10.5s + repository 31.1s + service 16.1s / 13.0s） |
| `go test -short <同 5 个包>` | 全部 ok（PG 用例按约定 skip） |
| 真库只读核对（`docker exec … psql`，未改数据） | 修复前悬空引用 5595 个、非数组行 3 行（与 change 背景一致） |

## 4. 与 design/tasks 的偏差（3 处，均为实现细节，未改设计意图）

1. **新增导出 `articlerefs.ExistingArticleIDs`**（design D1 的 API 清单未列）：D3 的「写库前存在性校验」和 D5 的修复都需要「哪些 id 还有行」，放进同一个 cast-safe 实现里，避免在 topicgraph 再复制一份查询。
2. **迁移加 `tableExists("daily_report_threads")` 守卫**（design D5 未提）：`daily_report_threads` 由 AutoMigrate 从日报模型创建（不是版本化迁移创建），`cmd/dump-sanitizer` 这类不注册日报模型的二进制跑 `database.InitDB` 时会撞 `relation "daily_report_threads" does not exist`。首次跑 articlerefs 包测试即真实复现（PL/pgSQL 42P01，迁移整体失败）。守卫沿用同表既有迁移（`20260403` era 线程索引）的写法。
3. **迁移测试文件名** `articlerefs_integrity_migration_test.go` → `heal_dangling_article_refs_migration_test.go`（tasks 4.2 与 §8 映射表已同步改口径）：该包测试按文件名顺序执行，原名的 `a` 前缀把 golden schema 构建时机提前到 `TestBoardLevelAnalysisScopeMigration` 之前，而后者中途跑 `database.RunAutoMigrate` 会把 migration `20260723_0001` 物化的 `scheduler_tasks.check_interval NOT NULL` 放松掉（GORM 会按模型 tag 放松 NOT NULL），导致后续 `TestModelTagConstraints_MaterializedInDB` 失败。改名保持既有测试顺序 + 文件头注释说明（见 §5 发现 1）。

另外：tasks 5.2 的「查询失败」分支以不可达 DSN 断言 `CountDanglingArticleRefs` 返回 error（job 内那 4 行日志胶水未单独测试，见 §5 遗留 3）。

## 5. 发现与遗留

1. **（既有测试脆弱点，本 change 未修）** `platform/database` 包内：`TestBoardLevelAnalysisScopeMigration` 用 `testutil.OpenTestDB` + 裸 `database.RunAutoMigrate` 且不恢复被放松的约束，该包「golden schema 由第一个 `SetupTestDB` 调用构建」的机制与文件名顺序耦合——任何排在该测试之前的 `SetupTestDB` 新用例都会让 `TestModelTagConstraints_MaterializedInDB` 变红。建议后续单独修（让该用例恢复约束或改用 SetupTestDB 的 schema）。本 change 只做了不改顺序的规避。
2. **真库效果未核对（8.3/8.4）**：迁移只在后端启动时执行，真库当前仍有 5595 个悬空引用。需要重启后端（跑迁移）后按 `verification.sql` 核对，再重跑一次今日日报确认写路径过滤生效。属部署后动作。
3. **巡检只在 job 成功路径记录**（design D6 的「收尾」语义）：`DailyReportJob` 提前 `return nil, err`（如 `CollectBoardIDsForDate` 失败）时不会记录悬空计数。
4. **快照计数不回填**（design D5 已声明取舍）：剪除死链后部分线索「N 篇」会大于展开列表长度。
5. **迁移不可逆**（无 Down）：修复删除的引用无法恢复，部署前建议 `pg_dump`（tasks 8.5）。
6. **工作树里的无关脏文件**：`internal/reader/service/feed_service.go`（+15 行 tag_count 重算）与 `feed_service_test.go`（+8 行）不是本 change 的改动，我未触碰；本 change 只跑影响包测试，未对这些改动做评判。

## 6. 未完成 / 阻塞

- 无阻塞。§7 文档（主线程）与 §8.3-8.5（部署后核对 + 完工汇报）留待主线程。

---

## Review 缺口修复（轮次 2）

**缺口**（主线程核验时发现）：原实现把存在性校验放在 Step 6 之后、Step 7 之前，参数是 `map[int][]repository.Thread`（常规聚类 thread）。但 **Step 7.5 watch 物化**（`daily_report_orchestrator.go` 351-402）会把 `MaterializeSentenceWatch` / `MaterializeKeywordWatches` 产生的 `[]repository.DailyReportThread` 追加进 `threadBatches`，那条路径是「查当天文章 → LLM 裁决（可达数十秒）→ 写库」，写进 `RelatedArticleIDs` 的文章 id 完全没被校验——正是 2026-09-16 出事的形态，缺口真实存在。

**改动点（文件:符号）**：

| 文件 | 改动 |
| --- | --- |
| `topicgraph/service/daily_report_article_filter.go` | `filterVanishedArticleRefs` 签名改为 `(db, threadBatches [][]repository.DailyReportThread) int`；新增 `decodeThreadArticleRefs`（jsonb 原始字节 → ids；非数组（JSON `null`/SQL NULL/对象）读作空集合并报告 `nonArray` 以便规范化写回 `[]`；无法解析报告 `undecodable`，原样保留 + Warn）；`applyExistingArticleRefs` 改为对 `[]repository.DailyReportThread` 工作（保序、只删死链、无变化不写、全删写 `[]`，写回走 `marshalJSONArray`）；新增 `collectThreadArticleRefs`（全批次候选并集，一次探测） |
| `topicgraph/service/daily_report_orchestrator.go` | 删除 Step 6 之后对 `threadsByCluster` 的那次调用；改在 Step 7.5 之后、`return report, sections, threadBatches, nil` 之前对**整个 `threadBatches`** 调用一次（常规 + 物化两类全覆盖，一份日报只探测一次） |
| `topicgraph/service/daily_report_article_filter_test.go` | 用例改到新签名；新增 `TestFilterVanishedArticleRefsCoversMaterializedBatches`（物化批次 `mustMarshalUintArray` 形态：候选被删 → 剔除保序、全删写 `[]`）；`TestApplyExistingArticleRefs` 补充「非数组（`null` / 对象）规范化为 `[]`」与「无法解析（`[1,`）原样保留」两类形态断言 |

**与 design D3 的关系**：D3 的意图是「日报写入的引用必须全部指向存在的文章」。原实现把校验挂在「组装 thread 批次前」，只是把校验放在了错误的位置（批次清单那时还没长齐）。轮次 2 把它收敛成**唯一一处、最后一道**——紧贴 `SaveReport` 之前的返回点，对最终批次清单校验，既覆盖物化轨，也消除了「同一份日报两次探测」的冗余。

**命令与结果**：

| 命令 | 结果 |
| --- | --- |
| `cd backend-go && golangci-lint run ./...` | 0 issues |
| `cd backend-go && go vet ./...` | 无输出（clean） |
| `cd backend-go && go build ./...` | OK |
| `go test -count=1 ./internal/topicgraph/...` | ok（handler 11.1s / repository 22.9s / service 13.6s） |
| `go test -count=1 ./internal/reader/...` | ok（handler 0.2s / repository 17.0s / service 21.3s） |
| `go test -count=1 ./internal/admin/...` | ok（handler 12.6s / scheduler 12.8s / service 23.5s） |
| `go test -count=1 ./internal/platform/articlerefs/... ./internal/platform/database/...` | ok（9.8s / 34.5s） |

**thread 写入路径复核（是否还有未覆盖的）**：thread 行只有 `repository.SaveReport(report, sections, threadBatches)` 一条写入路径（`GenerateAndSaveReport` 是唯一调用方）；`RelatedArticleIDs` 的产生点共 4 处——`daily_report_merge.go:29`（常规聚类，经 orchestrator 转批次）、`watch_materialize_keyword.go:181`、`watch_materialize_sentence.go:327`（两类物化）——**全部落在最终 `threadBatches` 上，现已统一覆盖**。`lane_snapshot_repository.go:119/291` 对 `daily_report_threads` 只有 `SELECT section_id, title`（只读）；`daily_report_llm.go:204` 是 operation 名字符串，不是写路径。未发现其他写入点。

---

## 轮次 3（review：删分类级联路径 + M1/M2）

**触发**：主线程 review 提出 H1（`_conventions.md` 称「全库 6 条 FK、DB 层未强制」，但真库有级联 FK，删分类可能级联删文章，属未接线路径）。主线程用真库 `pg_constraint` 核实的结论：

| 事实 | 证据 |
| --- | --- |
| 真库**存在** `articles.feed_id → feeds(id) ON DELETE CASCADE`（`fk_feeds_articles`） | `SELECT … FROM pg_constraint WHERE conrelid IN ('articles','feeds','categories')` |
| 真库**存在** `feeds.category_id → categories(id) ON DELETE CASCADE`（`fk_categories_feeds`） | 同上 → 删分类级联删 feed 再删文章 = 第三条真实删行路径 |
| 这两条 FK **不是**任何版本化迁移建的（历史 AutoMigrate 遗留） | `grep -n 'ADD CONSTRAINT\|REFERENCES' postgres_migrations.go` 无 articles/feeds 相关；`db.go:21 DisableForeignKeyConstraintWhenMigrating: true` |
| 测试库（testcontainer）**没有**这两条 FK | 同上配置 + `testutil.go:154` 同一开关；测试 golden schema 只跑 AutoMigrate(无 FK) + 版本化迁移 |
| `reading_behaviors.feed_id/article_id → feeds/articles` 与 `user_preferences.feed_id → feeds` 都是 **NO ACTION** | 同上查询，`confdeltype` 默认 NO ACTION |

**关键设计判断（相对轮次 3 任务书的一处偏离，需主线程确认）**：任务书给的形状是「pluck 文章 id → prune → 删 category 行（靠 FK 级联删 feed/articles）」。我改成**显式删文章 + 显式删 feed + 删 category**，理由是：级联只存在于「历史遗留库」，在**无级联的库**（新建库/测试库）里，按任务书形状执行会 prune 掉**仍然活着**的文章的引用——那不是修复而是数据丢失（把有效来源剪掉）。显式删除让「只对真会消失的行剪引用」这条保证与数据库无关，且在真库语义等价（级联本来就删同一批行）。**未**删除 `reading_behaviors`/`user_preferences`（任务书要求）：它们的 NO ACTION 外键今天就让删分类失败（今天同样失败：级联路径也过不了这两个约束），替它们删除等于把「删除失败」悄悄变成「删除成功」，属超范围行为变更。

**改动点（文件:符号）**：

| 文件 | 改动 |
| --- | --- |
| `reader/repository/repository.go` | 新增 `DeleteCategoryCascade(categoryID uint) error`（单事务：pluck 分类下 feed → `deleteArticlesOfFeedsWithRefs` → 删 feed → 删 category；`reading_behaviors`/`user_preferences` 不动，失败整体回滚）；新增私有批量核心 `deleteArticlesOfFeedsWithRefs(tx, feedIDs []uint)`，`deleteArticlesWithRefs(tx, feedID)` 改为委托它（消除 prune→delete 两处重复，使删分类一次 pluck/prune/delete 而非每 feed 一轮）；`DeleteCategory` 保留 + 注释标明不维护引用、删分类请用 `DeleteCategoryCascade`（M1 指定保留）；**删除** `DeleteFeed(feed *models.Feed)`（M1：grep 全仓无调用方，且是不维护引用的裸删原语） |
| `reader/handler/category_handler.go` | `DeleteCategory` handler 的内联 `repository.Repo.DeleteCategory(category)` → `DeleteCategoryCascade(category.ID)`；404/500 响应形状与 message 未动 |
| `reader/repository/article_refs_test.go` | 新增 `seedRefCategory`、`refThreadXMins`（xmin 写检测，与 `articlerefs/helpers_test.go` 同法；daily_report_threads 无 updated_at、裸 UPDATE 对 GORM callback 不可见）；新增 3 用例（见下） |
| `admin/scheduler/job_daily_report_test.go` | 新增 `TestDailyReportJobSucceedsWhenDanglingRefProbeFails`（M2） |
| `platform/articlerefs/repair_test.go` | `TestCountDanglingArticleRefs` 注释补范围说明（本文件只管计数器；job 胶水降级在 scheduler 包测试），互相指引（M2） |

**新增用例与断言**：

| 用例 | 断言 |
| --- | --- |
| `TestDeleteCategoryCascadePrunesArticleReferences` | 分类（2 feed）行、feed 行、文章行全删；分类外 feed 的文章保留；thread 1（`[A0, outside0, A1]`）剪成 `[outside0]` 且保序；thread 2（仅引用被删文章）剪成 `[]` 且 `jsonb_typeof='array'` |
| `TestDeleteCategoryCascadeLeavesUnrelatedRowsAlone` | 无人引用的分类被删时 **xmin 全表不变**（零 UPDATE）；SQL NULL 引用行保持 `sql-null`（规范化是迁移的职责，不是删除路径的） |
| `TestDeleteCategoryCascadeHandlesCategoryWithoutFeeds` | 空分类（0 feed）删除不报错、不写任何 thread 行（`len(feedIDs)==0` 分支） |
| `TestDailyReportJobSucceedsWhenDanglingRefProbeFails` | 探针失败（SQLite schema 无 `daily_report_threads`、且不支持 LATERAL）→ job `err == nil`、`report_count == 1`；测试自带 `HasTable` 前置断言保证「探针确实会失败」，日志实测打出 `WARN … dangling article ref check failed: … near "(": syntax error` |

**命令与结果**：

| 命令 | 结果 |
| --- | --- |
| `cd backend-go && golangci-lint run ./...` | 0 issues |
| `cd backend-go && go vet ./...` | 无输出（clean） |
| `cd backend-go && go build ./...` | OK |
| `go test -count=1 ./internal/reader/repository/... -run 'TestDeleteCategory\|TestDeleteFeedCascade\|TestDeleteArticlesByFeed' -v` | 6/6 PASS（category 三个新用例 0.09-0.10s） |
| `go test -count=1 ./internal/admin/scheduler/... -run TestDailyReportJobSucceedsWhenDanglingRefProbeFails -v` | PASS（0.047s，WARN 日志符合预期） |
| `go test -count=1 ./internal/platform/articlerefs/... ./internal/platform/database/... ./internal/reader/... ./internal/topicgraph/... ./internal/admin/scheduler/...` | 全绿（9.4s / 52.4s / handler 0.5s + repository 12.7s + service 18.4s / handler 14.6s + repository 33.8s + service 18.7s / 13.6s） |
| `gofmt -l` 于本轮触碰文件 | 无输出（仓库里另有若干既有未格式化测试文件，非本轮触碰，未动） |

**删行路径闭环复核（本轮重跑 grep）**：

| 路径 | 位置 | 是否维护引用 |
| --- | --- | --- |
| 去重归并删 loser | `postgres_migrations.go:2612` `DELETE FROM articles` | ✅ `RewireArticleRefs`（轮次 1） |
| 删 feed / 删 feed 的文章 | `reader/repository/repository.go:272` `deleteArticlesOfFeedsWithRefs`（`DeleteFeedCascade` / `DeleteArticlesByFeed` / `DeleteCascadeByFeed` 共用） | ✅ `PruneArticleRefs` |
| **删分类（级联删 feed 再删文章）** | `reader/repository/repository.go` `DeleteCategoryCascade`（本轮新增） | ✅ `PruneArticleRefs` |
| 真库历史级联 FK 旁路（`fk_feeds_articles` / `fk_categories_feeds`） | 数据库层，非代码 | ✅ 已因「显式删除 + 先 prune」而被覆盖 |
| 其它 `Delete(&models.Article{})` | 无（grep 全仓仅上述一处，`article_topic_tags` 的删除是边不是文章行） | — |

**结论**：**未发现剩余的未接线文章删行路径**。`articles` 作为 FK 被引用方的级联只有「feeds 删 → articles 删」（已覆盖）与「categories 删 → feeds 删 → articles 删」（本轮覆盖）；`article_topic_tags` / `firecrawl_jobs` / `tag_jobs` 是 articles 的从属表，cascade 方向不影响引用维护。

**仅报告、未处理（按任务书要求）**：`reading_behaviors.feed_id/article_id` 与 `user_preferences.feed_id` 的 NO ACTION 外键使「有阅读行为或偏好行的 feed/分类」删除失败——这是既有行为，本轮显式删除不改变它（同样失败、同样整体回滚）；若要让管理端删除可预期，需要单独 change 处理（删行为/偏好或改成 `ON DELETE CASCADE`），不在本 change 范围。

---

## 轮次 4：依赖行清理环境无关化（补完轮次 3「显式删除」的缺口）

**缺口**：轮次 3 把删 feed / 删分类改成显式删除 `articles` 行（正确：靠遗留 FK 级联会让「只对真会消失的行剪引用」这条保证在无 FK 库上失效）。但 `deleteArticlesOfFeedsWithRefs` 只删文章行本身，`article_topic_tags` / `tag_jobs` / `firecrawl_jobs` 在真库靠遗留 FK 级联消失，**新建库/测试库没有这些 FK**（`DisableForeignKeyConstraintWhenMigrating: true`，无版本化迁移创建它们）→ 显式删文章会留下孤儿行。

### 改动点（文件:符号）

| 文件 | 符号 | 改动 |
| --- | --- | --- |
| `backend-go/internal/reader/repository/repository.go` | `deleteArticlesOfFeedsWithRefs` | 顺序固定为「剪引用 → 删依赖行 → 删文章行」，全部在调用方事务内；文章行改按 id 分块删（`articleDeleteChunk=1000`，防 `IN (?)` 超 65535 绑定参数；`max_articles=9999` 即不限量）；`len(articleIDs)==0` 早返回不发 DELETE |
| 同上 | `deleteArticleDependents`（新增） | 对给定文章 id 显式删 `article_topic_tags` / `tag_jobs` / `firecrawl_jobs`；表缺失时 `Migrator().HasTable` 跳过（窄部署不因缺表失败） |
| 同上 | `articleDeleteChunk`（新增常量） | 分块窗口，注释说明理由（feed 行数与 PG 绑定参数上限） |
| 同上 | `DeleteFeedCascade` 注释 | 补「连同文章自己的依赖行一起删」 |

**显式清理 vs 交给既有机制（刻意划界）**：

| 依赖行 | 处置 | 理由 |
| --- | --- | --- |
| `article_topic_tags` | 本函数显式删 | 文章的边，文章没了边就该没；真库靠 `fk_article_topic_tags_article`，新建库没有 |
| `tag_jobs` / `firecrawl_jobs` | 本函数显式删 | 队列行指向不存在的文章没有意义（pending/leased 一并不留） |
| `topic_tags`（失去全部边的孤儿标签） | **不删**，归 `aux_label_cleanup` 维护任务 | 与「有 FK 时边被级联删除后由谁收孤儿」同一所有者，避免两套逻辑 |
| `reading_behaviors` | **不删** | `article_id`/`feed_id` 是 NO ACTION；`DeleteFeedCascade`/`DeleteCascadeByFeed` 已按 feed 先删，`DeleteArticlesByFeed` 保持既有「有行为则删除失败」语义 |
| `user_preferences` | **不删** | 同上，属管理操作语义，不在本 change 扩范围 |

### 测试（`backend-go/internal/reader/repository/article_refs_test.go`）

| 用例 | 断言 |
| --- | --- |
| `TestDeleteFeedCascadeRemovesArticleDependents` | 被删 feed 的 2 篇文章：边/标签 job/抓取 job 各为 0；另一 feed 的 1 篇各为 1；失去全部边的 tag 行仍在（归维护任务） |
| `TestDeleteCategoryCascadeRemovesArticleDependents` | 分类下 2 个 feed 的 3 篇文章依赖行清零；分类外 feed 的 1 篇各为 1 |
| `TestDeleteArticlesByFeedCoversEveryChunk` | 1200 篇（> 1000 窗口）删完不留尾巴（分块边界） |
| `TestDeleteFeedCascadeHandlesFeedWithoutArticles` | 无文章的 feed 删除不报错、不误伤其它 feed 的依赖行 |

**非空性验证（关键）**：临时把 `deleteArticleDependents` 调用短路后重跑，`TestDeleteFeedCascadeRemovesArticleDependents` 报 `Should be zero, but was 2`、`TestDeleteCategoryCascadeRemovesArticleDependents` 报 `... was 3`——证明这两个用例真的在验证显式清理（测试容器无级联 FK）。随后已恢复，`grep TEMP-NONVACUITY` 无残留。

### 命令与结果

| 命令 | 结果 |
| --- | --- |
| `cd backend-go && golangci-lint run ./...` | 0 issues |
| `cd backend-go && go vet ./...` | 无输出（clean） |
| `cd backend-go && go build ./...` | OK |
| `go test -count=1 -v ./internal/reader/repository/ -run 'TestDelete(Feed\|Category\|Articles)'` | 9/9 PASS（含本轮 3 个新用例 + 分块用例） |
| `go test -count=1 ./internal/platform/articlerefs/... ./internal/platform/database/... ./internal/reader/... ./internal/topicgraph/... ./internal/admin/scheduler/...` | 全绿（9.5s / 51.4s / handler 0.4s + repository 13.3s + service 22.7s / handler 14.4s + repository 31.6s + service 16.6s / 17.2s） |

### 同类隐式假设盘点（只报告，未扩范围）

「依赖遗留 FK 级联」在本仓不是孤例，标签/辅助标签层仍有同类路径（新建库无 FK 时会留孤儿子行）：

| 位置 | 依赖方式 |
| --- | --- |
| `tagmanagement/service/core/article_tagger.go:489` | 注释明写 "All child tables have ON DELETE CASCADE, so deleting topic_tags automatically cleans up embeddings, queues, relations, labels" |
| `tagmanagement/service/core/hard_merge.go:71` | 显式清 embedding/queue/边，但靠级联清 `topic_tag_semantic_labels`（注释自承 "before CASCADE deletes…"） |
| `tagmanagement/service/auxlabel/auxiliary_label_service.go:505` | 删 `semantic_labels` 靠级联清 `topic_tag_board_labels` / `topic_tag_semantic_labels` / `composite_components` |
| `admin/scheduler/job_tag_quality_score.go:46` | 批量删孤儿 auxiliary label，同上依赖级联 |

对照：`topic_tags.merged_into_id`、`topic_watch_hits.watch_id`、`topic_tag_embeddings.topic_tag_id`、`board_topic_watches.persistent_topic_id`、`topic_enrichment_result` 复合、`composite_components.composite_id`、`topic_lane_snapshots.persistent_topic_id` 这些 FK **由版本化迁移显式创建**，新建库里存在，不属同类隐患。

---

## 轮次 5（增量 review L1/L2/L3 —— 非空性加固 + 守卫提循环外）

范围：只改上表已交付代码的非空性/性能，**不动语义**；未 commit、未改 docs/design/specs、未碰其它 change 的脏文件。

### 改动点

| 项 | 文件:符号 | 改动 |
| --- | --- | --- |
| L3 | `reader/repository/repository.go`：`deleteArticleDependents` / `deleteArticlesOfFeedsWithRefs` | 新增 `articleDependent{table,model}` 类型 + 包级 `articleDependents` 清单 + `existingArticleDependents(tx)`（每次删除调用只解析一次表存在性，不再每分块对 3 张表各查一次目录）；`deleteArticleDependents(tx, articleIDs, dependents)` 接收已解析清单。注释写明 fail-open 边界：`HasTable` 只返 bool、元数据查询失败读作 false（跳过依赖清理、文章照删 → 可能留孤儿，刻意取舍），只守表存在不守列存在（缺列 → DELETE 报错 → 整事务回滚，fail-closed） |
| L1 | `admin/scheduler/job_daily_report_test.go`：`TestDailyReportJobSucceedsWhenDanglingRefProbeFails` | 调 job 前新增 `_, probeErr := articlerefs.CountDanglingArticleRefs(db); require.Error(t, probeErr, ...)`，并把「failure is asserted by the warn log」这句不实注释改成描述实际断言；新增 `articlerefs` 导入（platform 叶子包，无 import 环） |
| L2 | `reader/repository/article_refs_test.go`：`TestDeleteFeedCascadeHandlesFeedWithoutArticles` | 补断言被删 feed 行 `Count == 0`（否则 `DeleteFeedCascade` 退化成 no-op 该测试仍绿） |

### L1 最终断言形态（实测）

```go
require.False(t, db.Migrator().HasTable("daily_report_threads"),
    "this test needs the probe to fail: no thread table in the SQLite schema")
_, probeErr := articlerefs.CountDanglingArticleRefs(db)
require.Error(t, probeErr, "the integrity probe must fail on the SQLite schema, not return a silent zero")
```

依赖关系确认：`internal/platform/articlerefs` 只 import gorm，scheduler 包 import 它不构成环（非测试文件 `job_daily_report.go` 早已导入同一包）。

### 命令与结果（原始输出摘要）

| 命令 | 结果 |
| --- | --- |
| `cd backend-go && go build ./...` | 无输出（成功） |
| `cd backend-go && go vet ./internal/reader/... ./internal/admin/scheduler/...` | 无输出（clean） |
| `golangci-lint run ./...` | `0 issues.` |
| `go test -count=1 ./internal/reader/... ./internal/admin/scheduler/...` | `ok reader/handler 0.235s` / `ok reader/repository 11.732s` / `ok reader/service 18.246s` / `ok admin/scheduler 11.153s` |
| `go test -count=1 ./internal/platform/articlerefs/... ./internal/platform/database/... ./internal/topicgraph/...` | 全 ok（15.1s / 43.8s / 12.3s / 30.2s / 19.0s） |
| `go test -count=1 -v -run TestDailyReportJobSucceedsWhenDanglingRefProbeFails ./internal/admin/scheduler/` | `--- PASS`，日志 `WARN daily-report: dangling article ref check failed: count dangling article refs: SQL logic error: near "(": syntax error (1)`（证明探针在 SQLite 上确实失败且 job 仍成功） |
| `go test -count=1 -v -run TestDeleteFeedCascadeHandlesFeedWithoutArticles ./internal/reader/repository/` | `--- PASS`（12.54s） |
| `bash scripts/change-scope.sh` | 改动范围 47 文件（含其它 change 的脏文件；本轮的仅 3 个） |

### 本轮残余

- 无新增残余；review 的 M1（design/spec 与实现的偏差描述）与 L4（残余风险记录）属文档侧，由主线程处理。
- 未跑 `go test ./...` 全量（仓库纪律：只跑影响包）。
