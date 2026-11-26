<!-- complexity: moderate -->
<!-- ui-impact: none -->
<!-- constraint-domains: content-enrichment -->

## Why

`7f4cc2ff`（摘要入库纯文本化）把刷新判重的 description 比较键换成了 `PlainSummary` 派生投影，但存量 62,332 行仍是旧口径 raw HTML，导致「内容未变」的判定永久失真：feed 31（智源社区，单文档吐 7 个月全量历史）在 16:56-17:56 被整份重写 731 条、标签全清、重入队 731 个打标作业；feed 24/22/33/41 等 19 个源共 62k 行同病，只要它们的 RSS 窗口覆盖到旧口径行，每小时都在重复「误判变化 → 清标签 → 重入队」。

诊断（2026-11-20，只读核验）另发现两个独立的重复源：

1. **同文档重复 link**：feed 31 的 RSS 文档把同一 link 以两套标题吐两次（index 30/38），DB 按 link 只有一行，于是该行（id 167936）每小时被改写+清标签两次，永久循环。
2. **入队竞态**：tag_jobs 的 Enqueue 是事务内 check-then-insert（默认 read committed），两条并发 firecrawl 回调在同一秒各插入一条（实证：文章 178599 于 18:59:31 产生 2 条 pending）。

存量数据面已先行处置（属已归档 change `fix-reader-lede-blank-space` 交付物的既定用法 + 队列表运维清理，非本 change 任务）：回填 CLI `--apply` 清洗 62,332 行旧口径摘要；清除 1,150 条挂已归档文章的无意义 pending 作业与 1 条竞态重复。本 change 负责**代码根因**，让这类重复即使再遇到口径漂移/脏文档/并发也不复发。

## What Changes

- `RefreshFeed` 循环内对文档内重复 link 去重，每个 link 只处理 `pubDate` 最新的变体（`pubDate` 缺失/相等回退首现；后续出现不再二次改写/清标签/入队）。
- `refreshExistingArticle` 的 description 比较**两侧同投影**：存量侧含标记时先经 `htmltext.PlainSummary(raw, 300)` 归一再比，比较键对存量口径漂移免疫（红线：内容未变时不得触发任何处理链）。
- `refreshExistingArticle` 开头对 `archived` 行短路返回：归档行不再被刷新改写/清标签/重入队（归档=冻结快照；全量历史回放不再触碰冻结行）。
- `tag_jobs` 加 `(article_id) WHERE status IN ('pending','leased')` 部分唯一索引（版本化迁移，建索引前先清存量重复行），`Enqueue` 插入路径改 `ON CONFLICT DO NOTHING` 后补更新，数据库级兜底并发竞态。
- `detection_jobs`（add-ai-generated-article-detection 活跃 change 的同型实现）同步加部分唯一索引加固——该部分在该 change 内补任务执行。

## Impact

- **受影响代码**：`backend-go/internal/reader/service/feed_service.go`（RefreshFeed / refreshExistingArticle）、`backend-go/internal/tagmanagement/repository/tag_job_queue.go`、新迁移文件（tag_jobs 唯一索引）。
- **受影响 spec**：`rss-article-dedup`（更新语义 requirement 修改）、`tag-queue-scheduling`（新增队列幂等 requirement）。
- **风险**：归档行不再跟进上游修订（有意为之，冻结语义），且为**永久冻结**——当前无解档入口（用户解档/恢复跟进须另开 change）；行为变化已在 proposal 声明，部署后无需用户手动操作；旧数据已由回填处置，无降级路径。
- **不改动**：入库写入口径（`buildArticleFromEntry` 已是新口径）、content 字段、firecrawl 链、打标执行语义。
