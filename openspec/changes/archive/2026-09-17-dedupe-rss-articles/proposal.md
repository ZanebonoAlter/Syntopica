<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: content-enrichment, topic-graph -->

## Why

RSS 入库判重只按 `(feed_id, title)`（`feed_service.go` RefreshFeed），三类场景全部穿透：① 快讯源滚动更新同一 URL 的标题（华尔街见闻 livenews/凤凰资讯，主因，占 59%）；② 同一篇文章被同站点多个榜单 feed 收录（博客园/掘金多榜单，占 30%）；③ 同 feed 并发刷新竞态（10%）。`articles` 表已积累 409 个重复链接组且持续新增（单日峰值 56 组），每份副本独立进入打标队列、各自调用 AI——打标记录与 AI 调用记录里同一篇文章出现多次，AI 调用被重复浪费。

## What Changes

- **入库判重键改为 `(feed_id, link)`**：`RefreshFeed` 的 titleSet 快照改为本 feed 的 link 集；同 feed 同 link 不再插新条目。
- **快讯更新语义（upsert）**：link 命中已有条目时，比对 `title+description` 内容 hash——未变仅跳过；变了则 UPDATE 原条目内容并重置处理链（firecrawl/摘要按 feed 开关重走，retag 由处理链完成事件触发，复用现有 tag_jobs 事件路径）。
- **跨 feed 打标复用**：`tagArticle` 打标前查同 link 的其它副本，已有完整打标则复制 `article_topic_tags` 链接（score 沿用、source 标记复用），不调 AI；`RetagArticle`（Force）不适用复用，仍走 AI 重打。
- **存量清理迁移（Go 迁移，启动自动执行）**：409 组重复按"处理链完整度打分"选保留条（有标签 > 有正文/摘要 > 有阅读行为 > 最早创建），`article_topic_tags`/`reading_behaviors` 合并到保留条，悬挂的 `tag_jobs`/`firecrawl_jobs` 清理或改指向，删除多余副本后**同事务**建立 `(feed_id, link)` 唯一部分索引兜底并发。
- **支撑索引**：`articles.link` 普通索引（跨 feed 复用查询与迁移归并都需要，当前无 link 索引）。

## Capabilities

### New Capabilities

- `rss-article-dedup`: RSS 文章入库去重与更新语义——(feed_id, link) 判重、快讯内容 hash 差异检测与处理链重置、跨 feed 打标复用、存量重复归并迁移与唯一索引。

### Modified Capabilities

（无——现有 `tagging-domain`/`refresh-parallelization`/`article-retention` 的 requirement 均不涉及入库判重行为，本 change 全部收敛到新 capability。）
