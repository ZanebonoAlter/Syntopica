# Tasks — heal-dangling-article-refs

> 实现契约：`proposal.md` + `design.md`（D1-D6）+ `specs/article-reference-integrity/spec.md`；
> 用例账本：`test-cases.md`。改动一律最小化，不顺手重构无关代码。

## 1. 引用维护器（新包 `backend-go/internal/platform/articlerefs`）

- [x] 1.1 新增 `rewire.go`：`RewireArticleRefs(db *gorm.DB, oldID, keeperID uint) (int64, error)` 与 `PruneArticleRefs(db *gorm.DB, ids []uint) (int64, error)`——按 design D1：`jsonb_typeof(related_article_ids)='array' AND related_article_ids ? ?` 命中，Go 侧重写（保序、去重、空写 `'[]'::jsonb`），按主键 UPDATE。**存在性/匹配一律用文本比对（`a.id::text = elem`），禁止对 jsonb 元素做 `::bigint` 强转**（脏元素会抛错）。ids 为空时直接返回 0，不发查询。
- [x] 1.2 新增 `repair.go`：`NormalizeThreadRefs(db) (int64, error)`（`jsonb_typeof(...) IS DISTINCT FROM 'array'` → `'[]'::jsonb`，含 SQL NULL）、`PruneDanglingRefs(db, batch) (rows int64, refs int64, err error)`（分批，`batch<=0` → 500，按 id 窗口推进，逐行重写）、`CountDanglingArticleRefs(db) (int64, error)`（只读，返回悬空**引用数**）。
- [x] 1.3 单测 `rewire_test.go` / `repair_test.go`（PG testcontainer，禁 SQLite）：覆盖 `test-cases.md` 白盒分支表 B1-B7 + 分批边界（整除/余数/batch<=0）+ 脏元素 + 空 ids；并断言保序与 `jsonb_typeof='array'`。
- [x] 1.4 包内注释写清「为什么只剪不映射」（design D5）与调用点约定（删除路径必须先维护再删行）。

## 2. 删除路径接线（必须覆盖全部活删除路径）

- [x] 2.1 `postgres_migrations.go` 的 `mergeDuplicateArticleGroup`：在 `DELETE FROM articles WHERE id = ?` **之前**调用 `articlerefs.RewireArticleRefs(db, r.ID, keeper)`，错误上抛（迁移事务回滚）；保持既有 keeper 选择/边重指/计数重算语义不变。
- [x] 2.2 `backend-go/internal/platform/database/dedupe_rss_articles_migration_test.go` 新增用例：归并前某 thread 引用 loser（含「同时含 keeper」变体），归并后数组中不再有 loser、指向 keeper、保序、无重复。
- [x] 2.3 删订阅源：`backend-go/internal/reader/repository/repository.go` 的 `DeleteArticlesByFeed` 改为事务内「pluck 本 feed 文章 id → `PruneArticleRefs` → 删行」；`handler.DeleteFeed`（`reader/handler/feed_handler.go:377`）改为调用仓库事务方法（prune → 删 `reading_behaviors` → 删 feed 行），去掉 handler 内联的裸 DB 调用；`DeleteCascadeByFeed` 复用同一事务方法。
- [x] 2.4 仓储测试 `reader/repository/article_refs_test.go`：删源后引用被剔除、保序、空则 `[]`；无引用时零 UPDATE；既有删源行为回归（`feed_service_test.go` 同包必跑）。
- [x] 2.5 删分类级联路径（轮次 3，review H1 发现）：真库存在历史遗留下的 `feeds.category_id → categories(id) ON DELETE CASCADE`（`fk_categories_feeds`，无任何版本化迁移创建它，`DisableForeignKeyConstraintWhenMigrating` 又使新建库没有它）——删分类会级联删 feed 再删文章，是第三条真实删行路径。新增 `DeleteCategoryCascade(categoryID)`（单事务：pluck 分类下 feed 的文章 id → `PruneArticleRefs` → 显式删文章 → 显式删 feed → 删分类），`handler.DeleteCategory` 改走它；**不用**级联而显式删，因为否则「只对真会消失的行剪引用」这条保证在无级联库上会变成剪活文章的引用。不动 `reading_behaviors`/`user_preferences`（其 NO ACTION 外键今天就让该删除失败，替它删掉等于悄悄放宽管理操作）。
- [x] 2.6 M1（review）：`DeleteFeed(feed *models.Feed)` 无任何调用方（grep 确认）且是不维护引用的裸删原语 → 删除该方法；`DeleteCategory(cat)` 保留但注释标明「不维护引用，删分类用 DeleteCategoryCascade」。
- [x] 2.7 仓储测试新增 3 用例：删分类剪引用（2 个 feed + 分类外 feed 不受影响、保序、剪空写 `[]`）、无引用命中时 xmin 不变（零 UPDATE）、无 feed 的空分类不写 thread。
- [x] 2.8 依赖行清理环境无关化（轮次 4）：`deleteArticlesOfFeedsWithRefs` 在删文章行**之前**、同事务内显式删这些文章的依赖行——`article_topic_tags` / `tag_jobs` / `firecrawl_jobs`（真库靠遗留 FK `fk_article_topic_tags_article` / `fk_tag_jobs_article` / `fk_firecrawl_jobs_article` 级联，新建库/测试库没这些 FK，显式删文章会留下孤儿行）；文章行改按 id 分块删（`articleDeleteChunk=1000`，防 `IN (?)` 绑定参数超限，`max_articles=9999` 意味着不限量）；表缺失时跳过（`Migrator().HasTable`）；`topic_tags` 孤儿仍归 `aux_label_cleanup`、`reading_behaviors`/`user_preferences` 语义不变。
- [x] 2.9 仓储测试新增 4 用例：删 feed / 删分类后三张依赖表孤儿清零（分类外 feed 行不变、tag 行保留给维护任务）、无文章的 feed 空分支不报错、分块边界（1200 篇 > 1000 窗口全删不留尾巴）。**非空性验证**：临时注释掉依赖清理调用，两用例分别报 2 / 3 条孤儿边（已恢复，`grep TEMP-NONVACUITY` 无残留）。
- [x] 2.10 同类隐式假设盘点（只报告不改，超出本 change 范围）：标签/辅助标签层仍有「依赖遗留 FK 级联」的路径——`tagmanagement/service/core/article_tagger.go:489`（注释明写 "All child tables have ON DELETE CASCADE"）、`core/hard_merge.go:71`（显式清 embedding/queue/边，靠级联清 `topic_tag_semantic_labels`）、`auxlabel/auxiliary_label_service.go:505`（删 `semantic_labels` 靠级联清 `topic_tag_board_labels`/`topic_tag_semantic_labels`/`composite_components`）、`admin/scheduler/job_tag_quality_score.go:46`（批量删孤儿 label）。新建库无这些 FK 时会留孤儿子行。
- [x] 2.11 `HasTable` 守卫提出分块循环外（轮次 5，review L3）：新增 `articleDependent` 类型 + 包级 `articleDependents` 清单 + `existingArticleDependents(tx)`（一次调用只算一次表存在性，不再每分块对三张表各查一次目录），`deleteArticleDependents(tx, articleIDs, dependents)` 改为接收已解析清单；注释写明已知边界：`HasTable` 只返 bool、元数据查询失败读作 false（fail-open：跳过依赖清理、文章照删 → 可能留孤儿，为窄部署兼容的刻意取舍），只守表存在不守列存在（缺列 → DELETE 报错 → 整事务回滚，fail-closed）。
- [x] 2.12 非空性加固（轮次 5，review L1/L2）：job 测试 `TestDailyReportJobSucceedsWhenDanglingRefProbeFails` 在调 job 前直接断言 `articlerefs.CountDanglingArticleRefs(db)` 返回 error（实测 SQLite 上报 `SQL logic error: near "(": syntax error`），使「探针静默退化为 (0, nil)」再也不能让该测试误过；仓储测试 `TestDeleteFeedCascadeHandlesFeedWithoutArticles` 补断言被删 feed 行计数为 0（否则 `DeleteFeedCascade` 退化成 no-op 也能过）。

## 3. 写路径存在性校验 + JSON null 卫生

- [x] 3.1 `topicgraph/service/daily_report_orchestrator.go`：组装 thread 批次前对全部候选 `RelatedArticleIDs` 做一次存在性过滤（按 1000 分批 `SELECT id FROM articles WHERE id IN (?)`；查询失败 → `logging.Warnf` 后按原候选写入，不阻断）；过滤函数抽成可单测的纯函数（入参 db + 候选集合，出参过滤后集合）。
- [x] 3.2 单测 `daily_report_article_filter_test.go`：候选在写库前被删 → 不写入该 id；全候选消失 → 写 `[]`；查询失败 → 降级且不阻断（用 mock/失败 db 注入）。
- [x] 3.3 JSON null 卫生：`daily_report_orchestrator.go:281-282`（`th.TagIDs` / `th.RelatedArticleIDs`）与 `daily_report_merge.go:292`（`mergedTagIDs`）改用 `marshalJSONArray`；单测断言 nil/空切片产出 `[]` 且不产生 `'null'`（含 thread 两个字段）。
- [x] 3.4 review 缺口修复（轮次 2）：存在性校验收敛为**唯一一处、最后一道**——删掉 Step 6 之后那次对 `threadsByCluster` 的调用，改在 Step 7.5 watch 物化追加之后、`return report, sections, threadBatches, nil` 之前对**整个 `threadBatches`**（`[]repository.DailyReportThread`，`RelatedArticleIDs` 是 jsonb 原始字节）做一次校验；`decodeThreadArticleRefs` 把非数组（JSON `null`/SQL NULL/对象）读作空集合并规范化写回 `[]`，无法解析的值原样保留 + Warn；单测新增「物化批次（`mustMarshalUintArray` 形态）中的候选被删 → 剔除且保序、全删写 `[]`」与「非数组规范化 / 不可解析保留」用例。

## 4. 存量修复迁移 `20260917_0002`

- [x] 4.1 `postgres_migrations.go` 新增 `healDanglingArticleRefsMigration()`（Version `20260917_0002`，注册在 `postgresMigrations()` 链尾，Description 注明不可逆）：`Up` = `NormalizeThreadRefs` → `PruneDanglingRefs`，`logging.Infof` 记录 rows/refs 计数（不回填 `article_count` 等快照计数）。
- [x] 4.2 PG 迁移测试 `heal_dangling_article_refs_migration_test.go`：悬空剔除（保序）、JSON null/SQL NULL → `[]`、幂等重跑命中 0 行且零 UPDATE、无悬空时零写入；构造真实形态（loser/keeper 对 + 历史悬空混合）。
- [x] 4.3 迁移效果验证 SQL（部署后人工核对，落 `verification.sql`）：悬空引用计数 = 0、`jsonb_typeof` 非 array 行 = 0、今日 9 条线索 refs 全部可解析。

## 5. 巡检（只读告警）

- [x] 5.1 `backend-go/internal/admin/scheduler/job_daily_report.go`：当日报告收尾调用 `articlerefs.CountDanglingArticleRefs`，>0 用 `logging.Warnf`、查询失败用 `logging.Warnf` 且不影响 job 成功。
- [x] 5.2 单测（轮次 3 补齐落点）：计数器的三种分支（0 / >0 / 查询失败）在 `articlerefs/repair_test.go::TestCountDanglingArticleRefs`；job 胶水的降级语义在 `admin/scheduler/job_daily_report_test.go::TestDailyReportJobSucceedsWhenDanglingRefProbeFails`（SQLite schema 无 `daily_report_threads` 且不支持 LATERAL → 探针必失败 → job 仍成功、report_count 正常），两处注释互相指引。
- [x] 5.3 非空性加固（轮次 5，review L1）：同一 job 测试改为**直接断言** `articlerefs.CountDanglingArticleRefs(db)` 返回 error（不再靠 `HasTable` 间接推断 + 不再写「failure is asserted by the warn log」这种与实际断言不符的注释）——探针静默退化为 `(0, nil)` 时该测试会直接红。

## 6. 测试

- [x] 6.1 影响包（`bash scripts/change-scope.sh` 判定，至少）：`go test ./internal/platform/articlerefs/... ./internal/platform/database/... ./internal/reader/... ./internal/topicgraph/... ./internal/admin/scheduler/...`
- [x] 6.2 迁移 PG 集成测试全绿（testcontainer，禁用 SQLite）；脏元素与空 ids 边界有断言
- [x] 6.3 后端门禁四件套归档前跑（§11）
- [x] 6.4 前端文案改动（H1 处置）门禁：`cd front && pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm test:unit`（ui-impact 已提为 minor，见 ui-design.md）

## 7. 文档

<!-- doc-impact: flow database -->

- [x] 7.0 **H1 处置（用户决策：只改文案）**：`front/app/features/shell/components/FeedLayoutShell.vue` 删分类确认文案由「这个操作不会删除分类下的订阅源。」改为「该分类下的订阅源及其文章也会一并删除，且不可撤销。」；`proposal.md` 头 `ui-impact` 由 none 提为 **minor**，`ui-design.md` 改写为 minor 形态（复用契约 + 双视口人工验收）。

- [x] 7.1 `docs/reference/flow/daily-report.md` §业务约束与不变量：新增约束 21「thread 引用完整性」——四条删行路径（去重归并 / 删订阅源 / **删分类两级级联** / 仓库删行原语）MUST 同事务先维护引用再删行；**删除实现 SHALL 显式、不依赖 FK 是否存在**；日报写库前 MUST 做一次存在性校验（覆盖常规 + watch 物化轨）；数组保序去重、空写 `[]`、禁 JSON `null`；历史「只剪不映射」与快照计数不回填；巡检只读告警。
- [x] 7.2 `docs/reference/flow/reading.md` §业务约束与不变量（约束 6）：补「删订阅源/删分类会连带删文章，删除路径 MUST 同步维护按 ID 引用的 jsonb 数组（显式删除，不依赖 FK）」。
- [x] 7.3 `docs/reference/database/`：（a）`tables/_conventions.md` **修正过时外键清单**——原文「DB 级外键全库共 6 条」实测为 **25 条**（两条级联 FK `fk_feeds_articles` / `fk_categories_feeds` 为 AutoMigrate 时代遗留，存于真库、不入新建库），并写明「`DisableForeignKeyConstraintWhenMigrating` 只阻止新建、不清除存量」+ 删行路径清单与显式删除约束；（b）`tables/daily-report-watch.md` §9.3 登记 `related_article_ids` 列语义、写入校验与修复迁移 `20260917_0002`。`DATA_LIFECYCLE.md` 无删除语义变更（其「无 TTL 删除」清单已含 articles），不改。
- [ ] 7.4 两份 flow 文档「变更溯源」表追加本 change 行（§12，归档后随归档 commit 一并提交）。
- [x] 7.5 `bash scripts/doc-impact.sh verify openspec/changes/heal-dangling-article-refs` 对账通过（声明域 flow database，4 个文档变更）。

## 8. 验证

| Scenario | 测试文件 |
| --- | --- |
| 归并副本时引用改指保留条 | backend-go/internal/platform/database/dedupe_rss_articles_migration_test.go |
| 保留条已在数组内时只去重 | backend-go/internal/platform/database/dedupe_rss_articles_migration_test.go |
| 删除订阅源时引用被剔除 | backend-go/internal/reader/repository/article_refs_test.go |
| 删除分类的两级级联同样维护引用 | backend-go/internal/reader/repository/article_refs_test.go::TestDeleteCategoryCascadePrunesArticleReferences |
| 无引用命中时不产生写操作 | backend-go/internal/platform/articlerefs/rewire_test.go + backend-go/internal/reader/repository/article_refs_test.go::TestDeleteCategoryCascadeLeavesUnrelatedRowsAlone（xmin 零写）|
| 单引用的空数组规范化 | backend-go/internal/platform/articlerefs/rewire_test.go |
| 悬空 id 被剔除 | backend-go/internal/platform/database/heal_dangling_article_refs_migration_test.go |
| JSON null 规范化为数组 | backend-go/internal/platform/database/heal_dangling_article_refs_migration_test.go |
| 重复执行不改变已修复数据 | backend-go/internal/platform/database/heal_dangling_article_refs_migration_test.go |
| 无悬空数据时零写入 | backend-go/internal/platform/database/heal_dangling_article_refs_migration_test.go |
| 候选文章在写库前被删除 | backend-go/internal/topicgraph/service/daily_report_article_filter_test.go |
| 全部候选都已消失 | backend-go/internal/topicgraph/service/daily_report_article_filter_test.go |
| 存在性校验查询失败时降级 | backend-go/internal/topicgraph/service/daily_report_article_filter_test.go |
| 日报 job 收尾记录悬空计数 | backend-go/internal/platform/articlerefs/repair_test.go::TestCountDanglingArticleRefs（计数器）+ backend-go/internal/admin/scheduler/job_daily_report_test.go::TestDailyReportJobSucceedsWhenDanglingRefProbeFails（胶水降级） |
| 计数查询失败不影响 job | backend-go/internal/admin/scheduler/job_daily_report_test.go::TestDailyReportJobSucceedsWhenDanglingRefProbeFails |
| 人工：部署后悬空引用清零核查（今日 9 条 + 历史 5595 个引用） | change 目录 `verification.sql`（不设测试文件） |
| 人工：今日日报线索展开不再出现「文章 #id」降级条目 | 打开 🔗 TagsPage 今日日报线索展开抽查（不设测试文件） |

- [x] 8.1 `cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...`：0 issue、0 error
- [x] 8.2 `cd backend-go && go test ./internal/platform/articlerefs/... ./internal/platform/database/... ./internal/reader/... ./internal/topicgraph/... ./internal/admin/scheduler/...`：全绿
- [ ] 8.3 真库效果核对（部署后）：跑 `verification.sql`，悬空引用计数 = 0、非数组行 = 0
- [ ] 8.4 重跑今日日报生成一次后复查悬空计数仍为 0（验证写路径过滤生效）
- [ ] 8.5 完工汇报含「部署后影响 + 需要的操作」：迁移随启动自动跑（幂等、不可逆，建议先 `pg_dump`）；**历史日报线索的文章列表会少掉死链条目**（悬空引用被剪）、「N 篇」快照计数不重算；**删订阅源/删分类现在显式删除其文章行（含依赖行）**，在无 FK 的新建库里也从「只删 feed/分类」变为「连带删文章」（真库行为不变）；旧数据无需人工处理
- [x] 8.6 增量 review（轮次 3/4 新代码）已跑并处置：见 `review-report-round2.md`
