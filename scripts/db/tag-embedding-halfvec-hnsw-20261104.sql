-- speed-up-tag-embedding 任务 1.1：topic_tag_embeddings 三类 partial halfvec HNSW 索引
-- （semantic / identity / event_keyword）+ 清理坏索引尸体 tmp_hnsw_test。
--
-- 🛑 手动执行，不进自动迁移链：生产库曾在高负载叠加建索引时触发后端崩溃 ×3 +
-- 整机恢复 ×1（详见 docs/research/tag-embedding-slow/explore-findings.md）。
-- 必须在低负载窗口执行（建议先 analysispause 或停后端/暂停抓取），每索引约 1 分钟。
--
-- 前置：
--   1. docker-compose.pg.yml 已加 shm_size: 1g（已入库，需重建容器生效）
--   2. 确认维度：SELECT format_type(a.atttypid, a.atttypmod) FROM pg_attribute a
--      JOIN pg_class c ON c.oid=a.attrelid WHERE c.relname='topic_tag_embeddings'
--      AND a.attname='embedding';   -- 期望 vector(2560)；若维度不同，替换下方 (2560)
--
-- 用法（低负载窗口，逐条执行可观察进度）：
--   docker exec -i syntopica-postgres psql -U postgres -d syntopica -v ON_ERROR_STOP=1 \
--     < scripts/db/tag-embedding-halfvec-hnsw-20261104.sql
--
-- typmod 红线：索引表达式与查询 cast 必须写 ::halfvec(2560)（带维度），
-- 不带 (2560) 的 ::halfvec 不会走索引（生产实测 EXPLAIN 确认）。
-- CONCURRENTLY 不能在事务里跑；本文件不包 BEGIN/COMMIT，psql 逐条自动提交。

-- 构建内存：shm_size 1g 已生效时 512MB 并行构建约 54s/索引（实测）。
SET maintenance_work_mem = '512MB';

-- semantic：FindSimilarTags 主路径（TagMatch L2 相似召回）
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_topic_tag_embeddings_semantic_halfvec_hnsw
  ON topic_tag_embeddings USING hnsw ((embedding::halfvec(2560)) halfvec_cosine_ops)
  WHERE embedding_type = 'semantic' AND embedding IS NOT NULL;

-- identity：已有同名 partial 索引（迁移 20261023_0001 建成）则跳过
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_topic_tag_embeddings_identity_halfvec_hnsw
  ON topic_tag_embeddings USING hnsw ((embedding::halfvec(2560)) halfvec_cosine_ops)
  WHERE embedding_type = 'identity' AND embedding IS NOT NULL;

-- event_keyword：事件标签关键词匹配路径
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_topic_tag_embeddings_event_keyword_halfvec_hnsw
  ON topic_tag_embeddings USING hnsw ((embedding::halfvec(2560)) halfvec_cosine_ops)
  WHERE embedding_type = 'event_keyword' AND embedding IS NOT NULL;

-- 清理 float32 HNSW 建失败的坏索引尸体（indisvalid=false）
DROP INDEX CONCURRENTLY IF EXISTS tmp_hnsw_test;

RESET maintenance_work_mem;

-- 验证（无副作用，可单独跑）：
--   SELECT c.relname, i.indisvalid
--   FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
--   WHERE c.relname LIKE '%halfvec_hnsw' OR c.relname = 'tmp_hnsw_test';
--   期望：3 个 indisvalid=true，无 tmp_hnsw_test 行。
