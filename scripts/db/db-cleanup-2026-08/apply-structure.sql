-- analysis-remediation W1 (task 2.1/2.3/2.4/2.5)：结构修复，幂等，与迁移 20260820_0001/0002 内容一致。
-- 用法：docker exec -i syntopica-postgres psql -U postgres -d syntopica -v ON_ERROR_STOP=1 < scripts/db-cleanup-2026-08/apply-structure.sql
-- 前置：clean-orphan-embeddings.sh 已跑且孤儿计数 = 0（FK 校验现有行，脏数据会失败）。

-- 1) FK：topic_tag_embeddings.topic_tag_id → topic_tags.id ON DELETE CASCADE（幂等）
DO $$ BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.table_constraints
    WHERE constraint_name = 'fk_topic_tag_embeddings_tag'
      AND table_name = 'topic_tag_embeddings'
  ) THEN
    ALTER TABLE topic_tag_embeddings
      ADD CONSTRAINT fk_topic_tag_embeddings_tag
      FOREIGN KEY (topic_tag_id) REFERENCES topic_tags(id)
      ON DELETE CASCADE;
  END IF;
END $$;

-- 2) articles 全文搜索：删零使用 GIN 索引 + 禁用维护 trigger（tsvector 列保留，回滚可重建）
DROP INDEX IF EXISTS idx_articles_search_vector;
DROP TRIGGER IF EXISTS articles_search_vector_trigger ON articles;

-- 3) otel_spans 零使用索引 ×4（保留 pkey + start_time；按 trace_id 排障退化为顺序扫，当前 idx_scan=0）
DROP INDEX IF EXISTS idx_otel_spans_trace_id;
DROP INDEX IF EXISTS idx_otel_spans_kind;
DROP INDEX IF EXISTS idx_otel_spans_status;
DROP INDEX IF EXISTS idx_otel_spans_name;

-- 4) embedding_queues 清理查询支撑：completed 部分索引（job_log_cleanup 30 天保留策略用）
CREATE INDEX IF NOT EXISTS idx_embedding_queues_completed_created
  ON embedding_queues (created_at) WHERE status = 'completed';
