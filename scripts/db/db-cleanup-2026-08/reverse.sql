-- analysis-remediation W1 (task 2.7)：回滚参考（备查，不自动执行）。
-- 数据恢复用 backup.sh 的 pg_dump 文件；本文件只列结构反向操作。

-- FK 反向：
-- ALTER TABLE topic_tag_embeddings DROP CONSTRAINT IF EXISTS fk_topic_tag_embeddings_tag;

-- articles 全文搜索重建（分钟级）：
-- CREATE INDEX idx_articles_search_vector ON articles USING gin (search_vector);
-- CREATE TRIGGER articles_search_vector_trigger BEFORE INSERT OR UPDATE OF title, description
--   ON articles FOR EACH ROW EXECUTE FUNCTION articles_search_vector_update();
-- （如 trigger 函数也被删需先重建 articles_search_vector_update()；本轮未删函数）

-- otel_spans 索引重建：
-- CREATE INDEX idx_otel_spans_trace_id ON otel_spans (trace_id);
-- CREATE INDEX idx_otel_spans_kind ON otel_spans (kind);
-- CREATE INDEX idx_otel_spans_status ON otel_spans (status_code);
-- CREATE INDEX idx_otel_spans_name ON otel_spans (name);

-- embedding_queues 部分索引反向：
-- DROP INDEX IF EXISTS idx_embedding_queues_completed_created;
