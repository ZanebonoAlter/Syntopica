
## RSS 打标重复根因与关键代码事实（探索阶段调查）

**现象**：打标记录/AI 调用记录（ai_call_logs，AICallLog 模型，extractor 操作 tagmanagement.extractor_enhanced）里同一篇文章出现多次，重复源头是 articles 表本身有重复行。

**数据证据**（2026-09-17 查询）：409 个重复 link 组 = 同 feed 标题变化 243（wallstreetcn livenews feed 24 / ifeng feed 22 / wallstreetcn hot feed 17 快讯滚动更新标题）+ 跨 feed 123（博客园 feed 3/6/11、掘金 feed 25/26 同站多榜单，插入时间差可至 7 秒）+ 同 feed 同标题竞态 41 + 混合 3；89 组内 active 与 archived 混存。110 个重复副本已各自打标。article_topic_tags 无 (article_id, topic_tag_id) 精确重复（单篇打标内部幂等是好的）；articles 表无 link 唯一约束（仅主键）。

**代码根因**：backend-go/internal/reader/service/feed_service.go RefreshFeed L67-107：titleSet 仅 Pluck 本 feed 的 title（WHERE feed_id），只比对标题不看 link；db.Create 无唯一索引兜底。次要：tag_jobs 333 篇多次入队（firecrawl_completed+summary_completed 组合），但 tagArticle existingCount skip 保护挡住，非主因。

**关键代码事实（实现要用）**：
- enqueueArticleProcessing（feed_service.go L118）：FirecrawlEnabled → 入 firecrawl 队列；否则 TaggingEnabled → 入 tag 队列 reason=article_created
- buildArticleFromEntry（L218）：状态初始化矩阵——firecrawl 开 → firecrawl_status=pending，summary 开 → summary_status=pending/incomplete，都关 → complete/completed（更新语义的状态重置复用此规则）
- CleanupOldArticles（L143）：归档不删除，archived 行仍在库 → 迁移保留条打分必须 active 优先（!archived×1000）
- tagArticle（tagmanagement/service/core/article_tagger.go）：existingCount>0 skip → 之后是复用分支插入点；RetagArticle Force 模式删旧标签+CleanupOrphanedTags 可复用作更新路径的旧标签清理
- 迁移模式：platform/database/postgres_migrations.go Version: "YYYYMMDD_NNNN" 注册、默认事务内、migrator 按 appliedVersions 跳过（幂等）；tableExists/columnIsNullable helper 可用
- 测试环境 sqlite（AutoMigrate），PG-only 的索引/迁移需在迁移测试里单独建
- articles 无 updated_at 列；有 tag_count 冗余计数列需迁移后重算
- articles.link 现无任何索引，跨 feed 复用查询与迁移归并都需 idx_articles_link

<!-- pinned 2026-09-16T14:59:26Z -->
