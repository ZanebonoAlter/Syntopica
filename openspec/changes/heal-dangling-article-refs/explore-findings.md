
## 文章行删除路径清单（引用维护的接线点）

全库只有三处会删除 articles 行，引用维护只需覆盖它们：

1. `backend-go/internal/platform/database/postgres_migrations.go:2561` — `mergeDuplicateArticleGroup` 里 `db.Exec(\`DELETE FROM articles WHERE id = ?\`, r.ID)`（dedupe-rss-articles 迁移 20260917_0001 的 loser 删除）。该函数同事务内已重写 `article_topic_tags`（UPDATE 改指 keeper + 删除冲突行）、`reading_behaviors`（改指）、`tag_jobs`/`firecrawl_jobs`（completed 删、其余改指），**唯独没维护 `daily_report_threads.related_article_ids`**。接线位置：删除语句之前调用 RewireArticleRefs(r.ID → keeper)。
2. `backend-go/internal/reader/handler/feed_handler.go:377 DeleteFeed` — 内联 `DELETE FROM reading_behaviors WHERE feed_id` + `Delete(&feed)`；靠 `articles.feed_id → feeds.id ON DELETE CASCADE` 隐式删掉该 feed 的全部文章行（实测 FK 存在，ON DELETE CASCADE）。接线位置：改成仓库事务方法，删 feed 行前先 prune 引用。
3. `backend-go/internal/reader/repository/repository.go:247 DeleteArticlesByFeed`（及其包装 `DeleteCascadeByFeed:252`）——当前无调用方，但作为删行原语也应自带 prune。

`topic_analysis_cursors.last_article_id` 是高水位游标，不维护。

**引用**：backend-go/internal/platform/database/postgres_migrations.go:mergeDuplicateArticleGroup、backend-go/internal/reader/handler/feed_handler.go:DeleteFeed、backend-go/internal/reader/repository/repository.go:DeleteArticlesByFeed

<!-- pinned 2026-09-16T16:11:32Z -->

## jsonb 引用面盘点与实测 SQL 形态（含踩坑）

全库 jsonb 列逐列抽样后，**唯一持本地文章 ID 的列是 `daily_report_threads.related_article_ids`**（7796 行：7793 array + 3 个 JSON `null` 标量）。`board_daily_reports.raw_clusters` 只存 `{lane, tag_ids, group_name}`；`cross_board_relations.evidence` / `board_upgrade_suggestions.evidence` 是外部 URL/摘要；`topic_enrichment_result.input_snapshot` 是文本 digest。

实测踩坑（写查询/迁移时必须避开）：
- `jsonb_array_elements_text(related_article_ids)` 对 JSON `null` 行直接抛 `ERROR: cannot extract elements from a scalar`（SQLSTATE 22023），**会让整条查询静默失败**（psql stderr 不显示时看起来像"零命中"）。所有相关查询一律加 `CASE WHEN jsonb_typeof(col)='array' THEN col ELSE '[]'::jsonb END` 守卫，或先跑 NormalizeThreadRefs。
- 判断数组含某 id 用 `col ? '129447'`（元素文本匹配），不必强转；反过来做存在性比对时用 `a.id::text = elem`，避免脏元素 `::bigint` 抛错。
- 存量规模（2026-09-17 实测）：4774 条 thread / 5595 个引用悬空；今日（period_date 2026-09-16）9 条：report 807(thread 10173→128606, 10181→129500)、808(10186→129554, 10188→129425, 10190→129461, 10212→129447)、811(10227→128606)、814(10264→129447, 10274→129403)。
- 悬空 id 邻域是"偶数在、奇数没"的重复对空洞；thread 10264 的 refs = `[129446, 129447]` 是 keeper/loser 对（129446 存、129447 删）。

另注：`marshalJSONArray`（`topicgraph/service/marshal_array.go`）已把 nil 规范化为 `[]`，但 thread 写路径仍在用 `json.Marshal`（`daily_report_orchestrator.go:281-282` 的 `th.TagIDs`/`th.RelatedArticleIDs`、`daily_report_merge.go:292` 的 `mergedTagIDs`），这就是 JSON null 的来源。

**引用**：backend-go/internal/topicgraph/service/marshal_array.go:marshalJSONArray、backend-go/internal/topicgraph/service/daily_report_orchestrator.go、backend-go/internal/topicgraph/service/daily_report_merge.go:populateThreadArticles

<!-- pinned 2026-09-16T16:11:32Z -->
