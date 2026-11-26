-- cleanup-heuristic-tags-20261112.sql
--
-- retire-heuristic-tag-fallback 存量清理（2026-11-12）
--
-- 背景：heuristic 规则词兜底打标在 LLM 抽取失败时产出 Coding / AI Agent / Infra 等
--       泛词标签，且吞掉错误使打标任务被误标 completed、文章永不重打。
--       存量污染：article_topic_tags 1724 行（source='heuristic'）、topic_tags 28 个。
--
-- 处置口径（由真库实测数据定死，见 change 的 design D3）：
--   1) 删全部 heuristic 来源的**文章标签关联**（1724 行）——污染源头；
--   2) 删「无任何非 heuristic 引用」的 heuristic 标签（14 个纯规则凭空造的标签：
--      ai新闻 / 游戏相关 / 技术 / Infra / Multimodal / HackerNews / Recent /
--      GPT-4 / GPT-6.1 / Claude 1 / Claude 2026 / Claude 5.2 / Gemini 3 / Gemini CLI）——
--      FK 全为 ON DELETE CASCADE，连带清 topic_tag_board_labels 挂载行、
--      topic_tag_embeddings 向量行、topic_tag_semantic_labels 关联行；
--   3) 保留「已被 LLM 认领」的 14 个标签（Steam / Anthropic / OpenAI / AI Agent /
--      CnGal / V2EX / NVIDIA / GPT-6 / Gemini 4 / Gemini API / Claude 3.5 /
--      Gemini 3.8 / GPT-6.2 / Coding），source 改为 llm（source 字段反映最新来源）；
--   4) 清理无 FK 的悬挂引用（auxiliary_description_repair_items / embedding_queues /
--      tag_merge_suggestions）；
--   5) 只重算**受影响文章**的 articles.tag_count（存量另有 1157 行历史不一致，
--      不属本次范围，不动）。
--
-- 幂等：重复执行影响 0 行。
-- 红线（standard/backend/testing.md §🛑）：必须在事务内执行；执行前已 count(*) 留底
--       并确认 backups/pg-key-*.dump 备份可用。
-- 留底（2026-11-12 实测）：article_topic_tags 总 18530 / heuristic 1724；
--       topic_tags 总 13888 / heuristic 28；受影响文章 1141 篇；
--       board 1980 挂载行 944。

BEGIN;

-- 0) 先抓住受影响集合（删除后已无从查起）
CREATE TEMP TABLE _affected_articles ON COMMIT DROP AS
  SELECT DISTINCT article_id FROM article_topic_tags WHERE source = 'heuristic';

--    待删标签 id 集合：仅「零非-heuristic 引用」的 heuristic 标签。
--    注意：辅助表（repair_items / embedding_queues / merge_suggestions）对
--    topic_tags **无外键**，删标签后需显式清本次造成的悬挂引用；
--    但这些表**本就存在大量历史孤儿**（repair_items 53488 行、merge_suggestions
--    10802 行，属存量欠账），故只清「指向本次待删标签」的行，不越界清理历史孤儿。
CREATE TEMP TABLE _deleted_heur_tags ON COMMIT DROP AS
  SELECT t.id FROM topic_tags t
   WHERE t.source = 'heuristic'
     AND NOT EXISTS (SELECT 1 FROM article_topic_tags a WHERE a.topic_tag_id = t.id);

-- 1) 删 heuristic 来源的文章标签关联（污染源头）
DELETE FROM article_topic_tags WHERE source = 'heuristic';

-- 2) 删零引用的 heuristic 标签（FK CASCADE 连带清挂载行/向量行/语义标签行）
DELETE FROM topic_tags t
 WHERE t.source = 'heuristic'
   AND NOT EXISTS (SELECT 1 FROM article_topic_tags a WHERE a.topic_tag_id = t.id);

-- 3) 已被 LLM 认领的标签：source 改 llm
UPDATE topic_tags SET source = 'llm' WHERE source = 'heuristic';

-- 4) 悬挂引用清理：仅清「指向本次待删标签」的行（不越界清理存量孤儿）
DELETE FROM auxiliary_description_repair_items
 WHERE topic_tag_id IN (SELECT id FROM _deleted_heur_tags);

DELETE FROM embedding_queues
 WHERE tag_id IN (SELECT id FROM _deleted_heur_tags);

DELETE FROM tag_merge_suggestions
 WHERE existing_tag_id IN (SELECT id FROM _deleted_heur_tags)
    OR new_tag_id IN (SELECT id FROM _deleted_heur_tags);

-- 5) 只重算受影响文章的 tag_count（两段式：先置 0，再按剩余关联回填）。
--    注意：articles 行含 content/ai_content_summary 等宽列，UPDATE 会整行重写，
--    1141 行在本机实测约需数秒 + WAL 刷盘——**清理期间应停后端**（避免与后台
--    quality_score 作业抢 WAL/IO 造成长等待）。
UPDATE articles a
   SET tag_count = 0
 WHERE a.id IN (SELECT article_id FROM _affected_articles);

UPDATE articles a
   SET tag_count = sub.n
  FROM (SELECT x.article_id, count(*) AS n
          FROM article_topic_tags x
         WHERE x.article_id IN (SELECT article_id FROM _affected_articles)
         GROUP BY x.article_id) sub
 WHERE a.id = sub.article_id;

-- 6) 就地核对（ROLLBACK 时同样执行，便于演练核对影响面）
DO $$
DECLARE
  v_heur_rows  BIGINT;
  v_heur_tags  BIGINT;
  v_orphan_1   BIGINT;
  v_orphan_2   BIGINT;
  v_orphan_3   BIGINT;
BEGIN
  SELECT count(*) INTO v_heur_rows FROM article_topic_tags WHERE source = 'heuristic';
  SELECT count(*) INTO v_heur_tags FROM topic_tags WHERE source = 'heuristic';
  SELECT count(*) INTO v_orphan_1 FROM auxiliary_description_repair_items x
   WHERE x.topic_tag_id IN (SELECT id FROM _deleted_heur_tags);
  SELECT count(*) INTO v_orphan_2 FROM embedding_queues x
   WHERE x.tag_id IN (SELECT id FROM _deleted_heur_tags);
  SELECT count(*) INTO v_orphan_3 FROM tag_merge_suggestions x
   WHERE x.existing_tag_id IN (SELECT id FROM _deleted_heur_tags)
      OR x.new_tag_id IN (SELECT id FROM _deleted_heur_tags);
  RAISE NOTICE '清理后：heuristic 文章标签行=%  heuristic 标签=%  孤儿(repair_items)=% 孤儿(embedding_queues)=% 孤儿(merge_suggestions)=%',
    v_heur_rows, v_heur_tags, v_orphan_1, v_orphan_2, v_orphan_3;
END $$;

COMMIT;
