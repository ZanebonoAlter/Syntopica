## Review — heal-dangling-article-refs（轮次 3/4 增量，只读）

范围：`repository.go` 删除路径（轮次 3/4）、`category_handler.go`、`article_refs_test.go` 新增用例、`job_daily_report_test.go` 新用例、tasks.md §2.5-2.10 勾选真实性。轮次 1/2 已定论部分未复核。**我未运行任何命令**（只读），所有行为结论来自源码阅读 + 实现方记录的门禁输出。

### Correct（先说好的）

- **事务与顺序正确**：四个公开入口全部在调用方事务内成对提交/回滚 —— `DeleteArticlesByFeed`（repository.go:340-344）、`DeleteCascadeByFeed`（:346-353）、`DeleteFeedCascade`（:360-370）、`DeleteCategoryCascade`（:387-404）。`deleteArticlesOfFeedsWithRefs` 的 `PruneArticleRefs(tx, …)`、`deleteArticleDependents(tx, …)`、文章行删除（:287、:293、:296）都拿到同一个 `tx`，没有任何一处用 `r.db` 旁路。
- **不可能部分删除**：prune → 依赖行 → 文章行 → feed → category 全在同一条 PG 事务里，任何一步报错（含 `reading_behaviors`/`user_preferences` 的 NO ACTION FK）都让整条事务回滚，prune 一并回滚。真库里有级联 FK 时也不会双删/残留：显式删除先发生，级联再评估时已无行可删（`articles.feed_id` 的行已按 id 删净）。
- **分块无漏行/重复**：`articleIDs[start:end]` 是同一份 pluck 快照上的连续窗口（:284-299），互不重叠、无空隙；`len(feedIDs)==0`（:269）与 `len(articleIDs)==0`（:275）都早返回且**不发 DELETE**，调用方随后照常删 feed/category，不会误判「已删」。
- **依赖表名单与实现对齐**：`deleteArticleDependents` 里硬编码的三个表名与 `TableName()` 完全一致（`article_topic_tags` topic_graph.go:172、`tag_jobs`/`firecrawl_jobs` job_queue.go:32/56），模型的 `Article` 关联写法与三轮表本身都没有软删（无 `gorm.DeletedAt`），所以 `Delete` 是物理删。
- **`DeleteFeed` 原语确已删除且无悬空引用**：全仓 `backend-go` 内 `DeleteFeed` 的命中只剩 handler（feed_handler.go:377）、路由（routes.go:24）、图标清理函数、测试用例名与注释；仓库方法已不存在，无任何调用或接口签名残留。
- **tasks §2.5-2.10 与代码逐条对得上**：2.5 的删除顺序 = repository.go:388-403；2.6 = :72-77 注释 + 方法已删；2.7 三个用例 = article_refs_test.go:207/243/264；2.8 = :251/:268-337；2.9 四个用例 = :285/:316/:345/:369；2.10 四个引用位置全部核对存在且内容相符（article_tagger.go:488-489「All child tables have ON DELETE CASCADE」、hard_merge.go:71、auxiliary_label_service.go:505、job_tag_quality_score.go:46-49）。**没有「勾了没做」**。
- **无调试残留**：全仓 `TEMP-NONVACUITY` / `TEMP ` 0 命中（仅有 apply-report/tasks 的文字提及）。
- **文档侧已同步**（主线程在评审期间刚补完）：daily-report.md:225-236 约束 21、reading.md:128、`_conventions.md:78-90` 与 `daily-report-watch.md:80`；tasks §7.1-7.3/7.5 现已勾选且内容对得上，只有 7.4（归档后溯源）与 §8.3-8.5 仍未勾（属正常待办）。

---

## High

### H1（P1）删分类现在会连带删订阅源/文章/队列行，而前端确认文案明说「不会删除分类下的订阅源」——文案与实现相反，且在无遗留 FK 的库里是本轮**新引入**的破坏性行为

- 证据（实现）：`category_handler.go:208` 改调 `repository.Repo.DeleteCategoryCascade(category.ID)`；`repository.go:387-404` 显式删文章（:395 → :268-301）、显式删 feed（:399）、删 category（:403）。保留的 `DeleteCategory` 原语注释自己写明旧语义：「removes the category row only. It does not remove the feeds (and articles) filed under it」（repository.go:72-74）。
- 证据（前端承诺）：`front/app/features/shell/components/FeedLayoutShell.vue:420-421`
  `if (confirm(\`确定要删除分类 "${categoryName}" 吗？这个操作不会删除分类下的订阅源。\`))` —— 该按钮是 UI 上唯一的删分类入口（AppSidebarView.vue:232 发 `deleteCategory` 事件）。对照：删订阅源的确认框文案是对的（EditFeedDialog.vue:212-214「该订阅源下的文章也会一起删除」）。
- 为什么重要：
  - 真库（有存量 `fk_categories_feeds` CASCADE）：行为没变，文案**早就**是错的（既有缺陷，本轮不算回归）。
  - **无存量 FK 的库**（所有新建部署 / dev / 测试库 —— 这正是 `DisableForeignKeyConstraintWhenMigrating: true` 的后果）：本轮之前删分类只删分类行，feed 与文章留着；本轮之后一次点击会连带删掉 feed 行 + 全部文章行 + 三张依赖表里的 `article_topic_tags`/`tag_jobs`/`firecrawl_jobs` 行，而对话框刚刚向用户保证「不会删除订阅源」。这是用户可见、不可撤销、与承诺相反的数据删除。
  - `proposal.md` 头部 `ui-impact: none` + `ui-design.md`「不改任何前端页面、组件、接口契约或交互」的论断，在「删除语义」这一层不再成立：本 change 确实改变了由 UI 触发的破坏性操作的可观察后果（对 FK-less 库）。
- 建议动作（**建议：不因代码语义阻断归档，但必须在归档前落定处置，否则 §8.5 完工汇报会带着一条错误结论发出**）：
  1. 首选：本 change 顺手把 `FeedLayoutShell.vue:421` 文案改成与实现一致（例如「该分类下的订阅源及其文章也会一起删除，此操作不可撤销」），并把 `proposal.md` 的 `ui-impact: none` 提为 `minor`（纯文案）——注意这需要主线程确认 ui-design-gate 的相关影响（`ui-design.md` 已是 N/A 形态，提档要补最小档）。
  2. 次选：明确定为本 change 之外的独立 change（前端文案修正），但 **§8.5「部署后影响 + 需要用户操作」必须显式写出这一条**（「删分类会连带删除其订阅源与文章」），不能继续沿用 proposal 的「不改接口契约/交互」口径。

---

## Medium

### M1（P2，但建议归档前修）`design.md` D2.2/D2.3 描述的仍是旧形状，与实际实现契约不符；round 4 新增的「依赖行显式清理」在 spec 里没有条目

- 证据：
  - `design.md:63`（D2.3）：「单事务内 pluck 该分类下全部 feed 的文章 id → `PruneArticleRefs` → **删 category 行**」——代码实际是「prune → 删依赖行 → 删文章 → 删 feed → 删 category」（repository.go:388-403）。design 里的形状依赖 FK 级联，正是轮次 3 明确否定的做法。
  - `design.md:61`（D2.2）：顺序写作「pluck → `PruneArticleRefs` → 删 behaviors → 删 feed 行」，代码是「删 behaviors → prune → 删依赖行 → 删文章 → 删 feed」（repository.go:361-368）；且完全没有「依赖行显式清理」（轮次 4）这一节。
  - `specs/article-reference-integrity/spec.md` 只有四条 Requirement，没有覆盖「被删文章的 `article_topic_tags`/`tag_jobs`/`firecrawl_jobs` 行必须显式清理」这条新行为（spec 的 Purpose 只提「jsonb 引用维护」）。scenario「删除分类的两级级联同样维护引用」的措辞仍写「feed 与文章随之被**级联**删除」，与「显式删除、不依赖 FK」的实现口径不一致。
  - 偏差本身只记在 `apply-report.md` 的轮次 3/4 段落里（并自承「需主线程确认」）。
- 为什么重要：apply 全绿后归档，change 目录就是永久记录；下一个人读 `design.md` 会以为删分类靠级联、以为没有依赖行清理，等于把已经踩过的坑重新埋回去（`_conventions.md:78` 刚修正的就是这类「按文档信 FK」的误判）。
- 建议动作：归档前把 `design.md` D2.2/D2.3 改成实现契约（显式删行 + 依赖行清理 + `articleDeleteChunk`），或在 D2 末尾加一节「轮次 3/4 修订」；`spec.md` 补一条 Requirement/Scenario（「被删文章的从属行必须显式清理，环境无关」）并把 D2.3 scenario 的「级联」措辞改为「随之删除」。不建议为了对齐文档去改代码。

---

## Low

### L1（P2）`TestDailyReportJobSucceedsWhenDanglingRefProbeFails` 的「探针必失败」只是代理断言，注释与测试实际做的事不符
- 证据：job_daily_report_test.go:390-395 用 `require.False(t, db.Migrator().HasTable("daily_report_threads"))` 作为「探针会失败」的论据，测试注释（:368-373）却说「the failure is asserted by the warn log」——测试并没有捕获或断言日志，也没有断言 `CountDanglingArticleRefs` 返回 error。
- 为什么重要：这条用例的核心价值（「探针失败时 job 仍成功」）只有在探针**真的失败**时才成立；若将来有人把 `CountDanglingArticleRefs` 改成吞错返回 `(0, nil)`（read-only 探针很容易被这样"加固"），测试仍会通过，而它守的语义已经消失。现在 `repair.go:94-104` 确实会把 SQLite 的 `no such table` 上抛，所以**当前**用例是有效的。
- 建议动作：在 job 调用前加一行直接断言（`_, err := articlerefs.CountDanglingArticleRefs(db); require.Error(t, err, ...)`），把「探针必失败」从间接论据变成直接断言。不阻断归档。

### L2（P2）`TestDeleteFeedCascadeHandlesFeedWithoutArticles` 缺一条「feed 行确实被删」的断言，可空转通过
- 证据：article_refs_test.go:369-383 只断言 `DeleteFeedCascade(empty.ID)` 无错误 + 另一个 feed 的三张依赖行仍为 1：若该函数退化成 no-op，这些断言全部照旧通过（tasks 2.9 声称它覆盖「无文章的 feed 空分支不报错」，这一点是真的，但「删成功」未被证伪）。
- 建议动作：补一行 `require.Zero` 统计 `models.Feed{ID: empty.ID}` 的剩余行数。不阻断归档。

### L3（P2）`HasTable` 守卫放在分块循环内，且 GORM 的 `HasTable` 吞掉查询错误（fail-open）
- 证据：repository.go:293 每块都调 `deleteArticleDependents`，:330 每块对三张表各查一次 `information_schema`（n=100k 时约 300 次目录查询）；GORM 的 `Migrator().HasTable` 只返回 bool，元数据查询失败会被读成 false → 该块的依赖行**静默跳过清理**（随后文章行照删，孤儿留下）。
- 为什么重要：跳过语义对「窄部署缺表」是有意的，但在「表在、元数据查询失败」这种非预期路径上会静默产生孤儿行；另外它只守表存在、不守列存在（缺 `article_id` 列时 DELETE 报错 → 整事务回滚，属 fail-closed，可接受）。
- 建议动作：把「表存在性」判断提到循环外算一次（三张表的 bool 缓存），顺手把这个 fail-open 的边界写进注释。不阻断归档。

### L4（P2）「删除实现显式、不依赖 FK」的保证对**并发插入**的行不成立（残余窗口）
- 证据：`deleteArticlesOfFeedsWithRefs` 明确按 pluck 到的 id 删（repository.go:266-267 注释：「exactly the articles whose references were just pruned are the ones removed」）。若在 pluck 之后、删 feed 之前有刷新任务（feed_service 抓取）插入该 feed 的新文章：无 FK 库会**留下 feed_id 悬空的孤儿文章**；有遗留 FK 的真库会被 `fk_feeds_articles` 级联删掉——正是「未被剪引用就被删行」的原形态。
- 为什么重要：这不算实现缺陷（注释里的取舍是有意为之：宁可留孤儿也不剪活引用），但它是「环境无关性」这条卖点的最后一个例外，值得在 design/spec 的残余风险里点一句，避免以后有人以为已经完全与 FK 无关。
- 建议动作：仅记录（可在 design.md 残余风险表加一行）。不阻断归档。

---

### 我验证过的关键假设（含我无法复跑、只能推理的部分）

1. **测试库确实没有级联 FK**：`internal/platform/testutil/testutil.go:152-157` 走 `DisableForeignKeyConstraintWhenMigrating: true`，golden schema = AutoMigrate + 版本化迁移；`postgres_migrations.go` 全文件 grep `REFERENCES articles|REFERENCES feeds|REFERENCES categories` **0 命中**（只有其它表的 `ADD CONSTRAINT`），所以指向 articles/feeds/categories 的 FK 只可能是 AutoMigrate 时代遗留 → 只在真库存在。轮次 3/4 的前提成立。
2. **依赖表名单完整**：`internal/models` 内带 `ArticleID` 的表只有 `article_topic_tags`、`tag_jobs`、`firecrawl_jobs`、`reading_behaviors`（job_queue.go:16/37、reading_behavior.go:9、topic_graph.go:158）；`topic_analysis_cursors.last_article_id` 是推进型游标、不是引用（topic_tag_analysis.go:26）。3 张表覆盖正确，第 4 张（reading_behaviors）按设计不删且失败会整体回滚。
3. **`HasTable(string)` 语义成立**：GORM postgres migrator 用 `info_schema` + `CURRENT_SCHEMA()` 查表名，仓库测试与真库都在 `public`，两边一致；且测试里 seed 成功 → 表存在 → 断言不可能空转（若 `HasTable` 恒 false，删依赖行被跳过，`require.Zero` 会因 2/3 条孤儿而变红）。
4. **非空性论证成立**：测试库无 FK（第 1 条）+ 短路 `deleteArticleDependents` 后文章行仍能删掉（无 FK 拦截）→ 依赖行残留 2/3 条，与实现方记录的 `Should be zero, but was 2/3` 完全自洽；`grep TEMP-NONVACUITY` 无残留、无调试代码。
5. **注释里的两处事实性论据为真**：`max_articles=9999` 走无限量分支（feed_service.go:240、feed_service_test.go:311），所以分块是必要的；`aux_label_cleanup` job 与 `CleanupOrphanedTags` 确实存在（runtime.go:122-128、article_tagger.go:466），「孤儿 topic_tags 归维护任务」站得住。
6. **`reading_behaviors`/`user_preferences` 的 NO ACTION 不会留下部分删除**：单事务 + GORM 返回 error → `Transaction` 回滚，prune 一并撤销；且改造前后「有这些子行则删分类失败」等价（真库先前靠同两条约束拦截级联，现在拦显式删除）。这一点我认可 apply-report 的判断。
7. **前端删除入口唯一**：`删除分类` 只有 AppSidebarView.vue:232 → FeedLayoutShell.vue:420-421 这一条确认框（`grep 删除分类|deleteCategory *.vue` 全部命中已核对）。

### 建议主线程复跑（作为归档证据，我无法运行）
```
cd backend-go && go test -count=1 -run 'TestDeleteCategory|TestDeleteFeedCascade|TestDeleteArticlesByFeed' ./internal/reader/repository/ -v
cd backend-go && go test -count=1 -run TestDailyReportJobSucceedsWhenDanglingRefProbeFails ./internal/admin/scheduler/ -v
```

**合并/归档结论：OK with notes。** 轮次 3/4 的代码本身（事务、顺序、分块、依赖清理、空分支、无调试残留、tasks §2.5-2.10 勾选真实）我没有找到阻断性问题；H1 是**必须落定处置的对外一致性问题**（改文案 或 明写部署影响 + 独立 change），M1 建议归档前把 design.md/spec 与实现对齐，L1-L4 可留作后续。