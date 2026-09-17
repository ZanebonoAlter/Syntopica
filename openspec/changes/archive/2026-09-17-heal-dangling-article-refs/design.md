# Design — heal-dangling-article-refs

## 背景（实测事实，2026-09-17 快照）

**症状**：日报线索（`daily_report_threads`）的 `related_article_ids` 指向已不存在的文章行 → 前端 `getArticle(id)` 404 → 降级显示「文章 #129447」，线索追溯不到来源。

**当天新增悬空**（period_date 2026-09-16，共 9 条线索 / 9 个引用）：

| report | board | thread | 悬空 article id | thread 写库时间 |
| --- | --- | --- | --- | --- |
| 807 | 1974 | 10173 / 10181 | 128606 / 129500 | 23:24:10 |
| 808 | 1980 | 10186 / 10188 / 10190 / 10212 | 129554 / 129425 / 129461 / 129447 | 23:25:00 |
| 811 | 2197 | 10227 | 128606 | 23:26:52 |
| 814 | 3030 | 10264 / 10274 | 129447 / 129403 | 23:29:35 |

**根因**：文章行被物理删除时，没有同步维护「按 ID 引用文章」的 jsonb 数组。悬空 id 邻域全是重复对空洞（129424/129426 在、129425 没；129402/129404 在、129403 没），线索 10264 的 refs = `[129446, 129447]`（129446 存、129447 删）——正是 `dedupe-rss-articles` 存量归并迁移（`20260917_0001`，23:28:59 应用）删掉的 loser 副本；那批重复来自 21:23 唯一索引建立前的连续两次刷新。

**全库删除路径清单（grep + `pg_constraint` 实测，2026-09-17）**：

| # | 路径 | 现状 | 是否维护引用 |
| --- | --- | --- | --- |
| 1 | `mergeDuplicateArticleGroup`（`postgres_migrations.go:2561` `DELETE FROM articles`） | 归并 loser 副本 | ❌ **缺失（本次主因）** |
| 2 | 删订阅源 → 存量 FK `fk_feeds_articles`（`articles.feed_id → feeds(id)` ON DELETE CASCADE，`handler.DeleteFeed` 删 feed 行触发） | 级联删该源全部文章 | ❌ 缺失 |
| 3 | 删分类 → 存量 FK `fk_categories_feeds`（`feeds.category_id → categories(id)` ON DELETE CASCADE）**两级级联**（分类 → feed → 文章，`handler.DeleteCategory` 触发） | 级联删该分类下全部文章 | ❌ 缺失（review H1 发现） |
| 4 | `DeleteArticlesByFeed` / `DeleteCascadeByFeed`（reader 仓库 API，**当前无调用方**） | 原始删行原语 | ❌ 缺失 |

> **FK 事实修正（review H1 结案）**：`docs/reference/database/tables/_conventions.md` 原写「DB 级外键全库共 6 条、其余均为 GORM 逻辑关联」是**过时错误**文档——`DisableForeignKeyConstraintWhenMigrating: true` 只阻止**新建** FK，不清除存量。`pg_constraint` 实测全库 **25 条** FK，上表两条级联 FK 均在生效。本 change 顺带修正该文档（见 tasks §7.3）。

**jsonb 引用面盘点（全库 jsonb 列逐列抽样）**：唯一持本地文章 ID 的是 `daily_report_threads.related_article_ids`（7793 array + 3 个 JSON `null`）。`raw_clusters`（lane/tag_ids/group_name）、`board_upgrade_suggestions.evidence`、`cross_board_relations.evidence`（外部 URL）、`topic_enrichment_result.input_snapshot`（文本 digest）均不含本地文章 ID。标量列中 `article_topic_tags` / `reading_behaviors` / `tag_jobs` / `firecrawl_jobs` 已由归并迁移重指；`topic_analysis_cursors.last_article_id` 是高水位游标（比大小推进，指向行被删不影响语义）。

**存量规模**：4774 条线索 / 5595 个引用悬空（跨 12 板块，集中在 7~8 月，来自 2026-08-19 前的物理删除旧账）；3 条 thread 的 `related_article_ids` / `tag_ids` 为 JSON `null`（`jsonb_array_elements_text` 直接抛 22023，生产 SQL 靠 `jsonb_typeof` 守卫存活）。

## D1 维护器放哪、什么形状

新增包 `backend-go/internal/platform/articlerefs`（纯 SQL、只依赖 `*gorm.DB`，不 import 任何 domain 包，避免 `platform → topicgraph/repository` 的反向依赖）：

```go
// 一条语句替换：老 id → 保留 id（保序，数组内去重，空数组写 '[]'）
func RewireArticleRefs(db *gorm.DB, oldID, keeperID uint) (int64, error)
// 剔除这批 id（保序，空数组写 '[]'）
func PruneArticleRefs(db *gorm.DB, ids []uint) (int64, error)
// 分批剔除全库悬空引用（修复迁移用；batch<=0 用默认 500）
func PruneDanglingRefs(db *gorm.DB, batch int) (rowsTouched int64, refsRemoved int64, err error)
// 规范 JSON null / SQL NULL → '[]'
func NormalizeThreadRefs(db *gorm.DB) (int64, error)
// 巡检：悬空引用计数（只读）
func CountDanglingArticleRefs(db *gorm.DB) (int64, error)
```

**实现方式：Go 侧计算 + 按主键 UPDATE，不用纯 SQL 聚合**。理由：`jsonb` 数组保序需要 `WITH ORDINALITY` + `DISTINCT ON` 的嵌套子查询，可读性与可测性都差；而目标表只有 7796 行（历史全量修复也就 4774 行命中），分批（每批 500）在 Go 里重写既直观又能单测。查询命中用 `jsonb_typeof(related_article_ids) = 'array' AND related_article_ids ? ?`（jsonb `?` 对数组按元素文本匹配，无需 GIN 索引也能全表扫 7.8k 行）。

**语义约定**（三条路径共用，测试逐条覆盖）：
- 保序：保留原数组顺序，只做替换/剔除（前端 `related_article_ids.slice(0, 10)` 依赖顺序稳定）。
- 去重：改指后若保留条 id 已在数组内，重复项只保留首次出现。
- 空数组：写 `'[]'::jsonb`，绝不写 `'null'`（`jsonb_typeof` 守卫不普及，裸 null 会让朴素查询报错）。
- 幂等：无命中的调用不产生 UPDATE（`RowsAffected=0`）。

## D2 两条活删除路径接线

1. **归并迁移**（`mergeDuplicateArticleGroup`，同事务）：在 `DELETE FROM articles WHERE id = ?` **之前**调用 `RewireArticleRefs(db, r.ID, keeper)`；错误直接上抛（迁移事务回滚，宁可整批失败也不留死链）。
2. **删订阅源**：把 `handler.DeleteFeed` 里内联的「删 reading_behaviors + 删 feed 行（靠 FK 级联删文章）」收敛到仓库方法 `DeleteFeedCascade`，同一事务内：删 `reading_behaviors`（NO ACTION FK 要求先行）→ pluck 本 feed 文章 id → `PruneArticleRefs` → 删依赖行 → 删文章行 → 删 feed 行。既有的 `DeleteArticlesByFeed` / `DeleteCascadeByFeed` 复用同一核心（原始原语自身安全，未来调用方不会再踩），同样无调用方的 `DeleteFeed` 裸原语已删除。
   - 行为等价性：删除靠存量 FK 级联与显式删除净结果一致（同一批行消失），多出来的只是引用维护与依赖行显式清理。
3. **删分类两级级联**（review H1）：新增 `DeleteCategoryCascade(categoryID)`——单事务内：pluck 该分类下全部 feed id → 删文章（`deleteArticlesOfFeedsWithRefs`：prune 引用 → 删依赖行 → 按 `articleDeleteChunk=1000` 分块删文章）→ 删 feed 行 → 删 category 行。`handler.DeleteCategory` 改走该方法。**不依赖 FK 级联**（那是历史遗留物、新建库没有），否则「只对真会消失的行剪引用」会在无级联库里变成剪活引用。**不动** `reading_behaviors`/`user_preferences` 的 NO ACTION FK 行为（删分类若有这些子行仍会 FK 报错，属既有行为）。
4. **依赖行显式清理（轮次 4）**：`deleteArticleDependents` 对将删文章显式删 `article_topic_tags` / `tag_jobs` / `firecrawl_jobs`（同样不依赖 FK）；`topic_tags` 孤儿仍归 `aux_label_cleanup` 维护任务。表存在性在分块循环**外**解析一次（`existingArticleDependents`）。

## D3 日报写路径加存在性校验（压缩 TOCTOU 窗口）

今天的 814 号日报是「生成时读到 id、写库前该行被删」的形态（collectBoardTags 在 23:28:27 读到 129447，写库 23:29:35，迁移 23:28:59 删行）。光靠删除路径接线无法覆盖这个窗口，因此：

orchestrator 在**最终 `threadBatches`**（Step 7.5 watch 物化追加之后、`return` 之前）做一次存在性校验：把全部候选 `RelatedArticleIDs` 收集起来，一次 `SELECT id FROM articles WHERE id IN (...)`（按 1000 个一批），只保留仍存在的 id 写入。单份日报候选量级为当天文章数（实测 ≤ 数百），一次额外查询，成本可忽略。

**措辞纪律（review M3）**：这只能把窗口从数十秒压到毫秒级（探针与 `SaveReport` 事务之间仍有一次往返），**不声称「关闭」**；真正的端到端堆底是巡检（D6）。物化轨（`MaterializeSentenceWatch`/`MaterializeKeywordWatches`）走「查文章 → LLM 裁决数十秒 → 写库」，是窗口最大的一条，必须包含在同一轮校验里。

## D4 写路径卫生（JSON null 来源）

`marshalJSONArray`（`marshal_array.go`）已把 nil/空切片规范化为 `[]`，并写明「裸 null 会让 `jsonb_array_elements_text` 抛 22023」；但 thread 写入路径没用它：

- `daily_report_orchestrator.go:281-282`：`json.Marshal(th.TagIDs)` / `json.Marshal(th.RelatedArticleIDs)` → 改 `marshalJSONArray(...)`。
- `daily_report_merge.go:292`：`mergedTagIDs` → 同上。

（`cluster.TagIDs` 已用 helper，`highlights` / `clusters` 是对象数组不是 ID 数组，不动。）

## D5 存量修复迁移 `20260917_0002`

`Up` 顺序：① `NormalizeThreadRefs`（JSON null / SQL NULL → `[]`，保证后续 `jsonb_typeof='array'` 能命中）；② `PruneDanglingRefs`（分批 500，日志 `rows/refs` 计数）。放 `postgresMigrations()` 注册链（`healDanglingArticleRefsMigration()`，排在 `laneSnapshotFKMigration()` 之后；`migrationsSorted()` 按 Version 排序，顺序安全）。不做 Down（修复类迁移不可逆，与仓库既有约定一致），Description 里写明不可逆。

不重建 `daily_report_sections.article_count` / `board_daily_reports.article_count`：它们是**生成时快照**（flow 约束口径），历史不重算；修复只动引用数组。

**为什么只剪不映射到保留条**：归并迁移没有记录 loser→keeper 映射，历史悬空 id 的 (feed_id, link) 已随行消失，无法可靠推断目标；且重复副本的内容等价行通常已在同一数组内（线索 10264 即如此：剪掉 129447 后 129446 仍是有效来源）。新删除路径（D2）从今天起保证「改指保留条」，所以只有历史旧账走「剪除」。

## D6 巡检（只读告警）

`CountDanglingArticleRefs` 挂到日报生成 job 收尾（每天一次，一条 count 查询），日志：`daily-report: dangling article refs=%d`；>0 时用 Warn。不自动删数据（避免掩盖未知删除者），只作观测面 + 后续排查入口。

## 风险与回滚

| 风险 | 处理 |
| --- | --- |
| 迁移在启动事务里跑，全量修复耗时 | 分批 500 + 单条 UPDATE，实测命中 4774 行；仍在启动事务内（可接受，无 DDL） |
| 剪除历史悬空后「N 篇」计数与列表长度不一致（计数是快照，不重算） | 已知取舍：快照计数不回填，列表不再出现死链条目；部署汇报中明确告知 |
| `?` 操作符对 jsonb 数字元素匹配 | 用元素文本比较（`related_article_ids ? '129447'`），单测 + PG 迁移测试双重覆盖 |
| 归并迁移里新增 rewire 失败会中断整批 | 有意为之：死链比整批失败更贵；错误上抛让事务回滚 |
| **并发插入的窗口**（review L4） | 删除路径按 pluck 快照删行：pluck 与删 feed 之间若有刷新任务为该 feed 插入新文章，无 FK 库会留下 `feed_id` 悬空的孤儿文章（有遗留 FK 的真库会被级联删掉）。取舍：宁可留孤儿也不剪活引用；已记入残余风险，不在本 change 修复 |

## 不做（明确划界）

- 不改归档语义（`archived=true` 保留行）、边时间窗 GC、日报生成算法、前端组件（**除删分类确认文案一处**，见 D2.5）。
- 不改删分类的破坏性语义（用户 2026-09-17 决定：只改文案、保持删除语义）；不修标签层的同类 FK 级联隐患（轮次 4 报告已列，属独立 change）。
- 不为 `related_article_ids` 建 GIN 索引（7.8k 行全表扫足够，避免过度设计）。
- 不改 `topic_analysis_cursors.last_article_id`（高水位游标，语义与引用无关）。
