-- dedupe-rss-articles 部署后核对（tasks 3.3）
-- 迁移 20260917_0001 随后端启动自动执行；部署后按下述查询逐条核对。
-- 用法：docker exec syntopica-postgres psql -U postgres -d syntopica -c "<query>"

-- ① 同 feed 重复组必须为 0（迁移后唯一索引兜底，任何新增都会被拒绝）
SELECT feed_id, link, COUNT(*) AS copies
FROM articles
WHERE link <> ''
GROUP BY feed_id, link
HAVING COUNT(*) > 1;
-- 期望：0 行

-- ② 跨 feed 重复组保留（预期行为，非问题：各 feed 视图里各显示一次）
SELECT COUNT(*) AS cross_feed_groups FROM (
  SELECT link FROM articles WHERE link <> ''
  GROUP BY link HAVING COUNT(DISTINCT feed_id) > 1
) x;

-- ③ 保留条 tag_count 与实际标签数一致（迁移已重算，此处核对）
SELECT COUNT(*) AS mismatched FROM articles a
LEFT JOIN LATERAL (
  SELECT COUNT(*) AS n FROM article_topic_tags t WHERE t.article_id = a.id
) c ON true
WHERE a.tag_count <> c.n;
-- 期望：0 行

-- ④ 两个索引都在（普通 link 索引 + 唯一部分索引）
SELECT indexname, indexdef FROM pg_indexes
WHERE tablename = 'articles' AND indexname IN ('idx_articles_link', 'uq_articles_feed_link');
-- 期望：2 行；uq_articles_feed_link 带 WHERE (link <> '')

-- ⑤ 无悬挂 job（迁移把 pending 改指向、completed 删除）
SELECT
  (SELECT COUNT(*) FROM tag_jobs j LEFT JOIN articles a ON a.id = j.article_id WHERE a.id IS NULL) AS dangling_tag_jobs,
  (SELECT COUNT(*) FROM firecrawl_jobs j LEFT JOIN articles a ON a.id = j.article_id WHERE a.id IS NULL) AS dangling_firecrawl_jobs;
-- 期望：0, 0

-- ⑥ 重复打标 AI 调用不再增长（迁移后观察一个刷新周期）
-- 同一篇文章跨 feed 副本的 extractor 调用应降为 0（走 source='reuse' 标签复用）
SELECT COUNT(*) AS reuse_rows, COUNT(DISTINCT article_id) AS articles_reused
FROM article_topic_tags WHERE source = 'reuse';
