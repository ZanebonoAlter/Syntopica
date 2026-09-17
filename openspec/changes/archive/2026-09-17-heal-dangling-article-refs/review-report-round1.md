## Review — `heal-dangling-article-refs` 已实现代码（只读）

先给结论：**归并/删源两处接线的 SQL 语义、顺序去重、事务边界、分批游标、写路径收敛、JSON null 卫生——我逐条核对过，没有发现问题**。唯一让我不能放行的是**「删订阅源这条路径的行为等价前提（`articles.feed_id ON DELETE CASCADE`）」在本仓库内部自相矛盾**，而它不是措辞问题：结论不同，本 change 对「删订阅源」的净效果就从「无变化」变成「新增一次物理删行」。下面按你要的 High/Medium/Low 汇报。

---

## High

### H1. 删订阅源的「行为等价」前提与本仓库权威 DB 文档直接冲突；两种真值下都有实际后果

**问题**：design 把「`feeds` 删除 → `articles.feed_id ON DELETE CASCADE`」当成既有事实链（`openspec/changes/heal-dangling-article-refs/design.md:23` 的路径 #2、`design.md:59`「今天靠 …ON DELETE CASCADE 隐式删文章，改造后显式删同一批行，**净数据结果一致**」、`proposal.md:14`「全仓能删文章行的只有两处」、`explore-findings.md` 第 2 条「**实测 FK 存在，ON DELETE CASCADE**」）。但仓库自己的数据库权威文档写的是相反的：

- `docs/reference/database/tables/_conventions.md:22`：「`constraint_name` 形如 `fk_feeds_articles`、`fk_categories_feeds` **在数据库里并不存在**」
- `_conventions.md:76-87`「DB 级外键（全库共 6 条，权威清单）」里**没有** `articles.feed_id`；`:87`「其余所有表间关联均为 GORM 逻辑关联，**DB 层未强制**」
- 代码侧也支持「不存在」：`backend-go/internal/platform/database/db.go:21` 与 `db.go` 的 `DisableForeignKeyConstraintWhenMigrating: true`（`_conventions.md:19` 明说这条），迁移里只有 `20260601_0001` 显式 DROP/ADD 过若干 `fk_*`，**没有任何迁移给 articles 加过 FK**（`grep 'FOREIGN KEY' postgres_migrations.go` 命中的 8 处无一处涉及 articles/feeds）。

**为什么重要（两种真值都有具体后果）**：

- **若 FK 不存在**：改造前 `handler.DeleteFeed` 只删 `reading_behaviors` + feed 行（见 `postgres_migrations.go` 同族写法的对照与 `handler` 旧注释意图），文章行**留在库里**；改造后 `reader/repository/repository.go:286-295 DeleteFeedCascade` 会**显式硬删这批文章行**（`repository.go:253-263`）。也就是说这是**新增的一次不可逆删行**，而不是 design 声称的「等价改造」——文案层面「部署后行为无变化」（tasks 8.5 的汇报口径）会失真；同时被删文章的依赖行**没有**级联：`article_topic_tags` / `tag_jobs` / `firecrawl_jobs` 也没有 DB 外键（`_conventions.md:127-131` 明确列为「DB 层未强制」），而**归并路径是专门清它们的**（`postgres_migrations.go:2570-2600`：改指/删除 `article_topic_tags`、`reading_behaviors`、`tag_jobs`、`firecrawl_jobs`），删源路径没有对应清理 → 新增一批永久孤儿边（`flow/reading.md:125` 的边回收「仅限已归档文章」，未归档文章的孤儿边不会被回收，只会线性累积并影响共现/升级建议口径）。另外此时 `design.md:23` 那条路径根本不会产生悬空引用，「删源」在根因分析里的作用需要重述。
- **若 FK 存在**：按 `_conventions.md` 的 B-1 矩阵，`feeds.category_id → categories` 的 `OnDelete:CASCADE` 是同源同时期的声明（`models/category.go:17`），它存在的话，`DELETE /api/categories/:category_id`（`reader/routes.go:15` → `reader/handler/category_handler.go:178` → `reader/repository/repository.go:72-74` 的裸 `db.Delete(cat)`，**无任何守卫**）就会「删分类 → 级联删 feeds → 级联删 articles」，成为**第三条删行路径**，且完全没有接 `articlerefs`（GORM 的 `Delete` 不会模拟级联，是否发生完全由 DB FK 决定）。这条路径 today 就会继续造悬空引用，只能靠日报 job 的巡检 Warn 事后发现——正是本 change 想根治的形态。

**建议动作（阻断归档，直到有结论）**：在真库上跑一次判定，按结果二选一：

```sql
SELECT conrelid::regclass AS tbl, conname, pg_get_constraintdef(oid)
FROM pg_constraint
WHERE contype='f' AND conrelid IN ('articles'::regclass, 'feeds'::regclass, 'categories'::regclass);
```

- 若无 `articles.feed_id → feeds`：把 `design.md:59` / `proposal.md:14` / `flow/reading.md:128` / `flow/daily-report.md:227` / `_conventions.md:211` / `repository.go:282-284` 里「靠 FK 级联」的机制表述改成「显式删行」，在完工汇报里明确写「删订阅源现在会物理删除其全部文章行」，并补上依赖行（`article_topic_tags`/`tag_jobs`/`firecrawl_jobs`）的处置决策（清或明确声明留孤儿）。
- 若有该 FK：删分类路径必须进路径清单并接线（或加守卫拒绝删除含 feed 的分类），`flow/daily-report.md:225-228` 的「三条路径」要改成四条。

> 说明：我不把这条判成「已证实的数据删除 bug」，因为两处证据互相矛盾、我无法在只读环境里跑 SQL 定论；但判 High 是因为**这个前提决定的正是「改完之后删订阅源会不会真删文章」**，属于归档前必须落地的事实。

---

## Medium

### M1. `repository.DeleteFeed` 仍是未接线的「删 feed 行」原语，与本 change 自己的「原语自身安全」原则不一致

**证据**：`backend-go/internal/reader/repository/repository.go:130-132` `DeleteFeed(feed *models.Feed)` 仍是裸 `r.db.Delete(feed)`，全仓无调用方（`grep 'DeleteFeed\('` 只命中 handler 与它自己）。本 change 特意给 `DeleteArticlesByFeed`/`DeleteCascadeByFeed` 补了 prune（design.md:58「原始原语自身安全，未来调用方不会再踩」），却漏了同类的 `DeleteFeed`。
**为什么重要**：在 H1「FK 存在」的分支下，未来的调用方一旦用它删 feed 就会绕过引用维护，直接复现本次故障；在「FK 不存在」的分支下它同样语义模糊（只删 feed 行、留孤儿文章）。
**建议动作**：不阻断归档，但建议把 `DeleteFeed` 删除或改为委托 `DeleteFeedCascade`，并在 `flow/daily-report.md` 约束 21 的路径清单里显式点名这两个原语（现在只写了「任何新增的删行原语」）。

### M2. D6（巡检）的两个 Scenario 实际上没有对应断言，且 job 自己的测试环境根本跑不了这条探针

**证据**：`tasks.md:70-71` 把「日报 job 收尾记录悬空计数」「计数查询失败不影响 job」都映射到 `articlerefs/repair_test.go`，但该文件只有 `TestCountDanglingArticleRefs`（`repair_test.go:129-158`），断言的是**计数器本身**（0 / 3 / nil db / 不可达 DSN），**没有任何测试断言 job 里那 4 行日志**（`admin/scheduler/job_daily_report.go:127-133`）。而 job 的既有测试全部跑 **SQLite**（`admin/scheduler/job_daily_report_test.go:48`），我核对了自动插入的 SQLite schema 里连 `daily_report_threads` 都没有 AutoMigrate（`job_daily_report_test.go:60-70` 的表清单），探针用的 `CROSS JOIN LATERAL jsonb_array_elements_text(...)`（`repair.go:107-117`）在 SQLite 上必然报错 → 走 `logging.Warnf` 分支 → **job 照常成功，测试静默通过**。apply-report §5 遗留 3 已自认「job 内那 4 行日志胶水未单独测试」，但 tasks §8 与 spec Scenario 的措辞（「日志出现 `dangling article refs=` 计数，计数大于 0 时级别为 Warn」）会让人以为有覆盖。
**为什么重要**：D6 是「有未知删除者」的唯一观测面；缺覆盖 + 测试环境必然失败组合起来，日后有人改动这段胶水（比如漏判 `danglingErr`）不会有任何测试报警。
**建议动作**：不阻断归档。最小修正——在 tasks §8 的这两行标注真实落点（`articlerefs/repair_test.go` 只覆盖计数器，job 胶水未测），或加一个 PG 的 job 级用例（断言 `dangling>0` 时仍返回成功结果）。

### M3. 写路径只把 TOCTOU 窗口「缩到毫秒」，不是 design 说的「关闭」

**证据**：探针在 `topicgraph/service/daily_report_orchestrator.go:401`（`GenerateDailyReport` 返回前）执行，而 `SaveReport` 的事务在 `topicgraph/service/daily_report_watch.go:56→64` 之后才开（`topicgraph/repository/daily_report_repository.go:183` 起）。两者之间仍有一次查询往返（外加 watch 物化已完成，所以只剩这一小段）。
**为什么重要**：`proposal.md`「关闭 TOCTOU 窗口」/`flow/daily-report.md:230`「关闭…窗口」的措辞会被当成硬保证；实际语义是「窗口从数十秒缩到毫秒级 + 巡检兜底」。
**建议动作**：不阻断。要么在 `SaveReport` 事务内做最后一道校验（成本很低：一次 `SELECT id FROM articles WHERE id IN (...)`），要么把文档措辞改成「把窗口压缩到毫秒级；仍以巡检兜底」。

---

## Low（判断过、不值得阻断的点）

1. **账本漂移**：`test-cases.md:10/14` 仍写旧文件名 `articlerefs_integrity_migration_test.go`，`:18` 把巡检 Scenario 指向 `articlerefs/rewire_test.go`（实际在 `repair_test.go`）；tasks 4.2/§8 已改名但 test-cases.md 未同步。同时 tasks 7.x/8.x 仍 `[ ]`，而文档条目其实已落地（`flow/daily-report.md:225-230`、`flow/reading.md:128`、`_conventions.md:211`、`tables/daily-report-watch.md:80`）。归档前对账一次即可。
2. **测试注释延续了 H1 的错误前提**：`articlerefs/helpers_test.go:14-16` 写「articles.feed_id carries a foreign key, so every seed needs its own feed」，`reader/repository/article_refs_test.go:63-65` 写「articles used to disappear through the foreign-key cascade」。行为无影响（测试自己造 feed），但会在 H1 结论出来后一起改。
3. **verification.sql 的 ④ 是 ① 的原样复制**，标签却写「归并路径的引用改指（可抽查近 7 天合并过的组）」——查的不是这件事，容易在人工核对时造成「已核对过改指」的错觉。
4. **脏值策略两套**：`articlerefs.parseRefIDs`（`articlerefs.go:76-86`）对非数组/不可解析 → 丢弃并重写；`service.decodeThreadArticleRefs`（`daily_report_article_filter.go:24-46`）对不可解析 → 保留 + Warn。jsonb 列本身不可能存非法 JSON（PG 侧校验），所以这条分支从 DB 读不到，属报告项。
5. **「无悬空数据时零写入」比实现宽松**：`rewriteRefIDs` 会顺带去重所有重复元素（`articlerefs.go:96-121`），`PruneDanglingRefs` 也会重写含重复元素/不可解析元素的干净行（`repair.go:79-100`）。这类数据上迁移**会有 UPDATE**，语义无害（D1 本就要求去重），只是 spec Scenario 的措辞与实测不完全等价。
6. **单参数无上限**：`PruneArticleRefs`/`idStrings` 把整个 feed 的文章 id 合成**一个** jsonb 参数（`rewire.go:60-70` + `articlerefs.go:150-175`），`articleIDChunk=1000` 只用在存在性查询（`repair.go:120-152`）。`max_articles=9999` 视为无上限的源若有数万篇文章，删源事务里会出现一个数百 KB 的 `IN (SELECT jsonb_array_elements_text(?))` 匹配。当前量级（全库 articles ~13 万行）无碍，属余量问题。
7. **重复实现**：`DeleteReadingBehaviorsByFeed`（`repository.go:134`）已无调用方，`DeleteFeedCascade`/`DeleteCascadeByFeed` 各自内联同一段 behavior 删除；可合并，非问题。
8. **JSON null *元素***（`[null, 1]`）会被 `CountDanglingArticleRefs` 记为悬空（`repair.go:107-117` 的 `NOT EXISTS` 对 NULL 元素成立），迁移会剪掉它；只有当某个写路径造出这种元素时才会出现一次假 Warn。
9. `DATA_LIFECYCLE.md` 未同步（tasks 7.3 标了「若有相关段则同步」，其「无 TTL 删除」清单 `:284` 已含 articles，语义没变，可不改）。

---

## 已核对且确实正确（避免你重复验证）

- **保序 + 去重（首个为准）**：`rewriteRefIDs`（`articlerefs.go:96-121`）+ 用例 `rewire_test.go:TestRewireArticleRefsPointsReferencesAtKeeper` / `TestRewireArticleRefsPreservesOrderOnLongArrays` / `dedupe_rss_articles_migration_test.go:192-231`（三条形状：loser 在中间、loser 在前且 keeper 已在、无引用）逐条落到断言上。
- **文本比对避 cast**：全部用 `a.id::text = e.elem`（`repair.go:110`）与 `e.elem IN (SELECT jsonb_array_elements_text(?::jsonb))`（`articlerefs.go:161-164`）；无 `::bigint`；脏元素用例 `rewire_test.go:TestPruneArticleRefsDropsDirtyElements` 真跑。
- **JSON null / SQL NULL 安全**：读侧统一 `CASE WHEN jsonb_typeof(...)='array' THEN ... ELSE '[]'::jsonb END`（`articlerefs.go:69`），三条查询（load / count / verification.sql）都带守卫；`NormalizeThreadRefs` 用 `jsonb_typeof(...) IS DISTINCT FROM 'array'` 覆盖 SQL NULL（`repair.go:21-25`），用例 `TestNormalizeThreadRefsRewritesNonArrays` 覆盖两形态。
- **PruneDanglingRefs 游标**：`after = rows[len(rows)-1].ID` + `WHERE t.id > ? ORDER BY t.id LIMIT ?`（`repair.go:47-70`），id 严格递增 → 无漏行、无重复处理、`len(rows)==0` 必然终止；`batch=1` 走完全表的用例 `TestPruneDanglingRefsBatchBoundaries` 真的跑了。
- **归并路径的顺序与错误传播**：`postgres_migrations.go:2606-2615` 先 `RewireArticleRefs` 再 `DELETE FROM articles`，错误 `return` 上抛；迁移经 `migrator.go:170-190` 的 in-tx 路径执行（Up + 版本记录同事务），失败整体回滚、版本不记录。归并只走 `postgres_migrations.go:2484` 一个入口，传的就是 tx。
- **DeleteFeedCascade 顺序**：`repository.go:286-295` behaviors → （`deleteArticlesWithRefs`：pluck → prune → 删文章）→ feed 行；`DeleteArticlesByFeed`（`:266-270`）与 `DeleteCascadeByFeed`（`:272-279`）各自事务包裹，pluck→prune→delete 全在同一事务内。`reading_behaviors` 无软删（`models/reading_behavior.go` 无 `DeletedAt`），`articles` 亦无（`models/article.go` 无 `DeletedAt`），所以是硬删语义，与 design 一致。
- **迁移 `20260917_0002`**：注册在链尾并按 `migrationsSorted()`（`migrator.go:217-223`）字符串排序，`20260917_0002 > 20260917_0001` 正确；`tableExists` 守卫（`postgres_migrations.go:2422`，实现见 `:28-34`）必要且正确——`daily_report_threads` 确实由 AutoMigrate 建（`topicgraph/repository/daily_report_register_models.go:12`），窄二进制会缺表，测试 `TestHealDanglingArticleRefsMigrationSkipsMissingTable` 覆盖；`Up` 里没有触碰 `article_count` 之类快照计数（只 UPDATE 引用列），不可逆在 Description 里写明、`Down` 留空（与 `migrator.go:19-24` 的仓库约定一致）。
- **写路径覆盖**：`daily_report_threads` 的生产写点只有 `SaveReport`（`topicgraph/repository/daily_report_repository.go:183`，内部 `:224` 删旧 thread），其唯一生产调用方是 `GenerateAndSaveReport`（`daily_report_watch.go:56→64`）；`RelatedArticleIDs` 的产生点 3 处（`daily_report_merge.go:29` 经 orchestrator `:282` 序列化、`watch_materialize_keyword.go:181`、`watch_materialize_sentence.go:327`）全部落在最终 `threadBatches` 上，而校验在 `daily_report_orchestrator.go:401` 对最终批次执行一次——轮次 2 的缺口修复我认为是**成立且无遗漏**的。`lane_snapshot_repository.go:119/291` 是只读 SELECT。
- **不可解码值 = 保留 + Warn、空结果写 `[]`**：`daily_report_article_filter.go:48-71`（`undecodable` → Warn + 原样保留；`nonArray` → 写 `[]`），`marshalJSONArray`（`marshal_array.go:12-24`）保证 `[]`；用例 `TestApplyExistingArticleRefs` 覆盖 `null`/对象/`[1,` 三形态。
- **xmin 幂等断言可靠（不会假绿）**：PG 任何 UPDATE 都产生新元组和新的 `xmin`（即便值相同），两次 `up(db)` 是**不同事务**（不同 xid），所以只要第二次产生过一次写，`xmin` 必变；反向不成立的情形只有在「同一事务内二次 UPDATE」才会出现，本用例不存在。该断言比 `RowsAffected` 更强，而不是更弱。
- **PG 用例确实跑 testcontainer**：`articlerefs/{rewire,repair}_test.go`、`database/heal_dangling_article_refs_migration_test.go`、`reader/repository/article_refs_test.go`、`topicgraph/service/daily_report_article_filter_test.go` 全部 `testutil.SetupTestDB`（`platform/testutil/testutil.go:159-190`，且 `openGorm` 带 `DisableForeignKeyConstraintWhenMigrating: true` 与生产一致），`-short` 下按约定 skip。
- **无空转断言**：我逐个看过新增用例的断言（含 `TestDeleteFeedCascadeLeavesUnrelatedRowsAlone` 的 `sql-null` 不变式、`TestPruneDanglingRefsRemovesOrphanedReferences` 的 `rows/refs` 计数、`TestFilterVanishedArticleRefsNoCandidatesSkipsProbe` 用不可达 DSN 证明「没发查询」），没有恒真断言。
- **其它删行路径排查**：`grep -rn 'DELETE FROM articles|Delete(&models.Article'` 在生产代码只有 `postgres_migrations.go:2612` 与 `repository.go:263` 两处；归档路径只 `archived=true`（`reader/service/feed_service.go:238-305 CleanupOldArticles` 只 Update，不删行）；无 `TRUNCATE articles`。**未接线的候选只找到两条**：上面的 H1（删分类级联，取决于 FK）与 M1（`DeleteFeed` 原语）。

---

## Merge verdict

**BLOCK（仅 H1）**：代码本身没有发现需要改动的缺陷，H1 是「必须先拿到真库事实再决定改代码还是改文档」的前提校验；M1/M2/M3 建议随归档前的文档修订一并处理，不作阻断。另外归档门禁还有几项 open 项需要补齐：tasks `8.3/8.4`（真库跑 `verification.sql`、重跑当日日报后复检）与 `test-cases.md`「效果核对」表三行仍是「待实现后回填」——它们是本 change 唯一的端到端效果证据；`§7.5 doc-impact verify` 与两份 flow 的 §12 变更溯源自不必说。

需要 supervisor 执行（我只读、未跑任何命令）：

```bash
# ① H1 判定（必须）
docker exec syntopica-postgres psql -U postgres -d syntopica -c \
"SELECT conrelid::regclass AS tbl, conname, pg_get_constraintdef(oid) FROM pg_constraint WHERE contype='f' AND conrelid IN ('articles'::regclass,'feeds'::regclass,'categories'::regclass);"

# ② 后端门禁 + 影响包（复核 apply-report 结论）
cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...
cd backend-go && go test -count=1 ./internal/platform/articlerefs/... ./internal/platform/database/... ./internal/reader/... ./internal/topicgraph/... ./internal/admin/scheduler/...
```