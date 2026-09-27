<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report, reading -->

## Why

2026-09-16 晚的日报里，9 条线索（thread）的 `related_article_ids` 指向已经不存在的文章行，前端只能降级显示「文章 #129447」——线索追溯不到来源文章。

根因不是「延时分析归档」（边时间窗 GC 与归档留行都不是删除者，今日日报 tag 引用零悬空、edge GC 今日累计只回收 86 条边），而是**文章行被物理删除的路径没有同步维护「按 ID 引用文章」的 jsonb 数组**：

1. `dedupe-rss-articles` 的存量归并迁移（`20260917_0001`，2026-09-16 23:28:59 应用）里 `mergeDuplicateArticleGroup` 重写了 `article_topic_tags` / `reading_behaviors` / `tag_jobs` / `firecrawl_jobs`，然后 `DELETE FROM articles WHERE id = ?` —— **没碰 `daily_report_threads.related_article_ids`**。
2. 21:23 的连续两次刷新在唯一索引建立前成对插入重复行（本次留下 409 组），重复的两份各自打过标签、各自有边；23:23-23:29 重建今日日报时两份都进了线索引用（`collectBoardTags` 走 `article_topic_tags JOIN articles`，无 archived 过滤，符合既有约束）。
3. 归并删掉其中一份（loser）→ 已写入的引用成为死链。证据：线索 10264 的 refs = `[129446, 129447]`，129446 尚存（v2ex t/1242360）、129447 已删；悬空 id 的邻域全是「偶数存在、奇数消失」的重复对空洞（129424/129426 在、129425 没；129402/129404 在、129403 没）。
4. 删行路径的全量清单（`grep` + 真库 `pg_constraint` 实测，2026-09-17）：① 去重归并迁移；② 删订阅源（存量 FK `fk_feeds_articles` ON DELETE CASCADE）；③ 删分类（存量 FK `fk_categories_feeds` ON DELETE CASCADE，两级级联到文章）；④ reader 仓库删行原语。其中 ②③ 靠**存量 FK 静默级联**删行（仓库文档「全库只有 6 条 FK」是过时错误说法，实测 25 条）。相关 feed 健在、归档路径自 2026-08-19 起只置 `archived=true` 不删行 → 排除其它可能。

存量为 4774 条线索 / 5595 个引用悬空（主要来自 2026-08-19 前的物理删除旧账，与本次同形），另有 3 条历史 thread 把 `related_article_ids` / `tag_ids` 写成了 JSON `null` 标量（`jsonb_array_elements_text` 会直接抛 SQLSTATE 22023，生产 SQL 全靠 `jsonb_typeof` 守卫才没炸）。

## What Changes

- **新增共享引用维护器**：`internal/platform/articlerefs`——`RewireArticleRefs(老id→保留id)`、`PruneArticleRefs(ids...)`、`PruneDanglingRefs(分批)`、`CountDanglingArticleRefs()`；只操作 `daily_report_threads.related_article_ids`（实测全库唯一持本地文章 ID 的 jsonb 列），重写保序、去重、空数组写 `[]` 不写 `null`。
- **把四条删除路径全接进维护器**：`mergeDuplicateArticleGroup` 在 `DELETE FROM articles` **之前**把 loser 的引用改指 keeper（同事务）；删订阅源（`DeleteFeedCascade`）与删分类（`DeleteCategoryCascade`，两级级联）在删行前剔除这些文章的引用；仓库删行原语（`DeleteArticlesByFeed`/`DeleteCascadeByFeed`/`DeleteFeed`）自身安全化。
- **日报写路径加存在性校验**：orchestrator 在最终 thread 批次（含 Step7.5 watch 物化轨）上一次过 `articles` 存在性查询，只把仍存在的 ID 写进 `related_article_ids`——把「生成读到 id、写库前该行被删」的 TOCTOU 窗口从数十秒压缩到毫秒级（今天的 814 号日报正是这个形态），剩余窗口由巡检堆底。
- **写路径卫生**：thread 的 `tag_ids` / `related_article_ids` 与合并路径的 `mergedTagIDs` 改走既有 `marshalJSONArray`（已把 nil 规范化为 `[]`），消灭 JSON `null` 来源。
- **存量修复迁移**：新增 `20260917_0002`——先规范化 `related_article_ids` 的 JSON `null` / SQL NULL 为 `[]`，再分批剔除悬空 ID（幂等、可重跑、日志计数）。
- **悬空引用巡检（只读告警）**：日报 job 收尾统计悬空引用数并写日志，作为「有未知删除者」的兜底观测面；不自动删数据。
- **删分类确认文案修正（ui-impact: minor，前端唯一改动）**：`FeedLayoutShell.vue` 的删分类确认框原写「这个操作不会删除分类下的订阅源」，与实现相反（真库存量 FK 级联 + 本 change 的显式删除都会连带删订阅源与文章）——改为「该分类下的订阅源及其文章也会一并删除，且不可撤销」。用户 2026-09-17 明确选择「只改文案、保持删除语义」。
- 明确不改动：归档语义（`archived=true` 保留行）、边时间窗 GC、`daily_report_sections.article_count` / `board_daily_reports.article_count` 等生成时快照计数（历史不重算）、`topic_analysis_cursors.last_article_id`（高水位游标，指向行被删不影响推进语义）。

## Capabilities

### New Capabilities

- `article-reference-integrity`: 文章行被删除时按 ID 引用它的 jsonb 数组的同步维护（重写/剔除）、写路径的存在性校验、存量悬空引用的修复迁移与巡检观测。

### Modified Capabilities

（无——`daily-report-system` 的生成/归属/读路径行为不变，本 change 只在写库前做引用存在性过滤并修复存量数据；`rss-article-dedup` 的判重与归并策略不变，只是归并多维护一处引用。）

> **顺带修正的文档债**：`docs/reference/database/tables/_conventions.md` 的「DB 级外键全库共 6 条」清单过时（实测 25 条，含两条会级联删文章的存量 FK），本 change 一并修正——它是分析删行路径的关键事实源，错着会直接误导后续排查。
