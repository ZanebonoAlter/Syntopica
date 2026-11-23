# Design — fix-feed-refresh-churn

## Context

诊断结论（2026-11-20，主会话 + 只读子线程双重复核）：

- 比较键失真的机制：`refreshExistingArticle` 用 `buildArticleFromEntry`（新口径 `PlainSummary(desc, 300)`）的投影值直接比存量 raw 值，投影口径一变（7f4cc2ff）存量即全体失配；feed 31 因「单文档 735 条全量历史 + 100% 含 HTML description」最显眼（16:56 波 483 条 + 17:56 波 246 条 = 同一次被打断的顺序扫描，两段索引零重叠）。
- 同文档重复 link：同一 link 在文档内出现两次且内容不同，每轮刷新触发两次「变化→改写→清标签→入队」。
- 入队竞态：check-then-insert 无数据库级约束，并发回调双插（178599 实证）。

数据面已先行处置（不属本 change 任务）：回填 `--apply`（62,332 行，幂等可断点重跑）；清除 pending 中挂归档文章的 1,150 行 + 同文章双行 1 行（2048 → 897）。

## D1 同文档重复 link：pubDate 最新变体优先

`RefreshFeed` 先扫一遍 entries，按 link 分组选代表（`newestEntryByLink`）：同 link 多次出现时取 `pubDate` 最新的变体；`pubDate` 缺失或相等时回退首现。随后仍按文档顺序遍历，在首现位置处理选中的变体，`seenInDoc` 兜底挡掉同 link 的其余副本：

- 命中 `seenInDoc[entry.Link]` → `continue`（第二次及以后出现一律跳过，无论新旧路径）；
- 首现时把待处理条目替换为该 link 的代表变体（新建或刷新），并置 `seenInDoc[entry.Link] = true`。

为什么不是「首现优先」（审查 R-2 升级）：首现优先依赖「RSS 文档 newest-first」这条无法校验的惯例，oldest-first 源会把库内行永久钉在最旧变体上（稳态错误，非一次性抖动）。改成按 pubDate 取最新后，newest-first 假设不再必要；pubDate 全缺时行为与旧版一致（回退首现）。

已知一次性副作用：167936 当前存的是 index-38 变体，修复后下一轮会按 pubDate 翻到最新变体一次，之后稳定。

不改 `linkSet`（DB 存在性集合）语义，两者职责分开：`linkSet` 管「库里有没有」，`seenInDoc` 管「本文档内处理过没有」。

## D2 比较投影归一：两侧同投影

`refreshExistingArticle` 的 description 比较：

```go
storedDesc := existing.Description
if htmltext.HasMarkup(storedDesc) {
    storedDesc = htmltext.PlainSummary(storedDesc, descriptionSummaryMaxRunes)
}
unchanged := existing.Title == fresh.Title && storedDesc == fresh.Description && existing.Content == fresh.Content
```

- 存量干净（回填后常态）：`HasMarkup` 短路为 false，纯字符串比较，零 HTML 解析开销（仅一次实体解码 + 标记正则）。
- 存量含标记（未回填/漏网/未来再遇口径漂移）：归一后比较，内容真没变就不触发处理链。
- 语义不变量：比较与写入共用同一投影实现（`htmltext.PlainSummary`，唯一定义），不复制口径。
- 不比 `content` 的投影（content 两侧都是源站原文，直比已稳定）；title 同。

## D3 归档行刷新冻结

`refreshExistingArticle` 取到 `existing` 后、比较之前：

```go
if existing.Archived {
    return nil
}
```

- 归档 = 冻结快照（article-archive-instead-of-delete 语义的自然延伸）：不再跟进上游标题/正文修订，不清标签，不重入队。
- feed 元数据（title/last_refresh_at 等）照常更新，不受影响。
- 全量历史型源再回放时，637/737 这类归档占比不再产生写放大与无效作业。
- 归档行永久冻结（当前无解档入口；若产品需要解档另开 change）——全仓唯一写 `archived=true` 的位置是 `CleanupOldArticles`，没有任何置回 false 的路径，所以这是单向不可逆语义，不是「解档后恢复跟进」（审查 R-4 修正）。

## D4 tag_jobs 数据库级入队幂等

- 迁移（版本化，沿用 `postgresMigrations()` 登记模式）：先清存量重复（同 article_id 保留最小 id 的 pending/leased 行），再建 `CREATE UNIQUE INDEX IF NOT EXISTS uq_tag_jobs_article_active ON tag_jobs (article_id) WHERE status IN ('pending','leased')`。表 2 千行级，普通建索引即可，不需要 CONCURRENTLY（版本迁移本就在事务内跑，CONCURRENTLY 反而不可用）。
- `Enqueue` 插入分支改 `Clauses(clause.OnConflict{DoNothing: true})`：撞索引时插入零行，随后补一次与「已存在」分支相同语义的原地更新（reason/快照/force_retag 刷新），保持「复用而非报错」的既有契约。
- `detection_jobs` 同型加固（check-then-insert 同竞态）在活跃 change `add-ai-generated-article-detection` 内补任务：部分唯一索引 + ON CONFLICT，spec 场景「重复触发判定不产生重复作业」由数据库级兜底。

## 已否决的替代方案

- **恢复 04:00 备份**：用户否决——会丢当天正常数据；且回填是幂等前向修复，无需回滚。
- **比较键去掉 description 只比 title+content**：正文超 300 字截断外的摘要级修订会漏检，弱化 rss-article-dedup 既有更新语义。
- **加 `desc_clean_version` 版本列**：回填 CLI 已把存量洗平，版本列只防「未来再变口径」——D2 的双侧同投影已用更少状态覆盖同一目标。

## 部署后影响与操作

- 行为变化：归档文章永久冻结、不再跟进源站修订（当前无解档入口，需要解档另开 change）；同文档重复 link 按 `pubDate` 最新变体处理；未回填的旧口径行不再触发假变化。
- 用户操作：无（迁移随启动自动执行；数据面已处置完毕）。
- 旧数据：无降级路径需要处理（回填已完成或由 D2 兼容）。
